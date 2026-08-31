package metadata

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestJPEGOrientationOneToEight(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		for orientation := 1; orientation <= 8; orientation++ {
			data := syntheticJPEGWithEXIF(order, orientation)
			got, err := ReadJPEGMetadataReader(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("order %T orientation %d: %v", order, orientation, err)
			}
			if got.Orientation != orientation {
				t.Fatalf("order %T: got orientation %d, want %d", order, got.Orientation, orientation)
			}
		}
	}
}

func syntheticJPEGWithEXIF(order binary.ByteOrder, orientation int) []byte {
	tiff := make([]byte, 26)
	if order == binary.LittleEndian {
		copy(tiff, []byte("II"))
	} else {
		copy(tiff, []byte("MM"))
	}
	order.PutUint16(tiff[2:4], 42)
	order.PutUint32(tiff[4:8], 8)
	order.PutUint16(tiff[8:10], 1)
	order.PutUint16(tiff[10:12], 0x0112)
	order.PutUint16(tiff[12:14], 3)
	order.PutUint32(tiff[14:18], 1)
	order.PutUint16(tiff[18:20], uint16(orientation))
	payload := append([]byte("Exif\x00\x00"), tiff...)
	segmentLength := len(payload) + 2
	result := []byte{0xff, 0xd8, 0xff, 0xe1, byte(segmentLength >> 8), byte(segmentLength)}
	result = append(result, payload...)
	result = append(result, 0xff, 0xd9)
	return result
}
