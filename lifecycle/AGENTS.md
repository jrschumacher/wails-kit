# lifecycle — agent notes

## Purpose

`lifecycle` orders startup and shutdown of named services with dependency
declarations: topological sort, per-service and global timeouts, rollback on
partial startup failure, and a simple health-check aggregator. It owns
*ordering and timing* of `OnStartup`/`OnShutdown` calls across services it
doesn't otherwise know anything about. It does not own service
implementation, retries, or supervision after a service is running (no
crash-restart) — a service that dies quietly after `OnStartup` returns is
invisible to this package unless it implements `HealthChecker`.

## Public API (load-bearing signatures)

```go
type Service interface {
    OnStartup(ctx context.Context) error
    OnShutdown() error
}
type HealthChecker interface{ Health() HealthStatus } // optional

func NewManager(opts ...ManagerOption) (*Manager, error)
func WithService(name string, svc Service, opts ...ServiceOption) ManagerOption
func DependsOn(names ...string) ServiceOption
func WithServiceTimeout(d time.Duration) ServiceOption
func WithEmitter(e *events.Emitter) ManagerOption
func WithTimeout(d time.Duration) ManagerOption

func (m *Manager) Order() []string
func (m *Manager) Startup(ctx context.Context) error // ErrInvalidState if not idle
func (m *Manager) Shutdown() error                    // safe no-op if not started
func (m *Manager) Health() []ServiceHealth
```

## Invariants (do not break)

- **Duplicate service names are rejected at `NewManager`.** `topoSort`
  checks for a repeated name *before* building the dependency graph. Without
  this, the name→bool set used for cycle detection silently deduped two
  entries into one key, `entryMap()` (last-writer-wins) made one of the two
  services unreachable by name, and the visible symptom was "the wrong
  service started twice, the other never started, and shutdown ran twice
  against the survivor." `TestDuplicateServiceName` is the regression test.
- **`rollback` shuts services down through `stopService`, the same path
  `Shutdown` uses — never `e.service.OnShutdown()` directly.** This is what
  makes `WithTimeout`/`WithServiceTimeout` apply during rollback. Calling
  `OnShutdown` directly (the pre-fix behavior) meant a service hanging in
  `OnShutdown` during rollback blocked `Startup` forever, defeating the
  entire point of configuring a timeout. `TestRollbackHonorsTimeout` is the
  regression test — it asserts `Startup` returns well inside the configured
  timeout, not that it "eventually" returns.
- **`Manager` has one internal state machine (idle → starting → started →
  stopping → idle), guarded by `m.mu`, and `Startup`/`Shutdown` check it.**
  `Startup` while not idle returns `ErrInvalidState` instead of silently
  resetting `m.started` and re-running every service's `OnStartup` on top of
  whatever's already running (`TestStartup_TwiceWithoutShutdown_ReturnsError`,
  `TestStartup_ConcurrentCalls_OnlyOneSucceeds`). `Shutdown` while not
  started is a safe no-op (`TestShutdown_SafeNoOpWhenNeverStarted`) so
  `defer mgr.Shutdown()` is always safe to write unconditionally. Any new
  method that reads or writes `m.started` must take `m.mu` — `entries` and
  `order` are fixed after `NewManager` and don't need it.
- **A startup timeout reaps the service in the background if it later
  succeeds anyway.** `startService`'s timeout branch launches
  `reapTimedOutStartup`, which keeps waiting on the original `OnStartup`
  call and shuts the service back down if it does eventually return `nil`.
  Without this, a service that ignores `ctx` cancellation and finishes after
  the manager already reported it as failed would never be in `m.started`,
  so nothing would ever call its `OnShutdown` — it leaks, still running, for
  the life of the process. This is best-effort (see Landmines) — it does not
  make `startService` wait any longer than the configured timeout.
- Startup failure always rolls back already-started services in reverse
  order before returning; the *original* startup error is always what's
  returned (joined with rollback errors if any), never replaced by a
  rollback error.
- Shutdown never stops on the first error — it collects every service's
  error via `errors.Join` so one stuck service doesn't hide failures in the
  others.

## Dependencies & insulation

- `errors` (kit package) — every failure is an `errors.Code`-tagged
  `*errors.UserError`, registered via `RegisterMessages` in `init()`.
- `events` — optional (`WithEmitter`); nil emitter is a no-op via `m.emit`.
- No `wails/v3` import; this package is Wails-free per the AD-4 allowlist —
  it's meant to run identically inside Prune's CLI/TUI processes (no
  `application.App`) and inside a Wails app's `ServiceStartup`.

## Extension points

- New construction knobs go through `ManagerOption`/`ServiceOption`,
  following the existing `With*` pattern.
- A supervised-restart mode (react to a running service reporting
  `StatusUnhealthy` or dying) is explicitly out of scope for this package —
  it's an ordering/timing primitive, not a supervisor. If that's ever
  needed, it belongs in a new package built on top of `Health()`, not bolted
  onto `Manager`.

## Testing

- Doubles: `mockService` (tracks started/stopped, injectable errors and
  start-order slice), `slowService` (configurable start/stop delay, respects
  ctx cancellation on startup), `ignoresContextService` (deliberately does
  *not* respect ctx cancellation — the double for the reap tests),
  `healthyService` (wraps `mockService`, implements `HealthChecker`).
  `events.NewMemoryEmitter()` + `events.NewEmitter(...)` for event assertions.
- `go test -race ./lifecycle/` must cover: topological ordering (including
  diamond dependencies), missing-dependency and cyclic-dependency rejection,
  duplicate-name rejection, partial-failure rollback (including rollback
  itself failing), startup/shutdown timeouts (global and per-service),
  rollback honoring timeouts, the state-machine guard against repeated/
  concurrent `Startup` (run this one under `-race` — it's specifically
  testing the "no mutex" defect), `Shutdown`'s never-started no-op, health
  aggregation, and the timed-out-startup reap path.
- Timeout-related tests use short real durations (10s–500ms range) with
  generous assertion windows rather than a fake clock — keep new timeout
  tests fast (sub-second) so `-race` runs stay quick, but don't shave
  margins so tight that they flake under CI scheduling jitter.

## File map

- `lifecycle.go` — `Manager`, `Service`/`HealthChecker` interfaces, all
  `ManagerOption`/`ServiceOption` functions, `NewManager`, `Startup`,
  `Shutdown`, `Health`, `topoSort`, `startService`/`stopService`/`rollback`/
  `reapTimedOutStartup`, event/error definitions.
- `lifecycle_test.go` — full test suite plus the test-double types above.
- `README.md` — quickstart, Wails v3 integration example, options tables,
  behavior summary, events, the "Concurrency and reuse contract" section
  (read this before changing `Startup`/`Shutdown`'s state handling), health
  checks, error codes.

## Landmines

- **The reap path is best-effort and unbounded.** `reapTimedOutStartup`
  blocks on the original `OnStartup` call's result channel for as long as
  that call takes — if a service ignores `ctx` and also never returns, the
  goroutine leaks for the life of the process (though it does nothing
  harmful while parked). This is a deliberate tradeoff: the alternative
  (giving the reaper its own timeout) just moves the same "what if it never
  returns" problem one level down without solving it. The real fix is
  services that respect `ctx` cancellation — this exists as a safety net for
  the case where they don't, not a substitute for writing them correctly.
- **`Health()` returns a snapshot of `m.started` at call time**, not a live
  view — a service that stops being healthy microseconds after `Health()`
  reads it won't be reflected until the next call. This is inherent to
  polling, not a bug, but don't assume `Health()`'s result stays valid
  across an `await`/goroutine boundary.
- **State transitions don't roll back to `stateStarted` on a `Shutdown`
  error.** If `Shutdown` returns a joined error (some service failed to stop
  cleanly), the manager still transitions to `stateIdle` and a subsequent
  `Startup` is allowed — "shut down with errors" and "cleanly shut down" are
  treated the same for state-machine purposes. This matches the pre-existing
  contract (`Shutdown` always clears `m.started` regardless of errors); it's
  not new behavior from the mutex/state-machine work, just worth knowing
  it's intentional.
