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
//   - WithAsync: buffered async delivery to handlers
type Emitter struct {
	backend   Backend
	windows   map[string]Backend
	handlers  []*handler
	history   *history
	debounces map[string]*debouncer
	throttles map[string]*throttler
	batchers  map[string]*batcher
	asyncBuf  int // 0 = sync, >0 = async buffer size per handler
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
// batched events are flushed) and stops async handler goroutines. Call this
// when shutting down the application.
func (e *Emitter) Close() {
	for _, d := range e.debounces {
		d.flush()
	}
	for _, b := range e.batchers {
		b.flush()
	}
	// Stop all async handler goroutines.
	e.mu.RLock()
	snapshot := make([]*handler, len(e.handlers))
	copy(snapshot, e.handlers)
	e.mu.RUnlock()
	for _, h := range snapshot {
		h.stop()
	}
}

// ScopedEmitter emits events within a named scope. Events emitted through
// a ScopedEmitter are prefixed with the scope ID, allowing subscribers to
// listen for events from a specific scope or from all scopes.
type ScopedEmitter struct {
	parent *Emitter
	scope  string
}

// Scope returns a ScopedEmitter that prefixes all emitted event names
// with the given scope ID. Scoped events use the wire format "@scope/name".
//
// Subscribers registered with OnScoped for a matching scope receive scoped
// events. Subscribers registered with On (unscoped) also receive scoped
// events by matching on the base event name.
func (e *Emitter) Scope(id string) *ScopedEmitter {
	return &ScopedEmitter{parent: e, scope: id}
}

// Emit broadcasts a scoped event via the parent emitter.
func (s *ScopedEmitter) Emit(name string, data any) {
	s.parent.Emit(ScopedName(s.scope, name), data)
}

// EmitTo sends a scoped event to a specific window via the parent emitter.
func (s *ScopedEmitter) EmitTo(windowID, name string, data any) {
	s.parent.EmitTo(windowID, ScopedName(s.scope, name), data)
}

// ScopedName returns the wire-format name for a scoped event: "@scope/name".
// If scope is empty, the name is returned unchanged.
func ScopedName(scope, name string) string {
	if scope == "" {
		return name
	}
	return "@" + scope + "/" + name
}

// ParseScopedName extracts the scope and base name from a wire-format event
// name. For unscoped events, scope is empty and name is returned as-is.
func ParseScopedName(wireName string) (scope, name string) {
	if len(wireName) > 1 && wireName[0] == '@' {
		for i := 1; i < len(wireName); i++ {
			if wireName[i] == '/' {
				return wireName[1:i], wireName[i+1:]
			}
		}
	}
	return "", wireName
}

// handler is an internal wrapper for a subscription callback.
type handler struct {
	scope     string // non-empty for scoped subscriptions
	name      string
	fn        func(any)
	ch        chan any      // non-nil for async handlers
	done      chan struct{} // closed to signal async goroutine exit
	closeOnce sync.Once
}

// deliver sends data to the handler, either synchronously or via channel.
func (h *handler) deliver(data any) {
	if h.ch != nil {
		select {
		case h.ch <- data:
		default:
			// Buffer full — drop event to avoid blocking the emitter.
		}
	} else {
		h.fn(data)
	}
}

// stop signals the async goroutine to exit. Safe to call multiple times.
func (h *handler) stop() {
	if h.done != nil {
		h.closeOnce.Do(func() { close(h.done) })
	}
}

// registerHandler adds a handler to the emitter and returns an unsubscribe
// function. If async mode is enabled, starts a goroutine for the handler.
func registerHandler(e *Emitter, h *handler) func() {
	if e.asyncBuf > 0 {
		h.ch = make(chan any, e.asyncBuf)
		h.done = make(chan struct{})
		go func() {
			for {
				select {
				case data := <-h.ch:
					h.fn(data)
				case <-h.done:
					return
				}
			}
		}()
	}

	e.mu.Lock()
	e.handlers = append(e.handlers, h)
	e.mu.Unlock()

	return func() {
		e.mu.Lock()
		for i, existing := range e.handlers {
			if existing == h {
				e.handlers = append(e.handlers[:i], e.handlers[i+1:]...)
				break
			}
		}
		e.mu.Unlock()
		h.stop()
	}
}

// On registers a type-safe event handler on the emitter and returns an
// unsubscribe function. The handler receives events matching the given name,
// including scoped events with the same base name.
//
// In sync mode (default), the handler runs on the emitting goroutine.
// In async mode (WithAsync), the handler runs on its own goroutine with
// buffered delivery.
func On[T any](e *Emitter, name string, fn func(T)) func() {
	h := &handler{
		name: name,
		fn: func(data any) {
			if typed, ok := data.(T); ok {
				fn(typed)
			}
		},
	}
	return registerHandler(e, h)
}

// OnScoped registers a type-safe handler that only receives events emitted
// within the given scope. Returns an unsubscribe function.
func OnScoped[T any](e *Emitter, scope string, name string, fn func(T)) func() {
	h := &handler{
		scope: scope,
		name:  name,
		fn: func(data any) {
			if typed, ok := data.(T); ok {
				fn(typed)
			}
		},
	}
	return registerHandler(e, h)
}

// notify dispatches to all matching handlers. Copies the handler slice
// under read lock so callbacks run without holding the lock.
//
// Matching rules:
//   - Unscoped handlers (registered via On) match on the base event name,
//     receiving both scoped and unscoped events.
//   - Scoped handlers (registered via OnScoped) match only events with
//     the exact scope and base name.
func (e *Emitter) notify(name string, data any) {
	e.mu.RLock()
	if len(e.handlers) == 0 {
		e.mu.RUnlock()
		return
	}
	snapshot := make([]*handler, len(e.handlers))
	copy(snapshot, e.handlers)
	e.mu.RUnlock()

	_, baseName := ParseScopedName(name)

	for _, h := range snapshot {
		if h.scope != "" {
			// Scoped handler: match full wire name.
			if name == ScopedName(h.scope, h.name) {
				h.deliver(data)
			}
		} else {
			// Unscoped handler: match base name (catches all scopes).
			if h.name == baseName {
				h.deliver(data)
			}
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
