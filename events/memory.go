package events

import (
	"sync"
	"time"
)

// Record is a single emitted event captured by MemoryEmitter.
type Record struct {
	Name     string
	Data     any
	WindowID string // empty for broadcasts
}

// MemoryEmitter captures events in memory for testing. It implements
// Backend and can also be used as a window backend via MemoryWindow.
type MemoryEmitter struct {
	records []Record
	waiters []waiter
	mu      sync.Mutex
}

type waiter struct {
	name string
	ch   chan struct{}
}

// NewMemoryEmitter creates a MemoryEmitter.
func NewMemoryEmitter() *MemoryEmitter {
	return &MemoryEmitter{}
}

func (m *MemoryEmitter) Emit(name string, data any) {
	m.record(Record{Name: name, Data: data})
}

// MemoryWindow returns a Backend that records events with a window ID.
// Use this with Emitter.RegisterWindow to capture targeted events.
func (m *MemoryEmitter) MemoryWindow(id string) Backend {
	return BackendFunc(func(name string, data any) {
		m.record(Record{Name: name, Data: data, WindowID: id})
	})
}

func (m *MemoryEmitter) record(r Record) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, r)
	for i := range m.waiters {
		if m.waiters[i].name == r.Name {
			select {
			case m.waiters[i].ch <- struct{}{}:
			default:
			}
		}
	}
}

// Events returns all captured events.
func (m *MemoryEmitter) Events() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Record, len(m.records))
	copy(out, m.records)
	return out
}

// Clear removes all captured events.
func (m *MemoryEmitter) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = nil
}

// Count returns the number of captured events.
func (m *MemoryEmitter) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.records)
}

// Last returns the most recently emitted event, or nil if none.
func (m *MemoryEmitter) Last() *Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.records) == 0 {
		return nil
	}
	r := m.records[len(m.records)-1]
	return &r
}

// Broadcasts returns only events that were not targeted at a specific window.
func (m *MemoryEmitter) Broadcasts() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Record
	for _, r := range m.records {
		if r.WindowID == "" {
			out = append(out, r)
		}
	}
	return out
}

// EventsFor returns only events targeted at the given window ID.
func (m *MemoryEmitter) EventsFor(windowID string) []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Record
	for _, r := range m.records {
		if r.WindowID == windowID {
			out = append(out, r)
		}
	}
	return out
}

// WaitFor blocks until an event with the given name is emitted or the
// timeout expires. Returns true if the event was received, false on timeout.
func (m *MemoryEmitter) WaitFor(name string, timeout time.Duration) bool {
	ch := make(chan struct{}, 1)

	m.mu.Lock()
	// Check if already emitted.
	for _, r := range m.records {
		if r.Name == name {
			m.mu.Unlock()
			return true
		}
	}
	w := waiter{name: name, ch: ch}
	m.waiters = append(m.waiters, w)
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		for i, existing := range m.waiters {
			if existing.ch == ch {
				m.waiters = append(m.waiters[:i], m.waiters[i+1:]...)
				break
			}
		}
		m.mu.Unlock()
	}()

	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}
