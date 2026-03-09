package settings

import (
	"encoding/json"
	"testing"
)

func intPtr(n int) *int { return &n }

func makeSchema(fields ...Field) Schema {
	return Schema{
		Groups: []Group{
			{Key: "test", Label: "Test", Fields: fields},
		},
	}
}

func TestValidate_RequiredFieldMissing(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "name",
		Type:       FieldText,
		Label:      "Name",
		Validation: &Validation{Required: true},
	})

	errs := Validate(schema, map[string]any{})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Field != "name" {
		t.Errorf("expected field=name, got %s", errs[0].Field)
	}
	if errs[0].Message != "Name is required" {
		t.Errorf("unexpected message: %s", errs[0].Message)
	}
}

func TestValidate_RequiredFieldEmptyString(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "name",
		Type:       FieldText,
		Label:      "Name",
		Validation: &Validation{Required: true},
	})

	errs := Validate(schema, map[string]any{"name": ""})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
}

func TestValidate_RequiredFieldPresent(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "name",
		Type:       FieldText,
		Label:      "Name",
		Validation: &Validation{Required: true},
	})

	errs := Validate(schema, map[string]any{"name": "Alice"})
	if errs != nil {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_PatternMatch(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "email",
		Type:       FieldText,
		Label:      "Email",
		Validation: &Validation{Pattern: `^[^@]+@[^@]+\.[^@]+$`},
	})

	errs := Validate(schema, map[string]any{"email": "user@example.com"})
	if errs != nil {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_PatternMismatch(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "email",
		Type:       FieldText,
		Label:      "Email",
		Validation: &Validation{Pattern: `^[^@]+@[^@]+\.[^@]+$`},
	})

	errs := Validate(schema, map[string]any{"email": "notanemail"})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Message != "Email has invalid format" {
		t.Errorf("unexpected message: %s", errs[0].Message)
	}
}

func TestValidate_MinLen(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "password",
		Type:       FieldPassword,
		Label:      "Password",
		Validation: &Validation{MinLen: 8},
	})

	errs := Validate(schema, map[string]any{"password": "short"})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Message != "Password must be at least 8 characters" {
		t.Errorf("unexpected message: %s", errs[0].Message)
	}
}

func TestValidate_MaxLen(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "code",
		Type:       FieldText,
		Label:      "Code",
		Validation: &Validation{MaxLen: 4},
	})

	errs := Validate(schema, map[string]any{"code": "toolong"})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Message != "Code must be at most 4 characters" {
		t.Errorf("unexpected message: %s", errs[0].Message)
	}
}

func TestValidate_MinLenPass(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "password",
		Type:       FieldPassword,
		Label:      "Password",
		Validation: &Validation{MinLen: 8},
	})

	errs := Validate(schema, map[string]any{"password": "longenoughpassword"})
	if errs != nil {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_MinLen_UTF8(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "name",
		Type:       FieldText,
		Label:      "Name",
		Validation: &Validation{MinLen: 3},
	})

	// "日本語" is 3 runes but 9 bytes — should pass MinLen=3
	errs := Validate(schema, map[string]any{"name": "日本語"})
	if errs != nil {
		t.Fatalf("expected no errors for 3-rune string, got %v", errs)
	}
}

func TestValidate_MaxLen_UTF8(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "name",
		Type:       FieldText,
		Label:      "Name",
		Validation: &Validation{MaxLen: 4},
	})

	// "日本語" is 3 runes — should pass MaxLen=4
	errs := Validate(schema, map[string]any{"name": "日本語"})
	if errs != nil {
		t.Fatalf("expected no errors for 3-rune string with MaxLen=4, got %v", errs)
	}

	// "日本語五六" is 5 runes — should fail MaxLen=4
	errs = Validate(schema, map[string]any{"name": "日本語五六"})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for 5-rune string with MaxLen=4, got %d", len(errs))
	}
}

func TestValidate_NumberMin(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "age",
		Type:       FieldNumber,
		Label:      "Age",
		Validation: &Validation{Min: intPtr(18)},
	})

	errs := Validate(schema, map[string]any{"age": float64(10)})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Message != "Age must be at least 18" {
		t.Errorf("unexpected message: %s", errs[0].Message)
	}
}

func TestValidate_NumberMax(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "count",
		Type:       FieldNumber,
		Label:      "Count",
		Validation: &Validation{Max: intPtr(100)},
	})

	errs := Validate(schema, map[string]any{"count": float64(200)})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Message != "Count must be at most 100" {
		t.Errorf("unexpected message: %s", errs[0].Message)
	}
}

func TestValidate_NumberMinPass(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "age",
		Type:       FieldNumber,
		Label:      "Age",
		Validation: &Validation{Min: intPtr(18)},
	})

	errs := Validate(schema, map[string]any{"age": float64(25)})
	if errs != nil {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_NumberAsInt(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "count",
		Type:       FieldNumber,
		Label:      "Count",
		Validation: &Validation{Min: intPtr(1), Max: intPtr(10)},
	})

	// int type (not float64) should also be validated
	errs := Validate(schema, map[string]any{"count": 5})
	if errs != nil {
		t.Fatalf("expected no errors for int value, got %v", errs)
	}

	errs = Validate(schema, map[string]any{"count": 0})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for int below min, got %d", len(errs))
	}
}

func TestValidate_NumberAsJSONNumber(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "count",
		Type:       FieldNumber,
		Label:      "Count",
		Validation: &Validation{Min: intPtr(1), Max: intPtr(10)},
	})

	errs := Validate(schema, map[string]any{"count": json.Number("5")})
	if errs != nil {
		t.Fatalf("expected no errors for json.Number, got %v", errs)
	}

	errs = Validate(schema, map[string]any{"count": json.Number("15")})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for json.Number above max, got %d", len(errs))
	}
}

func TestValidate_ToggleValidation(t *testing.T) {
	schema := makeSchema(Field{
		Key:   "enabled",
		Type:  FieldToggle,
		Label: "Enabled",
	})

	// Valid: bool value
	errs := Validate(schema, map[string]any{"enabled": true})
	if errs != nil {
		t.Fatalf("expected no errors for bool toggle, got %v", errs)
	}

	// Valid: nil (not set)
	errs = Validate(schema, map[string]any{})
	if errs != nil {
		t.Fatalf("expected no errors for unset toggle, got %v", errs)
	}

	// Invalid: string value
	errs = Validate(schema, map[string]any{"enabled": "yes"})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for string toggle, got %d", len(errs))
	}
	if errs[0].Message != "Enabled must be true or false" {
		t.Errorf("unexpected message: %s", errs[0].Message)
	}
}

func TestValidate_ConditionalSkip(t *testing.T) {
	schema := makeSchema(
		Field{
			Key:   "provider",
			Type:  FieldSelect,
			Label: "Provider",
		},
		Field{
			Key:        "api_key",
			Type:       FieldPassword,
			Label:      "API Key",
			Validation: &Validation{Required: true},
			Condition:  &Condition{Field: "provider", Equals: []string{"openai", "anthropic"}},
		},
	)

	// provider is "local" -> condition not met -> api_key not validated
	errs := Validate(schema, map[string]any{"provider": "local"})
	if errs != nil {
		t.Fatalf("expected no errors when condition not met, got %v", errs)
	}

	// provider is "openai" -> condition met -> api_key required
	errs = Validate(schema, map[string]any{"provider": "openai"})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error when condition met, got %d", len(errs))
	}
	if errs[0].Field != "api_key" {
		t.Errorf("expected field=api_key, got %s", errs[0].Field)
	}
}

func TestValidate_SelectOptionMembership(t *testing.T) {
	schema := makeSchema(Field{
		Key:   "provider",
		Type:  FieldSelect,
		Label: "Provider",
		Options: []SelectOption{
			{Label: "Anthropic", Value: "anthropic"},
			{Label: "OpenAI", Value: "openai"},
		},
	})

	errs := Validate(schema, map[string]any{"provider": "invalid"})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Field != "provider" {
		t.Fatalf("expected provider error, got %q", errs[0].Field)
	}
}

func TestValidate_DynamicSelectOptionMembership(t *testing.T) {
	schema := makeSchema(
		Field{
			Key:   "provider",
			Type:  FieldSelect,
			Label: "Provider",
			Options: []SelectOption{
				{Label: "Anthropic", Value: "anthropic"},
				{Label: "OpenAI", Value: "openai"},
			},
		},
		Field{
			Key:   "model",
			Type:  FieldSelect,
			Label: "Model",
			DynamicOptions: &DynamicOptions{
				DependsOn: "provider",
				Options: map[string][]SelectOption{
					"anthropic": {
						{Label: "Claude", Value: "claude"},
					},
					"openai": {
						{Label: "GPT-4o", Value: "gpt-4o"},
					},
				},
			},
		},
	)

	errs := Validate(schema, map[string]any{
		"provider": "openai",
		"model":    "claude",
	})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Field != "model" {
		t.Fatalf("expected model error, got %q", errs[0].Field)
	}

	errs = Validate(schema, map[string]any{
		"provider": "openai",
		"model":    "gpt-4o",
	})
	if errs != nil {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_NoValidationRules(t *testing.T) {
	schema := makeSchema(Field{
		Key:   "notes",
		Type:  FieldText,
		Label: "Notes",
	})

	errs := Validate(schema, map[string]any{})
	if errs != nil {
		t.Fatalf("expected no errors for field without validation, got %v", errs)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	schema := makeSchema(
		Field{
			Key:        "name",
			Type:       FieldText,
			Label:      "Name",
			Validation: &Validation{Required: true},
		},
		Field{
			Key:        "email",
			Type:       FieldText,
			Label:      "Email",
			Validation: &Validation{Required: true, Pattern: `^[^@]+@[^@]+\.[^@]+$`},
		},
	)

	errs := Validate(schema, map[string]any{})
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}

	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	if !fields["name"] || !fields["email"] {
		t.Errorf("expected errors for name and email, got %v", errs)
	}
}
