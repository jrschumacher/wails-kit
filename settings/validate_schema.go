package settings

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
)

// ValidateSchema checks a Schema for structural problems and returns every one
// it finds, joined into a single error.
//
// This is the developer-facing half of validation, and the counterpart to
// Validate:
//
//   - ValidateSchema checks the schema the developer wrote. Everything it
//     reports is a bug in the application's own code — a regex that does not
//     compile, a Condition pointing at a field that does not exist, a select
//     with nothing to select. None of it is anything an end user can cause or
//     fix, so it is an error, not a []ValidationError, and the right response is
//     to fail startup.
//
//   - Validate checks the values the user entered. Everything it reports is
//     something the user typed and can correct, so it is a []ValidationError
//     addressed to them by field.
//
// NewService calls this, so an application that builds its service through
// NewService and handles the error already has this covered; call it directly
// only from a test that asserts a schema is well-formed before shipping it.
//
// A nil return means the schema is structurally sound. It says nothing about
// whether the values in the store satisfy it.
func ValidateSchema(schema Schema) error {
	var problems []error

	fieldOwner := make(map[string]string) // field key -> group key
	groupKeys := make(map[string]bool)

	// First pass: collect declared keys, so the second pass can resolve every
	// cross-reference regardless of the order groups and fields appear in.
	for _, group := range schema.Groups {
		if group.Key == "" {
			problems = append(problems, fmt.Errorf("settings: group %q has an empty key", group.Label))
		} else if groupKeys[group.Key] {
			problems = append(problems, fmt.Errorf("settings: duplicate group key %q", group.Key))
		}
		groupKeys[group.Key] = true

		for _, field := range group.Fields {
			if field.Key == "" {
				problems = append(problems, fmt.Errorf("settings: group %q has a field with an empty key", group.Key))
				continue
			}
			// A duplicate key is not a cosmetic problem: the store is a flat
			// map, so both fields read and write the same value while the UI
			// shows two independent controls.
			if owner, dup := fieldOwner[field.Key]; dup {
				problems = append(problems, fmt.Errorf(
					"settings: duplicate field key %q, declared in group %q and group %q",
					field.Key, owner, group.Key))
				continue
			}
			fieldOwner[field.Key] = group.Key
		}
	}

	for _, group := range schema.Groups {
		for _, field := range group.Fields {
			problems = append(problems, fieldProblems(group, field, fieldOwner)...)
		}
		problems = append(problems, computeFuncProblems(group, fieldOwner)...)
	}

	return errors.Join(problems...)
}

func fieldProblems(group Group, field Field, fieldOwner map[string]string) []error {
	if field.Key == "" {
		return nil // already reported
	}
	var problems []error

	switch field.Type {
	case FieldText, FieldPassword, FieldSelect, FieldToggle, FieldComputed, FieldNumber:
	case "":
		problems = append(problems, fmt.Errorf("settings: field %q has no type", field.Key))
	default:
		problems = append(problems, fmt.Errorf("settings: field %q has unknown type %q", field.Key, field.Type))
	}

	// A select with neither static nor dynamic options renders as an empty
	// dropdown the user can never satisfy.
	if field.Type == FieldSelect && len(field.Options) == 0 && field.DynamicOptions == nil {
		problems = append(problems, fmt.Errorf("settings: select field %q has no options and no dynamicOptions", field.Key))
	}

	// A password field's Default is silently discarded — the value lives in the
	// SecretStore, and a default there would be indistinguishable from a secret
	// the user actually saved. Saying so beats letting a developer wonder why
	// their default never appears.
	if field.Type == FieldPassword && field.Default != nil {
		problems = append(problems, fmt.Errorf(
			"settings: password field %q has a Default; secrets live in the SecretStore and a default is never applied — use Service.SetSecret instead",
			field.Key))
	}

	// A static select whose default is not one of its own options starts the UI
	// on a value the dropdown cannot display. Skipped when DynamicOptions is
	// set, because there the valid set depends on another field's value.
	if field.Type == FieldSelect && field.DynamicOptions == nil && len(field.Options) > 0 {
		if def, ok := field.Default.(string); ok && def != "" {
			if !slices.ContainsFunc(field.Options, func(o SelectOption) bool { return o.Value == def }) {
				problems = append(problems, fmt.Errorf(
					"settings: select field %q has default %q, which is not one of its options", field.Key, def))
			}
		}
	}

	if d := field.DynamicOptions; d != nil {
		switch {
		case d.DependsOn == "":
			problems = append(problems, fmt.Errorf("settings: field %q has dynamicOptions with an empty dependsOn", field.Key))
		case fieldOwner[d.DependsOn] == "":
			problems = append(problems, fmt.Errorf(
				"settings: field %q has dynamicOptions depending on %q, which is not declared in the schema", field.Key, d.DependsOn))
		case d.DependsOn == field.Key:
			problems = append(problems, fmt.Errorf("settings: field %q has dynamicOptions depending on itself", field.Key))
		}
	}

	if c := field.Condition; c != nil {
		switch {
		case c.Field == "":
			problems = append(problems, fmt.Errorf("settings: field %q has a condition with an empty field", field.Key))
		case c.Field == field.Key:
			problems = append(problems, fmt.Errorf("settings: field %q has a condition on itself", field.Key))
		case fieldOwner[c.Field] == "":
			// A condition on an undeclared key can never be met, so the field is
			// permanently invisible — almost always a typo in the key.
			problems = append(problems, fmt.Errorf(
				"settings: field %q has a condition on %q, which is not declared in the schema", field.Key, c.Field))
		}
		if len(c.Equals) == 0 {
			problems = append(problems, fmt.Errorf(
				"settings: field %q has a condition with no equals values, so the field is always hidden", field.Key))
		}
	}

	problems = append(problems, validationProblems(field)...)
	return problems
}

func validationProblems(field Field) []error {
	v := field.Validation
	if v == nil {
		return nil
	}
	var problems []error

	if v.Pattern != "" {
		if _, err := regexp.Compile(anchorPattern(v.Pattern)); err != nil {
			problems = append(problems, fmt.Errorf(
				"settings: field %q has an invalid validation pattern %q: %w", field.Key, v.Pattern, err))
		}
	}
	// Ranges that no value can satisfy: the field is unsavable by construction.
	if v.MinLen > 0 && v.MaxLen > 0 && v.MinLen > v.MaxLen {
		problems = append(problems, fmt.Errorf(
			"settings: field %q has minLen %d greater than maxLen %d, so no value can pass", field.Key, v.MinLen, v.MaxLen))
	}
	if v.Min != nil && v.Max != nil && *v.Min > *v.Max {
		problems = append(problems, fmt.Errorf(
			"settings: field %q has min %d greater than max %d, so no value can pass", field.Key, *v.Min, *v.Max))
	}
	return problems
}

// computeFuncProblems checks the two ways a ComputeFuncs map and the field list
// can disagree. Both produce values the frontend cannot round-trip.
func computeFuncProblems(group Group, fieldOwner map[string]string) []error {
	var problems []error

	declared := make(map[string]bool, len(group.Fields))
	for _, field := range group.Fields {
		declared[field.Key] = true
		if field.Type != FieldComputed {
			continue
		}
		if _, ok := group.ComputeFuncs[field.Key]; !ok {
			problems = append(problems, fmt.Errorf(
				"settings: computed field %q has no entry in group %q's ComputeFuncs, so it is never populated",
				field.Key, group.Key))
		}
	}

	keys := make([]string, 0, len(group.ComputeFuncs))
	for key, fn := range group.ComputeFuncs {
		if fn == nil {
			problems = append(problems, fmt.Errorf("settings: group %q has a nil ComputeFunc for %q", group.Key, key))
		}
		keys = append(keys, key)
	}
	// Deterministic order: ranging a map would shuffle the joined error text.
	slices.Sort(keys)

	for _, key := range keys {
		if declared[key] {
			continue
		}
		// GetValues would emit this key, but SetValues rejects keys the schema
		// does not declare — so a frontend that reads the values and posts them
		// back gets its save rejected on a key it never touched.
		if owner := fieldOwner[key]; owner != "" {
			problems = append(problems, fmt.Errorf(
				"settings: group %q has a ComputeFunc for %q, which is declared in group %q; a compute function must live in the group that declares its field",
				group.Key, key, owner))
			continue
		}
		problems = append(problems, fmt.Errorf(
			"settings: group %q has a ComputeFunc for %q, which is not declared as a field in any group",
			group.Key, key))
	}

	return problems
}
