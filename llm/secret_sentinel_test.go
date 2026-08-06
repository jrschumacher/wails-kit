package llm

import (
	"testing"

	"github.com/jrschumacher/wails-kit/settings"
)

// The tests in this file guard the failure mode the sentinel design invites:
// settings.Service.GetValues masks every password field with
// settings.SecretSentinel so the real key never crosses the Wails bridge, and a
// Go-side consumer that reads the same method gets a plausible-looking string
// back instead of a compile error. A provider built from it authenticates with
// the placeholder and fails as a 401 from someone else's API, far from the
// mistake. Everything here asserts against settings.SecretSentinel by name so
// the check cannot rot into a comparison with the empty string.

const testAPIKey = "sk-real-not-the-sentinel-8b31"

// captureKey registers a provider factory under name that records the
// ProviderConfig it was constructed with.
func captureKey(t *testing.T, name string) *ProviderConfig {
	t.Helper()

	var got ProviderConfig
	RegisterProvider(name, func(modelID string, config ProviderConfig) Provider {
		got = config
		return &stubProvider{name: name, model: modelID}
	})
	return &got
}

// TestProviderManager_UsesRealSecretNotSentinel is the regression test for the
// bug: ProviderManager.reload once called GetValues, which compiles and returns
// the mask.
func TestProviderManager_UsesRealSecretNotSentinel(t *testing.T) {
	for _, tc := range []struct {
		provider  string
		secretKey string
	}{
		{provider: "anthropic", secretKey: "llm.anthropic.secret"},
		{provider: "openai", secretKey: "llm.openai.secret"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			resetFactories()
			got := captureKey(t, tc.provider)

			svc, secrets := newTestService(t)
			if _, err := svc.SetValues(map[string]any{"llm.provider": tc.provider}); err != nil {
				t.Fatalf("SetValues: %v", err)
			}
			if err := svc.SetSecret(tc.secretKey, testAPIKey); err != nil {
				t.Fatalf("SetSecret: %v", err)
			}
			// Guard the guard: if the secret were never stored, every assertion
			// below would pass against an empty key and prove nothing.
			if stored := secrets.Snapshot()[tc.secretKey]; stored != testAPIKey {
				t.Fatalf("secret store holds %q, want %q", stored, testAPIKey)
			}

			if _, err := NewProviderManager(svc).Provider(); err != nil {
				t.Fatalf("Provider: %v", err)
			}

			if got.APIKey == settings.SecretSentinel {
				t.Fatalf("provider was constructed with settings.SecretSentinel (%q) as its API key; "+
					"the manager is reading GetValues instead of GetValuesWithSecrets", settings.SecretSentinel)
			}
			if got.APIKey != testAPIKey {
				t.Errorf("provider API key = %q, want the stored secret %q", got.APIKey, testAPIKey)
			}
		})
	}
}

// TestGetValuesMasksProviderSecret pins the premise of the test above: GetValues
// really does hand back the sentinel for an LLM password field. Without this, a
// future change that stopped masking would make the regression test pass for
// the wrong reason.
func TestGetValuesMasksProviderSecret(t *testing.T) {
	svc, _ := newTestService(t)
	if err := svc.SetSecret("llm.anthropic.secret", testAPIKey); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}

	masked, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if masked["llm.anthropic.secret"] != settings.SecretSentinel {
		t.Fatalf("GetValues llm.anthropic.secret = %v, want settings.SecretSentinel", masked["llm.anthropic.secret"])
	}

	withSecrets, err := svc.GetValuesWithSecrets()
	if err != nil {
		t.Fatalf("GetValuesWithSecrets: %v", err)
	}
	if withSecrets["llm.anthropic.secret"] != testAPIKey {
		t.Fatalf("GetValuesWithSecrets llm.anthropic.secret = %v, want %q",
			withSecrets["llm.anthropic.secret"], testAPIKey)
	}
}

// TestNewProviderFromValues_RejectsSentinel covers the other way into the same
// hazard: NewProviderFromValues is exported and takes a raw values map, so an
// application can hand it GetValues output directly. That must be an error, not
// a provider holding a placeholder credential.
func TestNewProviderFromValues_RejectsSentinel(t *testing.T) {
	resetFactories()
	got := captureKey(t, "anthropic")

	svc, _ := newTestService(t)
	if err := svc.SetSecret("llm.anthropic.secret", testAPIKey); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	masked, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}

	p, err := NewProviderFromValues(masked)
	if err == nil {
		t.Fatalf("NewProviderFromValues accepted a masked values map and returned %v", p)
	}
	if got.APIKey == settings.SecretSentinel {
		t.Error("the provider factory ran with settings.SecretSentinel as its API key; " +
			"the sentinel must be rejected before any provider is constructed")
	}
}
