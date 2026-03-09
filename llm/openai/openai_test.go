package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jrschumacher/wails-kit/llm"
	sdk "github.com/openai/openai-go/v2"
)

func TestBuildMessages_IncludesToolCallsAndResults(t *testing.T) {
	messages := buildMessages(llm.ChatRequest{
		SystemPrompt: "system",
		Messages: []llm.ChatMessage{
			{Role: "user", Content: "hello"},
			{
				Role:    "assistant",
				Content: "Calling tool",
				ToolUses: []llm.ToolUseBlock{
					{ID: "call_1", Name: "lookup_weather", Input: json.RawMessage(`{"city":"Paris"}`)},
				},
			},
			{
				ToolResults: []llm.ToolResult{
					{ToolUseID: "call_1", Content: `{"temp_c":18}`},
				},
			},
		},
	})

	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}

	payload, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	got := string(payload)

	for _, want := range []string{
		`"role":"system"`,
		`"role":"user"`,
		`"tool_calls":[`,
		`"name":"lookup_weather"`,
		`"arguments":"{\"city\":\"Paris\"}"`,
		`"role":"tool"`,
		`"tool_call_id":"call_1"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in payload: %s", want, got)
		}
	}
}

func TestConvertToolDefinitions(t *testing.T) {
	tools := convertToolDefinitions([]llm.ToolDefinition{
		{
			Name:        "lookup_weather",
			Description: "Look up weather by city",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
				},
				"required": []string{"city"},
			},
		},
	})

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].OfFunction == nil {
		t.Fatal("expected function tool definition")
	}
	if tools[0].OfFunction.Function.Name != "lookup_weather" {
		t.Fatalf("expected tool name lookup_weather, got %q", tools[0].OfFunction.Function.Name)
	}

	required, _ := tools[0].OfFunction.Function.Parameters["required"].([]string)
	if len(required) != 1 || required[0] != "city" {
		t.Fatalf("expected required=[city], got %#v", tools[0].OfFunction.Function.Parameters["required"])
	}
}

func TestFinalStreamResult_ReturnsToolUses(t *testing.T) {
	var toolCall sdk.ChatCompletionMessageToolCallUnion
	if err := json.Unmarshal([]byte(`{
		"id":"call_1",
		"type":"function",
		"function":{"name":"lookup_weather","arguments":"{\"city\":\"Paris\"}"}
	}`), &toolCall); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	stopReason, toolUses := finalStreamResult(sdk.ChatCompletionAccumulator{
		ChatCompletion: sdk.ChatCompletion{
			Choices: []sdk.ChatCompletionChoice{
				{
					FinishReason: "tool_calls",
					Message: sdk.ChatCompletionMessage{
						ToolCalls: []sdk.ChatCompletionMessageToolCallUnion{toolCall},
					},
				},
			},
		},
	})

	if stopReason != "tool_use" {
		t.Fatalf("expected stop reason tool_use, got %q", stopReason)
	}
	if len(toolUses) != 1 {
		t.Fatalf("expected 1 tool use, got %d", len(toolUses))
	}
	if toolUses[0].Name != "lookup_weather" {
		t.Fatalf("expected tool name lookup_weather, got %q", toolUses[0].Name)
	}
	if string(toolUses[0].Input) != `{"city":"Paris"}` {
		t.Fatalf("expected tool input JSON, got %s", toolUses[0].Input)
	}
}

func TestNormalizeToolInput_WrapsInvalidJSON(t *testing.T) {
	got := normalizeToolInput("test_tool", json.RawMessage(`{"city"`))
	if string(got) != `{"_raw":"{\"city\""}` {
		t.Fatalf("unexpected fallback JSON: %s", got)
	}
}
