package app

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"photosheet/internal/imaging"
	"photosheet/internal/model"
	"photosheet/internal/project"
)

func TestStrictLosslessRoundtripPixelMismatchZero(t *testing.T) {
	root := t.TempDir()
	inputDir := filepath.Join(root, "中文目录 😀")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(inputDir, "照片 #1.jpg")
	sourcePixels := image.NewNRGBA(image.Rect(0, 0, 11, 7))
	for y := 0; y < sourcePixels.Bounds().Dy(); y++ {
		for x := 0; x < sourcePixels.Bounds().Dx(); x++ {
			sourcePixels.SetNRGBA(x, y, color.NRGBA{R: uint8(7*x + y), G: uint8(13*y + x), B: uint8(31 + x*y), A: 255})
		}
	}
	file, err := os.Create(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(file, sourcePixels, &jpeg.Options{Quality: 95}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(root, "PhotoSheet 项目")
	settings := model.DefaultSettings()
	settings.MaximumLongEdge = 32
	settings.Padding = 0
	settings.MemoryLimitBytes = 1024
	settings.TileSize = 64
	preview, err := NewService().Preview(context.Background(), PackRequest{Inputs: []string{inputDir}, Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Sheets) != 1 || len(preview.Sheets[0].Images) != 1 {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	response, err := NewService().Pack(context.Background(), PackRequest{
		Inputs:    []string{inputDir},
		OutputDir: output,
		Settings:  settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Project.Sheets) != 1 {
		t.Fatalf("got %d sheets, want 1", len(response.Project.Sheets))
	}
	projectLoaded, err := project.Load(response.ProjectPath)
	if err != nil {
		t.Fatal(err)
	}
	if projectLoaded.Sheets[0].Images[0].Filename != "照片 #1.jpg" {
		t.Fatalf("Unicode filename was not preserved: %q", projectLoaded.Sheets[0].Images[0].Filename)
	}
	restoredDir := filepath.Join(output, "restored-test")
	if _, err := NewService().Unpack(context.Background(), UnpackRequest{
		ProjectPath: response.ProjectPath,
		SheetsDir:   filepath.Join(output, "sheets"),
		OutputDir:   restoredDir,
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(restoredDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d restored files, want 1", len(entries))
	}
	restored, err := imaging.DecodeFile(filepath.Join(restoredDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	normalizedSource, _, err := imaging.DecodeNormalizedFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	if equal, mismatch := imaging.EqualPixels(normalizedSource, restored); !equal {
		t.Fatalf("Strict Lossless Roundtrip pixel mismatch: %d, want 0", mismatch)
	}
}
