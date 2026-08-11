package settings

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/keyring"
)

func TestNewService_WithAppName(t *testing.T) {
	svc := NewService(WithAppName("testapp"))
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("no config dir available")
	}
	expected := filepath.Join(configDir, "testapp", "settings.json")
	if svc.store.Path() != expected {
		t.Errorf("expected store path %q, got %q", expected, svc.store.Path())
	}
}

func TestNewService_WithStoragePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workspace", "config.json")

	svc := NewService(
		WithStoragePath(path),
		WithGroup(Group{
			Key:   "general",
			Label: i18n.Text{Other: "General"},
			Fields: []Field{
				{Key: "name", Type: FieldText, Label: i18n.Text{Other: "Name"}, Default: "default"},
			},
		}),
	)

	if svc.store.Path() != path {
		t.Errorf("expected store path %q, got %q", path, svc.store.Path())
	}

	// Save and verify it writes to the custom path
	_, err := svc.SetValues(map[string]any{"name": "workspace-value"})
	if err != nil {
		t.Fatalf("save error: %v", err)
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if values["name"] != "workspace-value" {
		t.Errorf("expected name=workspace-value, got %v", values["name"])
	}
}

func TestWithStoragePath_OverridesWithAppName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.json")

	// WithStoragePath after WithAppName should override
	svc := NewService(
		WithAppName("myapp"),
		WithStoragePath(path),
	)

	if svc.store.Path() != path {
		t.Errorf("expected WithStoragePath to override WithAppName, got %q", svc.store.Path())
	}
}

func TestWithStoragePath_PasswordNeverInFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	secrets := keyring.NewMemoryStore()

	svc := NewService(
		WithStoragePath(path),
		WithKeyring(secrets),
		WithGroup(Group{
			Key:   "auth",
			Label: i18n.Text{Other: "Auth"},
			Fields: []Field{
				{Key: "token", Type: FieldPassword, Label: i18n.Text{Other: "Token"}},
				{Key: "host", Type: FieldText, Label: i18n.Text{Other: "Host"}},
			},
		}),
	)

	_, err := svc.SetValues(map[string]any{"token": "secret-value", "host": "example.com"})
	if err != nil {
		t.Fatal(err)
	}

	// Token should be in keyring, not in the file
	val, err := secrets.Get("token")
	if err != nil || val != "secret-value" {
		t.Fatalf("expected token in keyring, got %q, err=%v", val, err)
	}

	raw := NewStore("app", WithPath(path))
	saved, _ := raw.Load()
	if _, ok := saved["token"]; ok {
		t.Error("password must never be written to workspace-local file")
	}
	if saved["host"] != "example.com" {
		t.Errorf("expected host=example.com, got %v", saved["host"])
	}
}

func TestNewService_RegistersDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := NewService(
		WithStorePath(path),
		WithGroup(Group{
			Key:   "general",
			Label: i18n.Text{Other: "General"},
			Fields: []Field{
				{Key: "theme", Type: FieldSelect, Label: i18n.Text{Other: "Theme"}, Default: "dark"},
				{Key: "lang", Type: FieldSelect, Label: i18n.Text{Other: "Language"}, Default: "en"},
				{Key: "notes", Type: FieldText, Label: i18n.Text{Other: "Notes"}},
			},
		}),
	)

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values["theme"] != "dark" {
		t.Errorf("expected default theme=dark, got %v", values["theme"])
	}
	if values["lang"] != "en" {
		t.Errorf("expected default lang=en, got %v", values["lang"])
	}
	if _, ok := values["notes"]; ok {
		t.Errorf("expected notes to not be in defaults (no Default set)")
	}
}

func TestGetSchema_ReturnsAllGroups(t *testing.T) {
	g1 := Group{Key: "g1", Label: i18n.Text{Other: "Group 1"}, Fields: []Field{{Key: "f1", Type: FieldText, Label: i18n.Text{Other: "F1"}}}}
	g2 := Group{Key: "g2", Label: i18n.Text{Other: "Group 2"}, Fields: []Field{{Key: "f2", Type: FieldToggle, Label: i18n.Text{Other: "F2"}}}}

	svc := NewService(WithGroup(g1), WithGroup(g2))
	schema := svc.GetSchema()

	if len(schema.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(schema.Groups))
	}
	if schema.Groups[0].Key != "g1" {
		t.Errorf("expected first group key=g1, got %s", schema.Groups[0].Key)
	}
	if schema.Groups[1].Key != "g2" {
		t.Errorf("expected second group key=g2, got %s", schema.Groups[1].Key)
	}
}

func TestGetValues_AppliesComputeFuncs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := NewService(
		WithStorePath(path),
		WithGroup(Group{
			Key:   "info",
			Label: i18n.Text{Other: "Info"},
			Fields: []Field{
				{Key: "first", Type: FieldText, Label: i18n.Text{Other: "First"}, Default: "John"},
				{Key: "last", Type: FieldText, Label: i18n.Text{Other: "Last"}, Default: "Doe"},
				{Key: "full_name", Type: FieldComputed, Label: i18n.Text{Other: "Full Name"}},
			},
			ComputeFuncs: map[string]ComputeFunc{
				"full_name": func(values map[string]any) any {
					first, _ := values["first"].(string)
					last, _ := values["last"].(string)
					return first + " " + last
				},
			},
		}),
	)

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values["full_name"] != "John Doe" {
		t.Errorf("expected computed full_name=John Doe, got %v", values["full_name"])
	}
}

func TestSetValues_ValidatesAndSaves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := NewService(
		WithStorePath(path),
		WithGroup(Group{
			Key:   "config",
			Label: i18n.Text{Other: "Config"},
			Fields: []Field{
				{Key: "name", Type: FieldText, Label: i18n.Text{Other: "Name"}, Validation: &Validation{Required: true}},
			},
		}),
	)

	// Valid save
	errs, err := svc.SetValues(map[string]any{"name": "Alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if errs != nil {
		t.Fatalf("unexpected validation errors: %v", errs)
	}

	// Verify persisted
	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if values["name"] != "Alice" {
		t.Errorf("expected name=Alice after save, got %v", values["name"])
	}
}

func TestSetValues_ReturnsValidationErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := NewService(
		WithStorePath(path),
		WithGroup(Group{
			Key:   "config",
			Label: i18n.Text{Other: "Config"},
			Fields: []Field{
				{Key: "name", Type: FieldText, Label: i18n.Text{Other: "Name"}, Validation: &Validation{Required: true}},
			},
		}),
	)

	errs, err := svc.SetValues(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 validation error, got %d", len(errs))
	}
	if errs[0].Field != "name" {
		t.Errorf("expected validation error for name, got %s", errs[0].Field)
	}

	// Verify nothing was persisted (file should not exist)
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("expected settings file to not exist after validation failure")
	}
}

func TestSetValues_StripsComputedFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := NewService(
		WithStorePath(path),
		WithGroup(Group{
			Key:   "info",
			Label: i18n.Text{Other: "Info"},
			Fields: []Field{
				{Key: "first", Type: FieldText, Label: i18n.Text{Other: "First"}},
				{Key: "display", Type: FieldComputed, Label: i18n.Text{Other: "Display"}},
			},
			ComputeFuncs: map[string]ComputeFunc{
				"display": func(values map[string]any) any {
					return "computed"
				},
			},
		}),
	)

	_, err := svc.SetValues(map[string]any{"first": "Bob", "display": "should-be-stripped"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Load raw file to verify computed field was not saved
	raw := NewStore("app", WithPath(path))
	saved, err := raw.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if _, ok := saved["display"]; ok {
		t.Error("expected computed field 'display' to be stripped from saved data")
	}
	if saved["first"] != "Bob" {
		t.Errorf("expected first=Bob, got %v", saved["first"])
	}
}

func TestWithOnChange_CalledAfterSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	var called bool
	var receivedValues map[string]any

	svc := NewService(
		WithStorePath(path),
		WithGroup(Group{
			Key:   "config",
			Label: i18n.Text{Other: "Config"},
			Fields: []Field{
				{Key: "key", Type: FieldText, Label: i18n.Text{Other: "Key"}},
			},
		}),
		WithOnChange(func(values map[string]any) {
			called = true
			receivedValues = values
		}),
	)

	_, err := svc.SetValues(map[string]any{"key": "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected onChange callback to be called")
	}
	if receivedValues["key"] != "value" {
		t.Errorf("expected onChange to receive key=value, got %v", receivedValues["key"])
	}
}

func TestWithOnChange_NotCalledOnValidationFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	var called bool

	svc := NewService(
		WithStorePath(path),
		WithGroup(Group{
			Key:   "config",
			Label: i18n.Text{Other: "Config"},
			Fields: []Field{
				{Key: "name", Type: FieldText, Label: i18n.Text{Other: "Name"}, Validation: &Validation{Required: true}},
			},
		}),
		WithOnChange(func(values map[string]any) {
			called = true
		}),
	)

	_, _ = svc.SetValues(map[string]any{})
	if called {
		t.Error("expected onChange NOT to be called on validation failure")
	}
}

func TestMultipleGroups_Compose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := NewService(
		WithStorePath(path),
		WithGroup(Group{
			Key:   "appearance",
			Label: i18n.Text{Other: "Appearance"},
			Fields: []Field{
				{Key: "theme", Type: FieldSelect, Label: i18n.Text{Other: "Theme"}, Default: "light"},
			},
		}),
		WithGroup(Group{
			Key:   "connection",
			Label: i18n.Text{Other: "Connection"},
			Fields: []Field{
				{Key: "url", Type: FieldText, Label: i18n.Text{Other: "URL"}, Default: "https://example.com"},
				{Key: "timeout", Type: FieldNumber, Label: i18n.Text{Other: "Timeout"}, Default: float64(30)},
			},
		}),
	)

	schema := svc.GetSchema()
	if len(schema.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(schema.Groups))
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values["theme"] != "light" {
		t.Errorf("expected theme=light, got %v", values["theme"])
	}
	if values["url"] != "https://example.com" {
		t.Errorf("expected url=https://example.com, got %v", values["url"])
	}
	if values["timeout"] != float64(30) {
		t.Errorf("expected timeout=30, got %v", values["timeout"])
	}

	// Save and reload
	_, err = svc.SetValues(map[string]any{
		"theme":   "dark",
		"url":     "https://other.com",
		"timeout": float64(60),
	})
	if err != nil {
		t.Fatalf("save error: %v", err)
	}

	values, err = svc.GetValues()
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}
	if values["theme"] != "dark" {
		t.Errorf("expected theme=dark, got %v", values["theme"])
	}
	if values["timeout"] != float64(60) {
		t.Errorf("expected timeout=60, got %v", values["timeout"])
	}
}

// --- Password / Keyring tests ---

func TestPasswordField_StoredInKeyring(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	secrets := keyring.NewMemoryStore()

	svc := NewService(
		WithStorePath(path),
		WithKeyring(secrets),
		WithGroup(Group{
			Key:   "auth",
			Label: i18n.Text{Other: "Auth"},
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: i18n.Text{Other: "API Key"}},
				{Key: "host", Type: FieldText, Label: i18n.Text{Other: "Host"}},
			},
		}),
	)

	// Save with a password
	_, err := svc.SetValues(map[string]any{"api_key": "sk-secret123", "host": "example.com"})
	if err != nil {
		t.Fatal(err)
	}

	// Password should be in keyring
	val, err := secrets.Get("api_key")
	if err != nil {
		t.Fatalf("expected secret in keyring: %v", err)
	}
	if val != "sk-secret123" {
		t.Errorf("expected sk-secret123, got %s", val)
	}

	// Password should NOT be in the JSON file
	raw := NewStore("app", WithPath(path))
	saved, _ := raw.Load()
	if _, ok := saved["api_key"]; ok {
		t.Error("expected password to NOT be in JSON file")
	}
	if saved["host"] != "example.com" {
		t.Errorf("expected host=example.com, got %v", saved["host"])
	}
}

func TestPasswordField_MaskedInGetValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	secrets := keyring.NewMemoryStore()
	_ = secrets.Set("api_key", "realvalue")

	svc := NewService(
		WithStorePath(path),
		WithKeyring(secrets),
		WithGroup(Group{
			Key:   "auth",
			Label: i18n.Text{Other: "Auth"},
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: i18n.Text{Other: "API Key"}},
			},
		}),
	)

	values, err := svc.GetValues()
	if err != nil {
		t.Fatal(err)
	}
	if values["api_key"] != SecretMask {
		t.Errorf("expected masked value %q, got %v", SecretMask, values["api_key"])
	}
}

func TestPasswordField_MaskSentinelIsNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	secrets := keyring.NewMemoryStore()
	_ = secrets.Set("api_key", "original")

	svc := NewService(
		WithStorePath(path),
		WithKeyring(secrets),
		WithGroup(Group{
			Key:   "auth",
			Label: i18n.Text{Other: "Auth"},
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: i18n.Text{Other: "API Key"}},
			},
		}),
	)

	// Sending the mask sentinel should not change the stored value
	_, err := svc.SetValues(map[string]any{"api_key": SecretMask})
	if err != nil {
		t.Fatal(err)
	}

	val, _ := secrets.Get("api_key")
	if val != "original" {
		t.Errorf("expected original value preserved, got %s", val)
	}
}

func TestPasswordField_EmptyClearsSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	secrets := keyring.NewMemoryStore()
	_ = secrets.Set("api_key", "todelete")

	svc := NewService(
		WithStorePath(path),
		WithKeyring(secrets),
		WithGroup(Group{
			Key:   "auth",
			Label: i18n.Text{Other: "Auth"},
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: i18n.Text{Other: "API Key"}},
			},
		}),
	)

	// Empty string clears the secret
	_, err := svc.SetValues(map[string]any{"api_key": ""})
	if err != nil {
		t.Fatal(err)
	}

	if secrets.Has("api_key") {
		t.Error("expected secret to be deleted from keyring")
	}
}

func TestPasswordField_UnsetReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := NewService(
		WithStorePath(path),
		WithGroup(Group{
			Key:   "auth",
			Label: i18n.Text{Other: "Auth"},
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: i18n.Text{Other: "API Key"}},
			},
		}),
	)

	values, _ := svc.GetValues()
	if values["api_key"] != "" {
		t.Errorf("expected empty string for unset password, got %v", values["api_key"])
	}
}

func TestGetSecret_ReturnsActualValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	secrets := keyring.NewMemoryStore()

	svc := NewService(
		WithStorePath(path),
		WithKeyring(secrets),
		WithGroup(Group{
			Key:   "auth",
			Label: i18n.Text{Other: "Auth"},
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: i18n.Text{Other: "API Key"}},
			},
		}),
	)

	_, err := svc.SetValues(map[string]any{"api_key": "real-secret"})
	if err != nil {
		t.Fatal(err)
	}

	val, err := svc.GetSecret("api_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "real-secret" {
		t.Errorf("expected real-secret, got %s", val)
	}
}

// --- Defect regression tests (WP-03) ---

// TestValidateEffectiveState pins defect #2: validation must run against
// effective state (defaults + persisted + submission), not the raw
// submitted payload. Before the fix, omitting a condition's controlling
// field from a partial update made conditionMet see "" and treat the
// dependent field as hidden, skipping its validation even though the
// field's real (persisted) controlling value would have required it.
func TestValidateEffectiveState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	secrets := keyring.NewMemoryStore()

	svc := NewService(
		WithStoragePath(path),
		WithKeyring(secrets),
		WithGroup(Group{
			Key:   "llm",
			Label: i18n.Text{Other: "LLM"},
			Fields: []Field{
				{
					Key:   "provider",
					Type:  FieldSelect,
					Label: i18n.Text{Other: "Provider"},
					Options: []SelectOption{
						{Label: i18n.Text{Other: "OpenAI"}, Value: "openai"},
						{Label: i18n.Text{Other: "Local"}, Value: "local"},
					},
				},
				{
					Key:        "api_key",
					Type:       FieldPassword,
					Label:      i18n.Text{Other: "API Key"},
					Validation: &Validation{Required: true},
					Condition:  &Condition{Field: "provider", Equals: []string{"openai"}},
				},
			},
		}),
	)

	// Establish persisted state: provider=openai with a real api_key.
	if _, err := svc.SetValues(map[string]any{"provider": "openai", "api_key": "sk-real"}); err != nil {
		t.Fatalf("setup save error: %v", err)
	}

	// Submit an update that only touches api_key and omits "provider"
	// entirely. Before the fix, Validate ran on this partial payload alone:
	// values["provider"] is absent, conditionMet sees "" != "openai",
	// treats api_key as hidden, skips Required entirely, and the empty
	// value is accepted — silently deleting a secret that's still required
	// under the persisted provider=openai.
	errs, err := svc.SetValues(map[string]any{"api_key": ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 1 || errs[0].Field != "api_key" {
		t.Fatalf("expected a required-field error for api_key using the persisted provider=openai, got %v", errs)
	}
	if errs[0].Code != CodeRequired {
		t.Errorf("expected code=%s, got %s", CodeRequired, errs[0].Code)
	}

	// The secret must remain untouched — the rejected submission must not
	// have reached the keyring-deletion branch.
	val, err := secrets.Get("api_key")
	if err != nil || val != "sk-real" {
		t.Fatalf("expected api_key to remain sk-real, got %q, err=%v", val, err)
	}
}

// TestValidateEffectiveState_DynamicSelectUsesPersistedParent covers the
// "same class of bug" the WP-03 review called out: a dynamic-select field
// accepting an arbitrary value when its parent (DependsOn) field isn't part
// of the submission.
func TestValidateEffectiveState_DynamicSelectUsesPersistedParent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := NewService(
		WithStoragePath(path),
		WithGroup(Group{
			Key:   "llm",
			Label: i18n.Text{Other: "LLM"},
			Fields: []Field{
				{
					Key:   "provider",
					Type:  FieldSelect,
					Label: i18n.Text{Other: "Provider"},
					Options: []SelectOption{
						{Label: i18n.Text{Other: "Anthropic"}, Value: "anthropic"},
						{Label: i18n.Text{Other: "OpenAI"}, Value: "openai"},
					},
				},
				{
					Key:   "model",
					Type:  FieldSelect,
					Label: i18n.Text{Other: "Model"},
					DynamicOptions: &DynamicOptions{
						DependsOn: "provider",
						Options: map[string][]SelectOption{
							"anthropic": {{Label: i18n.Text{Other: "Claude"}, Value: "claude"}},
							"openai":    {{Label: i18n.Text{Other: "GPT-4o"}, Value: "gpt-4o"}},
						},
					},
				},
			},
		}),
	)

	// Establish persisted state: provider=anthropic.
	if _, err := svc.SetValues(map[string]any{"provider": "anthropic", "model": "claude"}); err != nil {
		t.Fatalf("setup save error: %v", err)
	}

	// Submit a model value that is only valid for "openai", without
	// resubmitting "provider". Before the fix, Validate saw only
	// {"model": "gpt-4o"}: DependsOn lookup on the submitted payload found
	// no "provider" key, hasSelectableOptions returned false for the
	// missing dependency, and the value was accepted outright — an
	// unvalidated write of an option that doesn't belong to the persisted
	// provider.
	errs, err := svc.SetValues(map[string]any{"model": "gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 1 || errs[0].Field != "model" {
		t.Fatalf("expected invalid-option error for model using persisted provider=anthropic, got %v", errs)
	}
}

// TestSetValues_DynamicOptionParentChangeResetsStrandedDependent pins H2: a
// submission that changes only a DynamicOptions parent field must not be
// rejected because a dependent field's stale value (persisted or default,
// from the *previous* parent value) is no longer a valid option for the new
// parent value. Before the fix, `{"llm.provider": "openai"}` alone — the
// single most likely thing a real app submits — failed validation with an
// invalid-option error on "llm.model", a field the caller never touched.
func TestSetValues_DynamicOptionParentChangeResetsStrandedDependent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := NewService(
		WithStoragePath(path),
		WithGroup(Group{
			Key:   "llm",
			Label: i18n.Text{Other: "LLM"},
			Fields: []Field{
				{
					Key:     "llm.provider",
					Type:    FieldSelect,
					Label:   i18n.Text{Other: "Provider"},
					Default: "anthropic",
					Options: []SelectOption{
						{Label: i18n.Text{Other: "Anthropic"}, Value: "anthropic"},
						{Label: i18n.Text{Other: "OpenAI"}, Value: "openai"},
					},
				},
				{
					Key:     "llm.model",
					Type:    FieldSelect,
					Label:   i18n.Text{Other: "Model"},
					Default: "claude-sonnet-4-6",
					DynamicOptions: &DynamicOptions{
						DependsOn: "llm.provider",
						Options: map[string][]SelectOption{
							"anthropic": {{Label: i18n.Text{Other: "Claude Sonnet 4.6"}, Value: "claude-sonnet-4-6"}},
							"openai":    {{Label: i18n.Text{Other: "GPT-4o"}, Value: "gpt-4o"}, {Label: i18n.Text{Other: "GPT-4o Mini"}, Value: "gpt-4o-mini"}},
						},
					},
				},
			},
		}),
	)

	// Defaults alone give provider=anthropic, model=claude-sonnet-4-6 —
	// nothing persisted yet, matching the reproduction in the defect report.
	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if values["llm.provider"] != "anthropic" || values["llm.model"] != "claude-sonnet-4-6" {
		t.Fatalf("unexpected starting defaults: %v", values)
	}

	// Submit only the provider switch — exactly the H2 reproduction.
	errs, err := svc.SetValues(map[string]any{"llm.provider": "openai"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected the provider-only switch to succeed, got validation errors: %v", errs)
	}

	values, err = svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if values["llm.provider"] != "openai" {
		t.Errorf("expected llm.provider=openai, got %v", values["llm.provider"])
	}
	if values["llm.model"] != "gpt-4o" {
		t.Errorf("expected llm.model reset to the first openai option gpt-4o, got %v", values["llm.model"])
	}
}

// TestSetValues_DynamicOptionExplicitMismatchStillRejected is the guard rail
// for the H2 fix: if the caller explicitly submits both the parent and an
// invalid dependent value in the same call, that is a genuinely inconsistent
// submission and effective-state validation must still catch it — only a
// value the caller didn't touch this call gets auto-corrected.
func TestSetValues_DynamicOptionExplicitMismatchStillRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := NewService(
		WithStoragePath(path),
		WithGroup(Group{
			Key:   "llm",
			Label: i18n.Text{Other: "LLM"},
			Fields: []Field{
				{
					Key:     "llm.provider",
					Type:    FieldSelect,
					Label:   i18n.Text{Other: "Provider"},
					Default: "anthropic",
					Options: []SelectOption{
						{Label: i18n.Text{Other: "Anthropic"}, Value: "anthropic"},
						{Label: i18n.Text{Other: "OpenAI"}, Value: "openai"},
					},
				},
				{
					Key:     "llm.model",
					Type:    FieldSelect,
					Label:   i18n.Text{Other: "Model"},
					Default: "claude-sonnet-4-6",
					DynamicOptions: &DynamicOptions{
						DependsOn: "llm.provider",
						Options: map[string][]SelectOption{
							"anthropic": {{Label: i18n.Text{Other: "Claude Sonnet 4.6"}, Value: "claude-sonnet-4-6"}},
							"openai":    {{Label: i18n.Text{Other: "GPT-4o"}, Value: "gpt-4o"}},
						},
					},
				},
			},
		}),
	)

	// Both provider and an invalid model for that provider submitted
	// together — this is a real inconsistency, not a stranded default.
	errs, err := svc.SetValues(map[string]any{"llm.provider": "openai", "llm.model": "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 1 || errs[0].Field != "llm.model" {
		t.Fatalf("expected an explicit mismatch to still be rejected, got %v", errs)
	}
}

// TestPasswordNonString pins defect #3: a non-string value submitted for a
// password field must be rejected as a validation error, never coerced to
// "" via a failed type assertion and used to delete the stored secret.
func TestPasswordNonString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	secrets := keyring.NewMemoryStore()
	if err := secrets.Set("api_key", "original-secret"); err != nil {
		t.Fatal(err)
	}

	svc := NewService(
		WithStoragePath(path),
		WithKeyring(secrets),
		WithGroup(Group{
			Key:   "auth",
			Label: i18n.Text{Other: "Auth"},
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: i18n.Text{Other: "API Key"}},
			},
		}),
	)

	errs, err := svc.SetValues(map[string]any{"api_key": 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 1 || errs[0].Field != "api_key" {
		t.Fatalf("expected 1 validation error for non-string password, got %v", errs)
	}
	if errs[0].Code != CodeInvalidType {
		t.Errorf("expected code=%s, got %s", CodeInvalidType, errs[0].Code)
	}

	val, err := secrets.Get("api_key")
	if err != nil || val != "original-secret" {
		t.Fatalf("expected secret to remain untouched, got %q, err=%v", val, err)
	}
}

// TestOnChangeNoLock pins defect #4: onChange callbacks must run after the
// Service's internal mutex is released. A callback that calls back into the
// Service (a very natural thing to do — e.g. re-reading values, or writing
// a derived setting) must not deadlock. This test fails by timeout rather
// than hanging the suite if the bug regresses.
func TestOnChangeNoLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	var svc *Service
	var reentering atomic.Bool
	reentered := make(chan struct{})

	svc = NewService(
		WithStoragePath(path),
		WithGroup(Group{
			Key:   "config",
			Label: i18n.Text{Other: "Config"},
			Fields: []Field{
				{Key: "key", Type: FieldText, Label: i18n.Text{Other: "Key"}},
				{Key: "other", Type: FieldText, Label: i18n.Text{Other: "Other"}},
			},
		}),
		WithOnChange(func(values map[string]any) {
			// Re-entrant call into the service from inside the callback.
			// Guarded with an atomic CAS (not sync.Once — Once.Do is not
			// reentrant and deadlocks if called again from within its own
			// function) because this same callback fires again for the
			// reentrant SetValues call below; without the guard it would
			// recurse forever instead of deadlocking, which would defeat
			// the test.
			if reentering.CompareAndSwap(false, true) {
				if _, err := svc.SetValues(map[string]any{"other": "reentrant"}); err != nil {
					t.Errorf("reentrant SetValues failed: %v", err)
				}
				close(reentered)
			}
		}),
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := svc.SetValues(map[string]any{"key": "value"}); err != nil {
			t.Errorf("SetValues error: %v", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: SetValues did not return — onChange callback likely still held s.mu while calling back into the service")
	}

	select {
	case <-reentered:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: reentrant SetValues from within the onChange callback never completed")
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values["key"] != "value" || values["other"] != "reentrant" {
		t.Errorf("expected key=value, other=reentrant, got %v", values)
	}
}

// TestNewService_MemoryKeyringDefaultWarns pins the "loud, not silent"
// fix for the memory-keyring default: forgetting WithKeyring must not
// silently lose API keys on restart without at least a visible warning.
func TestNewService_MemoryKeyringDefaultWarns(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	_ = NewService(WithGroup(Group{
		Key:    "auth",
		Label:  i18n.Text{Other: "Auth"},
		Fields: []Field{{Key: "api_key", Type: FieldPassword, Label: i18n.Text{Other: "API Key"}}},
	}))

	if !strings.Contains(buf.String(), "keyring") {
		t.Errorf("expected a warning about the default in-memory keyring, got: %q", buf.String())
	}
}

// TestNewService_ExplicitKeyringNoWarning is the counterpart to the above:
// an explicitly configured keyring must not trigger the warning.
func TestNewService_ExplicitKeyringNoWarning(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	_ = NewService(WithKeyring(keyring.NewMemoryStore()))

	if buf.Len() != 0 {
		t.Errorf("expected no warning when a keyring is explicitly configured, got: %q", buf.String())
	}
}

// TestServiceOptions_StoragePathSurvivesAppNameAfter pins the
// option-ordering fix: WithStoragePath must win over WithAppName
// regardless of which is passed first. Before the fix, WithAppName
// unconditionally reconstructed s.store, discarding a storage path set by
// an earlier WithStoragePath call.
func TestServiceOptions_StoragePathSurvivesAppNameAfter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.json")

	svc := NewService(
		WithStoragePath(path),
		WithAppName("myapp"),
	)

	if svc.store.Path() != path {
		t.Errorf("expected WithStoragePath to survive a later WithAppName, got %q", svc.store.Path())
	}
}

// TestAddOnChange covers the post-construction hook. Packages that compose
// with settings are usually built after the Service (they need it to exist),
// so WithOnChange alone forces a forward-declared-closure dance at every such
// call site. appearance hit this first.
func TestAddOnChange(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(
		WithStoragePath(filepath.Join(dir, "settings.json")),
		WithKeyring(keyring.NewMemoryStore()),
		WithGroup(Group{
			Key:   "g",
			Label: i18n.Text{Other: "G"},
			Fields: []Field{
				{Key: "g.name", Type: FieldText, Label: i18n.Text{Other: "Name"}},
			},
		}),
	)

	var got map[string]any
	svc.AddOnChange(func(values map[string]any) { got = values })
	svc.AddOnChange(nil) // must be ignored, not panic

	if _, err := svc.SetValues(map[string]any{"g.name": "hello"}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if got == nil {
		t.Fatal("callback registered after construction was never invoked")
	}
	if got["g.name"] != "hello" {
		t.Errorf("callback saw %v, want hello", got["g.name"])
	}
}

// TestAddOnChangeNoDeadlock pins the same guarantee WithOnChange has: the
// callback runs with the lock released, so re-entering the Service is safe.
func TestAddOnChangeNoDeadlock(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(
		WithStoragePath(filepath.Join(dir, "settings.json")),
		WithKeyring(keyring.NewMemoryStore()),
		WithGroup(Group{
			Key:   "g",
			Label: i18n.Text{Other: "G"},
			Fields: []Field{
				{Key: "g.name", Type: FieldText, Label: i18n.Text{Other: "Name"}},
			},
		}),
	)

	svc.AddOnChange(func(map[string]any) {
		if _, err := svc.GetValues(); err != nil {
			t.Errorf("re-entrant GetValues: %v", err)
		}
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := svc.SetValues(map[string]any{"g.name": "x"}); err != nil {
			t.Errorf("SetValues: %v", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: callback re-entered the Service while the lock was held")
	}
}
