package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestNewContextBuilder_Defaults(t *testing.T) {
	cb := NewContextBuilder("You are helpful.")
	if cb.WindowSize != DefaultWindowSize {
		t.Errorf("expected window size %d, got %d", DefaultWindowSize, cb.WindowSize)
	}
	if cb.MaxTokens != DefaultMaxTokens {
		t.Errorf("expected max tokens %d, got %d", DefaultMaxTokens, cb.MaxTokens)
	}
	if cb.maxTopics != DefaultMaxTopics {
		t.Errorf("expected max topics %d, got %d", DefaultMaxTopics, cb.maxTopics)
	}
	if cb.truncateLength != DefaultTruncateLength {
		t.Errorf("expected truncate length %d, got %d", DefaultTruncateLength, cb.truncateLength)
	}
}

func TestNewContextBuilder_WithOptions(t *testing.T) {
	cb := NewContextBuilder("prompt",
		WithWindowSize(10),
		WithMaxTokens(8192),
		WithMaxTopics(5),
		WithTruncateLength(50),
	)
	if cb.WindowSize != 10 {
		t.Errorf("expected window size 10, got %d", cb.WindowSize)
	}
	if cb.MaxTokens != 8192 {
		t.Errorf("expected max tokens 8192, got %d", cb.MaxTokens)
	}
	if cb.maxTopics != 5 {
		t.Errorf("expected max topics 5, got %d", cb.maxTopics)
	}
	if cb.truncateLength != 50 {
		t.Errorf("expected truncate length 50, got %d", cb.truncateLength)
	}
}

func TestNewContextBuilder_InvalidOptionsIgnored(t *testing.T) {
	cb := NewContextBuilder("prompt",
		WithWindowSize(0),
		WithMaxTokens(-1),
		WithMaxTopics(0),
		WithTruncateLength(-5),
	)
	if cb.WindowSize != DefaultWindowSize {
		t.Errorf("expected default window size, got %d", cb.WindowSize)
	}
	if cb.MaxTokens != DefaultMaxTokens {
		t.Errorf("expected default max tokens, got %d", cb.MaxTokens)
	}
}

func TestWithModelBudget(t *testing.T) {
	cb := NewContextBuilder("prompt", WithModelBudget("claude-sonnet-4-6"))
	if cb.MaxTokens != 16384 {
		t.Errorf("expected max tokens 16384 from model budget, got %d", cb.MaxTokens)
	}
	if cb.effectiveContextWindow() != 200000 {
		t.Errorf("expected context window 200000 from model budget, got %d", cb.effectiveContextWindow())
	}
	if cb.WindowSize != DefaultWindowSize {
		t.Errorf("expected message window size to stay at default %d, got %d", DefaultWindowSize, cb.WindowSize)
	}
}

func TestWithModelBudget_UnknownModel(t *testing.T) {
	cb := NewContextBuilder("prompt", WithModelBudget("unknown-model"))
	if cb.MaxTokens != DefaultMaxTokens {
		t.Errorf("expected default max tokens for unknown model, got %d", cb.MaxTokens)
	}
}

func TestBuildSystemPrompt_NoWidgetContext(t *testing.T) {
	cb := NewContextBuilder("Base prompt.")
	if cb.BuildSystemPrompt() != "Base prompt." {
		t.Errorf("unexpected: %s", cb.BuildSystemPrompt())
	}
}

func TestBuildSystemPrompt_WithWidgetContext(t *testing.T) {
	cb := NewContextBuilder("Base prompt.")
	cb.SetWidgetContext("viewing issue ABC-123")
	result := cb.BuildSystemPrompt()
	if !strings.Contains(result, "Base prompt.") {
		t.Error("expected base prompt")
	}
	if !strings.Contains(result, "viewing issue ABC-123") {
		t.Error("expected widget context")
	}
}

func TestBuildSystemPrompt_Segments(t *testing.T) {
	cb := NewContextBuilder("ignored when segments exist")
	cb.AddSystemSegment("base", 0, "You are a helpful assistant.")
	cb.AddSystemSegment("tools", 20, "You have access to tools.")
	cb.AddSystemSegment("widgets", 10, "Current widget: dashboard")

	result := cb.BuildSystemPrompt()

	// Segments should be ordered by priority: base(0), widgets(10), tools(20)
	baseIdx := strings.Index(result, "helpful assistant")
	widgetIdx := strings.Index(result, "Current widget")
	toolIdx := strings.Index(result, "access to tools")

	if baseIdx == -1 || widgetIdx == -1 || toolIdx == -1 {
		t.Fatalf("missing segment in prompt: %s", result)
	}
	if baseIdx >= widgetIdx || widgetIdx >= toolIdx {
		t.Errorf("segments not in priority order: base=%d, widget=%d, tool=%d", baseIdx, widgetIdx, toolIdx)
	}
}

func TestBuildSystemPrompt_SegmentsWithWidgetContext(t *testing.T) {
	cb := NewContextBuilder("")
	cb.AddSystemSegment("base", 0, "Base prompt.")
	cb.SetWidgetContext("issue ABC-123")

	result := cb.BuildSystemPrompt()
	if !strings.Contains(result, "Base prompt.") {
		t.Error("expected base segment")
	}
	if !strings.Contains(result, "issue ABC-123") {
		t.Error("expected widget context")
	}
}

func TestAddSystemSegment_Replace(t *testing.T) {
	cb := NewContextBuilder("")
	cb.AddSystemSegment("ctx", 10, "old context")
	cb.AddSystemSegment("ctx", 10, "new context")

	result := cb.BuildSystemPrompt()
	if strings.Contains(result, "old context") {
		t.Error("old segment should have been replaced")
	}
	if !strings.Contains(result, "new context") {
		t.Error("expected new segment content")
	}
}

func TestRemoveSystemSegment(t *testing.T) {
	cb := NewContextBuilder("")
	cb.AddSystemSegment("base", 0, "base")
	cb.AddSystemSegment("extra", 10, "extra info")
	cb.RemoveSystemSegment("extra")

	result := cb.BuildSystemPrompt()
	if strings.Contains(result, "extra info") {
		t.Error("removed segment should not appear")
	}
}

func TestBuildMessages_BelowWindow(t *testing.T) {
	cb := NewContextBuilder("prompt", WithWindowSize(5))
	msgs := makeMessages(3)
	result := cb.BuildMessages(msgs)
	if len(result) != 3 {
		t.Errorf("expected 3 messages unchanged, got %d", len(result))
	}
}

func TestBuildMessages_SlidingWindow(t *testing.T) {
	cb := NewContextBuilder("prompt", WithWindowSize(3))
	msgs := makeMessages(10) // 10 messages, window of 3

	result := cb.BuildMessages(msgs)

	// Should have: summary + 3 recent messages (+ possible bridge)
	if len(result) < 4 {
		t.Fatalf("expected at least 4 messages (summary + 3 recent), got %d", len(result))
	}

	// First message should be the summary
	if !strings.Contains(result[0].Content, "Summary of earlier conversation") {
		t.Errorf("expected summary message, got: %s", result[0].Content)
	}
}

func TestBuildMessages_ToolUsePairIntegrity(t *testing.T) {
	cb := NewContextBuilder("prompt", WithWindowSize(2))

	msgs := []ChatMessage{
		{Role: "user", Content: "old message 1"},
		{Role: "user", Content: "old message 2"},
		{Role: "assistant", Content: "let me check", ToolUses: []ToolUseBlock{{ID: "t1", Name: "search"}}},
		{Role: "user", Content: "result", ToolResults: []ToolResult{{ToolUseID: "t1", Content: "found it"}}},
		{Role: "assistant", Content: "here you go"},
	}

	result := cb.BuildMessages(msgs)

	// The tool-result at index 3 should pull in the tool-use at index 2.
	hasToolUse := false
	hasToolResult := false
	for _, m := range result {
		if len(m.ToolUses) > 0 {
			hasToolUse = true
		}
		if len(m.ToolResults) > 0 {
			hasToolResult = true
		}
	}
	if hasToolResult && !hasToolUse {
		t.Error("tool-result is in window but tool-use was split off")
	}
}

func TestBuildRequest(t *testing.T) {
	cb := NewContextBuilder("system prompt", WithMaxTokens(2048))

	msgs := makeMessages(3)
	req := cb.BuildRequest(msgs)

	if req.SystemPrompt != "system prompt" {
		t.Errorf("unexpected system prompt: %s", req.SystemPrompt)
	}
	if req.MaxTokens != 2048 {
		t.Errorf("expected max tokens 2048, got %d", req.MaxTokens)
	}
	if len(req.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(req.Messages))
	}
}

func TestSummarizeMessages_TruncatesLongContent(t *testing.T) {
	longContent := strings.Repeat("x", 200)
	msgs := []ChatMessage{{Role: "user", Content: longContent}}
	cb := NewContextBuilder("")
	summary := cb.summarizeMessages(msgs)

	if !strings.Contains(summary, "...") {
		t.Error("expected long content to be truncated with ...")
	}
}

func TestSummarizeMessages_CustomTruncateLength(t *testing.T) {
	content := strings.Repeat("x", 60)
	cb := NewContextBuilder("", WithTruncateLength(50))
	summary := cb.summarizeMessages([]ChatMessage{{Role: "user", Content: content}})

	if !strings.Contains(summary, "...") {
		t.Error("expected content to be truncated at custom length")
	}

	// With default length (100), 60 chars should not be truncated
	cbDefault := NewContextBuilder("")
	summaryDefault := cbDefault.summarizeMessages([]ChatMessage{{Role: "user", Content: content}})
	if strings.Contains(summaryDefault, "...") {
		t.Error("60 chars should not be truncated with default 100 length")
	}
}

func TestSummarizeMessages_IncludesToolCalls(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "assistant", ToolUses: []ToolUseBlock{{Name: "search_issues"}}},
	}
	cb := NewContextBuilder("")
	summary := cb.summarizeMessages(msgs)
	if !strings.Contains(summary, "Called search_issues") {
		t.Errorf("expected tool call in summary, got: %s", summary)
	}
}

func TestSummarizeMessages_CapsAtMaxTopics(t *testing.T) {
	var msgs []ChatMessage
	for i := 0; i < 20; i++ {
		msgs = append(msgs, ChatMessage{Role: "user", Content: fmt.Sprintf("topic %d", i)})
	}

	// Default: 8 topics
	cb := NewContextBuilder("")
	summary := cb.summarizeMessages(msgs)
	count := strings.Count(summary, "topic")
	if count > 8 {
		t.Errorf("expected at most 8 topics, found %d", count)
	}

	// Custom: 3 topics
	cb3 := NewContextBuilder("", WithMaxTopics(3))
	summary3 := cb3.summarizeMessages(msgs)
	count3 := strings.Count(summary3, "topic")
	if count3 > 3 {
		t.Errorf("expected at most 3 topics, found %d", count3)
	}
}

func TestSummarizeMessages_Empty(t *testing.T) {
	cb := NewContextBuilder("")
	summary := cb.summarizeMessages(nil)
	if !strings.Contains(summary, "no specific") {
		t.Errorf("expected empty summary message, got: %s", summary)
	}
}

func TestWithTokenCounter(t *testing.T) {
	// Simple token counter: 1 token per word
	wordCounter := func(s string) int {
		if s == "" {
			return 0
		}
		return len(strings.Fields(s))
	}

	// Set window size to token budget (context window in tokens)
	// System prompt = 3 words, MaxTokens = 10, budget = 100 - 3 - 10 = 87
	cb := NewContextBuilder("You are helpful.",
		WithTokenCounter(wordCounter),
		WithWindowSize(100),
		WithMaxTokens(10),
	)

	msgs := makeMessages(50) // "message 0" through "message 49" = 2 words each
	result := cb.BuildMessages(msgs)

	// Should have summary + some recent messages fitting in budget
	if len(result) < 2 {
		t.Fatalf("expected at least summary + 1 message, got %d", len(result))
	}
	if !strings.Contains(result[0].Content, "Summary of earlier conversation") {
		t.Errorf("expected summary as first message, got: %s", result[0].Content)
	}

	// Should trim a significant number of messages (not return all 50)
	if len(result) >= 50 {
		t.Error("expected token counter to trim messages")
	}
	// The windowed messages (excluding summary/bridge) should be fewer than original
	if len(result) > 47 {
		t.Errorf("expected significant trimming, got %d messages", len(result))
	}
}

func TestWithTokenCounterAndModelBudget_UsesModelContextWindow(t *testing.T) {
	wordCounter := func(s string) int {
		if s == "" {
			return 0
		}
		return len(strings.Fields(s))
	}

	cb := NewContextBuilder("You are helpful.",
		WithTokenCounter(wordCounter),
		WithModelBudget("gpt-4o"),
	)

	msgs := makeMessages(50)
	result := cb.BuildMessages(msgs)

	if len(result) != len(msgs) {
		t.Fatalf("expected all messages to fit within model context budget, got %d of %d", len(result), len(msgs))
	}
}

func TestWithTokenCounter_AllMessagesFit(t *testing.T) {
	wordCounter := func(s string) int {
		if s == "" {
			return 0
		}
		return len(strings.Fields(s))
	}

	// Large budget that fits everything
	cb := NewContextBuilder("Hi",
		WithTokenCounter(wordCounter),
		WithWindowSize(10000),
		WithMaxTokens(10),
	)

	msgs := makeMessages(5)
	result := cb.BuildMessages(msgs)

	// All messages should be returned unchanged
	if len(result) != 5 {
		t.Errorf("expected all 5 messages unchanged, got %d", len(result))
	}
}

func makeMessages(n int) []ChatMessage {
	msgs := make([]ChatMessage, n)
	for i := 0; i < n; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs[i] = ChatMessage{
			Role:    role,
			Content: fmt.Sprintf("message %d", i),
		}
	}
	return msgs
}

// Verify ChatMessage JSON serialization (used by context builder callers)
func TestChatMessage_JSON(t *testing.T) {
	msg := ChatMessage{
		Role:    "assistant",
		Content: "hello",
		ToolUses: []ToolUseBlock{
			{ID: "t1", Name: "search", Input: json.RawMessage(`{"q":"test"}`)},
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var out ChatMessage
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Role != "assistant" || len(out.ToolUses) != 1 {
		t.Errorf("round-trip failed: %+v", out)
	}
}
