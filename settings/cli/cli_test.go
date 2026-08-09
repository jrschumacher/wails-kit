package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/settings"
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
		Label: i18n.Text{Other: "General"},
		Fields: []settings.Field{
			{Key: "name", Type: settings.FieldText, Label: i18n.Text{Other: "Name"}, Default: "World"},
			{Key: "theme", Type: settings.FieldSelect, Label: i18n.Text{Other: "Theme"}, Default: "dark", Options: []settings.SelectOption{
				{Label: i18n.Text{Other: "Dark"}, Value: "dark"},
				{Label: i18n.Text{Other: "Light"}, Value: "light"},
			}},
			{Key: "notifications", Type: settings.FieldToggle, Label: i18n.Text{Other: "Notifications"}, Default: true},
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
		Label: i18n.Text{Other: "Auth"},
		Fields: []settings.Field{
			{Key: "api_key", Type: settings.FieldPassword, Label: i18n.Text{Other: "API Key"}},
		},
	})

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
		Label: i18n.Text{Other: "LLM"},
		Fields: []settings.Field{
			{Key: "provider", Type: settings.FieldSelect, Label: i18n.Text{Other: "Provider"}, Default: "openai", Options: []settings.SelectOption{
				{Label: i18n.Text{Other: "OpenAI"}, Value: "openai"},
				{Label: i18n.Text{Other: "Anthropic"}, Value: "anthropic"},
			}},
			{Key: "anthropic_key", Type: settings.FieldText, Label: i18n.Text{Other: "Anthropic Key"}, Condition: &settings.Condition{
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

func TestGet(t *testing.T) {
	svc := testService(t, basicGroup())

	val, err := Get(svc, "name")
	if err != nil {
		t.Fatal(err)
	}
	if val != "World" {
		t.Errorf("expected World, got %s", val)
	}
}

func TestGet_SelectWithLabel(t *testing.T) {
	svc := testService(t, basicGroup())

	val, err := Get(svc, "theme")
	if err != nil {
		t.Fatal(err)
	}
	if val != "dark (Dark)" {
		t.Errorf("expected 'dark (Dark)', got %s", val)
	}
}

func TestGet_Toggle(t *testing.T) {
	svc := testService(t, basicGroup())

	val, err := Get(svc, "notifications")
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("expected true, got %s", val)
	}
}

func TestGet_PasswordMasked(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "auth",
		Label: i18n.Text{Other: "Auth"},
		Fields: []settings.Field{
			{Key: "api_key", Type: settings.FieldPassword, Label: i18n.Text{Other: "API Key"}},
		},
	})

	_, err := svc.SetValues(map[string]any{"api_key": "secret123"})
	if err != nil {
		t.Fatal(err)
	}

	val, err := Get(svc, "api_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != settings.SecretMask {
		t.Errorf("expected masked value, got %s", val)
	}
}

func TestGet_UnknownKey(t *testing.T) {
	svc := testService(t, basicGroup())
	_, err := Get(svc, "nonexistent")
	if err == nil || !strings.Contains(err.Error(), "unknown setting") {
		t.Errorf("expected unknown setting error, got: %v", err)
	}
}

func TestGet_UnsetValue(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "test",
		Label: i18n.Text{Other: "Test"},
		Fields: []settings.Field{
			{Key: "optional", Type: settings.FieldText, Label: i18n.Text{Other: "Optional"}},
		},
	})

	val, err := Get(svc, "optional")
	if err != nil {
		t.Fatal(err)
	}
	if val != "(not set)" {
		t.Errorf("expected '(not set)', got %s", val)
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
		Label: i18n.Text{Other: "Preferences"},
		Fields: []settings.Field{
			{Key: "font_size", Type: settings.FieldNumber, Label: i18n.Text{Other: "Font Size"}, Default: 14,
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
		Label: i18n.Text{Other: "Info"},
		Fields: []settings.Field{
			{Key: "computed_field", Type: settings.FieldComputed, Label: i18n.Text{Other: "Computed"}},
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
		Label: i18n.Text{Other: "Preferences"},
		Fields: []settings.Field{
			{Key: "theme", Type: settings.FieldSelect, Label: i18n.Text{Other: "Theme"}, Options: []settings.SelectOption{
				{Label: i18n.Text{Other: "Dark"}, Value: "dark"},
				{Label: i18n.Text{Other: "Light"}, Value: "light"},
			}},
		},
	})

	err := Set(svc, "theme", "invalid")
	if err == nil {
		t.Fatal("expected validation error")
	}
	ve, ok := err.(*ValidationErrors)
	if !ok {
		t.Errorf("expected *ValidationErrors, got %T: %v", err, err)
	}
	if ve != nil && len(ve.Errors) != 1 {
		t.Errorf("expected 1 validation error, got %d", len(ve.Errors))
	}
}

func TestSet_DynamicOptions(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "llm",
		Label: i18n.Text{Other: "LLM"},
		Fields: []settings.Field{
			{Key: "provider", Type: settings.FieldSelect, Label: i18n.Text{Other: "Provider"}, Default: "anthropic", Options: []settings.SelectOption{
				{Label: i18n.Text{Other: "Anthropic"}, Value: "anthropic"},
				{Label: i18n.Text{Other: "OpenAI"}, Value: "openai"},
			}},
			{Key: "model", Type: settings.FieldSelect, Label: i18n.Text{Other: "Model"}, DynamicOptions: &settings.DynamicOptions{
				DependsOn: "provider",
				Options: map[string][]settings.SelectOption{
					"anthropic": {{Label: i18n.Text{Other: "Claude"}, Value: "claude"}},
					"openai":    {{Label: i18n.Text{Other: "GPT-4o"}, Value: "gpt-4o"}},
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
		field settings.ResolvedField
		input string
		want  any
		err   bool
	}{
		{settings.ResolvedField{Type: settings.FieldToggle}, "true", true, false},
		{settings.ResolvedField{Type: settings.FieldToggle}, "yes", true, false},
		{settings.ResolvedField{Type: settings.FieldToggle}, "false", false, false},
		{settings.ResolvedField{Type: settings.FieldToggle}, "no", false, false},
		{settings.ResolvedField{Type: settings.FieldToggle}, "invalid", nil, true},
		{settings.ResolvedField{Type: settings.FieldNumber}, "42", 42, false},
		{settings.ResolvedField{Type: settings.FieldNumber}, "3.14", 3.14, false},
		{settings.ResolvedField{Type: settings.FieldNumber}, "abc", nil, true},
		{settings.ResolvedField{Type: settings.FieldText}, "hello", "hello", false},
	}

	for _, tt := range tests {
		got, err := coerceValue(nil, tt.field, tt.input)
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

// frCLICatalog is a minimal WithCatalog source translating this package's
// own strings into French, used by every test below that needs a locale
// actually resolving through the catalog (as opposed to falling back to
// Text.Other).
func frCLICatalog() fstest.MapFS {
	return fstest.MapFS{
		"locales/fr.json": &fstest.MapFile{Data: []byte(`{
			"wailskit.settingscli.value.not_set": "(non défini)",
			"wailskit.settingscli.errors.unknown_setting": "paramètre inconnu",
			"wailskit.settingscli.errors.cannot_set_computed": "champ calculé non modifiable",
			"wailskit.settingscli.errors.invalid_toggle": "valeur de bascule invalide : %s (utilisez true/false)",
			"wailskit.settingscli.errors.validation_failed": "échec de la validation"
		}`)},
	}
}

func frLocalizer(t *testing.T) *i18n.Localizer {
	t.Helper()
	l, err := i18n.New(i18n.WithCatalog(frCLICatalog()), i18n.WithLocale("fr"))
	if err != nil {
		t.Fatalf("i18n.New: %v", err)
	}
	return l
}

func TestWithLocalizer_Show_NotSetPlaceholder(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "test",
		Label: i18n.Text{Other: "Test"},
		Fields: []settings.Field{
			{Key: "optional", Type: settings.FieldText, Label: i18n.Text{Other: "Optional"}},
		},
	})

	var buf bytes.Buffer
	if err := Show(svc, WithOutput(&buf), WithLocalizer(frLocalizer(t))); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "(non défini)") {
		t.Errorf("expected localized placeholder, got:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "(not set)") {
		t.Error("expected English placeholder to be replaced by the localized one")
	}
}

func TestWithLocalizer_Get_NotSetPlaceholder(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "test",
		Label: i18n.Text{Other: "Test"},
		Fields: []settings.Field{
			{Key: "optional", Type: settings.FieldText, Label: i18n.Text{Other: "Optional"}},
		},
	})

	val, err := Get(svc, "optional", WithLocalizer(frLocalizer(t)))
	if err != nil {
		t.Fatal(err)
	}
	if val != "(non défini)" {
		t.Errorf("expected localized placeholder, got %q", val)
	}
}

func TestWithLocalizer_Get_UnknownKeyError(t *testing.T) {
	svc := testService(t, basicGroup())
	_, err := Get(svc, "nonexistent", WithLocalizer(frLocalizer(t)))
	if err == nil || !strings.Contains(err.Error(), "paramètre inconnu") {
		t.Errorf("expected localized unknown-setting error, got: %v", err)
	}
}

func TestWithLocalizer_Set_ComputedFieldError(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "info",
		Label: i18n.Text{Other: "Info"},
		Fields: []settings.Field{
			{Key: "computed_field", Type: settings.FieldComputed, Label: i18n.Text{Other: "Computed"}},
		},
	})

	err := Set(svc, "computed_field", "value", WithLocalizer(frLocalizer(t)))
	if err == nil || !strings.Contains(err.Error(), "champ calculé non modifiable") {
		t.Errorf("expected localized computed-field error, got: %v", err)
	}
}

func TestWithLocalizer_Set_InvalidToggleError(t *testing.T) {
	svc := testService(t, basicGroup())
	err := Set(svc, "notifications", "not-a-bool", WithLocalizer(frLocalizer(t)))
	if err == nil || !strings.Contains(err.Error(), "valeur de bascule invalide") {
		t.Errorf("expected localized invalid-toggle error, got: %v", err)
	}
}

func TestWithLocalizer_ValidationErrors_Prefix(t *testing.T) {
	svc := testService(t, settings.Group{
		Key:   "prefs",
		Label: i18n.Text{Other: "Preferences"},
		Fields: []settings.Field{
			{Key: "theme", Type: settings.FieldSelect, Label: i18n.Text{Other: "Theme"}, Options: []settings.SelectOption{
				{Label: i18n.Text{Other: "Dark"}, Value: "dark"},
			}},
		},
	})

	err := Set(svc, "theme", "invalid", WithLocalizer(frLocalizer(t)))
	if err == nil || !strings.Contains(err.Error(), "échec de la validation") {
		t.Errorf("expected localized validation-failed prefix, got: %v", err)
	}
}

func TestNilLocalizer_FallsBackToEnglish(t *testing.T) {
	// No WithLocalizer at all — every message must be exactly the English
	// baked into this package's Text values, matching every pre-i18n test
	// in this file (none of which pass WithLocalizer).
	svc := testService(t, settings.Group{
		Key:   "test",
		Label: i18n.Text{Other: "Test"},
		Fields: []settings.Field{
			{Key: "optional", Type: settings.FieldText, Label: i18n.Text{Other: "Optional"}},
		},
	})

	val, err := Get(svc, "optional")
	if err != nil {
		t.Fatal(err)
	}
	if val != "(not set)" {
		t.Errorf("expected English fallback placeholder, got %q", val)
	}

	_, err = Get(svc, "nonexistent")
	if err == nil || !strings.Contains(err.Error(), "unknown setting") {
		t.Errorf("expected English fallback error, got: %v", err)
	}
}

// TestMachineValuesUnaffectedByLocalizer pins the WP-22 "machine output
// must be unchanged" requirement. settings/cli has no --json mode of its
// own to pin byte-for-byte, but formatValue's toggle/number tokens and
// coerceValue's accepted input alphabet ARE the machine-readable surface
// any consumer's own --json flag (or script piping settingscli.Get output)
// would depend on — see the doc comments on formatValue/coerceValue. This
// test proves configuring WithLocalizer with a real, actively-resolving
// non-English catalog never changes any of that, for both directions
// (format and parse).
func TestMachineValuesUnaffectedByLocalizer(t *testing.T) {
	l := frLocalizer(t)
	toggleField := settings.ResolvedField{Type: settings.FieldToggle}
	numberField := settings.ResolvedField{Type: settings.FieldNumber}

	for _, want := range []bool{true, false} {
		gotEN := formatValue(nil, toggleField, want)
		gotFR := formatValue(l, toggleField, want)
		if gotEN != gotFR {
			t.Errorf("formatValue toggle %v: localizer changed output: %q (nil) vs %q (fr)", want, gotEN, gotFR)
		}
	}

	for _, tok := range []string{"true", "false", "yes", "no", "1", "0", "on", "off"} {
		gotEN, errEN := coerceValue(nil, toggleField, tok)
		gotFR, errFR := coerceValue(l, toggleField, tok)
		if errEN != nil || errFR != nil {
			t.Fatalf("coerceValue(%q): unexpected error: nil-localizer=%v fr-localizer=%v", tok, errEN, errFR)
		}
		if gotEN != gotFR {
			t.Errorf("coerceValue(%q): localizer changed parsed value: %v (nil) vs %v (fr)", tok, gotEN, gotFR)
		}
	}

	gotEN := formatValue(nil, numberField, 42)
	gotFR := formatValue(l, numberField, 42)
	if gotEN != gotFR {
		t.Errorf("formatValue number: localizer changed output: %q (nil) vs %q (fr)", gotEN, gotFR)
	}

	numEN, errEN := coerceValue(nil, numberField, "3.14")
	numFR, errFR := coerceValue(l, numberField, "3.14")
	if errEN != nil || errFR != nil {
		t.Fatalf("coerceValue number: unexpected error: nil-localizer=%v fr-localizer=%v", errEN, errFR)
	}
	if numEN != numFR {
		t.Errorf("coerceValue number: localizer changed parsed value: %v (nil) vs %v (fr)", numEN, numFR)
	}
}
