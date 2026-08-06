package openai

import (
	"testing"

	"github.com/jrschumacher/wails-kit/llm"
)

// Parity with llm/anthropic: importing this package must be enough to make
// "openai" buildable through the factory. llm/anthropic also redirects to this
// name for the OpenAI-compatible API format, so a broken registration here
// breaks that path too.
func TestSelfRegistration(t *testing.T) {
	p, err := llm.NewProvider(ProviderName, "gpt-4o", llm.ProviderConfig{APIKey: "k"})
	if err != nil {
		t.Fatalf("NewProvider(%q): %v", ProviderName, err)
	}
	if p.ProviderName() != ProviderName {
		t.Errorf("ProviderName() = %q, want %q", p.ProviderName(), ProviderName)
	}
	if p.ModelID() != "gpt-4o" {
		t.Errorf("ModelID() = %q, want %q", p.ModelID(), "gpt-4o")
	}
}

// This package registers no transport resolver, so its own configuration must
// pass through ConfigFromValues untouched.
func TestNoTransportRedirection(t *testing.T) {
	transport, modelID, config := llm.ConfigFromValues(map[string]any{
		"llm.provider":       ProviderName,
		"llm.model":          "gpt-4o",
		"llm.openai.baseURL": "https://api.openai.com/v1",
		"llm.openai.secret":  "sk-test",
	})
	if transport != ProviderName || modelID != "gpt-4o" {
		t.Errorf("got (%q, %q), want (%q, %q)", transport, modelID, ProviderName, "gpt-4o")
	}
	if config.BaseURL != "https://api.openai.com/v1" || config.APIKey != "sk-test" {
		t.Errorf("config = %+v", config)
	}
}
