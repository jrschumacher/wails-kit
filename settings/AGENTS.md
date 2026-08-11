# settings — agent notes

## Purpose

`settings` is a schema-driven settings framework: the backend declares a `Schema` of
`Group`s and `Field`s (types, options, visibility conditions, validation rules), and any
frontend renders a form from it generically. It owns the settings file (atomic, durable
JSON), the password-field/keyring split, effective-state validation, and (WP-12)
resolving `i18n.Text` labels into the plain-string `ResolvedSchema` a frontend actually
renders. It does not own the CLI adapter (`settings/cli`) or LLM-specific schema
construction (`settings/templates/*`) — both owned separately, both broken after WP-12
until their owners thread i18n through them (Landmines).

## Public API (load-bearing signatures)

```go
type Service struct{ /* ... */ }
func NewService(opts ...ServiceOption) *Service
func WithAppName(name string) ServiceOption      // combines with WithStoragePath, any order
func WithStoragePath(path string) ServiceOption  // wins over WithAppName if both given
func WithKeyring(store keyring.Store) ServiceOption // omit -> in-memory + slog.Warn
func WithGroup(g Group) ServiceOption
func WithOnChange(fn func(values map[string]any)) ServiceOption
func WithLocalizer(l *i18n.Localizer) ServiceOption // optional; nil-safe, opt-in (WP-12)

func (s *Service) GetSchema() ResolvedSchema                   // resolved fresh every call
func (s *Service) GetValues() (map[string]any, error)          // secrets masked
func (s *Service) SetValues(values map[string]any) ([]ValidationError, error)
func (s *Service) GetSecret(key string) (string, error)        // backend-only, NEVER bind

func (s *Service) Binding() *Binding // the ONLY type to register with Wails
type Binding struct{ /* unexported svc *Service */ }
func (b *Binding) GetSchema() ResolvedSchema
func (b *Binding) GetValues() (map[string]any, error)
func (b *Binding) SetValues(values map[string]any) ([]ValidationError, error)

func Validate(schema Schema, values map[string]any, localizer *i18n.Localizer) []ValidationError
func LocaleGroup(l *i18n.Localizer) Group          // locale-picker group; see locale.go
func WireLocale(svc *Service, l *i18n.Localizer)   // live-switch glue (M6); see locale.go

// Group/Field/SelectOption (schema.go) are the authoring types WithGroup composes;
// every label is i18n.Text (WP-12). ResolvedGroup/ResolvedField/ResolvedSelectOption
// are what GetSchema returns — same JSON shape as the pre-WP-12 Group/Field/
// SelectOption, labels resolved to plain strings.
```

## Invariants (do not break)

- **Never register `*Service` with Wails — only `Service.Binding()`.** Wails v3 binds
  every exported method of a registered service; `GetSecret` returns raw secrets.
  `TestBindingSurface` pins `Binding`'s method set to exactly `{GetSchema, GetValues,
  SetValues}`.
- **`SetValues` validates effective state, not the raw payload.** Merges defaults →
  persisted → masked-secret-presence → submission before calling `Validate`, so a
  partial update can't dodge a `Condition`/`DynamicOptions` dependency by omitting the
  controlling field (`TestValidateEffectiveState*`). Persistence still writes only the
  *submitted* non-secret keys.
- **A `DynamicOptions` parent changing alone resets a stranded dependent, never fails
  validation for it.** `SetValues` merges submitted+persisted state and, when this
  submission changes a `DynamicOptions` parent but doesn't also resubmit the dependent,
  resets the dependent (to `Field.Default` if still valid, else the new parent's first
  option) instead of rejecting the whole call over a field the caller never touched
  (H2). An explicit, self-inconsistent submission of both parent and dependent still
  fails normally — only a value the caller didn't submit this call gets corrected. See
  `dynamicOptionCorrections` (`service.go`) and
  `TestSetValues_DynamicOptionParentChangeResetsStrandedDependent` /
  `TestSetValues_DynamicOptionExplicitMismatchStillRejected`.
- **A non-string password value is a validation error, never a delete.**
  `validateField` type-checks `FieldPassword` like `FieldToggle`; `SetValues` keeps a
  defensive backstop after. See `TestPasswordNonString`.
- **`onChange` callbacks run after `s.mu` is released.** Snapshot under the lock,
  unlock, then invoke — a callback calling back into the Service would otherwise
  deadlock. See `TestOnChangeNoLock`.
- **`Store.Save` never renames without a successful `fsync`.** Temp file → `Chmod(0600)`
  → `Write` → `Sync` → `Close` → rename. See `TestSaveDurability`.
- **`WithAppName`/`WithStoragePath` are order-independent**; `NewService` resolves
  `s.store` once, after every option has run. The default in-memory keyring warns
  (`slog.Warn`) rather than silently losing secrets on restart.
- Password values never touch the JSON file; computed fields are stripped before
  `Store.Save`; unknown keys in a saved file are stripped on `Load`.
- **A `nil` localizer resolves every `i18n.Text` to its literal `Other` value, never
  panics or returns `""`.** `resolveText` (`resolve.go`) is the single choke point —
  `GetSchema` and `Validate` both go through it. This is the entire "localization is
  opt-in" contract (WP-12). See `TestSchemaNoLocalizerFallsBackToLiteral`.
- **`GetSchema` resolves fresh on every call — never cache a resolved schema.** A
  cached copy would go stale the moment `Localizer.SetLocale` fires `i18n:changed`; the
  point of read-time resolution (mirrors `errors.GetUserMessage`, AD-5) is that the
  *next* call reflects the new locale for free. See `TestSchemaResolvesLocale`.
- **`ResolvedSchema`'s JSON shape is byte-for-byte the pre-WP-12 `Schema`'s.** Don't
  add/rename a JSON field on `ResolvedField`/`ResolvedGroup`/`ResolvedSelectOption`
  without treating it as a frontend-contract break. See `TestSchemaWireShapeUnchanged`.
- **`WireLocale` never forwards `"system"` to `l.SetLocale`.** `"system"` isn't a
  BCP-47 tag, and `Localizer` has no operation to revert an explicit `SetLocale` back
  to its env/OS/default tiers — see `TestWireLocale_LiveSwitch`'s last assertion. Don't
  "fix" this by making `WireLocale` special-case `"system"` into some resolution call;
  that capability doesn't exist on the `i18n` side to call into (i18n/AGENTS.md
  Landmines).

## Dependencies & insulation

- `keyring` for secret persistence; `appdirs` (via `Store`) for the default path. No
  `wails/v3` import — Wails-free per AD-4. `Binding` registration happens in the
  consuming app or `kit/wailsbridge`.
- **`i18n`, one direction only (WP-12).** This package imports `i18n` for `i18n.Text`
  and `*i18n.Localizer`; `i18n` must never import `settings` back. `i18n` used to
  (`Localizer.SettingsGroup()`, WP-10) — WP-12 removed that and moved the equivalent
  construction here as `LocaleGroup` (`locale.go`) specifically to break the would-be
  cycle. If `i18n` ever re-imports `settings`, this package's build breaks the instant
  it imports `i18n` too — that's the cycle, not a coincidence.
- `settings/cli` and `settings/templates/*` import this package but are owned
  separately — don't edit them from here; breaking their compile under this WP's
  no-backwards-compatibility mandate is expected. Report it, don't fix it here.

## Extension points

- New field types: add a `FieldType` in `schema.go`, a type-check block in
  `validateField`, and the type to `Validate`'s early-continue gate.
- New `ServiceOption`s: stage a field, resolve in `NewService` (like `appName`).
- New translatable surface: add an `i18n.Text` var with a `wailskit.settings.*` key,
  add it to `locales/en.json`, resolve via `resolveText`. The extraction lint is a
  source-text regex over `i18n.T("wailskit....", ...)` — don't spell out a fake call
  in a doc comment.

## Testing

- Doubles: `keyring.NewMemoryStore()`, `t.TempDir()` + `WithStoragePath` (never call
  `NewService` without one in a test that calls `SetValues` — the default path is a
  real OS directory). i18n: `i18n.New(i18n.WithCatalog(fstest.MapFS{...}), i18n.WithLocale("en"))`.
- `go test -race ./settings/` must cover: the binding reflection guard, effective-state
  validation, non-string password rejection, the onChange-outside-lock deadlock check,
  save durability, and (`i18n_test.go`) a label resolving through a catalog, `SetLocale`
  re-resolving the next `GetSchema` call, the no-localizer fallback, the `ResolvedSchema`
  wire shape, and validation messages localizing.
- Not automatable here: a real Wails binding call from a live webview —
  `TestBindingSurface` is the practical substitute.

## File map

- `service.go` — `Service`, options (incl. `WithLocalizer`), `NewService`, `GetSchema`,
  `GetValues`/`SetValues`, `GetSecret`, `effectiveValues`, secret masking.
- `binding.go` — `Binding`, the frontend-safe registration surface.
- `schema.go` — authoring types (`i18n.Text` labels) and `Resolved*` wire types.
- `resolve.go` — authoring -> resolved, nil-localizer-safe (`resolveText` etc.).
- `locale.go` — `LocaleGroup` (moved from `i18n.Localizer.SettingsGroup`, WP-12);
  `WireLocale` (M6 — `svc.AddOnChange` -> `l.SetLocale` glue for live switching).
- `validate.go` — `Validate`, `validateField`, `conditionMet`, the `msg*` templates.
- `store.go` — `Store`: path resolution, `Load`, durable `Save`.
- `locales/en.json` — this package's catalog: locale-picker labels + validation messages.
- `*_test.go` — see Testing; `i18n_test.go` is WP-12's.
- `README.md` — quickstart, `Binding()` rule, storage paths, field types, validation,
  durability, localization.

## Landmines

- `effectiveValues` must run while `s.mu` is held — don't hoist it out of `SetValues`.
- `Store.Save`'s `mergedValues` re-read-and-merge is a *different* merge than
  `Service.effectiveValues` — don't conflate them.
- `createTempFile` is a package-level `var`; tests overriding it must restore it in a
  `defer` or poison later tests in the same binary.
- `GetValues`'s compute-func pass runs after *all* groups' secrets are masked, not
  interleaved per group. The in-memory keyring default stays (loud, not removed) —
  `NewService()` with zero options must keep working for tests/examples.
- **`settings/cli`, `settings/templates/{llmconfig,anyllm}`, `examples/llmconfig`, and
  `diagnostics` all fail to build after WP-12** — each constructs a `Group`/`Field`/
  `SelectOption` with a plain-`string` `Label`. Expected, out of scope (same precedent
  WP-03 set). `settings/cli`'s fix is WP-22; the rest have no assigned follow-up WP.
- **`i18n.Text{}`/`i18n.Text{Other: "x"}` (empty `Key`) always resolve to `Other`**,
  localizer or not — the intentional way to write a literal label, not a bug to "fix".
