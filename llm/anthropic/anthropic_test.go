package anthropic

import (
	"os"
	"testing"

	"github.com/jrschumacher/wails-kit/llm"
)

func TestConvertToolDefinitions_PreservesRequiredStringSlice(t *testing.T) {
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
	if tools[0].OfTool == nil {
		t.Fatal("expected anthropic tool definition")
	}
	if len(tools[0].OfTool.InputSchema.Required) != 1 || tools[0].OfTool.InputSchema.Required[0] != "city" {
		t.Fatalf("expected required=[city], got %#v", tools[0].OfTool.InputSchema.Required)
	}
}

func TestNew_DoesNotUnsetAnthropicAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("CF_AIG_AUTHORIZATION", "cf-token")

	_ = New("claude-sonnet-4-6", llm.ProviderConfig{})

	if got := os.Getenv("ANTHROPIC_API_KEY"); got != "test-key" {
		t.Fatalf("expected ANTHROPIC_API_KEY to remain set, got %q", got)
	}
}
