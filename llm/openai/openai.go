package openai

import (
	"context"
	"os"

	openaisdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/jrschumacher/wails-kit/llm"
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
	messages := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)

	if req.SystemPrompt != "" {
		messages = append(messages, openaisdk.SystemMessage(req.SystemPrompt))
	}

	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			messages = append(messages, openaisdk.UserMessage(m.Content))
		case "assistant":
			messages = append(messages, openaisdk.AssistantMessage(m.Content))
		}
	}

	params := openaisdk.ChatCompletionNewParams{
		Model:    p.modelID,
		Messages: messages,
	}

	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = openaisdk.Int(int64(req.MaxTokens))
	}

	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	for stream.Next() {
		chunk := stream.Current()
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				handler(llm.StreamEvent{Type: "delta", Text: choice.Delta.Content})
			}
			if choice.FinishReason == "stop" {
				handler(llm.StreamEvent{Type: "done", StopReason: "end_turn"})
			}
		}
	}

	if err := stream.Err(); err != nil {
		handler(llm.StreamEvent{Type: "error", Err: err})
		return err
	}

	return nil
}
