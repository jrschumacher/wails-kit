// Package cli provides a headless/CLI adapter for the settings package.
// It renders the same schema used by the Wails frontend in a terminal,
// supporting non-interactive get, set, and show operations for CI/scripting.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

// Human-facing strings this package renders itself — everything that is
// not a schema label (schema Group/Field/SelectOption labels arrive
// already resolved on settings.ResolvedSchema; see SettingsProvider).
// Resolved via WithLocalizer; a nil localizer falls back to Other, exactly
// like every other localizer in the kit (see permissions.WithLocalizer).
//
// Deliberately NOT included here: the raw value tokens formatValue and
// coerceValue produce/accept for toggle and number fields ("true"/"false",
// "42"). Those are the machine-parseable surface a script piping
// settingscli.Get output (or building its own --json mode on top of
// SettingsProvider.GetValues) depends on; localizing them would silently
// break that round-trip depending on the active locale. See
// TestMachineValuesUnaffectedByLocalizer.
var (
	textNotSet            = i18n.T("wailskit.settingscli.value.not_set", "(not set)")
	textUnknownSetting    = i18n.T("wailskit.settingscli.errors.unknown_setting", "unknown setting")
	textLoadingValues     = i18n.T("wailskit.settingscli.errors.loading_values", "loading values")
	textCannotSetComputed = i18n.T("wailskit.settingscli.errors.cannot_set_computed", "cannot set computed field")
	textInvalidToggle     = i18n.T("wailskit.settingscli.errors.invalid_toggle", "invalid toggle value: %s (use true/false)")
	textInvalidNumber     = i18n.T("wailskit.settingscli.errors.invalid_number", "invalid number: %s")
	textValidationFailed  = i18n.T("wailskit.settingscli.errors.validation_failed", "validation failed")
)

// localize resolves t against l, falling back to t.Other (formatted with
// args via fmt.Sprintf, exactly like i18n.Localizer.T) when l is nil —
// callers in this package never have a guaranteed localizer, since every
// entry point (Show/Get/Set) takes it as an optional Option.
func localize(l *i18n.Localizer, t i18n.Text, args ...any) string {
	if l == nil {
		if len(args) == 0 {
			return t.Other
		}
		return fmt.Sprintf(t.Other, args...)
	}
	return l.T(t, args...)
}

// SettingsProvider is the subset of *settings.Service that the CLI adapter
// needs. It takes the resolved schema: the CLI renders labels, and by the
// time GetSchema returns, i18n.Text has already been resolved to strings for
// the active locale.
type SettingsProvider interface {
	GetSchema() settings.ResolvedSchema
	GetValues() (map[string]any, error)
	SetValues(values map[string]any) ([]settings.ValidationError, error)
}

// Option configures CLI adapter behavior.
type Option func(*config)

type config struct {
	out       io.Writer
	localizer *i18n.Localizer
}

func defaults() *config {
	return &config{out: os.Stdout}
}

// WithOutput sets the output writer (default: os.Stdout).
func WithOutput(w io.Writer) Option {
	return func(c *config) { c.out = w }
}

// WithLocalizer wires an *i18n.Localizer used to resolve this package's own
// human-facing strings: section/value placeholders ("(not set)") and error
// text from Show/Get/Set. It never changes what SettingsProvider itself
// returns — GetSchema's labels are already resolved (by the settings
// service's own localizer, wired separately via settings.WithLocalizer) by
// the time this package sees them, and GetValues/SetValues carry raw stored
// data this package only formats, never translates. Optional; a nil (or
// never-configured) localizer falls back to this package's built-in English
// text, the same guarantee permissions.WithLocalizer and
// settings.WithLocalizer make.
func WithLocalizer(l *i18n.Localizer) Option {
	return func(c *config) { c.localizer = l }
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
		return fmt.Errorf("%s: %w", localize(cfg.localizer, textLoadingValues), err)
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
			writef(w, "  %s = %s\n", field.Key, formatValue(cfg.localizer, field, val))
		}
	}
	return nil
}

// Get returns the current value of a single setting by key as a formatted string.
// Password fields are returned as masked. Returns an error if the key is unknown.
func Get(svc SettingsProvider, key string, opts ...Option) (string, error) {
	cfg := defaults()
	for _, o := range opts {
		o(cfg)
	}

	schema := svc.GetSchema()
	field, ok := findField(schema, key)
	if !ok {
		return "", fmt.Errorf("%s: %s", localize(cfg.localizer, textUnknownSetting), key)
	}

	values, err := svc.GetValues()
	if err != nil {
		return "", fmt.Errorf("%s: %w", localize(cfg.localizer, textLoadingValues), err)
	}

	return formatValue(cfg.localizer, field, values[key]), nil
}

// Set validates and saves a single setting by key.
// The value string is coerced to the field's type (bool for toggles, number for numbers).
func Set(svc SettingsProvider, key, value string, opts ...Option) error {
	cfg := defaults()
	for _, o := range opts {
		o(cfg)
	}

	schema := svc.GetSchema()
	field, ok := findField(schema, key)
	if !ok {
		return fmt.Errorf("%s: %s", localize(cfg.localizer, textUnknownSetting), key)
	}
	if field.Type == settings.FieldComputed {
		return fmt.Errorf("%s: %s", localize(cfg.localizer, textCannotSetComputed), key)
	}

	// Get current values so conditions and dynamic options can be evaluated
	current, err := svc.GetValues()
	if err != nil {
		return fmt.Errorf("%s: %w", localize(cfg.localizer, textLoadingValues), err)
	}

	coerced, err := coerceValue(cfg.localizer, field, value)
	if err != nil {
		return err
	}
	current[key] = coerced

	verrs, err := svc.SetValues(current)
	if err != nil {
		return err
	}
	if len(verrs) > 0 {
		return &ValidationErrors{Errors: verrs, localizer: cfg.localizer}
	}
	return nil
}

// ValidationErrors wraps one or more field validation failures.
type ValidationErrors struct {
	Errors []settings.ValidationError

	// localizer resolves the "validation failed" prefix in Error(). Set by
	// Set() from its own WithLocalizer option; a ValidationErrors built
	// directly (e.g. by a caller assembling its own, or in this package's
	// tests) leaves it nil and falls back to English, like every other
	// localizer in this package.
	localizer *i18n.Localizer
}

func (e *ValidationErrors) Error() string {
	var msgs []string
	for _, ve := range e.Errors {
		msgs = append(msgs, fmt.Sprintf("%s: %s", ve.Field, ve.Message))
	}
	return fmt.Sprintf("%s: %s", localize(e.localizer, textValidationFailed), strings.Join(msgs, "; "))
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

func findField(schema settings.ResolvedSchema, key string) (settings.ResolvedField, bool) {
	for _, group := range schema.Groups {
		for _, field := range group.Fields {
			if field.Key == key {
				return field, true
			}
		}
	}
	return settings.ResolvedField{}, false
}

// coerceValue converts value (a CLI argument string) to the type field
// expects. The tokens it accepts ("true"/"1"/"yes"/"y"/"on", and their
// false-ish counterparts) are fixed and never localized — they are the
// machine-readable counterpart to formatValue's toggle/number output, and
// must round-trip through Set/Get regardless of the active locale. Only the
// error text on an invalid input is localized.
func coerceValue(l *i18n.Localizer, field settings.ResolvedField, value string) (any, error) {
	switch field.Type {
	case settings.FieldToggle:
		switch strings.ToLower(value) {
		case "true", "1", "yes", "y", "on":
			return true, nil
		case "false", "0", "no", "n", "off":
			return false, nil
		default:
			return nil, errors.New(localize(l, textInvalidToggle, value))
		}
	case settings.FieldNumber:
		if i, err := strconv.Atoi(value); err == nil {
			return i, nil
		}
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, errors.New(localize(l, textInvalidNumber, value))
		}
		return f, nil
	default:
		return value, nil
	}
}

// formatValue renders val for display. The "(not set)" placeholder is
// localized; the toggle tokens "true"/"false" and default numeric/string
// formatting are not — see the package doc comment on textNotSet and
// friends for why (they are coerceValue's accepted input alphabet, and
// must stay stable across locales for Get/Set round-tripping and any
// consumer's own machine-readable output built on this package).
func formatValue(l *i18n.Localizer, field settings.ResolvedField, val any) string {
	if val == nil {
		return localize(l, textNotSet)
	}
	switch field.Type {
	case settings.FieldPassword:
		s, _ := val.(string)
		if s == settings.SecretMask {
			return settings.SecretMask
		}
		return localize(l, textNotSet)
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
