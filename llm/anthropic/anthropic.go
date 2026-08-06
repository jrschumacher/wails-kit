package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/jrschumacher/wails-kit/llm"
)

// ProviderName is the name this package registers itself under, the value
// stored in the llm.provider setting, and the middle segment of its setting
// keys ("llm.anthropic.secret" and so on).
const ProviderName = "anthropic"

// openAITransport is the provider whose wire protocol carries Anthropic
// traffic when the OpenAI-compatible API format is selected. It is a registry
// name, not an import: llm/openai must be imported by the application for the
// redirection to resolve, exactly as for any other provider.
const openAITransport = "openai"

// modelPrefix is what OpenAI-compatible gateways expect in front of an
// Anthropic model ID to route it to Anthropic.
const modelPrefix = ProviderName + "/"

func init() {
	llm.RegisterProvider(ProviderName,
		func(modelID string, config llm.ProviderConfig) llm.Provider {
			return New(modelID, config)
		},
		llm.WithTransportResolver(resolveTransport),
	)
}

// resolveTransport implements llm.TransportResolver.
//
// Anthropic is reachable two ways: its native API, and the OpenAI
// chat-completions protocol that gateways such as OpenRouter and Cloudflare AI
// Gateway expose. When the user picks the latter the request must be built by
// the OpenAI client, with the model ID namespaced so the gateway knows which
// upstream to route to.
//
// This lives here rather than in llm.ConfigFromValues because it is knowledge
// about Anthropic, not about provider configuration in general: a provider that
// wants the same treatment registers its own resolver and needs no change to
// the factory.
func resolveTransport(values map[string]any, keyPrefix, modelID string) (string, string) {
	if apiFormat, _ := values[keyPrefix+"apiFormat"].(string); apiFormat == llm.APIFormatOpenAICompatible {
		if !strings.HasPrefix(modelID, modelPrefix) {
			modelID = modelPrefix + modelID
		}
		return openAITransport, modelID
	}
	return ProviderName, modelID
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
	// Auth precedence (kept identical in llm/openai):
	//
	//  1. Provider auth. A configured APIKey always wins over the SDK's
	//     implicit ANTHROPIC_API_KEY environment fallback.
	//  2. If no APIKey is configured but a Cloudflare AI Gateway token is
	//     present, the gateway supplies the provider credential, so the
	//     implicit environment-derived key is suppressed for this client only.
	//  3. Otherwise the SDK's environment fallback is left untouched.
	//
	// Gateway auth is a separate, proxy-level concern and is applied whenever
	// CF_AIG_AUTHORIZATION is set, independently of provider auth.
	//
	// Suppression uses WithHeaderDel rather than WithAPIKey(""): in
	// anthropic-sdk-go v1.19.0 WithAPIKey sets the X-Api-Key header to its
	// argument, so WithAPIKey("") would send an empty header rather than none.
	// It must never be done by unsetting the environment variable - that is a
	// process-wide side effect that races with concurrent env access and breaks
	// unrelated code in the host application.
	cfAuth := os.Getenv("CF_AIG_AUTHORIZATION")
	switch {
	case config.APIKey != "":
		opts = append(opts, option.WithAPIKey(config.APIKey))
	case cfAuth != "":
		opts = append(opts, option.WithHeaderDel("X-Api-Key"))
	}
	if cfAuth != "" {
		opts = append(opts, option.WithHeader("cf-aig-authorization", "Bearer "+cfAuth))
	}

	return &Provider{
		client:  anthropicsdk.NewClient(opts...),
		modelID: modelID,
	}
}

func (p *Provider) ProviderName() string { return ProviderName }
func (p *Provider) ModelID() string      { return p.modelID }

func (p *Provider) StreamChat(ctx context.Context, req llm.ChatRequest, handler func(llm.StreamEvent)) error {
	maxTokens := int64(req.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	messages, systemBlocks, err := buildMessages(req.Messages)
	if err != nil {
		handler(llm.StreamEvent{Type: llm.EventError, Err: err})
		return err
	}

	params := anthropicsdk.MessageNewParams{
		Model:     anthropicsdk.Model(p.modelID),
		MaxTokens: maxTokens,
		Messages:  messages,
	}

	// The Anthropic API has no system role inside the message list; the system
	// prompt is a separate top-level parameter. Any llm.RoleSystem message is
	// therefore hoisted here, after ChatRequest.SystemPrompt and in the order it
	// appeared. See buildMessages.
	if req.SystemPrompt != "" {
		params.System = append(params.System, anthropicsdk.TextBlockParam{Text: req.SystemPrompt})
	}
	params.System = append(params.System, systemBlocks...)

	if len(req.Tools) > 0 {
		params.Tools = convertToolDefinitions(req.Tools)
	}

	stream := p.client.Messages.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	var accumulated anthropicsdk.Message
	for stream.Next() {
		event := stream.Current()
		if err := accumulated.Accumulate(event); err != nil {
			handler(llm.StreamEvent{Type: llm.EventError, Err: err})
			return err
		}

		switch eventVariant := event.AsAny().(type) {
		case anthropicsdk.ContentBlockDeltaEvent:
			switch deltaVariant := eventVariant.Delta.AsAny().(type) {
			case anthropicsdk.TextDelta:
				handler(llm.StreamEvent{Type: llm.EventDelta, Text: deltaVariant.Text})
			}
		}
	}

	if err := stream.Err(); err != nil {
		handler(llm.StreamEvent{Type: llm.EventError, Err: err})
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
			handler(llm.StreamEvent{Type: llm.EventToolUse, ToolUses: toolUses})
		}
	}

	// Exactly one terminal event is emitted for every successful stream,
	// carrying the reason the model actually stopped.
	handler(llm.StreamEvent{Type: llm.EventDone, StopReason: mapStopReason(accumulated.StopReason)})

	return nil
}

// mapStopReason translates an Anthropic stop reason to the provider-neutral
// llm.StopReason. An absent reason is treated as a natural end of turn;
// reasons this package does not model become llm.StopReasonUnknown rather than
// being silently reported as a normal completion.
func mapStopReason(reason anthropicsdk.StopReason) llm.StopReason {
	switch reason {
	case anthropicsdk.StopReasonEndTurn, "":
		return llm.StopReasonEndTurn
	case anthropicsdk.StopReasonToolUse:
		return llm.StopReasonToolUse
	case anthropicsdk.StopReasonMaxTokens:
		return llm.StopReasonMaxTokens
	case anthropicsdk.StopReasonStopSequence:
		return llm.StopReasonStopSequence
	case anthropicsdk.StopReasonRefusal:
		return llm.StopReasonContentFilter
	default:
		return llm.StopReasonUnknown
	}
}

// buildMessages converts provider-neutral chat messages into Anthropic message
// params, plus the system blocks that have to be hoisted out of the list.
//
// llm.RoleSystem is part of this module's public message model and works in the
// message list on OpenAI, so it must not be a hard error here just because the
// Anthropic API models the system prompt as a top-level parameter instead of a
// role. Such a message is returned separately for the caller to put in
// MessageNewParams.System; an empty one contributes nothing and is skipped,
// because the API rejects an empty text block.
//
// The consequence is that a system message loses its position relative to the
// surrounding turns — that is inherent to the API, not a choice made here.
//
// An unrecognized role is still reported as an error rather than dropped,
// because silently omitting a turn corrupts the conversation.
func buildMessages(messages []llm.ChatMessage) ([]anthropicsdk.MessageParam, []anthropicsdk.TextBlockParam, error) {
	var result []anthropicsdk.MessageParam
	var system []anthropicsdk.TextBlockParam
	for i, m := range messages {
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

		case m.Role == llm.RoleUser:
			result = append(result, anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock(m.Content)))

		case m.Role == llm.RoleAssistant:
			result = append(result, anthropicsdk.NewAssistantMessage(anthropicsdk.NewTextBlock(m.Content)))

		case m.Role == llm.RoleSystem:
			if m.Content != "" {
				system = append(system, anthropicsdk.TextBlockParam{Text: m.Content})
			}

		default:
			return nil, nil, fmt.Errorf("anthropic: message %d has unsupported role %q", i, m.Role)
		}
	}
	return result, system, nil
}

func convertToolDefinitions(tools []llm.ToolDefinition) []anthropicsdk.ToolUnionParam {
	result := make([]anthropicsdk.ToolUnionParam, len(tools))
	for i, t := range tools {
		properties := t.InputSchema["properties"]
		reqStrings := requiredStrings(t.InputSchema["required"])

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

// requiredStrings normalizes a JSON Schema "required" value into []string.
//
// A schema decoded from JSON yields []any, while a schema written by hand in Go
// naturally yields []string. Both are accepted; anything else yields no
// required parameters.
func requiredStrings(v any) []string {
	switch vv := v.(type) {
	case []string:
		if len(vv) == 0 {
			return nil
		}
		return append([]string(nil), vv...)
	case []any:
		var out []string
		for _, r := range vv {
			if s, ok := r.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
