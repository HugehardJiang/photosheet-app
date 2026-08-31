package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// CurrentSchemaVersion is incremented when the on-disk project shape gains
// fields that are needed for deterministic, backwards-compatible resolution.
const CurrentSchemaVersion = 3

type Mode string

const (
	ModeStrictLossless    Mode = "strict-lossless"
	ModeJPEGCompatibility Mode = "jpeg-compatibility"
)

type SheetFormat string

const (
	FormatPNG  SheetFormat = "png"
	FormatJPEG SheetFormat = "jpg"
	FormatTIFF SheetFormat = "tiff"
)

type StorageMode string

const (
	StorageLowDisk     StorageMode = "low-disk"
	StorageBalanced    StorageMode = "balanced"
	StoragePerformance StorageMode = "performance"
)

type TIFFCompression string

const (
	TIFFZIP          TIFFCompression = "zip"
	TIFFLZW          TIFFCompression = "lzw"
	TIFFUncompressed TIFFCompression = "none"
)

type WhiteBalanceMode string

const (
	WhiteBalanceCamera  WhiteBalanceMode = "camera"
	WhiteBalanceNeutral WhiteBalanceMode = "neutral"
	WhiteBalanceCustom  WhiteBalanceMode = "custom"
)

type DemosaicQuality string

const (
	DemosaicFast     DemosaicQuality = "fast"
	DemosaicBalanced DemosaicQuality = "balanced"
	DemosaicHigh     DemosaicQuality = "high"
)

type RawProcessingSettings struct {
	WhiteBalance       WhiteBalanceMode `json:"whiteBalance"`
	Demosaic           DemosaicQuality  `json:"demosaic"`
	AutoExposure       bool             `json:"autoExposure"`
	AutoContrast       bool             `json:"autoContrast"`
	Sharpen            bool             `json:"sharpen"`
	NoiseReduction     bool             `json:"noiseReduction"`
	CustomWhiteBalance []float64        `json:"customWhiteBalance,omitempty"`
}

// OutputFormat describes the format selected while extracting restored
// photos. It is deliberately separate from SheetFormat: a processed Sheet can
// be JPEG while the restored files are written as PNG (the default).
type OutputFormat string

const (
	OutputPNG  OutputFormat = "png"
	OutputJPEG OutputFormat = "jpg"
	OutputSame OutputFormat = "same"
	OutputTIFF OutputFormat = "tiff"
)

type PaddingMode string

const (
	PaddingSolid         PaddingMode = "solid"
	PaddingEdgeExtension PaddingMode = "edge-extension"
)

type SortStrategy string

const (
	SortAreaDescending    SortStrategy = "area-descending"
	SortMaxSideDescending SortStrategy = "max-side-descending"
	SortWidthDescending   SortStrategy = "width-descending"
	SortHeightDescending  SortStrategy = "height-descending"
	SortAspectRatio       SortStrategy = "aspect-ratio"
	SortPortraitLandscape SortStrategy = "portrait-landscape"
)

type Heuristic string

const (
	HeuristicBestShortSideFit Heuristic = "best-short-side-fit"
	HeuristicBestLongSideFit  Heuristic = "best-long-side-fit"
	HeuristicBestAreaFit      Heuristic = "best-area-fit"
	HeuristicBottomLeft       Heuristic = "bottom-left"
	HeuristicContactPoint     Heuristic = "contact-point"
)

type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (r Rect) Right() int  { return r.X + r.Width }
func (r Rect) Bottom() int { return r.Y + r.Height }
func (r Rect) Area() int64 { return int64(r.Width) * int64(r.Height) }

func (r Rect) Empty() bool { return r.Width <= 0 || r.Height <= 0 }

func (r Rect) Contains(other Rect) bool {
	return r.X <= other.X && r.Y <= other.Y && r.Right() >= other.Right() && r.Bottom() >= other.Bottom()
}

type Settings struct {
	MaximumLongEdge    int64                 `json:"maximumLongEdge"`
	MaximumShortEdge   int64                 `json:"maximumShortEdge,omitempty"`
	MaximumTotalPixels int64                 `json:"maximumTotalPixels,omitempty"`
	Mode               Mode                  `json:"mode"`
	Format             SheetFormat           `json:"format"`
	Padding            int                   `json:"padding"`
	PaddingMode        PaddingMode           `json:"paddingMode"`
	AllowRotation      bool                  `json:"allowRotation"`
	MemoryLimitBytes   int64                 `json:"memoryLimitBytes,omitempty"`
	TileSize           int                   `json:"tileSize,omitempty"`
	JPEGQuality        int                   `json:"jpegQuality,omitempty"`
	SortStrategy       SortStrategy          `json:"sortStrategy,omitempty"`
	Heuristic          Heuristic             `json:"heuristic,omitempty"`
	WorkingBitDepth    int                   `json:"workingBitDepth,omitempty"`
	WorkingColorSpace  string                `json:"workingColorSpace,omitempty"`
	TIFFCompression    TIFFCompression       `json:"tiffCompression,omitempty"`
	StorageMode        StorageMode           `json:"storageMode,omitempty"`
	RawProcessing      RawProcessingSettings `json:"rawProcessing,omitempty"`
}

func DefaultSettings() Settings {
	return Settings{
		MaximumLongEdge:   16384,
		Mode:              ModeStrictLossless,
		Format:            FormatPNG,
		PaddingMode:       PaddingSolid,
		TileSize:          512,
		JPEGQuality:       95,
		SortStrategy:      SortAreaDescending,
		Heuristic:         HeuristicBestShortSideFit,
		WorkingBitDepth:   16,
		WorkingColorSpace: "sRGB",
		TIFFCompression:   TIFFZIP,
		StorageMode:       StorageLowDisk,
		RawProcessing: RawProcessingSettings{
			WhiteBalance: WhiteBalanceCamera,
			Demosaic:     DemosaicBalanced,
		},
	}
}

func (s Settings) Normalize() (Settings, error) {
	if s.MaximumLongEdge < 0 || s.MaximumShortEdge < 0 || s.MaximumTotalPixels < 0 {
		return Settings{}, errors.New("最大尺寸和像素数不能为负数")
	}
	if s.MaximumLongEdge == 0 && s.MaximumShortEdge == 0 && s.MaximumTotalPixels == 0 {
		return Settings{}, errors.New("至少需要设置一个最大尺寸约束")
	}
	if s.MaximumLongEdge == 0 {
		if s.MaximumShortEdge > 0 {
			s.MaximumLongEdge = s.MaximumShortEdge
		} else {
			// A square is a deterministic, safe canvas when only total pixels is set.
			s.MaximumLongEdge = integerCeilSqrt(s.MaximumTotalPixels)
		}
	}
	if s.MaximumShortEdge == 0 {
		s.MaximumShortEdge = s.MaximumLongEdge
	}
	if s.MaximumLongEdge <= 0 || s.MaximumShortEdge <= 0 {
		return Settings{}, errors.New("最大边长必须大于 0")
	}
	if s.MaximumShortEdge > s.MaximumLongEdge {
		s.MaximumShortEdge = s.MaximumLongEdge
	}
	if s.Mode == "" {
		s.Mode = ModeStrictLossless
	}
	if s.Format == "" {
		s.Format = FormatPNG
	}
	if s.Mode != ModeStrictLossless && s.Mode != ModeJPEGCompatibility {
		return Settings{}, fmt.Errorf("不支持的模式 %q", s.Mode)
	}
	if s.Mode == ModeStrictLossless && s.Format == FormatJPEG {
		return Settings{}, errors.New("严格无损模式不能使用 JPG Sheet；JPEG 兼容模式才允许 JPG")
	}
	if s.Format != FormatPNG && s.Format != FormatJPEG && s.Format != FormatTIFF {
		return Settings{}, fmt.Errorf("不支持的 Sheet 格式 %q", s.Format)
	}
	if s.Padding < 0 {
		return Settings{}, errors.New("padding 不能为负数")
	}
	if s.Padding != 0 && s.Padding != 8 && s.Padding != 16 && s.Padding != 32 && s.Padding != 64 {
		return Settings{}, errors.New("padding 只支持 0、8、16、32、64 像素")
	}
	if s.PaddingMode == "" {
		s.PaddingMode = PaddingSolid
	}
	if s.PaddingMode != PaddingSolid && s.PaddingMode != PaddingEdgeExtension {
		return Settings{}, fmt.Errorf("不支持的 padding 模式 %q", s.PaddingMode)
	}
	if s.TileSize == 0 {
		s.TileSize = 512
	}
	if s.TileSize < 64 || s.TileSize > 4096 {
		return Settings{}, errors.New("tile size 必须在 64 到 4096 之间")
	}
	if s.JPEGQuality == 0 {
		s.JPEGQuality = 95
	}
	if s.JPEGQuality < 1 || s.JPEGQuality > 100 {
		return Settings{}, errors.New("JPEG 质量必须在 1 到 100 之间")
	}
	if s.SortStrategy == "" {
		s.SortStrategy = SortAreaDescending
	}
	if s.Heuristic == "" {
		s.Heuristic = HeuristicBestShortSideFit
	}
	if s.WorkingBitDepth == 0 {
		s.WorkingBitDepth = 16
	}
	if s.WorkingBitDepth != 8 && s.WorkingBitDepth != 16 {
		return Settings{}, errors.New("工作位深只支持 8 或 16")
	}
	if s.WorkingColorSpace == "" {
		s.WorkingColorSpace = "sRGB"
	}
	switch s.WorkingColorSpace {
	case "sRGB", "Adobe RGB", "Display P3", "ProPhoto RGB":
	default:
		return Settings{}, fmt.Errorf("不支持的工作色彩空间 %q", s.WorkingColorSpace)
	}
	if s.TIFFCompression == "" {
		s.TIFFCompression = TIFFZIP
	}
	if s.TIFFCompression != TIFFZIP && s.TIFFCompression != TIFFLZW && s.TIFFCompression != TIFFUncompressed {
		return Settings{}, fmt.Errorf("不支持的 TIFF 压缩方式 %q", s.TIFFCompression)
	}
	if s.StorageMode == "" {
		s.StorageMode = StorageLowDisk
	}
	if s.StorageMode != StorageLowDisk && s.StorageMode != StorageBalanced && s.StorageMode != StoragePerformance {
		return Settings{}, fmt.Errorf("不支持的存储策略 %q", s.StorageMode)
	}
	if s.RawProcessing.WhiteBalance == "" {
		s.RawProcessing.WhiteBalance = WhiteBalanceCamera
	}
	if s.RawProcessing.WhiteBalance != WhiteBalanceCamera && s.RawProcessing.WhiteBalance != WhiteBalanceNeutral && s.RawProcessing.WhiteBalance != WhiteBalanceCustom {
		return Settings{}, fmt.Errorf("不支持的 RAW 白平衡 %q", s.RawProcessing.WhiteBalance)
	}
	if s.RawProcessing.Demosaic == "" {
		s.RawProcessing.Demosaic = DemosaicBalanced
	}
	if s.RawProcessing.Demosaic != DemosaicFast && s.RawProcessing.Demosaic != DemosaicBalanced && s.RawProcessing.Demosaic != DemosaicHigh {
		return Settings{}, fmt.Errorf("不支持的 RAW demosaic 质量 %q", s.RawProcessing.Demosaic)
	}
	return s, nil
}

func integerCeilSqrt(n int64) int64 {
	if n <= 0 {
		return 1
	}
	var x int64 = 1
	for x*x < n && x < 1<<31 {
		x *= 2
	}
	lo, hi := x/2, x
	for lo < hi {
		mid := lo + (hi-lo)/2
		if mid*mid >= n {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}

type Image struct {
	ID                 string       `json:"id"`
	Filename           string       `json:"filename"`
	RelativePath       string       `json:"relativePath"`
	SHA256             string       `json:"sha256"`
	FileSize           int64        `json:"fileSize"`
	EncodedWidth       int          `json:"encodedWidth"`
	EncodedHeight      int          `json:"encodedHeight"`
	DisplayWidth       int          `json:"displayWidth"`
	DisplayHeight      int          `json:"displayHeight"`
	EXIFOrientation    int          `json:"exifOrientation"`
	Rotation           int          `json:"rotation"`
	Padding            int          `json:"padding"`
	ICCProfileHash     string       `json:"iccProfileHash,omitempty"`
	SourceFormat       string       `json:"sourceFormat,omitempty"`
	SourceBitDepth     int          `json:"sourceBitDepth,omitempty"`
	Raw                *RawMetadata `json:"raw,omitempty"`
	RawSourceIntegrity string       `json:"rawSourceIntegrity,omitempty"`
	OuterRect          Rect         `json:"outerRect"`
	ContentRect        Rect         `json:"contentRect"`
}

type RawMetadata struct {
	Format            string    `json:"format,omitempty"`
	CameraMake        string    `json:"cameraMake,omitempty"`
	CameraModel       string    `json:"cameraModel,omitempty"`
	EncodedWidth      int       `json:"encodedWidth,omitempty"`
	EncodedHeight     int       `json:"encodedHeight,omitempty"`
	ActiveWidth       int       `json:"activeWidth,omitempty"`
	ActiveHeight      int       `json:"activeHeight,omitempty"`
	Orientation       int       `json:"orientation,omitempty"`
	CFA               string    `json:"cfa,omitempty"`
	NativeBitDepth    int       `json:"nativeBitDepth,omitempty"`
	BlackLevel        []float64 `json:"blackLevel,omitempty"`
	WhiteLevel        float64   `json:"whiteLevel,omitempty"`
	Compression       string    `json:"compression,omitempty"`
	CompressionTag    uint16    `json:"compressionTag,omitempty"`
	CompressionState  string    `json:"compressionState,omitempty"`
	EmbeddedPreview   bool      `json:"embeddedPreview,omitempty"`
	DNGVersion        string    `json:"dngVersion,omitempty"`
	Supported         bool      `json:"supported,omitempty"`
	ProbeAvailable    bool      `json:"probeAvailable,omitempty"`
	DecodeAvailable   bool      `json:"decodeAvailable,omitempty"`
	PreviewAvailable  bool      `json:"previewAvailable,omitempty"`
	DecodeReason      string    `json:"decodeReason,omitempty"`
	DecodeSource      string    `json:"decodeSource,omitempty"`
	StoredBitDepth    int       `json:"storedBitDepth,omitempty"`
	EffectiveBitDepth int       `json:"effectiveBitDepth,omitempty"`
	Capabilities      []string  `json:"capabilities,omitempty"`
	RawBackend        string    `json:"rawBackend,omitempty"`
	RawBackendVersion string    `json:"rawBackendVersion,omitempty"`
	DecoderBackend    string    `json:"decoderBackend,omitempty"`
	DecoderVersion    string    `json:"decoderVersion,omitempty"`
	DNGSDKVersion     string    `json:"dngSdkVersion,omitempty"`
}

type Sheet struct {
	ID             string      `json:"id"`
	Index          int         `json:"index"`
	CanonicalStem  string      `json:"canonicalStem"`
	File           string      `json:"file"`
	Width          int         `json:"width"`
	Height         int         `json:"height"`
	Format         SheetFormat `json:"format"`
	ICCProfileHash string      `json:"iccProfileHash,omitempty"`
	Images         []Image     `json:"images"`
}

// ProjectShortID is used in generated filenames and stable Sheet identities.
// Keeping it deterministic prevents a project from changing identity merely
// because it was opened on another platform.
func ProjectShortID(projectID string) string {
	projectID = strings.TrimSpace(projectID)
	if len(projectID) > 8 {
		return projectID[:8]
	}
	return projectID
}

func StableSheetID(projectID string, index int) string {
	return fmt.Sprintf("%s-S%03d", ProjectShortID(projectID), index)
}

func StableSheetStem(projectID string, index int) string {
	return fmt.Sprintf("PS_%s_S%03d", ProjectShortID(projectID), index)
}

func CanonicalStemFromFile(filename string) string {
	base := filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	return strings.TrimSuffix(base, filepath.Ext(base))
}

type Project struct {
	SchemaVersion int      `json:"schemaVersion"`
	AppVersion    string   `json:"appVersion"`
	ProjectID     string   `json:"projectId"`
	CreatedAt     string   `json:"createdAt"`
	Mode          Mode     `json:"mode"`
	Settings      Settings `json:"settings"`
	Sheets        []Sheet  `json:"sheets"`
}

func (p *Project) Validate() error {
	if p.SchemaVersion <= 0 {
		return errors.New("project.json 缺少有效的 schemaVersion")
	}
	if p.SchemaVersion > CurrentSchemaVersion {
		return fmt.Errorf("project.json 版本 %d 高于当前支持的版本 %d", p.SchemaVersion, CurrentSchemaVersion)
	}
	if p.SchemaVersion < CurrentSchemaVersion {
		// Loading performs the same migration explicitly. This fallback keeps
		// callers that construct a legacy Project in memory compatible, and
		// makes an explicit SaveAtomic call the point at which it is upgraded.
		p.SchemaVersion = CurrentSchemaVersion
	}
	if strings.TrimSpace(p.ProjectID) == "" {
		return errors.New("project.json 缺少 projectId")
	}
	settings, err := p.Settings.Normalize()
	if err != nil {
		return fmt.Errorf("settings 无效: %w", err)
	}
	p.Settings = settings
	seenSheets := make(map[string]struct{}, len(p.Sheets))
	seenSheetIDs := make(map[string]struct{}, len(p.Sheets))
	seenSheetStems := make(map[string]struct{}, len(p.Sheets))
	seenSheetIndexes := make(map[int]struct{}, len(p.Sheets))
	seenImageIDs := make(map[string]struct{})
	for si := range p.Sheets {
		s := &p.Sheets[si]
		if s.Index <= 0 {
			s.Index = si + 1
		}
		if strings.TrimSpace(s.ID) == "" {
			s.ID = StableSheetID(p.ProjectID, s.Index)
		}
		if strings.TrimSpace(s.CanonicalStem) == "" {
			s.CanonicalStem = CanonicalStemFromFile(s.File)
		}
		if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.File) == "" || strings.TrimSpace(s.CanonicalStem) == "" {
			return fmt.Errorf("第 %d 个 Sheet 缺少 id、canonicalStem 或 file", si+1)
		}
		if _, exists := seenSheets[s.File]; exists {
			return fmt.Errorf("Sheet 文件名重复: %s", s.File)
		}
		seenSheets[s.File] = struct{}{}
		if _, exists := seenSheetIDs[s.ID]; exists {
			return fmt.Errorf("Sheet id 重复: %s", s.ID)
		}
		seenSheetIDs[s.ID] = struct{}{}
		stemKey := strings.ToLower(s.CanonicalStem)
		if _, exists := seenSheetStems[stemKey]; exists {
			return fmt.Errorf("Sheet canonicalStem 重复: %s", s.CanonicalStem)
		}
		seenSheetStems[stemKey] = struct{}{}
		if _, exists := seenSheetIndexes[s.Index]; exists {
			return fmt.Errorf("Sheet index 重复: %d", s.Index)
		}
		seenSheetIndexes[s.Index] = struct{}{}
		if filepath.Base(s.CanonicalStem) != s.CanonicalStem || strings.ContainsAny(s.CanonicalStem, `/\\`) || strings.ContainsRune(s.CanonicalStem, 0) {
			return fmt.Errorf("Sheet canonicalStem 包含不安全路径: %q", s.CanonicalStem)
		}
		if filepath.Base(s.File) != s.File || strings.ContainsAny(s.File, `/\\`) || s.File == "." || s.File == ".." {
			return fmt.Errorf("Sheet 文件名包含不安全路径: %q", s.File)
		}
		if s.Width <= 0 || s.Height <= 0 || int64(s.Width) > settings.MaximumLongEdge || int64(s.Height) > settings.MaximumLongEdge || minInt(s.Width, s.Height) > int(settings.MaximumShortEdge) {
			return fmt.Errorf("Sheet %s 尺寸超出限制", s.File)
		}
		if s.Format != settings.Format {
			return fmt.Errorf("Sheet %s 格式与项目设置不一致", s.File)
		}
		if settings.MaximumTotalPixels > 0 && int64(s.Width)*int64(s.Height) > settings.MaximumTotalPixels {
			return fmt.Errorf("Sheet %s 总像素超出限制", s.File)
		}
		seenImages := make(map[string]struct{}, len(s.Images))
		for ii := range s.Images {
			im := &s.Images[ii]
			if im.ID == "" || im.Filename == "" {
				return fmt.Errorf("Sheet %s 的第 %d 张图片缺少 id 或 filename", s.File, ii+1)
			}
			if _, exists := seenImages[im.ID]; exists {
				return fmt.Errorf("Sheet %s 中图片 id 重复: %s", s.File, im.ID)
			}
			seenImages[im.ID] = struct{}{}
			if _, exists := seenImageIDs[im.ID]; exists {
				return fmt.Errorf("图片 id 在多个 Sheet 中重复: %s", im.ID)
			}
			seenImageIDs[im.ID] = struct{}{}
			if im.DisplayWidth <= 0 || im.DisplayHeight <= 0 {
				return fmt.Errorf("图片 %s 的显示尺寸无效", im.Filename)
			}
			if im.EXIFOrientation < 1 || im.EXIFOrientation > 8 {
				return fmt.Errorf("图片 %s 的 EXIF Orientation 无效", im.Filename)
			}
			if im.Rotation != 0 && im.Rotation != 90 {
				return fmt.Errorf("图片 %s 的 rotation 只能为 0 或 90", im.Filename)
			}
			if im.Padding < 0 || im.Padding != settings.Padding {
				return fmt.Errorf("图片 %s 的 padding 与项目设置不一致", im.Filename)
			}
			if err := validateRect(im.OuterRect, s.Width, s.Height); err != nil {
				return fmt.Errorf("图片 %s 的 outerRect 无效: %w", im.Filename, err)
			}
			if err := validateRect(im.ContentRect, s.Width, s.Height); err != nil {
				return fmt.Errorf("图片 %s 的 contentRect 无效: %w", im.Filename, err)
			}
			if !im.OuterRect.Contains(im.ContentRect) {
				return fmt.Errorf("图片 %s 的 contentRect 超出 outerRect", im.Filename)
			}
			paddingX := im.ContentRect.X - im.OuterRect.X
			paddingY := im.ContentRect.Y - im.OuterRect.Y
			if paddingX != paddingY || paddingX < 0 || im.Padding != paddingX || im.OuterRect.Width != im.ContentRect.Width+2*paddingX || im.OuterRect.Height != im.ContentRect.Height+2*paddingY {
				return fmt.Errorf("图片 %s 的 outerRect/contentRect padding 不一致", im.Filename)
			}
			contentWidth, contentHeight := im.DisplayWidth, im.DisplayHeight
			if im.Rotation == 90 {
				contentWidth, contentHeight = contentHeight, contentWidth
			}
			if im.ContentRect.Width != contentWidth || im.ContentRect.Height != contentHeight {
				return fmt.Errorf("图片 %s 的 contentRect 与显示尺寸不一致", im.Filename)
			}
			for previous := 0; previous < ii; previous++ {
				if rectanglesOverlap(s.Images[previous].OuterRect, im.OuterRect) {
					return fmt.Errorf("Sheet %s 中图片 %s 与 %s 重叠", s.File, im.Filename, s.Images[previous].Filename)
				}
			}
		}
	}
	return nil
}

func rectanglesOverlap(a, b Rect) bool {
	return a.X < b.Right() && a.Right() > b.X && a.Y < b.Bottom() && a.Bottom() > b.Y
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func validateRect(r Rect, canvasWidth, canvasHeight int) error {
	if r.Empty() || r.X < 0 || r.Y < 0 || r.Right() > canvasWidth || r.Bottom() > canvasHeight {
		return fmt.Errorf("矩形 (%d,%d,%d,%d) 越界", r.X, r.Y, r.Width, r.Height)
	}
	return nil
}
