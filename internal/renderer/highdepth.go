package renderer

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"

	"photosheet/internal/model"
)

func WritePNG16Atomic(filename string, canvas Canvas16, iccProfile []byte) error {
	if canvas == nil {
		return errors.New("不能写入空 16-bit 画布")
	}
	if err := canvas.Flush(); err != nil {
		return fmt.Errorf("刷新 16-bit 画布失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(filename), ".sheet-*.png")
	if err != nil {
		return fmt.Errorf("创建 PNG16 临时文件失败: %w", err)
	}
	tmpName, keep := tmp.Name(), false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	if err := encodePNG16(tmp, canvas, iccProfile); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, filename); err != nil {
		return fmt.Errorf("原子替换 PNG16 Sheet 失败: %w", err)
	}
	keep = true
	return nil
}

func encodePNG16(w interface{ Write([]byte) (int, error) }, canvas Canvas16, iccProfile []byte) error {
	writer := &rawWriter{w: w}
	if _, err := writer.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10}); err != nil {
		return err
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(canvas.Width()))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(canvas.Height()))
	ihdr[8], ihdr[9] = 16, 6
	if err := writeChunk(writer, "IHDR", ihdr); err != nil {
		return err
	}
	if len(iccProfile) > 0 {
		if err := writeICCChunk(writer, iccProfile); err != nil {
			return err
		}
	}
	idat := &idatWriter{w: writer, buffer: make([]byte, 0, 256*1024)}
	compressed := zlib.NewWriter(idat)
	row := make([]byte, 1+canvas.Width()*8)
	for y := 0; y < canvas.Height(); y++ {
		row[0] = 0
		if err := canvas.Row16(y, row[1:]); err != nil {
			_ = compressed.Close()
			return fmt.Errorf("读取 PNG16 第 %d 行失败: %w", y, err)
		}
		if _, err := compressed.Write(row); err != nil {
			_ = compressed.Close()
			return err
		}
	}
	if err := compressed.Close(); err != nil {
		return err
	}
	if err := idat.Flush(); err != nil {
		return err
	}
	return writeChunk(writer, "IEND", nil)
}

func WriteTIFF16CanvasAtomic(filename string, canvas Canvas16, compression model.TIFFCompression, iccProfile []byte) error {
	if canvas == nil {
		return errors.New("不能写入空 16-bit TIFF 画布")
	}
	if compression == model.TIFFLZW {
		return errors.New("当前 TIFF backend 未启用 LZW；请使用 ZIP/Deflate 或无压缩")
	}
	if compression != model.TIFFZIP && compression != model.TIFFUncompressed {
		return fmt.Errorf("不支持的 TIFF 压缩方式 %q", compression)
	}
	if err := canvas.Flush(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(filename), ".sheet-*.tif")
	if err != nil {
		return err
	}
	tmpName, keep := tmp.Name(), false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	if err := encodeTIFF16(tmp, canvas, compression, iccProfile); err != nil {
		return err
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

func WriteTIFF16Atomic(filename string, img *image.NRGBA64, compression model.TIFFCompression, iccProfile []byte) error {
	if img == nil {
		return errors.New("不能写入空 16-bit TIFF 图片")
	}
	return WriteTIFF16AtomicWithOptions(filename, img, compression, iccProfile, defaultMemoryLimit, 512, "")
}

func WriteTIFF16AtomicWithOptions(filename string, img *image.NRGBA64, compression model.TIFFCompression, iccProfile []byte, memoryLimit int64, tileSize int, tempRoot string) error {
	return WriteTIFF16AtomicWithStorageOptions(filename, img, compression, iccProfile, memoryLimit, tileSize, tempRoot, model.StorageBalanced)
}

func WriteTIFF16AtomicWithStorageOptions(filename string, img *image.NRGBA64, compression model.TIFFCompression, iccProfile []byte, memoryLimit int64, tileSize int, tempRoot string, storageMode model.StorageMode) error {
	if img == nil {
		return errors.New("不能写入空 16-bit TIFF 图片")
	}
	canvas, err := NewCanvas16WithStorage(img.Bounds().Dx(), img.Bounds().Dy(), memoryLimit, tileSize, tempRoot, storageMode)
	if err != nil {
		return err
	}
	defer canvas.Close()
	if err := canvas.DrawImage16(0, 0, img, 0, model.PaddingSolid); err != nil {
		return err
	}
	return WriteTIFF16CanvasAtomic(filename, canvas, compression, iccProfile)
}

type tiffEntry16 struct {
	tag, typ     uint16
	count, value uint32
}

func encodeTIFF16(f *os.File, canvas Canvas16, compression model.TIFFCompression, profile []byte) error {
	width, height := canvas.Width(), canvas.Height()
	rowsPerStrip := 64
	if height < rowsPerStrip {
		rowsPerStrip = height
	}
	stripCount := (height + rowsPerStrip - 1) / rowsPerStrip
	entries := []tiffEntry16{
		{tag: 256, typ: 4, count: 1, value: uint32(width)}, {tag: 257, typ: 4, count: 1, value: uint32(height)},
		{tag: 258, typ: 3, count: 3}, {tag: 259, typ: 3, count: 1, value: uint32(1)}, {tag: 262, typ: 3, count: 1, value: uint32(2)},
		{tag: 273, typ: 4, count: uint32(stripCount)}, {tag: 277, typ: 3, count: 1, value: uint32(3)}, {tag: 278, typ: 4, count: 1, value: uint32(rowsPerStrip)},
		{tag: 279, typ: 4, count: uint32(stripCount)}, {tag: 284, typ: 3, count: 1, value: uint32(1)},
	}
	if compression == model.TIFFZIP {
		entries[3].value = 8
	}
	if len(profile) > 0 {
		entries = append(entries, tiffEntry16{tag: 34675, typ: 7, count: uint32(len(profile))})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].tag < entries[j].tag })
	const ifdOffset = uint32(8)
	extraOffset := ifdOffset + 2 + uint32(len(entries))*12 + 4
	extra := make([]byte, 0, 32+stripCount*8+len(profile))
	addExtra := func(data []byte, align int) uint32 {
		for len(extra)%align != 0 {
			extra = append(extra, 0)
		}
		offset := extraOffset + uint32(len(extra))
		extra = append(extra, data...)
		return offset
	}
	bits := make([]byte, 6)
	binary.LittleEndian.PutUint16(bits[0:2], 16)
	binary.LittleEndian.PutUint16(bits[2:4], 16)
	binary.LittleEndian.PutUint16(bits[4:6], 16)
	bitsOffset := addExtra(bits, 2)
	stripOffsetsOffset := uint32(0)
	stripByteCountsOffset := uint32(0)
	profileOffset := uint32(0)
	if stripCount > 1 {
		stripOffsetsOffset = addExtra(make([]byte, stripCount*4), 4)
		stripByteCountsOffset = addExtra(make([]byte, stripCount*4), 4)
	}
	if len(profile) > 0 {
		profileOffset = addExtra(profile, 2)
	}
	for i := range entries {
		switch entries[i].tag {
		case 258:
			entries[i].value = bitsOffset
		case 273:
			if stripCount > 1 {
				entries[i].value = stripOffsetsOffset
			}
		case 279:
			if stripCount > 1 {
				entries[i].value = stripByteCountsOffset
			}
		case 34675:
			entries[i].value = profileOffset
		}
	}
	var header [8]byte
	copy(header[:2], []byte{'I', 'I'})
	binary.LittleEndian.PutUint16(header[2:4], 42)
	binary.LittleEndian.PutUint32(header[4:8], ifdOffset)
	if _, err := f.Write(header[:]); err != nil {
		return err
	}
	var count [2]byte
	binary.LittleEndian.PutUint16(count[:], uint16(len(entries)))
	if _, err := f.Write(count[:]); err != nil {
		return err
	}
	for _, entry := range entries {
		var raw [12]byte
		binary.LittleEndian.PutUint16(raw[0:2], entry.tag)
		binary.LittleEndian.PutUint16(raw[2:4], entry.typ)
		binary.LittleEndian.PutUint32(raw[4:8], entry.count)
		if entry.typ == 3 && entry.count == 1 {
			binary.LittleEndian.PutUint16(raw[8:10], uint16(entry.value))
		} else {
			binary.LittleEndian.PutUint32(raw[8:12], entry.value)
		}
		if _, err := f.Write(raw[:]); err != nil {
			return err
		}
	}
	var next [4]byte
	if _, err := f.Write(next[:]); err != nil {
		return err
	}
	if _, err := f.Write(extra); err != nil {
		return err
	}
	rowRGBA := make([]byte, width*8)
	rowRGB := make([]byte, width*6)
	offsets := make([]uint32, stripCount)
	counts := make([]uint32, stripCount)
	for strip := 0; strip < stripCount; strip++ {
		startY, endY := strip*rowsPerStrip, (strip+1)*rowsPerStrip
		if endY > height {
			endY = height
		}
		rawStrip := bytes.NewBuffer(make([]byte, 0, (endY-startY)*width*6))
		for y := startY; y < endY; y++ {
			if err := canvas.Row16(y, rowRGBA); err != nil {
				return err
			}
			for x := 0; x < width; x++ {
				for channel := 0; channel < 3; channel++ {
					value := binary.BigEndian.Uint16(rowRGBA[x*8+channel*2 : x*8+channel*2+2])
					binary.LittleEndian.PutUint16(rowRGB[x*6+channel*2:x*6+channel*2+2], value)
				}
			}
			_, _ = rawStrip.Write(rowRGB)
		}
		data := rawStrip.Bytes()
		if compression == model.TIFFZIP {
			var compressed bytes.Buffer
			writer := zlib.NewWriter(&compressed)
			if _, err := writer.Write(data); err != nil {
				return err
			}
			if err := writer.Close(); err != nil {
				return err
			}
			data = compressed.Bytes()
		}
		position, err := f.Seek(0, 2)
		if err != nil {
			return err
		}
		offsets[strip] = uint32(position)
		if _, err := f.Write(data); err != nil {
			return err
		}
		counts[strip] = uint32(len(data))
	}
	if stripCount > 1 {
		for i, value := range offsets {
			var raw [4]byte
			binary.LittleEndian.PutUint32(raw[:], value)
			if _, err := f.WriteAt(raw[:], int64(stripOffsetsOffset)+int64(i*4)); err != nil {
				return err
			}
		}
		for i, value := range counts {
			var raw [4]byte
			binary.LittleEndian.PutUint32(raw[:], value)
			if _, err := f.WriteAt(raw[:], int64(stripByteCountsOffset)+int64(i*4)); err != nil {
				return err
			}
		}
	} else {
		for i, entry := range entries {
			if entry.tag != 273 && entry.tag != 279 {
				continue
			}
			value := offsets[0]
			if entry.tag == 279 {
				value = counts[0]
			}
			var raw [4]byte
			binary.LittleEndian.PutUint32(raw[:], value)
			if _, err := f.WriteAt(raw[:], int64(ifdOffset)+2+int64(i*12)+8); err != nil {
				return err
			}
		}
	}
	return nil
}
