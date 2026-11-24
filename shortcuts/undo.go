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
	TextFocus bool       `json:"textFocus"` // a text field has focus: the menu undoes typing first
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
	action Action
}

// UndoStack is the app's undo and redo history for the session. It is safe
// for concurrent use. Actions run outside its lock, one at a time.
type UndoStack struct {
	mu         sync.Mutex
	emitter    *events.Emitter
	limit      int
	undo       []undoEntry
	redo       []undoEntry
	busy       bool
	textFocus  bool
	seq        int
	menus      []func(MenuState)
	undoFormat string
	redoFormat string
	plainUndo  string
	plainRedo  string
}

// NewUndoStack creates an empty stack.
func NewUndoStack(opts ...UndoOption) *UndoStack {
	s := &UndoStack{limit: DefaultUndoLimit, undoFormat: "Undo %s", redoFormat: "Redo %s", plainUndo: "Undo", plainRedo: "Redo"}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Push records an action the app has just done and returns its ID, for a
// toast's Undo (wkUndo.action(id) in the frontend). It clears the redo
// stack.
func (s *UndoStack) Push(a Action) (string, error) {
	if a.Name == "" || a.Undo == nil {
		return "", errors.New(ErrUndoInvalid, "an undo action needs a name and an Undo function", nil)
	}
	s.mu.Lock()
	s.seq++
	id := fmt.Sprintf("undo-%d", s.seq)
	s.undo = append(s.undo, undoEntry{id: id, action: a})
	if len(s.undo) > s.limit {
		s.undo = append([]undoEntry(nil), s.undo[len(s.undo)-s.limit:]...)
	}
	s.redo = nil
	s.mu.Unlock()
	s.changed()
	return id, nil
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
	from := &s.undo
	if op == "redo" {
		from = &s.redo
	}
	if s.busy {
		s.mu.Unlock()
		return errors.New(ErrUndoBusy, op+" is already under way", nil)
	}
	if len(*from) == 0 {
		s.mu.Unlock()
		if id != "" {
			return errors.New(ErrUndoStale, fmt.Sprintf("%s is not the most recent action", id), nil)
		}
		return errors.New(ErrUndoEmpty, "nothing to "+op, nil)
	}
	top := (*from)[len(*from)-1]
	if id != "" && top.id != id {
		s.mu.Unlock()
		return errors.New(ErrUndoStale, fmt.Sprintf("%s is not the most recent action", id), nil)
	}
	fn := top.action.Undo
	if op == "redo" {
		fn = top.action.Redo
	}
	s.busy = true
	s.mu.Unlock()
	s.changed()

	err := fn(ctx)

	s.mu.Lock()
	s.busy = false
	if err == nil {
		// The entry is still on top: nothing else changes the stack while busy,
		// except Clear, which empties it.
		if n := len(*from); n > 0 && (*from)[n-1].id == top.id {
			*from = (*from)[:n-1]
			if op == "undo" {
				if top.action.Redo != nil {
					s.redo = append(s.redo, top)
				} else {
					s.redo = nil
				}
			} else {
				s.undo = append(s.undo, top)
			}
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
// document closed, the account signed out).
func (s *UndoStack) Clear() {
	s.mu.Lock()
	s.undo, s.redo = nil, nil
	s.mu.Unlock()
	s.changed()
}

// SetTextFocus records whether a text field has focus. The frontend
// (wkUndo.connect) reports it. While it does, Edit > Undo and Redo undo
// typing first.
func (s *UndoStack) SetTextFocus(focused bool) {
	s.mu.Lock()
	same := s.textFocus == focused
	s.textFocus = focused
	s.mu.Unlock()
	if !same {
		s.changed()
	}
}

// State returns the stack as the frontend sees it.
func (s *UndoStack) State() UndoState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateLocked()
}

func (s *UndoStack) stateLocked() UndoState {
	st := UndoState{Busy: s.busy, TextFocus: s.textFocus}
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
// focus they read plain "Undo" and "Redo" and stay enabled, since the
// field may have typing to undo; while an undo runs both are disabled.
func (s *UndoStack) Menu() MenuState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.menuLocked()
}

func (s *UndoStack) menuLocked() MenuState {
	st := s.stateLocked()
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

// MenuUndo is what Edit > Undo does: with text focus, it asks the frontend
// to undo typing (EventUndoText); otherwise it undoes the most recent
// action, and a failure goes out as EventUndoFailed. Apply wires it to the
// menu; an app with its own menu calls it from its item.
func (s *UndoStack) MenuUndo(ctx context.Context) { s.menu(ctx, "undo") }

// MenuRedo is what Edit > Redo does; see MenuUndo.
func (s *UndoStack) MenuRedo(ctx context.Context) { s.menu(ctx, "redo") }

func (s *UndoStack) menu(ctx context.Context, op string) {
	s.mu.Lock()
	text := s.textFocus
	st := s.stateLocked()
	s.mu.Unlock()
	if text {
		s.emit(EventUndoText, UndoTextPayload{Op: op})
		return
	}
	top := st.Undo
	if op == "redo" {
		top = st.Redo
	}
	if top == nil || st.Busy {
		return // the item is disabled; a shortcut for it does nothing
	}
	var err error
	if op == "redo" {
		err = s.run(ctx, op, top.ID)
	} else {
		err = s.UndoID(ctx, top.ID)
	}
	if err != nil && !errors.IsCode(err, ErrUndoStale) && !errors.IsCode(err, ErrUndoBusy) {
		s.emit(EventUndoFailed, UndoFailedPayload{Op: op, ID: top.ID, Name: top.Name, Error: undoErrorInfo(err)})
	}
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
	s.mu.Lock()
	s.menus = append(s.menus, fn)
	idx := len(s.menus) - 1
	m := s.menuLocked()
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

func (s *UndoStack) changed() {
	s.mu.Lock()
	st := s.stateLocked()
	m := s.menuLocked()
	menus := append([](func(MenuState)){}, s.menus...)
	s.mu.Unlock()
	for _, fn := range menus {
		if fn != nil {
			fn(m)
		}
	}
	s.emit(EventUndoChanged, st)
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

// SetTextFocus reports whether a text field has focus.
func (b *UndoBindings) SetTextFocus(focused bool) { b.s.SetTextFocus(focused) }
