# state — agent notes

## Purpose

`state` provides generic typed JSON persistence to disk for a single Go
struct — the gap between no persistence and the full `settings` package.
It owns atomic load/save/delete for one file per `Store[T]` and optional
change events; it does not own schema, validation, or secrets (that's
`settings`), and it does not own multi-key stores or migrations.

## Public API (load-bearing signatures)

```go
type Store[T any] struct{ /* unexported */ }

func New[T any](opts ...Option[T]) (*Store[T], error)
// Returns ErrStateConfig if neither WithAppName nor WithStoragePath is set.

func WithAppName[T any](appName string) Option[T]     // {dataDir}/state/{name}.json
func WithName[T any](name string) Option[T]           // file base name; order-independent with WithAppName
func WithStoragePath[T any](path string) Option[T]    // exact path override
func WithEmitter[T any](e *events.Emitter) Option[T]
func WithDefaults[T any](defaults T) Option[T]

func (s *Store[T]) Load() (T, error)  // returns defaults/zero value if file absent OR corrupt (quarantined)
func (s *Store[T]) LoadDetailed() (value T, recovered bool, err error) // Load + recovered=true iff this call just quarantined a corrupt file
func (s *Store[T]) Save(value T) error // atomic + durable: write-to-tmp + fsync + rename
func (s *Store[T]) Delete() error
func (s *Store[T]) Path() string

const StateLoaded    = "state:loaded"
const StateSaved     = "state:saved"
const StateCorrupted = "state:corrupted" // fires when Load/LoadDetailed quarantines a corrupt file
```

## Invariants (do not break)

- **Never emit `state:loaded`/`state:saved` while holding `s.mu`.** This is
  the WP-06 fix for a real deadlock: `events.Emitter` runs handlers
  *synchronously* by default (see `events` package docs), so a handler that
  calls `store.Load()` in response to `state:saved` would block forever on
  `s.mu` if `Save` still held it — `sync.RWMutex` is not re-entrant. Both
  `Load` and `Save` snapshot whatever fields they need (`path`, `name`,
  defaults) under the lock, release it, *then* call `s.emit`. Preserve this
  shape for any new emission point. `TestEmitOutsideLock` is the regression
  test — it registers a handler that calls back into the store and fails by
  timing out (not by a Go deadlock detector panic, since the deadlock is on
  a mutex a *different* logical caller — the handler — is blocked on) if
  the lock is held during emit.
- **`New` requires `WithAppName` or `WithStoragePath`.** Without this, a
  misconfigured `Store` used to fail silently: `Load()` on an empty path
  returns `os.ErrNotExist` → defaults, so it looks like it's "working" with
  no file, and `Save()` fails later with a confusing `os.Rename` error on
  `.tmp` disconnected from the missing option. `New` now returns
  `ErrStateConfig` immediately instead.
- **Writes are atomic and durable**: temp file (`0600`) → `Write` → `Sync`
  → `Close` → `os.Rename`, mirroring `settings.Store.Save`'s
  `writeTempFile`/`createTempFile` shape exactly (including the same
  test-injection seam for asserting `Sync` is actually called —
  `TestSaveDurability`). Don't write directly to `s.path`, and don't drop
  the `Sync` before `Rename`: a rename that lands before the data does can
  leave a zero-length or truncated file as the rename's winner after a
  crash.
- **A corrupt file is quarantined, not fatal.** `Load`/`LoadDetailed`
  finding a file that fails to parse as JSON renames it aside
  (`<path>.corrupt-<unix-nano>`, preserving the bytes) under a re-verify
  pass taken with the write lock (guards against racing a concurrent
  `Save` that just repaired the file), then returns defaults with `err ==
  nil` — never an error, and never the same failure forever. `LoadDetailed`
  additionally reports `recovered == true` for that call so a caller that
  must distinguish "no file" from "corrupt file, now reset" (firstrun is
  the motivating case) can. See `TestLoadCorruptedFile`,
  `TestLoadDetailedDistinguishesCorruptionFromMissing`.
- **State directory is `0700`**, matching the kit's convention for
  user-content directories (mirrors `logging`, `keyring`, `database`).

## Dependencies & insulation

- `appdirs` — resolves `{dataDir}/state/` when `WithAppName` is used.
- `errors` (kit package) — every failure is an `errors.Code`-tagged
  `*errors.UserError` (`ErrStateLoad`, `ErrStateSave`, `ErrStateConfig`),
  registered via `RegisterMessages` in `init()`.
- `events` — optional (`WithEmitter`); nil emitter is a no-op via `s.emit`.
  No `wails/v3` import; this package is Wails-free per the AD-4 allowlist
  and must stay that way — it's designed to back `windowstate` (Phase 1,
  not yet built), which is itself Wails-importing, but `state` underneath
  it is not.

## Extension points

- New construction knobs go through `Option[T]`, following `WithDefaults`'s
  pattern — don't add non-functional-option constructor parameters.
- A multi-key variant (many named states sharing one directory) would be a
  new type, not an expansion of `Store[T]`'s single-file model — keep this
  package's "one struct, one file" contract simple.

## Testing

- Doubles: `t.TempDir()` + `WithStoragePath`, `events.NewMemoryEmitter()` +
  `events.NewEmitter(...)`.
- `go test -race ./state/` must cover: the deadlock regression
  (`TestEmitOutsideLock`, using real goroutines + a timeout, not just a
  sequential call — a sequential reentrant call from the *same* goroutine
  that holds a non-blocking-checked lock wouldn't reliably surface the
  defect the way a genuine handler callback does), `New`'s config
  validation (`TestNewRequiresPathOption`), atomic-write cleanup
  (`TestSaveIsAtomic`), and `WithName`/`WithAppName` order-independence
  (`TestWithNameOrderIndependent`).
- Not automatable: real multi-process file contention (this package assumes
  single-process, mutex-only concurrency control — no file locking).

## File map

- `state.go` — `Store[T]`, all `Option[T]` functions, `New`, `Load`,
  `Save`, `Delete`, `Path`, error codes, event names/payloads.
- `state_test.go` — full test suite.
- `README.md` — quickstart, options table, storage paths per OS, events,
  error codes.

## Landmines

- `WithName` re-derives the path from whatever `s.path` currently is
  (`filepath.Dir(s.path)` + new name), so it works whether called before or
  after `WithAppName`/`WithStoragePath` in the option list — but combining
  `WithStoragePath` (exact path) with a later `WithName` still rewrites the
  filename portion of that exact path, which may surprise a caller who
  expected `WithStoragePath` to be the final word. This is pre-existing
  behavior, not changed by WP-06 — just easy to trip over.
- `Load`'s "file does not exist" branch returns `s.defaultValue()`, *not*
  an error — this is intentional (first run always looks like "loaded
  defaults"), but means a genuinely missing file and an intentionally-fresh
  store are indistinguishable from `Load`'s return value alone.
