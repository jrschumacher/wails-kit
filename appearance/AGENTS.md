# appearance — agent notes

## Purpose

`appearance` is the single resolved source of truth for light/dark theme:
an OS signal (`Source`), an optional persisted user override (via
`settings`), and their merge into one `Theme` (`Resolved()`). It owns the
resolution rule and change notification (`appearance:changed`). It does
**not** own how the Theme gets applied (CSS class/vars, that's the
frontend's job) or live OS-flip detection on any platform except the
no-Wails darwin default (a real live source across all platforms is
`kit/wailsbridge`'s job, WP-31, not built here).

## Public API (load-bearing signatures)

```go
type Mode string   // "system" | "light" | "dark" — user preference
type Theme string  // "light" | "dark" — resolved truth, never "system"

type Source interface {
    IsDark() bool
    Subscribe(fn func(dark bool)) (cancel func())
}

func NewService(opts ...Option) *Service
func WithSource(s Source) Option       // nil source -> light; omit -> platform default
func WithSettings(svc *settings.Service) Option
func WithEmitter(e *events.Emitter) Option

func (s *Service) Mode() Mode
func (s *Service) Resolved() Theme
func (s *Service) SetMode(m Mode) error
func (s *Service) Refresh() // re-resolve + emit-if-changed; see Landmines
func (s *Service) Close()
func (s *Service) Binding() *Binding

const SettingMode = "appearance.mode"
func SettingsGroup() settings.Group // theme picker; i18n.Text labels (WP-12)
const EventChanged = "appearance:changed" // ChangedPayload{Mode, Resolved}
```

## Invariants (do not break)

- **Never emit `appearance:changed` while holding `s.mu`.** Two kit packages
  (`state`, `settings`) shipped this exact deadlock before being fixed in
  Phase 0. `resolveAndMaybeEmit` (`resolve.go`) snapshots `changed` under
  the lock, releases it, then calls `emitChanged`. Keep that order in any
  new call path that can emit.
- **Emission is gated on `Resolved()` changing, not on `Mode` changing.**
  `SetMode(ModeDark)` while the OS is already dark under `ModeSystem` is a
  real mode change but must not emit — `lastResolved` is the only thing
  compared. `TestNoEventWhenResolvedUnchanged` pins this.
- **`Mode()`/`currentMode()` read the wired `settings.Service` live, never
  cached**, matching `docs/settings-integration.md`. `s.mode` is only the
  in-memory fallback for when no `settings.Service` is wired, or its read
  fails/holds an unrecognized value.
- **The darwin default `Source` resolves once, at construction, and never
  re-execs.** `TestDarwinSource_CachesAtConstruction` pins this. It exists
  because there's no event loop to poll without Wails — don't "fix" this
  with polling; that's `kit/wailsbridge`'s live source, not this one.
- **No test ever shells out.** `readAppleInterfaceStyle` (darwin-only) is a
  substitutable package-level `var`, exactly `i18n/os_locale_darwin.go`'s
  pattern. Tests substitute it and restore the original via `t.Cleanup`.
- **`SettingsGroup()` builds `i18n.Text`, it does not resolve it.** Text ->
  string resolution is `settings.Service.GetSchema`'s job (WP-12). Don't add
  a `*i18n.Localizer` field/option here to do that resolution a second time.

## Dependencies & insulation

- No `wails/v3` import — not on the AD-4 allowlist by design (see README's
  "why appearance is Wails-free"). `check-wails-imports.sh` must stay green
  on this package.
- `settings` (`*settings.Service`, `settings.Group`/`Field`/`SelectOption`),
  `events` (`*events.Emitter`, nil-safe), `errors` (`Code` +
  `RegisterMessages` in `init()`), `i18n` (`i18n.T` literals only — no
  `*i18n.Localizer` dependency; see above).

## Extension points

- A live `Source` (any platform) plugs in via `WithSource` with zero changes
  here — that's the entire point of the interface. `kit/wailsbridge` is
  expected to supply one wrapping Wails' `ThemeChanged` event.
- New `Option`s follow the `config` staging pattern in `appearance.go`
  (`WithSource`'s `sourceSet` flag is the one subtlety: distinguish "not
  passed" from "passed nil" so an explicit opt-out doesn't get silently
  replaced by the platform default).

## Testing

- Doubles: `fakeSource` (`fake_test.go`) — a controllable `Source` with a
  `flip(dark bool)` helper that synchronously notifies subscribers,
  simulating a live OS change. Settings: `newTestSettings` in
  `appearance_test.go` always passes `WithStoragePath(t.TempDir()/...)` —
  never call `settings.NewService` without it in a test that calls
  `SetValues` (see `settings/AGENTS.md`'s landmine).
- `go test -race ./appearance/` must cover: all three `Mode` states
  (`TestResolveTheme`); override precedence over the OS
  (`TestResolveOverridesSystem`); exactly one event on an OS flip while on
  follow-system (`TestSystemFlipEmits`); no event when resolved doesn't
  change (`TestNoEventWhenResolvedUnchanged`, three subcases); settings
  persistence + live read-back including cross-Service and
  bypass-`SetMode` cases (`TestPersistViaSettings`); the `Refresh` wiring
  (`TestRefreshEmitsOnExternalSettingsChange`); `Close`'s unsubscribe;
  concurrent access (`TestConcurrentAccess`).
- **Not automatable here**: a real OS theme change on a live session — see
  README's last section. The darwin-only tests
  (`os_source_darwin_test.go`, build-tag gated) cover `IsDark` parsing and
  the cache-at-construction/inert-Subscribe behavior against substituted
  output, never a real `defaults` invocation.

## File map

- `appearance.go` — `Mode`/`Theme`, error codes + `init()`, event
  name/payload, `Option`/`config`, `Service` struct, `NewService`, `Close`,
  `Mode`/`Resolved`/`SetMode`/`Refresh`.
- `resolve.go` — pure `resolveTheme`, `Service.currentMode`,
  `resolveAndMaybeEmit` (the one place that decides whether to emit),
  `onSourceChange`, `emitChanged`.
- `source.go` — the `Source` interface.
- `os_source_darwin.go` / `os_source_other.go` — platform default `Source`
  (build-tag split, mirrors `i18n/os_locale_*.go`).
- `binding.go` — `Binding`, the frontend-safe registration surface.
- `settings.go` — `SettingMode`, `SettingsGroup`, the `i18n.Text` catalog
  vars.
- `locales/en.json` — catalog entries for every `wailskit.appearance.*` key
  used in `settings.go` (checked by `i18n`'s repo-wide
  `TestCatalogKeysExist`).
- `*_test.go` — see Testing above.

## Landmines

- `Refresh()` closes a real composition gap: `settings.Service` has no way
  to notify a dependent package after construction other than
  `settings.WithOnChange`, registered at `settings.NewService` time. Wiring
  it requires the forward-declared-variable trick shown in the README and
  `examples/appearance/main.go` — don't "simplify" that example by removing
  the indirection; it's there because the construction order is genuinely
  circular (settings needs the callback before appearance exists to build
  the callback's target).
- `settings.Service` gaining a post-construction "add a change listener"
  method would let this package expose live push notification directly
  instead of via `Refresh()` — flagged in this WP's final report as a
  cross-cutting suggestion for whoever owns `settings/` next, not something
  to build here.
- `onSourceChange`'s `dark bool` parameter is intentionally unused — it
  re-derives the answer via `resolveTheme` -> `source.IsDark()` rather than
  trusting the callback argument, so a `Source` implementation can't cause a
  stale emit by passing a value inconsistent with its own `IsDark()`.
