# firstrun — agent notes

## Purpose

`firstrun` detects, on each launch, whether the app is being run for the
first time, upgraded from a recorded version, downgraded, or relaunched
unchanged, and runs ordered `Hook`s in response. It owns transition
detection and hook sequencing only. It does not own the migrations
themselves (apps write those), version *discovery* from a release feed
(that's `updates`), or comparison semantics (that's `semver` — `firstrun`
never reimplements version ordering).

## Public API (load-bearing signatures)

```go
type Kind string // Fresh | Upgrade | Downgrade | Same

type Info struct {
    Kind     Kind
    Previous semver.Version // zero value when Kind == Fresh
    Current  semver.Version
}

type Hook struct {
    Name string
    When func(Info) bool // nil means "always"
    Run  func(context.Context, Info) error
}
func OnFresh() func(Info) bool
func OnUpgrade() func(Info) bool
func OnDowngrade() func(Info) bool
func OnUpgradeThrough(version string) func(Info) bool // Previous < v <= Current; Kind==Upgrade only

type Service struct{ /* unexported */ }
func New(opts ...Option) (*Service, error)
func WithAppName(name string) Option
func WithDirs(d *appdirs.Dirs) Option
func WithStoragePath(path string) Option
func WithVersion(v string) Option        // required
func WithBaselineVersion(v string) Option
func WithEmitter(e *events.Emitter) Option
func WithHooks(hooks ...Hook) Option

func (s *Service) Detect() (Info, error) // read-only; no hooks, no writes
func (s *Service) Run(ctx context.Context) (Info, error)
func (s *Service) Reset() error // deletes the stamp; next Detect/Run is Fresh (or baseline-Upgrade)

const EventTransition = "firstrun:transition"
var ErrRefuse error // sentinel for OnDowngrade hooks that refuse to proceed
```

## Invariants (do not break)

- **Hooks run in registration order, filtered by `When`.** `Run` does not
  sort by any embedded version — a caller registering `OnUpgradeThrough`
  hooks out of ascending order gets them evaluated out of order. This is
  the mechanism behind "every intervening hook fires on a multi-version
  jump" (WP-20's primary requirement) and is easy to defeat by "cleaning
  up" the loop into something that sorts hooks — don't.
- **The stamp is written only after every matching hook succeeds.** A
  failing hook must leave the previous stamp untouched so the next `Run`
  retries the *entire* matching set, not just the failed hook — see
  `TestStampOnlyAfterSuccess` / `TestUpgradeStopsAtFirstFailure`. There is
  no per-hook "already ran" bookkeeping; hooks must be idempotent
  (documented loudly in the README) rather than this package tracking
  partial progress.
- **Never emit while holding `s.mu`.** `Run` releases the lock before
  calling `s.emit` — `state`, `settings`, and now `firstrun` all had or
  avoided this deadlock; see `TestRunEmitsTransitionAfterSuccess`, which
  registers a handler that calls back into `Detect` and would hang if emit
  held the lock.
- **Never run a `Hook.Run` (or the final `state.Store.Save`) while holding
  `s.mu` either.** This is a distinct instance of the same bug class, not
  covered by the emit invariant above: the old `runLocked` held `s.mu` for
  the entire hook loop, so a hook that called back into `Detect` or `Run`
  self-deadlocked (same goroutine re-entering a non-reentrant
  `sync.Mutex`; Go does not detect this, it just hangs). The fix
  (`detectAndSnapshotHooks`) takes `s.mu` only long enough to `detect()`
  and copy the hook slice, then releases it before `run` invokes anything
  external. See `TestHookReentryDoesNotDeadlock`.
- **`EventTransition` fires only after a fully successful `Run`.** A failed
  `Run` (hook error, save error) never emits — an app must be able to treat
  "I got `firstrun:transition`" as proof the run completed.
  `TestRunDoesNotEmitOnFailure`.
- **A missing stamp is not the same as `Fresh`.** `Detect` only reports
  `Fresh` when no stamp exists *and* no `WithBaselineVersion` was set. This
  is what makes adopting the package mid-life safe — see `TestAdoptedMidLife`
  and the README's "Adopting mid-life" section. Do not special-case an
  empty on-disk file as automatically `Fresh` without checking `s.baseline`.
- **`OnUpgradeThrough` never matches `Fresh`, `Same`, or `Downgrade`,**
  even when the version arithmetic would technically overlap — a fresh
  install has nothing to migrate *from*. Gate explicitly on
  `Kind == Upgrade`, don't rely on the comparison alone.
- **A corrupt stamp is a distinct outcome, not `Fresh`.** `detect()` calls
  `state.Store.LoadDetailed()`; when it reports `recovered == true` (the
  stamp file existed but failed to parse as JSON and `state` has already
  quarantined it), `detect()` returns `Kind == Same` with `Previous ==
  Current`, never `Fresh`. Collapsing corruption into `Fresh` would re-run
  every `OnFresh` onboarding hook for what is almost certainly an existing
  user — worse than doing nothing. Only unfiltered (`When == nil`) hooks
  run on this recovery pass; `Run` then writes a fresh valid stamp so
  recovery is a one-launch event. See `TestDetectRecoversFromCorruptStamp`,
  `TestRunRecoversFromCorruptStampWithoutRerunningOnboarding`. `Reset()` is
  the separate, explicit, user-triggerable counterpart (delete the stamp
  outright) — it deliberately *does* report `Fresh` next time, unlike
  automatic corruption recovery.

## Dependencies & insulation

- `semver` — all version parsing/comparison; `firstrun` never reimplements
  ordering.
- `state` — persists the version stamp (`state.Store[stamp]`, unexported
  type) at `{dataDir}/state/firstrun.json` by default. `stamp.Version == ""`
  is the sentinel for "no stamp recorded", relying on `state`'s documented
  behavior that a missing file and an empty struct are otherwise
  indistinguishable from `Load` alone (see `state/AGENTS.md` Landmines).
- `appdirs`, `events`, `errors`, `i18n` — standard AD-9 pattern.
- No `wails/v3` import — **not** on the AD-4 allowlist and must stay off it.
  `Service` runs identically in a CLI/TUI entry point.

## Extension points

- New hook filters follow the `func(Info) bool` shape — add alongside
  `OnFresh`/`OnUpgrade`/`OnDowngrade`/`OnUpgradeThrough`, don't change
  `Hook.When`'s type.
- A future `SettingsGroup()` (e.g. "show what's new") would follow the
  `health`/`i18n` pattern: unresolved `i18n.Text` fields, resolved
  centrally by `settings.Service`.

## Testing

- Doubles: `t.TempDir()` + `WithStoragePath` (see `newTestService` in
  `hooks_test.go`), `events.NewMemoryEmitter()` + `events.NewEmitter(...)`.
- `go test -race ./firstrun/` must cover: fresh install, unchanged version,
  single upgrade, a multi-version upgrade running every intervening hook in
  order (`TestHookOrder`), a hook failure stopping the sequence and leaving
  the stamp untouched (`TestUpgradeStopsAtFirstFailure`,
  `TestStampOnlyAfterSuccess`), downgrade with and without refusal
  (`TestDowngradeRefusal`, `TestDowngradeWithoutHookProceeds`), the
  adopted-mid-life baseline behavior (`TestAdoptedMidLife`), event
  emission/non-emission (`TestRunEmitsTransitionAfterSuccess`,
  `TestRunDoesNotEmitOnFailure`), corrupt-stamp recovery without re-running
  onboarding (`TestDetectRecoversFromCorruptStamp`,
  `TestRunRecoversFromCorruptStampWithoutRerunningOnboarding`), `Reset`
  (`TestReset`), and the hook-reentrancy deadlock regression, run with a
  real timeout so a regression fails instead of hanging the suite
  (`TestHookReentryDoesNotDeadlock`).
- Not automatable: none — this package is pure logic plus one JSON file, no
  OS/GUI signal to fake.

## File map

- `firstrun.go` — `Kind`, `Info`, event/payload types, error codes, `init()`,
  `ErrRefuse`.
- `hooks.go` — `Hook`, `OnFresh`/`OnUpgrade`/`OnDowngrade`/`OnUpgradeThrough`.
- `service.go` — `stamp`, `Service`, `Option`s, `New`, `Detect`, `Run`,
  `classify`.
- `locales/en.json` — this package's catalog (checked by `i18n`'s repo-wide
  `TestCatalogKeysExist`).

## Landmines

- **`OnUpgradeThrough` panics on an unparseable version string.** The
  boundary is normally a source literal (`OnUpgradeThrough("1.3.0")`), so a
  typo is a programmer error caught the moment the filter runs, not
  something that should silently never match — see
  `TestOnUpgradeThroughPanicsOnInvalidVersion`.
- **`Info.Previous` is the zero `semver.Version` for `Fresh`,** which
  `.String()`s as `"v0.0.0"` — don't display it directly; `TransitionPayload`
  handles this by emitting `Previous: ""` for `Fresh` specifically (see
  `Service.emit`), and any new consumer of `Info` should do the same rather
  than trusting `Previous.String()` to mean "no previous version".
- **`WithAppName`/`WithDirs`/`WithStoragePath` are mutually exclusive by
  last-write-wins**, not additive — each option clears the other two's
  fields. Passing more than one is not an error; only the last one in the
  option list takes effect.
