package state

import (
	"os"
	"path/filepath"
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

func TestLoadCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "window.json")

	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}

	store := mustNew(t,
		WithStoragePath[testState](path),
	)

	_, err := store.Load()
	if err == nil {
		t.Fatal("expected error for corrupted file")
	}
}
