package llm

import (
	"fmt"
	"strings"
)

const DefaultWindowSize = 20

// ContextBuilder manages conversation history for LLM chat with bounded context.
// It applies a sliding window to keep recent messages and summarizes older ones.
type ContextBuilder struct {
	WindowSize   int
	SystemPrompt string
	MaxTokens    int
	widgetCtx    string
}

// NewContextBuilder creates a ContextBuilder with defaults.
func NewContextBuilder(systemPrompt string) *ContextBuilder {
	return &ContextBuilder{
		WindowSize:   DefaultWindowSize,
		SystemPrompt: systemPrompt,
		MaxTokens:    4096,
	}
}

// SetWidgetContext appends additional context to the system prompt.
func (cb *ContextBuilder) SetWidgetContext(ctx string) {
	cb.widgetCtx = ctx
}

// BuildSystemPrompt returns the system prompt, optionally with widget context appended.
func (cb *ContextBuilder) BuildSystemPrompt() string {
	if cb.widgetCtx == "" {
		return cb.SystemPrompt
	}
	return fmt.Sprintf("%s\n\n## Current Context\n%s", cb.SystemPrompt, cb.widgetCtx)
}

// BuildMessages applies a sliding window to the message history.
// It keeps the most recent WindowSize messages, summarizes older ones,
// and ensures tool-use/tool-result pairs are never split.
func (cb *ContextBuilder) BuildMessages(messages []ChatMessage) []ChatMessage {
	if len(messages) <= cb.WindowSize {
		return messages
	}

	windowStart := len(messages) - cb.WindowSize

	// Keep tool-use/tool-result pairs atomic: if the window starts on a
	// tool-result message, pull its preceding tool-use into the window.
	if windowStart > 0 && len(messages[windowStart].ToolResults) > 0 {
		windowStart--
	}

	older := messages[:windowStart]
	recent := messages[windowStart:]

	summary := summarizeMessages(older)

	result := make([]ChatMessage, 0, len(recent)+2)
	result = append(result, ChatMessage{
		Role:    "user",
		Content: fmt.Sprintf("[Summary of earlier conversation: %s]", summary),
	})

	// Ensure alternating roles: if recent starts with assistant, add a bridge
	if len(recent) > 0 && recent[0].Role != "user" {
		result = append(result, ChatMessage{
			Role:    "assistant",
			Content: "Understood, continuing from the summary.",
		})
	}
	result = append(result, recent...)

	return result
}

// BuildRequest assembles a ChatRequest from the conversation history.
func (cb *ContextBuilder) BuildRequest(messages []ChatMessage) ChatRequest {
	maxTokens := cb.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return ChatRequest{
		SystemPrompt: cb.BuildSystemPrompt(),
		Messages:     cb.BuildMessages(messages),
		MaxTokens:    maxTokens,
	}
}

func summarizeMessages(messages []ChatMessage) string {
	var topics []string
	for _, m := range messages {
		if m.Role == "user" && len(m.Content) > 0 {
			content := m.Content
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			topics = append(topics, content)
		}
		if m.Role == "assistant" && len(m.ToolUses) > 0 {
			for _, tu := range m.ToolUses {
				topics = append(topics, fmt.Sprintf("Called %s", tu.Name))
			}
		}
	}
	if len(topics) == 0 {
		return "Previous conversation with no specific user queries."
	}
	if len(topics) > 8 {
		topics = topics[:8]
	}
	return fmt.Sprintf("The user previously discussed: %s", strings.Join(topics, "; "))
}
