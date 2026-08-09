# shortcuts — agent notes

## Purpose

`shortcuts` builds native application menus with standard keyboard
shortcuts for Wails v3 apps (`Manager.Apply(app)`), handling macOS-vs-other
platform differences (About/Services/Hide/Quit placement, the Settings
shortcut's home in the app menu vs. the Edit menu). It owns menu
*construction*; it does not own menu item click handling beyond forwarding
one event (`EventSettingsOpen`) through the kit's `events.Emitter`, and it
does not own label translation yet (arrives in WP-31, per the roadmap).

## Public API (load-bearing signatures)

```go
type Manager struct{ /* unexported */ }

func New(opts ...Option) *Manager
func WithDefaults() Option    // App + File + Edit + View + Window menus
func WithAppMenu() Option
func WithFileMenu() Option
func WithEditMenu() Option
func WithViewMenu() Option
func WithWindowMenu() Option
func WithSettings() Option    // implies AppMenu (darwin) / EditMenu (else)
func WithEmitter(e *events.Emitter) Option

func (m *Manager) Apply(app *application.App) // sets app.Menu's application menu

const EventSettingsOpen = "settings:open"
```

## Invariants (do not break)

- **No build tag on `apply.go`.** It used to sit behind
  `//go:build (darwin || linux || windows) && wails` — undocumented, so
  `Apply` silently didn't exist unless a consumer knew the magic `wails`
  tag, and no CI run ever compiled, vetted, or ran this file. WP-06 removed
  the tag per AD-4: `shortcuts` is on the Wails-import allowlist, so it may
  import `wails/v3` plainly and unconditionally — no tag, ever. If you're
  tempted to reintroduce a build constraint here to "fix" a platform issue,
  that's very likely the wrong tool; the right tool is a `runtime.GOOS`
  branch inside `Apply` (see the existing darwin-vs-other logic).
- **This package is the ONE place other kit packages may import `wails/v3`
  without special justification** (AD-4 allowlist: `shortcuts`,
  `windowstate`, `permissions`, `kit/wailsbridge`). Removing the tag makes
  `wails/v3/pkg/application` — and, transitively on Linux, its cgo
  dependency on `gtk+-3.0`/`webkit2gtk-4.1`/`gdk-3.0` via pkg-config in
  `linux_cgo_gtk3.go` — part of the default build graph (`go build ./...`)
  for the first time in the repo's history. If CI ever fails with a
  pkg-config/cgo error tracing back to this package, that's the cause; the
  fix is installing the system packages in CI, not re-adding a build tag.
- **`Apply` always sets a menu**, even with zero options (`New().Apply(app)`
  sets an empty `application.NewMenu()`). Don't special-case "no options"
  into a no-op — a caller re-applying shortcuts after a settings change
  relies on `Apply` fully replacing the previous menu, not merging into it.
- **`WithSettings()` implies the platform-appropriate container menu** if
  not already enabled: `AppMenu` on darwin, `EditMenu` elsewhere. This is
  intentional convenience documented on `WithSettings`'s doc comment — the
  Settings shortcut needs *somewhere* to live.

## Dependencies & insulation

- `github.com/wailsapp/wails/v3/pkg/application` — see the Invariants entry
  above; this is the whole point of the package.
- `events` (kit package) — `Manager.emit` is a nil-safe no-op when no
  emitter is wired.
- No other kit-package dependencies.

## Extension points

- New menu roles: add a `With*Menu()` option following the existing
  boolean-flag-on-`Manager` pattern, then branch on it in `Apply`.
- New standard shortcuts beyond Settings (e.g. a future Preferences/About
  variant): follow the `WithSettings`/`EventSettingsOpen` shape — a bool
  flag, an `Add(...).SetAccelerator(...).OnClick(func(*application.Context)
  { m.emit(...) })` wiring, and a documented platform placement rule.
- Label localization (WP-31, not yet built): will change the literal
  `"Settings…"` / `"Settings"` / `"Edit"` strings in `apply.go` to
  `i18n.Text` and add a `WithLocalizer` option. Until then those strings
  are the source of truth for tests that match on label.

## Testing

- Doubles: `events.NewMemoryEmitter()` + `events.NewEmitter(...)` for
  option/emission tests (`shortcuts_test.go`).
- `apply_test.go` exercises `Apply` itself against a real
  `*application.App` from `application.New(...)`. **wails/v3's `*App` is a
  process-wide singleton** — `application.New` stores the first result in
  a package-level variable (`globalApplication`) and every later call
  returns that same instance regardless of the `Options` passed. All tests
  in `apply_test.go` therefore share one app via the `testApp(t)` helper
  (a `sync.Once`); do not call `application.New` directly in a new test —
  you'll get back whatever the first test created, silently.
- `go test -race ./shortcuts/` must cover per-role menu construction
  (`TestApply_FileMenu`, `TestApply_EditMenu`, etc., using
  `Menu.FindByRole`/`FindByLabel`), the darwin/non-darwin Settings
  placement split (`TestApply_SettingsPlacement`), and that repeated
  `Apply` calls replace rather than accumulate
  (`TestApply_MultipleCallsReplaceMenu`).
- **Not automatable in this suite**: `wails/v3`'s `MenuItem.handleClick` is
  unexported and normally invoked by the platform's native menu
  implementation, not by Go test code — there is no public API to
  synthesize a real click. `TestApply_SettingsItemWiring` verifies the item
  exists with the right label/accelerator instead; the emit-on-click
  behavior itself is covered by `TestEmitWithEmitter` in
  `shortcuts_test.go`, which calls `Manager.emit` directly. CI runs
  `ubuntu-latest` only (see `.github/workflows/ci.yml`), so the
  darwin-specific branches in `apply.go` (About/Services/Hide, the app-menu
  Settings placement, `Cmd+,`) are compiled and vetted by CI but not
  behaviorally exercised there — only the non-darwin assertions in
  `apply_test.go` run for real on CI. Verify darwin behavior locally on a
  Mac (as this file's author did) before trusting a change to that branch.

## File map

- `shortcuts.go` — `Manager`, all `Option` functions, `New`, `emit`.
- `apply.go` — `Apply` and the platform-specific menu-building helpers
  (`addDarwinAppMenuWithSettings`, `addEditMenuWithSettings`).
- `shortcuts_test.go` — option/constructor/emission tests.
- `apply_test.go` — `Apply` behavior tests against a real `*application.App`.
- `README.md` — quickstart, options table, platform behavior, events.

## Landmines

- `addDarwinAppMenuWithSettings` and the plain `menu.AddRole(application.AppMenu)`
  path are two different code paths that both produce "the app menu" —
  the former is used when `m.settings` is true, the latter only when
  `m.appMenu` is true and `m.settings` is false. `wails/v3`'s
  `application.NewAppMenu()` (the role path) reads
  `globalApplication.options.Name`, the process-wide singleton, not the
  `app` parameter `Apply` was called with — harmless in real usage (Wails
  only ever has one `*App` per process, so they're always the same object)
  but a landmine if you ever try to unit-test that path against a
  differently-named app than the process's first one.
- `MenuItem.handleClick` dispatches the registered callback via `go
  func(){...}()` (see wails/v3 `menuitem.go`) — click handling is
  inherently asynchronous relative to whatever triggered it. Not
  observable from this package's test suite (see Testing above), but worth
  knowing if you ever add synchronous-looking logic around a click
  callback.
