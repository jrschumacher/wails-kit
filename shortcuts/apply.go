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

	sub.Add(resolveText(labelSettingsDarwin, m.localizer)).
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
	// AddRole(EditMenu) rather than AddSubmenu("Edit") + hand-rolled items.
	// Two reasons, both load-bearing:
	//
	// AddSubmenu produces a NewSubMenuItem, which carries no Role, so the
	// resulting menu is invisible to FindByRole(EditMenu) — including to the
	// OS on platforms that identify the Edit menu that way, and to any test
	// asserting the menu exists.
	//
	// It also gets the platform-correct item set for free: NewEditMenu adds
	// PasteAndMatchStyle and a Speech submenu on darwin and orders SelectAll
	// differently elsewhere, none of which a hand-rolled list tracks as Wails
	// evolves.
	parent.AddRole(application.EditMenu)

	edit := parent.FindByRole(application.EditMenu)
	if edit == nil {
		// AddRole logs and skips on an unsupported role rather than
		// returning an error. Losing Settings entirely is worse than a
		// misplaced Settings, so fall back to a plain submenu.
		sub := parent.AddSubmenu("Edit")
		m.addSettingsItem(sub)
		return
	}
	m.addSettingsItem(edit.GetSubmenu())
}

// addSettingsItem appends the app-specific Settings entry. Standard roles keep
// their OS-supplied labels; only this one comes from our catalog.
func (m *Manager) addSettingsItem(sub *application.Menu) {
	sub.AddSeparator()
	sub.Add(resolveText(labelSettings, m.localizer)).
		SetAccelerator("Ctrl+,").
		OnClick(func(_ *application.Context) {
			m.emit(EventSettingsOpen, nil)
		})
}
