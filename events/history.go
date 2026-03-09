package events

import "sync"

// history is a ring buffer of emitted events, enabling replay to
// late-joining windows.
type history struct {
	mu    sync.RWMutex
	buf   []Record
	size  int
	pos   int
	count int
}

// record adds an event to the ring buffer.
func (h *history) record(r Record) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf[h.pos] = r
	h.pos = (h.pos + 1) % h.size
	if h.count < h.size {
		h.count++
	}
}

// last returns the most recent event with the given name, or nil.
func (h *history) last(name string) *Record {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for i := 0; i < h.count; i++ {
		idx := (h.pos - 1 - i + h.size) % h.size
		if h.buf[idx].Name == name {
			r := h.buf[idx]
			return &r
		}
	}
	return nil
}

// latest returns the most recent event for each distinct event name.
func (h *history) latest() map[string]Record {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make(map[string]Record)
	for i := 0; i < h.count; i++ {
		idx := (h.pos - 1 - i + h.size) % h.size
		r := h.buf[idx]
		if _, ok := result[r.Name]; !ok {
			result[r.Name] = r
		}
	}
	return result
}
