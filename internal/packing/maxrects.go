package packing

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"photosheet/internal/model"
)

type Item struct {
	ID     string
	Width  int
	Height int
	Order  int
}

type Placement struct {
	ItemID     string
	SheetIndex int
	Rotation   int
	Outer      model.Rect
	Content    model.Rect
}

type SheetPlan struct {
	Width          int
	Height         int
	Placements     []Placement
	UsedArea       int64
	FreeRectangles int
}

type Plan struct {
	Sheets        []SheetPlan
	Candidate     string
	EmptyArea     int64
	Fragmentation int
	CanvasArea    int64
}

type Options struct {
	MaximumLongEdge    int
	MaximumShortEdge   int
	MaximumTotalPixels int64
	Padding            int
	AllowRotation      bool
}

func OptionsFromSettings(settings model.Settings) (Options, error) {
	normalized, err := settings.Normalize()
	if err != nil {
		return Options{}, err
	}
	return Options{
		MaximumLongEdge:    int(normalized.MaximumLongEdge),
		MaximumShortEdge:   int(normalized.MaximumShortEdge),
		MaximumTotalPixels: normalized.MaximumTotalPixels,
		Padding:            normalized.Padding,
		AllowRotation:      normalized.AllowRotation,
	}, nil
}

type candidate struct {
	heuristic model.Heuristic
	sort      model.SortStrategy
	width     int
	height    int
	index     int
}

func Optimize(ctx context.Context, items []Item, options Options) (Plan, error) {
	if len(items) == 0 {
		return Plan{}, errors.New("没有可排版的图片")
	}
	if options.MaximumLongEdge <= 0 {
		return Plan{}, errors.New("最大长边必须大于 0")
	}
	if options.MaximumShortEdge <= 0 || options.MaximumShortEdge > options.MaximumLongEdge {
		options.MaximumShortEdge = options.MaximumLongEdge
	}
	if options.Padding < 0 {
		return Plan{}, errors.New("padding 不能为负数")
	}
	for _, item := range items {
		if item.Width <= 0 || item.Height <= 0 {
			return Plan{}, fmt.Errorf("图片 %s 尺寸无效: %dx%d", item.ID, item.Width, item.Height)
		}
	}
	variants := canvasVariants(options)
	heuristics := []model.Heuristic{
		model.HeuristicBestShortSideFit,
		model.HeuristicBestLongSideFit,
		model.HeuristicBestAreaFit,
		model.HeuristicBottomLeft,
		model.HeuristicContactPoint,
	}
	sorts := []model.SortStrategy{
		model.SortAreaDescending,
		model.SortMaxSideDescending,
		model.SortWidthDescending,
		model.SortHeightDescending,
		model.SortAspectRatio,
		model.SortPortraitLandscape,
	}
	candidates := make([]candidate, 0, len(variants)*len(heuristics)*len(sorts))
	index := 0
	for _, variant := range variants {
		for _, heuristic := range heuristics {
			for _, order := range sorts {
				candidates = append(candidates, candidate{heuristic: heuristic, sort: order, width: variant.width, height: variant.height, index: index})
				index++
			}
		}
	}
	results := make(chan Plan, len(candidates))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, current := range candidates {
		current := current
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			plan, err := runCandidate(ctx, items, options, current)
			if err == nil {
				results <- plan
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	var best Plan
	got := false
	for plan := range results {
		if !got || better(plan, best) {
			best, got = plan, true
		}
	}
	if err := ctx.Err(); err != nil {
		return Plan{}, fmt.Errorf("排版已取消: %w", err)
	}
	if !got {
		return Plan{}, errors.New("没有候选布局能够满足当前画布限制")
	}
	return best, nil
}

// canvasVariants returns deterministic canvas orientations. A square is enough
// for the common MaximumLongEdge-only case; short-edge and total-pixel limits
// add smaller landscape/portrait candidates.
func canvasVariants(options Options) []struct{ width, height int } {
	long, short := options.MaximumLongEdge, options.MaximumShortEdge
	variants := []struct{ width, height int }{{long, long}}
	if short < long {
		variants = append(variants, struct{ width, height int }{long, short}, struct{ width, height int }{short, long})
	}
	if options.MaximumTotalPixels > 0 {
		capPixels := options.MaximumTotalPixels
		for _, v := range append([]struct{ width, height int }(nil), variants...) {
			if int64(v.width)*int64(v.height) > capPixels {
				h := int(capPixels / int64(v.width))
				if h > 0 {
					variants = append(variants, struct{ width, height int }{v.width, h})
				}
			}
		}
		side := intCeilSqrt(capPixels)
		if side > long {
			side = long
		}
		variants = append(variants, struct{ width, height int }{side, side})
	}
	return uniqueCanvasVariants(variants)
}

func uniqueCanvasVariants(input []struct{ width, height int }) []struct{ width, height int } {
	seen := make(map[string]struct{}, len(input))
	result := make([]struct{ width, height int }, 0, len(input))
	for _, v := range input {
		if v.width <= 0 || v.height <= 0 {
			continue
		}
		key := fmt.Sprintf("%d:%d", v.width, v.height)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, v)
	}
	return result
}

func intCeilSqrt(n int64) int {
	if n <= 1 {
		return 1
	}
	lo, hi := int64(1), n
	for lo < hi {
		mid := lo + (hi-lo)/2
		if mid >= n/mid {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return int(lo)
}

func runCandidate(ctx context.Context, items []Item, options Options, current candidate) (Plan, error) {
	ordered := append([]Item(nil), items...)
	sortItems(ordered, current.sort)
	result := Plan{Candidate: fmt.Sprintf("%s/%s/%dx%d", current.heuristic, current.sort, current.width, current.height)}
	sheets := make([]sheetWithAllocator, 0)
	for i, item := range ordered {
		if i%32 == 0 {
			if err := ctx.Err(); err != nil {
				return Plan{}, err
			}
		}
		outerWidth, outerHeight := item.Width+2*options.Padding, item.Height+2*options.Padding
		if outerWidth <= 0 || outerHeight <= 0 {
			return Plan{}, fmt.Errorf("图片 %s padding 后尺寸溢出", item.ID)
		}
		if (outerWidth > current.width || outerHeight > current.height) && (!options.AllowRotation || outerHeight > current.width || outerWidth > current.height) {
			return Plan{}, fmt.Errorf("图片 %s 尺寸 %dx%d 超过画布 %dx%d", item.ID, outerWidth, outerHeight, current.width, current.height)
		}
		placed := false
		for sheetIndex := range sheets {
			allocation, ok := sheets[sheetIndex].allocator.insert(item, options.Padding, options.AllowRotation, current.heuristic)
			if !ok {
				continue
			}
			sheets[sheetIndex].Placements = append(sheets[sheetIndex].Placements, makePlacement(item, sheetIndex, options.Padding, allocation))
			sheets[sheetIndex].UsedArea += allocation.rect.Area()
			placed = true
			break
		}
		if !placed {
			allocator := newAllocator(current.width, current.height)
			allocation, ok := allocator.insert(item, options.Padding, options.AllowRotation, current.heuristic)
			if !ok {
				return Plan{}, fmt.Errorf("图片 %s 无法放入画布", item.ID)
			}
			sheets = append(sheets, sheetWithAllocator{SheetPlan: SheetPlan{Placements: []Placement{makePlacement(item, len(sheets), options.Padding, allocation)}}, allocator: allocator})
			sheets[len(sheets)-1].UsedArea = allocation.rect.Area()
		}
	}
	for i := range sheets {
		sheet := &sheets[i]
		maxRight, maxBottom := 0, 0
		for _, placement := range sheet.Placements {
			if placement.Outer.Right() > maxRight {
				maxRight = placement.Outer.Right()
			}
			if placement.Outer.Bottom() > maxBottom {
				maxBottom = placement.Outer.Bottom()
			}
		}
		sheet.Width, sheet.Height = maxRight, maxBottom
		if sheet.Width <= 0 || sheet.Height <= 0 {
			return Plan{}, errors.New("生成了空 Sheet")
		}
		if sheet.Width > current.width || sheet.Height > current.height || int64(sheet.Width)*int64(sheet.Height) > options.MaximumTotalPixels && options.MaximumTotalPixels > 0 {
			return Plan{}, errors.New("布局裁剪后的画布仍超出限制")
		}
		if min(sheet.Width, sheet.Height) > options.MaximumShortEdge {
			return Plan{}, errors.New("布局超出最大短边限制")
		}
		sheet.FreeRectangles = len(sheet.allocator.free)
		result.CanvasArea += int64(sheet.Width) * int64(sheet.Height)
		result.Fragmentation += sheet.FreeRectangles
		result.Sheets = append(result.Sheets, sheet.SheetPlan)
	}
	var used int64
	for _, sheet := range result.Sheets {
		used += sheet.UsedArea
	}
	result.EmptyArea = result.CanvasArea - used
	return result, nil
}

type sheetWithAllocator struct {
	SheetPlan
	allocator *allocator
}

// custom type conversion helpers keep allocator state private while exposing a
// plain SheetPlan in the public result.
func (s sheetWithAllocator) toPlan() SheetPlan { return s.SheetPlan }

func makePlacement(item Item, sheetIndex, padding int, allocation allocation) Placement {
	contentWidth, contentHeight := allocation.rect.Width-2*padding, allocation.rect.Height-2*padding
	return Placement{
		ItemID:     item.ID,
		SheetIndex: sheetIndex,
		Rotation:   allocation.rotation,
		Outer:      model.Rect{X: allocation.rect.X, Y: allocation.rect.Y, Width: allocation.rect.Width, Height: allocation.rect.Height},
		Content:    model.Rect{X: allocation.rect.X + padding, Y: allocation.rect.Y + padding, Width: contentWidth, Height: contentHeight},
	}
}

func better(a, b Plan) bool {
	if len(a.Sheets) != len(b.Sheets) {
		return len(a.Sheets) < len(b.Sheets)
	}
	if a.EmptyArea != b.EmptyArea {
		return a.EmptyArea < b.EmptyArea
	}
	if a.Fragmentation != b.Fragmentation {
		return a.Fragmentation < b.Fragmentation
	}
	if a.CanvasArea != b.CanvasArea {
		return a.CanvasArea < b.CanvasArea
	}
	return strings.Compare(a.Candidate, b.Candidate) < 0
}

func sortItems(items []Item, strategy model.SortStrategy) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		areaA, areaB := int64(a.Width)*int64(a.Height), int64(b.Width)*int64(b.Height)
		maxA, maxB := max(a.Width, a.Height), max(b.Width, b.Height)
		minA, minB := min(a.Width, a.Height), min(b.Width, b.Height)
		less := false
		switch strategy {
		case model.SortMaxSideDescending:
			less = maxA > maxB || maxA == maxB && minA > minB
		case model.SortWidthDescending:
			less = a.Width > b.Width || a.Width == b.Width && a.Height > b.Height
		case model.SortHeightDescending:
			less = a.Height > b.Height || a.Height == b.Height && a.Width > b.Width
		case model.SortAspectRatio:
			less = int64(a.Width)*int64(b.Height) > int64(b.Width)*int64(a.Height)
		case model.SortPortraitLandscape:
			portraitA, portraitB := a.Height > a.Width, b.Height > b.Width
			less = boolRank(portraitA) < boolRank(portraitB)
			if portraitA == portraitB {
				less = areaA > areaB
			}
		default:
			less = areaA > areaB || areaA == areaB && maxA > maxB
		}
		if areaA == areaB && maxA == maxB && minA == minB {
			return a.ID < b.ID
		}
		if a.Width == b.Width && a.Height == b.Height {
			return a.ID < b.ID
		}
		return less
	})
}

func boolRank(value bool) int {
	if value {
		return 0
	}
	return 1
}

type allocation struct {
	rect     model.Rect
	rotation int
	score    score
}

type score struct {
	primary   int64
	secondary int64
	touch     int64
	x         int
	y         int
	width     int
	height    int
	rotation  int
}

type allocator struct {
	width  int
	height int
	free   []model.Rect
	used   []model.Rect
}

func newAllocator(width, height int) *allocator {
	return &allocator{width: width, height: height, free: []model.Rect{{Width: width, Height: height}}}
}

func (a *allocator) insert(item Item, padding int, allowRotation bool, heuristic model.Heuristic) (allocation, bool) {
	outerW, outerH := item.Width+2*padding, item.Height+2*padding
	best := allocation{}
	found := false
	for freeIndex, free := range a.free {
		orientations := []struct {
			width, height, rotation int
		}{{outerW, outerH, 0}}
		if allowRotation && outerW != outerH {
			orientations = append(orientations, struct{ width, height, rotation int }{outerH, outerW, 90})
		}
		for _, orientation := range orientations {
			if orientation.width > free.Width || orientation.height > free.Height {
				continue
			}
			rect := model.Rect{X: free.X, Y: free.Y, Width: orientation.width, Height: orientation.height}
			candidateScore := scoreFor(heuristic, a, free, rect, orientation.rotation, freeIndex)
			candidate := allocation{rect: rect, rotation: orientation.rotation, score: candidateScore}
			if !found || scoreLess(candidate.score, best.score) {
				best, found = candidate, true
			}
		}
	}
	if !found {
		return allocation{}, false
	}
	a.place(best.rect)
	return best, true
}

func scoreFor(heuristic model.Heuristic, a *allocator, free, rect model.Rect, rotation, freeIndex int) score {
	leftoverHoriz := abs(free.Width - rect.Width)
	leftoverVert := abs(free.Height - rect.Height)
	shortSide, longSide := min(leftoverHoriz, leftoverVert), max(leftoverHoriz, leftoverVert)
	areaFit := free.Area() - rect.Area()
	contact := contactPointScore(a, rect)
	base := score{x: rect.X, y: rect.Y, width: rect.Width, height: rect.Height, rotation: rotation}
	switch heuristic {
	case model.HeuristicBestLongSideFit:
		base.primary, base.secondary = int64(longSide), int64(shortSide)
	case model.HeuristicBestAreaFit:
		base.primary, base.secondary = areaFit, int64(shortSide)
	case model.HeuristicBottomLeft:
		base.primary, base.secondary = int64(rect.Bottom()), int64(rect.X)
	case model.HeuristicContactPoint:
		base.primary, base.secondary, base.touch = -contact, int64(shortSide), int64(longSide)
	default:
		base.primary, base.secondary = int64(shortSide), int64(longSide)
	}
	// freeIndex is a final deterministic tie breaker before geometry fields.
	base.touch += int64(freeIndex)
	return base
}

func scoreLess(a, b score) bool {
	if a.primary != b.primary {
		return a.primary < b.primary
	}
	if a.secondary != b.secondary {
		return a.secondary < b.secondary
	}
	if a.touch != b.touch {
		return a.touch < b.touch
	}
	if a.y != b.y {
		return a.y < b.y
	}
	if a.x != b.x {
		return a.x < b.x
	}
	if a.rotation != b.rotation {
		return a.rotation < b.rotation
	}
	if a.width != b.width {
		return a.width < b.width
	}
	return a.height < b.height
}

func (a *allocator) place(used model.Rect) {
	next := make([]model.Rect, 0, len(a.free)+4)
	for _, free := range a.free {
		if !intersects(free, used) {
			next = append(next, free)
			continue
		}
		if used.X > free.X {
			next = append(next, model.Rect{X: free.X, Y: free.Y, Width: used.X - free.X, Height: free.Height})
		}
		if used.Right() < free.Right() {
			next = append(next, model.Rect{X: used.Right(), Y: free.Y, Width: free.Right() - used.Right(), Height: free.Height})
		}
		if used.Y > free.Y {
			next = append(next, model.Rect{X: free.X, Y: free.Y, Width: free.Width, Height: used.Y - free.Y})
		}
		if used.Bottom() < free.Bottom() {
			next = append(next, model.Rect{X: free.X, Y: used.Bottom(), Width: free.Width, Height: free.Bottom() - used.Bottom()})
		}
	}
	a.free = pruneFreeRectangles(next)
	a.used = append(a.used, used)
}

func pruneFreeRectangles(rectangles []model.Rect) []model.Rect {
	valid := rectangles[:0]
	for _, rect := range rectangles {
		if rect.Empty() {
			continue
		}
		valid = append(valid, rect)
	}
	result := make([]model.Rect, 0, len(valid))
	for i, rect := range valid {
		contained := false
		for j, other := range valid {
			if i != j && other.Contains(rect) {
				contained = true
				break
			}
		}
		if !contained {
			result = append(result, rect)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		if a.X != b.X {
			return a.X < b.X
		}
		if a.Width != b.Width {
			return a.Width < b.Width
		}
		return a.Height < b.Height
	})
	return result
}

func intersects(a, b model.Rect) bool {
	return a.X < b.Right() && a.Right() > b.X && a.Y < b.Bottom() && a.Bottom() > b.Y
}

func contactPointScore(a *allocator, rect model.Rect) int64 {
	var score int64
	if rect.X == 0 || rect.Right() == a.width {
		score += int64(rect.Height)
	}
	if rect.Y == 0 || rect.Bottom() == a.height {
		score += int64(rect.Width)
	}
	for _, used := range a.used {
		if used.X == rect.Right() || used.Right() == rect.X {
			score += int64(commonLength(used.Y, used.Bottom(), rect.Y, rect.Bottom()))
		}
		if used.Y == rect.Bottom() || used.Bottom() == rect.Y {
			score += int64(commonLength(used.X, used.Right(), rect.X, rect.Right()))
		}
	}
	return score
}

func commonLength(aStart, aEnd, bStart, bEnd int) int {
	start, end := max(aStart, bStart), min(aEnd, bEnd)
	if end <= start {
		return 0
	}
	return end - start
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
