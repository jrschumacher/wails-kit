// Package appearance is one resolved source of truth for light/dark theme:
// an OS signal, an optional persisted user override, and the merge of the
// two into a single Theme both the Go side (e.g. the Wails window's
// background colour at startup) and the webview can read. The concrete
// problem it solves is documented in the package README — Prune's
// three-layer background-colour seam (window frame, base CSS, component
// CSS all having to agree by hand).
//
// This package is Wails-free (AD-4, docs/v2-roadmap.md): it must run in a
// CLI or TUI process with no GUI, so the OS theme signal is a narrow
// interface (Source) satisfied here by a no-Wails darwin default and,
// later, by a live Wails-backed implementation from kit/wailsbridge
// (WP-31, not built by this package). The frontend decides how to *apply*
// the resolved Theme (CSS class/vars, window BackgroundColour); this
// package only decides what it *is*.
package appearance

import (
	"fmt"
	"sync"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

// Mode is the user's stated preference: follow the OS, or pin to a theme.
type Mode string

const (
	// ModeSystem is the default: the resolved Theme tracks the OS signal
	// (Source.IsDark) and flips live when it changes.
	ModeSystem Mode = "system"
	ModeLight  Mode = "light"
	ModeDark   Mode = "dark"
)

// Theme is the resolved truth: exactly two states, never "system" — that
// ambiguity is Mode's job. Everything downstream (window background colour,
// CSS class) consumes Theme, never Mode.
type Theme string

const (
	ThemeLight Theme = "light"
	ThemeDark  Theme = "dark"
)

func validMode(m Mode) bool {
	switch m {
	case ModeSystem, ModeLight, ModeDark:
		return true
	}
	return false
}

// Error codes for the appearance package.
const (
	ErrInvalidMode errors.Code = "appearance_invalid_mode"
	ErrPersist     errors.Code = "appearance_persist"
)

func init() {
	errors.RegisterMessages(map[errors.Code]i18n.Text{
		ErrInvalidMode: i18n.T("wailskit.appearance.errors.invalid_mode", "Invalid appearance setting. Please choose System, Light, or Dark."),
		ErrPersist:     i18n.T("wailskit.appearance.errors.persist", "Failed to save your appearance preference. Please try again."),
	})
}

// EventChanged is emitted whenever the *resolved* Theme changes — never
// merely because Mode was set to the same effective theme it already was.
// Fired both for an explicit SetMode call and for an OS flip while Mode is
// ModeSystem. Payload: ChangedPayload.
const EventChanged = "appearance:changed"

// ChangedPayload is EventChanged's payload.
type ChangedPayload struct {
	Mode     Mode  `json:"mode"`
	Resolved Theme `json:"resolved"`
}

// SettingMode is the settings key SettingsGroup's field is registered
// under. Its persisted value is one of Mode's three string values.
const SettingMode = "appearance.mode"

// Option configures NewService.
type Option func(*config)

type config struct {
	source      Source
	sourceSet   bool // true once WithSource has run, even with a nil Source — see NewService
	emitter     *events.Emitter
	settingsSvc *settings.Service
}

// WithSource wires the OS theme signal used to resolve ModeSystem. Without
// this, NewService uses the platform default (the no-Wails darwin source on
// macOS; nil — meaning "always light" — everywhere else until a Source is
// wired, e.g. by kit/wailsbridge). Passing WithSource(nil) explicitly opts
// out of the platform default rather than leaving it unset.
func WithSource(s Source) Option {
	return func(c *config) {
		c.source = s
		c.sourceSet = true
	}
}

// WithSettings wires a *settings.Service so SetMode persists the user's
// override and Mode/Resolved read it back live (settings values are read
// at call time, not cached — docs/settings-integration.md). Without this,
// SetMode still works but only updates the in-memory value for the life of
// the process.
//
// SettingsGroup() must be registered with the same settings.Service (via
// settings.WithGroup) for the SettingMode field to exist in its schema —
// this option does not do that for you.
func WithSettings(svc *settings.Service) Option {
	return func(c *config) { c.settingsSvc = svc }
}

// WithEmitter wires an optional *events.Emitter so resolved-Theme changes
// fire EventChanged. Nil (the default) makes emission a no-op.
func WithEmitter(e *events.Emitter) Option {
	return func(c *config) { c.emitter = e }
}

// Service is the single resolved source of truth for light/dark theme.
// Build one with NewService; it is safe for concurrent use.
type Service struct {
	source      Source
	emitter     *events.Emitter
	settingsSvc *settings.Service

	mu           sync.Mutex
	mode         Mode  // in-memory fallback/current value when settingsSvc is nil
	lastResolved Theme // cached purely for change detection — see resolveAndMaybeEmit

	unsubscribe func() // cancels the Source subscription; nil if source is nil
}

// NewService builds a Service. With no options: Mode is ModeSystem, the
// resolved Theme comes from the platform-default Source (darwin: real OS
// detection; elsewhere: nil, meaning light — see WithSource), no settings
// persistence, and no event emission.
func NewService(opts ...Option) *Service {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	if !cfg.sourceSet {
		cfg.source = newDefaultSource()
	}

	s := &Service{
		source:      cfg.source,
		emitter:     cfg.emitter,
		settingsSvc: cfg.settingsSvc,
		mode:        ModeSystem,
	}

	// Seed lastResolved without emitting: there is no prior state to
	// compare against at construction, so NewService must never itself
	// fire EventChanged.
	s.lastResolved = resolveTheme(s.currentMode(), s.source)

	if s.source != nil {
		s.unsubscribe = s.source.Subscribe(s.onSourceChange)
	}

	return s
}

// Close cancels the Source subscription, if one was made. Call it during
// app shutdown; safe to call on a Service with no Source (no-op).
func (s *Service) Close() {
	if s.unsubscribe != nil {
		s.unsubscribe()
	}
}

// Mode returns the current user preference. When a settings.Service is
// wired (WithSettings), this reads it live (call-time, not cached) so a
// change made through the generic settings UI (settings.Binding.SetValues)
// is reflected immediately, not just changes made through SetMode.
func (s *Service) Mode() Mode {
	return s.currentMode()
}

// Resolved returns the current resolved Theme: mode's pinned theme if
// mode != ModeSystem, otherwise the Source's live signal (light if no
// Source is wired). This is a pure read — it never emits EventChanged;
// only SetMode, an OS flip, and Refresh do that.
func (s *Service) Resolved() Theme {
	return resolveTheme(s.currentMode(), s.source)
}

// SetMode validates and applies a new mode: persists it via the wired
// settings.Service (if any), updates the in-memory fallback, and emits
// EventChanged if — and only if — the resolved Theme actually changes as a
// result. Setting the same mode again, or a different mode that resolves
// to the same Theme (e.g. ModeSystem while the OS is dark -> ModeDark),
// is not an error and does not emit.
func (s *Service) SetMode(m Mode) error {
	if !validMode(m) {
		return errors.New(ErrInvalidMode, fmt.Sprintf("appearance: invalid mode %q", m), nil)
	}

	if s.settingsSvc != nil {
		verrs, err := s.settingsSvc.SetValues(map[string]any{SettingMode: string(m)})
		if err != nil {
			return errors.Wrap(ErrPersist, "appearance: failed to persist mode", err)
		}
		if len(verrs) > 0 {
			return errors.New(ErrPersist, fmt.Sprintf("appearance: mode rejected by settings validation: %v", verrs), nil)
		}
	}

	s.mu.Lock()
	s.mode = m
	s.mu.Unlock()

	s.resolveAndMaybeEmit(m)
	return nil
}

// Refresh re-resolves the current mode (read live, see Mode) against the
// current Source and emits EventChanged if the resolved Theme differs from
// the last known value.
//
// This exists for the composition gap SetMode alone can't close: a generic
// settings form built from SettingsGroup() writes through
// settings.Service.SetValues (or settings.Binding.SetValues) directly, not
// through SetMode, so nothing tells this Service to re-resolve and emit.
// The recommended fix is to wire Refresh into the settings.Service's own
// change notification at construction time:
//
//	var appearanceSvc *appearance.Service // assigned below; captured by the closure
//	settingsSvc := settings.NewService(
//	    settings.WithOnChange(func(map[string]any) {
//	        if appearanceSvc != nil {
//	            appearanceSvc.Refresh()
//	        }
//	    }),
//	)
//	appearanceSvc = appearance.NewService(appearance.WithSettings(settingsSvc))
//
// See the README "Reacting to settings-UI changes" section.
func (s *Service) Refresh() {
	s.resolveAndMaybeEmit(s.currentMode())
}
