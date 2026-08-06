package settings

import (
	"encoding/json"
	"path/filepath"
	"strings"
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
			Condition:  &Condition{Field: "provider", Equals: []any{"openai", "anthropic"}},
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

// A field must be able to depend on a toggle or a number, not only a select.
// "Show proxy host when proxy.enabled is true" is the motivating case.
func TestConditionMet_ValueKinds(t *testing.T) {
	cases := []struct {
		name  string
		cond  Condition
		value any
		want  bool
	}{
		// bool: the toggle case.
		{"bool true matches true", Condition{Field: "f", Equals: []any{true}}, true, true},
		{"bool true does not match false", Condition{Field: "f", Equals: []any{true}}, false, false},
		{"bool false matches false", Condition{Field: "f", Equals: []any{false}}, false, true},

		// numbers: the schema is authored in Go (int), the value arrives from
		// JSON (float64). Both sides normalize, so they must compare equal.
		{"schema int matches json float64", Condition{Field: "f", Equals: []any{2}}, float64(2), true},
		{"schema float64 matches go int", Condition{Field: "f", Equals: []any{float64(2)}}, 2, true},
		{"int mismatch", Condition{Field: "f", Equals: []any{2}}, float64(3), false},
		{"fractional values compare exactly", Condition{Field: "f", Equals: []any{1.5}}, 1.5, true},
		{"int64 value normalizes", Condition{Field: "f", Equals: []any{7}}, int64(7), true},

		// strings: the select case, unchanged behaviour.
		{"string matches", Condition{Field: "f", Equals: []any{"openai"}}, "openai", true},
		{"string mismatch", Condition{Field: "f", Equals: []any{"openai"}}, "anthropic", false},

		// mixed lists work; any one member matching is enough.
		{"mixed list matches bool member", Condition{Field: "f", Equals: []any{"on", true}}, true, true},
		{"mixed list matches string member", Condition{Field: "f", Equals: []any{"on", true}}, "on", true},

		// Cross-kind comparison never matches. This is deliberate: coercing
		// "true" to true or "1" to 1 makes a schema's behaviour depend on
		// whichever side of the bridge a value came from.
		{"string does not match bool", Condition{Field: "f", Equals: []any{true}}, "true", false},
		{"bool does not match string", Condition{Field: "f", Equals: []any{"true"}}, true, false},
		{"string does not match number", Condition{Field: "f", Equals: []any{1}}, "1", false},
		{"number does not match string", Condition{Field: "f", Equals: []any{"1"}}, 1, false},
		{"bool does not match number", Condition{Field: "f", Equals: []any{1}}, true, false},

		// nil is the value of a field that has never been set, and is
		// addressable so a schema can say "show this while unset".
		{"nil matches explicit nil", Condition{Field: "f", Equals: []any{nil}}, nil, true},
		{"nil does not match a string", Condition{Field: "f", Equals: []any{""}}, nil, false},
		{"empty equals never matches", Condition{Field: "f", Equals: nil}, "anything", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]any{}
			// A nil value is stored explicitly so the "missing key" and
			// "key present but null" paths are both exercised.
			values[tc.cond.Field] = tc.value

			if got := conditionMet(&tc.cond, values); got != tc.want {
				t.Errorf("conditionMet(%v, %#v) = %v, want %v", tc.cond.Equals, tc.value, got, tc.want)
			}
		})
	}
}

func TestConditionMet_MissingKeyBehavesAsNil(t *testing.T) {
	c := Condition{Field: "absent", Equals: []any{nil}}
	if !conditionMet(&c, map[string]any{}) {
		t.Error("a key that is absent entirely should compare equal to nil")
	}

	c = Condition{Field: "absent", Equals: []any{true}}
	if conditionMet(&c, map[string]any{}) {
		t.Error("an absent key must not satisfy a bool condition")
	}
}

// End-to-end through Validate: a required field gated on a toggle.
func TestValidate_ConditionOnBoolField(t *testing.T) {
	schema := makeSchema(
		Field{Key: "proxy.enabled", Type: FieldToggle, Label: "Use Proxy"},
		Field{
			Key:        "proxy.host",
			Type:       FieldText,
			Label:      "Proxy Host",
			Validation: &Validation{Required: true},
			Condition:  &Condition{Field: "proxy.enabled", Equals: []any{true}},
		},
	)

	if errs := Validate(schema, map[string]any{"proxy.enabled": false}); errs != nil {
		t.Fatalf("proxy.host must not be validated while the toggle is off, got %v", errs)
	}

	errs := Validate(schema, map[string]any{"proxy.enabled": true})
	if len(errs) != 1 || errs[0].Field != "proxy.host" {
		t.Fatalf("expected proxy.host to be required while the toggle is on, got %v", errs)
	}
}

func TestValidate_ConditionOnNumberField(t *testing.T) {
	schema := makeSchema(
		Field{Key: "retries", Type: FieldNumber, Label: "Retries"},
		Field{
			Key:        "backoff",
			Type:       FieldText,
			Label:      "Backoff",
			Validation: &Validation{Required: true},
			Condition:  &Condition{Field: "retries", Equals: []any{3}},
		},
	)

	if errs := Validate(schema, map[string]any{"retries": float64(1)}); errs != nil {
		t.Fatalf("expected backoff to be skipped, got %v", errs)
	}
	// float64(3) is what the JSON bridge delivers for a schema written as 3.
	if errs := Validate(schema, map[string]any{"retries": float64(3)}); len(errs) != 1 {
		t.Fatalf("expected backoff to be required when retries=3, got %v", errs)
	}
}

// The schema is serialized to the frontend, which renders conditions itself.
// The JSON shape must stay a plain array of literals.
func TestCondition_JSONShape(t *testing.T) {
	cases := []struct {
		cond Condition
		want string
	}{
		{Condition{Field: "provider", Equals: []any{"openai"}}, `{"field":"provider","equals":["openai"]}`},
		{Condition{Field: "proxy.enabled", Equals: []any{true}}, `{"field":"proxy.enabled","equals":[true]}`},
		{Condition{Field: "retries", Equals: []any{3}}, `{"field":"retries","equals":[3]}`},
		{Condition{Field: "mixed", Equals: []any{"a", 1, false}}, `{"field":"mixed","equals":["a",1,false]}`},
	}

	for _, tc := range cases {
		got, err := json.Marshal(tc.cond)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(got) != tc.want {
			t.Errorf("got %s, want %s", got, tc.want)
		}
	}
}

// A condition authored in TypeScript and sent back as JSON must behave the
// same as one authored in Go.
func TestCondition_SurvivesJSONRoundTrip(t *testing.T) {
	original := Condition{Field: "proxy.enabled", Equals: []any{true}}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Condition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !conditionMet(&decoded, map[string]any{"proxy.enabled": true}) {
		t.Error("a JSON round-tripped bool condition must still match")
	}

	// Numbers decode as float64; a Go-side int value must still match.
	numeric := Condition{Field: "retries", Equals: []any{3}}
	data, _ = json.Marshal(numeric)
	var decodedNum Condition
	if err := json.Unmarshal(data, &decodedNum); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !conditionMet(&decodedNum, map[string]any{"retries": 3}) {
		t.Error("a JSON round-tripped numeric condition must match a Go int value")
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

// --- Fix #4: patterns must be anchored, and bad patterns must reach the developer ---

func TestValidate_PatternIsAnchoredNotSubstring(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "pin",
		Type:       FieldText,
		Label:      "PIN",
		Validation: &Validation{Pattern: `[0-9]{4}`},
	})

	errs := Validate(schema, map[string]any{"pin": "abcd1234efgh"})
	if len(errs) != 1 {
		t.Fatalf("pattern [0-9]{4} must reject %q; a substring match is not a format check (got %v)", "abcd1234efgh", errs)
	}

	if errs := Validate(schema, map[string]any{"pin": "1234"}); errs != nil {
		t.Fatalf("expected exact match to pass, got %v", errs)
	}
}

func TestValidate_AlternationIsAnchoredAsAWhole(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "env",
		Type:       FieldText,
		Label:      "Env",
		Validation: &Validation{Pattern: `dev|prod`},
	})

	if errs := Validate(schema, map[string]any{"env": "prod"}); errs != nil {
		t.Fatalf("expected prod to pass, got %v", errs)
	}
	// Anchors must wrap the whole alternation, not bind to a single branch.
	if errs := Validate(schema, map[string]any{"env": "development"}); len(errs) != 1 {
		t.Fatalf("expected %q to be rejected, got %v", "development", errs)
	}
}

func TestValidate_AlreadyAnchoredPatternStillWorks(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "email",
		Type:       FieldText,
		Label:      "Email",
		Validation: &Validation{Pattern: `^[^@]+@[^@]+\.[^@]+$`},
	})

	if errs := Validate(schema, map[string]any{"email": "user@example.com"}); errs != nil {
		t.Fatalf("expected already-anchored pattern to keep working, got %v", errs)
	}
}

// NewService now has an error return, so an uncompilable schema pattern is
// reported through it rather than by panicking.
func TestNewService_MalformedPatternIsADeveloperError(t *testing.T) {
	svc, err := NewService(
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithGroup(Group{
			Key:   "g",
			Label: "G",
			Fields: []Field{
				{Key: "api_key", Type: FieldText, Label: "API Key", Validation: &Validation{Pattern: `^sk-[a-z`}},
			},
		}),
	)
	if err == nil {
		t.Fatal("expected NewService to fail on a schema pattern that does not compile")
	}
	if svc != nil {
		t.Error("expected a nil service alongside the error")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("error should name the offending field key, got: %v", err)
	}
}

// Validate reports what the user got wrong. An uncompilable pattern is not
// that: it is neither the user's fault nor something they can fix, and a
// function whose contract is to return []ValidationError must not panic. The
// pattern check is skipped and ValidateSchema reports it instead.
func TestValidate_MalformedPatternIsSkippedNotPanicked(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "api_key",
		Type:       FieldText,
		Label:      "API Key",
		Validation: &Validation{Pattern: `^sk-[a-z`, MinLen: 4},
	})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Validate must not panic on an uncompilable pattern, got: %v", r)
		}
	}()

	if errs := Validate(schema, map[string]any{"api_key": "sk-abc"}); errs != nil {
		t.Errorf(`expected no user-facing error from a broken pattern, got %v`, errs)
	}

	// The field's other rules must still apply — only the pattern is dropped.
	errs := Validate(schema, map[string]any{"api_key": "ab"})
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "at least 4 characters") {
		t.Errorf("expected the remaining rules to still run, got %v", errs)
	}

	// And the developer is told, through the function meant for it.
	if err := ValidateSchema(schema); err == nil {
		t.Error("expected ValidateSchema to report the uncompilable pattern")
	}
}

// --- Fix #5: min/max must apply to Go-native numeric types, not just float64 ---

func TestValidate_NumberMaxAcrossNumericTypes(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "count",
		Type:       FieldNumber,
		Label:      "Count",
		Validation: &Validation{Max: intPtr(100)},
	})

	cases := []struct {
		name string
		val  any
	}{
		{"float64", float64(200)}, // what arrives over the JSON bridge
		{"int", int(200)},         // what a Go-side caller naturally passes
		{"int8", int8(101)},
		{"int16", int16(200)},
		{"int32", int32(200)},
		{"int64", int64(200)},
		{"uint", uint(200)},
		{"uint8", uint8(200)},
		{"uint16", uint16(200)},
		{"uint32", uint32(200)},
		{"uint64", uint64(200)},
		{"float32", float32(200)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := Validate(schema, map[string]any{"count": tc.val})
			if len(errs) != 1 {
				t.Fatalf("expected max violation for %T(%v) to be caught, got %v", tc.val, tc.val, errs)
			}
			if errs[0].Message != "Count must be at most 100" {
				t.Errorf("unexpected message: %s", errs[0].Message)
			}
		})
	}
}

func TestValidate_NumberMinAcrossNumericTypes(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "age",
		Type:       FieldNumber,
		Label:      "Age",
		Validation: &Validation{Min: intPtr(18)},
	})

	for _, val := range []any{float64(10), int(10), int64(10), uint8(10), float32(10)} {
		errs := Validate(schema, map[string]any{"age": val})
		if len(errs) != 1 {
			t.Errorf("expected min violation for %T(%v) to be caught, got %v", val, val, errs)
		}
	}

	for _, val := range []any{float64(25), int(25), int64(25), uint8(25), float32(25)} {
		if errs := Validate(schema, map[string]any{"age": val}); errs != nil {
			t.Errorf("expected %T(%v) to pass min, got %v", val, val, errs)
		}
	}
}

func TestValidate_NonNumericValuesAreNotRangeChecked(t *testing.T) {
	schema := makeSchema(Field{
		Key:        "count",
		Type:       FieldNumber,
		Label:      "Count",
		Validation: &Validation{Min: intPtr(1)},
	})

	// bool and string must not be coerced into the numeric comparison.
	for _, val := range []any{true, false, "0", nil} {
		if errs := Validate(schema, map[string]any{"count": val}); errs != nil {
			t.Errorf("expected non-numeric %T(%v) to skip range checks, got %v", val, val, errs)
		}
	}
}

func TestValidate_IntDefaultFromSchemaIsRangeChecked(t *testing.T) {
	// A schema author writing Default: 5 gets an int, not a float64.
	schema := makeSchema(Field{
		Key:        "retries",
		Type:       FieldNumber,
		Label:      "Retries",
		Default:    5,
		Validation: &Validation{Max: intPtr(3)},
	})

	if errs := Validate(schema, map[string]any{"retries": 5}); len(errs) != 1 {
		t.Fatalf("expected int default 5 to violate max 3, got %v", errs)
	}
}
