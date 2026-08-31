package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"photosheet/internal/extractor"
	"photosheet/internal/model"
	"photosheet/internal/packing"
	"photosheet/internal/project"
	"photosheet/internal/raw"
	"photosheet/internal/renderer"
	"photosheet/internal/resolver"
	"photosheet/internal/scanner"
)

type Progress struct {
	Status    string `json:"status,omitempty"`
	Phase     string `json:"phase"`
	Processed int    `json:"processed"`
	Total     int    `json:"total"`
	Current   string `json:"current"`
	Message   string `json:"message"`
}

type PackRequest struct {
	Inputs      []string
	OutputDir   string
	Settings    model.Settings
	SkipInvalid bool
	Progress    func(Progress)
}

type PackResponse struct {
	ProjectPath string              `json:"projectPath"`
	OutputDir   string              `json:"outputDir"`
	Project     *model.Project      `json:"project"`
	Warnings    []string            `json:"warnings"`
	ScanSummary scanner.ScanSummary `json:"scanSummary"`
}

type PreviewImage struct {
	Filename string     `json:"filename"`
	Rect     model.Rect `json:"rect"`
	Rotation int        `json:"rotation"`
}

type PreviewSheet struct {
	Width  int            `json:"width"`
	Height int            `json:"height"`
	Images []PreviewImage `json:"images"`
}

type PreviewResponse struct {
	Sheets      []PreviewSheet      `json:"sheets"`
	Warnings    []string            `json:"warnings"`
	HasRAW      bool                `json:"hasRaw"`
	ScanSummary scanner.ScanSummary `json:"scanSummary"`
}

type UnpackRequest struct {
	ProjectPath string
	// SheetsDir is retained for v1 callers. New callers should use explicit
	// processed directories/files so the original project location is not
	// assumed.
	SheetsDir              string
	ProcessedDirs          []string
	ProcessedFiles         []string
	Recursive              bool
	Bindings               map[string]string
	ResolvedSheets         []resolver.ResolvedSheet
	OutputDir              string
	AllowProportionalScale bool
	OutputFormat           model.OutputFormat
	JPEGQuality            int
	Progress               func(Progress)
}

type UnpackResponse struct {
	Report extractor.Report `json:"report"`
}

type ResolveRequest struct {
	ProjectPath    string
	SheetsDir      string
	ProcessedDirs  []string
	ProcessedFiles []string
	Recursive      bool
	Bindings       map[string]string
	Progress       func(Progress)
}

type ResolveResponse struct {
	ProjectPath string                   `json:"projectPath"`
	Sheets      []resolver.ResolvedSheet `json:"sheets"`
	Warnings    []string                 `json:"warnings"`
}

type VerifyRequest struct {
	ProjectPath string
	SheetsDir   string
	CheckFiles  bool
}

type Service struct {
	AppVersion string
}

func NewService() *Service { return &Service{AppVersion: project.AppVersion} }

// Diagnostics is intentionally cheap and does not probe a user image. Native
// backend self-tests run when the registry is created, so this reflects the
// exact capability of the running binary.
func (s *Service) Diagnostics() raw.CapabilityReport {
	return raw.NewRegistry().CapabilityReport()
}

func (s *Service) Preview(ctx context.Context, request PackRequest) (PreviewResponse, error) {
	settings, err := request.Settings.Normalize()
	if err != nil {
		return PreviewResponse{}, fmt.Errorf("预览设置无效: %w", err)
	}
	if len(request.Inputs) == 0 {
		return PreviewResponse{}, errors.New("没有选择输入照片或文件夹")
	}
	emit := func(progress Progress) {
		if request.Progress != nil {
			request.Progress(progress)
		}
	}
	scanResult, err := scanner.Scan(ctx, request.Inputs, scanner.Options{
		SkipInvalid: request.SkipInvalid,
		Progress: func(p scanner.Progress) {
			emit(Progress{Phase: "扫描", Processed: p.Processed, Total: p.Total, Current: p.Current, Message: "正在准备布局预览"})
		},
	})
	if err != nil {
		return PreviewResponse{}, err
	}
	infoByID := make(map[string]scanner.ImageInfo, len(scanResult.Images))
	for _, info := range scanResult.Images {
		infoByID[info.ID] = info
	}
	packOptions, err := packing.OptionsFromSettings(settings)
	if err != nil {
		return PreviewResponse{}, err
	}
	warnings := make([]string, 0)
	if len(scanResult.Errors) > 0 {
		warnings = append(warnings, fmt.Sprintf("已跳过 %d 个无效文件", len(scanResult.Errors)))
	}
	if len(scanResult.Duplicates) > 0 {
		warnings = append(warnings, fmt.Sprintf("已跳过 %d 个重复或导出副本", len(scanResult.Duplicates)))
	}
	groups := groupByICC(scanResult.Images)
	if len(groups) > 1 {
		warnings = append(warnings, fmt.Sprintf("检测到 %d 个 ICC Profile 分组", len(groups)))
	}
	warnings = append(warnings, scanDetailWarnings(scanResult)...)
	response := PreviewResponse{Sheets: make([]PreviewSheet, 0), Warnings: warnings, ScanSummary: scanResult.Summary}
	for _, info := range scanResult.Images {
		if info.Raw != nil {
			response.HasRAW = true
			break
		}
	}
	for groupIndex, group := range groups {
		items := make([]packing.Item, 0, len(group.Images))
		for _, info := range group.Images {
			items = append(items, packing.Item{ID: info.ID, Width: info.DisplayWidth, Height: info.DisplayHeight})
		}
		plan, err := packing.Optimize(ctx, items, packOptions)
		if err != nil {
			return PreviewResponse{}, fmt.Errorf("预览排版失败: %w", err)
		}
		emit(Progress{Phase: "排版", Processed: groupIndex + 1, Total: len(groups), Message: "正在计算真实布局"})
		for _, sheet := range plan.Sheets {
			previewSheet := PreviewSheet{Width: sheet.Width, Height: sheet.Height, Images: make([]PreviewImage, 0, len(sheet.Placements))}
			for _, placement := range sheet.Placements {
				info := infoByID[placement.ItemID]
				previewSheet.Images = append(previewSheet.Images, PreviewImage{Filename: info.Filename, Rect: placement.Content, Rotation: placement.Rotation})
			}
			response.Sheets = append(response.Sheets, previewSheet)
		}
	}
	emit(Progress{Status: "completed", Phase: "完成", Processed: 1, Total: 1, Message: fmt.Sprintf("已完成 %d 个 Sheet 的预览", len(response.Sheets))})
	return response, nil
}

func (s *Service) Pack(ctx context.Context, request PackRequest) (PackResponse, error) {
	startedAt := time.Now()
	settings, err := request.Settings.Normalize()
	if err != nil {
		return PackResponse{}, fmt.Errorf("拼接设置无效: %w", err)
	}
	if len(request.Inputs) == 0 {
		return PackResponse{}, errors.New("没有选择输入照片或文件夹")
	}
	if strings.TrimSpace(request.OutputDir) == "" {
		return PackResponse{}, errors.New("输出目录不能为空")
	}
	if err := os.MkdirAll(request.OutputDir, 0o755); err != nil {
		return PackResponse{}, fmt.Errorf("创建输出目录失败: %w", err)
	}
	emit := func(progress Progress) {
		if request.Progress != nil {
			request.Progress(progress)
		}
	}
	scanResult, err := scanner.Scan(ctx, request.Inputs, scanner.Options{
		SkipInvalid: request.SkipInvalid,
		Progress: func(p scanner.Progress) {
			emit(Progress{Phase: "扫描", Processed: p.Processed, Total: p.Total, Current: p.Current, Message: "正在读取尺寸、EXIF、ICC 和 SHA-256"})
		},
	})
	if err != nil {
		return PackResponse{}, fmt.Errorf("扫描失败: %w", err)
	}
	projectID, err := newProjectID()
	if err != nil {
		return PackResponse{}, fmt.Errorf("生成 project id 失败: %w", err)
	}
	p, err := project.New(projectID, settings)
	if err != nil {
		return PackResponse{}, err
	}
	warnings := make([]string, 0)
	if len(scanResult.Errors) > 0 {
		warnings = append(warnings, fmt.Sprintf("已跳过 %d 个无效文件", len(scanResult.Errors)))
	}
	if len(scanResult.Duplicates) > 0 {
		warnings = append(warnings, fmt.Sprintf("已跳过 %d 个重复或导出副本", len(scanResult.Duplicates)))
	}
	warnings = append(warnings, scanDetailWarnings(scanResult)...)
	groups := groupByICC(scanResult.Images)
	if len(groups) > 1 {
		warnings = append(warnings, fmt.Sprintf("检测到 %d 个 ICC Profile 分组，已分别生成 Sheet", len(groups)))
	}
	if settings.Mode == model.ModeJPEGCompatibility {
		warnings = append(warnings, "JPEG Sheet 会重新编码，不能保证数学意义上的完全无损")
	}
	if err := createOutputTree(request.OutputDir); err != nil {
		return PackResponse{}, err
	}
	infoByID := make(map[string]scanner.ImageInfo, len(scanResult.Images))
	for _, info := range scanResult.Images {
		infoByID[info.ID] = info
	}
	packOptions, err := packing.OptionsFromSettings(settings)
	if err != nil {
		return PackResponse{}, err
	}
	globalSheetIndex := 1
	for groupIndex, group := range groups {
		if err := ctx.Err(); err != nil {
			return PackResponse{}, fmt.Errorf("拼接已取消: %w", err)
		}
		emit(Progress{Phase: "排版", Processed: groupIndex, Total: len(groups), Message: fmt.Sprintf("正在计算 ICC 分组 %d/%d", groupIndex+1, len(groups))})
		items := make([]packing.Item, 0, len(group.Images))
		for _, info := range group.Images {
			items = append(items, packing.Item{ID: info.ID, Width: info.DisplayWidth, Height: info.DisplayHeight})
		}
		plan, err := packing.Optimize(ctx, items, packOptions)
		if err != nil {
			return PackResponse{}, fmt.Errorf("ICC 分组 %q 排版失败: %w", group.Key, err)
		}
		profile := group.Profile
		for sheetOffset, sheetPlan := range plan.Sheets {
			if err := ctx.Err(); err != nil {
				return PackResponse{}, fmt.Errorf("渲染已取消: %w", err)
			}
			extension := string(settings.Format)
			if extension == "" {
				extension = "png"
			}
			canonicalStem := model.StableSheetStem(projectID, globalSheetIndex)
			filename := fmt.Sprintf("%s.%s", canonicalStem, extension)
			sheet := model.Sheet{
				ID:             model.StableSheetID(projectID, globalSheetIndex),
				Index:          globalSheetIndex,
				CanonicalStem:  canonicalStem,
				File:           filename,
				Width:          sheetPlan.Width,
				Height:         sheetPlan.Height,
				Format:         settings.Format,
				ICCProfileHash: group.Key,
				Images:         make([]model.Image, 0, len(sheetPlan.Placements)),
			}
			sources := make([]renderer.SourceImage, 0, len(sheetPlan.Placements))
			for _, placement := range sheetPlan.Placements {
				info, ok := infoByID[placement.ItemID]
				if !ok {
					return PackResponse{}, fmt.Errorf("排版结果引用了未知图片 %s", placement.ItemID)
				}
				record := model.Image{
					ID:                 info.ID,
					Filename:           info.Filename,
					RelativePath:       info.RelativePath,
					SHA256:             info.SHA256,
					FileSize:           info.FileSize,
					EncodedWidth:       info.EncodedWidth,
					EncodedHeight:      info.EncodedHeight,
					DisplayWidth:       info.DisplayWidth,
					DisplayHeight:      info.DisplayHeight,
					EXIFOrientation:    info.EXIFOrientation,
					Rotation:           placement.Rotation,
					Padding:            settings.Padding,
					ICCProfileHash:     info.ICCProfileHash,
					SourceFormat:       info.SourceFormat,
					SourceBitDepth:     info.SourceBitDepth,
					RawSourceIntegrity: rawIntegrity(info),
					OuterRect:          placement.Outer,
					ContentRect:        placement.Content,
				}
				if info.Raw != nil {
					record.Raw = modelRawMetadata(info.Raw)
				}
				sheet.Images = append(sheet.Images, record)
				sources = append(sources, renderer.SourceImage{Record: record, SourcePath: info.SourcePath})
			}
			outputPath := filepath.Join(request.OutputDir, "sheets", filepath.FromSlash(sheet.File))
			if err := renderer.CheckDiskSpaceForDepth(outputPath, sheet.Width, sheet.Height, settings.Format, settings.WorkingBitDepth); err != nil {
				return PackResponse{}, fmt.Errorf("渲染 %s 前磁盘检查失败: %w", filename, err)
			}
			emit(Progress{Phase: "渲染", Processed: sheetOffset, Total: len(plan.Sheets), Current: filename, Message: "正在逐张解码并写入无损 Sheet"})
			err := renderer.RenderSheet(ctx, outputPath, sheet, sources, settings, profile, renderer.Options{
				MemoryLimitBytes: settings.MemoryLimitBytes,
				TileSize:         settings.TileSize,
				Progress: func(p renderer.Progress) {
					emit(Progress{Phase: "渲染", Processed: p.Completed, Total: p.Total, Current: p.Current, Message: fmt.Sprintf("Sheet %d/%d", sheetOffset+1, len(plan.Sheets))})
				},
			})
			if err != nil {
				return PackResponse{}, fmt.Errorf("渲染 %s 失败: %w", filename, err)
			}
			if err := renderer.VerifyImageSize(outputPath, sheet.Width, sheet.Height); err != nil {
				return PackResponse{}, fmt.Errorf("验证 %s 失败: %w", filename, err)
			}
			p.Sheets = append(p.Sheets, sheet)
			globalSheetIndex++
		}
	}
	if err := p.Validate(); err != nil {
		return PackResponse{}, fmt.Errorf("生成的 project 无效: %w", err)
	}
	projectPath := filepath.Join(request.OutputDir, "project.json")
	if err := project.SaveAtomic(p, projectPath); err != nil {
		return PackResponse{}, err
	}
	for _, sheet := range p.Sheets {
		sidecar := filepath.Join(request.OutputDir, "sheets", filepath.FromSlash(strings.TrimSuffix(sheet.File, filepath.Ext(sheet.File))+".json"))
		if err := project.WriteSidecar(sheet, sidecar, p); err != nil {
			return PackResponse{}, fmt.Errorf("写入 Sheet sidecar 失败: %w", err)
		}
	}
	if err := writeScanLog(request.OutputDir, scanResult, warnings, time.Since(startedAt)); err != nil {
		return PackResponse{}, err
	}
	emit(Progress{Status: "completed", Phase: "完成", Processed: 1, Total: 1, Message: fmt.Sprintf("已生成 %d 个 Sheet", len(p.Sheets))})
	return PackResponse{ProjectPath: projectPath, OutputDir: request.OutputDir, Project: p, Warnings: warnings, ScanSummary: scanResult.Summary}, nil
}

func (s *Service) Unpack(ctx context.Context, request UnpackRequest) (UnpackResponse, error) {
	p, err := project.Load(request.ProjectPath)
	if err != nil {
		return UnpackResponse{}, err
	}
	if request.OutputDir == "" {
		request.OutputDir = filepath.Join(filepath.Dir(request.ProjectPath), "restored")
	}
	extractorOptions := extractor.Options{
		AllowProportionalScale: request.AllowProportionalScale,
		OutputFormat:           request.OutputFormat,
		JPEGQuality:            request.JPEGQuality,
		MemoryLimitBytes:       p.Settings.MemoryLimitBytes,
		TileSize:               p.Settings.TileSize,
		StorageMode:            p.Settings.StorageMode,
		Progress: func(progress extractor.Progress) {
			if request.Progress != nil {
				request.Progress(Progress{Phase: "拆分", Processed: progress.Completed, Total: progress.Total, Current: progress.Current, Message: "正在按 JSON 坐标恢复照片"})
			}
		},
	}
	var report extractor.Report
	if len(request.ResolvedSheets) > 0 {
		report, err = extractor.UnpackResolved(ctx, p, request.ResolvedSheets, request.OutputDir, extractorOptions)
	} else if hasProcessedSources(request) {
		resolved, resolveErr := s.resolveProject(ctx, p, ResolveRequest{
			SheetsDir: request.SheetsDir, ProcessedDirs: request.ProcessedDirs, ProcessedFiles: request.ProcessedFiles,
			Recursive: request.Recursive, Bindings: request.Bindings, Progress: request.Progress,
		})
		if resolveErr != nil {
			return UnpackResponse{}, resolveErr
		}
		report, err = extractor.UnpackResolved(ctx, p, resolved.Sheets, request.OutputDir, extractorOptions)
	} else {
		if request.SheetsDir == "" {
			request.SheetsDir = filepath.Join(filepath.Dir(request.ProjectPath), "sheets")
		}
		report, err = extractor.Unpack(ctx, p, request.SheetsDir, request.OutputDir, extractorOptions)
	}
	if err != nil {
		return UnpackResponse{}, err
	}
	if request.Progress != nil {
		request.Progress(Progress{Status: "completed", Phase: "完成", Processed: 1, Total: 1, Message: fmt.Sprintf("已恢复 %d 张照片", len(report.RestoredFiles))})
	}
	return UnpackResponse{Report: report}, nil
}

func (s *Service) ResolveProcessed(ctx context.Context, request ResolveRequest) (ResolveResponse, error) {
	p, err := project.Load(request.ProjectPath)
	if err != nil {
		return ResolveResponse{}, err
	}
	resolved, err := s.resolveProject(ctx, p, request)
	if err != nil {
		return ResolveResponse{}, err
	}
	resolved.ProjectPath = request.ProjectPath
	if request.Progress != nil {
		request.Progress(Progress{Status: "completed", Phase: "完成", Processed: 1, Total: 1, Current: request.ProjectPath, Message: "处理后的 Sheet 匹配完成"})
	}
	return resolved, nil
}

func (s *Service) resolveProject(ctx context.Context, p *model.Project, request ResolveRequest) (ResolveResponse, error) {
	sources := append([]string(nil), request.ProcessedDirs...)
	sources = append(sources, request.ProcessedFiles...)
	if len(sources) == 0 && strings.TrimSpace(request.SheetsDir) != "" {
		sources = append(sources, request.SheetsDir)
	}
	result, err := resolver.Resolve(ctx, p, resolver.Options{
		Sources: sources, Recursive: request.Recursive, Bindings: request.Bindings,
		Progress: func(progress resolver.Progress) {
			if request.Progress != nil {
				request.Progress(Progress{Phase: "匹配", Processed: progress.Processed, Total: progress.Total, Current: progress.Current, Message: progress.Message})
			}
		},
	})
	if err != nil {
		return ResolveResponse{}, err
	}
	return ResolveResponse{ProjectPath: "", Sheets: result.Sheets, Warnings: result.Warnings}, nil
}

func hasProcessedSources(request UnpackRequest) bool {
	return len(request.ProcessedDirs) > 0 || len(request.ProcessedFiles) > 0 || len(request.Bindings) > 0
}

func (s *Service) Verify(request VerifyRequest) error {
	p, err := project.Load(request.ProjectPath)
	if err != nil {
		return err
	}
	if !request.CheckFiles {
		return nil
	}
	if request.SheetsDir == "" {
		request.SheetsDir = filepath.Join(filepath.Dir(request.ProjectPath), "sheets")
	}
	for _, sheet := range p.Sheets {
		filename, err := safeJoin(request.SheetsDir, filepath.FromSlash(sheet.File))
		if err != nil {
			return err
		}
		if err := renderer.VerifyImageSize(filename, sheet.Width, sheet.Height); err != nil {
			return fmt.Errorf("Sheet %s 校验失败: %w", sheet.File, err)
		}
	}
	return nil
}

type iccGroup struct {
	Key     string
	Images  []scanner.ImageInfo
	Profile []byte
}

func groupByICC(images []scanner.ImageInfo) []iccGroup {
	groups := make(map[string]*iccGroup)
	for _, info := range images {
		group := groups[info.ICCProfileHash]
		if group == nil {
			group = &iccGroup{Key: info.ICCProfileHash, Images: make([]scanner.ImageInfo, 0)}
			group.Profile = append([]byte(nil), info.ICCProfile...)
			groups[info.ICCProfileHash] = group
		}
		group.Images = append(group.Images, info)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]iccGroup, 0, len(keys))
	for _, key := range keys {
		result = append(result, *groups[key])
	}
	return result
}

func scanDetailWarnings(result scanner.Result) []string {
	warnings := make([]string, 0)
	for _, scanErr := range result.Errors {
		warnings = append(warnings, fmt.Sprintf("[%s] %s: %v", scanErr.Category, filepath.Base(scanErr.Path), scanErr.Err))
	}
	for _, info := range result.Images {
		if info.Raw != nil && info.Raw.CompressionState == raw.CompressionLossy {
			warnings = append(warnings, fmt.Sprintf("%s: 源 RAW 使用有损压缩（tag %d），无法恢复压缩前信息", info.Filename, info.Raw.CompressionTag))
		}
	}
	return warnings
}

func rawIntegrity(info scanner.ImageInfo) string {
	if info.Raw == nil {
		return ""
	}
	switch info.Raw.CompressionState {
	case "lossless":
		return "RawLossless"
	case "lossy":
		return "RawLossy"
	default:
		return "RawUnknown"
	}
}

func modelRawMetadata(value *raw.Metadata) *model.RawMetadata {
	if value == nil {
		return nil
	}
	capabilities := make([]string, 0, len(value.Capabilities.Formats))
	for _, format := range value.Capabilities.Formats {
		capabilities = append(capabilities, string(format))
	}
	return &model.RawMetadata{
		Format: string(value.Format), CameraMake: value.CameraMake, CameraModel: value.CameraModel,
		EncodedWidth: value.EncodedWidth, EncodedHeight: value.EncodedHeight, ActiveWidth: value.ActiveWidth, ActiveHeight: value.ActiveHeight,
		Orientation: value.DisplayOrientation, CFA: value.CFA, NativeBitDepth: value.NativeBitDepth, BlackLevel: append([]float64(nil), value.BlackLevel...), WhiteLevel: value.WhiteLevel,
		Compression: value.Compression, CompressionTag: value.CompressionTag, CompressionState: string(value.CompressionState), EmbeddedPreview: value.EmbeddedPreview,
		DNGVersion: value.DNGVersion, Supported: value.Supported, ProbeAvailable: value.ProbeAvailable, DecodeAvailable: value.DecodeAvailable, PreviewAvailable: value.PreviewAvailable,
		DecodeReason: value.DecodeReason, DecodeSource: value.DecodeSource, StoredBitDepth: value.StoredBitDepth, EffectiveBitDepth: value.EffectiveBitDepth,
		Capabilities: capabilities, RawBackend: value.Backend, RawBackendVersion: value.BackendVersion,
		DecoderBackend: value.Backend, DecoderVersion: value.BackendVersion, DNGSDKVersion: value.DNGSDKVersion,
	}
}

func createOutputTree(root string) error {
	for _, directory := range []string{"sheets", "restored", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			return fmt.Errorf("创建 %s 目录失败: %w", directory, err)
		}
	}
	return nil
}

func writeScanLog(root string, result scanner.Result, warnings []string, duration time.Duration) error {
	filename := filepath.Join(root, "logs", "pack.jsonl")
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建日志失败: %w", err)
	}
	defer f.Close()
	write := func(value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = f.Write(append(data, '\n'))
		return err
	}
	if err := write(map[string]any{"operation": "pack", "time": time.Now().UTC(), "durationMs": duration.Milliseconds(), "images": len(result.Images), "summary": result.Summary, "warnings": warnings}); err != nil {
		return err
	}
	for _, scanErr := range result.Errors {
		if err := write(map[string]any{"operation": "scan", "filename": filepath.Base(scanErr.Path), "category": scanErr.Category, "format": scanErr.Format, "compression": scanErr.Compression, "error": scanErr.Error()}); err != nil {
			return err
		}
	}
	for _, duplicate := range result.Duplicates {
		if err := write(map[string]any{
			"operation": "scan",
			"kind":      "duplicate",
			"kept":      duplicate.KeptPath,
			"skipped":   duplicate.SkippedPath,
			"reason":    duplicate.Reason,
		}); err != nil {
			return err
		}
	}
	return nil
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

func newProjectID() (string, error) {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
