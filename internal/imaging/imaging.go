package imaging

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"strings"

	"golang.org/x/image/tiff"
	"photosheet/internal/metadata"
	"photosheet/internal/raw"
)

// Codec is the extension point for future PNG/TIFF/HEIC/AVIF/RAW preview codecs.
// Core packing and extraction code only deals with normalized pixel images.
type Codec interface {
	Name() string
	Extensions() []string
	DecodeConfig(filename string) (image.Config, error)
	Decode(filename string) (image.Image, error)
}

type Registry struct {
	codecs []Codec
}

func NewRegistry() *Registry {
	return &Registry{codecs: []Codec{jpegCodec{}, pngCodec{}, tiffCodec{}, rawCodec{}}}
}

func (r *Registry) CodecFor(filename string) (Codec, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepathExt(filename), "."))
	for _, codec := range r.codecs {
		for _, candidate := range codec.Extensions() {
			if ext == strings.ToLower(strings.TrimPrefix(candidate, ".")) {
				return codec, nil
			}
		}
	}
	return nil, fmt.Errorf("没有可用 codec 读取 %s", filename)
}

// filepathExt is kept tiny so imaging's codec registry remains easy to test.
func filepathExt(filename string) string {
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '/' || filename[i] == '\\' {
			break
		}
		if filename[i] == '.' {
			return filename[i:]
		}
	}
	return ""
}

type jpegCodec struct{}

func (jpegCodec) Name() string         { return "jpeg" }
func (jpegCodec) Extensions() []string { return []string{".jpg", ".jpeg"} }
func (jpegCodec) DecodeConfig(filename string) (image.Config, error) {
	f, err := os.Open(filename)
	if err != nil {
		return image.Config{}, err
	}
	defer f.Close()
	return jpeg.DecodeConfig(f)
}
func (jpegCodec) Decode(filename string) (image.Image, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	result, _, err := image.Decode(f)
	return result, err
}

type pngCodec struct{}

func (pngCodec) Name() string         { return "png" }
func (pngCodec) Extensions() []string { return []string{".png"} }
func (pngCodec) DecodeConfig(filename string) (image.Config, error) {
	f, err := os.Open(filename)
	if err != nil {
		return image.Config{}, err
	}
	defer f.Close()
	return png.DecodeConfig(f)
}
func (pngCodec) Decode(filename string) (image.Image, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	result, _, err := image.Decode(f)
	return result, err
}

type tiffCodec struct{}

func (tiffCodec) Name() string         { return "tiff" }
func (tiffCodec) Extensions() []string { return []string{".tif", ".tiff"} }
func (tiffCodec) DecodeConfig(filename string) (image.Config, error) {
	f, err := os.Open(filename)
	if err != nil {
		return image.Config{}, err
	}
	defer f.Close()
	return tiff.DecodeConfig(f)
}
func (tiffCodec) Decode(filename string) (image.Image, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return tiff.Decode(f)
}

type rawCodec struct{}

func (rawCodec) Name() string {
	return "raw"
}
func (rawCodec) Extensions() []string {
	return []string{".dng", ".arw", ".cr2", ".cr3", ".nef", ".rw2", ".raf", ".orf", ".pef"}
}
func (rawCodec) DecodeConfig(filename string) (image.Config, error) {
	meta, err := raw.NewRegistry().Probe(context.Background(), filename)
	if err != nil {
		return image.Config{}, err
	}
	if !meta.IsRAW || meta.EncodedWidth <= 0 || meta.EncodedHeight <= 0 {
		return image.Config{}, errors.New("文件不是有效 DNG")
	}
	return image.Config{ColorModel: color.RGBA64Model, Width: meta.EncodedWidth, Height: meta.EncodedHeight}, nil
}
func (rawCodec) Decode(filename string) (image.Image, error) {
	decoded, _, err := raw.NewRegistry().Decode(context.Background(), filename, raw.DecodeOptions{WorkingColorSpace: "sRGB"})
	return decoded, err
}

// FileInspection is the inexpensive metadata result used by the processed
// Sheet resolver. It validates the file header and reads dimensions without
// allocating the complete pixel matrix.
type FileInspection struct {
	Format        string
	EncodedWidth  int
	EncodedHeight int
	DisplayWidth  int
	DisplayHeight int
	Orientation   int
	ICCProfile    []byte
	BitDepth      int
	Compression   string
}

func SupportedProcessedExtension(filename string) bool {
	switch strings.ToLower(filepathExt(filename)) {
	case ".jpg", ".jpeg", ".png", ".tif", ".tiff":
		return true
	default:
		return false
	}
}

// InspectFile trusts the decoded format marker rather than the filename
// extension. This catches renamed/corrupt candidates while still permitting a
// valid PNG or TIFF whose external tool changed the extension.
func InspectFile(filename string) (FileInspection, error) {
	if raw.IsRAWPath(filename) {
		meta, err := raw.NewRegistry().Probe(context.Background(), filename)
		if err != nil {
			return FileInspection{}, fmt.Errorf("读取 DNG metadata 失败: %w", err)
		}
		width, height := meta.EncodedWidth, meta.EncodedHeight
		if meta.ActiveWidth > 0 && meta.ActiveHeight > 0 {
			width, height = meta.ActiveWidth, meta.ActiveHeight
		}
		if meta.DisplayOrientation >= 5 && meta.DisplayOrientation <= 8 {
			width, height = height, width
		}
		return FileInspection{Format: "dng", EncodedWidth: meta.EncodedWidth, EncodedHeight: meta.EncodedHeight, DisplayWidth: width, DisplayHeight: height, Orientation: meta.DisplayOrientation, BitDepth: meta.NativeBitDepth, Compression: meta.Compression}, nil
	}
	f, err := os.Open(filename)
	if err != nil {
		return FileInspection{}, fmt.Errorf("打开图片失败: %w", err)
	}
	defer f.Close()
	config, format, err := image.DecodeConfig(f)
	if err != nil {
		return FileInspection{}, fmt.Errorf("读取图片尺寸失败: %w", err)
	}
	if format != "jpeg" && format != "jpg" && format != "png" && format != "tiff" {
		return FileInspection{}, fmt.Errorf("不支持的图片格式 %q", format)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return FileInspection{}, errors.New("图片尺寸无效")
	}
	orientation := 1
	var profile []byte
	compression := ""
	if format == "jpeg" || format == "jpg" {
		meta, metaErr := metadata.ReadJPEGMetadata(filename)
		if metaErr != nil {
			return FileInspection{}, fmt.Errorf("读取 JPEG 元数据失败: %w", metaErr)
		}
		orientation = meta.Orientation
		profile = meta.ICCProfile
	} else if format == "png" {
		profile, _ = metadata.ReadPNGICCProfile(filename)
	} else if format == "tiff" {
		profile, _ = metadata.ReadTIFFICProfile(filename)
		if value, compressionErr := metadata.ReadTIFFCompression(filename); compressionErr == nil {
			compression = fmt.Sprintf("TIFF compression %d", value)
		}
	}
	displayWidth, displayHeight := config.Width, config.Height
	if orientation >= 5 && orientation <= 8 {
		displayWidth, displayHeight = displayHeight, displayWidth
	}
	return FileInspection{
		Format:        format,
		EncodedWidth:  config.Width,
		EncodedHeight: config.Height,
		DisplayWidth:  displayWidth,
		DisplayHeight: displayHeight,
		Orientation:   orientation,
		ICCProfile:    profile,
		BitDepth:      bitDepthForColorModel(config.ColorModel),
		Compression:   compression,
	}, nil
}

func bitDepthForColorModel(model color.Model) int {
	switch model {
	case color.RGBA64Model, color.NRGBA64Model, color.Gray16Model:
		return 16
	default:
		return 8
	}
}

func DecodeFile(filename string) (*image.NRGBA, error) {
	if raw.IsRAWPath(filename) {
		decoded, _, err := DecodeNormalizedFile16WithOptions(filename, raw.DecodeOptions{WorkingColorSpace: "sRGB"})
		if err != nil {
			return nil, err
		}
		return ToNRGBA(decoded), nil
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("打开图片失败: %w", err)
	}
	defer f.Close()
	decoded, format, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("解码图片失败: %w", err)
	}
	result := ToNRGBA(decoded)
	if format == "jpeg" || format == "jpg" {
		meta, metaErr := metadata.ReadJPEGMetadata(filename)
		if metaErr != nil {
			return nil, fmt.Errorf("读取 JPEG Orientation 失败: %w", metaErr)
		}
		result = TransformOrientation(result, meta.Orientation)
	}
	return result, nil
}

func DecodeNormalizedFile(filename string) (*image.NRGBA, []byte, error) {
	if raw.IsRAWPath(filename) {
		decoded, profile, err := DecodeNormalizedFile16WithOptions(filename, raw.DecodeOptions{WorkingColorSpace: "sRGB"})
		if err != nil {
			return nil, nil, err
		}
		return ToNRGBA(decoded), profile, nil
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, nil, fmt.Errorf("打开图片失败: %w", err)
	}
	defer f.Close()
	decoded, format, err := image.Decode(f)
	if err != nil {
		return nil, nil, fmt.Errorf("解码图片失败: %w", err)
	}
	result := ToNRGBA(decoded)
	var profile []byte
	if format == "jpeg" || format == "jpg" {
		meta, metaErr := metadata.ReadJPEGMetadata(filename)
		if metaErr != nil {
			return nil, nil, fmt.Errorf("读取 JPEG 元数据失败: %w", metaErr)
		}
		result = TransformOrientation(result, meta.Orientation)
		profile = meta.ICCProfile
	} else if format == "png" {
		profile, _ = metadata.ReadPNGICCProfile(filename)
	} else if format == "tiff" {
		profile, _ = metadata.ReadTIFFICProfile(filename)
	}
	return result, profile, nil
}

func DecodeNormalizedFile16(filename string) (*image.NRGBA64, []byte, error) {
	return DecodeNormalizedFile16Context(context.Background(), filename, raw.DecodeOptions{WorkingColorSpace: "sRGB"})
}

func DecodeNormalizedFile16WithOptions(filename string, options raw.DecodeOptions) (*image.NRGBA64, []byte, error) {
	return DecodeNormalizedFile16Context(context.Background(), filename, options)
}

func DecodeNormalizedFile16Context(ctx context.Context, filename string, options raw.DecodeOptions) (*image.NRGBA64, []byte, error) {
	if raw.IsRAWPath(filename) {
		decoded, _, err := raw.NewRegistry().Decode(ctx, filename, options)
		return decoded, nil, err
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, nil, fmt.Errorf("打开图片失败: %w", err)
	}
	defer f.Close()
	decoded, format, err := image.Decode(f)
	if err != nil {
		return nil, nil, fmt.Errorf("解码图片失败: %w", err)
	}
	result := ToNRGBA64(decoded)
	var profile []byte
	if format == "jpeg" || format == "jpg" {
		meta, metaErr := metadata.ReadJPEGMetadata(filename)
		if metaErr != nil {
			return nil, nil, fmt.Errorf("读取 JPEG 元数据失败: %w", metaErr)
		}
		result = TransformOrientation16(result, meta.Orientation)
		profile = meta.ICCProfile
	} else if format == "png" {
		profile, _ = metadata.ReadPNGICCProfile(filename)
	} else if format == "tiff" {
		profile, _ = metadata.ReadTIFFICProfile(filename)
	}
	return result, profile, nil
}

func ToNRGBA(src image.Image) *image.NRGBA {
	if nrgba, ok := src.(*image.NRGBA); ok {
		copyImage := image.NewNRGBA(nrgba.Bounds())
		copy(copyImage.Pix, nrgba.Pix)
		return copyImage
	}
	bounds := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			dst.SetNRGBA(x, y, colorToNRGBA(src.At(bounds.Min.X+x, bounds.Min.Y+y)))
		}
	}
	return dst
}

func ToNRGBA64(src image.Image) *image.NRGBA64 {
	if nrgba, ok := src.(*image.NRGBA64); ok {
		copyImage := image.NewNRGBA64(nrgba.Bounds())
		copy(copyImage.Pix, nrgba.Pix)
		return copyImage
	}
	bounds := src.Bounds()
	dst := image.NewNRGBA64(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			dst.SetNRGBA64(x, y, colorToNRGBA64(src.At(bounds.Min.X+x, bounds.Min.Y+y)))
		}
	}
	return dst
}

func colorToNRGBA(c color.Color) color.NRGBA {
	r, g, b, a := c.RGBA()
	if a == 0 {
		return color.NRGBA{}
	}
	if a != 0xffff {
		r = min32(0xffff, (r*0xffff+a/2)/a)
		g = min32(0xffff, (g*0xffff+a/2)/a)
		b = min32(0xffff, (b*0xffff+a/2)/a)
	}
	return color.NRGBA{R: uint8((r + 128) / 257), G: uint8((g + 128) / 257), B: uint8((b + 128) / 257), A: uint8((a + 128) / 257)}
}

func colorToNRGBA64(c color.Color) color.NRGBA64 {
	r, g, b, a := c.RGBA()
	if a == 0 {
		return color.NRGBA64{}
	}
	if a != 0xffff {
		r = min32(0xffff, (r*0xffff+a/2)/a)
		g = min32(0xffff, (g*0xffff+a/2)/a)
		b = min32(0xffff, (b*0xffff+a/2)/a)
	}
	return color.NRGBA64{R: uint16(r), G: uint16(g), B: uint16(b), A: uint16(a)}
}

func min32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

func TransformOrientation(src *image.NRGBA, orientation int) *image.NRGBA {
	if src == nil {
		return nil
	}
	if orientation < 1 || orientation > 8 {
		orientation = 1
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := w, h
	if orientation >= 5 && orientation <= 8 {
		dw, dh = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			sx, sy := orientationSourceCoordinate(x, y, w, h, orientation)
			dst.SetNRGBA(x, y, src.NRGBAAt(sx, sy))
		}
	}
	return dst
}

func TransformOrientation16(src *image.NRGBA64, orientation int) *image.NRGBA64 {
	if src == nil {
		return nil
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := w, h
	if orientation >= 5 && orientation <= 8 {
		dw, dh = h, w
	}
	if orientation < 1 || orientation > 8 {
		orientation = 1
	}
	dst := image.NewNRGBA64(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			sx, sy := orientationSourceCoordinate(x, y, w, h, orientation)
			dst.SetNRGBA64(x, y, src.NRGBA64At(sx, sy))
		}
	}
	return dst
}

func orientationSourceCoordinate(x, y, w, h, orientation int) (int, int) {
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

func Rotate90CW(src *image.NRGBA) *image.NRGBA {
	if src == nil {
		return nil
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.SetNRGBA(h-1-y, x, src.NRGBAAt(x, y))
		}
	}
	return dst
}

func Rotate90CCW(src *image.NRGBA) *image.NRGBA {
	if src == nil {
		return nil
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.SetNRGBA(y, w-1-x, src.NRGBAAt(x, y))
		}
	}
	return dst
}

func Rotate90CW16(src *image.NRGBA64) *image.NRGBA64 {
	if src == nil {
		return nil
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewNRGBA64(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.SetNRGBA64(h-1-y, x, src.NRGBA64At(x, y))
		}
	}
	return dst
}
func Rotate90CCW16(src *image.NRGBA64) *image.NRGBA64 {
	if src == nil {
		return nil
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewNRGBA64(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.SetNRGBA64(y, w-1-x, src.NRGBA64At(x, y))
		}
	}
	return dst
}

func Crop(src *image.NRGBA, rect image.Rectangle) (*image.NRGBA, error) {
	if src == nil {
		return nil, errors.New("不能裁剪空图片")
	}
	if rect.Min.X < 0 || rect.Min.Y < 0 || rect.Max.X > src.Bounds().Dx() || rect.Max.Y > src.Bounds().Dy() || rect.Empty() {
		return nil, fmt.Errorf("裁剪矩形越界: %v", rect)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := 0; y < rect.Dy(); y++ {
		for x := 0; x < rect.Dx(); x++ {
			dst.SetNRGBA(x, y, src.NRGBAAt(rect.Min.X+x, rect.Min.Y+y))
		}
	}
	return dst, nil
}

func Crop16(src *image.NRGBA64, rect image.Rectangle) (*image.NRGBA64, error) {
	if src == nil {
		return nil, errors.New("不能裁剪空图片")
	}
	if rect.Min.X < 0 || rect.Min.Y < 0 || rect.Max.X > src.Bounds().Dx() || rect.Max.Y > src.Bounds().Dy() || rect.Empty() {
		return nil, fmt.Errorf("裁剪矩形越界: %v", rect)
	}
	dst := image.NewNRGBA64(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := 0; y < rect.Dy(); y++ {
		for x := 0; x < rect.Dx(); x++ {
			dst.SetNRGBA64(x, y, src.NRGBA64At(rect.Min.X+x, rect.Min.Y+y))
		}
	}
	return dst, nil
}

func EqualPixels(a, b *image.NRGBA) (bool, int) {
	if a == nil || b == nil || a.Bounds().Dx() != b.Bounds().Dx() || a.Bounds().Dy() != b.Bounds().Dy() {
		return false, -1
	}
	diff := 0
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			if a.NRGBAAt(x, y) != b.NRGBAAt(x, y) {
				diff++
			}
		}
	}
	return diff == 0, diff
}

func EqualPixels16(a, b *image.NRGBA64) (bool, int) {
	if a == nil || b == nil || a.Bounds().Dx() != b.Bounds().Dx() || a.Bounds().Dy() != b.Bounds().Dy() {
		return false, -1
	}
	diff := 0
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			if a.NRGBA64At(x, y) != b.NRGBA64At(x, y) {
				diff++
			}
		}
	}
	return diff == 0, diff
}
