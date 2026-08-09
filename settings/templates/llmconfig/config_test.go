package llmconfig

import (
	"path/filepath"
	"testing"

	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

func newTestService(t *testing.T, group settings.Group) *settings.Service {
	t.Helper()
	return settings.NewService(
		settings.WithStoragePath(filepath.Join(t.TempDir(), "settings.json")),
		settings.WithKeyring(keyring.NewMemoryStore()),
		settings.WithGroup(group),
	)
}

func TestConfig_Selection_Empty(t *testing.T) {
	// An empty provider list with no default: nothing pre-selected, nothing
	// to resolve.
	group, cfg := New(WithProviders(), WithDefaultProvider(""))
	svc := newTestService(t, group)

	providerID, modelID, apiKey, err := cfg.Selection(svc)
	if err != nil {
		t.Fatalf("Selection: %v", err)
	}
	if providerID != "" {
		t.Errorf("providerID = %q, want empty", providerID)
	}
	if modelID != "" {
		t.Errorf("modelID = %q, want empty", modelID)
	}
	if apiKey != "" {
		t.Errorf("apiKey = %q, want empty", apiKey)
	}
}

func TestConfig_Selection_DefaultsAndSecret(t *testing.T) {
	group, cfg := New()
	svc := newTestService(t, group)

	if errs, err := svc.SetValues(map[string]any{
		"llm.provider":         "anthropic",
		"llm.model":            "claude-sonnet-4-6",
		"llm.anthropic.secret": "sk-ant-test-key",
	}); err != nil {
		t.Fatalf("SetValues: %v", err)
	} else if len(errs) != 0 {
		t.Fatalf("SetValues validation errors: %+v", errs)
	}

	providerID, modelID, apiKey, err := cfg.Selection(svc)
	if err != nil {
		t.Fatalf("Selection: %v", err)
	}
	if providerID != "anthropic" {
		t.Errorf("providerID = %q, want %q", providerID, "anthropic")
	}
	if modelID != "claude-sonnet-4-6" {
		t.Errorf("modelID = %q, want %q", modelID, "claude-sonnet-4-6")
	}
	if apiKey != "sk-ant-test-key" {
		t.Errorf("apiKey = %q, want %q", apiKey, "sk-ant-test-key")
	}
}

func TestConfig_Selection_CustomModelOverride(t *testing.T) {
	group, cfg := New()
	svc := newTestService(t, group)

	if _, err := svc.SetValues(map[string]any{
		"llm.provider":              "anthropic",
		"llm.model":                 "claude-sonnet-4-6",
		"llm.anthropic.customModel": "my-fine-tune",
	}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}

	_, modelID, _, err := cfg.Selection(svc)
	if err != nil {
		t.Fatalf("Selection: %v", err)
	}
	if modelID != "my-fine-tune" {
		t.Errorf("modelID = %q, want %q", modelID, "my-fine-tune")
	}
}

func TestConfig_Selection_NoSecretSetIsNotAnError(t *testing.T) {
	group, cfg := New()
	svc := newTestService(t, group)

	if _, err := svc.SetValues(map[string]any{"llm.provider": "anthropic"}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}

	providerID, _, apiKey, err := cfg.Selection(svc)
	if err != nil {
		t.Fatalf("Selection: %v", err)
	}
	if providerID != "anthropic" {
		t.Errorf("providerID = %q, want %q", providerID, "anthropic")
	}
	if apiKey != "" {
		t.Errorf("apiKey = %q, want empty when no secret was ever set", apiKey)
	}
}

func TestConfig_BaseURL(t *testing.T) {
	group, cfg := New()
	svc := newTestService(t, group)

	if _, err := svc.SetValues(map[string]any{
		"llm.provider":          "anthropic",
		"llm.anthropic.baseURL": "https://custom.example.com",
	}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}

	baseURL, err := cfg.BaseURL(svc, "anthropic")
	if err != nil {
		t.Fatalf("BaseURL: %v", err)
	}
	if baseURL != "https://custom.example.com" {
		t.Errorf("baseURL = %q, want %q", baseURL, "https://custom.example.com")
	}
}

func TestConfig_BaseURL_EmptyProvider(t *testing.T) {
	group, cfg := New()
	svc := newTestService(t, group)

	baseURL, err := cfg.BaseURL(svc, "")
	if err != nil {
		t.Fatalf("BaseURL: %v", err)
	}
	if baseURL != "" {
		t.Errorf("baseURL = %q, want empty", baseURL)
	}
}

func TestConfig_GroupKey(t *testing.T) {
	_, cfg := New(WithGroupKey("ai"))
	if cfg.GroupKey() != "ai" {
		t.Errorf("GroupKey() = %q, want %q", cfg.GroupKey(), "ai")
	}
}
