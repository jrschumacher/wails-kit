# lifecycle

Ordered startup and shutdown of services with dependency tracking.

## Usage

```go
import "github.com/jrschumacher/wails-kit/lifecycle"

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
- **Partial failure rollback** — if service N fails to start, services 1..N-1 are shut down in reverse order.
- **All-errors shutdown** — shutdown continues through errors, collecting them all via `errors.Join`.
- **Order** — `mgr.Order()` returns the resolved startup order for inspection.
- **Timeouts** — global and per-service timeouts for startup/shutdown. Startup timeouts trigger rollback; shutdown timeouts are collected but don't stop other services from shutting down.
- **Health checks** — services implementing `HealthChecker` report their status via `mgr.Health()`. Non-implementing services default to healthy.

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
