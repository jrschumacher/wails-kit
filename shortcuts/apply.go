//go:build (darwin || linux || windows) && wails

package shortcuts

import (
	"context"
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
	// non-macOS platforms, or the app's undo stack runs Undo and Redo;
	// otherwise use the standard role.
	if m.undo != nil || (m.settings && runtime.GOOS != "darwin") {
		m.addEditMenu(menu, m.settings && runtime.GOOS != "darwin")
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

// addEditMenu builds the Edit menu by hand: with the app's undo stack
// behind Undo and Redo when WithUndo is set (else the native roles), and
// with a Settings item appended on non-macOS platforms, where Settings goes
// in Edit.
func (m *Manager) addEditMenu(parent *application.Menu, settings bool) {
	sub := parent.AddSubmenu("Edit")
	if m.undo != nil {
		stack := m.undo
		first := stack.Menu()
		undo := sub.Add(first.UndoLabel).
			SetAccelerator("CmdOrCtrl+z").
			OnClick(func(_ *application.Context) { go stack.MenuUndo(context.Background()) })
		redo := sub.Add(first.RedoLabel).
			SetAccelerator(RedoAccelerator(runtime.GOOS)).
			OnClick(func(_ *application.Context) { go stack.MenuRedo(context.Background()) })
		stack.OnMenu(func(s MenuState) {
			undo.SetLabel(s.UndoLabel).SetEnabled(s.UndoEnabled)
			redo.SetLabel(s.RedoLabel).SetEnabled(s.RedoEnabled)
		})
	} else {
		sub.AddRole(application.Undo)
		sub.AddRole(application.Redo)
	}
	sub.AddSeparator()
	sub.AddRole(application.Cut)
	sub.AddRole(application.Copy)
	sub.AddRole(application.Paste)
	sub.AddRole(application.Delete)
	sub.AddSeparator()
	sub.AddRole(application.SelectAll)
	if !settings {
		return
	}
	sub.AddSeparator()

	sub.Add("Settings").
		SetAccelerator("Ctrl+,").
		OnClick(func(_ *application.Context) {
			m.emit(EventSettingsOpen, nil)
		})
}
