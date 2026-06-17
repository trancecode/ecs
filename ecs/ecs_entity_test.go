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
