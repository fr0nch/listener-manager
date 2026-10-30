package listeners

import (
	"fmt"
	"slices"
	"sync"
	"testing"
)

func collectPost(lm *ListenerManager[func() int]) []int {
	var got []int
	lm.InvokePost(func(callback func() int) {
		got = append(got, callback())
	})
	return got
}

func TestAddStartsAtOne(t *testing.T) {
	lm := NewListener[func() int]()

	if id := lm.Add(func() int { return 1 }, Post); id != 1 {
		t.Fatalf("first id = %d; want 1", id)
	}
	if id := lm.Add(func() int { return 2 }, Post); id != 2 {
		t.Fatalf("second id = %d; want 2", id)
	}
}

// Regression: Remove indexed order by id and panicked with index out of range.
func TestRemove(t *testing.T) {
	lm := NewListener[func() int]()

	a := lm.Add(func() int { return 1 }, Post)
	b := lm.Add(func() int { return 2 }, Post)
	c := lm.Add(func() int { return 3 }, Post)

	lm.Remove(a)
	lm.Remove(c)
	if got := collectPost(lm); !slices.Equal(got, []int{2}) {
		t.Fatalf("after removing a, c: %v; want [2]", got)
	}

	lm.Remove(b)
	lm.Remove(b) // again: no effect
	lm.Remove(0) // nonexistent id
	if got := collectPost(lm); len(got) != 0 {
		t.Fatalf("after removing all: %v; want []", got)
	}
}

func TestInvokeOrderAndModes(t *testing.T) {
	lm := NewListener[func() PluginResult]()

	var calls []string
	lm.Add(func() PluginResult { calls = append(calls, "pre1"); return Changed }, Pre)
	lm.Add(func() PluginResult { calls = append(calls, "post"); return Continue }, Post)
	lm.Add(func() PluginResult { calls = append(calls, "pre2"); return Handled }, Pre)
	lm.Add(func() PluginResult { calls = append(calls, "pre3"); return Continue }, Pre)

	result := lm.InvokePre(func(callback func() PluginResult) PluginResult { return callback() })
	if result != Handled || !slices.Equal(calls, []string{"pre1", "pre2"}) {
		t.Fatalf("InvokePre = %d, calls %v; want Handled, [pre1 pre2]", result, calls)
	}

	calls = nil
	lm.InvokePost(func(callback func() PluginResult) { callback() })
	if !slices.Equal(calls, []string{"post"}) {
		t.Fatalf("InvokePost calls %v; want [post]", calls)
	}
}

// A callback may add and remove listeners during invocation; this used to deadlock.
func TestModifyInsideCallback(t *testing.T) {
	lm := NewListener[func()]()

	var self ListenerID
	calls := 0
	self = lm.Add(func() {
		calls++
		lm.Remove(self)
		lm.Add(func() { calls += 10 }, Post)
	}, Post)

	lm.InvokePost(func(callback func()) { callback() })
	if calls != 1 {
		t.Fatalf("first invoke: calls = %d; want 1 (added listener runs next time)", calls)
	}

	lm.InvokePost(func(callback func()) { callback() })
	if calls != 11 {
		t.Fatalf("second invoke: calls = %d; want 11", calls)
	}
}

// A panicking callback must not leave the manager locked.
func TestPanicDoesNotKeepLock(t *testing.T) {
	lm := NewListener[func()]()
	lm.Add(func() { panic("boom") }, Post)

	func() {
		defer func() { _ = recover() }()
		lm.InvokePost(func(callback func()) { callback() })
	}()

	lm.Add(func() {}, Post) // would hang on Lock
}

// Concurrent Invoke/Add/Remove, meant to be run with -race.
func TestConcurrent(t *testing.T) {
	lm := NewListener[func()]()
	lm.Add(func() {}, Pre)

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 1000 {
				lm.InvokePre(func(callback func()) PluginResult { callback(); return Continue })
			}
		})
		wg.Go(func() {
			for range 1000 {
				lm.Remove(lm.Add(func() {}, Pre))
			}
		})
	}
	wg.Wait()
}

func BenchmarkInvokePre(b *testing.B) {
	for _, n := range []int{1, 8, 64} {
		lm := NewListener[func() PluginResult]()
		for range n {
			lm.Add(func() PluginResult { return Continue }, Pre)
			lm.Add(func() PluginResult { return Continue }, Post)
		}
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				lm.InvokePre(func(callback func() PluginResult) PluginResult { return callback() })
			}
		})
	}
}
