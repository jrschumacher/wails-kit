# tasks

The app's list of tasks, kept on the Go side so it survives a reloaded web view and
closed windows. The service emits one event, `tasks:changed`, with a snapshot of the
whole list: running tasks, and finished ones not yet dismissed.

It implements "Go task events" and the action lifecycle in
[`frontend/ui/BEHAVIOUR.md`](../frontend/ui/BEHAVIOUR.md): its states, the timeout on
silence, retries with backoff, and sleep and wake. The kit's frontend follows it with
`wkTask.follow` and the task list. The app does the work and writes the words; the
service runs the clocks.

## Usage

```go
import (
    "github.com/jrschumacher/wails-kit/events"
    "github.com/jrschumacher/wails-kit/tasks"
)

emitter := events.NewEmitter(backend, events.WithHistory(32))
svc := tasks.NewService(tasks.WithEmitter(emitter))
defer svc.Close()

task, err := svc.Add(tasks.Spec{
    Kind:    "export",
    Subject: "report.pdf",
    Words:   "Exporting",
    Stop:    tasks.StopStop, // no OnStop: Stop cancels the task's context
})

go func() {
    ctx := task.Context() // ends on Stop, a timeout, or Dismiss
    for i, f := range files {
        if ctx.Err() != nil {
            task.Stopped(fmt.Sprintf("%d of %d files copied", i, len(files)))
            return
        }
        if err := copyFile(ctx, f); err != nil {
            task.Fail(err, "Couldn't export the report")
            return
        }
        task.Progress(tasks.Progress{Done: int64(i + 1), Total: int64(len(files)), Unit: "files"})
    }
    task.Done("Report exported")
}()
```

Bind the service (or the methods you need) for the frontend: `Tasks()` returns the
current list, and `Stop`, `Pause`, `Resume`, `Retry` and `Dismiss` take a task's ID.

### A reloaded window

With `events.WithHistory`, call `emitter.ReplayAll(windowID)` when a window loads: it
sends the latest `tasks:changed`, so the window draws the tasks as they are. A window
can also call `Tasks()`. One event name keeps this right: the history's ring buffer
always holds the latest snapshot.

### Work in parts

```go
review, _ := svc.Add(tasks.Spec{Kind: "review", Words: "Running reviewers"})
for _, r := range reviewers {
    part, _ := review.AddPart(tasks.Spec{Subject: r.Name})
    go run(part)
}
```

A part's snapshot names its `parent`. Each part shows its own state; the app reports
the whole task Done only once every part is, and `Dismiss` on the parent removes its
parts.

## The snapshot

`tasks:changed` sends `ChangedPayload{Tasks []Snapshot}`: every task, in the order
they were added. In TypeScript it is `TasksChangedPayload` and `TaskSnapshot` from
`@wails-kit/types`. The shape is a contract shared with the frontend: fields are
only ever added, never renamed or removed.

```json
{
  "tasks": [{
    "id": "task-1", "parent": "", "subject": "report.pdf", "kind": "export",
    "state": "running",
    "done": 12, "total": 40, "unit": "files", "step": 2, "steps": 3, "stage": "Uploading",
    "words": "Exporting", "stop": "stop",
    "error": null, "retry": "manual", "nextTry": null,
    "startedAt": 1795442400000, "updatedAt": 1795442412000
  }]
}
```

| Field | Meaning |
|---|---|
| `id` | The task's ID: `Spec.ID`, or generated (`task-1`, `task-2`, …) |
| `parent` | The task this is a part of; `""` for a top-level task |
| `subject`, `kind` | App-defined: what it acts on, and what sort of work |
| `state` | `queued`, `running`, `waiting`, `paused`, `stalled`, `done`, `failed`, `cancelled` or `stopped` |
| `done`, `total`, `unit` | Progress in units; `total` is 0 until the work reports one |
| `step`, `steps`, `stage` | Progress in stages; `step` is 1-based, 0 for work with no stages |
| `words` | The app's words for the current state |
| `stop` | How it can be stopped: `cancel`, `stop`, `stop-asks`, `pause`, `unsafe`, or `false` |
| `error` | `{ code, userMsg, message }` while failed, or while waiting for an automatic retry; else `null` |
| `retry` | `auto`, `manual` or `unsafe` |
| `nextTry` | When the next automatic retry runs; else `null` |
| `startedAt` | When it first ran; `null` while it has never run (queued) |
| `updatedAt` | The last change of any kind |

Times are Unix milliseconds. `error.userMsg` comes from `errors.RegisterMessages`
(its code from `errors.GetCode`); `error.message` is the raw error, for an ErrorBox.

## States

| State | Reported by | Timeout on silence |
|---|---|---|
| `queued` | `Spec.Queued`, `task.Queued(words)` | off |
| `running` | `Add`, `task.Running(words)`, a retry | on |
| `waiting` | `task.Waiting(words)`; or the service, between automatic retries | on, unless the task turns it off; off between retries |
| `paused` | `task.Paused(words)` | off |
| `stalled` | `task.Stalled(words)` | off |
| `done` | `task.Done(words)` | — |
| `failed` | `task.Fail(err, words)`, or a timeout | — |
| `cancelled` | `task.Cancelled()`, or `Stop` while it waits to retry | — |
| `stopped` | `task.Stopped(words)` | — |

Empty words keep the current ones. `task.Progress` moves a `waiting` task back to
`running`. `task.SetWords` changes the words without changing the state.

Reports after a task has finished, been dismissed or the service has closed are
ignored, so work that carries on after a timeout or a Stop can't bring the task back.
Reports to a task waiting for an automatic retry are ignored too: only the retry,
`Retry` or `Stop` move it on.

## Coalescing

A change of state goes out at once. Progress and words alone go out at most once per
progress interval (four snapshots a second), and the last one always goes out: a
held-back snapshot is folded into the next change of state, so a final Done or Failed
is never dropped. Don't use `events.WithThrottle` on `tasks:changed`; it keeps the
first event in each window and drops the rest.

A `tasks:changed` handler may call back into the service, even to report: one
goroutine emits at a time, and a change made during an emit goes out after it.

## Timeout on silence

Every running task fails after 2 minutes without a report ("Timed out after 2 min",
code `timeout`), and its context is cancelled. Progress, words and changes of state
all restart the clock. `Spec.Timeout` sets a task's own; `tasks.NoTimeout` turns it
off, and `task.SetTimeout` changes it later, for a task that waits on something it has
named (a job queued on GitHub). Queued, paused and stalled tasks have no timeout.

## Retries

| `Spec.Retry` | On `Fail` |
|---|---|
| `manual` (default) | Failed, until the person retries (`Retry`) or dismisses it |
| `auto` | Waiting, with `error` and `nextTry` set; after the backoff the service runs `OnRetry`. Neutral: it may still finish by itself |
| `unsafe` | Failed; never retried by itself. The app checks, then offers Retry ("Couldn't confirm the comment was posted") |

The backoff is 2, 4, 8 and 16 s, then every 30 s, each ±20%. `Spec.MaxAttempts` caps
an automatic task's attempts (0: no cap). `Retry(id)` runs a failed task, or a waiting
one at once ("Retry now"); if that attempt fails too, the backoff carries on from where
it was. A task without `OnRetry` can't be retried, and an `auto` one fails instead.

A retry starts a clean attempt: Running, a new `Context()`, no error or next try, and
the words back to `Spec.Words`. Then `OnRetry` runs the work again.

## Sleep and wake

Time asleep doesn't count. On wake, every running task's timeout starts again from
zero, and a task waiting for an automatic retry drops its backoff and tries at once.

Wails reports sleep and wake only on Windows: call `svc.Sleep()` and `svc.Wake()` from
its power events there. Elsewhere the service detects wake from a jump in the wall
clock: while any task is active, it checks every 5 s, and treats a gap of more than
5 s + 15 s as a wake. Turn detection off with `WithWakeDetection(0, 0)` if the app
calls `Sleep` and `Wake` itself everywhere.

## Options

| Option | Default | Description |
|---|---|---|
| `WithEmitter(e)` | none | Where `tasks:changed` goes. Without it the list is kept but nothing is sent |
| `WithClock(c)` | real clock | Time source (`Now`, `AfterFunc`); tests inject a fake |
| `WithProgressInterval(d)` | 250 ms | Shortest gap between snapshots for progress alone |
| `WithTimeout(d)` | 2 min | Default timeout on silence; `NoTimeout` turns it off |
| `WithBackoff(waits, jitter)` | 2, 4, 8, 16, 30 s; 0.2 | Waits before automatic retries (the last repeats) and their spread |
| `WithRandom(f)` | `math/rand/v2` | Source of jitter, in [0, 1) |
| `WithWakeDetection(interval, threshold)` | 5 s, 15 s | Wake detection from a clock jump; `(0, 0)` turns it off |

## Spec

| Field | Description |
|---|---|
| `ID` | The task's ID; empty generates one |
| `Parent` | The task this is a part of (or use `task.AddPart`) |
| `Subject`, `Kind`, `Words` | Sent in the snapshot; a retry starts again from `Words` |
| `Stop` | `StopNone` (default), `StopCancel`, `StopStop`, `StopStopAsks`, `StopPause`, `StopUnsafe` |
| `Retry`, `MaxAttempts` | Retry policy; see [Retries](#retries) |
| `Timeout` | Timeout on silence: 0 for the service's, `NoTimeout` for none |
| `Queued` | Start Queued instead of Running |
| `OnStop` | Runs on `Stop(id)`; the app reports Cancelled or Stopped. Without it, `Stop` cancels the task's context |
| `OnPause`, `OnResume` | Run on `Pause(id)` and `Resume(id)`; the app reports Paused or Running |
| `OnRetry` | Runs the work again, on `Retry(id)` or an automatic retry |

## Service methods

| Method | Description |
|---|---|
| `Add(spec) (*Task, error)` | Adds a task and sends a snapshot |
| `Tasks() []Snapshot` | The current list |
| `Get(id) (Snapshot, bool)` | One task |
| `Stop(id) error` | Asks to stop; a task waiting to retry is Cancelled at once |
| `Pause(id)`, `Resume(id) error` | Ask to pause or resume, through `OnPause` and `OnResume` |
| `Retry(id) error` | Runs a failed task again, or a waiting one now |
| `Dismiss(id) error` | Removes a finished task and its parts |
| `DismissFinished() int` | Removes every finished task whose parts have finished |
| `Sleep()`, `Wake()` | Stop every clock; start them again from zero |
| `Close()` | Stops the timers, cancels every task's context, sends any held-back snapshot |

## Events

| Event | Payload | Description |
|---|---|---|
| `tasks:changed` | `ChangedPayload{Tasks}` | The whole list, after any change |

## Error codes

| Code | User message |
|---|---|
| `tasks_not_found` | That task is no longer in the list |
| `tasks_duplicate_id` | A task with this ID is already in the list |
| `tasks_invalid_spec` | The task isn't set up correctly |
| `tasks_invalid_state` | The task can't do that in its current state |
| `tasks_unsupported` | This task can't do that |

A timeout fails the task with the `errors` package's `timeout` code.
