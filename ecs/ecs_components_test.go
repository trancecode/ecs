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

func TestComponentsGetOrAddInsertsWhenMissing(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()

	p := pos.GetOrAdd(a, position{X: 1, Y: 2})
	if p == nil {
		t.Fatal("GetOrAdd must never return nil")
	}
	if p.X != 1 || p.Y != 2 {
		t.Fatalf("GetOrAdd returned %+v, want {1 2}", p)
	}
	if !pos.Has(a) {
		t.Fatal("GetOrAdd must attach the component when missing")
	}
	// The returned pointer must be the interior pointer into storage.
	p.X = 99
	got, _ := pos.Get(a)
	if got.X != 99 {
		t.Fatalf("write through GetOrAdd pointer did not persist: got %v, want 99", got.X)
	}
}

func TestComponentsGetOrAddReturnsExistingWithoutOverwriting(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	pos.Add(a, position{X: 1, Y: 2})

	p := pos.GetOrAdd(a, position{X: 7, Y: 8})
	if p.X != 1 || p.Y != 2 {
		t.Fatalf("GetOrAdd on an existing component returned %+v, want the existing {1 2}", p)
	}
	got, _ := pos.Get(a)
	if got.X != 1 || got.Y != 2 {
		t.Fatalf("GetOrAdd must not overwrite an existing component: got %+v", got)
	}
}

func TestComponentsGetOrAddFuncInsertsWhenMissing(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()

	p := pos.GetOrAddFunc(a, func() position { return position{X: 3, Y: 4} })
	if p == nil || p.X != 3 || p.Y != 4 {
		t.Fatalf("GetOrAddFunc returned %+v, want {3 4}", p)
	}
	if !pos.Has(a) {
		t.Fatal("GetOrAddFunc must attach the component when missing")
	}
}

func TestComponentsGetOrAddFuncDoesNotCallMakeOnHit(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	pos.Add(a, position{X: 1})

	called := false
	p := pos.GetOrAddFunc(a, func() position {
		called = true
		return position{X: 9}
	})
	if called {
		t.Fatal("GetOrAddFunc must not call make when the component already exists")
	}
	if p.X != 1 {
		t.Fatalf("GetOrAddFunc returned %+v, want existing {1 0}", p)
	}
}

func TestComponentsGetOrAddOnDeadEntityDoesNotStore(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	w.RemoveEntity(a) // immediate at depth 0

	p := pos.GetOrAdd(a, position{X: 1})
	if p == nil {
		t.Fatal("GetOrAdd must never return nil, even for a dead entity")
	}
	if pos.Has(a) {
		t.Fatal("GetOrAdd on a dead entity must not store a component")
	}
}

func TestComponentsGetOrAddDuringIterationIsDeferred(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	pos.Add(a, position{X: 1})

	target := w.NewEntity()
	var staged *position
	for range pos.All() {
		staged = pos.GetOrAdd(target, position{X: 5})
		if pos.Has(target) {
			t.Fatal("GetOrAdd during iteration must be deferred, not visible mid-loop")
		}
		// The returned pointer must be usable for the rest of the loop.
		staged.X = 42
	}
	// After the loop flushes, the deferred insert carries the mutation made
	// through the staged pointer.
	got, ok := pos.Get(target)
	if !ok {
		t.Fatal("deferred GetOrAdd must materialize after the iteration ends")
	}
	if got.X != 42 {
		t.Fatalf("mutation through the staged pointer was lost: got %v, want 42", got.X)
	}
}

func TestComponentsGetOrAddDuringIterationHitReturnsLivePointer(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	pos.Add(a, position{X: 1})

	for id := range pos.All() {
		if id != a {
			continue
		}
		p := pos.GetOrAdd(a, position{X: 99}) // hit path: must not overwrite
		p.X += 10
	}
	got, _ := pos.Get(a)
	if got.X != 11 {
		t.Fatalf("hit-path GetOrAdd during iteration: got %v, want 11", got.X)
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
