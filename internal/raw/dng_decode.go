package raw

import (
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
)

// Decode implements the conservative, fully-Go fallback backend. It handles
// DNG IFDs whose pixel payload is stored as uncompressed or Deflate strips or
// tiles. JPEG lossless, baseline JPEG, JPEG XL, packed exotic layouts and
// camera-specific color matrices are intentionally rejected until a native
// backend is installed; Probe still reports those files accurately.
func (dngBackend) Decode(ctx context.Context, filename string, options DecodeOptions) (*image.NRGBA64, Metadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, Metadata{}, err
	}
	if options.WorkingColorSpace != "" && options.WorkingColorSpace != "sRGB" {
		return nil, Metadata{}, fmt.Errorf("内置 DNG backend 尚未实现 %s 色彩空间转换", options.WorkingColorSpace)
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("打开 DNG 失败: %w", err)
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, Metadata{}, err
	}
	parsed, err := parseDNG(f, stat.Size())
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("%w: %v", ErrRawCorrupt, err)
	}
	meta := parsed.metadata(filename)
	meta.DecodeSource = "pure-go-dng-pixel-decode"
	if meta.DNGVersion == "" && !parsed.hasDNGMarker() {
		return nil, Metadata{}, ErrNotRAW
	}
	if meta.CompressionTag != compressionNone && meta.CompressionTag != compressionDeflate {
		if meta.CompressionTag == compressionJXL {
			return nil, meta, fmt.Errorf("%w: JPEG XL (tag %d)", ErrUnsupportedCompression, meta.CompressionTag)
		}
		return nil, meta, fmt.Errorf("%w: %s", ErrUnsupportedCompression, meta.Compression)
	}
	if meta.NativeBitDepth != 8 && meta.NativeBitDepth != 16 && meta.NativeBitDepth != 10 && meta.NativeBitDepth != 12 && meta.NativeBitDepth != 14 {
		return nil, meta, fmt.Errorf("%w: RAW 位深 %d", ErrUnsupportedVariant, meta.NativeBitDepth)
	}
	rawIFD := parsed.rawIFD()
	width := int(tagValueUint(parsed.order, rawIFD.tags, tagImageWidth))
	height := int(tagValueUint(parsed.order, rawIFD.tags, tagImageLength))
	if width <= 0 || height <= 0 {
		return nil, meta, errors.New("DNG 原始图像尺寸无效")
	}
	samples := int(tagValueUint(parsed.order, rawIFD.tags, tagSamplesPerPixel))
	if samples == 0 {
		samples = 1
	}
	if samples != 1 && samples < 3 {
		return nil, meta, fmt.Errorf("%w: samplesPerPixel=%d", ErrUnsupportedVariant, samples)
	}
	bytes, err := readPixelPayload(ctx, f, parsed, rawIFD, width, height, samples)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, ErrRawUnsupportedCompression) || errors.Is(err, ErrRawUnsupportedVariant) {
			return nil, meta, err
		}
		return nil, meta, fmt.Errorf("%w: %v", ErrRawCorrupt, err)
	}
	values, err := unpackSamples(parsed.order, bytes, width*height*samples, meta.NativeBitDepth)
	if err != nil {
		return nil, meta, fmt.Errorf("%w: %v", ErrRawCorrupt, err)
	}
	if samples >= 3 {
		img, err := decodeRGB16(values, width, height, samples, meta)
		if err != nil {
			return nil, meta, err
		}
		meta.EffectiveBitDepth = 16
		return applyDNGCropAndOrientation(img, parsed, rawIFD, meta), meta, nil
	}
	pattern, err := cfaPattern(parsed.order, rawIFD)
	if err != nil {
		return nil, meta, err
	}
	img, err := decodeBayer16(ctx, values, width, height, pattern, meta, options)
	if err != nil {
		return nil, meta, err
	}
	meta.EffectiveBitDepth = 16
	return applyDNGCropAndOrientation(img, parsed, rawIFD, meta), meta, nil
}

func readPixelPayload(ctx context.Context, f *os.File, parsed *dngFile, ifd tiffIFD, width, height, samples int) ([]byte, error) {
	if offsetsTag, ok := ifd.tags[tagStripOffsets]; ok {
		offsets, err := tagUint64s(parsed.order, offsetsTag)
		if err != nil {
			return nil, err
		}
		counts, err := tagUint64s(parsed.order, ifd.tags[tagStripByteCounts])
		if err != nil {
			return nil, err
		}
		if len(offsets) != len(counts) || len(offsets) == 0 {
			return nil, errors.New("DNG strip offsets/byte counts 不匹配")
		}
		result := make([]byte, 0)
		for i, offset := range offsets {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			chunk, err := readChunk(f, int64(offset), int64(counts[i]), parsed.size, tagValueUint(parsed.order, ifd.tags, tagCompression))
			if err != nil {
				return nil, err
			}
			result = append(result, chunk...)
		}
		return result, nil
	}
	offsets, err := tagUint64s(parsed.order, ifd.tags[tagTileOffsets])
	if err != nil {
		return nil, err
	}
	counts, err := tagUint64s(parsed.order, ifd.tags[tagTileByteCounts])
	if err != nil {
		return nil, err
	}
	tileWidth := tagValueUint(parsed.order, ifd.tags, tagTileWidth)
	if tileWidth == 0 || tileWidth != uint64(width) {
		return nil, fmt.Errorf("%w: tiled DNG currently requires full-width tiles", ErrUnsupportedVariant)
	}
	if len(offsets) != len(counts) || len(offsets) == 0 {
		return nil, errors.New("DNG tile offsets/byte counts 不匹配")
	}
	result := make([]byte, 0)
	for i, offset := range offsets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chunk, err := readChunk(f, int64(offset), int64(counts[i]), parsed.size, tagValueUint(parsed.order, ifd.tags, tagCompression))
		if err != nil {
			return nil, err
		}
		result = append(result, chunk...)
	}
	return result, nil
}

func readChunk(f *os.File, offset, length, fileSize int64, compression uint64) ([]byte, error) {
	if offset < 0 || length < 0 || offset > fileSize || length > fileSize-offset || length > 1<<31 {
		return nil, errors.New("DNG pixel chunk 越界")
	}
	data := make([]byte, int(length))
	if _, err := readAtFull(f, data, offset); err != nil {
		return nil, fmt.Errorf("读取 DNG pixel chunk 失败: %w", err)
	}
	if compression == compressionNone {
		return data, nil
	}
	if compression != compressionDeflate {
		return nil, fmt.Errorf("%w: compression=%d", ErrUnsupportedCompression, compression)
	}
	reader, err := zlib.NewReader(bytesReader(data))
	if err != nil {
		return nil, fmt.Errorf("解压 DNG Deflate chunk 失败: %w", err)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(io.LimitReader(reader, 1<<31-1))
	if err != nil {
		return nil, fmt.Errorf("读取 DNG Deflate chunk 失败: %w", err)
	}
	return decoded, nil
}

// bytesReader keeps the zlib call local without exposing an implementation
// detail in the package API.
func bytesReader(data []byte) io.Reader { return &sliceReader{data: data} }

type sliceReader struct {
	data []byte
	pos  int
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func unpackSamples(order binary.ByteOrder, data []byte, count, bits int) ([]uint32, error) {
	if count <= 0 {
		return nil, errors.New("DNG 样本数量无效")
	}
	values := make([]uint32, count)
	if bits == 8 {
		if len(data) < count {
			return nil, errors.New("DNG 8-bit pixel 数据不足")
		}
		for i := range values {
			values[i] = uint32(data[i])
		}
		return values, nil
	}
	if bits == 16 {
		if len(data) < count*2 {
			return nil, errors.New("DNG 16-bit pixel 数据不足")
		}
		for i := range values {
			values[i] = uint32(order.Uint16(data[i*2 : i*2+2]))
		}
		return values, nil
	}
	if bits < 1 || bits > 16 {
		return nil, fmt.Errorf("%w: %d-bit packed samples", ErrUnsupportedVariant, bits)
	}
	reader := &bitReader{data: data}
	for i := range values {
		value, err := reader.read(bits)
		if err != nil {
			return nil, fmt.Errorf("读取 DNG packed samples 失败: %w", err)
		}
		values[i] = value
	}
	return values, nil
}

type bitReader struct {
	data []byte
	bit  int
}

func (r *bitReader) read(bits int) (uint32, error) {
	if r.bit+bits > len(r.data)*8 {
		return 0, io.ErrUnexpectedEOF
	}
	var result uint32
	for i := 0; i < bits; i++ {
		result = (result << 1) | uint32((r.data[r.bit/8]>>uint(7-r.bit%8))&1)
		r.bit++
	}
	return result, nil
}

func cfaPattern(order binary.ByteOrder, ifd tiffIFD) ([4]byte, error) {
	var pattern [4]byte
	dims, err := tagUint64s(order, ifd.tags[tagCFARepeatPatternDim])
	if err != nil || len(dims) < 2 || dims[0] != 2 || dims[1] != 2 {
		return pattern, fmt.Errorf("%w: 当前仅支持 2x2 Bayer CFA", ErrUnsupportedVariant)
	}
	data := ifd.tags[tagCFAPattern].data
	if len(data) < 4 {
		return pattern, errors.New("DNG CFA pattern 不完整")
	}
	copy(pattern[:], data[:4])
	return pattern, nil
}

func decodeRGB16(values []uint32, width, height, samples int, meta Metadata) (*image.NRGBA64, error) {
	if len(values) < width*height*samples {
		return nil, errors.New("DNG RGB samples 不足")
	}
	img := image.NewNRGBA64(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := (y*width + x) * samples
			img.SetNRGBA64(x, y, color.NRGBA64{R: color16(meta, values[index]), G: color16(meta, values[index+1]), B: color16(meta, values[index+2]), A: 0xffff})
		}
	}
	return img, nil
}

func decodeBayer16(ctx context.Context, values []uint32, width, height int, pattern [4]byte, meta Metadata, options DecodeOptions) (*image.NRGBA64, error) {
	if len(values) < width*height {
		return nil, errors.New("DNG Bayer samples 不足")
	}
	if options.Demosaic == "fast" { /* nearest-neighbour is reserved for preview callers */
	}
	img := image.NewNRGBA64(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		if y%32 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		for x := 0; x < width; x++ {
			var r, g, b uint32
			for channel := byte(0); channel < 3; channel++ {
				value := interpolateBayer(values, width, height, x, y, pattern, channel)
				switch channel {
				case 0:
					r = value
				case 1:
					g = value
				case 2:
					b = value
				}
			}
			img.SetNRGBA64(x, y, color.NRGBA64{R: color16(meta, r), G: color16(meta, g), B: color16(meta, b), A: 0xffff})
		}
	}
	return img, nil
}

func interpolateBayer(values []uint32, width, height, x, y int, pattern [4]byte, wanted byte) uint32 {
	var sum uint64
	count := uint64(0)
	for radius := 0; radius <= 2 && count == 0; radius++ {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if absInt(dx)+absInt(dy) != radius {
					continue
				}
				sx, sy := x+dx, y+dy
				if sx < 0 || sy < 0 || sx >= width || sy >= height {
					continue
				}
				if bayerChannel(pattern, sx, sy) == wanted {
					sum += uint64(values[sy*width+sx])
					count++
				}
			}
		}
	}
	if count == 0 {
		return values[y*width+x]
	}
	return uint32(sum / count)
}

func bayerChannel(pattern [4]byte, x, y int) byte {
	value := pattern[(y&1)*2+(x&1)]
	switch value {
	case 0:
		return 0
	case 1:
		return 1
	case 2:
		return 2
	default:
		return 1
	}
}
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func color16(meta Metadata, value uint32) uint16 {
	black := 0.0
	if len(meta.BlackLevel) > 0 {
		for _, item := range meta.BlackLevel {
			black += item
		}
		black /= float64(len(meta.BlackLevel))
	}
	white := meta.WhiteLevel
	if white <= black {
		white = float64((uint64(1) << uint(meta.NativeBitDepth)) - 1)
		if white > 65535 {
			white = 65535
		}
	}
	normalized := (float64(value) - black) / (white - black)
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}
	return uint16(math.Round(normalized * 65535))
}

func applyDNGCropAndOrientation(src *image.NRGBA64, parsed *dngFile, ifd tiffIFD, meta Metadata) *image.NRGBA64 {
	// The built-in fallback keeps the full active raster when the optional crop
	// tags are absent. Orientation is applied because packing uses display size.
	if src == nil {
		return nil
	}
	if meta.ActiveWidth > 0 && meta.ActiveHeight > 0 && (meta.ActiveWidth != src.Bounds().Dx() || meta.ActiveHeight != src.Bounds().Dy()) && meta.ActiveX >= 0 && meta.ActiveY >= 0 && meta.ActiveX+meta.ActiveWidth <= src.Bounds().Dx() && meta.ActiveY+meta.ActiveHeight <= src.Bounds().Dy() {
		cropped := image.NewNRGBA64(image.Rect(0, 0, meta.ActiveWidth, meta.ActiveHeight))
		for y := 0; y < meta.ActiveHeight; y++ {
			for x := 0; x < meta.ActiveWidth; x++ {
				cropped.SetNRGBA64(x, y, src.NRGBA64At(meta.ActiveX+x, meta.ActiveY+y))
			}
		}
		src = cropped
	}
	orientation := meta.DisplayOrientation
	if orientation < 1 || orientation > 8 {
		orientation = 1
	}
	if orientation == 1 {
		return src
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := w, h
	if orientation >= 5 {
		dw, dh = h, w
	}
	dst := image.NewNRGBA64(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			sx, sy := orientationSource(x, y, w, h, orientation)
			dst.SetNRGBA64(x, y, src.NRGBA64At(sx, sy))
		}
	}
	return dst
}
func orientationSource(x, y, w, h, orientation int) (int, int) {
	switch orientation {
	case 2:
		return w - 1 - x, y
	case 3:
		return w - 1 - x, h - 1 - y
	case 4:
		return x, h - 1 - y
	case 5:
		return y, x
	case 6:
		return y, h - 1 - x
	case 7:
		return w - 1 - y, h - 1 - x
	case 8:
		return w - 1 - y, x
	default:
		return x, y
	}
}
