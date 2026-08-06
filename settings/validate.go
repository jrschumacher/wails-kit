package settings

import (
	"encoding/json"
	"fmt"
	"regexp"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Validate checks the user's values against the schema's validation rules and
// returns one ValidationError per problem, addressed to the end user by field.
//
// It is the user-facing half of validation; ValidateSchema is the
// developer-facing half. Anything wrong with the schema itself belongs there:
// a Validation.Pattern that does not compile is skipped here rather than
// reported, because "API Key has invalid format" is a lie to a user who typed
// a perfectly good key, and a panic from a function whose whole contract is to
// return []ValidationError is worse still. Call ValidateSchema — or build the
// service with NewService, which calls it — to catch such a pattern as the
// developer error it is.
//
// A nil return means every value in the schema's scope is acceptable. Keys not
// declared in the schema are ignored here; Service.SetValues rejects those.
func Validate(schema Schema, values map[string]any) []ValidationError {
	return validate(schema, values, compilablePatterns(schema))
}

// validate is Validate with patterns already compiled, so that SetValues does
// not recompile every schema pattern on each call.
//
// values must be the complete effective value set, never a partial submission.
// Both Required and Condition are judged against it: a field whose controlling
// field is absent looks hidden and escapes validation, and a field that is
// merely absent looks empty. Service.SetValues therefore merges a partial
// submission onto the persisted state before calling this — see
// Service.effectiveValues.
func validate(schema Schema, values map[string]any, patterns map[string]*regexp.Regexp) []ValidationError {
	var errs []ValidationError

	for _, group := range schema.Groups {
		for _, field := range group.Fields {
			if field.Validation == nil {
				continue
			}

			// Skip validation for hidden fields
			if field.Condition != nil && !conditionMet(field.Condition, values) {
				continue
			}

			val := values[field.Key]
			fieldErrs := validateField(field, val, patterns[field.Key])
			errs = append(errs, fieldErrs...)
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// anchorPattern makes a validation pattern match the whole value rather than
// any substring of it: without this, Pattern "[0-9]{4}" accepts
// "abcd1234efgh". The non-capturing group keeps the anchors bound to the
// entire expression, so an alternation like "dev|prod" cannot anchor just one
// of its branches.
func anchorPattern(pattern string) string {
	return "^(?:" + pattern + ")$"
}

// compilePatterns compiles every validation pattern in the schema, anchored,
// keyed by field key. Compiling up front turns a malformed pattern into one
// developer-facing error at construction instead of a per-request "invalid
// format" message shown to the end user.
func compilePatterns(schema Schema) (map[string]*regexp.Regexp, error) {
	patterns := make(map[string]*regexp.Regexp)
	for _, group := range schema.Groups {
		for _, field := range group.Fields {
			if field.Validation == nil || field.Validation.Pattern == "" {
				continue
			}
			re, err := regexp.Compile(anchorPattern(field.Validation.Pattern))
			if err != nil {
				return nil, fmt.Errorf("settings: field %q has an invalid validation pattern %q: %w", field.Key, field.Validation.Pattern, err)
			}
			patterns[field.Key] = re
		}
	}
	return patterns, nil
}

// compilablePatterns compiles what it can and silently drops what it cannot.
// A field whose pattern does not compile ends up with no pattern check at all,
// which is the right behaviour for Validate: the remaining rules still apply,
// and the broken pattern is ValidateSchema's problem to report.
func compilablePatterns(schema Schema) map[string]*regexp.Regexp {
	patterns := make(map[string]*regexp.Regexp)
	for _, group := range schema.Groups {
		for _, field := range group.Fields {
			if field.Validation == nil || field.Validation.Pattern == "" {
				continue
			}
			if re, err := regexp.Compile(anchorPattern(field.Validation.Pattern)); err == nil {
				patterns[field.Key] = re
			}
		}
	}
	return patterns
}

// conditionMet reports whether the controlling field's current value matches
// any member of c.Equals. See the Condition doc comment for the semantics this
// implements; the short version is that numbers are compared numerically across
// Go types and everything else is compared without crossing kinds.
func conditionMet(c *Condition, values map[string]any) bool {
	val := values[c.Field]
	for _, eq := range c.Equals {
		if conditionValuesEqual(val, eq) {
			return true
		}
	}
	return false
}

// conditionValuesEqual compares one condition operand against the field's
// value. It is deliberately narrow: only the numeric case coerces, because a
// schema author writing 3 and a JSON bridge delivering float64(3) mean the same
// thing, whereas "true" and true do not.
func conditionValuesEqual(val, eq any) bool {
	// Numeric on both sides: compare as float64 so int/int64/float64 agree.
	// Checked first because a bool is not a number here (toFloat64 rejects it),
	// which keeps true from ever comparing equal to 1.
	if valNum, valIsNum := toFloat64(val); valIsNum {
		if eqNum, eqIsNum := toFloat64(eq); eqIsNum {
			return valNum == eqNum
		}
		return false
	}

	switch v := val.(type) {
	case nil:
		return eq == nil
	case bool:
		e, ok := eq.(bool)
		return ok && v == e
	case string:
		e, ok := eq.(string)
		return ok && v == e
	default:
		// Slices, maps and other composite values are not sensible condition
		// operands; treating them as never-matching keeps the field hidden
		// rather than guessing.
		return false
	}
}

// toFloat64 normalizes any Go numeric type to float64 for range comparison.
// Values crossing the JSON bridge arrive as float64, but a Go-side caller —
// a test, a programmatic SetValues, or a schema written as Default: 5 — passes
// a native int. Asserting float64 alone silently skips min/max for those.
// bool, string and nil are deliberately not numbers here.
func toFloat64(val any) (float64, bool) {
	switch n := val.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func validateField(field Field, val any, pattern *regexp.Regexp) []ValidationError {
	var errs []ValidationError
	v := field.Validation

	str, isStr := val.(string)
	num, isNum := toFloat64(val)

	if v.Required {
		if val == nil || (isStr && str == "") {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s is required", field.Label)})
			return errs
		}
	}

	if isStr && str != "" {
		if pattern != nil && !pattern.MatchString(str) {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s has invalid format", field.Label)})
		}
		if v.MinLen > 0 && len(str) < v.MinLen {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s must be at least %d characters", field.Label, v.MinLen)})
		}
		if v.MaxLen > 0 && len(str) > v.MaxLen {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s must be at most %d characters", field.Label, v.MaxLen)})
		}
	}

	if isNum {
		if v.Min != nil && num < float64(*v.Min) {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s must be at least %d", field.Label, *v.Min)})
		}
		if v.Max != nil && num > float64(*v.Max) {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s must be at most %d", field.Label, *v.Max)})
		}
	}

	return errs
}
