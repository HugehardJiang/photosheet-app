package raw

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDNGProbeAndBuiltinDecode(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "fixture.dng")
	if err := os.WriteFile(filename, minimalDNG(1), 0o644); err != nil {
		t.Fatal(err)
	}
	meta, err := NewRegistry().Probe(context.Background(), filename)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.IsRAW || meta.Format != FormatDNG || meta.EncodedWidth != 2 || meta.EncodedHeight != 2 || meta.NativeBitDepth != 16 {
		t.Fatalf("unexpected probe: %#v", meta)
	}
	if meta.CompressionState != CompressionLossless || meta.CFA == "" || meta.DNGVersion != "1.4.0.0" {
		t.Fatalf("unexpected DNG metadata: %#v", meta)
	}
	decoded, _, err := NewRegistry().Decode(context.Background(), filename, DecodeOptions{WorkingColorSpace: "sRGB", Demosaic: "balanced"})
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 2 || decoded.Bounds().Dy() != 2 {
		t.Fatalf("decoded bounds=%v", decoded.Bounds())
	}
}

func TestJXLDNGProbeDoesNotClaimInvalidPayload(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "jxl-fixture.dng")
	if err := os.WriteFile(filename, minimalDNG(52546), 0o644); err != nil {
		t.Fatal(err)
	}
	meta, err := NewRegistry().Probe(context.Background(), filename)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Compression != "JPEG XL" || meta.CompressionState != CompressionUnknown || meta.Supported {
		t.Fatalf("unexpected JXL probe: %#v", meta)
	}
	_, _, err = NewRegistry().Decode(context.Background(), filename, DecodeOptions{})
	if !errors.Is(err, ErrUnsupportedCompression) && !errors.Is(err, ErrRawCorrupt) {
		t.Fatalf("decode error=%v, want unsupported compression or corruption", err)
	}
}

func TestUncompressedAndDeflateDNGDecode(t *testing.T) {
	for _, compression := range []uint16{compressionNone, compressionDeflate} {
		filename := filepath.Join(t.TempDir(), "fixture.dng")
		if err := os.WriteFile(filename, minimalDNG(compression), 0o644); err != nil {
			t.Fatal(err)
		}
		meta, err := NewRegistry().Probe(context.Background(), filename)
		if err != nil {
			t.Fatal(err)
		}
		if meta.CompressionTag != compression || !meta.DecodeAvailable {
			t.Fatalf("compression %d probe=%#v", compression, meta)
		}
		image, decodedMeta, err := NewRegistry().Decode(context.Background(), filename, DecodeOptions{WorkingColorSpace: "sRGB"})
		if err != nil {
			t.Fatalf("compression %d decode: %v", compression, err)
		}
		if decodedMeta.EffectiveBitDepth != 16 || image.Bounds().Dx() != 2 || image.Bounds().Dy() != 2 {
			t.Fatalf("compression %d output=%v meta=%#v", compression, image.Bounds(), decodedMeta)
		}
	}
}

func TestCorruptDNGIsNotReportedAsMissing(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "corrupt.dng")
	data := minimalDNG(compressionNone)
	data = data[:len(data)-1]
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := NewRegistry().Decode(context.Background(), filename, DecodeOptions{})
	if !errors.Is(err, ErrRawCorrupt) {
		t.Fatalf("decode error=%v, want ErrRawCorrupt", err)
	}
}

func minimalDNG(compression uint16) []byte {
	type entry struct {
		tag, typ uint16
		count    uint32
		value    []byte
	}
	entries := []entry{
		{tagImageWidth, 4, 1, u32(2)}, {tagImageLength, 4, 1, u32(2)}, {tagBitsPerSample, 3, 1, u16(16)}, {tagCompression, 3, 1, u16(compression)},
		{tagPhotometric, 3, 1, u16(32803)}, {tagStripOffsets, 4, 1, u32(0)}, {tagOrientation, 3, 1, u16(1)}, {tagSamplesPerPixel, 3, 1, u16(1)},
		{tagRowsPerStrip, 4, 1, u32(2)}, {tagStripByteCounts, 4, 1, u32(8)}, {tagCFARepeatPatternDim, 3, 2, []byte{2, 0, 2, 0}},
		{tagCFAPattern, 1, 4, []byte{0, 1, 1, 2}}, {tagDNGVersion, 1, 4, []byte{1, 4, 0, 0}}, {tagWhiteLevel, 4, 1, u32(65535)},
	}
	payload := []byte{0xe8, 0x03, 0xd0, 0x07, 0xb8, 0x0b, 0xa0, 0x0f}
	if compression == compressionDeflate {
		var compressed bytes.Buffer
		writer := zlib.NewWriter(&compressed)
		_, _ = writer.Write(payload)
		_ = writer.Close()
		payload = compressed.Bytes()
	}
	dataOffset := 8 + 2 + len(entries)*12 + 4
	entries[5].value = u32(uint32(dataOffset))
	entries[9].value = u32(uint32(len(payload)))
	result := make([]byte, dataOffset+len(payload))
	copy(result[:2], []byte{'I', 'I'})
	binary.LittleEndian.PutUint16(result[2:4], 42)
	binary.LittleEndian.PutUint32(result[4:8], 8)
	binary.LittleEndian.PutUint16(result[8:10], uint16(len(entries)))
	for i, item := range entries {
		offset := 10 + i*12
		binary.LittleEndian.PutUint16(result[offset:offset+2], item.tag)
		binary.LittleEndian.PutUint16(result[offset+2:offset+4], item.typ)
		binary.LittleEndian.PutUint32(result[offset+4:offset+8], item.count)
		copy(result[offset+8:offset+12], item.value)
	}
	copy(result[dataOffset:], payload)
	return result
}

func u16(value uint16) []byte {
	result := make([]byte, 2)
	binary.LittleEndian.PutUint16(result, value)
	return result
}
func u32(value uint32) []byte {
	result := make([]byte, 4)
	binary.LittleEndian.PutUint32(result, value)
	return result
}
