package shortcuts

import (
	"context"
	stderrors "errors"
	"sync"
	"testing"

	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/events"
)

// doc is a stand-in for app state: the actions archive and restore items.
type doc struct {
	mu       sync.Mutex
	archived map[string]bool
}

func (d *doc) set(name string, v bool) func(context.Context) error {
	return func(context.Context) error {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.archived[name] = v
		return nil
	}
}

func newUndoHarness(opts ...UndoOption) (*UndoStack, *events.MemoryEmitter, *doc) {
	mem := events.NewMemoryEmitter()
	s := NewUndoStack(append([]UndoOption{WithUndoEmitter(events.NewEmitter(mem))}, opts...)...)
	return s, mem, &doc{archived: map[string]bool{}}
}

func archive(d *doc, item string) Action {
	return Action{Name: "Archive", Undo: d.set(item, false), Redo: d.set(item, true)}
}

func lastState(t *testing.T, mem *events.MemoryEmitter) UndoState {
	t.Helper()
	evs := mem.Events()
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Name == EventUndoChanged {
			return evs[i].Data.(UndoState)
		}
	}
	t.Fatal("no undo:changed event")
	return UndoState{}
}

func TestUndoPushUndoRedo(t *testing.T) {
	s, mem, d := newUndoHarness()
	ctx := context.Background()
	if err := s.Undo(ctx); !errors.IsCode(err, ErrUndoEmpty) {
		t.Fatalf("Undo on an empty stack: %v, want %s", err, ErrUndoEmpty)
	}
	d.archived["a"] = true
	id, err := s.Push(archive(d, "a"))
	if err != nil || id == "" {
		t.Fatalf("Push: %q, %v", id, err)
	}
	st := lastState(t, mem)
	if st.Undo == nil || st.Undo.ID != id || st.Undo.Label != "Undo Archive" || st.Redo != nil {
		t.Fatalf("after push: %+v", st)
	}
	if err := s.Undo(ctx); err != nil {
		t.Fatal(err)
	}
	if d.archived["a"] {
		t.Error("undo did not run")
	}
	st = lastState(t, mem)
	if st.Undo != nil || st.Redo == nil || st.Redo.Label != "Redo Archive" || st.Redo.ID != id {
		t.Fatalf("after undo: %+v", st)
	}
	if err := s.Redo(ctx); err != nil {
		t.Fatal(err)
	}
	if !d.archived["a"] {
		t.Error("redo did not run")
	}
	if st := s.State(); st.Undo == nil || st.Undo.ID != id || st.Redo != nil {
		t.Fatalf("after redo: %+v", st)
	}
}

func TestUndoPushClearsRedoAndKeepsTheLimit(t *testing.T) {
	s, _, d := newUndoHarness(WithUndoLimit(2))
	ctx := context.Background()
	for _, item := range []string{"a", "b", "c"} {
		if _, err := s.Push(archive(d, item)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Undo(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Undo(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Undo(ctx); !errors.IsCode(err, ErrUndoEmpty) {
		t.Fatalf("a third undo with a limit of 2: %v", err)
	}
	if s.State().Redo == nil {
		t.Fatal("expected something to redo")
	}
	if _, err := s.Push(archive(d, "d")); err != nil {
		t.Fatal(err)
	}
	if s.State().Redo != nil {
		t.Error("a push must clear the redo stack")
	}
}

func TestUndoActionWithoutRedoClearsRedo(t *testing.T) {
	s, _, d := newUndoHarness()
	ctx := context.Background()
	_, _ = s.Push(Action{Name: "Send", Undo: d.set("sent", false)})
	_, _ = s.Push(archive(d, "a"))
	if err := s.Undo(ctx); err != nil { // Archive: can be redone
		t.Fatal(err)
	}
	if err := s.Undo(ctx); err != nil { // Send: can't
		t.Fatal(err)
	}
	// Redoing Archive now would skip Send, out of order.
	if st := s.State(); st.Redo != nil || st.Undo != nil {
		t.Fatalf("after undoing an action with no Redo: %+v", st)
	}
}

func TestUndoInvalidAction(t *testing.T) {
	s, _, d := newUndoHarness()
	for _, a := range []Action{{Undo: d.set("a", false)}, {Name: "Archive"}} {
		if _, err := s.Push(a); !errors.IsCode(err, ErrUndoInvalid) {
			t.Errorf("Push(%+v): %v, want %s", a, err, ErrUndoInvalid)
		}
	}
}

func TestUndoIDOnlyUndoesTheMostRecent(t *testing.T) {
	s, _, d := newUndoHarness()
	ctx := context.Background()
	first, _ := s.Push(archive(d, "a"))
	second, _ := s.Push(archive(d, "b"))
	if err := s.UndoID(ctx, first); !errors.IsCode(err, ErrUndoStale) {
		t.Fatalf("UndoID of an older action: %v, want %s", err, ErrUndoStale)
	}
	if err := s.UndoID(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := s.UndoID(ctx, second); !errors.IsCode(err, ErrUndoStale) {
		t.Fatalf("UndoID of an action already undone: %v", err)
	}
	s.Clear()
	if err := s.UndoID(ctx, first); !errors.IsCode(err, ErrUndoStale) {
		t.Fatalf("UndoID on an empty stack: %v", err)
	}
}

func TestUndoFailureKeepsTheAction(t *testing.T) {
	s, _, _ := newUndoHarness()
	ctx := context.Background()
	boom := stderrors.New("disk full")
	_, _ = s.Push(Action{Name: "Archive", Undo: func(context.Context) error { return boom }})
	err := s.Undo(ctx)
	if !errors.IsCode(err, ErrUndoFailed) || !stderrors.Is(err, boom) {
		t.Fatalf("Undo: %v, want %s wrapping the app's error", err, ErrUndoFailed)
	}
	if st := s.State(); st.Undo == nil || st.Busy {
		t.Fatalf("a failed undo stays on the stack, not busy: %+v", st)
	}
}

func TestUndoBusyWhileRunning(t *testing.T) {
	s, mem, _ := newUndoHarness()
	ctx := context.Background()
	started, release := make(chan struct{}), make(chan struct{})
	_, _ = s.Push(Action{Name: "Archive", Undo: func(context.Context) error { close(started); <-release; return nil }})
	done := make(chan error)
	go func() { done <- s.Undo(ctx) }()
	<-started
	if !lastState(t, mem).Busy {
		t.Error("expected a busy state while the undo runs")
	}
	if m := s.Menu(); m.UndoEnabled || m.RedoEnabled {
		t.Errorf("menu while busy: %+v", m)
	}
	if err := s.Undo(ctx); !errors.IsCode(err, ErrUndoBusy) {
		t.Errorf("a second undo while busy: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if lastState(t, mem).Busy {
		t.Error("still busy")
	}
}

func TestUndoMenuState(t *testing.T) {
	s, _, d := newUndoHarness()
	var got []MenuState
	stop := s.OnMenu(func(m MenuState) { got = append(got, m) })
	_, _ = s.Push(archive(d, "a"))
	s.SetTextFocus(true)
	s.SetTextFocus(true) // no change, no update
	s.SetTextFocus(false)
	_ = s.Undo(context.Background())
	stop()
	s.Clear()
	want := []MenuState{
		{UndoLabel: "Undo", RedoLabel: "Redo"},
		{UndoLabel: "Undo Archive", UndoEnabled: true, RedoLabel: "Redo"},
		{UndoLabel: "Undo", UndoEnabled: true, RedoLabel: "Redo", RedoEnabled: true},
		{UndoLabel: "Undo Archive", UndoEnabled: true, RedoLabel: "Redo"},
		{UndoLabel: "Undo", RedoLabel: "Redo"}, // busy
		{UndoLabel: "Undo", RedoLabel: "Redo Archive", RedoEnabled: true},
	}
	if len(got) != len(want) {
		t.Fatalf("menu updates: %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("update %d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestUndoLabels(t *testing.T) {
	s, _, d := newUndoHarness(WithUndoLabels("Annuler « %s »", "Rétablir « %s »", "Annuler", "Rétablir"))
	if m := s.Menu(); m.UndoLabel != "Annuler" || m.RedoLabel != "Rétablir" {
		t.Errorf("plain labels: %+v", m)
	}
	_, _ = s.Push(archive(d, "a"))
	if st := s.State(); st.Undo.Label != "Annuler « Archive »" {
		t.Errorf("label: %q", st.Undo.Label)
	}
}

func TestMenuUndoRoutesTextFocusToTheFrontend(t *testing.T) {
	s, mem, d := newUndoHarness()
	ctx := context.Background()
	d.archived["a"] = true
	_, _ = s.Push(archive(d, "a"))
	s.SetTextFocus(true)
	mem.Clear()
	s.MenuUndo(ctx)
	s.MenuRedo(ctx)
	evs := mem.Events()
	if len(evs) != 2 || evs[0].Name != EventUndoText || evs[0].Data.(UndoTextPayload).Op != "undo" || evs[1].Data.(UndoTextPayload).Op != "redo" {
		t.Fatalf("with text focus the menu asks the frontend: %+v", evs)
	}
	if !d.archived["a"] || s.State().Undo == nil {
		t.Fatal("with text focus the menu must not touch the app's stack")
	}
	s.SetTextFocus(false)
	s.MenuUndo(ctx)
	if d.archived["a"] {
		t.Fatal("without text focus the menu undoes the app's action")
	}
	s.MenuRedo(ctx)
	if !d.archived["a"] {
		t.Fatal("the menu's Redo redoes it")
	}
}

func TestMenuUndoFailureIsEmitted(t *testing.T) {
	s, mem, _ := newUndoHarness()
	appErr := errors.New(errors.ErrStorageWrite, "write failed", nil)
	id, _ := s.Push(Action{Name: "Archive", Undo: func(context.Context) error { return appErr }})
	mem.Clear()
	s.MenuUndo(context.Background())
	var failed *UndoFailedPayload
	for _, e := range mem.Events() {
		if e.Name == EventUndoFailed {
			p := e.Data.(UndoFailedPayload)
			failed = &p
		}
	}
	if failed == nil {
		t.Fatal("no undo:failed event")
	}
	if failed.Op != "undo" || failed.ID != id || failed.Name != "Archive" || failed.Error.Code != errors.ErrStorageWrite || failed.Error.UserMsg != appErr.UserMsg {
		t.Errorf("payload: %+v", *failed)
	}
	// Nothing to undo: the menu item is disabled, and its shortcut does nothing.
	s.Clear()
	mem.Clear()
	s.MenuUndo(context.Background())
	if n := mem.Count(); n != 0 {
		t.Errorf("events from an empty menu undo: %d", n)
	}
}

func TestUndoBindings(t *testing.T) {
	s, _, d := newUndoHarness()
	b := s.Bindings()
	ctx := context.Background()
	id, _ := s.Push(archive(d, "a"))
	if st := b.State(); st.Undo == nil || st.Undo.ID != id {
		t.Fatalf("State: %+v", st)
	}
	if err := b.UndoID(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := b.Redo(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.Undo(ctx); err != nil {
		t.Fatal(err)
	}
	b.SetTextFocus(ctx, true)
	if !s.State().TextFocus {
		t.Error("SetTextFocus")
	}
}

func TestRedoAccelerator(t *testing.T) {
	for goos, want := range map[string]string{"windows": "Ctrl+y", "darwin": "CmdOrCtrl+Shift+z", "linux": "CmdOrCtrl+Shift+z"} {
		if got := RedoAccelerator(goos); got != want {
			t.Errorf("%s: %q, want %q", goos, got, want)
		}
	}
}

func TestWithUndo(t *testing.T) {
	s := NewUndoStack()
	m := New(WithUndo(s))
	if m.undo != s || !m.editMenu {
		t.Fatal("WithUndo sets the stack and the Edit menu")
	}
	if m := New(WithUndo(nil)); m.editMenu {
		t.Error("WithUndo(nil) adds nothing")
	}
}

// An action pushed while an undo runs stays, and the undone action is undone
// once: re-entrant (from inside Undo) and from another goroutine.
func TestPushDuringUndo(t *testing.T) {
	for _, mode := range []string{"re-entrant", "goroutine"} {
		t.Run(mode, func(t *testing.T) {
			s, _, _ := newUndoHarness()
			ctx := context.Background()
			var undone int
			var mu sync.Mutex
			started, release := make(chan struct{}), make(chan struct{})
			_, _ = s.Push(Action{Name: "Archive",
				Undo: func(context.Context) error {
					mu.Lock()
					undone++
					mu.Unlock()
					if mode == "re-entrant" {
						_, _ = s.Push(Action{Name: "Rename", Undo: func(context.Context) error { return nil }})
						return nil
					}
					close(started)
					<-release
					return nil
				},
				Redo: func(context.Context) error { return nil }})
			done := make(chan error, 1)
			go func() { done <- s.Undo(ctx) }()
			if mode == "goroutine" {
				<-started
				if _, err := s.Push(Action{Name: "Rename", Undo: func(context.Context) error { return nil }}); err != nil {
					t.Fatal(err)
				}
				close(release)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			st := s.State()
			if st.Undo == nil || st.Undo.Name != "Rename" {
				t.Fatalf("the pushed action must stay on top: %+v", st)
			}
			if st.Redo != nil {
				t.Fatalf("the push cleared redo while the undo ran: %+v", st.Redo)
			}
			// Undo Rename, then nothing: Archive must not be undone again.
			if err := s.Undo(ctx); err != nil {
				t.Fatal(err)
			}
			if err := s.Undo(ctx); !errors.IsCode(err, ErrUndoEmpty) {
				t.Fatalf("Archive is still on the stack: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if undone != 1 {
				t.Fatalf("Archive undone %d times", undone)
			}
		})
	}
}

// An action pushed while a redo runs stays, and the redone action goes back
// on the undo stack below it.
func TestPushDuringRedo(t *testing.T) {
	for _, mode := range []string{"re-entrant", "goroutine"} {
		t.Run(mode, func(t *testing.T) {
			s, _, _ := newUndoHarness()
			ctx := context.Background()
			started, release := make(chan struct{}), make(chan struct{})
			_, _ = s.Push(Action{Name: "Archive",
				Undo: func(context.Context) error { return nil },
				Redo: func(context.Context) error {
					if mode == "re-entrant" {
						_, _ = s.Push(Action{Name: "Rename", Undo: func(context.Context) error { return nil }})
						return nil
					}
					close(started)
					<-release
					return nil
				}})
			if err := s.Undo(ctx); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- s.Redo(ctx) }()
			if mode == "goroutine" {
				<-started
				_, _ = s.Push(Action{Name: "Rename", Undo: func(context.Context) error { return nil }})
				close(release)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			var names []string
			for {
				st := s.State()
				if st.Undo == nil {
					break
				}
				names = append(names, st.Undo.Name)
				if err := s.Undo(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if len(names) != 2 || names[0] != "Rename" || names[1] != "Archive" {
				t.Fatalf("undo order after a push during redo: %v, want [Rename Archive]", names)
			}
		})
	}
}

// A Clear while an undo runs wins: nothing comes back.
func TestClearDuringUndo(t *testing.T) {
	s, _, _ := newUndoHarness()
	_, _ = s.Push(Action{Name: "Archive",
		Undo: func(context.Context) error { s.Clear(); return nil },
		Redo: func(context.Context) error { return nil }})
	if err := s.Undo(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := s.State(); st.Undo != nil || st.Redo != nil {
		t.Fatalf("after a clear during undo: %+v", st)
	}
}

// The menu says nothing when there is nothing to undo or an undo runs.
func TestMenuUndoQuietWhenEmptyOrBusy(t *testing.T) {
	s, mem, _ := newUndoHarness()
	failed := func() int {
		n := 0
		for _, e := range mem.Events() {
			if e.Name == EventUndoFailed {
				n++
			}
		}
		return n
	}
	ctx := context.Background()
	s.MenuUndo(ctx)
	s.MenuRedo(ctx)
	if n := failed(); n != 0 {
		t.Fatalf("undo:failed on an empty stack: %d", n)
	}
	started, release := make(chan struct{}), make(chan struct{})
	_, _ = s.Push(Action{Name: "Archive", Undo: func(context.Context) error { close(started); <-release; return nil }})
	done := make(chan error, 1)
	go func() { done <- s.Undo(ctx) }()
	<-started
	s.MenuUndo(ctx)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := failed(); n != 0 {
		t.Fatalf("undo:failed while busy: %d", n)
	}
}

// Text focus belongs to a window: the menu sends undo:text to the window in
// front only when that window reported it.
func TestTextFocusPerWindow(t *testing.T) {
	s, mem, d := newUndoHarness()
	ctx := context.Background()
	d.archived["a"] = true
	_, _ = s.Push(archive(d, "a"))
	front := "main"
	var sent []string
	s.setWindows(
		func(ctx context.Context) string { name, _ := ctx.Value(ctxWindow{}).(string); return name },
		func() string { return front },
		func(window string, p UndoTextPayload) bool { sent = append(sent, window+":"+p.Op); return true },
	)
	b := s.Bindings()
	b.SetTextFocus(context.WithValue(ctx, ctxWindow{}, "inspector"), true)
	if s.State().TextFocus || s.Menu().UndoLabel != "Undo Archive" {
		t.Fatal("text focus in another window must not change the menu of the window in front")
	}
	s.MenuUndo(ctx)
	if len(sent) != 0 || d.archived["a"] {
		t.Fatalf("with no text focus in front, the menu undoes the action: sent %v", sent)
	}
	_ = s.Redo(ctx)
	front = "inspector"
	mem.Clear()
	s.MenuUndo(ctx)
	if len(sent) != 1 || sent[0] != "inspector:undo" || !d.archived["a"] {
		t.Fatalf("undo:text goes to the window with text focus: %v", sent)
	}
	if n := mem.Count(); n != 0 {
		t.Fatalf("undo:text went through the emitter too (%d events)", n)
	}
	b.SetTextFocus(context.WithValue(ctx, ctxWindow{}, "inspector"), false)
	if s.State().TextFocus {
		t.Fatal("text focus cleared")
	}
}

type ctxWindow struct{}

// Without Apply's hook, undo:text goes to the emitter's window of that name.
func TestTextRequestThroughEmitter(t *testing.T) {
	mem := events.NewMemoryEmitter()
	em := events.NewEmitter(mem)
	other := events.NewMemoryEmitter()
	em.RegisterWindow("inspector", other.MemoryWindow("inspector"))
	s := NewUndoStack(WithUndoEmitter(em))
	s.setWindows(nil, func() string { return "inspector" }, nil)
	s.SetTextFocusIn("inspector", true)
	mem.Clear()
	s.MenuUndo(context.Background())
	for _, e := range mem.Events() {
		if e.Name == EventUndoText {
			t.Fatal("undo:text was broadcast")
		}
	}
	if other.Count() != 1 || other.Last().Name != EventUndoText {
		t.Fatalf("undo:text to the window: %+v", other.Events())
	}
	if got := undoTextScript(UndoTextPayload{Op: "redo"}); got != `document.dispatchEvent(new CustomEvent("wk-undo-text", { detail: { op: "redo" } }))` {
		t.Errorf("script: %s", got)
	}
}

// The last state delivered is the newest: many pushes at once, and a
// change made from a menu callback, end with the final state.
func TestChangedDeliversTheNewestLast(t *testing.T) {
	s, mem, _ := newUndoHarness()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.Push(Action{Name: "Archive", Undo: func(context.Context) error { return nil }})
		}()
	}
	wg.Wait()
	if got, want := lastState(t, mem), s.State(); got.Undo == nil || got.Undo.ID != want.Undo.ID {
		t.Fatalf("last event %+v, state %+v", got.Undo, want.Undo)
	}
	// Re-entrant: a menu callback that changes the stack while a change is
	// being delivered doesn't deadlock, and its change goes out last.
	s2, mem2, _ := newUndoHarness()
	var menu []MenuState
	armed, cleared := false, false
	s2.OnMenu(func(m MenuState) {
		menu = append(menu, m)
		if armed && !cleared && m.UndoEnabled {
			cleared = true
			s2.Clear()
		}
	})
	armed = true
	_, _ = s2.Push(Action{Name: "Archive", Undo: func(context.Context) error { return nil }})
	if !cleared {
		t.Fatal("the callback never ran with the pushed state")
	}
	if st := lastState(t, mem2); st.Undo != nil {
		t.Fatalf("the clear made from the callback is the last state: %+v", st)
	}
	if last := menu[len(menu)-1]; last.UndoEnabled {
		t.Fatalf("menu ends enabled: %+v", menu)
	}
}

// A delivery held up (a slow backend) doesn't send its older state after a
// newer one made meanwhile on another goroutine.
func TestChangedNeverDeliversAStaleStateLast(t *testing.T) {
	var mu sync.Mutex
	var got []UndoState
	entered, release := make(chan struct{}), make(chan struct{})
	first := true
	backend := events.BackendFunc(func(name string, data any) {
		if name != EventUndoChanged {
			return
		}
		mu.Lock()
		block := first
		first = false
		mu.Unlock()
		if block {
			close(entered)
			<-release
		}
		mu.Lock()
		got = append(got, data.(UndoState))
		mu.Unlock()
	})
	s := NewUndoStack(WithUndoEmitter(events.NewEmitter(backend)))
	done := make(chan struct{})
	go func() {
		_, _ = s.Push(Action{Name: "Archive", Undo: func(context.Context) error { return nil }})
		close(done)
	}()
	<-entered
	_, _ = s.Push(Action{Name: "Rename", Undo: func(context.Context) error { return nil }})
	close(release)
	<-done
	mu.Lock()
	defer mu.Unlock()
	if last := got[len(got)-1]; last.Undo == nil || last.Undo.Name != "Rename" {
		t.Fatalf("last state delivered: %+v", last.Undo)
	}
}
