package events

import (
	"testing"
)

func TestEmitter(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	emitter.Emit(SettingsChanged, SettingsChangedPayload{Keys: []string{"theme"}})

	if mem.Count() != 1 {
		t.Fatalf("expected 1 event, got %d", mem.Count())
	}

	last := mem.Last()
	if last.Name != SettingsChanged {
		t.Errorf("expected event name %s, got %s", SettingsChanged, last.Name)
	}

	payload, ok := last.Data.(SettingsChangedPayload)
	if !ok {
		t.Fatalf("expected SettingsChangedPayload, got %T", last.Data)
	}
	if len(payload.Keys) != 1 || payload.Keys[0] != "theme" {
		t.Errorf("unexpected payload: %+v", payload)
	}
}

func TestMemoryEmitter_Clear(t *testing.T) {
	mem := NewMemoryEmitter()
	mem.Emit("a", nil)
	mem.Emit("b", nil)
	if mem.Count() != 2 {
		t.Fatalf("expected 2, got %d", mem.Count())
	}
	mem.Clear()
	if mem.Count() != 0 {
		t.Fatalf("expected 0 after clear, got %d", mem.Count())
	}
}

func TestMemoryEmitter_Last_Empty(t *testing.T) {
	mem := NewMemoryEmitter()
	if mem.Last() != nil {
		t.Error("expected nil for empty emitter")
	}
}

func TestMemoryEmitter_Events(t *testing.T) {
	mem := NewMemoryEmitter()
	mem.Emit("first", 1)
	mem.Emit("second", 2)

	events := mem.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Name != "first" {
		t.Errorf("expected first, got %s", events[0].Name)
	}
	if events[1].Name != "second" {
		t.Errorf("expected second, got %s", events[1].Name)
	}
}

func TestBackendFunc(t *testing.T) {
	var captured string
	fn := BackendFunc(func(name string, data any) {
		captured = name
	})

	emitter := NewEmitter(fn)
	emitter.Emit("test:event", nil)

	if captured != "test:event" {
		t.Errorf("expected test:event, got %s", captured)
	}
}
