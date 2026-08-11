package windowstate

import (
	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Manage wires geometry persistence for win: it tracks move/resize
// (debounced — see WithDebounce), skipping saves while minimised and
// recording the maximised flag separately from geometry so a maximised
// window's saved bounds are its pre-maximise size (see manager.go).
//
// Call Restore before win.Show() to apply any previously saved geometry,
// clamped to a currently attached display, and Close during shutdown to
// flush any pending debounced save. A multi-window app calls Manage once
// per window, each with a distinct WithName key (default "main").
func Manage(app *application.App, win *application.WebviewWindow, opts ...Option) (*Manager, error) {
	if app == nil {
		return nil, errors.New(ErrConfig, "windowstate: app must not be nil", nil)
	}
	if win == nil {
		return nil, errors.New(ErrConfig, "windowstate: win must not be nil", nil)
	}
	return newManager(&wailsWindow{win: win}, app.Screen, app.Config().Name, opts...)
}
