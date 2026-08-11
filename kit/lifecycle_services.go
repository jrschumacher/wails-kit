package kit

import (
	"context"
	"time"

	"github.com/jrschumacher/wails-kit/v2/health"
	"github.com/jrschumacher/wails-kit/v2/settings"
	"github.com/jrschumacher/wails-kit/v2/updates"
)

// healthTicker adapts health.Registry.Start (which blocks until its ctx is
// Done) to the lifecycle.Service interface (OnStartup/OnShutdown, neither
// of which is expected to block past its own call). It runs Start in its
// own goroutine and cancels an internally-owned context on OnShutdown,
// waiting for Start to actually return before OnShutdown does — so
// lifecycle.Manager's shutdown-timeout machinery still applies (a
// healthTicker that hung would be caught the same way any other service's
// hung OnShutdown is).
type healthTicker struct {
	reg    *health.Registry
	cancel context.CancelFunc
	done   chan struct{}
}

func (h *healthTicker) OnStartup(_ context.Context) error {
	// Deliberately not derived from the ctx OnStartup receives: that ctx is
	// only valid for the duration of the startup call (and, per
	// lifecycle.WithServiceTimeout, may be shorter-lived than the service
	// itself), but health probing needs to keep running for the life of the
	// app. context.Background() plus explicit cancellation on OnShutdown is
	// the same pattern lifecycle.Manager itself expects from a long-running
	// service.
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.done = make(chan struct{})
	go func() {
		defer close(h.done)
		h.reg.Start(ctx)
	}()
	return nil
}

func (h *healthTicker) OnShutdown() error {
	if h.cancel != nil {
		h.cancel()
		<-h.done
	}
	return nil
}

// updatesTicker periodically calls updates.Service.CheckForUpdate according
// to the persisted updates.SettingCheckFrequency value ("startup" | "daily"
// | "weekly" | "never"), reading it fresh from Settings on every decision
// point rather than caching it at construction — the same "app reads it, app
// decides" responsibility updates.WithSettings' doc comment assigns to
// whichever code composes the Service, which for a kit-built app is kit
// itself. It never calls DownloadUpdate or ApplyUpdate: those replace the
// running binary and are treated as a user-facing decision the app drives
// from the updates:available event, not something kit does silently in the
// background.
type updatesTicker struct {
	svc      *updates.Service
	settings *settings.Service
	cancel   context.CancelFunc
	done     chan struct{}
}

func (u *updatesTicker) OnStartup(_ context.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	u.cancel = cancel
	u.done = make(chan struct{})
	go u.run(ctx)
	return nil
}

func (u *updatesTicker) OnShutdown() error {
	if u.cancel != nil {
		u.cancel()
		<-u.done
	}
	return nil
}

func (u *updatesTicker) run(ctx context.Context) {
	defer close(u.done)

	freq := u.frequency()
	if freq == "never" {
		return
	}

	u.check(ctx)

	interval := frequencyInterval(freq)
	if interval <= 0 {
		// "startup": one check per launch, no recurring ticker.
		return
	}

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			u.check(ctx)
		}
	}
}

func (u *updatesTicker) check(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// Errors are already reported via the updates:error event (see
	// updates.Service.emitError) — nothing further to do with them here.
	_, _ = u.svc.CheckForUpdate(cctx)
}

// frequency reads updates.SettingCheckFrequency from Settings, defaulting to
// "daily" (matching updates.SettingsGroup's own field default) if it is
// unset, unreadable, or not a recognized value.
func (u *updatesTicker) frequency() string {
	const def = "daily"
	values, err := u.settings.GetValues()
	if err != nil {
		return def
	}
	v, _ := values[updates.SettingCheckFrequency].(string)
	switch v {
	case "startup", "daily", "weekly", "never":
		return v
	default:
		return def
	}
}

func frequencyInterval(freq string) time.Duration {
	switch freq {
	case "daily":
		return 24 * time.Hour
	case "weekly":
		return 7 * 24 * time.Hour
	default: // "startup"
		return 0
	}
}
