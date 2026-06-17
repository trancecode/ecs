package ecs

// componentStorage is the untyped view of a component store, so the World can
// remove an entity from every store without knowing the component type.
type componentStorage interface {
	removeEntity(id EntityId)
}

// componentStore is a sparse set for one component type C: a dense slice of
// values plus an identifier-to-slot map. Removal is swap-and-pop. Iteration
// over dense is cache-friendly.
type componentStore[C any] struct {
	dense []C
	ids   []EntityId
	index map[EntityId]int
}

func newComponentStore[C any]() *componentStore[C] {
	return &componentStore[C]{index: make(map[EntityId]int)}
}

func (s *componentStore[C]) has(id EntityId) bool {
	_, ok := s.index[id]
	return ok
}

// get returns an interior pointer into dense storage. It is valid until the
// next structural change to this store.
func (s *componentStore[C]) get(id EntityId) (*C, bool) {
	i, ok := s.index[id]
	if !ok {
		return nil, false
	}
	return &s.dense[i], true
}

// applyAdd inserts or overwrites the component for id. This is the immediate,
// low-level mutation; deferral is decided by the caller.
func (s *componentStore[C]) applyAdd(id EntityId, c C) {
	if i, ok := s.index[id]; ok {
		s.dense[i] = c
		return
	}
	s.index[id] = len(s.dense)
	s.dense = append(s.dense, c)
	s.ids = append(s.ids, id)
}

// applyRemove deletes the component for id with swap-and-pop. Absent id is a
// no-op.
func (s *componentStore[C]) applyRemove(id EntityId) {
	i, ok := s.index[id]
	if !ok {
		return
	}
	last := len(s.dense) - 1
	if i != last {
		movedID := s.ids[last]
		s.dense[i] = s.dense[last]
		s.ids[i] = movedID
		s.index[movedID] = i
	}
	s.dense = s.dense[:last]
	s.ids = s.ids[:last]
	delete(s.index, id)
}

func (s *componentStore[C]) removeEntity(id EntityId) {
	s.applyRemove(id)
}

// raw exposes the parallel id and value slices for iteration. ids[i] owns
// comps[i].
func (s *componentStore[C]) raw() ([]EntityId, []C) {
	return s.ids, s.dense
}
