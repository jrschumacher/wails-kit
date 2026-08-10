# shortcuts — agent notes

## Purpose

`shortcuts` builds native application menus with standard keyboard
shortcuts for Wails v3 apps (`Manager.Apply(app)`), handling macOS-vs-other
platform differences (About/Services/Hide/Quit placement, the Settings
shortcut's home in the app menu vs. the Edit menu). It owns menu
*construction*; it does not own menu item click handling beyond forwarding
one event (`EventSettingsOpen`) through the kit's `events.Emitter`. It owns
translating the one label it authors itself (Settings); every other label
comes from `application.Menu.AddRole` and is never touched by this
package's localizer (see Invariants).

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
func WithLocalizer(l *i18n.Localizer) Option // resolves the Settings label only

func (m *Manager) Apply(app *application.App) // sets app.Menu's application menu

const EventSettingsOpen = "settings:open"
```

## Invariants (do not break)

- **No build tag on `apply.go`, ever.** It used to sit behind an
  undocumented `//go:build (darwin || linux || windows) && wails`, so
  `Apply` silently didn't exist unless a consumer knew the magic tag, and
  no CI run ever compiled, vetted, or ran this file. WP-06 removed it per
  AD-4. A platform issue is a `runtime.GOOS` branch inside `Apply` (see the
  existing darwin-vs-other logic), never a build constraint.
- **This package may import `wails/v3` without special justification**
  (AD-4 allowlist: `shortcuts`, `windowstate`, `permissions`,
  `kit/wailsbridge`). That makes `wails/v3/pkg/application` — and,
  transitively on Linux, its cgo dependency on
  `gtk+-3.0`/`webkit2gtk-4.1`/`gdk-3.0` via pkg-config — part of the
  default build graph. A pkg-config/cgo CI failure tracing back here means
  installing the system packages, not re-adding a build tag.
- **`Apply` always sets a menu**, even with zero options (`New().Apply(app)`
  sets an empty `application.NewMenu()`). Don't special-case "no options"
  into a no-op — a caller re-applying shortcuts after a settings change
  relies on `Apply` fully replacing the previous menu, not merging into it.
- **`WithSettings()` implies the platform-appropriate container menu** if
  not already enabled: `AppMenu` on darwin, `EditMenu` elsewhere. This is
  intentional convenience documented on `WithSettings`'s doc comment — the
  Settings shortcut needs *somewhere* to live.
- **`WithLocalizer` only ever resolves the Settings label** (`labels.go`'s
  `labelSettings`/`labelSettingsDarwin`). Every role item
  (`menu.AddRole(...)`) renders whatever text the OS supplies for that
  role — macOS and the platform toolkit already localize About/Services/
  Hide/Quit/Edit correctly, and routing them through our catalog would
  replace a correct native label with a worse one. Before adding a label to
  `labels.go`, check whether it comes from `AddRole` — if so, it doesn't
  belong here.

## Dependencies & insulation

- `github.com/wailsapp/wails/v3/pkg/application` — see the Invariants entry
  above; this is the whole point of the package.
- `events` (kit package) — `Manager.emit` is a nil-safe no-op when no
  emitter is wired.
- `i18n` (kit package) — `Manager.localizer`/`resolveText` are nil-safe the
  same way: no `WithLocalizer` means every label resolves to its
  `i18n.Text.Other` literal.

## Extension points

- New menu roles: add a `With*Menu()` option following the existing
  boolean-flag-on-`Manager` pattern, then branch on it in `Apply`.
- New standard shortcuts beyond Settings: follow the
  `WithSettings`/`EventSettingsOpen` shape — a bool flag, an
  `Add(...).SetAccelerator(...).OnClick(func(*application.Context)
  { m.emit(...) })` wiring, and a documented platform placement rule. If
  app-authored text (not a role), add its `i18n.Text` to `labels.go` and
  its catalog entry to `locales/en.json`.

## Testing

- Doubles: `events.NewMemoryEmitter()` + `events.NewEmitter(...)` for
  option/emission tests (`shortcuts_test.go`).
- `apply_test.go` exercises `Apply` against a real `*application.App`.
  **wails/v3's `*App` is a process-wide singleton** — `application.New`
  stores the first result in a package-level variable and every later call
  returns that same instance regardless of the `Options` passed. All tests
  share one app via the `testApp(t)` helper (`sync.Once`); do not call
  `application.New` directly in a new test.
- `go test -race ./shortcuts/` must cover per-role menu construction
  (`TestApply_FileMenu`, `TestApply_EditMenu`, etc., using
  `Menu.FindByRole`/`FindByLabel`), the darwin/non-darwin Settings
  placement split (`TestApply_SettingsPlacement`), that repeated
  `Apply` calls replace rather than accumulate
  (`TestApply_MultipleCallsReplaceMenu`), and that `WithLocalizer` actually
  changes the resolved Settings label (`TestApply_SettingsItemWiring_Localized`,
  which builds a real `*i18n.Localizer` over a hand-built French
  `fstest.MapFS` catalog via the `frenchCatalog` helper).
- **Not automatable in this suite**: `wails/v3`'s `MenuItem.handleClick` is
  unexported and normally invoked by the platform's native menu
  implementation — no public API to synthesize a real click.
  `TestApply_SettingsItemWiring` verifies the item exists with the right
  label/accelerator instead; the emit-on-click behavior is covered by
  `TestEmitWithEmitter` in `shortcuts_test.go`, which calls `Manager.emit`
  directly. CI runs `ubuntu-latest` only, so darwin-specific branches
  (About/Services/Hide, the app-menu Settings placement, `Cmd+,`) are
  compiled and vetted but not behaviorally exercised there. Verify darwin
  behavior locally on a Mac before trusting a change to that branch.

## File map

- `shortcuts.go` — `Manager`, all `Option` functions, `New`, `emit`.
- `apply.go` — `Apply` and the platform-specific menu-building helpers
  (`addDarwinAppMenuWithSettings`, `addEditMenuWithSettings`).
- `labels.go` — `labelSettings`/`labelSettingsDarwin` and `resolveText`,
  the nil-safe localizer helper `apply.go` calls.
- `locales/en.json` — catalog entries for `labels.go`'s keys (extraction
  lint fodder — see `i18n/AGENTS.md` `TestCatalogKeysExist`).
- `shortcuts_test.go` — option/constructor/emission/localizer-setter tests.
- `apply_test.go` — `Apply` behavior tests against a real `*application.App`,
  including the localized-label test and its `frenchCatalog` helper.
- `README.md` — quickstart, options table, platform behavior, events,
  localization.

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
  func(){...}()` (wails/v3 `menuitem.go`) — click handling is asynchronous
  relative to whatever triggered it. Not observable from this suite (see
  Testing), but worth knowing before adding synchronous-looking logic
  around a click callback.
