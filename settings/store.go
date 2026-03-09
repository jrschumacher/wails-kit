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
	// knownKeys tracks schema-defined keys; on load, unknown keys are stripped.
	knownKeys map[string]bool
	mu        sync.RWMutex
}

type StoreOption func(*Store)

// WithPath overrides the default settings file path (useful for tests).
func WithPath(path string) StoreOption {
	return func(s *Store) { s.path = path }
}

// NewStore creates a settings store. The default path uses os.UserConfigDir():
//   - macOS: ~/Library/Application Support/{appName}/settings.json
//   - Linux: $XDG_CONFIG_HOME/{appName}/settings.json
//   - Windows: %AppData%/{appName}/settings.json
func NewStore(appName string, opts ...StoreOption) *Store {
	configDir, err := os.UserConfigDir()
	if err != nil {
		// Fallback to home directory dot-dir
		home, _ := os.UserHomeDir()
		configDir = home
		appName = "." + appName
	}
	s := &Store{
		path:      filepath.Join(configDir, appName, "settings.json"),
		defaults:  make(map[string]any),
		knownKeys: make(map[string]bool),
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

// SetKnownKeys sets the set of schema-defined keys. On Load, keys not in this
// set are stripped from saved data. If empty, no stripping occurs.
func (s *Store) SetKnownKeys(keys map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.knownKeys = keys
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
		// Strip unknown keys if knownKeys is populated
		if len(s.knownKeys) > 0 && !s.knownKeys[k] {
			continue
		}
		result[k] = v
	}

	return result, nil
}

// Save merges values into the existing saved settings and writes atomically.
// Only the keys present in values are updated; existing keys not in values
// are preserved. Password keys (managed by keyring) should not be included.
func (s *Store) Save(values map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Load existing saved data to merge with
	existing := make(map[string]any)
	data, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(data, &existing)
	}

	// Merge: new values override existing
	for k, v := range values {
		existing[k] = v
	}

	merged, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write: write to temp file, then rename
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, merged, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func (s *Store) Path() string {
	return s.path
}
