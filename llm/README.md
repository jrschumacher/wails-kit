# llm

LLM provider management for Wails v3 apps. Provides a provider interface with factory pattern, a provider manager, context window builder, and a built-in settings group.

## Usage

```go
import (
    "github.com/jrschumacher/wails-kit/llm"
    "github.com/jrschumacher/wails-kit/settings"
    _ "github.com/jrschumacher/wails-kit/llm/anthropic"
    _ "github.com/jrschumacher/wails-kit/llm/openai"
)

svc := settings.NewService(
    settings.WithAppName("my-app"),
    settings.WithGroup(llm.LLMSettingsGroup()),
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

## Provider interface

```go
type Provider interface {
    StreamChat(ctx context.Context, req ChatRequest, handler func(StreamEvent)) error
    Name() string
    Model() string
}
```

## Built-in providers

| Package | Provider | Features |
|---------|----------|----------|
| `llm/anthropic` | Anthropic | Claude models, Cloudflare AI Gateway support |
| `llm/openai` | OpenAI | GPT models, Cloudflare AI Gateway support |
| `llm/mock` | Mock | Test helper with configurable responses |

Providers self-register via `init()`. Import with blank identifier to activate:

```go
import _ "github.com/jrschumacher/wails-kit/llm/anthropic"
```

## Settings group

`llm.LLMSettingsGroup()` returns a settings group with:

- Provider selection (Anthropic / OpenAI)
- Model selection (dynamic by provider)
- Per-provider advanced settings: base URL, API key, API format, custom model ID
- Computed resolved model ID

## Context window builder

Manages conversation history with bounded context windows:

```go
cb := llm.NewContextBuilder("You are a helpful assistant.")
cb.WindowSize = 20  // keep last 20 messages (default)
cb.MaxTokens = 4096 // default

// Optionally add context to the system prompt
cb.SetWidgetContext("User is viewing issue ABC-123")

// Build a request from full conversation history
req := cb.BuildRequest(allMessages)
provider.StreamChat(ctx, req, handler)
```

### Behavior

- Sliding window keeps the last N messages
- Tool-use / tool-result pairs are kept atomic (never split across the window boundary)
- Older messages beyond the window are summarized into a synthetic message
- Summary caps at 8 topics with content truncated to 100 chars

## Mock provider

For testing:

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

## Environment variables

| Variable | Purpose |
|----------|---------|
| `ANTHROPIC_API_KEY` | Anthropic API key (used by SDK if no secret in settings) |
| `OPENAI_API_KEY` | OpenAI API key (used by SDK if no secret in settings) |
| `CF_AIG_AUTHORIZATION` | Cloudflare AI Gateway token |
