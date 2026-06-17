package ecs

import (
	"iter"
	"testing"
)

// depthTrackedRange mirrors the real iterator pattern (Accessor.All): it brackets
// the loop with beginIteration and a deferred endIteration, so it exercises the
// exact depth-detection mechanism the framework relies on, independent of the
// component handles. n is how many times it yields.
func depthTrackedRange(w *World, n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		w.beginIteration()
		defer w.endIteration()
		for i := 0; i < n; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

func TestDepthIsZeroOutsideIteration(t *testing.T) {
	w := NewWorld()
	if d := w.depth.Load(); d != 0 {
		t.Fatalf("initial depth = %d, want 0", d)
	}
}

func TestDepthIsOneDuringIteration(t *testing.T) {
	w := NewWorld()
	for range depthTrackedRange(w, 3) {
		if d := w.depth.Load(); d != 1 {
			t.Fatalf("depth during iteration = %d, want 1", d)
		}
	}
}

func TestDepthReturnsToZeroAfterNormalLoop(t *testing.T) {
	w := NewWorld()
	for range depthTrackedRange(w, 3) {
	}
	if d := w.depth.Load(); d != 0 {
		t.Fatalf("depth after loop = %d, want 0", d)
	}
}

// Critical: defer must fire on early break so depth is restored.
func TestDepthReturnsToZeroAfterBreak(t *testing.T) {
	w := NewWorld()
	for range depthTrackedRange(w, 10) {
		break
	}
	if d := w.depth.Load(); d != 0 {
		t.Fatalf("depth after break = %d, want 0", d)
	}
}

// Critical: defer must fire while a panic unwinds so depth is restored.
func TestDepthReturnsToZeroAfterPanic(t *testing.T) {
	w := NewWorld()
	func() {
		defer func() { _ = recover() }()
		for range depthTrackedRange(w, 10) {
			panic("boom")
		}
	}()
	if d := w.depth.Load(); d != 0 {
		t.Fatalf("depth after recovered panic = %d, want 0", d)
	}
}

func TestDepthNestsAndUnwinds(t *testing.T) {
	w := NewWorld()
	for range depthTrackedRange(w, 1) {
		if d := w.depth.Load(); d != 1 {
			t.Fatalf("outer depth = %d, want 1", d)
		}
		for range depthTrackedRange(w, 1) {
			if d := w.depth.Load(); d != 2 {
				t.Fatalf("nested depth = %d, want 2", d)
			}
		}
		if d := w.depth.Load(); d != 1 {
			t.Fatalf("depth after nested loop = %d, want 1", d)
		}
	}
	if d := w.depth.Load(); d != 0 {
		t.Fatalf("final depth = %d, want 0", d)
	}
}

func TestDeferredCommandRunsOnUnwindToZero(t *testing.T) {
	w := NewWorld()
	ran := false
	for range depthTrackedRange(w, 1) {
		w.enqueue(func() { ran = true })
		if ran {
			t.Fatal("command must not run during the iteration")
		}
	}
	if !ran {
		t.Fatal("command must run when depth unwinds to 0")
	}
}

// Critical: a change queued before an early break must still flush, because
// endIteration fires on break.
func TestDeferredCommandFlushesEvenAfterBreak(t *testing.T) {
	w := NewWorld()
	ran := false
	for range depthTrackedRange(w, 10) {
		w.enqueue(func() { ran = true })
		break
	}
	if !ran {
		t.Fatal("deferred command must flush even when the loop breaks early")
	}
}

func TestNestedIterationDoesNotFlushEarly(t *testing.T) {
	w := NewWorld()
	ran := false
	for range depthTrackedRange(w, 1) { // depth 0->1
		w.enqueue(func() { ran = true })
		for range depthTrackedRange(w, 1) { // depth 1->2->1, no flush
			if ran {
				t.Fatal("nested iteration must not flush")
			}
		}
		if ran {
			t.Fatal("command must not run while outer iteration is active")
		}
	} // depth 1->0, flush
	if !ran {
		t.Fatal("command must run after the outermost iteration ends")
	}
}

func TestCommandsRunInFifoOrder(t *testing.T) {
	w := NewWorld()
	var order []int
	for range depthTrackedRange(w, 1) {
		w.enqueue(func() { order = append(order, 1) })
		w.enqueue(func() { order = append(order, 2) })
		w.enqueue(func() { order = append(order, 3) })
	}
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("commands ran out of order: %v", order)
	}
}

func TestExplicitFlushAtDepthZero(t *testing.T) {
	w := NewWorld()
	ran := false
	w.enqueue(func() { ran = true }) // queued outside any iteration
	if ran {
		t.Fatal("enqueue must not run on its own")
	}
	w.Flush()
	if !ran {
		t.Fatal("explicit Flush must run queued commands")
	}
}
