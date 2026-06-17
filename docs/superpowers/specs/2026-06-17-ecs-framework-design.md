# ECS framework design

Status: approved design, ready for implementation planning
Date: 2026-06-17
Module: `github.com/trancecode/ecs`, package `ecs`
Go version: 1.26 (relies on range-over-func and the `iter` package, stable since 1.23)

## Overview

A standalone Entity Component System (ECS) framework in Go, extracted and improved from the
ECS embedded in the `nrg` repository. The long-term goal is to migrate `nrg` onto it and reuse
it in at least one other game. Because the framework will be a shared dependency of multiple
repositories, the primary design driver is a public API that can grow (events, resources,
alternate storage, parallelism) without breaking callers.

This is the version 1 scope: the "EC" of ECS (entities and components, with deferred structural
mutation and iteration). System scheduling is intentionally left to the consumer. See "Scope".

## North star: API stability across repositories

Once `nrg` and a second game both depend on this module, every breaking change to a public
signature becomes a multi-repository migration. Every decision below is biased toward keeping the
storage model, concurrency model, and feature set as private implementation details behind a
small, stable public surface. Where a choice trades present convenience for future freedom, it is
called out.

## Scope

In scope for version 1:

* Entities with opaque identifiers and liveness tracking.
* Plain-data components stored in per-type sparse sets.
* Pointer-based read and in-place mutation of existing components.
* Deferred structural mutation (add, remove, create, destroy) with an automatic flush model.
* Single and multi-component typed access and iteration handles.

Explicitly out of scope for version 1, with extension seams preserved (see "Future extension
seams"):

* A systems scheduler, stages, ordering, or a parallel executor. The consumer owns the run loop.
* Events, resources/singletons, and change detection (seams kept; nothing built).
* Archetype storage (sparse set is the version 1 implementation; the seam to swap it is kept).
* Serialization.

## Core decisions

### 1. Component storage and access: contiguous values, interior pointers, immediate writes

Components are stored as values packed contiguously per component type ("structure of arrays" per
type), which gives cache-friendly iteration. Reads and mutations of an existing component go
through an interior pointer into that storage:

* Reading or mutating an existing component returns a `*C` that points into storage. Field writes
  through that pointer take effect immediately on the stored component. There is no separate write
  or save call.
* The cache-locality benefit comes from contiguous storage and iteration, not from the return
  type. The return type choice is about mutation ergonomics, not throughput.

Lifetime rule: a `*C` is valid until the next structural change to that component's store. During
iteration this is automatic because structural changes are deferred (see decision 4). Outside
iteration the rule is "do not hold a `*C` across an `Add` or `Remove` on the same component store."

Tradeoff accepted: raw interior pointers make a mutation invisible to the framework, so transparent
automatic change detection is not possible. This is acceptable because Go has no operator
overloading and therefore cannot offer transparent detection under any scheme; change detection
remains achievable through additive, opt-in seams (see "Future extension seams").

### 2. Components are plain data

A component is any plain Go struct. There is no embedded entity identifier and no required
interface or base type. The component store is generic over `C any`. Iteration yields
`(EntityId, *C)` pairs, because the identifier is owned by the store, not the component.

Rationale:

* The store already owns the `EntityId`-to-slot mapping, so an embedded identifier would be
  redundant and a source of desynchronization if a value were copied into another entity's slot.
* Plain-data components have zero coupling to the framework, which is the most stable possible
  public contract and matches the conventions of Ark and Bevy.
* Smaller components pack more per cache line.

Cost accepted: migrating `nrg` rewrites every `component.EntityId()` call site, since `nrg`
components currently embed `BaseEntity`.

### 3. Entity identifier: opaque value type

```go
type EntityId struct { v uint64 } // unexported field
func (EntityId) String() string
func (EntityId) IsValid() bool
```

* The zero value is the invalid identifier; real identifiers are allocated monotonically and never
  reused in version 1.
* `String` renders a prefixed, greppable form: `ent_<n>` (for example `ent_123`) for a real
  identifier, and `ent_invalid` for the zero value. It is for human-readable logs only; there is no
  parse-back, since the type is opaque and identifiers are allocated at runtime.
* The field is unexported so the type is opaque by construction. Callers can pass it around,
  compare it, use it as a map key, write `EntityId{}` for the invalid value, and call its methods.
  Callers cannot fabricate a valid identifier, do arithmetic, or convert to or from `uint64`.

Rationale: opacity is enforced by the compiler rather than by convention, which matters precisely
because in a multi-repository setting we cannot police what other repositories do with the type.
This preserves the freedom to later change the representation (for example pack a generation into
the bits, or switch to recycled indices and array-based storage) with no caller-visible change. A
bare `type EntityId uint64` would only request that discipline; the struct guarantees it.

No generations and no recycling in version 1. Stale references resolve safely to "not alive"
because identifiers are never reused. Generations and array-based sparse sets are an internal
optimization that the opaque type and the handle-based access surface keep available for later.

### 4. Deferred structural changes and the flush model

Structural changes are: add a component, remove a component, create an entity, destroy an entity.
Value-field mutation through an interior pointer is not a structural change.

Rules:

* Structural changes issued while no iteration is active (depth 0) apply immediately. There are no
  live interior pointers to invalidate, so there is nothing to protect.
* Structural changes issued while an iteration is active (depth at least 1) are deferred into a
  command buffer.
* The command buffer is flushed when the outermost iteration unwinds to depth 0 (the 1-to-0
  transition). At that moment every deferred change is applied.
* An explicit `world.Flush()` is available as an escape hatch, for the rare case where pending
  changes must materialize before non-iterating code (point reads, serialization, handing the
  world to another subsystem).

Invariant: at depth 0 the world is fully materialized. Deferral is purely an inside-iteration
safety mechanism and is invisible from the outside. Consequently a point read after an iteration
sees the structural changes that iteration queued.

Depth tracking uses range-over-func: iteration is exposed as an iterator function whose body the
framework controls, so it brackets the iteration with an increment on entry and a deferred
decrement on exit. The deferred decrement runs on normal completion, `break`, `return`, and panic.
Nested iteration increments depth above 1 and therefore does not flush, which keeps interior
pointers held by an outer loop valid. Depth is an atomic counter so the design is concurrency
ready (see decision 8).

Apply-time rules:

* All registry effects of a destroy (generation bump in a future version, free-list push in a
  future version, and removal from all stores) happen at apply time, not at queue time. Until the
  flush, a destroyed-but-not-yet-flushed entity stays alive and readable.
* Every deferred command re-validates entity liveness at apply time. Operating on a dead entity is
  a no-op. This makes ordering robust: `RemoveEntity(E)` then `Add(E, ...)` queued together applies
  as destroy followed by a no-op add.

Spawn asymmetry: `NewEntity()` allocates the identifier immediately and atomically, even during
iteration, because the caller needs a usable identifier to attach components or store a reference.
Component attachment is deferred. So registry allocation is immediate; component-store mutation is
the deferred, flush-only writer.

Timeline example, a behavior stage feeding a movement stage:

```
spawn 3 units            depth 0   -> applied immediately, world has 3 units
behavior stage:
  for ... := range behavior.All()   depth 0->1
    decide moves                    depth 1   -> Add(Velocity) queued for movers
  loop ends                         depth 1->0 -> FLUSH, Velocity components materialize
movement stage:
  for ... := range moving.All()     depth 0->1, iterates movers including the new ones
  loop ends                         depth 1->0
```

### 5. Storage model: sparse set, kept internal

Version 1 stores each component type in its own sparse set (a dense value slice plus an identifier
to slot mapping), the same family `nrg` uses today. Single-component iteration is cache-perfect; a
multi-component query iterates the smaller set and looks up the others; add and remove touch only
the one store and are cheap and localized.

The storage model is a private implementation detail behind the access handles and the `World`
methods. The public surface is the typed handles and `World`, never named per-component store
fields. The raw fast path, if provided, is chunked iteration ("hand me values in bulk, chunk by
chunk"), not a single-slice-per-component accessor, because a single-slice promise would leak the
sparse-set shape and foreclose archetype storage.

Rationale: for this genre (moderate entity counts, expensive per-entity work) the join cost of
sparse sets stays inside L2 and L3 cache and is dominated by per-entity logic, so the archetype
advantage does not justify its complexity now. Keeping the model internal preserves the option to
introduce archetype storage later, informed by the access signatures that multi-component handles
declare, without breaking callers.

### 6. Public API surface

`World` carries the operations that are not parameterized by component type:

```go
func NewWorld() *World
func (w *World) NewEntity() EntityId
func (w *World) RemoveEntity(id EntityId)
func (w *World) IsAlive(id EntityId) bool
func (w *World) Flush()
```

Everything parameterized by component type is a reusable typed handle, constructed once (for
example stored as a field of a consumer's system) and reused. This is required because Go methods
cannot have their own type parameters, so generic access cannot be a method on `World`; the type
parameter lives on the handle. It also caches the store resolution for hot paths.

```go
// Construction. The handle type names are provisional; see "Deferred decisions".
func Components[A any](w *World) Accessor[A]
func Components2[A, B any](w *World) Accessor2[A, B]
func Components3[A, B, C any](w *World) Accessor3[A, B, C]

// Single-component handle.
func (h Accessor[A]) Get(id EntityId) (*A, bool)
func (h Accessor[A]) Has(id EntityId) bool
func (h Accessor[A]) Add(id EntityId, a A)
func (h Accessor[A]) Remove(id EntityId)
func (h Accessor[A]) All() iter.Seq2[EntityId, *A]

// Two-component handle. Get reports true when the entity has both.
func (h Accessor2[A, B]) Get(id EntityId) (*A, *B, bool)
func (h Accessor2[A, B]) Has(id EntityId) bool
func (h Accessor2[A, B]) Add(id EntityId, a A, b B) // bundle attach
func (h Accessor2[A, B]) All() iter.Seq2[EntityId, Tuple2[A, B]]

// Iteration element for arity 2. The component pointers are unexported and
// unpacked with Values. TupleN exists for each arity.
type Tuple2[A, B any] struct { /* unexported *A, *B */ }
func (t Tuple2[A, B]) Values() (*A, *B)
```

The numbered variants (`Components2`, `Components3`, and so on) exist because Go has no variadic
type parameters. The unnumbered `Components` is the arity-1 case, following the same convention as
Ark's `Map` and `Map2`.

Iteration is range-over-func at every arity, which is what keeps depth tracking and auto-flush
robust against `break`, `return`, and panic; a manual cursor (`Next`/`Get`) could not hook those
early exits and would leak the depth counter. Go's range-over-func allows at most two loop
variables, so the uniform shape is "identifier first, payload second":

* Arity 1: the payload is the component pointer directly, `iter.Seq2[EntityId, *A]`.
* Arity 2 and higher: the payload is a `TupleN` of component pointers, unpacked with `Values`.

```go
for id, pos := range positions.All() { // arity 1
    pos.X += 1
    _ = id
}

for id, c := range moving.All() {      // arity 2
    pos, vel := c.Values()
    pos.X += vel.DX
    _ = id
}
```

Construction and structural mutation:

```go
positions := ecs.Components[Position](world)
moving    := ecs.Components2[Position, Velocity](world)

e := world.NewEntity()
positions.Add(e, Position{X: 10})
```

Vocabulary summary:

* Components: `Get[C]`, `Has[C]`, `Add[C]`, `Remove[C]`, and `All` iteration, all via `Components`
  handles. Short names; the type parameter conveys "component". Future resources take the qualified
  names (for example `Resource[T]`).
* Entities: `world.NewEntity()`, `world.RemoveEntity(id)`.
* Lifecycle: `world.Flush()`.

### 7. Entity registry

The registry is the authority on liveness, independent of components, because an entity can be
alive with zero components. In version 1 it allocates monotonic identifiers and tracks which
identifiers are alive, so that `IsAlive` and destroy work correctly.

No per-entity component mask in version 1. Destroy probes all registered stores (the component type
count is small, on the order of `nrg`'s sixteen). A mask would speed destroy and enable cheap
"which components does this entity have" introspection, but it is another invariant to maintain on
every add and remove, and it can be added later as a pure internal optimization.

### 8. Concurrency readiness

Version 1 runs sequentially, but the design does not foreclose parallel systems, and because all
concurrency lives under the iterate-and-flush API, parallelism can be added later without changing
the public surface.

* The depth counter is atomic. With parallel iterations, depth rises above 1 and the flush fires
  only when the last concurrent iteration exits. The goroutine that brings depth to 0 performs the
  flush, under a lock, with a concurrency-safe command buffer.
* The only structural writer to the stores is the flush, which runs when no iteration is live, so
  structural changes never race with iteration.

Caveat: value writes through interior pointers are immediate and unsynchronized, so two parallel
systems mutating the same component field still race. Preventing that requires a scheduler that
parallelizes only systems with disjoint mutable access (the Bevy model). That scheduler is the
consumer's responsibility; the framework enables concurrency but does not by itself guarantee
data-race freedom across overlapping mutable access.

## Future extension seams

These are not built in version 1. The point is that each can be added without breaking callers.

* Structural lifecycle events (component added or removed, entity created or destroyed): the
  command buffer and flush are a single choke point where these can be emitted. Adding listener
  registration (for example `world.OnAdd[C](fn)`) is purely additive.
* Resources and singletons, and domain pub/sub events: structurally these are typed handles over a
  buffer or a single slot, the same pattern as `Components`. They are added as sibling handle
  families (for example `Resource[T]`, `Events[E]`). The internal type-to-slot registry should be
  built as a reusable mechanism rather than a components-only special case so these slot in
  cleanly; the public surface is unaffected.
* Change detection: achievable on top of raw pointers two ways, both additive. A conservative
  mark-on-mutable-access route (a read-only versus mutable handle distinction marks every entity a
  mutable handle hands out, as Bevy does for mutable access) and a precise opt-in `Modify(id,
  func(*C))` route (the closure call is the hook). Neither is in version 1; the read-only versus
  mutable handle distinction and the `Modify` method are recorded as the seams.
* Archetype storage: the access handles hide the storage model, and multi-component handles declare
  the access signatures an archetype backend would use. Swapping or adding archetype storage is an
  internal change.
* Systems scheduler: the consumer owns the run loop in version 1. An optional, separable scheduler
  module can be added later once a second game reveals the common shape.

## Accepted foreclosures and tradeoffs

* Transparent automatic change detection (Bevy's deref-driven model) is not possible, because Go
  has no operator overloading. Change detection, if needed, is conservative or opt-in (see seams).
* Raw interior pointers carry the "do not hold across a structural change to the same store"
  lifetime rule. It is automatic during iteration and a documented rule at depth 0.
* Value-copy access (a `Get` that returns a copy plus a `Set`) was rejected. Its only real
  advantage, a guaranteed write hook for precise detection, costs a mandatory write-back ceremony
  on every mutation and a silent forget-to-write footgun, and its apparent concurrency benefit is
  illusory: the standard disjoint-access scheduling model makes raw pointers safe without copies,
  per-getter-and-setter locking is insufficient for correct read-modify-write, and genuine snapshot
  isolation is a separate double-buffering feature.

## nrg migration notes

* `EntityId` widens from a bare `uint32` to the opaque struct. Call sites that compare to `0` or
  convert to or from an integer must move to `IsValid()` and to identifiers obtained from the API.
* Components drop the embedded `BaseEntity`; every `component.EntityId()` call site changes to use
  the identifier yielded by iteration or held alongside.
* `nrg` keeps its own run loop and event-driven time stepping. It calls `world.Flush()` at its
  stage boundaries where needed, although the automatic flush on iteration covers the common case.
* Code that caches a component pointer across a structural change relies today on `nrg`'s eternally
  stable pointers and must be audited, since interior pointers here are valid only until the next
  structural change to that store.

## Deferred decisions

* Exact handle type names (`Accessor`, `Accessor2`, and so on are provisional). The constructor
  function names `Components`, `Components2`, `Components3` are settled.
* Mutation semantics on arity-2-and-higher handles. The leaning is bundle `Add`, with `Remove`
  restricted to the arity-1 handle to avoid a surprising "remove both" meaning. To be settled when
  the command-buffer surface is specified.
* Whether to offer thin free-function convenience wrappers (for example `ecs.Get[C](world, id)`)
  for one-off access, in addition to handles. Additive, can come later.
* The internal mechanism for keying heterogeneous `ComponentStore[C]` values in one registry (the
  Go generics wrinkle). An implementation detail, not a public-surface decision.
