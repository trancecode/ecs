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
