package llm

import (
	"fmt"
	"slices"
	"sync"

	"github.com/jrschumacher/wails-kit/settings"
)

type ProviderFactory func(modelID string, config ProviderConfig) Provider

// TransportResolver lets a registered provider send its traffic over a
// different registered provider's transport, without the factory needing to
// know that any such arrangement exists.
//
// ConfigFromValues calls it with the full settings values map, the provider's
// own settings key prefix (e.g. "llm.anthropic.", so a resolver never has to
// hardcode the name it was registered under), and the model ID resolved so far
// — after any "llm.<name>.customModel" override. It returns the name of the
// provider whose factory should build the client and the model ID to give it.
//
// Returning transport unchanged (or empty) means "no redirection". The returned
// model ID is used as-is, so a resolver that rewrites nothing must return the
// modelID it was handed.
//
// llm/anthropic registers one: selecting the OpenAI-compatible API format
// routes Anthropic models through the OpenAI transport under an "anthropic/"
// model prefix. Nothing in this package knows that; the provider does.
type TransportResolver func(values map[string]any, keyPrefix, modelID string) (transport, resolvedModelID string)

// RegisterOption attaches optional behaviour to a provider registration.
type RegisterOption func(*registration)

// WithTransportResolver attaches a TransportResolver to a registration.
func WithTransportResolver(resolve TransportResolver) RegisterOption {
	return func(r *registration) {
		r.transport = resolve
	}
}

type registration struct {
	factory   ProviderFactory
	transport TransportResolver
}

var (
	factoryMu sync.RWMutex
	factories = map[string]registration{}
)

// RegisterProvider registers a provider implementation under name.
//
// It is meant to be called from a provider package's init, so that importing
// that package — usually as a blank import — is all it takes to make the
// provider available to NewProvider and NewProviderFromValues. See the package
// comment.
//
// Re-registering a name replaces the previous registration, including any
// options it carried: an application can substitute its own implementation for
// a built-in one by registering after the built-in package's init has run. It
// panics on an empty name or a nil factory, both of which would otherwise fail
// far from the mistake — at the first attempt to build a provider.
func RegisterProvider(name string, factory ProviderFactory, opts ...RegisterOption) {
	if name == "" {
		panic("llm: RegisterProvider called with an empty provider name")
	}
	if factory == nil {
		panic("llm: RegisterProvider called with a nil factory for provider " + name)
	}
	reg := registration{factory: factory}
	for _, opt := range opts {
		opt(&reg)
	}

	factoryMu.Lock()
	defer factoryMu.Unlock()
	factories[name] = reg
}

// RegisteredProviders returns the names of every registered provider, sorted.
//
// Which providers are available depends on which provider packages the binary
// imports, so this is the way to find out what the current process can actually
// build — for a diagnostics screen, or to validate a configured provider name
// before reaching the factory.
func RegisteredProviders() []string {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func NewProvider(name, modelID string, config ProviderConfig) (Provider, error) {
	factoryMu.RLock()
	reg, ok := factories[name]
	factoryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("llm: unknown provider %q; registered: %v "+
			"(provider packages register themselves from init, so a missing one usually "+
			"means the blank import is absent)", name, RegisteredProviders())
	}
	p := reg.factory(modelID, config)
	if p == nil {
		return nil, fmt.Errorf("llm: the factory registered for provider %q returned nil", name)
	}
	return p, nil
}

// transportResolver returns the TransportResolver registered for name, or nil.
func transportResolver(name string) TransportResolver {
	factoryMu.RLock()
	defer factoryMu.RUnlock()
	return factories[name].transport
}

// ConfigFromValues extracts the transport provider name, the resolved model ID,
// and the ProviderConfig from a settings values map.
//
// The returned name is the transport to build — which is not always the
// configured provider, because a provider may redirect through another one's
// wire protocol via a registered TransportResolver. Everything provider-
// specific lives in that resolver; this function is provider-agnostic.
//
// values should come from settings.Service.GetValuesWithSecrets. See
// NewProviderFromValues.
func ConfigFromValues(values map[string]any) (transportProvider, modelID string, config ProviderConfig) {
	provider, _ := values["llm.provider"].(string)
	if provider == "" {
		provider = DefaultProvider
	}
	modelID, _ = values["llm.model"].(string)

	prefix := "llm." + provider + "."
	config.BaseURL, _ = values[prefix+"baseURL"].(string)
	config.APIKey, _ = values[prefix+"secret"].(string)

	if custom, _ := values[prefix+"customModel"].(string); custom != "" {
		modelID = custom
	}

	transportProvider = provider
	if resolve := transportResolver(provider); resolve != nil {
		transport, resolved := resolve(values, prefix, modelID)
		if transport != "" {
			transportProvider = transport
		}
		modelID = resolved
	}

	return
}

// NewProviderFromValues creates a Provider using registered factories
// and the current settings values.
//
// values must come from settings.Service.GetValuesWithSecrets, not GetValues:
// the latter masks password fields with settings.SecretSentinel, and a provider
// built from that would authenticate with a placeholder. Passing a masked map is
// rejected rather than accepted, because the resulting failure would otherwise
// surface only as an opaque 401 from the provider's API at the first request.
func NewProviderFromValues(values map[string]any) (Provider, error) {
	transport, modelID, config := ConfigFromValues(values)
	if config.APIKey == settings.SecretSentinel {
		return nil, fmt.Errorf(
			"llm: API key is the settings secret sentinel; the values map came from "+
				"settings.Service.GetValues, which masks secrets — use GetValuesWithSecrets "+
				"(provider %q)", transport,
		)
	}
	return NewProvider(transport, modelID, config)
}

// ProviderManager builds a Provider from a settings.Service on demand, caches
// it, and rebuilds it when the settings change.
//
// It is the path application code should use, because it resolves real secrets
// internally and never hands them out — see the package comment. It is safe for
// concurrent use.
type ProviderManager struct {
	svc      *settings.Service
	provider Provider
	mu       sync.Mutex
}

func NewProviderManager(svc *settings.Service) *ProviderManager {
	return &ProviderManager{svc: svc}
}

// Provider returns the current provider, building it on first use and after any
// Reload.
//
// The returned Provider is a snapshot: it keeps working after the settings
// change, and callers that hold one across a settings change keep talking to
// the old configuration until they ask for a new one.
func (m *ProviderManager) Provider() (Provider, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.provider != nil {
		return m.provider, nil
	}
	return m.reload()
}

// Reload re-reads the settings and rebuilds the provider. Call it from a
// settings.WithOnChange hook, or after applying settings from the frontend.
//
// On failure the cached provider is dropped rather than kept, so a subsequent
// Provider call reports the same error instead of silently handing back a
// client built from the previous configuration. Retaining it would mean a user
// who switched provider, saw an error, and carried on would have their requests
// go to the old provider and model while the settings UI showed the new ones.
func (m *ProviderManager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.reload()
	return err
}

// reload builds a provider from the current settings. The caller holds m.mu.
//
// It reads GetValuesWithSecrets, not GetValues: GetValues replaces every
// password field with settings.SecretSentinel for the frontend's benefit, so a
// provider built from it would carry the sentinel as its API key and fail
// authentication at the first request rather than at construction. The resolved
// values never leave this function.
func (m *ProviderManager) reload() (Provider, error) {
	m.provider = nil

	values, err := m.svc.GetValuesWithSecrets()
	if err != nil {
		return nil, err
	}
	p, err := NewProviderFromValues(values)
	if err != nil {
		return nil, err
	}
	m.provider = p
	return p, nil
}
