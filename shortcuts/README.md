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
| `WithUndo(stack)` | Puts the app's undo stack behind Edit > Undo and Redo (implies `WithEditMenu`); see [App undo and redo](#app-undo-and-redo) |

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
| `undo:changed` | The undo stack changed: a push, an undo or redo starting or ending, a clear, text focus changing | `UndoState` |
| `undo:text` | Edit > Undo or Redo while a text field has focus | `UndoTextPayload` |
| `undo:failed` | An undo or redo started from the menu failed | `UndoFailedPayload` |

## App undo and redo

[`frontend/ui/BEHAVIOUR.md`](../frontend/ui/BEHAVIOUR.md#undo-confirm-or-neither) is the
spec: Undo is never only in a toast. `UndoStack` is a stack for the session, behind
Edit > Undo and Redo, named for the action ("Undo Archive") and enabled to match.
While a text field has focus, the menu undoes typing first. A toast's Undo
(`wkUndo.action()` in `@wails-kit/ui`) uses the same stack.

```go
var app *application.App
emitter := events.NewEmitter(events.BackendFunc(func(name string, data any) {
    app.Event.Emit(name, data)
}))
undo := shortcuts.NewUndoStack(shortcuts.WithUndoEmitter(emitter))
app = application.New(application.Options{
    Name:     "MyApp",
    Services: []application.Service{application.NewService(undo.Bindings())},
})

mgr := shortcuts.New(shortcuts.WithDefaults(), shortcuts.WithUndo(undo), shortcuts.WithEmitter(emitter))
mgr.Apply(app)

// In the app: do the action, then push how to undo it.
func (a *App) Archive(id string) (string, error) {
    if err := a.store.Archive(id); err != nil {
        return "", err
    }
    return a.undo.Push(shortcuts.Action{
        Name: "Archive",
        Undo: func(ctx context.Context) error { return a.store.Unarchive(id) },
        Redo: func(ctx context.Context) error { return a.store.Archive(id) },
    })
}
```

```js
import { Events } from '@wailsio/runtime';
import * as Undo from './bindings/…/shortcuts/undobindings.js'; // the generated bindings for UndoBindings
import { wkUndo, wkToast } from '@wails-kit/ui';

wkUndo.connect({ on: (name, fn) => Events.On(name, e => fn(e.data)), bindings: Undo });

const id = await Archive(conversation.id);
wkToast({ words: 'Conversation archived', action: wkUndo.action(id) });
```

- **`Push(Action)`** records an action the app has just done and returns its ID; it
  clears the redo stack. `Name` and `Undo` are required (else `undo_invalid`). A nil
  `Redo` means it can't be redone: undoing it clears the redo stack, so nothing is
  redone out of order.
- **`Undo(ctx)`, `Redo(ctx)`** run the top action's function outside the lock, one at a
  time: `undo_busy` while one runs (both menu items are disabled then), `undo_empty`
  with nothing to undo, and `undo_failed`, wrapping the app's error, when its function
  fails. A failed action stays on its stack.
- **`UndoID(ctx, id)`** undoes the action only while it is the most recent, else
  `undo_stale`. A toast's Undo uses it, so it never undoes a later action.
- **`Clear()`** empties both stacks, for when what they act on has gone.
- **`SetTextFocusIn(window, bool)`** (and `SetTextFocus(bool)`, for window `""`) records
  whether a text field has focus, per window. The frontend (`wkUndo.connect`) reports it
  through `Bindings().SetTextFocus`, and the binding's context names the calling window
  once `Apply` has run. While the window in front has text focus, the items read plain
  "Undo" and "Redo", stay enabled, and send `undo:text` to that window only, instead of
  touching the stack. The window's field or editor then undoes its typing. A plain
  field with nothing to undo hands over to `Undo` (or `Redo`); the kit's editors never
  do.
- **How `undo:text` reaches one window:** with `Apply`, the stack runs a script in that
  window's page that dispatches the DOM event `wk-undo-text` (`DOMEventUndoText`),
  which `wkUndo` handles. Wails's own window events go to every window. Without
  `Apply` (an app's own menu, calling `MenuIn(ctx, window, op)`), it goes through
  `Emitter.EmitTo(window, …)`: register each window with the emitter under its window
  name. With no window name it is broadcast, and only the focused window acts.
- **A race:** focus can leave the field between the key press and the page handling
  `undo:text`. The page then finds no field with focus and undoes the app's action,
  which is what the menu would have done without text focus.
- **Pushing while an undo runs.** `Push` and `Clear` may be called while an undo or
  redo runs, including from inside it. An action pushed meanwhile stays on top; the one
  being undone is removed wherever it sits, and goes to the redo stack only if no push
  cleared that stack meanwhile. A redone action goes back below anything pushed while
  it ran. A `Clear` meanwhile wins: nothing comes back.
- **`MenuUndo(ctx)`, `MenuRedo(ctx)`** are what the menu items do: `Apply` wires them.
  A failure goes out as `undo:failed`, which the frontend says in words. An app with
  its own menu calls them from its items and keeps them in step with `OnMenu`.
- **`State()`**, **`Menu()`**, **`OnMenu(fn)`**: the state the frontend sees, the menu's
  labels and enabled state, and a callback for every change.
- **`Bindings()`** is what to register with Wails: `State`, `Undo`, `Redo`, `UndoID` and
  `SetTextFocus` only (`Push` takes functions).

Redo is on the platform's shortcut (`RedoAccelerator`): ⇧⌘Z on macOS, Ctrl+Y on Windows
and Ctrl+Shift+Z on Linux; Undo is ⌘Z / Ctrl+Z. With `WithUndo`, the Edit menu is built
by hand on every platform, laid out as Wails's own: Undo, Redo, Cut, Copy, Paste, then
on macOS Paste and Match Style, Delete, Select All and Speech, and elsewhere Delete
and Select All, plus Settings on Windows and Linux with `WithSettings`. Its Undo and
Redo are the app's items, not the native roles.

Every state goes out newest last: one goroutine delivers at a time, and delivers
again while newer changes came in, so the menu and `undo:changed` never end on an
older state. A change made from an `OnMenu` callback or an event handler is delivered
after the one that caused it.

**Limits in Wails v3.0.0-alpha.64:**

- **Linux:** menu accelerators aren't attached to the GTK menu items (`setAccelerator`
  is a stub), so Edit > Undo and Redo run only when clicked. ⌘Z / Ctrl+Z in a field is
  the web view's own typing undo.
- **Windows:** a menu item's native handle belongs to the window that built the menu
  last. With several windows, only that window's Edit menu shows the updated labels
  and enabled state; clicking the item works in each.
- **Windows and Linux, mouse:** clicking the menu bar may take focus from the web view
  first, so the page reports no text focus and Edit > Undo clicked with the mouse may
  undo the app's action instead of the typing. The shortcut isn't affected. This
  hasn't been checked on real hardware.

| Option | Description |
|---|---|
| `WithUndoEmitter(e)` | Emitter for `undo:changed`, `undo:text` and `undo:failed` |
| `WithUndoLimit(n)` | How many actions the stack keeps (default 100); the oldest go first |
| `WithUndoLabels(undo, redo, plainUndo, plainRedo)` | The menu's words: `"Undo %s"`, `"Redo %s"`, `"Undo"`, `"Redo"` |

### Error codes

| Code | Meaning |
|---|---|
| `undo_empty` | Nothing to undo or redo |
| `undo_busy` | An undo or redo is already running |
| `undo_stale` | `UndoID` named an action that isn't the most recent |
| `undo_invalid` | `Push` without a name or an Undo function |
| `undo_failed` | The action's Undo or Redo failed; it wraps the app's error |

The payloads are mirrored in `@wails-kit/types` (`UndoChangedPayload`,
`UndoTextPayload`, `UndoFailedPayload`).

## Pairing with settings

If the app uses `settings.Service`, listen for the event to open the settings UI:

```js
// Frontend
Events.On('settings:open', () => {
    settingsOpen = true
})
```
