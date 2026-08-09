# kit — agent notes

## Purpose

`kit` is AD-1's wired-components constructor (`docs/v2-roadmap.md`): one
call, `kit.New`, composes `appdirs`, `logging`, `events`, `keyring`, `i18n`,
`settings`, `appearance`, `health`, `firstrun`, `updates`, and
`diagnostics`, in dependency order, into a `*Kit`. It owns *composition
order and defaults* — not any component's own logic, which stays in that
component's package. It does not own an `*application.App`, does not run
an event loop, and imports no Wails code — that's `kit/wailsbridge`'s job
(WP-31, not yet built as of this WP).

## Public API (load-bearing signatures)

```go
type AppInfo struct{ Name, ID, Version string }

type Kit struct {
    Info AppInfo
    Dirs *appdirs.Dirs
    Logger *slog.Logger
    Events *events.Emitter
    Keyring keyring.Store
    Settings *settings.Service
    I18n *i18n.Localizer
    Appearance *appearance.Service
    Health *health.Registry       // nil when WithoutHealth()
    Updates *updates.Service      // nil unless WithGitHubRepo() && !WithoutUpdates()
    FirstRun *firstrun.Service
    Diag *diagnostics.Service     // nil when WithoutDiagnostics()
    Lifecycle *lifecycle.Manager
}

func New(info AppInfo, opts ...Option) (*Kit, error)
func (k *Kit) Start(ctx context.Context) error
func (k *Kit) Close() error
func (k *Kit) SetEventsBackend(b events.Backend)       // kit/wailsbridge seam
func (k *Kit) SetAppearanceSource(s appearance.Source) // kit/wailsbridge seam
func GetComponent(err error) (string, bool)            // which component failed
```

## Invariants (do not break)

- **Wiring order is fixed and encoded in exactly one place** — `New` in
  `kit.go`, numbered comments `1.` through `11.`. Every dependency
  (`appdirs` before `logging`; `keyring`/`i18n` before `settings`;
  `firstrun` detected before `updates`/`diagnostics` constructed) exists
  because something later needs something earlier. Read the numbered
  comments before reordering anything.
- **Every construction failure is attributed via `componentErr`** — never
  `return nil, err` directly from a component's constructor; wrap with
  `componentErr("name", err)` so `GetComponent` can recover which piece
  failed. See `errors.go`.
- **`Kit` fields are not a facade.** Every field is the real
  `*settings.Service`, `*health.Registry`, etc. — never a kit-defined
  wrapper. A consumer must always be able to drop to the underlying
  package's own docs.
- **Every path New derives is rooted under the single resolved `k.Dirs`**,
  never a component's own independent `appdirs.New(name)` call — see
  Landmines for the one exception this package cannot close.
- **`settings.WithStoragePath` is used unconditionally, not `WithAppName`**
  — the latter derives its own fresh `appdirs.Dirs` internally, silently
  diverging from `k.Dirs` under a `WithDirs` override. The i18n peek
  (`settings_peek.go`) computes the same path and must keep reading it.

## Dependencies & insulation

- Imports (Wails-free, all on the AD-4 right column): `appdirs`, `logging`,
  `errors`, `events`, `settings`, `keyring`, `lifecycle`, `updates`,
  `diagnostics`, `i18n`, `appearance`, `health`, `firstrun`.
- **No `wails/v3` import, anywhere, ever** — enforced by
  `.github/scripts/check-wails-imports.sh` (kit is not on the allowlist) and
  `TestNoWailsImport` here (`go list -deps .` scoped to `kit` itself, not
  `kit/wailsbridge`).
- `kit/wailsbridge` (WP-31, separate package/directory) is the *only* place
  Wails glue belongs. It depends on this package; never the reverse.

## Extension points

- New kit-internal component: add a step to `New` in dependency order, wire
  its settings group (if any) into `settingsOpts` before app groups, add a
  `Without*`/`With*Options` pair to `options.go`, document the step in
  `kit.go`'s numbered list and `README.md`'s "What New wires" table.
- New lifecycle-managed background service: implement `lifecycle.Service`
  (see `lifecycle_services.go`'s `healthTicker`/`updatesTicker` for the
  "own context, cancel + wait on `OnShutdown`" pattern) and register it in
  step 11.

## Testing

- Doubles, all three used together via `baseOpts`/`newTestKit`
  (`kit_test.go`): `WithDirs(appdirs.New(name, appdirs.WithConfigDir(t.TempDir()), ...))`,
  `WithKeyring(keyring.NewMemoryStore())`,
  `WithHealthOptions(health.WithoutDefaultConnectivityCheck())`. Never
  construct a real `keyring.NewOSStore` or let the default health
  connectivity probe run in a test.
- `go test -race ./kit/...` must cover: default wiring succeeding
  (`TestNewDefaultWiring`); each `Without*` disabling its component
  (`TestWithout*`); a construction failure naming the responsible component
  (`TestConstructionFailureNames*Component`); `Start`/`Close` end-to-end
  including firstrun hooks and the health ticker
  (`TestStartRunsFirstRunAndLifecycle`); the events/appearance
  swap-after-construction seams (`events_test.go`,
  `appearance_source_test.go`); and `TestNoWailsImport`.
- Not automatable: real OS keychain behavior (`keyring`'s suite, not this
  package's), real Wails theme/event wiring (`kit/wailsbridge`'s, WP-31).

## File map

- `kit.go` — `AppInfo`, `Kit`, `New` (the numbered wiring order),
  `Start`, `Close`, `SetEventsBackend`, `SetAppearanceSource`.
- `options.go` — `Option`, `config`, every `With*`/`Without*` function.
- `errors.go` — `ErrComponentInit`, `componentErr`, `GetComponent`.
- `events.go` — `dynamicBackend`: the events seam `SetEventsBackend` retargets.
- `appearance_source.go` — `dynamicSource`: the appearance seam
  `SetAppearanceSource` retargets. Read this file's doc comment before
  touching appearance wiring — the darwin-default trade-off is explained
  there, not repeated elsewhere.
- `settings_peek.go` — `settingsPeek`: reads a persisted locale override
  before the real `*settings.Service` exists (breaks the i18n/settings
  construction cycle — see kit.go step 5's comment).
- `lifecycle_services.go` — `healthTicker`, `updatesTicker`: the
  `lifecycle.Service` adapters registered in step 11.

## Landmines

- **`updates.Service.appDirs()` always calls `appdirs.New(appName)`
  internally** — it takes no `*appdirs.Dirs`, only a name. A `WithDirs`
  override redirects everything else, but the update staging directory
  (used only inside `DownloadUpdate`) still resolves against the *real* OS
  cache dir. Not fixable from here — `updates` would need a
  `WithDirs`-equivalent option. Harmless for this package's tests (none
  call `DownloadUpdate`); real for a production consumer combining a
  custom `Dirs` with enabled updates.
- **`kit.New` does not use `appearance`'s darwin-native default Source**,
  even on darwin. Every platform gets light-until-wired via `dynamicSource`
  uniformly, trading a macOS-only nicety for a `SetAppearanceSource` seam
  that actually works for `kit/wailsbridge` (WP-31) — `appearance` exposes
  no post-construction Source setter and its darwin constructor is
  unexported, so there was no way to keep both. A consumer wanting real
  darwin detection without attaching can pass `WithAppearanceSource` with
  a Source built the way `appearance/os_source_darwin.go` does.
- **`logging.Init` and `errors.SetLocalizer` are process-global**, not
  per-`Kit`. Two `kit.New` calls in one process share one log destination
  and localizer; the second call wins. Fine for one `Kit` per process; a
  landmine for a test suite building many and asserting on either.
- **`WithHealthCheck`/`WithHealthOptions`/`WithFirstRunHook` silently no-op
  when their component is disabled** — consistent with `updates.WithHealth(nil)`,
  easy to forget when debugging "why didn't my check register".
