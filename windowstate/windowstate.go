// Package windowstate persists window geometry (position, size, and
// maximised state) across app restarts and restores it safely — clamped to
// a currently attached display so a window saved on an external monitor
// never comes back off-screen when that monitor is unplugged.
//
// This is Wails-importing (AD-4 allowlist: shortcuts, windowstate,
// permissions, kit/wailsbridge). It builds headlessly on every platform
// (no build tag — AD-4 explicitly replaced that pattern) but Manage/Restore
// only do anything useful against a real *application.WebviewWindow, which
// needs a live display/session to run.
package windowstate

import (
	"sync"
	"time"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/state"
)

// Error codes for the windowstate package.
const (
	ErrConfig errors.Code = "windowstate_config"
	ErrSave   errors.Code = "windowstate_save"
	ErrLoad   errors.Code = "windowstate_load"
)

func init() {
	errors.RegisterMessages(map[errors.Code]string{
		ErrConfig: "Window state is misconfigured. Please contact support.",
		ErrSave:   "Failed to save window position. Please try again.",
		ErrLoad:   "Failed to restore window position. Please try again.",
	})
}

// Event names emitted by the windowstate package. Saving itself is
// reported by the underlying state.Store's own state:saved/state:loaded
// events when an emitter is wired (see WithEmitter) — windowstate adds two
// events of its own: one for the outcome of Restore (including whether the
// saved geometry had to be clamped), and one for save/load failures, which
// otherwise happen silently on a background debounce timer with nowhere
// else to surface.
const (
	// EventRestored fires after Restore applies geometry to the window.
	// Payload: RestoredPayload.
	EventRestored = "windowstate:restored"
	// EventError fires when a save or load fails. Payload: ErrorPayload.
	EventError = "windowstate:error"
)

// RestoredPayload is the payload for EventRestored.
type RestoredPayload struct {
	Name     string   `json:"name"`
	Geometry Geometry `json:"geometry"`
	Clamped  bool     `json:"clamped"` // true if the saved geometry didn't fit an attached display
}

// ErrorPayload is the payload for EventError.
type ErrorPayload struct {
	Name  string `json:"name"`
	Op    string `json:"op"` // "save" or "load"
	Error string `json:"error"`
}

// Geometry is the persisted shape of a window: its restored (non-maximised,
// non-minimised) bounds, whether it was maximised, and which screen it was
// last on (informational only — restore clamps by intersection, not by
// matching ScreenID, since a saved ID is meaningless once that display is
// detached).
type Geometry struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`

	Maximised bool   `json:"maximised"`
	ScreenID  string `json:"screenId"`
}

// Defaults for options not explicitly set.
const (
	DefaultName     = "main"
	DefaultDebounce = 500 * time.Millisecond
)

// Option configures Manage.
type Option func(*config)

type config struct {
	name     string
	store    *state.Store[Geometry]
	debounce time.Duration
	emitter  *events.Emitter
}

func defaultConfig() config {
	return config{
		name:     DefaultName,
		debounce: DefaultDebounce,
	}
}

// WithName sets the per-window state key, letting a multi-window app track
// independent geometry for each window (e.g. "main", "inspector"). It is
// also the default state.Store file base name when WithStore is not given.
func WithName(name string) Option {
	return func(c *config) {
		if name != "" {
			c.name = name
		}
	}
}

// WithStore overrides the default state.Store used for persistence. Without
// this, Manage derives one from the app's name (Config().Name) and the
// window's WithName key, mirroring state.WithAppName + state.WithName.
func WithStore(st *state.Store[Geometry]) Option {
	return func(c *config) {
		c.store = st
	}
}

// WithDebounce sets how long Manager waits after the last move/resize event
// before writing to disk. A drag-resize fires a continuous stream of
// events; without debouncing, every one of them would hit disk.
func WithDebounce(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.debounce = d
		}
	}
}

// WithEmitter wires an optional event emitter. Passed through to the
// underlying state.Store (so state:saved/state:loaded fire) and used
// directly for EventRestored/EventError. Nil (the default) makes all
// emission a no-op.
func WithEmitter(e *events.Emitter) Option {
	return func(c *config) {
		c.emitter = e
	}
}

// Manager tracks and persists geometry for one window. Create one via
// Manage per window; a multi-window app creates one Manager per window,
// each with its own WithName key.
type Manager struct {
	win      Window
	screens  ScreenSource
	store    *state.Store[Geometry]
	name     string
	debounce time.Duration
	emitter  *events.Emitter

	mu           sync.Mutex
	timer        *time.Timer
	pending      *Geometry
	lastRestored Geometry
	unsub        []func()
	closed       bool
}

// newManager is the Wails-free constructor used by Manage and by tests: it
// takes the narrow Window/ScreenSource interfaces instead of concrete Wails
// types, so tests exercise real save/restore/debounce logic against fakes
// without a display or cgo backend.
func newManager(win Window, screens ScreenSource, appName string, opts ...Option) (*Manager, error) {
	if win == nil {
		return nil, errors.New(ErrConfig, "windowstate: window must not be nil", nil)
	}
	if screens == nil {
		return nil, errors.New(ErrConfig, "windowstate: screen source must not be nil", nil)
	}

	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	store := cfg.store
	if store == nil {
		var storeOpts []state.Option[Geometry]
		if appName != "" {
			storeOpts = append(storeOpts, state.WithAppName[Geometry](appName))
		}
		storeOpts = append(storeOpts, state.WithName[Geometry](cfg.name))
		if cfg.emitter != nil {
			storeOpts = append(storeOpts, state.WithEmitter[Geometry](cfg.emitter))
		}
		var err error
		store, err = state.New(storeOpts...)
		if err != nil {
			return nil, errors.Wrap(ErrConfig, "windowstate: failed to build default store", err)
		}
	}

	m := &Manager{
		win:      win,
		screens:  screens,
		store:    store,
		name:     cfg.name,
		debounce: cfg.debounce,
		emitter:  cfg.emitter,
	}
	m.track()
	return m, nil
}

func (m *Manager) emit(name string, data any) {
	if m.emitter != nil {
		m.emitter.Emit(name, data)
	}
}

func (m *Manager) emitError(op string, err error) {
	m.emit(EventError, ErrorPayload{Name: m.name, Op: op, Error: err.Error()})
}
