# settings/templates/anyllm — agent notes

## Purpose

`anyllm` turns the LLM selection produced by `settings/templates/llmconfig` into a
runnable `any-llm-go` client (`BuildProvider`) and offers live model enumeration where
the underlying provider supports it (`ListModels`). It owns client construction and
provider-ID-to-SDK-package mapping only. It does not own settings-schema construction,
the model catalog, or reading raw settings values — all of that is `llmconfig`; this
package calls `*llmconfig.Config`'s exported methods and nothing else.

**This is a separate Go module from the rest of wails-kit** (own `go.mod`, path
`github.com/jrschumacher/wails-kit/settings/templates/anyllm/v2`), by design (AD-3 in
`docs/v2-roadmap.md`). That split is the entire reason this package is allowed to exist —
see the root `CLAUDE.md`/roadmap §1 "Amended philosophy" for the carve-out argument. If
you add an import here that isn't `any-llm-go`, its subpackages, `llmconfig`, or
`settings`, stop and ask whether it belongs in this module at all.

## Public API (load-bearing signatures)

```go
var ErrNoProviderSelected = errors.New(...)
var ErrModelListingUnsupported = errors.New(...)

func BuildProvider(svc *settings.Service, cfg *llmconfig.Config) (anyllmsdk.Provider, string, error)
func ListModels(ctx context.Context, p anyllmsdk.Provider) ([]anyllmsdk.Model, error)
```

## Invariants (do not break)

- **The main module (`github.com/jrschumacher/wails-kit/v2`) must never require
  `any-llm-go`.** That's the measurable point of WP-07 (`grep -n "any-llm-go" go.mod` at
  the repo root must return nothing). Any change here that would force `any-llm-go` back
  into the main module's `go.mod` is the change to revert, not push through.
- **`newProvider`'s switch is the only place a `llmconfig` provider ID becomes an
  any-llm-go client.** It is intentionally not a `With*` option — see README "Provider
  coverage." Don't confuse it with `llmconfig.WithProvider`/`WithModels`, which are the
  actual fix for the kit's stale-model-list defect; that fix lives one module down and
  should not be re-litigated here.
- **`BuildProvider` never makes a network call.** Constructing an any-llm-go provider
  (`anthropic.New`, `openai.New`, etc.) only builds an HTTP client; it does not hit the
  network. Every test and the example rely on this to stay network-free — don't add a
  connectivity check or "validate the key works" call here.
- **`ListModels` returning `ErrModelListingUnsupported` is a type assertion, not a
  request.** The `ModelLister` check happens before any I/O. Don't reorder it behind a
  call that might itself fail for other reasons — callers rely on `errors.Is(err,
  ErrModelListingUnsupported)` meaning "this provider can't do this," not "this provider
  tried and failed."
- **Provider support for `ModelLister` is not what you'd guess from provider "size."**
  Anthropic does not implement it; OpenAI, DeepSeek, Groq, and Mistral all do (inherited
  from any-llm-go's shared `CompatibleProvider`), as do Gemini and Ollama directly. Verify
  against `any-llm-go`'s source before changing any doc comment that names which
  providers support listing — this file and the README got it wrong once already during
  this package's own construction and had to be corrected against the SDK source.

## Dependencies & insulation

`any-llm-go` and its provider subpackages (`anthropic`, `openai`, `deepseek`, `gemini`,
`groq`, `mistral`, `ollama`) — the entire reason this module is nested. `llmconfig` and
`settings` from the main module, resolved locally via the repo-root `go.work` during
development and via this module's own `require`+`replace` (see go.mod comment) when built
standalone. No `wails/v3` import — not on the AD-4 allowlist, no reason to be.

## Extension points

- A new any-llm-go provider package: add one `case` to `newProvider`. Pair it with adding
  the provider to `llmconfig.builtinProviders` (a separate module, separate PR/commit) if
  it should also get a default model list.
- If a consumer needs to construct a provider `newProvider` doesn't know about (a custom
  `llmconfig.WithProvider` ID, or an any-llm-go provider this switch hasn't caught up to
  yet), they call the any-llm-go provider package directly with `cfg.Selection`/`BaseURL`
  output — `BuildProvider` is a convenience, not the only path.

## Testing

Doubles: `settings.NewService` with `settings.WithStoragePath(t.TempDir()/...)` and
`keyring.NewMemoryStore()` (see `newTestService` in `anyllm_test.go`) — same pattern as
`llmconfig`. API keys used in tests are fake strings; provider construction never
validates them over the network. `go test ./... -race` (run from inside this directory —
it is its own module, `go test` at the repo root does **not** reach it) must cover:
`ErrNoProviderSelected`, successful construction for at least one non-OpenAI-compatible
provider (Anthropic) and one OpenAI-compatible one, custom-model and base-URL overrides
flowing through, an unknown provider ID producing an error, and both branches of
`ListModels` (`ErrModelListingUnsupported` for Anthropic, interface satisfaction for
OpenAI) without ever calling the real `ListModels(ctx)` network path.

## File map

- `anyllm.go` — `BuildProvider`, `ListModels`, `newProvider`, sentinel errors.
- `anyllm_test.go` — construction, override, and model-listing tests against a real
  (in-memory) `settings.Service`.
- `example/main.go` — runnable example; lives inside this module (not the main module's
  `examples/`) because it needs `any-llm-go`.
- `go.mod` — nested module; see the file's own comment and README "Release tagging" for
  the `require`+`replace` bootstrapping story.

## Landmines

- **`go test ./...` from the repo root does not run this package's tests.** This is a
  separate module; CI and any verification script must `cd` into it explicitly. The
  repo-root `go.work` makes `go build`/`go vet`/editor tooling see both modules together,
  but `go test ./...` is scoped to whichever module you're standing in.
- **The `replace` in `go.mod` is a development convenience, not a permanent fixture.**
  Once the main module has a real `v2.x.y` tag, the `require` should point at it and the
  `replace` should come out — leaving it in means this module can never actually be
  published standalone (its `go.sum` would be unverifiable for a consumer without the
  same local checkout).
- `BuildProvider`'s error for an apiKey-required-but-missing provider (e.g. Anthropic)
  comes from any-llm-go's own `errors.NewMissingAPIKeyError`, not a sentinel this package
  defines — don't add a redundant check here that duplicates or contradicts it.
