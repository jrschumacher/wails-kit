package wailsbridge

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// themeSource implements appearance.Source over a live Wails app: IsDark
// reads the OS theme via app.Env.IsDarkMode(), and Subscribe follows OS
// theme flips via events.Common.ThemeChanged. Every desktop platform's
// theme-change handler (application_darwin.go, application_windows.go,
// linux_cgo_gtk3.go / application_linux_dbus.go) sets the new dark-mode
// value on the event's ApplicationEventContext *before* dispatching it, so
// Subscribe's callback never needs to re-query app.Env — it just reads
// event.Context().IsDarkMode().
//
// kit.New always seeds Kit.Appearance with *some* Source before a Wails
// app exists — nil off darwin, the no-Wails native default on darwin (see
// kit/appearance_source.go) — so a CLI/TUI process still reports something
// sensible. Wiring this one in via kit.SetAppearanceSource (see Attach)
// replaces that source outright, on every platform including darwin: this
// is the one that actually updates when the OS theme flips while the app
// is running, which the darwin default (a single point-in-time read) does
// not.
type themeSource struct {
	app *application.App
}

func newThemeSource(app *application.App) *themeSource {
	return &themeSource{app: app}
}

// IsDark implements appearance.Source.
func (s *themeSource) IsDark() bool {
	return s.app.Env.IsDarkMode()
}

// Subscribe implements appearance.Source. The returned cancel func is
// app.Event.OnApplicationEvent's own unsubscribe closure.
func (s *themeSource) Subscribe(fn func(dark bool)) (cancel func()) {
	return s.app.Event.OnApplicationEvent(events.Common.ThemeChanged, func(e *application.ApplicationEvent) {
		fn(e.Context().IsDarkMode())
	})
}
