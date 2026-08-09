# health — agent notes

## Purpose

`health` is the kit's single connectivity registry: apps and other kit
packages register `Check`s (name + `Class` + `Probe`), the registry probes
them on a cadence or on demand, caches each one's last-known `CheckStatus`,
and exposes a resolved `Snapshot` plus `health:changed` events on state
transitions. It owns failure-mode classification (connectivity vs backend
vs provider) and offline suppression. It does not own presentation (a
status page/badge is the app's job) or retry/backoff policy beyond "probe
again next interval" — a `Probe` that wants backoff implements it itself.

## Public API (load-bearing signatures)

```go
type Class string // ClassConnectivity | ClassBackend | ClassProvider
type State string  // StateUnknown | StateHealthy | StateDegraded | StateDown

type Probe interface{ Probe(ctx context.Context) error }
func HTTPProbe(url string, opts ...HTTPOption) Probe // GET, expect <400, 10s timeout, all overridable

type Check struct{ Name string; Class Class; Critical bool; Interval time.Duration; Probe Probe }
type CheckStatus struct{ Name string; Class Class; State State; Critical bool; Err string; CheckedAt time.Time; Latency time.Duration }
type Snapshot struct{ Overall State; Offline bool; Checks []CheckStatus }

func New(opts ...Option) *Registry // auto-registers a default connectivity check
func WithEmitter(e *events.Emitter) Option
func WithDefaultInterval(d time.Duration) Option
func WithConnectivityProbe(p Probe) Option
func WithoutDefaultConnectivityCheck() Option

func (r *Registry) Register(c Check) (remove func(), err error) // errors on duplicate Name
func (r *Registry) Unregister(name string) bool
func (r *Registry) Snapshot() Snapshot
func (r *Registry) Trigger(names ...string) // blocks until named probes complete
func (r *Registry) Start(ctx context.Context) // blocks until ctx.Done + goroutines joined
func (r *Registry) Binding() *Binding // GetSnapshot/Trigger
func (r *Registry) SettingsGroup() settings.Group // check-interval picker
```

## Invariants (do not break)

- **Never emit while holding `r.mu`.** `probeOnce` snapshots the changed
  `CheckStatus` and `Snapshot` under the lock, releases it, *then* calls
  `r.emitter.Emit`. `state` and `settings` both shipped this deadlock in
  Phase 0 — see `docs/v2-roadmap.md` WP-06/WP-03.
- **A freshly `Register`-ed check starts at `StateUnknown`, never
  `StateHealthy`.** A green badge for something nobody has checked is worse
  than no badge. `TestUnknownVersusHealthy`.
- **`Snapshot.Overall` is `StateUnknown`, not `StateHealthy`, with no
  critical checks (including an empty registry).** Same reasoning, one
  level up. `TestEmptyRegistryOverallIsUnknown`.
- **Offline suppression never touches the connectivity check's own
  entry.** Only non-`ClassConnectivity` checks get suppressed to
  `StateUnknown` in `Snapshot.Checks` when `Offline` is true — the
  connectivity check itself must keep reporting its real (`StateDown`)
  state, or nothing could ever detect "we're back online".
  `TestOfflineDoesNotSuppressConnectivityItself`.
- **`health:changed` fires only on an actual `State` transition**, never on
  every probe (`probeOnce` compares `prevState != newState`). A healthy
  endpoint polled every 30s must not spam listeners. `TestTransitionEventsOnly`.
- **Every probe call is bounded.** `HTTPProbe` always layers its own
  `WithTimeout` (default 10s) on top of the passed `context.Context` via
  `context.WithTimeout` in `Probe`. A custom `Probe` that ignores `ctx` and
  blocks forever will leak its own goroutine — document this expectation
  wherever a new `Probe` implementation is added, don't just assume it.
- **`Start` blocks until `ctx.Done()` *and* every spawned goroutine has
  returned** (`r.wg.Wait()`), so a caller doing `go r.Start(ctx); cancel();
  <-done` gets a genuinely clean shutdown, not "probably stopped by now".

## Dependencies & insulation

- `events`, `errors` — standard AD-9 pattern (`*events.Emitter` optional,
  `errors.Code` + `RegisterMessages` in `init()`).
- `i18n`, `settings` — for `SettingsGroup()`'s `i18n.Text`-typed labels
  (WP-12). **One direction only**: `health` imports both; neither may ever
  import `health` back, and `i18n` must never import `settings` (see
  `i18n/AGENTS.md` Landmines on that cycle). `health.SettingsGroup()`
  returns unresolved `i18n.Text` fields — resolution happens centrally in
  `settings.Service`'s own localizer, not here.
- No `wails/v3` import — **not** on the AD-4 allowlist, and must stay off
  it. `Registry` and every `Probe` run identically in Prune's CLI/TUI.

## Extension points

- New `Probe` implementations: implement the one-method interface directly,
  or wrap `ProbeFunc`. Reuse `HTTPProbe` for anything HTTP-shaped rather
  than hand-rolling request/timeout logic — see `updates/health.go`'s
  `githubReleasesProbe` for the pattern (build a fresh `HTTPProbe` per call
  when the target URL/client can change at runtime).
- A package that polls or probes something should register with an
  app-supplied `*Registry` via its own `With*` option (see
  `updates.WithHealth`), not embed its own connectivity logic — AD-9.

## Testing

- Doubles: `noopProbe`/`countingProbe`/`blockingProbe` (test-local, no
  network). `httptest.Server` for reachable endpoints; a server created
  then immediately `.Close()`d for an unreachable address (connection
  refused, deterministic, no real network).
- **Every test constructing a `Registry` via `New` must override the
  built-in connectivity check** (`WithConnectivityProbe` or
  `WithoutDefaultConnectivityCheck`) — otherwise `Start`/`Trigger` reach out
  to the real default connectivity URL. See `newTestRegistry` in
  `registry_test.go`.
- `go test -race ./health/` must cover: registration + probing against
  `httptest`, critical-down -> overall down, non-critical-down -> overall
  unaffected, unknown-vs-healthy, offline suppression (and that it spares
  the connectivity check itself), manual `Trigger` overriding cadence,
  transition-only events, and `Start`'s clean shutdown under cancellation
  (`schedule_test.go` — a `blockingProbe` that only returns once `ctx.Done()`
  fires, asserting `Start` returns promptly after `cancel()`).

## File map

- `health.go` — `Class`, `State` (+ severity ordering), `Probe`/`ProbeFunc`,
  `Check`, `CheckStatus`, `Snapshot`, event/error consts, `init()`.
- `httpprobe.go` — `HTTPProbe`, `HTTPOption`s, the default connectivity
  probe/URL.
- `registry.go` — `Registry`, `Option`s, `New`, `Register`, `Unregister`,
  `Snapshot`/`snapshotLocked` (offline suppression + `Overall` computation).
- `schedule.go` — `Start`, `runCheck` (per-check goroutine), `probeOnce`,
  `Trigger`.
- `binding.go` — `Binding` (Wails-safe surface).
- `settings.go` — `SettingsGroup`, interval choices/labels.
- `locales/en.json` — this package's catalog (checked by `i18n`'s
  repo-wide `TestCatalogKeysExist`).

## Landmines

- **`New`'s default connectivity check makes a real network request** (GET
  against a Google captive-portal-check URL) the moment anything calls
  `Trigger("connectivity")` or `Start`. It does *not* fire just from being
  registered — `Register` alone never probes anything — but any test that
  calls `Start`/`Trigger` without overriding it will hit the real network.
  Always pass `WithConnectivityProbe` or `WithoutDefaultConnectivityCheck`
  in tests.
- **`Trigger` and `Start` use different contexts.** `Trigger` always uses
  `context.Background()` (its signature takes no `ctx` — see WP-15) with
  bounding left entirely to the `Probe`'s own timeout; `Start`'s probes
  share the `ctx` passed to `Start`, so they *also* stop early on
  cancellation, not just on their own timeout. A `Probe` that only checks
  `ctx.Err()` and doesn't have its own timeout will hang forever under
  `Trigger` if it also ignores cancellation — this is why `HTTPProbe`
  always sets one itself.
- **`Registry.SettingsGroup()`'s `Default` is always present in `Options`**,
  even for a non-standard `WithDefaultInterval` value — it gets appended as
  an extra, untranslated (`Text.Key == ""`) option. Don't "clean up" that
  append by assuming `Default` is always one of the six fixed choices.
