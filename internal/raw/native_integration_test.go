//go:build photosheet_native && ((windows && amd64) || (darwin && (amd64 || arm64)))

package raw

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Set PHOTOSHEET_NATIVE_DNG_FIXTURE to a real DNG when running the optional
// integration test. The repository's unit suite remains self-contained.
func TestNativeDNGIntegrationFixture(t *testing.T) {
	filename := os.Getenv("PHOTOSHEET_NATIVE_DNG_FIXTURE")
	if filename == "" {
		t.Skip("PHOTOSHEET_NATIVE_DNG_FIXTURE is not set")
	}
	if _, err := os.Stat(filename); err != nil {
		t.Skipf("fixture is unavailable: %v", err)
	}
	validateNativeFixture(t, filename)
}

func TestDNGProbeAndDecodeCapabilities(t *testing.T) {
	filename := nativeFixtureForTag(t, compressionJXL)
	validateNativeFixture(t, filename)
}

func TestJPEGCompressedDNGDecode(t *testing.T) {
	filename := nativeFixtureForTag(t, compressionJPEG)
	validateNativeFixture(t, filename)
}

func TestJPEGXLDNGDecode(t *testing.T) {
	filename := nativeFixtureForTag(t, compressionJXL)
	validateNativeFixture(t, filename)
}

func validateNativeFixture(t *testing.T, filename string) {
	t.Helper()
	ctx := context.Background()
	meta, err := NewRegistry().Probe(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.DecodeAvailable || meta.Backend != nativeBackendName {
		t.Fatalf("native probe did not claim fixture: %#v", meta)
	}
	img, decodedMeta, err := NewRegistry().Decode(ctx, filename, DecodeOptions{WorkingColorSpace: "sRGB", Demosaic: "balanced"})
	if err != nil {
		t.Fatal(err)
	}
	if decodedMeta.Backend != nativeBackendName || decodedMeta.NativeBitDepth != 16 {
		t.Fatalf("unexpected decode metadata: %#v", decodedMeta)
	}
	expectedWidth, expectedHeight := meta.EncodedWidth, meta.EncodedHeight
	if meta.DisplayOrientation >= 5 && meta.DisplayOrientation <= 8 {
		expectedWidth, expectedHeight = expectedHeight, expectedWidth
	}
	if img.Bounds().Dx() != expectedWidth || img.Bounds().Dy() != expectedHeight {
		t.Fatalf("orientation/display bounds=%v, want %dx%d (orientation %d)", img.Bounds(), expectedWidth, expectedHeight, meta.DisplayOrientation)
	}
	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		t.Fatalf("invalid decoded bounds: %v", img.Bounds())
	}
	if len(img.Pix) == 0 {
		t.Fatal("native decoder returned an empty pixel buffer")
	}
	if !strings.Contains(decodedMeta.DecodeSource, "LibRaw") || !strings.Contains(decodedMeta.DecodeSource, "DNG SDK") {
		t.Fatalf("final render source is not the native RAW pipeline: %q", decodedMeta.DecodeSource)
	}
	var nonZero, non8BitExpanded int
	var sums [3]uint64
	var mins = [3]uint16{0xffff, 0xffff, 0xffff}
	var maxs [3]uint16
	var samples int
	for offset := 0; offset+8 <= len(img.Pix); offset += 8 * 10000 {
		r := binary.BigEndian.Uint16(img.Pix[offset : offset+2])
		g := binary.BigEndian.Uint16(img.Pix[offset+2 : offset+4])
		b := binary.BigEndian.Uint16(img.Pix[offset+4 : offset+6])
		values := [3]uint16{r, g, b}
		for index, value := range values {
			sums[index] += uint64(value)
			if value < mins[index] {
				mins[index] = value
			}
			if value > maxs[index] {
				maxs[index] = value
			}
		}
		samples++
		if r != 0 || g != 0 || b != 0 {
			nonZero++
		}
		if r%257 != 0 || g%257 != 0 || b%257 != 0 {
			non8BitExpanded++
		}
	}
	if nonZero == 0 {
		t.Fatal("native decoder returned only black pixels")
	}
	if non8BitExpanded == 0 {
		t.Fatal("native decoder output looks like 8-bit values expanded by 257")
	}
	if samples == 0 {
		t.Fatal("native decoder produced no color samples")
	}
	means := [3]float64{float64(sums[0]) / float64(samples), float64(sums[1]) / float64(samples), float64(sums[2]) / float64(samples)}
	for index := range means {
		if means[index] <= 0 || means[index] >= 65535 || mins[index] == maxs[index] {
			t.Fatalf("implausible color channel %d: mean=%.1f min=%d max=%d", index, means[index], mins[index], maxs[index])
		}
	}
	if means[0] == means[1] && means[1] == means[2] {
		t.Fatal("all decoded color channels are identical")
	}
}

func nativeFixtureForTag(t *testing.T, tag uint16) string {
	t.Helper()
	root := os.Getenv("PHOTOSHEET_NATIVE_DNG_FIXTURES")
	if root == "" {
		root = os.Getenv("PHOTOSHEET_NATIVE_DNG_FIXTURE")
	}
	if root == "" {
		t.Skip("PHOTOSHEET_NATIVE_DNG_FIXTURE(S) is not set")
	}
	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		meta, err := NewRegistry().Probe(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		if meta.CompressionTag != tag {
			t.Skipf("fixture compression tag=%d, want %d", meta.CompressionTag, tag)
		}
		return root
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Skipf("fixture directory is unavailable: %v", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".dng") {
			paths = append(paths, filepath.Join(root, entry.Name()))
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		meta, err := NewRegistry().Probe(context.Background(), path)
		if err == nil && meta.CompressionTag == tag && meta.DecodeAvailable {
			return path
		}
	}
	t.Skipf("no decodable DNG with compression tag %d in %s", tag, root)
	return ""
}
