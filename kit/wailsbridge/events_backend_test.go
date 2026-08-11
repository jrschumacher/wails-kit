package wailsbridge

import (
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TestEventsBackend_Emit verifies eventsBackend.Emit reaches the app's own
// event system, not just that it doesn't panic. app.Event.Emit dispatches
// to app.Event.On listeners on a goroutine (wails/v3 pkg/application
// events.go, EventProcessor.Emit — `go func(){ e.dispatchEventToListeners
// (...) }()`), so this synchronizes on a channel rather than asserting
// immediately after the call.
func TestEventsBackend_Emit(t *testing.T) {
	app := testApp(t)
	backend := newEventsBackend(app)

	const name = "wailsbridge-test:events-backend-emit"
	received := make(chan any, 1)
	off := app.Event.On(name, func(e *application.CustomEvent) {
		received <- e.Data
	})
	defer off()

	backend.Emit(name, map[string]any{"ok": true})

	select {
	case data := <-received:
		m, ok := data.(map[string]any)
		if !ok || m["ok"] != true {
			t.Errorf("received data = %#v, want map[ok:true]", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for app.Event.On listener to fire")
	}
}
