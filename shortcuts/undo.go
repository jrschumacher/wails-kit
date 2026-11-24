package shortcuts

import (
	"context"
	stderrors "errors"
	"fmt"
	"sync"

	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/events"
)

// App undo and redo (frontend/ui/BEHAVIOUR.md, "Undo, confirm or neither"):
// a stack for the session, Edit > Undo and Redo named for the action
// ("Undo Archive") and enabled to match, typing undone first while a text
// field has focus, and the same stack behind a toast's Undo.

// Event names emitted by the undo stack.
const (
	// EventUndoChanged carries the stack's state (UndoState) after every
	// change: a push, an undo or redo starting or ending, a clear, or text
	// focus changing.
	EventUndoChanged = "undo:changed"
	// EventUndoText asks the focused window's text field to undo or redo
	// typing (UndoTextPayload). Edit > Undo and Redo send it, instead of
	// changing the stack, while the frontend reports text focus; the
	// frontend falls back to the stack when the field has nothing to undo.
	EventUndoText = "undo:text"
	// EventUndoFailed says an undo or redo started from the menu failed
	// (UndoFailedPayload). A call through Bindings returns the error instead.
	EventUndoFailed = "undo:failed"
)

// Error codes.
const (
	ErrUndoEmpty   errors.Code = "undo_empty"
	ErrUndoBusy    errors.Code = "undo_busy"
	ErrUndoStale   errors.Code = "undo_stale"
	ErrUndoInvalid errors.Code = "undo_invalid"
	ErrUndoFailed  errors.Code = "undo_failed"
)

func init() {
	errors.RegisterMessages(map[errors.Code]string{
		ErrUndoEmpty:   "There's nothing to undo",
		ErrUndoBusy:    "An undo is already under way",
		ErrUndoStale:   "That action can no longer be undone from here",
		ErrUndoInvalid: "The action can't be undone",
		ErrUndoFailed:  "Couldn't undo the action",
	})
}

// DefaultUndoLimit is how many actions the stack keeps by default.
const DefaultUndoLimit = 100

// Action is something the app did that can be undone. The app does it
// first, then pushes it.
type Action struct {
	// Name names the action for the menu: "Archive" reads "Undo Archive".
	Name string
	// Undo puts things back. It is required.
	Undo func(ctx context.Context) error
	// Redo does the action again after an undo. Nil means it can't be
	// redone: undoing it clears the redo stack.
	Redo func(ctx context.Context) error
}

// UndoEntry is the action at the top of the undo or redo stack.
type UndoEntry struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Label string `json:"label"` // "Undo Archive", "Redo Archive"
}

// UndoState is the stack as the frontend and the menu see it. Its shape is
// a contract shared with @wails-kit/types (UndoChangedPayload).
type UndoState struct {
	Undo      *UndoEntry `json:"undo"`      // the next action to undo; nil when there is none
	Redo      *UndoEntry `json:"redo"`      // the next action to redo; nil when there is none
	Busy      bool       `json:"busy"`      // an undo or redo is running
	TextFocus bool       `json:"textFocus"` // a text field has focus in the current window: the menu undoes typing first
}

// UndoTextPayload is EventUndoText's payload.
type UndoTextPayload struct {
	Op string `json:"op"` // "undo" or "redo"
}

// UndoError is a failed undo's error. UserMsg comes from
// errors.RegisterMessages (or the app's own UserError); Message is the raw
// error.
type UndoError struct {
	Code    errors.Code `json:"code"`
	UserMsg string      `json:"userMsg"`
	Message string      `json:"message"`
}

// UndoFailedPayload is EventUndoFailed's payload.
type UndoFailedPayload struct {
	Op    string    `json:"op"` // "undo" or "redo"
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Error UndoError `json:"error"`
}

// MenuState is what Edit > Undo and Edit > Redo show.
type MenuState struct {
	UndoLabel   string
	UndoEnabled bool
	RedoLabel   string
	RedoEnabled bool
}

// UndoOption configures an UndoStack.
type UndoOption func(*UndoStack)

// WithUndoEmitter sets the emitter for EventUndoChanged, EventUndoText and
// EventUndoFailed.
func WithUndoEmitter(e *events.Emitter) UndoOption {
	return func(s *UndoStack) { s.emitter = e }
}

// WithUndoLimit sets how many actions the stack keeps (default
// DefaultUndoLimit); the oldest go first. A limit below 1 keeps the default.
func WithUndoLimit(n int) UndoOption {
	return func(s *UndoStack) {
		if n > 0 {
			s.limit = n
		}
	}
}

// WithUndoLabels sets the menu's words, for an app in another language:
// undo and redo are fmt formats taking the action's name ("Undo %s"), and
// plainUndo and plainRedo are the labels with nothing named ("Undo").
func WithUndoLabels(undo, redo, plainUndo, plainRedo string) UndoOption {
	return func(s *UndoStack) {
		s.undoFormat, s.redoFormat, s.plainUndo, s.plainRedo = undo, redo, plainUndo, plainRedo
	}
}

type undoEntry struct {
	id     string
	seq    int
	action Action
}

// UndoStack is the app's undo and redo history for the session. It is safe
// for concurrent use. Actions run outside its lock, one at a time; the app
// may Push (or Clear) while one runs, including from inside it.
type UndoStack struct {
	mu         sync.Mutex
	emitter    *events.Emitter
	limit      int
	undo       []undoEntry
	redo       []undoEntry
	busy       bool
	textFocus  map[string]bool // by window name ("" when windows aren't known)
	seq        int
	redoEpoch  int // bumped whenever the redo stack is cleared
	clearEpoch int // bumped by Clear
	menus      []func(MenuState)
	undoFormat string
	redoFormat string
	plainUndo  string
	plainRedo  string

	// Delivery of states: one goroutine at a time sends the latest, so a
	// state that is out of date never goes out last.
	gen        int
	delivering bool

	// Window hooks, set by Apply: the window a binding call came from, the
	// window in front, and how to send undo:text to one window.
	windowOf func(context.Context) string
	current  func() string
	sendText func(window string, p UndoTextPayload) bool
}

// NewUndoStack creates an empty stack.
func NewUndoStack(opts ...UndoOption) *UndoStack {
	s := &UndoStack{limit: DefaultUndoLimit, undoFormat: "Undo %s", redoFormat: "Redo %s", plainUndo: "Undo", plainRedo: "Redo", textFocus: map[string]bool{}}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Push records an action the app has just done and returns its ID, for a
// toast's Undo (wkUndo.action(id) in the frontend). It clears the redo
// stack. It may be called while an undo or redo runs.
func (s *UndoStack) Push(a Action) (string, error) {
	if a.Name == "" || a.Undo == nil {
		return "", errors.New(ErrUndoInvalid, "an undo action needs a name and an Undo function", nil)
	}
	s.mu.Lock()
	s.seq++
	id := fmt.Sprintf("undo-%d", s.seq)
	s.undo = s.trimmed(append(s.undo, undoEntry{id: id, seq: s.seq, action: a}))
	s.redo = nil
	s.redoEpoch++
	s.mu.Unlock()
	s.changed()
	return id, nil
}

func (s *UndoStack) trimmed(list []undoEntry) []undoEntry {
	if len(list) > s.limit {
		return append([]undoEntry(nil), list[len(list)-s.limit:]...)
	}
	return list
}

func withoutID(list []undoEntry, id string) []undoEntry {
	for i, e := range list {
		if e.id == id {
			return append(list[:i:i], list[i+1:]...)
		}
	}
	return list
}

// Undo undoes the most recent action. It returns ErrUndoEmpty with nothing
// to undo, ErrUndoBusy while another undo or redo runs, and ErrUndoFailed
// (wrapping the app's error) when the action's Undo fails; a failed action
// stays on the stack.
func (s *UndoStack) Undo(ctx context.Context) error { return s.run(ctx, "undo", "") }

// Redo does the most recently undone action again. Its errors are Undo's.
func (s *UndoStack) Redo(ctx context.Context) error { return s.run(ctx, "redo", "") }

// UndoID undoes the action with this ID, only while it is the most recent:
// otherwise ErrUndoStale. A toast's Undo uses it, so it never undoes a
// later action than the one it named.
func (s *UndoStack) UndoID(ctx context.Context, id string) error { return s.run(ctx, "undo", id) }

func (s *UndoStack) run(ctx context.Context, op, id string) error {
	s.mu.Lock()
	from := s.undo
	if op == "redo" {
		from = s.redo
	}
	if s.busy {
		s.mu.Unlock()
		return errors.New(ErrUndoBusy, op+" is already under way", nil)
	}
	if len(from) == 0 {
		s.mu.Unlock()
		if id != "" {
			return errors.New(ErrUndoStale, fmt.Sprintf("%s is not the most recent action", id), nil)
		}
		return errors.New(ErrUndoEmpty, "nothing to "+op, nil)
	}
	top := from[len(from)-1]
	if id != "" && top.id != id {
		s.mu.Unlock()
		return errors.New(ErrUndoStale, fmt.Sprintf("%s is not the most recent action", id), nil)
	}
	fn := top.action.Undo
	if op == "redo" {
		fn = top.action.Redo
	}
	s.busy = true
	startSeq, redoEpoch, clearEpoch := s.seq, s.redoEpoch, s.clearEpoch
	s.mu.Unlock()
	s.changed()

	err := fn(ctx)

	s.mu.Lock()
	s.busy = false
	// Actions pushed while it ran stay where they are, above it; a Clear
	// while it ran wins.
	if err == nil && s.clearEpoch == clearEpoch {
		if op == "undo" {
			s.undo = withoutID(s.undo, top.id)
			switch {
			case top.action.Redo == nil:
				s.redo = nil // can't be redone: nothing older may be redone out of order
				s.redoEpoch++
			case s.redoEpoch == redoEpoch:
				s.redo = append(s.redo, top)
				// else a Push cleared redo while it ran: the history has moved on
			}
		} else {
			s.redo = withoutID(s.redo, top.id)
			// Back on the undo stack, below anything pushed while it ran.
			at := len(s.undo)
			for at > 0 && s.undo[at-1].seq > startSeq {
				at--
			}
			list := append([]undoEntry(nil), s.undo[:at]...)
			list = append(list, top)
			s.undo = s.trimmed(append(list, s.undo[at:]...))
		}
	}
	s.mu.Unlock()
	s.changed()
	if err != nil {
		return errors.Wrap(ErrUndoFailed, fmt.Sprintf("%s %s failed", op, top.action.Name), err)
	}
	return nil
}

// Clear empties both stacks, for when what they act on has gone (the
// document closed, the account signed out). An undo or redo running then
// leaves nothing behind when it ends.
func (s *UndoStack) Clear() {
	s.mu.Lock()
	s.undo, s.redo = nil, nil
	s.redoEpoch++
	s.clearEpoch++
	s.mu.Unlock()
	s.changed()
}

// SetTextFocus records whether a text field has focus, when the app has one
// window or windows aren't known. See SetTextFocusIn.
func (s *UndoStack) SetTextFocus(focused bool) { s.SetTextFocusIn("", focused) }

// SetTextFocusIn records whether a text field has focus in the named window.
// The frontend (wkUndo.connect) reports it through Bindings, which name the
// calling window. While the window in front has text focus, Edit > Undo
// and Redo undo typing first.
func (s *UndoStack) SetTextFocusIn(window string, focused bool) {
	s.mu.Lock()
	same := s.textFocus[window] == focused
	if focused {
		s.textFocus[window] = true
	} else {
		delete(s.textFocus, window)
	}
	s.mu.Unlock()
	if !same {
		s.changed()
	}
}

// The window in front, read outside the lock (it may call into Wails).
func (s *UndoStack) front() string {
	s.mu.Lock()
	current := s.current
	s.mu.Unlock()
	if current == nil {
		return ""
	}
	return current()
}

// setWindows sets the window hooks (Apply does, for Wails).
func (s *UndoStack) setWindows(windowOf func(context.Context) string, current func() string, sendText func(string, UndoTextPayload) bool) {
	s.mu.Lock()
	s.windowOf, s.current, s.sendText = windowOf, current, sendText
	s.mu.Unlock()
}

// State returns the stack as the frontend sees it, with TextFocus for the
// window in front.
func (s *UndoStack) State() UndoState {
	win := s.front()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateLocked(win)
}

func (s *UndoStack) stateLocked(win string) UndoState {
	st := UndoState{Busy: s.busy, TextFocus: s.textFocus[win]}
	if n := len(s.undo); n > 0 {
		e := s.undo[n-1]
		st.Undo = &UndoEntry{ID: e.id, Name: e.action.Name, Label: fmt.Sprintf(s.undoFormat, e.action.Name)}
	}
	if n := len(s.redo); n > 0 {
		e := s.redo[n-1]
		st.Redo = &UndoEntry{ID: e.id, Name: e.action.Name, Label: fmt.Sprintf(s.redoFormat, e.action.Name)}
	}
	return st
}

// Menu returns what Edit > Undo and Redo show. While a text field has
// focus in the window in front they read plain "Undo" and "Redo" and stay
// enabled, since the field may have typing to undo; while an undo runs
// both are disabled.
func (s *UndoStack) Menu() MenuState {
	win := s.front()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.menuLocked(win)
}

func (s *UndoStack) menuLocked(win string) MenuState {
	st := s.stateLocked(win)
	m := MenuState{UndoLabel: s.plainUndo, RedoLabel: s.plainRedo}
	switch {
	case st.Busy:
	case st.TextFocus:
		m.UndoEnabled, m.RedoEnabled = true, true
	default:
		if st.Undo != nil {
			m.UndoLabel, m.UndoEnabled = st.Undo.Label, true
		}
		if st.Redo != nil {
			m.RedoLabel, m.RedoEnabled = st.Redo.Label, true
		}
	}
	return m
}

// MenuUndo is what Edit > Undo does, in the window in front: with text
// focus there, it asks that window to undo typing (EventUndoText);
// otherwise it undoes the most recent action, and a failure goes out as
// EventUndoFailed. Apply wires it to the menu; an app with its own menu
// calls it from its item.
//
// Focus can leave the field between the key press and the window handling
// undo:text. The frontend then finds no field with focus and undoes the
// app's action instead, which is what the menu would have done.
func (s *UndoStack) MenuUndo(ctx context.Context) { s.MenuIn(ctx, s.front(), "undo") }

// MenuRedo is what Edit > Redo does; see MenuUndo.
func (s *UndoStack) MenuRedo(ctx context.Context) { s.MenuIn(ctx, s.front(), "redo") }

// MenuIn does Edit > Undo (op "undo") or Redo ("redo") for the named window.
func (s *UndoStack) MenuIn(ctx context.Context, window, op string) {
	if op != "redo" {
		op = "undo"
	}
	s.mu.Lock()
	text := s.textFocus[window]
	st := s.stateLocked(window)
	s.mu.Unlock()
	if text {
		s.textRequest(window, UndoTextPayload{Op: op})
		return
	}
	top := st.Undo
	if op == "redo" {
		top = st.Redo
	}
	// The item is disabled with nothing to undo or while an undo runs; its
	// shortcut then does nothing, and says nothing.
	id, name := "", ""
	if top != nil {
		id, name = top.ID, top.Name
	}
	err := s.run(ctx, op, id)
	if err != nil && !errors.IsCode(err, ErrUndoEmpty) && !errors.IsCode(err, ErrUndoStale) && !errors.IsCode(err, ErrUndoBusy) {
		s.emit(EventUndoFailed, UndoFailedPayload{Op: op, ID: id, Name: name, Error: undoErrorInfo(err)})
	}
}

// Sends undo:text to one window: through Apply's hook (the Wails window
// itself), else to the emitter's window of that name, else to every window
// (the frontend acts only in the focused one).
func (s *UndoStack) textRequest(window string, p UndoTextPayload) {
	s.mu.Lock()
	send := s.sendText
	s.mu.Unlock()
	if send != nil && send(window, p) {
		return
	}
	if s.emitter == nil {
		return
	}
	if window != "" {
		s.emitter.EmitTo(window, EventUndoText, p)
		return
	}
	s.emitter.Emit(EventUndoText, p)
}

// DOMEventUndoText is the DOM event (on the document) that carries an
// undo:text request into one window's page, where wkUndo handles it.
const DOMEventUndoText = "wk-undo-text"

// undoTextScript is the script that delivers p to one window's page.
func undoTextScript(p UndoTextPayload) string {
	op := "undo"
	if p.Op == "redo" {
		op = "redo"
	}
	return `document.dispatchEvent(new CustomEvent("` + DOMEventUndoText + `", { detail: { op: "` + op + `" } }))`
}

func undoErrorInfo(err error) UndoError {
	// The app's own UserError underneath says it best; else the stack's.
	code, msg := errors.GetCode(err), errors.GetUserMessage(err)
	var outer, inner *errors.UserError
	if stderrors.As(err, &outer) && outer.Underlying != nil && stderrors.As(outer.Underlying, &inner) {
		code, msg = inner.Code, inner.UserMsg
	}
	return UndoError{Code: code, UserMsg: msg, Message: err.Error()}
}

// OnMenu calls fn with the menu's state now and after every change. Apply
// uses it to keep Edit > Undo and Redo in step. It returns a function that
// stops it.
func (s *UndoStack) OnMenu(fn func(MenuState)) func() {
	win := s.front()
	s.mu.Lock()
	s.menus = append(s.menus, fn)
	idx := len(s.menus) - 1
	m := s.menuLocked(win)
	s.mu.Unlock()
	fn(m)
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if idx < len(s.menus) {
			s.menus[idx] = nil
		}
	}
}

// changed sends the latest state to the menus and the frontend. Only one
// goroutine delivers at a time, and it delivers again while newer changes
// came in, so the last state to go out is always the newest. A change made
// during delivery (from a menu callback or an event handler) is picked up
// by that loop, not delivered from inside it.
func (s *UndoStack) changed() {
	s.mu.Lock()
	s.gen++
	if s.delivering {
		s.mu.Unlock()
		return
	}
	s.delivering = true
	for {
		gen := s.gen
		s.mu.Unlock()
		win := s.front()
		s.mu.Lock()
		st := s.stateLocked(win)
		m := s.menuLocked(win)
		menus := append([](func(MenuState)){}, s.menus...)
		s.mu.Unlock()
		for _, fn := range menus {
			if fn != nil {
				fn(m)
			}
		}
		s.emit(EventUndoChanged, st)
		s.mu.Lock()
		if s.gen == gen {
			s.delivering = false
			s.mu.Unlock()
			return
		}
	}
}

func (s *UndoStack) emit(name string, data any) {
	if s.emitter != nil {
		s.emitter.Emit(name, data)
	}
}

// UndoBindings is the part of the stack the frontend may call. Wails v3
// binds every exported method of a registered service, so register
// Bindings() rather than the stack, whose Push takes functions:
//
//	application.NewService(stack.Bindings())
type UndoBindings struct {
	s *UndoStack
}

// Bindings returns the frontend's view of the stack.
func (s *UndoStack) Bindings() *UndoBindings { return &UndoBindings{s: s} }

// State returns the stack's state, for a reloaded window.
func (b *UndoBindings) State() UndoState { return b.s.State() }

// Undo undoes the most recent action.
func (b *UndoBindings) Undo(ctx context.Context) error { return b.s.Undo(ctx) }

// Redo does the most recently undone action again.
func (b *UndoBindings) Redo(ctx context.Context) error { return b.s.Redo(ctx) }

// UndoID undoes the action with this ID while it is the most recent.
func (b *UndoBindings) UndoID(ctx context.Context, id string) error { return b.s.UndoID(ctx, id) }

// SetTextFocus reports whether a text field has focus in the calling
// window (named by the call's context, once Apply has run).
func (b *UndoBindings) SetTextFocus(ctx context.Context, focused bool) {
	b.s.mu.Lock()
	windowOf := b.s.windowOf
	b.s.mu.Unlock()
	win := ""
	if windowOf != nil {
		win = windowOf(ctx)
	}
	b.s.SetTextFocusIn(win, focused)
}
