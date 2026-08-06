// Package mock provides an llm.Provider implementation for tests.
//
// It is useful two ways. Constructed directly it is a hand-configured stub:
//
//	p := &mock.Provider{Name: "mock", Model: "test-model"}
//
// Imported for effect it registers itself with the llm factory registry, the
// same way llm/anthropic and llm/openai do, so the whole settings-to-provider
// path can be exercised end to end without a network:
//
//	import _ "github.com/jrschumacher/wails-kit/llm/mock"
//
//	p, err := llm.NewProviderFromValues(map[string]any{"llm.provider": "mock"})
//
// Registration happens in init, so it only affects binaries that import this
// package — which for a test-only provider means test binaries. Nothing here
// reaches the network or the OS keyring.
package mock

import (
	"context"

	"github.com/jrschumacher/wails-kit/llm"
)

// ProviderName is the name this package registers itself under, and the value
// to put in llm.provider to have the factory build a mock.
const ProviderName = "mock"

// DefaultResponse is the text the zero-configuration mock streams.
const DefaultResponse = "mock response"

func init() {
	llm.RegisterProvider(ProviderName, func(modelID string, config llm.ProviderConfig) llm.Provider {
		return &Provider{Name: ProviderName, Model: modelID, Config: config}
	})
}

// Provider is a scriptable llm.Provider.
//
// The zero value streams one DefaultResponse delta followed by a terminal done
// event; set OnStreamChat to script anything else.
type Provider struct {
	// Name is reported by ProviderName. The registered factory sets it to
	// ProviderName.
	Name string
	// Model is reported by ModelID. The registered factory sets it to the
	// model ID it was given.
	Model string
	// Config records the ProviderConfig the registered factory was called
	// with, so a test can assert that the base URL and API key survived the
	// trip through the settings values map. It is left zero when the Provider
	// is constructed directly.
	Config llm.ProviderConfig
	// OnStreamChat, when non-nil, replaces the default script entirely. It is
	// handed the request unchanged.
	OnStreamChat func(ctx context.Context, req llm.ChatRequest, handler func(llm.StreamEvent)) error
}

func (p *Provider) ProviderName() string { return p.Name }
func (p *Provider) ModelID() string      { return p.Model }

func (p *Provider) StreamChat(ctx context.Context, req llm.ChatRequest, handler func(llm.StreamEvent)) error {
	if p.OnStreamChat != nil {
		return p.OnStreamChat(ctx, req, handler)
	}
	handler(llm.StreamEvent{Type: llm.EventDelta, Text: DefaultResponse})
	handler(llm.StreamEvent{Type: llm.EventDone, StopReason: llm.StopReasonEndTurn})
	return nil
}
