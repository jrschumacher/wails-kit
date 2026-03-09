package llm

import (
	"fmt"
	"strings"
	"sync"

	"github.com/jrschumacher/wails-kit/settings"
)

type ProviderFactory func(modelID string, config ProviderConfig) Provider

var (
	factoryMu sync.RWMutex
	factories = map[string]ProviderFactory{}
)

func RegisterProvider(name string, factory ProviderFactory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	factories[name] = factory
}

func NewProvider(name, modelID string, config ProviderConfig) (Provider, error) {
	factoryMu.RLock()
	factory, ok := factories[name]
	factoryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
	p := factory(modelID, config)
	if p == nil {
		return nil, fmt.Errorf("provider factory %q returned nil", name)
	}
	return p, nil
}

// ResolveModelID determines the effective model ID from settings values.
// It applies custom model overrides and anthropic openai-compatible prefixing.
// This is the single source of truth for model resolution — used by both
// ConfigFromValues and the computed settings field.
func ResolveModelID(values map[string]any) string {
	provider, _ := values["llm.provider"].(string)
	if provider == "" {
		provider = "anthropic"
	}
	modelID, _ := values["llm.model"].(string)

	prefix := "llm." + provider + "."
	if custom, _ := values[prefix+"customModel"].(string); custom != "" {
		modelID = custom
	}

	if provider == "anthropic" {
		apiFormat, _ := values["llm.anthropic.apiFormat"].(string)
		if apiFormat == "openai-compatible" && !strings.HasPrefix(modelID, "anthropic/") {
			modelID = "anthropic/" + modelID
		}
	}

	return modelID
}

// resolveTransportProvider determines which provider transport to use.
func resolveTransportProvider(values map[string]any) string {
	provider, _ := values["llm.provider"].(string)
	if provider == "" {
		provider = "anthropic"
	}
	if provider == "anthropic" {
		apiFormat, _ := values["llm.anthropic.apiFormat"].(string)
		if apiFormat == "openai-compatible" {
			return "openai"
		}
	}
	return provider
}

// ConfigFromValues extracts provider name, resolved model ID, and ProviderConfig
// from a settings values map.
func ConfigFromValues(values map[string]any) (transportProvider, modelID string, config ProviderConfig) {
	provider, _ := values["llm.provider"].(string)
	if provider == "" {
		provider = "anthropic"
	}

	prefix := "llm." + provider + "."
	config.BaseURL, _ = values[prefix+"baseURL"].(string)
	config.APIKey, _ = values[prefix+"secret"].(string)

	transportProvider = resolveTransportProvider(values)
	modelID = ResolveModelID(values)

	return
}

// NewProviderFromValues creates a Provider using registered factories
// and the current settings values.
func NewProviderFromValues(values map[string]any) (Provider, error) {
	transport, modelID, config := ConfigFromValues(values)
	return NewProvider(transport, modelID, config)
}

// ProviderManager handles lazy initialization and reloading of the LLM provider.
type ProviderManager struct {
	svc      *settings.Service
	provider Provider
	mu       sync.Mutex
}

func NewProviderManager(svc *settings.Service) *ProviderManager {
	return &ProviderManager{svc: svc}
}

// Provider returns the current provider, initializing lazily if needed.
func (m *ProviderManager) Provider() (Provider, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.provider != nil {
		return m.provider, nil
	}
	return m.reload()
}

// Reload re-reads settings and creates a new provider.
func (m *ProviderManager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.reload()
	return err
}

func (m *ProviderManager) reload() (Provider, error) {
	values, err := m.svc.GetValues()
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
