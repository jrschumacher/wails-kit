package permissions

import (
	"context"
	"fmt"
	"sync"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/i18n"
)

// Option configures a Service. See NewService.
type Option func(*config)

type config struct {
	emitter   *events.Emitter
	localizer *i18n.Localizer
}

// WithEmitter wires an optional event emitter so EventChanged fires on
// Status transitions. Nil (the default) makes emission a no-op.
func WithEmitter(e *events.Emitter) Option {
	return func(c *config) { c.emitter = e }
}

// WithLocalizer wires an *i18n.Localizer used by Text to resolve the
// package's exported prompt-copy i18n.Text constants (see copy.go).
// Optional — without it, Text returns each Text's literal Other (English)
// value, the same "no localizer, still sensible English" guarantee the
// errors and settings packages make.
func WithLocalizer(l *i18n.Localizer) Option {
	return func(c *config) { c.localizer = l }
}

// Service checks, requests, and provides a System-Settings escape hatch
// for OS permissions. Build one with NewService; it is safe for
// concurrent use.
type Service struct {
	platform  platform
	emitter   *events.Emitter
	localizer *i18n.Localizer

	mu   sync.Mutex
	seen map[Kind]bool   // has Request or OpenSystemSettings been called for this Kind this process
	last map[Kind]Status // last Status observed per Kind, for transition-only EventChanged
}

// NewService builds a Service backed by the real platform implementation
// for GOOS (currently: darwin; every other platform reports
// StatusUnsupported for every Kind — see platform_other.go).
func NewService(opts ...Option) *Service {
	return newService(newPlatform(), opts...)
}

// newService is the platform-injectable constructor used by NewService and
// by tests: it takes the narrow platform interface instead of relying on
// the build-tagged real implementation, so tests exercise Service's state
// machine (the denied/not_determined synthesis, context handling in
// Request, transition-only event emission) against a fake without a
// display, a real permission prompt, or user interaction.
func newService(p platform, opts ...Option) *Service {
	cfg := config{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Service{
		platform:  p,
		emitter:   cfg.emitter,
		localizer: cfg.localizer,
		seen:      make(map[Kind]bool),
		last:      make(map[Kind]Status),
	}
}

// Check reports the current authorization state for k without prompting.
// See the package doc for how Denied vs NotDetermined is derived when the
// OS/SDK itself only reports a granted/not-granted boolean.
func (s *Service) Check(k Kind) Status {
	granted, supported := s.platform.check(k)
	return s.resolveAndEmit(k, granted, supported)
}

// Request triggers the OS-level authorization flow for k. It is
// asynchronous by nature — the user may leave the OS dialog open
// indefinitely — so Request races the underlying platform call against
// ctx: if ctx is done first, Request returns immediately with the current
// Check(k) result and ctx.Err(), leaving the platform call running in the
// background (it cannot be cancelled — see platform.go). Calling Request
// at all — regardless of outcome or whether ctx cancels first — marks k as
// "seen" for the rest of this Service's life: a subsequent not-granted
// Check(k) reports Denied, never NotDetermined, because the app has now
// asked and must not re-prompt.
func (s *Service) Request(ctx context.Context, k Kind) (Status, error) {
	if err := ctx.Err(); err != nil {
		return s.Check(k), err
	}

	s.markSeen(k)

	type result struct {
		granted, supported bool
		err                error
	}
	ch := make(chan result, 1)
	go func() {
		granted, supported, err := s.platform.request(k)
		ch <- result{granted, supported, err}
	}()

	select {
	case r := <-ch:
		status := s.resolveAndEmit(k, r.granted, r.supported)
		if r.err != nil {
			return status, errors.Wrap(ErrRequest, "permissions: request failed", r.err)
		}
		return status, nil
	case <-ctx.Done():
		return s.Check(k), ctx.Err()
	}
}

// OpenSystemSettings opens the OS settings pane relevant to k — the only
// way to grant Accessibility or FullDiskAccess, and the correct next step
// after Notifications reports Denied. Like Request, calling this marks k
// as "seen".
func (s *Service) OpenSystemSettings(k Kind) error {
	s.markSeen(k)
	if err := s.platform.openSystemSettings(k); err != nil {
		return errors.Wrap(ErrOpenSettings, "permissions: failed to open System Settings", err)
	}
	return nil
}

// Text resolves t — typically one of the exported prompt-copy constants in
// copy.go — against the localizer configured via WithLocalizer, falling
// back to t.Other when none is set. Go-side (CLI/TUI/backend) callers use
// this to render rationale/denied copy; a webview frontend instead reads
// the same catalog keys via the app's i18n.Binding.GetCatalog (see
// README.md).
func (s *Service) Text(t i18n.Text, args ...any) string {
	if s.localizer == nil {
		if len(args) == 0 {
			return t.Other
		}
		return fmt.Sprintf(t.Other, args...)
	}
	return s.localizer.T(t, args...)
}

// markSeen records that k has been actively asked about (Request or
// OpenSystemSettings) this process. See the package doc's "synthetic
// denied/not_determined split".
func (s *Service) markSeen(k Kind) {
	s.mu.Lock()
	s.seen[k] = true
	s.mu.Unlock()
}

// resolveAndEmit turns a platform-reported (granted, supported) pair into
// a Status, using the seen map to split not-granted into Denied vs
// NotDetermined, and emits EventChanged if this is the first observation
// or a change from the last one recorded for k. The emitter call happens
// after mu is released — never emit while holding a lock (see AGENTS.md;
// three other kit packages shipped that deadlock before this rule was
// enforced everywhere).
func (s *Service) resolveAndEmit(k Kind, granted, supported bool) Status {
	s.mu.Lock()
	var status Status
	switch {
	case !supported:
		status = StatusUnsupported
	case granted:
		status = StatusGranted
	case s.seen[k]:
		status = StatusDenied
	default:
		status = StatusNotDetermined
	}
	changed := s.last[k] != status
	if changed {
		s.last[k] = status
	}
	s.mu.Unlock()

	if changed && s.emitter != nil {
		s.emitter.Emit(EventChanged, ChangedPayload{Kind: k, Status: status})
	}
	return status
}
