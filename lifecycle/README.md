# lifecycle

Ordered startup and shutdown of services with dependency tracking.

## Usage

```go
import "github.com/jrschumacher/wails-kit/v2/lifecycle"

// Services implement the lifecycle.Service interface.
type DatabaseService struct{ /* ... */ }
func (d *DatabaseService) OnStartup(ctx context.Context) error { /* ... */ }
func (d *DatabaseService) OnShutdown() error                   { /* ... */ }

// Register services with dependency declarations.
mgr, err := lifecycle.NewManager(
    lifecycle.WithService("database", dbService),
    lifecycle.WithService("settings", settingsService, lifecycle.DependsOn("database")),
    lifecycle.WithService("storage", storageService, lifecycle.DependsOn("database")),
    lifecycle.WithService("updates", updateService, lifecycle.DependsOn("settings")),
    lifecycle.WithEmitter(emitter),            // optional
    lifecycle.WithTimeout(10 * time.Second),   // optional global timeout
)
// err if cyclic or missing dependencies

// Start all services in dependency order.
err = mgr.Startup(ctx)

// Check health of running services.
health := mgr.Health() // []ServiceHealth

// Shut down in reverse order (collects all errors).
err = mgr.Shutdown()
```

## Integration with Wails v3

The manager can be used inside your app's `ServiceStartup` and `ServiceShutdown`:

```go
type App struct {
    mgr *lifecycle.Manager
}

func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
    mgr, err := lifecycle.NewManager(
        lifecycle.WithService("database", a.db),
        lifecycle.WithService("settings", a.settings, lifecycle.DependsOn("database")),
    )
    if err != nil {
        return err
    }
    a.mgr = mgr
    return mgr.Startup(ctx)
}

func (a *App) ServiceShutdown() error {
    return a.mgr.Shutdown()
}
```

## Options

### Manager options

| Option | Description |
|--------|-------------|
| `WithService(name, svc, ...ServiceOption)` | Register a named service |
| `WithEmitter(emitter)` | Set event emitter for lifecycle events |
| `WithTimeout(d)` | Global timeout for startup/shutdown of each service |

### Service options

| Option | Description |
|--------|-------------|
| `DependsOn(names...)` | Declare dependencies that must start first |
| `WithServiceTimeout(d)` | Per-service timeout override (takes precedence over global) |

## Behavior

- **Dependency ordering** — topological sort via Kahn's algorithm. Cycle detection at construction time.
- **Duplicate names rejected** — `NewManager` returns `lifecycle_duplicate_service` if two `WithService` calls register the same name, rather than silently letting the second registration shadow the first.
- **Partial failure rollback** — if service N fails to start, services 1..N-1 are shut down in reverse order, **through the same timeout machinery as `Shutdown`** (`WithTimeout`/`WithServiceTimeout` apply during rollback, not just during a normal `Shutdown` call).
- **All-errors shutdown** — shutdown continues through errors, collecting them all via `errors.Join`.
- **Order** — `mgr.Order()` returns the resolved startup order for inspection.
- **Timeouts** — global and per-service timeouts for startup/shutdown. Startup timeouts trigger rollback; shutdown timeouts are collected but don't stop other services from shutting down.
- **Health checks** — services implementing `HealthChecker` report their status via `mgr.Health()`. Non-implementing services default to healthy.

## Concurrency and reuse contract

A `Manager` is **not** a reusable, freely-concurrent object — it's a single
startup/shutdown cycle with a small internal state machine (idle → starting →
started → stopping → idle) guarded by a mutex:

- **`Startup` twice without an intervening `Shutdown` returns `lifecycle_invalid_state`** instead of silently re-running every service's `OnStartup` on top of whatever's already running. This also makes concurrent `Startup` calls safe: exactly one wins, the other gets `lifecycle_invalid_state`.
- **`Shutdown` is a safe no-op** if the manager was never successfully started (or a `Shutdown`/`Startup` is already in flight), so `defer mgr.Shutdown()` is always safe to write unconditionally, even after a `Startup` failure that already rolled back.
- **A full `Startup` → `Shutdown` cycle can be repeated** — after `Shutdown` returns, the manager is idle again and `Startup` can be called fresh.
- **`Health()` is safe to call concurrently** with `Startup`/`Shutdown` — it takes a consistent snapshot rather than racing on the internal started-services list.

### Orphaned timed-out startups

If a service's `OnStartup` ignores context cancellation and keeps running past
its configured timeout, the manager has already reported it as failed (and
moved on to rollback, or returned the error to the caller) by the time it
finishes. If it then finishes *successfully*, it is never added to the
started-services list, so nothing would ever call its `OnShutdown` — it would
otherwise leak, still running, for the life of the process.

wails-kit's chosen contract: the manager reaps this in the background. It
keeps waiting on the original `OnStartup` call after returning the timeout
error, and if that call does eventually return `nil`, immediately calls
`OnShutdown` on it. This is best-effort — a failure during that reap is only
reported via `lifecycle:error`, since there's no in-flight `Startup`/`Shutdown`
call left to return it to. **Write services that respect `ctx` cancellation in
`OnStartup`** so this path is never exercised in practice; it exists as a
safety net, not a supported way to abort slow work.

## Events

| Event | Payload | Description |
|-------|---------|-------------|
| `lifecycle:started` | `ServiceStartedPayload{Name}` | Service started successfully |
| `lifecycle:stopped` | `ServiceStoppedPayload{Name}` | Service stopped successfully |
| `lifecycle:error` | `ErrorPayload{Name, Message, Code}` | Service failed to start or stop |
| `lifecycle:rollback` | `RollbackPayload{FailedService, RollingBack, RollbackErrors}` | Partial failure triggered rollback |
| `lifecycle:timeout` | `TimeoutPayload{Name, Phase, Timeout}` | Service startup or shutdown timed out |

## Health checks

Services can optionally implement `HealthChecker` to report their status:

```go
type MyService struct{ /* ... */ }

func (s *MyService) Health() lifecycle.HealthStatus {
    if s.isConnected() {
        return lifecycle.StatusHealthy
    }
    return lifecycle.StatusDegraded
}
```

Call `mgr.Health()` to get a `[]ServiceHealth` slice with each service's name and status (`healthy`, `degraded`, or `unhealthy`). Services that don't implement `HealthChecker` default to `healthy`.

## Error codes

| Code | User message |
|------|-------------|
| `lifecycle_cyclic_dependency` | Service configuration error: circular dependency detected. |
| `lifecycle_missing_dependency` | Service configuration error: a required dependency is missing. |
| `lifecycle_startup` | Failed to start a required service. Please try restarting the application. |
| `lifecycle_shutdown` | An error occurred while shutting down. Some resources may not have been cleaned up. |
| `lifecycle_timeout` | A service took too long to respond. Please try restarting the application. |
| `lifecycle_duplicate_service` | Service configuration error: a service name is registered more than once. |
| `lifecycle_invalid_state` | The application's service manager is busy or already running. Please try again. |
