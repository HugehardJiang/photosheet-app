package renderer

import (
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"

	"golang.org/x/image/tiff"
	"photosheet/internal/metadata"
)

func WritePNGAtomic(filename string, canvas Canvas, iccProfile []byte) error {
	if canvas == nil {
		return errors.New("不能写入空画布")
	}
	if err := canvas.Flush(); err != nil {
		return fmt.Errorf("刷新画布失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return fmt.Errorf("创建 Sheet 目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(filename), ".sheet-*.png")
	if err != nil {
		return fmt.Errorf("创建 PNG 临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	if err := encodePNG(tmp, canvas, iccProfile); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("同步 PNG 文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭 PNG 临时文件失败: %w", err)
	}
	if err := os.Rename(tmpName, filename); err != nil {
		return fmt.Errorf("原子替换 PNG Sheet 失败: %w", err)
	}
	keep = true
	return nil
}

func encodePNG(w interface {
	Write([]byte) (int, error)
}, canvas Canvas, iccProfile []byte) error {
	writer := &rawWriter{w: w}
	if _, err := writer.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10}); err != nil {
		return fmt.Errorf("写入 PNG 签名失败: %w", err)
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(canvas.Width()))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(canvas.Height()))
	ihdr[8] = 8 // bit depth
	ihdr[9] = 6 // color type RGBA
	if err := writeChunk(writer, "IHDR", ihdr); err != nil {
		return err
	}
	if len(iccProfile) > 0 {
		if err := writeICCChunk(writer, iccProfile); err != nil {
			return err
		}
	}
	idat := &idatWriter{w: writer, buffer: make([]byte, 0, 256*1024)}
	zlibWriter := zlib.NewWriter(idat)
	row := make([]byte, 1+canvas.Width()*4)
	for y := 0; y < canvas.Height(); y++ {
		row[0] = 0 // filter None; it is slower than adaptive filters but exact and simple.
		if err := canvas.Row(y, row[1:]); err != nil {
			_ = zlibWriter.Close()
			return fmt.Errorf("读取 PNG 第 %d 行失败: %w", y, err)
		}
		if _, err := zlibWriter.Write(row); err != nil {
			_ = zlibWriter.Close()
			return fmt.Errorf("压缩 PNG 第 %d 行失败: %w", y, err)
		}
	}
	if err := zlibWriter.Close(); err != nil {
		return fmt.Errorf("关闭 PNG 压缩流失败: %w", err)
	}
	if err := idat.Flush(); err != nil {
		return fmt.Errorf("写入 PNG IDAT 失败: %w", err)
	}
	if err := writeChunk(writer, "IEND", nil); err != nil {
		return err
	}
	return nil
}

type rawWriter struct {
	w interface {
		Write([]byte) (int, error)
	}
}

func (w *rawWriter) Write(p []byte) (int, error) { return w.w.Write(p) }

type idatWriter struct {
	w      *rawWriter
	buffer []byte
}

func (w *idatWriter) Write(p []byte) (int, error) {
	total := 0
	for len(p) > 0 {
		space := 256*1024 - len(w.buffer)
		if space > len(p) {
			space = len(p)
		}
		w.buffer = append(w.buffer, p[:space]...)
		total += space
		p = p[space:]
		if len(w.buffer) == 256*1024 {
			if err := w.Flush(); err != nil {
				return total, err
			}
		}
	}
	return total, nil
}

func (w *idatWriter) Flush() error {
	if len(w.buffer) == 0 {
		return nil
	}
	if err := writeChunk(w.w, "IDAT", w.buffer); err != nil {
		return err
	}
	w.buffer = w.buffer[:0]
	return nil
}

func writeICCChunk(w *rawWriter, profile []byte) error {
	compressed := make([]byte, 0, len(profile))
	buffer := &byteBuffer{data: &compressed}
	zw := zlib.NewWriter(buffer)
	if _, err := zw.Write(profile); err != nil {
		return fmt.Errorf("压缩 ICC Profile 失败: %w", err)
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("关闭 ICC Profile 压缩流失败: %w", err)
	}
	payload := make([]byte, 0, len(compressed)+16)
	payload = append(payload, []byte("ICC Profile")...)
	payload = append(payload, 0, 0)
	payload = append(payload, compressed...)
	return writeChunk(w, "iCCP", payload)
}

type byteBuffer struct{ data *[]byte }

func (b *byteBuffer) Write(p []byte) (int, error) {
	*b.data = append(*b.data, p...)
	return len(p), nil
}

func writeChunk(w *rawWriter, kind string, data []byte) error {
	if len(kind) != 4 {
		return errors.New("PNG chunk 类型必须为 4 字节")
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data)))
	if _, err := w.Write(length[:]); err != nil {
		return err
	}
	if _, err := w.Write([]byte(kind)); err != nil {
		return err
	}
	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte(kind))
	_, _ = crc.Write(data)
	var checksum [4]byte
	binary.BigEndian.PutUint32(checksum[:], crc.Sum32())
	_, err := w.Write(checksum[:])
	return err
}

func WriteJPEGAtomic(filename string, canvas Canvas, quality int) error {
	if quality < 1 || quality > 100 {
		return errors.New("JPEG 质量必须在 1 到 100 之间")
	}
	img := image.NewNRGBA(image.Rect(0, 0, canvas.Width(), canvas.Height()))
	row := make([]byte, canvas.Width()*4)
	for y := 0; y < canvas.Height(); y++ {
		if err := canvas.Row(y, row); err != nil {
			return fmt.Errorf("读取 JPEG 第 %d 行失败: %w", y, err)
		}
		copy(img.Pix[y*img.Stride:y*img.Stride+len(row)], row)
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(filename), ".sheet-*.jpg")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	if err := jpeg.Encode(tmp, img, &jpeg.Options{Quality: quality}); err != nil {
		return fmt.Errorf("JPEG 重新编码失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, filename); err != nil {
		return err
	}
	keep = true
	return nil
}

// WriteTIFFAtomic is used by extraction when the user explicitly requests a
// lossless TIFF result. The processed-Sheet resolver and the output writer are
// separate so adding a future tiled TIFF encoder does not change PNG/JPEG
// behavior.
func WriteTIFFAtomic(filename string, img image.Image) error {
	if img == nil {
		return errors.New("不能写入空 TIFF 图片")
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return fmt.Errorf("创建 TIFF 输出目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(filename), ".restored-*.tif")
	if err != nil {
		return fmt.Errorf("创建 TIFF 临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tiff.Encode(tmp, img, &tiff.Options{Compression: tiff.Deflate}); err != nil {
		return fmt.Errorf("TIFF 编码失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("同步 TIFF 文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭 TIFF 临时文件失败: %w", err)
	}
	if err := os.Rename(tmpName, filename); err != nil {
		return fmt.Errorf("原子替换 TIFF 文件失败: %w", err)
	}
	keep = true
	return nil
}

func ICCHash(profile []byte) string { return metadata.ICCProfileHash(profile) }
