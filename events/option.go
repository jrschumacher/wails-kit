package events

import "time"

// EmitterOption configures an Emitter.
type EmitterOption func(*Emitter)

// WithHistory enables event history with a ring buffer of the given size.
// This allows replaying recent events to late-joining windows via
// Replay and ReplayAll.
func WithHistory(size int) EmitterOption {
	return func(e *Emitter) {
		if size > 0 {
			e.history = &history{
				buf:  make([]Record, size),
				size: size,
			}
		}
	}
}

// WithDebounce adds debounce middleware for the named event.
// Events are delayed by d; if another event with the same name is emitted
// before the timer fires, the timer resets. Only the last payload is emitted.
// Applies only to broadcast Emit, not EmitTo.
func WithDebounce(name string, d time.Duration) EmitterOption {
	return func(e *Emitter) {
		if e.debounces == nil {
			e.debounces = make(map[string]*debouncer)
		}
		e.debounces[name] = &debouncer{duration: d}
	}
}

// WithThrottle adds throttle middleware for the named event.
// At most one event per duration is emitted (leading edge). Events arriving
// within the throttle window are dropped.
// Applies only to broadcast Emit, not EmitTo.
func WithThrottle(name string, d time.Duration) EmitterOption {
	return func(e *Emitter) {
		if e.throttles == nil {
			e.throttles = make(map[string]*throttler)
		}
		e.throttles[name] = &throttler{duration: d}
	}
}

// WithBatching adds batch middleware for the named event.
// Events are collected until maxSize is reached or d elapses, then emitted
// as a single event with data of type []any containing the collected payloads.
// Applies only to broadcast Emit, not EmitTo.
func WithBatching(name string, d time.Duration, maxSize int) EmitterOption {
	return func(e *Emitter) {
		if e.batchers == nil {
			e.batchers = make(map[string]*batcher)
		}
		e.batchers[name] = &batcher{
			duration: d,
			maxSize:  maxSize,
		}
	}
}

// WithAsync enables asynchronous event delivery to handlers. Each handler
// gets a dedicated goroutine with a buffered channel of the given size.
// When the buffer is full, events are dropped to avoid blocking the emitter.
// Call Close to stop handler goroutines on shutdown.
func WithAsync(bufferSize int) EmitterOption {
	return func(e *Emitter) {
		if bufferSize > 0 {
			e.asyncBuf = bufferSize
		}
	}
}
