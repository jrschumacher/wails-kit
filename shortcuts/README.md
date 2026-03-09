# shortcuts

Package `shortcuts` builds native application menus with standard keyboard shortcuts for Wails v3 apps. It handles platform differences automatically and integrates with the kit event system.

## Usage

```go
import (
    "github.com/jrschumacher/wails-kit/events"
    "github.com/jrschumacher/wails-kit/shortcuts"
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
)
mgr.Apply(app)
```

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

## Pairing with settings

If the app uses `settings.Service`, listen for the event to open the settings UI:

```js
// Frontend
Events.On('settings:open', () => {
    settingsOpen = true
})
```
