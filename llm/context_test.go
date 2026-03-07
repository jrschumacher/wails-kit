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
	if cb.MaxTokens != 4096 {
		t.Errorf("expected max tokens 4096, got %d", cb.MaxTokens)
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

func TestBuildMessages_BelowWindow(t *testing.T) {
	cb := NewContextBuilder("prompt")
	cb.WindowSize = 5
	msgs := makeMessages(3)
	result := cb.BuildMessages(msgs)
	if len(result) != 3 {
		t.Errorf("expected 3 messages unchanged, got %d", len(result))
	}
}

func TestBuildMessages_SlidingWindow(t *testing.T) {
	cb := NewContextBuilder("prompt")
	cb.WindowSize = 3
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
	cb := NewContextBuilder("prompt")
	cb.WindowSize = 2

	msgs := []ChatMessage{
		{Role: "user", Content: "old message 1"},
		{Role: "user", Content: "old message 2"},
		{Role: "assistant", Content: "let me check", ToolUses: []ToolUseBlock{{ID: "t1", Name: "search"}}},
		{Role: "user", Content: "result", ToolResults: []ToolResult{{ToolUseID: "t1", Content: "found it"}}},
		{Role: "assistant", Content: "here you go"},
	}

	result := cb.BuildMessages(msgs)

	// The tool-result at index 3 should pull in the tool-use at index 2.
	// Window of 2 from end = [3,4], but 3 has tool results, so pull back to [2,3,4].
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
	cb := NewContextBuilder("system prompt")
	cb.MaxTokens = 2048

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
	summary := summarizeMessages(msgs)

	if !strings.Contains(summary, "...") {
		t.Error("expected long content to be truncated with ...")
	}
}

func TestSummarizeMessages_IncludesToolCalls(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "assistant", ToolUses: []ToolUseBlock{{Name: "search_issues"}}},
	}
	summary := summarizeMessages(msgs)
	if !strings.Contains(summary, "Called search_issues") {
		t.Errorf("expected tool call in summary, got: %s", summary)
	}
}

func TestSummarizeMessages_CapsAt8Topics(t *testing.T) {
	var msgs []ChatMessage
	for i := 0; i < 20; i++ {
		msgs = append(msgs, ChatMessage{Role: "user", Content: fmt.Sprintf("topic %d", i)})
	}
	summary := summarizeMessages(msgs)

	// Should only mention 8 topics
	count := strings.Count(summary, "topic")
	if count > 8 {
		t.Errorf("expected at most 8 topics, found %d", count)
	}
}

func TestSummarizeMessages_Empty(t *testing.T) {
	summary := summarizeMessages(nil)
	if !strings.Contains(summary, "no specific") {
		t.Errorf("expected empty summary message, got: %s", summary)
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
