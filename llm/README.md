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
cb := llm.NewContextBuilder("You are a helpful assistant.",
    llm.WithWindowSize(20),     // keep last 20 messages (default)
    llm.WithMaxTokens(4096),    // default
    llm.WithMaxTopics(8),       // max topics in summary (default)
    llm.WithTruncateLength(100),// max chars per topic (default)
)

// Use a model's registered budget for automatic max tokens
cb = llm.NewContextBuilder("prompt", llm.WithModelBudget("claude-sonnet-4-6"))

// Optionally add context to the system prompt
cb.SetWidgetContext("User is viewing issue ABC-123")

// Build a request from full conversation history
req := cb.BuildRequest(allMessages)
provider.StreamChat(ctx, req, handler)
```

### System prompt composition

Compose system prompts from multiple sources with priority ordering:

```go
cb := llm.NewContextBuilder("")
cb.AddSystemSegment("base", 0, "You are a helpful assistant.")
cb.AddSystemSegment("widgets", 10, "Current widget: dashboard")
cb.AddSystemSegment("tools", 20, "You have access to: search, calculator")

// Segments are sorted by priority (lower first) and joined
prompt := cb.BuildSystemPrompt()
// "You are a helpful assistant.\n\nCurrent widget: dashboard\n\nYou have access to: search, calculator"

// Replace or remove segments dynamically
cb.AddSystemSegment("widgets", 10, "Current widget: settings") // replaces existing
cb.RemoveSystemSegment("tools")
```

### Token-based windowing

For precise token budgeting, provide a token counter:

```go
cb := llm.NewContextBuilder("You are helpful.",
    llm.WithTokenCounter(myTokenCounter),
    llm.WithWindowSize(200000),  // set to model's context window in tokens
    llm.WithMaxTokens(16384),
)
```

When a token counter is set, the builder fits as many recent messages as possible within the token budget (context window minus system prompt minus max reply tokens) instead of using a fixed message count.

### Per-model token budgets

Register model budgets so the context builder can size automatically:

```go
// Built-in budgets are registered for Claude and GPT models.
// Register custom models:
llm.RegisterModelBudget("my-model", llm.ModelBudget{
    ContextWindow:   32000,
    DefaultMaxReply: 2048,
})

// Query a model's budget:
budget, ok := llm.GetModelBudget("claude-sonnet-4-6")
// budget.ContextWindow == 200000, budget.DefaultMaxReply == 16384
```

### Behavior

- Sliding window keeps the last N messages (or token-budget-based window when a counter is set)
- Tool-use / tool-result pairs are kept atomic (never split across the window boundary)
- Older messages beyond the window are summarized into a synthetic message
- Summary topic count and truncation length are configurable

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
