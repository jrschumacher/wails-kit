package wailsbridge

import (
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/jrschumacher/wails-kit/v2/appearance"
	"github.com/jrschumacher/wails-kit/v2/kit"
)

// eventTimeout bounds how long tests wait for an async event delivery
// (app.Event.Emit dispatches to app.Event.On listeners on a goroutine —
// see events_backend_test.go) before failing.
const eventTimeout = 2 * time.Second

// alwaysDarkSource is a fake appearance.Source that always reports dark —
// used to prove Attach's SetAppearanceSource call actually *replaces* the
// previously-wired source rather than merely adding to it. If Attach
// regressed to a no-op here, Resolved() would stay ThemeDark after Attach.
type alwaysDarkSource struct{}

func (alwaysDarkSource) IsDark() bool                         { return true }
func (alwaysDarkSource) Subscribe(func(bool)) (cancel func()) { return func() {} }

func TestAttach_NilArgs(t *testing.T) {
	k := newTestKit(t)
	app := testApp(t)

	if err := Attach(nil, app); err == nil {
		t.Error("expected an error for a nil *kit.Kit")
	}
	if err := Attach(k, nil); err == nil {
		t.Error("expected an error for a nil *application.App")
	}
}

// TestAttach_WiresEventsBackend proves Attach's SetEventsBackend call
// actually retargets Kit.Events at the live app, not just that Attach
// returns no error. Settings.SetValues triggers Kit's onChange wiring,
// which flows through appearance.Service.Refresh — cheaper and just as
// conclusive is emitting directly through k.Events and observing it reach
// app.Event.On, mirroring TestEventsBackend_Emit.
func TestAttach_WiresEventsBackend(t *testing.T) {
	k := newTestKit(t)
	app := testApp(t)

	if err := Attach(k, app, WithoutMenu(), WithoutBindings()); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	const name = "wailsbridge-test:attach-events"
	received := make(chan any, 1)
	off := app.Event.On(name, func(e *application.CustomEvent) { received <- e.Data })
	defer off()

	k.Events.Emit(name, "hello")

	select {
	case data := <-received:
		if data != "hello" {
			t.Errorf("received %#v, want %q", data, "hello")
		}
	case <-time.After(eventTimeout):
		t.Fatal("timed out waiting for k.Events.Emit to reach app.Event.On after Attach")
	}
}

// TestAttach_WiresAppearanceSource proves Attach's SetAppearanceSource call
// replaces whatever source kit.New seeded — including a working one — with
// the live Wails-backed themeSource. A kit built with WithAppearanceSource
// wired to alwaysDarkSource resolves ThemeDark before Attach; after Attach,
// it must resolve ThemeLight (a headless test app's themeSource.IsDark()
// is deterministically false — see appearance_source_test.go), proving
// replacement, not mere addition.
func TestAttach_WiresAppearanceSource(t *testing.T) {
	k := newTestKit(t, kit.WithAppearanceSource(alwaysDarkSource{}))
	app := testApp(t)

	if got := k.Appearance.Resolved(); got != appearance.ThemeDark {
		t.Fatalf("precondition: Resolved() = %v before Attach, want ThemeDark", got)
	}

	if err := Attach(k, app, WithoutMenu(), WithoutBindings()); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	if got := k.Appearance.Resolved(); got != appearance.ThemeLight {
		t.Errorf("Resolved() = %v after Attach, want ThemeLight (Attach must replace the seeded source)", got)
	}
}

func TestAttach_BuildsDefaultMenu(t *testing.T) {
	k := newTestKit(t)
	app := testApp(t)

	if err := Attach(k, app, WithoutBindings()); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	menu := app.Menu.GetApplicationMenu()
	if menu == nil {
		t.Fatal("expected Attach's default shortcuts.Manager to set an application menu")
	}
	if menu.FindByRole(application.FileMenu) == nil {
		t.Error("expected the default menu (WithDefaults) to include a File menu")
	}
}

func TestAttach_WithoutMenu(t *testing.T) {
	k := newTestKit(t)
	app := testApp(t)

	// Establish a known menu first so WithoutMenu's "don't touch it" claim
	// is falsifiable: if Attach called Apply anyway, this would be
	// replaced with the (menu-less) default and the File-menu assertion
	// below would fail.
	app.Menu.SetApplicationMenu(application.NewMenu())
	sentinel := application.NewMenu()
	sentinel.AddRole(application.FileMenu)
	app.Menu.SetApplicationMenu(sentinel)

	if err := Attach(k, app, WithoutMenu(), WithoutBindings()); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	menu := app.Menu.GetApplicationMenu()
	if menu.FindByRole(application.FileMenu) == nil {
		t.Error("expected WithoutMenu to leave the previously-set menu untouched")
	}
}

func TestAttach_WithoutBindings(t *testing.T) {
	k := newTestKit(t)
	app := testApp(t)

	before := len(app.Config().Services)
	if err := Attach(k, app, WithoutMenu(), WithoutBindings()); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	after := len(app.Config().Services)

	if after != before {
		t.Errorf("Services grew from %d to %d despite WithoutBindings", before, after)
	}
}

func TestAttach_RegistersBindings(t *testing.T) {
	k := newTestKit(t)
	app := testApp(t)

	before := len(app.Config().Services)
	if err := Attach(k, app, WithoutMenu()); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	after := len(app.Config().Services)

	if after <= before {
		t.Errorf("Services count = %d after Attach, want > %d (before)", after, before)
	}
}
