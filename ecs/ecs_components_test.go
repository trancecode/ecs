package ecs

import (
	"sort"
	"testing"
)

func TestComponentsAddGetHas(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()

	if pos.Has(a) {
		t.Fatal("entity must not have the component yet")
	}
	pos.Add(a, position{X: 1, Y: 2})
	if !pos.Has(a) {
		t.Fatal("entity must have the component after Add")
	}
	p, ok := pos.Get(a)
	if !ok || p.X != 1 || p.Y != 2 {
		t.Fatalf("Get returned ok=%v %+v, want true {1 2}", ok, p)
	}
}

func TestComponentsGetMissingReturnsFalse(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	_, ok := pos.Get(w.NewEntity())
	if ok {
		t.Fatal("Get on a missing component must return false")
	}
}

func TestComponentsAddOnDeadEntityIsNoop(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	w.RemoveEntity(a) // immediate at depth 0
	pos.Add(a, position{X: 1})
	if pos.Has(a) {
		t.Fatal("Add on a dead entity must be a no-op")
	}
}

func TestComponentsRemoveImmediate(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	pos.Add(a, position{X: 1})
	pos.Remove(a)
	if pos.Has(a) {
		t.Fatal("Remove at depth 0 must take effect immediately")
	}
}

func TestComponentsAllIteratesAndMutatesThroughPointer(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	b := w.NewEntity()
	pos.Add(a, position{X: 1})
	pos.Add(b, position{X: 2})

	var seen []float64
	for id, p := range pos.All() {
		if !id.IsValid() {
			t.Fatal("iteration yielded an invalid id")
		}
		p.X += 10 // mutate through the interior pointer
		seen = append(seen, p.X)
	}
	sort.Float64s(seen)
	if len(seen) != 2 || seen[0] != 11 || seen[1] != 12 {
		t.Fatalf("iteration values = %v, want [11 12]", seen)
	}

	// Mutations through the pointer must have persisted.
	pa, _ := pos.Get(a)
	pb, _ := pos.Get(b)
	if pa.X != 11 || pb.X != 12 {
		t.Fatalf("mutations not persisted: a=%v b=%v", pa.X, pb.X)
	}
}

// The production All() iterator (not just the test mirror) must restore depth
// to 0 when the caller breaks out early.
func TestComponentsAllRestoresDepthAfterBreak(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	pos.Add(w.NewEntity(), position{X: 1})
	pos.Add(w.NewEntity(), position{X: 2})

	for range pos.All() {
		break
	}
	if d := w.depth.Load(); d != 0 {
		t.Fatalf("depth after break in All() = %d, want 0", d)
	}
}
