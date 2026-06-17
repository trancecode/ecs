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
