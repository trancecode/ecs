package ecs

import "fmt"

// EntityCounter returns the entity-allocation counter: the number of
// identifiers ever allocated by NewEntity. It only grows; removing an entity
// does not shrink it. Savegames persist it so RestoreEntityCounter can reseat
// allocation on load. The exact value matters for determinism: identifiers
// participate in ordering downstream (for example as an event-queue
// tie-breaker), so a loaded run must allocate the same ids the saved run would
// have.
func (w *World) EntityCounter() uint64 {
	return w.nextID.Load()
}

// RestoreEntityCounter reseats the allocation counter from a savegame so that
// entities allocated after a load continue the saved run's id sequence instead
// of colliding with restored identifiers. Call it on a fresh world, before any
// RestoreEntity call.
func (w *World) RestoreEntityCounter(n uint64) {
	w.nextID.Store(n)
}

// RestoreEntity marks a saved identifier alive again, preserving it exactly
// (NewEntity would allocate a fresh one instead). Components are then attached
// through the usual Accessor.Add. Restore is a load-time operation on a fresh
// world: call it after RestoreEntityCounter, once per saved identifier, and
// never during an iteration. It returns an error for input a well-formed save
// cannot produce: the zero id, an id beyond the allocation counter (counter
// not restored first), or an id that is already alive (duplicate).
func (w *World) RestoreEntity(id EntityId) error {
	if !id.IsValid() {
		return fmt.Errorf("ecs.World.RestoreEntity: invalid (zero) id")
	}
	if w.depth.Load() != 0 {
		return fmt.Errorf("ecs.World.RestoreEntity: called during an iteration; restore is a load-time operation")
	}
	if id.v > w.nextID.Load() {
		return fmt.Errorf("ecs.World.RestoreEntity: %v is beyond the allocation counter %d; call RestoreEntityCounter first", id, w.nextID.Load())
	}
	if _, ok := w.alive[id]; ok {
		return fmt.Errorf("ecs.World.RestoreEntity: %v is already alive", id)
	}
	w.alive[id] = struct{}{}
	return nil
}
