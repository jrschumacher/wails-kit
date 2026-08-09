# settings/templates/llmconfig — agent notes

## Purpose

`llmconfig` builds a `settings.Group` for LLM provider configuration — provider select,
model select, API key (password field), per-provider base-URL override — and a `Config`
for reading the effective selection back out of a `settings.Service`. It owns schema
construction and value reading only. It does **not** own client construction, HTTP calls,
or any LLM SDK dependency — that is `settings/templates/anyllm`, a separate nested Go
module one layer up. This split (AD-3 in `docs/v2-roadmap.md`) exists so a consumer that
only wants the schema never acquires `any-llm-go` or its provider SDK graph. If you find
yourself wanting to import an LLM SDK from this package, stop — that work belongs in
`anyllm` instead.

## Public API (load-bearing signatures)

```go
type Provider struct {
    ID     string
    Label  string
    Models []settings.SelectOption
}
func Builtin() []Provider // fresh copy every call

func New(opts ...Option) (settings.Group, *Config)
func WithProviders(ids ...string) Option
func WithProvider(p Provider) Option                            // add/override a definition
func WithModels(providerID string, models []settings.SelectOption) Option
func WithDefaultProvider(id string) Option
func WithGroupKey(key string) Option
func WithGroupLabel(label string) Option

type Config struct{ /* unexported groupKey */ }
func (c *Config) GroupKey() string
func (c *Config) Selection(svc *settings.Service) (providerID, modelID, apiKey string, err error)
func (c *Config) BaseURL(svc *settings.Service, providerID string) (string, error)
```

## Invariants (do not break)

- **No LLM SDK import, ever.** This is the entire point of the package existing
  separately from `anyllm`. `go.mod` at the repo root must never gain an `any-llm-go`
  require because of a change here.
- **`Builtin()` returns an independent copy on every call.** `Provider.clone()` deep-copies
  `Models`; a caller mutating the slice from one call must not affect another. See
  `TestBuiltin_ReturnsIndependentCopy`.
- **`WithProvider`/`WithModels` merge, they don't replace the whole registry.**
  `resolvedRegistry` starts from `builtinProviders`, then overlays `cfg.overrides` by ID.
  Overriding `"anthropic"`'s models must not remove `"openai"` from the resolved set.
- **`WithProviders` controls the dropdown; `WithProvider`/`WithModels` control
  definitions.** They're independent knobs on purpose — see the README's "combine with"
  example. Don't collapse them into one option; a consumer overriding just the Anthropic
  model list shouldn't be forced to restate the whole provider list.
- **`Config` methods re-read `svc` every call; they cache nothing.** `Selection` calling
  `svc.GetValues()` twice across two calls must see the second `SetValues`. Don't add a
  cache without a cache-invalidation story (there is a settings `onChange` hook available
  if that's ever needed).
- **A missing API key is not an error.** `Config.Selection` treats `keyring.ErrNotFound`
  from `svc.GetSecret` as `apiKey = ""`, not a returned error — a provider selected but
  not yet configured with a key is normal state, not a fault.

## Dependencies & insulation

`settings` (schema types, `Service`) and `keyring` (`ErrNotFound` sentinel only, to
distinguish "no secret set" from a real keyring failure in `Config.Selection`). No
`wails/v3` import — not on the AD-4 allowlist and has no reason to be. No `any-llm-go` or
any other LLM SDK — see Invariants.

## Extension points

- New per-provider advanced fields (e.g. an org-ID field some providers need): add to the
  per-provider loop in `buildGroup`, following the `secret`/`baseURL`/`customModel`
  pattern (key = `prefix + "." + id + "." + fieldName`, `Condition` gated on the provider
  select).
- i18n: `Field.Label` etc. are still plain `string`. WP-12 will change kit-wide field
  labels to `i18n.Text` — don't pre-empt that here in isolation.
- If a consumer needs the resolved model ID without a full `Selection` call (e.g. just for
  display), `resolveModelID` is already factored out — consider exporting it if that need
  is real rather than duplicating its logic.

## Testing

Doubles: `settings.NewService` with `settings.WithStoragePath(t.TempDir()/...)` and
`keyring.NewMemoryStore()` — never the default OS-standard path or a real OS keyring (see
`newTestService` in `config_test.go`). No network, no filesystem outside `t.TempDir()`.

`go test ./settings/templates/llmconfig/ -race` must cover: schema shape for default and
custom provider sets, per-provider advanced field conditions, unknown-provider-ID is
silently skipped (not an error), `WithProvider`/`WithModels` add and override paths,
`Builtin()` copy independence, and `Config.Selection`/`BaseURL` against a real (in-memory)
`settings.Service` round-trip including the "no secret set" non-error case.

## File map

- `llmconfig.go` — `Provider`, `builtinProviders`, `Builtin`, `Option`s, `New`,
  `resolvedRegistry`, `buildGroup`, `resolveModelID`.
- `config.go` — `Config`, `Selection`, `BaseURL`.
- `*_test.go` — schema-shape tests (`llmconfig_test.go`) and `Config` round-trip tests
  against a real `settings.Service` (`config_test.go`).

## Landmines

- **A provider's `Default` is set on the field regardless of whether that provider ID
  appears in `cfg.providers`.** `New(WithProviders(), WithDefaultProvider("anthropic"))`
  still stores `"anthropic"` as the default value even though the dropdown has zero
  options — this mirrors the pre-split behavior and was not treated as a bug to fix in
  this pass. If you rely on "no providers configured" meaning "no default value gets
  persisted," pass `WithDefaultProvider("")` explicitly (see
  `TestConfig_Selection_Empty`).
- `WithModels` on a provider ID that's neither built-in nor already added via
  `WithProvider` creates a `Provider{ID: id}` with an empty `Label`. That provider will
  render in the dropdown (if included via `WithProviders`) with no display label — call
  `WithProvider` first if you want a label.
- Field keys are string-built (`prefix + "." + id + ".secret"`, etc.) — there's no
  validation that `groupKey` or a provider ID doesn't contain a literal `.` that would
  collide with this scheme. Not currently guarded; keep IDs simple identifiers.
