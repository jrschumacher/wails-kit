package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jrschumacher/wails-kit/events"
)

type testState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Maximized bool `json:"maximized"`
}

func TestLoadReturnsDefaultsWhenNoFile(t *testing.T) {
	dir := t.TempDir()
	store := New[testState](
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
	store := New[testState](
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
	store := New[testState](
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
	store := New[testState](
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
	store := New[testState](
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
	store := New[testState](
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
	store := New[testState](
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

	store := New[testState](
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

func TestPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	store := New[testState](
		WithStoragePath[testState](path),
	)

	if store.Path() != path {
		t.Fatalf("expected %q, got %q", path, store.Path())
	}
}

func TestWithName(t *testing.T) {
	dir := t.TempDir()
	store := New[testState](
		WithStoragePath[testState](filepath.Join(dir, "default.json")),
		WithName[testState]("custom"),
	)

	if filepath.Base(store.Path()) != "custom.json" {
		t.Fatalf("expected custom.json, got %s", filepath.Base(store.Path()))
	}
}

func TestLoadCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "window.json")

	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}

	store := New[testState](
		WithStoragePath[testState](path),
	)

	_, err := store.Load()
	if err == nil {
		t.Fatal("expected error for corrupted file")
	}
}
