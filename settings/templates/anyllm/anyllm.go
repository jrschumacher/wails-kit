// Package anyllm builds a runnable any-llm-go client from the LLM selection
// produced by settings/templates/llmconfig, and offers live model
// enumeration where the underlying any-llm-go provider supports it.
//
// This package is a nested Go module (its own go.mod, tagged separately —
// see README.md) so that depending on it, and therefore on any-llm-go, is a
// choice a consumer makes explicitly. Importing llmconfig never pulls this
// package or any-llm-go along with it.
package anyllm

import (
	"context"
	"errors"
	"fmt"

	anyllmsdk "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers/anthropic"
	"github.com/mozilla-ai/any-llm-go/providers/deepseek"
	"github.com/mozilla-ai/any-llm-go/providers/gemini"
	"github.com/mozilla-ai/any-llm-go/providers/groq"
	"github.com/mozilla-ai/any-llm-go/providers/mistral"
	"github.com/mozilla-ai/any-llm-go/providers/ollama"
	"github.com/mozilla-ai/any-llm-go/providers/openai"

	"github.com/jrschumacher/wails-kit/v2/settings"
	"github.com/jrschumacher/wails-kit/v2/settings/templates/llmconfig"
)

// ErrNoProviderSelected is returned by BuildProvider when the settings
// selection has no provider chosen — a fresh install with no default, or a
// llmconfig.Group built with no providers at all.
var ErrNoProviderSelected = errors.New("anyllm: no provider selected")

// ErrModelListingUnsupported is returned by ListModels when the given
// provider's any-llm-go client does not implement the optional ModelLister
// interface. Of the providers newProvider knows about, this is currently
// only Anthropic — the OpenAI-compatible providers (OpenAI, DeepSeek, Groq,
// Mistral) inherit ListModels via any-llm-go's shared CompatibleProvider,
// and Gemini and Ollama implement it directly.
var ErrModelListingUnsupported = errors.New("anyllm: provider does not support live model listing")

// BuildProvider reads the effective provider/model/API-key/base-URL
// selection out of svc via cfg (a *llmconfig.Config from llmconfig.New) and
// constructs the corresponding any-llm-go provider. It returns the provider
// and the resolved model ID to pass as CompletionParams.Model.
//
//	group, cfg := llmconfig.New(llmconfig.WithProviders("anthropic", "openai"))
//	svc := settings.NewService(settings.WithGroup(group), ...)
//	provider, modelID, err := anyllm.BuildProvider(svc, cfg)
func BuildProvider(svc *settings.Service, cfg *llmconfig.Config) (anyllmsdk.Provider, string, error) {
	providerID, modelID, apiKey, err := cfg.Selection(svc)
	if err != nil {
		return nil, "", fmt.Errorf("anyllm: read selection: %w", err)
	}
	if providerID == "" {
		return nil, "", ErrNoProviderSelected
	}

	baseURL, err := cfg.BaseURL(svc, providerID)
	if err != nil {
		return nil, "", fmt.Errorf("anyllm: read base URL: %w", err)
	}

	var opts []anyllmsdk.Option
	if apiKey != "" {
		opts = append(opts, anyllmsdk.WithAPIKey(apiKey))
	}
	if baseURL != "" {
		opts = append(opts, anyllmsdk.WithBaseURL(baseURL))
	}

	p, err := newProvider(providerID, opts...)
	if err != nil {
		return nil, "", fmt.Errorf("anyllm: create %s provider: %w", providerID, err)
	}
	return p, modelID, nil
}

// ListModels returns the live model catalog reported by the provider's API,
// for providers whose any-llm-go client implements ModelLister. Use
// errors.Is(err, ErrModelListingUnsupported) to distinguish "this provider
// can't do this" from a real request failure.
func ListModels(ctx context.Context, p anyllmsdk.Provider) ([]anyllmsdk.Model, error) {
	lister, ok := p.(anyllmsdk.ModelLister)
	if !ok {
		return nil, ErrModelListingUnsupported
	}
	resp, err := lister.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("anyllm: list models: %w", err)
	}
	if resp == nil {
		return nil, nil
	}
	return resp.Data, nil
}

// newProvider maps a llmconfig provider ID to the any-llm-go subpackage that
// implements it. This is a mechanical mapping tied to which provider
// packages any-llm-go ships, not the kind of business data (model names,
// snapshot IDs) that goes stale — see llmconfig.Builtin for that. Adding
// support for a provider any-llm-go adds after this switch was last updated
// means adding a case here.
func newProvider(id string, opts ...anyllmsdk.Option) (anyllmsdk.Provider, error) {
	switch id {
	case "anthropic":
		return anthropic.New(opts...)
	case "openai":
		return openai.New(opts...)
	case "deepseek":
		return deepseek.New(opts...)
	case "gemini":
		return gemini.New(opts...)
	case "groq":
		return groq.New(opts...)
	case "mistral":
		return mistral.New(opts...)
	case "ollama":
		return ollama.New(opts...)
	default:
		return nil, fmt.Errorf("anyllm: unknown provider %q", id)
	}
}
