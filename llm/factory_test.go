package llm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrschumacher/wails-kit/settings"
)

// stubProvider is a minimal Provider for testing the factory and manager.
type stubProvider struct {
	name  string
	model string
}

func (s *stubProvider) ProviderName() string { return s.name }
func (s *stubProvider) ModelID() string      { return s.model }
func (s *stubProvider) StreamChat(_ context.Context, _ ChatRequest, handler func(StreamEvent)) error {
	handler(StreamEvent{Type: "done", StopReason: "end_turn"})
	return nil
}

// resetFactories clears the global registry between tests.
func resetFactories() {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	factories = map[string]ProviderFactory{}
}

func TestRegisterAndNewProvider(t *testing.T) {
	resetFactories()
	RegisterProvider("test", func(modelID string, config ProviderConfig) Provider {
		return &stubProvider{name: "test", model: modelID}
	})

	p, err := NewProvider("test", "my-model", ProviderConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ProviderName() != "test" {
		t.Errorf("expected provider name 'test', got %q", p.ProviderName())
	}
	if p.ModelID() != "my-model" {
		t.Errorf("expected model 'my-model', got %q", p.ModelID())
	}
}

func TestNewProviderUnknown(t *testing.T) {
	resetFactories()
	_, err := NewProvider("nonexistent", "model", ProviderConfig{})
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}

func TestNewProvider_NilFactory(t *testing.T) {
	resetFactories()
	RegisterProvider("nil-factory", func(modelID string, config ProviderConfig) Provider {
		return nil
	})

	_, err := NewProvider("nil-factory", "model", ProviderConfig{})
	if err == nil {
		t.Fatal("expected error when factory returns nil, got nil")
	}
}

func TestConfigFromValues_AnthropicDefaults(t *testing.T) {
	values := map[string]any{
		"llm.provider": "anthropic",
		"llm.model":    "claude-sonnet-4-6",
	}
	transport, modelID, config := ConfigFromValues(values)
	if transport != "anthropic" {
		t.Errorf("expected transport 'anthropic', got %q", transport)
	}
	if modelID != "claude-sonnet-4-6" {
		t.Errorf("expected model 'claude-sonnet-4-6', got %q", modelID)
	}
	if config.BaseURL != "" {
		t.Errorf("expected empty BaseURL, got %q", config.BaseURL)
	}
	if config.APIKey != "" {
		t.Errorf("expected empty APIKey, got %q", config.APIKey)
	}
}

func TestConfigFromValues_OpenAI(t *testing.T) {
	values := map[string]any{
		"llm.provider":       "openai",
		"llm.model":          "gpt-4o",
		"llm.openai.baseURL": "https://api.openai.com",
		"llm.openai.secret":  "sk-test",
	}
	transport, modelID, config := ConfigFromValues(values)
	if transport != "openai" {
		t.Errorf("expected transport 'openai', got %q", transport)
	}
	if modelID != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", modelID)
	}
	if config.BaseURL != "https://api.openai.com" {
		t.Errorf("expected BaseURL 'https://api.openai.com', got %q", config.BaseURL)
	}
	if config.APIKey != "sk-test" {
		t.Errorf("expected APIKey 'sk-test', got %q", config.APIKey)
	}
}

func TestConfigFromValues_AnthropicOpenAICompatible(t *testing.T) {
	values := map[string]any{
		"llm.provider":            "anthropic",
		"llm.model":               "claude-sonnet-4-6",
		"llm.anthropic.apiFormat": "openai-compatible",
	}
	transport, modelID, _ := ConfigFromValues(values)
	if transport != "openai" {
		t.Errorf("expected transport 'openai', got %q", transport)
	}
	if modelID != "anthropic/claude-sonnet-4-6" {
		t.Errorf("expected model 'anthropic/claude-sonnet-4-6', got %q", modelID)
	}
}

func TestConfigFromValues_AnthropicOpenAICompatible_AlreadyPrefixed(t *testing.T) {
	values := map[string]any{
		"llm.provider":            "anthropic",
		"llm.model":               "anthropic/claude-sonnet-4-6",
		"llm.anthropic.apiFormat": "openai-compatible",
	}
	_, modelID, _ := ConfigFromValues(values)
	if modelID != "anthropic/claude-sonnet-4-6" {
		t.Errorf("expected model 'anthropic/claude-sonnet-4-6', got %q", modelID)
	}
}

func TestConfigFromValues_CustomModelOverride(t *testing.T) {
	values := map[string]any{
		"llm.provider":              "anthropic",
		"llm.model":                 "claude-sonnet-4-6",
		"llm.anthropic.customModel": "my-custom-model",
	}
	_, modelID, _ := ConfigFromValues(values)
	if modelID != "my-custom-model" {
		t.Errorf("expected model 'my-custom-model', got %q", modelID)
	}
}

func TestConfigFromValues_EmptyProviderDefaultsToAnthropic(t *testing.T) {
	values := map[string]any{
		"llm.model": "claude-sonnet-4-6",
	}
	transport, _, _ := ConfigFromValues(values)
	if transport != "anthropic" {
		t.Errorf("expected transport 'anthropic', got %q", transport)
	}
}

func TestNewProviderFromValues(t *testing.T) {
	resetFactories()
	RegisterProvider("anthropic", func(modelID string, config ProviderConfig) Provider {
		return &stubProvider{name: "anthropic", model: modelID}
	})

	values := map[string]any{
		"llm.provider": "anthropic",
		"llm.model":    "claude-sonnet-4-6",
	}
	p, err := NewProviderFromValues(values)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ProviderName() != "anthropic" {
		t.Errorf("expected provider name 'anthropic', got %q", p.ProviderName())
	}
	if p.ModelID() != "claude-sonnet-4-6" {
		t.Errorf("expected model 'claude-sonnet-4-6', got %q", p.ModelID())
	}
}

func TestProviderManager_LazyInit(t *testing.T) {
	resetFactories()
	RegisterProvider("anthropic", func(modelID string, config ProviderConfig) Provider {
		return &stubProvider{name: "anthropic", model: modelID}
	})

	dir := t.TempDir()
	storePath := filepath.Join(dir, "settings.json")

	svc := settings.NewService(
		settings.WithStorePath(storePath),
		settings.WithGroup(LLMSettingsGroup()),
	)

	mgr := NewProviderManager(svc)

	p, err := mgr.Provider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ProviderName() != "anthropic" {
		t.Errorf("expected 'anthropic', got %q", p.ProviderName())
	}

	// Second call returns cached provider
	p2, err := mgr.Provider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != p2 {
		t.Error("expected same provider instance on second call")
	}
}

func TestProviderManager_Reload(t *testing.T) {
	resetFactories()
	callCount := 0
	RegisterProvider("anthropic", func(modelID string, config ProviderConfig) Provider {
		callCount++
		return &stubProvider{name: "anthropic", model: modelID}
	})

	dir := t.TempDir()
	storePath := filepath.Join(dir, "settings.json")

	svc := settings.NewService(
		settings.WithStorePath(storePath),
		settings.WithGroup(LLMSettingsGroup()),
	)

	mgr := NewProviderManager(svc)

	_, err := mgr.Provider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected factory called once, got %d", callCount)
	}

	err = mgr.Reload()
	if err != nil {
		t.Fatalf("unexpected error on reload: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected factory called twice after reload, got %d", callCount)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
