package anyllm

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/settings"
	"github.com/jrschumacher/wails-kit/v2/settings/templates/llmconfig"
	anyllmsdk "github.com/mozilla-ai/any-llm-go"
)

func newTestService(t *testing.T, group settings.Group) *settings.Service {
	t.Helper()
	return settings.NewService(
		settings.WithStoragePath(filepath.Join(t.TempDir(), "settings.json")),
		settings.WithKeyring(keyring.NewMemoryStore()),
		settings.WithGroup(group),
	)
}

func TestBuildProvider_NoProviderSelected(t *testing.T) {
	group, cfg := llmconfig.New(llmconfig.WithProviders(), llmconfig.WithDefaultProvider(""))
	svc := newTestService(t, group)

	_, _, err := BuildProvider(svc, cfg)
	if !errors.Is(err, ErrNoProviderSelected) {
		t.Fatalf("err = %v, want ErrNoProviderSelected", err)
	}
}

func TestBuildProvider_ConstructsAnthropicProvider(t *testing.T) {
	group, cfg := llmconfig.New(llmconfig.WithProviders("anthropic"))
	svc := newTestService(t, group)

	if _, err := svc.SetValues(map[string]any{
		"llm.provider":         "anthropic",
		"llm.model":            "claude-sonnet-4-6",
		"llm.anthropic.secret": "sk-ant-test-key-not-real",
	}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}

	provider, modelID, err := BuildProvider(svc, cfg)
	if err != nil {
		t.Fatalf("BuildProvider: %v", err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}
	if provider.Name() != "anthropic" {
		t.Errorf("provider.Name() = %q, want %q", provider.Name(), "anthropic")
	}
	if modelID != "claude-sonnet-4-6" {
		t.Errorf("modelID = %q, want %q", modelID, "claude-sonnet-4-6")
	}
}

func TestBuildProvider_ConstructsOpenAIProvider(t *testing.T) {
	group, cfg := llmconfig.New(llmconfig.WithProviders("openai"), llmconfig.WithDefaultProvider("openai"))
	svc := newTestService(t, group)

	if _, err := svc.SetValues(map[string]any{
		"llm.provider":      "openai",
		"llm.model":         "gpt-4o",
		"llm.openai.secret": "sk-test-key-not-real",
	}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}

	provider, modelID, err := BuildProvider(svc, cfg)
	if err != nil {
		t.Fatalf("BuildProvider: %v", err)
	}
	if provider.Name() != "openai" {
		t.Errorf("provider.Name() = %q, want %q", provider.Name(), "openai")
	}
	if modelID != "gpt-4o" {
		t.Errorf("modelID = %q, want %q", modelID, "gpt-4o")
	}
}

func TestBuildProvider_CustomModelOverride(t *testing.T) {
	group, cfg := llmconfig.New(llmconfig.WithProviders("anthropic"))
	svc := newTestService(t, group)

	if _, err := svc.SetValues(map[string]any{
		"llm.provider":              "anthropic",
		"llm.model":                 "claude-sonnet-4-6",
		"llm.anthropic.secret":      "sk-ant-test-key-not-real",
		"llm.anthropic.customModel": "my-fine-tune",
	}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}

	_, modelID, err := BuildProvider(svc, cfg)
	if err != nil {
		t.Fatalf("BuildProvider: %v", err)
	}
	if modelID != "my-fine-tune" {
		t.Errorf("modelID = %q, want %q", modelID, "my-fine-tune")
	}
}

func TestBuildProvider_BaseURLOverride(t *testing.T) {
	group, cfg := llmconfig.New(llmconfig.WithProviders("anthropic"))
	svc := newTestService(t, group)

	if _, err := svc.SetValues(map[string]any{
		"llm.provider":          "anthropic",
		"llm.anthropic.secret":  "sk-ant-test-key-not-real",
		"llm.anthropic.baseURL": "https://custom.example.com",
	}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}

	// Constructing the provider must succeed with a base-URL override; this
	// only exercises client construction, never an outbound request.
	provider, _, err := BuildProvider(svc, cfg)
	if err != nil {
		t.Fatalf("BuildProvider: %v", err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}
}

func TestBuildProvider_MissingAPIKey(t *testing.T) {
	group, cfg := llmconfig.New(llmconfig.WithProviders("anthropic"))
	svc := newTestService(t, group)

	if _, err := svc.SetValues(map[string]any{"llm.provider": "anthropic"}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}

	// any-llm-go's anthropic.New requires an API key (from options or env);
	// with none set (and assuming ANTHROPIC_API_KEY isn't set in the test
	// environment) provider construction itself fails.
	_, _, err := BuildProvider(svc, cfg)
	if err == nil {
		t.Skip("ANTHROPIC_API_KEY appears to be set in this environment; skipping")
	}
}

func TestBuildProvider_UnknownProviderID(t *testing.T) {
	// A provider registered with llmconfig but unknown to newProvider's
	// switch — simulates any-llm-go not (yet) shipping a package for it.
	group, cfg := llmconfig.New(
		llmconfig.WithProvider(llmconfig.Provider{ID: "acme", Label: "Acme LLM"}),
		llmconfig.WithProviders("acme"),
		llmconfig.WithDefaultProvider("acme"),
	)
	svc := newTestService(t, group)

	if _, err := svc.SetValues(map[string]any{"llm.provider": "acme"}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}

	_, _, err := BuildProvider(svc, cfg)
	if err == nil {
		t.Fatal("expected an error for a provider ID with no any-llm-go mapping")
	}
}

func TestListModels_UnsupportedProvider(t *testing.T) {
	group, cfg := llmconfig.New(llmconfig.WithProviders("anthropic"))
	svc := newTestService(t, group)

	if _, err := svc.SetValues(map[string]any{
		"llm.provider":         "anthropic",
		"llm.anthropic.secret": "sk-ant-test-key-not-real",
	}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}

	provider, _, err := BuildProvider(svc, cfg)
	if err != nil {
		t.Fatalf("BuildProvider: %v", err)
	}

	_, err = ListModels(t.Context(), provider)
	if !errors.Is(err, ErrModelListingUnsupported) {
		t.Fatalf("err = %v, want ErrModelListingUnsupported", err)
	}
}

func TestListModels_SupportedProviderImplementsModelLister(t *testing.T) {
	// OpenAI (and the other OpenAI-compatible providers: DeepSeek, Groq,
	// Mistral) inherit ListModels from any-llm-go's shared
	// CompatibleProvider. This test only checks the interface is satisfied
	// — it never performs the network call ListModels(ctx) would make.
	group, cfg := llmconfig.New(llmconfig.WithProviders("openai"), llmconfig.WithDefaultProvider("openai"))
	svc := newTestService(t, group)

	if _, err := svc.SetValues(map[string]any{
		"llm.provider":      "openai",
		"llm.openai.secret": "sk-test-key-not-real",
	}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}

	provider, _, err := BuildProvider(svc, cfg)
	if err != nil {
		t.Fatalf("BuildProvider: %v", err)
	}

	// This only checks the interface is satisfied; it never calls
	// ListModels(ctx), which would make a real HTTP request.
	if _, ok := provider.(anyllmsdk.ModelLister); !ok {
		t.Fatal("openai provider does not implement ModelLister; ListModels would incorrectly report unsupported")
	}
}
