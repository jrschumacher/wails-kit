package llm

import (
	"context"
	"encoding/json"
)

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type ToolUseBlock struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error"`
}

type ChatMessage struct {
	Role        string         `json:"role"`
	Content     string         `json:"content"`
	ToolUses    []ToolUseBlock `json:"tool_uses,omitempty"`
	ToolResults []ToolResult   `json:"tool_results,omitempty"`
}

type ChatRequest struct {
	SystemPrompt string
	Messages     []ChatMessage
	MaxTokens    int
	Tools        []ToolDefinition
}

type ProviderConfig struct {
	BaseURL string
	APIKey  string
}

type StreamEvent struct {
	Type       string // "delta", "done", "error", "tool_use"
	Text       string
	Err        error
	ToolUses   []ToolUseBlock
	StopReason string // e.g. "end_turn", "tool_use", "max_tokens"
}

type Provider interface {
	StreamChat(ctx context.Context, req ChatRequest, handler func(StreamEvent)) error
	ProviderName() string
	ModelID() string
}
