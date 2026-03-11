// Package cli provides a headless/CLI adapter for the settings package.
// It renders the same schema used by the Wails frontend in a terminal,
// supporting non-interactive get, set, and show operations for CI/scripting.
package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/jrschumacher/wails-kit/settings"
)

// SettingsProvider is the subset of *settings.Service that the CLI adapter needs.
type SettingsProvider interface {
	GetSchema() settings.Schema
	GetValues() (map[string]any, error)
	SetValues(values map[string]any) ([]settings.ValidationError, error)
}

// Option configures CLI adapter behavior.
type Option func(*config)

type config struct {
	out io.Writer
}

func defaults() *config {
	return &config{out: os.Stdout}
}

// WithOutput sets the output writer (default: os.Stdout).
func WithOutput(w io.Writer) Option {
	return func(c *config) { c.out = w }
}

// Show prints all current settings values grouped by section.
// Password fields are displayed as masked. Computed fields are included.
// Fields hidden by conditions are omitted.
func Show(svc SettingsProvider, opts ...Option) error {
	cfg := defaults()
	for _, o := range opts {
		o(cfg)
	}

	schema := svc.GetSchema()
	values, err := svc.GetValues()
	if err != nil {
		return fmt.Errorf("loading values: %w", err)
	}

	w := cfg.out
	for i, group := range schema.Groups {
		if i > 0 {
			writef(w, "\n")
		}
		writef(w, "[%s]\n", group.Label)
		for _, field := range group.Fields {
			if field.Condition != nil && !conditionMet(field.Condition, values) {
				continue
			}
			val := values[field.Key]
			writef(w, "  %s = %s\n", field.Key, formatValue(field, val))
		}
	}
	return nil
}

// Get returns the current value of a single setting by key as a formatted string.
// Password fields are returned as masked. Returns an error if the key is unknown.
func Get(svc SettingsProvider, key string, opts ...Option) (string, error) {
	schema := svc.GetSchema()
	field, ok := findField(schema, key)
	if !ok {
		return "", fmt.Errorf("unknown setting: %s", key)
	}

	values, err := svc.GetValues()
	if err != nil {
		return "", fmt.Errorf("loading values: %w", err)
	}

	return formatValue(field, values[key]), nil
}

// Set validates and saves a single setting by key.
// The value string is coerced to the field's type (bool for toggles, number for numbers).
func Set(svc SettingsProvider, key, value string, opts ...Option) error {
	schema := svc.GetSchema()
	field, ok := findField(schema, key)
	if !ok {
		return fmt.Errorf("unknown setting: %s", key)
	}
	if field.Type == settings.FieldComputed {
		return fmt.Errorf("cannot set computed field: %s", key)
	}

	// Get current values so conditions and dynamic options can be evaluated
	current, err := svc.GetValues()
	if err != nil {
		return fmt.Errorf("loading values: %w", err)
	}

	coerced, err := coerceValue(field, value)
	if err != nil {
		return err
	}
	current[key] = coerced

	verrs, err := svc.SetValues(current)
	if err != nil {
		return err
	}
	if len(verrs) > 0 {
		return &ValidationErrors{Errors: verrs}
	}
	return nil
}

// ValidationErrors wraps one or more field validation failures.
type ValidationErrors struct {
	Errors []settings.ValidationError
}

func (e *ValidationErrors) Error() string {
	var msgs []string
	for _, ve := range e.Errors {
		msgs = append(msgs, fmt.Sprintf("%s: %s", ve.Field, ve.Message))
	}
	return "validation failed: " + strings.Join(msgs, "; ")
}

// writef writes formatted output, discarding any write error.
func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func conditionMet(c *settings.Condition, values map[string]any) bool {
	val, _ := values[c.Field].(string)
	for _, eq := range c.Equals {
		if val == eq {
			return true
		}
	}
	return false
}

func findField(schema settings.Schema, key string) (settings.Field, bool) {
	for _, group := range schema.Groups {
		for _, field := range group.Fields {
			if field.Key == key {
				return field, true
			}
		}
	}
	return settings.Field{}, false
}

func coerceValue(field settings.Field, value string) (any, error) {
	switch field.Type {
	case settings.FieldToggle:
		switch strings.ToLower(value) {
		case "true", "1", "yes", "y", "on":
			return true, nil
		case "false", "0", "no", "n", "off":
			return false, nil
		default:
			return nil, fmt.Errorf("invalid toggle value: %s (use true/false)", value)
		}
	case settings.FieldNumber:
		if i, err := strconv.Atoi(value); err == nil {
			return i, nil
		}
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number: %s", value)
		}
		return f, nil
	default:
		return value, nil
	}
}

func formatValue(field settings.Field, val any) string {
	if val == nil {
		return "(not set)"
	}
	switch field.Type {
	case settings.FieldPassword:
		s, _ := val.(string)
		if s == settings.SecretMask {
			return settings.SecretMask
		}
		return "(not set)"
	case settings.FieldToggle:
		b, _ := val.(bool)
		if b {
			return "true"
		}
		return "false"
	case settings.FieldSelect:
		s, _ := val.(string)
		for _, opt := range field.Options {
			if opt.Value == s {
				return fmt.Sprintf("%s (%s)", s, opt.Label)
			}
		}
		return s
	default:
		return fmt.Sprintf("%v", val)
	}
}
