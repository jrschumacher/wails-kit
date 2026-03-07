# wails-kit

Reusable Go module for Wails apps. Provides a schema-driven settings framework and LLM provider management.

## Packages

### `settings` — Schema-Driven Settings

The backend defines a settings schema (fields, types, options, visibility conditions) and any frontend renders from it dynamically.

```go
import "github.com/jrschumacher/wails-kit/settings"

svc := settings.NewService(
    settings.WithAppName("my-app"),             // persists to ~/.my-app/settings.json
    settings.WithGroup(mySettingsGroup()),       // register settings groups
    settings.WithOnChange(func(v map[string]any) {
        // react to settings changes
    }),
)

// Register as a Wails service
app := application.New(application.Options{
    Services: []application.Service{
        application.NewService(svc),
    },
})
```

**Frontend contract** (3 Wails bindings):
- `GetSchema()` — returns JSON describing all fields, types, options, conditions
- `GetValues()` — returns current values with defaults and computed fields
- `SetValues(values)` — validates, saves, triggers onChange callbacks

**Defining a settings group:**

```go
func mySettingsGroup() settings.Group {
    return settings.Group{
        Key:   "appearance",
        Label: "Appearance",
        Fields: []settings.Field{
            {
                Key:     "appearance.theme",
                Type:    settings.FieldSelect,
                Label:   "Theme",
                Default: "system",
                Options: []settings.SelectOption{
                    {Label: "System", Value: "system"},
                    {Label: "Light", Value: "light"},
                    {Label: "Dark", Value: "dark"},
                },
            },
        },
    }
}
```

**Schema features:**
- `Field.DynamicOptions` — select options that change based on another field's value
- `Field.Condition` — show/hide field based on another field's value
- `Field.Advanced` — render behind a "Show advanced" toggle
- `Field.Validation` — required, pattern, min/max length, min/max number
- `Group.ComputeFuncs` — server-side computed readonly fields

### `llm` — LLM Provider Management

Provider interface, factory pattern, and a built-in settings group for LLM configuration.

```go
import (
    "github.com/jrschumacher/wails-kit/llm"
    "github.com/jrschumacher/wails-kit/settings"
    _ "github.com/jrschumacher/wails-kit/llm/anthropic"
    _ "github.com/jrschumacher/wails-kit/llm/openai"
)

svc := settings.NewService(
    settings.WithAppName("my-app"),
    settings.WithGroup(llm.LLMSettingsGroup()),  // adds provider/model/advanced fields
)

mgr := llm.NewProviderManager(svc)

// Get the provider (lazy-initialized from settings)
provider, err := mgr.Provider()

// Stream a chat
provider.StreamChat(ctx, llm.ChatRequest{
    SystemPrompt: "You are helpful.",
    Messages:     []llm.ChatMessage{{Role: "user", Content: "Hello"}},
}, func(event llm.StreamEvent) {
    switch event.Type {
    case "delta":
        fmt.Print(event.Text)
    case "done":
        fmt.Println()
    }
})

// After settings change, reload the provider
mgr.Reload()
```

**Built-in providers:**
- `llm/anthropic` — Anthropic SDK with Cloudflare AI Gateway support
- `llm/openai` — OpenAI SDK with Cloudflare AI Gateway support
- `llm/mock` — Mock provider for testing

Providers self-register via `init()`. Import with blank identifier to activate.

**LLM settings group fields:**
- Provider selection (Anthropic / OpenAI)
- Model selection (dynamic by provider)
- Per-provider advanced: base URL, API key, API format, custom model ID
- Computed resolved model ID

### `llm/mock` — Test Helper

```go
import "github.com/jrschumacher/wails-kit/llm/mock"

p := &mock.Provider{
    Name:  "test",
    Model: "test-model",
    OnStreamChat: func(ctx context.Context, req llm.ChatRequest, handler func(llm.StreamEvent)) error {
        handler(llm.StreamEvent{Type: "delta", Text: "test response"})
        handler(llm.StreamEvent{Type: "done", StopReason: "end_turn"})
        return nil
    },
}
```

## Frontend Integration

The schema returned by `GetSchema()` is framework-agnostic JSON. Any frontend renders it with a generic loop:

```
for each group in schema.groups:
  render group heading
  for each field in group.fields:
    if field.advanced: group behind toggle
    if field.condition: check values[condition.field] in condition.equals
    if field.dynamicOptions: lookup options[values[dependsOn]]
    render input for field.type (text/password/select/toggle/number/computed)
```

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `ANTHROPIC_API_KEY` | Anthropic API key (used by SDK if no secret in settings) |
| `OPENAI_API_KEY` | OpenAI API key (used by SDK if no secret in settings) |
| `CF_AIG_AUTHORIZATION` | Cloudflare AI Gateway token |
