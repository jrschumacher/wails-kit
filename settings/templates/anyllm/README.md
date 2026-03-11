# settings/templates/anyllm

Settings template for LLM provider configuration via [any-llm-go](https://github.com/mozilla-ai/any-llm-go).

Generates a settings group (provider dropdown, model selection, API key, advanced options) and a builder function that constructs a configured any-llm-go provider from the current settings values.

## Usage

```go
import (
    "github.com/jrschumacher/wails-kit/settings"
    "github.com/jrschumacher/wails-kit/settings/templates/anyllm"
)

group, buildProvider := anyllm.New(
    anyllm.WithProviders("anthropic", "openai", "mistral"),
    anyllm.WithDefaultProvider("anthropic"),
)

svc := settings.NewService(
    settings.WithAppName("my-app"),
    settings.WithKeyring(store),
    settings.WithGroup(group),
)

// Build an any-llm-go provider from current settings values.
provider, modelID, err := buildProvider(svc)

// Use the provider with any-llm-go.
resp, err := provider.Completion(ctx, anyllm.CompletionParams{
    Model:    modelID,
    Messages: []anyllm.Message{{Role: anyllm.RoleUser, Content: "Hello"}},
})
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `WithProviders(names...)` | Providers in the dropdown | `"anthropic"`, `"openai"` |
| `WithDefaultProvider(name)` | Default selection | `"anthropic"` |
| `WithGroupKey(key)` | Settings group key | `"llm"` |
| `WithGroupLabel(label)` | Settings group label | `"LLM"` |

## Supported providers

`"anthropic"`, `"openai"`, `"deepseek"`, `"gemini"`, `"groq"`, `"mistral"`, `"ollama"`

Each provider gets these advanced settings fields (shown only when that provider is selected):

- **API Key** — stored in the OS keyring via the settings password field
- **Base URL** — override the provider's default API endpoint
- **Custom Model ID** — override the model dropdown selection

## Settings fields

With the default group key `llm`, the template creates:

| Key | Type | Description |
|-----|------|-------------|
| `llm.provider` | select | Provider selection |
| `llm.model` | select (dynamic) | Model selection, options vary by provider |
| `llm.{provider}.secret` | password | API key (per-provider, advanced) |
| `llm.{provider}.baseURL` | text | Base URL override (per-provider, advanced) |
| `llm.{provider}.customModel` | text | Custom model ID (per-provider, advanced) |
| `llm.resolvedModelID` | computed | Resolved model (custom model or selected model) |
