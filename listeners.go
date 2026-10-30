package listeners

import (
	"slices"
	"sync"
	"sync/atomic"
)

// PluginResult represents the result of a callback execution.
// It is used to control further processing of listeners.
type PluginResult int32

const (
	Continue PluginResult = 0 // Continue execution without any changes.
	Changed  PluginResult = 1 // State or behavior has been modified.
	Handled  PluginResult = 2 // Event has been handled, no further actions are required.
	Stop     PluginResult = 3 // Stop processing, no further steps are executed.
)

type HookMode int32

const (
	Pre  HookMode = 0
	Post HookMode = 1
)

type listenerHolder[T any] struct {
	id       ListenerID
	callback T
}

// ListenerID is a unique identifier of a registered listener.
// IDs start at 1, so 0 can be used as "no listener".
type ListenerID int32

// ListenerManager manages a set of listeners with execution order and supports pre- and post-invocation phases.
type ListenerManager[T any] struct {
	pre  atomic.Pointer[[]listenerHolder[T]]
	post atomic.Pointer[[]listenerHolder[T]]
	id   ListenerID
	mu   sync.Mutex
}

// NewListener creates and initializes a new ListenerManager instance.
func NewListener[T any]() *ListenerManager[T] {
	return &ListenerManager[T]{}
}

// Add registers a new listener with the specified hook mode.
//
// Returns a unique ListenerID that can be used to remove the listener later.
func (lm *ListenerManager[T]) Add(callback T, mode HookMode) ListenerID {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.id++
	list := lm.list(mode)
	// Clip forces append to copy, so readers of the old slice are not affected.
	holders := append(slices.Clip(load(list)), listenerHolder[T]{lm.id, callback})
	list.Store(&holders)

	return lm.id
}

// Remove unregisters a listener by its ListenerID.
// If the listener does not exist, the call has no effect.
func (lm *ListenerManager[T]) Remove(index ListenerID) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	for _, list := range [...]*atomic.Pointer[[]listenerHolder[T]]{&lm.pre, &lm.post} {
		old := load(list)
		i := slices.IndexFunc(old, func(holder listenerHolder[T]) bool {
			return holder.id == index
		})
		if i >= 0 {
			holders := slices.Delete(slices.Clone(old), i, i+1)
			list.Store(&holders)
			return
		}
	}
}

// InvokePre invokes all listeners registered with Pre hook mode in the order they were added.
//
// The invokeFunc is called for each listener and returns a PluginResult.
// The highest PluginResult is propagated as the final result.
// If the result reaches Handled or Stop, further invocation is stopped.
func (lm *ListenerManager[T]) InvokePre(invokeFunc func(T) PluginResult) PluginResult {
	finalResult := Continue
	for _, holder := range load(&lm.pre) {
		result := invokeFunc(holder.callback)

		if result > finalResult {
			finalResult = result
		}

		if finalResult >= Handled {
			break
		}
	}

	return finalResult
}

// InvokePost invokes all listeners registered with Post hook mode in the order they were added.
//
// Post listeners do not affect the execution flow and their results are ignored.
func (lm *ListenerManager[T]) InvokePost(invokeFunc func(T)) {
	for _, holder := range load(&lm.post) {
		invokeFunc(holder.callback)
	}
}

func (lm *ListenerManager[T]) list(mode HookMode) *atomic.Pointer[[]listenerHolder[T]] {
	if mode == Pre {
		return &lm.pre
	}
	return &lm.post
}

// load returns the current listener slice; it must not be modified.
func load[T any](list *atomic.Pointer[[]listenerHolder[T]]) []listenerHolder[T] {
	if holders := list.Load(); holders != nil {
		return *holders
	}
	return nil
}
