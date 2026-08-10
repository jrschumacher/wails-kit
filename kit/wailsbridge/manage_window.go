package wailsbridge

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/jrschumacher/wails-kit/v2/appearance"
	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/kit"
	"github.com/jrschumacher/wails-kit/v2/windowstate"
)

// backgroundLight and backgroundDark are the default window background
// colours ManageWindow applies from appearance.Resolved() — the same
// values appearance/README.md's own recommended snippet uses, so a
// ManageWindow caller and a caller following that README by hand land on
// identical colours. Override by calling win.SetBackgroundColour yourself
// after ManageWindow returns; there is no Option for this because it is a
// window-specific value most apps should derive from their own CSS
// variables, not a wailsbridge concept.
var (
	backgroundLight = application.NewRGB(255, 255, 255)
	backgroundDark  = application.NewRGB(23, 23, 23)
)

func backgroundFor(theme appearance.Theme) application.RGBA {
	if theme == appearance.ThemeDark {
		return backgroundDark
	}
	return backgroundLight
}

// ManageWindow wires per-window geometry persistence and background-colour
// sync for win: windowstate.Manage + Restore (clamped to a currently
// attached display — see windowstate/AGENTS.md), and win.SetBackgroundColour
// from appearance.Resolved(), kept in sync for the life of the window as
// the resolved theme changes (OS flips while in "system" mode, or an
// explicit SetMode) — the third layer of the seam appearance/README.md
// documents ("Recommended: set the Wails window background at startup").
// Setting it here too, not just once at window-creation time, is what
// keeps it from drifting back out of sync after a theme change.
//
// windowstate.Manage needs a *application.App (for app.Screen, to clamp
// restored geometry to an attached display) that AD-1's ManageWindow stub
// doesn't list as a parameter; win alone cannot recover it (WebviewWindow
// exposes no App accessor), so this signature adds it — see AGENTS.md,
// "ManageWindow takes app, not just win".
//
// Call this after Attach and before win.Show(): Restore runs synchronously
// inside this call, and a caller that shows the window first will see it
// jump when the saved geometry is applied.
func ManageWindow(k *kit.Kit, app *application.App, win *application.WebviewWindow, opts ...windowstate.Option) error {
	if k == nil {
		return errors.New(ErrConfig, "wailsbridge: k must not be nil", nil)
	}
	if app == nil {
		return errors.New(ErrConfig, "wailsbridge: app must not be nil", nil)
	}
	if win == nil {
		return errors.New(ErrConfig, "wailsbridge: win must not be nil", nil)
	}

	wsOpts := append([]windowstate.Option{windowstate.WithEmitter(k.Events)}, opts...)
	mgr, err := windowstate.Manage(app, win, wsOpts...)
	if err != nil {
		return err
	}
	mgr.Restore()

	win.SetBackgroundColour(backgroundFor(k.Appearance.Resolved()))
	cancelBG := events.On(k.Events, appearance.EventChanged, func(p appearance.ChangedPayload) {
		win.SetBackgroundColour(backgroundFor(p.Resolved))
	})

	app.OnShutdown(func() {
		cancelBG()
		mgr.Close()
	})

	return nil
}
