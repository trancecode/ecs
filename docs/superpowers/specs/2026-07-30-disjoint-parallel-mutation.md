# Concurrent mutation of disjoint entity sets

Status: **proposed design, awaiting agreement. Not ready for implementation.**
Date: 2026-07-30
Issue: trancecode/ecs#14
Module: `github.com/trancecode/ecs`, package `ecs`
Supersedes nothing. Extends `2026-06-17-ecs-framework-design.md`, decision 8 ("Concurrency readiness").

## Why this document exists and what it does not authorise

Issue #14 asks for three things, in order: an agreed design, a race-detector test proving
sequential equivalence, and no single-threaded regression. The first is a gate on the other two.
This document is that first deliverable and nothing more.

Implementation remains blocked. The single-threaded work the issue names (trancecode/vantage#15,
#16, #17 and lockstep's in-flight-effect indexing) is still open as of this writing, and until it
lands and is measured we do not know where the performance ceiling actually is. A four-line
pathfinding fix was recently worth 176x to 374x; parallelism is not the next lever. Nothing here
should be built until those land, the ceiling is re-measured, and this design is agreed.

## The requirement, restated

A future parallel simulation scheme runs **disjoint sets of entities** on separate goroutines
within one world, synchronising at barriers. The ECS must tolerate that. Specifically:

* Concurrent reads and writes to components of disjoint entity sets, without a global lock.
* Structural changes (create, destroy, add or remove component) that do not invalidate pointers
  held by other goroutines operating on unrelated entities.
* **Determinism preserved.** Both consuming games depend on bit-identical replay and savegames.
  Any parallel path must produce results identical to the sequential path, and merges must happen
  in a fixed order. This is a hard requirement.

### The one interpretation this design makes, and needs agreement on

"Structural changes that do not invalidate pointers held by other goroutines" admits two readings:

1. Structural changes take effect **at the barrier**, so no pointer is invalidated during the
   parallel region because storage does not change shape while goroutines are running.
2. Structural changes take effect **immediately and are visible inside the region**, and storage is
   engineered so that doing so relocates nothing.

**This design takes reading 1.** It is the cheaper reading by a wide margin, it matches the
consumer's stated model ("synchronising at barriers"), and it is the semantics the sequential API
already has: structural changes issued during an iteration are deferred to the flush today. Reading
2 requires replacing the sparse-set storage model wholesale (see "Rejected alternatives", A) and
pays for it on the single-threaded path, which the issue's third acceptance criterion protects.

If the consumer genuinely needs a structural change made by group 0 to be observable by group 1
before the barrier, reading 1 is wrong and this design must be reworked. That is the question to
settle before anything is built.

## Why today's world is unsafe

Concrete, with references, because the failure modes drive the design.

1. **Storage growth relocates everything.** `componentStore.applyAdd` appends to `dense`
   (`ecs/ecs_store.go:49`). When `append` reallocates, every interior pointer into that store
   dangles, including pointers held by goroutines touching entirely unrelated entities.

2. **Removal relocates a bystander.** `applyRemove` is swap-and-pop (`ecs/ecs_store.go:56`): it
   moves the *last* entity's component into the removed entity's slot. So a structural change to
   entity X silently relocates entity Y's component, where Y is whatever happened to be last. Y may
   belong to another goroutine's group. This is worse than the growth case, because it invalidates
   pointers even when capacity is untouched, and it is invisible in the source at the call site.

3. **Unsynchronised registry maps.** `w.alive` is a plain `map[EntityId]struct{}` written by
   `NewEntity` (`ecs/ecs_world.go:44`) and read by `IsAlive`. `w.stores` is a plain map written by
   `storeOf` on first use of a component type (`ecs/ecs_world.go:89`). `componentStore.index` is a
   plain map written by every add and remove. Concurrent write plus any access is a hard fault in
   Go's map implementation, not merely a logical race.

4. **Non-deterministic command order.** `enqueue` appends under `w.mu` (`ecs/ecs_world.go:75`).
   Correct, but the resulting order is whatever the scheduler produced. Feeding that to `Flush`
   (`ecs/ecs_world.go:118`) yields a different world per run. This alone disqualifies the current
   command buffer from carrying parallel work.

5. **Non-deterministic identifier allocation.** `NewEntity` takes `w.nextID.Add(1)`
   (`ecs/ecs_world.go:43`). Atomic, so memory-safe, but which goroutine wins each increment is
   scheduling-dependent. Identifiers participate in downstream ordering (the `RestoreEntity`
   documentation already records that ids are used as tie-breakers), so scheduling-dependent ids
   are scheduling-dependent simulation.

6. **The flush trigger fires at the wrong time.** `endIteration` flushes on the 1-to-0 depth
   transition (`ecs/ecs_world.go:103`). With several goroutines iterating, whichever exits last
   performs the flush, mutating storage while nothing guarantees the others have finished.

What is *already* right: the depth counter is atomic, the command buffer is mutex-guarded, and
deferral is the established mechanism for protecting interior pointers. The design below reuses all
three rather than replacing them.

## Core decision: structural quiescence for the duration of a parallel region

**Inside a parallel region, the world is structurally frozen.** No storage is allocated, grown,
compacted, or reordered; no map is written; no entity enters or leaves the liveness set. The only
writes are value writes through interior pointers into slots that already exist, each performed by
the single goroutine that owns the entity in that slot.

Everything else follows from that one property:

* No pointer can be invalidated, because nothing moves. Requirement 2 is satisfied by construction
  rather than by careful storage engineering.
* Concurrent map *reads* with no concurrent writer are safe in Go and are accepted by the race
  detector, so `Get`, `Has`, and liveness queries need no lock. Requirement 1's "no global lock"
  falls out.
* Value writes land on disjoint addresses, so their interleaving is unobservable.
* Every structural change is buffered per goroutine and applied by one goroutine at the barrier, in
  an order fixed by group index. Requirement 3 reduces to "define one fixed merge order".

The sequential fast path is completely untouched: it is the same sparse set, the same swap-and-pop,
the same interior pointers. This is the whole reason to prefer quiescence over stable storage.

### 1. The region: `RunDisjoint`

```go
// Scope is one goroutine's view of the world inside a parallel region. It is
// valid only for the duration of the callback that receives it; retaining a
// Scope past that callback is a programmer error.
type Scope struct{ /* unexported */ }

func (s *Scope) Index() int                 // this scope's group index
func (s *Scope) Group() []EntityId          // this scope's entities, in caller order
func (s *Scope) NewEntity() EntityId
func (s *Scope) RemoveEntity(id EntityId)
func (s *Scope) IsAlive(id EntityId) bool

// RunDisjoint runs fn once per group, each group on its own goroutine, and
// returns once every group has finished and their structural changes have been
// applied. The groups must be pairwise disjoint.
func (w *World) RunDisjoint(groups [][]EntityId, fn func(s *Scope)) error

// RunDisjointSerial runs the same work on the calling goroutine, groups in
// index order. It produces a bit-identical world to RunDisjoint. It exists to
// make that equivalence testable, to give a fallback on single-core targets,
// and to bisect a determinism divergence.
func (w *World) RunDisjointSerial(groups [][]EntityId, fn func(s *Scope)) error
```

`fn` receives only a `Scope`, not a group index and a world, because the scope is the complete
capability set for in-region work. The consumer partitions; the ECS guarantees safety and
determinism for a partition it is handed.

Sequence for a region:

1. Require depth 0 and no region already active; otherwise return an error.
2. `w.Flush()`, so the world is fully materialised before any goroutine reads it.
3. Validate pairwise disjointness of `groups` and that every identifier is alive.
4. Snapshot `base = w.nextID.Load()` and reserve identifier blocks (decision 3).
5. Set the region flag and raise depth once. Depth stays at or above 1 for the whole region, so no
   goroutine can ever trigger the 1-to-0 auto-flush.
6. Run the groups, each with its own `Scope` carrying a private command buffer.
7. Join. If any group panicked, re-panic on the calling goroutine with the panic from the
   **lowest-numbered** group that panicked, so even a crash is deterministic.
8. Concatenate the scope buffers into `w.commands` in group-index order, fold the per-scope
   counters into the world's, clear the region flag, advance `nextID`, and lower depth. The
   1-to-0 transition flushes, applying every buffered change on one goroutine in the merged order.

### 2. Binding handles to a scope

```go
// Bind returns a copy of the handle scoped to s: structural changes are recorded
// in s's private command buffer instead of the world's, and All iterates only
// the entities in s's group. The returned handle must not outlive s.
func (h Accessor[A]) Bind(s *Scope) Accessor[A]
func (h Accessor2[A, B]) Bind(s *Scope) Accessor2[A, B]
```

`Bind` returns the same type, so every existing method works unchanged and the public surface grows
by one method per handle family. `Accessor` gains one unexported `*Scope` field, nil when unbound.

* `Get`, `Has`: identical to today. Pure reads of frozen structures, no branch added.
* `Add`, `Remove`, `GetOrAdd`, `GetOrAddFunc`: the deferral branch chooses `s.enqueue` over
  `w.enqueue`. This is the cold path, which already takes a mutex today; one nil check is noise.
  `GetOrAdd`'s staged-value semantics are unchanged — the staged value is goroutine-local.
* `All`: iterates `s.Group()` in caller order and point-looks-up the component, skipping entities
  that lack it. It does **not** touch the depth counter: the region already holds depth above zero,
  and having *n* goroutines contend on one atomic per loop entry is pure loss.

Unbound `All` inside a region iterates the whole dense array, which reads other groups' components
while they are being written. That is a contract violation, not a supported mode.

Accepted cost: the parallel path trades cache-perfect dense iteration for one sparse-set lookup per
entity. That is the price of partitioning by consumer-chosen entity sets rather than by storage
order. See "Deferred: dense-range partitioning" for the complementary cheap case.

### 3. Deterministic identifier allocation

`nextID.Add(1)` from several goroutines is memory-safe and order-nondeterministic, so in-region
allocation cannot use it. Instead, identifier space is carved into blocks by a pure function of the
group index.

With `base` the counter at region entry, `n = len(groups)`, and a fixed `stride`, group `g`'s
`r`-th block is the half-open raw range

    ( base + (r*n + g)*stride , base + (r*n + g + 1)*stride ]

Each scope allocates from its current block and moves to its next block (`r+1`) on exhaustion, with
no coordination. At the barrier, with `R` the number of rounds any group reached (`R = 0` if
nothing spawned), the world sets `nextID = base + R*n*stride`.

This is deterministic because each group's spawn count is a deterministic function of its own
entities, which is exactly the precondition the consumer already accepts by claiming the groups are
disjoint and the simulation is replayable.

Accepted consequences, both worth agreeing to explicitly:

* **Identifier space is sparse.** Unused block tails are skipped forever. Identifiers are `uint64`
  and never reused, so exhaustion is not a practical concern, but `EntityCounter` grows faster than
  the entity count and savegames record the larger number. Nothing breaks; the number is just
  bigger.
* **Allocation order within a region reflects (group, spawn index), not wall-clock spawn order.**
  Two entities spawned "at the same time" in different groups order by group index. Deterministic,
  which is what the requirement asks for, but it is a semantic change from the global monotonic
  sequence, and anything downstream that reads meaning into id ordering should be checked.

The alternative — allocate identifiers at the barrier in merge order, keeping the space dense — was
rejected because the identifier would not be usable inside the region, and `NewEntity`'s whole
contract (design doc, decision 4, "spawn asymmetry") is that the caller gets a usable identifier
immediately.

### 4. Liveness during a region

`w.alive` must not be written while goroutines read it. So:

* `Scope.NewEntity` allocates the identifier immediately from the group's block and records the
  entity in a **scope-local** spawn set plus the scope's command buffer. Insertion into `w.alive`
  happens at the barrier.
* `Scope.IsAlive` consults `w.alive` (a read of a frozen map) and the scope-local spawn set, so a
  goroutine sees its own spawns as alive. It does not see another group's in-region spawns, which
  is correct: those entities are not the caller's to touch.
* `Scope.RemoveEntity` buffers the destroy. The entity stays alive and readable until the barrier,
  exactly as `RemoveEntity` behaves during an iteration today.

So the spawn asymmetry is preserved in the part that matters (the identifier is immediate) and
relaxed in the part that races (the registry write is deferred, with a local shadow).

### 5. Preconditions and how each is enforced

The contract is a set of caller obligations. Each is either checked cheaply, checked behind a build
tag, or documented — never silently assumed when a cheap check exists.

| Obligation | Enforcement |
|---|---|
| Groups are pairwise disjoint | Checked on entry, `O(total)` against a `World`-owned scratch map reused across regions. Returns an error. |
| Every listed entity is alive | Checked in the same pass. Returns an error. |
| Depth is 0 and no region is active on entry | Checked. Returns an error. |
| No component store is created during a region | `storeOf`'s miss path panics when the region flag is set. Zero cost on the hit path; construct all handles before entering. |
| `World.Flush` is not called during a region | Panics. `Flush` has no error return, and the alternative — rewriting storage while goroutines hold interior pointers — is memory-unsafe. |
| A scope touches only its own entities | Checked only under the `ecs_debug` build tag, using the ownership map the disjointness check already built. Always-on per-access checking would cost more than the parallelism buys. |
| Unbound handles are not used inside a region | Documented. Cannot be checked without a per-access cost. |

Errors rather than panics for the first three, per the style guide: they are input validation a
caller could reasonably hit. Panics for the store-creation and `Flush` cases: those are invariant
violations with no safe continuation.

### 6. Contention removed from shared counters

`enqueue` increments `w.deferredOps` atomically today (`ecs/ecs_world.go:79`). From *n* goroutines
that is a contended cacheline on the structural path. Scopes count locally and fold into the world
at the barrier. The total is a sum, so it is order-independent and therefore deterministic.

`Stats` during a region reports the pre-region structure with nothing pending, since in-region
changes live in scope buffers. It is documented as a barrier-time operation.

## The determinism argument

Claim: for a given world state, a given `groups` argument, and a deterministic `fn`, `RunDisjoint`
produces a bit-identical world to `RunDisjointSerial`, on every run, regardless of goroutine
scheduling and `GOMAXPROCS`.

The proof is by cases over everything a region can change.

1. **Value writes through interior pointers.** Group `g` writes only components of entities in
   group `g`. Groups are disjoint (checked on entry) and each entity occupies exactly one slot per
   store, so the write sets of any two groups are disjoint address sets. Writes to disjoint
   addresses commute, and no group reads another group's components (contract, checked under
   `ecs_debug`), so no group can observe another's partial state. The post-region byte content of
   every slot is therefore independent of interleaving, and equals what serial execution in any
   group order would produce.

2. **Structural changes.** Every one is buffered, never applied, during the region. At the barrier
   they are concatenated in group-index order, each group's commands in that group's program order,
   and applied by one goroutine. That total order is a pure function of `groups` and `fn`, with no
   scheduling input. It is by definition the order serial execution in group order produces.

3. **Identifier allocation.** Group `g`'s `k`-th identifier is `base + (r*n + g)*stride + j` where
   `r` and `j` are determined by `k` and `stride` alone. It is a pure function of `(base, n, g, k)`.
   `base` is read before any goroutine starts; `nextID` at exit is a pure function of `base`, `n`,
   `stride`, and the per-group spawn counts. No scheduling input anywhere.

4. **Storage layout.** Dense-array order after the region is determined entirely by the sequence of
   `applyAdd` and `applyRemove` calls the flush makes, which by case 2 is fixed. So not only the
   logical world but the physical `dense` and `ids` slices are identical between parallel and
   serial runs — which is what makes a byte-level test possible rather than a semantic one.

5. **No map iteration order leaks.** `applyRemoveEntity` ranges `w.stores`, a map
   (`ecs/ecs_world.go:68`). Map iteration order is randomised in Go, so this must be shown not to
   matter: each store's `applyRemove` touches only that store, and the stores are independent, so
   the set of effects is the same for any visiting order. `Stats` and `String` already sort by type
   name (`ecs/ecs_stats.go:60`). No other map range affects observable state. This must be
   re-checked whenever a map range is added; a note belongs next to that loop.

The load-bearing preconditions are disjointness (checked), no cross-group reads (contract), and a
deterministic `fn` (the consumer's, and already required for replay today).

## Single-threaded cost

The third acceptance criterion. The mechanism by which the cost is near-zero:

* `Get`, `Has`, `All`, and every value write: **byte-identical code paths**. Not "optimised to be
  as fast"; unchanged.
* `Accessor` grows one pointer field (16 to 24 bytes). Handles are constructed once and reused per
  the design doc, so this is not per-operation cost.
* `Add`, `Remove`, `GetOrAdd`: one nil check on the deferral branch, which already takes a mutex.
* `storeOf`: the region-flag check sits on the miss path only, after the map lookup fails. The hit
  path is unchanged.
* Sparse-set storage, swap-and-pop removal, and dense iteration are all retained. Nothing about
  the single-threaded shape is compromised to enable the parallel path.

Verification: the existing `ecs/ecs_bench_test.go` benchmarks must be unchanged within noise, run
before and after with `benchstat`. Any statistically significant regression on a sequential
benchmark blocks the change; that is the criterion, and it is on the implementer to produce the
numbers rather than argue from the list above.

## Acceptance test plan

The issue's second criterion, made concrete.

**Sequential-equivalence test, under `-race`.** Build a world with entities split into two groups.
The per-entity workload must exercise every category the design touches, not just value writes:
mutate a component in place; conditionally `Add` a second component; conditionally `Remove` one;
conditionally `RemoveEntity`; conditionally `NewEntity` and attach components to it. Run it two
ways and compare:

* `RunDisjoint(groups, fn)` on a freshly built world.
* `RunDisjointSerial(groups, fn)` on an identically built world.

Compare with a byte-level digest, not a semantic one. Because the test is white-box in
`package ecs`, it can hash each store's `ids` and `dense` slices directly, plus the sorted liveness
set and `nextID`, walking stores in sorted type-name order. Case 4 of the determinism argument is
what licenses comparing physical layout rather than logical content, and hashing layout is what
turns "probably equivalent" into a real assertion.

**Repetition, because one passing run proves little about a race.** Run the parallel side many
times (order 50) and assert an identical digest every time. Vary `GOMAXPROCS` across the runs,
including 1. A determinism bug that only shows under a particular interleaving will not survive
this, whereas a single comparison could easily miss it.

**Targeted tests** for the parts the equivalence test would only cover incidentally:

* Identifier blocks: assert the exact identifiers a group allocates, and `nextID` after the region,
  against the formula. Include block exhaustion, with a small `stride` so it is reachable.
* Precondition errors: overlapping groups, a dead entity in a group, non-zero depth on entry, a
  nested region. Each returns an error and leaves the world untouched.
* Precondition panics: creating a store inside a region, calling `Flush` inside a region.
* A group panicking: the panic surfaces on the calling goroutine, and with two groups panicking,
  it is the lower-numbered group's panic.
* Empty groups, a single group, and an empty `groups` slice.
* Scope liveness: a scope sees its own in-region spawn as alive; the world does not until the
  barrier.

**Benchmarks:** the sequential ones above for the no-regression gate, plus a parallel benchmark at
1, 2, 4, and 8 groups. The parallel benchmark is not a gate — it measures whether the whole exercise
buys anything, which is a question the post-optimisation measurement should answer before any of
this is built.

## Rejected alternatives

**A. Chunked or paged storage with stable addresses.** Replace `dense []C` with fixed-capacity
chunks that are allocated once and never resized, making a component's address stable for as long
as it occupies a slot. This is the direct answer to reading 2 of the requirement.

Rejected for now. It does not, on its own, solve the registry map races, the command-order
determinism, or the identifier-allocation determinism — the three hardest parts — so it is an
addition to this design's work rather than a replacement for it. It forces tombstone removal,
because swap-and-pop is exactly the "relocate a bystander" bug, and tombstones mean iteration skips
holes, density decays, and compaction becomes a new thing that must run at a barrier. Every one of
those costs is paid on the single-threaded path, which is the common case and which acceptance
criterion 3 protects. Keep it available: it is the escalation if reading 2 turns out to be
required, and the design doc already records archetype-or-other storage as an internal-only change.

**B. A global lock around the world.** Trivially correct, and explicitly excluded by the
requirement ("safe without a global lock"). It also would not deliver determinism, since lock
acquisition order is scheduling-dependent.

**C. Per-goroutine shadow worlds merged at the barrier.** Each group gets a private `World`
containing copies of its entities, merged on completion. Rejected: copying components in and out
each region costs more than the parallel work for the entity counts in play, cross-group reads
become silently stale rather than loudly forbidden, and the merge has to reconcile two identifier
spaces. Quiescence gets the same isolation with no copy.

**D. Bevy-style scheduling on disjoint component types.** Run systems in parallel when their
mutable component-type access does not overlap. This is the model the existing design doc's
decision 8 gestures at, and it is a good model — for a different problem. It does not address this
requirement at all: two spatial partitions both want `Position` mutably. Type-disjointness and
entity-disjointness are orthogonal, and the consumer asked for the latter. They compose later if
both are wanted.

**E. Handle-based access instead of interior pointers on the concurrent path.** Return an opaque
handle that re-resolves through the sparse set on each access, so nothing can dangle. Rejected: it
pays a lookup per field access, it is a second access idiom for consumers to learn and to get wrong,
and quiescence makes interior pointers safe without any of that. Re-resolution also would not fix
the value-write race it appears to address, since two goroutines writing the same component through
handles still race.

## Deferred, not rejected

**Dense-range partitioning.** Because storage is frozen during a region, a store's dense array can
be split into *n* contiguous index ranges, one per goroutine. Disjointness then holds by
construction rather than by check, iteration stays cache-perfect with no sparse-set lookup, and the
entry cost drops to nothing. It suits the homogeneous case ("advance every entity's physics in
parallel"); it cannot express a consumer-chosen spatial partition, which is what #14 asks for. It is
purely additive — a sibling entry point over the same region machinery — and worth revisiting if
measurement shows the per-entity lookup in `Bind(s).All()` is what costs.

**Composition with type-disjoint scheduling** (alternative D), if a systems scheduler ever lands.

**Nested regions.** Rejected on entry today. A group subdividing its own entities further is
conceivable and the block-allocation scheme would need a second level. No consumer wants it yet.

## Sequencing

1. Land and measure trancecode/vantage#15, #16, #17 and lockstep's in-flight-effect indexing.
2. Re-measure where the ceiling is. If single-threaded work still dominates, stop here.
3. Agree this design, in particular the reading-1-versus-reading-2 question above.
4. Only then write the implementation plan and build it.

## Open questions for agreement

* **Reading 1 versus reading 2** (barrier-visible versus immediately-visible structural changes).
  The single most consequential question; reading 2 invalidates most of this document.
* **`stride`**: a package constant, a `World` field, or a per-region argument? A per-region argument
  keeps identifier space tightest but adds a parameter callers must reason about.
* Is the sparser identifier space acceptable to the consumers' savegame and replay formats?
* Is `Bind` the right shape, against the alternative of scope-taking method variants
  (`h.AddIn(s, id, a)`)? `Bind` keeps the method set uniform; the variants make the scope visible at
  every call site.
* Does either consumer need cross-group **reads** inside a region? This design forbids them. If they
  are needed, the answer is probably a read-only snapshot taken at region entry, which is a
  significant addition.
