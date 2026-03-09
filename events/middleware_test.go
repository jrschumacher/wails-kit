package events

import (
	"sync"
	"testing"
	"time"
)

func TestWithDebounce(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithDebounce("rapid", 50*time.Millisecond))

	emitter.Emit("rapid", "first")
	emitter.Emit("rapid", "second")
	emitter.Emit("rapid", "third")

	// Nothing emitted yet (debounced).
	if mem.Count() != 0 {
		t.Fatalf("expected 0 events during debounce, got %d", mem.Count())
	}

	// Wait for debounce to fire.
	time.Sleep(100 * time.Millisecond)

	if mem.Count() != 1 {
		t.Fatalf("expected 1 debounced event, got %d", mem.Count())
	}
	if mem.Last().Data != "third" {
		t.Errorf("expected last payload 'third', got %v", mem.Last().Data)
	}
}

func TestWithDebounce_NonDebounced_PassThrough(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithDebounce("debounced", 50*time.Millisecond))

	emitter.Emit("normal", "data")

	if mem.Count() != 1 {
		t.Fatalf("expected non-debounced event to pass through, got %d", mem.Count())
	}
}

func TestWithDebounce_Handler_Called(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithDebounce("test", 30*time.Millisecond))

	var received string
	sub := On(emitter, "test", func(s string) { received = s })
	defer sub.Cancel()

	emitter.Emit("test", "hello")
	time.Sleep(80 * time.Millisecond)

	if received != "hello" {
		t.Errorf("expected handler called with 'hello', got %q", received)
	}
}

func TestWithThrottle(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithThrottle("fast", 50*time.Millisecond))

	emitter.Emit("fast", "first")  // allowed (leading edge)
	emitter.Emit("fast", "second") // dropped
	emitter.Emit("fast", "third")  // dropped

	if mem.Count() != 1 {
		t.Fatalf("expected 1 throttled event, got %d", mem.Count())
	}
	if mem.Last().Data != "first" {
		t.Errorf("expected 'first', got %v", mem.Last().Data)
	}

	// After throttle window.
	time.Sleep(60 * time.Millisecond)
	emitter.Emit("fast", "fourth") // allowed
	if mem.Count() != 2 {
		t.Fatalf("expected 2 events after throttle window, got %d", mem.Count())
	}
	if mem.Last().Data != "fourth" {
		t.Errorf("expected 'fourth', got %v", mem.Last().Data)
	}
}

func TestWithThrottle_NonThrottled_PassThrough(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithThrottle("throttled", 50*time.Millisecond))

	emitter.Emit("normal", "a")
	emitter.Emit("normal", "b")

	if mem.Count() != 2 {
		t.Fatalf("expected non-throttled events to pass through, got %d", mem.Count())
	}
}

func TestWithBatching_MaxSize(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithBatching("log", 1*time.Second, 3))

	emitter.Emit("log", "a")
	emitter.Emit("log", "b")

	if mem.Count() != 0 {
		t.Fatalf("expected 0 events before batch full, got %d", mem.Count())
	}

	emitter.Emit("log", "c") // triggers flush at maxSize=3

	if mem.Count() != 1 {
		t.Fatalf("expected 1 batched event, got %d", mem.Count())
	}

	batch, ok := mem.Last().Data.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", mem.Last().Data)
	}
	if len(batch) != 3 || batch[0] != "a" || batch[1] != "b" || batch[2] != "c" {
		t.Errorf("unexpected batch: %v", batch)
	}
}

func TestWithBatching_Duration(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithBatching("log", 50*time.Millisecond, 100))

	emitter.Emit("log", "x")
	emitter.Emit("log", "y")

	if mem.Count() != 0 {
		t.Fatalf("expected 0 events before timer, got %d", mem.Count())
	}

	time.Sleep(100 * time.Millisecond)

	if mem.Count() != 1 {
		t.Fatalf("expected 1 batched event after timer, got %d", mem.Count())
	}

	batch, ok := mem.Last().Data.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", mem.Last().Data)
	}
	if len(batch) != 2 || batch[0] != "x" || batch[1] != "y" {
		t.Errorf("unexpected batch: %v", batch)
	}
}

func TestWithBatching_NonBatched_PassThrough(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithBatching("batched", 1*time.Second, 10))

	emitter.Emit("normal", "data")

	if mem.Count() != 1 {
		t.Fatalf("expected non-batched event to pass through, got %d", mem.Count())
	}
}

func TestClose_FlushesDebounce(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithDebounce("pending", 1*time.Second))

	emitter.Emit("pending", "data")
	if mem.Count() != 0 {
		t.Fatal("expected 0 events before close")
	}

	emitter.Close()

	if mem.Count() != 1 {
		t.Fatalf("expected 1 event after close, got %d", mem.Count())
	}
	if mem.Last().Data != "data" {
		t.Errorf("expected 'data', got %v", mem.Last().Data)
	}
}

func TestClose_FlushesBatch(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem, WithBatching("pending", 1*time.Second, 100))

	emitter.Emit("pending", "a")
	emitter.Emit("pending", "b")
	emitter.Close()

	if mem.Count() != 1 {
		t.Fatalf("expected 1 batched event after close, got %d", mem.Count())
	}
	batch := mem.Last().Data.([]any)
	if len(batch) != 2 {
		t.Errorf("expected 2 items in batch, got %d", len(batch))
	}
}

func TestMiddleware_WithHistory(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem,
		WithHistory(10),
		WithDebounce("test", 30*time.Millisecond),
	)
	emitter.RegisterWindow("w", mem.MemoryWindow("w"))

	emitter.Emit("test", "debounced-data")
	time.Sleep(80 * time.Millisecond)

	// Debounced event should be in history.
	emitter.Replay("w", "test")
	events := mem.EventsFor("w")
	if len(events) != 1 || events[0].Data != "debounced-data" {
		t.Errorf("expected debounced event in history, got %v", events)
	}
}

func TestMiddleware_ConcurrentSafety(t *testing.T) {
	mem := NewMemoryEmitter()
	emitter := NewEmitter(mem,
		WithDebounce("debounced", 5*time.Millisecond),
		WithThrottle("throttled", 5*time.Millisecond),
		WithBatching("batched", 5*time.Millisecond, 10),
		WithHistory(100),
	)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(4)
		go func() {
			defer wg.Done()
			emitter.Emit("debounced", "d")
		}()
		go func() {
			defer wg.Done()
			emitter.Emit("throttled", "t")
		}()
		go func() {
			defer wg.Done()
			emitter.Emit("batched", "b")
		}()
		go func() {
			defer wg.Done()
			emitter.Emit("normal", "n")
		}()
	}
	wg.Wait()
	emitter.Close()
}
