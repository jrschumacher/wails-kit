# settings/cli — agent notes

## Purpose

`settings/cli` is a headless/CLI adapter over `*settings.Service` (or
anything satisfying `SettingsProvider`): `Show`/`Get`/`Set` render and edit
settings in a terminal, using the same resolved schema that drives the
Wails frontend. It owns terminal rendering and its own human-facing prose
(headers, placeholders, error text) and their localization. It does **not**
own schema label resolution (that's `settings.Service.GetSchema`, driven by
`settings.WithLocalizer` — a separate localizer instance from this
package's own `WithLocalizer`), validation logic (`settings.Validate`), or
persistence (`settings.Service.SetValues`/store).

## Public API (load-bearing signatures)

```go
type SettingsProvider interface {
    GetSchema() settings.ResolvedSchema
    GetValues() (map[string]any, error)
    SetValues(values map[string]any) ([]settings.ValidationError, error)
}

func Show(svc SettingsProvider, opts ...Option) error
func Get(svc SettingsProvider, key string, opts ...Option) (string, error)
func Set(svc SettingsProvider, key, value string, opts ...Option) error

func WithOutput(w io.Writer) Option
func WithLocalizer(l *i18n.Localizer) Option

type ValidationErrors struct{ Errors []settings.ValidationError }
```

## Invariants (do not break)

- **`SettingsProvider.GetSchema` returns `settings.ResolvedSchema`, not
  `settings.Schema`.** Labels arrive as plain, already-localized strings —
  this package never resolves an `i18n.Text` itself for schema content. If
  a future change makes this package need `i18n.Text` from the schema
  again, that's a sign `settings.WithLocalizer` isn't being used correctly
  upstream, not a reason to add resolution logic here.
- **`WithLocalizer` only affects this package's own strings** — the
  `"(not set)"` placeholder and `Show`/`Get`/`Set` error text. It must never
  change what `SettingsProvider.GetSchema()`/`GetValues()`/`SetValues()`
  return; this package only formats and passes through that data.
- **Toggle/number tokens are never localized.** `formatValue`'s
  `"true"`/`"false"` output and `coerceValue`'s accepted input alphabet
  (`"true"`/`"1"`/`"yes"`/`"y"`/`"on"`, and their false-ish counterparts)
  are fixed regardless of the configured locale — see
  `TestMachineValuesUnaffectedByLocalizer`. This is what lets `Get`'s output
  round-trip back through `Set`, and what an app's own machine-readable
  (`--json`) mode would depend on if built on top of this package or
  directly on `SettingsProvider.GetValues`. Don't route these through
  `localize()` even though it would be a one-line change — see that test's
  doc comment for the reasoning.
- **A nil (unconfigured) localizer always falls back to English**, exactly
  matching `permissions.WithLocalizer` and `settings.WithLocalizer`'s
  guarantee. Every test written before `WithLocalizer` existed
  (`TestShow`, `TestGet_*`, `TestSet_*`, etc.) still passes unmodified
  because of this — don't change that fallback behavior without checking
  those tests stay green.
- **Password fields never render their actual value.** `formatValue`
  returns `settings.SecretMask` for a set password, the localized
  "not set" placeholder otherwise — never the raw string, even if
  `SettingsProvider.GetValues()` somehow returned one unmasked (it
  shouldn't; that's `settings.Service`'s job via `Binding`/masking).

## Dependencies & insulation

- `settings` — the schema/value/validation types this package renders;
  `SettingsProvider` is satisfied by `*settings.Service`.
- `i18n` — `Text`, `T`, `Localizer` for this package's own strings.
- Stdlib only otherwise (`fmt`, `io`, `os`, `strconv`, `strings`, `errors`).
  No `wails/v3` import — this package must stay Wails-free (AD-4
  allowlist); it's meant to run in a CLI/TUI process with no
  `application.App` in sight.

## Extension points

- New localized string: add an `i18n.Text` var near the top of `cli.go`
  (follow the `text*` naming), add its key to `locales/en.json`, and thread
  a `*i18n.Localizer` argument (or `cfg.localizer`) to wherever it's
  rendered — mirror the existing `localize(l, textFoo, args...)` call
  sites. `i18n/extract_test.go`'s `TestCatalogKeysExist` fails the build if
  the key doesn't exist in `locales/en.json`.
- New adapter entry point (e.g. a future `Delete` or `Reset`): take
  `opts ...Option`, build `cfg` via `defaults()` + apply opts (like
  `Show`/`Get`/`Set` do — `Get` didn't do this before `WithLocalizer`
  existed, which was a latent bug fixed alongside this option), and use
  `cfg.localizer` for any human-facing text it emits.
- New `Option`: add a field to `config`, a `With*` constructor next to
  `WithOutput`/`WithLocalizer`.

## Testing

- Doubles: `settings.NewService` against `t.TempDir()` + `keyring.NewMemoryStore()`
  (see `testService` in `cli_test.go`), `bytes.Buffer` for `WithOutput`,
  `i18n.New(i18n.WithCatalog(fstest.MapFS{...}), i18n.WithLocale(...))` for
  an actually-resolving non-English localizer (see `frLocalizer`/
  `frCLICatalog`).
- `go test -race ./settings/cli/` must cover: every adapter operation
  against defaults/masked-passwords/conditional-fields (pre-existing),
  `WithLocalizer` actually changing the placeholder and error strings
  (`TestWithLocalizer_*`), a nil localizer falling back to English
  (`TestNilLocalizer_FallsBackToEnglish`), and — the WP-22 requirement —
  toggle/number tokens staying byte-identical regardless of locale
  (`TestMachineValuesUnaffectedByLocalizer`).
- Not automatable here: this package has no actual `--json` flag of its
  own (see Landmines) — the "machine output" pin is necessarily at the
  `formatValue`/`coerceValue` token level, not an end-to-end CLI flag test.

## File map

- `cli.go` — `SettingsProvider`, `Option`/`config`, `WithOutput`,
  `WithLocalizer`, `localize`, the `text*` i18n.Text vars, `Show`/`Get`/`Set`,
  `ValidationErrors`, `findField`/`formatValue`/`coerceValue`/`conditionMet`.
- `locales/en.json` — this package's own catalog; keys are
  `wailskit.settingscli.*`.
- `cli_test.go` — adapter behavior tests plus the `WithLocalizer`/
  machine-value-stability tests.

## Landmines

- **There is no `--json` flag in this package.** The WP-22 roadmap entry
  (`docs/v2-roadmap.md`) describes "machine output (`--json`) unchanged" as
  if one exists; it doesn't, here or anywhere else in the kit as of this
  writing. `TestMachineValuesUnaffectedByLocalizer` pins the closest actual
  contract this package has: `formatValue`/`coerceValue`'s token stability.
  If a real `--json` mode is added later (to this package or a downstream
  CLI built on `SettingsProvider.GetValues` directly), re-derive that test
  against the real flag instead of assuming this one already covers it.
- `Get` used to accept `opts ...Option` and silently ignore them (only
  `WithOutput` existed, and `Get` never writes to `cfg.out`). Adding
  `WithLocalizer` meant `Get` now actually applies `opts` — a latent no-op
  became live. If you add another `Option` that only makes sense for one of
  `Show`/`Get`/`Set`, remember all three now process the full `opts` chain.
- `ValidationErrors.localizer` is unexported and only ever set by `Set()`.
  A `*ValidationErrors` built directly (tests do this) has a nil localizer
  and renders the English "validation failed" prefix — that's intentional,
  not a gap to fix by exporting the field.
