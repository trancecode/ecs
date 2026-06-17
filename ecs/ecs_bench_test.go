package ecs

import (
	"strconv"
	"testing"
)

// benchSizes spans the cache regimes discussed in the design: a few thousand
// entities sit in L1/L2, tens of thousands in L2/L3, a hundred thousand spills
// toward L3 and beyond. Running each benchmark across them surfaces where the
// per-component array stops fitting in fast cache.
var benchSizes = []int{1_000, 10_000, 100_000}

// benchPositions builds a world of n entities each carrying a position, and
// returns the world plus the ids in allocation order.
func benchPositions(n int) (*World, []EntityId) {
	w := NewWorld()
	pos := Components[position](w)
	ids := make([]EntityId, n)
	for i := range ids {
		e := w.NewEntity()
		ids[i] = e
		pos.Add(e, position{X: float64(i)})
	}
	return w, ids
}

// BenchmarkGet measures random-access point lookup (the per-access map cost).
func BenchmarkGet(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			w, ids := benchPositions(n)
			pos := Components[position](w)
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				_, _ = pos.Get(ids[i%n])
			}
		})
	}
}

// BenchmarkHas measures the presence check (map lookup, no pointer return).
func BenchmarkHas(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			w, ids := benchPositions(n)
			pos := Components[position](w)
			b.ResetTimer()
			for i := range b.N {
				_ = pos.Has(ids[i%n])
			}
		})
	}
}

// BenchmarkAddOverwrite measures Add on an entity that already has the component
// (the in-place overwrite path), keeping the store size stable across iterations.
func BenchmarkAddOverwrite(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			w, ids := benchPositions(n)
			pos := Components[position](w)
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				pos.Add(ids[i%n], position{X: 1})
			}
		})
	}
}

// BenchmarkIterateSingle measures streaming iteration over one component type,
// the cache-friendly best case.
func BenchmarkIterateSingle(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			w, _ := benchPositions(n)
			pos := Components[position](w)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				var sum float64
				for _, p := range pos.All() {
					sum += p.X
				}
				_ = sum
			}
		})
	}
}

// BenchmarkIterateJoin2 measures two-component joined iteration, where each entity
// in store A costs one lookup into store B. This is the per-entity join cost that
// the sparse-set-versus-archetype decision hinges on.
func BenchmarkIterateJoin2(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			w := NewWorld()
			moving := Components2[position, velocity](w)
			for i := range n {
				e := w.NewEntity()
				moving.Add(e, position{X: float64(i)}, velocity{DX: 1})
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				var sum float64
				for _, tup := range moving.All() {
					p, v := tup.Values()
					sum += p.X + v.DX
				}
				_ = sum
			}
		})
	}
}
