package ecs

import (
	"reflect"
	"sync"
	"sync/atomic"
)

// World owns the entities and all component stores.
type World struct {
	nextID atomic.Uint64

	// alive is the liveness set. In v1 (sequential use) it is unsynchronized; it
	// is the remaining registry state to put behind mu when parallel iteration is
	// introduced (depth and commands are already concurrency-ready).
	alive  map[EntityId]struct{}
	stores map[reflect.Type]componentStorage

	depth atomic.Int64

	mu       sync.Mutex
	commands []func()

	// Observability counters, maintained only on cold paths (enqueue and flush)
	// so the read and iterate paths stay untouched. Atomic so Stats can read them
	// without locking.
	flushes     atomic.Uint64
	deferredOps atomic.Uint64
}

// NewWorld returns an empty world.
func NewWorld() *World {
	return &World{
		alive:  make(map[EntityId]struct{}),
		stores: make(map[reflect.Type]componentStorage),
	}
}

// NewEntity allocates a fresh entity and returns its identifier. Allocation is
// immediate even during iteration, because the caller needs a usable identifier
// right away; component attachment is what gets deferred.
func (w *World) NewEntity() EntityId {
	id := newEntityId(w.nextID.Add(1)) // first id is 1; 0 stays invalid
	w.alive[id] = struct{}{}
	return id
}

// IsAlive reports whether the identifier refers to a live entity.
func (w *World) IsAlive(id EntityId) bool {
	_, ok := w.alive[id]
	return ok
}

// RemoveEntity destroys an entity. At depth 0 it applies immediately; during an
// iteration it is deferred to the flush.
func (w *World) RemoveEntity(id EntityId) {
	if w.depth.Load() == 0 {
		w.applyRemoveEntity(id)
		return
	}
	w.enqueue(func() { w.applyRemoveEntity(id) })
}

func (w *World) applyRemoveEntity(id EntityId) {
	if _, ok := w.alive[id]; !ok {
		return
	}
	for _, s := range w.stores {
		s.removeEntity(id)
	}
	delete(w.alive, id)
}

// enqueue records a deferred structural change.
func (w *World) enqueue(cmd func()) {
	w.mu.Lock()
	w.commands = append(w.commands, cmd)
	w.mu.Unlock()
	w.deferredOps.Add(1)
}

// storeOf returns the store for component type C, creating it on first use.
func storeOf[C any](w *World) *componentStore[C] {
	t := reflect.TypeFor[C]()
	if s, ok := w.stores[t]; ok {
		return s.(*componentStore[C])
	}
	s := newComponentStore[C]()
	w.stores[t] = s
	return s
}

// beginIteration marks the start of an iteration. Structural changes issued
// while depth is above 0 are deferred. Callers must pair this with a deferred
// call to endIteration so the depth is restored on every exit path.
func (w *World) beginIteration() {
	w.depth.Add(1)
}

// endIteration marks the end of an iteration. When the outermost iteration ends
// (depth returns to 0) the deferred command buffer is flushed. It is registered
// with defer by iterators, so it runs on normal completion, break, and panic.
func (w *World) endIteration() {
	if w.depth.Add(-1) == 0 {
		w.Flush()
	}
}

// Flush applies all deferred structural changes in the order they were queued.
// It is invoked automatically when an iteration unwinds to depth 0, and is also
// available as an explicit escape hatch for code that must materialize pending
// changes before a non-iterating step (point reads, serialization).
//
// Commands enqueued by a running command are not drained in the same call; an
// explicit Flush (or the next iteration) runs them. This does not arise in
// version 1, where deferred commands apply their changes directly rather than
// enqueueing more work.
func (w *World) Flush() {
	w.mu.Lock()
	cmds := w.commands
	w.commands = nil
	w.mu.Unlock()
	if len(cmds) == 0 {
		return
	}
	w.flushes.Add(1)
	for _, cmd := range cmds {
		cmd()
	}
}
