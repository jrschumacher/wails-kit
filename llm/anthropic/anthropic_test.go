package anthropic

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/jrschumacher/wails-kit/llm"
)

// assertJSONEqual compares two JSON documents structurally, so the test does not
// depend on SDK struct field ordering.
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
		name       string
		messages   []llm.ChatMessage
		want       string
		wantSystem []string
		wantErr    bool
	}{
		{
			name:     "empty",
			messages: nil,
			want:     `null`,
		},
		{
			name:     "user message",
			messages: []llm.ChatMessage{{Role: llm.RoleUser, Content: "hi"}},
			want:     `[{"role":"user","content":[{"type":"text","text":"hi"}]}]`,
		},
		{
			name:     "assistant message",
			messages: []llm.ChatMessage{{Role: llm.RoleAssistant, Content: "yo"}},
			want:     `[{"role":"assistant","content":[{"type":"text","text":"yo"}]}]`,
		},
		{
			name: "user then assistant",
			messages: []llm.ChatMessage{
				{Role: llm.RoleUser, Content: "hi"},
				{Role: llm.RoleAssistant, Content: "yo"},
			},
			want: `[{"role":"user","content":[{"type":"text","text":"hi"}]},` +
				`{"role":"assistant","content":[{"type":"text","text":"yo"}]}]`,
		},
		{
			name: "assistant tool use with preamble text",
			messages: []llm.ChatMessage{{
				Role:    llm.RoleAssistant,
				Content: "let me check",
				ToolUses: []llm.ToolUseBlock{
					{ID: "tu_1", Name: "get_weather", Input: json.RawMessage(`{"city":"NYC"}`)},
				},
			}},
			want: `[{"role":"assistant","content":[` +
				`{"type":"text","text":"let me check"},` +
				`{"type":"tool_use","id":"tu_1","name":"get_weather","input":{"city":"NYC"}}]}]`,
		},
		{
			name: "assistant tool use without text",
			messages: []llm.ChatMessage{{
				Role:     llm.RoleAssistant,
				ToolUses: []llm.ToolUseBlock{{ID: "tu_1", Name: "f", Input: json.RawMessage(`{}`)}},
			}},
			want: `[{"role":"assistant","content":[{"type":"tool_use","id":"tu_1","name":"f","input":{}}]}]`,
		},
		{
			name: "assistant tool use with invalid input json falls back to empty object",
			messages: []llm.ChatMessage{{
				Role:     llm.RoleAssistant,
				ToolUses: []llm.ToolUseBlock{{ID: "tu_1", Name: "f", Input: json.RawMessage(`not json`)}},
			}},
			want: `[{"role":"assistant","content":[{"type":"tool_use","id":"tu_1","name":"f","input":{}}]}]`,
		},
		{
			name: "tool results become a user message",
			messages: []llm.ChatMessage{{
				Role: llm.RoleUser,
				ToolResults: []llm.ToolResult{
					{ToolUseID: "tu_1", Content: "sunny"},
					{ToolUseID: "tu_2", Content: "boom", IsError: true},
				},
			}},
			want: `[{"role":"user","content":[` +
				`{"type":"tool_result","tool_use_id":"tu_1","content":[{"type":"text","text":"sunny"}],"is_error":false},` +
				`{"type":"tool_result","tool_use_id":"tu_2","content":[{"type":"text","text":"boom"}],"is_error":true}]}]`,
		},
		{
			name: "tool results take precedence over tool uses on the same message",
			messages: []llm.ChatMessage{{
				Role:        llm.RoleUser,
				ToolResults: []llm.ToolResult{{ToolUseID: "tu_1", Content: "ok"}},
				ToolUses:    []llm.ToolUseBlock{{ID: "tu_9", Name: "f", Input: json.RawMessage(`{}`)}},
			}},
			want: `[{"role":"user","content":[` +
				`{"type":"tool_result","tool_use_id":"tu_1","content":[{"type":"text","text":"ok"}],"is_error":false}]}]`,
		},
		{
			// llm.RoleSystem is part of the shared message model and works in
			// the list on OpenAI. Anthropic has no system role in the list, so
			// it is hoisted to the top-level system parameter rather than
			// rejected — which is what it used to be.
			name: "system message is hoisted out of the list",
			messages: []llm.ChatMessage{
				{Role: llm.RoleSystem, Content: "be terse"},
				{Role: llm.RoleUser, Content: "hi"},
			},
			want:       `[{"role":"user","content":[{"type":"text","text":"hi"}]}]`,
			wantSystem: []string{"be terse"},
		},
		{
			name: "multiple system messages keep their order",
			messages: []llm.ChatMessage{
				{Role: llm.RoleSystem, Content: "first"},
				{Role: llm.RoleUser, Content: "hi"},
				{Role: llm.RoleSystem, Content: "second"},
			},
			want:       `[{"role":"user","content":[{"type":"text","text":"hi"}]}]`,
			wantSystem: []string{"first", "second"},
		},
		{
			// An empty text block is a 400 from the API, and an empty system
			// message carries nothing.
			name:       "empty system message contributes no block",
			messages:   []llm.ChatMessage{{Role: llm.RoleSystem}},
			want:       `null`,
			wantSystem: nil,
		},
		{
			// Regression guard: an unrecognized role must not be silently dropped.
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
			got, system, err := buildMessages(tt.messages)
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

			var gotSystem []string
			for _, block := range system {
				gotSystem = append(gotSystem, block.Text)
			}
			if !reflect.DeepEqual(gotSystem, tt.wantSystem) {
				t.Errorf("system blocks = %v, want %v", gotSystem, tt.wantSystem)
			}
		})
	}
}

// The hoisted system messages must actually reach the wire, after the
// request-level system prompt.
func TestStreamChatSendsSystemPromptAndSystemMessages(t *testing.T) {
	t.Setenv("CF_AIG_AUTHORIZATION", "")
	_ = os.Unsetenv("CF_AIG_AUTHORIZATION")

	srv, _, body := newRecordingSSEServer(t, textStream("ok", "end_turn"))
	p := New("claude-sonnet-4-6", llm.ProviderConfig{BaseURL: srv.URL + "/", APIKey: "k"})

	var events []llm.StreamEvent
	if err := p.StreamChat(context.Background(), llm.ChatRequest{
		SystemPrompt: "from the request",
		Messages: []llm.ChatMessage{
			{Role: llm.RoleSystem, Content: "from the list"},
			{Role: llm.RoleUser, Content: "hi"},
		},
	}, func(e llm.StreamEvent) { events = append(events, e) }); err != nil {
		t.Fatalf("StreamChat: %v", err)
	}

	system, _ := (*body)["system"].([]any)
	if len(system) != 2 {
		t.Fatalf("system = %v, want 2 blocks", (*body)["system"])
	}
	for i, want := range []string{"from the request", "from the list"} {
		block, _ := system[i].(map[string]any)
		if block["text"] != want {
			t.Errorf("system block %d = %v, want %q", i, block["text"], want)
		}
	}

	msgs, _ := (*body)["messages"].([]any)
	if len(msgs) != 1 {
		t.Errorf("sent %d messages, want 1 (the system turn is not one)", len(msgs))
	}
}

func TestConvertToolDefinitions(t *testing.T) {
	tests := []struct {
		name         string
		tool         llm.ToolDefinition
		wantRequired []string
	}{
		{
			name: "required as []any (JSON-decoded schema)",
			tool: llm.ToolDefinition{
				Name:        "f",
				Description: "d",
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"a": map[string]any{"type": "string"}},
					"required":   []any{"a", "b"},
				},
			},
			wantRequired: []string{"a", "b"},
		},
		{
			// Regression guard for the []string case: a Go caller writing the
			// schema by hand naturally reaches for []string.
			name: "required as []string (hand-written Go schema)",
			tool: llm.ToolDefinition{
				Name: "f",
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"a": map[string]any{"type": "string"}},
					"required":   []string{"a", "b"},
				},
			},
			wantRequired: []string{"a", "b"},
		},
		{
			name: "required absent",
			tool: llm.ToolDefinition{
				Name:        "f",
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			},
			wantRequired: nil,
		},
		{
			name: "required with non-string entries skips them",
			tool: llm.ToolDefinition{
				Name:        "f",
				InputSchema: map[string]any{"required": []any{"a", 42, "b"}},
			},
			wantRequired: []string{"a", "b"},
		},
		{
			name: "required of an unsupported type yields none",
			tool: llm.ToolDefinition{
				Name:        "f",
				InputSchema: map[string]any{"required": "a"},
			},
			wantRequired: nil,
		},
		{
			// An empty list must serialize as absent, not as an empty array.
			name: "required as an empty []string yields none",
			tool: llm.ToolDefinition{
				Name:        "f",
				InputSchema: map[string]any{"required": []string{}},
			},
			wantRequired: nil,
		},
		{
			name: "required as an empty []any yields none",
			tool: llm.ToolDefinition{
				Name:        "f",
				InputSchema: map[string]any{"required": []any{}},
			},
			wantRequired: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToolDefinitions([]llm.ToolDefinition{tt.tool})
			if len(got) != 1 {
				t.Fatalf("expected 1 tool, got %d", len(got))
			}
			tool := got[0].OfTool
			if tool == nil {
				t.Fatalf("expected OfTool variant to be set")
			}
			if tool.Name != tt.tool.Name {
				t.Errorf("Name = %q, want %q", tool.Name, tt.tool.Name)
			}
			if !reflect.DeepEqual(tool.InputSchema.Required, tt.wantRequired) {
				t.Errorf("Required = %#v, want %#v", tool.InputSchema.Required, tt.wantRequired)
			}
			if !reflect.DeepEqual(tool.InputSchema.Properties, tt.tool.InputSchema["properties"]) {
				t.Errorf("Properties = %#v, want %#v",
					tool.InputSchema.Properties, tt.tool.InputSchema["properties"])
			}
		})
	}
}

func TestConvertToolDefinitionsPreservesOrderAndDescription(t *testing.T) {
	tools := []llm.ToolDefinition{
		{Name: "first", Description: "the first"},
		{Name: "second", Description: "the second"},
	}
	got := convertToolDefinitions(tools)
	if len(got) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(got))
	}
	for i, want := range tools {
		if got[i].OfTool.Name != want.Name {
			t.Errorf("tool %d Name = %q, want %q", i, got[i].OfTool.Name, want.Name)
		}
		if got[i].OfTool.Description.Value != want.Description {
			t.Errorf("tool %d Description = %q, want %q",
				i, got[i].OfTool.Description.Value, want.Description)
		}
	}
}

func TestMapStopReason(t *testing.T) {
	tests := []struct {
		in   anthropicsdk.StopReason
		want llm.StopReason
	}{
		{anthropicsdk.StopReasonEndTurn, llm.StopReasonEndTurn},
		{anthropicsdk.StopReasonToolUse, llm.StopReasonToolUse},
		{anthropicsdk.StopReasonMaxTokens, llm.StopReasonMaxTokens},
		{anthropicsdk.StopReasonStopSequence, llm.StopReasonStopSequence},
		{"", llm.StopReasonEndTurn},
		{"something_new", llm.StopReasonUnknown},
	}
	for _, tt := range tests {
		if got := mapStopReason(tt.in); got != tt.want {
			t.Errorf("mapStopReason(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
