package llm

import (
	"fmt"
	"sort"
	"strings"
)

const (
	DefaultWindowSize     = 20
	DefaultMaxTokens      = 4096
	DefaultMaxTopics      = 8
	DefaultTruncateLength = 100
)

// SystemSegment is a named block of system prompt text with a priority.
// Lower priority values appear first in the composed prompt.
type SystemSegment struct {
	Name     string
	Priority int
	Content  string
}

// ContextBuilderOption configures a ContextBuilder.
type ContextBuilderOption func(*ContextBuilder)

// ContextBuilder manages conversation history for LLM chat with bounded context.
// It applies a sliding window to keep recent messages and summarizes older ones.
type ContextBuilder struct {
	WindowSize     int
	SystemPrompt   string
	MaxTokens      int
	contextWindow  int
	maxTopics      int
	truncateLength int
	widgetCtx      string
	segments       []SystemSegment
	tokenCounter   func(string) int
}

// NewContextBuilder creates a ContextBuilder with defaults and applies options.
func NewContextBuilder(systemPrompt string, opts ...ContextBuilderOption) *ContextBuilder {
	cb := &ContextBuilder{
		WindowSize:     DefaultWindowSize,
		SystemPrompt:   systemPrompt,
		MaxTokens:      DefaultMaxTokens,
		maxTopics:      DefaultMaxTopics,
		truncateLength: DefaultTruncateLength,
	}
	for _, opt := range opts {
		opt(cb)
	}
	return cb
}

// WithWindowSize sets the sliding window size.
func WithWindowSize(n int) ContextBuilderOption {
	return func(cb *ContextBuilder) {
		if n > 0 {
			cb.WindowSize = n
		}
	}
}

// WithMaxTokens sets the maximum reply tokens.
func WithMaxTokens(n int) ContextBuilderOption {
	return func(cb *ContextBuilder) {
		if n > 0 {
			cb.MaxTokens = n
		}
	}
}

// WithMaxTopics sets the maximum number of topics in conversation summaries.
func WithMaxTopics(n int) ContextBuilderOption {
	return func(cb *ContextBuilder) {
		if n > 0 {
			cb.maxTopics = n
		}
	}
}

// WithTruncateLength sets the max character length for each topic in summaries.
func WithTruncateLength(n int) ContextBuilderOption {
	return func(cb *ContextBuilder) {
		if n > 0 {
			cb.truncateLength = n
		}
	}
}

// WithModelBudget configures the builder using a registered model's budget.
// It sets MaxTokens to the model's DefaultMaxReply if one is registered.
func WithModelBudget(modelID string) ContextBuilderOption {
	return func(cb *ContextBuilder) {
		if budget, ok := GetModelBudget(modelID); ok {
			if budget.ContextWindow > 0 {
				cb.contextWindow = budget.ContextWindow
			}
			if budget.DefaultMaxReply > 0 {
				cb.MaxTokens = budget.DefaultMaxReply
			}
		}
	}
}

// WithTokenCounter sets a function that counts tokens in a string.
// When set, the context builder uses token-based budgeting for the sliding window
// instead of message-count-based windowing.
func WithTokenCounter(fn func(string) int) ContextBuilderOption {
	return func(cb *ContextBuilder) {
		cb.tokenCounter = fn
	}
}

// AddSystemSegment adds a named segment to the system prompt composition pipeline.
// Segments are ordered by priority (lower values first). If a segment with the
// same name already exists, it is replaced.
func (cb *ContextBuilder) AddSystemSegment(name string, priority int, content string) {
	for i, seg := range cb.segments {
		if seg.Name == name {
			cb.segments[i] = SystemSegment{Name: name, Priority: priority, Content: content}
			return
		}
	}
	cb.segments = append(cb.segments, SystemSegment{Name: name, Priority: priority, Content: content})
}

// RemoveSystemSegment removes a named segment from the composition pipeline.
func (cb *ContextBuilder) RemoveSystemSegment(name string) {
	for i, seg := range cb.segments {
		if seg.Name == name {
			cb.segments = append(cb.segments[:i], cb.segments[i+1:]...)
			return
		}
	}
}

// SetWidgetContext appends additional context to the system prompt.
func (cb *ContextBuilder) SetWidgetContext(ctx string) {
	cb.widgetCtx = ctx
}

// BuildSystemPrompt returns the composed system prompt.
// If segments are defined, they are sorted by priority and joined.
// Otherwise, falls back to the base SystemPrompt with optional widget context.
func (cb *ContextBuilder) BuildSystemPrompt() string {
	var base string
	if len(cb.segments) > 0 {
		sorted := make([]SystemSegment, len(cb.segments))
		copy(sorted, cb.segments)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Priority < sorted[j].Priority
		})
		parts := make([]string, len(sorted))
		for i, seg := range sorted {
			parts[i] = seg.Content
		}
		base = strings.Join(parts, "\n\n")
	} else {
		base = cb.SystemPrompt
	}

	if cb.widgetCtx == "" {
		return base
	}
	return fmt.Sprintf("%s\n\n## Current Context\n%s", base, cb.widgetCtx)
}

// BuildMessages applies a sliding window to the message history.
// It keeps the most recent WindowSize messages, summarizes older ones,
// and ensures tool-use/tool-result pairs are never split.
//
// If a token counter is set, windowing is based on token budget instead of
// message count. The builder fits as many recent messages as possible within
// the model's context window (minus system prompt and max reply tokens).
func (cb *ContextBuilder) BuildMessages(messages []ChatMessage) []ChatMessage {
	if len(messages) <= cb.WindowSize && cb.tokenCounter == nil {
		return messages
	}

	windowStart := cb.computeWindowStart(messages)
	if windowStart <= 0 {
		return messages
	}

	older := messages[:windowStart]
	recent := messages[windowStart:]

	summary := cb.summarizeMessages(older)

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

// computeWindowStart determines where the sliding window begins.
func (cb *ContextBuilder) computeWindowStart(messages []ChatMessage) int {
	if cb.tokenCounter != nil {
		return cb.computeTokenWindowStart(messages)
	}

	if len(messages) <= cb.WindowSize {
		return 0
	}

	windowStart := len(messages) - cb.WindowSize
	return cb.adjustForToolPairs(messages, windowStart)
}

// computeTokenWindowStart finds the window start using token counting.
func (cb *ContextBuilder) computeTokenWindowStart(messages []ChatMessage) int {
	// Determine available token budget for messages
	systemTokens := cb.tokenCounter(cb.BuildSystemPrompt())
	budget := cb.effectiveContextWindow() - systemTokens - cb.effectiveMaxTokens()
	if budget <= 0 {
		// Not enough room; keep only the last message
		if len(messages) > 0 {
			return len(messages) - 1
		}
		return 0
	}

	// Walk backwards, accumulating tokens
	used := 0
	windowStart := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := cb.countMessageTokens(messages[i])
		if used+msgTokens > budget {
			break
		}
		used += msgTokens
		windowStart = i
	}

	if windowStart >= len(messages) {
		windowStart = len(messages) - 1
	}

	return cb.adjustForToolPairs(messages, windowStart)
}

// effectiveContextWindow returns the context window size for token budgeting.
func (cb *ContextBuilder) effectiveContextWindow() int {
	if cb.contextWindow > 0 {
		return cb.contextWindow
	}
	// Fall back to WindowSize for callers that set token budgets manually.
	return cb.WindowSize
}

// effectiveMaxTokens returns the max reply tokens.
func (cb *ContextBuilder) effectiveMaxTokens() int {
	if cb.MaxTokens > 0 {
		return cb.MaxTokens
	}
	return DefaultMaxTokens
}

// countMessageTokens counts tokens in a single message.
func (cb *ContextBuilder) countMessageTokens(msg ChatMessage) int {
	tokens := cb.tokenCounter(msg.Content)
	for _, tu := range msg.ToolUses {
		tokens += cb.tokenCounter(tu.Name)
		tokens += cb.tokenCounter(string(tu.Input))
	}
	for _, tr := range msg.ToolResults {
		tokens += cb.tokenCounter(tr.Content)
	}
	return tokens
}

// adjustForToolPairs ensures tool-use/tool-result pairs are not split.
func (cb *ContextBuilder) adjustForToolPairs(messages []ChatMessage, windowStart int) int {
	if windowStart > 0 && len(messages[windowStart].ToolResults) > 0 {
		windowStart--
	}
	return windowStart
}

// BuildRequest assembles a ChatRequest from the conversation history.
func (cb *ContextBuilder) BuildRequest(messages []ChatMessage) ChatRequest {
	maxTokens := cb.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	return ChatRequest{
		SystemPrompt: cb.BuildSystemPrompt(),
		Messages:     cb.BuildMessages(messages),
		MaxTokens:    maxTokens,
	}
}

func (cb *ContextBuilder) summarizeMessages(messages []ChatMessage) string {
	var topics []string
	for _, m := range messages {
		if m.Role == "user" && len(m.Content) > 0 {
			content := m.Content
			if len(content) > cb.truncateLength {
				content = content[:cb.truncateLength] + "..."
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
	if len(topics) > cb.maxTopics {
		topics = topics[:cb.maxTopics]
	}
	return fmt.Sprintf("The user previously discussed: %s", strings.Join(topics, "; "))
}
