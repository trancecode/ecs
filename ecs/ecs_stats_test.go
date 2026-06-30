package ecs

import (
	"fmt"
	"reflect"
	"testing"
)

func TestStatsCountsEntitiesStoresComponents(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	vel := Components[velocity](w)
	a := w.NewEntity()
	b := w.NewEntity()
	pos.Add(a, position{})
	pos.Add(b, position{})
	vel.Add(a, velocity{})

	s := w.Stats()
	if s.Entities != 2 {
		t.Fatalf("Entities = %d, want 2", s.Entities)
	}
	if s.Stores != 2 {
		t.Fatalf("Stores = %d, want 2", s.Stores)
	}
	posName := reflect.TypeFor[position]().String()
	velName := reflect.TypeFor[velocity]().String()
	if s.Components[posName] != 2 {
		t.Fatalf("position count = %d, want 2 (map=%v)", s.Components[posName], s.Components)
	}
	if s.Components[velName] != 1 {
		t.Fatalf("velocity count = %d, want 1 (map=%v)", s.Components[velName], s.Components)
	}
}

func TestStatsCountsDeferredOpsAndFlushes(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	pos.Add(a, position{}) // immediate at depth 0, not deferred

	if s := w.Stats(); s.DeferredOps != 0 || s.Flushes != 0 {
		t.Fatalf("before iteration: DeferredOps=%d Flushes=%d, want 0 0", s.DeferredOps, s.Flushes)
	}

	b := w.NewEntity()
	for range pos.All() {
		pos.Add(b, position{}) // deferred during iteration
	}
	s := w.Stats()
	if s.DeferredOps != 1 {
		t.Fatalf("DeferredOps = %d, want 1", s.DeferredOps)
	}
	if s.Flushes != 1 {
		t.Fatalf("Flushes = %d, want 1", s.Flushes)
	}
}

func TestStatsPendingCommands(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	a := w.NewEntity()
	pos.Add(a, position{})
	b := w.NewEntity()

	for range pos.All() {
		pos.Add(b, position{}) // deferred; sits in the buffer until the loop ends
		if s := w.Stats(); s.PendingCommands != 1 {
			t.Fatalf("PendingCommands during iteration = %d, want 1", s.PendingCommands)
		}
	}
	if s := w.Stats(); s.PendingCommands != 0 {
		t.Fatalf("PendingCommands after flush = %d, want 0", s.PendingCommands)
	}
}

func TestWorldStringListsComponentsSortedByName(t *testing.T) {
	w := NewWorld()
	pos := Components[position](w)
	vel := Components[velocity](w)
	a := w.NewEntity()
	b := w.NewEntity()
	pos.Add(a, position{})
	pos.Add(b, position{})
	vel.Add(a, velocity{})

	posName := reflect.TypeFor[position]().String()
	velName := reflect.TypeFor[velocity]().String()
	// Components are listed by sorted type name; "ecs.position" sorts before
	// "ecs.velocity".
	want := fmt.Sprintf("World(entities=2, %s=2, %s=1)", posName, velName)
	if got := w.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestWorldStringEmpty(t *testing.T) {
	if got := NewWorld().String(); got != "World(entities=0)" {
		t.Fatalf("String() = %q, want %q", got, "World(entities=0)")
	}
}
