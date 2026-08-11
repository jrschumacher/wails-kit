# kit

Package `kit` is the wired-components constructor for a Wails v3 desktop
app: `appdirs`, `logging`, `events`, `keyring`, `i18n`, `settings`,
`appearance`, `health`, `firstrun`, `updates`, `diagnostics`, and
`lifecycle`, composed in the order that makes each one work correctly, with
one call — `kit.New` — instead of a dozen.

## Why this package exists

Standing up a new Wails app means importing about a dozen wails-kit packages
and wiring them in the right order with the right options. That's exactly
the boilerplate wails-kit is supposed to eliminate — it was just moved up a
level. `kit.New` is where it goes.

`kit` is **not** a composition root. It does not construct or own an
`*application.App`, and it imports no Wails code at all (verified by CI —
see "kit is Wails-free" below). It returns a `*kit.Kit`: a struct of
already-wired components. A Cobra CLI or Bubbletea TUI entry point calls
`kit.New` and `kit.Start` and stops there. A GUI entry point does the same,
then additionally calls `kit/wailsbridge.Attach(k, app)` to wire the same
`*Kit` into a developer-owned `*application.App`. See
`docs/v2-roadmap.md`, AD-1, for the full rationale.

## Quickstart

```go
import (
    "context"

    "github.com/jrschumacher/wails-kit/v2/kit"
)

k, err := kit.New(kit.AppInfo{
    Name:    "prune",              // drives appdirs, keyring service name, log paths
    ID:      "com.aboldnewlook.prune",
    Version: "1.4.0",              // must parse as semver
})
if err != nil {
    // See "Partial failure is legible" below.
    log.Fatal(err)
}

ctx := context.Background()
if err := k.Start(ctx); err != nil {
    log.Fatal(err)
}
defer k.Close()

// Everything else is reachable directly — no facade hides it:
k.Settings.GetSchema()
k.Health.Snapshot()
k.Appearance.Resolved()
```

A CLI or TUI process stops here. A GUI process additionally does:

```go
app := application.New(application.Options{ /* ... */ })
if err := wailsbridge.Attach(k, app); err != nil { // kit/wailsbridge, WP-31
    log.Fatal(err)
}
```

## What `New` wires, and in what order

Order is the point — get it wrong once, by hand, in every app, or encode it
here once. See `AGENTS.md` for the full reasoning behind each step; the
short version:

1. **appdirs** — every other on-disk path (log, settings, keyring,
   firstrun, diagnostics) is rooted here.
2. **logging** — needs a log directory, hence after appdirs.
3. **events** — one `*events.Emitter` shared by everything that follows.
   Its backend starts as a drop-everything placeholder; a CLI/TUI process
   never changes that (see "Events before Attach", below).
4. **keyring** — an `EnvelopeStore` over the OS keyring by default (AD-8);
   before settings, which needs it for secret fields.
5. **i18n** — before settings, because `settings.LocaleGroup` needs a built
   `*i18n.Localizer` to enumerate its locale picker's options.
6. **settings** — registers the kit's own groups (locale, appearance,
   health, updates, diagnostics — whichever end up enabled) before any
   app-supplied group (`WithSettingsGroup`).
7. **appearance** — always on; wired to `Settings` so a change made through
   a generic settings form (not just `SetMode`) still triggers a resolve +
   `appearance:changed`.
8. **firstrun** — detected (not run) here; hooks run in `Start`.
9. **updates** — opt-in via `WithGitHubRepo`.
10. **diagnostics** — on by default.
11. **lifecycle** — registers the background services (health's probe loop,
    updates' check-frequency ticker) in dependency order.

`Start(ctx)` runs firstrun's hooks, then starts every lifecycle-managed
background service. `Close()` reverses it.

## Secure defaults, opt-out not opt-in

Every component is on unless you turn it off:

| Option | Effect |
|---|---|
| `WithoutHealth()` | `Kit.Health` stays `nil`; no health settings group; nothing (including Updates) can register a check. |
| `WithoutDiagnostics()` | `Kit.Diag` stays `nil`; no consent settings group. |
| `WithoutUpdates()` | `Kit.Updates` stays `nil`, even if `WithGitHubRepo` is also given. |

Updates is the one exception to "on by default": there's nothing to check
without a repo, so it's off until `WithGitHubRepo(owner, repo)` turns it on.

## Options

| Option | Purpose |
|---|---|
| `WithDirs(d)` | Inject an `*appdirs.Dirs` instead of deriving one from `AppInfo.Name`. Tests; consumers with a pre-existing `Dirs`. |
| `WithKeyring(store)` | Override the default `EnvelopeStore`. Tests: `keyring.NewMemoryStore()`. |
| `WithLogLevel(level)` | `"debug"`, `"info"` (default), `"warn"`, `"error"`. |
| `WithLocale(tag)` | Force the initial resolved locale (e.g. a CLI `--locale` flag). |
| `WithLocales(fsys)` | Add an app catalog (`locales/<bcp47>.json`). Repeatable. |
| `WithSettingsGroup(g)` | Add an app-defined `settings.Group`, after every kit-internal group. Repeatable. |
| `WithSettingsStoragePath(path)` | Workspace-local settings (git-tracked config) instead of the OS config directory — see "Prune's workspace settings" below. |
| `WithoutUpdates()` | Force `Updates` off regardless of `WithGitHubRepo`. |
| `WithoutHealth()` | Force `Health` off. |
| `WithoutDiagnostics()` | Force `Diag` off. |
| `WithGitHubRepo(owner, repo)` | The only thing that turns `Updates` on. |
| `WithUpdatesOptions(opts...)` | Pass-through to `updates.NewService` — e.g. `updates.WithPublicKey` for signature verification. |
| `WithHealthCheck(check)` | Register an additional `health.Check` once `Health` exists. |
| `WithHealthOptions(opts...)` | Pass-through to `health.New` — e.g. `health.WithConnectivityProbe` (every real app should set this) or `health.WithoutDefaultConnectivityCheck` (tests). |
| `WithFirstRunHook(h)` | Append a `firstrun.Hook`. Repeatable; registration order is evaluation order. |
| `WithFirstRunBaseline(version)` | For an app adopting `kit` mid-life — see `firstrun.WithBaselineVersion`. |
| `WithAppearanceSource(s)` | Pre-seed the OS theme signal — equivalent to calling `SetAppearanceSource` right after `New`. |

## `Kit` fields

```go
type Kit struct {
    Info       AppInfo
    Dirs       *appdirs.Dirs
    Logger     *slog.Logger
    Events     *events.Emitter
    Keyring    keyring.Store
    Settings   *settings.Service
    I18n       *i18n.Localizer
    Appearance *appearance.Service
    Health     *health.Registry   // nil when WithoutHealth()
    Updates    *updates.Service   // nil unless WithGitHubRepo() (and not WithoutUpdates())
    FirstRun   *firstrun.Service
    Diag       *diagnostics.Service // nil when WithoutDiagnostics()
    Lifecycle  *lifecycle.Manager
}
```

Every field is exported and directly usable — `kit` is a constructor, not a
facade. Reach for `k.Settings.Binding()`, `k.Health.Trigger(...)`,
`k.I18n.T(...)`, whatever the app needs; nothing here re-wraps a component's
own API.

## Partial failure is legible

If one component fails to construct, `New` returns an error naming *which*
one — not a bare wrapped string:

```go
k, err := kit.New(info, opts...)
if err != nil {
    if comp, ok := kit.GetComponent(err); ok {
        log.Fatalf("kit: %s failed to initialize: %v", comp, err)
    }
    log.Fatal(err)
}
```

`comp` is one of `"kit"` (AppInfo itself — currently only an empty `Name`),
`"appdirs"`, `"logging"`, `"keyring"`, `"i18n"`, `"firstrun"`, `"updates"`,
`"diagnostics"`, or `"lifecycle"` — whichever step failed first (`settings`
and `appearance` construction never fail — see their own package docs).

## Prune's workspace settings

Prune keeps its config in git, alongside project files, so it stays
human-readable and reviewable by an LLM — not tucked away in the OS config
directory. `WithSettingsStoragePath` is exactly that:

```go
kit.New(info, kit.WithSettingsStoragePath(filepath.Join(workspaceDir, "config.json")))
```

Password/secret fields still go through `Keyring` regardless — this option
only relocates `settings.json`.

## Events before `Attach`

`Kit.Events` is fully usable from construction — `Settings`, `Appearance`,
`Health`, `FirstRun`, and `Updates` all emit through it — but nothing is
listening until something calls `Kit.SetEventsBackend`. A CLI/TUI process
never does; every event is silently dropped, which is correct: there's
nothing subscribed to a Backend in a process with no window. A GUI process
gets this wired automatically by `kit/wailsbridge.Attach`.

## Appearance before `Attach`

Similarly, `Kit.Appearance.Resolved()` reports light until something calls
`Kit.SetAppearanceSource` — even on macOS, where the `appearance` package
ships a real (but static, non-live) native default. `kit.New` doesn't use
that default so every platform and every entry point behaves the same way
until a real Source is wired in. See `AGENTS.md`, "Landmines" for the
detail and the trade-off.

## `kit` is Wails-free

`kit` is not on the AD-4 Wails-import allowlist (`docs/v2-roadmap.md`) —
only `shortcuts`, `windowstate`, `permissions`, and `kit/wailsbridge` may
import `github.com/wailsapp/wails/v3`. This is the property that makes the
CLI/TUI use case (`examples/kit-headless`) work at all: a Cobra command or a
Bubbletea program can import `kit` without pulling in a GUI toolkit,
X11/Cocoa/Win32 bindings, or an event loop it will never run. Enforced two
ways:

- `.github/scripts/check-wails-imports.sh` in CI, repo-wide.
- `go test ./kit/... -run TestNoWailsImport`, scoped to this package, so the
  property shows up in a normal `go test` run too.

## Example

`examples/kit-headless/main.go` — settings (including a password field),
`Start`/`Close`, a manual health check, and `SetAppearanceSource`, all with
no `application.App` anywhere in the binary. Run it:

```sh
go run ./examples/kit-headless/
```
