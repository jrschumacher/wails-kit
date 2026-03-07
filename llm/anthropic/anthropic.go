package anthropic

import (
	"context"
	"encoding/json"
	"os"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/jrschumacher/wails-kit/llm"
)

func init() {
	llm.RegisterProvider("anthropic", func(modelID string, config llm.ProviderConfig) llm.Provider {
		return New(modelID, config)
	})
}

type Provider struct {
	client  anthropicsdk.Client
	modelID string
}

func New(modelID string, config llm.ProviderConfig) *Provider {
	var opts []option.RequestOption
	if config.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(config.BaseURL))
	}
	if cfAuth := os.Getenv("CF_AIG_AUTHORIZATION"); cfAuth != "" {
		_ = os.Unsetenv("ANTHROPIC_API_KEY")
		opts = append(opts, option.WithHeader("cf-aig-authorization", "Bearer "+cfAuth))
	} else if config.APIKey != "" {
		opts = append(opts, option.WithAPIKey(config.APIKey))
	}
	return &Provider{
		client:  anthropicsdk.NewClient(opts...),
		modelID: modelID,
	}
}

func (p *Provider) ProviderName() string { return "anthropic" }
func (p *Provider) ModelID() string      { return p.modelID }

func (p *Provider) StreamChat(ctx context.Context, req llm.ChatRequest, handler func(llm.StreamEvent)) error {
	maxTokens := int64(req.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	messages := buildMessages(req.Messages)

	params := anthropicsdk.MessageNewParams{
		Model:     anthropicsdk.Model(p.modelID),
		MaxTokens: maxTokens,
		Messages:  messages,
	}

	if req.SystemPrompt != "" {
		params.System = []anthropicsdk.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	if len(req.Tools) > 0 {
		params.Tools = convertToolDefinitions(req.Tools)
	}

	stream := p.client.Messages.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	var accumulated anthropicsdk.Message
	for stream.Next() {
		event := stream.Current()
		if err := accumulated.Accumulate(event); err != nil {
			handler(llm.StreamEvent{Type: "error", Err: err})
			return err
		}

		switch eventVariant := event.AsAny().(type) {
		case anthropicsdk.ContentBlockDeltaEvent:
			switch deltaVariant := eventVariant.Delta.AsAny().(type) {
			case anthropicsdk.TextDelta:
				handler(llm.StreamEvent{Type: "delta", Text: deltaVariant.Text})
			}
		}
	}

	if err := stream.Err(); err != nil {
		handler(llm.StreamEvent{Type: "error", Err: err})
		return err
	}

	if accumulated.StopReason == anthropicsdk.StopReasonToolUse {
		var toolUses []llm.ToolUseBlock
		for _, block := range accumulated.Content {
			switch variant := block.AsAny().(type) {
			case anthropicsdk.ToolUseBlock:
				toolUses = append(toolUses, llm.ToolUseBlock{
					ID:    variant.ID,
					Name:  variant.Name,
					Input: variant.Input,
				})
			}
		}
		if len(toolUses) > 0 {
			handler(llm.StreamEvent{Type: "tool_use", ToolUses: toolUses})
		}
		handler(llm.StreamEvent{Type: "done", StopReason: "tool_use"})
	} else {
		handler(llm.StreamEvent{Type: "done", StopReason: "end_turn"})
	}

	return nil
}

func buildMessages(messages []llm.ChatMessage) []anthropicsdk.MessageParam {
	var result []anthropicsdk.MessageParam
	for _, m := range messages {
		switch {
		case len(m.ToolResults) > 0:
			var blocks []anthropicsdk.ContentBlockParamUnion
			for _, tr := range m.ToolResults {
				blocks = append(blocks, anthropicsdk.NewToolResultBlock(tr.ToolUseID, tr.Content, tr.IsError))
			}
			result = append(result, anthropicsdk.NewUserMessage(blocks...))

		case len(m.ToolUses) > 0:
			var blocks []anthropicsdk.ContentBlockParamUnion
			if m.Content != "" {
				blocks = append(blocks, anthropicsdk.NewTextBlock(m.Content))
			}
			for _, tu := range m.ToolUses {
				var input any
				if err := json.Unmarshal(tu.Input, &input); err != nil {
					input = map[string]any{}
				}
				blocks = append(blocks, anthropicsdk.NewToolUseBlock(tu.ID, input, tu.Name))
			}
			result = append(result, anthropicsdk.NewAssistantMessage(blocks...))

		case m.Role == "user":
			result = append(result, anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock(m.Content)))

		case m.Role == "assistant":
			result = append(result, anthropicsdk.NewAssistantMessage(anthropicsdk.NewTextBlock(m.Content)))
		}
	}
	return result
}

func convertToolDefinitions(tools []llm.ToolDefinition) []anthropicsdk.ToolUnionParam {
	result := make([]anthropicsdk.ToolUnionParam, len(tools))
	for i, t := range tools {
		properties := t.InputSchema["properties"]
		required, _ := t.InputSchema["required"].([]any)

		var reqStrings []string
		for _, r := range required {
			if s, ok := r.(string); ok {
				reqStrings = append(reqStrings, s)
			}
		}

		result[i] = anthropicsdk.ToolUnionParam{
			OfTool: &anthropicsdk.ToolParam{
				Name:        t.Name,
				Description: anthropicsdk.String(t.Description),
				InputSchema: anthropicsdk.ToolInputSchemaParam{
					Properties: properties,
					Required:   reqStrings,
				},
			},
		}
	}
	return result
}
