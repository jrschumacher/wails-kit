# i18n — agent notes

## Purpose

`i18n` is the kit's single translation catalog, owned in Go so Prune's Cobra CLI and
Bubbletea TUI can localize output too — a frontend-only catalog can't reach either
(AD-5, `docs/v2-roadmap.md`). It owns: `Text`/`T` (key+fallback declaration), catalog
loading/merging from `fs.FS` sources, locale resolution (OQ-5's five-tier order), CLDR
plural selection via `golang.org/x/text`, and the frontend-safe `Binding`. It does
**not** own: wiring itself into `errors`/`settings` (WP-11/WP-12), the TypeScript
plural resolver `@wails-kit/i18n` (WP-33), or OS-level locale registration beyond
reading it (no writing/setting the OS locale).

## Public API (load-bearing signatures)

```go
type Text struct{ Key, Other string }
func T(key, other string) Text

func New(opts ...Option) (*Localizer, error)
func WithCatalog(fsys fs.FS) Option
func WithLocale(tag string) Option
func WithDefault(tag string) Option
func WithSettings(src SettingsSource) Option // SettingsSource: GetValues() (map[string]any, error)
func WithEmitter(e *events.Emitter) Option

func (l *Localizer) Locale() string
func (l *Localizer) SetLocale(tag string) error          // emits EventChanged, transition-only
func (l *Localizer) T(t Text, args ...any) string
func (l *Localizer) TN(t Text, n int, args ...any) string // n is always the first fmt arg
func (l *Localizer) Catalog() map[string]any
func (l *Localizer) Binding() *Binding                    // GetCatalog/GetLocale/SetLocale
func (l *Localizer) SettingsGroup() settings.Group        // locale picker
```

## Invariants (do not break)

- **Resolution runs once, at `New`, and is cached.** `osLocales()` (the OS source) and
  `WithSettings`'s `GetValues()` are each called exactly once. Never call either from
  `T`/`TN`/`Locale`/`Catalog` — those must be cheap, lock-only reads. `SetLocale` is
  the *only* post-construction way to change `l.locale`.
- **`osLocales` is a package-level func var, never called directly by a test.** Tests
  substitute it (`withOSLocales`/`withOSLocalesFunc` in `resolve_test.go`) so no test
  in this package shells out. If you add a new OS-source platform, wire it through this
  var in a `//go:build <os>` file, not inline in `resolve.go`.
- **Resolution order is exactly OQ-5's, and it's load-bearing for WP-11/12/13/15.**
  explicit > settings (skip `"system"`) > `LC_ALL`/`LC_MESSAGES`/`LANG` > OS source >
  default. Don't reorder tiers or add new ones without checking `resolve_test.go`'s
  `TestLocaleResolutionOrder` subtests, which pin each transition individually.
- **`Locale()` reports what was detected, not what has a catalog.** Tiers 1-3 accept
  any tag that parses as BCP-47, even with no matching catalog (see
  `TestMissingLocaleFallsBackGracefully`). Only tier 4 (OS source, an *ordered
  preference list*) runs through `language.Matcher` — that's what an ordered list is
  for. Catalog lookup fallback (`lookupCandidates`) is a separate concern, applied at
  `T`/`TN`/`Catalog` call time, not at resolution time.
- **Kit catalog merges first; app catalogs win on collision.** `New` merges
  `kitLocalesFS` before any `WithCatalog` option runs. Don't reorder this — it's the
  literal mechanism behind AD-5's "app wins on key collision."
- **A plural entry always has `"other"`.** `catalogEntry.UnmarshalJSON` rejects a
  plural object missing it, or with an unrecognized category name, at catalog-load
  time (`New` returns an error) — not silently at first lookup.
- **`T` with zero args returns the format string unchanged**, never through
  `fmt.Sprintf`. A message containing a literal `%` (e.g. "100% off") must not become
  `%!s(MISSING)`. `TN` always calls `Sprintf` (n is always at least one arg).
- **`SetLocale` emits only on an actual transition**, matching the kit's other
  `*:changed` events (e.g. `health`). Setting the same locale twice emits once, not
  twice.

## Dependencies & insulation

- `golang.org/x/text` (`language`, `feature/plural`, `language/display`) for tag
  parsing/matching and CLDR plural rules — never hand-roll plural logic here.
- `github.com/jrschumacher/wails-kit/v2/events` for `*events.Emitter` (optional,
  nil-safe). `github.com/jrschumacher/wails-kit/v2/settings` for `SettingsGroup`'s
  return type only — `SettingsSource` is a narrow local interface, not an import of
  `*settings.Service`, so this package doesn't couple to `settings`'s internals.
- No `wails/v3` import — Wails-free per the AD-4 allowlist. `Binding` registration
  happens in the consuming app or `kit/wailsbridge` (WP-31).

## Extension points

- New OS-source platform: add `os_locale_<goos>.go` with `//go:build <goos>` defining
  `var osLocales = ...`, plus a same-tag `_test.go` for its pure-parsing half (see
  `os_locale_darwin.go` / `os_locale_darwin_test.go`).
- New plural category or CLDR edge case: `golang.org/x/text` owns the rule tables;
  this package only maps category *names* (`catalog.go`'s `pluralForms`) — there is
  nothing to extend here for a new language, only for a new *category name spelling*.
- Wiring into `errors`/`settings` (WP-11/12): both should treat `Localizer` as already
  frozen API — extend by consuming `T`/`Catalog`/`WithSettings`, not by reaching into
  unexported fields.

## Testing

- Doubles: `testing/fstest.MapFS` for catalog sources, `t.Setenv` for env vars,
  `withOSLocales`/`withOSLocalesFunc` for the OS-source tier, a local `fakeSettings`
  (`map[string]any`) for `SettingsSource`, `events.NewMemoryEmitter()` for emission
  assertions. Every test that touches env vars must call `clearLocaleEnv(t)` first —
  the host running `go test` may have `LANG` set, and ambient env leakage is exactly
  the bug `TestLocaleResolutionOrder`'s first failed draft hit.
- `go test -race ./i18n/` must cover (and does): the full resolution order tier by
  tier, CLDR plural selection for Polish (`few`/`many` — English can't exercise
  those), missing-key fallback, missing-locale fallback, catalog merge precedence, the
  substituted OS source, `SetLocale` transition-only emission, and the extraction
  lint (`TestCatalogKeysExist`).
- Not automatable here: a real `defaults read -g AppleLanguages` against a live macOS
  session with a non-English language configured — `TestParseAppleLanguages` covers
  the parsing logic against fixed sample output instead.

## File map

- `i18n.go` — `Text`, `T`.
- `localizer.go` — `Localizer`, `New`, `Locale`, `SetLocale`, `T`, `Catalog`, `lookup`.
- `resolve.go` — `resolveInitial` (the five-tier order), env var parsing,
  `lookupCandidates` (catalog fallback chain).
- `plural.go` — `TN`.
- `catalog.go` — `catalogEntry` (+ JSON unmarshal), `mergeFS`, embedded `kitLocalesFS`.
- `option.go` — `Option`s, `SettingsSource`.
- `binding.go` — `Binding`.
- `settings.go` — `SettingsGroup`, `localeDisplayName`.
- `os_locale_darwin.go` / `os_locale_other.go` — `osLocales` per platform.
- `locales/en.json` — the kit's own catalog (currently one key: the locale picker's
  "System default" option label).

## Landmines

- Tiers 1-3 do **not** run through the language matcher — only tier 4 does. This is
  intentional (see Invariants), but it means `Locale()` after `WithLocale("de-DE")` is
  literally `"de-DE"`, not canonicalized to `"de"` even if only a `"de"` catalog
  exists. Don't "fix" this without re-reading OQ-5 — `lookupCandidates` already
  handles the fallback at lookup time.
- `go.work`'s workspace build list already resolves `golang.org/x/text` (via the
  `settings/templates/anyllm` nested module's transitive deps), so `go build` works
  without a `golang.org/x/text` line in the root `go.mod`. That's real but incidental
  — don't rely on it staying true; root `go.mod`/`go.work` are owned by WP-01/WP-07,
  not this package.
