// Package state provides generic typed state persistence to disk.
// It fills the gap between no persistence and the full settings package —
// for cases where you just need to save/load a struct without schema,
// validation, or keyring integration.
//
// State files are stored as JSON in the app's data directory by default.
// Writes are atomic (write-to-tmp + rename) to prevent corruption.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/jrschumacher/wails-kit/appdirs"
	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/events"
)

// Error codes for the state package.
const (
	ErrStateLoad errors.Code = "state_load"
	ErrStateSave errors.Code = "state_save"
)

func init() {
	errors.RegisterMessages(map[errors.Code]string{
		ErrStateLoad: "Failed to load application state. Please try again.",
		ErrStateSave: "Failed to save application state. Please try again.",
	})
}

// Event names emitted by the state package.
const (
	StateLoaded = "state:loaded"
	StateSaved  = "state:saved"
)

// StateLoadedPayload is the payload for StateLoaded events.
type StateLoadedPayload struct {
	Name string `json:"name"`
}

// StateSavedPayload is the payload for StateSaved events.
type StateSavedPayload struct {
	Name string `json:"name"`
}

// Store provides type-safe load/save for a single state struct.
type Store[T any] struct {
	path     string
	name     string
	defaults *T
	emitter  *events.Emitter
	mu       sync.RWMutex
}

// Option configures a Store.
type Option[T any] func(*Store[T])

// WithAppName sets the storage directory using appdirs.Data().
// The state file is stored at {dataDir}/state/{name}.json.
func WithAppName[T any](appName string) Option[T] {
	return func(s *Store[T]) {
		dirs := appdirs.New(appName)
		s.path = filepath.Join(dirs.Data(), "state", s.name+".json")
	}
}

// WithName sets the state file name (without extension).
// Must be called before WithAppName for the name to take effect in the path.
func WithName[T any](name string) Option[T] {
	return func(s *Store[T]) {
		s.name = name
		// Re-derive path if it was already set from defaults
		if s.path != "" {
			dir := filepath.Dir(s.path)
			s.path = filepath.Join(dir, name+".json")
		}
	}
}

// WithStoragePath overrides the full file path for the state file.
func WithStoragePath[T any](path string) Option[T] {
	return func(s *Store[T]) {
		s.path = path
	}
}

// WithEmitter sets an optional event emitter for state:loaded and state:saved events.
func WithEmitter[T any](e *events.Emitter) Option[T] {
	return func(s *Store[T]) {
		s.emitter = e
	}
}

// WithDefaults sets the default value returned when no state file exists.
func WithDefaults[T any](defaults T) Option[T] {
	return func(s *Store[T]) {
		s.defaults = &defaults
	}
}

// New creates a Store for the given type. Options are applied in order.
//
// At minimum, either WithAppName or WithStoragePath must be provided so
// the store knows where to persist state.
func New[T any](opts ...Option[T]) *Store[T] {
	s := &Store[T]{
		name: "state",
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Load reads the state from disk. If the file does not exist, the defaults
// value is returned (or the zero value of T if no defaults were set).
func (s *Store[T]) Load() (T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var zero T

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			result := s.defaultValue()
			s.emit(StateLoaded, StateLoadedPayload{Name: s.name})
			return result, nil
		}
		return zero, errors.Wrap(ErrStateLoad, "failed to read state file", err)
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, errors.Wrap(ErrStateLoad, "failed to parse state file", err)
	}

	s.emit(StateLoaded, StateLoadedPayload{Name: s.name})
	return result, nil
}

// Save writes the state to disk atomically (write-to-tmp + rename).
func (s *Store[T]) Save(value T) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.Wrap(ErrStateSave, "failed to create state directory", err)
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return errors.Wrap(ErrStateSave, "failed to marshal state", err)
	}

	// Atomic write: write to temp file, then rename
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return errors.Wrap(ErrStateSave, "failed to write temp state file", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return errors.Wrap(ErrStateSave, "failed to rename temp state file", err)
	}

	s.emit(StateSaved, StateSavedPayload{Name: s.name})
	return nil
}

// Delete removes the state file from disk.
func (s *Store[T]) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := os.Remove(s.path)
	if err != nil && !os.IsNotExist(err) {
		return errors.Wrap(ErrStateSave, "failed to delete state file", err)
	}
	return nil
}

// Path returns the resolved file path for the state file.
func (s *Store[T]) Path() string {
	return s.path
}

func (s *Store[T]) defaultValue() T {
	if s.defaults != nil {
		return *s.defaults
	}
	var zero T
	return zero
}

func (s *Store[T]) emit(name string, data any) {
	if s.emitter != nil {
		s.emitter.Emit(name, data)
	}
}
