# runner

Package `runner` is a durable background job queue plus a worker that
ticks. It's Watermill-inspired — message/handler separation,
middleware-style retry — **not** Temporal-style durable execution.
Temporal's model is heavier than a desktop app needs; this package is
deliberately simpler and does not try to replace it.

## Why a package, not a goroutine and a `time.Ticker`

Every non-trivial desktop app eventually needs "run this later, reliably,
even across a restart" — an LLM call, a sync, a retry-worthy network
request. Hand-rolled versions of this consistently get three things wrong:

1. **Sleep/wake.** A laptop that suspends for 8 hours and wakes up must not
   fire 28,800 missed one-second ticks in a burst. `runner` coalesces a
   sleep/wake gap into a single due-scan, detected via wall-clock jump
   rather than any OS-specific sleep/wake hook (there is no portable one
   without extra dependencies).
2. **Crash mid-job.** A process that dies between "handler succeeded" and
   "the queue recorded that" must not lose the job, and must not corrupt
   the store. `runner` recovers a job left `running` by an unclean shutdown
   by redispatching it — see "At-least-once, not exactly-once" below.
3. **Unbounded shutdown.** Closing the app must not hang forever waiting on
   a stuck handler, and must not leak goroutines either.

## Profiles, not knobs

Pick a starting point, override the field or two you actually care about,
ignore the rest:

```go
p := runner.Durable()
p.MaxAttempts = 10 // still dead-lettered afterward — Durable's DeadLetter=true is untouched
```

| Profile | Persistence | MaxAttempts | DeadLetter | Suited to |
|---|---|---|---|---|
| `Ephemeral()` | in-memory | 3 | drop | UI-scoped retries, debounced work — fine to lose on exit |
| `Durable()` | a `Store` | unlimited | retain | Expensive, side-effecting work (LLM calls, syncs) — never silently lost |
| `BestEffort()` | a `Store` | 5 | drop | Worth surviving a restart, not worth retrying forever |

Every `Profile` field is exported and independently overridable — `Backoff`,
`ResumeOnWake`, `MaxQueueDepth`, `Retention` included.

## Quickstart

```go
import (
    "context"

    "github.com/jrschumacher/wails-kit/v2/events"
    "github.com/jrschumacher/wails-kit/v2/runner"
    "github.com/jrschumacher/wails-kit/v2/runner/flatfile"
)

store, err := flatfile.New(flatfile.WithPath("queue.jsonl"))
if err != nil {
    log.Fatal(err)
}

q, err := runner.New(runner.Durable(), runner.WithStore(store), runner.WithEmitter(emitter))
if err != nil {
    log.Fatal(err)
}

q.Handle("send_email", func(ctx context.Context, job runner.Job) error {
    var payload struct{ To string `json:"to"` }
    if err := json.Unmarshal(job.Payload, &payload); err != nil {
        return err // non-retryable in practice, but the queue doesn't know that — see below
    }
    return sendEmail(ctx, payload.To)
})

id, err := q.Enqueue(ctx, "send_email", map[string]string{"to": "user@example.com"})

go q.Start(ctx) // blocks until ctx is done; run it in its own goroutine
// ... later, on shutdown:
cancel()      // stop Start's tick loop
q.Close()     // wait for in-flight handlers, close the Store
```

## At-least-once, not exactly-once

Handlers can run more than once for the same logical job. This is a
deliberate trade: at-most-once silently loses work, and exactly-once is not
achievable without a much heavier system than a desktop app needs. The case
that causes a re-run: the process crashes after a handler returns `nil` but
before the queue finishes recording `done`. On the next `Start`, that job is
found still marked `running` and is redispatched.

If your handler's side effect can't tolerate running twice, use
`WithIdempotencyKey` **and** make the handler itself idempotent against that
key (e.g. an upsert, or a "have I already done this?" check against your own
side-effect's target). `Enqueue`'s own idempotency-key dedupe only prevents
a duplicate *enqueue* of the same logical job while it's pending or
running — it does not protect against the crash-recovery re-run described
above; that's the handler's job.

## Options

| Option | Applies to | Effect |
|---|---|---|
| `WithStore(s)` | `New` | Sets the persistence backend. Required when `Profile.Persistence == PersistStore`. |
| `WithEmitter(e)` | `New` | Sets the `*events.Emitter` for job/wake events. `nil` (default) drops events silently. |
| `WithWorkers(n)` | `New` | Max concurrent dispatches. Default 1 — this is desktop infrastructure, not a server pool. |
| `WithClock(f)` | `New` | Overrides "now". Tests only. |
| `WithIdempotencyKey(k)` | `Enqueue` | Dedupes against a pending/running job with the same key. |
| `WithDelay(d)` | `Enqueue` | Job becomes due `d` after enqueue instead of immediately. |

## Events

| Event | Fires when |
|---|---|
| `runner:job_done` | A job's handler returned `nil`. |
| `runner:job_failed` | A job exhausted `MaxAttempts` with `DeadLetter == false` (dropped). |
| `runner:job_dead` | A job exhausted `MaxAttempts` with `DeadLetter == true` (retained, queryable via `Dead()`). |
| `runner:wake` | `Start` detected a wall-clock gap > 2x its tick interval between ticks, with `ResumeOnWake == true`. |

Every job event carries `JobEventPayload{Job Job}`. All four fire only on
the actual transition — a mid-retry failure with attempts remaining doesn't
emit anything, and the due-scan runs every tick regardless of `ResumeOnWake`
(it only gates whether `runner:wake` is emitted).

## Error codes

| Code | Meaning |
|---|---|
| `runner_queue_full` | `Enqueue` beyond `Profile.MaxQueueDepth`. |
| `runner_no_handler` | A due job's `Type` has no registered `Handle`r — the job fails per the profile's retry/dead-letter rules. |
| `runner_store_required` | `New` with `Persistence == PersistStore` and no `WithStore`. |
| `runner_job_not_found` | `Requeue` on an ID that isn't currently dead-lettered. |
| `runner_queue_closed` | `Enqueue` or `Start` called after `Close`. |

## Dead-letter and requeue

```go
dead, err := q.Dead()          // every job currently JobStateDead
err = q.Requeue(dead[0].ID)    // back to pending, Attempts reset, picked up on the next due-scan
```

## Stores

- `runner/flatfile` — JSONL, human-readable, the kit default. Suited to
  Prune's use case of keeping the queue in git.
- `runner/sqlitestore` (WP-41) — for higher job volumes or when a JSONL
  file isn't the right fit.
- A minimal custom `Store` needs only `Append`/`Update`/`Due`/`Sweep`/
  `Close`. Also implement `Lister` (`List(state) ([]Job, error)`) to get
  full `MaxQueueDepth`/idempotency-dedupe/`Dead()` fidelity across process
  restarts — without it, that bookkeeping only reflects what happened since
  the current process started. Both built-in stores implement `Lister`.

## Not included, on purpose

No priority queues, no cron expressions, no distributed workers, no
web UI. This is single-process, desktop-scale infrastructure. See
`docs/v2-roadmap.md` WP-40 for the full design rationale.
