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
