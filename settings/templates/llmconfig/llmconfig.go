// Package llmconfig builds a settings.Group for LLM provider configuration:
// provider select, model select, API key (password field), and a per-provider
// base-URL override. It is pure schema — no LLM SDK import, no network call,
// no client construction. Any app that wants only "let the user pick a
// provider/model and store an API key" can depend on this package alone.
//
// Client construction and live model enumeration live one layer up, in the
// nested module settings/templates/anyllm, which depends on this package
// plus any-llm-go. That split exists so importing llmconfig never pulls the
// LLM SDK dependency graph into a consumer's build — see that package's
// README for why the split is a nested Go module rather than a build tag.
package llmconfig

import "github.com/jrschumacher/wails-kit/v2/settings"

// Provider describes one LLM provider as it appears in the generated
// settings schema: its stable ID (used as the settings value and as the key
// suffix for its secret/base-URL fields), a display label, and its default
// model list.
type Provider struct {
	ID     string
	Label  string
	Models []settings.SelectOption
}

func (p Provider) clone() Provider {
	models := make([]settings.SelectOption, len(p.Models))
	copy(models, p.Models)
	return Provider{ID: p.ID, Label: p.Label, Models: models}
}

// builtinProviders is the kit's default provider catalog, in display order.
// It is business data (provider names, model IDs) and will drift out of date
// — that is expected and is exactly why it is overridable rather than fixed.
// Nothing in this package or in settings/templates/anyllm treats this list as
// authoritative; it exists so a new consumer has a working default without
// having to type out every field by hand.
var builtinProviders = []Provider{
	{
		ID:    "anthropic",
		Label: "Anthropic",
		Models: []settings.SelectOption{
			{Label: "Claude Sonnet 4.6", Value: "claude-sonnet-4-6"},
			{Label: "Claude Opus 4.6", Value: "claude-opus-4-6"},
			{Label: "Claude Haiku 4.5", Value: "claude-haiku-4-5-20251001"},
		},
	},
	{
		ID:    "openai",
		Label: "OpenAI",
		Models: []settings.SelectOption{
			{Label: "GPT-4o", Value: "gpt-4o"},
			{Label: "GPT-4o Mini", Value: "gpt-4o-mini"},
			{Label: "o3", Value: "o3"},
		},
	},
	{
		ID:    "deepseek",
		Label: "DeepSeek",
		Models: []settings.SelectOption{
			{Label: "DeepSeek Chat", Value: "deepseek-chat"},
			{Label: "DeepSeek Reasoner", Value: "deepseek-reasoner"},
		},
	},
	{
		ID:    "gemini",
		Label: "Gemini",
		Models: []settings.SelectOption{
			{Label: "Gemini 2.0 Flash", Value: "gemini-2.0-flash"},
			{Label: "Gemini 2.5 Pro", Value: "gemini-2.5-pro-preview-06-05"},
		},
	},
	{
		ID:    "groq",
		Label: "Groq",
		Models: []settings.SelectOption{
			{Label: "Llama 3 70B", Value: "llama3-70b-8192"},
		},
	},
	{
		ID:    "mistral",
		Label: "Mistral",
		Models: []settings.SelectOption{
			{Label: "Mistral Large", Value: "mistral-large-latest"},
			{Label: "Mistral Small", Value: "mistral-small-latest"},
		},
	},
	{
		ID:    "ollama",
		Label: "Ollama",
		Models: []settings.SelectOption{
			{Label: "Llama 3", Value: "llama3"},
		},
	},
}

// Builtin returns the kit's default provider catalog, in display order. Each
// call returns a fresh copy — callers may freely mutate the result without
// affecting other callers or the package default.
func Builtin() []Provider {
	out := make([]Provider, len(builtinProviders))
	for i, p := range builtinProviders {
		out[i] = p.clone()
	}
	return out
}

// Option configures New.
type Option func(*config)

type config struct {
	providers       []string
	overrides       map[string]Provider
	defaultProvider string
	groupKey        string
	groupLabel      string
}

// WithProviders sets which provider IDs appear in the settings dropdown, and
// in what order. Any ID not found in the built-in catalog or added via
// WithProvider is silently skipped, so a typo drops a provider rather than
// panicking or erroring — check the generated Schema in a test if that
// matters to you.
func WithProviders(ids ...string) Option {
	return func(c *config) { c.providers = ids }
}

// WithProvider adds a new provider definition or replaces a built-in one
// entirely (label and model list both). It does not by itself add the
// provider to the dropdown — combine with WithProviders, e.g.:
//
//	llmconfig.New(
//	    llmconfig.WithProvider(llmconfig.Provider{ID: "acme", Label: "Acme LLM", Models: myModels}),
//	    llmconfig.WithProviders("anthropic", "acme"),
//	)
func WithProvider(p Provider) Option {
	return func(c *config) {
		if c.overrides == nil {
			c.overrides = make(map[string]Provider)
		}
		c.overrides[p.ID] = p
	}
}

// WithModels replaces the model list for a single provider (built-in or
// previously added via WithProvider) without having to restate its label.
// This is the fix for the kit's historical defect of a hardcoded,
// inevitably-stale model list: call WithModels to add, remove, or replace
// models for any provider, no fork required.
func WithModels(providerID string, models []settings.SelectOption) Option {
	return func(c *config) {
		if c.overrides == nil {
			c.overrides = make(map[string]Provider)
		}
		p, ok := c.overrides[providerID]
		if !ok {
			p = lookupBuiltin(providerID)
		}
		p.ID = providerID
		p.Models = models
		c.overrides[providerID] = p
	}
}

func lookupBuiltin(id string) Provider {
	for _, p := range builtinProviders {
		if p.ID == id {
			return p.clone()
		}
	}
	return Provider{ID: id}
}

// WithDefaultProvider sets the default provider selection.
func WithDefaultProvider(id string) Option {
	return func(c *config) { c.defaultProvider = id }
}

// WithGroupKey overrides the settings group key (default: "llm"). The group
// key is also the field-key prefix, e.g. "<key>.provider", "<key>.model".
func WithGroupKey(key string) Option {
	return func(c *config) { c.groupKey = key }
}

// WithGroupLabel overrides the settings group label (default: "LLM").
func WithGroupLabel(label string) Option {
	return func(c *config) { c.groupLabel = label }
}

// New builds the LLM settings group and a Config for reading the effective
// selection back out of a settings.Service. The group contains provider and
// model selects, plus per-provider advanced fields (API key, base URL,
// custom model ID override).
func New(opts ...Option) (settings.Group, *Config) {
	cfg := &config{
		providers:       []string{"anthropic", "openai"},
		defaultProvider: "anthropic",
		groupKey:        "llm",
		groupLabel:      "LLM",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	registry := resolvedRegistry(cfg)
	group := buildGroup(cfg, registry)
	return group, &Config{groupKey: cfg.groupKey}
}

// resolvedRegistry merges the built-in catalog with cfg's overrides
// (WithProvider / WithModels), overrides winning on ID collision.
func resolvedRegistry(cfg *config) map[string]Provider {
	reg := make(map[string]Provider, len(builtinProviders)+len(cfg.overrides))
	for _, p := range builtinProviders {
		reg[p.ID] = p.clone()
	}
	for id, p := range cfg.overrides {
		reg[id] = p
	}
	return reg
}

func buildGroup(cfg *config, registry map[string]Provider) settings.Group {
	prefix := cfg.groupKey

	var providerOpts []settings.SelectOption
	modelsByProvider := make(map[string][]settings.SelectOption)
	for _, id := range cfg.providers {
		def, ok := registry[id]
		if !ok {
			continue
		}
		providerOpts = append(providerOpts, settings.SelectOption{Label: def.Label, Value: id})
		modelsByProvider[id] = def.Models
	}

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

	for _, id := range cfg.providers {
		if _, ok := registry[id]; !ok {
			continue
		}
		cond := &settings.Condition{Field: prefix + ".provider", Equals: []string{id}}
		fields = append(fields,
			settings.Field{
				Key:       prefix + "." + id + ".secret",
				Type:      settings.FieldPassword,
				Label:     "API Key",
				Advanced:  true,
				Condition: cond,
			},
			settings.Field{
				Key:       prefix + "." + id + ".baseURL",
				Type:      settings.FieldText,
				Label:     "Base URL",
				Advanced:  true,
				Condition: cond,
			},
			settings.Field{
				Key:       prefix + "." + id + ".customModel",
				Type:      settings.FieldText,
				Label:     "Custom Model ID",
				Advanced:  true,
				Condition: cond,
			},
		)
	}

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

// resolveModelID returns the effective model ID: the per-provider custom
// model override if set, otherwise the selected model.
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
