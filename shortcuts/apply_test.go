package shortcuts

import (
	"runtime"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// frenchCatalog is a minimal, hand-built catalog overriding just the two
// labels apply.go resolves through a localizer — enough to prove Apply
// actually threads Manager.localizer through to the menu items it builds,
// without dragging in a full translation.
func frenchCatalog(t *testing.T) *i18n.Localizer {
	t.Helper()
	fr := fstest.MapFS{
		"locales/fr.json": &fstest.MapFile{Data: []byte(`{
			"wailskit.shortcuts.settings.label": "Paramètres",
			"wailskit.shortcuts.settings.label_darwin": "Paramètres…"
		}`)},
	}
	loc, err := i18n.New(i18n.WithCatalog(fr))
	if err != nil {
		t.Fatalf("i18n.New: %v", err)
	}
	if err := loc.SetLocale("fr"); err != nil {
		t.Fatalf("SetLocale: %v", err)
	}
	return loc
}

// Apply's menu-building logic sat behind an undocumented build tag
// ((darwin||linux||windows) && wails) until WP-06 removed it. That meant no
// CI run ever compiled, vetted, or tested this file — the package's
// reported "100% coverage" only ever covered the pure option setters in
// shortcuts.go. These tests exercise Apply itself for the first time.
//
// wails/v3's *application.App is a process-wide singleton: application.New
// stores the first result in a package-level variable and every subsequent
// call returns that same instance regardless of the Options passed
// (see application.go:New — "if globalApplication != nil { return
// globalApplication }"). So every test in this file must share one App
// rather than creating its own; see testApp below.
var (
	testAppOnce sync.Once
	testAppVal  *application.App
)

const testAppName = "wails-kit-shortcuts-test"

func testApp(t *testing.T) *application.App {
	t.Helper()
	testAppOnce.Do(func() {
		testAppVal = application.New(application.Options{Name: testAppName})
	})
	return testAppVal
}

func TestApply_FileMenu(t *testing.T) {
	app := testApp(t)
	New(WithFileMenu()).Apply(app)

	menu := app.Menu.GetApplicationMenu()
	if menu == nil {
		t.Fatal("expected application menu to be set")
	}
	if item := menu.FindByRole(application.FileMenu); item == nil {
		t.Error("expected a File menu (role FileMenu)")
	}
}

func TestApply_EditMenu(t *testing.T) {
	app := testApp(t)
	New(WithEditMenu()).Apply(app)

	menu := app.Menu.GetApplicationMenu()
	editItem := menu.FindByRole(application.EditMenu)
	if editItem == nil {
		t.Fatal("expected an Edit menu (role EditMenu)")
	}
	for _, role := range []application.Role{application.Undo, application.Redo, application.Cut, application.Copy, application.Paste, application.SelectAll} {
		if menu.FindByRole(role) == nil {
			t.Errorf("expected Edit menu to contain role %v", role)
		}
	}
}

func TestApply_ViewMenu(t *testing.T) {
	app := testApp(t)
	New(WithViewMenu()).Apply(app)

	menu := app.Menu.GetApplicationMenu()
	if menu.FindByRole(application.ViewMenu) == nil {
		t.Error("expected a View menu (role ViewMenu)")
	}
}

func TestApply_WindowMenu(t *testing.T) {
	app := testApp(t)
	New(WithWindowMenu()).Apply(app)

	menu := app.Menu.GetApplicationMenu()
	if menu.FindByRole(application.WindowMenu) == nil {
		t.Error("expected a Window menu (role WindowMenu)")
	}
}

func TestApply_Defaults(t *testing.T) {
	app := testApp(t)
	New(WithDefaults()).Apply(app)

	menu := app.Menu.GetApplicationMenu()
	if menu.FindByRole(application.FileMenu) == nil {
		t.Error("expected File menu from WithDefaults")
	}
	if menu.FindByRole(application.EditMenu) == nil {
		t.Error("expected Edit menu from WithDefaults")
	}
	if menu.FindByRole(application.ViewMenu) == nil {
		t.Error("expected View menu from WithDefaults")
	}
	if menu.FindByRole(application.WindowMenu) == nil {
		t.Error("expected Window menu from WithDefaults")
	}
	if runtime.GOOS == "darwin" {
		if menu.FindByRole(application.AppMenu) == nil {
			t.Error("expected App menu from WithDefaults on darwin")
		}
	}
}

func TestApply_NoOptionsProducesEmptyMenu(t *testing.T) {
	app := testApp(t)
	New().Apply(app)

	menu := app.Menu.GetApplicationMenu()
	if menu == nil {
		t.Fatal("expected Apply to always set an application menu, even with no options")
	}
	if menu.FindByRole(application.FileMenu) != nil {
		t.Error("expected no File menu when no options are set")
	}
}

// TestApply_SettingsItemWiring verifies the Settings menu item Apply builds:
// correct label, correct platform accelerator, and (via a nil-emitter
// Manager) that clicking-path construction doesn't require an emitter to
// exist. The click callback itself forwards to Manager.emit, which is
// covered directly by TestEmitWithEmitter in shortcuts_test.go — wails'
// MenuItem exposes no public way to synthesize a native click from a test,
// since click delivery is normally triggered by the platform's real menu
// implementation (menuItemImpl), not by application-level Go code.
func TestApply_SettingsItemWiring(t *testing.T) {
	app := testApp(t)
	New(WithSettings()).Apply(app)

	menu := app.Menu.GetApplicationMenu()
	label := "Settings"
	wantAccel := "Ctrl+,"
	if runtime.GOOS == "darwin" {
		label = "Settings…"
		// accelerator.String() resolves the platform-neutral "CmdOrCtrl"
		// modifier to its darwin label ("Cmd") — see keys.go
		// modifierStringMap — so the accelerator we set as "CmdOrCtrl+,"
		// round-trips as "Cmd+,", not literally "CmdOrCtrl+,".
		wantAccel = "Cmd+,"
	}

	item := menu.FindByLabel(label)
	if item == nil {
		t.Fatalf("expected to find menu item labeled %q", label)
	}
	if got := item.Label(); got != label {
		t.Errorf("Label() = %q, want %q", got, label)
	}
	if got := item.GetAccelerator(); got != wantAccel {
		t.Errorf("GetAccelerator() = %q, want %q", got, wantAccel)
	}
}

// TestApply_SettingsPlacement documents and verifies the documented
// placement split from shortcuts.go's WithSettings doc comment: on darwin
// the Settings item lives in the app menu (About/Services/Hide/Quit); on
// other platforms it is appended to the Edit menu, and WithSettings alone
// (no WithEditMenu) still produces an Edit menu to hold it.
func TestApply_SettingsPlacement(t *testing.T) {
	app := testApp(t)
	New(WithSettings()).Apply(app)

	menu := app.Menu.GetApplicationMenu()

	if runtime.GOOS == "darwin" {
		appItem := menu.FindByRole(application.About)
		if appItem == nil {
			t.Fatal("expected the darwin app menu (containing About) when WithSettings is set")
		}
		if menu.FindByLabel("Settings…") == nil {
			t.Error("expected Settings… item in the darwin app menu")
		}
		// WithSettings must not, by itself, add a separate Edit menu on darwin.
		if menu.FindByRole(application.EditMenu) != nil {
			t.Error("did not expect an Edit menu from WithSettings alone on darwin")
		}
	} else {
		editItem := menu.FindByRole(application.EditMenu)
		if editItem == nil {
			t.Fatal("expected an Edit menu to be synthesized to hold Settings on non-darwin")
		}
		if menu.FindByLabel("Settings") == nil {
			t.Error("expected a Settings item in the Edit menu on non-darwin")
		}
	}
}

// TestApply_MultipleCallsReplaceMenu guards against Apply accumulating
// menu items across repeated calls (e.g. a caller re-applying shortcuts
// after a settings change).
func TestApply_MultipleCallsReplaceMenu(t *testing.T) {
	app := testApp(t)
	New(WithFileMenu()).Apply(app)
	New(WithViewMenu()).Apply(app)

	menu := app.Menu.GetApplicationMenu()
	if menu.FindByRole(application.FileMenu) != nil {
		t.Error("expected the second Apply call to replace, not accumulate onto, the first menu")
	}
	if menu.FindByRole(application.ViewMenu) == nil {
		t.Error("expected the second Apply call's View menu to be present")
	}
}

// TestApply_SettingsItemWiring_Localized verifies WithLocalizer actually
// changes the Settings item's label — proving Apply threads
// Manager.localizer through to labels.go's resolveText calls, not just
// that the option setter stores it (that part is covered by
// TestWithLocalizer in shortcuts_test.go, which doesn't call Apply at
// all). Standard-role items are untouched by this: About/Services/Hide/
// Quit/Edit's role children keep rendering the OS's own localized text
// regardless of Manager.localizer, per this package's Invariants.
func TestApply_SettingsItemWiring_Localized(t *testing.T) {
	app := testApp(t)
	loc := frenchCatalog(t)
	New(WithSettings(), WithLocalizer(loc)).Apply(app)

	menu := app.Menu.GetApplicationMenu()
	label := "Paramètres"
	if runtime.GOOS == "darwin" {
		label = "Paramètres…"
	}
	if menu.FindByLabel(label) == nil {
		t.Errorf("expected a menu item labeled %q with a French localizer wired", label)
	}
	if menu.FindByLabel("Settings") != nil || menu.FindByLabel("Settings…") != nil {
		t.Error("expected the English fallback label to be gone once a localizer resolves the key")
	}
}

// TestAddEditMenuWithSettings_Direct exercises the non-darwin Settings
// placement path on any host OS.
//
// TestApply_SettingsPlacement only reaches this code when GOOS != darwin, so
// on a macOS dev machine it is never run — and CI runs on Linux, where a
// failure would surface for the first time. That gap already hid a real bug:
// the menu was built with AddSubmenu("Edit"), which produces a MenuItem with
// no Role, so FindByRole(EditMenu) returned nil and the Linux branch of
// TestApply_SettingsPlacement would have fataled. Calling the builder
// directly removes the platform dependence from the assertion.
func TestAddEditMenuWithSettings_Direct(t *testing.T) {
	app := testApp(t)
	_ = app // keep the application initialized for menu construction

	m := New(WithSettings())
	menu := application.NewMenu()
	m.addEditMenuWithSettings(menu)

	edit := menu.FindByRole(application.EditMenu)
	if edit == nil {
		t.Fatal("Edit menu is not discoverable by role; AddSubmenu does not assign one — use AddRole(EditMenu)")
	}

	sub := edit.GetSubmenu()
	if sub == nil {
		t.Fatal("Edit menu item has no submenu")
	}
	if sub.FindByLabel("Settings") == nil {
		t.Error("Settings item missing from the Edit menu")
	}

	// The standard roles must still be present — the point of AddRole is
	// getting the platform-correct set rather than a hand-rolled list.
	for _, r := range []application.Role{application.Undo, application.Cut, application.Copy, application.Paste} {
		if sub.FindByRole(r) == nil {
			t.Errorf("standard Edit role %v missing", r)
		}
	}
}
