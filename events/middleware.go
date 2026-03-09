package events

import (
	"sync"
	"time"
)

// debouncer delays emission until no new events arrive for the configured
// duration. Only the most recent payload is emitted.
type debouncer struct {
	mu       sync.Mutex
	duration time.Duration
	timer    *time.Timer
	name     string
	data     any
	emit     func(string, any)
}

func (d *debouncer) push(name string, data any, emit func(string, any)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
	}
	d.name = name
	d.data = data
	d.emit = emit
	d.timer = time.AfterFunc(d.duration, func() {
		d.mu.Lock()
		n, da, em := d.name, d.data, d.emit
		d.timer = nil
		d.mu.Unlock()
		if em != nil {
			em(n, da)
		}
	})
}

// flush fires the pending debounced event immediately, if any.
func (d *debouncer) flush() {
	d.mu.Lock()
	if d.timer == nil {
		d.mu.Unlock()
		return
	}
	d.timer.Stop()
	d.timer = nil
	n, da, em := d.name, d.data, d.emit
	d.mu.Unlock()
	if em != nil {
		em(n, da)
	}
}

// throttler allows at most one event per duration (leading edge).
type throttler struct {
	mu       sync.Mutex
	duration time.Duration
	lastEmit time.Time
}

func (t *throttler) allow() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if now.Sub(t.lastEmit) >= t.duration {
		t.lastEmit = now
		return true
	}
	return false
}

// batcher collects events and flushes them as a []any slice when maxSize
// is reached or the duration elapses.
type batcher struct {
	mu       sync.Mutex
	duration time.Duration
	maxSize  int
	buffer   []any
	timer    *time.Timer
	name     string
	emit     func(string, any)
}

func (b *batcher) add(name string, data any, emit func(string, any)) {
	b.mu.Lock()
	b.name = name
	b.emit = emit
	b.buffer = append(b.buffer, data)

	if len(b.buffer) >= b.maxSize {
		batch := b.buffer
		b.buffer = nil
		if b.timer != nil {
			b.timer.Stop()
			b.timer = nil
		}
		b.mu.Unlock()
		emit(name, batch)
		return
	}

	if b.timer == nil {
		b.timer = time.AfterFunc(b.duration, func() {
			b.flush()
		})
	}
	b.mu.Unlock()
}

// flush emits any buffered events immediately.
func (b *batcher) flush() {
	b.mu.Lock()
	if len(b.buffer) == 0 {
		b.mu.Unlock()
		return
	}
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	batch := b.buffer
	name := b.name
	emit := b.emit
	b.buffer = nil
	b.mu.Unlock()
	if emit != nil {
		emit(name, batch)
	}
}
