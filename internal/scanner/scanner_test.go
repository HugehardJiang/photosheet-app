package scanner

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestScanSkipInvalidAndCancellation(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "中文 😀.jpg")
	f, err := os.Create(valid)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 5, 3))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "损坏.jpg"), []byte("not jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(context.Background(), []string{root}, Options{SkipInvalid: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Images) != 1 || len(result.Errors) != 1 {
		t.Fatalf("got %d valid and %d errors, want 1 and 1", len(result.Images), len(result.Errors))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, []string{root}, Options{SkipInvalid: true}); err == nil {
		t.Fatal("expected cancelled scan error")
	}
}

func TestScanFailureCategories(t *testing.T) {
	t.Run("no supported files", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("not an image"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Scan(context.Background(), []string{root}, Options{})
		var failure *ScanFailure
		if !errors.As(err, &failure) || failure.Category != NoSupportedFilesFound {
			t.Fatalf("error=%v, failure=%+v, want NoSupportedFilesFound", err, failure)
		}
	})

	t.Run("unsupported raw compression", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "unsupported.dng")
		if err := os.WriteFile(path, []byte("not a dng"), 0o644); err != nil {
			t.Fatal(err)
		}
		// The extension is supported, but invalid bytes are classified as
		// corrupt rather than silently becoming "no supported files".
		_, err := Scan(context.Background(), []string{root}, Options{})
		var failure *ScanFailure
		if !errors.As(err, &failure) || failure.Category != FilesCorrupted {
			t.Fatalf("error=%v, failure=%+v, want FilesCorrupted", err, failure)
		}
	})

	t.Run("corrupt file skipped with valid image", func(t *testing.T) {
		root := t.TempDir()
		if err := writeTestJPEG(filepath.Join(root, "valid.jpg"), color.NRGBA{R: 220, A: 255}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "corrupt.jpg"), []byte("not jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
		result, err := Scan(context.Background(), []string{root}, Options{SkipInvalid: true})
		if err != nil || result.Summary.Successful != 1 || result.Summary.Corrupted != 1 {
			t.Fatalf("result=%+v error=%v, want one success and one corrupt", result.Summary, err)
		}
	})
}

func TestScanDeduplicatesExactCopiesAndExportCopies(t *testing.T) {
	root := t.TempDir()
	firstDir := filepath.Join(root, "第一组")
	secondDir := filepath.Join(root, "第二组")
	exportDir := filepath.Join(root, "第二组_导出")
	for _, directory := range []string{firstDir, secondDir, exportDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	firstPath := filepath.Join(firstDir, "same.jpg")
	secondPath := filepath.Join(secondDir, "same.jpg")
	exportPath := filepath.Join(exportDir, "same.jpg")
	if err := writeTestJPEG(firstPath, color.NRGBA{R: 240, A: 255}); err != nil {
		t.Fatal(err)
	}
	if err := writeTestJPEG(secondPath, color.NRGBA{G: 240, A: 255}); err != nil {
		t.Fatal(err)
	}
	if err := writeTestJPEG(exportPath, color.NRGBA{B: 240, A: 255}); err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(root, "same-copy.jpg")
	data, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copyPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Selecting the parent plus the export folder explicitly exercises both
	// recursive export filtering and the post-scan safety net.
	result, err := Scan(context.Background(), []string{root, exportDir}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Images) != 2 {
		t.Fatalf("got %d images, want the two distinct non-export photos", len(result.Images))
	}
	if len(result.Duplicates) != 2 {
		t.Fatalf("got %d duplicate records, want exact copy and export copy", len(result.Duplicates))
	}
	for index, info := range result.Images {
		wantID := []string{"image-000001", "image-000002"}[index]
		if info.ID != wantID {
			t.Fatalf("image %d has ID %q, want %q", index, info.ID, wantID)
		}
	}
}

func writeTestJPEG(filename string, fill color.NRGBA) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	img := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.SetNRGBA(x, y, fill)
		}
	}
	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 90}); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
