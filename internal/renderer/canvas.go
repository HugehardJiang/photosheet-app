package renderer

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"photosheet/internal/model"
)

const defaultMemoryLimit = int64(512 << 20)

type Canvas interface {
	Width() int
	Height() int
	DrawImage(x, y int, src *image.NRGBA, padding int, paddingMode model.PaddingMode) error
	Row(y int, dst []byte) error
	Flush() error
	Close() error
}

func NewCanvas(width, height int, memoryLimit int64, tileSize int, tempRoot string) (Canvas, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("画布尺寸必须大于 0")
	}
	if memoryLimit <= 0 {
		memoryLimit = defaultMemoryLimit
	}
	if tileSize <= 0 {
		tileSize = 512
	}
	bytes := int64(width) * int64(height) * 4
	if bytes <= memoryLimit {
		return &memoryCanvas{img: image.NewNRGBA(image.Rect(0, 0, width, height))}, nil
	}
	return newTiledCanvas(width, height, tileSize, memoryLimit, tempRoot)
}

type memoryCanvas struct {
	img *image.NRGBA
}

func (c *memoryCanvas) Width() int  { return c.img.Bounds().Dx() }
func (c *memoryCanvas) Height() int { return c.img.Bounds().Dy() }

func (c *memoryCanvas) DrawImage(x, y int, src *image.NRGBA, padding int, paddingMode model.PaddingMode) error {
	return drawToCanvas(c, x, y, src, padding, paddingMode)
}

func (c *memoryCanvas) Row(y int, dst []byte) error {
	if y < 0 || y >= c.Height() || len(dst) < c.Width()*4 {
		return errors.New("读取画布行参数无效")
	}
	start := y * c.img.Stride
	copy(dst[:c.Width()*4], c.img.Pix[start:start+c.Width()*4])
	return nil
}

func (c *memoryCanvas) Flush() error { return nil }
func (c *memoryCanvas) Close() error {
	c.img = nil
	return nil
}

func (c *memoryCanvas) set(x, y int, value color.NRGBA) error {
	if x < 0 || y < 0 || x >= c.Width() || y >= c.Height() {
		return fmt.Errorf("像素坐标越界: (%d,%d)", x, y)
	}
	c.img.SetNRGBA(x, y, value)
	return nil
}

type tileKey struct{ x, y int }

type tile struct {
	key      tileKey
	data     []byte
	dirty    bool
	lastUsed uint64
}

type tiledCanvas struct {
	width, height int
	tileSize      int
	tileBytes     int
	maxTiles      int
	tempDir       string
	cache         map[tileKey]*tile
	clock         uint64
	mu            sync.Mutex
	closed        bool
}

func newTiledCanvas(width, height, tileSize int, memoryLimit int64, tempRoot string) (*tiledCanvas, error) {
	if tempRoot == "" {
		tempRoot = canvasTempRoot()
	}
	if err := os.MkdirAll(tempRoot, 0o700); err != nil {
		return nil, fmt.Errorf("创建 PhotoSheet temp 目录失败: %w", err)
	}
	cleanupStaleCanvasTemp(tempRoot)
	tempDir, err := os.MkdirTemp(tempRoot, "photosheet-canvas-*")
	if err != nil {
		return nil, fmt.Errorf("创建磁盘画布目录失败: %w", err)
	}
	tileBytes := tileSize * tileSize * 4
	maxTiles := int(memoryLimit / int64(tileBytes))
	if maxTiles < 4 {
		maxTiles = 4
	}
	if maxTiles > 64 {
		maxTiles = 64
	}
	return &tiledCanvas{
		width: width, height: height, tileSize: tileSize,
		tileBytes: tileBytes, maxTiles: maxTiles,
		tempDir: tempDir, cache: make(map[tileKey]*tile),
	}, nil
}

func canvasTempRoot() string {
	if cache, err := os.UserCacheDir(); err == nil && cache != "" {
		return filepath.Join(cache, "PhotoSheet", "temp")
	}
	return filepath.Join(os.TempDir(), "PhotoSheet", "temp")
}

func cleanupStaleCanvasTemp(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-48 * time.Hour)
	for _, entry := range entries {
		if !entry.IsDir() || len(entry.Name()) < 4 || entry.Name()[:4] != "job-" {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, entry.Name()))
	}
}

func (c *tiledCanvas) Width() int  { return c.width }
func (c *tiledCanvas) Height() int { return c.height }

func (c *tiledCanvas) DrawImage(x, y int, src *image.NRGBA, padding int, paddingMode model.PaddingMode) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("画布已关闭")
	}
	return drawToCanvasLocked(c, x, y, src, padding, paddingMode)
}

func (c *tiledCanvas) Row(y int, dst []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || y < 0 || y >= c.height || len(dst) < c.width*4 {
		return errors.New("读取磁盘画布行参数无效")
	}
	for i := 0; i < c.width*4; i++ {
		dst[i] = 0
	}
	remaining := c.width
	x := 0
	for remaining > 0 {
		tileX := x / c.tileSize
		localX := x % c.tileSize
		width := c.tileSize - localX
		if width > remaining {
			width = remaining
		}
		current, err := c.load(tileKey{x: tileX, y: y / c.tileSize})
		if err != nil {
			return err
		}
		localY := y % c.tileSize
		srcStart := (localY*c.tileSize + localX) * 4
		dstStart := x * 4
		copy(dst[dstStart:dstStart+width*4], current.data[srcStart:srcStart+width*4])
		x += width
		remaining -= width
	}
	return nil
}

func (c *tiledCanvas) Flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.flushAll()
}

func (c *tiledCanvas) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	err := c.flushAll()
	removeErr := os.RemoveAll(c.tempDir)
	c.closed = true
	if err != nil {
		return err
	}
	return removeErr
}

func (c *tiledCanvas) set(x, y int, value color.NRGBA) error {
	if x < 0 || y < 0 || x >= c.width || y >= c.height {
		return fmt.Errorf("像素坐标越界: (%d,%d)", x, y)
	}
	current, err := c.load(tileKey{x: x / c.tileSize, y: y / c.tileSize})
	if err != nil {
		return err
	}
	localX, localY := x%c.tileSize, y%c.tileSize
	offset := (localY*c.tileSize + localX) * 4
	current.data[offset] = value.R
	current.data[offset+1] = value.G
	current.data[offset+2] = value.B
	current.data[offset+3] = value.A
	current.dirty = true
	return nil
}

func (c *tiledCanvas) load(key tileKey) (*tile, error) {
	if current, ok := c.cache[key]; ok {
		c.clock++
		current.lastUsed = c.clock
		return current, nil
	}
	if len(c.cache) >= c.maxTiles {
		if err := c.evictOne(); err != nil {
			return nil, err
		}
	}
	data := make([]byte, c.tileBytes)
	filename := c.tileFilename(key)
	f, err := os.Open(filename)
	if err == nil {
		_, readErr := io.ReadFull(f, data)
		closeErr := f.Close()
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return nil, fmt.Errorf("读取磁盘画布 tile 失败: %w", readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("关闭磁盘画布 tile 失败: %w", closeErr)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("打开磁盘画布 tile 失败: %w", err)
	}
	c.clock++
	current := &tile{key: key, data: data, lastUsed: c.clock}
	c.cache[key] = current
	return current, nil
}

func (c *tiledCanvas) evictOne() error {
	var oldest *tile
	for _, current := range c.cache {
		if oldest == nil || current.lastUsed < oldest.lastUsed {
			oldest = current
		}
	}
	if oldest == nil {
		return nil
	}
	if err := c.flushTile(oldest); err != nil {
		return err
	}
	delete(c.cache, oldest.key)
	return nil
}

func (c *tiledCanvas) flushAll() error {
	for _, current := range c.cache {
		if err := c.flushTile(current); err != nil {
			return err
		}
	}
	return nil
}

func (c *tiledCanvas) flushTile(current *tile) error {
	if !current.dirty {
		return nil
	}
	f, err := os.OpenFile(c.tileFilename(current.key), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("创建磁盘画布 tile 失败: %w", err)
	}
	if _, err := f.Write(current.data); err != nil {
		_ = f.Close()
		return fmt.Errorf("写入磁盘画布 tile 失败: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("关闭磁盘画布 tile 失败: %w", err)
	}
	current.dirty = false
	return nil
}

func (c *tiledCanvas) tileFilename(key tileKey) string {
	return filepath.Join(c.tempDir, fmt.Sprintf("tile-%05d-%05d.bin", key.y, key.x))
}

func drawToCanvas(c Canvas, x, y int, src *image.NRGBA, padding int, paddingMode model.PaddingMode) error {
	if locked, ok := c.(*tiledCanvas); ok {
		locked.mu.Lock()
		defer locked.mu.Unlock()
		if locked.closed {
			return errors.New("画布已关闭")
		}
		return drawToCanvasLocked(locked, x, y, src, padding, paddingMode)
	}
	return drawToCanvasLocked(c.(*memoryCanvas), x, y, src, padding, paddingMode)
}

type pixelCanvas interface {
	Width() int
	Height() int
	set(x, y int, value color.NRGBA) error
}

func drawToCanvasLocked(c pixelCanvas, x, y int, src *image.NRGBA, padding int, paddingMode model.PaddingMode) error {
	if src == nil {
		return errors.New("不能绘制空图片")
	}
	if padding < 0 {
		return errors.New("padding 不能为负数")
	}
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if x < 0 || y < 0 || x+sw+2*padding > c.Width() || y+sh+2*padding > c.Height() {
		return fmt.Errorf("照片绘制区域越界: (%d,%d,%d,%d)", x, y, sw+2*padding, sh+2*padding)
	}
	for py := 0; py < sh+2*padding; py++ {
		for px := 0; px < sw+2*padding; px++ {
			inside := px >= padding && px < padding+sw && py >= padding && py < padding+sh
			if !inside && paddingMode == model.PaddingSolid {
				continue
			}
			sx := px - padding
			sy := py - padding
			if sx < 0 {
				sx = 0
			}
			if sx >= sw {
				sx = sw - 1
			}
			if sy < 0 {
				sy = 0
			}
			if sy >= sh {
				sy = sh - 1
			}
			if err := c.set(x+px, y+py, src.NRGBAAt(sx, sy)); err != nil {
				return err
			}
		}
	}
	return nil
}
