package imaging

import (
	"image"
	"image/color"
	"testing"
)

func TestTransformOrientationDimensionsAndPixels(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(10 + x), G: uint8(20 + y), B: uint8(30 + x + y), A: 255})
		}
	}
	for orientation := 1; orientation <= 8; orientation++ {
		got := TransformOrientation(src, orientation)
		wantWidth, wantHeight := 3, 2
		if orientation >= 5 {
			wantWidth, wantHeight = 2, 3
		}
		if got.Bounds().Dx() != wantWidth || got.Bounds().Dy() != wantHeight {
			t.Fatalf("orientation %d dimensions %dx%d, want %dx%d", orientation, got.Bounds().Dx(), got.Bounds().Dy(), wantWidth, wantHeight)
		}
		for y := 0; y < wantHeight; y++ {
			for x := 0; x < wantWidth; x++ {
				sx, sy := orientationSourceCoordinate(x, y, 3, 2, orientation)
				if got.NRGBAAt(x, y) != src.NRGBAAt(sx, sy) {
					t.Fatalf("orientation %d pixel (%d,%d) mapped to (%d,%d) incorrectly", orientation, x, y, sx, sy)
				}
			}
		}
	}
}

func TestRotateInverse(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), A: 255})
		}
	}
	rotated := Rotate90CW(src)
	restored := Rotate90CCW(rotated)
	if equal, diff := EqualPixels(src, restored); !equal {
		t.Fatalf("rotation inverse differs in %d pixels", diff)
	}
}
