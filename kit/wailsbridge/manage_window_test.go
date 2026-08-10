package wailsbridge

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/jrschumacher/wails-kit/v2/appearance"
)

func TestManageWindow_NilArgs(t *testing.T) {
	k := newTestKit(t)
	app := testApp(t)
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{})

	if err := ManageWindow(nil, app, win); err == nil {
		t.Error("expected an error for a nil *kit.Kit")
	}
	if err := ManageWindow(k, nil, win); err == nil {
		t.Error("expected an error for a nil *application.App")
	}
	if err := ManageWindow(k, app, nil); err == nil {
		t.Error("expected an error for a nil *application.WebviewWindow")
	}
}

// TestBackgroundFor verifies the light/dark RGBA mapping matches
// appearance/README.md's own recommended values — the whole point of
// wiring this in ManageWindow is that a caller relying on it and a caller
// following that README by hand land on the same colours.
func TestBackgroundFor(t *testing.T) {
	if got := backgroundFor(appearance.ThemeLight); got != backgroundLight {
		t.Errorf("backgroundFor(ThemeLight) = %v, want %v", got, backgroundLight)
	}
	if got := backgroundFor(appearance.ThemeDark); got != backgroundDark {
		t.Errorf("backgroundFor(ThemeDark) = %v, want %v", got, backgroundDark)
	}
}

// TestManageWindow_SucceedsAndSurvivesAppearanceChange exercises
// ManageWindow's full happy path against a real (headless) *application.App
// and *application.WebviewWindow: it must return no error, and the
// appearance:changed subscription it registers on k.Events must not panic
// or deadlock when a mode change fires it (win.SetBackgroundColour is a
// no-op against a headless window with no platform impl — see
// webview_window.go's own `if w.impl != nil` guard — so this cannot
// observe the resulting colour; *application.WebviewWindow exposes no
// getter for it either. See AGENTS.md, "Not automatable in this suite".
func TestManageWindow_SucceedsAndSurvivesAppearanceChange(t *testing.T) {
	k := newTestKit(t)
	app := testApp(t)
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{})

	if err := ManageWindow(k, app, win); err != nil {
		t.Fatalf("ManageWindow: %v", err)
	}

	if err := k.Appearance.SetMode(appearance.ModeDark); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	if err := k.Appearance.SetMode(appearance.ModeLight); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
}
