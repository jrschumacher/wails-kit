# kit/wailsbridge

Package `wailsbridge` is the single adapter between a Wails-free `*kit.Kit`
(package `kit`) and a developer-owned Wails v3 `*application.App`. Per
`docs/v2-roadmap.md` AD-1, `kit.New` never touches `wails/v3` — Prune's CLI
and TUI entry points need the exact same wired spine with no
`application.App` in the process — so every piece of Wails glue lives here
instead.

## Why this package exists

`kit.New` builds `appdirs`, `logging`, `events`, `keyring`, `i18n`,
`settings`, `appearance`, `health`, `firstrun`, `updates`, and
`diagnostics`, but two of its seams are intentionally inert until a real
Wails app exists: `Kit.Events` drops everything it's given, and
`Kit.Appearance` only sees the OS theme once (or never, off darwin). A
GUI entry point calls `wailsbridge.Attach` once to close both seams, build
and apply the application menu, and register the frontend-safe binding
surface. `wailsbridge.ManageWindow` does the same for a specific window:
geometry persistence and background-colour sync.

## Quickstart

```go
import (
    "log"

    "github.com/jrschumacher/wails-kit/v2/kit"
    "github.com/jrschumacher/wails-kit/v2/kit/wailsbridge"
    "github.com/wailsapp/wails/v3/pkg/application"
)

k, err := kit.New(kit.AppInfo{Name: "myapp", ID: "com.example.myapp", Version: "1.0.0"})
if err != nil {
    log.Fatal(err)
}

app := application.New(application.Options{Name: "myapp"})
if err := wailsbridge.Attach(k, app); err != nil {
    log.Fatal(err)
}

win := app.Window.NewWithOptions(application.WebviewWindowOptions{
    Title: "myapp",
})
if err := wailsbridge.ManageWindow(k, app, win); err != nil {
    log.Fatal(err)
}

win.Show()
app.Run()
```

## What `Attach` wires

| Seam | How |
|---|---|
| Events | `k.SetEventsBackend` retargeted at `app.Event.Emit` — every event any kit component emits now reaches the frontend. |
| Appearance | `k.SetAppearanceSource` retargeted at a live Wails theme source (`app.Env.IsDarkMode()` + `events.Common.ThemeChanged`) — replaces whatever `kit.New` seeded, including the working no-Wails darwin default, with one that updates on OS theme flips. |
| Menu | A `shortcuts.Manager` (default: `WithDefaults()` + `WithSettings()`, wired to `k.Events`/`k.I18n`) applies the application menu. Override with `WithShortcuts`, skip with `WithoutMenu`. |
| Bindings | `settings.Binding`, `i18n.Binding`, `health.Binding` (if `k.Health != nil`), `permissions.Binding`, `UpdatesBinding` (if `k.Updates != nil`), `DiagnosticsBinding` (if `k.Diag != nil`) — registered via `app.RegisterService`. Skip with `WithoutBindings`. |

`Attach` does not touch any window — call `ManageWindow` separately, once
per window.

## What `ManageWindow` wires

- `windowstate.Manage` + `Restore()` — geometry persistence, clamped to a
  currently attached display. Pass `windowstate.Option` values (e.g.
  `windowstate.WithName` for a second window) as trailing arguments.
- `win.SetBackgroundColour` from `k.Appearance.Resolved()`, kept in sync
  for the life of the window via an `appearance:changed` subscription —
  the third layer of the seam `appearance/README.md` documents (window
  frame / base CSS / component CSS must all agree). Default colours match
  that README's own recommended snippet (`RGB(255,255,255)` light,
  `RGB(23,23,23)` dark); override by calling `win.SetBackgroundColour`
  yourself after `ManageWindow` returns.

Call `ManageWindow` after `Attach` and before `win.Show()` — `Restore()`
runs synchronously inside the call.

## Options

| Option | Applies to | Effect |
|---|---|---|
| `WithShortcuts(m)` | `Attach` | Use a pre-built `*shortcuts.Manager` instead of the default. |
| `WithoutMenu()` | `Attach` | Skip menu construction entirely. |
| `WithoutBindings()` | `Attach` | Skip binding registration — pair with `Services(k)` passed to `application.Options.Services`. |
| `WithPermissionsOptions(opts...)` | `Attach`, `Services` | Extra `permissions.Option` values for the internal `permissions.Service`. |
| `WithDiagnosticsWebhookURL(url)` | `Attach`, `Services` | Fixed destination for `DiagnosticsBinding.Submit`. Without this, `Submit` always refuses. |

`windowstate.Option` values pass straight through to `ManageWindow` — they
are not `wailsbridge.Option`.

## `Services` — the `application.Options.Services` alternative

```go
app := application.New(application.Options{
    Name:     "myapp",
    Services: wailsbridge.Services(k),
})
if err := wailsbridge.Attach(k, app, wailsbridge.WithoutBindings()); err != nil {
    log.Fatal(err)
}
```

Use `WithoutBindings` on `Attach` when doing this — `Services` builds its
own internal `permissions.Service`, so calling both would register two
independent instances.

## Security: why bindings are never raw Services

Wails v3 binds every exported method of a registered service. `settings`,
`i18n`, `health`, and `permissions` each ship a narrow `*Binding` type for
exactly this reason (registering `*settings.Service` directly would
publish `GetSecret` — raw, unmasked API keys — to webview JS; this was a
real Phase 0 finding). `updates` and `diagnostics` don't ship one yet, so
this package supplies `UpdatesBinding` and `DiagnosticsBinding`:

- `UpdatesBinding` mirrors `*updates.Service` 1:1 — every exported method
  is niladic or takes only a `context.Context`, so there's nothing to
  narrow, but the mirror still decouples the webview-exposed surface from
  whatever `updates.Service` grows next.
- `DiagnosticsBinding` is **not** a mirror. `diagnostics.Service.CreateBundle`
  and `Submit` both take a caller-chosen filesystem path or URL — safe from
  Go code, not safe as a direct webview binding (an arbitrary path is a
  traversal primitive; an arbitrary URL turns "submit" into exfiltration).
  `DiagnosticsBinding.CreateBundle` always writes to `k.Dirs.Cache()`;
  `Submit` always uploads to the URL from `WithDiagnosticsWebhookURL` and
  refuses (`ErrWebhookUnconfigured`) if none was configured — neither
  parameter is ever caller-supplied on the webview-facing surface.

## Errors

| Code | Meaning |
|---|---|
| `wailsbridge_config` | `nil` `*kit.Kit`, `*application.App`, or `*application.WebviewWindow` passed to `Attach`/`ManageWindow`. |
| `wailsbridge_webhook_unconfigured` | `DiagnosticsBinding.Submit` called without `WithDiagnosticsWebhookURL`. |

## Testing without a display

Every test in this package builds a real, headless `*application.App` via
`application.New` (never calling `Run()`) — the same pattern
`shortcuts/apply_test.go` established. `wails/v3`'s `*App` is a
process-wide singleton, so all tests in the package share one instance via
a `sync.Once`-guarded helper (`testkit_test.go`). See `AGENTS.md` for what
isn't automatable this way (real OS theme-change delivery, reading a
window's live background colour back).
