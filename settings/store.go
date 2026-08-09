package settings

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
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

// NewStore creates a settings store. The default path uses appdirs.Config():
//   - macOS: ~/Library/Application Support/{appName}/settings.json
//   - Linux: $XDG_CONFIG_HOME/{appName}/settings.json
//   - Windows: %AppData%/{appName}/settings.json
func NewStore(appName string, opts ...StoreOption) *Store {
	dirs := appdirs.New(appName)
	s := &Store{
		path:      filepath.Join(dirs.Config(), "settings.json"),
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
//
// The write is durable: data is written to a temp file in the same
// directory, fsynced, closed, and only then renamed over the target path.
// A rename that lands before the data does (WriteFile+Rename with no Sync)
// can leave a zero-length or truncated file after a crash between the
// write and the next flush — mirrors keyring.EnvelopeStore's
// writeTempLocked.
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
	mergedValues := make(map[string]any)
	for k, v := range existing {
		if len(s.knownKeys) > 0 && !s.knownKeys[k] {
			continue
		}
		mergedValues[k] = v
	}

	// Merge: new values override existing
	for k, v := range values {
		if len(s.knownKeys) > 0 && !s.knownKeys[k] {
			continue
		}
		mergedValues[k] = v
	}

	merged, err := json.MarshalIndent(mergedValues, "", "  ")
	if err != nil {
		return err
	}

	tmpPath, err := writeTempFile(dir, merged)
	if err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	syncDir(dir)
	return nil
}

func (s *Store) Path() string {
	return s.path
}

// fileHandle is the subset of *os.File that writeTempFile needs. It exists
// so tests can substitute a fake and assert the fsync-before-rename
// sequence (and its failure behavior) without simulating an OS crash.
type fileHandle interface {
	io.Writer
	Sync() error
	Close() error
	Chmod(os.FileMode) error
	Name() string
}

// createTempFile is overridable in tests.
var createTempFile = func(dir, pattern string) (fileHandle, error) {
	return os.CreateTemp(dir, pattern)
}

// writeTempFile serializes data into a 0600 temp file inside dir and fsyncs
// it before returning. The caller renames it into place (or removes it on
// failure). Same directory as the target: rename is only atomic within a
// filesystem.
func writeTempFile(dir string, data []byte) (string, error) {
	tf, err := createTempFile(dir, ".settings-*.tmp")
	if err != nil {
		return "", err
	}
	name := tf.Name()

	if err := tf.Chmod(0600); err != nil {
		_ = tf.Close()
		_ = os.Remove(name)
		return "", err
	}
	if _, err := tf.Write(data); err != nil {
		_ = tf.Close()
		_ = os.Remove(name)
		return "", err
	}
	// Sync before rename: a rename that lands before the data does leaves a
	// zero-length or truncated settings file after a crash.
	if err := tf.Sync(); err != nil {
		_ = tf.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := tf.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

// syncDir best-effort fsyncs a directory so the rename itself (the entry
// pointing at the new file) is durable, not just the file's contents.
// Windows does not support fsync on directories.
func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
