package events

import (
	"sync"
	"testing"
	"time"
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

func TestEmitTo(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	mainWin := mem.MemoryWindow("main")
	prefsWin := mem.MemoryWindow("preferences")
	emitter.RegisterWindow("main", mainWin)
	emitter.RegisterWindow("preferences", prefsWin)

	emitter.EmitTo("main", "nav:change", "page1")

	mainEvents := mem.EventsFor("main")
	if len(mainEvents) != 1 {
		t.Fatalf("expected 1 event for main, got %d", len(mainEvents))
	}
	if mainEvents[0].Name != "nav:change" {
		t.Errorf("expected nav:change, got %s", mainEvents[0].Name)
	}

	prefsEvents := mem.EventsFor("preferences")
	if len(prefsEvents) != 0 {
		t.Errorf("expected 0 events for preferences, got %d", len(prefsEvents))
	}
}

func TestEmitTo_UnregisteredWindow(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	// Should not panic or error — silently dropped.
	emitter.EmitTo("nonexistent", "test:event", nil)

	// No events should be recorded via the window backend.
	if len(mem.EventsFor("nonexistent")) != 0 {
		t.Error("expected no events for unregistered window")
	}
}

func TestUnregisterWindow(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)
	emitter.RegisterWindow("temp", mem.MemoryWindow("temp"))

	emitter.EmitTo("temp", "test", nil)
	if len(mem.EventsFor("temp")) != 1 {
		t.Fatal("expected 1 event before unregister")
	}

	emitter.UnregisterWindow("temp")
	emitter.EmitTo("temp", "test", nil)
	if len(mem.EventsFor("temp")) != 1 {
		t.Error("expected still 1 event after unregister")
	}
}

func TestBroadcasts(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)
	emitter.RegisterWindow("main", mem.MemoryWindow("main"))

	emitter.Emit("broadcast:event", "data")
	emitter.EmitTo("main", "targeted:event", "data")

	broadcasts := mem.Broadcasts()
	if len(broadcasts) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(broadcasts))
	}
	if broadcasts[0].Name != "broadcast:event" {
		t.Errorf("expected broadcast:event, got %s", broadcasts[0].Name)
	}
}

func TestOn_TypedSubscription(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	var received SettingsChangedPayload
	sub := On(emitter, SettingsChanged, func(p SettingsChangedPayload) {
		received = p
	})
	defer sub.Cancel()

	emitter.Emit(SettingsChanged, SettingsChangedPayload{Keys: []string{"lang"}})

	if len(received.Keys) != 1 || received.Keys[0] != "lang" {
		t.Errorf("handler not called with correct payload: %+v", received)
	}
}

func TestOn_Cancel(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	callCount := 0
	sub := On(emitter, "test", func(_ string) {
		callCount++
	})

	emitter.Emit("test", "a")
	sub.Cancel()
	emitter.Emit("test", "b")

	if callCount != 1 {
		t.Errorf("expected handler called once, got %d", callCount)
	}
}

func TestOn_WrongType(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	called := false
	sub := On(emitter, "test", func(_ int) {
		called = true
	})
	defer sub.Cancel()

	emitter.Emit("test", "not-an-int")

	if called {
		t.Error("handler should not be called for wrong payload type")
	}
}

func TestOn_MultipleHandlers(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	var calls []string
	sub1 := On(emitter, "test", func(s string) { calls = append(calls, "h1:"+s) })
	sub2 := On(emitter, "test", func(s string) { calls = append(calls, "h2:"+s) })
	defer sub1.Cancel()
	defer sub2.Cancel()

	emitter.Emit("test", "x")

	if len(calls) != 2 || calls[0] != "h1:x" || calls[1] != "h2:x" {
		t.Errorf("expected both handlers called: %v", calls)
	}
}

func TestOn_EmitToNotifiesHandlers(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)
	emitter.RegisterWindow("main", mem.MemoryWindow("main"))

	var received string
	sub := On(emitter, "test", func(s string) { received = s })
	defer sub.Cancel()

	emitter.EmitTo("main", "test", "targeted")

	if received != "targeted" {
		t.Errorf("expected handler notified on EmitTo, got %q", received)
	}
}

func TestWaitFor(t *testing.T) {
	mem := NewMemoryEmitter()

	go func() {
		time.Sleep(10 * time.Millisecond)
		mem.Emit("delayed", nil)
	}()

	if !mem.WaitFor("delayed", 1*time.Second) {
		t.Error("expected WaitFor to succeed")
	}
}

func TestWaitFor_AlreadyEmitted(t *testing.T) {
	mem := NewMemoryEmitter()
	mem.Emit("already", nil)

	if !mem.WaitFor("already", 10*time.Millisecond) {
		t.Error("expected WaitFor to succeed for already-emitted event")
	}
}

func TestWaitFor_Timeout(t *testing.T) {
	mem := NewMemoryEmitter()

	if mem.WaitFor("never", 10*time.Millisecond) {
		t.Error("expected WaitFor to timeout")
	}
}

func TestEmitter_ConcurrentSafety(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)
	emitter.RegisterWindow("w1", mem.MemoryWindow("w1"))

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			emitter.Emit("concurrent", nil)
		}()
		go func() {
			defer wg.Done()
			emitter.EmitTo("w1", "concurrent", nil)
		}()
		go func() {
			defer wg.Done()
			sub := On(emitter, "concurrent", func(_ any) {})
			sub.Cancel()
		}()
	}
	wg.Wait()
}
