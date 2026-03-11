// Package cli provides a headless/CLI adapter for the settings package.
// It renders the same schema used by the Wails frontend in a terminal,
// supporting interactive configuration, non-interactive set, show, and
// editor-based editing.
package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
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
	in  io.Reader
	out io.Writer
}

func defaults() *config {
	return &config{in: os.Stdin, out: os.Stdout}
}

// WithInput sets the input reader (default: os.Stdin).
func WithInput(r io.Reader) Option {
	return func(c *config) { c.in = r }
}

// WithOutput sets the output writer (default: os.Stdout).
func WithOutput(w io.Writer) Option {
	return func(c *config) { c.out = w }
}

// Show prints all current settings values grouped by section.
// Password fields are displayed as masked. Computed fields are included.
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

// Configure walks through all schema fields interactively, prompting the
// user for each value. Fields hidden by conditions are skipped.
// Pressing Enter without input keeps the current value.
func Configure(svc SettingsProvider, opts ...Option) error {
	cfg := defaults()
	for _, o := range opts {
		o(cfg)
	}

	schema := svc.GetSchema()
	values, err := svc.GetValues()
	if err != nil {
		return fmt.Errorf("loading values: %w", err)
	}

	scanner := bufio.NewScanner(cfg.in)
	updated := make(map[string]any)
	for k, v := range values {
		updated[k] = v
	}

	w := cfg.out
	for _, group := range schema.Groups {
		writef(w, "\n[%s]\n", group.Label)
		for _, field := range group.Fields {
			if field.Type == settings.FieldComputed {
				continue
			}
			if field.Condition != nil && !conditionMet(field.Condition, updated) {
				continue
			}

			input, err := promptField(w, scanner, field, updated)
			if err != nil {
				return err
			}
			if input != nil {
				updated[field.Key] = input
			}
		}
	}

	verrs, err := svc.SetValues(updated)
	if err != nil {
		return err
	}
	if len(verrs) > 0 {
		return &ValidationErrors{Errors: verrs}
	}
	return nil
}

// Edit opens the current settings in $EDITOR (or vi) as JSON.
// Computed and password fields are excluded from the editable file.
// After the editor exits, changes are validated and saved.
func Edit(svc SettingsProvider, opts ...Option) error {
	cfg := defaults()
	for _, o := range opts {
		o(cfg)
	}

	schema := svc.GetSchema()
	values, err := svc.GetValues()
	if err != nil {
		return fmt.Errorf("loading values: %w", err)
	}

	// Build editable map (exclude computed and password fields)
	editable := make(map[string]any)
	for _, group := range schema.Groups {
		for _, field := range group.Fields {
			if field.Type == settings.FieldComputed || field.Type == settings.FieldPassword {
				continue
			}
			if v, ok := values[field.Key]; ok {
				editable[field.Key] = v
			}
		}
	}

	data, err := json.MarshalIndent(editable, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling settings: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "settings-*.json")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = cfg.in
	cmd.Stdout = cfg.out
	if f, ok := cfg.out.(*os.File); ok {
		cmd.Stderr = f
	} else {
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor exited with error: %w", err)
	}

	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("reading edited file: %w", err)
	}

	var newValues map[string]any
	if err := json.Unmarshal(edited, &newValues); err != nil {
		return fmt.Errorf("parsing edited JSON: %w", err)
	}

	// Merge password fields back (they were excluded from editing)
	for _, group := range schema.Groups {
		for _, field := range group.Fields {
			if field.Type == settings.FieldPassword {
				newValues[field.Key] = values[field.Key]
			}
		}
	}

	verrs, err := svc.SetValues(newValues)
	if err != nil {
		return err
	}
	if len(verrs) > 0 {
		return &ValidationErrors{Errors: verrs}
	}

	writef(cfg.out, "Settings saved.\n")
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

// promptField displays a prompt for a single field and reads user input.
// Returns nil if the user presses Enter (keep current value).
func promptField(w io.Writer, scanner *bufio.Scanner, field settings.Field, values map[string]any) (any, error) {
	current := values[field.Key]

	// Show description if present
	if field.Description != "" {
		writef(w, "  # %s\n", field.Description)
	}

	switch field.Type {
	case settings.FieldSelect:
		return promptSelect(w, scanner, field, values, current)
	case settings.FieldToggle:
		return promptToggle(w, scanner, field, current)
	case settings.FieldPassword:
		return promptPassword(w, scanner, field, current)
	case settings.FieldNumber:
		return promptNumber(w, scanner, field, current)
	default: // FieldText
		return promptText(w, scanner, field, current)
	}
}

func promptText(w io.Writer, scanner *bufio.Scanner, field settings.Field, current any) (any, error) {
	cur, _ := current.(string)
	if cur != "" {
		writef(w, "  %s [%s]: ", field.Label, cur)
	} else if field.Placeholder != "" {
		writef(w, "  %s (%s): ", field.Label, field.Placeholder)
	} else {
		writef(w, "  %s: ", field.Label)
	}

	if !scanner.Scan() {
		return nil, scanner.Err()
	}
	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return nil, nil // keep current
	}
	return input, nil
}

func promptPassword(w io.Writer, scanner *bufio.Scanner, field settings.Field, current any) (any, error) {
	cur, _ := current.(string)
	if cur == settings.SecretMask {
		writef(w, "  %s [set]: ", field.Label)
	} else {
		writef(w, "  %s: ", field.Label)
	}

	if !scanner.Scan() {
		return nil, scanner.Err()
	}
	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		if cur == settings.SecretMask {
			return settings.SecretMask, nil // keep existing secret
		}
		return nil, nil
	}
	return input, nil
}

func promptSelect(w io.Writer, scanner *bufio.Scanner, field settings.Field, values map[string]any, current any) (any, error) {
	options := resolveOptions(field, values)
	if len(options) == 0 {
		return nil, nil
	}

	cur, _ := current.(string)
	writef(w, "  %s", field.Label)
	if cur != "" {
		writef(w, " [%s]", cur)
	}
	writef(w, ":\n")

	for i, opt := range options {
		marker := "  "
		if opt.Value == cur {
			marker = "> "
		}
		writef(w, "    %s%d) %s\n", marker, i+1, opt.Label)
	}
	writef(w, "  Choice: ")

	if !scanner.Scan() {
		return nil, scanner.Err()
	}
	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return nil, nil // keep current
	}

	// Accept number or value directly
	if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(options) {
		return options[idx-1].Value, nil
	}
	// Try matching by value string
	for _, opt := range options {
		if opt.Value == input {
			return input, nil
		}
	}
	return nil, fmt.Errorf("invalid option for %s: %s", field.Key, input)
}

func promptToggle(w io.Writer, scanner *bufio.Scanner, field settings.Field, current any) (any, error) {
	cur, _ := current.(bool)
	label := "n"
	if cur {
		label = "y"
	}
	writef(w, "  %s (y/n) [%s]: ", field.Label, label)

	if !scanner.Scan() {
		return nil, scanner.Err()
	}
	input := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if input == "" {
		return nil, nil // keep current
	}
	switch input {
	case "y", "yes", "true", "1", "on":
		return true, nil
	case "n", "no", "false", "0", "off":
		return false, nil
	default:
		return nil, fmt.Errorf("invalid toggle value for %s: %s (use y/n)", field.Key, input)
	}
}

func promptNumber(w io.Writer, scanner *bufio.Scanner, field settings.Field, current any) (any, error) {
	hint := ""
	if field.Validation != nil {
		if field.Validation.Min != nil && field.Validation.Max != nil {
			hint = fmt.Sprintf(" (%d-%d)", *field.Validation.Min, *field.Validation.Max)
		} else if field.Validation.Min != nil {
			hint = fmt.Sprintf(" (min %d)", *field.Validation.Min)
		} else if field.Validation.Max != nil {
			hint = fmt.Sprintf(" (max %d)", *field.Validation.Max)
		}
	}

	curStr := ""
	if current != nil {
		curStr = fmt.Sprintf("%v", current)
	}
	if curStr != "" {
		writef(w, "  %s%s [%s]: ", field.Label, hint, curStr)
	} else {
		writef(w, "  %s%s: ", field.Label, hint)
	}

	if !scanner.Scan() {
		return nil, scanner.Err()
	}
	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return nil, nil // keep current
	}

	// Try int first, fall back to float
	if i, err := strconv.Atoi(input); err == nil {
		return i, nil
	}
	f, err := strconv.ParseFloat(input, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid number for %s: %s", field.Key, input)
	}
	return f, nil
}

// resolveOptions returns the applicable select options, considering dynamic options.
func resolveOptions(field settings.Field, values map[string]any) []settings.SelectOption {
	if field.DynamicOptions != nil {
		dep, _ := values[field.DynamicOptions.DependsOn].(string)
		return field.DynamicOptions.Options[dep]
	}
	return field.Options
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
		// Try to find the label for the value
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

// Keys returns all setting keys from the schema, sorted alphabetically.
func Keys(svc SettingsProvider) []string {
	schema := svc.GetSchema()
	var keys []string
	for _, group := range schema.Groups {
		for _, field := range group.Fields {
			keys = append(keys, field.Key)
		}
	}
	sort.Strings(keys)
	return keys
}
