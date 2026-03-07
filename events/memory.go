package events

import "sync"

// Record is a single emitted event captured by MemoryEmitter.
type Record struct {
	Name string
	Data any
}

// MemoryEmitter captures events in memory for testing.
type MemoryEmitter struct {
	records []Record
	mu      sync.Mutex
}

// NewMemoryEmitter creates a MemoryEmitter.
func NewMemoryEmitter() *MemoryEmitter {
	return &MemoryEmitter{}
}

func (m *MemoryEmitter) Emit(name string, data any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, Record{Name: name, Data: data})
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
