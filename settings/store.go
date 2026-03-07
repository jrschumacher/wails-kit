package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	path     string
	defaults map[string]any
	mu       sync.RWMutex
}

type StoreOption func(*Store)

func WithPath(path string) StoreOption {
	return func(s *Store) { s.path = path }
}

func NewStore(appName string, opts ...StoreOption) *Store {
	home, _ := os.UserHomeDir()
	s := &Store{
		path:     filepath.Join(home, "."+appName, "settings.json"),
		defaults: make(map[string]any),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Store) SetDefaults(defaults map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range defaults {
		s.defaults[k] = v
	}
}

func (s *Store) Load() (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]any)
	for k, v := range s.defaults {
		result[k] = v
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, err
	}

	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil {
		return result, err
	}

	for k, v := range saved {
		result[k] = v
	}

	return result, nil
}

func (s *Store) Save(values map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0600)
}

func (s *Store) Path() string {
	return s.path
}
