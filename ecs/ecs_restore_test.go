package ecs

import "testing"

func TestEntityCounterCountsAllocations(t *testing.T) {
	w := NewWorld()
	if got := w.EntityCounter(); got != 0 {
		t.Fatalf("fresh world counter = %d, want 0", got)
	}
	w.NewEntity()
	b := w.NewEntity()
	w.RemoveEntity(b)
	if got := w.EntityCounter(); got != 2 {
		t.Fatalf("counter after two allocations = %d, want 2 (removal must not shrink it)", got)
	}
}

func TestRestoreEntityRoundTrip(t *testing.T) {
	// Save: three entities, the middle one removed.
	saved := NewWorld()
	a := saved.NewEntity()
	b := saved.NewEntity()
	c := saved.NewEntity()
	saved.RemoveEntity(b)

	// Load into a fresh world.
	loaded := NewWorld()
	loaded.RestoreEntityCounter(saved.EntityCounter())
	for _, id := range []EntityId{a, c} {
		if err := loaded.RestoreEntity(id); err != nil {
			t.Fatalf("RestoreEntity: %v", err)
		}
	}

	if !loaded.IsAlive(a) || !loaded.IsAlive(c) {
		t.Fatal("restored entities must be alive")
	}
	if loaded.IsAlive(b) {
		t.Fatal("an id absent from the save must not be alive")
	}

	// Allocation must resume exactly where the saved run left off: the next id
	// in both worlds must be identical. Ids participate in deterministic
	// ordering downstream, so divergence after a load breaks replay.
	if next, orig := loaded.NewEntity(), saved.NewEntity(); next != orig {
		t.Fatal("NewEntity after restore must continue the saved sequence")
	}
}

func TestRestoreEntityAcceptsComponents(t *testing.T) {
	saved := NewWorld()
	id := saved.NewEntity()

	loaded := NewWorld()
	loaded.RestoreEntityCounter(saved.EntityCounter())
	if err := loaded.RestoreEntity(id); err != nil {
		t.Fatalf("RestoreEntity: %v", err)
	}

	positions := Components[position](loaded)
	positions.Add(id, position{X: 7})
	p, ok := positions.Get(id)
	if !ok || p.X != 7 {
		t.Fatal("components must attach to a restored entity through Accessor.Add")
	}
}

func TestRestoreEntityRejectsInvalidId(t *testing.T) {
	w := NewWorld()
	if err := w.RestoreEntity(EntityId{}); err == nil {
		t.Fatal("restoring the zero id must fail")
	}
}

func TestRestoreEntityRejectsAliveId(t *testing.T) {
	w := NewWorld()
	id := w.NewEntity()
	if err := w.RestoreEntity(id); err == nil {
		t.Fatal("restoring an already-alive id must fail")
	}
}

func TestRestoreEntityRejectsIdBeyondCounter(t *testing.T) {
	saved := NewWorld()
	id := saved.NewEntity()

	loaded := NewWorld() // counter still 0: any allocated id is beyond it
	if err := loaded.RestoreEntity(id); err == nil {
		t.Fatal("restoring an id beyond the counter must fail (restore the counter first)")
	}
}

func TestRestoreEntityRejectsDuringIteration(t *testing.T) {
	saved := NewWorld()
	a := saved.NewEntity()
	b := saved.NewEntity()

	loaded := NewWorld()
	loaded.RestoreEntityCounter(saved.EntityCounter())
	if err := loaded.RestoreEntity(a); err != nil {
		t.Fatalf("RestoreEntity: %v", err)
	}
	positions := Components[position](loaded)
	positions.Add(a, position{})

	for range positions.All() {
		if err := loaded.RestoreEntity(b); err == nil {
			t.Fatal("restore during iteration must fail; restore is a load-time operation")
		}
	}
}
