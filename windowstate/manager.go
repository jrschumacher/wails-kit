package windowstate

import (
	"time"

	wevents "github.com/wailsapp/wails/v3/pkg/events"
)

// track wires move/resize listeners. Called once from newManager; there is
// no exported Track — a Manager tracks for its whole lifetime, from Manage
// until Close.
func (m *Manager) track() {
	onEvent := func() { m.captureAndScheduleSave() }
	m.unsub = []func(){
		m.win.OnWindowEvent(wevents.Common.WindowDidMove, onEvent),
		m.win.OnWindowEvent(wevents.Common.WindowDidResize, onEvent),
	}
}

// captureAndScheduleSave is the move/resize handler. It never saves while
// the window is minimised (a minimised window's reported bounds are
// platform-dependent garbage, not something a user wants restored), and it
// separates "restored" geometry from the maximised flag: while maximised,
// Position/Size report the maximised bounds, so those are never written
// into lastRestored — only the flag changes. This is what makes restoring
// a maximised window come back at its pre-maximise size instead of stuck
// full-screen-sized forever.
func (m *Manager) captureAndScheduleSave() {
	if m.win.IsMinimised() {
		return
	}
	maximised := m.win.IsMaximised()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}

	if !maximised {
		x, y := m.win.Position()
		w, h := m.win.Size()
		m.lastRestored = Geometry{X: x, Y: y, W: w, H: h}
	}

	pending := m.lastRestored
	pending.Maximised = maximised
	m.pending = &pending

	if m.timer != nil {
		m.timer.Stop()
	}
	m.timer = time.AfterFunc(m.debounce, m.flush)
}

// flush writes the pending geometry to disk. It runs on the debounce
// timer's own goroutine (or synchronously from Close), never while mu is
// held for longer than copying the pending pointer out — Store.Save does
// its own I/O and locking and must not be called with Manager's mu held.
func (m *Manager) flush() {
	m.mu.Lock()
	pending := m.pending
	m.pending = nil
	m.mu.Unlock()

	if pending == nil {
		return
	}
	if err := m.store.Save(*pending); err != nil {
		m.emitError("save", err)
	}
}

// Restore applies previously saved geometry to the window, clamped to a
// currently attached display (see clamp.go). Call it before win.Show().
//
// Restore never returns an error: a load failure or absent save is not
// exceptional (first run always has no saved geometry) and the window is
// simply left at whatever Wails' own window options already configured.
// Failures are still reported via EventError for a caller who wants to
// know, and a successful restore fires EventRestored (with Clamped=true
// when the saved geometry didn't fit any attached display).
func (m *Manager) Restore() {
	g, err := m.store.Load()
	if err != nil {
		m.emitError("load", err)
		return
	}
	if g.W <= 0 || g.H <= 0 {
		// Zero value from Load (no file on disk yet, and no WithDefaults
		// on the store) — nothing to restore.
		return
	}

	screens := m.screens.GetAll()
	resolved, clamped := clamp(g, screens)

	m.mu.Lock()
	if !m.closed && !resolved.Maximised {
		m.lastRestored = Geometry{X: resolved.X, Y: resolved.Y, W: resolved.W, H: resolved.H}
	}
	m.mu.Unlock()

	m.win.SetPosition(resolved.X, resolved.Y)
	m.win.SetSize(resolved.W, resolved.H)
	if resolved.Maximised {
		m.win.Maximise()
	}

	m.emit(EventRestored, RestoredPayload{Name: m.name, Geometry: resolved, Clamped: clamped})
}

// Close stops tracking, flushes any pending debounced save synchronously,
// and unregisters the window event listeners. Call it during window/app
// shutdown so the last drag before quit isn't lost to the debounce window.
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	pending := m.pending
	m.pending = nil
	unsub := m.unsub
	m.unsub = nil
	m.mu.Unlock()

	for _, fn := range unsub {
		if fn != nil {
			fn()
		}
	}

	if pending != nil {
		if err := m.store.Save(*pending); err != nil {
			m.emitError("save", err)
		}
	}
}
