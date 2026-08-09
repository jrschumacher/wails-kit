package windowstate

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	wevents "github.com/wailsapp/wails/v3/pkg/events"
)

// Window is the narrow slice of *application.WebviewWindow that windowstate
// needs. It exists so Manager's save/restore/debounce logic can be tested
// against a fake instead of a real Wails window — constructing a real
// *application.WebviewWindow needs a live display/session to run (though
// not to build, per AD-4).
type Window interface {
	Position() (x, y int)
	SetPosition(x, y int)
	Size() (w, h int)
	SetSize(width, height int)
	IsMinimised() bool
	IsMaximised() bool
	Maximise()
	// OnWindowEvent registers callback for eventType and returns an
	// unsubscribe func. Unlike the real Wails method, callback takes no
	// arguments — windowstate never needs the *application.WindowEvent
	// payload, only the fact that a move/resize happened, so the adapter
	// discards it and the fake in tests doesn't need to construct one.
	OnWindowEvent(eventType wevents.WindowEventType, callback func()) func()
}

// ScreenSource is the narrow slice of *application.ScreenManager (an App's
// Screen field) that windowstate needs for clamping. *application.
// ScreenManager satisfies this interface structurally — Manage passes
// app.Screen directly. Tests supply a fake returning fabricated screens
// (application.Screen is a plain data struct, safe to construct without a
// display).
type ScreenSource interface {
	GetAll() []*application.Screen
	GetPrimary() *application.Screen
}

// wailsWindow adapts *application.WebviewWindow to Window.
type wailsWindow struct {
	win *application.WebviewWindow
}

func (w *wailsWindow) Position() (int, int)      { return w.win.Position() }
func (w *wailsWindow) SetPosition(x, y int)      { w.win.SetPosition(x, y) }
func (w *wailsWindow) Size() (int, int)          { return w.win.Size() }
func (w *wailsWindow) SetSize(width, height int) { w.win.SetSize(width, height) }
func (w *wailsWindow) IsMinimised() bool         { return w.win.IsMinimised() }
func (w *wailsWindow) IsMaximised() bool         { return w.win.IsMaximised() }
func (w *wailsWindow) Maximise()                 { w.win.Maximise() }

func (w *wailsWindow) OnWindowEvent(eventType wevents.WindowEventType, callback func()) func() {
	return w.win.OnWindowEvent(eventType, func(_ *application.WindowEvent) {
		callback()
	})
}
