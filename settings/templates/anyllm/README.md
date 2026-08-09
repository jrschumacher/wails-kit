# settings/templates/anyllm

Client construction and live model enumeration on top of [`settings/templates/llmconfig`](../llmconfig/README.md),
built on [any-llm-go](https://github.com/mozilla-ai/any-llm-go). Given a `settings.Service`
wired up with `llmconfig`'s settings group, `BuildProvider` reads the effective
provider/model/API-key/base-URL selection and constructs a ready-to-use `any-llm-go`
provider.

## Why this is a separate, nested Go module

`any-llm-go` pulls in a provider-SDK zoo (Anthropic, OpenAI, Gemini's `google.golang.org/genai`,
gRPC, and more) as transitive dependencies. Most wails-kit consumers don't want any of
that just because they use `settings/templates/llmconfig` to render a provider picker. So
this package lives in its **own Go module** (own `go.mod`, tagged independently — see
"Release tagging" below) rather than the main `github.com/jrschumacher/wails-kit/v2`
module. Importing `llmconfig` never pulls this module or `any-llm-go` in; importing this
module is an explicit, separate `go get`.

Be precise about what this buys you: Go 1.17+ module-graph pruning already means an
*unimported* package's dependencies don't reach a consumer's build — measured elsewhere in
this repo, importing seven kit packages adds only 4 modules to a consumer's build list, no
`any-llm-go` transitive graph included, even before this split existed. The nested module
doesn't change that build-graph outcome; what it changes is making the optionality
**structural** rather than incidental — there is no possible import path from
`llmconfig` (or the rest of wails-kit) into `any-llm-go` for the module graph to prune in
the first place. The practical, measurable win today is a smaller `go.sum` and less
noise from dependency-vulnerability scanners for consumers who never wanted this
dependency. Don't oversell it as a build-performance feature; it isn't one.

## Usage

```go
import (
    anyllmsdk "github.com/mozilla-ai/any-llm-go"

    "github.com/jrschumacher/wails-kit/v2/settings"
    "github.com/jrschumacher/wails-kit/v2/settings/templates/llmconfig"
    "github.com/jrschumacher/wails-kit/settings/templates/anyllm/v2"
)

group, cfg := llmconfig.New(
    llmconfig.WithProviders("anthropic", "openai"),
    llmconfig.WithDefaultProvider("anthropic"),
)

svc := settings.NewService(
    settings.WithAppName("my-app"),
    settings.WithKeyring(store),
    settings.WithGroup(group),
)

provider, modelID, err := anyllm.BuildProvider(svc, cfg)
if err != nil {
    // handle: no provider selected (anyllm.ErrNoProviderSelected), missing/invalid
    // API key, or a provider ID llmconfig knows about that this package doesn't
    // (see "Provider coverage" below).
}

resp, err := provider.Completion(ctx, anyllmsdk.CompletionParams{
    Model:    modelID,
    Messages: []anyllmsdk.Message{{Role: anyllmsdk.RoleUser, Content: "Hello"}},
})
```

## Live model enumeration

```go
models, err := anyllm.ListModels(ctx, provider)
if errors.Is(err, anyllm.ErrModelListingUnsupported) {
    // fall back to llmconfig's static model list for this provider
}
```

Not every provider supports listing. Of the providers `BuildProvider` knows how to
construct: OpenAI, DeepSeek, Groq, and Mistral inherit `ListModels` from any-llm-go's
shared OpenAI-compatible client; Gemini and Ollama implement it directly; **Anthropic does
not** — `ListModels` returns `ErrModelListingUnsupported` for it today. This is a property
of each provider's any-llm-go client, not something this package controls.

## Provider coverage

`BuildProvider` maps a `llmconfig` provider ID to an any-llm-go constructor via a fixed
switch: `anthropic`, `openai`, `deepseek`, `gemini`, `groq`, `mistral`, `ollama`. This is
a mechanical mapping to which provider packages any-llm-go ships — not the kind of
business data (model names, snapshot IDs) that goes stale, which is why it isn't a
`With*` option the way `llmconfig`'s model catalog is. If `llmconfig` is configured with a
custom provider ID via `llmconfig.WithProvider` that has no case in this switch,
`BuildProvider` returns an error naming the unknown ID — construct the client yourself in
that case; `BuildProvider` is a convenience for the providers any-llm-go ships, not a
requirement.

## The model-list staleness fix lives in `llmconfig`

The kit previously hardcoded a hand-picked model list per provider directly in this
package (e.g. specific dated snapshots). That list is business data that will always drift
out of date. It has moved to `llmconfig.Builtin()` and is overridable there via
`llmconfig.WithModels`/`llmconfig.WithProvider` — see that package's README. This package
has no model list of its own to go stale.

## Release tagging

This is a nested Go module: it has its own `go.mod` and is versioned and tagged
independently of the main module, using the standard Go nested-module tag scheme:

```
settings/templates/anyllm/v2.x.y
```

e.g. `settings/templates/anyllm/v2.1.0`. Consumers `go get` it as
`github.com/jrschumacher/wails-kit/settings/templates/anyllm/v2` — Go resolves the tag
prefix to the module path automatically. It depends on a tagged release of the main
module (`github.com/jrschumacher/wails-kit/v2`); bump that `require` and re-tag this
module whenever it needs a newer main-module release, same as any other dependency.

During local development, the repo-root `go.work` (`use .` + `use ./settings/templates/anyllm`)
resolves the main-module import to the working tree directly, so day-to-day work never
needs a real tag. This module's `go.mod` also carries a `replace` to `../../..` so it
builds standalone (no `GOWORK`) before the main module has ever been tagged; drop that
`replace` and point the `require` at a real tag once one exists.

## Example

`example/main.go` (inside this module, since it needs `any-llm-go`) builds a provider
against a fake API key and constructs the client — it never makes a real network call.
