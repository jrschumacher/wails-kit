package mock

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jrschumacher/wails-kit/llm"
	"github.com/jrschumacher/wails-kit/settings"
)

// Importing this package must be enough to make "mock" buildable through the
// factory, exactly as for llm/anthropic and llm/openai.
func TestSelfRegistration(t *testing.T) {
	p, err := llm.NewProvider(ProviderName, "test-model", llm.ProviderConfig{})
	if err != nil {
		t.Fatalf("NewProvider(%q): %v", ProviderName, err)
	}
	if p.ProviderName() != ProviderName {
		t.Errorf("ProviderName() = %q, want %q", p.ProviderName(), ProviderName)
	}
	if p.ModelID() != "test-model" {
		t.Errorf("ModelID() = %q, want %q", p.ModelID(), "test-model")
	}
}

// The registered factory must record the config it was handed, so a test can
// prove the base URL and API key survived the trip through the values map.
func TestNewProviderFromValues(t *testing.T) {
	p, err := llm.NewProviderFromValues(map[string]any{
		"llm.provider":     ProviderName,
		"llm.model":        "test-model",
		"llm.mock.baseURL": "https://mock.example",
		"llm.mock.secret":  "mock-key",
	})
	if err != nil {
		t.Fatalf("NewProviderFromValues: %v", err)
	}
	mp, ok := p.(*Provider)
	if !ok {
		t.Fatalf("got %T, want *mock.Provider", p)
	}
	if mp.Config.BaseURL != "https://mock.example" || mp.Config.APIKey != "mock-key" {
		t.Errorf("Config = %+v, want the configured base URL and key", mp.Config)
	}
	if mp.Model != "test-model" {
		t.Errorf("Model = %q, want %q", mp.Model, "test-model")
	}
}

// A provider built by the factory must stream without further configuration —
// that is what makes it usable as the far end of an integration test.
func TestFactoryBuiltProviderStreams(t *testing.T) {
	p, err := llm.NewProvider(ProviderName, "test-model", llm.ProviderConfig{})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	var events []llm.StreamEvent
	if err := p.StreamChat(context.Background(), llm.ChatRequest{}, func(e llm.StreamEvent) {
		events = append(events, e)
	}); err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) != 2 || events[0].Text != DefaultResponse || events[1].Type != llm.EventDone {
		t.Fatalf("events = %+v", events)
	}
}

// The end-to-end path the registration exists for: a settings.Service holding a
// real (in-memory) secret, through ProviderManager, to a built provider — with
// no network and no OS keyring.
func TestProviderManagerEndToEnd(t *testing.T) {
	secrets := settings.NewMemorySecretStore()
	svc, err := settings.NewService(
		settings.WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		settings.WithSecretStore(secrets),
		settings.WithGroup(llm.LLMSettingsGroup(llm.WithProvider(llm.ProviderSpec{
			Name:   ProviderName,
			Label:  "Mock",
			Models: []settings.SelectOption{{Label: "Test Model", Value: "test-model"}},
		}))),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.SetValues(map[string]any{
		"llm.provider": ProviderName,
		"llm.model":    "test-model",
	}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if err := svc.SetSecret("llm.mock.secret", "mock-key"); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	p, err := llm.NewProviderManager(svc).Provider()
	if err != nil {
		t.Fatalf("Provider: %v", err)
	}
	mp, ok := p.(*Provider)
	if !ok {
		t.Fatalf("got %T, want *mock.Provider", p)
	}
	if mp.Model != "test-model" {
		t.Errorf("Model = %q, want %q", mp.Model, "test-model")
	}
	// The manager resolves real secrets internally; a masked map would have put
	// settings.SecretSentinel here instead.
	if mp.Config.APIKey != "mock-key" {
		t.Errorf("APIKey = %q, want the stored secret; got the sentinel? (%q)",
			mp.Config.APIKey, settings.SecretSentinel)
	}
}
