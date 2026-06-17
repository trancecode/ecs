package ecs

import "iter"

// Tuple2 is the iteration element for a two-component join. The pointers are
// unexported; unpack them with Values.
type Tuple2[A, B any] struct {
	a *A
	b *B
}

// Values returns the component pointers in handle order.
func (t Tuple2[A, B]) Values() (*A, *B) {
	return t.a, t.b
}

// Accessor2 is a reusable, typed handle for entities that have both component
// types A and B. Construct once and reuse.
type Accessor2[A, B any] struct {
	w      *World
	storeA *componentStore[A]
	storeB *componentStore[B]
}

// Components2 returns the two-component handle for A and B in world w.
func Components2[A, B any](w *World) Accessor2[A, B] {
	return Accessor2[A, B]{
		w:      w,
		storeA: storeOf[A](w),
		storeB: storeOf[B](w),
	}
}

// Get returns interior pointers to both components. The bool is true only when
// the entity has both.
func (h Accessor2[A, B]) Get(id EntityId) (*A, *B, bool) {
	a, ok := h.storeA.get(id)
	if !ok {
		return nil, nil, false
	}
	b, ok := h.storeB.get(id)
	if !ok {
		return nil, nil, false
	}
	return a, b, true
}

// Has reports whether the entity has both components.
func (h Accessor2[A, B]) Has(id EntityId) bool {
	return h.storeA.has(id) && h.storeB.has(id)
}

// Add attaches both components to the entity as a bundle. Immediate at depth 0
// (ignored if the entity is dead), deferred during an iteration with the
// liveness check at apply time.
func (h Accessor2[A, B]) Add(id EntityId, a A, b B) {
	if h.w.depth.Load() == 0 {
		if h.w.IsAlive(id) {
			h.storeA.applyAdd(id, a)
			h.storeB.applyAdd(id, b)
		}
		return
	}
	storeA, storeB, w := h.storeA, h.storeB, h.w
	w.enqueue(func() {
		if w.IsAlive(id) {
			storeA.applyAdd(id, a)
			storeB.applyAdd(id, b)
		}
	})
}

// All iterates every entity that has both components, yielding the identifier
// and a Tuple2 of interior pointers. It iterates the A store and looks up B.
func (h Accessor2[A, B]) All() iter.Seq2[EntityId, Tuple2[A, B]] {
	return func(yield func(EntityId, Tuple2[A, B]) bool) {
		h.w.beginIteration()
		defer h.w.endIteration()
		ids, as := h.storeA.raw()
		for i := range as {
			id := ids[i]
			b, ok := h.storeB.get(id)
			if !ok {
				continue
			}
			if !yield(id, Tuple2[A, B]{a: &as[i], b: b}) {
				return
			}
		}
	}
}
