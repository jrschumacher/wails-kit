package keyring

import "sync"

// MemoryStore is an in-memory Store for testing.
type MemoryStore struct {
	data map[string]string
	mu   sync.RWMutex
}

// NewMemoryStore creates a new in-memory keyring store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]string)}
}

func (s *MemoryStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

func (s *MemoryStore) Get(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *MemoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

func (s *MemoryStore) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.data[key]
	return ok
}
