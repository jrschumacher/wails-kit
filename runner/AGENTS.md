# runner — agent notes

## Purpose

`runner` is a durable background job queue plus a worker that ticks —
Watermill-inspired (message/handler separation, middleware-style retry),
**not** Temporal-style durable execution. It owns job lifecycle
(pending/running/done/failed/dead), retry-with-backoff, dead-lettering,
backpressure, idempotency-key dedupe, and sleep/wake-aware ticking. It does
not own persistence mechanics (that's a `Store` implementation — see
`runner/flatfile`) or how a handler does its work.

## Public API (load-bearing signatures)

```go
type Profile struct {
    Persistence Persistence; MaxAttempts int; Backoff func(attempt int) time.Duration
    DeadLetter bool; ResumeOnWake bool; MaxQueueDepth int; Retention time.Duration
}
func Ephemeral() Profile; func Durable() Profile; func BestEffort() Profile

type JobState string // pending | running | done | failed | dead
type Job struct { ID, Type string; Payload json.RawMessage; State JobState
    Attempts int; IdempotencyKey string; EnqueuedAt, NextRunAt time.Time; LastError string }
type Handler func(ctx context.Context, job Job) error

type Store interface {
    Append(Job) error; Update(Job) error
    Due(now time.Time, limit int) ([]Job, error) // pending-due, PLUS any running job (crash recovery)
    Sweep(retention time.Duration, now time.Time) error; Close() error
}
type Lister interface { List(state JobState) ([]Job, error) } // optional; enables full Dead()/depth fidelity
type Deleter interface { Delete(id string) error } // optional; enables Discard actually purging a Store record

func New(p Profile, opts ...Option) (*Queue, error)
func (q *Queue) Handle(jobType string, h Handler)
func (q *Queue) Enqueue(ctx, jobType string, payload any, opts ...EnqueueOption) (id string, err error)
func (q *Queue) Start(ctx context.Context) error // blocks until ctx.Done() and all handlers returned
func (q *Queue) Close() error
func (q *Queue) Dead() ([]Job, error); func (q *Queue) Requeue(id string) error
func (q *Queue) Discard(id string) error // permanently removes a dead job; needs a Deleter Store or returns ErrDiscardUnsupported
```

## Invariants (do not break)

- **Never emit while holding `q.mu`.** Every call site releases the lock
  (or never took it) before `q.emit` — four kit packages have shipped or
  narrowly avoided this exact deadlock.
- **`Job.NextRunAt` is repurposed as the terminal-transition timestamp**
  once a job reaches done/failed/dead — the authoritative `Job` shape (see
  `docs/v2-roadmap.md` WP-40) has no separate `CompletedAt`, and `Sweep`
  needs *some* timestamp to compare against `Retention`. Don't "fix" this
  by reading `NextRunAt` as a future due-time on a terminal job.
- **`Store.Due` must return running jobs too, not just pending-due ones.**
  This is the entire crash-recovery mechanism: a job a `Store` reports as
  `running` is either tracked in `q.inFlight` (this process, skip it) or
  wasn't (a previous process crashed mid-job; redispatch it, incrementing
  `Attempts`). A `Store` that only returns pending jobs from `Due` silently
  breaks crash recovery.
- **The state-transition ordering that makes at-least-once true:**
  `dispatch` writes `State=running` via `store.Update` *before* calling the
  handler. A crash between a successful handler return and the matching
  `Update(done)` leaves the job `running` on disk — the next process's
  first `Due` scan finds and reruns it. This is deliberate (see the
  package's at-least-once doc comment); the queue never tries to detect
  "did the handler actually already run" — that's what `IdempotencyKey`
  is for, and it's the handler's job, not the queue's.
- **`processTick` never fires more than one due-scan per call, regardless
  of gap size.** Coalescing a sleep/wake gap means "the code path that
  could produce a burst does not exist", not "detect a burst and
  suppress it". Don't reintroduce a loop over missed intervals.
- **A Handler must respect `ctx`.** `Start` cancels the shared `ctx` on
  shutdown and waits for every dispatched goroutine via `q.wg.Wait()`
  before returning — a Handler that ignores `ctx` and never returns blocks
  shutdown and leaks the goroutine. This is a Handler contract violation
  (documented on `Handler`), not something `Start` works around.

## Dependencies & insulation

- `events`, `errors`, `i18n` — standard AD-9 pattern (`*events.Emitter`
  optional/nil-safe, `errors.Code` + `RegisterMessages` in `init()`).
- No `wails/v3` import — **not** on the AD-4 allowlist and must stay off
  it. `Queue` runs identically in a CLI/TUI entry point.
- `runner/flatfile` and (WP-41) `runner/sqlitestore` depend on `runner`
  for `Store`/`Lister`/`Job`; `runner` must never import either back.

## Extension points

- New `Store` implementations: only the four `Store` methods are required;
  implement `Lister` too for full `Dead()`/`MaxQueueDepth`/idempotency
  fidelity across restarts, and `Deleter` so `Queue.Discard` can actually
  purge a dead-lettered job's record instead of returning
  `ErrDiscardUnsupported` (see `runner/storetest` for the shared contract
  suite every `Store` should pass).
- New profiles: add a constructor alongside `Ephemeral`/`Durable`/
  `BestEffort` returning a `Profile` literal — don't add a fourth
  `Persistence` value or a new field unless every existing profile's
  behavior needs to change, per "profiles, not knobs, not modes".

## Testing

- Doubles: `newMemoryStore()` (unexported, in-package), `fakeClock` (mutable
  injectable clock — advance it explicitly, never `time.Sleep` for gap
  logic), `events.NewMemoryEmitter()`.
- Call `q.processTick()` directly (package-internal tests) for anything
  clock-driven — retry backoff, wake-gap coalescing, dead-letter — rather
  than waiting on `Start`'s real ticker. Reserve real `Start` usage for
  shutdown-boundedness, where the real tick loop + real `ctx` cancellation
  is the thing under test (`q.tickInterval` is an unexported field tests
  set directly to keep that fast — there is no public `WithTickInterval`).
- `go test -race ./runner/` must cover: each profile's distinguishing
  behavior, retry-with-backoff, dead-letter vs. drop, idempotency dedupe
  (while pending and released after completion), backpressure, wake-gap
  coalescing (with and without `ResumeOnWake`), crash recovery of a
  `JobStateRunning` job, and bounded shutdown with a handler that honors
  `ctx`.
- Not automatable: real OS sleep/wake — always test via injected clock.

## File map

- `runner.go` — `Persistence`, `JobState`, `Job`, `Handler`, `Store`,
  `Lister`, `Deleter`, event/error consts, `init()`.
- `profile.go` — `Profile`, `Ephemeral`/`Durable`/`BestEffort`,
  `DefaultBackoff`.
- `queue.go` — `Queue`, `Option`s, `New`, `Handle`, `Enqueue`,
  `EnqueueOption`s, `Start`/`processTick`/`scanDue`/`dispatch`,
  `completeJob`/`failJob`, `Dead`/`Requeue`/`Discard`, `Close`.
- `memory_store.go` — the built-in `Store`+`Lister`+`Deleter` used for
  `PersistNone`.
- `id.go` — `newJobID`.
- `storetest/storetest.go` — shared `Store` contract test suite.
- `locales/en.json` — this package's i18n catalog.

## Landmines

- **`MaxQueueDepth`/idempotency-key/`Dead()` bookkeeping is only fully
  restart-durable when the `Store` implements `Lister`.** `New` bootstraps
  it from `Lister.List` once, before returning the `Queue` to the caller
  (see `recoverOnce`, called from `New` and, redundantly but harmlessly,
  from `Start` — the second call is a no-op guarded by `q.recovered`); a
  minimal custom `Store` without `Lister` still works, but that bookkeeping
  then only reflects what happened since the current process started. Both
  built-in stores implement `Lister`.
- **The bootstrap must run in `New`, not `Start`.** It used to run only in
  `Start`, which double-counted `q.depth`: `Enqueue` (called any time after
  `New`) increments `q.depth` for a job it just persisted, and if the same
  job were later found again by a `Start`-time bootstrap scanning the
  Store, it got counted a second time — permanently inflating `q.depth`
  relative to reality once drained (the `depth > 0` floor prevents
  underflow, not the residue). See `TestEnqueueBeforeStartDoesNotDoubleCountDepth`.
  Enqueuing before the first `Start()` is exactly the order the package doc
  example itself uses — it must work, not merely be tolerated.
- **`Close` racing a still-running `Start` is a caller bug.** Cancel
  `Start`'s `ctx` and let it return before calling `Close` — otherwise
  `Store.Close` can run while `Start` is still using the store.
- **No `WithTickInterval` option** — fixed at `defaultTickInterval` (1s) on
  purpose ("desktop infrastructure, not a server queue you tune"). Tests
  reach past this via the unexported `tickInterval` field.
- **`Requeue` isn't guarded against two concurrent calls on the same dead
  job ID** — both can read it as dead and double-count `q.depth`. Untested;
  low risk (normally one explicit user action) but real if a UI double-fires it.
