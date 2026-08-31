package packing

import (
	"context"
	"math/rand"
	"testing"

	"photosheet/internal/model"
)

func TestMaxRectsDeterministicAndValid(t *testing.T) {
	items := []Item{
		{ID: "a", Width: 31, Height: 17},
		{ID: "b", Width: 12, Height: 27},
		{ID: "c", Width: 19, Height: 19},
		{ID: "d", Width: 7, Height: 11},
		{ID: "e", Width: 9, Height: 23},
	}
	options := Options{MaximumLongEdge: 64, MaximumShortEdge: 64, Padding: 2, AllowRotation: true}
	first, err := Optimize(context.Background(), items, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Optimize(context.Background(), items, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Sheets) != len(second.Sheets) {
		t.Fatalf("sheet count changed: %d vs %d", len(first.Sheets), len(second.Sheets))
	}
	for i := range first.Sheets {
		if first.Sheets[i].Width != second.Sheets[i].Width || first.Sheets[i].Height != second.Sheets[i].Height {
			t.Fatalf("sheet %d dimensions changed", i)
		}
		if len(first.Sheets[i].Placements) != len(second.Sheets[i].Placements) {
			t.Fatalf("sheet %d placement count changed", i)
		}
		for j := range first.Sheets[i].Placements {
			if first.Sheets[i].Placements[j] != second.Sheets[i].Placements[j] {
				t.Fatalf("placement %d/%d changed: %#v vs %#v", i, j, first.Sheets[i].Placements[j], second.Sheets[i].Placements[j])
			}
		}
		assertValidSheet(t, first.Sheets[i])
	}
}

func TestMaxRectsRandomNoOverlap(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for iteration := 0; iteration < 500; iteration++ {
		items := make([]Item, 0, 20)
		for i := 0; i < 20; i++ {
			items = append(items, Item{ID: string(rune('a' + i)), Width: 1 + rng.Intn(15), Height: 1 + rng.Intn(15)})
		}
		plan, err := Optimize(context.Background(), items, Options{MaximumLongEdge: 64, MaximumShortEdge: 64, Padding: rng.Intn(3), AllowRotation: true})
		if err != nil {
			t.Fatalf("iteration %d: %v", iteration, err)
		}
		for _, sheet := range plan.Sheets {
			assertValidSheet(t, sheet)
		}
	}
}

func assertValidSheet(t *testing.T, sheet SheetPlan) {
	t.Helper()
	for i, placement := range sheet.Placements {
		if placement.Outer.X < 0 || placement.Outer.Y < 0 || placement.Outer.Right() > sheet.Width || placement.Outer.Bottom() > sheet.Height {
			t.Fatalf("placement %s outside sheet %dx%d: %#v", placement.ItemID, sheet.Width, sheet.Height, placement.Outer)
		}
		if !placement.Outer.Contains(placement.Content) {
			t.Fatalf("content outside outer: %#v vs %#v", placement.Outer, placement.Content)
		}
		for j := 0; j < i; j++ {
			if overlaps(sheet.Placements[j].Outer, placement.Outer) {
				t.Fatalf("overlap: %#v and %#v", sheet.Placements[j].Outer, placement.Outer)
			}
		}
	}
}

func overlaps(a, b model.Rect) bool {
	return a.X < b.Right() && a.Right() > b.X && a.Y < b.Bottom() && a.Bottom() > b.Y
}
