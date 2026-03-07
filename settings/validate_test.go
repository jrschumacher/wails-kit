package settings

import (
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
