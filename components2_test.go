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

func TestComponents2AllExcludesBOnlyEntity(t *testing.T) {
	w := NewWorld()
	moving := Components2[position, velocity](w)
	vel := Components[velocity](w)

	bOnly := w.NewEntity()
	vel.Add(bOnly, velocity{DX: 7}) // has velocity (B) but not position (A)

	for id := range moving.All() {
		if id == bOnly {
			t.Fatal("entity with only B must be excluded from the join")
		}
	}
}
