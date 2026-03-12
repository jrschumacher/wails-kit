// Package anyllm provides a settings template for configuring LLM providers
// via any-llm-go. It generates a settings group with provider/model/API key
// fields and a function to build a configured any-llm-go provider from the
// current settings values.
package anyllm

import (
	"fmt"

	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers/anthropic"
	"github.com/mozilla-ai/any-llm-go/providers/deepseek"
	"github.com/mozilla-ai/any-llm-go/providers/gemini"
	"github.com/mozilla-ai/any-llm-go/providers/groq"
	"github.com/mozilla-ai/any-llm-go/providers/mistral"
	"github.com/mozilla-ai/any-llm-go/providers/ollama"
	"github.com/mozilla-ai/any-llm-go/providers/openai"

	"github.com/jrschumacher/wails-kit/settings"
)

// providerDef holds display info and default models for a provider.
type providerDef struct {
	Label  string
	Models []settings.SelectOption
}

// registry of known providers and their default model lists.
var registry = map[string]providerDef{
	"anthropic": {
		Label: "Anthropic",
		Models: []settings.SelectOption{
			{Label: "Claude Sonnet 4.6", Value: "claude-sonnet-4-6"},
			{Label: "Claude Opus 4.6", Value: "claude-opus-4-6"},
			{Label: "Claude Haiku 4.5", Value: "claude-haiku-4-5-20251001"},
		},
	},
	"openai": {
		Label: "OpenAI",
		Models: []settings.SelectOption{
			{Label: "GPT-4o", Value: "gpt-4o"},
			{Label: "GPT-4o Mini", Value: "gpt-4o-mini"},
			{Label: "o3", Value: "o3"},
		},
	},
	"deepseek": {
		Label: "DeepSeek",
		Models: []settings.SelectOption{
			{Label: "DeepSeek Chat", Value: "deepseek-chat"},
			{Label: "DeepSeek Reasoner", Value: "deepseek-reasoner"},
		},
	},
	"gemini": {
		Label: "Gemini",
		Models: []settings.SelectOption{
			{Label: "Gemini 2.0 Flash", Value: "gemini-2.0-flash"},
			{Label: "Gemini 2.5 Pro", Value: "gemini-2.5-pro-preview-06-05"},
		},
	},
	"groq": {
		Label: "Groq",
		Models: []settings.SelectOption{
			{Label: "Llama 3 70B", Value: "llama3-70b-8192"},
		},
	},
	"mistral": {
		Label: "Mistral",
		Models: []settings.SelectOption{
			{Label: "Mistral Large", Value: "mistral-large-latest"},
			{Label: "Mistral Small", Value: "mistral-small-latest"},
		},
	},
	"ollama": {
		Label: "Ollama",
		Models: []settings.SelectOption{
			{Label: "Llama 3", Value: "llama3"},
		},
	},
}

// BuildProviderFunc builds an any-llm-go Provider from the current settings.
type BuildProviderFunc func(svc *settings.Service) (anyllm.Provider, string, error)

// Option configures the template.
type Option func(*config)

type config struct {
	providers       []string
	defaultProvider string
	groupKey        string
	groupLabel      string
}

// WithProviders sets which providers appear in the settings dropdown.
// Provider names must match registry keys: "anthropic", "openai", "deepseek",
// "gemini", "groq", "mistral", "ollama".
func WithProviders(providers ...string) Option {
	return func(c *config) {
		c.providers = providers
	}
}

// WithDefaultProvider sets the default provider selection.
func WithDefaultProvider(provider string) Option {
	return func(c *config) {
		c.defaultProvider = provider
	}
}

// WithGroupKey overrides the settings group key (default: "llm").
func WithGroupKey(key string) Option {
	return func(c *config) {
		c.groupKey = key
	}
}

// WithGroupLabel overrides the settings group label (default: "LLM").
func WithGroupLabel(label string) Option {
	return func(c *config) {
		c.groupLabel = label
	}
}

// New creates a settings group and a provider builder function.
// The group contains provider/model selection, API key, and advanced fields.
// The builder reads current settings values to construct the appropriate
// any-llm-go provider.
func New(opts ...Option) (settings.Group, BuildProviderFunc) {
	cfg := &config{
		providers:       []string{"anthropic", "openai"},
		defaultProvider: "anthropic",
		groupKey:        "llm",
		groupLabel:      "LLM",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	group := buildGroup(cfg)
	builder := buildProviderFunc(cfg)
	return group, builder
}

func buildGroup(cfg *config) settings.Group {
	prefix := cfg.groupKey

	// Provider select options.
	var providerOpts []settings.SelectOption
	modelsByProvider := make(map[string][]settings.SelectOption)
	for _, name := range cfg.providers {
		def, ok := registry[name]
		if !ok {
			continue
		}
		providerOpts = append(providerOpts, settings.SelectOption{
			Label: def.Label,
			Value: name,
		})
		modelsByProvider[name] = def.Models
	}

	// Determine default model from default provider.
	var defaultModel string
	if def, ok := registry[cfg.defaultProvider]; ok && len(def.Models) > 0 {
		defaultModel = def.Models[0].Value
	}

	fields := []settings.Field{
		{
			Key:     prefix + ".provider",
			Type:    settings.FieldSelect,
			Label:   "Provider",
			Default: cfg.defaultProvider,
			Options: providerOpts,
		},
		{
			Key:     prefix + ".model",
			Type:    settings.FieldSelect,
			Label:   "Model",
			Default: defaultModel,
			DynamicOptions: &settings.DynamicOptions{
				DependsOn: prefix + ".provider",
				Options:   modelsByProvider,
			},
		},
	}

	// Per-provider advanced fields: API key, base URL, custom model.
	for _, name := range cfg.providers {
		if _, ok := registry[name]; !ok {
			continue
		}
		cond := &settings.Condition{
			Field:  prefix + ".provider",
			Equals: []string{name},
		}
		fields = append(fields,
			settings.Field{
				Key:       prefix + "." + name + ".secret",
				Type:      settings.FieldPassword,
				Label:     "API Key",
				Advanced:  true,
				Condition: cond,
			},
			settings.Field{
				Key:       prefix + "." + name + ".baseURL",
				Type:      settings.FieldText,
				Label:     "Base URL",
				Advanced:  true,
				Condition: cond,
			},
			settings.Field{
				Key:       prefix + "." + name + ".customModel",
				Type:      settings.FieldText,
				Label:     "Custom Model ID",
				Advanced:  true,
				Condition: cond,
			},
		)
	}

	// Computed resolved model.
	resolvedKey := prefix + ".resolvedModelID"
	fields = append(fields, settings.Field{
		Key:      resolvedKey,
		Type:     settings.FieldComputed,
		Label:    "Resolved Model ID",
		Advanced: true,
	})

	return settings.Group{
		Key:    cfg.groupKey,
		Label:  cfg.groupLabel,
		Fields: fields,
		ComputeFuncs: map[string]settings.ComputeFunc{
			resolvedKey: func(values map[string]any) any {
				return resolveModelID(cfg.groupKey, values)
			},
		},
	}
}

func resolveModelID(prefix string, values map[string]any) string {
	provider, _ := values[prefix+".provider"].(string)
	if provider == "" {
		return ""
	}
	if custom, _ := values[prefix+"."+provider+".customModel"].(string); custom != "" {
		return custom
	}
	model, _ := values[prefix+".model"].(string)
	return model
}

func buildProviderFunc(cfg *config) BuildProviderFunc {
	return func(svc *settings.Service) (anyllm.Provider, string, error) {
		values, err := svc.GetValues()
		if err != nil {
			return nil, "", fmt.Errorf("anyllm: read settings: %w", err)
		}

		prefix := cfg.groupKey
		providerName, _ := values[prefix+".provider"].(string)
		if providerName == "" {
			providerName = cfg.defaultProvider
		}

		modelID := resolveModelID(prefix, values)

		// Read API key from keyring (secret field).
		secretKey := prefix + "." + providerName + ".secret"
		apiKey, _ := svc.GetSecret(secretKey)

		// Read optional base URL.
		baseURL, _ := values[prefix+"."+providerName+".baseURL"].(string)

		var providerOpts []anyllm.Option
		if apiKey != "" {
			providerOpts = append(providerOpts, anyllm.WithAPIKey(apiKey))
		}
		if baseURL != "" {
			providerOpts = append(providerOpts, anyllm.WithBaseURL(baseURL))
		}

		p, err := newProvider(providerName, providerOpts...)
		if err != nil {
			return nil, "", fmt.Errorf("anyllm: create %s provider: %w", providerName, err)
		}
		return p, modelID, nil
	}
}

func newProvider(name string, opts ...anyllm.Option) (anyllm.Provider, error) {
	switch name {
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
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}
