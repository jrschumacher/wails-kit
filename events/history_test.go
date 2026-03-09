package events

import (
	"testing"
)

func TestWithHistory_Replay(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithHistory(100))
	emitter.RegisterWindow("prefs", mem.MemoryWindow("prefs"))

	emitter.Emit(SettingsChanged, SettingsChangedPayload{Keys: []string{"theme"}})
	emitter.Emit(SettingsChanged, SettingsChangedPayload{Keys: []string{"lang"}})

	emitter.Replay("prefs", SettingsChanged)

	prefsEvents := mem.EventsFor("prefs")
	if len(prefsEvents) != 1 {
		t.Fatalf("expected 1 replayed event, got %d", len(prefsEvents))
	}
	payload, ok := prefsEvents[0].Data.(SettingsChangedPayload)
	if !ok {
		t.Fatalf("expected SettingsChangedPayload, got %T", prefsEvents[0].Data)
	}
	if len(payload.Keys) != 1 || payload.Keys[0] != "lang" {
		t.Errorf("expected replayed payload with 'lang', got %+v", payload)
	}
}

func TestWithHistory_ReplayAll(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithHistory(100))
	emitter.RegisterWindow("new-win", mem.MemoryWindow("new-win"))

	emitter.Emit("settings:changed", "settings-data")
	emitter.Emit("user:login", "user-data")
	emitter.Emit("settings:changed", "settings-data-v2")

	emitter.ReplayAll("new-win")

	events := mem.EventsFor("new-win")
	if len(events) != 2 {
		t.Fatalf("expected 2 replayed events (one per name), got %d", len(events))
	}

	names := map[string]any{}
	for _, e := range events {
		names[e.Name] = e.Data
	}
	if names["settings:changed"] != "settings-data-v2" {
		t.Errorf("expected latest settings data, got %v", names["settings:changed"])
	}
	if names["user:login"] != "user-data" {
		t.Errorf("expected user data, got %v", names["user:login"])
	}
}

func TestWithHistory_Replay_NoHistory(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem) // no WithHistory
	emitter.RegisterWindow("w", mem.MemoryWindow("w"))

	emitter.Emit("test", "data")
	emitter.Replay("w", "test")

	if len(mem.EventsFor("w")) != 0 {
		t.Error("expected no replay when history is disabled")
	}
}

func TestWithHistory_Replay_UnregisteredWindow(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithHistory(10))

	emitter.Emit("test", "data")
	// Should not panic.
	emitter.Replay("nonexistent", "test")
}

func TestWithHistory_Replay_NoMatchingEvent(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithHistory(10))
	emitter.RegisterWindow("w", mem.MemoryWindow("w"))

	emitter.Emit("other", "data")
	emitter.Replay("w", "missing")

	if len(mem.EventsFor("w")) != 0 {
		t.Error("expected no replay for non-existent event name")
	}
}

func TestWithHistory_RingBufferOverflow(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithHistory(3))
	emitter.RegisterWindow("w", mem.MemoryWindow("w"))

	emitter.Emit("a", "a1")
	emitter.Emit("b", "b1")
	emitter.Emit("c", "c1")
	emitter.Emit("d", "d1") // overwrites "a1"

	// "a" should no longer be in history.
	emitter.Replay("w", "a")
	if len(mem.EventsFor("w")) != 0 {
		t.Error("expected 'a' to be evicted from ring buffer")
	}

	// "d" should be in history.
	emitter.Replay("w", "d")
	evts := mem.EventsFor("w")
	if len(evts) != 1 || evts[0].Data != "d1" {
		t.Errorf("expected replayed 'd1', got %v", evts)
	}
}

func TestWithHistory_EmitToRecordsHistory(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithHistory(10))
	emitter.RegisterWindow("main", mem.MemoryWindow("main"))
	emitter.RegisterWindow("prefs", mem.MemoryWindow("prefs"))

	emitter.EmitTo("main", "nav:change", "page1")

	// Replay to a different window should find the event.
	emitter.Replay("prefs", "nav:change")
	prefsEvents := mem.EventsFor("prefs")
	if len(prefsEvents) != 1 || prefsEvents[0].Data != "page1" {
		t.Errorf("expected EmitTo event in history, got %v", prefsEvents)
	}
}
