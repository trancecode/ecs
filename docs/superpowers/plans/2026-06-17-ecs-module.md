# ECS module implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the version 1 `github.com/trancecode/ecs` module: entities, plain-data components in per-type sparse sets, interior-pointer access, and deferred structural changes with auto-flush on iteration.

**Architecture:** A `World` owns a registry of per-component-type sparse-set stores plus a deferred command buffer guarded by an atomic iteration-depth counter. Reads and value mutations go through interior pointers returned by typed handles (`Components[A]`, `Components2[A, B]`). Structural changes (add, remove, create, destroy) apply immediately when no iteration is active and are deferred to a flush when one is, so component pointers stay valid for the duration of any iteration.

**Tech Stack:** Go 1.26, standard library only (`sync/atomic`, `reflect`, `iter`, `testing`). Module path `github.com/trancecode/ecs`, package `ecs`.

**Source of truth:** `docs/superpowers/specs/2026-06-17-ecs-framework-design.md`. This plan implements that spec's version 1 scope.

**Out of scope (do not build):** the nrg migration (separate session), `Components3` and higher arities, archetype storage, events, resources, change detection, read-only handles, a systems scheduler, generational identifiers or identifier recycling, and serialization. These are recorded as future seams in the spec.

---

## File structure

* `go.mod` — module definition.
* `doc.go` — package documentation.
* `entity.go` — `EntityId` opaque value type with `String` and `IsValid`.
* `store.go` — internal generic sparse-set `componentStore[C]` and the untyped `componentStorage` interface. No public surface.
* `world.go` — `World`: entity registry (monotonic allocation, liveness), store registry, atomic depth counter, command buffer, `NewWorld`, `NewEntity`, `RemoveEntity`, `IsAlive`, `Flush`, and the internal `beginIteration`/`endIteration`/`enqueue` helpers.
* `components.go` — `storeOf`, the single-component handle `Components`/`Accessor[A]` with `Get`/`Has`/`Add`/`Remove`/`All`.
* `components2.go` — the two-component handle `Components2`/`Accessor2[A, B]`, `Tuple2[A, B]`, with `Get`/`Has`/`Add`/`All`.
* `*_test.go` — white-box tests in `package ecs` alongside each file.

Files are split by responsibility and kept small so each can be held in context whole.

---

## Task 1: Module scaffold

**Files:**
- Create: `go.mod`
- Create: `doc.go`
- Test: `doc_test.go`

- [ ] **Step 1: Write the failing test**

`doc_test.go`:

```go
package ecs

import "testing"

func TestPackageCompiles(t *testing.T) {
	// Placeholder so the package has a test target from the start.
	// Replaced by real tests in later tasks.
	t.Parallel()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./...`
Expected: FAIL, `go.mod` is missing so the build errors with "go.mod file not found".

- [ ] **Step 3: Write minimal implementation**

`go.mod`:

```
module github.com/trancecode/ecs

go 1.26
```

`doc.go`:

```go
// Package ecs is an Entity Component System for Go.
//
// Entities are opaque identifiers. Components are plain Go structs stored in
// per-type sparse sets. Typed handles (Components, Components2) read and mutate
// components through interior pointers. Structural changes (adding or removing
// components, creating or destroying entities) apply immediately when no
// iteration is in progress and are deferred to a flush when one is, so pointers
// obtained during iteration stay valid for that iteration.
package ecs
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./...`
Expected: PASS (`ok  github.com/trancecode/ecs`).

- [ ] **Step 5: Commit**

```bash
git add go.mod doc.go doc_test.go
git commit -m "Scaffold ecs module"
```

---

## Task 2: EntityId

**Files:**
- Create: `entity.go`
- Test: `entity_test.go`

- [ ] **Step 1: Write the failing test**

`entity_test.go`:

```go
package ecs

import "testing"

func TestEntityIdZeroValueIsInvalid(t *testing.T) {
	var zero EntityId
	if zero.IsValid() {
		t.Fatal("zero EntityId must be invalid")
	}
	if got := zero.String(); got != "ent_invalid" {
		t.Fatalf("zero String() = %q, want %q", got, "ent_invalid")
	}
}

func TestEntityIdStringAndValidity(t *testing.T) {
	id := newEntityId(123)
	if !id.IsValid() {
		t.Fatal("non-zero EntityId must be valid")
	}
	if got := id.String(); got != "ent_123" {
		t.Fatalf("String() = %q, want %q", got, "ent_123")
	}
}

func TestEntityIdIsComparableMapKey(t *testing.T) {
	m := map[EntityId]int{}
	m[newEntityId(1)] = 10
	m[newEntityId(2)] = 20
	if m[newEntityId(1)] != 10 || m[newEntityId(2)] != 20 {
		t.Fatal("EntityId must work as a map key")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestEntityId -v`
Expected: FAIL, "undefined: EntityId" / "undefined: newEntityId".

- [ ] **Step 3: Write minimal implementation**

`entity.go`:

```go
package ecs

import "strconv"

// EntityId is an opaque, comparable identifier for an entity.
//
// The zero value is invalid. Identifiers are allocated by World.NewEntity and
// are never reused in this version. The unexported field makes the type opaque:
// callers cannot fabricate a valid identifier, do arithmetic, or convert to or
// from an integer, which keeps the representation free to change later.
type EntityId struct {
	v uint64
}

// newEntityId builds an EntityId from a raw counter value. Internal only.
func newEntityId(v uint64) EntityId {
	return EntityId{v: v}
}

// IsValid reports whether the identifier is a real, allocated entity (not the
// zero value).
func (e EntityId) IsValid() bool {
	return e.v != 0
}

// String renders a prefixed, greppable form for logs: "ent_123" for a real
// identifier, "ent_invalid" for the zero value. It is for humans only; there is
// no parse-back.
func (e EntityId) String() string {
	if e.v == 0 {
		return "ent_invalid"
	}
	return "ent_" + strconv.FormatUint(e.v, 10)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestEntityId -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add entity.go entity_test.go
git commit -m "Add opaque EntityId with String and IsValid"
```

---

## Task 3: Internal sparse-set component store

**Files:**
- Create: `store.go`
- Test: `store_test.go`

This is a pure data structure with no `World` dependency. Tests are white-box (same package) because the store is unexported.

- [ ] **Step 1: Write the failing test**

`store_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestComponentStore -v`
Expected: FAIL, "undefined: newComponentStore".

- [ ] **Step 3: Write minimal implementation**

`store.go`:

```go
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
	movedID := s.ids[last]
	s.dense[i] = s.dense[last]
	s.ids[i] = movedID
	s.index[movedID] = i
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestComponentStore -v`
Expected: PASS (all store tests).

- [ ] **Step 5: Commit**

```bash
git add store.go store_test.go
git commit -m "Add internal sparse-set component store"
```

---

## Task 4: World core and entity lifecycle

**Files:**
- Create: `world.go`
- Test: `world_test.go`

Implements `World` with the entity registry, the store registry, `storeOf`, and immediate (depth 0) entity lifecycle. The depth counter and command buffer fields are declared here; their deferral behavior is exercised in Task 5.

- [ ] **Step 1: Write the failing test**

`world_test.go`:

```go
package ecs

import "testing"

func TestNewEntityAllocatesValidIncreasingIds(t *testing.T) {
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestNewEntity|TestIsAlive|TestRemoveEntity|TestStoreOf' -v`
Expected: FAIL, "undefined: NewWorld" / "undefined: storeOf".

- [ ] **Step 3: Write minimal implementation**

`world.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestNewEntity|TestIsAlive|TestRemoveEntity|TestStoreOf' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add world.go world_test.go
git commit -m "Add World with entity registry and store registry"
```

---

## Task 5: Deferred command buffer and depth-tracked flush

**Files:**
- Modify: `world.go` (add `beginIteration`, `endIteration`, `Flush`)
- Test: `flush_test.go`

Implements the auto-flush mechanics in isolation, tested with sentinel closures so the behavior is verified without depending on the component handles built next.

- [ ] **Step 1: Write the failing test**

`flush_test.go`:

```go
package ecs

import "testing"

// runScope simulates one iterator's lifetime: begin, run body, end.
func runScope(w *World, body func()) {
	w.beginIteration()
	defer w.endIteration()
	body()
}

func TestDeferredCommandRunsOnUnwindToZero(t *testing.T) {
	w := NewWorld()
	ran := false
	runScope(w, func() {
		w.enqueue(func() { ran = true })
		if ran {
			t.Fatal("command must not run during the iteration")
		}
	})
	if !ran {
		t.Fatal("command must run when depth unwinds to 0")
	}
}

func TestNestedIterationDoesNotFlushEarly(t *testing.T) {
	w := NewWorld()
	ran := false
	runScope(w, func() { // depth 0->1
		w.enqueue(func() { ran = true })
		runScope(w, func() { // depth 1->2->1, no flush
			if ran {
				t.Fatal("nested iteration must not flush")
			}
		})
		if ran {
			t.Fatal("command must not run while outer iteration is active")
		}
	}) // depth 1->0, flush
	if !ran {
		t.Fatal("command must run after the outermost iteration ends")
	}
}

func TestCommandsRunInFifoOrder(t *testing.T) {
	w := NewWorld()
	var order []int
	runScope(w, func() {
		w.enqueue(func() { order = append(order, 1) })
		w.enqueue(func() { order = append(order, 2) })
		w.enqueue(func() { order = append(order, 3) })
	})
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("commands ran out of order: %v", order)
	}
}

func TestExplicitFlushAtDepthZero(t *testing.T) {
	w := NewWorld()
	ran := false
	w.enqueue(func() { ran = true }) // queued outside any iteration
	if ran {
		t.Fatal("enqueue must not run on its own")
	}
	w.Flush()
	if !ran {
		t.Fatal("explicit Flush must run queued commands")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestDeferred|TestNested|TestCommandsRun|TestExplicitFlush' -v`
Expected: FAIL, "w.beginIteration undefined" / "w.Flush undefined".

- [ ] **Step 3: Write minimal implementation**

Append to `world.go`:

```go
// beginIteration marks the start of an iteration. Structural changes issued
// while depth is above 0 are deferred.
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
func (w *World) Flush() {
	w.mu.Lock()
	cmds := w.commands
	w.commands = nil
	w.mu.Unlock()
	for _, cmd := range cmds {
		cmd()
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestDeferred|TestNested|TestCommandsRun|TestExplicitFlush' -v`
Expected: PASS (all four).

- [ ] **Step 5: Commit**

```bash
git add world.go flush_test.go
git commit -m "Add deferred command buffer with depth-tracked auto-flush"
```

---

## Task 6: Single-component handle, reads and immediate writes

**Files:**
- Create: `components.go`
- Test: `components_test.go`

Implements `Components[A]` / `Accessor[A]` with `Get`, `Has`, `Add`, `Remove`, and `All` iteration. This task covers the depth-0 (immediate) behavior and iteration; Task 7 covers deferral during iteration.

- [ ] **Step 1: Write the failing test**

`components_test.go`:

```go
package ecs

import (
	"sort"
	"testing"
)

func TestComponentsAddGetHas(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()

	if pos.Has(a) {
		t.Fatal("entity must not have the component yet")
	}
	pos.Add(a, position{X: 1, Y: 2})
	if !pos.Has(a) {
		t.Fatal("entity must have the component after Add")
	}
	p, ok := pos.Get(a)
	if !ok || p.X != 1 || p.Y != 2 {
		t.Fatalf("Get returned ok=%v %+v, want true {1 2}", ok, p)
	}
}

func TestComponentsGetMissingReturnsFalse(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	_, ok := pos.Get(w.NewEntity())
	if ok {
		t.Fatal("Get on a missing component must return false")
	}
}

func TestComponentsAddOnDeadEntityIsNoop(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	w.RemoveEntity(a) // immediate at depth 0
	pos.Add(a, position{X: 1})
	if pos.Has(a) {
		t.Fatal("Add on a dead entity must be a no-op")
	}
}

func TestComponentsRemoveImmediate(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	pos.Add(a, position{X: 1})
	pos.Remove(a)
	if pos.Has(a) {
		t.Fatal("Remove at depth 0 must take effect immediately")
	}
}

func TestComponentsAllIteratesAndMutatesThroughPointer(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	b := w.NewEntity()
	pos.Add(a, position{X: 1})
	pos.Add(b, position{X: 2})

	var seen []float64
	for id, p := range pos.All() {
		if !id.IsValid() {
			t.Fatal("iteration yielded an invalid id")
		}
		p.X += 10 // mutate through the interior pointer
		seen = append(seen, p.X)
	}
	sort.Float64s(seen)
	if len(seen) != 2 || seen[0] != 11 || seen[1] != 12 {
		t.Fatalf("iteration values = %v, want [11 12]", seen)
	}

	// Mutations through the pointer must have persisted.
	pa, _ := pos.Get(a)
	pb, _ := pos.Get(b)
	if pa.X != 11 || pb.X != 12 {
		t.Fatalf("mutations not persisted: a=%v b=%v", pa.X, pb.X)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestComponents -v`
Expected: FAIL, "undefined: Components".

- [ ] **Step 3: Write minimal implementation**

`components.go`:

```go
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
// an interior pointer. It is range-over-func, so the depth counter is
// incremented on entry and decremented on exit (via defer) for any exit path,
// including break and panic.
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestComponents -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add components.go components_test.go
git commit -m "Add single-component handle with iteration"
```

---

## Task 7: Deferred structural changes during iteration

**Files:**
- Test: `deferral_test.go` (no production changes; verifies the integrated behavior of Tasks 5 and 6)

If any test here fails, the bug is in the depth checks in `components.go` (Task 6) or the flush in `world.go` (Task 5); fix there.

- [ ] **Step 1: Write the failing test**

`deferral_test.go`:

```go
package ecs

import "testing"

func TestAddDuringIterationIsDeferredThenVisible(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	pos.Add(a, position{X: 1})

	newEnt := w.NewEntity() // id allocated immediately

	count := 0
	for range pos.All() {
		count++
		// Attach to newEnt while iterating; must be deferred.
		pos.Add(newEnt, position{X: 9})
		if pos.Has(newEnt) {
			t.Fatal("Add during iteration must not be visible mid-iteration")
		}
	}
	if count != 1 {
		t.Fatalf("iteration visited %d entities, want 1 (deferred add excluded)", count)
	}
	// After the loop unwinds to depth 0, the deferred add has been flushed.
	if !pos.Has(newEnt) {
		t.Fatal("deferred Add must be visible after the iteration ends")
	}
}

func TestRemoveEntityDuringIterationIsDeferred(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	pos.Add(a, position{X: 1})

	for range pos.All() {
		w.RemoveEntity(a)
		if !w.IsAlive(a) {
			t.Fatal("RemoveEntity during iteration must be deferred")
		}
	}
	if w.IsAlive(a) {
		t.Fatal("entity must be removed after the iteration flushes")
	}
	if pos.Has(a) {
		t.Fatal("removed entity's component must be gone after flush")
	}
}

func TestPointReadAfterIterationSeesDeferredChanges(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	pos.Add(a, position{X: 1})
	b := w.NewEntity()

	for range pos.All() {
		pos.Add(b, position{X: 5})
	}
	// Depth is back to 0, so a plain Get sees the materialized component.
	if p, ok := pos.Get(b); !ok || p.X != 5 {
		t.Fatalf("point read after iteration: ok=%v p=%v, want true {5}", ok, p)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestAddDuringIteration|TestRemoveEntityDuringIteration|TestPointReadAfterIteration' -v`
Expected: PASS already, because Tasks 5 and 6 implement this behavior. If any FAIL, fix the depth check in `Accessor.Add`/`Remove` or `World.endIteration`/`Flush`.

- [ ] **Step 3: Confirm behavior (no new production code expected)**

These tests assert the integration of the deferral mechanics. If they pass, no code change is needed. If they fail, the minimal fix lives in `components.go` or `world.go`; make it and re-run.

- [ ] **Step 4: Run the full suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add deferral_test.go
git commit -m "Verify deferred structural changes during iteration"
```

---

## Task 8: Two-component handle and Tuple2

**Files:**
- Create: `components2.go`
- Test: `components2_test.go`

Implements `Components2[A, B]` / `Accessor2[A, B]` with `Get`, `Has`, `Add` (bundle), and `All` joined iteration yielding `Tuple2`. Per the spec, `Remove` is not offered on the arity-2 handle (remove a single component via the arity-1 handle).

- [ ] **Step 1: Write the failing test**

`components2_test.go`:

```go
package ecs

import (
	"sort"
	"testing"
)

type velocity struct{ DX, DY float64 }

func TestComponents2GetRequiresBoth(t *testing.T) {
	w := NewWorld()
	moving := Components2[position, velocity](w)
	pos := Components[position](w)

	a := w.NewEntity()
	pos.Add(a, position{X: 1}) // has position only

	if moving.Has(a) {
		t.Fatal("Has must be false when only one component is present")
	}
	if _, _, ok := moving.Get(a); ok {
		t.Fatal("Get must be false when only one component is present")
	}

	moving.Add(a, position{X: 2}, velocity{DX: 3}) // bundle attach both
	if !moving.Has(a) {
		t.Fatal("Has must be true when both components are present")
	}
	p, v, ok := moving.Get(a)
	if !ok || p.X != 2 || v.DX != 3 {
		t.Fatalf("Get = %v %+v %+v, want true {2 ..} {3 ..}", ok, p, v)
	}
}

func TestComponents2AllJoinsAndUnpacks(t *testing.T) {
	w := NewWorld()
	moving := Components2[position, velocity](w)
	pos := Components[position](w)

	a := w.NewEntity()
	b := w.NewEntity()
	c := w.NewEntity()
	moving.Add(a, position{X: 1}, velocity{DX: 1})
	moving.Add(b, position{X: 2}, velocity{DX: 2})
	pos.Add(c, position{X: 3}) // c has position only, must be skipped by the join

	var xs []float64
	for id, tup := range moving.All() {
		if !id.IsValid() {
			t.Fatal("join iteration yielded an invalid id")
		}
		p, v := tup.Values()
		p.X += v.DX // mutate position through the interior pointer
		xs = append(xs, p.X)
	}
	sort.Float64s(xs)
	if len(xs) != 2 || xs[0] != 2 || xs[1] != 4 {
		t.Fatalf("join values = %v, want [2 4] (c excluded)", xs)
	}

	// Mutation through the tuple pointer must persist.
	pa, _ := pos.Get(a)
	if pa.X != 2 {
		t.Fatalf("mutation through tuple pointer not persisted: %v, want 2", pa.X)
	}
}

func TestComponents2AddOnDeadEntityIsNoop(t *testing.T) {
	w := NewWorld()
	moving := Components2[position, velocity](w)
	a := w.NewEntity()
	w.RemoveEntity(a)
	moving.Add(a, position{X: 1}, velocity{DX: 1})
	if moving.Has(a) {
		t.Fatal("bundle Add on a dead entity must be a no-op")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestComponents2 -v`
Expected: FAIL, "undefined: Components2".

- [ ] **Step 3: Write minimal implementation**

`components2.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestComponents2 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add components2.go components2_test.go
git commit -m "Add two-component handle with Tuple2 joined iteration"
```

---

## Task 9: End-to-end pipeline test

**Files:**
- Test: `pipeline_test.go`

A capstone test reproducing the spec's behavior-to-movement pipeline: one pass reads a component and attaches another to some entities; a second pass iterates the newly attached set. Verifies the whole module composes, including auto-flush between passes.

- [ ] **Step 1: Write the failing test**

`pipeline_test.go`:

```go
package ecs

import (
	"sort"
	"testing"
)

type wantsToMove struct{ Speed float64 }

func TestBehaviorThenMovementPipeline(t *testing.T) {
	w := NewWorld()
	intent := Components[wantsToMove](w)
	positions := Components[position](w)
	velocities := Components[velocity](w)

	// Three entities with a position; two of them want to move.
	a := w.NewEntity()
	b := w.NewEntity()
	c := w.NewEntity()
	for _, e := range []EntityId{a, b, c} {
		positions.Add(e, position{X: 0})
	}
	intent.Add(a, wantsToMove{Speed: 2})
	intent.Add(c, wantsToMove{Speed: 5})

	// Behavior pass: for each mover, attach a velocity. Attaches are deferred
	// during iteration and flushed when the loop ends.
	for id, wm := range intent.All() {
		velocities.Add(id, velocity{DX: wm.Speed})
	}

	// Movement pass: every entity that now has both position and velocity moves.
	moving := Components2[position, velocity](w)
	for id, tup := range moving.All() {
		p, v := tup.Values()
		p.X += v.DX
		_ = id
	}

	// a and c moved; b did not.
	pa, _ := positions.Get(a)
	pb, _ := positions.Get(b)
	pc, _ := positions.Get(c)
	got := []float64{pa.X, pb.X, pc.X}
	sort.Float64s(got)
	want := []float64{0, 2, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("final X positions = %v, want %v", got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestBehaviorThenMovementPipeline -v`
Expected: PASS if Tasks 1 through 8 are correct (this test uses only their public APIs and verifies they compose). If it FAILS, the bug is in an earlier task's behavior; fix the responsible file and re-run.

- [ ] **Step 3: Confirm behavior (no new production code expected)**

If Tasks 1 through 8 are correct, this passes with no production change. If it fails, fix the responsible earlier file.

- [ ] **Step 4: Run the full suite with the race detector**

Run: `go test ./... -race`
Expected: PASS with no data-race reports. (Sequential use must be race-clean.)

- [ ] **Step 5: Commit**

```bash
git add pipeline_test.go
git commit -m "Add end-to-end behavior-to-movement pipeline test"
```

---

## Task 10: Documentation pass

**Files:**
- Create: `README.md`
- Modify: `doc.go` (expand with a usage example if helpful)

- [ ] **Step 1: Write a runnable example test**

`example_test.go`:

```go
package ecs_test

import (
	"fmt"

	"github.com/trancecode/ecs"
)

type Position struct{ X, Y float64 }
type Velocity struct{ DX, DY float64 }

func Example() {
	w := ecs.NewWorld()
	positions := ecs.Components[Position](w)
	moving := ecs.Components2[Position, Velocity](w)

	e := w.NewEntity()
	positions.Add(e, Position{X: 0})
	moving.Add(e, Position{X: 0}, Velocity{DX: 3})

	for _, tup := range moving.All() {
		p, v := tup.Values()
		p.X += v.DX
	}

	p, _ := positions.Get(e)
	fmt.Printf("%.0f\n", p.X)
	// Output: 3
}
```

- [ ] **Step 2: Run the example to verify it fails**

Run: `go test ./... -run Example -v`
Expected: FAIL only if the public API names drift from the example; otherwise it passes and documents usage. (An `Example` with an `// Output:` is verified by `go test`.)

- [ ] **Step 3: Write the README**

`README.md`:

```markdown
# ecs

An Entity Component System for Go.

Entities are opaque identifiers. Components are plain structs in per-type sparse
sets. Typed handles read and mutate components through interior pointers.
Structural changes apply immediately outside iteration and are deferred to an
automatic flush during iteration, so pointers stay valid for the iteration.

## Usage

    w := ecs.NewWorld()
    positions := ecs.Components[Position](w)
    moving := ecs.Components2[Position, Velocity](w)

    e := w.NewEntity()
    positions.Add(e, Position{X: 0})
    moving.Add(e, Position{X: 0}, Velocity{DX: 3})

    for _, tup := range moving.All() {
        p, v := tup.Values()
        p.X += v.DX
    }

See `docs/superpowers/specs/2026-06-17-ecs-framework-design.md` for the design
and the deferred future seams.
```

- [ ] **Step 4: Run the full suite and vet**

Run: `go test ./... && go vet ./...`
Expected: PASS, no vet warnings.

- [ ] **Step 5: Commit**

```bash
git add README.md doc.go example_test.go
git commit -m "Add README and runnable usage example"
```

---

## Definition of done

* `go test ./... -race` passes.
* `go vet ./...` is clean.
* Public surface matches the spec: `EntityId`, `World` (`NewWorld`, `NewEntity`, `RemoveEntity`, `IsAlive`, `Flush`), `Components`/`Accessor[A]`, `Components2`/`Accessor2[A, B]`, `Tuple2`.
* Deferred structural changes, auto-flush on iteration unwind, immediate-at-depth-0, and liveness validation at apply time all have passing tests.

## Notes for the implementer

* The handle type name `Accessor` is provisional per the spec's deferred decisions; if a better name is chosen, rename `Accessor`/`Accessor2` consistently in `components.go`, `components2.go`, and the example.
* `Components3` and higher arities are deliberately out of scope. They are a mechanical replication of the `Components2`/`Tuple2` pattern (one more store field, one more `applyAdd` in the bundle, one more lookup in `All`, a `Tuple3.Values` returning three pointers) and should be added only when a consumer needs them.
* The command buffer uses closures captured per deferred operation. This allocates on the deferral path only, which is acceptable because structural changes are far rarer than reads; do not move this allocation onto the read or iteration path.
* Concurrency is sequential in version 1. The atomic depth counter and the mutex around the command buffer are forward-compatibility scaffolding, not a guarantee of parallel-safety; do not advertise parallel-safety.
