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
	"photosheet/internal/resolver"
)

func TestUnpackUsesArbitraryProcessedLocationAndManualResolver(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "input.jpg")
	writeTestJPEG(t, input, 9, 7)
	settings := model.DefaultSettings()
	settings.MaximumLongEdge = 32
	settings.MemoryLimitBytes = 1
	settings.TileSize = 64
	projectDir := filepath.Join(root, "project")
	packed, err := NewService().Pack(context.Background(), PackRequest{Inputs: []string{input}, OutputDir: projectDir, Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	sheet := packed.Project.Sheets[0]
	processedDir := filepath.Join(root, "外部修图结果", "nested")
	if err := os.MkdirAll(processedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	processed := filepath.Join(processedDir, "export_"+sheet.CanonicalStem+".png")
	data, err := os.ReadFile(filepath.Join(projectDir, "sheets", sheet.File))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(processed, data, 0o644); err != nil {
		t.Fatal(err)
	}

	service := NewService()
	resolved, err := service.ResolveProcessed(context.Background(), ResolveRequest{ProjectPath: packed.ProjectPath, ProcessedDirs: []string{filepath.Dir(processed)}, Bindings: map[string]string{sheet.ID: processed}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Sheets) != 1 || resolved.Sheets[0].Status != resolver.StatusMatched || !resolved.Sheets[0].Manual {
		t.Fatalf("resolved=%#v", resolved)
	}
	output := filepath.Join(root, "restored")
	if _, err := service.Unpack(context.Background(), UnpackRequest{
		ProjectPath: packed.ProjectPath, ProcessedDirs: []string{filepath.Dir(processed)}, Bindings: map[string]string{sheet.ID: processed}, OutputDir: output,
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("restored files=%d, want 1", len(entries))
	}
	if _, err := imaging.DecodeFile(filepath.Join(output, entries[0].Name())); err != nil {
		t.Fatal(err)
	}
	tiffOutput := filepath.Join(root, "restored-tiff")
	if _, err := service.Unpack(context.Background(), UnpackRequest{
		ProjectPath: packed.ProjectPath, ProcessedDirs: []string{filepath.Dir(processed)}, Bindings: map[string]string{sheet.ID: processed},
		OutputDir: tiffOutput, OutputFormat: model.OutputTIFF,
	}); err != nil {
		t.Fatal(err)
	}
	tiffEntries, err := os.ReadDir(tiffOutput)
	if err != nil {
		t.Fatal(err)
	}
	if len(tiffEntries) != 1 || filepath.Ext(tiffEntries[0].Name()) != ".tif" {
		t.Fatalf("TIFF output=%#v", tiffEntries)
	}
	if _, err := imaging.DecodeFile(filepath.Join(tiffOutput, tiffEntries[0].Name())); err != nil {
		t.Fatal(err)
	}
}

func TestResolveAmbiguityBlocksExtraction(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "input.jpg")
	writeTestJPEG(t, input, 9, 7)
	settings := model.DefaultSettings()
	settings.MaximumLongEdge = 32
	projectDir := filepath.Join(root, "project")
	packed, err := NewService().Pack(context.Background(), PackRequest{Inputs: []string{input}, OutputDir: projectDir, Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	sheet := packed.Project.Sheets[0]
	processedDir := filepath.Join(root, "edited")
	if err := os.MkdirAll(processedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	originalData, err := os.ReadFile(filepath.Join(projectDir, "sheets", sheet.File))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{sheet.File, "export_" + sheet.CanonicalStem + ".png"} {
		if err := os.WriteFile(filepath.Join(processedDir, name), originalData, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := NewService().ResolveProcessed(context.Background(), ResolveRequest{ProjectPath: packed.ProjectPath, ProcessedDirs: []string{processedDir}})
	if err != nil {
		t.Fatal(err)
	}
	if got := resolved.Sheets[0].Status; got != resolver.StatusAmbiguous {
		t.Fatalf("status=%s, want %s", got, resolver.StatusAmbiguous)
	}
	if _, err := NewService().Unpack(context.Background(), UnpackRequest{ProjectPath: packed.ProjectPath, ProcessedDirs: []string{processedDir}, OutputDir: filepath.Join(root, "restored")}); err == nil {
		t.Fatal("expected ambiguous resolution to block extraction")
	}
}

func writeTestJPEG(t *testing.T, filename string, width, height int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(20 + x), G: uint8(40 + y), B: uint8(60 + x + y), A: 255})
		}
	}
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 95}); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
