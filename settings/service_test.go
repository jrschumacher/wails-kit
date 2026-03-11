package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jrschumacher/wails-kit/keyring"
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
			Label: "General",
			Fields: []Field{
				{Key: "name", Type: FieldText, Label: "Name", Default: "default"},
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
			Label: "Auth",
			Fields: []Field{
				{Key: "token", Type: FieldPassword, Label: "Token"},
				{Key: "host", Type: FieldText, Label: "Host"},
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
			Label: "General",
			Fields: []Field{
				{Key: "theme", Type: FieldSelect, Label: "Theme", Default: "dark"},
				{Key: "lang", Type: FieldSelect, Label: "Language", Default: "en"},
				{Key: "notes", Type: FieldText, Label: "Notes"},
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
	g1 := Group{Key: "g1", Label: "Group 1", Fields: []Field{{Key: "f1", Type: FieldText, Label: "F1"}}}
	g2 := Group{Key: "g2", Label: "Group 2", Fields: []Field{{Key: "f2", Type: FieldToggle, Label: "F2"}}}

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
			Label: "Info",
			Fields: []Field{
				{Key: "first", Type: FieldText, Label: "First", Default: "John"},
				{Key: "last", Type: FieldText, Label: "Last", Default: "Doe"},
				{Key: "full_name", Type: FieldComputed, Label: "Full Name"},
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
			Label: "Config",
			Fields: []Field{
				{Key: "name", Type: FieldText, Label: "Name", Validation: &Validation{Required: true}},
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
			Label: "Config",
			Fields: []Field{
				{Key: "name", Type: FieldText, Label: "Name", Validation: &Validation{Required: true}},
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
			Label: "Info",
			Fields: []Field{
				{Key: "first", Type: FieldText, Label: "First"},
				{Key: "display", Type: FieldComputed, Label: "Display"},
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
			Label: "Config",
			Fields: []Field{
				{Key: "key", Type: FieldText, Label: "Key"},
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
			Label: "Config",
			Fields: []Field{
				{Key: "name", Type: FieldText, Label: "Name", Validation: &Validation{Required: true}},
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
			Label: "Appearance",
			Fields: []Field{
				{Key: "theme", Type: FieldSelect, Label: "Theme", Default: "light"},
			},
		}),
		WithGroup(Group{
			Key:   "connection",
			Label: "Connection",
			Fields: []Field{
				{Key: "url", Type: FieldText, Label: "URL", Default: "https://example.com"},
				{Key: "timeout", Type: FieldNumber, Label: "Timeout", Default: float64(30)},
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
			Label: "Auth",
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: "API Key"},
				{Key: "host", Type: FieldText, Label: "Host"},
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
			Label: "Auth",
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: "API Key"},
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
			Label: "Auth",
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: "API Key"},
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
			Label: "Auth",
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: "API Key"},
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
			Label: "Auth",
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: "API Key"},
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
			Label: "Auth",
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: "API Key"},
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
