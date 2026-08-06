package settings

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestStore builds a Store at an explicit path, which keeps every test off
// the real user config directory.
func newTestStore(t *testing.T, path string) *Store {
	t.Helper()
	s, err := NewStore("app", WithPath(path))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

// redirectUserConfigDir points os.UserConfigDir at a temp directory for the
// duration of the test, so nothing in the suite can touch the developer's real
// application-support directory. The env var differs per platform.
func redirectUserConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", dir)
		return dir
	case "darwin":
		// os.UserConfigDir is $HOME/Library/Application Support on darwin.
		t.Setenv("HOME", dir)
		return filepath.Join(dir, "Library", "Application Support")
	default:
		t.Setenv("XDG_CONFIG_HOME", dir)
		return dir
	}
}

// The default location must be the OS convention for application data
// (~/Library/Application Support, %AppData%, ~/.config), not a CLI-style
// dotdir in $HOME.
func TestNewStore_DefaultPathIsPlatformConfigDir(t *testing.T) {
	configDir := redirectUserConfigDir(t)

	s, err := NewStore("myapp")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	expected := filepath.Join(configDir, "myapp", "settings.json")
	if s.Path() != expected {
		t.Errorf("expected path %q, got %q", expected, s.Path())
	}

	home, _ := os.UserHomeDir()
	if strings.Contains(s.Path(), filepath.Join(home, ".myapp")) {
		t.Errorf("default path must not be a dotdir in $HOME, got %q", s.Path())
	}
}

// A failure to resolve the config dir previously produced a silent relative,
// cwd-dependent path via `home, _ := os.UserHomeDir()`.
func TestNewStore_ReturnsErrorWhenConfigDirUnresolvable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unsetting AppData is not a reliable failure trigger on windows")
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	s, err := NewStore("myapp")
	if err == nil {
		t.Fatalf("expected an error when the config dir cannot be resolved, got store with path %q", s.Path())
	}
	if s != nil {
		t.Errorf("expected a nil store alongside the error, got %+v", s)
	}
}

// WithPath remains the escape hatch and must not consult the environment at
// all, so an unresolvable config dir is irrelevant when a path is supplied.
func TestNewStore_WithPath(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	custom := filepath.Join(t.TempDir(), "custom", "settings.json")
	s, err := NewStore("myapp", WithPath(custom))
	if err != nil {
		t.Fatalf("NewStore with explicit path must not fail: %v", err)
	}
	if s.Path() != custom {
		t.Errorf("expected path %q, got %q", custom, s.Path())
	}
}

func TestLoad_DefaultsWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent", "settings.json")
	s := newTestStore(t, path)
	s.SetDefaults(map[string]any{
		"theme": "dark",
		"count": 42,
	})

	values, err := s.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values["theme"] != "dark" {
		t.Errorf("expected theme=dark, got %v", values["theme"])
	}
	if values["count"] != 42 {
		t.Errorf("expected count=42, got %v", values["count"])
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s := newTestStore(t, path)

	input := map[string]any{
		"name":    "test",
		"enabled": true,
		"count":   float64(7),
	}
	if err := s.Save(input); err != nil {
		t.Fatalf("save error: %v", err)
	}

	values, err := s.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if values["name"] != "test" {
		t.Errorf("expected name=test, got %v", values["name"])
	}
	if values["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", values["enabled"])
	}
	if values["count"] != float64(7) {
		t.Errorf("expected count=7, got %v", values["count"])
	}
}

func TestLoad_SavedValuesOverrideDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s := newTestStore(t, path)
	s.SetDefaults(map[string]any{
		"theme":    "light",
		"language": "en",
	})

	if err := s.Save(map[string]any{"theme": "dark"}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	values, err := s.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if values["theme"] != "dark" {
		t.Errorf("expected saved theme=dark to override default, got %v", values["theme"])
	}
	if values["language"] != "en" {
		t.Errorf("expected default language=en to remain, got %v", values["language"])
	}
}

func TestSave_DirectoryPermissions(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "newdir")
	path := filepath.Join(subdir, "settings.json")
	s := newTestStore(t, path)

	if err := s.Save(map[string]any{"key": "value"}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	info, err := os.Stat(subdir)
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("expected directory permissions 0700, got %04o", perm)
	}
}

func TestSave_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s := newTestStore(t, path)

	if err := s.Save(map[string]any{"key": "value"}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected file permissions 0600, got %04o", perm)
	}
}

// --- Corrupt config quarantine ---

// A config file that cannot be parsed must not become an unrecoverable dead
// end. Loading falls back to defaults so the app stays usable, and the caller
// is told so it can explain the reset to the user.
func TestLoad_InvalidJSONIsQuarantined(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	original := []byte("{invalid json")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	s := newTestStore(t, path)
	s.SetDefaults(map[string]any{"fallback": "yes"})

	values, err := s.Load()
	if err != nil {
		t.Fatalf("expected corrupt file to be recovered from, got error: %v", err)
	}
	if values["fallback"] != "yes" {
		t.Errorf("expected defaults after quarantine, got %v", values["fallback"])
	}

	c := s.Corruption()
	if c == nil {
		t.Fatal("expected Corruption() to report the quarantine, got nil")
	}
	if c.Path != path {
		t.Errorf("expected Path %q, got %q", path, c.Path)
	}
	if !strings.HasPrefix(c.QuarantinePath, path+".corrupt-") {
		t.Errorf("expected quarantine path alongside the original, got %q", c.QuarantinePath)
	}
	if c.Reason == "" {
		t.Error("expected a decoding reason on the report")
	}
	if c.DetectedAt.IsZero() {
		t.Error("expected DetectedAt to be set")
	}

	// The whole point of quarantining rather than deleting: the original bytes
	// must still be there for manual recovery.
	preserved, err := os.ReadFile(c.QuarantinePath)
	if err != nil {
		t.Fatalf("reading quarantined file: %v", err)
	}
	if !bytes.Equal(preserved, original) {
		t.Errorf("quarantined file was modified: got %q, want %q", preserved, original)
	}

	// The corrupt file is gone from the live path, so the app can save again.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected the corrupt file to be moved off %q, stat err: %v", path, err)
	}
}

// The timestamp suffix must be fixed-width and lexically sortable, so a
// directory listing shows quarantined copies oldest-first.
func TestQuarantinePath_TimestampIsSortable(t *testing.T) {
	earlier := time.Date(2026, 8, 5, 9, 4, 5, 0, time.UTC)
	later := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)

	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	a, err := uniqueQuarantinePath(path, earlier)
	if err != nil {
		t.Fatalf("uniqueQuarantinePath: %v", err)
	}
	b, err := uniqueQuarantinePath(path, later)
	if err != nil {
		t.Fatalf("uniqueQuarantinePath: %v", err)
	}

	if a != path+".corrupt-20260805T090405Z" {
		t.Errorf("unexpected quarantine name %q", a)
	}
	if !(a < b) {
		t.Errorf("expected %q to sort before %q", a, b)
	}
}

// A second corruption must not overwrite the copy kept from the first.
func TestQuarantine_DoesNotClobberAnExistingCopy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	for i, payload := range []string{"{first", "{second"} {
		if err := os.WriteFile(path, []byte(payload), 0600); err != nil {
			t.Fatalf("write error: %v", err)
		}
		s := newTestStore(t, path)
		if _, err := s.Load(); err != nil {
			t.Fatalf("load %d: %v", i, err)
		}
	}

	entries, err := filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 quarantined copies, got %d: %v", len(entries), entries)
	}

	var contents []string
	for _, e := range entries {
		data, err := os.ReadFile(e)
		if err != nil {
			t.Fatalf("read %s: %v", e, err)
		}
		contents = append(contents, string(data))
	}
	slices.Sort(contents)
	if !slices.Equal(contents, []string{"{first", "{second"}) {
		t.Errorf("expected both corrupt payloads preserved, got %v", contents)
	}
}

// Well-formed JSON that is not an object is just as unrecoverable as a
// truncated file: no retry will ever turn it into a settings map.
func TestLoad_WellFormedJSONThatIsNotAnObjectIsQuarantined(t *testing.T) {
	for _, payload := range []string{`[1, 2, 3]`, `"a string"`, `42`} {
		t.Run(payload, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.json")
			if err := os.WriteFile(path, []byte(payload), 0600); err != nil {
				t.Fatalf("write error: %v", err)
			}

			s := newTestStore(t, path)
			if _, err := s.Load(); err != nil {
				t.Fatalf("expected quarantine, got error: %v", err)
			}
			if s.Corruption() == nil {
				t.Fatalf("expected %q to be quarantined", payload)
			}
		})
	}
}

// "null" is valid JSON and decodes to an absent-but-fine settings map. It must
// be treated as empty, not quarantined.
func TestLoad_JSONNullIsNotCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("null"), 0600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	s := newTestStore(t, path)
	s.SetDefaults(map[string]any{"fallback": "yes"})
	values, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Corruption() != nil {
		t.Error("expected JSON null to be treated as an empty settings file, not corruption")
	}
	if values["fallback"] != "yes" {
		t.Errorf("expected defaults, got %v", values)
	}
}

// A file that cannot be read at all is a transient failure: the user's settings
// are intact and a retry (or a permissions fix) recovers them. Destroying them
// would be exactly the wrong answer, so it stays an error.
func TestLoad_UnreadableFileIsAnErrorNotAQuarantine(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission bits are not enforced")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"a": 1}`), 0000); err != nil {
		t.Fatalf("write error: %v", err)
	}

	s := newTestStore(t, path)
	s.SetDefaults(map[string]any{"fallback": "yes"})

	values, err := s.Load()
	if err == nil {
		t.Fatal("expected a permission error to surface, got nil")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("expected a permission error, got %T: %v", err, err)
	}
	if s.Corruption() != nil {
		t.Error("a permission error must never quarantine the user's settings")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("the config file must be left where it is, stat err: %v", statErr)
	}
	if values["fallback"] != "yes" {
		t.Errorf("expected defaults alongside the error, got %v", values)
	}
}

// If the file cannot be moved aside, continuing on defaults would let the very
// next Save overwrite the only copy of the user's settings. That must be an
// error instead.
func TestQuarantine_FailureToMoveAsideIsAnError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; directory permissions are not enforced")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("{corrupt"), 0600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	s := newTestStore(t, path)
	if _, err := s.Load(); err == nil {
		t.Fatal("expected an error when the corrupt file cannot be moved aside")
	}
	if s.Corruption() != nil {
		t.Error("a failed quarantine must not be reported as a completed one")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the corrupt file must be left in place, stat err: %v", err)
	}
}

// Concurrent readers must produce exactly one quarantine, not one per reader.
func TestQuarantine_ConcurrentLoadsQuarantineOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("{corrupt"), 0600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	s := newTestStore(t, path)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = s.Load()
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	entries, err := filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected exactly 1 quarantined copy, got %d: %v", len(entries), entries)
	}
}

// --- Fix #2: Save must be atomic ---

// A second Store on the same path stands in for another process reading the
// config while a write is in flight. os.WriteFile truncates in place, so a
// reader can observe an empty or partial file; an atomic rename cannot be seen.
func TestSave_ConcurrentReaderNeverSeesPartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	writer := newTestStore(t, path)
	reader := newTestStore(t, path)

	payload := make(map[string]any, 200)
	for i := 0; i < 200; i++ {
		payload[fmt.Sprintf("key%03d", i)] = strings.Repeat("v", 256)
	}
	if err := writer.Save(payload); err != nil {
		t.Fatalf("seed save error: %v", err)
	}

	var mu sync.Mutex
	var readErr error
	record := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if readErr == nil {
			readErr = err
		}
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for r := 0; r < 2; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				values, err := reader.Load()
				if err != nil {
					record(fmt.Errorf("load error mid-write: %w", err))
					return
				}
				if len(values) != len(payload) {
					record(fmt.Errorf("read %d keys, want %d", len(values), len(payload)))
					return
				}
			}
		}()
	}

	for i := 0; i < 400; i++ {
		if err := writer.Save(payload); err != nil {
			close(stop)
			wg.Wait()
			t.Fatalf("save error: %v", err)
		}
	}
	close(stop)
	wg.Wait()

	if readErr != nil {
		t.Fatalf("reader observed a non-atomic write: %v", readErr)
	}
}

func TestSave_LeavesNoTempFilesBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s := newTestStore(t, path)

	for i := 0; i < 3; i++ {
		if err := s.Save(map[string]any{"key": "value"}); err != nil {
			t.Fatalf("save error: %v", err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "settings.json" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("expected only settings.json in config dir, got %v", names)
	}
}

func TestSave_MarshalFailureLeavesLiveConfigIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s := newTestStore(t, path)

	if err := s.Save(map[string]any{"secret": "keep-me"}); err != nil {
		t.Fatalf("seed save error: %v", err)
	}

	// chan is not JSON-marshalable: the write must fail before the live file is touched.
	if err := s.Save(map[string]any{"secret": "keep-me", "bad": make(chan int)}); err == nil {
		t.Fatal("expected marshal error, got nil")
	}

	values, err := s.Load()
	if err != nil {
		t.Fatalf("load error after failed save: %v", err)
	}
	if values["secret"] != "keep-me" {
		t.Errorf("expected live config to be intact, got %v", values)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir error: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("expected no temp files after failed save, got %v", names)
	}
}
