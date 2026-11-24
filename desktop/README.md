# desktop

Shows the app's long work where a desktop puts it, outside the window: one system
notification when long work ends while the window isn't focused, progress on the
Windows taskbar button, a badge with the count of things that need the person, a
question before quitting while work runs, and each window's background colour before
the web view paints.

It implements the "Desktop" section of
[`frontend/ui/BEHAVIOUR.md`](../frontend/ui/BEHAVIOUR.md). It follows the
[`tasks`](../tasks/README.md) service's `tasks:changed` snapshots, so the app reports
its work once, to `tasks`, and this package draws it on the desktop.

The rules live in the core and are tested with fakes. The operating system is reached
through four small interfaces (`Notifier`, `Badge`, `Taskbar`, `Asker`); the Wails
adapters that implement them are behind the `wails` build tag, like the `shortcuts`
package's `Apply`.

## Usage

```go
import (
    "github.com/jrschumacher/wails-kit/desktop"
    "github.com/jrschumacher/wails-kit/events"
    "github.com/jrschumacher/wails-kit/tasks"
    "github.com/wailsapp/wails/v3/pkg/application"
    "github.com/wailsapp/wails/v3/pkg/services/dock"
    "github.com/wailsapp/wails/v3/pkg/services/notifications"
)

emitter := events.NewEmitter(backend, events.WithHistory(32))
work := tasks.NewService(tasks.WithEmitter(emitter))

badge := dock.New()
notes := notifications.New()
desk := desktop.NewService(
    desktop.WithEmitter(emitter), // the emitter tasks:changed goes out on
    desktop.WithTasks(work),
    desktop.WithBadge(badge),      // Wails's dock service is a desktop.Badge
)
desk.UseNotifications(notes, nil) // build tag "wails"

app := application.New(application.Options{
    Name:       "Example",
    ShouldQuit: desk.ShouldQuit, // the quit question
    Services: []application.Service{
        application.NewService(work.Bindings()),
        application.NewService(desk.Bindings()),
        application.NewService(badge),
        application.NewService(notes),
    },
})
app.Window.NewWithOptions(application.WebviewWindowOptions{
    BackgroundColour: desk.WindowBackground(app), // the theme's bg before the page paints
})
desk.Attach(app) // build tag "wails": focus, last window, background, taskbar, sleep and wake
app.Run()
```

Build the app with `-tags wails` (the tag the kit's Wails adapters sit behind).

### What Attach does

`Attach(app)` wires the service to a Wails app. Call it after `application.New` and
before `app.Run`. It:

- follows every window's focus, so notifications and the taskbar's Error state know
  whether the person is watching;
- asks the quit question when the last window closes, where that quits the app
  (`Windows.DisableQuitOnLastWindowClosed` and `Linux.DisableQuitOnLastWindowClosed`
  unset, or `Mac.ApplicationShouldTerminateAfterLastWindowClosed` set);
- paints every window's background with the theme's `bg` for the current appearance
  when it is created, and again on Wails's `ThemeChanged`;
- draws taskbar progress on Windows through `ITaskbarList3`;
- with `WithTasks`, calls the tasks service's `Sleep` and `Wake` from Windows's
  `APMSuspend` and `APMResumeAutomatic` (Wails reports sleep and wake only on
  Windows; elsewhere `tasks` detects wake from a jump in the clock).

It fills in what wasn't set with an option: the quit function (`app.Quit`), the window
to bring forward on a notification click, the notifications' title (the app's name),
the quit question (a Wails question dialog) and the taskbar.

## Notifications

Work over 10 s that ends (Done or Failed) while no window of the app is focused sends
one system notification. Stopped and cancelled work doesn't: the person ended it. Parts
aren't notified on their own; their task is.

- **A sentence, nothing private.** The default body is the task's own words as a
  sentence ("Report exported."), the part before a `" · "` hint ("Couldn't export the
  report · Retry" reads "Couldn't export the report."). It never uses the task's
  subject or raw error. The words leave the app for the system's notification centre,
  so the app's task words must hold no secrets or private content; `WithWords` sets
  the app's own.
- **Grouped.** Tasks that end within 1 s of each other share one notification: "3 tasks
  finished.", "2 tasks finished and 1 failed.".
- **Not while watched.** Work that ends while a window is focused isn't notified, and
  focusing a window drops a notification not yet sent.
- **Permission on first use.** The service asks for permission the first time a
  notification would help (`RequestAuthorization`), once per run, never at launch.
  Without it, the badge and the window's words carry the outcome; a later notification
  is sent if the person turns notifications on in the system's settings. The system's
  own settings (Do Not Disturb, Focus) apply as usual.
- **A click shows the result.** Clicking a notification brings a window forward
  (restores, shows and focuses it) and emits `desktop:show-tasks` with the task IDs, for
  the frontend to select their results. `UseNotifications` routes Wails's response
  callback; Wails keeps one, so pass the handler for the app's own notifications as its
  second argument.

## Taskbar (Windows)

The taskbar button shows the running work's combined progress:

- **Normal** with the summed `done` of `total` when every running or waiting top-level
  task has reported a total;
- **Indeterminate** when any of them hasn't;
- nothing when none runs. Queued, paused and stalled work isn't drawn: **Paused** (yellow)
  is never used, so paused work shows in the window's words;
- **Error** (red) only for a failure that needs the person (`WithNeedsPerson`) while no
  window is focused. The value goes to full first, since Error over Indeterminate
  draws a made-up percentage. It clears when a window is focused, or when nothing needs
  the person any more.

Wails doesn't expose `ITaskbarList3`'s progress: the adapter uses the object Wails's
`w32` package creates and calls `SetProgressState` and `SetProgressValue` through its
vtable, on the main thread, for every window. No cgo. macOS has no standard Dock
progress (it needs a custom Dock tile view, which Wails doesn't expose), and Linux has
no standard equivalent, so there is no taskbar there.

## Badge

The app icon's badge counts the top-level tasks that need the person: by default, a
task that is failed or stalled, or has a part that is. It drops as each is dealt with
(retried, dismissed) and is removed at 0. Wails's dock service draws it as a macOS
badge and a Windows overlay icon, and does nothing on Linux. Work that retries by
itself is neutral and doesn't count.

## Quitting while work runs

Pass `desk.ShouldQuit` as `application.Options.ShouldQuit`. With no unfinished
top-level task, quitting goes ahead. Otherwise the quit waits and the service asks:
"Quit while 2 tasks are running?", with **Keep Running** as the default and cancel
button, **Quit When Done** and **Quit**.

- **Quit** quits at once.
- **Quit When Done** emits `desktop:quit-pending` (`pending: true`) and quits as soon as
  every top-level task has finished. `CancelQuit` (also in `Bindings`) takes it back.
- **Keep Running** leaves the app as it was.

`ShouldQuit` is called on Wails's main thread, which the dialog needs, so it returns
false and asks on another goroutine; the answer calls the quit function again. A second
quit while the question is open doesn't ask twice. Without an `Asker`, quitting is never
held up.

Wails's question dialog on Windows is a MessageBox with Yes and No only, so there it
asks "Quit while 2 tasks are running?" with **Yes** (quit) and **No** (keep running, the
default), without Quit When Done. An app that wants all three choices on Windows sets
its own `Asker` (`WithAsker`), such as a sheet in the window. A Linux (GTK) dialog closed
with Esc runs no button, so it counts as Keep Running after a minute.

## Launch background

`desk.WindowBackground(app)` is the theme's `--wk-color-bg` for the current appearance,
for a new window's `BackgroundColour`, so a dark launch doesn't flash white. `Attach`
repaints every window when the system appearance changes. The defaults are the kit's
own (`#fcfcfb` and `#181a1f`, checked against `tokens.css` by a test); an app with its
own theme passes its values:

```go
desktop.WithBackground(desktop.MustParseColour("#fbfaf7"), desktop.MustParseColour("#1c1a17"))
```

When the app has its own light or dark setting (`<html data-theme>`), tell the service
with `SetAppearance(desktop.AppearanceLight | AppearanceDark | AppearanceSystem)`;
`Attach` repaints the windows.

Increased contrast is followed in the frontend: `@wails-kit/ui`'s tokens strengthen
lines, outlines and secondary text under `prefers-contrast: more`, live.

## Options

| Option | Default | Description |
|---|---|---|
| `WithEmitter(e)` | none | The emitter `tasks:changed` goes out on; the service's own events go there too. Without it the service sees no tasks |
| `WithTasks(t)` | none | The tasks service: its current list is read at once, and `Attach` routes sleep and wake to it |
| `WithNotifier(n)` | none | Where notifications go (`UseNotifications` sets Wails's) |
| `WithBadge(b)` | none | The badge, usually Wails's `dock.New()` |
| `WithTaskbar(t)` | Windows: `ITaskbarList3` (from `Attach`) | Taskbar progress |
| `WithAsker(a)` | a Wails question dialog (from `Attach`) | Asks the quit question; `AskerFunc` adapts a function |
| `WithQuit(f)` | `app.Quit` (from `Attach`) | Quits the app after the person chose to |
| `WithShow(f)` | restore, show and focus a window (from `Attach`) | Runs on a notification click |
| `WithAppName(s)` | the app's name (from `Attach`) | The notifications' title |
| `WithWords(w)` | English | Notification sentences, the quit question and its buttons; empty fields keep the defaults |
| `WithNeedsPerson(f)` | failed or stalled | Which tasks count on the badge and turn the taskbar red |
| `WithLongWork(d)` | 10 s | How long work must have run for its end to be notified |
| `WithGroupWindow(d)` | 1 s | How long to wait for other tasks to end with one, to share a notification |
| `WithBackground(light, dark)` | the kit's `bg` | The window background in each appearance |
| `WithClock(c)` | real clock | The tasks package's `Clock`; tests inject a fake |
| `WithErrorHandler(f)` | ignore | Errors from the notifier, badge and taskbar, which never stop the work |

## Methods

| Method | Description |
|---|---|
| `ShouldQuit() bool` | For `application.Options.ShouldQuit`; see [Quitting while work runs](#quitting-while-work-runs) |
| `QuitPending()`, `CancelQuit()` | Whether Quit When Done is pending; take it back |
| `Running() int` | Top-level tasks not finished |
| `SetFocused(bool)` | Whether any window is focused (`Attach` calls it) |
| `NotificationClicked(id) bool` | Handles a click on one of its notifications; false for one it didn't send |
| `SetNotifier(n)` | Sets the notifier after `NewService` |
| `Background(systemDark) Colour`, `SetAppearance(a)` | The window background for an appearance; the app's own light or dark setting |
| `Bindings() *Bindings` | The frontend's view: `QuitPending` and `CancelQuit`. Bind it, not the Service |
| `Close()` | Stops following tasks; drops notifications not yet sent |
| `Attach(app)`, `UseNotifications(ns, other)`, `WindowBackground(app)` | Wails adapters, build tag `wails` |

## Events

| Event | Payload | Description |
|---|---|---|
| `desktop:show-tasks` | `ShowTasksPayload{Tasks []string}` | A notification was clicked: select these tasks' results |
| `desktop:quit-pending` | `QuitPendingPayload{Pending bool, Running int}` | Quit When Done was chosen, taken back, or is quitting now |

TypeScript: `DesktopShowTasksPayload` and `DesktopQuitPendingPayload` in
`@wails-kit/types`.

## Error codes

| Code | User message |
|---|---|
| `desktop_invalid_colour` | The colour isn't a #rrggbb value |

## Testing

The core is tested with the real `tasks` service, a fake clock and fakes for the
notifier, badge, taskbar and asker. The Wails adapters compile only with `-tags wails`;
the Windows one is checked with `GOOS=windows go vet -tags wails ./desktop/` (the macOS
and Linux builds need cgo and the platform's SDK).
