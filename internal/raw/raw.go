// Package raw contains the RAW decoder boundary used by the scanner and the
// renderer.  The package deliberately owns no UI or project types so a
// native LibRaw/Adobe DNG SDK backend can be added without leaking CGO types
// into the rest of PhotoSheet.
package raw

import (
	"context"
	"errors"
	"fmt"
	"image"
	"path/filepath"
	"strings"
)

var (
	// Stable error taxonomy for the Go/native boundary. The legacy aliases
	// below remain for compatibility with existing callers and project files.
	ErrRawUnsupportedCompression = errors.New("RAW compression is unsupported")
	ErrRawUnsupportedVariant     = errors.New("RAW variant is unsupported")
	ErrRawCorrupt                = errors.New("RAW data is corrupt")
	ErrRawDecode                 = errors.New("RAW decode failed")
	ErrRawOutOfMemory            = errors.New("RAW decode ran out of memory")
	ErrRawCancelled              = errors.New("RAW decode was cancelled")
	ErrRawBackendUnavailable     = errors.New("RAW backend unavailable")

	ErrBackendUnavailable = ErrRawBackendUnavailable
	ErrDecodeFailed       = ErrRawDecode
)

var (
	ErrNotRAW                 = errors.New("文件不是受支持的 RAW 容器")
	ErrUnsupportedCompression = ErrRawUnsupportedCompression
	ErrUnsupportedVariant     = ErrRawUnsupportedVariant
)

type Format string

const (
	FormatDNG Format = "dng"
	FormatARW Format = "arw"
	FormatCR2 Format = "cr2"
	FormatCR3 Format = "cr3"
	FormatNEF Format = "nef"
	FormatRW2 Format = "rw2"
	FormatRAF Format = "raf"
	FormatORF Format = "orf"
	FormatPEF Format = "pef"
)

type CompressionState string

const (
	CompressionLossless CompressionState = "lossless"
	CompressionLossy    CompressionState = "lossy"
	CompressionUnknown  CompressionState = "unknown"
)

type Metadata struct {
	IsRAW              bool
	Format             Format
	CameraMake         string
	CameraModel        string
	EncodedWidth       int
	EncodedHeight      int
	ActiveWidth        int
	ActiveHeight       int
	ActiveX            int
	ActiveY            int
	DisplayOrientation int
	CFA                string
	NativeBitDepth     int
	BlackLevel         []float64
	WhiteLevel         float64
	Compression        string
	CompressionTag     uint16
	CompressionState   CompressionState
	ColorInformation   string
	EmbeddedPreview    bool
	DNGVersion         string
	FileSize           int64
	Supported          bool
	ProbeAvailable     bool
	DecodeAvailable    bool
	PreviewAvailable   bool
	DecodeReason       string
	DecodeSource       string
	StoredBitDepth     int
	EffectiveBitDepth  int
	Backend            string
	BackendVersion     string
	DNGSDKVersion      string
	Capabilities       BackendCapabilities
}

type DecodeOptions struct {
	WhiteBalance      string
	Demosaic          string
	WorkingColorSpace string
}

// BackendError preserves the native error code while exposing a stable Go
// category to scanner/UI callers.
type BackendError struct {
	Code    int32
	Kind    error
	Message string
}

func (e *BackendError) Error() string {
	if e == nil {
		return "RAW backend error"
	}
	return fmt.Sprintf("%s (native code %d): %s", e.Kind, e.Code, e.Message)
}

func (e *BackendError) Unwrap() error {
	if e == nil || e.Kind == nil {
		return ErrRawDecode
	}
	return e.Kind
}

// Decoder is intentionally small. Probe must not demosaic a complete RAW;
// Decode is only called by the final renderer.
type Decoder interface {
	Name() string
	Version() string
	Probe(context.Context, string) (Metadata, error)
	Decode(context.Context, string, DecodeOptions) (*image.NRGBA64, Metadata, error)
}

// BackendCapabilities is the explicit feature contract exposed by a RAW
// backend. Supported is retained on Metadata for project-file compatibility;
// callers should use DecodeAvailable and DecodeReason for new decisions.
type BackendCapabilities struct {
	Formats              []Format
	CompressionTags      []uint16
	ProbeSupported       bool
	DecodeSupported      bool
	PreviewSupported     bool
	Supports16BitRGB     bool
	SupportsJPEGXL       bool
	DNGSDKEnabled        bool
	DNGOpcodeProcessing  bool
	SupportsUnicodePaths bool
	OutputColorSpace     string
}

// RawBackend is the production extension point for the native LibRaw/DNG SDK
// backend and the conservative pure-Go fallback.
type RawBackend interface {
	Decoder
	Capabilities() BackendCapabilities
}

type Registry struct {
	decoders       []RawBackend
	native         RawBackend
	backendInfos   []BackendInfo
	unavailableRaw []BackendInfo
}

type BackendInfo struct {
	Name         string
	Version      string
	Available    bool
	Error        string
	Capabilities BackendCapabilities
}

type CapabilityReport struct {
	LibRawEnabled       bool
	DNGSDKEnabled       bool
	JPEGXLDNGEnabled    bool
	Output16BitEnabled  bool
	DNGOpcodeProcessing bool
	Backends            []BackendInfo
}

func NewRegistry() *Registry {
	fallback := dngBackend{}
	registry := &Registry{decoders: []RawBackend{fallback}}
	registry.backendInfos = append(registry.backendInfos, BackendInfo{
		Name: fallback.Name(), Version: fallback.Version(), Available: true,
		Capabilities: fallback.Capabilities(),
	})
	if native, nativeErr := newNativeBackend(); native != nil {
		registry.native = native
		registry.decoders = append([]RawBackend{native}, registry.decoders...)
		registry.backendInfos = append([]BackendInfo{{
			Name: native.Name(), Version: native.Version(), Available: true,
			Capabilities: native.Capabilities(),
		}}, registry.backendInfos...)
	} else if nativeErr != nil {
		registry.unavailableRaw = append(registry.unavailableRaw, BackendInfo{
			Name: nativeBackendName, Version: nativeBackendVersion, Available: false,
			Error: nativeErr.Error(), Capabilities: nativeBackend{}.Capabilities(),
		})
	}
	return registry
}

func (r *Registry) Backends() []BackendInfo {
	result := make([]BackendInfo, 0, len(r.backendInfos)+len(r.unavailableRaw))
	result = append(result, r.backendInfos...)
	result = append(result, r.unavailableRaw...)
	return result
}

func (r *Registry) CapabilityReport() CapabilityReport {
	report := CapabilityReport{Backends: r.Backends()}
	for _, backend := range report.Backends {
		if !backend.Available {
			continue
		}
		if backend.Name == nativeBackendName {
			report.LibRawEnabled = true
		}
		report.DNGSDKEnabled = report.DNGSDKEnabled || backend.Capabilities.DNGSDKEnabled
		report.JPEGXLDNGEnabled = report.JPEGXLDNGEnabled || backend.Capabilities.SupportsJPEGXL
		report.Output16BitEnabled = report.Output16BitEnabled || backend.Capabilities.Supports16BitRGB
		report.DNGOpcodeProcessing = report.DNGOpcodeProcessing || backend.Capabilities.DNGOpcodeProcessing
	}
	return report
}

// Resolve performs capability-aware backend selection. Extension is only
// used to narrow candidates; the backend's Probe result remains authoritative.
func (r *Registry) Resolve(ctx context.Context, filename string) (RawBackend, Metadata, error) {
	format := Format(strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), "."))
	var lastErr error
	for _, decoder := range r.decoders {
		if !supportsFormat(decoder.Capabilities(), format) {
			continue
		}
		meta, err := decoder.Probe(ctx, filename)
		if err == nil {
			return decoder, meta, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, Metadata{}, lastErr
	}
	return nil, Metadata{}, fmt.Errorf("没有可用 RAW backend 读取 %s", filename)
}

func supportsFormat(capabilities BackendCapabilities, format Format) bool {
	for _, candidate := range capabilities.Formats {
		if candidate == format {
			return true
		}
	}
	return false
}

func (r *Registry) DecoderFor(filename string) (Decoder, error) {
	format := Format(strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), "."))
	for _, decoder := range r.decoders {
		if supportsFormat(decoder.Capabilities(), format) {
			return decoder, nil
		}
	}
	return nil, fmt.Errorf("没有可用 RAW backend 读取 %s", filename)
}

func (r *Registry) Probe(ctx context.Context, filename string) (Metadata, error) {
	_, meta, err := r.Resolve(ctx, filename)
	return meta, err
}

func (r *Registry) Decode(ctx context.Context, filename string, options DecodeOptions) (*image.NRGBA64, Metadata, error) {
	backend, meta, resolveErr := r.Resolve(ctx, filename)
	if resolveErr != nil {
		return nil, Metadata{}, resolveErr
	}
	decoded, decodedMeta, err := backend.Decode(ctx, filename, options)
	if err == nil {
		return decoded, decodedMeta, nil
	}
	// Native is preferred for every DNG. The pure-Go implementation is a
	// genuine fallback only for the formats it actually decodes, not a way to
	// hide a failed JPEG/JXL native path.
	if backend.Name() == nativeBackendName && IsDNGPath(filename) &&
		(meta.CompressionTag == compressionNone || meta.CompressionTag == compressionDeflate) {
		fallback := dngBackend{}
		if fallbackMeta, fallbackErr := fallback.Probe(ctx, filename); fallbackErr == nil && fallbackMeta.DecodeAvailable {
			return fallback.Decode(ctx, filename, options)
		}
	}
	if IsDNGPath(filename) &&
		(meta.CompressionTag == compressionJPEG || meta.CompressionTag == compressionLossyJPEG || meta.CompressionTag == compressionJXL) &&
		errors.Is(err, ErrRawUnsupportedVariant) {
		return nil, meta, fmt.Errorf("%w: %s: %v", ErrRawUnsupportedCompression, meta.Compression, err)
	}
	return nil, meta, err
}

func IsRAWPath(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".dng", ".arw", ".cr2", ".cr3", ".nef", ".rw2", ".raf", ".orf", ".pef":
		return true
	default:
		return false
	}
}

func IsDNGPath(filename string) bool { return strings.EqualFold(filepath.Ext(filename), ".dng") }
