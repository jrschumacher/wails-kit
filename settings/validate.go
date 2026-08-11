package settings

import (
	"fmt"
	"regexp"
	"unicode/utf8"

	"github.com/jrschumacher/wails-kit/v2/i18n"
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

// Validation message templates (settings/locales/en.json). Each is a %s
// (field label) plus, where relevant, one more verb — resolved through the
// same localizer as field labels, so "Foo is required" is fully localized,
// not just the field name.
var (
	msgRequired        = i18n.T("wailskit.settings.validation.required", "%s is required")
	msgPattern         = i18n.T("wailskit.settings.validation.pattern", "%s has invalid format")
	msgMinLen          = i18n.T("wailskit.settings.validation.min_length", "%s must be at least %d characters")
	msgMaxLen          = i18n.T("wailskit.settings.validation.max_length", "%s must be at most %d characters")
	msgMin             = i18n.T("wailskit.settings.validation.min", "%s must be at least %d")
	msgMax             = i18n.T("wailskit.settings.validation.max", "%s must be at most %d")
	msgInvalidToggle   = i18n.T("wailskit.settings.validation.invalid_type_toggle", "%s must be true or false")
	msgInvalidPassword = i18n.T("wailskit.settings.validation.invalid_type_password", "%s must be a string")
	msgInvalidOption   = i18n.T("wailskit.settings.validation.invalid_option", "%s has an invalid option")
)

// ValidationError represents a field-level validation failure. Message is
// already resolved to a plain string (via localizer, or the built-in
// English fallback when localizer is nil) — like Field.Label on the wire,
// this is never an i18n.Text on the JSON/Go-caller surface.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// Validate checks values against schema, returning one ValidationError per
// failed rule (nil if none). localizer resolves field labels and message
// templates; pass nil for the built-in English messages — see resolveText.
func Validate(schema Schema, values map[string]any, localizer *i18n.Localizer) []ValidationError {
	var errs []ValidationError

	for _, group := range schema.Groups {
		for _, field := range group.Fields {
			if field.Validation == nil && field.Type != FieldSelect && field.Type != FieldToggle && field.Type != FieldPassword {
				continue
			}

			// Skip validation for hidden fields
			if field.Condition != nil && !conditionMet(field.Condition, values) {
				continue
			}

			val := values[field.Key]
			fieldErrs := validateField(field, val, values, localizer)
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

func validateField(field Field, val any, values map[string]any, l *i18n.Localizer) []ValidationError {
	var errs []ValidationError
	v := field.Validation
	label := resolveText(field.Label, l)

	str, isStr := val.(string)
	num := toFloat64(val)
	isNum := num != nil

	if v != nil && v.Required {
		if val == nil || (isStr && str == "") {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf(resolveText(msgRequired, l), label), Code: CodeRequired})
			return errs
		}
	}

	// Toggle type validation: must be a bool if provided
	if field.Type == FieldToggle && val != nil {
		if _, ok := val.(bool); !ok {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf(resolveText(msgInvalidToggle, l), label), Code: CodeInvalidType})
		}
	}

	// Password type validation: must be a string if provided. A non-string
	// value (e.g. a stray number) must be rejected here rather than falling
	// through to a raw type assertion that silently treats it as "" and
	// deletes the stored secret.
	if field.Type == FieldPassword && val != nil {
		if _, ok := val.(string); !ok {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf(resolveText(msgInvalidPassword, l), label), Code: CodeInvalidType})
		}
	}

	if isStr && str != "" && v != nil {
		if v.Pattern != "" {
			if matched, _ := regexp.MatchString(v.Pattern, str); !matched {
				errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf(resolveText(msgPattern, l), label), Code: CodePattern})
			}
		}
		if v.MinLen > 0 && utf8.RuneCountInString(str) < v.MinLen {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf(resolveText(msgMinLen, l), label, v.MinLen), Code: CodeMinLen})
		}
		if v.MaxLen > 0 && utf8.RuneCountInString(str) > v.MaxLen {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf(resolveText(msgMaxLen, l), label, v.MaxLen), Code: CodeMaxLen})
		}
	}

	if isNum && v != nil {
		n := *num
		if v.Min != nil && n < float64(*v.Min) {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf(resolveText(msgMin, l), label, *v.Min), Code: CodeMin})
		}
		if v.Max != nil && n > float64(*v.Max) {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf(resolveText(msgMax, l), label, *v.Max), Code: CodeMax})
		}
	}

	if field.Type == FieldSelect && isStr && str != "" && hasSelectableOptions(field, values) && !selectOptionAllowed(field, str, values) {
		errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf(resolveText(msgInvalidOption, l), label), Code: CodeInvalidOption})
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
