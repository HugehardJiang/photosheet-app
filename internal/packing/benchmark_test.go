package packing

import (
	"context"
	"strconv"
	"testing"
)

func benchmarkPacking(b *testing.B, count int) {
	items := make([]Item, 0, count)
	for i := 0; i < count; i++ {
		items = append(items, Item{ID: "image-" + strconv.Itoa(i), Width: 64 + i%257, Height: 48 + (i*17)%193})
	}
	options := Options{MaximumLongEdge: 4096, MaximumShortEdge: 4096, Padding: 8, AllowRotation: true}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Optimize(context.Background(), items, options); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPacking100(b *testing.B)  { benchmarkPacking(b, 100) }
func BenchmarkPacking1000(b *testing.B) { benchmarkPacking(b, 1000) }
