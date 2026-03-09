//go:build (darwin || linux || windows) && wails

package shortcuts

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

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
