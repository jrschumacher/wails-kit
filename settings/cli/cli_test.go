package cli

import (
	"bytes"
	"strings"
	"fmt"
	"testing"

	"github.com/jrschumacher/wails-kit/keyring"
	"github.com/jrschumacher/wails-kit/settings"
)

func testService(t *testing.T, groups ...settings.Group) *settings.Service {
	t.Helper()
	dir := t.TempDir()
	opts := []settings.ServiceOption{
		settings.WithStorePath(dir + "/settings.json"),
		settings.WithKeyring(keyring.NewMemoryStore()),
	}
	for _, g := range groups {
		opts = append(opts, settings.WithGroup(g))
	}
	return settings.NewService(opts...)
}

func basicGroup() settings.Group {
	return settings.Group{
		Key:   "general",
		Label: "General",
		Fields: []settings.Field{
			{Key: "name", Type: settings.FieldText, Label: "Name", Default: "World"},
			{Key: "theme", Type: settings.FieldSelect, Label: "Theme", Default: "dark", Options: []settings.SelectOption{
				{Label: "Dark", Value: "dark"},
				{Label: "Light", Value: "light"},
			}},
			{Key: "notifications", Type: settings.FieldToggle, Label: "Notifications", Default: true},
		},
	}
}

func TestShow(t *testing.T) {
	svc := testService(t, basicGroup())
	var buf bytes.Buffer
	err := Show(svc, WithOutput(&buf))
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, "[General]") {
		t.Error("expected group header")
	}
	if !strings.Contains(out, "name = World") {
		t.Errorf("expected default name, got:\n%s", out)
	}
	if !strings.Contains(out, "theme = dark") {
		t.Errorf("expected default theme, got:\n%s", out)
	}
	if !strings.Contains(out, "notifications = true") {
		t.Errorf("expected default notifications, got:\n%s", out)
	}
}

func TestShow_PasswordMasked(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "auth",
		Label: "Auth",
		Fields: []settings.Field{
			{Key: "api_key", Type: settings.FieldPassword, Label: "API Key"},
		},
	})

	// Set a password
	_, err := svc.SetValues(map[string]any{"api_key": "secret123"})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := Show(svc, WithOutput(&buf)); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), settings.SecretMask) {
		t.Errorf("expected masked password, got:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "secret123") {
		t.Error("password should not appear in output")
	}
}

func TestShow_ConditionalFieldHidden(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "llm",
		Label: "LLM",
		Fields: []settings.Field{
			{Key: "provider", Type: settings.FieldSelect, Label: "Provider", Default: "openai", Options: []settings.SelectOption{
				{Label: "OpenAI", Value: "openai"},
				{Label: "Anthropic", Value: "anthropic"},
			}},
			{Key: "anthropic_key", Type: settings.FieldText, Label: "Anthropic Key", Condition: &settings.Condition{
				Field: "provider", Equals: []string{"anthropic"},
			}},
		},
	})

	var buf bytes.Buffer
	if err := Show(svc, WithOutput(&buf)); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(buf.String(), "anthropic_key") {
		t.Error("conditional field should be hidden when condition not met")
	}
}

func TestSet(t *testing.T) {
	svc := testService(t, basicGroup())

	if err := Set(svc, "name", "Alice"); err != nil {
		t.Fatal(err)
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatal(err)
	}
	if values["name"] != "Alice" {
		t.Errorf("expected Alice, got %v", values["name"])
	}
}

func TestSet_Toggle(t *testing.T) {
	svc := testService(t, basicGroup())

	if err := Set(svc, "notifications", "false"); err != nil {
		t.Fatal(err)
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatal(err)
	}
	if values["notifications"] != false {
		t.Errorf("expected false, got %v", values["notifications"])
	}
}

func TestSet_Number(t *testing.T) {
	min, max := 1, 100
	svc := testService(t, settings.Group{
		Key:   "prefs",
		Label: "Preferences",
		Fields: []settings.Field{
			{Key: "font_size", Type: settings.FieldNumber, Label: "Font Size", Default: 14,
				Validation: &settings.Validation{Min: &min, Max: &max}},
		},
	})

	if err := Set(svc, "font_size", "16"); err != nil {
		t.Fatal(err)
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatal(err)
	}
	// JSON round-trip converts integers to float64
	if fmt.Sprintf("%v", values["font_size"]) != "16" {
		t.Errorf("expected 16, got %v (%T)", values["font_size"], values["font_size"])
	}
}

func TestSet_UnknownKey(t *testing.T) {
	svc := testService(t, basicGroup())
	err := Set(svc, "nonexistent", "value")
	if err == nil || !strings.Contains(err.Error(), "unknown setting") {
		t.Errorf("expected unknown setting error, got: %v", err)
	}
}

func TestSet_ComputedField(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "info",
		Label: "Info",
		Fields: []settings.Field{
			{Key: "computed_field", Type: settings.FieldComputed, Label: "Computed"},
		},
	})

	err := Set(svc, "computed_field", "value")
	if err == nil || !strings.Contains(err.Error(), "cannot set computed") {
		t.Errorf("expected computed field error, got: %v", err)
	}
}

func TestSet_ValidationError(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "prefs",
		Label: "Preferences",
		Fields: []settings.Field{
			{Key: "theme", Type: settings.FieldSelect, Label: "Theme", Options: []settings.SelectOption{
				{Label: "Dark", Value: "dark"},
				{Label: "Light", Value: "light"},
			}},
		},
	})

	err := Set(svc, "theme", "invalid")
	if err == nil {
		t.Fatal("expected validation error")
	}
	var verr *ValidationErrors
	if ve, ok := err.(*ValidationErrors); !ok {
		t.Errorf("expected *ValidationErrors, got %T: %v", err, err)
	} else {
		verr = ve
	}
	if verr != nil && len(verr.Errors) != 1 {
		t.Errorf("expected 1 validation error, got %d", len(verr.Errors))
	}
}

func TestConfigure_Interactive(t *testing.T) {
	svc := testService(t, basicGroup())

	// Provide input: name=Alice, theme=1 (dark), notifications=n
	input := "Alice\n1\nn\n"
	var buf bytes.Buffer
	err := Configure(svc, WithInput(strings.NewReader(input)), WithOutput(&buf))
	if err != nil {
		t.Fatal(err)
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatal(err)
	}
	if values["name"] != "Alice" {
		t.Errorf("expected Alice, got %v", values["name"])
	}
	if values["theme"] != "dark" {
		t.Errorf("expected dark, got %v", values["theme"])
	}
	if values["notifications"] != false {
		t.Errorf("expected false, got %v", values["notifications"])
	}
}

func TestConfigure_KeepDefaults(t *testing.T) {
	svc := testService(t, basicGroup())

	// All empty lines = keep defaults
	input := "\n\n\n"
	var buf bytes.Buffer
	err := Configure(svc, WithInput(strings.NewReader(input)), WithOutput(&buf))
	if err != nil {
		t.Fatal(err)
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatal(err)
	}
	if values["name"] != "World" {
		t.Errorf("expected World (default), got %v", values["name"])
	}
}

func TestConfigure_SkipsComputedFields(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "info",
		Label: "Info",
		Fields: []settings.Field{
			{Key: "first", Type: settings.FieldText, Label: "First Name"},
			{Key: "display", Type: settings.FieldComputed, Label: "Display Name"},
		},
		ComputeFuncs: map[string]settings.ComputeFunc{
			"display": func(values map[string]any) any {
				return values["first"]
			},
		},
	})

	// Only one prompt expected (first), not two
	input := "Alice\n"
	var buf bytes.Buffer
	err := Configure(svc, WithInput(strings.NewReader(input)), WithOutput(&buf))
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(buf.String(), "Display Name") {
		t.Error("computed field should not be prompted")
	}
}

func TestConfigure_SkipsConditionalFields(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "llm",
		Label: "LLM",
		Fields: []settings.Field{
			{Key: "provider", Type: settings.FieldSelect, Label: "Provider", Options: []settings.SelectOption{
				{Label: "OpenAI", Value: "openai"},
				{Label: "Anthropic", Value: "anthropic"},
			}},
			{Key: "anthropic_model", Type: settings.FieldText, Label: "Anthropic Model", Condition: &settings.Condition{
				Field: "provider", Equals: []string{"anthropic"},
			}},
			{Key: "openai_model", Type: settings.FieldText, Label: "OpenAI Model", Condition: &settings.Condition{
				Field: "provider", Equals: []string{"openai"},
			}},
		},
	})

	// Select openai (option 1), then provide openai model
	input := "1\ngpt-4\n"
	var buf bytes.Buffer
	err := Configure(svc, WithInput(strings.NewReader(input)), WithOutput(&buf))
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if strings.Contains(out, "Anthropic Model") {
		t.Error("anthropic_model should be skipped when provider=openai")
	}
	if !strings.Contains(out, "OpenAI Model") {
		t.Error("openai_model should be prompted when provider=openai")
	}
}

func TestConfigure_Password(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "auth",
		Label: "Auth",
		Fields: []settings.Field{
			{Key: "api_key", Type: settings.FieldPassword, Label: "API Key"},
		},
	})

	input := "sk-secret\n"
	var buf bytes.Buffer
	err := Configure(svc, WithInput(strings.NewReader(input)), WithOutput(&buf))
	if err != nil {
		t.Fatal(err)
	}

	secret, err := svc.GetSecret("api_key")
	if err != nil {
		t.Fatal(err)
	}
	if secret != "sk-secret" {
		t.Errorf("expected sk-secret, got %s", secret)
	}
}

func TestKeys(t *testing.T) {
	svc := testService(t, basicGroup())
	keys := Keys(svc)
	expected := []string{"name", "notifications", "theme"}
	if len(keys) != len(expected) {
		t.Fatalf("expected %d keys, got %d", len(expected), len(keys))
	}
	for i, k := range keys {
		if k != expected[i] {
			t.Errorf("expected key %s at position %d, got %s", expected[i], i, k)
		}
	}
}

func TestValidationErrors_Error(t *testing.T) {
	ve := &ValidationErrors{
		Errors: []settings.ValidationError{
			{Field: "name", Message: "Name is required", Code: "required"},
			{Field: "theme", Message: "Theme has an invalid option", Code: "invalid_option"},
		},
	}
	msg := ve.Error()
	if !strings.Contains(msg, "validation failed") {
		t.Error("expected 'validation failed' prefix")
	}
	if !strings.Contains(msg, "name: Name is required") {
		t.Error("expected first error in message")
	}
	if !strings.Contains(msg, "theme: Theme has an invalid option") {
		t.Error("expected second error in message")
	}
}

func TestCoerceValue(t *testing.T) {
	tests := []struct {
		field settings.Field
		input string
		want  any
		err   bool
	}{
		{settings.Field{Type: settings.FieldToggle}, "true", true, false},
		{settings.Field{Type: settings.FieldToggle}, "yes", true, false},
		{settings.Field{Type: settings.FieldToggle}, "false", false, false},
		{settings.Field{Type: settings.FieldToggle}, "no", false, false},
		{settings.Field{Type: settings.FieldToggle}, "invalid", nil, true},
		{settings.Field{Type: settings.FieldNumber}, "42", 42, false},
		{settings.Field{Type: settings.FieldNumber}, "3.14", 3.14, false},
		{settings.Field{Type: settings.FieldNumber}, "abc", nil, true},
		{settings.Field{Type: settings.FieldText}, "hello", "hello", false},
	}

	for _, tt := range tests {
		got, err := coerceValue(tt.field, tt.input)
		if tt.err && err == nil {
			t.Errorf("coerceValue(%s, %q): expected error", tt.field.Type, tt.input)
		}
		if !tt.err && err != nil {
			t.Errorf("coerceValue(%s, %q): unexpected error: %v", tt.field.Type, tt.input, err)
		}
		if !tt.err && got != tt.want {
			t.Errorf("coerceValue(%s, %q) = %v, want %v", tt.field.Type, tt.input, got, tt.want)
		}
	}
}

func TestSet_DynamicOptions(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "llm",
		Label: "LLM",
		Fields: []settings.Field{
			{Key: "provider", Type: settings.FieldSelect, Label: "Provider", Default: "anthropic", Options: []settings.SelectOption{
				{Label: "Anthropic", Value: "anthropic"},
				{Label: "OpenAI", Value: "openai"},
			}},
			{Key: "model", Type: settings.FieldSelect, Label: "Model", DynamicOptions: &settings.DynamicOptions{
				DependsOn: "provider",
				Options: map[string][]settings.SelectOption{
					"anthropic": {{Label: "Claude", Value: "claude"}},
					"openai":    {{Label: "GPT-4o", Value: "gpt-4o"}},
				},
			}},
		},
	})

	// Provider defaults to anthropic, so "claude" should be valid
	if err := Set(svc, "model", "claude"); err != nil {
		t.Fatal(err)
	}

	// "gpt-4o" should be invalid for anthropic provider
	err := Set(svc, "model", "gpt-4o")
	if err == nil {
		t.Fatal("expected validation error for wrong dynamic option")
	}
}
