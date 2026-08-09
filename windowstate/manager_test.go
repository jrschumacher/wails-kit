package windowstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/state"
)

func newTestStore(t *testing.T, name string) *state.Store[Geometry] {
	t.Helper()
	st, err := state.New[Geometry](
		state.WithStoragePath[Geometry](filepath.Join(t.TempDir(), name+".json")),
	)
	if err != nil {
		t.Fatalf("state.New: %v", err)
	}
	return st
}

// newTestStoreWithEmitter wires the given emitter directly into the
// state.Store (not into Manager) so state:saved/state:loaded events fire —
// that only happens if the store itself was built with state.WithEmitter,
// mirroring how newManager wires WithEmitter into its auto-derived store.
func newTestStoreWithEmitter(t *testing.T, name string, e *events.Emitter) *state.Store[Geometry] {
	t.Helper()
	st, err := state.New[Geometry](
		state.WithStoragePath[Geometry](filepath.Join(t.TempDir(), name+".json")),
		state.WithEmitter[Geometry](e),
	)
	if err != nil {
		t.Fatalf("state.New: %v", err)
	}
	return st
}

// TestDebounce: a rapid stream of move/resize events (a drag-resize) must
// coalesce into a single write, not one per event. We drive this off the
// underlying state.Store's own state:saved event (via WithEmitter) rather
// than counting flush() calls directly, since that's an outcome an app
// author can actually observe too.
func TestDebounce(t *testing.T) {
	win := newFakeWindow(100, 100, 800, 600)
	screens := singleScreen1080p()
	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)
	store := newTestStoreWithEmitter(t, "debounce", emitter)

	m, err := newManager(win, screens, "", WithStore(store), WithDebounce(30*time.Millisecond), WithEmitter(emitter))
	if err != nil {
		t.Fatalf("newManager: %v", err)
	}
	defer m.Close()

	// Simulate a continuous drag-resize: many events in quick succession,
	// each well inside the debounce window.
	for i := range 20 {
		win.Move(100+i, 100+i)
		win.Resize(800+i, 600+i)
	}

	if !mem.WaitFor(state.StateSaved, time.Second) {
		t.Fatal("expected a state:saved event within 1s of the last move/resize")
	}
	// Give any (incorrect) extra saves a chance to land before asserting count.
	time.Sleep(100 * time.Millisecond)

	saved := mem.Events()
	count := 0
	for _, r := range saved {
		if r.Name == state.StateSaved {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 state:saved event from 20 coalesced move/resize events, got %d", count)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if got.X != 119 || got.Y != 119 || got.W != 819 || got.H != 619 {
		t.Errorf("expected the last geometry (119,119,819,619) to win, got %+v", got)
	}
}

// TestSkipWhileMinimised: geometry events fired while the window reports
// minimised must never reach disk — a minimised window's bounds are
// platform-dependent garbage, not something to restore next launch.
func TestSkipWhileMinimised(t *testing.T) {
	win := newFakeWindow(50, 50, 1024, 768)
	screens := singleScreen1080p()
	store := newTestStore(t, "minimised")

	m, err := newManager(win, screens, "", WithStore(store), WithDebounce(10*time.Millisecond))
	if err != nil {
		t.Fatalf("newManager: %v", err)
	}
	defer m.Close()

	// A real minimise generates a resize-shaped event; captureAndScheduleSave
	// must see IsMinimised()==true and refuse to schedule anything.
	win.Minimise()
	time.Sleep(50 * time.Millisecond)

	// Load() doesn't error on a missing file, it returns the zero value —
	// so assert directly on disk that no file was ever created.
	if _, statErr := os.Stat(store.Path()); statErr == nil {
		t.Fatal("expected no state file to be written while minimised")
	}
}

// TestMaximisedSavesRestoredGeometryNotMaximisedBounds covers bug #2: once
// maximised, Position/Size report the maximised bounds, not the window's
// "normal" size. Manager must keep saving the pre-maximise geometry plus a
// Maximised flag, not whatever the maximised bounds happen to be.
func TestMaximisedSavesRestoredGeometryNotMaximisedBounds(t *testing.T) {
	win := newFakeWindow(200, 150, 900, 700)
	screens := singleScreen1080p()
	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)
	store := newTestStoreWithEmitter(t, "maximised", emitter)

	m, err := newManager(win, screens, "", WithStore(store), WithDebounce(10*time.Millisecond), WithEmitter(emitter))
	if err != nil {
		t.Fatalf("newManager: %v", err)
	}
	defer m.Close()

	// Establish a known "restored" geometry first.
	win.Move(200, 150)
	if !mem.WaitFor(state.StateSaved, time.Second) {
		t.Fatal("expected initial save")
	}
	mem.Clear()

	// Now maximise: the fake reports maximised bounds via Size()/Position()
	// changing underneath, mimicking a real window filling the screen.
	win.Maximise()
	win.SetSize(1920, 1055) // what a real WebviewWindow would now report
	win.SetPosition(0, 25)
	win.Resize(1920, 1055) // fires the resize listener as the OS would

	if !mem.WaitFor(state.StateSaved, time.Second) {
		t.Fatal("expected a save after maximise-triggered resize event")
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if !got.Maximised {
		t.Fatal("expected Maximised=true after the window was maximised")
	}
	if got.W == 1920 || got.H == 1055 {
		t.Errorf("expected pre-maximise geometry (900x700) to be preserved, got maximised bounds %dx%d", got.W, got.H)
	}
	if got.W != 900 || got.H != 700 || got.X != 200 || got.Y != 150 {
		t.Errorf("expected saved geometry to remain the pre-maximise bounds (200,150,900,700), got %+v", got)
	}
}

// TestRestoreReappliesMaximisedFlag: Restore on a Manager loaded with a
// Maximised=true saved geometry must set the pre-maximise bounds first and
// then call Maximise(), not leave the window at the small restored size.
func TestRestoreReappliesMaximisedFlag(t *testing.T) {
	win := newFakeWindow(0, 0, 400, 300)
	screens := singleScreen1080p()
	store := newTestStore(t, "restore-maximised")
	if err := store.Save(Geometry{X: 10, Y: 10, W: 900, H: 700, Maximised: true}); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	m, err := newManager(win, screens, "", WithStore(store))
	if err != nil {
		t.Fatalf("newManager: %v", err)
	}
	defer m.Close()

	m.Restore()

	if !win.IsMaximised() {
		t.Error("expected window to be maximised after Restore")
	}
	x, y := win.Position()
	w, h := win.Size()
	if x != 10 || y != 10 || w != 900 || h != 700 {
		t.Errorf("expected pre-maximise bounds (10,10,900,700) applied before Maximise(), got (%d,%d,%d,%d)", x, y, w, h)
	}
}

// TestPerWindowKeyIsolation covers bug #4: two Managers for two named
// windows ("main", "inspector") must persist and load independently.
func TestPerWindowKeyIsolation(t *testing.T) {
	dir := t.TempDir()
	mainStore, err := state.New[Geometry](state.WithStoragePath[Geometry](filepath.Join(dir, "main.json")))
	if err != nil {
		t.Fatal(err)
	}
	inspectorStore, err := state.New[Geometry](state.WithStoragePath[Geometry](filepath.Join(dir, "inspector.json")))
	if err != nil {
		t.Fatal(err)
	}

	mainWin := newFakeWindow(0, 0, 800, 600)
	inspectorWin := newFakeWindow(0, 0, 300, 900)
	screens := singleScreen1080p()

	mainMgr, err := newManager(mainWin, screens, "", WithName("main"), WithStore(mainStore), WithDebounce(10*time.Millisecond))
	if err != nil {
		t.Fatalf("newManager(main): %v", err)
	}
	defer mainMgr.Close()

	inspectorMgr, err := newManager(inspectorWin, screens, "", WithName("inspector"), WithStore(inspectorStore), WithDebounce(10*time.Millisecond))
	if err != nil {
		t.Fatalf("newManager(inspector): %v", err)
	}
	defer inspectorMgr.Close()

	mainWin.Move(111, 222)
	inspectorWin.Move(333, 444)

	mainMgr.Close()
	inspectorMgr.Close()

	mainGeo, err := mainStore.Load()
	if err != nil {
		t.Fatalf("mainStore.Load: %v", err)
	}
	inspectorGeo, err := inspectorStore.Load()
	if err != nil {
		t.Fatalf("inspectorStore.Load: %v", err)
	}

	if mainGeo.X != 111 || mainGeo.Y != 222 {
		t.Errorf("expected main window geometry (111,222,...), got %+v", mainGeo)
	}
	if inspectorGeo.X != 333 || inspectorGeo.Y != 444 {
		t.Errorf("expected inspector window geometry (333,444,...), got %+v", inspectorGeo)
	}
	if mainGeo.W == inspectorGeo.W && mainGeo.H == inspectorGeo.H {
		t.Errorf("expected independent geometry per window key, got matching dimensions %+v vs %+v", mainGeo, inspectorGeo)
	}
}

// TestCloseFlushesPendingSaveSynchronously: a pending debounced save must
// not be lost if Close is called before the debounce timer fires (e.g. app
// quitting right after the user finishes dragging).
func TestCloseFlushesPendingSaveSynchronously(t *testing.T) {
	win := newFakeWindow(1, 2, 300, 400)
	screens := singleScreen1080p()
	store := newTestStore(t, "close-flush")

	m, err := newManager(win, screens, "", WithStore(store), WithDebounce(time.Hour)) // long enough that only Close can flush it
	if err != nil {
		t.Fatalf("newManager: %v", err)
	}

	win.Move(500, 600)
	m.Close()

	got, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if got.X != 500 || got.Y != 600 {
		t.Errorf("expected Close to flush the pending save (500,600), got %+v", got)
	}
}

// TestNoSavedGeometryLeavesWindowUntouched: Restore on a fresh store (no
// file ever written) must not move/resize the window at all.
func TestNoSavedGeometryLeavesWindowUntouched(t *testing.T) {
	win := newFakeWindow(42, 43, 640, 480)
	screens := singleScreen1080p()
	store := newTestStore(t, "never-saved")

	m, err := newManager(win, screens, "", WithStore(store))
	if err != nil {
		t.Fatalf("newManager: %v", err)
	}
	defer m.Close()

	m.Restore()

	x, y := win.Position()
	w, h := win.Size()
	if x != 42 || y != 43 || w != 640 || h != 480 {
		t.Errorf("expected window untouched at (42,43,640,480), got (%d,%d,%d,%d)", x, y, w, h)
	}
}
