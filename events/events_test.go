package events

import (
	"sync"
	"sync/atomic"
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
	unsub := On(emitter, SettingsChanged, func(p SettingsChangedPayload) {
		received = p
	})
	defer unsub()

	emitter.Emit(SettingsChanged, SettingsChangedPayload{Keys: []string{"lang"}})

	if len(received.Keys) != 1 || received.Keys[0] != "lang" {
		t.Errorf("handler not called with correct payload: %+v", received)
	}
}

func TestOn_Unsubscribe(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	callCount := 0
	unsub := On(emitter, "test", func(_ string) {
		callCount++
	})

	emitter.Emit("test", "a")
	unsub()
	emitter.Emit("test", "b")

	if callCount != 1 {
		t.Errorf("expected handler called once, got %d", callCount)
	}
}

func TestOn_WrongType(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	called := false
	unsub := On(emitter, "test", func(_ int) {
		called = true
	})
	defer unsub()

	emitter.Emit("test", "not-an-int")

	if called {
		t.Error("handler should not be called for wrong payload type")
	}
}

func TestOn_MultipleHandlers(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	var calls []string
	unsub1 := On(emitter, "test", func(s string) { calls = append(calls, "h1:"+s) })
	unsub2 := On(emitter, "test", func(s string) { calls = append(calls, "h2:"+s) })
	defer unsub1()
	defer unsub2()

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
	unsub := On(emitter, "test", func(s string) { received = s })
	defer unsub()

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
			unsub := On(emitter, "concurrent", func(_ any) {})
			unsub()
		}()
	}
	wg.Wait()
}

// --- Scoped emitter tests ---

func TestScope_Emit(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	scoped := emitter.Scope("tab:abc123")
	scoped.Emit("stream:delta", "chunk1")

	if mem.Count() != 1 {
		t.Fatalf("expected 1 event, got %d", mem.Count())
	}

	last := mem.Last()
	if last.Name != "@tab:abc123/stream:delta" {
		t.Errorf("expected scoped wire name, got %s", last.Name)
	}
	if last.Data != "chunk1" {
		t.Errorf("expected chunk1, got %v", last.Data)
	}
}

func TestScope_EmitTo(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)
	emitter.RegisterWindow("main", mem.MemoryWindow("main"))

	scoped := emitter.Scope("tab:abc")
	scoped.EmitTo("main", "update", "data")

	events := mem.EventsFor("main")
	if len(events) != 1 {
		t.Fatalf("expected 1 event for main, got %d", len(events))
	}
	if events[0].Name != "@tab:abc/update" {
		t.Errorf("expected scoped wire name, got %s", events[0].Name)
	}
}

func TestOnScoped(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	var received string
	unsub := OnScoped(emitter, "tab:abc", "stream:delta", func(s string) {
		received = s
	})
	defer unsub()

	// Emit from matching scope.
	scoped := emitter.Scope("tab:abc")
	scoped.Emit("stream:delta", "chunk1")

	if received != "chunk1" {
		t.Errorf("expected scoped handler to receive chunk1, got %q", received)
	}
}

func TestOnScoped_DifferentScope(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	called := false
	unsub := OnScoped(emitter, "tab:abc", "stream:delta", func(_ string) {
		called = true
	})
	defer unsub()

	// Emit from a different scope — handler should NOT fire.
	other := emitter.Scope("tab:xyz")
	other.Emit("stream:delta", "chunk")

	if called {
		t.Error("scoped handler should not receive events from a different scope")
	}
}

func TestOnScoped_UnscopedEmit(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	called := false
	unsub := OnScoped(emitter, "tab:abc", "test", func(_ string) {
		called = true
	})
	defer unsub()

	// Unscoped emit — scoped handler should NOT fire.
	emitter.Emit("test", "data")

	if called {
		t.Error("scoped handler should not receive unscoped events")
	}
}

func TestOn_ReceivesScopedEvents(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	var received []string
	unsub := On(emitter, "stream:delta", func(s string) {
		received = append(received, s)
	})
	defer unsub()

	// Unscoped emit.
	emitter.Emit("stream:delta", "global")

	// Scoped emit from two different scopes.
	emitter.Scope("tab:a").Emit("stream:delta", "from-a")
	emitter.Scope("tab:b").Emit("stream:delta", "from-b")

	if len(received) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(received), received)
	}
	if received[0] != "global" || received[1] != "from-a" || received[2] != "from-b" {
		t.Errorf("unexpected events: %v", received)
	}
}

func TestScopedName(t *testing.T) {
	tests := []struct {
		scope, name, want string
	}{
		{"tab:abc", "stream:delta", "@tab:abc/stream:delta"},
		{"", "event", "event"},
		{"simple", "name", "@simple/name"},
	}
	for _, tt := range tests {
		got := ScopedName(tt.scope, tt.name)
		if got != tt.want {
			t.Errorf("ScopedName(%q, %q) = %q, want %q", tt.scope, tt.name, got, tt.want)
		}
	}
}

func TestParseScopedName(t *testing.T) {
	tests := []struct {
		wire      string
		wantScope string
		wantName  string
	}{
		{"@tab:abc/stream:delta", "tab:abc", "stream:delta"},
		{"event", "", "event"},
		{"settings:changed", "", "settings:changed"},
		{"@simple/name", "simple", "name"},
		{"@noslash", "", "@noslash"}, // no separator — treat as unscoped
	}
	for _, tt := range tests {
		scope, name := ParseScopedName(tt.wire)
		if scope != tt.wantScope || name != tt.wantName {
			t.Errorf("ParseScopedName(%q) = (%q, %q), want (%q, %q)",
				tt.wire, scope, name, tt.wantScope, tt.wantName)
		}
	}
}

func TestScope_ConcurrentSafety(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			scoped := emitter.Scope("tab:a")
			scoped.Emit("event", nil)
		}()
		go func() {
			defer wg.Done()
			unsub := OnScoped(emitter, "tab:a", "event", func(_ any) {})
			unsub()
		}()
	}
	wg.Wait()
}

// --- Async emission tests ---

func TestWithAsync(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithAsync(10))
	defer emitter.Close()

	ch := make(chan string, 1)
	unsub := On(emitter, "test", func(s string) {
		ch <- s
	})
	defer unsub()

	emitter.Emit("test", "hello")

	select {
	case got := <-ch:
		if got != "hello" {
			t.Errorf("expected hello, got %q", got)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for async handler")
	}
}

func TestWithAsync_NonBlocking(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithAsync(1))
	defer emitter.Close()

	// Register a handler that blocks.
	block := make(chan struct{})
	unsub := On(emitter, "test", func(_ string) {
		<-block
	})
	defer unsub()

	// Fill the buffer (size 1) + one more to verify non-blocking drop.
	emitter.Emit("test", "a")
	emitter.Emit("test", "b") // may fill buffer while handler blocks on "a"
	emitter.Emit("test", "c") // should not block even if buffer is full

	// If we get here without hanging, the test passes (non-blocking emit).
	close(block)
}

func TestWithAsync_Unsubscribe_StopsGoroutine(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithAsync(10))
	defer emitter.Close()

	var count atomic.Int32
	unsub := On(emitter, "test", func(_ string) {
		count.Add(1)
	})

	emitter.Emit("test", "a")
	time.Sleep(20 * time.Millisecond) // let async handler process
	unsub()
	time.Sleep(10 * time.Millisecond) // let goroutine exit

	before := count.Load()
	emitter.Emit("test", "b")
	time.Sleep(20 * time.Millisecond)

	if count.Load() != before {
		t.Error("handler should not receive events after unsubscribe")
	}
}

func TestWithAsync_Close_StopsGoroutines(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithAsync(10))

	var count atomic.Int32
	On(emitter, "test", func(_ string) {
		count.Add(1)
	})

	emitter.Emit("test", "a")
	time.Sleep(20 * time.Millisecond)
	emitter.Close()

	before := count.Load()
	// Handler goroutine is stopped — new events should not be processed
	// (handler is still in slice but goroutine exited).
	emitter.Emit("test", "b")
	time.Sleep(20 * time.Millisecond)

	if count.Load() != before {
		t.Error("handler goroutine should be stopped after Close")
	}
}

func TestWithAsync_ScopedEvents(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithAsync(10))
	defer emitter.Close()

	ch := make(chan string, 2)
	unsub := OnScoped(emitter, "tab:a", "event", func(s string) {
		ch <- s
	})
	defer unsub()

	emitter.Scope("tab:a").Emit("event", "scoped-data")

	select {
	case got := <-ch:
		if got != "scoped-data" {
			t.Errorf("expected scoped-data, got %q", got)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for async scoped handler")
	}
}
