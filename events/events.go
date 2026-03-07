package events

// Emitter sends events to the frontend. In a Wails v3 app, wrap
// *application.App with NewEmitter. For tests, use MemoryEmitter.
type Emitter struct {
	backend Backend
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
	return &Emitter{backend: backend}
}

// Emit sends a named event with a typed payload to the frontend.
func (e *Emitter) Emit(name string, data any) {
	e.backend.Emit(name, data)
}

// Common event names for kit-level events.
const (
	SettingsChanged = "settings:changed"
)

// SettingsChangedPayload is the payload for SettingsChanged events.
type SettingsChangedPayload struct {
	Keys []string `json:"keys"`
}
