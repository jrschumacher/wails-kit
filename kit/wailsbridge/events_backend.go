package wailsbridge

import "github.com/wailsapp/wails/v3/pkg/application"

// eventsBackend adapts a live *application.App to events.Backend
// (Emit(name string, data any)) — the seam kit.Kit.SetEventsBackend
// retargets. app.Event.Emit's real signature is variadic
// (Emit(name string, data ...any) bool); this always passes exactly one
// data argument, matching events.Backend's contract and how every other
// kit package calls Emitter.Emit.
type eventsBackend struct {
	app *application.App
}

func newEventsBackend(app *application.App) *eventsBackend {
	return &eventsBackend{app: app}
}

// Emit implements events.Backend.
func (b *eventsBackend) Emit(name string, data any) {
	b.app.Event.Emit(name, data)
}
