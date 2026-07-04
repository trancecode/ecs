package ecs

import (
	"slices"
	"testing"
)

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

func TestEntityIdCompareOrdersByAllocationOrder(t *testing.T) {
	earlier := newEntityId(1)
	later := newEntityId(2)

	if got := earlier.Compare(later); got >= 0 {
		t.Fatalf("earlier.Compare(later) = %d, want negative", got)
	}
	if got := later.Compare(earlier); got <= 0 {
		t.Fatalf("later.Compare(earlier) = %d, want positive", got)
	}
	if got := earlier.Compare(earlier); got != 0 {
		t.Fatalf("earlier.Compare(earlier) = %d, want 0", got)
	}
}

func TestEntityIdCompareInvalidSortsFirst(t *testing.T) {
	var invalid EntityId
	allocated := newEntityId(1)

	if got := invalid.Compare(allocated); got >= 0 {
		t.Fatalf("invalid.Compare(allocated) = %d, want negative", got)
	}
	if got := invalid.Compare(invalid); got != 0 {
		t.Fatalf("invalid.Compare(invalid) = %d, want 0", got)
	}
}

func TestEntityIdCompareSortsDeterministically(t *testing.T) {
	ids := []EntityId{newEntityId(5), newEntityId(1), newEntityId(3), newEntityId(2), newEntityId(4)}
	slices.SortFunc(ids, EntityId.Compare)

	want := []EntityId{newEntityId(1), newEntityId(2), newEntityId(3), newEntityId(4), newEntityId(5)}
	if !slices.Equal(ids, want) {
		t.Fatalf("sorted = %v, want %v", ids, want)
	}
}
