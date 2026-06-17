package ecs

import "testing"

func TestNewEntityAllocatesDistinctValidIds(t *testing.T) {
	// Ids are opaque, so strict ordering cannot be asserted from outside the
	// package; distinctness and validity are the observable contract.
	w := NewWorld()
	a := w.NewEntity()
	b := w.NewEntity()
	if !a.IsValid() || !b.IsValid() {
		t.Fatal("allocated ids must be valid")
	}
	if a == b {
		t.Fatal("ids must be distinct")
	}
}

func TestRemoveEntityUnknownIdIsNoop(t *testing.T) {
	w := NewWorld()
	ghost := newEntityId(999) // never allocated
	w.RemoveEntity(ghost)     // must not panic
	if w.IsAlive(ghost) {
		t.Fatal("a never-allocated id must not be alive")
	}
}

func TestIsAliveAfterCreateAndRemove(t *testing.T) {
	w := NewWorld()
	a := w.NewEntity()
	if !w.IsAlive(a) {
		t.Fatal("entity must be alive after NewEntity")
	}
	w.RemoveEntity(a)
	if w.IsAlive(a) {
		t.Fatal("entity must be dead after RemoveEntity at depth 0")
	}
}

func TestRemoveEntityClearsComponentsFromStores(t *testing.T) {
	w := NewWorld()
	a := w.NewEntity()
	store := storeOf[position](w)
	store.applyAdd(a, position{X: 1})

	w.RemoveEntity(a)

	if store.has(a) {
		t.Fatal("RemoveEntity must clear the entity from every store")
	}
}

func TestStoreOfReturnsSameStorePerType(t *testing.T) {
	w := NewWorld()
	s1 := storeOf[position](w)
	s2 := storeOf[position](w)
	if s1 != s2 {
		t.Fatal("storeOf must return the same store instance for a type")
	}
}
