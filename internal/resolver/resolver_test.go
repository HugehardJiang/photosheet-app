package resolver

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"photosheet/internal/model"
)

func TestResolveMatchesSuffixAndDoesNotAutoSelectAmbiguity(t *testing.T) {
	root := t.TempDir()
	processed := filepath.Join(root, "edited", "nested")
	if err := os.MkdirAll(processed, 0o755); err != nil {
		t.Fatal(err)
	}
	p := resolverTestProject()
	canonical := p.Sheets[0].CanonicalStem
	exact := filepath.Join(processed, canonical+"_final.PNG")
	other := filepath.Join(processed, "export_"+canonical+".png")
	writeResolverPNG(t, exact, 10, 8)
	writeResolverPNG(t, other, 10, 8)

	result, err := Resolve(context.Background(), p, Options{Sources: []string{filepath.Join(root, "edited")}, Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Sheets[0].Status; got != StatusAmbiguous {
		t.Fatalf("status=%s, want %s", got, StatusAmbiguous)
	}
	if len(result.Sheets[0].Candidates) != 2 {
		t.Fatalf("candidates=%d, want 2", len(result.Sheets[0].Candidates))
	}

	result, err = Resolve(context.Background(), p, Options{
		Sources: []string{filepath.Join(root, "edited")}, Recursive: true,
		Bindings: map[string]string{p.Sheets[0].ID: other},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved := result.Sheets[0]
	if resolved.Status != StatusMatched || !resolved.Manual || resolved.Path != other {
		t.Fatalf("manual resolution=%#v", resolved)
	}
}

func TestResolveReportsMissingInvalidAndDimensionStates(t *testing.T) {
	tests := []struct {
		name       string
		write      func(t *testing.T, path string)
		wantStatus Status
	}{
		{name: "missing", wantStatus: StatusMissing},
		{name: "invalid", write: func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("not an image"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, wantStatus: StatusInvalid},
		{name: "uniform", write: func(t *testing.T, path string) { writeResolverPNG(t, path, 5, 4) }, wantStatus: StatusUniformScaled},
		{name: "unsafe", write: func(t *testing.T, path string) { writeResolverPNG(t, path, 7, 4) }, wantStatus: StatusUnsafeTransform},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			p := resolverTestProject()
			dir := filepath.Join(root, "results")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if test.write != nil {
				test.write(t, filepath.Join(dir, p.Sheets[0].CanonicalStem+".png"))
			}
			result, err := Resolve(context.Background(), p, Options{Sources: []string{dir}})
			if err != nil {
				t.Fatal(err)
			}
			if got := result.Sheets[0].Status; got != test.wantStatus {
				t.Fatalf("status=%s, want %s; result=%#v", got, test.wantStatus, result.Sheets[0])
			}
		})
	}
}

func resolverTestProject() *model.Project {
	settings := model.DefaultSettings()
	settings.MaximumLongEdge = 100
	settings.MaximumShortEdge = 100
	return &model.Project{
		SchemaVersion: model.CurrentSchemaVersion,
		AppVersion:    "test",
		ProjectID:     "abc12345",
		Mode:          model.ModeStrictLossless,
		Settings:      settings,
		Sheets: []model.Sheet{{
			ID: "abc12345-S001", Index: 1, CanonicalStem: "PS_abc12345_S001", File: "PS_abc12345_S001.png",
			Width: 10, Height: 8, Format: model.FormatPNG,
			Images: []model.Image{{
				ID: "image-1", Filename: "照片.png", DisplayWidth: 10, DisplayHeight: 8, EncodedWidth: 10, EncodedHeight: 8,
				EXIFOrientation: 1, OuterRect: model.Rect{Width: 10, Height: 8}, ContentRect: model.Rect{Width: 10, Height: 8},
			}},
		}},
	}
}

func writeResolverPNG(t *testing.T, filename string, width, height int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 20, A: 255})
		}
	}
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
