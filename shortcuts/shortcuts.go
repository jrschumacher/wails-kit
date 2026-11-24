// Package shortcuts builds native application menus with standard keyboard
// shortcuts for Wails v3 apps. It handles platform differences automatically:
// on macOS the app menu includes About, Services, Hide/Show, and Quit; on
// other platforms Quit goes in the File menu.
//
// When Settings is enabled, a "Settings" menu item with ⌘, (macOS) or
// Ctrl+, (others) is added and emits an event via the kit event system.
package shortcuts

import (
	"github.com/jrschumacher/wails-kit/events"
)

// Event names emitted by the shortcuts manager.
const (
	// EventSettingsOpen is emitted when the user triggers the Settings shortcut.
	EventSettingsOpen = "settings:open"
)

// Option configures a Manager.
type Option func(*Manager)

// Manager builds and applies a native application menu with standard
// keyboard shortcuts. Use New to create a Manager, then call Apply
// to set the menu on the Wails app.
type Manager struct {
	emitter    *events.Emitter
	appMenu    bool
	editMenu   bool
	viewMenu   bool
	windowMenu bool
	fileMenu   bool
	settings   bool
	undo       *UndoStack
}

// WithEmitter sets the event emitter used to broadcast shortcut events
// such as EventSettingsOpen.
func WithEmitter(e *events.Emitter) Option {
	return func(m *Manager) { m.emitter = e }
}

// WithAppMenu enables the application menu. On macOS this includes About,
// Services, Hide/Show, and Quit. On other platforms this is a no-op (use
// WithFileMenu for Quit).
func WithAppMenu() Option {
	return func(m *Manager) { m.appMenu = true }
}

// WithFileMenu enables the File menu.
func WithFileMenu() Option {
	return func(m *Manager) { m.fileMenu = true }
}

// WithEditMenu enables the Edit menu (Undo, Redo, Cut, Copy, Paste,
// Delete, Select All).
func WithEditMenu() Option {
	return func(m *Manager) { m.editMenu = true }
}

// WithViewMenu enables the View menu (Reload, Zoom, Fullscreen).
func WithViewMenu() Option {
	return func(m *Manager) { m.viewMenu = true }
}

// WithWindowMenu enables the Window menu (Minimize, Zoom).
func WithWindowMenu() Option {
	return func(m *Manager) { m.windowMenu = true }
}

// WithSettings adds a Settings menu item with the standard accelerator
// (⌘, on macOS, Ctrl+, elsewhere). On macOS the item is placed in the
// application menu; on other platforms it is appended to the Edit menu.
// Triggering the shortcut emits EventSettingsOpen via the emitter.
//
// WithSettings implies WithAppMenu on macOS and WithEditMenu on other
// platforms if they are not already enabled.
func WithSettings() Option {
	return func(m *Manager) { m.settings = true }
}

// WithUndo puts the app's undo stack behind Edit > Undo and Redo: the items
// are named for the action ("Undo Archive"), enabled to match, and undo
// typing first while a text field has focus (see UndoStack.MenuUndo). It
// implies WithEditMenu. Redo is on the platform's shortcut: ⇧⌘Z on macOS,
// Ctrl+Y on Windows and Ctrl+Shift+Z on Linux.
//
// Apply also tells the stack about Wails's windows: text focus is kept per
// window, and Edit > Undo with text focus is sent to the page of the window
// in front only.
//
// Limits in Wails v3.0.0-alpha.64: on Linux, menu accelerators are not
// attached to the GTK menu items (setAccelerator is a stub), so Edit > Undo
// and Redo run only when clicked; ⌘Z/Ctrl+Z in a field is the web view's own
// typing undo there. On Windows, a menu item's native handle is the one for
// the window that built the menu last, so with several windows only one
// window's Edit menu shows the updated labels and enabled state.
func WithUndo(stack *UndoStack) Option {
	return func(m *Manager) {
		m.undo = stack
		if stack != nil {
			m.editMenu = true
		}
	}
}

// RedoAccelerator is Redo's shortcut on the platform named by goos
// (runtime.GOOS), in Wails's accelerator syntax.
func RedoAccelerator(goos string) string {
	if goos == "windows" {
		return "Ctrl+y"
	}
	return "CmdOrCtrl+Shift+z"
}

// WithDefaults enables the standard set of menus: App, File, Edit, View,
// and Window.
func WithDefaults() Option {
	return func(m *Manager) {
		m.appMenu = true
		m.fileMenu = true
		m.editMenu = true
		m.viewMenu = true
		m.windowMenu = true
	}
}

// New creates a Manager with the given options.
func New(opts ...Option) *Manager {
	m := &Manager{}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Manager) emit(name string, data any) {
	if m.emitter != nil {
		m.emitter.Emit(name, data)
	}
}
