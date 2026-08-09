# windowstate

Package `windowstate` persists window position, size, and maximised state
across app restarts, and restores it safely — clamped to a currently
attached display so a window saved on an external monitor never comes back
off-screen once that monitor is unplugged. This is absent from Wails v3
itself; every app that wants it otherwise hand-rolls the same ~200 lines,
usually with the same bugs (see "Why not just save `Position`/`Size`
directly?" below).

## Usage

```go
import (
    "github.com/jrschumacher/wails-kit/v2/windowstate"
    "github.com/wailsapp/wails/v3/pkg/application"
)

app := application.New(application.Options{Name: "my-app"})
win := app.Window.NewWithOptions(application.WebviewWindowOptions{
    Name:   "main",
    Width:  1024,
    Height: 768,
})

// Manage starts tracking move/resize immediately (debounced).
mgr, err := windowstate.Manage(app, win, windowstate.WithName("main"))
if err != nil {
    log.Fatal(err)
}

// Restore before Show — applies any previously saved geometry, clamped to
// a currently attached display.
mgr.Restore()
win.Show()

// ... app.Run() ...

// Close flushes any pending debounced save. Call it during shutdown so the
// last drag/resize before quit isn't lost to the debounce window.
mgr.Close()
```

With no `WithStore` override, geometry is persisted via the kit's `state`
package at `{appdirs.Data()}/state/{name}.json` — `app.Config().Name`
supplies the app name, `WithName` (default `"main"`) supplies the file base
name. A multi-window app calls `Manage` once per window, each with its own
`WithName` key, so each window's geometry is independent.

## Options

| Option | Description |
|---|---|
| `WithName(name)` | Per-window state key; also the default store's file base name. Default `"main"`. |
| `WithStore(st)` | Overrides the default `state.Store[Geometry]` (e.g. to share one with other kit state, or to point at a temp dir in tests). |
| `WithDebounce(d)` | How long to wait after the last move/resize before writing to disk. Default `500ms`. |
| `WithEmitter(e)` | Optional `*events.Emitter`. Passed through to the underlying `state.Store` (so `state:saved`/`state:loaded` fire) and used directly for `windowstate:restored`/`windowstate:error`. |

## Events

| Event | Payload | Description |
|---|---|---|
| `state:saved` / `state:loaded` | `state.StateSavedPayload` / `state.StateLoadedPayload` | Fired by the underlying `state.Store` when `WithEmitter` is set — `Name` is the window's `WithName` key. |
| `windowstate:restored` | `RestoredPayload{Name, Geometry, Clamped}` | Fired after `Restore` applies geometry. `Clamped=true` means the saved geometry didn't fit any attached display and was re-centered on the primary instead. |
| `windowstate:error` | `ErrorPayload{Name, Op, Error}` | Fired when a save or load fails. `Op` is `"save"` or `"load"`. Debounced saves happen on a background timer with no other way to surface a failure — subscribe to this if you want to know. |

## Error codes

| Code | User message |
|---|---|
| `windowstate_config` | Window state is misconfigured. Please contact support. (`Manage`/`newManager` called with a nil app/window, or the default store couldn't be built) |
| `windowstate_save` | Failed to save window position. Please try again. |
| `windowstate_load` | Failed to restore window position. Please try again. |

## The bugs this package exists to fix

A window position/size persistence helper looks trivial and isn't, for four
reasons:

1. **Restoring onto a detached display.** If a window was last on an
   external monitor and that monitor is gone at next launch, naively
   restoring `(X, Y)` puts the window off-screen — invisible, and on some
   platforms unreachable without a keyboard shortcut to recenter it.
   `Restore` clamps: saved geometry is applied only if at least half its
   area overlaps some currently attached screen's work area (from
   `app.Screen.GetAll()`); otherwise the window is re-centered on the
   primary screen at its saved size (shrunk to fit if needed).
2. **Saving while minimised or maximised.** A minimised window's reported
   bounds are platform-dependent garbage — saving them corrupts the next
   restore. A maximised window's `Position()`/`Size()` report the maximised
   bounds, not the size you'd want back on unmaximise. `Manager` never
   saves while `IsMinimised()`, and separately tracks the last known
   *restored* (non-maximised, non-minimised) geometry plus a `Maximised`
   flag — so a maximised window restores at its pre-maximise size and then
   re-maximises, rather than being stuck at full-screen dimensions forever.
3. **Debounce.** A drag-resize fires a continuous stream of move/resize
   events. Writing to disk on every one is wasteful and, worse, means a mid
   -drag crash can save transient geometry. `Manager` coalesces bursts
   within `WithDebounce` (default 500ms) into a single write, and `Close`
   flushes any pending write synchronously so a quit right after a drag
   isn't lost.
4. **Per-window keys.** A multi-window app needs independent geometry per
   window. `WithName` scopes storage to one window; call `Manage` once per
   window with a distinct name.

## What automated tests cover, and what they can't

`go test -race ./windowstate/` covers all four bugs above against fakes: a
`Window` implementation that never touches a real display (`fakeWindow` in
`fake_test.go`) and fabricated `[]*application.Screen` slices (screens are
plain data — no display needed to construct one). See `clamp_test.go` for
the detached-display and partial-overlap clamp math, and `manager_test.go`
for debounce coalescing, minimised-skip, maximised save/restore, per-window
isolation, and `Close`'s synchronous flush.

**What this suite cannot cover:** real multi-monitor behavior — actually
plugging/unplugging a monitor, real OS-reported `Screen.WorkArea` values
(menu bars, taskbars, HiDPI scale factors), and the platform-specific
`WindowDidMove`/`WindowDidResize` event delivery quirks noted in Wails'
`webview_window_options.go` (e.g. Windows debounces those events itself
before `windowstate` ever sees them). Verify those by hand: build the
example in `examples/windowstate/`, run it on a laptop with an external
monitor attached, move the window onto the external display, quit,
disconnect the monitor, and relaunch — the window should come back on the
laptop screen, not off-screen.

## Why not just save `Position`/`Size` directly?

Because "just save Position/Size" is exactly the naive version that has all
four bugs above. This package is the ~200 lines that get it right once.
