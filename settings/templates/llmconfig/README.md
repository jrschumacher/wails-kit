# settings/templates/llmconfig

Settings-schema template for LLM provider configuration: provider dropdown, model
selection, API key (password field), and a per-provider base URL override. Pure schema —
no LLM SDK import, no network call, no client construction. If all you need is "let the
user pick a provider/model and store an API key," this package is the whole story.

If you also need to *use* the selection — construct an actual LLM client and call it —
see [`settings/templates/anyllm`](../anyllm/README.md), a separate nested Go module built
on top of this one. The split exists so that importing `llmconfig` never pulls in
`any-llm-go` (and its provider SDKs) — see that package's README for the reasoning.

## Usage

```go
import (
    "github.com/jrschumacher/wails-kit/v2/settings"
    "github.com/jrschumacher/wails-kit/v2/settings/templates/llmconfig"
)

group, cfg := llmconfig.New(
    llmconfig.WithProviders("anthropic", "openai", "mistral"),
    llmconfig.WithDefaultProvider("anthropic"),
)

svc := settings.NewService(
    settings.WithAppName("my-app"),
    settings.WithKeyring(store),
    settings.WithGroup(group),
)

// Read the effective selection back out at call time.
providerID, modelID, apiKey, err := cfg.Selection(svc)
baseURL, err := cfg.BaseURL(svc, providerID)
```

`Config.Selection` and `Config.BaseURL` are the entire "read side" of this package. What
you do with `(providerID, modelID, apiKey, baseURL)` — build an HTTP client, call a
vendor SDK directly, hand them to `anyllm.BuildProvider` — is up to you.

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `WithProviders(ids...)` | Providers in the dropdown, in order | `"anthropic"`, `"openai"` |
| `WithProvider(p)` | Add a new provider or replace a built-in one's label/models | — |
| `WithModels(id, models)` | Replace one provider's model list without restating its label | — |
| `WithDefaultProvider(id)` | Default selection | `"anthropic"` |
| `WithGroupKey(key)` | Settings group key (also the field-key prefix) | `"llm"` |
| `WithGroupLabel(label)` | Settings group label | `"LLM"` |

## The built-in catalog is a default, not a promise

`Builtin()` returns the kit's default provider/model catalog (currently `anthropic`,
`openai`, `deepseek`, `gemini`, `groq`, `mistral`, `ollama`). Model names and IDs in any
shipped LLM catalog go stale — that's a property of the domain, not a bug to eventually
fix. Rather than pretend a static list can stay current, this package makes it
overridable:

```go
// Replace a provider's model list outright — no need to restate its label.
llmconfig.WithModels("openai", []settings.SelectOption{
    {Label: "GPT-5", Value: "gpt-5"},
})

// Or start from the built-in list and add to it.
var anthropicModels []settings.SelectOption
for _, p := range llmconfig.Builtin() {
    if p.ID == "anthropic" {
        anthropicModels = p.Models
    }
}
llmconfig.WithModels("anthropic", append(anthropicModels,
    settings.SelectOption{Label: "Claude Next", Value: "claude-next-id"},
))

// Or add a provider the kit doesn't ship at all.
llmconfig.WithProvider(llmconfig.Provider{
    ID:     "acme",
    Label:  "Acme LLM",
    Models: []settings.SelectOption{{Label: "Acme Large", Value: "acme-large"}},
})
```

No fork required for any of the above.

## Settings fields

With the default group key `llm`, `New` creates:

| Key | Type | Description |
|-----|------|--------------|
| `llm.provider` | select | Provider selection |
| `llm.model` | select (dynamic) | Model selection, options vary by provider |
| `llm.{provider}.secret` | password | API key (per-provider, advanced) |
| `llm.{provider}.baseURL` | text | Base URL override (per-provider, advanced) |
| `llm.{provider}.customModel` | text | Custom model ID override (per-provider, advanced) |
| `llm.resolvedModelID` | computed | Resolved model (custom model or selected model) |

## Example

`examples/llmconfig/main.go` builds a group, wires it into an in-memory `settings.Service`,
sets values, and reads the selection back — no network, no real keyring.
