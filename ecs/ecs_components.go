package ecs

import "iter"

// Accessor is a reusable, typed handle for one component type. Construct it once
// (for example as a field of a system) and reuse it; it caches the store.
type Accessor[A any] struct {
	w     *World
	store *componentStore[A]
}

// Components returns the handle for component type A in world w.
func Components[A any](w *World) Accessor[A] {
	return Accessor[A]{w: w, store: storeOf[A](w)}
}

// Get returns an interior pointer to the component for id, valid until the next
// structural change to this component's store. The bool is false if the entity
// does not have the component.
func (h Accessor[A]) Get(id EntityId) (*A, bool) {
	return h.store.get(id)
}

// Has reports whether the entity has this component.
func (h Accessor[A]) Has(id EntityId) bool {
	return h.store.has(id)
}

// Add attaches the component to the entity. At depth 0 it applies immediately
// (ignored if the entity is dead); during an iteration it is deferred and the
// liveness check happens at apply time.
func (h Accessor[A]) Add(id EntityId, a A) {
	if h.w.depth.Load() == 0 {
		if h.w.IsAlive(id) {
			h.store.applyAdd(id, a)
		}
		return
	}
	store := h.store
	w := h.w
	w.enqueue(func() {
		if w.IsAlive(id) {
			store.applyAdd(id, a)
		}
	})
}

// GetOrAdd returns the interior pointer to the entity's component, adding value
// first if the component is absent. The result is never nil and, like Get and
// iteration, is valid until the next structural change to this store. The add
// honours the same deferral rules as Add: immediate at depth 0, deferred during
// an iteration (where the returned pointer is to a staged value that the flush
// inserts, so writes through it survive into the store).
func (h Accessor[A]) GetOrAdd(id EntityId, value A) *A {
	if p, ok := h.store.get(id); ok {
		return p
	}
	return h.insertMissing(id, value)
}

// GetOrAddFunc returns the interior pointer to the entity's component, adding a
// value built by make first if the component is absent. make is called only when
// the component does not already exist, so nothing is constructed on the hit
// path. The result follows the same validity and deferral rules as GetOrAdd.
func (h Accessor[A]) GetOrAddFunc(id EntityId, make func() A) *A {
	if p, ok := h.store.get(id); ok {
		return p
	}
	return h.insertMissing(id, make())
}

// insertMissing attaches value for an entity already known to lack the component
// and returns a non-nil pointer to it. At depth 0 the add is immediate (skipped
// for a dead entity, in which case the returned pointer is to the unstored
// value); during an iteration it is deferred and the returned pointer is to the
// staged value the flush inserts.
func (h Accessor[A]) insertMissing(id EntityId, value A) *A {
	if h.w.depth.Load() == 0 {
		if !h.w.IsAlive(id) {
			return &value
		}
		return h.store.applyAdd(id, value)
	}
	store := h.store
	w := h.w
	w.enqueue(func() {
		if w.IsAlive(id) {
			store.applyAdd(id, value)
		}
	})
	return &value
}

// Remove detaches the component from the entity. Immediate at depth 0, deferred
// during an iteration.
func (h Accessor[A]) Remove(id EntityId) {
	if h.w.depth.Load() == 0 {
		h.store.applyRemove(id)
		return
	}
	store := h.store
	h.w.enqueue(func() { store.applyRemove(id) })
}

// All iterates every entity that has this component, yielding the identifier and
// an interior pointer. The pointer is valid only within the loop; do not retain
// it across iterations or past a structural change to this store. It is
// range-over-func, so the depth counter is incremented on entry and decremented
// on exit (via defer) for any exit path, including break and panic.
func (h Accessor[A]) All() iter.Seq2[EntityId, *A] {
	return func(yield func(EntityId, *A) bool) {
		h.w.beginIteration()
		defer h.w.endIteration()
		ids, comps := h.store.raw()
		for i := range comps {
			if !yield(ids[i], &comps[i]) {
				return
			}
		}
	}
}
