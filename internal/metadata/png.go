package metadata

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

func ReadPNGICCProfile(filename string) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readPNGICCProfile(f)
}

func readPNGICCProfile(r io.Reader) ([]byte, error) {
	var signature [8]byte
	if _, err := io.ReadFull(r, signature[:]); err != nil {
		return nil, err
	}
	if !bytes.Equal(signature[:], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return nil, errors.New("文件不是有效 PNG")
	}
	for {
		var header [8]byte
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return nil, err
		}
		length := binary.BigEndian.Uint32(header[:4])
		if length > 128<<20 {
			return nil, errors.New("PNG chunk 过大")
		}
		kind := string(header[4:8])
		data := make([]byte, int(length))
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}
		var crc [4]byte
		if _, err := io.ReadFull(r, crc[:]); err != nil {
			return nil, err
		}
		if kind == "iCCP" {
			zero := bytes.IndexByte(data, 0)
			if zero < 1 || zero+2 > len(data) || data[zero+1] != 0 {
				return nil, errors.New("PNG iCCP chunk 无效")
			}
			profile, err := zlibDecode(data[zero+2:])
			if err != nil {
				return nil, fmt.Errorf("解压 PNG ICC Profile 失败: %w", err)
			}
			return profile, nil
		}
		if kind == "IEND" {
			return nil, nil
		}
	}
}

func zlibDecode(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, 128<<20))
}
