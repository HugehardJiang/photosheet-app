package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"photosheet/internal/app"
	"photosheet/internal/model"
	"photosheet/internal/project"
	"photosheet/internal/raw"
	"photosheet/internal/resolver"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ", ") }
func (s *stringList) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("输入路径不能为空")
	}
	*s = append(*s, value)
	return nil
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printRootUsage(stderr)
		return 2
	}
	switch args[0] {
	case "pack":
		if err := runPack(ctx, args[1:], stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "错误：%v\n", err)
			return 1
		}
		return 0
	case "unpack":
		if err := runUnpack(ctx, args[1:], stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "错误：%v\n", err)
			return 1
		}
		return 0
	case "verify":
		if err := runVerify(args[1:], stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "错误：%v\n", err)
			return 1
		}
		return 0
	case "inspect":
		if err := runInspect(args[1:], stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "错误：%v\n", err)
			return 1
		}
		return 0
	case "diagnostics":
		if err := runDiagnostics(stdout); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		return 0
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "PhotoSheet %s\n", project.AppVersion)
		return 0
	case "help", "--help", "-h":
		printRootUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "未知命令：%s\n\n", args[0])
		printRootUsage(stderr)
		return 2
	}
}

func runPack(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	settings := model.DefaultSettings()
	var inputs stringList
	flags := flag.NewFlagSet("pack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Var(&inputs, "input", "输入 JPG/JPEG/PNG/TIFF/DNG 文件或文件夹，可重复指定")
	output := flags.String("output", "", "项目输出目录")
	maxLong := flags.Int64("max-long-edge", settings.MaximumLongEdge, "Sheet 最大长边像素")
	maxShort := flags.Int64("max-short-edge", 0, "可选：Sheet 最大短边像素")
	maxPixels := flags.Int64("max-total-pixels", 0, "可选：单个 Sheet 最大总像素数")
	mode := flags.String("mode", string(settings.Mode), "模式：strict-lossless 或 jpeg-compatibility")
	format := flags.String("format", string(settings.Format), "Sheet 格式：png、tiff 或 jpg")
	sheetFormat := flags.String("sheet-format", "", "兼容示例别名：png、tiff 或 jpg")
	padding := flags.Int("padding", settings.Padding, "照片 padding：0、8、16、32、64")
	paddingMode := flags.String("padding-mode", string(settings.PaddingMode), "padding：solid 或 edge-extension")
	allowRotation := flags.Bool("allow-rotation", false, "允许排版时旋转 90 度")
	skipInvalid := flags.Bool("skip-invalid", false, "跳过损坏或不支持的图片，并写入日志")
	memoryLimit := flags.String("memory-limit", "", "画布内存上限，例如 512MiB")
	tileSize := flags.Int("tile-size", settings.TileSize, "磁盘画布 tile 边长")
	jpegQuality := flags.Int("jpeg-quality", settings.JPEGQuality, "JPEG 兼容模式质量 1-100")
	bitDepth := flags.Int("bit-depth", settings.WorkingBitDepth, "工作位深：8 或 16")
	rawWB := flags.String("raw-wb", string(settings.RawProcessing.WhiteBalance), "RAW 白平衡：camera、neutral 或 custom")
	rawDemosaic := flags.String("raw-demosaic", string(settings.RawProcessing.Demosaic), "RAW demosaic：fast、balanced 或 high")
	workingSpace := flags.String("working-space", settings.WorkingColorSpace, "工作色彩空间：sRGB、Adobe RGB、Display P3 或 ProPhoto RGB")
	storageMode := flags.String("storage-mode", string(settings.StorageMode), "存储策略：low-disk、balanced 或 performance")
	tiffCompression := flags.String("tiff-compression", string(settings.TIFFCompression), "TIFF 压缩：zip、lzw 或 none")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if len(inputs) == 0 {
		return errors.New("至少需要一个 --input")
	}
	if strings.TrimSpace(*output) == "" {
		return errors.New("必须指定 --output")
	}
	settings.MaximumLongEdge = *maxLong
	settings.MaximumShortEdge = *maxShort
	settings.MaximumTotalPixels = *maxPixels
	settings.Mode = model.Mode(*mode)
	selectedFormat := *format
	if strings.TrimSpace(*sheetFormat) != "" {
		selectedFormat = *sheetFormat
	}
	settings.Format = model.SheetFormat(strings.ToLower(selectedFormat))
	settings.Padding = *padding
	settings.PaddingMode = model.PaddingMode(*paddingMode)
	settings.AllowRotation = *allowRotation
	settings.TileSize = *tileSize
	settings.JPEGQuality = *jpegQuality
	settings.WorkingBitDepth = *bitDepth
	settings.RawProcessing.WhiteBalance = model.WhiteBalanceMode(strings.ToLower(*rawWB))
	settings.RawProcessing.Demosaic = model.DemosaicQuality(strings.ToLower(*rawDemosaic))
	settings.WorkingColorSpace = *workingSpace
	settings.StorageMode = model.StorageMode(strings.ToLower(*storageMode))
	settings.TIFFCompression = model.TIFFCompression(strings.ToLower(*tiffCompression))
	if *memoryLimit != "" {
		parsed, err := parseByteSize(*memoryLimit)
		if err != nil {
			return fmt.Errorf("--memory-limit 无效: %w", err)
		}
		settings.MemoryLimitBytes = parsed
	}
	service := app.NewService()
	response, err := service.Pack(ctx, app.PackRequest{
		Inputs: inputs, OutputDir: *output, Settings: settings, SkipInvalid: *skipInvalid,
		Progress: progressWriter(stdout),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "完成：已生成 %d 个 Sheet\n项目文件：%s\n", len(response.Project.Sheets), response.ProjectPath)
	for _, warning := range response.Warnings {
		fmt.Fprintf(stdout, "提示：%s\n", warning)
	}
	return nil
}

func runInspect(args []string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		return errors.New("用法：photosheet inspect <RAW 文件>")
	}
	if !raw.IsRAWPath(args[0]) {
		return errors.New("inspect 当前只接受 RAW 文件")
	}
	meta, err := raw.NewRegistry().Probe(context.Background(), args[0])
	if err != nil {
		return err
	}
	state := string(meta.CompressionState)
	if state == "" {
		state = "unknown"
	}
	fmt.Fprintf(stdout, "Format: %s\nCamera: %s %s\nDimensions: %dx%d\nActive Area: %dx%d+%d+%d\nOrientation: %d\nStored Bit Depth: %d\nEffective Bit Depth: %d\nCompression: %s\nCompression State: %s\nCFA: %s\nBlack Level: %v\nWhite Level: %.6g\nDNG Version: %s\nEmbedded Preview: %t\nBackend: %s %s\nProbe: %t\nDecode: %t\nPreview: %t\nDecode Source: %s\nDecode Reason: %s\nCapabilities: %+v\n", meta.Format, meta.CameraMake, meta.CameraModel, meta.EncodedWidth, meta.EncodedHeight, meta.ActiveWidth, meta.ActiveHeight, meta.ActiveX, meta.ActiveY, meta.DisplayOrientation, meta.StoredBitDepth, meta.EffectiveBitDepth, meta.Compression, state, meta.CFA, meta.BlackLevel, meta.WhiteLevel, meta.DNGVersion, meta.EmbeddedPreview, meta.Backend, meta.BackendVersion, meta.ProbeAvailable, meta.DecodeAvailable, meta.PreviewAvailable, meta.DecodeSource, meta.DecodeReason, meta.Capabilities)
	return nil
}

func runDiagnostics(stdout io.Writer) error {
	registry := raw.NewRegistry()
	report := registry.CapabilityReport()
	fmt.Fprintln(stdout, "PhotoSheet RAW diagnostics")
	fmt.Fprintf(stdout, "LibRaw: %s\n", yesNo(report.LibRawEnabled))
	fmt.Fprintf(stdout, "Adobe DNG SDK: %s\n", yesNo(report.DNGSDKEnabled))
	fmt.Fprintf(stdout, "JPEG XL DNG: %s\n", yesNo(report.JPEGXLDNGEnabled))
	fmt.Fprintf(stdout, "16-bit output: %s\n", yesNo(report.Output16BitEnabled))
	fmt.Fprintf(stdout, "DNG Opcode Processing: %s\n", yesNo(report.DNGOpcodeProcessing))
	for _, backend := range registry.Backends() {
		fmt.Fprintf(stdout, "Backend: %s %s available=%t\n  formats=%v\n  compressionTags=%v\n  probe=%t decode=%t preview=%t 16bitRGB=%t JPEGXL=%t DNGSDK=%t Opcode=%t UnicodePaths=%t outputColorSpace=%s\n", backend.Name, backend.Version, backend.Available, backend.Capabilities.Formats, backend.Capabilities.CompressionTags, backend.Capabilities.ProbeSupported, backend.Capabilities.DecodeSupported, backend.Capabilities.PreviewSupported, backend.Capabilities.Supports16BitRGB, backend.Capabilities.SupportsJPEGXL, backend.Capabilities.DNGSDKEnabled, backend.Capabilities.DNGOpcodeProcessing, backend.Capabilities.SupportsUnicodePaths, backend.Capabilities.OutputColorSpace)
		if backend.Error != "" {
			fmt.Fprintf(stdout, "  error=%s\n", backend.Error)
		}
	}
	return nil
}

func yesNo(value bool) string {
	if value {
		return "enabled"
	}
	return "disabled"
}

func runUnpack(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("unpack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	projectPath := flags.String("project", "", "project.json 路径")
	sheetsDir := flags.String("sheets", "", "兼容旧版：处理后 Sheet 文件夹")
	var processedDirs stringList
	var processedFiles stringList
	var mappings stringList
	flags.Var(&processedDirs, "processed-dir", "处理后 Sheet 文件夹，可重复指定")
	flags.Var(&processedFiles, "processed-file", "处理后 Sheet 文件，可重复指定")
	flags.Var(&mappings, "map", "手动绑定，格式为 sheet-id=处理后文件路径，可重复指定")
	recursive := flags.Bool("recursive", false, "递归扫描处理后文件夹")
	outputDir := flags.String("output", "", "恢复照片输出文件夹")
	allowScale := flags.Bool("allow-proportional-scale", false, "允许检测到等比例缩放后按比例恢复")
	legacyFormat := flags.String("format", "", "兼容旧版：输出格式 png 或 jpg")
	outputFormat := flags.String("output-format", "", "恢复格式：png、jpg、same 或 tiff；默认 png")
	quality := flags.Int("jpeg-quality", 95, "JPEG 输出质量 1-100")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*projectPath) == "" {
		return errors.New("必须指定 --project")
	}
	service := app.NewService()
	bindings, err := parseMappings(mappings)
	if err != nil {
		return err
	}
	selectedFormat := strings.ToLower(strings.TrimSpace(*outputFormat))
	if selectedFormat == "" {
		selectedFormat = strings.ToLower(strings.TrimSpace(*legacyFormat))
	}
	if selectedFormat == "" {
		selectedFormat = string(model.OutputPNG)
	}
	request := app.UnpackRequest{
		ProjectPath: *projectPath, SheetsDir: *sheetsDir, ProcessedDirs: processedDirs, ProcessedFiles: processedFiles,
		Recursive: *recursive, Bindings: bindings, OutputDir: *outputDir,
		AllowProportionalScale: *allowScale, OutputFormat: model.OutputFormat(selectedFormat), JPEGQuality: *quality,
		Progress: progressWriter(stdout),
	}
	var response app.UnpackResponse
	if len(processedDirs) > 0 || len(processedFiles) > 0 || len(bindings) > 0 {
		resolved, resolveErr := service.ResolveProcessed(ctx, app.ResolveRequest{
			ProjectPath: *projectPath, SheetsDir: *sheetsDir, ProcessedDirs: processedDirs, ProcessedFiles: processedFiles,
			Recursive: *recursive, Bindings: bindings, Progress: progressWriter(stdout),
		})
		if resolveErr != nil {
			return resolveErr
		}
		printResolution(stdout, resolved)
		if err := resolver.ValidateForExtraction(resolver.Result{Sheets: resolved.Sheets}, *allowScale); err != nil {
			return err
		}
		request.ResolvedSheets = resolved.Sheets
	}
	response, err = service.Unpack(ctx, request)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "完成：已恢复 %d 张照片\n", len(response.Report.RestoredFiles))
	for _, warning := range response.Report.Warnings {
		fmt.Fprintf(stdout, "提示：%s\n", warning)
	}
	return nil
}

func parseMappings(values stringList) (map[string]string, error) {
	result := make(map[string]string, len(values))
	for _, value := range values {
		key, path, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(key) == "" || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("--map 格式无效 %q，应为 sheet-id=文件路径", value)
		}
		result[strings.TrimSpace(key)] = strings.TrimSpace(path)
	}
	return result, nil
}

func printResolution(stdout io.Writer, response app.ResolveResponse) {
	for _, sheet := range response.Sheets {
		path := sheet.Path
		if path == "" && sheet.Candidate != nil {
			path = sheet.Candidate.Path
		}
		fmt.Fprintf(stdout, "[%s] %s %s\n", sheet.Status, sheet.CanonicalStem, path)
		if sheet.Status == resolver.StatusAmbiguous {
			for _, candidate := range sheet.Candidates {
				fmt.Fprintf(stdout, "  候选 score=%d %s\n", candidate.Score, candidate.Path)
			}
		}
	}
	for _, warning := range response.Warnings {
		fmt.Fprintf(stdout, "提示：%s\n", warning)
	}
}

func runVerify(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	projectPath := flags.String("project", "", "project.json 路径")
	sheetsDir := flags.String("sheets", "", "可选：Sheet 文件夹")
	checkFiles := flags.Bool("check-files", false, "同时校验 Sheet 文件尺寸")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *projectPath == "" {
		return errors.New("必须指定 --project")
	}
	if err := app.NewService().Verify(app.VerifyRequest{ProjectPath: *projectPath, SheetsDir: *sheetsDir, CheckFiles: *checkFiles}); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "校验通过：project.json 与 Sheet 坐标结构有效")
	return nil
}

func progressWriter(stdout io.Writer) func(app.Progress) {
	return func(p app.Progress) {
		if p.Total > 0 {
			fmt.Fprintf(stdout, "[%s] %d/%d %s\n", p.Phase, p.Processed, p.Total, p.Current)
		} else if p.Message != "" {
			fmt.Fprintf(stdout, "[%s] %s\n", p.Phase, p.Message)
		}
	}
}

func parseByteSize(value string) (int64, error) {
	trimmed := strings.TrimSpace(strings.ToUpper(value))
	units := []struct {
		suffix string
		factor int64
	}{
		{"GIB", 1 << 30}, {"GB", 1 << 30}, {"MIB", 1 << 20}, {"MB", 1 << 20}, {"KIB", 1 << 10}, {"KB", 1 << 10}, {"B", 1},
	}
	for _, unit := range units {
		if strings.HasSuffix(trimmed, unit.suffix) {
			number := strings.TrimSpace(strings.TrimSuffix(trimmed, unit.suffix))
			value, err := strconv.ParseInt(number, 10, 64)
			if err != nil || value <= 0 {
				return 0, errors.New("必须是正整数加单位，例如 512MiB")
			}
			return value * unit.factor, nil
		}
	}
	valueNumber, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || valueNumber <= 0 {
		return 0, errors.New("必须是正整数加单位，例如 512MiB")
	}
	return valueNumber, nil
}

func printRootUsage(w io.Writer) {
	fmt.Fprintln(w, "PhotoSheet：中文优先的多照片拼接与拆分工具")
	fmt.Fprintln(w, "用法：photosheet <命令> [选项]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "命令：")
	fmt.Fprintln(w, "  pack       扫描照片并生成严格无损 PNG Sheet 与 project.json")
	fmt.Fprintln(w, "  unpack     根据 project.json 从处理后的 Sheet 恢复照片")
	fmt.Fprintln(w, "  verify     校验 project.json，以及可选的 Sheet 尺寸")
	fmt.Fprintln(w, "  inspect    读取 DNG metadata，不执行完整 RAW 解码")
	fmt.Fprintln(w, "  diagnostics 显示 RAW backend、压缩格式和 16-bit 能力")
	fmt.Fprintln(w, "  version    显示版本")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "严格无损模式不会把 JPEG 重新编码；JPEG compatibility 不是数学意义上的完全无损。")
}
