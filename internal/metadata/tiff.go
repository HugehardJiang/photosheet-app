package metadata

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// ReadTIFFICProfile reads the standard TIFF ICCProfile tag without decoding
// the image. It is intentionally small and bounded because the full TIFF
// decoder is not needed during Scanner/Resolver metadata inspection.
func ReadTIFFICProfile(filename string) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var header [8]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return nil, err
	}
	var order binary.ByteOrder
	switch string(header[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil, errors.New("文件不是有效 TIFF")
	}
	if order.Uint16(header[2:4]) != 42 {
		return nil, errors.New("TIFF magic 无效")
	}
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := stat.Size()
	offset := int64(order.Uint32(header[4:8]))
	if offset < 8 || offset >= size {
		return nil, errors.New("TIFF IFD 偏移越界")
	}
	var countBytes [2]byte
	if _, err := f.ReadAt(countBytes[:], offset); err != nil {
		return nil, err
	}
	count := int(order.Uint16(countBytes[:]))
	if count > 4096 || offset+2+int64(count)*12 > size {
		return nil, errors.New("TIFF IFD 无效")
	}
	var entry [12]byte
	for i := 0; i < count; i++ {
		if _, err := f.ReadAt(entry[:], offset+2+int64(i)*12); err != nil {
			return nil, err
		}
		if order.Uint16(entry[0:2]) != 34675 {
			continue
		}
		typ := order.Uint16(entry[2:4])
		items := uint64(order.Uint32(entry[4:8]))
		if typ != 7 || items == 0 || items > 128<<20 {
			return nil, errors.New("TIFF ICC Profile tag 无效")
		}
		dataOffset := int64(order.Uint32(entry[8:12]))
		if items <= 4 {
			return append([]byte(nil), entry[8:8+items]...), nil
		}
		if dataOffset < 0 || dataOffset+int64(items) > size {
			return nil, errors.New("TIFF ICC Profile 越界")
		}
		profile := make([]byte, int(items))
		if _, err := f.ReadAt(profile, dataOffset); err != nil {
			return nil, fmt.Errorf("读取 TIFF ICC Profile 失败: %w", err)
		}
		return profile, nil
	}
	return nil, nil
}

func ReadTIFFCompression(filename string) (uint16, error) {
	f, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	var header [8]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return 0, err
	}
	var order binary.ByteOrder
	switch string(header[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0, errors.New("文件不是有效 TIFF")
	}
	if order.Uint16(header[2:4]) != 42 {
		return 0, errors.New("TIFF magic 无效")
	}
	stat, err := f.Stat()
	if err != nil {
		return 0, err
	}
	offset := int64(order.Uint32(header[4:8]))
	if offset < 8 || offset >= stat.Size() {
		return 0, errors.New("TIFF IFD 偏移越界")
	}
	var countBytes [2]byte
	if _, err := f.ReadAt(countBytes[:], offset); err != nil {
		return 0, err
	}
	count := int(order.Uint16(countBytes[:]))
	var entry [12]byte
	for i := 0; i < count; i++ {
		if _, err := f.ReadAt(entry[:], offset+2+int64(i)*12); err != nil {
			return 0, err
		}
		if order.Uint16(entry[0:2]) == 259 {
			if order.Uint16(entry[2:4]) != 3 {
				return 0, errors.New("TIFF Compression 类型无效")
			}
			return order.Uint16(entry[8:10]), nil
		}
	}
	return 0, nil
}
