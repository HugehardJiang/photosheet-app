package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"photosheet/internal/imaging"
	"photosheet/internal/metadata"
	"photosheet/internal/raw"
)

type Progress struct {
	Processed int
	Total     int
	Current   string
}

type ErrorCategory string

const (
	NoSupportedFilesFound          ErrorCategory = "NoSupportedFilesFound"
	FilesFoundButDecodeUnsupported ErrorCategory = "FilesFoundButDecodeUnsupported"
	FilesCorrupted                 ErrorCategory = "FilesCorrupted"
	FilesSkipped                   ErrorCategory = "FilesSkipped"
	Cancelled                      ErrorCategory = "Cancelled"
)

var (
	ErrNoSupportedFilesFound          = errors.New("no supported image files found")
	ErrFilesFoundButDecodeUnsupported = errors.New("files found but RAW decode is unsupported")
	ErrFilesCorrupted                 = errors.New("one or more image files are corrupt")
	ErrFilesSkipped                   = errors.New("one or more image files were skipped")
	ErrCancelled                      = errors.New("scan was cancelled")
)

type ScanSummary struct {
	Found       int `json:"found"`
	Successful  int `json:"successful"`
	Unsupported int `json:"unsupported"`
	Corrupted   int `json:"corrupted"`
	Skipped     int `json:"skipped"`
	Cancelled   int `json:"cancelled"`
}

type ScanFailure struct {
	Category      ErrorCategory
	Summary       ScanSummary
	ByCompression map[string]int
	Message       string
}

func (e *ScanFailure) Error() string {
	if e == nil {
		return "scan failed"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Category == FilesFoundButDecodeUnsupported {
		parts := make([]string, 0, len(e.ByCompression))
		for compression, count := range e.ByCompression {
			parts = append(parts, fmt.Sprintf("%d × %s", count, compression))
		}
		sort.Strings(parts)
		return fmt.Sprintf("发现 %d 个文件，但当前 RAW 解码器无法处理：%s（成功 %d，Unsupported %d）", e.Summary.Found, strings.Join(parts, "，"), e.Summary.Successful, e.Summary.Unsupported)
	}
	switch e.Category {
	case FilesCorrupted:
		return fmt.Sprintf("发现 %d 个文件，其中 %d 个文件损坏或无法读取（成功 %d）", e.Summary.Found, e.Summary.Corrupted, e.Summary.Successful)
	case FilesSkipped:
		return fmt.Sprintf("扫描完成，但有 %d 个文件被跳过（成功 %d）", e.Summary.Skipped, e.Summary.Successful)
	case Cancelled:
		return fmt.Sprintf("扫描已取消：发现 %d 个文件，已完成 %d 个", e.Summary.Found, e.Summary.Successful)
	}
	return fmt.Sprintf("scan failed (%s): found=%d successful=%d unsupported=%d corrupted=%d skipped=%d", e.Category, e.Summary.Found, e.Summary.Successful, e.Summary.Unsupported, e.Summary.Corrupted, e.Summary.Skipped)
}

func (e *ScanFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	switch e.Category {
	case NoSupportedFilesFound:
		return ErrNoSupportedFilesFound
	case FilesFoundButDecodeUnsupported:
		return ErrFilesFoundButDecodeUnsupported
	case FilesCorrupted:
		return ErrFilesCorrupted
	case FilesSkipped:
		return ErrFilesSkipped
	case Cancelled:
		return ErrCancelled
	default:
		return nil
	}
}

type Options struct {
	SkipInvalid bool
	Workers     int
	Progress    func(Progress)
}

type ImageInfo struct {
	ID              string
	SourcePath      string
	Filename        string
	RelativePath    string
	SHA256          string
	FileSize        int64
	EncodedWidth    int
	EncodedHeight   int
	DisplayWidth    int
	DisplayHeight   int
	EXIFOrientation int
	ICCProfileHash  string
	ICCProfile      []byte
	SourceFormat    string
	SourceBitDepth  int
	Raw             *raw.Metadata
}

type ScanError struct {
	Path        string
	Err         error
	Category    ErrorCategory
	Format      string
	Compression string
}

func (e ScanError) Error() string { return fmt.Sprintf("无法扫描 %s: %v", e.Path, e.Err) }
func (e ScanError) Unwrap() error { return e.Err }

type rawProbeError struct {
	meta raw.Metadata
	err  error
}

func (e rawProbeError) Error() string { return e.err.Error() }
func (e rawProbeError) Unwrap() error { return e.err }

func classifyError(err error) ErrorCategory {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return Cancelled
	}
	if errors.Is(err, raw.ErrRawUnsupportedCompression) || errors.Is(err, raw.ErrRawUnsupportedVariant) || errors.Is(err, raw.ErrRawBackendUnavailable) {
		return FilesFoundButDecodeUnsupported
	}
	if errors.Is(err, raw.ErrRawCorrupt) || errors.Is(err, raw.ErrNotRAW) {
		return FilesCorrupted
	}
	// A supported extension whose bytes fail parsing is a corrupt input, not
	// an absent input.
	return FilesCorrupted
}

func failureFor(summary ScanSummary, errorsFound []ScanError) *ScanFailure {
	category := FilesSkipped
	if summary.Cancelled > 0 {
		category = Cancelled
	} else if summary.Unsupported > 0 && summary.Corrupted == 0 && summary.Unsupported == len(errorsFound) {
		category = FilesFoundButDecodeUnsupported
	} else if summary.Corrupted > 0 && summary.Unsupported == 0 {
		category = FilesCorrupted
	}
	byCompression := make(map[string]int)
	for _, item := range errorsFound {
		if item.Category == FilesFoundButDecodeUnsupported {
			label := item.Compression
			if label == "" {
				label = "RAW variant"
			}
			byCompression[label]++
		}
	}
	return &ScanFailure{Category: category, Summary: summary, ByCompression: byCompression}
}

// Duplicate records an input file that was not sent to the packer because it
// was an exact copy or a likely exported copy of another input file.
type Duplicate struct {
	KeptPath    string `json:"keptPath"`
	SkippedPath string `json:"skippedPath"`
	Reason      string `json:"reason"`
}

type Result struct {
	Root       string
	Images     []ImageInfo
	Errors     []ScanError
	Duplicates []Duplicate
	Summary    ScanSummary
}

func Scan(ctx context.Context, inputs []string, options Options) (Result, error) {
	if len(inputs) == 0 {
		return Result{}, errors.New("没有选择输入照片或文件夹")
	}
	paths, root, err := collectPaths(ctx, inputs)
	if err != nil {
		if ctx.Err() != nil {
			return Result{}, &ScanFailure{Category: Cancelled, Summary: ScanSummary{Cancelled: 1}, Message: "扫描已取消：尚未完成输入目录扫描"}
		}
		return Result{}, err
	}
	if len(paths) == 0 {
		result := Result{Root: root, Summary: ScanSummary{}}
		return result, &ScanFailure{Category: NoSupportedFilesFound, Summary: result.Summary, Message: "输入中没有找到受支持的 JPG/JPEG/PNG/TIFF/DNG 文件"}
	}
	workers := options.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > 8 {
		workers = 8
	}
	if workers > len(paths) {
		workers = len(paths)
	}
	type job struct {
		index int
		path  string
	}
	type result struct {
		index int
		info  ImageInfo
		err   error
	}
	jobs := make(chan job)
	results := make(chan result, len(paths))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for current := range jobs {
				if ctx.Err() != nil {
					results <- result{index: current.index, err: ctx.Err()}
					continue
				}
				info, err := inspectOne(ctx, current.path, root, current.index+1)
				results <- result{index: current.index, info: info, err: err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i, path := range paths {
			select {
			case jobs <- job{index: i, path: path}:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	ordered := make([]result, len(paths))
	completed := 0
	for current := range results {
		if current.index >= 0 && current.index < len(ordered) {
			ordered[current.index] = current
		}
		completed++
		if options.Progress != nil {
			name := ""
			if current.index >= 0 && current.index < len(paths) {
				name = filepath.Base(paths[current.index])
			}
			options.Progress(Progress{Processed: completed, Total: len(paths), Current: name})
		}
	}
	if err := ctx.Err(); err != nil {
		result := Result{Root: root, Summary: ScanSummary{Found: len(paths), Cancelled: len(paths)}}
		return result, &ScanFailure{Category: Cancelled, Summary: result.Summary, Message: fmt.Sprintf("扫描已取消：发现 %d 个文件", len(paths))}
	}
	final := Result{Root: root, Images: make([]ImageInfo, 0, len(paths)), Summary: ScanSummary{Found: len(paths)}}
	for _, current := range ordered {
		if current.err != nil {
			category := classifyError(current.err)
			scanErr := ScanError{Path: paths[current.index], Err: current.err, Category: category}
			if details, ok := current.err.(rawProbeError); ok {
				scanErr.Format = string(details.meta.Format)
				scanErr.Compression = details.meta.Compression
			}
			final.Errors = append(final.Errors, scanErr)
			switch category {
			case FilesFoundButDecodeUnsupported:
				final.Summary.Unsupported++
			case FilesCorrupted:
				final.Summary.Corrupted++
			case Cancelled:
				final.Summary.Cancelled++
			default:
				final.Summary.Skipped++
			}
			continue
		}
		final.Images = append(final.Images, current.info)
	}
	final.Images, final.Duplicates = deduplicate(final.Images)
	final.Summary.Successful = len(final.Images)
	final.Summary.Skipped += len(final.Duplicates)
	if !options.SkipInvalid && len(final.Errors) > 0 {
		return final, failureFor(final.Summary, final.Errors)
	}
	if len(final.Images) == 0 {
		return final, failureFor(final.Summary, final.Errors)
	}
	return final, nil
}

func deduplicate(images []ImageInfo) ([]ImageInfo, []Duplicate) {
	if len(images) < 2 {
		return images, nil
	}

	kept := make([]ImageInfo, 0, len(images))
	duplicates := make([]Duplicate, 0)
	byHash := make(map[string]int, len(images))
	for _, image := range images {
		if image.SHA256 != "" {
			if keptIndex, exists := byHash[image.SHA256]; exists {
				if preferImage(image, kept[keptIndex]) {
					duplicates = append(duplicates, Duplicate{
						KeptPath:    image.SourcePath,
						SkippedPath: kept[keptIndex].SourcePath,
						Reason:      "SHA-256 相同",
					})
					kept[keptIndex] = image
				} else {
					duplicates = append(duplicates, Duplicate{
						KeptPath:    kept[keptIndex].SourcePath,
						SkippedPath: image.SourcePath,
						Reason:      "SHA-256 相同",
					})
				}
				continue
			}
		}
		if image.SHA256 != "" {
			byHash[image.SHA256] = len(kept)
		}
		kept = append(kept, image)
	}

	// A common photo workflow keeps originals beside an external tool's
	// *_导出/export/output directory. If both copies are selected recursively,
	// keep the non-export copy while retaining legitimate same-name photos from
	// unrelated directories. This preserves the duplicate-filename contract.
	byFilename := make(map[string][]int, len(kept))
	for index, image := range kept {
		key := strings.ToLower(filepath.Base(image.SourcePath))
		byFilename[key] = append(byFilename[key], index)
	}
	drop := make([]bool, len(kept))
	for _, indexes := range byFilename {
		hasNonExport := false
		for _, index := range indexes {
			if !isGeneratedExportPath(kept[index].SourcePath) {
				hasNonExport = true
				break
			}
		}
		if !hasNonExport {
			continue
		}
		for _, index := range indexes {
			if isGeneratedExportPath(kept[index].SourcePath) {
				drop[index] = true
			}
		}
	}

	result := make([]ImageInfo, 0, len(kept))
	for index, image := range kept {
		if drop[index] {
			keptPath := ""
			for _, candidate := range kept {
				if strings.EqualFold(filepath.Base(candidate.SourcePath), filepath.Base(image.SourcePath)) && !isGeneratedExportPath(candidate.SourcePath) {
					keptPath = candidate.SourcePath
					break
				}
			}
			duplicates = append(duplicates, Duplicate{
				KeptPath:    keptPath,
				SkippedPath: image.SourcePath,
				Reason:      "同名文件位于导出目录",
			})
			continue
		}
		image.ID = fmt.Sprintf("image-%06d", len(result)+1)
		result = append(result, image)
	}
	return result, duplicates
}

func preferImage(candidate, current ImageInfo) bool {
	candidateExport := isGeneratedExportPath(candidate.SourcePath)
	currentExport := isGeneratedExportPath(current.SourcePath)
	if candidateExport != currentExport {
		return !candidateExport
	}
	candidateDepth := strings.Count(filepath.Clean(candidate.SourcePath), string(filepath.Separator))
	currentDepth := strings.Count(filepath.Clean(current.SourcePath), string(filepath.Separator))
	if candidateDepth != currentDepth {
		return candidateDepth < currentDepth
	}
	return candidate.SourcePath < current.SourcePath
}

func isGeneratedExportPath(path string) bool {
	for current := filepath.Dir(filepath.Clean(path)); ; current = filepath.Dir(current) {
		if isGeneratedExportDirectory(current) {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return false
}

func isGeneratedExportDirectory(path string) bool {
	name := strings.ToLower(filepath.Base(filepath.Clean(path)))
	switch {
	case name == "sheets", name == "restored", name == "logs":
		return true
	case strings.HasSuffix(name, "导出"), strings.HasSuffix(name, "export"), strings.HasSuffix(name, "exports"):
		return true
	case strings.HasSuffix(name, "output"), strings.HasSuffix(name, "outputs"), strings.HasSuffix(name, "输出"):
		return true
	default:
		return false
	}
}

func collectPaths(ctx context.Context, inputs []string) ([]string, string, error) {
	var normalized []string
	for _, input := range inputs {
		if strings.TrimSpace(input) == "" {
			continue
		}
		abs, err := filepath.Abs(input)
		if err != nil {
			return nil, "", fmt.Errorf("解析输入路径 %q 失败: %w", input, err)
		}
		abs, err = filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, "", fmt.Errorf("无法访问输入 %s: %w", input, err)
		}
		normalized = append(normalized, abs)
	}
	if len(normalized) == 0 {
		return nil, "", errors.New("输入路径为空")
	}
	var paths []string
	for _, input := range normalized {
		if err := filepath.WalkDir(input, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return fmt.Errorf("访问 %s 失败: %w", path, walkErr)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if entry.IsDir() {
				// Do not recursively ingest PhotoSheet's own output folders or
				// an external editor's export folder when the parent is selected.
				// An explicitly selected folder is still used as an input root.
				if filepath.Clean(path) != filepath.Clean(input) && isGeneratedExportDirectory(path) {
					return filepath.SkipDir
				}
				return nil
			}
			if isSupportedInput(path) {
				paths = append(paths, path)
			}
			return nil
		}); err != nil {
			return nil, "", err
		}
	}
	sort.Strings(paths)
	paths = unique(paths)
	return paths, commonRoot(normalized, paths), nil
}

func isSupportedInput(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".tif" || ext == ".tiff" || raw.IsRAWPath(path)
}

func unique(paths []string) []string {
	if len(paths) < 2 {
		return paths
	}
	result := paths[:0]
	var last string
	for _, path := range paths {
		if path == last {
			continue
		}
		result = append(result, path)
		last = path
	}
	return result
}

func commonRoot(inputs, paths []string) string {
	if len(inputs) == 1 {
		if info, err := os.Stat(inputs[0]); err == nil && info.IsDir() {
			return inputs[0]
		}
		return filepath.Dir(inputs[0])
	}
	if len(paths) == 0 {
		return filepath.Dir(inputs[0])
	}
	root := filepath.Dir(paths[0])
	for _, path := range paths[1:] {
		for !isWithin(path, root) {
			parent := filepath.Dir(root)
			if parent == root {
				return root
			}
			root = parent
		}
	}
	return root
}

func isWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func inspectOne(ctx context.Context, path, root string, index int) (ImageInfo, error) {
	if err := ctx.Err(); err != nil {
		return ImageInfo{}, err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return ImageInfo{}, fmt.Errorf("读取文件信息失败: %w", err)
	}
	var encodedWidth, encodedHeight, displayWidth, displayHeight, orientation, sourceBitDepth int
	var sourceFormat string
	var profile []byte
	var rawMetadata *raw.Metadata
	if raw.IsRAWPath(path) {
		value, probeErr := raw.NewRegistry().Probe(ctx, path)
		if probeErr != nil {
			return ImageInfo{}, rawProbeError{err: fmt.Errorf("读取 RAW metadata 失败: %w", probeErr)}
		}
		if !value.DecodeAvailable {
			reason := value.DecodeReason
			if reason == "" {
				reason = "backend did not advertise a decoder"
			}
			cause := raw.ErrRawUnsupportedVariant
			if strings.Contains(strings.ToLower(value.Compression), "jpeg") {
				cause = raw.ErrRawUnsupportedCompression
			}
			return ImageInfo{}, rawProbeError{meta: value, err: fmt.Errorf("RAW file found but cannot be decoded: %s: %w", reason, cause)}
		}
		rawMetadata = &value
		encodedWidth, encodedHeight = value.EncodedWidth, value.EncodedHeight
		displayWidth, displayHeight = encodedWidth, encodedHeight
		orientation, sourceBitDepth, sourceFormat = value.DisplayOrientation, value.NativeBitDepth, string(value.Format)
		if orientation >= 5 && orientation <= 8 {
			displayWidth, displayHeight = displayHeight, displayWidth
		}
	} else {
		inspection, inspectErr := imaging.InspectFile(path)
		if inspectErr != nil {
			return ImageInfo{}, fmt.Errorf("读取图片 metadata 失败: %w", inspectErr)
		}
		encodedWidth, encodedHeight = inspection.EncodedWidth, inspection.EncodedHeight
		displayWidth, displayHeight = inspection.DisplayWidth, inspection.DisplayHeight
		orientation, sourceFormat, sourceBitDepth, profile = inspection.Orientation, inspection.Format, inspection.BitDepth, inspection.ICCProfile
	}
	sum, err := hashFile(ctx, path)
	if err != nil {
		return ImageInfo{}, err
	}
	if orientation < 1 || orientation > 8 {
		orientation = 1
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		relative = filepath.Base(path)
	}
	return ImageInfo{
		ID:              fmt.Sprintf("image-%06d", index),
		SourcePath:      path,
		Filename:        filepath.Base(path),
		RelativePath:    filepath.ToSlash(relative),
		SHA256:          hex.EncodeToString(sum[:]),
		FileSize:        stat.Size(),
		EncodedWidth:    encodedWidth,
		EncodedHeight:   encodedHeight,
		DisplayWidth:    displayWidth,
		DisplayHeight:   displayHeight,
		EXIFOrientation: orientation,
		ICCProfileHash:  metadata.ICCProfileHash(profile),
		ICCProfile:      profile,
		SourceFormat:    sourceFormat,
		SourceBitDepth:  sourceBitDepth,
		Raw:             rawMetadata,
	}, nil
}

func hashFile(ctx context.Context, path string) ([32]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, fmt.Errorf("打开文件计算 SHA-256 失败: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	buffer := make([]byte, 256*1024)
	for {
		if err := ctx.Err(); err != nil {
			return [32]byte{}, fmt.Errorf("哈希计算已取消: %w", err)
		}
		n, readErr := f.Read(buffer)
		if n > 0 {
			_, _ = h.Write(buffer[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return [32]byte{}, fmt.Errorf("读取文件计算 SHA-256 失败: %w", readErr)
		}
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}
