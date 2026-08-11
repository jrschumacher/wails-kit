# firstrun — Version Transition Detection

Detects, on each app launch, whether this is a fresh install, an upgrade
from a previously-recorded version, a downgrade, or an unchanged relaunch —
and runs ordered hooks in response. Every app that ships more than once
needs this to run data migrations, show a what's-new screen, or refuse to
open data written by a newer version; `firstrun` exists so you don't
hand-roll the same ~80 lines and get the ordering wrong.

## Quickstart

```go
import (
    "context"

    "github.com/jrschumacher/wails-kit/v2/firstrun"
)

fr, err := firstrun.New(
    firstrun.WithAppName("my-app"),
    firstrun.WithVersion("1.4.0"),
    firstrun.WithHooks(
        // Fires once, the very first time the app ever runs.
        firstrun.Hook{
            Name: "welcome",
            When: firstrun.OnFresh(),
            Run: func(ctx context.Context, info firstrun.Info) error {
                return showWelcomeScreen(ctx)
            },
        },
        // Fires for every upgrade whose (previous, current] range crosses
        // 1.3.0 — register one hook per version boundary, in ascending
        // order, and a user jumping 1.0.0 -> 1.4.0 runs all of them in
        // that order, not just the last one.
        firstrun.Hook{
            Name: "migrate-workspace-layout",
            When: firstrun.OnUpgradeThrough("1.3.0"),
            Run: func(ctx context.Context, info firstrun.Info) error {
                return migrateWorkspaceLayout(ctx)
            },
        },
        // Downgrade is not an error by default — decide what "unsafe" means
        // for your app's data.
        firstrun.Hook{
            Name: "refuse-downgrade",
            When: firstrun.OnDowngrade(),
            Run: func(ctx context.Context, info firstrun.Info) error {
                return firstrun.ErrRefuse
            },
        },
    ),
)
if err != nil {
    log.Fatal(err)
}

result, err := fr.Run(context.Background())
if err != nil {
    // A hook failed (or refused, e.g. ErrRefuse). The version stamp was
    // NOT updated — surface this to the user and/or retry next launch.
    log.Fatal(err)
}
fmt.Println(result.Kind, result.Previous, result.Current)
```

## How detection works

`firstrun` persists the last version a successful `Run` completed for
(a "stamp") via the `state` package, in the app's data directory. On each
call:

1. `Detect` (called internally by `Run`, or callable standalone) loads the
   stamp and classifies the transition into a `Kind`:
   - `Fresh` — no stamp was ever recorded (see "Adopting mid-life" below).
   - `Upgrade` — the current version is newer than the recorded one.
   - `Downgrade` — the current version is older than the recorded one.
   - `Same` — the current version matches the recorded one exactly.
2. `Run` evaluates every registered `Hook`, **in registration order**,
   running it if `Hook.When(info)` returns `true` (or `When` is `nil`,
   meaning "always run").
3. Only if every matching hook succeeds does `Run` update the stamp to the
   current version and emit `firstrun:transition`.

Version comparison (including prerelease ordering, e.g. `2.0.0-beta.1` <
`2.0.0`) comes from the `semver` package — `firstrun` does not reimplement
it.

## Multi-version upgrades run every intervening hook

This is the single most valuable (and easiest to get wrong) behavior here.
A user who skips several releases and upgrades directly from `1.0.0` to
`1.4.0` must run the `1.1.0`, `1.2.0`, `1.3.0`, and `1.4.0` migrations in
that order — not just whatever the latest release happens to need.

`OnUpgradeThrough(version)` returns a filter matching `Previous < version
<= Current`. Register one `Hook` per version boundary that ever needed a
migration, **in ascending version order** — hooks run in the order you
register them, not sorted by their boundary, so registering them out of
order runs them out of order. This is your responsibility; the package
cannot infer intent from version numbers alone.

## Partial failure: hooks must be idempotent

If a hook fails partway through a multi-hook upgrade, `Run` stops
immediately and does **not** update the stamp. The next launch re-detects
the *same* transition and re-runs **every** matching hook from the start —
including ones that already succeeded this run. There is no per-hook
"already done" tracking.

This is a deliberate choice: silently skipping hooks that already ran
would require persisting partial progress, which adds a second piece of
state that can itself get out of sync with the data it's meant to protect.
Instead, every hook you register must be safe to run more than once against
data it may have already migrated (check before you migrate, or make the
migration itself a no-op on already-migrated data).

## Downgrade is not an error by default

Reinstalling an older build, or a rollback, is a real scenario — not
automatically a failure. With no `OnDowngrade` hook registered, `Run`
proceeds normally and records the (lower) current version as the new
stamp.

If your app's on-disk data written by a newer version may be unreadable by
an older one, register a hook to decide what happens:

```go
firstrun.Hook{
    Name: "refuse-downgrade",
    When: firstrun.OnDowngrade(),
    Run: func(ctx context.Context, info firstrun.Info) error {
        if !dataFormatIsBackwardCompatible(info) {
            return firstrun.ErrRefuse
        }
        return warnUserAboutDowngrade(ctx)
    },
}
```

`ErrRefuse` is a ready-made sentinel; `Run` doesn't treat it specially, so
returning any other error from a downgrade hook refuses just as cleanly.
`errors.Is(err, firstrun.ErrRefuse)` still works on the error `Run`
returns — it wraps the hook's error rather than replacing it.

## Adopting `firstrun` mid-life

An app that adds `firstrun` after already shipping has existing users with
no recorded stamp. **Those users are not fresh installs** — without help,
`Detect` cannot tell them apart from someone installing for the very first
time, and will report `Fresh` for both, incorrectly re-running onboarding
and skipping every upgrade hook.

Fix this with `WithBaselineVersion`, set once to the last version you
shipped before adopting `firstrun`:

```go
firstrun.New(
    firstrun.WithAppName("my-app"),
    firstrun.WithVersion("1.5.0"),
    firstrun.WithBaselineVersion("1.2.0"), // last release before firstrun
    // ...
)
```

With a baseline set, a missing stamp is treated as if the last successful
run was `baseline` — `Detect` reports `Upgrade` (or `Same`, if `baseline`
equals the app's current version) instead of `Fresh`, and
`OnUpgradeThrough` hooks between `baseline` and `Current` fire exactly as
they would for a user who really was on `baseline`.

Pick `baseline` once and never change it — it's a fixed historical marker,
not something to bump per release. **If you skip this option**, every
pre-adoption install is silently treated as `Fresh` the first time it runs
with `firstrun` — document that loudly if you choose to accept it.

## Recovering from a corrupt stamp

The stamp file can end up corrupt (disk full mid-write on an older build,
hand-editing, etc.). `Detect`/`Run` never fail forever because of it: the
underlying `state.Store` quarantines the corrupt file (renamed aside with a
`.corrupt-<unix-nano>` suffix, preserving the original bytes) and reports it
as recovered. `firstrun` treats that recovery as its own outcome — `Kind ==
Same`, with `Previous == Current` — rather than as `Fresh`: reporting
`Fresh` would re-run every `OnFresh` onboarding hook for what is almost
certainly an existing user, which is worse than doing nothing. Hooks with no
`When` filter still run; `OnFresh`/`OnUpgrade`/`OnDowngrade`-gated hooks do
not. `Run` then writes a fresh, valid stamp at the current version, so
recovery is a one-launch event — the next launch behaves normally.

For an explicit, user-triggerable reset (e.g. a "repair my installation"
action) rather than relying on automatic recovery, call `Service.Reset()`,
which deletes the stamp so the next `Detect`/`Run` treats the install as
`Fresh` (or `Upgrade` from `WithBaselineVersion`, if set) — including
re-running `OnFresh` hooks.

## Options

| Option | Description |
|--------|-------------|
| `WithAppName(name)` | Storage location via `appdirs.New(name)`; stamp lives at `{dataDir}/state/firstrun.json`. |
| `WithDirs(d)` | Storage location from an already-constructed `*appdirs.Dirs`. |
| `WithStoragePath(path)` | Exact file path override — primarily for tests (`t.TempDir()`). |
| `WithVersion(v)` | **Required.** The app's current version (semver string). |
| `WithBaselineVersion(v)` | See "Adopting mid-life" above. |
| `WithEmitter(e)` | Optional `*events.Emitter`; enables `firstrun:transition`. |
| `WithHooks(...)` | Appends hooks, in call order. Safe to call more than once. |

`New` requires `WithVersion` and exactly one storage option
(`WithAppName`, `WithDirs`, or `WithStoragePath`); it returns
`firstrun_config` otherwise.

## Events

| Event | Payload | Description |
|-------|---------|--------------|
| `firstrun:transition` | `TransitionPayload{Kind, Previous, Current}` | Emitted once, after a `Run` whose hooks all succeeded and whose stamp was updated. Not emitted on failure. `Previous` is `""` for `Fresh`. |

## Error codes

| Code | User message |
|------|--------------|
| `firstrun_config` | First-run detection is misconfigured. Please contact support. |
| `firstrun_stamp_invalid` | The recorded application version could not be read. |
| `firstrun_hook_failed` | An update step failed to complete. Please try relaunching the app. |
| `firstrun_downgrade_refused` | This version can't open data saved by a newer version of the app. (`ErrRefuse`'s code) |

## Hook filter helpers

| Helper | Matches |
|--------|---------|
| `OnFresh()` | `Kind == Fresh` |
| `OnUpgrade()` | `Kind == Upgrade` (any magnitude) |
| `OnDowngrade()` | `Kind == Downgrade` |
| `OnUpgradeThrough(v)` | `Kind == Upgrade` and `Previous < v <= Current` |

A `Hook` with `When == nil` runs unconditionally, for every `Kind`
including `Same` — most hooks should use one of the helpers above instead.
