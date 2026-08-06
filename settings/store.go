package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// QuarantineTimeFormat is the timestamp appended to a quarantined config file's
// name. It is UTC and lexically sortable, so `ls` and any file browser list the
// quarantined copies oldest-first.
const QuarantineTimeFormat = "20060102T150405Z"

// CorruptConfig describes a settings file that could not be parsed and was
// therefore moved aside so the application could continue with defaults.
//
// It is a report, not an error: by the time a caller can observe one, the
// damaged file has already been preserved and the app is working again. It
// exists so the app can tell the user what happened and where their old file
// went — see Service.Corruption.
type CorruptConfig struct {
	// Path is the config file that could not be parsed.
	Path string `json:"path"`
	// QuarantinePath is where the unparseable bytes were preserved, verbatim.
	QuarantinePath string `json:"quarantinePath"`
	// DetectedAt is when the quarantine happened, in UTC.
	DetectedAt time.Time `json:"detectedAt"`
	// Reason is the JSON decoding error, for a log or a details pane. It is
	// developer-facing text; do not show it to an end user on its own.
	Reason string `json:"reason"`
}

type Store struct {
	path     string
	defaults map[string]any
	mu       sync.RWMutex
	// corrupt records the most recent quarantine, and is nil until one happens.
	// Guarded by mu.
	corrupt *CorruptConfig
}

type StoreOption func(*Store)

func WithPath(path string) StoreOption {
	return func(s *Store) { s.path = path }
}

// NewStore returns a Store for appName.
//
// The default location is the platform's user configuration directory —
// ~/Library/Application Support/<appName>/settings.json on macOS,
// %AppData%\<appName>\settings.json on Windows and
// ~/.config/<appName>/settings.json on Linux — because this is a desktop
// application's data, not a CLI's dotfile.
//
// An error is returned when the config directory cannot be resolved (no $HOME,
// no %AppData%). That case must not be swallowed: silently falling back to a
// relative path makes the config file follow the process's working directory,
// so the app appears to lose every setting when launched from elsewhere.
// WithPath bypasses the lookup entirely and therefore never fails.
func NewStore(appName string, opts ...StoreOption) (*Store, error) {
	s := &Store{defaults: make(map[string]any)}
	for _, opt := range opts {
		opt(s)
	}
	if s.path == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("settings: cannot determine the user config directory for %q: %w", appName, err)
		}
		s.path = filepath.Join(dir, appName, "settings.json")
	}
	return s, nil
}

func (s *Store) SetDefaults(defaults map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	maps.Copy(s.defaults, defaults)
}

func (s *Store) Load() (map[string]any, error) {
	result := s.defaultsClone()

	saved, err := s.read()
	if err != nil {
		return result, err
	}
	maps.Copy(result, saved)

	return result, nil
}

func (s *Store) defaultsClone() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]any, len(s.defaults))
	maps.Copy(result, s.defaults)
	return result
}

// loadPersisted returns only the values written to disk, with no defaults
// merged in. Callers performing a read-modify-write of the config file must use
// this rather than Load, so that schema defaults are never baked into the file
// (a persisted default would shadow any later change to the schema).
func (s *Store) loadPersisted() (map[string]any, error) {
	return s.read()
}

// Corruption returns a copy of the most recent quarantine report, or nil if the
// config file has always parsed. See CorruptConfig and Service.Corruption.
func (s *Store) Corruption() *CorruptConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.corrupt == nil {
		return nil
	}
	c := *s.corrupt
	return &c
}

// read decodes the config file, quarantining it first if it cannot be parsed.
//
// The happy path takes only the read lock. Quarantine needs the write lock (it
// renames the file and records the report), so it is split into its own method
// rather than being done here under a read lock held by any number of readers.
func (s *Store) read() (map[string]any, error) {
	s.mu.RLock()
	saved, err := s.readLocked()
	s.mu.RUnlock()

	if err == nil || !isMalformedJSON(err) {
		return saved, err
	}
	return s.quarantine()
}

// quarantineIfCorrupt reads the config file for the sole purpose of moving it
// aside if it is unparseable, so that corruption is detected at startup rather
// than whenever the app first happens to read its settings.
//
// Errors other than corruption are deliberately dropped: a transient read
// failure resurfaces on the first real Load or Save, and there is nothing
// useful to do with it here.
func (s *Store) quarantineIfCorrupt() {
	_, _ = s.read()
}

// quarantine renames an unparseable config file out of the way and reports the
// move through Corruption, leaving the caller with an empty value set so the
// app continues on defaults and can save again.
//
// Only malformed JSON reaches here. A permission or I/O error is transient in
// the sense that matters — retrying, or fixing the permissions, recovers the
// user's settings — so it stays an error and is never "recovered from" by
// destroying state. Malformed JSON is the opposite: no amount of retrying will
// ever parse it, and refusing to continue leaves a non-technical user with an
// app that cannot save anything and a file they cannot find.
func (s *Store) quarantine() (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Re-read under the write lock: between dropping the read lock and taking
	// this one, another goroutine may already have quarantined the file, or the
	// app may have rewritten it.
	saved, err := s.readLocked()
	if err == nil || !isMalformedJSON(err) {
		return saved, err
	}

	dst, dstErr := uniqueQuarantinePath(s.path, time.Now())
	if dstErr == nil {
		dstErr = os.Rename(s.path, dst)
	}
	if dstErr != nil {
		// A read-only directory, or any other failure to move the file aside.
		// Returning an error is the only honest outcome: continuing would hand
		// back defaults that the very next Save would write straight over the
		// only copy of the user's settings.
		return map[string]any{}, fmt.Errorf(
			"settings: %q is not valid JSON (%v) and could not be moved aside to %q: %w",
			s.path, err, dst, dstErr,
		)
	}

	s.corrupt = &CorruptConfig{
		Path:           s.path,
		QuarantinePath: dst,
		DetectedAt:     time.Now().UTC(),
		Reason:         err.Error(),
	}
	return map[string]any{}, nil
}

// uniqueQuarantinePath returns "<path>.corrupt-<timestamp>", stepping through
// "-2", "-3", … if that name is taken. Two quarantines within the same second
// are unlikely, but overwriting the first one would throw away the file this
// whole mechanism exists to preserve.
func uniqueQuarantinePath(path string, now time.Time) (string, error) {
	base := fmt.Sprintf("%s.corrupt-%s", path, now.UTC().Format(QuarantineTimeFormat))
	for i := 1; i <= 100; i++ {
		candidate := base
		if i > 1 {
			candidate = fmt.Sprintf("%s-%d", base, i)
		}
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return candidate, err
		}
	}
	return base, fmt.Errorf("settings: too many quarantined copies of %q already exist", path)
}

// isMalformedJSON reports whether err means "these bytes cannot be a settings
// object", as opposed to "the file could not be read".
//
// json.UnmarshalTypeError counts: a file containing a valid JSON array or
// string is well-formed JSON but still not a settings map, and is just as
// unrecoverable by retrying as a truncated file.
func isMalformedJSON(err error) bool {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &syntaxErr) || errors.As(err, &typeErr)
}

// readLocked reads and decodes the config file. The caller must hold s.mu.
// A missing file is not an error; it decodes to an empty map.
func (s *Store) readLocked() (map[string]any, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return map[string]any{}, err
	}

	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil {
		return map[string]any{}, err
	}
	if saved == nil {
		saved = map[string]any{}
	}
	return saved, nil
}

// Save atomically replaces the config file with values. See writeFileAtomic.
func (s *Store) Save(values map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Marshal before touching the filesystem so an unmarshalable payload
	// cannot leave a directory created or a temp file behind.
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}

	return writeFileAtomic(s.path, data)
}

// writeFileAtomic replaces path with data, creating the parent directory as
// 0700 and the file as 0600.
//
// The payload is written to a temp file in the same directory (same filesystem,
// so the rename is atomic), fsynced, then renamed over the target. A crash or
// error at any point leaves the previous file fully intact rather than
// truncated — these files hold every setting, and in the plaintext-file secret
// backend, the user's API keys.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	f, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	// Removed on every error path; cleared once the rename succeeds.
	defer func() {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()

	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	tmpPath = ""

	return nil
}

func (s *Store) Path() string {
	return s.path
}
