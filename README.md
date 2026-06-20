# ecs

A small, dependency-free Entity Component System (ECS) for Go.

Entities are opaque identifiers. Components are plain structs stored in per-type
sparse sets. Typed handles read and mutate components through interior pointers.
Structural changes (adding or removing components, creating or destroying entities)
apply immediately outside iteration and are **deferred to an automatic flush during
iteration**, so the pointers you hold stay valid for the whole loop.

Requires Go 1.26. Standard library only.

## Documentation

* API reference (canonical, always current): https://pkg.go.dev/github.com/trancecode/ecs/ecs
* Hosted godoc (GitHub Pages): https://trancecode.github.io/ecs/
* Design and rationale: [`docs/superpowers/specs/2026-06-17-ecs-framework-design.md`](docs/superpowers/specs/2026-06-17-ecs-framework-design.md)

## Install

    go get github.com/trancecode/ecs/ecs

    import "github.com/trancecode/ecs/ecs"

The module is `github.com/trancecode/ecs`; the package lives in the `ecs/` subdirectory,
so the import path ends in `/ecs/ecs`.

## Quick start

```go
package main

import (
	"fmt"

	"github.com/trancecode/ecs/ecs"
)

type Position struct{ X, Y float64 }
type Velocity struct{ DX, DY float64 }

func main() {
	w := ecs.NewWorld()

	// Reusable typed handles. Build them once and keep them (for example as
	// fields of a system); they cache their backing store.
	positions := ecs.Components[Position](w)
	moving := ecs.Components2[Position, Velocity](w)

	e := w.NewEntity()
	positions.Add(e, Position{X: 0})
	moving.Add(e, Position{X: 0}, Velocity{DX: 3})

	// Iterate every entity that has both Position and Velocity.
	for _, c := range moving.All() {
		p, v := c.Values()
		p.X += v.DX // written in place through the interior pointer
	}

	p, _ := positions.Get(e)
	fmt.Println(p.X) // 3
}
```

## Core concepts

* **Entity** — an opaque `EntityId`. Allocate one with `w.NewEntity()`. The zero value is
  invalid; `id.IsValid()` and `id.String()` (`"ent_42"`) are available.
* **Component** — any plain struct. No interface, no embedding, no required id field.
* **Handle** — `Components[A]` (single) and `Components2[A, B]` / `Components3[...]` (joins)
  give `Get`, `Has`, `Add`, and `All`. The single-component handle also has `Remove` and the
  get-or-create pair `GetOrAdd`/`GetOrAddFunc`. Construct once and reuse.
* **Interior pointers** — `Get` and iteration return `*A` pointing into storage. Write through
  them and the change is immediate. A pointer is valid until the next structural change to that
  store; do not retain it across one.
* **Deferred structural changes** — outside any iteration, `Add`/`Remove`/`NewEntity`/
  `RemoveEntity` apply immediately. Inside an `All()` loop they are queued and applied when the
  loop ends (an automatic flush), so your pointers stay valid. Call `w.Flush()` explicitly only
  in the rare case you need pending changes materialized before non-iterating code. This is why a
  "behavior" pass that attaches components, followed by a "movement" pass that reads them, just
  works: the first loop's additions are flushed before the second loop runs.

## API at a glance

```go
type EntityId struct{ /* opaque */ }
func (EntityId) IsValid() bool
func (EntityId) String() string

func NewWorld() *World
func (*World) NewEntity() EntityId
func (*World) RemoveEntity(EntityId)
func (*World) IsAlive(EntityId) bool
func (*World) Flush()
func (*World) Stats() Stats         // observability snapshot

func Components[A any](*World) Accessor[A]
func (Accessor[A]) Get(EntityId) (*A, bool)
func (Accessor[A]) GetOrAdd(EntityId, A) *A
func (Accessor[A]) GetOrAddFunc(EntityId, func() A) *A
func (Accessor[A]) Has(EntityId) bool
func (Accessor[A]) Add(EntityId, A)
func (Accessor[A]) Remove(EntityId)
func (Accessor[A]) All() iter.Seq2[EntityId, *A]

func Components2[A, B any](*World) Accessor2[A, B]
func (Accessor2[A, B]) Get(EntityId) (*A, *B, bool)
func (Accessor2[A, B]) Has(EntityId) bool
func (Accessor2[A, B]) Add(EntityId, A, B)
func (Accessor2[A, B]) All() iter.Seq2[EntityId, Tuple2[A, B]]
func (Tuple2[A, B]) Values() (*A, *B)
```

## Observability

`w.Stats()` returns a cheap snapshot (entity, store, and per-type component counts, pending
deferred commands, and cumulative flush and deferred-op counters) with no effect on the read or
iterate paths. It carries no timing and pulls in no metrics dependency; map the numbers to your
own system (Prometheus, OpenTelemetry, logs). To measure per-operation cost, use the benchmarks
in `ecs/ecs_bench_test.go`.

## Migrating from nrg's ECS

This module is the successor to the ECS embedded in `nrg`. The main differences a consumer hits:

| nrg ECS | this module |
| --- | --- |
| `EntityId` is a bare `uint32` | `EntityId` is an opaque struct; use `IsValid()` instead of `== 0`, never cast to/from an integer |
| Components embed `ecs.BaseEntity` and implement `EntityId()` | Components are plain structs with no embedding and no required methods |
| `component.EntityId()` to get the owner | the id comes from iteration (`for id, c := range h.All()`) or is held alongside |
| One `ComponentStore[*T]` per type, accessed directly | typed handles `Components[T]` / `Components2[A, B]`; the store is internal |
| Multi-component queries are hand-written joins with map lookups | `Components2[A, B].All()` yields `(id, Tuple2)` |
| Component pointers are stable forever | interior pointers are valid only until the next structural change to that store |
| Structural changes apply immediately | immediate outside iteration, deferred-and-auto-flushed during iteration |

Migration is mechanical: swap the id type, drop `BaseEntity` and the `EntityId()` call sites,
replace direct store access with handles, and audit any code that cached a component pointer across
a structural change.

## Versioning

The module follows [semantic versioning](https://semver.org). The version lives in
`ecs/VERSION` (embedded into the binary) and is reported at runtime by `ecs.Version()`;
releases are published as matching `vMAJOR.MINOR.PATCH` git tags. The current version is
`0.1.0` — pre-1.0, so the public API may still change as it stabilizes.

## Stability

Version 1 is the entity-and-component core plus deferred structural changes. Systems scheduling,
events, resources, change detection, and archetype storage are intentionally out of scope and are
recorded as additive future seams in the design doc, so they can be introduced without breaking the
current API. API stability is the project's primary goal.
