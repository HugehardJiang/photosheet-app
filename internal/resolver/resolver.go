// Package resolver matches processed Sheet files to the stable Sheet
// identities stored in project.json. It intentionally has no dependency on
// the original-photo directory: an edited export may live anywhere.
package resolver

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"photosheet/internal/imaging"
	"photosheet/internal/model"
)

type Status string

const (
	StatusMatched          Status = "matched"
	StatusAmbiguous        Status = "ambiguous"
	StatusMissing          Status = "missing"
	StatusInvalid          Status = "invalid"
	StatusDimensionChanged Status = "dimension-changed"
	StatusUniformScaled    Status = "uniform-scaled"
	StatusUnsafeTransform  Status = "unsafe-transform"
)

type DimensionStatus string

const (
	DimensionExact           DimensionStatus = "exact"
	DimensionUniformScaled   DimensionStatus = "uniform-scaled"
	DimensionUnsafeTransform DimensionStatus = "unsafe-transform"
)

type Progress struct {
	Processed int    `json:"processed"`
	Total     int    `json:"total"`
	Current   string `json:"current"`
	Message   string `json:"message"`
}

type Options struct {
	Sources   []string
	Recursive bool
	// Bindings maps a Sheet ID, canonicalStem, or project Sheet filename to an
	// explicit processed file path. Manual bindings always win over scoring.
	Bindings map[string]string
	Progress func(Progress)
}

type CandidateMatch struct {
	Path            string          `json:"path"`
	SheetID         string          `json:"sheetId"`
	Status          Status          `json:"status"`
	Score           int             `json:"score"`
	Reasons         []string        `json:"reasons"`
	EncodedWidth    int             `json:"encodedWidth"`
	EncodedHeight   int             `json:"encodedHeight"`
	DisplayWidth    int             `json:"displayWidth"`
	DisplayHeight   int             `json:"displayHeight"`
	Format          string          `json:"format"`
	ModifiedAt      string          `json:"modifiedAt"`
	DimensionStatus DimensionStatus `json:"dimensionStatus"`
	InvalidReason   string          `json:"invalidReason,omitempty"`
}

type ResolvedSheet struct {
	SheetID         string           `json:"sheetId"`
	Index           int              `json:"index"`
	CanonicalStem   string           `json:"canonicalStem"`
	Status          Status           `json:"status"`
	DimensionStatus DimensionStatus  `json:"dimensionStatus,omitempty"`
	Path            string           `json:"path,omitempty"`
	Reason          string           `json:"reason,omitempty"`
	ScaleX          float64          `json:"scaleX,omitempty"`
	ScaleY          float64          `json:"scaleY,omitempty"`
	Manual          bool             `json:"manual"`
	Candidate       *CandidateMatch  `json:"candidate,omitempty"`
	Candidates      []CandidateMatch `json:"candidates"`
}

type Result struct {
	Sheets   []ResolvedSheet `json:"sheets"`
	Warnings []string        `json:"warnings"`
}

type candidateFile struct {
	Path       string
	Stem       string
	Size       int64
	ModifiedAt time.Time
}

// NormalizeStem removes only the filename extension and normalizes matching
// case. Unicode filenames remain intact; the comparison is intentionally
// separator-aware so a short token cannot match an unrelated Sheet.
func NormalizeStem(filename string) string {
	base := filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	base = strings.TrimSpace(base)
	return strings.ToLower(strings.TrimSuffix(base, filepath.Ext(base)))
}

func Resolve(ctx context.Context, p *model.Project, options Options) (Result, error) {
	if p == nil {
		return Result{}, errors.New("project 为空")
	}
	if err := p.Validate(); err != nil {
		return Result{}, fmt.Errorf("project 无效: %w", err)
	}
	if len(options.Sources) == 0 && len(options.Bindings) == 0 {
		return Result{}, errors.New("至少需要一个处理后 Sheet 文件夹、文件或手动绑定")
	}
	files, err := collectCandidates(ctx, options.Sources, options.Recursive)
	if err != nil {
		return Result{}, err
	}
	byPath := make(map[string]candidateFile, len(files))
	for _, file := range files {
		byPath[file.Path] = file
	}

	result := Result{Sheets: make([]ResolvedSheet, 0, len(p.Sheets)), Warnings: make([]string, 0)}
	for index, sheet := range p.Sheets {
		if err := ctx.Err(); err != nil {
			return Result{}, fmt.Errorf("解析处理后 Sheet 已取消: %w", err)
		}
		resolved, err := resolveSheet(ctx, p, sheet, files, byPath, options.Bindings)
		if err != nil {
			return Result{}, fmt.Errorf("解析 Sheet %s 失败: %w", sheet.File, err)
		}
		result.Sheets = append(result.Sheets, resolved)
		if options.Progress != nil {
			options.Progress(Progress{Processed: index + 1, Total: len(p.Sheets), Current: sheet.File, Message: "正在匹配处理后的 Sheet"})
		}
	}
	return result, nil
}

func collectCandidates(ctx context.Context, sources []string, recursive bool) ([]candidateFile, error) {
	seen := make(map[string]struct{})
	files := make([]candidateFile, 0)
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("扫描处理后位置已取消: %w", err)
		}
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		info, err := os.Stat(source)
		if err != nil {
			return nil, fmt.Errorf("无法读取处理后位置 %s: %w", source, err)
		}
		if !info.IsDir() {
			if imaging.SupportedProcessedExtension(source) {
				file, buildErr := makeCandidate(source, info)
				if buildErr != nil {
					return nil, buildErr
				}
				if _, exists := seen[file.Path]; !exists {
					seen[file.Path] = struct{}{}
					files = append(files, file)
				}
			}
			continue
		}
		if recursive {
			err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return fmt.Errorf("读取 %s 失败: %w", path, walkErr)
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				if path != source && entry.Type()&os.ModeSymlink != 0 {
					if entry.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if entry.IsDir() || !imaging.SupportedProcessedExtension(path) {
					return nil
				}
				info, statErr := entry.Info()
				if statErr != nil {
					return fmt.Errorf("读取 %s 信息失败: %w", path, statErr)
				}
				file, buildErr := makeCandidate(path, info)
				if buildErr != nil {
					return buildErr
				}
				if _, exists := seen[file.Path]; !exists {
					seen[file.Path] = struct{}{}
					files = append(files, file)
				}
				return nil
			})
		} else {
			entries, readErr := os.ReadDir(source)
			if readErr != nil {
				return nil, fmt.Errorf("读取处理后文件夹 %s 失败: %w", source, readErr)
			}
			for _, entry := range entries {
				if err := ctx.Err(); err != nil {
					return nil, fmt.Errorf("扫描处理后位置已取消: %w", err)
				}
				if entry.IsDir() || !imaging.SupportedProcessedExtension(entry.Name()) {
					continue
				}
				path := filepath.Join(source, entry.Name())
				info, statErr := entry.Info()
				if statErr != nil {
					return nil, fmt.Errorf("读取 %s 信息失败: %w", path, statErr)
				}
				file, buildErr := makeCandidate(path, info)
				if buildErr != nil {
					return nil, buildErr
				}
				if _, exists := seen[file.Path]; !exists {
					seen[file.Path] = struct{}{}
					files = append(files, file)
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("扫描处理后位置 %s 失败: %w", source, err)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func makeCandidate(path string, info os.FileInfo) (candidateFile, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return candidateFile{}, fmt.Errorf("规范化处理后路径失败: %w", err)
	}
	return candidateFile{Path: abs, Stem: NormalizeStem(path), Size: info.Size(), ModifiedAt: info.ModTime()}, nil
}

func resolveSheet(ctx context.Context, p *model.Project, sheet model.Sheet, files []candidateFile, byPath map[string]candidateFile, bindings map[string]string) (ResolvedSheet, error) {
	base := ResolvedSheet{
		SheetID: sheet.ID, Index: sheet.Index, CanonicalStem: sheet.CanonicalStem,
		Status: StatusMissing, Candidates: make([]CandidateMatch, 0),
	}
	if binding := findBinding(sheet, bindings); binding != "" {
		base.Manual = true
		path, err := filepath.Abs(binding)
		if err != nil {
			base.Status = StatusInvalid
			base.Reason = fmt.Sprintf("手动绑定路径无效: %v", err)
			return base, nil
		}
		candidate, exists := byPath[path]
		if !exists {
			info, statErr := os.Stat(path)
			if statErr != nil {
				base.Path = path
				base.Reason = fmt.Sprintf("手动绑定文件不存在: %v", statErr)
				return base, nil
			}
			candidate, err = makeCandidate(path, info)
			if err != nil {
				return base, err
			}
		}
		matched, status, err := inspectMatch(ctx, sheet, candidate, 100, []string{"用户手动绑定"})
		if err != nil {
			return base, err
		}
		base.Path = candidate.Path
		base.Candidate = &matched
		base.Candidates = []CandidateMatch{matched}
		base.Status = status
		base.DimensionStatus = matched.DimensionStatus
		base.ScaleX, base.ScaleY = scaleFor(sheet, matched)
		base.Reason = statusReason(sheet, status, matched)
		return base, nil
	}

	type scoredFile struct {
		file   candidateFile
		score  int
		reason []string
	}
	canonical := NormalizeStem(sheet.CanonicalStem)
	scored := make([]scoredFile, 0)
	for _, file := range files {
		if score, reasons := scoreStem(canonical, file.Stem); score > 0 {
			scored = append(scored, scoredFile{file: file, score: score, reason: reasons})
		}
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if !scored[i].file.ModifiedAt.Equal(scored[j].file.ModifiedAt) {
			return scored[i].file.ModifiedAt.After(scored[j].file.ModifiedAt)
		}
		return scored[i].file.Path < scored[j].file.Path
	})
	for _, item := range scored {
		matched, _, err := inspectMatch(ctx, sheet, item.file, item.score, item.reason)
		if err != nil {
			return base, err
		}
		base.Candidates = append(base.Candidates, matched)
	}
	if len(base.Candidates) == 0 {
		base.Reason = fmt.Sprintf("未找到与 %s 匹配的文件名", sheet.CanonicalStem)
		return base, nil
	}
	if len(base.Candidates) > 1 {
		base.Status = StatusAmbiguous
		base.Reason = fmt.Sprintf("找到 %d 个候选，必须手动选择；不会按修改时间自动猜测", len(base.Candidates))
		return base, nil
	}
	selected := base.Candidates[0]
	base.Candidate = &selected
	base.Path = selected.Path
	if selected.InvalidReason != "" {
		base.Status = StatusInvalid
		base.Reason = selected.InvalidReason
		return base, nil
	}
	base.Status = dimensionToStatus(selected.DimensionStatus)
	base.DimensionStatus = selected.DimensionStatus
	base.ScaleX, base.ScaleY = scaleFor(sheet, selected)
	base.Reason = statusReason(sheet, base.Status, selected)
	return base, nil
}

func findBinding(sheet model.Sheet, bindings map[string]string) string {
	if len(bindings) == 0 {
		return ""
	}
	keys := []string{sheet.ID, sheet.CanonicalStem, sheet.File}
	for _, key := range keys {
		for candidateKey, path := range bindings {
			if strings.EqualFold(strings.TrimSpace(candidateKey), strings.TrimSpace(key)) {
				return strings.TrimSpace(path)
			}
		}
	}
	return ""
}

func inspectMatch(ctx context.Context, sheet model.Sheet, file candidateFile, score int, reasons []string) (CandidateMatch, Status, error) {
	if err := ctx.Err(); err != nil {
		return CandidateMatch{}, StatusInvalid, fmt.Errorf("检查候选已取消: %w", err)
	}
	match := CandidateMatch{Path: file.Path, SheetID: sheet.ID, Score: score, Reasons: append([]string(nil), reasons...), ModifiedAt: file.ModifiedAt.UTC().Format(time.RFC3339Nano)}
	inspection, err := imaging.InspectFile(file.Path)
	if err != nil {
		match.Status = StatusInvalid
		match.InvalidReason = fmt.Sprintf("候选文件无效 %s: %v", filepath.Base(file.Path), err)
		return match, StatusInvalid, nil
	}
	match.EncodedWidth = inspection.EncodedWidth
	match.EncodedHeight = inspection.EncodedHeight
	match.DisplayWidth = inspection.DisplayWidth
	match.DisplayHeight = inspection.DisplayHeight
	match.Format = inspection.Format
	match.DimensionStatus = dimensionStatus(sheet, match)
	match.Status = dimensionToStatus(match.DimensionStatus)
	match.Reasons = append(match.Reasons, fmt.Sprintf("显示尺寸 %dx%d", match.DisplayWidth, match.DisplayHeight))
	return match, dimensionToStatus(match.DimensionStatus), nil
}

func dimensionStatus(sheet model.Sheet, candidate CandidateMatch) DimensionStatus {
	if candidate.DisplayWidth == sheet.Width && candidate.DisplayHeight == sheet.Height {
		return DimensionExact
	}
	if sheet.Width <= 0 || sheet.Height <= 0 || candidate.DisplayWidth <= 0 || candidate.DisplayHeight <= 0 {
		return DimensionUnsafeTransform
	}
	scaleX := float64(candidate.DisplayWidth) / float64(sheet.Width)
	scaleY := float64(candidate.DisplayHeight) / float64(sheet.Height)
	if math.Abs(scaleX-scaleY) <= 1e-9 {
		return DimensionUniformScaled
	}
	return DimensionUnsafeTransform
}

func scaleFor(sheet model.Sheet, candidate CandidateMatch) (float64, float64) {
	if sheet.Width <= 0 || sheet.Height <= 0 {
		return 0, 0
	}
	return float64(candidate.DisplayWidth) / float64(sheet.Width), float64(candidate.DisplayHeight) / float64(sheet.Height)
}

func dimensionToStatus(status DimensionStatus) Status {
	switch status {
	case DimensionExact:
		return StatusMatched
	case DimensionUniformScaled:
		return StatusUniformScaled
	default:
		return StatusUnsafeTransform
	}
}

func statusReason(sheet model.Sheet, status Status, candidate CandidateMatch) string {
	switch status {
	case StatusMatched:
		return fmt.Sprintf("已匹配，显示尺寸 %dx%d 与 JSON 一致", candidate.DisplayWidth, candidate.DisplayHeight)
	case StatusUniformScaled:
		scaleX, scaleY := scaleFor(sheet, candidate)
		return fmt.Sprintf("检测到等比例缩放 %.6fx/%.6fy；需确认后按比例恢复", scaleX, scaleY)
	case StatusUnsafeTransform:
		return fmt.Sprintf("显示尺寸 %dx%d 与 JSON 不一致，且不是等比例缩放", candidate.DisplayWidth, candidate.DisplayHeight)
	default:
		return string(status)
	}
}

func scoreStem(canonical, candidate string) (int, []string) {
	canonical = strings.TrimSpace(strings.ToLower(canonical))
	candidate = strings.TrimSpace(strings.ToLower(candidate))
	if canonical == "" || candidate == "" {
		return 0, nil
	}
	if candidate == canonical {
		return 100, []string{"文件名 stem 完全一致"}
	}
	if hasSuffixToken(candidate, canonical) {
		return 90, []string{"canonicalStem 一致，文件名带处理后缀"}
	}
	for _, prefix := range []string{"export_", "export-", "processed_", "processed-", "edited_", "edited-", "retouched_", "retouched-"} {
		if strings.HasPrefix(candidate, prefix) {
			rest := strings.TrimPrefix(candidate, prefix)
			if rest == canonical {
				return 75, []string{"检测到 export/processed 前缀，置信度较低"}
			}
			if hasSuffixToken(rest, canonical) {
				return 70, []string{"检测到 export/processed 前缀与处理后缀，置信度较低"}
			}
		}
	}
	if tokenContains(candidate, canonical) {
		return 60, []string{"仅检测到带边界的 canonicalStem token，置信度较低"}
	}
	return 0, nil
}

func hasSuffixToken(candidate, canonical string) bool {
	if !strings.HasPrefix(candidate, canonical) || len(candidate) == len(canonical) {
		return false
	}
	return boundaryAfter(candidate[len(canonical):])
}

func tokenContains(candidate, token string) bool {
	start := 0
	for {
		i := strings.Index(candidate[start:], token)
		if i < 0 {
			return false
		}
		i += start
		if boundaryBefore(candidate[:i]) && boundaryAfter(candidate[i+len(token):]) {
			return true
		}
		start = i + len(token)
		if start >= len(candidate) {
			return false
		}
	}
}

func boundaryBefore(value string) bool {
	if value == "" {
		return true
	}
	runes := []rune(value)
	if len(runes) == 0 {
		return true
	}
	r := runes[len(runes)-1]
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return r > unicode.MaxASCII
	}
	return true
}

func boundaryAfter(value string) bool {
	if value == "" {
		return true
	}
	runes := []rune(value)
	if len(runes) == 0 {
		return true
	}
	r := runes[0]
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return r > unicode.MaxASCII
	}
	return true
}

// ValidateForExtraction rejects every unresolved or unsafe state. Uniform
// scaling is accepted only when the caller explicitly confirms it.
func ValidateForExtraction(result Result, allowUniformScale bool) error {
	for _, sheet := range result.Sheets {
		switch sheet.Status {
		case StatusMatched:
			continue
		case StatusUniformScaled:
			if allowUniformScale {
				continue
			}
			return fmt.Errorf("Sheet %s 检测到等比例缩放，必须显式确认按比例恢复", sheet.CanonicalStem)
		default:
			return fmt.Errorf("Sheet %s 无法安全解析：%s%s", sheet.CanonicalStem, sheet.Status, reasonSuffix(sheet.Reason))
		}
	}
	return nil
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return "（" + reason + "）"
}
