package settings

import (
	"fmt"
	"regexp"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func Validate(schema Schema, values map[string]any) []ValidationError {
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
			fieldErrs := validateField(field, val)
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

func validateField(field Field, val any) []ValidationError {
	var errs []ValidationError
	v := field.Validation

	str, isStr := val.(string)
	num, isNum := val.(float64)

	if v.Required {
		if val == nil || (isStr && str == "") {
			errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s is required", field.Label)})
			return errs
		}
	}

	if isStr && str != "" {
		if v.Pattern != "" {
			if matched, _ := regexp.MatchString(v.Pattern, str); !matched {
				errs = append(errs, ValidationError{Field: field.Key, Message: fmt.Sprintf("%s has invalid format", field.Label)})
			}
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
