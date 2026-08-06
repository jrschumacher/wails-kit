package llm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	handler(StreamEvent{Type: EventDone, StopReason: StopReasonEndTurn})
	return nil
}

// resetFactories clears the global registry between tests.
func resetFactories() {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	factories = map[string]registration{}
}

// newTestService builds a settings.Service backed by a temp file and an
// in-memory secret store.
//
// The memory store is not optional. The LLM settings group declares
// llm.anthropic.secret and llm.openai.secret as password fields, so any call
// that resolves values reaches the SecretStore; with the default backend that
// is the developer's real login keyring, which a test suite must never touch.
// It also makes these tests hermetic on machines where the keyring is
// unavailable, where the default backend fails rather than reporting "unset".
func newTestService(t *testing.T, opts ...settings.ServiceOption) (*settings.Service, *settings.MemorySecretStore) {
	t.Helper()

	secrets := settings.NewMemorySecretStore()
	base := []settings.ServiceOption{
		settings.WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		settings.WithGroup(LLMSettingsGroup()),
		settings.WithSecretStore(secrets),
	}

	svc, err := settings.NewService(append(base, opts...)...)
	if err != nil {
		t.Fatalf("settings.NewService: %v", err)
	}
	return svc, secrets
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

// The transport-redirection tests that used to live here — Anthropic over the
// OpenAI-compatible API format — moved to llm/anthropic, which is where that
// knowledge now lives. What is tested here is the mechanism: this package must
// know nothing about any particular provider.

func TestConfigFromValues_TransportResolverRedirects(t *testing.T) {
	resetFactories()
	RegisterProvider("acme",
		func(modelID string, config ProviderConfig) Provider {
			return &stubProvider{name: "acme", model: modelID}
		},
		WithTransportResolver(func(values map[string]any, keyPrefix, modelID string) (string, string) {
			// The resolver is handed its own key prefix rather than having to
			// hardcode the name it was registered under.
			if keyPrefix != "llm.acme." {
				t.Errorf("resolver got keyPrefix %q, want %q", keyPrefix, "llm.acme.")
			}
			if format, _ := values[keyPrefix+"apiFormat"].(string); format == "openai-compatible" {
				return "openai", "acme/" + modelID
			}
			return "acme", modelID
		}),
	)

	t.Run("redirects", func(t *testing.T) {
		transport, modelID, _ := ConfigFromValues(map[string]any{
			"llm.provider":       "acme",
			"llm.model":          "acme-1",
			"llm.acme.apiFormat": "openai-compatible",
		})
		if transport != "openai" {
			t.Errorf("transport = %q, want %q", transport, "openai")
		}
		if modelID != "acme/acme-1" {
			t.Errorf("modelID = %q, want %q", modelID, "acme/acme-1")
		}
	})

	t.Run("declines", func(t *testing.T) {
		transport, modelID, _ := ConfigFromValues(map[string]any{
			"llm.provider": "acme",
			"llm.model":    "acme-1",
		})
		if transport != "acme" {
			t.Errorf("transport = %q, want %q", transport, "acme")
		}
		if modelID != "acme-1" {
			t.Errorf("modelID = %q, want %q", modelID, "acme-1")
		}
	})

	t.Run("credentials stay with the configured provider, not the transport", func(t *testing.T) {
		_, _, config := ConfigFromValues(map[string]any{
			"llm.provider":       "acme",
			"llm.model":          "acme-1",
			"llm.acme.apiFormat": "openai-compatible",
			"llm.acme.baseURL":   "https://gateway.example",
			"llm.acme.secret":    "acme-key",
			"llm.openai.baseURL": "https://api.openai.com",
			"llm.openai.secret":  "sk-openai",
		})
		if config.BaseURL != "https://gateway.example" || config.APIKey != "acme-key" {
			t.Errorf("config = %+v, want acme's own baseURL and secret", config)
		}
	})
}

// A resolver runs after the customModel override, so a hand-typed model ID gets
// the same treatment as one picked from the list.
func TestConfigFromValues_TransportResolverSeesCustomModel(t *testing.T) {
	resetFactories()
	RegisterProvider("acme",
		func(modelID string, config ProviderConfig) Provider { return &stubProvider{name: "acme"} },
		WithTransportResolver(func(_ map[string]any, _, modelID string) (string, string) {
			return "acme", "seen:" + modelID
		}),
	)

	_, modelID, _ := ConfigFromValues(map[string]any{
		"llm.provider":         "acme",
		"llm.model":            "listed",
		"llm.acme.customModel": "typed-by-hand",
	})
	if modelID != "seen:typed-by-hand" {
		t.Errorf("modelID = %q, want %q", modelID, "seen:typed-by-hand")
	}
}

// A provider registered without a resolver must be left alone.
func TestConfigFromValues_NoResolverIsIdentity(t *testing.T) {
	resetFactories()
	RegisterProvider("acme", func(modelID string, config ProviderConfig) Provider {
		return &stubProvider{name: "acme", model: modelID}
	})

	transport, modelID, _ := ConfigFromValues(map[string]any{
		"llm.provider": "acme",
		"llm.model":    "acme-1",
	})
	if transport != "acme" || modelID != "acme-1" {
		t.Errorf("got (%q, %q), want (%q, %q)", transport, modelID, "acme", "acme-1")
	}
}

// An unregistered provider must not make ConfigFromValues misbehave: it is a
// pure extractor, and the "unknown provider" error belongs to NewProvider.
func TestConfigFromValues_UnregisteredProviderPassesThrough(t *testing.T) {
	resetFactories()
	transport, modelID, _ := ConfigFromValues(map[string]any{
		"llm.provider": "never-registered",
		"llm.model":    "m",
	})
	if transport != "never-registered" || modelID != "m" {
		t.Errorf("got (%q, %q), want (%q, %q)", transport, modelID, "never-registered", "m")
	}
}

func TestRegisterProviderRejectsBadRegistrations(t *testing.T) {
	tests := []struct {
		name    string
		provide func()
	}{
		{"empty name", func() { RegisterProvider("", func(string, ProviderConfig) Provider { return nil }) }},
		{"nil factory", func() { RegisterProvider("acme", nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected a panic at registration time")
				}
			}()
			tt.provide()
		})
	}
}

func TestNewProviderRejectsNilFromFactory(t *testing.T) {
	resetFactories()
	RegisterProvider("acme", func(string, ProviderConfig) Provider { return nil })

	if p, err := NewProvider("acme", "m", ProviderConfig{}); err == nil {
		t.Fatalf("NewProvider returned (%v, nil) for a factory that produced nil", p)
	}
}

func TestRegisteredProviders(t *testing.T) {
	resetFactories()
	if got := RegisteredProviders(); len(got) != 0 {
		t.Errorf("RegisteredProviders() = %v, want empty", got)
	}
	for _, name := range []string{"zeta", "acme"} {
		RegisterProvider(name, func(modelID string, _ ProviderConfig) Provider {
			return &stubProvider{name: name, model: modelID}
		})
	}
	got := RegisteredProviders()
	if len(got) != 2 || got[0] != "acme" || got[1] != "zeta" {
		t.Errorf("RegisteredProviders() = %v, want [acme zeta]", got)
	}
}

// Re-registering a name replaces the whole registration, resolver included.
// Without this an application that substitutes its own implementation for a
// built-in would silently inherit the built-in's transport redirection.
func TestRegisterProviderReplacesResolver(t *testing.T) {
	resetFactories()
	RegisterProvider("acme",
		func(modelID string, _ ProviderConfig) Provider { return &stubProvider{name: "acme"} },
		WithTransportResolver(func(_ map[string]any, _, modelID string) (string, string) {
			return "somewhere-else", modelID
		}),
	)
	RegisterProvider("acme", func(modelID string, _ ProviderConfig) Provider {
		return &stubProvider{name: "acme", model: modelID}
	})

	transport, _, _ := ConfigFromValues(map[string]any{"llm.provider": "acme", "llm.model": "m"})
	if transport != "acme" {
		t.Errorf("transport = %q, want %q; the replaced registration kept its resolver", transport, "acme")
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

	svc, _ := newTestService(t)

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

	svc, _ := newTestService(t)

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

// A failed Reload must not leave the previous provider cached. Keeping it means
// a user who switches provider, sees the error and carries on has their
// requests silently go to the old provider while the settings UI shows the new
// one.
func TestProviderManager_FailedReloadDropsTheCachedProvider(t *testing.T) {
	resetFactories()
	RegisterProvider("anthropic", func(modelID string, config ProviderConfig) Provider {
		return &stubProvider{name: "anthropic", model: modelID}
	})

	svc, _ := newTestService(t)
	mgr := NewProviderManager(svc)

	if _, err := mgr.Provider(); err != nil {
		t.Fatalf("Provider: %v", err)
	}

	// Point the settings at a provider no factory is registered for.
	if _, err := svc.SetValues(map[string]any{"llm.provider": "never-registered"}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if err := mgr.Reload(); err == nil {
		t.Fatal("Reload succeeded for an unregistered provider")
	}

	p, err := mgr.Provider()
	if err == nil {
		t.Fatalf("Provider returned %v (name %q) after a failed Reload; want the same error",
			p, p.ProviderName())
	}
}

// Once the configuration is fixed, the manager must recover rather than stay
// stuck on the error.
func TestProviderManager_RecoversAfterAFailedReload(t *testing.T) {
	resetFactories()
	RegisterProvider("openai", func(modelID string, config ProviderConfig) Provider {
		return &stubProvider{name: "openai", model: modelID}
	})

	svc, _ := newTestService(t)
	mgr := NewProviderManager(svc)

	// anthropic is the default and is not registered here.
	if _, err := mgr.Provider(); err == nil {
		t.Fatal("Provider succeeded with no registered anthropic factory")
	}

	if _, err := svc.SetValues(map[string]any{"llm.provider": "openai"}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	p, err := mgr.Provider()
	if err != nil {
		t.Fatalf("Provider: %v", err)
	}
	if p.ProviderName() != "openai" {
		t.Errorf("ProviderName() = %q, want openai", p.ProviderName())
	}
}

// Reload must pick up a changed model, not just a changed provider.
func TestProviderManager_ReloadPicksUpNewValues(t *testing.T) {
	resetFactories()
	RegisterProvider("anthropic", func(modelID string, config ProviderConfig) Provider {
		return &stubProvider{name: "anthropic", model: modelID}
	})

	svc, _ := newTestService(t)
	mgr := NewProviderManager(svc)

	p, err := mgr.Provider()
	if err != nil {
		t.Fatalf("Provider: %v", err)
	}
	if p.ModelID() != "claude-sonnet-4-6" {
		t.Fatalf("ModelID() = %q, want the default", p.ModelID())
	}

	if _, err := svc.SetValues(map[string]any{"llm.model": "claude-opus-4-6"}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if err := mgr.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	p, err = mgr.Provider()
	if err != nil {
		t.Fatalf("Provider: %v", err)
	}
	if p.ModelID() != "claude-opus-4-6" {
		t.Errorf("ModelID() = %q, want claude-opus-4-6", p.ModelID())
	}
}

// ProviderManager is documented as safe for concurrent use; this fails under
// -race if the mutex ever stops covering the cache.
func TestProviderManager_ConcurrentUse(t *testing.T) {
	resetFactories()
	RegisterProvider("anthropic", func(modelID string, config ProviderConfig) Provider {
		return &stubProvider{name: "anthropic", model: modelID}
	})

	svc, _ := newTestService(t)
	mgr := NewProviderManager(svc)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				if _, err := mgr.Provider(); err != nil {
					t.Errorf("Provider: %v", err)
					return
				}
				if err := mgr.Reload(); err != nil {
					t.Errorf("Reload: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// The registry itself is documented as concurrency-safe, and provider packages
// register from init while an application may already be building providers.
func TestRegistryConcurrentUse(t *testing.T) {
	resetFactories()

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("p%d", i)
			for range 20 {
				RegisterProvider(name, func(modelID string, _ ProviderConfig) Provider {
					return &stubProvider{name: name, model: modelID}
				})
				_, _, _ = ConfigFromValues(map[string]any{"llm.provider": name})
				_, _ = NewProvider(name, "m", ProviderConfig{})
				_ = RegisteredProviders()
			}
		}()
	}
	wg.Wait()
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
