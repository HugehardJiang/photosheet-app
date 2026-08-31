package renderer

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"photosheet/internal/metadata"
	"photosheet/internal/model"
)

func TestPNGWriterPreservesICCProfileMetadata(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "sheet.png")
	canvas, err := NewCanvas(2, 2, 1<<20, 64, root)
	if err != nil {
		t.Fatal(err)
	}
	defer canvas.Close()
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	src.SetNRGBA(0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	src.SetNRGBA(1, 0, color.NRGBA{R: 40, G: 50, B: 60, A: 255})
	src.SetNRGBA(0, 1, color.NRGBA{R: 70, G: 80, B: 90, A: 255})
	src.SetNRGBA(1, 1, color.NRGBA{R: 100, G: 110, B: 120, A: 255})
	if err := canvas.DrawImage(0, 0, src, 0, model.PaddingSolid); err != nil {
		t.Fatal(err)
	}
	profile := []byte("synthetic ICC profile for test")
	if err := WritePNGAtomic(filename, canvas, profile); err != nil {
		t.Fatal(err)
	}
	if err := VerifyImageSize(filename, 2, 2); err != nil {
		t.Fatal(err)
	}
	got, err := metadata.ReadPNGICCProfile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(profile) {
		t.Fatalf("ICC profile changed: %q", got)
	}
	if _, err := os.Stat(filename); err != nil {
		t.Fatal(err)
	}
}
