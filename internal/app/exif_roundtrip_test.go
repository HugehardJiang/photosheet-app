package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"photosheet/internal/imaging"
	"photosheet/internal/model"
)

func TestEXIFOrientationRoundtripOneToEight(t *testing.T) {
	for orientation := 1; orientation <= 8; orientation++ {
		t.Run(string(rune('0'+orientation)), func(t *testing.T) {
			root := t.TempDir()
			source := image.NewNRGBA(image.Rect(0, 0, 3, 2))
			for y := 0; y < 2; y++ {
				for x := 0; x < 3; x++ {
					source.SetNRGBA(x, y, color.NRGBA{R: uint8(20 + x), G: uint8(40 + y), B: uint8(60 + x + y), A: 255})
				}
			}
			encoded := new(bytes.Buffer)
			if err := jpeg.Encode(encoded, source, &jpeg.Options{Quality: 100}); err != nil {
				t.Fatal(err)
			}
			jpegBytes := injectEXIFOrientation(encoded.Bytes(), orientation)
			input := filepath.Join(root, "orientation.jpg")
			if err := os.WriteFile(input, jpegBytes, 0o644); err != nil {
				t.Fatal(err)
			}
			settings := model.DefaultSettings()
			settings.MaximumLongEdge = 16
			settings.MemoryLimitBytes = 1
			settings.TileSize = 64
			output := filepath.Join(root, "project")
			response, err := NewService().Pack(context.Background(), PackRequest{Inputs: []string{input}, OutputDir: output, Settings: settings})
			if err != nil {
				t.Fatal(err)
			}
			if got := response.Project.Sheets[0].Images[0].EXIFOrientation; got != orientation {
				t.Fatalf("project orientation %d, want %d", got, orientation)
			}
			restoredDir := filepath.Join(output, "restored")
			if _, err := NewService().Unpack(context.Background(), UnpackRequest{ProjectPath: response.ProjectPath, OutputDir: restoredDir}); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(restoredDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Fatalf("restored %d files, want 1", len(entries))
			}
			restored, err := imaging.DecodeFile(filepath.Join(restoredDir, entries[0].Name()))
			if err != nil {
				t.Fatal(err)
			}
			normalized, _, err := imaging.DecodeNormalizedFile(input)
			if err != nil {
				t.Fatal(err)
			}
			if equal, mismatch := imaging.EqualPixels(normalized, restored); !equal {
				t.Fatalf("orientation %d mismatch: %d pixels", orientation, mismatch)
			}
		})
	}
}

func injectEXIFOrientation(jpegBytes []byte, orientation int) []byte {
	tiff := make([]byte, 26)
	copy(tiff, []byte("II"))
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint16(tiff[8:10], 1)
	binary.LittleEndian.PutUint16(tiff[10:12], 0x0112)
	binary.LittleEndian.PutUint16(tiff[12:14], 3)
	binary.LittleEndian.PutUint32(tiff[14:18], 1)
	binary.LittleEndian.PutUint16(tiff[18:20], uint16(orientation))
	payload := append([]byte("Exif\x00\x00"), tiff...)
	length := len(payload) + 2
	segment := []byte{0xff, 0xe1, byte(length >> 8), byte(length)}
	segment = append(segment, payload...)
	return append(append(append([]byte{}, jpegBytes[:2]...), segment...), jpegBytes[2:]...)
}
