// Package wailsbridge is the single adapter between the Wails-free kit
// spine (package kit) and a developer-owned Wails v3 *application.App. Per
// docs/v2-roadmap.md AD-1, kit.New deliberately never touches wails/v3 —
// Prune's CLI and TUI entry points need the exact same *kit.Kit with no
// application.App in the process — so everything that needs a live Wails
// app lives here instead: retargeting the kit's event/appearance seams at
// the real app, building and applying the menu, registering the
// frontend-safe binding surface, persisting window geometry, and setting
// the window background colour from appearance's single resolved Theme.
//
// wailsbridge is on the AD-4 Wails-import allowlist alongside shortcuts,
// windowstate, and permissions — it is the only kit package that may
// import wails/v3 without a specific per-package justification, because
// gluing kit to Wails is the whole reason it exists. Every other kit
// package stays Wails-free; see AGENTS.md, "Dependencies & insulation".
//
// # Usage
//
//	k, err := kit.New(kit.AppInfo{Name: "myapp", Version: "1.0.0"})
//	// ... handle err ...
//	app := application.New(application.Options{Name: "myapp"})
//	if err := wailsbridge.Attach(k, app); err != nil {
//	    log.Fatal(err)
//	}
//	win := app.Window.NewWithOptions(application.WebviewWindowOptions{ /* ... */ })
//	if err := wailsbridge.ManageWindow(k, app, win); err != nil {
//	    log.Fatal(err)
//	}
//	win.Show()
//	app.Run()
package wailsbridge

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/kit"
	"github.com/jrschumacher/wails-kit/v2/permissions"
	"github.com/jrschumacher/wails-kit/v2/shortcuts"
)

// Error codes for the wailsbridge package.
const (
	ErrConfig              errors.Code = "wailsbridge_config"
	ErrWebhookUnconfigured errors.Code = "wailsbridge_webhook_unconfigured"
)

func init() {
	errors.RegisterMessages(map[errors.Code]i18n.Text{
		ErrConfig: i18n.T("wailskit.wailsbridge.errors.config", "This app was not wired up correctly. Please contact support."),
		ErrWebhookUnconfigured: i18n.T("wailskit.wailsbridge.errors.webhook_unconfigured",
			"Diagnostics submission is not configured for this app."),
	})
}

// Option configures Attach and Services.
type Option func(*config)

type config struct {
	shortcuts       *shortcuts.Manager
	withoutMenu     bool
	withoutBindings bool
	permissionsOpts []permissions.Option
	diagWebhookURL  string
}

func newConfig() *config {
	return &config{}
}

// WithShortcuts supplies a pre-built *shortcuts.Manager for Attach to apply
// instead of its default (WithDefaults + WithSettings, wired to the same
// Kit's emitter and localizer). Use this to customize which menus are
// enabled; Attach still calls Apply for you.
func WithShortcuts(m *shortcuts.Manager) Option {
	return func(c *config) { c.shortcuts = m }
}

// WithoutMenu skips menu construction entirely — Attach never calls
// shortcuts.Manager.Apply. Use this if you build and apply your own menu.
func WithoutMenu() Option {
	return func(c *config) { c.withoutMenu = true }
}

// WithoutBindings skips Wails service registration in Attach. Use this
// together with Services(k, opts...) passed to application.Options.Services
// at application.New time instead — calling both would double-register.
func WithoutBindings() Option {
	return func(c *config) { c.withoutBindings = true }
}

// WithPermissionsOptions passes additional permissions.Option values to the
// permissions.Service Attach/Services constructs internally (kit.Kit has no
// Permissions field — see AGENTS.md, "Why permissions.Service is built
// here, not in kit").
func WithPermissionsOptions(opts ...permissions.Option) Option {
	return func(c *config) { c.permissionsOpts = append(c.permissionsOpts, opts...) }
}

// WithDiagnosticsWebhookURL configures the fixed destination the
// diagnostics binding's Submit method uploads to. Without this, Submit
// always returns ErrWebhookUnconfigured — see bindings.go's
// DiagnosticsBinding doc comment for why the webview never gets to choose
// the URL itself.
func WithDiagnosticsWebhookURL(url string) Option {
	return func(c *config) { c.diagWebhookURL = url }
}

// Attach wires a *kit.Kit into a developer-owned *application.App:
//
//  1. Retargets k's events backend at app.Event.Emit (see
//     kit.Kit.SetEventsBackend) — every event k's components emit (settings
//     changes, appearance flips, health transitions, ...) now reaches the
//     frontend.
//  2. Retargets k's appearance source at a live Wails theme signal (see
//     appearance_source.go) — replacing the no-Wails platform default
//     kit.New seeds on darwin with one that actually updates on OS theme
//     flips.
//  3. Builds and applies the application menu via shortcuts.Manager,
//     unless WithoutMenu is given.
//  4. Registers the frontend-safe binding surface (settings, i18n, health,
//     permissions, updates, diagnostics) as Wails services, unless
//     WithoutBindings is given — see bindings.go for exactly what "safe"
//     means for each.
//
// Attach does not touch any window; call ManageWindow separately for
// per-window geometry persistence and background-colour wiring.
func Attach(k *kit.Kit, app *application.App, opts ...Option) error {
	if k == nil {
		return errors.New(ErrConfig, "wailsbridge: k must not be nil", nil)
	}
	if app == nil {
		return errors.New(ErrConfig, "wailsbridge: app must not be nil", nil)
	}

	cfg := newConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	k.SetEventsBackend(newEventsBackend(app))
	k.SetAppearanceSource(newThemeSource(app))

	if !cfg.withoutMenu {
		mgr := cfg.shortcuts
		if mgr == nil {
			mgr = shortcuts.New(
				shortcuts.WithDefaults(),
				shortcuts.WithSettings(),
				shortcuts.WithEmitter(k.Events),
				shortcuts.WithLocalizer(k.I18n),
			)
		}
		mgr.Apply(app)
	}

	if !cfg.withoutBindings {
		for _, svc := range bindingServices(k, cfg) {
			app.RegisterService(svc)
		}
	}

	return nil
}

// Services returns the frontend-safe binding surface as a slice of
// application.Service, for a developer who prefers passing
// application.Options{Services: wailsbridge.Services(k)} to application.New
// instead of letting Attach call app.RegisterService for them. Pair this
// with WithoutBindings on Attach to avoid double-registering — Services
// builds its own permissions.Service instance (kit.Kit has none), so
// calling both Attach's default binding registration and Services would
// register two independent permissions.Service instances, each with its
// own internal state.
func Services(k *kit.Kit, opts ...Option) []application.Service {
	cfg := newConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	return bindingServices(k, cfg)
}
