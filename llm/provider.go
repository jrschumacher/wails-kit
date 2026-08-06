package llm

import (
	"context"
	"encoding/json"
)

// Role identifies the author of a ChatMessage.
//
// The underlying type is string, so existing consumers that assign untyped
// string literals (ChatMessage{Role: "user"}) continue to compile.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

// EventType identifies the kind of StreamEvent delivered to a stream handler.
//
// The underlying type is string, so existing consumers that assign or switch on
// untyped string literals continue to compile.
type EventType string

const (
	// EventDelta carries an incremental chunk of assistant text in Text.
	EventDelta EventType = "delta"
	// EventDone is the terminal event. Exactly one is emitted per successful
	// stream, carrying the StopReason. Providers must emit it for every
	// terminal condition, not just a natural end of turn.
	EventDone EventType = "done"
	// EventError carries a stream failure in Err.
	EventError EventType = "error"
	// EventToolUse carries the fully accumulated tool calls in ToolUses. It is
	// emitted before the terminal EventDone.
	EventToolUse EventType = "tool_use"
)

// StopReason describes why the model stopped generating.
//
// The underlying type is string, so existing consumers that assign or switch on
// untyped string literals continue to compile.
type StopReason string

const (
	// StopReasonEndTurn is a natural end of the assistant turn.
	StopReasonEndTurn StopReason = "end_turn"
	// StopReasonToolUse means the model is waiting on tool results.
	StopReasonToolUse StopReason = "tool_use"
	// StopReasonMaxTokens means the token budget was exhausted mid-response.
	StopReasonMaxTokens StopReason = "max_tokens"
	// StopReasonContentFilter means the provider truncated the response.
	StopReasonContentFilter StopReason = "content_filter"
	// StopReasonStopSequence means a configured stop sequence was hit.
	StopReasonStopSequence StopReason = "stop_sequence"
	// StopReasonUnknown is used when the provider reports a reason this package
	// does not model; the raw provider value is not preserved.
	StopReasonUnknown StopReason = "unknown"
)

// ToolDefinition declares a tool the model may call.
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// InputSchema is a JSON Schema object describing the tool's parameters. It
	// is passed to the provider as written, so it may be a hand-built Go map or
	// the result of decoding a schema document; a "required" list works both as
	// []string and as the []any that JSON decoding produces.
	InputSchema map[string]any `json:"input_schema"`
}

// ToolUseBlock is one tool call the model asked for.
type ToolUseBlock struct {
	// ID correlates this call with the ToolResult that answers it.
	ID   string `json:"id"`
	Name string `json:"name"`
	// Input is the tool's arguments as a JSON document. It is complete: a
	// provider only emits EventToolUse once the fragments have been reassembled.
	Input json.RawMessage `json:"input"`
}

// ToolResult is the answer to one ToolUseBlock, sent back in the next request.
type ToolResult struct {
	// ToolUseID must match the ToolUseBlock.ID being answered.
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	// IsError marks a failed tool call. Providers whose wire format has no
	// error flag surface it by prefixing Content instead.
	IsError bool `json:"is_error"`
}

// ChatMessage is one turn of a conversation.
//
// A turn carries either text, tool calls, or tool results. When more than one
// is set, ToolResults wins over ToolUses, and both win over Content — except
// that a turn with ToolUses also sends Content as preamble text.
type ChatMessage struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
	// ToolUses are the calls an assistant turn asked for.
	ToolUses []ToolUseBlock `json:"tool_uses,omitempty"`
	// ToolResults are the answers a user turn supplies.
	ToolResults []ToolResult `json:"tool_results,omitempty"`
}

// ChatRequest is one call to StreamChat.
type ChatRequest struct {
	// SystemPrompt is prepended to the conversation. A RoleSystem message in
	// Messages is equivalent, and comes after this one.
	SystemPrompt string
	Messages     []ChatMessage
	// MaxTokens caps the response length. Zero means "the provider's default",
	// which differs by provider: Anthropic's API requires a limit, so
	// llm/anthropic substitutes 4096; llm/openai leaves it unset and lets the
	// model decide. Set it explicitly if the cap matters.
	MaxTokens int
	// Tools the model may call. An empty list disables tool use.
	Tools []ToolDefinition
}

// ProviderConfig is the connection configuration for one provider.
type ProviderConfig struct {
	// BaseURL overrides the provider's default endpoint, for a proxy or a
	// gateway. Empty means the SDK default.
	BaseURL string
	// APIKey authenticates to the provider. Empty leaves the SDK's own
	// environment-variable fallback in place. It must never be
	// settings.SecretSentinel — see NewProviderFromValues.
	APIKey string
}

// StreamEvent is one event from a streaming response. Which fields are set
// depends on Type; see the EventType constants and the package comment for the
// full contract.
type StreamEvent struct {
	Type EventType
	// Text is an incremental chunk of assistant output, on EventDelta.
	Text string
	// Err is the stream failure, on EventError.
	Err error
	// ToolUses are the fully accumulated tool calls, on EventToolUse.
	ToolUses []ToolUseBlock
	// StopReason is why the model stopped, on EventDone.
	StopReason StopReason
}

// Provider is a model endpoint that can stream a chat response.
//
// Implementations live in subpackages and register themselves with
// RegisterProvider; see the package comment.
type Provider interface {
	// StreamChat streams a response, calling handler synchronously on the
	// calling goroutine for each event and returning when the stream ends.
	// handler is never called after StreamChat returns.
	//
	// Every stream ends with exactly one terminal event: EventDone on success
	// (StreamChat returns nil), or EventError carrying the same error StreamChat
	// returns. Cancelling ctx ends the stream as an error.
	StreamChat(ctx context.Context, req ChatRequest, handler func(StreamEvent)) error

	// ProviderName is the name the provider is registered under.
	ProviderName() string

	// ModelID is the model this provider was built for, after any custom-model
	// override and transport-specific rewriting.
	ModelID() string
}
