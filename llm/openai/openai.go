package openai

import (
	"context"
	"fmt"
	"os"

	openaisdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"

	"github.com/jrschumacher/wails-kit/llm"
)

// ProviderName is the name this package registers itself under, the value
// stored in the llm.provider setting, and the middle segment of its setting
// keys ("llm.openai.secret" and so on). llm/anthropic also names it as the
// transport it redirects to for the OpenAI-compatible API format.
const ProviderName = "openai"

func init() {
	llm.RegisterProvider(ProviderName, func(modelID string, config llm.ProviderConfig) llm.Provider {
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

	// Auth precedence (kept identical in llm/anthropic):
	//
	//  1. Provider auth. A configured APIKey always wins over the SDK's
	//     implicit OPENAI_API_KEY environment fallback.
	//  2. If no APIKey is configured but a Cloudflare AI Gateway token is
	//     present, the gateway supplies the provider credential, so the
	//     implicit environment-derived key is suppressed for this client only.
	//  3. Otherwise the SDK's environment fallback is left untouched.
	//
	// Gateway auth is a separate, proxy-level concern and is applied whenever
	// CF_AIG_AUTHORIZATION is set, independently of provider auth.
	//
	// Suppression uses WithHeaderDel rather than WithAPIKey(""): in openai-go
	// v2 WithAPIKey sets "Authorization: Bearer <arg>", so WithAPIKey("") would
	// send an empty bearer token rather than no header. It must never be done
	// by unsetting the environment variable - that is a process-wide side
	// effect that races with concurrent env access and breaks unrelated code in
	// the host application.
	cfAuth := os.Getenv("CF_AIG_AUTHORIZATION")
	switch {
	case config.APIKey != "":
		opts = append(opts, option.WithAPIKey(config.APIKey))
	case cfAuth != "":
		opts = append(opts, option.WithHeaderDel("Authorization"))
	}
	if cfAuth != "" {
		opts = append(opts, option.WithHeader("cf-aig-authorization", "Bearer "+cfAuth))
	}

	return &Provider{
		client:  openaisdk.NewClient(opts...),
		modelID: modelID,
	}
}

func (p *Provider) ProviderName() string { return ProviderName }
func (p *Provider) ModelID() string      { return p.modelID }

func (p *Provider) StreamChat(ctx context.Context, req llm.ChatRequest, handler func(llm.StreamEvent)) error {
	messages, err := buildMessages(req.SystemPrompt, req.Messages)
	if err != nil {
		handler(llm.StreamEvent{Type: llm.EventError, Err: err})
		return err
	}

	params := openaisdk.ChatCompletionNewParams{
		Model:    p.modelID,
		Messages: messages,
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
	stopReason := llm.StopReasonEndTurn
	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				handler(llm.StreamEvent{Type: llm.EventDelta, Text: choice.Delta.Content})
			}
			if choice.FinishReason != "" {
				stopReason = mapFinishReason(choice.FinishReason)
			}
		}
	}

	if err := stream.Err(); err != nil {
		handler(llm.StreamEvent{Type: llm.EventError, Err: err})
		return err
	}

	if toolUses := accumulatedToolUses(&acc); len(toolUses) > 0 {
		handler(llm.StreamEvent{Type: llm.EventToolUse, ToolUses: toolUses})
		// Some gateways and OpenAI-compatible servers report "stop" even when
		// tool calls were produced; the presence of tool calls is definitive.
		stopReason = llm.StopReasonToolUse
	}

	// Exactly one terminal event is emitted for every successful stream,
	// carrying the reason the model actually stopped. Emitting it only for a
	// "stop" finish leaves consumers keyed on "done" hanging whenever the model
	// finishes for any other reason (length, content_filter, tool_calls).
	handler(llm.StreamEvent{Type: llm.EventDone, StopReason: stopReason})

	return nil
}

// accumulatedToolUses extracts the fully assembled tool calls from a finished
// stream. Tool-call arguments arrive as fragments across chunks, so they are
// read from the accumulator rather than from individual deltas.
func accumulatedToolUses(acc *openaisdk.ChatCompletionAccumulator) []llm.ToolUseBlock {
	var toolUses []llm.ToolUseBlock
	for _, choice := range acc.Choices {
		for _, call := range choice.Message.ToolCalls {
			if call.Type != "" && call.Type != "function" {
				continue
			}
			args := call.Function.Arguments
			if args == "" {
				args = "{}"
			}
			toolUses = append(toolUses, llm.ToolUseBlock{
				ID:    call.ID,
				Name:  call.Function.Name,
				Input: []byte(args),
			})
		}
	}
	return toolUses
}

// mapFinishReason translates an OpenAI finish_reason to the provider-neutral
// llm.StopReason. Reasons this package does not model become
// llm.StopReasonUnknown rather than being reported as a normal completion.
func mapFinishReason(reason string) llm.StopReason {
	switch reason {
	case "stop", "":
		return llm.StopReasonEndTurn
	case "length":
		return llm.StopReasonMaxTokens
	case "content_filter":
		return llm.StopReasonContentFilter
	case "tool_calls", "function_call":
		return llm.StopReasonToolUse
	default:
		return llm.StopReasonUnknown
	}
}

// buildMessages converts provider-neutral chat messages into OpenAI chat
// completion params.
//
// Tool content is mapped explicitly: ToolUses become assistant tool_calls and
// ToolResults become role:"tool" messages carrying the matching tool_call_id.
// An unrecognized role is reported as an error rather than dropped, because
// silently omitting a turn corrupts the conversation.
//
// OpenAI's tool message has no is_error flag, so an errored ToolResult is
// surfaced to the model by prefixing its content with "Error: ".
func buildMessages(systemPrompt string, messages []llm.ChatMessage) ([]openaisdk.ChatCompletionMessageParamUnion, error) {
	var result []openaisdk.ChatCompletionMessageParamUnion
	if systemPrompt != "" {
		result = append(result, openaisdk.SystemMessage(systemPrompt))
	}

	for i, m := range messages {
		switch {
		case len(m.ToolResults) > 0:
			for _, tr := range m.ToolResults {
				content := tr.Content
				if tr.IsError {
					content = "Error: " + content
				}
				result = append(result, openaisdk.ToolMessage(content, tr.ToolUseID))
			}

		case len(m.ToolUses) > 0:
			var assistant openaisdk.ChatCompletionAssistantMessageParam
			if m.Content != "" {
				assistant.Content.OfString = openaisdk.String(m.Content)
			}
			for _, tu := range m.ToolUses {
				arguments := string(tu.Input)
				if arguments == "" {
					arguments = "{}"
				}
				assistant.ToolCalls = append(assistant.ToolCalls,
					openaisdk.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openaisdk.ChatCompletionMessageFunctionToolCallParam{
							ID: tu.ID,
							Function: openaisdk.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tu.Name,
								Arguments: arguments,
							},
						},
					})
			}
			result = append(result, openaisdk.ChatCompletionMessageParamUnion{OfAssistant: &assistant})

		case m.Role == llm.RoleUser:
			result = append(result, openaisdk.UserMessage(m.Content))

		case m.Role == llm.RoleAssistant:
			result = append(result, openaisdk.AssistantMessage(m.Content))

		case m.Role == llm.RoleSystem:
			result = append(result, openaisdk.SystemMessage(m.Content))

		default:
			return nil, fmt.Errorf("openai: message %d has unsupported role %q", i, m.Role)
		}
	}
	return result, nil
}

// convertToolDefinitions maps provider-neutral tool definitions onto OpenAI
// function tools. The JSON Schema in InputSchema is passed through unchanged,
// so a "required" list works whether it is []string or []any.
func convertToolDefinitions(tools []llm.ToolDefinition) []openaisdk.ChatCompletionToolUnionParam {
	result := make([]openaisdk.ChatCompletionToolUnionParam, len(tools))
	for i, t := range tools {
		fn := openaisdk.FunctionDefinitionParam{Name: t.Name}
		if t.Description != "" {
			fn.Description = openaisdk.String(t.Description)
		}
		if len(t.InputSchema) > 0 {
			fn.Parameters = openaisdk.FunctionParameters(t.InputSchema)
		}
		result[i] = openaisdk.ChatCompletionFunctionTool(fn)
	}
	return result
}
