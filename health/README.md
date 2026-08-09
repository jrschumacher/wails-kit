# health

Package `health` is the kit's single connectivity registry. Apps and other
kit packages register endpoints; the registry probes them on a cadence (or
on demand), caches each endpoint's last-known state, and exposes a resolved
snapshot plus change events — one place to drive a status page or badge.

## Why this is a package, not a `navigator.onLine` check

A frontend online/offline check collapses three distinct failure modes into
one frequently-wrong answer (it reports "online" when connected to a router
with no upstream). This package keeps them separate, because each has a
different remedy:

| `Class` | Means | Remedy |
|---|---|---|
| `ClassConnectivity` | No network at all | Nothing the app can do — tell the user, don't blame a backend |
| `ClassBackend` | Your own API is down | Status page, retry |
| `ClassProvider` | A third party is down | Different messaging — it isn't the app's fault |

The other half of the value: **nothing else in the app should embed its own
connectivity check.** One registry knows reachability; everything else
subscribes. `updates` follows this — see `updates/health.go`'s `WithHealth`,
which registers the GitHub Releases API as a `ClassProvider` check without
the app having to know it exists.

## Usage

```go
import (
    "context"
    "time"

    "github.com/jrschumacher/wails-kit/v2/health"
)

registry := health.New(
    health.WithEmitter(emitter),              // optional; nil is fine
    health.WithDefaultInterval(30*time.Second),
)

remove, err := registry.Register(health.Check{
    Name:     "api",
    Class:    health.ClassBackend,
    Critical: true, // the app degrades to "offline mode" when this is down
    Probe:    health.HTTPProbe("https://api.example.com/health"),
})
if err != nil {
    log.Fatal(err)
}
defer remove()

// Manual, user-initiated "check now":
registry.Trigger("api")

// A resolved, point-in-time view — what a status page or badge renders:
snap := registry.Snapshot()

// Run the background scheduler until ctx is cancelled. Call this in its
// own goroutine; cancel ctx to stop it.
go registry.Start(ctx)
```

`New` auto-registers a built-in connectivity check (name `"connectivity"`,
class `ClassConnectivity`, critical) that GETs a well-known captive-portal
-check URL and expects a bare `204`. Override its `Probe` with
`WithConnectivityProbe`, or remove it entirely with
`WithoutDefaultConnectivityCheck()` (at construction) or
`registry.Unregister(health.DefaultCheckName)` (at runtime).

## The "not yet checked" vs "checked and healthy" distinction

A freshly registered check starts at `StateUnknown`, never `StateHealthy`.
`Snapshot.Overall` is `StateUnknown` whenever the registry has no critical
checks at all, or any critical check hasn't been probed yet — a snapshot
that reported an unprobed endpoint as healthy would drive a green badge for
something nobody has actually checked.

## Offline suppression

When the connectivity check (`ClassConnectivity`) is `StateDown`,
`Snapshot.Offline` is `true` and every other check's reported `State` is
`StateUnknown` in `Snapshot.Checks` — not `StateDown`. "Your API is down"
must never be claimed when the real problem is "there is no network at
all". The underlying probed state isn't lost: it already drove
`health:changed` when it transitioned, and resumes reporting normally once
connectivity returns.

`Snapshot.Overall` is the worst state among **critical** checks after this
suppression is applied (severity order: healthy < unknown < degraded <
down). Non-critical checks never affect `Overall`, whatever their state.

## Options

| Option | Description |
|---|---|
| `WithEmitter(e)` | Optional `*events.Emitter`. `health:changed` fires on state transitions. |
| `WithDefaultInterval(d)` | Probing cadence for any `Check` that doesn't set its own `Interval`. Default 60s. |
| `WithConnectivityProbe(p)` | Overrides the built-in connectivity check's `Probe`. Default: HTTP GET expecting 204 from a Google captive-portal-check endpoint — override this for tests and for apps with their own notion of "online" (e.g. probing a LAN gateway). |
| `WithoutDefaultConnectivityCheck()` | Skip auto-registering the built-in connectivity check entirely. |

## `Probe` and `HTTPProbe`

```go
type Probe interface{ Probe(ctx context.Context) error }
```

Implement `Probe` for anything (a raw TCP dial, a gRPC health-check call,
whatever). `health.HTTPProbe(url, opts...)` covers the common case: GET,
expect status `< 400` by default, and — critically — a per-request timeout
(`WithTimeout`, default 10s) layered on top of whatever `context.Context`
it's called with, so one hung endpoint can never stall the calling goroutine
indefinitely.

| `HTTPOption` | Description |
|---|---|
| `WithMethod(m)` | HTTP method. Default `GET`. |
| `WithTimeout(d)` | Per-request timeout. Default 10s. |
| `WithExpectStatus(fn)` | Status-code predicate. Default `code < 400`. |
| `WithHTTPClient(c)` | Override the `*http.Client`. Default `http.DefaultClient`. |

## Events

| Event | Payload | Description |
|---|---|---|
| `health:changed` | `ChangedPayload{Check, Overall}` | Fired **only when a check's `State` actually changes** — not on every probe. A healthy endpoint pinging every 30s does not spam the frontend. |

## Error codes

| Code | User message |
|---|---|
| `health_duplicate_check` | This health check is already registered. |
| `health_invalid_check` | This health check is missing required configuration. |

## Settings

`registry.SettingsGroup()` returns a `settings.Group` with one field — a
check-interval picker (`health.check_interval`, a `time.Duration.String()`
value like `"30s"`). The registry does not read this setting itself; wire
the submitted value to `WithDefaultInterval` (at next construction) or a
specific `Check.Interval`, the same "app reads it, app decides" pattern
`updates.SettingsGroup` uses for `check_frequency`.

## Frontend binding

`registry.Binding()` returns the Wails-safe surface (`GetSnapshot`,
`Trigger`) — register `registry.Binding()` with Wails, never `*Registry`
directly (it has no unsafe methods today, but the split is the load-bearing
convention every other kit binding follows; see `settings.Binding`).

## Other kit packages registering their own checks

`updates` registers the GitHub Releases API as a `ClassProvider` check when
constructed with `updates.WithHealth(registry)` — see `updates/health.go`'s
doc comments (as of this writing, `updates/README.md` doesn't yet document
`WithHealth`; that's a gap for whoever next touches that file). Follow the
same pattern for any package that polls or probes a remote endpoint: build
a `health.Probe` (or reuse `health.HTTPProbe`), and register it with the
app's registry rather than implementing a separate "is X reachable"
mechanism.

## Testing

No test in this package (or `updates/health_test.go`) touches the real
network: `httptest` servers stand in for reachable endpoints, and a closed
listener's URL stands in for an unreachable one. Every test that constructs
a `Registry` via `New` overrides the built-in connectivity check's probe
(`WithConnectivityProbe` or `WithoutDefaultConnectivityCheck`) — otherwise
`Start`/`Trigger` would make a real request to the default connectivity URL.
