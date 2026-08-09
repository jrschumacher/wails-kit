package kit

import (
	"sync"

	"github.com/jrschumacher/wails-kit/v2/appearance"
)

// dynSourceSub is one Subscribe registration on a dynamicSource. A pointer
// type so Subscribe's cancel func can remove it by identity — plain funcs
// aren't comparable in Go, so the subscriber needs a comparable handle.
type dynSourceSub struct{ fn func(dark bool) }

// dynamicSource is an appearance.Source whose backing Source can be
// retargeted after construction, mirroring dynamicBackend's role for
// events. appearance.Service subscribes to whatever Source it was built
// with exactly once, at construction (see appearance.NewService), and has
// no way to accept a replacement afterward. A CLI/TUI process built from
// kit.New has no live OS theme signal to offer at construction time — there
// is no application.App yet — while a GUI process needs
// kit/wailsbridge.Attach (WP-31) to wire in the real Wails-backed Source
// (application.Env.IsDarkMode() + events.Common.ThemeChanged) onto the
// exact same appearance.Service already handed out via Kit.Appearance.
// Kit.SetAppearanceSource retargets a dynamicSource to close that gap.
//
// Trade-off (documented, not accidental): kit.New always wraps with this —
// even on darwin, where the appearance package ships a real (static,
// non-live) native default Source — so every platform and every entry
// point behaves uniformly: Resolved() reports light (dynamicSource's zero
// value has no real Source) until something calls SetAppearanceSource. A
// consumer that wants real theme detection in a CLI/TUI process that never
// attaches must supply its own Source via kit.WithAppearanceSource. See
// AGENTS.md, "Landmines".
type dynamicSource struct {
	mu         sync.Mutex
	real       appearance.Source
	realCancel func()
	subs       []*dynSourceSub
}

// IsDark implements appearance.Source.
func (d *dynamicSource) IsDark() bool {
	d.mu.Lock()
	real := d.real
	d.mu.Unlock()
	if real == nil {
		return false
	}
	return real.IsDark()
}

// Subscribe implements appearance.Source. fn is called whenever the
// currently-wired real Source reports a change, for as long as one is
// wired (see setReal); if none is ever wired, fn is simply never called —
// the same honest "static source" behavior appearance.Source documents for
// any implementation with no live signal.
func (d *dynamicSource) Subscribe(fn func(dark bool)) (cancel func()) {
	sub := &dynSourceSub{fn: fn}
	d.mu.Lock()
	d.subs = append(d.subs, sub)
	d.mu.Unlock()

	return func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		for i, s := range d.subs {
			if s == sub {
				d.subs = append(d.subs[:i], d.subs[i+1:]...)
				return
			}
		}
	}
}

// notify fans a real Source's change out to every dynamicSource subscriber.
func (d *dynamicSource) notify(dark bool) {
	d.mu.Lock()
	subs := make([]*dynSourceSub, len(d.subs))
	copy(subs, d.subs)
	d.mu.Unlock()

	for _, s := range subs {
		s.fn(dark)
	}
}

// setReal retargets IsDark/Subscribe at real, cancelling the previous real
// Source's subscription (if any) first. Passing nil reverts to "no real
// Source" — IsDark reports light and Subscribe's fn is never called, same
// as a freshly constructed dynamicSource.
func (d *dynamicSource) setReal(real appearance.Source) {
	d.mu.Lock()
	if d.realCancel != nil {
		d.realCancel()
		d.realCancel = nil
	}
	d.real = real
	d.mu.Unlock()

	if real != nil {
		cancel := real.Subscribe(d.notify)
		d.mu.Lock()
		d.realCancel = cancel
		d.mu.Unlock()
	}
}
