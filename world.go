package ecs

import (
	"reflect"
	"sync"
	"sync/atomic"
)

// World owns the entities and all component stores.
type World struct {
	nextID atomic.Uint64

	alive  map[EntityId]struct{}
	stores map[reflect.Type]componentStorage

	depth atomic.Int64

	mu       sync.Mutex
	commands []func()
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
