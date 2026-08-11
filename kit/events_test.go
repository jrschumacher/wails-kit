package kit

import (
	"testing"

	"github.com/jrschumacher/wails-kit/v2/events"
)

// TestEventsDropUntilBackendSet covers AD-1's documented default: before
// SetEventsBackend is called, every Emit is silently dropped — the
// behavior a CLI/TUI process that never attaches to Wails gets by design.
func TestEventsDropUntilBackendSet(t *testing.T) {
	k := newTestKit(t)

	k.Events.Emit("test:event", "payload")
	// No assertion possible against "dropped" directly — the assertion is
	// that Set later starts receiving what's emitted afterward, proving the
	// pre-Set emit truly went nowhere rather than being buffered.

	mem := events.NewMemoryEmitter()
	k.SetEventsBackend(mem)

	k.Events.Emit("test:event", "after-set")

	got := mem.Events()
	if len(got) != 1 {
		t.Fatalf("MemoryEmitter recorded %d events, want 1 (only the post-Set emit)", len(got))
	}
	if got[0].Name != "test:event" || got[0].Data != "after-set" {
		t.Errorf("recorded event = %+v, want {test:event after-set}", got[0])
	}
}

// TestSetEventsBackendRetargets covers switching the backend more than
// once, which wailsbridge would only ever do once in practice but which
// the underlying dynamicBackend must support cleanly regardless.
func TestSetEventsBackendRetargets(t *testing.T) {
	k := newTestKit(t)

	first := events.NewMemoryEmitter()
	second := events.NewMemoryEmitter()

	k.SetEventsBackend(first)
	k.Events.Emit("e", 1)
	k.SetEventsBackend(second)
	k.Events.Emit("e", 2)

	if got := first.Count(); got != 1 {
		t.Errorf("first backend recorded %d events, want 1", got)
	}
	if got := second.Count(); got != 1 {
		t.Errorf("second backend recorded %d events, want 1", got)
	}
}
