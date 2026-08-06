package settings

import (
	"path/filepath"
	"strings"
	"testing"
)

// A schema exercising every construct the package supports must pass cleanly,
// so that ValidateSchema does not become a source of false positives that
// developers learn to ignore.
func TestValidateSchema_AcceptsAWellFormedSchema(t *testing.T) {
	schema := Schema{Groups: []Group{
		{
			Key:   "general",
			Label: "General",
			Fields: []Field{
				{Key: "general.theme", Type: FieldSelect, Label: "Theme", Default: "dark", Options: []SelectOption{
					{Label: "Dark", Value: "dark"},
					{Label: "Light", Value: "light"},
				}},
				{Key: "general.notify", Type: FieldToggle, Label: "Notify", Default: true},
				{Key: "general.port", Type: FieldNumber, Label: "Port", Validation: &Validation{Min: intPtr(1), Max: intPtr(65535)}},
			},
		},
		{
			Key:   "api",
			Label: "API",
			Fields: []Field{
				{Key: "api.provider", Type: FieldSelect, Label: "Provider", Options: []SelectOption{{Label: "A", Value: "a"}}},
				{Key: "api.model", Type: FieldSelect, Label: "Model", DynamicOptions: &DynamicOptions{
					DependsOn: "api.provider",
					Options:   map[string][]SelectOption{"a": {{Label: "M", Value: "m"}}},
				}},
				{Key: "api.key", Type: FieldPassword, Label: "Key", Validation: &Validation{Pattern: `sk-[a-z]+`, MinLen: 4, MaxLen: 64}},
				{Key: "api.host", Type: FieldText, Label: "Host", Condition: &Condition{Field: "api.provider", Equals: []any{"a"}}},
				{Key: "api.resolved", Type: FieldComputed, Label: "Resolved"},
			},
			ComputeFuncs: map[string]ComputeFunc{
				"api.resolved": func(values map[string]any) any { return values["api.model"] },
			},
		},
	}}

	if err := ValidateSchema(schema); err != nil {
		t.Fatalf("expected a well-formed schema to pass, got: %v", err)
	}
	if err := ValidateSchema(Schema{}); err != nil {
		t.Errorf("expected an empty schema to pass, got: %v", err)
	}
}

func TestValidateSchema_ReportsStructuralProblems(t *testing.T) {
	tests := []struct {
		name   string
		schema Schema
		want   string
	}{
		{
			name:   "uncompilable pattern",
			schema: makeSchema(Field{Key: "k", Type: FieldText, Label: "K", Validation: &Validation{Pattern: `^sk-[a-z`}}),
			want:   "invalid validation pattern",
		},
		{
			name: "duplicate field key across groups",
			schema: Schema{Groups: []Group{
				{Key: "a", Label: "A", Fields: []Field{{Key: "shared", Type: FieldText, Label: "S"}}},
				{Key: "b", Label: "B", Fields: []Field{{Key: "shared", Type: FieldText, Label: "S"}}},
			}},
			want: `duplicate field key "shared"`,
		},
		{
			name: "duplicate group key",
			schema: Schema{Groups: []Group{
				{Key: "a", Label: "A", Fields: []Field{{Key: "x", Type: FieldText, Label: "X"}}},
				{Key: "a", Label: "A again", Fields: []Field{{Key: "y", Type: FieldText, Label: "Y"}}},
			}},
			want: `duplicate group key "a"`,
		},
		{
			name:   "empty group key",
			schema: Schema{Groups: []Group{{Label: "No Key"}}},
			want:   "empty key",
		},
		{
			name:   "empty field key",
			schema: Schema{Groups: []Group{{Key: "g", Label: "G", Fields: []Field{{Type: FieldText, Label: "X"}}}}},
			want:   "empty key",
		},
		{
			name:   "select with no options",
			schema: makeSchema(Field{Key: "k", Type: FieldSelect, Label: "K"}),
			want:   "no options and no dynamicOptions",
		},
		{
			name: "select default not among options",
			schema: makeSchema(Field{Key: "k", Type: FieldSelect, Label: "K", Default: "c",
				Options: []SelectOption{{Label: "A", Value: "a"}}}),
			want: "not one of its options",
		},
		{
			name:   "condition on an undeclared field",
			schema: makeSchema(Field{Key: "k", Type: FieldText, Label: "K", Condition: &Condition{Field: "nope", Equals: []any{1}}}),
			want:   `condition on "nope", which is not declared`,
		},
		{
			name:   "condition on itself",
			schema: makeSchema(Field{Key: "k", Type: FieldText, Label: "K", Condition: &Condition{Field: "k", Equals: []any{1}}}),
			want:   "condition on itself",
		},
		{
			name:   "condition with no equals",
			schema: makeSchema(Field{Key: "k", Type: FieldText, Label: "K", Condition: &Condition{Field: "k2"}}),
			want:   "no equals values",
		},
		{
			name: "dynamicOptions depending on an undeclared field",
			schema: makeSchema(Field{Key: "k", Type: FieldSelect, Label: "K",
				DynamicOptions: &DynamicOptions{DependsOn: "nope"}}),
			want: `dynamicOptions depending on "nope"`,
		},
		{
			name:   "unknown field type",
			schema: makeSchema(Field{Key: "k", Type: FieldType("wat"), Label: "K"}),
			want:   `unknown type "wat"`,
		},
		{
			name:   "missing field type",
			schema: makeSchema(Field{Key: "k", Label: "K"}),
			want:   "has no type",
		},
		{
			name:   "password field with a default",
			schema: makeSchema(Field{Key: "k", Type: FieldPassword, Label: "K", Default: "hunter2"}),
			want:   "secrets live in the SecretStore",
		},
		{
			name:   "computed field with no compute func",
			schema: makeSchema(Field{Key: "k", Type: FieldComputed, Label: "K"}),
			want:   "no entry in group",
		},
		{
			name: "compute func for an undeclared key",
			schema: Schema{Groups: []Group{{
				Key: "g", Label: "G",
				Fields:       []Field{{Key: "k", Type: FieldText, Label: "K"}},
				ComputeFuncs: map[string]ComputeFunc{"ghost": func(map[string]any) any { return 1 }},
			}}},
			want: `"ghost", which is not declared as a field`,
		},
		{
			name: "nil compute func",
			schema: Schema{Groups: []Group{{
				Key: "g", Label: "G",
				Fields:       []Field{{Key: "k", Type: FieldComputed, Label: "K"}},
				ComputeFuncs: map[string]ComputeFunc{"k": nil},
			}}},
			want: "nil ComputeFunc",
		},
		{
			name:   "minLen above maxLen",
			schema: makeSchema(Field{Key: "k", Type: FieldText, Label: "K", Validation: &Validation{MinLen: 10, MaxLen: 3}}),
			want:   "no value can pass",
		},
		{
			name:   "min above max",
			schema: makeSchema(Field{Key: "k", Type: FieldNumber, Label: "K", Validation: &Validation{Min: intPtr(10), Max: intPtr(3)}}),
			want:   "no value can pass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSchema(tt.schema)
			if err == nil {
				t.Fatalf("expected an error mentioning %q, got nil", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected error to mention %q, got: %v", tt.want, err)
			}
		})
	}
}

// Every problem at once, not just the first: a developer fixing a schema should
// not have to run the check n times to find n bugs.
func TestValidateSchema_ReportsEveryProblem(t *testing.T) {
	schema := Schema{Groups: []Group{{
		Key:   "g",
		Label: "G",
		Fields: []Field{
			{Key: "bad_pattern", Type: FieldText, Label: "P", Validation: &Validation{Pattern: `[`}},
			{Key: "empty_select", Type: FieldSelect, Label: "S"},
			{Key: "dangling", Type: FieldText, Label: "D", Condition: &Condition{Field: "nowhere", Equals: []any{true}}},
		},
	}}}

	err := ValidateSchema(schema)
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{"bad_pattern", "empty_select", "dangling"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected every problem to be reported; %q is missing from: %v", want, err)
		}
	}
}

// NewService must remain the single clear construction-time path, and must now
// reject the broader class of schema problems ValidateSchema catches.
func TestNewService_RejectsAStructurallyBrokenSchema(t *testing.T) {
	svc, err := NewService(
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithSecretStore(NewMemorySecretStore()),
		WithGroup(Group{
			Key:    "g",
			Label:  "G",
			Fields: []Field{{Key: "mode", Type: FieldSelect, Label: "Mode"}},
		}),
	)
	if err == nil {
		t.Fatal("expected NewService to reject a select field with no options")
	}
	if svc != nil {
		t.Error("expected a nil service alongside the error")
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Errorf("error should name the offending field key, got: %v", err)
	}
}

// The trap this closes: a ComputeFunc keyed on something the schema does not
// declare is emitted by GetValues and then rejected by SetValues, so a frontend
// that reads values and posts them back fails on a key it never touched.
func TestNewService_RejectsAComputeFuncForAnUndeclaredKey(t *testing.T) {
	_, err := NewService(
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithSecretStore(NewMemorySecretStore()),
		WithGroup(Group{
			Key:          "g",
			Label:        "G",
			Fields:       []Field{{Key: "g.a", Type: FieldText, Label: "A"}},
			ComputeFuncs: map[string]ComputeFunc{"g.derived": func(map[string]any) any { return "x" }},
		}),
	)
	if err == nil {
		t.Fatal("expected NewService to reject a ComputeFunc whose key is not a declared field")
	}
	if !strings.Contains(err.Error(), "g.derived") {
		t.Errorf("error should name the offending key, got: %v", err)
	}
}
