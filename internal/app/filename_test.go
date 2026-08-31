package app

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"photosheet/internal/model"
)

func TestDuplicateFilenamesNeverOverwrite(t *testing.T) {
	root := t.TempDir()
	for index, directory := range []string{"一", "二"} {
		directoryPath := filepath.Join(root, directory)
		if err := os.MkdirAll(directoryPath, 0o755); err != nil {
			t.Fatal(err)
		}
		filename, err := os.Create(filepath.Join(directoryPath, "same.jpg"))
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
		img.SetNRGBA(index, index, color.NRGBA{R: uint8(20 + index), A: 255})
		if err := jpeg.Encode(filename, img, &jpeg.Options{Quality: 90}); err != nil {
			_ = filename.Close()
			t.Fatal(err)
		}
		if err := filename.Close(); err != nil {
			t.Fatal(err)
		}
	}
	settings := model.DefaultSettings()
	settings.MaximumLongEdge = 32
	output := filepath.Join(root, "project")
	response, err := NewService().Pack(context.Background(), PackRequest{Inputs: []string{root}, OutputDir: output, Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewService().Unpack(context.Background(), UnpackRequest{ProjectPath: response.ProjectPath}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(output, "restored"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d restored files, want 2", len(entries))
	}
	if entries[0].Name() == entries[1].Name() {
		t.Fatalf("duplicate output names were not disambiguated: %q", entries[0].Name())
	}
}
