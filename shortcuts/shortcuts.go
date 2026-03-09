// Package shortcuts builds native application menus with standard keyboard
// shortcuts for Wails v3 apps. It handles platform differences automatically:
// on macOS the app menu includes About, Services, Hide/Show, and Quit; on
// other platforms Quit goes in the File menu.
//
// When Settings is enabled, a "Settings" menu item with ⌘, (macOS) or
// Ctrl+, (others) is added and emits an event via the kit event system.
package shortcuts

import (
	"runtime"

	"github.com/jrschumacher/wails-kit/events"
	"github.com/wailsapp/wails/v3/pkg/application"
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

// Apply builds the application menu and sets it on the given Wails app.
// It must be called after application.New.
func (m *Manager) Apply(app *application.App) {
	menu := application.NewMenu()

	// macOS app menu (first menu in the menu bar).
	if runtime.GOOS == "darwin" {
		if m.settings {
			m.addDarwinAppMenuWithSettings(menu, app.Config().Name)
		} else if m.appMenu {
			menu.AddRole(application.AppMenu)
		}
	}

	if m.fileMenu {
		menu.AddRole(application.FileMenu)
	}

	// Build the edit menu manually when Settings needs to be injected on
	// non-macOS platforms; otherwise use the standard role.
	if m.settings && runtime.GOOS != "darwin" {
		m.addEditMenuWithSettings(menu)
	} else if m.editMenu {
		menu.AddRole(application.EditMenu)
	}

	if m.viewMenu {
		menu.AddRole(application.ViewMenu)
	}

	if m.windowMenu {
		menu.AddRole(application.WindowMenu)
	}

	app.Menu.SetApplicationMenu(menu)
}

// addDarwinAppMenuWithSettings builds a macOS application menu with a
// Settings item placed after About, matching macOS Human Interface Guidelines.
func (m *Manager) addDarwinAppMenuWithSettings(parent *application.Menu, appName string) {
	sub := parent.AddSubmenu(appName)
	sub.AddRole(application.About)
	sub.AddSeparator()

	sub.Add("Settings\u2026").
		SetAccelerator("CmdOrCtrl+,").
		OnClick(func(_ *application.Context) {
			m.emit(EventSettingsOpen, nil)
		})
	sub.AddSeparator()

	sub.AddRole(application.ServicesMenu)
	sub.AddSeparator()
	sub.AddRole(application.Hide)
	sub.AddRole(application.HideOthers)
	sub.AddRole(application.UnHide)
	sub.AddSeparator()
	sub.AddRole(application.Quit)
}

// addEditMenuWithSettings builds an Edit menu with a Settings item appended,
// used on non-macOS platforms where Settings goes in Edit > Preferences.
func (m *Manager) addEditMenuWithSettings(parent *application.Menu) {
	sub := parent.AddSubmenu("Edit")
	sub.AddRole(application.Undo)
	sub.AddRole(application.Redo)
	sub.AddSeparator()
	sub.AddRole(application.Cut)
	sub.AddRole(application.Copy)
	sub.AddRole(application.Paste)
	sub.AddRole(application.Delete)
	sub.AddSeparator()
	sub.AddRole(application.SelectAll)
	sub.AddSeparator()

	sub.Add("Settings").
		SetAccelerator("Ctrl+,").
		OnClick(func(_ *application.Context) {
			m.emit(EventSettingsOpen, nil)
		})
}

func (m *Manager) emit(name string, data any) {
	if m.emitter != nil {
		m.emitter.Emit(name, data)
	}
}
