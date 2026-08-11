package state

import (
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/v2/events"
)

type testState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Maximized bool `json:"maximized"`
}

func mustNew(t *testing.T, opts ...Option[testState]) *Store[testState] {
	t.Helper()
	s, err := New[testState](opts...)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	return s
}

func TestNewRequiresPathOption(t *testing.T) {
	// Neither WithAppName nor WithStoragePath is set. Without this check,
	// Load silently returns defaults and Save fails later with a
	// confusing os.Rename error against an empty path — enforce it at
	// construction time instead.
	_, err := New[testState]()
	if err == nil {
		t.Fatal("expected error when neither WithAppName nor WithStoragePath is provided")
	}
}

func TestNewWithAppNameSucceeds(t *testing.T) {
	s, err := New[testState](WithAppName[testState]("wails-kit-state-test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Path() == "" {
		t.Fatal("expected a resolved path")
	}
}

func TestNewWithStoragePathSucceeds(t *testing.T) {
	dir := t.TempDir()
	s, err := New[testState](WithStoragePath[testState](filepath.Join(dir, "window.json")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Path() == "" {
		t.Fatal("expected a resolved path")
	}
}

func TestLoadReturnsDefaultsWhenNoFile(t *testing.T) {
	dir := t.TempDir()
	store := mustNew(t,
		WithStoragePath[testState](filepath.Join(dir, "window.json")),
		WithDefaults[testState](testState{Width: 800, Height: 600}),
	)

	s, err := store.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Width != 800 || s.Height != 600 {
		t.Fatalf("expected defaults, got %+v", s)
	}
}

func TestLoadReturnsZeroValueWhenNoFileAndNoDefaults(t *testing.T) {
	dir := t.TempDir()
	store := mustNew(t,
		WithStoragePath[testState](filepath.Join(dir, "window.json")),
	)

	s, err := store.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Width != 0 || s.Height != 0 {
		t.Fatalf("expected zero value, got %+v", s)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := mustNew(t,
		WithStoragePath[testState](filepath.Join(dir, "window.json")),
	)

	want := testState{Width: 1024, Height: 768, X: 100, Y: 50, Maximized: true}
	if err := store.Save(want); err != nil {
		t.Fatalf("save error: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestSaveCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "window.json")
	store := mustNew(t,
		WithStoragePath[testState](path),
	)

	if err := store.Save(testState{Width: 640}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "window.json")
	store := mustNew(t,
		WithStoragePath[testState](path),
	)

	if err := store.Save(testState{Width: 640}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Tmp file should not remain
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("tmp file was not cleaned up")
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "window.json")
	store := mustNew(t,
		WithStoragePath[testState](path),
	)

	if err := store.Save(testState{Width: 640}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	if err := store.Delete(); err != nil {
		t.Fatalf("delete error: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file was not deleted")
	}
}

func TestDeleteNoFileIsNoOp(t *testing.T) {
	dir := t.TempDir()
	store := mustNew(t,
		WithStoragePath[testState](filepath.Join(dir, "nonexistent.json")),
	)

	if err := store.Delete(); err != nil {
		t.Fatalf("unexpected error deleting nonexistent file: %v", err)
	}
}

func TestEventsEmitted(t *testing.T) {
	dir := t.TempDir()
	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)

	store := mustNew(t,
		WithStoragePath[testState](filepath.Join(dir, "window.json")),
		WithEmitter[testState](emitter),
		WithName[testState]("window"),
	)

	if _, err := store.Load(); err != nil {
		t.Fatalf("load error: %v", err)
	}

	if err := store.Save(testState{Width: 800}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	evts := mem.Events()
	if len(evts) != 2 {
		t.Fatalf("expected 2 events, got %d", len(evts))
	}
	if evts[0].Name != StateLoaded {
		t.Fatalf("expected %s, got %s", StateLoaded, evts[0].Name)
	}
	if evts[1].Name != StateSaved {
		t.Fatalf("expected %s, got %s", StateSaved, evts[1].Name)
	}

	payload, ok := evts[0].Data.(StateLoadedPayload)
	if !ok {
		t.Fatal("expected StateLoadedPayload")
	}
	if payload.Name != "window" {
		t.Fatalf("expected name 'window', got %q", payload.Name)
	}
}

// TestEmitOutsideLock is the regression test for the deadlock defect: Save
// (and Load) used to emit their change event while still holding the store
// lock. Emitter handlers run synchronously by default, so a handler that
// calls back into the store — a very natural thing to do in response to
// "state changed" — would deadlock forever on a non-reentrant
// sync.RWMutex. This test fails by hanging (caught via a timeout) if the
// regression reappears.
func TestEmitOutsideLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "window.json")

	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)

	store := mustNew(t,
		WithStoragePath[testState](path),
		WithEmitter[testState](emitter),
	)

	reentrantErrCh := make(chan error, 1)
	handlerDone := make(chan struct{})
	events.On(emitter, StateSaved, func(StateSavedPayload) {
		defer close(handlerDone)
		// Reentrant call from within a synchronous event handler. If Save
		// still held s.mu while emitting, this would deadlock (Load takes
		// RLock; Save's write lock is not released until after emit
		// returns, and RWMutex is not reentrant). Only Load is called here
		// — a reentrant Save would itself emit state:saved and re-trigger
		// this same handler, which tests recursion, not the lock defect.
		_, err := store.Load()
		reentrantErrCh <- err
	})

	saveDone := make(chan error, 1)
	go func() {
		saveDone <- store.Save(testState{Width: 800})
	}()

	select {
	case err := <-saveDone:
		if err != nil {
			t.Fatalf("Save returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Save deadlocked (event emitted while holding the store lock)")
	}

	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("reentrant handler deadlocked")
	}

	if err := <-reentrantErrCh; err != nil {
		t.Fatalf("reentrant store call failed: %v", err)
	}
}

func TestPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	store := mustNew(t,
		WithStoragePath[testState](path),
	)

	if store.Path() != path {
		t.Fatalf("expected %q, got %q", path, store.Path())
	}
}

func TestWithName(t *testing.T) {
	dir := t.TempDir()
	store := mustNew(t,
		WithStoragePath[testState](filepath.Join(dir, "default.json")),
		WithName[testState]("custom"),
	)

	if filepath.Base(store.Path()) != "custom.json" {
		t.Fatalf("expected custom.json, got %s", filepath.Base(store.Path()))
	}
}

// TestWithNameOrderIndependent is the regression test for the stale doc
// comment claiming WithName must precede WithAppName. The path is
// re-derived on each option application, so either order resolves to the
// same file.
func TestWithNameOrderIndependent(t *testing.T) {
	before, err := New[testState](
		WithName[testState]("custom"),
		WithAppName[testState]("wails-kit-state-test"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after, err := New[testState](
		WithAppName[testState]("wails-kit-state-test"),
		WithName[testState]("custom"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if before.Path() != after.Path() {
		t.Fatalf("expected order-independent path, got %q vs %q", before.Path(), after.Path())
	}
	if filepath.Base(before.Path()) != "custom.json" {
		t.Fatalf("expected custom.json, got %s", filepath.Base(before.Path()))
	}
}

// TestLoadCorruptedFile is the regression test for the "corrupt stamp
// bricks the app forever" defect: before the fix, Load returned
// ErrStateLoad for a corrupt file and every subsequent Load failed exactly
// the same way, with no way to recover short of a user finding and
// deleting the file by hand. Load must now quarantine the corrupt file
// (preserving its bytes for support/debugging) and recover with defaults —
// mirroring settings.Store's quarantine-on-corrupt precedent — instead of
// returning an error.
func TestLoadCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "window.json")

	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}

	store := mustNew(t,
		WithStoragePath[testState](path),
		WithDefaults[testState](testState{Width: 800, Height: 600}),
	)

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned an error for a corrupt file, want recovery with defaults: %v", err)
	}
	if got.Width != 800 || got.Height != 600 {
		t.Fatalf("Load = %+v, want defaults after recovering from corruption", got)
	}

	// The original file must be gone from its original path (moved aside,
	// not left in place) and a quarantine copy must exist preserving the
	// original bytes.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected corrupt file to be moved aside, got err = %v", err)
	}
	matches, err := filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one quarantined file, got %v", matches)
	}
	quarantinedData, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("reading quarantined file: %v", err)
	}
	if string(quarantinedData) != "not json" {
		t.Fatalf("quarantined file contents = %q, want original bytes preserved", quarantinedData)
	}

	// Loading again must not error a second time (the corrupt file is gone
	// now, so this exercises the plain "file does not exist" path) and must
	// not create a second quarantine file.
	if _, err := store.Load(); err != nil {
		t.Fatalf("second Load returned an error: %v", err)
	}
	matches, err = filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected still exactly one quarantined file after a second Load, got %v", matches)
	}
}

// TestLoadDetailedDistinguishesCorruptionFromMissing is the regression test
// for the interaction firstrun depends on: a caller that needs to tell "no
// file at all" apart from "file existed but was corrupt, now quarantined
// and reset" must be able to, since collapsing the two (as plain Load does)
// would make firstrun mistake a corrupted stamp for a fresh install.
func TestLoadDetailedDistinguishesCorruptionFromMissing(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing file is not reported as recovered", func(t *testing.T) {
		path := filepath.Join(dir, "missing.json")
		store := mustNew(t, WithStoragePath[testState](path))

		_, recovered, err := store.LoadDetailed()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if recovered {
			t.Fatal("a genuinely missing file must not report recovered = true")
		}
	})

	t.Run("corrupt file is reported as recovered", func(t *testing.T) {
		path := filepath.Join(dir, "corrupt.json")
		if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
			t.Fatal(err)
		}
		store := mustNew(t, WithStoragePath[testState](path))

		_, recovered, err := store.LoadDetailed()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !recovered {
			t.Fatal("a corrupt-and-quarantined file must report recovered = true")
		}
	})

	t.Run("a valid file is not reported as recovered", func(t *testing.T) {
		path := filepath.Join(dir, "valid.json")
		store := mustNew(t, WithStoragePath[testState](path))
		if err := store.Save(testState{Width: 1}); err != nil {
			t.Fatalf("save error: %v", err)
		}

		_, recovered, err := store.LoadDetailed()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if recovered {
			t.Fatal("a successfully-parsed file must not report recovered = true")
		}
	})
}

// TestLoadCorruptedFileEmitsStateCorrupted checks that recovering from a
// corrupt file reports the event (in addition to state:loaded) so an app
// with an emitter configured can tell the user their saved state was
// reset — quarantine-and-recover must not be silent when the app asked to
// be told.
func TestLoadCorruptedFileEmitsStateCorrupted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "window.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}

	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)
	store := mustNew(t,
		WithStoragePath[testState](path),
		WithEmitter[testState](emitter),
		WithName[testState]("window"),
	)

	if _, err := store.Load(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	evts := mem.Events()
	if len(evts) != 2 {
		t.Fatalf("expected 2 events (loaded, corrupted), got %d: %+v", len(evts), evts)
	}
	if evts[0].Name != StateLoaded {
		t.Fatalf("evts[0].Name = %s, want %s", evts[0].Name, StateLoaded)
	}
	if evts[1].Name != StateCorrupted {
		t.Fatalf("evts[1].Name = %s, want %s", evts[1].Name, StateCorrupted)
	}
	payload, ok := evts[1].Data.(StateCorruptedPayload)
	if !ok {
		t.Fatalf("evts[1].Data is %T, want StateCorruptedPayload", evts[1].Data)
	}
	if payload.Name != "window" {
		t.Fatalf("payload.Name = %q, want %q", payload.Name, "window")
	}
	if payload.Path != path {
		t.Fatalf("payload.Path = %q, want %q", payload.Path, path)
	}
	if payload.QuarantinePath == "" {
		t.Fatal("payload.QuarantinePath must be set when the rename succeeds")
	}
	if payload.Err == "" {
		t.Fatal("payload.Err must report the parse error")
	}
}

// --- Durability (fsync) regression test ---

// recordingFile wraps a real *os.File so the test can observe or fail the
// Sync() call without simulating an actual OS crash. Write/Close/Chmod/Name
// delegate to the embedded file; only Sync is intercepted. Mirrors
// settings.Store's identical test double for the same defect class.
type recordingFile struct {
	*os.File
	onSync   func()
	failSync bool
}

func (f *recordingFile) Sync() error {
	if f.onSync != nil {
		f.onSync()
	}
	if f.failSync {
		return stderrors.New("simulated fsync failure")
	}
	return f.File.Sync()
}

// TestSaveDurability is the regression test for "Save is not fsynced": the
// pre-fix Save did os.WriteFile(tmp) + os.Rename with no Sync() at all, so a
// crash between the write and the OS's own background flush could leave the
// rename's winner truncated or zero-length — losing the saved state
// entirely. Run against that pre-fix Save, the first subtest fails with
// syncCalls == 0, which is the demonstration that the bug existed; this is
// deliberately an intercept-and-assert test (not just a round-trip), since a
// round-trip test passes identically whether or not Sync is ever called.
func TestSaveDurability(t *testing.T) {
	t.Run("syncs before rename", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "window.json")
		store := mustNew(t, WithStoragePath[testState](path))

		var syncCalls int
		orig := createTempFile
		defer func() { createTempFile = orig }()
		createTempFile = func(d, pattern string) (fileHandle, error) {
			f, err := os.CreateTemp(d, pattern)
			if err != nil {
				return nil, err
			}
			return &recordingFile{File: f, onSync: func() { syncCalls++ }}, nil
		}

		if err := store.Save(testState{Width: 800}); err != nil {
			t.Fatalf("save error: %v", err)
		}
		if syncCalls != 1 {
			t.Errorf("expected exactly 1 Sync() call before rename, got %d", syncCalls)
		}

		got, err := store.Load()
		if err != nil {
			t.Fatalf("load error: %v", err)
		}
		if got.Width != 800 {
			t.Errorf("expected Width=800 after sync+rename, got %+v", got)
		}
	})

	t.Run("sync failure aborts rename and leaves no temp file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "window.json")
		store := mustNew(t, WithStoragePath[testState](path))

		orig := createTempFile
		defer func() { createTempFile = orig }()
		createTempFile = func(d, pattern string) (fileHandle, error) {
			f, err := os.CreateTemp(d, pattern)
			if err != nil {
				return nil, err
			}
			return &recordingFile{File: f, failSync: true}, nil
		}

		err := store.Save(testState{Width: 800})
		if err == nil {
			t.Fatal("expected Save to fail when fsync fails")
		}

		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Error("expected no state file to be written when fsync fails — a rename must never happen without a successful sync")
		}

		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			t.Fatalf("readdir error: %v", readErr)
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".state-") {
				t.Errorf("expected temp file to be cleaned up after a sync failure, found %s", e.Name())
			}
		}
	})

	t.Run("real fsync round-trips correctly (no mock)", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "window.json")
		store := mustNew(t, WithStoragePath[testState](path))

		if err := store.Save(testState{Width: 640}); err != nil {
			t.Fatalf("save error: %v", err)
		}
		if err := store.Save(testState{Width: 1024}); err != nil {
			t.Fatalf("save error: %v", err)
		}

		got, err := store.Load()
		if err != nil {
			t.Fatalf("load error: %v", err)
		}
		if got.Width != 1024 {
			t.Errorf("expected Width=1024 after two durable saves, got %+v", got)
		}
	})
}
