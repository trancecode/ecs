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

func TestRemoveComponentDuringIterationIsDeferred(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	b := w.NewEntity()
	pos.Add(a, position{X: 1})
	pos.Add(b, position{X: 2})

	for id := range pos.All() {
		if id == a {
			pos.Remove(a) // deferred during iteration
			if !pos.Has(a) {
				t.Fatal("Remove during iteration must be deferred")
			}
		}
	}
	if pos.Has(a) {
		t.Fatal("deferred Remove must take effect after the iteration flushes")
	}
	if !pos.Has(b) {
		t.Fatal("an untouched entity must be unaffected")
	}
}

func TestComponents2AddDuringIterationIsDeferred(t *testing.T) {
	w := NewWorld()
	moving := Components2[position, velocity](w)
	seed := w.NewEntity()
	moving.Add(seed, position{X: 1}, velocity{DX: 1})

	target := w.NewEntity()
	for range moving.All() {
		moving.Add(target, position{X: 5}, velocity{DX: 5}) // deferred bundle attach
		if moving.Has(target) {
			t.Fatal("bundle Add during iteration must be deferred")
		}
	}
	if !moving.Has(target) {
		t.Fatal("deferred bundle Add must be visible after the iteration ends")
	}
}

func TestRemoveEntityThenAddOrderingAtFlush(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	e := w.NewEntity()
	pos.Add(e, position{X: 1})

	// A separate, non-empty store to iterate so the two commands below defer.
	marker := Components[velocity](w)
	marker.Add(w.NewEntity(), velocity{})

	for range marker.All() {
		w.RemoveEntity(e)          // queued first: destroys e at flush
		pos.Add(e, position{X: 9}) // queued second: must no-op (e is dead at apply time)
	}

	if w.IsAlive(e) {
		t.Fatal("RemoveEntity queued before Add must win: entity destroyed at flush")
	}
	if pos.Has(e) {
		t.Fatal("Add after RemoveEntity on the same entity must no-op at apply time")
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
