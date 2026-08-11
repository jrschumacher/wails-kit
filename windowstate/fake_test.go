package windowstate

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	wevents "github.com/wailsapp/wails/v3/pkg/events"
)

// fakeWindow implements Window without any Wails runtime/display/cgo
// dependency, so Manager's save/restore/debounce logic can be tested
// directly. Move/Resize/Minimise/Maximise helper methods mutate state and
// fire the registered listeners, mimicking what a real WebviewWindow would
// report to a subscriber.
type fakeWindow struct {
	mu sync.Mutex

	x, y, w, h int
	minimised  bool
	maximised  bool

	moveListeners   []func()
	resizeListeners []func()
}

func newFakeWindow(x, y, w, h int) *fakeWindow {
	return &fakeWindow{x: x, y: y, w: w, h: h}
}

func (f *fakeWindow) Position() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.x, f.y
}

func (f *fakeWindow) SetPosition(x, y int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.x, f.y = x, y
}

func (f *fakeWindow) Size() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.w, f.h
}

func (f *fakeWindow) SetSize(width, height int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.w, f.h = width, height
}

func (f *fakeWindow) IsMinimised() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.minimised
}

func (f *fakeWindow) IsMaximised() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maximised
}

func (f *fakeWindow) Maximise() {
	f.mu.Lock()
	f.maximised = true
	f.mu.Unlock()
}

func (f *fakeWindow) OnWindowEvent(eventType wevents.WindowEventType, callback func()) func() {
	switch eventType {
	case wevents.Common.WindowDidMove:
		f.mu.Lock()
		f.moveListeners = append(f.moveListeners, callback)
		idx := len(f.moveListeners) - 1
		f.mu.Unlock()
		return func() {
			f.mu.Lock()
			f.moveListeners[idx] = nil
			f.mu.Unlock()
		}
	case wevents.Common.WindowDidResize:
		f.mu.Lock()
		f.resizeListeners = append(f.resizeListeners, callback)
		idx := len(f.resizeListeners) - 1
		f.mu.Unlock()
		return func() {
			f.mu.Lock()
			f.resizeListeners[idx] = nil
			f.mu.Unlock()
		}
	default:
		return func() {}
	}
}

// Move simulates a drag: updates position and fires WindowDidMove listeners.
func (f *fakeWindow) Move(x, y int) {
	f.SetPosition(x, y)
	f.fire(f.moveListenersSnapshot())
}

// Resize simulates a resize: updates size and fires WindowDidResize listeners.
func (f *fakeWindow) Resize(w, h int) {
	f.SetSize(w, h)
	f.fire(f.resizeListenersSnapshot())
}

// Minimise sets the minimised flag and fires the resize listeners, mirroring
// how a real platform tends to also emit a resize-shaped event on minimise.
func (f *fakeWindow) Minimise() {
	f.mu.Lock()
	f.minimised = true
	f.mu.Unlock()
	f.fire(f.resizeListenersSnapshot())
}

func (f *fakeWindow) Unminimise() {
	f.mu.Lock()
	f.minimised = false
	f.mu.Unlock()
}

func (f *fakeWindow) moveListenersSnapshot() []func() {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]func(), len(f.moveListeners))
	copy(out, f.moveListeners)
	return out
}

func (f *fakeWindow) resizeListenersSnapshot() []func() {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]func(), len(f.resizeListeners))
	copy(out, f.resizeListeners)
	return out
}

func (f *fakeWindow) fire(listeners []func()) {
	for _, l := range listeners {
		if l != nil {
			l()
		}
	}
}

// fakeScreens implements ScreenSource over a fixed, fabricated screen list.
type fakeScreens struct {
	screens []*application.Screen
}

func (f *fakeScreens) GetAll() []*application.Screen { return f.screens }

func (f *fakeScreens) GetPrimary() *application.Screen {
	for _, s := range f.screens {
		if s.IsPrimary {
			return s
		}
	}
	if len(f.screens) > 0 {
		return f.screens[0]
	}
	return nil
}

// singleScreen1080p is a common attached-display fixture: a 1920x1080
// primary screen at the origin with a slightly inset work area (as if a
// menu bar/taskbar were present).
func singleScreen1080p() *fakeScreens {
	return &fakeScreens{screens: []*application.Screen{
		{
			ID:        "screen-primary",
			IsPrimary: true,
			Bounds:    application.Rect{X: 0, Y: 0, Width: 1920, Height: 1080},
			WorkArea:  application.Rect{X: 0, Y: 25, Width: 1920, Height: 1055},
		},
	}}
}
