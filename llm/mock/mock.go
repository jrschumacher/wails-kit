package mock

import (
	"context"

	"github.com/jrschumacher/wails-kit/llm"
)

type Provider struct {
	Name         string
	Model        string
	OnStreamChat func(ctx context.Context, req llm.ChatRequest, handler func(llm.StreamEvent)) error
}

func (p *Provider) ProviderName() string { return p.Name }
func (p *Provider) ModelID() string      { return p.Model }

func (p *Provider) StreamChat(ctx context.Context, req llm.ChatRequest, handler func(llm.StreamEvent)) error {
	if p.OnStreamChat != nil {
		return p.OnStreamChat(ctx, req, handler)
	}
	handler(llm.StreamEvent{Type: "delta", Text: "mock response"})
	handler(llm.StreamEvent{Type: "done", StopReason: "end_turn"})
	return nil
}
