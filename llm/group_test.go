package llm

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jrschumacher/wails-kit/settings"
)

// fieldKeys lists a group's field keys in order.
func fieldKeys(g settings.Group) []string {
	keys := make([]string, len(g.Fields))
	for i, f := range g.Fields {
		keys[i] = f.Key
	}
	return keys
}

// field returns the field with the given key, or fails.
func field(t *testing.T, g settings.Group, key string) settings.Field {
	t.Helper()
	for _, f := range g.Fields {
		if f.Key == key {
			return f
		}
	}
	t.Fatalf("no field %q in group; have %v", key, fieldKeys(g))
	return settings.Field{}
}

// LLMSettingsGroup() with no options is the compatibility contract: the options
// are additive, and the zero-option schema must stay exactly what consumers
// already have persisted values for. This pins the key set, the order, the
// defaults and the conditions.
func TestLLMSettingsGroup_ZeroOptionsIsUnchanged(t *testing.T) {
	g := LLMSettingsGroup()

	if g.Key != "llm" || g.Label != "LLM" {
		t.Errorf("group identity = %q/%q, want llm/LLM", g.Key, g.Label)
	}

	wantKeys := []string{
		"llm.provider",
		"llm.model",
		"llm.anthropic.baseURL",
		"llm.anthropic.secret",
		"llm.anthropic.apiFormat",
		"llm.anthropic.customModel",
		"llm.openai.baseURL",
		"llm.openai.secret",
		"llm.openai.customModel",
		"llm.resolvedModelID",
	}
	if got := fieldKeys(g); !reflect.DeepEqual(got, wantKeys) {
		t.Errorf("field keys =\n %v\nwant\n %v", got, wantKeys)
	}

	provider := field(t, g, "llm.provider")
	if provider.Default != "anthropic" {
		t.Errorf("llm.provider default = %v, want anthropic", provider.Default)
	}
	if !reflect.DeepEqual(provider.Options, []settings.SelectOption{
		{Label: "Anthropic", Value: "anthropic"},
		{Label: "OpenAI", Value: "openai"},
	}) {
		t.Errorf("llm.provider options = %+v", provider.Options)
	}

	model := field(t, g, "llm.model")
	if model.Default != "claude-sonnet-4-6" {
		t.Errorf("llm.model default = %v, want claude-sonnet-4-6", model.Default)
	}
	if model.DynamicOptions == nil || model.DynamicOptions.DependsOn != "llm.provider" {
		t.Fatalf("llm.model dynamic options = %+v", model.DynamicOptions)
	}
	if got := model.DynamicOptions.Options["anthropic"]; len(got) != 3 || got[0].Value != "claude-sonnet-4-6" {
		t.Errorf("anthropic model options = %+v", got)
	}
	if got := model.DynamicOptions.Options["openai"]; len(got) != 3 || got[0].Value != "gpt-4o" {
		t.Errorf("openai model options = %+v", got)
	}

	apiFormat := field(t, g, "llm.anthropic.apiFormat")
	if apiFormat.Default != APIFormatAnthropicNative {
		t.Errorf("apiFormat default = %v, want %v", apiFormat.Default, APIFormatAnthropicNative)
	}

	// Types, advanced flags and conditions.
	for _, tc := range []struct {
		key       string
		typ       settings.FieldType
		advanced  bool
		condition string // the provider it is gated on, "" for none
	}{
		{"llm.provider", settings.FieldSelect, false, ""},
		{"llm.model", settings.FieldSelect, false, ""},
		{"llm.anthropic.baseURL", settings.FieldText, true, "anthropic"},
		{"llm.anthropic.secret", settings.FieldPassword, true, "anthropic"},
		{"llm.anthropic.apiFormat", settings.FieldSelect, true, "anthropic"},
		{"llm.anthropic.customModel", settings.FieldText, true, "anthropic"},
		{"llm.openai.baseURL", settings.FieldText, true, "openai"},
		{"llm.openai.secret", settings.FieldPassword, true, "openai"},
		{"llm.openai.customModel", settings.FieldText, true, "openai"},
		{"llm.resolvedModelID", settings.FieldComputed, true, ""},
	} {
		f := field(t, g, tc.key)
		if f.Type != tc.typ {
			t.Errorf("%s type = %q, want %q", tc.key, f.Type, tc.typ)
		}
		if f.Advanced != tc.advanced {
			t.Errorf("%s advanced = %v, want %v", tc.key, f.Advanced, tc.advanced)
		}
		switch {
		case tc.condition == "":
			if f.Condition != nil {
				t.Errorf("%s has condition %+v, want none", tc.key, f.Condition)
			}
		case f.Condition == nil:
			t.Errorf("%s has no condition, want gating on %q", tc.key, tc.condition)
		default:
			want := &settings.Condition{Field: "llm.provider", Equals: []any{tc.condition}}
			if !reflect.DeepEqual(f.Condition, want) {
				t.Errorf("%s condition = %+v, want %+v", tc.key, f.Condition, want)
			}
		}
	}

	if _, ok := g.ComputeFuncs["llm.resolvedModelID"]; !ok {
		t.Error("llm.resolvedModelID has no compute function")
	}
}

func TestWithProviders_Restricts(t *testing.T) {
	g := LLMSettingsGroup(WithProviders("openai"))

	if got := fieldKeys(g); !reflect.DeepEqual(got, []string{
		"llm.provider", "llm.model",
		"llm.openai.baseURL", "llm.openai.secret", "llm.openai.customModel",
		"llm.resolvedModelID",
	}) {
		t.Errorf("field keys = %v", got)
	}

	// The defaults follow the surviving list: leaving llm.provider defaulting
	// to a provider that is no longer offered would be unselectable in the UI.
	if got := field(t, g, "llm.provider").Default; got != "openai" {
		t.Errorf("llm.provider default = %v, want openai", got)
	}
	if got := field(t, g, "llm.model").Default; got != "gpt-4o" {
		t.Errorf("llm.model default = %v, want gpt-4o", got)
	}
	if _, ok := field(t, g, "llm.model").DynamicOptions.Options["anthropic"]; ok {
		t.Error("anthropic still has model options after being excluded")
	}
}

func TestWithProviders_Reorders(t *testing.T) {
	g := LLMSettingsGroup(WithProviders("openai", "anthropic"))

	opts := field(t, g, "llm.provider").Options
	if len(opts) != 2 || opts[0].Value != "openai" || opts[1].Value != "anthropic" {
		t.Errorf("provider options = %+v, want openai then anthropic", opts)
	}
	if got := field(t, g, "llm.provider").Default; got != "openai" {
		t.Errorf("default provider = %v, want openai", got)
	}
	// Reordering must carry each provider's own fields with it.
	if got := fieldKeys(g)[2]; got != "llm.openai.baseURL" {
		t.Errorf("first provider field = %q, want llm.openai.baseURL", got)
	}
}

func TestWithProviders_IgnoresUnknownAndDuplicates(t *testing.T) {
	g := LLMSettingsGroup(WithProviders("openai", "nope", "openai"))
	opts := field(t, g, "llm.provider").Options
	if len(opts) != 1 || opts[0].Value != "openai" {
		t.Errorf("provider options = %+v, want just openai", opts)
	}
}

// Calling it with no names is documented as a no-op, not as a request for an
// empty provider select.
func TestWithProviders_NoNamesIsNoOp(t *testing.T) {
	if got, want := fieldKeys(LLMSettingsGroup(WithProviders())), fieldKeys(LLMSettingsGroup()); !reflect.DeepEqual(got, want) {
		t.Errorf("WithProviders() changed the group: %v", got)
	}
}

func TestWithModels_Replaces(t *testing.T) {
	g := LLMSettingsGroup(WithModels("anthropic",
		settings.SelectOption{Label: "Claude Next", Value: "claude-next"},
	))

	models := field(t, g, "llm.model").DynamicOptions.Options["anthropic"]
	if len(models) != 1 || models[0].Value != "claude-next" {
		t.Errorf("anthropic models = %+v, want only claude-next", models)
	}
	// Replacing the first provider's list moves the default model with it.
	if got := field(t, g, "llm.model").Default; got != "claude-next" {
		t.Errorf("llm.model default = %v, want claude-next", got)
	}
	// The other provider is untouched.
	if got := field(t, g, "llm.model").DynamicOptions.Options["openai"]; len(got) != 3 {
		t.Errorf("openai models = %+v, want the built-in three", got)
	}
}

func TestWithExtraModels_Appends(t *testing.T) {
	g := LLMSettingsGroup(WithExtraModels("anthropic",
		settings.SelectOption{Label: "Claude Next", Value: "claude-next"},
	))

	models := field(t, g, "llm.model").DynamicOptions.Options["anthropic"]
	if len(models) != 4 || models[0].Value != "claude-sonnet-4-6" || models[3].Value != "claude-next" {
		t.Errorf("anthropic models = %+v, want the built-ins plus claude-next", models)
	}
	// Appending must not change the default.
	if got := field(t, g, "llm.model").Default; got != "claude-sonnet-4-6" {
		t.Errorf("llm.model default = %v, want claude-sonnet-4-6", got)
	}
}

func TestWithModels_UnknownProviderIsNoOp(t *testing.T) {
	for _, opt := range []GroupOption{
		WithModels("nope", settings.SelectOption{Value: "x"}),
		WithExtraModels("nope", settings.SelectOption{Value: "x"}),
	} {
		g := LLMSettingsGroup(opt)
		if got, want := fieldKeys(g), fieldKeys(LLMSettingsGroup()); !reflect.DeepEqual(got, want) {
			t.Errorf("option on an unknown provider changed the group: %v", got)
		}
	}
}

func TestWithProvider_AddsThirdProvider(t *testing.T) {
	g := LLMSettingsGroup(WithProvider(ProviderSpec{
		Name:   "acme",
		Label:  "Acme AI",
		Models: []settings.SelectOption{{Label: "Acme One", Value: "acme-1"}},
	}))

	opts := field(t, g, "llm.provider").Options
	if len(opts) != 3 || opts[2].Value != "acme" || opts[2].Label != "Acme AI" {
		t.Errorf("provider options = %+v, want Acme AI appended", opts)
	}
	for _, key := range []string{"llm.acme.baseURL", "llm.acme.secret", "llm.acme.customModel"} {
		f := field(t, g, key)
		if f.Condition == nil || f.Condition.Equals[0] != "acme" {
			t.Errorf("%s condition = %+v, want gating on acme", key, f.Condition)
		}
	}
	// A spec with no APIFormats gets no apiFormat field.
	for _, f := range g.Fields {
		if f.Key == "llm.acme.apiFormat" {
			t.Error("a spec with no APIFormats produced an apiFormat field")
		}
	}
	if got := field(t, g, "llm.model").DynamicOptions.Options["acme"]; len(got) != 1 {
		t.Errorf("acme models = %+v", got)
	}
}

func TestWithProvider_ReplacesInPlace(t *testing.T) {
	g := LLMSettingsGroup(WithProvider(ProviderSpec{
		Name:   "anthropic",
		Label:  "Anthropic (pinned)",
		Models: []settings.SelectOption{{Label: "Pinned", Value: "pinned"}},
	}))

	opts := field(t, g, "llm.provider").Options
	if len(opts) != 2 || opts[0].Value != "anthropic" || opts[0].Label != "Anthropic (pinned)" {
		t.Errorf("provider options = %+v, want anthropic replaced in place", opts)
	}
	// The replacement carried no APIFormats, so the field is gone.
	for _, f := range g.Fields {
		if f.Key == "llm.anthropic.apiFormat" {
			t.Error("apiFormat survived a replacement that declared no APIFormats")
		}
	}
}

func TestProviderSpec_LabelDefaultsToName(t *testing.T) {
	g := LLMSettingsGroup(WithProvider(ProviderSpec{Name: "acme"}))
	opts := field(t, g, "llm.provider").Options
	if opts[2].Label != "acme" {
		t.Errorf("label = %q, want the name %q", opts[2].Label, "acme")
	}
}

// A provider with no models still gets a Custom Model ID field, so the schema
// must stay coherent rather than carrying an empty select.
func TestProviderSpec_NoModels(t *testing.T) {
	g := LLMSettingsGroup(WithProviders("openai"), WithModels("openai"))
	if got := field(t, g, "llm.model").Default; got != nil {
		t.Errorf("llm.model default = %v, want nil", got)
	}
	if _, ok := field(t, g, "llm.model").DynamicOptions.Options["openai"]; ok {
		t.Error("a provider with no models produced an empty options entry")
	}
	field(t, g, "llm.openai.customModel")
}

func TestWithProvider_CustomAPIFormats(t *testing.T) {
	g := LLMSettingsGroup(WithProvider(ProviderSpec{
		Name: "acme",
		APIFormats: []settings.SelectOption{
			{Label: "Acme Native", Value: "acme-native"},
			{Label: "OpenAI Compatible", Value: APIFormatOpenAICompatible},
		},
	}))
	f := field(t, g, "llm.acme.apiFormat")
	if f.Default != "acme-native" {
		t.Errorf("apiFormat default = %v, want the first option", f.Default)
	}
	if len(f.Options) != 2 {
		t.Errorf("apiFormat options = %+v", f.Options)
	}
}

// Options apply in order, so a provider must be added before it can be edited.
func TestGroupOptionsApplyInOrder(t *testing.T) {
	g := LLMSettingsGroup(
		WithProvider(ProviderSpec{Name: "acme", Models: []settings.SelectOption{{Value: "acme-1"}}}),
		WithExtraModels("acme", settings.SelectOption{Value: "acme-2"}),
		WithProviders("acme"),
	)
	models := field(t, g, "llm.model").DynamicOptions.Options["acme"]
	if len(models) != 2 || models[1].Value != "acme-2" {
		t.Errorf("acme models = %+v, want acme-1 then acme-2", models)
	}
	if got := field(t, g, "llm.provider").Default; got != "acme" {
		t.Errorf("default provider = %v, want acme", got)
	}
}

// BuiltinProviderSpecs must hand out a fresh copy: a caller that edits the
// result and feeds it back through WithProvider must not have mutated the
// defaults every other call sees.
func TestBuiltinProviderSpecsIsACopy(t *testing.T) {
	specs := BuiltinProviderSpecs()
	specs[0].Models[0].Value = "mutated"
	specs[0].Label = "mutated"

	if got := BuiltinProviderSpecs()[0].Models[0].Value; got != "claude-sonnet-4-6" {
		t.Errorf("BuiltinProviderSpecs()[0].Models[0].Value = %q after a caller mutated an earlier copy", got)
	}
	if got := LLMSettingsGroup(); field(t, got, "llm.model").Default != "claude-sonnet-4-6" {
		t.Error("LLMSettingsGroup was affected by a mutation of an earlier BuiltinProviderSpecs result")
	}
}

// Two groups built in sequence must not share slice backing, or an option
// applied to one would leak into the other.
func TestLLMSettingsGroupIsIndependentPerCall(t *testing.T) {
	restricted := LLMSettingsGroup(WithProviders("openai"))
	full := LLMSettingsGroup()

	if len(field(t, full, "llm.provider").Options) != 2 {
		t.Error("a later LLMSettingsGroup() inherited an earlier call's restriction")
	}
	if len(field(t, restricted, "llm.provider").Options) != 1 {
		t.Error("the restricted group changed after another was built")
	}
}

func TestComputeResolvedModelID(t *testing.T) {
	resetFactories()
	RegisterProvider("acme",
		func(modelID string, _ ProviderConfig) Provider { return &stubProvider{name: "acme"} },
		WithTransportResolver(func(values map[string]any, keyPrefix, modelID string) (string, string) {
			if format, _ := values[keyPrefix+"apiFormat"].(string); format == APIFormatOpenAICompatible {
				return "openai", "acme/" + modelID
			}
			return "acme", modelID
		}),
	)

	tests := []struct {
		name   string
		values map[string]any
		want   any
	}{
		{
			name:   "plain model",
			values: map[string]any{"llm.provider": "acme", "llm.model": "acme-1"},
			want:   "acme-1",
		},
		{
			name: "custom model wins over the selected one",
			values: map[string]any{
				"llm.provider": "acme", "llm.model": "acme-1",
				"llm.acme.customModel": "acme-experimental",
			},
			want: "acme-experimental",
		},
		{
			// The displayed value must match what the factory would send, which
			// is the whole point of routing it through ConfigFromValues.
			name: "reflects the provider's transport resolver",
			values: map[string]any{
				"llm.provider": "acme", "llm.model": "acme-1",
				"llm.acme.apiFormat": APIFormatOpenAICompatible,
			},
			want: "acme/acme-1",
		},
		{
			name:   "no model configured",
			values: map[string]any{"llm.provider": "acme"},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := computeResolvedModelID(tt.values); got != tt.want {
				t.Errorf("computeResolvedModelID = %v, want %v", got, tt.want)
			}
			// It must agree with the factory, always.
			_, modelID, _ := ConfigFromValues(tt.values)
			if got := computeResolvedModelID(tt.values); got != modelID {
				t.Errorf("computed %v but ConfigFromValues resolved %q", got, modelID)
			}
		})
	}
}

// Whatever the options, the group handed to settings.NewService must be a
// structurally valid schema. This is the guard against drift between what this
// package generates and what settings.ValidateSchema requires.
func TestLLMSettingsGroupPassesSchemaValidation(t *testing.T) {
	tests := []struct {
		name string
		opts []GroupOption
	}{
		{"no options", nil},
		{"restricted", []GroupOption{WithProviders("openai")}},
		{"reordered", []GroupOption{WithProviders("openai", "anthropic")}},
		{"models replaced", []GroupOption{WithModels("anthropic",
			settings.SelectOption{Label: "Claude Next", Value: "claude-next"})}},
		{"models appended", []GroupOption{WithExtraModels("openai",
			settings.SelectOption{Label: "GPT Next", Value: "gpt-next"})}},
		{"third provider", []GroupOption{WithProvider(ProviderSpec{
			Name:   "acme",
			Label:  "Acme AI",
			Models: []settings.SelectOption{{Label: "Acme One", Value: "acme-1"}},
			APIFormats: []settings.SelectOption{
				{Label: "Acme Native", Value: "acme-native"},
				{Label: "OpenAI Compatible", Value: APIFormatOpenAICompatible},
			},
		})}},
		{"provider with no models", []GroupOption{WithProviders("openai"), WithModels("openai")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := LLMSettingsGroup(tt.opts...)
			if err := settings.ValidateSchema(settings.Schema{Groups: []settings.Group{g}}); err != nil {
				t.Fatalf("ValidateSchema: %v", err)
			}
			// And it must survive an actual service construction, which is what
			// consumers do with it.
			svc, err := settings.NewService(
				settings.WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
				settings.WithSecretStore(settings.NewMemorySecretStore()),
				settings.WithGroup(g),
			)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}
			if _, err := svc.GetValues(); err != nil {
				t.Fatalf("GetValues: %v", err)
			}
		})
	}
}

// Naming only providers that do not exist filters everything away. That is a
// developer error, and it must surface as a rejected schema at startup rather
// than as a settings screen with an unusable provider dropdown.
func TestWithProviders_FilteringEverythingAwayIsRejected(t *testing.T) {
	g := LLMSettingsGroup(WithProviders("nope"))
	if err := settings.ValidateSchema(settings.Schema{Groups: []settings.Group{g}}); err == nil {
		t.Fatal("a group with no providers passed schema validation")
	}
}
