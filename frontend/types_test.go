package frontend_test

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/events"
	"github.com/jrschumacher/wails-kit/settings"
	"github.com/jrschumacher/wails-kit/tasks"
	"github.com/jrschumacher/wails-kit/updates"
)

// TestSettingsTypeParity checks that the TypeScript settings types match Go structs.
func TestSettingsTypeParity(t *testing.T) {
	ts := readFile(t, "types/settings.ts")

	// Check FieldType values
	fieldTypes := map[settings.FieldType]bool{
		settings.FieldText:     true,
		settings.FieldPassword: true,
		settings.FieldSelect:   true,
		settings.FieldToggle:   true,
		settings.FieldComputed: true,
		settings.FieldNumber:   true,
	}
	for ft := range fieldTypes {
		if !strings.Contains(ts, `"`+string(ft)+`"`) {
			t.Errorf("FieldType %q missing from settings.ts", ft)
		}
	}

	// Check struct fields via JSON tags
	structs := map[string]reflect.Type{
		"SelectOption":   reflect.TypeOf(settings.SelectOption{}),
		"DynamicOptions": reflect.TypeOf(settings.DynamicOptions{}),
		"Condition":      reflect.TypeOf(settings.Condition{}),
		"Validation":     reflect.TypeOf(settings.Validation{}),
		"Check":          reflect.TypeOf(settings.Check{}),
		"Field":          reflect.TypeOf(settings.Field{}),
		"Group":          reflect.TypeOf(settings.Group{}),
		"Schema":         reflect.TypeOf(settings.Schema{}),
	}

	for name, typ := range structs {
		t.Run(name, func(t *testing.T) {
			checkJSONFields(t, ts, name, typ)
		})
	}
}

// TestEventsTypeParity checks that TypeScript event constants and payloads match Go.
func TestEventsTypeParity(t *testing.T) {
	ts := readFile(t, "types/events.ts")

	// Check event name constants
	eventConstants := map[string]string{
		"SettingsChanged":     events.SettingsChanged,
		"SettingsCheckFailed": settings.EventCheckFailed,
		"UpdateAvailable":     updates.EventAvailable,
		"UpdateDownloading":   updates.EventDownloading,
		"UpdateReady":         updates.EventReady,
		"UpdateError":         updates.EventError,
		"TasksChanged":        tasks.EventChanged,
	}
	for name, value := range eventConstants {
		if !strings.Contains(ts, `"`+value+`"`) {
			t.Errorf("event constant %s = %q missing from events.ts", name, value)
		}
	}

	// Check payload struct fields
	payloads := map[string]reflect.Type{
		"SettingsChangedPayload":     reflect.TypeOf(events.SettingsChangedPayload{}),
		"SettingsCheckFailedPayload": reflect.TypeOf(settings.CheckFailedPayload{}),
		"UpdateAvailablePayload":     reflect.TypeOf(updates.AvailablePayload{}),
		"UpdateDownloadingPayload":   reflect.TypeOf(updates.DownloadingPayload{}),
		"UpdateReadyPayload":         reflect.TypeOf(updates.ReadyPayload{}),
		"UpdateErrorPayload":         reflect.TypeOf(updates.ErrorPayload{}),
		"TasksChangedPayload":        reflect.TypeOf(tasks.ChangedPayload{}),
	}

	for name, typ := range payloads {
		t.Run(name, func(t *testing.T) {
			checkJSONFields(t, ts, name, typ)
		})
	}
}

// TestErrorsTypeParity checks that TypeScript error codes match Go constants.
func TestErrorsTypeParity(t *testing.T) {
	ts := readFile(t, "types/errors.ts")

	// All error codes from errors package and updates package
	codes := []errors.Code{
		errors.ErrAuthInvalid,
		errors.ErrAuthExpired,
		errors.ErrAuthMissing,
		errors.ErrNotFound,
		errors.ErrPermission,
		errors.ErrValidation,
		errors.ErrRateLimited,
		errors.ErrTimeout,
		errors.ErrCancelled,
		errors.ErrInternal,
		errors.ErrStorageRead,
		errors.ErrStorageWrite,
		errors.ErrConfigInvalid,
		errors.ErrConfigMissing,
		errors.ErrProvider,
		updates.ErrUpdateCheck,
		updates.ErrUpdateDownload,
		updates.ErrUpdateApply,
		updates.ErrUpdateVerify,
		settings.ErrCheckFailed,
		settings.ErrCheckTimeout,
		settings.ErrUnknownSetting,
		tasks.ErrNotFound,
		tasks.ErrDuplicateID,
		tasks.ErrInvalidSpec,
		tasks.ErrInvalidState,
		tasks.ErrUnsupported,
	}

	for _, code := range codes {
		if !strings.Contains(ts, `"`+string(code)+`"`) {
			t.Errorf("ErrorCode %q missing from errors.ts", code)
		}
	}

	// Check UserError fields
	checkJSONFields(t, ts, "UserError", reflect.TypeOf(errors.UserError{}))
}

// TestTasksTypeParity checks the tasks:changed snapshot contract exactly:
// each union type holds the same values as Go, and each snapshot field is
// present with the same nullability (a Go pointer is "| null" in TypeScript).
func TestTasksTypeParity(t *testing.T) {
	ts := readFile(t, "types/tasks.ts")

	var states []string
	for _, s := range tasks.States {
		states = append(states, `"`+string(s)+`"`)
	}
	checkUnion(t, ts, "TaskState", states)
	checkUnion(t, ts, "TaskRetry", []string{
		`"` + string(tasks.RetryAuto) + `"`, `"` + string(tasks.RetryManual) + `"`, `"` + string(tasks.RetryUnsafe) + `"`,
	})
	stops := []string{"false"} // StopNone is sent as false
	for _, m := range []tasks.StopMode{tasks.StopCancel, tasks.StopStop, tasks.StopStopAsks, tasks.StopPause, tasks.StopUnsafe} {
		stops = append(stops, `"`+string(m)+`"`)
	}
	checkUnion(t, ts, "TaskStop", stops)

	checkFieldTypes(t, ts, "TaskSnapshot", reflect.TypeOf(tasks.Snapshot{}))
	checkFieldTypes(t, ts, "TaskError", reflect.TypeOf(tasks.ErrorInfo{}))

	events := readFile(t, "types/events.ts")
	checkFieldTypes(t, events, "TasksChangedPayload", reflect.TypeOf(tasks.ChangedPayload{}))
}

// checkUnion compares the members of `export type name = a | b | ...;` with
// want, as sets.
func checkUnion(t *testing.T, ts, name string, want []string) {
	t.Helper()
	re := regexp.MustCompile(`(?s)export type\s+` + regexp.QuoteMeta(name) + `\s*=(.*?);`)
	m := re.FindStringSubmatch(ts)
	if m == nil {
		t.Fatalf("type %s not found", name)
	}
	got := map[string]bool{}
	for _, member := range regexp.MustCompile(`"[^"]*"|\bfalse\b|\btrue\b|\bnull\b|\bstring\b|\bnumber\b`).FindAllString(m[1], -1) {
		got[member] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("%s: %s missing", name, w)
		}
		delete(got, w)
	}
	for extra := range got {
		t.Errorf("%s: %s is not in Go", name, extra)
	}
}

// checkFieldTypes checks that each JSON field of typ is declared in the
// interface, and is nullable exactly when the Go field is a pointer.
func checkFieldTypes(t *testing.T, ts, interfaceName string, typ reflect.Type) {
	t.Helper()
	block := extractInterface(ts, interfaceName)
	if block == "" {
		t.Fatalf("interface %s not found", interfaceName)
	}
	for i := range typ.NumField() {
		field := typ.Field(i)
		tag := strings.Split(field.Tag.Get("json"), ",")[0]
		if !field.IsExported() || tag == "" || tag == "-" {
			continue
		}
		re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(tag) + `(\??):\s*([^;]+);`)
		m := re.FindStringSubmatch(block)
		if m == nil {
			t.Errorf("%s: field %q (Go: %s) not declared", interfaceName, tag, field.Name)
			continue
		}
		if m[1] == "?" {
			t.Errorf("%s.%s: optional, but Go always sends it", interfaceName, tag)
		}
		nullable := regexp.MustCompile(`\|\s*null\b`).MatchString(m[2])
		if pointer := field.Type.Kind() == reflect.Pointer; pointer != nullable {
			t.Errorf("%s.%s: TypeScript type %q, Go type %s: nullability differs", interfaceName, tag, strings.TrimSpace(m[2]), field.Type)
		}
	}
}

// checkJSONFields verifies that every exported JSON-tagged field in a Go struct
// appears in the TypeScript interface definition.
func checkJSONFields(t *testing.T, ts, interfaceName string, typ reflect.Type) {
	t.Helper()

	// Extract the interface block from the TS source
	ifaceBlock := extractInterface(ts, interfaceName)
	if ifaceBlock == "" {
		t.Fatalf("interface %s not found in TypeScript source", interfaceName)
	}

	for i := range typ.NumField() {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}

		jsonName := strings.Split(tag, ",")[0]
		if !strings.Contains(ifaceBlock, jsonName) {
			t.Errorf("%s: JSON field %q (Go: %s) missing from TypeScript interface", interfaceName, jsonName, field.Name)
		}
	}
}

// extractInterface pulls the body of a TypeScript interface from source text.
func extractInterface(ts, name string) string {
	re := regexp.MustCompile(`interface\s+` + regexp.QuoteMeta(name) + `\s*\{`)
	loc := re.FindStringIndex(ts)
	if loc == nil {
		return ""
	}

	// Find matching closing brace
	start := loc[1] - 1 // include opening brace
	depth := 0
	for i := start; i < len(ts); i++ {
		switch ts[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return ts[start : i+1]
			}
		}
	}
	return ""
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(data)
}
