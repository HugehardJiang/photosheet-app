package metadata

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

type JPEGMetadata struct {
	Orientation int
	ICCProfile  []byte
}

func ReadJPEGMetadata(filename string) (JPEGMetadata, error) {
	f, err := os.Open(filename)
	if err != nil {
		return JPEGMetadata{}, err
	}
	defer f.Close()
	return ReadJPEGMetadataReader(f)
}

func ReadJPEGMetadataReader(r io.Reader) (JPEGMetadata, error) {
	br := bufio.NewReaderSize(r, 32*1024)
	first := make([]byte, 2)
	if _, err := io.ReadFull(br, first); err != nil {
		return JPEGMetadata{}, fmt.Errorf("读取 JPEG 标记失败: %w", err)
	}
	if first[0] != 0xff || first[1] != 0xd8 {
		return JPEGMetadata{}, errors.New("文件不是有效的 JPEG SOI")
	}

	type iccPart struct {
		sequence byte
		total    byte
		data     []byte
	}
	parts := make(map[byte]iccPart)
	orientation := 1
	for {
		marker, err := nextMarker(br)
		if err != nil {
			return JPEGMetadata{}, fmt.Errorf("读取 JPEG marker 失败: %w", err)
		}
		if marker == 0xd8 || marker == 0xd9 {
			if marker == 0xd9 {
				break
			}
			continue
		}
		if marker == 0xda {
			// EXIF and ICC segments occur before SOS in normal JPEG files.
			break
		}
		if marker >= 0xd0 && marker <= 0xd7 || marker == 0x01 {
			continue
		}
		var lengthBytes [2]byte
		if _, err := io.ReadFull(br, lengthBytes[:]); err != nil {
			return JPEGMetadata{}, fmt.Errorf("读取 JPEG segment 长度失败: %w", err)
		}
		length := int(binary.BigEndian.Uint16(lengthBytes[:]))
		if length < 2 {
			return JPEGMetadata{}, errors.New("JPEG segment 长度无效")
		}
		payload := make([]byte, length-2)
		if _, err := io.ReadFull(br, payload); err != nil {
			return JPEGMetadata{}, fmt.Errorf("读取 JPEG segment 内容失败: %w", err)
		}
		switch marker {
		case 0xe1:
			if bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
				if parsed, err := parseEXIFOrientation(payload[6:]); err == nil && parsed >= 1 && parsed <= 8 {
					orientation = parsed
				}
			}
		case 0xe2:
			const prefix = "ICC_PROFILE\x00"
			if bytes.HasPrefix(payload, []byte(prefix)) && len(payload) >= len(prefix)+2 {
				sequence := payload[len(prefix)]
				total := payload[len(prefix)+1]
				if sequence > 0 && total > 0 && sequence <= total {
					parts[sequence] = iccPart{sequence: sequence, total: total, data: append([]byte(nil), payload[len(prefix)+2:]...)}
				}
			}
		}
	}

	var profile []byte
	if len(parts) > 0 {
		var total byte
		for _, part := range parts {
			if total == 0 {
				total = part.total
			}
			if part.total != total || len(parts) != int(total) {
				return JPEGMetadata{Orientation: orientation}, errors.New("ICC Profile 分段不完整")
			}
		}
		var ok bool = true
		for i := byte(1); i <= total; i++ {
			part, exists := parts[i]
			if !exists {
				ok = false
				break
			}
			profile = append(profile, part.data...)
		}
		if !ok {
			profile = nil
		}
	}
	return JPEGMetadata{Orientation: orientation, ICCProfile: profile}, nil
}

func nextMarker(r *bufio.Reader) (byte, error) {
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		if b != 0xff {
			continue
		}
		for {
			b, err = r.ReadByte()
			if err != nil {
				return 0, err
			}
			if b != 0xff {
				break
			}
		}
		if b == 0x00 {
			continue
		}
		return b, nil
	}
}

func parseEXIFOrientation(tiff []byte) (int, error) {
	if len(tiff) < 8 {
		return 0, errors.New("TIFF header 太短")
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0, errors.New("未知 TIFF 字节序")
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return 0, errors.New("TIFF magic 无效")
	}
	ifdOffset := uint64(order.Uint32(tiff[4:8]))
	if ifdOffset+2 > uint64(len(tiff)) {
		return 0, errors.New("IFD 偏移越界")
	}
	count := int(order.Uint16(tiff[ifdOffset : ifdOffset+2]))
	base := ifdOffset + 2
	for i := 0; i < count; i++ {
		off := base + uint64(i*12)
		if off+12 > uint64(len(tiff)) {
			return 0, errors.New("IFD 条目越界")
		}
		entry := tiff[off : off+12]
		tag := order.Uint16(entry[0:2])
		typ := order.Uint16(entry[2:4])
		items := uint64(order.Uint32(entry[4:8]))
		if tag != 0x0112 || typ != 3 || items < 1 {
			continue
		}
		if items*2 <= 4 {
			return int(order.Uint16(entry[8:10])), nil
		}
		valueOffset := uint64(order.Uint32(entry[8:12]))
		if valueOffset+2 > uint64(len(tiff)) {
			return 0, errors.New("Orientation 值越界")
		}
		return int(order.Uint16(tiff[valueOffset : valueOffset+2])), nil
	}
	return 1, nil
}

func ICCProfileHash(profile []byte) string {
	if len(profile) == 0 {
		return ""
	}
	sum := sha256.Sum256(profile)
	return fmt.Sprintf("%x", sum[:])
}
