package llm

import (
	"slices"

	"github.com/jrschumacher/wails-kit/settings"
)

// DefaultProvider is the provider assumed when llm.provider is unset. It is
// also the first entry of the built-in provider list, and therefore the default
// value of the llm.provider field in the settings group.
const DefaultProvider = "anthropic"

// ProviderSpec describes one provider's presence in the LLM settings group.
//
// It only concerns the settings UI. Registering the provider's implementation
// is separate and independent — see RegisterProvider and the blank-import
// pattern documented in the package comment.
type ProviderSpec struct {
	// Name is the value stored in llm.provider, the name the provider is
	// registered under with RegisterProvider, and the middle segment of the
	// provider's own setting keys: "llm.<Name>.baseURL", "llm.<Name>.secret",
	// "llm.<Name>.customModel", "llm.<Name>.apiFormat".
	Name string

	// Label is shown in the provider select. It defaults to Name when empty.
	Label string

	// Models are the options offered for llm.model while this provider is
	// selected. The first model of the first provider becomes the default
	// value of llm.model.
	//
	// A provider with no models still gets a Custom Model ID field, so an empty
	// list is legal — it just means every model must be typed in by hand.
	Models []settings.SelectOption

	// APIFormats, when non-empty, adds an advanced "llm.<Name>.apiFormat"
	// select carrying these options; the first is the default. It exists so a
	// provider can offer more than one wire protocol — Anthropic uses it to
	// route through OpenAI-compatible gateways. The value is interpreted by the
	// provider's own TransportResolver, not by this package.
	APIFormats []settings.SelectOption
}

func (s ProviderSpec) label() string {
	if s.Label != "" {
		return s.Label
	}
	return s.Name
}

// Built-in API format values for a provider that speaks both its native
// protocol and the OpenAI chat-completions protocol.
const (
	APIFormatAnthropicNative  = "anthropic-native"
	APIFormatOpenAICompatible = "openai-compatible"
)

// BuiltinProviderSpecs returns the provider list LLMSettingsGroup uses when no
// options are supplied. The result is a fresh copy on every call, so it is safe
// to modify and feed back through WithProvider.
func BuiltinProviderSpecs() []ProviderSpec {
	return []ProviderSpec{
		{
			Name:  "anthropic",
			Label: "Anthropic",
			Models: []settings.SelectOption{
				{Label: "Claude Sonnet 4.6", Value: "claude-sonnet-4-6"},
				{Label: "Claude Opus 4.6", Value: "claude-opus-4-6"},
				{Label: "Claude Haiku 4.5", Value: "claude-haiku-4-5-20251001"},
			},
			APIFormats: []settings.SelectOption{
				{Label: "Anthropic Native", Value: APIFormatAnthropicNative},
				{Label: "OpenAI Compatible", Value: APIFormatOpenAICompatible},
			},
		},
		{
			Name:  "openai",
			Label: "OpenAI",
			Models: []settings.SelectOption{
				{Label: "GPT-4o", Value: "gpt-4o"},
				{Label: "GPT-4o Mini", Value: "gpt-4o-mini"},
				{Label: "o3", Value: "o3"},
			},
		},
	}
}

// GroupOption customizes the schema LLMSettingsGroup produces.
//
// Options are applied in the order given, over the built-in provider list. That
// matters in one place only: an option that edits a provider added by
// WithProvider must come after it.
type GroupOption func(*groupConfig)

type groupConfig struct {
	specs []ProviderSpec
}

// find returns a pointer to the spec named name so callers can edit in place,
// or nil when no such provider is configured.
func (c *groupConfig) find(name string) *ProviderSpec {
	for i := range c.specs {
		if c.specs[i].Name == name {
			return &c.specs[i]
		}
	}
	return nil
}

// WithProviders restricts the group to the named providers, in the order given.
//
// The first surviving provider supplies the default value of llm.provider, and
// its first model the default value of llm.model — so this option also chooses
// the defaults. A name that is not configured (built-in or added by a preceding
// WithProvider) is skipped, and duplicates are collapsed.
//
// Calling it with no names is a no-op rather than a request for an empty
// provider select, which is never useful. Filtering every provider away — by
// naming only providers that are not configured — does produce an empty one,
// and settings.NewService rejects the resulting schema.
func WithProviders(names ...string) GroupOption {
	return func(c *groupConfig) {
		if len(names) == 0 {
			return
		}
		kept := make([]ProviderSpec, 0, len(names))
		for _, name := range names {
			if slices.ContainsFunc(kept, func(s ProviderSpec) bool { return s.Name == name }) {
				continue
			}
			if spec := c.find(name); spec != nil {
				kept = append(kept, *spec)
			}
		}
		c.specs = kept
	}
}

// WithProvider adds a provider to the group, or replaces the existing entry of
// the same name in place, keeping its position.
//
// This is how a third provider — one registered with RegisterProvider from
// outside this module — gets its own fields (base URL, API key, custom model
// ID) and model list in the settings UI.
func WithProvider(spec ProviderSpec) GroupOption {
	return func(c *groupConfig) {
		if existing := c.find(spec.Name); existing != nil {
			*existing = spec
			return
		}
		c.specs = append(c.specs, spec)
	}
}

// WithModels replaces the model list offered for provider.
//
// The built-in lists are a snapshot and go stale as soon as a provider ships a
// new model; this is the way to pin your own. If provider is not configured the
// option does nothing — add it with WithProvider first.
func WithModels(provider string, models ...settings.SelectOption) GroupOption {
	return func(c *groupConfig) {
		if spec := c.find(provider); spec != nil {
			spec.Models = slices.Clone(models)
		}
	}
}

// WithExtraModels appends to the model list offered for provider, keeping the
// built-in entries and the default model.
//
// If provider is not configured the option does nothing — add it with
// WithProvider first.
func WithExtraModels(provider string, models ...settings.SelectOption) GroupOption {
	return func(c *groupConfig) {
		if spec := c.find(provider); spec != nil {
			spec.Models = append(slices.Clone(spec.Models), models...)
		}
	}
}

// LLMSettingsGroup returns the settings group describing provider selection,
// model selection, and each provider's advanced connection fields.
//
// Called with no options it describes the built-in providers exactly as
// BuiltinProviderSpecs lists them: Anthropic (default, with Claude Sonnet 4.6
// as the default model) and OpenAI. Every option is additive; see WithProviders,
// WithProvider, WithModels and WithExtraModels.
func LLMSettingsGroup(opts ...GroupOption) settings.Group {
	cfg := groupConfig{specs: BuiltinProviderSpecs()}
	for _, opt := range opts {
		opt(&cfg)
	}

	providerOptions := make([]settings.SelectOption, 0, len(cfg.specs))
	modelOptions := make(map[string][]settings.SelectOption, len(cfg.specs))
	for _, spec := range cfg.specs {
		providerOptions = append(providerOptions, settings.SelectOption{
			Label: spec.label(),
			Value: spec.Name,
		})
		if len(spec.Models) > 0 {
			modelOptions[spec.Name] = slices.Clone(spec.Models)
		}
	}

	// The defaults follow the list rather than being pinned to "anthropic", so
	// that restricting or reordering providers cannot leave llm.provider
	// defaulting to one that is no longer offered.
	var defaultProvider, defaultModel any
	if len(cfg.specs) > 0 {
		defaultProvider = cfg.specs[0].Name
		if len(cfg.specs[0].Models) > 0 {
			defaultModel = cfg.specs[0].Models[0].Value
		}
	}

	fields := []settings.Field{
		{
			Key:     "llm.provider",
			Type:    settings.FieldSelect,
			Label:   "Provider",
			Default: defaultProvider,
			Options: providerOptions,
		},
		{
			Key:     "llm.model",
			Type:    settings.FieldSelect,
			Label:   "Model",
			Default: defaultModel,
			DynamicOptions: &settings.DynamicOptions{
				DependsOn: "llm.provider",
				Options:   modelOptions,
			},
		},
	}

	for _, spec := range cfg.specs {
		prefix := "llm." + spec.Name + "."
		only := &settings.Condition{Field: "llm.provider", Equals: []any{spec.Name}}

		fields = append(fields,
			settings.Field{
				Key:       prefix + "baseURL",
				Type:      settings.FieldText,
				Label:     "Base URL",
				Advanced:  true,
				Condition: only,
			},
			settings.Field{
				Key:       prefix + "secret",
				Type:      settings.FieldPassword,
				Label:     "API Key",
				Advanced:  true,
				Condition: only,
			},
		)

		if len(spec.APIFormats) > 0 {
			fields = append(fields, settings.Field{
				Key:       prefix + "apiFormat",
				Type:      settings.FieldSelect,
				Label:     "API Format",
				Default:   spec.APIFormats[0].Value,
				Advanced:  true,
				Condition: only,
				Options:   slices.Clone(spec.APIFormats),
			})
		}

		fields = append(fields, settings.Field{
			Key:       prefix + "customModel",
			Type:      settings.FieldText,
			Label:     "Custom Model ID",
			Advanced:  true,
			Condition: only,
		})
	}

	fields = append(fields, settings.Field{
		Key:      "llm.resolvedModelID",
		Type:     settings.FieldComputed,
		Label:    "Resolved Model ID",
		Advanced: true,
	})

	return settings.Group{
		Key:          "llm",
		Label:        "LLM",
		Fields:       fields,
		ComputeFuncs: map[string]settings.ComputeFunc{"llm.resolvedModelID": computeResolvedModelID},
	}
}

// computeResolvedModelID reports the model ID that would actually be sent to
// the provider, for display in the settings UI.
//
// It delegates to ConfigFromValues so the displayed value cannot drift from the
// one the factory uses. Like the factory, it therefore reflects a provider's
// registered TransportResolver — the "anthropic/" prefix an OpenAI-compatible
// gateway needs shows up here only when llm/anthropic is imported, which is the
// same condition under which a provider can be built at all.
func computeResolvedModelID(values map[string]any) any {
	_, modelID, _ := ConfigFromValues(values)
	return modelID
}
