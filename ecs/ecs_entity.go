package ecs

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"strconv"
)

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

// Compare defines a total order over entity identifiers, consistent with
// allocation order: an earlier-allocated entity sorts before a later one. It
// returns a negative value, zero, or a positive value as e sorts before, equal
// to, or after other. The zero (invalid) identifier sorts before every
// allocated one. Comparison does not expose the representation the way an
// integer conversion would, so it preserves the type's opacity.
func (e EntityId) Compare(other EntityId) int {
	return cmp.Compare(e.v, other.v)
}

// MarshalBinary encodes the identifier as 8 big-endian bytes. It implements
// encoding.BinaryMarshaler so entity ids can be persisted in savegames. The
// encoding is stable.
func (e EntityId) MarshalBinary() ([]byte, error) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, e.v)
	return b, nil
}

// UnmarshalBinary decodes 8 big-endian bytes produced by MarshalBinary. It
// returns an error if data is not exactly 8 bytes. It is used only when
// restoring a saved world.
func (e *EntityId) UnmarshalBinary(data []byte) error {
	if len(data) != 8 {
		return fmt.Errorf("ecs.EntityId.UnmarshalBinary: expected 8 bytes, got %d", len(data))
	}
	e.v = binary.BigEndian.Uint64(data)
	return nil
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
