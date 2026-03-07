package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jrschumacher/wails-kit/llm"
	openaisdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
)

func init() {
	llm.RegisterProvider("openai", func(modelID string, config llm.ProviderConfig) llm.Provider {
		return New(modelID, config)
	})
}

type Provider struct {
	client  openaisdk.Client
	modelID string
}

func New(modelID string, config llm.ProviderConfig) *Provider {
	var opts []option.RequestOption
	if config.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(config.BaseURL))
	}
	if config.APIKey != "" {
		opts = append(opts, option.WithAPIKey(config.APIKey))
	}
	if cfAuth := os.Getenv("CF_AIG_AUTHORIZATION"); cfAuth != "" {
		opts = append(opts, option.WithHeader("cf-aig-authorization", "Bearer "+cfAuth))
	}
	return &Provider{
		client:  openaisdk.NewClient(opts...),
		modelID: modelID,
	}
}

func (p *Provider) ProviderName() string { return "openai" }
func (p *Provider) ModelID() string      { return p.modelID }

func (p *Provider) StreamChat(ctx context.Context, req llm.ChatRequest, handler func(llm.StreamEvent)) error {
	params := openaisdk.ChatCompletionNewParams{
		Model:    p.modelID,
		Messages: buildMessages(req),
	}

	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = openaisdk.Int(int64(req.MaxTokens))
	}
	if len(req.Tools) > 0 {
		params.Tools = convertToolDefinitions(req.Tools)
	}

	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	var acc openaisdk.ChatCompletionAccumulator
	for stream.Next() {
		chunk := stream.Current()
		if !acc.AddChunk(chunk) {
			err := fmt.Errorf("failed to accumulate openai streaming response")
			handler(llm.StreamEvent{Type: "error", Err: err})
			return err
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				handler(llm.StreamEvent{Type: "delta", Text: choice.Delta.Content})
			}
		}
	}

	if err := stream.Err(); err != nil {
		handler(llm.StreamEvent{Type: "error", Err: err})
		return err
	}

	stopReason, toolUses := finalStreamResult(acc)
	if len(toolUses) > 0 {
		handler(llm.StreamEvent{Type: "tool_use", ToolUses: toolUses})
	}
	handler(llm.StreamEvent{Type: "done", StopReason: stopReason})

	return nil
}

func buildMessages(req llm.ChatRequest) []openaisdk.ChatCompletionMessageParamUnion {
	messages := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)

	if req.SystemPrompt != "" {
		messages = append(messages, openaisdk.SystemMessage(req.SystemPrompt))
	}

	for _, m := range req.Messages {
		switch {
		case len(m.ToolResults) > 0:
			for _, tr := range m.ToolResults {
				messages = append(messages, openaisdk.ToolMessage(tr.Content, tr.ToolUseID))
			}
		case len(m.ToolUses) > 0:
			assistant := openaisdk.ChatCompletionAssistantMessageParam{}
			if m.Content != "" {
				assistant.Content.OfString = openaisdk.String(m.Content)
			}
			for _, tu := range m.ToolUses {
				assistant.ToolCalls = append(assistant.ToolCalls, openaisdk.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openaisdk.ChatCompletionMessageFunctionToolCallParam{
						ID: tu.ID,
						Function: openaisdk.ChatCompletionMessageFunctionToolCallFunctionParam{
							Arguments: string(normalizeToolInput(tu.Input)),
							Name:      tu.Name,
						},
					},
				})
			}
			messages = append(messages, openaisdk.ChatCompletionMessageParamUnion{OfAssistant: &assistant})
		case m.Role == "user":
			messages = append(messages, openaisdk.UserMessage(m.Content))
		case m.Role == "assistant":
			messages = append(messages, openaisdk.AssistantMessage(m.Content))
		}
	}

	return messages
}

func convertToolDefinitions(tools []llm.ToolDefinition) []openaisdk.ChatCompletionToolUnionParam {
	result := make([]openaisdk.ChatCompletionToolUnionParam, len(tools))
	for i, t := range tools {
		result[i] = openaisdk.ChatCompletionToolUnionParam{
			OfFunction: &openaisdk.ChatCompletionFunctionToolParam{
				Function: openaisdk.FunctionDefinitionParam{
					Name:        t.Name,
					Description: openaisdk.String(t.Description),
					Parameters:  openaisdk.FunctionParameters(t.InputSchema),
				},
			},
		}
	}
	return result
}

func finalStreamResult(acc openaisdk.ChatCompletionAccumulator) (string, []llm.ToolUseBlock) {
	if len(acc.Choices) == 0 {
		return "end_turn", nil
	}

	choice := acc.Choices[0]
	stopReason := mapStopReason(choice.FinishReason)
	if stopReason != "tool_use" {
		return stopReason, nil
	}

	return stopReason, convertToolUses(choice.Message.ToolCalls)
}

func convertToolUses(toolCalls []openaisdk.ChatCompletionMessageToolCallUnion) []llm.ToolUseBlock {
	result := make([]llm.ToolUseBlock, 0, len(toolCalls))
	for _, tc := range toolCalls {
		switch variant := tc.AsAny().(type) {
		case openaisdk.ChatCompletionMessageFunctionToolCall:
			result = append(result, llm.ToolUseBlock{
				ID:    variant.ID,
				Name:  variant.Function.Name,
				Input: normalizeToolInput(json.RawMessage(variant.Function.Arguments)),
			})
		case openaisdk.ChatCompletionMessageCustomToolCall:
			result = append(result, llm.ToolUseBlock{
				ID:    variant.ID,
				Name:  variant.Custom.Name,
				Input: normalizeToolInput(json.RawMessage(variant.Custom.Input)),
			})
		}
	}
	return result
}

func normalizeToolInput(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return json.RawMessage(`{}`)
	}
	if json.Valid(input) {
		return input
	}

	fallback, err := json.Marshal(map[string]any{"_raw": string(input)})
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(fallback)
}

func mapStopReason(reason string) string {
	switch reason {
	case "", "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return reason
	}
}
