# settings — agent notes

## Purpose

`settings` is a schema-driven settings framework: the backend declares a `Schema` of
`Group`s and `Field`s (types, options, visibility conditions, validation rules), and any
frontend renders a form from it generically. It owns the settings file (atomic, durable
JSON), the password-field/keyring split, and effective-state validation. It does not own
i18n (labels are still plain `string` — WP-12), the CLI adapter (`settings/cli`, owned
separately), or LLM-specific schema construction (`settings/templates/*`, owned
separately).

## Public API (load-bearing signatures)

```go
type Service struct{ /* ... */ }
func NewService(opts ...ServiceOption) *Service
func WithAppName(name string) ServiceOption      // combines with WithStoragePath, any order
func WithStoragePath(path string) ServiceOption  // wins over WithAppName if both given
func WithKeyring(store keyring.Store) ServiceOption // omit -> in-memory + slog.Warn
func WithGroup(g Group) ServiceOption
func WithOnChange(fn func(values map[string]any)) ServiceOption

func (s *Service) GetSchema() Schema
func (s *Service) GetValues() (map[string]any, error)          // secrets masked
func (s *Service) SetValues(values map[string]any) ([]ValidationError, error)
func (s *Service) GetSecret(key string) (string, error)        // backend-only, NEVER bind

func (s *Service) Binding() *Binding // the ONLY type to register with Wails
type Binding struct{ /* unexported svc *Service */ }
func (b *Binding) GetSchema() Schema
func (b *Binding) GetValues() (map[string]any, error)
func (b *Binding) SetValues(values map[string]any) ([]ValidationError, error)

func Validate(schema Schema, values map[string]any) []ValidationError
```

## Invariants (do not break)

- **Never register `*Service` with Wails — only `Service.Binding()`.** Wails v3 binds
  every exported method of a registered service; `GetSecret` returns raw secrets.
  `TestBindingSurface` (`binding_test.go`) is a reflection test pinning `Binding`'s
  method set to exactly `{GetSchema, GetValues, SetValues}`.
- **`SetValues` validates effective state, not the raw payload.** It merges
  defaults → persisted (`store.Load()`) → masked-secret-presence → submission before
  calling `Validate`, so a partial update can't dodge a `Condition` or a
  `DynamicOptions` dependency by omitting the controlling field. See
  `TestValidateEffectiveState*`. Persistence still writes only the *submitted*
  non-secret keys — the merge is for validation only.
- **A non-string password value is a validation error, never a delete.**
  `validateField` type-checks `FieldPassword` like `FieldToggle`. `SetValues` keeps a
  defensive `ok` check after, but `Validate` is the real gate. See
  `TestPasswordNonString`.
- **`onChange` callbacks run after `s.mu` is released.** Snapshot `toSave` + masked
  secrets + a copy of `s.onChange` under the lock, unlock, then invoke. A callback that
  calls back into the Service would otherwise deadlock. See `TestOnChangeNoLock`
  (5s-timeout test, fails loud instead of hanging `go test`).
- **`Store.Save` never renames without a successful `fsync`.** `writeTempFile`:
  temp file → `Chmod(0600)` → `Write` → `Sync` → `Close` → caller renames.
  `createTempFile` is an overridable package var (`fileHandle` interface) so tests can
  intercept `Sync` without a real crash. See `TestSaveDurability`. Mirrors
  `keyring.EnvelopeStore.writeTempLocked`.
- **`WithAppName`/`WithStoragePath` are order-independent.** Both stage fields
  (`s.appName`, `s.storagePath`); `NewService` resolves `s.store` once, after every
  option has run. Don't go back to eagerly building `s.store` inside an option func.
- **The default in-memory keyring warns.** No `WithKeyring` → `slog.Warn(...)` then
  `keyring.NewMemoryStore()`. Don't remove the warning to quiet test output — pass
  `WithKeyring` explicitly instead.
- Password values never touch the JSON file; computed fields are always stripped before
  `Store.Save`; unknown keys in a saved file are stripped on `Load`.

## Dependencies & insulation

- `keyring` (`Store` interface) for secret persistence; `appdirs` (via `Store`) for the
  default OS-standard path. No `wails/v3` import — Wails-free per the AD-4 allowlist.
  `Binding` is still Wails-free itself; actual registration happens in the consuming app
  or `kit/wailsbridge`.
- `settings/cli` and `settings/templates/*` import this package but are owned
  separately — don't edit them from here. Breaking their compile is expected under this
  burn-down's no-backwards-compatibility mandate; report it, don't fix it here.

## Extension points

- New field types: add a `FieldType` in `schema.go`, add a type-check block in
  `validateField` (`validate.go`), and add the type to `Validate`'s early-continue gate
  — a field with no explicit `Validation` never reaches your check otherwise.
- New `ServiceOption`s: stage a field on `Service`, resolve it in `NewService` (like
  `appName`/`storagePath`). Don't resolve inside the option func unless it's fully
  independent of every other option.
- i18n (WP-12) will turn `Field.Label/Description/Placeholder`, `Group.Label`, and
  `SelectOption.Label` into `i18n.Text`. Don't pre-empt that here.

## Testing

- Doubles: `keyring.NewMemoryStore()`, `t.TempDir()` + `WithStoragePath`. **Never** call
  `NewService` without `WithStoragePath`/`WithStorePath` in a test that calls
  `SetValues` — the default path is a real OS-standard directory, and writing there
  pollutes the developer's filesystem across separate `go test` runs. (This happened
  while writing `TestBinding_DelegatesToService` in this WP — see its fix for the
  pattern to copy.)
- `go test -race ./settings/` must cover: the binding reflection guard, effective-state
  validation (condition + dynamic-select), non-string password rejection, the
  onChange-outside-lock deadlock check, and save durability (fake `Sync` + a real
  round-trip subtest).
- Not automatable here: a real Wails binding call from a live webview.
  `TestBindingSurface` is the practical substitute; true end-to-end belongs to
  `kit/wailsbridge` or the consuming app.

## File map

- `service.go` — `Service`, options, `NewService`, `GetValues`/`SetValues`,
  `GetSecret`, effective-state merge (`effectiveValues`), secret masking.
- `binding.go` — `Binding`, the frontend-safe registration surface.
- `schema.go` — `Schema`/`Group`/`Field`/`SelectOption`/`Condition`/`DynamicOptions`/
  `Validation` types.
- `validate.go` — `Validate`, `validateField`, `conditionMet`, select-option membership.
- `store.go` — `Store`: path resolution, `Load`, durable `Save`.
- `*_test.go` — see Testing above for which regression test guards which defect.
- `README.md` — quickstart, the `Binding()` registration rule, storage paths, field
  types, validation semantics, durability guarantees.

## Landmines

- `effectiveValues` (reads `s.store`/`s.secrets`) must run while `s.mu` is held — don't
  hoist it outside the lock in `SetValues`.
- `Store.Save`'s internal `mergedValues` re-read-and-merge is a *different* merge than
  `Service.effectiveValues` (disk-level key preservation vs. validation semantics).
  Don't conflate or "simplify" one into the other.
- `createTempFile` is a package-level `var`. Tests that override it must restore the
  original in a `defer`, or they poison every later test in the same binary.
- `GetValues`'s compute-func pass runs after *all* groups' secrets are masked (not
  interleaved per group). Intentional — a compute func can reference any group's masked
  secret — but cross-group compute-func ordering is otherwise unspecified.
- The in-memory keyring default is still the default (loud, not removed) — `NewService()`
  with zero options must keep working for tests/examples. Making it a hard error is a
  bigger decision than this WP made; raise it with the roadmap owner first.
