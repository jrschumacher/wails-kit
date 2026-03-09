package events

import "sync"

// Emitter sends events to the frontend. In a Wails v3 app, wrap
// *application.App with NewEmitter. For tests, use MemoryEmitter.
//
// Emitter supports multi-window apps via window registration and targeted
// emission. Windows are registered with RegisterWindow and events can be
// sent to specific windows with EmitTo, or broadcast to all with Emit.
type Emitter struct {
	backend  Backend
	windows  map[string]Backend
	handlers []*handler
	mu       sync.RWMutex
}

// Backend is the interface for the underlying event emission mechanism.
// Wails v3 apps implement this by wrapping app.Event.Emit.
type Backend interface {
	Emit(name string, data any)
}

// BackendFunc adapts a plain function to the Backend interface.
type BackendFunc func(name string, data any)

func (f BackendFunc) Emit(name string, data any) { f(name, data) }

// NewEmitter creates an Emitter backed by the given Backend.
func NewEmitter(backend Backend) *Emitter {
	return &Emitter{
		backend: backend,
		windows: make(map[string]Backend),
	}
}

// Emit broadcasts a named event with a typed payload to all windows
// via the default backend. Registered handlers are also notified.
func (e *Emitter) Emit(name string, data any) {
	e.backend.Emit(name, data)
	e.notify(name, data)
}

// EmitTo sends a named event to a specific registered window.
// If the window ID is not registered, the event is silently dropped.
// Registered handlers are notified regardless.
func (e *Emitter) EmitTo(windowID string, name string, data any) {
	e.mu.RLock()
	w, ok := e.windows[windowID]
	e.mu.RUnlock()
	if ok {
		w.Emit(name, data)
	}
	e.notify(name, data)
}

// RegisterWindow adds a window backend that can receive targeted events.
func (e *Emitter) RegisterWindow(id string, backend Backend) {
	e.mu.Lock()
	e.windows[id] = backend
	e.mu.Unlock()
}

// UnregisterWindow removes a previously registered window.
func (e *Emitter) UnregisterWindow(id string) {
	e.mu.Lock()
	delete(e.windows, id)
	e.mu.Unlock()
}

// Subscription represents a cancellable event subscription.
type Subscription struct {
	cancel func()
}

// Cancel unsubscribes the handler.
func (s *Subscription) Cancel() {
	s.cancel()
}

// handler is an internal wrapper for a subscription callback.
type handler struct {
	name string
	fn   func(any)
}

// On registers a type-safe event handler on the emitter and returns a
// Subscription that can be cancelled. The handler is called on the
// emitting goroutine whenever Emit or EmitTo fires a matching event name.
func On[T any](e *Emitter, name string, fn func(T)) *Subscription {
	h := &handler{
		name: name,
		fn: func(data any) {
			if typed, ok := data.(T); ok {
				fn(typed)
			}
		},
	}

	e.mu.Lock()
	e.handlers = append(e.handlers, h)
	e.mu.Unlock()

	return &Subscription{
		cancel: func() {
			e.mu.Lock()
			for i, existing := range e.handlers {
				if existing == h {
					e.handlers = append(e.handlers[:i], e.handlers[i+1:]...)
					break
				}
			}
			e.mu.Unlock()
		},
	}
}

// notify dispatches to all matching handlers. Copies the handler slice
// under read lock so callbacks run without holding the lock.
func (e *Emitter) notify(name string, data any) {
	e.mu.RLock()
	if len(e.handlers) == 0 {
		e.mu.RUnlock()
		return
	}
	snapshot := make([]*handler, len(e.handlers))
	copy(snapshot, e.handlers)
	e.mu.RUnlock()

	for _, h := range snapshot {
		if h.name == name {
			h.fn(data)
		}
	}
}

// Common event names for kit-level events.
const (
	SettingsChanged = "settings:changed"
)

// SettingsChangedPayload is the payload for SettingsChanged events.
type SettingsChangedPayload struct {
	Keys []string `json:"keys"`
}
