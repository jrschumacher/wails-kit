package anthropic

import (
	"testing"

	"github.com/jrschumacher/wails-kit/llm"
)

// These cases used to live in llm/factory_test.go, back when llm.ConfigFromValues
// special-cased Anthropic itself. They belong here now: the behaviour is
// registered by this package's init, so this is the only package that can
// observe it without an import cycle.

func TestConfigFromValues_OpenAICompatibleRedirectsTransport(t *testing.T) {
	tests := []struct {
		name          string
		values        map[string]any
		wantTransport string
		wantModel     string
	}{
		{
			name: "native format keeps the anthropic transport",
			values: map[string]any{
				"llm.provider": "anthropic",
				"llm.model":    "claude-sonnet-4-6",
			},
			wantTransport: "anthropic",
			wantModel:     "claude-sonnet-4-6",
		},
		{
			name: "explicit native format keeps the anthropic transport",
			values: map[string]any{
				"llm.provider":            "anthropic",
				"llm.model":               "claude-sonnet-4-6",
				"llm.anthropic.apiFormat": llm.APIFormatAnthropicNative,
			},
			wantTransport: "anthropic",
			wantModel:     "claude-sonnet-4-6",
		},
		{
			name: "openai-compatible redirects and namespaces the model",
			values: map[string]any{
				"llm.provider":            "anthropic",
				"llm.model":               "claude-sonnet-4-6",
				"llm.anthropic.apiFormat": llm.APIFormatOpenAICompatible,
			},
			wantTransport: "openai",
			wantModel:     "anthropic/claude-sonnet-4-6",
		},
		{
			name: "an already namespaced model is not double prefixed",
			values: map[string]any{
				"llm.provider":            "anthropic",
				"llm.model":               "anthropic/claude-sonnet-4-6",
				"llm.anthropic.apiFormat": llm.APIFormatOpenAICompatible,
			},
			wantTransport: "openai",
			wantModel:     "anthropic/claude-sonnet-4-6",
		},
		{
			name: "a custom model ID is namespaced too",
			values: map[string]any{
				"llm.provider":              "anthropic",
				"llm.model":                 "claude-sonnet-4-6",
				"llm.anthropic.customModel": "claude-experimental",
				"llm.anthropic.apiFormat":   llm.APIFormatOpenAICompatible,
			},
			wantTransport: "openai",
			wantModel:     "anthropic/claude-experimental",
		},
		{
			// The provider key defaults to anthropic, so the redirection must
			// apply to a values map that never had one written to it.
			name: "an absent provider still resolves as anthropic",
			values: map[string]any{
				"llm.model":               "claude-sonnet-4-6",
				"llm.anthropic.apiFormat": llm.APIFormatOpenAICompatible,
			},
			wantTransport: "openai",
			wantModel:     "anthropic/claude-sonnet-4-6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport, modelID, _ := llm.ConfigFromValues(tt.values)
			if transport != tt.wantTransport {
				t.Errorf("transport = %q, want %q", transport, tt.wantTransport)
			}
			if modelID != tt.wantModel {
				t.Errorf("modelID = %q, want %q", modelID, tt.wantModel)
			}
		})
	}
}

// Redirecting the transport must not redirect the credentials with it: the
// gateway is reached with the Anthropic entry's own base URL and API key, and
// anything configured under llm.openai belongs to a different account.
func TestConfigFromValues_OpenAICompatibleKeepsAnthropicCredentials(t *testing.T) {
	_, _, config := llm.ConfigFromValues(map[string]any{
		"llm.provider":            "anthropic",
		"llm.model":               "claude-sonnet-4-6",
		"llm.anthropic.apiFormat": llm.APIFormatOpenAICompatible,
		"llm.anthropic.baseURL":   "https://gateway.example/v1",
		"llm.anthropic.secret":    "anthropic-key",
		"llm.openai.baseURL":      "https://api.openai.com/v1",
		"llm.openai.secret":       "sk-openai",
	})
	if config.BaseURL != "https://gateway.example/v1" {
		t.Errorf("BaseURL = %q, want the anthropic entry's own", config.BaseURL)
	}
	if config.APIKey != "anthropic-key" {
		t.Errorf("APIKey = %q, want the anthropic entry's own", config.APIKey)
	}
}

// The settings group's API Format options and the resolver must agree on the
// same string values, or selecting "OpenAI Compatible" in the UI would resolve
// to no redirection at all.
func TestSettingsGroupAPIFormatsMatchResolver(t *testing.T) {
	var formats []string
	for _, field := range llm.LLMSettingsGroup().Fields {
		if field.Key == "llm.anthropic.apiFormat" {
			for _, opt := range field.Options {
				formats = append(formats, opt.Value)
			}
		}
	}
	want := []string{llm.APIFormatAnthropicNative, llm.APIFormatOpenAICompatible}
	if len(formats) != len(want) {
		t.Fatalf("llm.anthropic.apiFormat options = %v, want %v", formats, want)
	}
	for i := range want {
		if formats[i] != want[i] {
			t.Errorf("apiFormat option %d = %q, want %q", i, formats[i], want[i])
		}
	}
}

// The registration must survive as a usable factory as well as a resolver.
func TestRegisteredFactoryBuildsThisProvider(t *testing.T) {
	p, err := llm.NewProvider(ProviderName, "claude-sonnet-4-6", llm.ProviderConfig{APIKey: "k"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.ProviderName() != ProviderName {
		t.Errorf("ProviderName() = %q, want %q", p.ProviderName(), ProviderName)
	}
	if p.ModelID() != "claude-sonnet-4-6" {
		t.Errorf("ModelID() = %q, want %q", p.ModelID(), "claude-sonnet-4-6")
	}
}
