package raw

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
)

const (
	nativeBackendName    = "libraw-dngsdk"
	nativeBackendVersion = "LibRaw 0.22.2 + Adobe DNG SDK 1.7.1 + JPEG XL"
	nativeDNGSDKVersion  = "1.7.1"
)

type nativeProbeInfo struct {
	width       uint32
	height      uint32
	channels    uint32
	bits        uint32
	orientation uint32
}

type nativeBackend struct {
	capabilities BackendCapabilities
}

func (nativeBackend) Name() string    { return nativeBackendName }
func (nativeBackend) Version() string { return nativeBackendVersion }
func (backend nativeBackend) Capabilities() BackendCapabilities {
	if backend.capabilities.Formats != nil {
		return backend.capabilities
	}
	return BackendCapabilities{
		Formats: []Format{
			FormatDNG, FormatARW, FormatCR2, FormatCR3, FormatNEF,
			FormatRW2, FormatRAF, FormatORF, FormatPEF,
		},
		CompressionTags:      []uint16{compressionNone, compressionDeflate, compressionJPEG, compressionLossyJPEG, compressionJXL},
		ProbeSupported:       true,
		DecodeSupported:      true,
		PreviewSupported:     false,
		Supports16BitRGB:     true,
		SupportsJPEGXL:       true,
		DNGSDKEnabled:        true,
		DNGOpcodeProcessing:  true,
		SupportsUnicodePaths: true,
		OutputColorSpace:     "sRGB",
	}
}

func (backend nativeBackend) Probe(ctx context.Context, filename string) (Metadata, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}
	capabilities := backend.Capabilities()
	if IsDNGPath(filename) {
		meta, err := (dngBackend{}).Probe(ctx, filename)
		if err != nil {
			return Metadata{}, err
		}
		meta.Capabilities = capabilities
		meta.ProbeAvailable = true
		info, probeErr := nativeProbe(filename)
		if probeErr != nil {
			// A valid uncompressed/Deflate DNG remains usable by the pure-Go
			// fallback. Keep its metadata while retaining the native diagnostic.
			if meta.Supported {
				meta.DecodeAvailable = true
				meta.DecodeReason = "native probe failed; using pure-Go fallback: " + probeErr.Error()
				return meta, nil
			}
			meta.DecodeAvailable = false
			meta.DecodeReason = probeErr.Error()
			return meta, nil
		}
		if info.width == 0 || info.height == 0 {
			meta.DecodeAvailable = false
			meta.DecodeReason = "native backend reported an empty image"
			return meta, nil
		}
		if info.bits > 0 {
			meta.EffectiveBitDepth = int(info.bits)
			meta.NativeBitDepth = int(info.bits)
		}
		// LibRaw returns the display-oriented raster. Some externally produced
		// DNGs omit Orientation while their native pipeline still rotates the
		// result; keep scanner dimensions consistent with the actual decode.
		if info.orientation >= 1 && info.orientation <= 8 {
			meta.DisplayOrientation = int(info.orientation)
		} else if meta.DisplayOrientation == 1 && info.width == uint32(meta.EncodedHeight) && info.height == uint32(meta.EncodedWidth) {
			meta.DisplayOrientation = 6
		}
		meta.Supported = true
		meta.DecodeAvailable = true
		meta.DecodeReason = ""
		meta.Backend = backend.Name()
		meta.BackendVersion = backend.Version()
		meta.DNGSDKVersion = nativeDNGSDKVersion
		meta.DecodeSource = "LibRaw + Adobe DNG SDK raw pixel pipeline"
		return meta, nil
	}

	info, err := nativeProbe(filename)
	if err != nil {
		return Metadata{}, fmt.Errorf("%w: %v", ErrDecodeFailed, err)
	}
	format := Format(strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), "."))
	meta := Metadata{
		IsRAW:              true,
		Format:             format,
		EncodedWidth:       int(info.width),
		EncodedHeight:      int(info.height),
		ActiveWidth:        int(info.width),
		ActiveHeight:       int(info.height),
		DisplayOrientation: 1,
		NativeBitDepth:     int(info.bits),
		FileSize:           fileSize(filename),
		Supported:          true,
		ProbeAvailable:     true,
		DecodeAvailable:    true,
		PreviewAvailable:   false,
		EffectiveBitDepth:  int(info.bits),
		DecodeSource:       "LibRaw raw pixel pipeline",
		Backend:            backend.Name(),
		BackendVersion:     backend.Version(),
		DNGSDKVersion:      nativeDNGSDKVersion,
		Capabilities:       capabilities,
	}
	if meta.NativeBitDepth == 0 {
		meta.NativeBitDepth = 16
	}
	meta.StoredBitDepth = meta.NativeBitDepth
	return meta, nil
}

func (nativeBackend) Decode(ctx context.Context, filename string, options DecodeOptions) (*image.NRGBA64, Metadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, Metadata{}, err
	}
	if options.WorkingColorSpace != "" && options.WorkingColorSpace != "sRGB" {
		return nil, Metadata{}, fmt.Errorf("%w: native backend currently outputs sRGB, requested %s", ErrUnsupportedVariant, options.WorkingColorSpace)
	}
	result, err := nativeDecode(ctx, filename)
	if err != nil {
		if errors.Is(err, ErrRawCorrupt) || errors.Is(err, ErrRawUnsupportedVariant) ||
			errors.Is(err, ErrRawUnsupportedCompression) || errors.Is(err, ErrRawOutOfMemory) ||
			errors.Is(err, ErrRawCancelled) || errors.Is(err, ErrRawBackendUnavailable) {
			return nil, Metadata{}, err
		}
		return nil, Metadata{}, fmt.Errorf("%w: %v", ErrRawDecode, err)
	}
	if result.info.width == 0 || result.info.height == 0 || result.info.channels != 3 || result.info.bits != 16 {
		return nil, Metadata{}, fmt.Errorf("%w: native output is %dx%d, %d channels, %d-bit", ErrUnsupportedVariant, result.info.width, result.info.height, result.info.channels, result.info.bits)
	}
	if err := ctx.Err(); err != nil {
		return nil, Metadata{}, err
	}
	if uint64(len(result.data)) != uint64(result.info.width)*uint64(result.info.height)*6 {
		return nil, Metadata{}, fmt.Errorf("%w: native RGB16 buffer has unexpected size %d", ErrDecodeFailed, len(result.data))
	}

	width, height := int(result.info.width), int(result.info.height)
	img := image.NewNRGBA64(image.Rect(0, 0, width, height))
	for pixel := 0; pixel < width*height; pixel++ {
		src := pixel * 6
		dst := pixel * 8
		// LibRaw returns native-endian uint16 samples. image.NRGBA64 stores
		// each channel in big-endian byte order in Pix.
		img.Pix[dst+0], img.Pix[dst+1] = result.data[src+1], result.data[src+0]
		img.Pix[dst+2], img.Pix[dst+3] = result.data[src+3], result.data[src+2]
		img.Pix[dst+4], img.Pix[dst+5] = result.data[src+5], result.data[src+4]
		img.Pix[dst+6], img.Pix[dst+7] = 0xff, 0xff
	}

	meta, probeErr := (nativeBackend{}).Probe(ctx, filename)
	if probeErr != nil {
		meta = Metadata{
			IsRAW: true, Format: Format(strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")),
			EncodedWidth: width, EncodedHeight: height, ActiveWidth: width, ActiveHeight: height,
			DisplayOrientation: 1, NativeBitDepth: 16, StoredBitDepth: 16, EffectiveBitDepth: 16, Supported: true, ProbeAvailable: true, DecodeAvailable: true,
			Backend: nativeBackendName, BackendVersion: nativeBackendVersion, Capabilities: nativeBackend{}.Capabilities(),
			DNGSDKVersion: nativeDNGSDKVersion,
		}
	}
	meta.EffectiveBitDepth = int(result.info.bits)
	meta.NativeBitDepth = int(result.info.bits)
	meta.DecodeAvailable = true
	meta.ProbeAvailable = true
	meta.DecodeSource = "LibRaw + Adobe DNG SDK raw pixel pipeline"
	return img, meta, nil
}

func fileSize(filename string) int64 {
	info, err := os.Stat(filename)
	if err != nil {
		return 0
	}
	return info.Size()
}
