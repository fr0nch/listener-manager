package listeners

import (
	"slices"
	"sync"
)

// PluginResult represents the result of a callback execution.
// It is used to control further processing of listeners.
type PluginResult = int32

const (
	Continue PluginResult = 0 // Continue execution without any changes.
	Changed  PluginResult = 1 // State or behavior has been modified.
	Handled  PluginResult = 2 // Event has been handled, no further actions are required.
	Stop     PluginResult = 3 // Stop processing, no further steps are executed.
)

type HookMode = int32

const (
	Pre  HookMode = 0
	Post HookMode = 1
)

type listenerHolder[T any] struct {
	callback T
	mode     HookMode
}

// ListenerID is a unique identifier of a registered listener.
type ListenerID = int32

// ListenerManager manages a set of listeners with execution order
// and supports pre- and post-invocation phases.
type ListenerManager[T any] struct {
	listeners map[ListenerID]listenerHolder[T]
	order     []ListenerID
	id        ListenerID
	mu        sync.RWMutex
}

// NewListener creates and initializes a new ListenerManager instance.
func NewListener[T any]() *ListenerManager[T] {
	return &ListenerManager[T]{
		listeners: make(map[ListenerID]listenerHolder[T]),
	}
}

// Add registers a new listener with the specified hook mode.
//
// Returns a unique ListenerID that can be used to remove the listener later.
func (cm *ListenerManager[T]) Add(callback T, mode HookMode) ListenerID {
	cm.mu.Lock()

	id := cm.id
	cm.listeners[id] = listenerHolder[T]{
		callback,
		mode,
	}

	cm.order = append(cm.order, id)
	cm.id++

	cm.mu.Unlock()

	return id
}

// Remove unregisters a listener by its ListenerID.
// If the listener does not exist, the call has no effect.
func (cm *ListenerManager[T]) Remove(index ListenerID) {
	cm.mu.Lock()

	delete(cm.listeners, index)
	cm.order = slices.DeleteFunc(cm.order, func(i ListenerID) bool {
		return cm.order[i] == index
	})

	cm.mu.Unlock()
}

// InvokePre invokes all listeners registered with Pre hook mode in the order they were added.
//
// The invokeFunc is called for each listener and returns a PluginResult.
// The highest PluginResult is propagated as the final result.
// If the result reaches Handled or Stop, further invocation is stopped.
func (cm *ListenerManager[T]) InvokePre(invokeFunc func(T) PluginResult) PluginResult {
	cm.mu.RLock()

	finalResult := Continue
	for _, idx := range cm.order {
		holder := cm.listeners[idx]
		if holder.mode == Pre {
			result := invokeFunc(holder.callback)

			if result > finalResult {
				finalResult = result
			}

			if finalResult >= Handled {
				break
			}
		}
	}

	cm.mu.RUnlock()

	return finalResult

}

// InvokePost invokes all listeners registered with Post hook mode
// in the order they were added.
//
// Post listeners do not affect the execution flow and their results
// are ignored.
func (cm *ListenerManager[T]) InvokePost(invokeFunc func(T)) {
	cm.mu.RLock()

	for _, idx := range cm.order {
		holder := cm.listeners[idx]
		if holder.mode == Post {
			invokeFunc(holder.callback)
		}
	}

	cm.mu.RUnlock()
}
