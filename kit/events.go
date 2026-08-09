package kit

import (
	"sync"

	"github.com/jrschumacher/wails-kit/v2/events"
)

// dynamicBackend is an events.Backend whose target can be retargeted after
// construction. New wires the Kit's single Emitter against one of these
// instead of a fixed Backend because a CLI/TUI process built from kit.New
// never has an application.App to hand a Backend to — there is nothing to
// construct a real Backend from at New time — while a GUI process needs
// kit/wailsbridge.Attach (WP-31) to retarget that same Emitter (already
// handed out to Settings, Appearance, Health, ...) at the real
// app.EmitEvent once the application.App exists.
//
// Before Set is ever called, Emit drops every event silently — this is
// AD-1's documented choice ("events backend -> a nil-safe backend
// placeholder that buffers-or-drops until wailsbridge.Attach ... drop") and
// is correct for a CLI/TUI process that never attaches: every kit-internal
// Emit call is simply a no-op there, exactly as if nothing were listening.
type dynamicBackend struct {
	mu      sync.RWMutex
	backend events.Backend
}

// Emit implements events.Backend.
func (b *dynamicBackend) Emit(name string, data any) {
	b.mu.RLock()
	target := b.backend
	b.mu.RUnlock()
	if target != nil {
		target.Emit(name, data)
	}
}

// Set retargets future Emit calls at backend. Passing nil reverts to
// dropping events. Safe to call at any time, including concurrently with
// Emit.
func (b *dynamicBackend) Set(backend events.Backend) {
	b.mu.Lock()
	b.backend = backend
	b.mu.Unlock()
}
