package settings

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

// Validation error codes returned in ValidationError.Code.
const (
	CodeRequired      = "required"
	CodePattern       = "pattern"
	CodeMinLen        = "min_length"
	CodeMaxLen        = "max_length"
	CodeMin           = "min"
	CodeMax           = "max"
	CodeInvalidType   = "invalid_type"
	CodeInvalidOption = "invalid_option"
)

// ValidationError represents a field-level validation failure.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

func Validate(schema Schema, values map[string]any) []ValidationError {
	var errs []ValidationError

	for _, group := range schema.Groups {
		for _, field := range group.Fields {
			if field.Validation == nil && field.Type != FieldSelect && field.Type != FieldToggle {
				continue
			}

			// Skip validation for hidden fields
			if field.Condition != nil && !conditionMet(field.Condition, values) {
				continue
			}

			val := values[field.Key]
			fieldErrs := validateField(field, val, values)
			errs = append(errs, fieldErrs...)
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

func conditionMet(c *Condition, values map[string]any) bool {
	val, _ := values[c.Field].(string)
	for _, eq := range c.Equals {
		if val == eq {
			return true
		}
	}
	return false
}

func validateField(field Field, val any, values map[string]any) []ValidationError {
	var errs []ValidationError
	v := field.Validation

	str, isStr := val.(string)
	num := toFloat64(val)
	isNum := num != nil

	if v != nil && v.Required {
		if val == nil || (isStr && str == "") {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s is required", field.Label), Code: CodeRequired})
			return errs
		}
	}

	// Toggle type validation: must be a bool if provided
	if field.Type == FieldToggle && val != nil {
		if _, ok := val.(bool); !ok {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s must be true or false", field.Label), Code: CodeInvalidType})
		}
	}

	if isStr && str != "" && v != nil {
		if v.Pattern != "" {
			if matched, _ := regexp.MatchString(v.Pattern, str); !matched {
				errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s has invalid format", field.Label), Code: CodePattern})
			}
		}
		if v.MinLen > 0 && utf8.RuneCountInString(str) < v.MinLen {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s must be at least %d characters", field.Label, v.MinLen), Code: CodeMinLen})
		}
		if v.MaxLen > 0 && utf8.RuneCountInString(str) > v.MaxLen {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s must be at most %d characters", field.Label, v.MaxLen), Code: CodeMaxLen})
		}
	}

	if isNum && v != nil {
		n := *num
		if v.Min != nil && n < float64(*v.Min) {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s must be at least %d", field.Label, *v.Min), Code: CodeMin})
		}
		if v.Max != nil && n > float64(*v.Max) {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s must be at most %d", field.Label, *v.Max), Code: CodeMax})
		}
	}

	if field.Type == FieldSelect && isStr && str != "" && hasSelectableOptions(field, values) && !selectOptionAllowed(field, str, values) {
		errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s has an invalid option", field.Label), Code: CodeInvalidOption})
	}

	return errs
}

// toFloat64 converts a numeric value (float64, int, json.Number) to *float64.
// Returns nil if the value is not numeric.
func toFloat64(val any) *float64 {
	switch v := val.(type) {
	case float64:
		return &v
	case int:
		f := float64(v)
		return &f
	case int64:
		f := float64(v)
		return &f
	default:
		// Check for json.Number via its String/Float64 method
		type jsonNumber interface {
			Float64() (float64, error)
		}
		if jn, ok := val.(jsonNumber); ok {
			if f, err := jn.Float64(); err == nil {
				return &f
			}
		}
		return nil
	}
}

func hasSelectableOptions(field Field, values map[string]any) bool {
	if field.DynamicOptions != nil {
		dependsOn, _ := values[field.DynamicOptions.DependsOn].(string)
		return len(field.DynamicOptions.Options[dependsOn]) > 0
	}
	return len(field.Options) > 0
}

func selectOptionAllowed(field Field, value string, values map[string]any) bool {
	options := field.Options
	if field.DynamicOptions != nil {
		dependsOn, _ := values[field.DynamicOptions.DependsOn].(string)
		options = field.DynamicOptions.Options[dependsOn]
	}

	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}
