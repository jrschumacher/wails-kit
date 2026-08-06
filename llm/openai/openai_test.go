package openai

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/jrschumacher/wails-kit/llm"
)

func assertJSONEqual(t *testing.T, gotRaw []byte, want string) {
	t.Helper()
	var got, expected any
	if err := json.Unmarshal(gotRaw, &got); err != nil {
		t.Fatalf("unmarshal got: %v (raw: %s)", err, gotRaw)
	}
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("JSON mismatch\n got: %s\nwant: %s", gotRaw, want)
	}
}

func TestBuildMessages(t *testing.T) {
	tests := []struct {
		name         string
		systemPrompt string
		messages     []llm.ChatMessage
		want         string
		wantErr      bool
	}{
		{
			name:     "empty",
			messages: nil,
			want:     `null`,
		},
		{
			name:         "system prompt is prepended",
			systemPrompt: "be helpful",
			messages:     []llm.ChatMessage{{Role: llm.RoleUser, Content: "hi"}},
			want: `[{"role":"system","content":"be helpful"},` +
				`{"role":"user","content":"hi"}]`,
		},
		{
			name: "user and assistant",
			messages: []llm.ChatMessage{
				{Role: llm.RoleUser, Content: "hi"},
				{Role: llm.RoleAssistant, Content: "yo"},
			},
			want: `[{"role":"user","content":"hi"},{"role":"assistant","content":"yo"}]`,
		},
		{
			name:     "system role in the message list",
			messages: []llm.ChatMessage{{Role: llm.RoleSystem, Content: "rules"}},
			want:     `[{"role":"system","content":"rules"}]`,
		},
		{
			// Regression: tool uses used to be dropped entirely.
			name: "assistant tool calls with preamble text",
			messages: []llm.ChatMessage{{
				Role:    llm.RoleAssistant,
				Content: "let me check",
				ToolUses: []llm.ToolUseBlock{
					{ID: "tu_1", Name: "get_weather", Input: json.RawMessage(`{"city":"NYC"}`)},
				},
			}},
			want: `[{"role":"assistant","content":"let me check","tool_calls":[` +
				`{"id":"tu_1","type":"function",` +
				`"function":{"name":"get_weather","arguments":"{\"city\":\"NYC\"}"}}]}]`,
		},
		{
			name: "assistant tool calls without text",
			messages: []llm.ChatMessage{{
				Role:     llm.RoleAssistant,
				ToolUses: []llm.ToolUseBlock{{ID: "tu_1", Name: "f", Input: json.RawMessage(`{}`)}},
			}},
			want: `[{"role":"assistant","tool_calls":[` +
				`{"id":"tu_1","type":"function","function":{"name":"f","arguments":"{}"}}]}]`,
		},
		{
			name: "assistant tool call with empty input becomes an empty object",
			messages: []llm.ChatMessage{{
				Role:     llm.RoleAssistant,
				ToolUses: []llm.ToolUseBlock{{ID: "tu_1", Name: "f"}},
			}},
			want: `[{"role":"assistant","tool_calls":[` +
				`{"id":"tu_1","type":"function","function":{"name":"f","arguments":"{}"}}]}]`,
		},
		{
			// Regression: tool results used to be dropped entirely.
			name: "tool results become one tool message each",
			messages: []llm.ChatMessage{{
				Role: llm.RoleUser,
				ToolResults: []llm.ToolResult{
					{ToolUseID: "tu_1", Content: "sunny"},
					{ToolUseID: "tu_2", Content: "boom", IsError: true},
				},
			}},
			want: `[{"role":"tool","tool_call_id":"tu_1","content":"sunny"},` +
				`{"role":"tool","tool_call_id":"tu_2","content":"Error: boom"}]`,
		},
		{
			name: "full tool round trip",
			messages: []llm.ChatMessage{
				{Role: llm.RoleUser, Content: "weather?"},
				{
					Role:     llm.RoleAssistant,
					ToolUses: []llm.ToolUseBlock{{ID: "tu_1", Name: "w", Input: json.RawMessage(`{"c":"NYC"}`)}},
				},
				{Role: llm.RoleUser, ToolResults: []llm.ToolResult{{ToolUseID: "tu_1", Content: "sunny"}}},
				{Role: llm.RoleAssistant, Content: "It is sunny."},
			},
			want: `[{"role":"user","content":"weather?"},` +
				`{"role":"assistant","tool_calls":[{"id":"tu_1","type":"function",` +
				`"function":{"name":"w","arguments":"{\"c\":\"NYC\"}"}}]},` +
				`{"role":"tool","tool_call_id":"tu_1","content":"sunny"},` +
				`{"role":"assistant","content":"It is sunny."}]`,
		},
		{
			name:     "unknown role is an error, not a silent drop",
			messages: []llm.ChatMessage{{Role: "developer", Content: "hi"}},
			wantErr:  true,
		},
		{
			name:     "empty role is an error",
			messages: []llm.ChatMessage{{Content: "hi"}},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildMessages(tt.systemPrompt, tt.messages)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none (result: %+v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			assertJSONEqual(t, raw, tt.want)
		})
	}
}

func TestConvertToolDefinitions(t *testing.T) {
	tools := []llm.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "look up the weather",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"city": map[string]any{"type": "string"}},
				"required":   []string{"city"},
			},
		},
		{Name: "noop"},
	}

	got := convertToolDefinitions(tools)
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	assertJSONEqual(t, raw, `[
		{"type":"function","function":{
			"name":"get_weather",
			"description":"look up the weather",
			"parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}
		}},
		{"type":"function","function":{"name":"noop"}}
	]`)
}

func TestMapFinishReason(t *testing.T) {
	tests := []struct {
		in   string
		want llm.StopReason
	}{
		{"stop", llm.StopReasonEndTurn},
		{"", llm.StopReasonEndTurn},
		{"length", llm.StopReasonMaxTokens},
		{"content_filter", llm.StopReasonContentFilter},
		{"tool_calls", llm.StopReasonToolUse},
		{"function_call", llm.StopReasonToolUse},
		{"something_new", llm.StopReasonUnknown},
	}
	for _, tt := range tests {
		if got := mapFinishReason(tt.in); got != tt.want {
			t.Errorf("mapFinishReason(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
