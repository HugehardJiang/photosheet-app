package renderer

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"photosheet/internal/imaging"
	"photosheet/internal/model"
)

func TestPNG16AndTIFFZIPPreservePixels(t *testing.T) {
	root := t.TempDir()
	src := image.NewNRGBA64(image.Rect(0, 0, 2, 2))
	src.SetNRGBA64(0, 0, color.NRGBA64{R: 0x1234, G: 0x2345, B: 0x3456, A: 0xffff})
	src.SetNRGBA64(1, 0, color.NRGBA64{R: 0xabcd, G: 0xbcde, B: 0xcdef, A: 0xffff})
	src.SetNRGBA64(0, 1, color.NRGBA64{R: 0x0102, G: 0x0304, B: 0x0506, A: 0xffff})
	src.SetNRGBA64(1, 1, color.NRGBA64{R: 0xff00, G: 0x00ff, B: 0x7f7f, A: 0xffff})

	canvas, err := NewCanvas16(2, 2, 1<<20, 64, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := canvas.DrawImage16(0, 0, src, 0, model.PaddingSolid); err != nil {
		t.Fatal(err)
	}
	pngName := filepath.Join(root, "sheet.png")
	if err := WritePNG16Atomic(pngName, canvas, nil); err != nil {
		t.Fatal(err)
	}
	pngBytes, err := os.ReadFile(pngName)
	if err != nil {
		t.Fatal(err)
	}
	if len(pngBytes) < 25 || pngBytes[24] != 16 {
		t.Fatalf("PNG bit depth byte=%d", pngBytes[24])
	}
	decodedPNG, _, err := imaging.DecodeNormalizedFile16(pngName)
	if err != nil {
		t.Fatal(err)
	}
	if equal, mismatch := imaging.EqualPixels16(src, decodedPNG); !equal {
		t.Fatalf("PNG16 mismatch=%d", mismatch)
	}
	if err := canvas.Close(); err != nil {
		t.Fatal(err)
	}

	canvas, err = NewCanvas16(2, 2, 1<<20, 64, root)
	if err != nil {
		t.Fatal(err)
	}
	defer canvas.Close()
	if err := canvas.DrawImage16(0, 0, src, 0, model.PaddingSolid); err != nil {
		t.Fatal(err)
	}
	tiffName := filepath.Join(root, "sheet.tif")
	if err := WriteTIFF16CanvasAtomic(tiffName, canvas, model.TIFFZIP, nil); err != nil {
		t.Fatal(err)
	}
	inspection, err := imaging.InspectFile(tiffName)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.BitDepth != 16 || inspection.Compression == "" {
		t.Fatalf("TIFF inspection=%#v", inspection)
	}
	decodedTIFF, _, err := imaging.DecodeNormalizedFile16(tiffName)
	if err != nil {
		t.Fatal(err)
	}
	if equal, mismatch := imaging.EqualPixels16(src, decodedTIFF); !equal {
		t.Fatalf("TIFF16 mismatch=%d", mismatch)
	}
}

func TestLegacyTIFFExtractionUsesDeflate(t *testing.T) {
	root := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff})
	filename := filepath.Join(root, "restored.tif")
	if err := WriteTIFFAtomic(filename, img); err != nil {
		t.Fatal(err)
	}
	inspection, err := imaging.InspectFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Compression != "TIFF compression 8" {
		t.Fatalf("legacy TIFF compression=%q, want TIFF compression 8", inspection.Compression)
	}
}
