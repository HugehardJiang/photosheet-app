package extractor

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"

	"photosheet/internal/imaging"
	"photosheet/internal/metadata"
	"photosheet/internal/model"
	"photosheet/internal/renderer"
	"photosheet/internal/resolver"
)

type Options struct {
	AllowProportionalScale bool
	OutputFormat           model.OutputFormat
	JPEGQuality            int
	MemoryLimitBytes       int64
	TileSize               int
	StorageMode            model.StorageMode
	TempRoot               string
	Progress               func(Progress)
}

type Progress struct {
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Current   string `json:"current"`
}

type Report struct {
	RestoredFiles         []string `json:"restoredFiles"`
	UsedProportionalScale bool     `json:"usedProportionalScale"`
	ScaleX                float64  `json:"scaleX"`
	ScaleY                float64  `json:"scaleY"`
	OutputFormat          string   `json:"outputFormat"`
	LossyReencode         bool     `json:"lossyReencode"`
	Warnings              []string `json:"warnings"`
	SourceWorkingDepth    int      `json:"sourceWorkingDepth,omitempty"`
	ProcessedDepth        int      `json:"processedDepth,omitempty"`
	RGBPipelineIntegrity  string   `json:"rgbPipelineIntegrity,omitempty"`
}

// Unpack is the backwards-compatible v1 entry point. New callers should use
// UnpackResolved so processed files can come from arbitrary directories.
func Unpack(ctx context.Context, p *model.Project, sheetsDir, outputDir string, options Options) (Report, error) {
	if p == nil {
		return Report{}, errors.New("project 为空")
	}
	if strings.TrimSpace(sheetsDir) == "" {
		return Report{}, errors.New("Sheet 目录不能为空")
	}
	resolved := make([]resolver.ResolvedSheet, 0, len(p.Sheets))
	for _, sheet := range p.Sheets {
		path, err := safeJoin(sheetsDir, filepath.FromSlash(sheet.File))
		if err != nil {
			return Report{}, fmt.Errorf("Sheet %s 路径不安全: %w", sheet.File, err)
		}
		resolved = append(resolved, resolver.ResolvedSheet{
			SheetID: sheet.ID, Index: sheet.Index, CanonicalStem: sheet.CanonicalStem,
			Status: resolver.StatusMatched, Path: path,
		})
	}
	return UnpackResolved(ctx, p, resolved, outputDir, options)
}

// UnpackResolved extracts using the explicit resolver result. It never derives
// a processed path from the original project directory, which is what allows
// users to keep edited exports on another disk or in a different tree.
func UnpackResolved(ctx context.Context, p *model.Project, resolved []resolver.ResolvedSheet, outputDir string, options Options) (Report, error) {
	if p == nil {
		return Report{}, errors.New("project 为空")
	}
	if err := p.Validate(); err != nil {
		return Report{}, fmt.Errorf("project 无效: %w", err)
	}
	if strings.TrimSpace(outputDir) == "" {
		return Report{}, errors.New("输出目录不能为空")
	}
	if len(resolved) != len(p.Sheets) {
		return Report{}, fmt.Errorf("解析结果包含 %d 个 Sheet，项目需要 %d 个", len(resolved), len(p.Sheets))
	}
	if err := resolver.ValidateForExtraction(resolver.Result{Sheets: resolved}, options.AllowProportionalScale); err != nil {
		return Report{}, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return Report{}, fmt.Errorf("创建恢复目录失败: %w", err)
	}
	format, err := normalizeOutputFormat(options.OutputFormat)
	if err != nil {
		return Report{}, err
	}
	quality := options.JPEGQuality
	if quality == 0 {
		quality = p.Settings.JPEGQuality
	}
	if quality == 0 {
		quality = 95
	}
	if quality < 1 || quality > 100 {
		return Report{}, errors.New("JPEG 质量必须在 1 到 100 之间")
	}

	resolvedByID := make(map[string]resolver.ResolvedSheet, len(resolved))
	for _, item := range resolved {
		if item.SheetID == "" || item.Path == "" {
			return Report{}, errors.New("解析结果缺少 Sheet ID 或文件路径")
		}
		resolvedByID[item.SheetID] = item
	}
	report := Report{RestoredFiles: make([]string, 0), OutputFormat: string(format), Warnings: make([]string, 0)}
	if format == model.OutputJPEG {
		report.LossyReencode = true
		report.Warnings = append(report.Warnings, "恢复为 JPEG 会发生重新编码，不能保证数学意义上的完全无损")
	}
	total := 0
	for _, sheet := range p.Sheets {
		total += len(sheet.Images)
	}
	completed := 0
	usedScale := false
	commonScaleX, commonScaleY := 1.0, 1.0
	seenOutput := make(map[string]struct{})
	for _, sheet := range p.Sheets {
		if err := ctx.Err(); err != nil {
			return report, fmt.Errorf("拆分已取消: %w", err)
		}
		resolvedSheet, ok := resolvedByID[sheet.ID]
		if !ok {
			return report, fmt.Errorf("缺少 Sheet %s 的解析结果", sheet.File)
		}
		inspection, inspectErr := imaging.InspectFile(resolvedSheet.Path)
		if inspectErr != nil {
			return report, fmt.Errorf("读取处理后 Sheet %s 元数据失败: %w", sheet.File, inspectErr)
		}
		use16 := p.Settings.WorkingBitDepth >= 16
		var actual *image.NRGBA
		var actual16 *image.NRGBA64
		if use16 {
			actual16, _, err = imaging.DecodeNormalizedFile16(resolvedSheet.Path)
		} else {
			actual, err = imaging.DecodeFile(resolvedSheet.Path)
		}
		if err != nil {
			return report, fmt.Errorf("无法解码处理后的 Sheet %s: %w", filepath.Base(resolvedSheet.Path), err)
		}
		if use16 {
			report.SourceWorkingDepth = 16
		} else {
			report.SourceWorkingDepth = 8
		}
		report.ProcessedDepth = inspection.BitDepth
		if use16 {
			report.RGBPipelineIntegrity = "RGB16Lossless"
		} else {
			report.RGBPipelineIntegrity = "RGB8Lossless"
		}
		if use16 && inspection.BitDepth < 16 {
			report.RGBPipelineIntegrity = "ExternalBitDepthReduction"
			report.Warnings = append(report.Warnings, "外部软件已将 16-bit 工作文件转换为 8-bit；高位深信息已在外部处理阶段丢失，PhotoSheet 无法恢复")
		}
		actualWidth, actualHeight := 0, 0
		if use16 {
			actualWidth, actualHeight = actual16.Bounds().Dx(), actual16.Bounds().Dy()
		} else {
			actualWidth, actualHeight = actual.Bounds().Dx(), actual.Bounds().Dy()
		}
		scaleX, scaleY, scaled, err := resolveScale(actualWidth, actualHeight, sheet.Width, sheet.Height, options.AllowProportionalScale)
		if err != nil {
			return report, fmt.Errorf("Sheet %s 校验失败: %w", sheet.File, err)
		}
		if scaled {
			if usedScale && (math.Abs(commonScaleX-scaleX) > 1e-9 || math.Abs(commonScaleY-scaleY) > 1e-9) {
				return report, errors.New("不同 Sheet 的比例缩放不一致，拒绝自动拆分")
			}
			usedScale = true
			commonScaleX, commonScaleY = scaleX, scaleY
		}
		selectedFormat := format
		if format == model.OutputSame {
			selectedFormat, err = outputFormatForInput(inspection.Format)
			if err != nil {
				return report, fmt.Errorf("Sheet %s: %w", sheet.File, err)
			}
		}
		if selectedFormat == model.OutputJPEG {
			report.LossyReencode = true
		}
		profile := inspection.ICCProfile
		if inspection.Format == "png" {
			profile, _ = metadata.ReadPNGICCProfile(resolvedSheet.Path)
		}
		for _, record := range sheet.Images {
			if err := ctx.Err(); err != nil {
				return report, fmt.Errorf("拆分已取消: %w", err)
			}
			rect := scaledRect(record.ContentRect, scaleX, scaleY)
			name := outputName(record.Filename, selectedFormat, outputDir, seenOutput)
			if use16 {
				cropped, cropErr := imaging.Crop16(actual16, image.Rect(rect.X, rect.Y, rect.Right(), rect.Bottom()))
				if cropErr != nil {
					return report, fmt.Errorf("裁剪 %s 失败: %w", record.Filename, cropErr)
				}
				if record.Rotation == 90 {
					cropped = imaging.Rotate90CCW16(cropped)
				}
				if err := writeImage16(name, cropped, selectedFormat, quality, profile, options); err != nil {
					return report, fmt.Errorf("写出 %s 失败: %w", record.Filename, err)
				}
			} else {
				cropped, cropErr := imaging.Crop(actual, image.Rect(rect.X, rect.Y, rect.Right(), rect.Bottom()))
				if cropErr != nil {
					return report, fmt.Errorf("裁剪 %s 失败: %w", record.Filename, cropErr)
				}
				if record.Rotation == 90 {
					cropped = imaging.Rotate90CCW(cropped)
				}
				if err := writeImage(name, cropped, selectedFormat, quality, profile, options); err != nil {
					return report, fmt.Errorf("写出 %s 失败: %w", record.Filename, err)
				}
			}
			report.RestoredFiles = append(report.RestoredFiles, name)
			completed++
			if options.Progress != nil {
				options.Progress(Progress{Completed: completed, Total: total, Current: record.Filename})
			}
		}
	}
	if report.LossyReencode && format == model.OutputSame {
		report.Warnings = append(report.Warnings, "至少一个处理后 Sheet 为 JPEG；same 输出会进行 JPEG 重新编码")
	}
	if usedScale {
		report.UsedProportionalScale = true
		report.ScaleX, report.ScaleY = commonScaleX, commonScaleY
		report.Warnings = append(report.Warnings, "Sheet 按等比例恢复；输出尺寸已改变，不再是原始像素尺寸")
	}
	return report, nil
}

func normalizeOutputFormat(format model.OutputFormat) (model.OutputFormat, error) {
	switch strings.ToLower(strings.TrimSpace(string(format))) {
	case "", "png":
		return model.OutputPNG, nil
	case "jpg", "jpeg":
		return model.OutputJPEG, nil
	case "same":
		return model.OutputSame, nil
	case "tif", "tiff":
		return model.OutputTIFF, nil
	default:
		return "", fmt.Errorf("不支持的恢复格式 %q，可选 png、jpg、same、tiff", format)
	}
}

func outputFormatForInput(format string) (model.OutputFormat, error) {
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		return model.OutputJPEG, nil
	case "png":
		return model.OutputPNG, nil
	case "tiff", "tif":
		return model.OutputTIFF, nil
	default:
		return "", fmt.Errorf("不支持的处理后格式 %q", format)
	}
}

func resolveScale(actualWidth, actualHeight, expectedWidth, expectedHeight int, allow bool) (float64, float64, bool, error) {
	if actualWidth == expectedWidth && actualHeight == expectedHeight {
		return 1, 1, false, nil
	}
	if expectedWidth <= 0 || expectedHeight <= 0 {
		return 0, 0, false, errors.New("JSON Sheet 尺寸无效")
	}
	scaleX := float64(actualWidth) / float64(expectedWidth)
	scaleY := float64(actualHeight) / float64(expectedHeight)
	if math.Abs(scaleX-scaleY) > 1e-9 {
		return 0, 0, false, fmt.Errorf("发生非等比例形变：scaleX=%.8f, scaleY=%.8f", scaleX, scaleY)
	}
	if !allow {
		return 0, 0, false, fmt.Errorf("实际尺寸为 %dx%d，JSON 要求 %dx%d；如果确认是等比例缩放，请显式启用按比例恢复", actualWidth, actualHeight, expectedWidth, expectedHeight)
	}
	return scaleX, scaleY, true, nil
}

func scaledRect(r model.Rect, scaleX, scaleY float64) model.Rect {
	left := int(math.Round(float64(r.X) * scaleX))
	top := int(math.Round(float64(r.Y) * scaleY))
	right := int(math.Round(float64(r.Right()) * scaleX))
	bottom := int(math.Round(float64(r.Bottom()) * scaleY))
	return model.Rect{X: left, Y: top, Width: right - left, Height: bottom - top}
}

func outputName(original string, format model.OutputFormat, outputDir string, seen map[string]struct{}) string {
	clean := strings.ReplaceAll(original, "\\", "/")
	clean = filepath.Base(clean)
	clean = strings.TrimSpace(clean)
	if clean == "" || clean == "." || clean == ".." || strings.ContainsRune(clean, 0) {
		clean = "restored"
	}
	base := strings.TrimSuffix(clean, filepath.Ext(clean))
	ext := ".png"
	switch format {
	case model.OutputJPEG:
		ext = ".jpg"
	case model.OutputTIFF:
		ext = ".tif"
	}
	for i := 0; ; i++ {
		candidate := base + ext
		if i == 1 {
			candidate = base + "_edited" + ext
		} else if i > 1 {
			candidate = fmt.Sprintf("%s_edited_%d%s", base, i, ext)
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		if _, err := os.Stat(filepath.Join(outputDir, candidate)); err == nil {
			continue
		}
		seen[candidate] = struct{}{}
		return filepath.Join(outputDir, candidate)
	}
}

func writeImage(filename string, img *image.NRGBA, format model.OutputFormat, quality int, profile []byte, options Options) error {
	if format == model.OutputTIFF {
		return renderer.WriteTIFFAtomic(filename, img)
	}
	canvas, err := renderer.NewCanvas(img.Bounds().Dx(), img.Bounds().Dy(), options.MemoryLimitBytes, options.TileSize, options.TempRoot)
	if err != nil {
		return err
	}
	defer canvas.Close()
	if err := canvas.DrawImage(0, 0, img, 0, model.PaddingSolid); err != nil {
		return err
	}
	if format == model.OutputJPEG {
		return renderer.WriteJPEGAtomic(filename, canvas, quality)
	}
	return renderer.WritePNGAtomic(filename, canvas, profile)
}

func writeImage16(filename string, img *image.NRGBA64, format model.OutputFormat, quality int, profile []byte, options Options) error {
	if format == model.OutputTIFF {
		return renderer.WriteTIFF16AtomicWithStorageOptions(filename, img, model.TIFFZIP, profile, options.MemoryLimitBytes, options.TileSize, options.TempRoot, options.StorageMode)
	}
	if format == model.OutputJPEG {
		// JPEG is the one explicit boundary where the user requested an 8-bit
		// external format. The caller reports ExternalBitDepthReduction.
		legacy := imaging.ToNRGBA(img)
		return writeImage(filename, legacy, format, quality, profile, options)
	}
	canvas, err := renderer.NewCanvas16WithStorage(img.Bounds().Dx(), img.Bounds().Dy(), options.MemoryLimitBytes, options.TileSize, options.TempRoot, options.StorageMode)
	if err != nil {
		return err
	}
	defer canvas.Close()
	if err := canvas.DrawImage16(0, 0, img, 0, model.PaddingSolid); err != nil {
		return err
	}
	return renderer.WritePNG16Atomic(filename, canvas, profile)
}

func safeJoin(root, child string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	childAbs, err := filepath.Abs(filepath.Join(rootAbs, child))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, childAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("路径逃出指定目录")
	}
	if info, statErr := os.Lstat(childAbs); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("拒绝通过符号链接读取目录外文件")
	}
	return childAbs, nil
}
