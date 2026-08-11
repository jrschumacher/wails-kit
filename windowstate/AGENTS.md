# windowstate — agent notes

## Purpose

`windowstate` persists one window's position/size/maximised-state across
restarts and restores it clamped to a currently attached display. It owns
geometry capture (debounced, skip-while-minimised, maximised-flag
separation) and restore-time clamping; it does not own the storage format
or atomic writes (that's `state.Store[Geometry]`), and it does not own
window *creation* — the caller builds the `*application.WebviewWindow`
before calling `Manage`.

## Public API (load-bearing signatures)

```go
type Geometry struct {
    X, Y, W, H int
    Maximised  bool
    ScreenID   string // informational only; restore clamps by intersection, not ID match
}

type Manager struct{ /* unexported */ }

func Manage(app *application.App, win *application.WebviewWindow, opts ...Option) (*Manager, error)
func WithName(name string) Option          // per-window state key; default "main"
func WithStore(st *state.Store[Geometry]) Option
func WithDebounce(d time.Duration) Option  // default 500ms
func WithEmitter(e *events.Emitter) Option

func (m *Manager) Restore() // call before win.Show(); never errors, see manager.go
func (m *Manager) Close()   // flushes pending debounced save synchronously

const EventRestored = "windowstate:restored" // RestoredPayload{Name, Geometry, Clamped}
const EventError    = "windowstate:error"    // ErrorPayload{Name, Op, Error}
```

## Invariants (do not break)

- **Never save while `win.IsMinimised()`.** A minimised window's reported
  bounds are platform garbage. `captureAndScheduleSave` (manager.go) checks
  this before doing anything else, including before acquiring `m.mu`.
- **Maximised bounds never overwrite `lastRestored`.** `lastRestored` is
  only updated from `Position()`/`Size()` when `!maximised`; while
  maximised, only the `Maximised` flag changes on the pending save. Break
  this and a maximised window's saved geometry becomes the maximised
  bounds, so the next restore-then-unmaximise leaves it stuck full-screen
  -sized. `TestMaximisedSavesRestoredGeometryNotMaximisedBounds` is the
  regression test.
- **`Store.Save` is never called with `m.mu` held.** `flush` copies
  `m.pending` out under the lock, releases it, then calls `store.Save`
  outside — mirrors the `state` package's own emit-outside-lock rule (its
  `AGENTS.md`). `state.Store` does its own internal locking; nesting would
  risk unnecessary contention, not deadlock, but keep the shape anyway.
- **`Restore` never returns an error.** A missing save (first run) and a
  load failure are both "leave the window at its Wails-configured
  defaults" — not exceptional. Failures still go out via `EventError` for
  callers that want to know. Don't change the signature to `error` without
  updating every call site that relies on "safe to call unconditionally
  before Show".
- **Debounce coalesces to the last event, not the first.** `resetTimerLocked`-
  equivalent logic in `captureAndScheduleSave` stops and replaces the timer
  on every event; it does not use a leading-edge or max-wait strategy.
  `TestDebounce` asserts both "exactly one write" and "the last geometry
  wins".

## Dependencies & insulation

- `github.com/wailsapp/wails/v3/pkg/application` and
  `.../pkg/events` (aliased `wevents`) — on the AD-4 allowlist
  (`shortcuts`, `windowstate`, `permissions`, `kit/wailsbridge`). Imported
  plainly, no build tag — see `docs/v2-roadmap.md` AD-4. `window.go` and
  `clamp.go` are the only files touching Wails types directly.
- `state` (kit package) — `Manager` wraps one `state.Store[Geometry]` for
  all persistence. Do not hand-roll JSON/file I/O here; if `state` is
  missing a capability you need, that's a `state` change, not a
  `windowstate` workaround.
- `errors`, `events` (kit packages) — standard AD-9 pattern: `errors.Code`
  + `RegisterMessages` in `init()`; `*events.Emitter` optional, nil is a
  no-op via `Manager.emit`.

## Extension points

- New construction knobs go through `Option`/`config`, following
  `WithDebounce`'s pattern.
- A `Track`-without-`Manage` split (separating "start listening" from
  "construct") was considered and rejected: `Manage` wires listeners
  immediately and unconditionally for the Manager's whole lifetime (until
  `Close`), matching the roadmap's WP-14 API. If a future use case needs
  pause/resume, add it as a new method rather than changing `Manage`'s
  contract.
- Screen-selection strategy (which screen counts as "the" match) lives
  entirely in `clamp.go`'s `clamp`/`primaryScreen`/`centerOn`. It's pure —
  no `Manager` state — so new clamping heuristics are testable without
  touching `Manager` at all.

## Testing

- Doubles: `fakeWindow` (`fake_test.go`) implements `Window` without any
  Wails runtime dependency — `Move`/`Resize`/`Minimise` mutate state and
  fire registered listeners like a real window would report. `fakeScreens`
  implements `ScreenSource` over a fixed `[]*application.Screen` (plain
  data, fine to fabricate without a display).
- `go test -race ./windowstate/` must cover: detached-display clamp
  (`TestClampDetachedDisplay` — the highest-value case), partial-overlap
  and oversized-geometry clamp math (`clamp_test.go`), debounce coalescing
  (`TestDebounce`), skip-while-minimised (`TestSkipWhileMinimised`),
  maximised save/restore round-trip, per-window key isolation
  (`TestPerWindowKeyIsolation`), and `Close`'s synchronous flush.
- **Not automatable here**: real multi-monitor behavior (actual
  plug/unplug, real OS `WorkArea` values, HiDPI scale factors,
  platform-specific move/resize event debounce quirks — e.g. Windows
  applies its own `WindowDidMoveDebounceMS` before this package ever sees
  an event). Verify by hand per the README's "What automated tests cover"
  section using `examples/windowstate/`. Do not construct a real
  `*application.App`/`*application.WebviewWindow` in this package's test
  suite to work around that gap — `manage_test.go`'s `TestManageRequiresApp`
  is the one place `Manage` itself is touched, and only for its nil-guard.

## File map

- `windowstate.go` — `Geometry`, error codes, event names/payloads,
  `Option`/`config`, `Manager` struct, `newManager` (Wails-free
  constructor), `emit`/`emitError`.
- `window.go` — `Window`, `ScreenSource` interfaces; `wailsWindow` adapter
  over `*application.WebviewWindow`.
- `manage.go` — `Manage`, the public Wails-typed constructor.
- `manager.go` — `track`, `captureAndScheduleSave`, `flush`, `Restore`,
  `Close`.
- `clamp.go` — pure clamping math: `clamp`, `intersectArea`, `rectArea`,
  `primaryScreen`, `centerOn`.
- `fake_test.go` — `fakeWindow`, `fakeScreens` test doubles.
- `clamp_test.go`, `manager_test.go`, `manage_test.go` — test suite.

## Landmines

- `state.Store[Geometry].Load()` returns the *zero* `Geometry` when no file
  exists yet — indistinguishable from "someone saved a 0x0 window" at the
  type level. `Restore` treats `W<=0 || H<=0` as "nothing saved" and
  returns early. If you ever add `WithDefaults` support to the underlying
  store, this sentinel check needs revisiting.
- `WithStore`'s type is the concrete `*state.Store[Geometry]`, not an
  interface — by design, per the task's "build on `state`, don't invent
  storage" instruction. Tests that want isolation use a real
  `state.Store` backed by `t.TempDir()`, not a fake; only `Window` and
  `ScreenSource` are faked.
- The two Wails `events` packages collide by name: this package's own
  events are `github.com/jrschumacher/wails-kit/v2/events`, unaliased;
  Wails' `github.com/wailsapp/wails/v3/pkg/events` (for
  `WindowEventType`/`Common.WindowDidMove`) is aliased `wevents`
  everywhere. Keep that alias if you touch `window.go`/`manager.go`.
