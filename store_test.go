package ecs

import "testing"

type position struct{ X, Y float64 }

func TestComponentStoreAddGetHas(t *testing.T) {
	s := newComponentStore[position]()
	a := newEntityId(1)

	if s.has(a) {
		t.Fatal("empty store must not have the entity")
	}
	s.applyAdd(a, position{X: 1, Y: 2})
	if !s.has(a) {
		t.Fatal("store must have the entity after applyAdd")
	}
	p, ok := s.get(a)
	if !ok {
		t.Fatal("get must succeed after applyAdd")
	}
	if p.X != 1 || p.Y != 2 {
		t.Fatalf("get returned %+v, want {1 2}", *p)
	}
}

func TestComponentStoreGetReturnsInteriorPointer(t *testing.T) {
	s := newComponentStore[position]()
	a := newEntityId(1)
	s.applyAdd(a, position{X: 1})

	p, _ := s.get(a)
	p.X = 99 // mutate through the interior pointer

	p2, _ := s.get(a)
	if p2.X != 99 {
		t.Fatalf("mutation through pointer not persisted: got %v, want 99", p2.X)
	}
}

func TestComponentStoreApplyAddOverwrites(t *testing.T) {
	s := newComponentStore[position]()
	a := newEntityId(1)
	s.applyAdd(a, position{X: 1})
	s.applyAdd(a, position{X: 2}) // same entity overwrites
	p, _ := s.get(a)
	if p.X != 2 {
		t.Fatalf("second applyAdd did not overwrite: got %v, want 2", p.X)
	}
	if n := len(s.dense); n != 1 {
		t.Fatalf("overwrite must not grow dense slice: len = %d, want 1", n)
	}
}

func TestComponentStoreRemoveSwapPop(t *testing.T) {
	s := newComponentStore[position]()
	a, b, c := newEntityId(1), newEntityId(2), newEntityId(3)
	s.applyAdd(a, position{X: 1})
	s.applyAdd(b, position{X: 2})
	s.applyAdd(c, position{X: 3})

	s.applyRemove(b) // removes middle, last (c) swaps into its slot

	if s.has(b) {
		t.Fatal("removed entity must be absent")
	}
	if !s.has(a) || !s.has(c) {
		t.Fatal("untouched entities must remain")
	}
	pc, ok := s.get(c)
	if !ok || pc.X != 3 {
		t.Fatalf("swapped entity c corrupted: ok=%v val=%v", ok, pc)
	}
	if n := len(s.dense); n != 2 {
		t.Fatalf("dense slice len = %d, want 2", n)
	}
}

func TestComponentStoreRemoveAbsentIsNoop(t *testing.T) {
	s := newComponentStore[position]()
	s.applyRemove(newEntityId(1)) // must not panic
}

func TestComponentStoreRaw(t *testing.T) {
	s := newComponentStore[position]()
	a, b := newEntityId(1), newEntityId(2)
	s.applyAdd(a, position{X: 1})
	s.applyAdd(b, position{X: 2})

	ids, comps := s.raw()
	if len(ids) != 2 || len(comps) != 2 {
		t.Fatalf("raw lengths = %d,%d, want 2,2", len(ids), len(comps))
	}
	// Parallel arrays: ids[i] owns comps[i].
	for i := range ids {
		p, _ := s.get(ids[i])
		if p.X != comps[i].X {
			t.Fatalf("raw arrays not aligned at %d", i)
		}
	}
}
