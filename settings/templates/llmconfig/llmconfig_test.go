package llmconfig

import (
	"testing"

	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

func TestNew_DefaultConfig(t *testing.T) {
	group, cfg := New()

	if group.Key != "llm" {
		t.Errorf("group key = %q, want %q", group.Key, "llm")
	}
	if group.Label.Other != "LLM" {
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
		WithGroupLabel(i18n.Text{Other: "AI Provider"}),
	)

	if group.Key != "ai" {
		t.Errorf("group key = %q, want %q", group.Key, "ai")
	}
	if group.Label.Other != "AI Provider" {
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
		Label: i18n.Text{Other: "Acme LLM"},
		Models: []settings.SelectOption{
			{Label: i18n.Text{Other: "Acme Large"}, Value: "acme-large"},
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
		if o.Value == "acme" && o.Label.Other == "Acme LLM" {
			found = true
		}
	}
	if !found {
		t.Errorf("custom provider %q not present in options: %+v", "acme", pf.Options)
	}
}

func TestWithProvider_OverridesBuiltin(t *testing.T) {
	group, _ := New(
		WithProvider(Provider{ID: "anthropic", Label: i18n.Text{Other: "Anthropic (custom)"}, Models: nil}),
		WithProviders("anthropic"),
	)

	pf := group.Fields[0]
	if pf.Options[0].Label.Other != "Anthropic (custom)" {
		t.Errorf("provider label = %q, want %q", pf.Options[0].Label, "Anthropic (custom)")
	}
}

func TestWithModels_ReplacesBuiltinModelList(t *testing.T) {
	newModels := []settings.SelectOption{
		{Label: i18n.Text{Other: "Claude Next"}, Value: "claude-next"},
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
		WithProvider(Provider{ID: "acme", Label: i18n.Text{Other: "Acme"}}),
		WithModels("acme", []settings.SelectOption{{Label: i18n.Text{Other: "Acme v1"}, Value: "acme-v1"}}),
		WithProviders("acme"),
	)

	pf := group.Fields[0]
	if pf.Options[0].Label.Other != "Acme" {
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
	a[0].Label = i18n.Text{Other: "mutated"}
	a[0].Models[0].Label = i18n.Text{Other: "mutated model"}

	b := Builtin()
	if b[0].Label.Other == "mutated" {
		t.Error("Builtin() mutation leaked into a later call (Label)")
	}
	if b[0].Models[0].Label.Other == "mutated model" {
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

// TestSwitchProviderAlone_DoesNotFailValidation is the exact H2 reproduction
// against the real llmconfig-built schema (not a synthetic settings-package
// double): with defaults anthropic/claude-sonnet-4-6, a caller submitting
// only {"llm.provider": "openai"} — the single most likely thing a real app
// does with this schema — must succeed, with llm.model reset to a valid
// openai option rather than the whole submission being rejected because the
// stale anthropic model isn't a valid openai option.
func TestSwitchProviderAlone_DoesNotFailValidation(t *testing.T) {
	dir := t.TempDir()
	group, _ := New(WithProviders("anthropic", "openai"), WithDefaultProvider("anthropic"))

	svc := settings.NewService(
		settings.WithStoragePath(dir+"/settings.json"),
		settings.WithGroup(group),
	)

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if values["llm.provider"] != "anthropic" || values["llm.model"] != "claude-sonnet-4-6" {
		t.Fatalf("unexpected starting defaults: provider=%v model=%v", values["llm.provider"], values["llm.model"])
	}

	errs, err := svc.SetValues(map[string]any{"llm.provider": "openai"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected switching provider alone to succeed, got validation errors: %v", errs)
	}

	values, err = svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if values["llm.provider"] != "openai" {
		t.Errorf("expected llm.provider=openai, got %v", values["llm.provider"])
	}
	model, _ := values["llm.model"].(string)
	validModel := false
	for _, opt := range Builtin()[1].Models { // Builtin()[1] is openai
		if opt.Value == model {
			validModel = true
		}
	}
	if !validModel {
		t.Errorf("expected llm.model reset to a valid openai option, got %q", model)
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
