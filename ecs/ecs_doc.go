// Package ecs is an Entity Component System for Go.
//
// Entities are opaque identifiers. Components are plain Go structs stored in
// per-type sparse sets. Typed handles (Components, Components2) read and mutate
// components through interior pointers. Structural changes (adding or removing
// components, creating or destroying entities) apply immediately when no
// iteration is in progress and are deferred to a flush when one is, so pointers
// obtained during iteration stay valid for that iteration.
//
// For savegames, a world can be reconstructed exactly: EntityCounter and
// RestoreEntityCounter round-trip the allocation counter, RestoreEntity marks a
// saved identifier alive again, and EntityId marshals to a stable binary form.
// Component encoding stays with the caller, who iterates each Accessor to save
// and calls Accessor.Add to load.
package ecs
