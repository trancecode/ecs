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
