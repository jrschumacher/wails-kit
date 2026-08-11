# kit/wailsbridge — agent notes

## Purpose

`wailsbridge` is the single adapter between the Wails-free `*kit.Kit`
(package `kit`) and a developer-owned `*application.App` (Wails v3
beta.4). It owns: retargeting `Kit.Events`/`Kit.Appearance` at a live app,
building/applying the menu, registering the frontend-safe binding
surface, and per-window geometry + background-colour wiring. It does not
own any component's own logic (that stays in `settings`, `appearance`,
etc.) and does not own the `*application.App` itself — the developer
constructs and runs it; this package only wires things onto it.

## Public API (load-bearing signatures)

```go
func Attach(k *kit.Kit, app *application.App, opts ...Option) error
func ManageWindow(k *kit.Kit, app *application.App, win *application.WebviewWindow, opts ...windowstate.Option) error
func Services(k *kit.Kit, opts ...Option) []application.Service

func WithShortcuts(m *shortcuts.Manager) Option
func WithoutMenu() Option
func WithoutBindings() Option
func WithPermissionsOptions(opts ...permissions.Option) Option
func WithDiagnosticsWebhookURL(url string) Option

type UpdatesBinding struct{ /* unexported */ }
type DiagnosticsBinding struct{ /* unexported */ }
```

## Invariants (do not break)

- **Never register a raw Service.** `settings.Service`, `i18n.Localizer`,
  `health.Registry`, `permissions.Service`, `updates.Service`,
  `diagnostics.Service` are never passed to `application.NewService`
  directly — only `.Binding()` results, or `UpdatesBinding`/
  `DiagnosticsBinding`. Wails binds every exported method of a registered
  service; `settings.Service.GetSecret` reaching the webview was a real
  Phase 0 finding. `bindingInstances` is the one place this list is built —
  extend it, don't bypass it — and `TestBindingInstancesNeverRegisterRawService`
  (bindings_test.go) enforces it by type-switch, failing loudly on an
  unrecognized type rather than silently allowing one.
- **`DiagnosticsBinding` never takes a path or URL from the caller.**
  `diagnostics.Service.CreateBundle`/`Submit` accept a caller-chosen
  filesystem path / webhook URL — fine from Go code, a traversal/
  exfiltration primitive as a direct webview binding. `bundleDir` and
  `webhookURL` are fixed at construction (`k.Dirs.Cache()`, config-time
  `WithDiagnosticsWebhookURL`), never per-call arguments.
- **`Attach` replaces the appearance source, it does not merely add one.**
  `kit.New` always seeds `Kit.Appearance` with something — the no-Wails
  darwin default, or nothing off darwin — via `dynamicSource` (see
  `kit/appearance_source.go`). `SetAppearanceSource` swaps it out
  entirely; `TestAttach_WiresAppearanceSource` proves this by wiring a
  fake always-dark source first and asserting `Resolved()` flips to light
  after `Attach` (a headless test app's `themeSource.IsDark()` is
  deterministically false).
- **Never emit while holding a lock.** This package holds no locks of its
  own; the landmine is indirect — `manage_window.go`'s
  `appearance.EventChanged` handler and `events_backend.go`'s `Emit` must
  stay lock-free wrappers, not grow one.
- **`ManageWindow` takes `app`, not just `win`** — a deliberate deviation
  from `docs/v2-roadmap.md` AD-1's stub signature. `*application.WebviewWindow`
  exposes no way to recover its owning `*App` (needed by
  `windowstate.Manage` for `app.Screen`), and `kit.Kit` (owned by WP-30,
  out of this package's reach) stores no `*application.App` either. See
  Landmines.

## Dependencies & insulation

- `wails/v3/pkg/application`, `wails/v3/pkg/events` — the whole point of
  this package (AD-4 allowlist: `shortcuts`, `windowstate`, `permissions`,
  `kit/wailsbridge`).
- `kit` — the spine this package attaches to. Never the reverse import.
- `shortcuts`, `windowstate`, `permissions` — the other AD-4 allowlist
  members; this package is what composes them with a `*kit.Kit`.
- `appearance`, `settings`, `i18n`, `health`, `updates`, `diagnostics`,
  `events`, `errors` — Wails-free kit packages, consumed for their
  `.Binding()` surfaces and types.

## Extension points

- New bindable component: add a case to both `bindingInstances`
  (bindings.go) and `bindingServices`'s type switch. If the component has
  no narrow `Binding` type yet, write one here (see `UpdatesBinding`) —
  don't register the raw service and don't wait for the upstream package
  to grow one.
- New `Attach`/`Services` configuration: add a `config` field + `With*`
  `Option` in `wailsbridge.go`, following the existing pattern.

## Testing

- Every test builds a real, headless `*application.App` via
  `application.New(application.Options{...})`, **never calling `Run()`** —
  identical to `shortcuts/apply_test.go`. `wails/v3`'s `*App` is a
  process-wide singleton (`globalApplication`); all tests in this package
  share one via `testApp(t)` (`testkit_test.go`, `sync.Once`). `newTestKit`
  mirrors `kit/kit_test.go`'s `baseOpts` (temp-dir `Dirs`, memory keyring,
  no default health probe) — duplicated rather than imported because
  those helpers are unexported in package `kit`.
- `go test -race ./kit/wailsbridge/` must cover: nil-arg rejection
  (`Attach`, `ManageWindow`); events backend reaching `app.Event.On`
  (`TestEventsBackend_Emit`, `TestAttach_WiresEventsBackend`); appearance
  source *replacement*, not addition (`TestAttach_WiresAppearanceSource`);
  default menu construction and `WithoutMenu` (`TestAttach_BuildsDefaultMenu`,
  `TestAttach_WithoutMenu`); binding registration count and
  `WithoutBindings` (`TestAttach_RegistersBindings`,
  `TestAttach_WithoutBindings`); the raw-service guard
  (`TestBindingInstancesNeverRegisterRawService`); `DiagnosticsBinding.Submit`
  refusing without a configured URL.
- **Not automatable in this suite**: real OS theme-change delivery
  (`themeSource.Subscribe`'s callback — `events.Common.ThemeChanged` is
  dispatched by platform code paths, `ApplicationEventContext.setIsDarkMode`
  is unexported, so no test can synthesize one) and reading a window's
  live background colour back (`*application.WebviewWindow.options` is
  unexported with no getter, and `SetBackgroundColour`'s platform call is
  a no-op without a real `impl`, i.e. without `Run()`/a display). Verify
  both manually on a real desktop session before trusting a change to
  `appearance_source.go` or `manage_window.go`'s colour wiring.

## File map

- `wailsbridge.go` — `Attach`, `Services`, `Option`/`config`, error codes.
- `events_backend.go` — `eventsBackend` (`events.Backend` over `app.Event.Emit`).
- `appearance_source.go` — `themeSource` (`appearance.Source` over `app.Env`/`events.Common.ThemeChanged`).
- `bindings.go` — `bindingInstances`, `bindingServices`, `UpdatesBinding`, `DiagnosticsBinding`.
- `manage_window.go` — `ManageWindow`, `backgroundFor`, default colours.
- `testkit_test.go` — `newTestKit`, `testApp` shared test helpers.

## Landmines

- `ManageWindow`'s `app` parameter (see Invariants) means callers porting
  from a literal reading of AD-1's stub will get a compile error, not a
  silently wrong result — a deliberate tradeoff over inventing a stateful
  "Bridge" handle `Attach` would have to return instead.
- `updates`/`diagnostics` having no upstream `.Binding()` type is a gap in
  those packages, not this one — flagged in the WP-31 final report. If
  either package grows one later, prefer switching this package's binding
  to it only if it is at least as narrow as `UpdatesBinding`/
  `DiagnosticsBinding` already are.
