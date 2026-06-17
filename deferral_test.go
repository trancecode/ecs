package ecs

import "testing"

func TestAddDuringIterationIsDeferredThenVisible(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	pos.Add(a, position{X: 1})

	newEnt := w.NewEntity() // id allocated immediately

	count := 0
	for range pos.All() {
		count++
		// Attach to newEnt while iterating; must be deferred.
		pos.Add(newEnt, position{X: 9})
		if pos.Has(newEnt) {
			t.Fatal("Add during iteration must not be visible mid-iteration")
		}
	}
	if count != 1 {
		t.Fatalf("iteration visited %d entities, want 1 (deferred add excluded)", count)
	}
	// After the loop unwinds to depth 0, the deferred add has been flushed.
	if !pos.Has(newEnt) {
		t.Fatal("deferred Add must be visible after the iteration ends")
	}
}

func TestRemoveEntityDuringIterationIsDeferred(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	pos.Add(a, position{X: 1})

	for range pos.All() {
		w.RemoveEntity(a)
		if !w.IsAlive(a) {
			t.Fatal("RemoveEntity during iteration must be deferred")
		}
	}
	if w.IsAlive(a) {
		t.Fatal("entity must be removed after the iteration flushes")
	}
	if pos.Has(a) {
		t.Fatal("removed entity's component must be gone after flush")
	}
}

func TestPointReadAfterIterationSeesDeferredChanges(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	pos.Add(a, position{X: 1})
	b := w.NewEntity()

	for range pos.All() {
		pos.Add(b, position{X: 5})
	}
	// Depth is back to 0, so a plain Get sees the materialized component.
	if p, ok := pos.Get(b); !ok || p.X != 5 {
		t.Fatalf("point read after iteration: ok=%v p=%v, want true {5}", ok, p)
	}
}
