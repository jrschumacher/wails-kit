// Package state provides generic typed state persistence to disk.
// It fills the gap between no persistence and the full settings package —
// for cases where you just need to save/load a struct without schema,
// validation, or keyring integration.
//
// State files are stored as JSON in the app's data directory by default.
// Writes are atomic and durable (write-to-tmp + fsync + rename) to prevent
// corruption. A file that is corrupt anyway (hand-edited, or written by an
// older build that predates the fsync fix) is not a permanent dead end:
// Load quarantines it and recovers with defaults — see Load and
// LoadDetailed.
package state

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/i18n"
)

// Error codes for the state package.
const (
	ErrStateLoad   errors.Code = "state_load"
	ErrStateSave   errors.Code = "state_save"
	ErrStateConfig errors.Code = "state_config"
)

func init() {
	errors.RegisterMessages(map[errors.Code]i18n.Text{
		ErrStateLoad:   i18n.T("wailskit.state.errors.load", "Failed to load application state. Please try again."),
		ErrStateSave:   i18n.T("wailskit.state.errors.save", "Failed to save application state. Please try again."),
		ErrStateConfig: i18n.T("wailskit.state.errors.config", "State store is misconfigured. Please contact support."),
	})
}

// Event names emitted by the state package.
const (
	StateLoaded = "state:loaded"
	StateSaved  = "state:saved"
	// StateCorrupted fires when Load/LoadDetailed finds a state file that
	// exists but fails to parse as JSON. By the time it fires, the corrupt
	// file has already been quarantined (renamed aside, not deleted) and
	// the load has already recovered with defaults — this event exists so
	// the app can tell the user something was reset, not to gate recovery.
	StateCorrupted = "state:corrupted"
)

// StateLoadedPayload is the payload for StateLoaded events.
type StateLoadedPayload struct {
	Name string `json:"name"`
}

// StateSavedPayload is the payload for StateSaved events.
type StateSavedPayload struct {
	Name string `json:"name"`
}

// StateCorruptedPayload is the payload for StateCorrupted events.
type StateCorruptedPayload struct {
	Name string `json:"name"`
	// Path is the original state file path.
	Path string `json:"path"`
	// QuarantinePath is where the corrupt file was moved so its bytes are
	// preserved for support/debugging. Empty if the rename itself failed
	// (e.g. a permissions problem) — Load still recovers with defaults in
	// that case, but the corrupt bytes were not preserved.
	QuarantinePath string `json:"quarantinePath"`
	// Err is the JSON parse error that triggered quarantine.
	Err string `json:"err"`
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

// WithName sets the state file name (without extension). It may be called
// before or after WithAppName / WithStoragePath — the path is re-derived
// from whichever name is current, so either order produces the same result.
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
// the store knows where to persist state; New returns ErrStateConfig if
// neither is set. Without this check, a misconfigured Store silently
// succeeds at Load (returning defaults, since the empty path "doesn't
// exist") while Save fails later with a confusing os.Rename error — by
// the time that surfaces, it is disconnected from the missing option.
func New[T any](opts ...Option[T]) (*Store[T], error) {
	s := &Store[T]{
		name: "state",
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.path == "" {
		return nil, errors.New(ErrStateConfig, "state.New requires WithAppName or WithStoragePath", nil)
	}
	return s, nil
}

// Load reads the state from disk. If the file does not exist, the defaults
// value is returned (or the zero value of T if no defaults were set).
//
// If the file exists but fails to parse as JSON, Load does not return an
// error for that case: the corrupt file is quarantined (renamed aside with
// a "<path>.corrupt-<unix-nano>" suffix, preserving the original bytes) and
// Load returns defaults, exactly as if the file had never existed. This
// mirrors settings.Store's quarantine-on-corrupt behavior — an
// unrecoverable dead end (every future Load erroring the same way forever,
// recoverable only by a user who knows the file path deleting it by hand)
// is worse than a silent reset with the old file preserved. A
// StateCorrupted event is emitted (if an emitter is configured) so the app
// can tell the user their saved state was reset.
//
// Callers that must distinguish a genuinely missing file from one that was
// just recovered from corruption should call LoadDetailed instead — Load
// collapses both into "defaults, nil error".
func (s *Store[T]) Load() (T, error) {
	v, _, err := s.LoadDetailed()
	return v, err
}

// LoadDetailed behaves exactly like Load but additionally reports whether
// this call recovered from a corrupt file: recovered is true only when the
// file existed but failed to parse as JSON and has just been quarantined
// and reset to defaults. recovered is false both when the file never
// existed and when it loaded successfully.
//
// firstrun is the motivating caller: collapsing "no stamp" and "corrupt
// stamp, now reset" into the same signal would make a corrupted stamp
// indistinguishable from a fresh install and re-run first-run onboarding
// for what is almost certainly an existing user.
func (s *Store[T]) LoadDetailed() (value T, recovered bool, err error) {
	s.mu.RLock()
	path := s.path
	name := s.name

	var zero T
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			result := s.defaultValue()
			s.mu.RUnlock()
			// Emit after releasing the lock: emitter handlers run
			// synchronously by default, so a handler that calls back into
			// Load/Save on this store would otherwise deadlock on RLock
			// held by this goroutine (Go's sync.RWMutex is not
			// re-entrant).
			s.emit(StateLoaded, StateLoadedPayload{Name: name})
			return result, false, nil
		}
		s.mu.RUnlock()
		return zero, false, errors.Wrap(ErrStateLoad, "failed to read state file", readErr)
	}

	var result T
	if unmarshalErr := json.Unmarshal(data, &result); unmarshalErr != nil {
		s.mu.RUnlock()
		return s.recoverCorrupt(path, name, unmarshalErr)
	}
	s.mu.RUnlock()

	s.emit(StateLoaded, StateLoadedPayload{Name: name})
	return result, false, nil
}

// recoverCorrupt runs when the initial (read-lock) pass through
// LoadDetailed finds a file that exists but fails to parse as JSON. It
// re-reads and re-verifies under the write lock — a concurrent Save may
// have raced and replaced the file with a valid one between the RUnlock in
// LoadDetailed and here — and only quarantines a file that is still
// unparseable, so a repaired-in-the-meantime file is never mistaken for
// corruption and moved aside anyway.
func (s *Store[T]) recoverCorrupt(path, name string, parseErr error) (T, bool, error) {
	s.mu.Lock()

	var result T
	data, err := os.ReadFile(path)
	switch {
	case err != nil && os.IsNotExist(err):
		// Deleted, or already quarantined by a racing call, between the
		// RUnlock in LoadDetailed and here — nothing left to recover from.
		result = s.defaultValue()
		s.mu.Unlock()
		s.emit(StateLoaded, StateLoadedPayload{Name: name})
		return result, false, nil
	case err != nil:
		s.mu.Unlock()
		return result, false, errors.Wrap(ErrStateLoad, "failed to read state file", err)
	}

	if json.Unmarshal(data, &result) == nil {
		// A concurrent Save repaired the file — nothing to quarantine.
		s.mu.Unlock()
		s.emit(StateLoaded, StateLoadedPayload{Name: name})
		return result, false, nil
	}

	quarantinePath := fmt.Sprintf("%s.corrupt-%d", path, time.Now().UnixNano())
	quarantined := ""
	if renameErr := os.Rename(path, quarantinePath); renameErr == nil {
		quarantined = quarantinePath
	}
	result = s.defaultValue()
	s.mu.Unlock()

	s.emit(StateLoaded, StateLoadedPayload{Name: name})
	s.emit(StateCorrupted, StateCorruptedPayload{
		Name:           name,
		Path:           path,
		QuarantinePath: quarantined,
		Err:            parseErr.Error(),
	})
	return result, true, nil
}

// Save writes the state to disk atomically and durably: write to a temp
// file in the same directory, fsync it, close it, then rename it over the
// target path. The fsync matters — a rename that lands before the data
// does can leave a zero-length or partially-written file as the rename
// winner if the process crashes between the write and the next flush;
// mirrors settings.Store.Save's writeTempFile.
func (s *Store[T]) Save(value T) error {
	s.mu.Lock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		s.mu.Unlock()
		return errors.Wrap(ErrStateSave, "failed to create state directory", err)
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		s.mu.Unlock()
		return errors.Wrap(ErrStateSave, "failed to marshal state", err)
	}

	tmpPath, err := writeTempFile(dir, data)
	if err != nil {
		s.mu.Unlock()
		return errors.Wrap(ErrStateSave, "failed to write temp state file", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		s.mu.Unlock()
		return errors.Wrap(ErrStateSave, "failed to rename temp state file", err)
	}
	syncDir(dir)

	name := s.name
	s.mu.Unlock()

	// Emit after releasing the lock (see Load for why): a handler that
	// calls store.Load() or store.Save() synchronously in response to
	// state:saved must not deadlock against the write lock this goroutine
	// still held a moment ago.
	s.emit(StateSaved, StateSavedPayload{Name: name})
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
	tf, err := createTempFile(dir, ".state-*.tmp")
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
	// zero-length or truncated state file after a crash.
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
