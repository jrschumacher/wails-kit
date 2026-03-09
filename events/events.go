package events

import "sync"

// Emitter sends events to the frontend. In a Wails v3 app, wrap
// *application.App with NewEmitter. For tests, use MemoryEmitter.
//
// Emitter supports multi-window apps via window registration and targeted
// emission. Windows are registered with RegisterWindow and events can be
// sent to specific windows with EmitTo, or broadcast to all with Emit.
//
// Optional features can be enabled via EmitterOption functions:
//   - WithHistory: ring buffer for replaying events to late-joining windows
//   - WithDebounce: delay emission until quiet period (broadcast only)
//   - WithThrottle: rate-limit emission (broadcast only)
//   - WithBatching: collect events into batches (broadcast only)
type Emitter struct {
	backend   Backend
	windows   map[string]Backend
	handlers  []*handler
	history   *history
	debounces map[string]*debouncer
	throttles map[string]*throttler
	batchers  map[string]*batcher
	mu        sync.RWMutex
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
func NewEmitter(backend Backend, opts ...EmitterOption) *Emitter {
	e := &Emitter{
		backend: backend,
		windows: make(map[string]Backend),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// rawEmit sends the event through the backend, records history, and
// notifies handlers. This is the final step after middleware processing.
func (e *Emitter) rawEmit(name string, data any) {
	e.backend.Emit(name, data)
	if e.history != nil {
		e.history.record(Record{Name: name, Data: data})
	}
	e.notify(name, data)
}

// Emit broadcasts a named event with a typed payload to all windows
// via the default backend. Middleware (debounce, throttle, batch) is
// applied if configured for this event name. Registered handlers are
// also notified.
func (e *Emitter) Emit(name string, data any) {
	if d, ok := e.debounces[name]; ok {
		d.push(name, data, e.rawEmit)
		return
	}
	if t, ok := e.throttles[name]; ok {
		if t.allow() {
			e.rawEmit(name, data)
		}
		return
	}
	if b, ok := e.batchers[name]; ok {
		b.add(name, data, e.rawEmit)
		return
	}
	e.rawEmit(name, data)
}

// EmitTo sends a named event to a specific registered window.
// If the window ID is not registered, the event is silently dropped.
// Registered handlers are notified regardless. Middleware is not applied
// to targeted emissions.
func (e *Emitter) EmitTo(windowID string, name string, data any) {
	e.mu.RLock()
	w, ok := e.windows[windowID]
	e.mu.RUnlock()
	if ok {
		w.Emit(name, data)
	}
	if e.history != nil {
		e.history.record(Record{Name: name, Data: data, WindowID: windowID})
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

// Replay sends the most recent event of the given name to the specified
// window. If history is not enabled or no matching event exists, this is
// a no-op.
func (e *Emitter) Replay(windowID string, eventName string) {
	if e.history == nil {
		return
	}
	r := e.history.last(eventName)
	if r == nil {
		return
	}
	e.mu.RLock()
	w, ok := e.windows[windowID]
	e.mu.RUnlock()
	if ok {
		w.Emit(r.Name, r.Data)
	}
}

// ReplayAll sends the latest event of each distinct name to the specified
// window. Useful for initializing a new window with current state.
func (e *Emitter) ReplayAll(windowID string) {
	if e.history == nil {
		return
	}
	e.mu.RLock()
	w, ok := e.windows[windowID]
	e.mu.RUnlock()
	if !ok {
		return
	}
	for _, r := range e.history.latest() {
		w.Emit(r.Name, r.Data)
	}
}

// Close flushes any pending middleware events (debounced events are emitted,
// batched events are flushed). Call this when shutting down the application.
func (e *Emitter) Close() {
	for _, d := range e.debounces {
		d.flush()
	}
	for _, b := range e.batchers {
		b.flush()
	}
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
