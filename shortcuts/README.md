# shortcuts

Package `shortcuts` builds native application menus with standard keyboard shortcuts for Wails v3 apps. It handles platform differences automatically and integrates with the kit event system.

## Usage

```go
import (
    "github.com/jrschumacher/wails-kit/v2/events"
    "github.com/jrschumacher/wails-kit/v2/shortcuts"
    "github.com/wailsapp/wails/v3/pkg/application"
)

app := application.New(application.Options{Name: "MyApp"})

emitter := events.NewEmitter(events.BackendFunc(func(name string, data any) {
    app.Event.Emit(name, data)
}))

mgr := shortcuts.New(
    shortcuts.WithDefaults(),   // App, File, Edit, View, Window menus
    shortcuts.WithSettings(),   // ⌘, / Ctrl+, → emits "settings:open"
    shortcuts.WithEmitter(emitter),
    shortcuts.WithLocalizer(localizer), // optional — see Localization below
)
mgr.Apply(app)
```

A `kit/wailsbridge.Attach` call builds and applies a `Manager` for you
(wired to the same `*kit.Kit`'s emitter and localizer) unless you pass
`wailsbridge.WithShortcuts` with your own pre-built `Manager` or
`wailsbridge.WithoutMenu()` to skip menu wiring entirely — see that
package's README.

## Options

| Option | Description |
|---|---|
| `WithDefaults()` | Enables App, File, Edit, View, and Window menus |
| `WithAppMenu()` | Application menu (macOS: About, Hide, Quit) |
| `WithFileMenu()` | File menu |
| `WithEditMenu()` | Edit menu (Undo, Redo, Cut, Copy, Paste, Delete, Select All) |
| `WithViewMenu()` | View menu (Reload, Zoom, Fullscreen) |
| `WithWindowMenu()` | Window menu (Minimize, Zoom) |
| `WithSettings()` | Settings shortcut (⌘, / Ctrl+,) |
| `WithEmitter(e)` | Event emitter for shortcut events |
| `WithLocalizer(l)` | Localizer for app-specific item labels (Settings only — see Localization) |

## Platform behavior

### macOS

- App menu includes About, Services, Hide/Show, and Quit
- Settings item appears in the app menu (standard macOS placement)
- Accelerator: `⌘,`

### Windows / Linux

- Quit goes in the File menu
- Settings item appears in the Edit menu
- Accelerator: `Ctrl+,`

## Events

| Event | Trigger | Payload |
|---|---|---|
| `settings:open` | Settings shortcut activated | `nil` |

## Localization

Only the Settings item's label is ours to translate. Every other label in
the menus this package builds comes from `application.Menu.AddRole`, which
renders whatever localized text the OS itself supplies for that role
(About, Services, Hide, Quit, Edit's individual Undo/Redo/Cut/Copy/Paste/
Delete/Select All entries, ...) — passing those through our own catalog
would replace a correctly-localized native label with a worse one, so
`WithLocalizer` deliberately never touches them.

Wire a localizer to translate "Settings…" / "Settings":

```go
mgr := shortcuts.New(
    shortcuts.WithSettings(),
    shortcuts.WithLocalizer(localizer), // *i18n.Localizer
)
```

Without `WithLocalizer`, labels resolve to their literal English fallback —
identical to pre-WP-31 behavior, so existing `FindByLabel("Settings…")`-
style lookups in your own tests keep working unless you wire a localizer.
The catalog keys are `wailskit.shortcuts.settings.label` (non-macOS) and
`wailskit.shortcuts.settings.label_darwin` (macOS, with the HIG-standard
trailing ellipsis) — see `locales/en.json`.

## Pairing with settings

If the app uses `settings.Service`, listen for the event to open the settings UI:

```js
// Frontend
Events.On('settings:open', () => {
    settingsOpen = true
})
```
