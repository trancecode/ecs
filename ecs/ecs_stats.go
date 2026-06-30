package ecs

import (
	"fmt"
	"sort"
	"strings"
)

// Stats is a point-in-time snapshot of a World's size and activity, for
// observability.
//
// The size fields (Entities, Stores, Components, PendingCommands) are computed
// on demand when Stats is called, so taking a snapshot adds nothing to the read
// or iterate paths. Flushes and DeferredOps are cumulative counters maintained
// only on the cold structural paths (flush and enqueue).
//
// Stats deliberately carries no timing. Per-operation timing of nanosecond-scale
// lookups costs more than the operations it measures and swamps the signal; use
// the package benchmarks to measure operation cost instead. Consumers map these
// numbers to their own metrics system (Prometheus, OpenTelemetry, logs); the
// library takes no such dependency.
type Stats struct {
	Entities        int            // live entities
	Stores          int            // component types in use
	Components      map[string]int // component type name -> live count
	PendingCommands int            // deferred commands awaiting flush
	Flushes         uint64         // flushes that applied at least one command
	DeferredOps     uint64         // structural changes that took the deferred path
}

// Stats returns a snapshot of the world's current size and cumulative activity.
func (w *World) Stats() Stats {
	components := make(map[string]int, len(w.stores))
	for t, s := range w.stores {
		components[t.String()] = s.count()
	}
	w.mu.Lock()
	pending := len(w.commands)
	w.mu.Unlock()
	return Stats{
		Entities:        len(w.alive),
		Stores:          len(w.stores),
		Components:      components,
		PendingCommands: pending,
		Flushes:         w.flushes.Load(),
		DeferredOps:     w.deferredOps.Load(),
	}
}

// String returns a human-readable one-line summary of the world: the live entity
// count followed by each component type and its live count, ordered by type name.
// The component inventory is derived generically from the registered stores via
// Stats, so World satisfies fmt.Stringer with no per-type maintenance.
func (w *World) String() string {
	stats := w.Stats()
	names := make([]string, 0, len(stats.Components))
	for name := range stats.Components {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "World(entities=%d", stats.Entities)
	for _, name := range names {
		fmt.Fprintf(&b, ", %s=%d", name, stats.Components[name])
	}
	b.WriteByte(')')
	return b.String()
}
