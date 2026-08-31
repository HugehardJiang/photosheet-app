package raw

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
)

const (
	tagImageWidth          = 256
	tagImageLength         = 257
	tagBitsPerSample       = 258
	tagCompression         = 259
	tagPhotometric         = 262
	tagImageDescription    = 270
	tagMake                = 271
	tagModel               = 272
	tagStripOffsets        = 273
	tagOrientation         = 274
	tagSamplesPerPixel     = 277
	tagRowsPerStrip        = 278
	tagStripByteCounts     = 279
	tagPlanarConfiguration = 284
	tagSubIFDs             = 330
	tagTileWidth           = 322
	tagTileLength          = 323
	tagTileOffsets         = 324
	tagTileByteCounts      = 325
	tagCFARepeatPatternDim = 33421
	tagCFAPattern          = 33422
	tagPreviewImageStart   = 513
	tagPreviewImageLength  = 514
	tagDNGVersion          = 50706
	tagDefaultCropSize     = 50720
	tagBlackLevel          = 50714
	tagWhiteLevel          = 50717
	tagAsShotNeutral       = 50728
	tagActiveArea          = 50829
)

const (
	compressionNone      = 1
	compressionJPEG      = 7
	compressionDeflate   = 8
	compressionLossyJPEG = 34892
	compressionJXL       = 52546
)

type dngBackend struct{}

func NewDNGBackend() RawBackend { return dngBackend{} }

func (dngBackend) Name() string    { return "photosheet-dng" }
func (dngBackend) Version() string { return "builtin-tiff-probe-1" }
func (dngBackend) Capabilities() BackendCapabilities {
	return BackendCapabilities{
		Formats:              []Format{FormatDNG},
		CompressionTags:      []uint16{compressionNone, compressionDeflate},
		ProbeSupported:       true,
		DecodeSupported:      true,
		PreviewSupported:     false,
		Supports16BitRGB:     true,
		SupportsUnicodePaths: true,
		OutputColorSpace:     "sRGB",
	}
}

type tiffTag struct {
	typ   uint16
	count uint64
	data  []byte
}

type tiffIFD struct {
	tags map[uint16]tiffTag
	next uint32
}

type dngFile struct {
	file  *os.File
	order binary.ByteOrder
	ifds  []tiffIFD
	size  int64
}

func (dngBackend) Probe(ctx context.Context, filename string) (Metadata, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}
	f, err := os.Open(filename)
	if err != nil {
		return Metadata{}, fmt.Errorf("打开 DNG 失败: %w", err)
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return Metadata{}, fmt.Errorf("读取 DNG 文件信息失败: %w", err)
	}
	parsed, err := parseDNG(f, stat.Size())
	if err != nil {
		return Metadata{}, err
	}
	meta := parsed.metadata(filename)
	if meta.DNGVersion == "" && !parsed.hasDNGMarker() {
		return Metadata{}, ErrNotRAW
	}
	return meta, nil
}

func parseDNG(f *os.File, size int64) (*dngFile, error) {
	if size < 8 {
		return nil, ErrNotRAW
	}
	var header [8]byte
	if _, err := readAtFull(f, header[:], 0); err != nil {
		return nil, fmt.Errorf("读取 DNG TIFF header 失败: %w", err)
	}
	var order binary.ByteOrder
	switch string(header[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil, ErrNotRAW
	}
	if order.Uint16(header[2:4]) != 42 {
		return nil, ErrNotRAW
	}
	first := order.Uint32(header[4:8])
	if first < 8 || int64(first) >= size {
		return nil, errors.New("DNG 首个 IFD 偏移越界")
	}
	parsed := &dngFile{file: f, order: order, size: size}
	seen := make(map[uint32]struct{})
	queue := []uint32{first}
	for len(queue) > 0 && len(parsed.ifds) < 64 {
		offset := queue[0]
		queue = queue[1:]
		if offset == 0 {
			continue
		}
		if _, ok := seen[offset]; ok {
			continue
		}
		seen[offset] = struct{}{}
		if int64(offset) >= size {
			return nil, errors.New("DNG IFD 偏移越界")
		}
		ifd, err := readIFD(f, order, size, offset)
		if err != nil {
			return nil, err
		}
		parsed.ifds = append(parsed.ifds, ifd)
		if ifd.next != 0 {
			queue = append(queue, ifd.next)
		}
		if sub, ok := ifd.tags[tagSubIFDs]; ok {
			offsets, err := tagUint64s(order, sub)
			if err == nil {
				for _, value := range offsets {
					if value > 0 && value <= uint64(^uint32(0)) {
						queue = append(queue, uint32(value))
					}
				}
			}
		}
	}
	if len(parsed.ifds) == 0 {
		return nil, errors.New("DNG 没有可读取的 IFD")
	}
	return parsed, nil
}

func readIFD(f *os.File, order binary.ByteOrder, size int64, offset uint32) (tiffIFD, error) {
	var countBytes [2]byte
	if _, err := readAtFull(f, countBytes[:], int64(offset)); err != nil {
		return tiffIFD{}, fmt.Errorf("读取 DNG IFD 条目数失败: %w", err)
	}
	count := int(order.Uint16(countBytes[:]))
	if count > 4096 {
		return tiffIFD{}, errors.New("DNG IFD 条目数异常")
	}
	if int64(offset)+2+int64(count)*12+4 > size {
		return tiffIFD{}, errors.New("DNG IFD 条目越界")
	}
	result := tiffIFD{tags: make(map[uint16]tiffTag, count)}
	entry := make([]byte, 12)
	for i := 0; i < count; i++ {
		entryOffset := int64(offset) + 2 + int64(i)*12
		if _, err := readAtFull(f, entry, entryOffset); err != nil {
			return tiffIFD{}, fmt.Errorf("读取 DNG IFD 条目失败: %w", err)
		}
		tag := order.Uint16(entry[0:2])
		typ := order.Uint16(entry[2:4])
		countValue := uint64(order.Uint32(entry[4:8]))
		unit, ok := tiffTypeSize(typ)
		if !ok || countValue > uint64((16<<20)/maxInt64(unit, 1)) {
			return tiffIFD{}, fmt.Errorf("DNG tag %d 的类型或长度不支持", tag)
		}
		length := countValue * uint64(unit)
		data := make([]byte, int(length))
		if length <= 4 {
			copy(data, entry[8:12])
		} else {
			valueOffset := uint64(order.Uint32(entry[8:12]))
			if valueOffset > uint64(size) || length > uint64(size)-valueOffset {
				return tiffIFD{}, fmt.Errorf("DNG tag %d 数据越界", tag)
			}
			if _, err := readAtFull(f, data, int64(valueOffset)); err != nil {
				return tiffIFD{}, fmt.Errorf("读取 DNG tag %d 失败: %w", tag, err)
			}
		}
		result.tags[tag] = tiffTag{typ: typ, count: countValue, data: data}
	}
	nextOffset := int64(offset) + 2 + int64(count)*12
	var next [4]byte
	if _, err := readAtFull(f, next[:], nextOffset); err != nil {
		return tiffIFD{}, fmt.Errorf("读取 DNG IFD next 偏移失败: %w", err)
	}
	result.next = order.Uint32(next[:])
	return result, nil
}

func readAtFull(f *os.File, dst []byte, offset int64) (int, error) {
	read := 0
	for read < len(dst) {
		n, err := f.ReadAt(dst[read:], offset+int64(read))
		read += n
		if err != nil {
			if err == io.EOF && read == len(dst) {
				return read, nil
			}
			return read, err
		}
	}
	return read, nil
}

func tiffTypeSize(typ uint16) (int, bool) {
	switch typ {
	case 1, 2, 6, 7:
		return 1, true
	case 3, 8:
		return 2, true
	case 4, 9, 11:
		return 4, true
	case 5, 10, 12:
		return 8, true
	case 16, 17, 18:
		return 8, true
	default:
		return 0, false
	}
}

func maxInt64(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func tagUint64(order binary.ByteOrder, tag tiffTag) (uint64, bool) {
	values, err := tagUint64s(order, tag)
	return firstUint(values, err)
}

func firstUint(values []uint64, err error) (uint64, bool) {
	if err != nil || len(values) == 0 {
		return 0, false
	}
	return values[0], true
}

func tagUint64s(order binary.ByteOrder, tag tiffTag) ([]uint64, error) {
	result := make([]uint64, 0, tag.count)
	for i := uint64(0); i < tag.count; i++ {
		offset, ok := tiffValueOffset(tag.typ, i)
		if !ok || offset >= len(tag.data) {
			return nil, errors.New("TIFF 数值类型不支持")
		}
		switch tag.typ {
		case 1, 6, 7:
			result = append(result, uint64(tag.data[offset]))
		case 3, 8:
			if offset+2 > len(tag.data) {
				return nil, errors.New("TIFF SHORT 越界")
			}
			result = append(result, uint64(order.Uint16(tag.data[offset:offset+2])))
		case 4, 9:
			if offset+4 > len(tag.data) {
				return nil, errors.New("TIFF LONG 越界")
			}
			result = append(result, uint64(order.Uint32(tag.data[offset:offset+4])))
		case 16, 17, 18:
			if offset+8 > len(tag.data) {
				return nil, errors.New("TIFF LONG8 越界")
			}
			result = append(result, order.Uint64(tag.data[offset:offset+8]))
		default:
			return nil, errors.New("TIFF 数值类型不支持")
		}
	}
	return result, nil
}

func tiffValueOffset(typ uint16, index uint64) (int, bool) {
	size, ok := tiffTypeSize(typ)
	maxInt := int(^uint(0) >> 1)
	if !ok || index > uint64(maxInt/size) {
		return 0, false
	}
	return int(index) * size, true
}

func tagString(tag tiffTag) string {
	if tag.typ != 2 && tag.typ != 1 && tag.typ != 7 {
		return ""
	}
	return strings.TrimSpace(strings.TrimRight(string(tag.data), "\x00"))
}

func tagFloat64s(order binary.ByteOrder, tag tiffTag) ([]float64, error) {
	values := make([]float64, 0, tag.count)
	for i := uint64(0); i < tag.count; i++ {
		offset, ok := tiffValueOffset(tag.typ, i)
		if !ok {
			return nil, errors.New("TIFF 浮点类型不支持")
		}
		switch tag.typ {
		case 5, 10:
			if offset+8 > len(tag.data) {
				return nil, errors.New("TIFF RATIONAL 越界")
			}
			numerator := order.Uint32(tag.data[offset : offset+4])
			denominator := order.Uint32(tag.data[offset+4 : offset+8])
			if denominator == 0 {
				values = append(values, 0)
				continue
			}
			values = append(values, float64(numerator)/float64(denominator))
		case 11:
			if offset+4 > len(tag.data) {
				return nil, errors.New("TIFF FLOAT 越界")
			}
			values = append(values, float64(math.Float32frombits(order.Uint32(tag.data[offset:offset+4]))))
		case 12:
			if offset+8 > len(tag.data) {
				return nil, errors.New("TIFF DOUBLE 越界")
			}
			values = append(values, math.Float64frombits(order.Uint64(tag.data[offset:offset+8])))
		default:
			ints, err := tagUint64s(order, tag)
			if err != nil {
				return nil, err
			}
			for _, value := range ints {
				values = append(values, float64(value))
			}
			return values, nil
		}
	}
	return values, nil
}

func (d *dngFile) rawIFD() tiffIFD {
	best := d.ifds[0]
	bestArea := int64(0)
	for _, ifd := range d.ifds {
		width, _ := tagUint64(d.order, ifd.tags[tagImageWidth])
		height, _ := tagUint64(d.order, ifd.tags[tagImageLength])
		photometric, _ := tagUint64(d.order, ifd.tags[tagPhotometric])
		_, hasCFA := ifd.tags[tagCFAPattern]
		area := int64(width) * int64(height)
		if hasCFA || photometric == 32803 {
			return ifd
		}
		if area > bestArea {
			best, bestArea = ifd, area
		}
	}
	return best
}

func (d *dngFile) hasDNGMarker() bool {
	for _, ifd := range d.ifds {
		for _, tag := range []uint16{tagDNGVersion, tagBlackLevel, tagWhiteLevel, tagCFARepeatPatternDim, tagCFAPattern, tagActiveArea} {
			if _, ok := ifd.tags[tag]; ok {
				return true
			}
		}
	}
	return false
}

func (d *dngFile) metadata(filename string) Metadata {
	raw := d.rawIFD()
	meta := Metadata{IsRAW: true, Format: FormatDNG, DisplayOrientation: 1, CompressionState: CompressionUnknown, Backend: "photosheet-dng", BackendVersion: "builtin-tiff-probe-1", Capabilities: (dngBackend{}).Capabilities(), ProbeAvailable: true, PreviewAvailable: false}
	if stat, err := os.Stat(filename); err == nil {
		meta.FileSize = stat.Size()
	}
	meta.EncodedWidth = int(tagValueUint(d.order, raw.tags, tagImageWidth))
	meta.EncodedHeight = int(tagValueUint(d.order, raw.tags, tagImageLength))
	meta.ActiveWidth, meta.ActiveHeight = meta.EncodedWidth, meta.EncodedHeight
	meta.CameraMake = tagString(raw.tags[tagMake])
	meta.CameraModel = tagString(raw.tags[tagModel])
	if value := tagValueUint(d.order, raw.tags, tagOrientation); value >= 1 && value <= 8 {
		meta.DisplayOrientation = int(value)
	}
	meta.NativeBitDepth = int(tagValueUint(d.order, raw.tags, tagBitsPerSample))
	if meta.NativeBitDepth == 0 {
		meta.NativeBitDepth = 16
	}
	meta.StoredBitDepth = meta.NativeBitDepth
	meta.CompressionTag = uint16(tagValueUint(d.order, raw.tags, tagCompression))
	meta.Compression, meta.CompressionState = compressionDescription(meta.CompressionTag)
	if cfa := raw.tags[tagCFAPattern]; len(cfa.data) > 0 {
		dims := raw.tags[tagCFARepeatPatternDim]
		width, height := uint64(2), uint64(2)
		if values, err := tagUint64s(d.order, dims); err == nil && len(values) >= 2 {
			width, height = values[0], values[1]
		}
		if width == 2 && height == 2 && len(cfa.data) >= 4 {
			meta.CFA = bayerName(cfa.data[:4])
		} else {
			meta.CFA = fmt.Sprintf("CFA %dx%d", width, height)
		}
	}
	if meta.CFA == "" && tagValueUint(d.order, raw.tags, tagPhotometric) == 32803 {
		meta.CFA = "CFA"
	}
	if version := raw.tags[tagDNGVersion]; len(version.data) >= 4 {
		meta.DNGVersion = fmt.Sprintf("%d.%d.%d.%d", version.data[0], version.data[1], version.data[2], version.data[3])
	}
	if meta.DNGVersion == "" {
		for _, ifd := range d.ifds {
			if version := ifd.tags[tagDNGVersion]; len(version.data) >= 4 {
				meta.DNGVersion = fmt.Sprintf("%d.%d.%d.%d", version.data[0], version.data[1], version.data[2], version.data[3])
				break
			}
		}
	}
	if level := raw.tags[tagBlackLevel]; len(level.data) > 0 {
		if values, err := tagFloat64s(d.order, level); err == nil {
			meta.BlackLevel = values
		}
	}
	if white := raw.tags[tagWhiteLevel]; len(white.data) > 0 {
		if values, err := tagFloat64s(d.order, white); err == nil && len(values) > 0 {
			meta.WhiteLevel = values[0]
		}
	}
	if active := raw.tags[tagActiveArea]; len(active.data) > 0 {
		if values, err := tagUint64s(d.order, active); err == nil && len(values) >= 4 && values[2] > values[0] && values[3] > values[1] {
			meta.ActiveY, meta.ActiveX = int(values[0]), int(values[1])
			meta.ActiveWidth, meta.ActiveHeight = int(values[3]-values[1]), int(values[2]-values[0])
		}
	}
	if crop := raw.tags[tagDefaultCropSize]; len(crop.data) > 0 {
		if values, err := tagFloat64s(d.order, crop); err == nil && len(values) >= 2 && values[0] > 0 && values[1] > 0 {
			meta.ActiveWidth, meta.ActiveHeight = int(math.Round(values[0])), int(math.Round(values[1]))
		}
	}
	meta.EmbeddedPreview = hasPreview(d, raw)
	hasRaster := len(raw.tags[tagStripOffsets].data) > 0
	if !hasRaster && len(raw.tags[tagTileOffsets].data) > 0 {
		tileWidth := tagValueUint(d.order, raw.tags, tagTileWidth)
		hasRaster = tileWidth > 0 && tileWidth == uint64(meta.EncodedWidth)
	}
	meta.Supported = meta.EncodedWidth > 0 && meta.EncodedHeight > 0 && (meta.CompressionTag == compressionNone || meta.CompressionTag == compressionDeflate) && hasRaster
	meta.DecodeAvailable = meta.Supported
	meta.EffectiveBitDepth = 0 // Pure probe cannot prove post-demosaic precision.
	if !meta.DecodeAvailable {
		meta.DecodeReason = fmt.Sprintf("pure-Go fallback does not decode compression tag %d", meta.CompressionTag)
	}
	return meta
}

func tagValueUint(order binary.ByteOrder, tags map[uint16]tiffTag, id uint16) uint64 {
	if tag, ok := tags[id]; ok {
		value, _ := tagUint64(order, tag)
		return value
	}
	return 0
}

func compressionDescription(value uint16) (string, CompressionState) {
	switch value {
	case compressionNone:
		return "Uncompressed", CompressionLossless
	case compressionJPEG:
		return "JPEG (lossless/baseline; backend must verify)", CompressionUnknown
	case compressionDeflate:
		return "Deflate", CompressionLossless
	case compressionLossyJPEG:
		return "Lossy JPEG", CompressionLossy
	case compressionJXL:
		return "JPEG XL", CompressionUnknown
	default:
		return fmt.Sprintf("Unknown (%d)", value), CompressionUnknown
	}
}

func bayerName(pattern []byte) string {
	if len(pattern) < 4 {
		return "Bayer"
	}
	key := fmt.Sprintf("%d%d%d%d", pattern[0], pattern[1], pattern[2], pattern[3])
	switch key {
	case "0101":
		return "Bayer RGGB"
	case "1010":
		return "Bayer BGGR"
	case "0110":
		return "Bayer GRBG"
	case "1001":
		return "Bayer GBRG"
	default:
		return "Bayer"
	}
}

func hasPreview(d *dngFile, raw tiffIFD) bool {
	if _, ok := raw.tags[tagPreviewImageStart]; ok {
		if _, ok = raw.tags[tagPreviewImageLength]; ok {
			return true
		}
	}
	rawWidth := tagValueUint(d.order, raw.tags, tagImageWidth)
	rawHeight := tagValueUint(d.order, raw.tags, tagImageLength)
	for _, ifd := range d.ifds {
		width := tagValueUint(d.order, ifd.tags, tagImageWidth)
		height := tagValueUint(d.order, ifd.tags, tagImageLength)
		if width > 0 && height > 0 && (width != rawWidth || height != rawHeight) && tagValueUint(d.order, ifd.tags, tagPhotometric) == 2 {
			return true
		}
	}
	return false
}
