package llmconfig

import (
	"testing"

	"github.com/jrschumacher/wails-kit/v2/settings"
)

func TestNew_DefaultConfig(t *testing.T) {
	group, cfg := New()

	if group.Key != "llm" {
		t.Errorf("group key = %q, want %q", group.Key, "llm")
	}
	if group.Label != "LLM" {
		t.Errorf("group label = %q, want %q", group.Label, "LLM")
	}
	if cfg == nil {
		t.Fatal("cfg is nil")
	}
	if cfg.GroupKey() != "llm" {
		t.Errorf("cfg.GroupKey() = %q, want %q", cfg.GroupKey(), "llm")
	}

	if len(group.Fields) < 2 {
		t.Fatalf("expected at least 2 fields, got %d", len(group.Fields))
	}

	pf := group.Fields[0]
	if pf.Key != "llm.provider" {
		t.Errorf("first field key = %q, want %q", pf.Key, "llm.provider")
	}
	if pf.Default != "anthropic" {
		t.Errorf("default provider = %v, want %q", pf.Default, "anthropic")
	}
	if len(pf.Options) != 2 {
		t.Errorf("provider options count = %d, want 2", len(pf.Options))
	}

	mf := group.Fields[1]
	if mf.Key != "llm.model" {
		t.Errorf("second field key = %q, want %q", mf.Key, "llm.model")
	}
	if mf.DynamicOptions == nil {
		t.Fatal("model field missing dynamic options")
	}
	if mf.DynamicOptions.DependsOn != "llm.provider" {
		t.Errorf("model depends on = %q, want %q", mf.DynamicOptions.DependsOn, "llm.provider")
	}
}

func TestNew_CustomProviders(t *testing.T) {
	group, _ := New(
		WithProviders("openai", "mistral", "deepseek"),
		WithDefaultProvider("openai"),
	)

	pf := group.Fields[0]
	if len(pf.Options) != 3 {
		t.Errorf("provider options count = %d, want 3", len(pf.Options))
	}
	if pf.Default != "openai" {
		t.Errorf("default provider = %v, want %q", pf.Default, "openai")
	}

	mf := group.Fields[1]
	if len(mf.DynamicOptions.Options) != 3 {
		t.Errorf("dynamic option providers = %d, want 3", len(mf.DynamicOptions.Options))
	}
}

func TestNew_CustomGroupKey(t *testing.T) {
	group, cfg := New(
		WithGroupKey("ai"),
		WithGroupLabel("AI Provider"),
	)

	if group.Key != "ai" {
		t.Errorf("group key = %q, want %q", group.Key, "ai")
	}
	if group.Label != "AI Provider" {
		t.Errorf("group label = %q, want %q", group.Label, "AI Provider")
	}
	if cfg.GroupKey() != "ai" {
		t.Errorf("cfg.GroupKey() = %q, want %q", cfg.GroupKey(), "ai")
	}

	for _, f := range group.Fields {
		if f.Key[:3] != "ai." {
			t.Errorf("field key %q doesn't start with %q", f.Key, "ai.")
		}
	}
}

func TestNew_PerProviderAdvancedFields(t *testing.T) {
	group, _ := New(WithProviders("anthropic", "openai"))

	var advancedFields []settings.Field
	for _, f := range group.Fields {
		if f.Advanced {
			advancedFields = append(advancedFields, f)
		}
	}

	// 2 providers * 3 (secret, baseURL, customModel) + 1 computed = 7.
	if len(advancedFields) != 7 {
		t.Errorf("advanced fields = %d, want 7", len(advancedFields))
	}

	for _, f := range advancedFields {
		if f.Type == settings.FieldComputed {
			continue
		}
		if f.Condition == nil {
			t.Errorf("field %q missing condition", f.Key)
		}
	}
}

func TestNew_UnknownProviderIgnored(t *testing.T) {
	group, _ := New(WithProviders("anthropic", "nonexistent"))

	pf := group.Fields[0]
	if len(pf.Options) != 1 {
		t.Errorf("provider options = %d, want 1 (unknown should be ignored)", len(pf.Options))
	}
}

func TestWithProvider_AddsCustomProvider(t *testing.T) {
	custom := Provider{
		ID:    "acme",
		Label: "Acme LLM",
		Models: []settings.SelectOption{
			{Label: "Acme Large", Value: "acme-large"},
		},
	}
	group, _ := New(
		WithProvider(custom),
		WithProviders("anthropic", "acme"),
	)

	pf := group.Fields[0]
	if len(pf.Options) != 2 {
		t.Fatalf("provider options = %d, want 2", len(pf.Options))
	}
	found := false
	for _, o := range pf.Options {
		if o.Value == "acme" && o.Label == "Acme LLM" {
			found = true
		}
	}
	if !found {
		t.Errorf("custom provider %q not present in options: %+v", "acme", pf.Options)
	}
}

func TestWithProvider_OverridesBuiltin(t *testing.T) {
	group, _ := New(
		WithProvider(Provider{ID: "anthropic", Label: "Anthropic (custom)", Models: nil}),
		WithProviders("anthropic"),
	)

	pf := group.Fields[0]
	if pf.Options[0].Label != "Anthropic (custom)" {
		t.Errorf("provider label = %q, want %q", pf.Options[0].Label, "Anthropic (custom)")
	}
}

func TestWithModels_ReplacesBuiltinModelList(t *testing.T) {
	newModels := []settings.SelectOption{
		{Label: "Claude Next", Value: "claude-next"},
	}
	group, _ := New(
		WithModels("anthropic", newModels),
		WithProviders("anthropic"),
	)

	mf := group.Fields[1]
	got := mf.DynamicOptions.Options["anthropic"]
	if len(got) != 1 || got[0].Value != "claude-next" {
		t.Errorf("anthropic models = %+v, want %+v", got, newModels)
	}
}

func TestWithModels_OnCustomProvider(t *testing.T) {
	group, _ := New(
		WithProvider(Provider{ID: "acme", Label: "Acme"}),
		WithModels("acme", []settings.SelectOption{{Label: "Acme v1", Value: "acme-v1"}}),
		WithProviders("acme"),
	)

	pf := group.Fields[0]
	if pf.Options[0].Label != "Acme" {
		t.Errorf("provider label = %q, want %q", pf.Options[0].Label, "Acme")
	}
	mf := group.Fields[1]
	models := mf.DynamicOptions.Options["acme"]
	if len(models) != 1 || models[0].Value != "acme-v1" {
		t.Errorf("acme models = %+v", models)
	}
}

func TestBuiltin_ReturnsIndependentCopy(t *testing.T) {
	a := Builtin()
	a[0].Label = "mutated"
	a[0].Models[0].Label = "mutated model"

	b := Builtin()
	if b[0].Label == "mutated" {
		t.Error("Builtin() mutation leaked into a later call (Label)")
	}
	if b[0].Models[0].Label == "mutated model" {
		t.Error("Builtin() mutation leaked into a later call (Models)")
	}
}

func TestResolveModelID(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]any
		want   string
	}{
		{
			name:   "standard model",
			values: map[string]any{"llm.provider": "anthropic", "llm.model": "claude-sonnet-4-6"},
			want:   "claude-sonnet-4-6",
		},
		{
			name:   "custom model overrides",
			values: map[string]any{"llm.provider": "anthropic", "llm.model": "claude-sonnet-4-6", "llm.anthropic.customModel": "my-custom-model"},
			want:   "my-custom-model",
		},
		{
			name:   "empty provider",
			values: map[string]any{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveModelID("llm", tt.values)
			if got != tt.want {
				t.Errorf("resolveModelID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComputeFunc_ResolvedModelID(t *testing.T) {
	group, _ := New()

	fn, ok := group.ComputeFuncs["llm.resolvedModelID"]
	if !ok {
		t.Fatal("compute func for llm.resolvedModelID not found")
	}

	values := map[string]any{
		"llm.provider": "openai",
		"llm.model":    "gpt-4o",
	}
	got := fn(values)
	if got != "gpt-4o" {
		t.Errorf("computed resolved model = %v, want %q", got, "gpt-4o")
	}
}
