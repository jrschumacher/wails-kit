# appearance

Package `appearance` is one resolved source of truth for light/dark theme:
an OS signal, an optional persisted user override, and the merge of the two
into a single `Theme` that both the Go side (e.g. the Wails window's
background colour at startup) and the webview can read.

## The problem this solves

Prune's `CLAUDE.md` documents a three-layer background-colour consistency
requirement: the Go window frame (`application.NewRGB(...)`), base CSS, and
component CSS must all agree, or you get a visible seam between the window
chrome and the page content the moment they don't. Today that's maintained
by hand in three places, and it's exactly the kind of thing that silently
drifts. With one resolved `Theme` that every layer reads from, the seam
becomes structurally impossible instead of a documented gotcha.

## Three states, not two

`Mode` is the user's *preference*: `"system"` (default), `"light"`, or
`"dark"`. `Theme` is the *resolved truth*: always `"light"` or `"dark"`,
never `"system"` — that ambiguity is `Mode`'s job, not `Theme`'s. Everything
downstream (window background colour, CSS class) consumes `Theme`.

```
Mode = "light" | "dark"  -> Theme = that value, unconditionally
Mode = "system"          -> Theme = Source.IsDark() ? "dark" : "light"
```

## Usage

```go
import "github.com/jrschumacher/wails-kit/v2/appearance"

svc := appearance.NewService(
    appearance.WithSettings(settingsSvc), // persist the user's override
    appearance.WithEmitter(emitter),      // fire appearance:changed
)
defer svc.Close()

svc.Mode()     // "system" (default) | "light" | "dark"
svc.Resolved() // "light" | "dark" — what to actually apply

svc.SetMode(appearance.ModeDark) // pins dark; persists via settings if wired
```

Register `appearance.SettingsGroup()` with your `settings.Service` (via
`settings.WithGroup`) to expose the theme picker in a generically-rendered
settings form, and pass the *same* `*settings.Service` to
`appearance.WithSettings` so this package reads/writes the same value the
form does.

### Recommended: set the Wails window background at startup

This is what actually kills the three-layer seam — read `Resolved()` once
before creating the window, and use it for all three layers from one value:

```go
theme := svc.Resolved()

bg := application.NewRGB(255, 255, 255) // light
if theme == appearance.ThemeDark {
    bg = application.NewRGB(23, 23, 23) // dark — match your dark CSS background
}

win := app.Window.NewWithOptions(application.WebviewWindowOptions{
    BackgroundColour: bg,
    // ...
})
```

The frontend applies `Resolved()` (and `appearance:changed` events
thereafter) however it likes — a `data-theme` attribute, a CSS class, CSS
custom properties. This package only decides *what* the theme is, never how
it's rendered.

## The `Source` interface — why appearance is Wails-free

Per `docs/v2-roadmap.md` AD-4, `appearance` is **not** on the Wails-import
allowlist (`shortcuts`, `windowstate`, `permissions`, `kit/wailsbridge` are).
That's deliberate: appearance must work in a CLI or TUI process with no GUI
at all — Prune's Cobra CLI and Bubbletea TUI need a real resolved theme too,
not just the GUI.

So the OS theme signal is a narrow interface:

```go
type Source interface {
    IsDark() bool
    Subscribe(fn func(dark bool)) (cancel func())
}
```

Without `WithSource`, `NewService` uses the platform default:

- **macOS**: a real, no-Wails detector — `defaults read -g AppleInterfaceStyle`.
  It resolves once, at construction, and caches the result: without a Wails
  event loop there's no live signal to poll, so this default source does
  **not** detect OS flips while the process keeps running. `Subscribe` is
  inert (registers nothing, never calls back) rather than pretending to
  offer live updates it can't deliver.
- **Everywhere else**: `nil`, which resolves to light. Documented, not a bug.

A live-updating `Source` — real OS flip detection while the app runs, on
every platform — is `kit/wailsbridge`'s job (a later work package, not built
here): it wraps Wails' `ThemeChanged` event and `Env.IsDarkMode()`, and
plugs in via `appearance.WithSource(wailsbridge.ThemeSource(app))` with zero
changes needed in this package.

## Options

| Option | Description |
|---|---|
| `WithSource(s)` | The OS theme signal. Omit to use the platform default (see above). Pass `WithSource(nil)` to explicitly opt out of the platform default. |
| `WithSettings(svc)` | Persists `SetMode`'s value and makes `Mode()`/`Resolved()` read it back live (settings values are read at call time, not cached, per every other kit package's settings integration — see `docs/settings-integration.md`). |
| `WithEmitter(e)` | Optional `*events.Emitter`. Nil (the default) makes emission a no-op. |

## Events

| Event | Payload | Fires when |
|---|---|---|
| `appearance:changed` | `ChangedPayload{Mode, Resolved}` | The *resolved* Theme changes — an explicit `SetMode` call, an OS flip while `Mode() == "system"`, or `Refresh()`. Never fires just because `Mode` changed without `Resolved` changing (e.g. `"system"` -> `"dark"` while the OS was already dark). |

## Reacting to settings-UI changes

A settings form rendered generically from `SettingsGroup()` writes through
`settings.Service.SetValues` (or `settings.Binding.SetValues` from the
frontend) directly — not through `appearance.Service.SetMode`. `Mode()` and
`Resolved()` pick that up immediately because they read settings live, but
**`appearance:changed` does not fire on its own** for a write that never
went through this package. `settings.Service` has no way to notify a
dependent service after construction other than the `WithOnChange` callback
registered at `settings.NewService` time, so close the loop with `Refresh`:

```go
var appearanceSvc *appearance.Service // assigned below; captured by the closure

settingsSvc := settings.NewService(
    settings.WithGroup(appearance.SettingsGroup()),
    settings.WithOnChange(func(map[string]any) {
        if appearanceSvc != nil {
            appearanceSvc.Refresh()
        }
    }),
)

appearanceSvc = appearance.NewService(
    appearance.WithSettings(settingsSvc),
    appearance.WithEmitter(emitter),
)
```

`Refresh()` re-resolves the current (live-read) mode and emits
`appearance:changed` only if the resolved theme actually differs from the
last known value — same no-spurious-events guarantee as everything else in
this package. See `examples/appearance/main.go` for this wired end-to-end.

## Error codes

| Code | User message |
|---|---|
| `appearance_invalid_mode` | Invalid appearance setting. Please choose System, Light, or Dark. (`SetMode` called with anything other than `"system"`/`"light"`/`"dark"`) |
| `appearance_persist` | Failed to save your appearance preference. Please try again. (the wired `settings.Service.SetValues` failed or rejected the value) |

## i18n

`SettingsGroup()`'s `Group.Label`/`Field.Label`/`Field.Description`/
`SelectOption.Label` are `i18n.Text` (per AD-5 and `settings`' WP-12
schema). This package builds them; resolving `i18n.Text` to a plain string
on the wire is `settings.Service.GetSchema`'s job (via whichever
`*i18n.Localizer` you pass to `settings.WithLocalizer`) — `appearance`
itself does no resolution. Catalog keys live in `locales/en.json`,
namespaced `wailskit.appearance.*`.

## What automated tests cover, and what they can't

`go test -race ./appearance/` covers: all three `Mode` states resolving
correctly against every `Source` state (including nil); override precedence
over the OS signal in both directions; an OS flip while on follow-system
producing exactly one `appearance:changed` event; no event when the
resolved theme doesn't change (same mode reapplied, or a mode change that
resolves to the theme the OS already had, or an OS flip absorbed by an
active override); settings persistence and live read-back, including from a
second `Service` sharing the same `settings.Service` (the realistic
"app restart" shape) and from a write that bypasses `SetMode` entirely; the
`Refresh()` wiring; `Close`'s unsubscribe; concurrent access under `-race`;
and, on darwin, the substituted `readAppleInterfaceStyle` func var parsing
Dark/Light/error output without ever shelling out.

**Not automatable here**: a real OS theme change on a live session. The
darwin default `Source`'s `IsDark()` is verified against substituted
command output, and its `Subscribe` is verified to be inert as documented —
but nothing in this suite flips an actual macOS System Settings toggle
while the process is running, because doing so from a headless test
wouldn't prove anything a real desktop session doesn't already need for
`kit/wailsbridge`'s live Source anyway. Verify that by hand once
`kit/wailsbridge`'s `ThemeChanged`-backed `Source` exists (WP-31): run a
Wails app wired to it, flip the OS theme while it's running, and confirm
the window and webview both update from a single `appearance:changed`
event.
