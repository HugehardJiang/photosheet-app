package renderer

import (
	"compress/zlib"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"sync"

	"photosheet/internal/model"
)

// Canvas16 is the high-bit-depth sibling of the legacy NRGBA canvas. Keeping
// the interfaces separate preserves the existing 8-bit JPEG workflow while
// making it impossible for a RAW render to accidentally pass through uint8.
type Canvas16 interface {
	Width() int
	Height() int
	DrawImage16(x, y int, src *image.NRGBA64, padding int, paddingMode model.PaddingMode) error
	Row16(y int, dst []byte) error
	Flush() error
	Close() error
}

func NewCanvas16(width, height int, memoryLimit int64, tileSize int, tempRoot string) (Canvas16, error) {
	return NewCanvas16WithStorage(width, height, memoryLimit, tileSize, tempRoot, model.StorageBalanced)
}

func NewCanvas16WithStorage(width, height int, memoryLimit int64, tileSize int, tempRoot string, storageMode model.StorageMode) (Canvas16, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("16-bit 画布尺寸必须大于 0")
	}
	if memoryLimit <= 0 {
		memoryLimit = defaultMemoryLimit
	}
	if tileSize <= 0 {
		tileSize = 512
	}
	bytes := int64(width) * int64(height) * 8
	if bytes <= memoryLimit {
		return &memoryCanvas16{img: image.NewNRGBA64(image.Rect(0, 0, width, height))}, nil
	}
	return newTiledCanvas16(width, height, tileSize, memoryLimit, tempRoot, storageMode)
}

type memoryCanvas16 struct{ img *image.NRGBA64 }

func (c *memoryCanvas16) Width() int  { return c.img.Bounds().Dx() }
func (c *memoryCanvas16) Height() int { return c.img.Bounds().Dy() }
func (c *memoryCanvas16) DrawImage16(x, y int, src *image.NRGBA64, padding int, mode model.PaddingMode) error {
	return drawToCanvas16(c, x, y, src, padding, mode)
}
func (c *memoryCanvas16) Row16(y int, dst []byte) error {
	if y < 0 || y >= c.Height() || len(dst) < c.Width()*8 {
		return errors.New("读取 16-bit 画布行参数无效")
	}
	start := y * c.img.Stride
	copy(dst[:c.Width()*8], c.img.Pix[start:start+c.Width()*8])
	return nil
}
func (c *memoryCanvas16) Flush() error { return nil }
func (c *memoryCanvas16) Close() error { c.img = nil; return nil }
func (c *memoryCanvas16) set16(x, y int, value color.NRGBA64) error {
	if x < 0 || y < 0 || x >= c.Width() || y >= c.Height() {
		return fmt.Errorf("像素坐标越界: (%d,%d)", x, y)
	}
	c.img.SetNRGBA64(x, y, value)
	return nil
}

type tile16 struct {
	key      tileKey
	data     []byte
	dirty    bool
	lastUsed uint64
}
type tiledCanvas16 struct {
	width, height, tileSize, tileBytes, maxTiles int
	compressionLevel                             int
	tempDir                                      string
	cache                                        map[tileKey]*tile16
	clock                                        uint64
	mu                                           sync.Mutex
	closed                                       bool
}

func newTiledCanvas16(width, height, tileSize int, memoryLimit int64, tempRoot string, storageMode model.StorageMode) (*tiledCanvas16, error) {
	if tempRoot == "" {
		tempRoot = canvasTempRoot()
	}
	if err := os.MkdirAll(tempRoot, 0o700); err != nil {
		return nil, fmt.Errorf("创建 PhotoSheet temp 目录失败: %w", err)
	}
	cleanupStaleCanvasTemp(tempRoot)
	tempDir, err := os.MkdirTemp(tempRoot, "job-")
	if err != nil {
		return nil, fmt.Errorf("创建 16-bit 磁盘画布目录失败: %w", err)
	}
	tileBytes := tileSize * tileSize * 8
	maxTiles := int(memoryLimit / int64(tileBytes))
	if maxTiles < 4 {
		maxTiles = 4
	}
	if maxTiles > 64 {
		maxTiles = 64
	}
	return &tiledCanvas16{width: width, height: height, tileSize: tileSize, tileBytes: tileBytes, maxTiles: maxTiles, compressionLevel: tileCompressionLevel(storageMode), tempDir: tempDir, cache: make(map[tileKey]*tile16)}, nil
}

func tileCompressionLevel(storageMode model.StorageMode) int {
	switch storageMode {
	case model.StorageLowDisk:
		return zlib.BestCompression
	case model.StoragePerformance:
		return zlib.BestSpeed
	default:
		return zlib.DefaultCompression
	}
}
func (c *tiledCanvas16) Width() int  { return c.width }
func (c *tiledCanvas16) Height() int { return c.height }
func (c *tiledCanvas16) DrawImage16(x, y int, src *image.NRGBA64, padding int, mode model.PaddingMode) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("画布已关闭")
	}
	return drawToCanvas16Locked(c, x, y, src, padding, mode)
}
func (c *tiledCanvas16) Row16(y int, dst []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || y < 0 || y >= c.height || len(dst) < c.width*8 {
		return errors.New("读取磁盘 16-bit 画布行参数无效")
	}
	for i := 0; i < c.width*8; i++ {
		dst[i] = 0
	}
	remaining, x := c.width, 0
	for remaining > 0 {
		tileX, localX := x/c.tileSize, x%c.tileSize
		width := c.tileSize - localX
		if width > remaining {
			width = remaining
		}
		current, err := c.load16(tileKey{x: tileX, y: y / c.tileSize})
		if err != nil {
			return err
		}
		localY := y % c.tileSize
		srcStart := (localY*c.tileSize + localX) * 8
		dstStart := x * 8
		copy(dst[dstStart:dstStart+width*8], current.data[srcStart:srcStart+width*8])
		x += width
		remaining -= width
	}
	return nil
}
func (c *tiledCanvas16) Flush() error { c.mu.Lock(); defer c.mu.Unlock(); return c.flushAll16() }
func (c *tiledCanvas16) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	err := c.flushAll16()
	removeErr := os.RemoveAll(c.tempDir)
	c.closed = true
	if err != nil {
		return err
	}
	return removeErr
}
func (c *tiledCanvas16) set16(x, y int, value color.NRGBA64) error {
	if x < 0 || y < 0 || x >= c.width || y >= c.height {
		return fmt.Errorf("像素坐标越界: (%d,%d)", x, y)
	}
	current, err := c.load16(tileKey{x: x / c.tileSize, y: y / c.tileSize})
	if err != nil {
		return err
	}
	offset := ((y%c.tileSize)*c.tileSize + x%c.tileSize) * 8
	current.data[offset], current.data[offset+1] = byte(value.R>>8), byte(value.R)
	current.data[offset+2], current.data[offset+3] = byte(value.G>>8), byte(value.G)
	current.data[offset+4], current.data[offset+5] = byte(value.B>>8), byte(value.B)
	current.data[offset+6], current.data[offset+7] = byte(value.A>>8), byte(value.A)
	current.dirty = true
	return nil
}
func (c *tiledCanvas16) load16(key tileKey) (*tile16, error) {
	if current, ok := c.cache[key]; ok {
		c.clock++
		current.lastUsed = c.clock
		return current, nil
	}
	if len(c.cache) >= c.maxTiles {
		if err := c.evictOne16(); err != nil {
			return nil, err
		}
	}
	data := make([]byte, c.tileBytes)
	filename := c.tileFilename16(key)
	f, err := os.Open(filename)
	if err == nil {
		reader, readErr := zlib.NewReader(f)
		if readErr != nil {
			_ = f.Close()
			return nil, fmt.Errorf("读取磁盘 16-bit tile 压缩流失败: %w", readErr)
		}
		_, readErr = io.ReadFull(reader, data)
		closeReaderErr := reader.Close()
		closeFileErr := f.Close()
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return nil, fmt.Errorf("读取磁盘 16-bit tile 失败: %w", readErr)
		}
		if closeReaderErr != nil {
			return nil, closeReaderErr
		}
		if closeFileErr != nil {
			return nil, closeFileErr
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("打开磁盘 16-bit tile 失败: %w", err)
	}
	c.clock++
	current := &tile16{key: key, data: data, lastUsed: c.clock}
	c.cache[key] = current
	return current, nil
}
func (c *tiledCanvas16) evictOne16() error {
	var oldest *tile16
	for _, current := range c.cache {
		if oldest == nil || current.lastUsed < oldest.lastUsed {
			oldest = current
		}
	}
	if oldest == nil {
		return nil
	}
	if err := c.flushTile16(oldest); err != nil {
		return err
	}
	delete(c.cache, oldest.key)
	return nil
}
func (c *tiledCanvas16) flushAll16() error {
	for _, current := range c.cache {
		if err := c.flushTile16(current); err != nil {
			return err
		}
	}
	return nil
}
func (c *tiledCanvas16) flushTile16(current *tile16) error {
	if !current.dirty {
		return nil
	}
	f, err := os.OpenFile(c.tileFilename16(current.key), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	writer, err := zlib.NewWriterLevel(f, c.compressionLevel)
	if err != nil {
		_ = f.Close()
		return err
	}
	if _, err := writer.Write(current.data); err != nil {
		_ = writer.Close()
		_ = f.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	current.dirty = false
	return nil
}
func (c *tiledCanvas16) tileFilename16(key tileKey) string {
	return filepath.Join(c.tempDir, fmt.Sprintf("tile-%05d-%05d.bin.z", key.y, key.x))
}

func drawToCanvas16(c Canvas16, x, y int, src *image.NRGBA64, padding int, mode model.PaddingMode) error {
	if locked, ok := c.(*tiledCanvas16); ok {
		locked.mu.Lock()
		defer locked.mu.Unlock()
		if locked.closed {
			return errors.New("画布已关闭")
		}
		return drawToCanvas16Locked(locked, x, y, src, padding, mode)
	}
	return drawToCanvas16Locked(c.(*memoryCanvas16), x, y, src, padding, mode)
}

type pixelCanvas16 interface {
	Width() int
	Height() int
	set16(x, y int, value color.NRGBA64) error
}

func drawToCanvas16Locked(c pixelCanvas16, x, y int, src *image.NRGBA64, padding int, mode model.PaddingMode) error {
	if src == nil {
		return errors.New("不能绘制空 16-bit 图片")
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
			if !inside && mode == model.PaddingSolid {
				continue
			}
			sx, sy := px-padding, py-padding
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
			if err := c.set16(x+px, y+py, src.NRGBA64At(sx, sy)); err != nil {
				return err
			}
		}
	}
	return nil
}
