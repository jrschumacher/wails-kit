package frontend_test

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/v2/appearance"
	"github.com/jrschumacher/wails-kit/v2/diagnostics"
	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/firstrun"
	"github.com/jrschumacher/wails-kit/v2/health"
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/permissions"
	"github.com/jrschumacher/wails-kit/v2/settings"
	"github.com/jrschumacher/wails-kit/v2/updates"
	"github.com/jrschumacher/wails-kit/v2/windowstate"
)

// TestSettingsTypeParity checks that the TypeScript settings types match the
// Go wire types. GetSchema/Binding.GetSchema return settings.ResolvedSchema,
// not settings.Schema — WP-12 moved Label/Description/Placeholder to
// i18n.Text on the authoring-time types (Schema, Group, Field, SelectOption,
// DynamicOptions), which stripped their JSON tags entirely (they are never
// marshaled directly). The Resolved* family is what actually crosses the
// wire, and its JSON tags were deliberately kept byte-identical to the
// pre-WP-12 authoring types, which is why the TS interface names below
// (Schema, Group, Field, SelectOption, DynamicOptions) still read naturally
// even though they now check against Resolved* on the Go side. See
// types/settings.ts's header comment for the full history.
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

	// Check struct fields via JSON tags. Condition and Validation carry no
	// translatable text, so the authoring and Resolved Go types are the same
	// struct for both — there is nothing to redirect for them.
	structs := map[string]reflect.Type{
		"SelectOption":   reflect.TypeOf(settings.ResolvedSelectOption{}),
		"DynamicOptions": reflect.TypeOf(settings.ResolvedDynamicOptions{}),
		"Condition":      reflect.TypeOf(settings.Condition{}),
		"Validation":     reflect.TypeOf(settings.Validation{}),
		"Field":          reflect.TypeOf(settings.ResolvedField{}),
		"Group":          reflect.TypeOf(settings.ResolvedGroup{}),
		"Schema":         reflect.TypeOf(settings.ResolvedSchema{}),
	}

	for name, typ := range structs {
		t.Run(name, func(t *testing.T) {
			checkJSONFields(t, ts, name, typ)
		})
	}
}

// TestEventsTypeParity checks that TypeScript event constants and payloads
// match Go, for the events that live in events.ts itself (settings, and
// updates' original four events plus the newer EventManaged). Events
// contributed by other packages (health, appearance, i18n, permissions,
// firstrun, windowstate, diagnostics) each have their own file and their own
// Test*TypeParity function below, mirroring the file split in frontend/types.
func TestEventsTypeParity(t *testing.T) {
	ts := readFile(t, "types/events.ts")

	// Check event name constants
	eventConstants := map[string]string{
		"SettingsChanged":   events.SettingsChanged,
		"UpdateAvailable":   updates.EventAvailable,
		"UpdateDownloading": updates.EventDownloading,
		"UpdateReady":       updates.EventReady,
		"UpdateError":       updates.EventError,
		"UpdateManaged":     updates.EventManaged,
	}
	for name, value := range eventConstants {
		if !strings.Contains(ts, `"`+value+`"`) {
			t.Errorf("event constant %s = %q missing from events.ts", name, value)
		}
	}

	// Check payload struct fields
	payloads := map[string]reflect.Type{
		"SettingsChangedPayload":   reflect.TypeOf(events.SettingsChangedPayload{}),
		"UpdateAvailablePayload":   reflect.TypeOf(updates.AvailablePayload{}),
		"UpdateDownloadingPayload": reflect.TypeOf(updates.DownloadingPayload{}),
		"UpdateReadyPayload":       reflect.TypeOf(updates.ReadyPayload{}),
		"UpdateErrorPayload":       reflect.TypeOf(updates.ErrorPayload{}),
		"UpdateManagedPayload":     reflect.TypeOf(updates.ManagedPayload{}),
	}

	for name, typ := range payloads {
		t.Run(name, func(t *testing.T) {
			checkJSONFields(t, ts, name, typ)
		})
	}

	// InstallMethod's non-empty value.
	if !strings.Contains(ts, `"homebrew"`) {
		t.Error(`InstallMethod value "homebrew" missing from events.ts`)
	}
}

// TestErrorsTypeParity checks that TypeScript error codes match Go
// constants. ErrorCode is a deliberately curated subset — the core errors
// package plus updates (the only other package whose codes a generic
// frontend is expected to branch on: update_verify driving a "reinstall"
// prompt, update_managed driving a "use your package manager" message).
// Every other kit package's error codes (health_*, appearance_*,
// permissions_*, firstrun_*, windowstate_*, diagnostics_*, state_*, ...)
// are deliberately not enumerated here, matching the pre-existing
// convention this test predates. See this WP's final report for whether
// that convention should change.
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
		updates.ErrUpdateManaged,
	}

	for _, code := range codes {
		if !strings.Contains(ts, `"`+string(code)+`"`) {
			t.Errorf("ErrorCode %q missing from errors.ts", code)
		}
	}

	// Check UserError fields
	checkJSONFields(t, ts, "UserError", reflect.TypeOf(errors.UserError{}))
}

// TestHealthTypeParity checks health.ts against health/health.go and
// health/binding.go.
//
// health.Snapshot and health.CheckStatus carry no `json:"..."` struct tags
// (unlike every other resolved type in the kit), so reflection over field
// tags cannot check them — there is nothing to read. checkWireShape checks
// the real encoding/json output instead: it is the general fix for "a type
// with no tags makes the tag-based check vacuously pass", and doubles as
// the regression test proving health.ts intentionally matches the actual
// (PascalCase) wire bytes rather than the kit's usual camelCase convention.
// See health.ts's header comment.
func TestHealthTypeParity(t *testing.T) {
	ts := readFile(t, "types/health.ts")

	for _, v := range []string{"unknown", "healthy", "degraded", "down"} {
		if !strings.Contains(ts, `"`+v+`"`) {
			t.Errorf("health.State value %q missing from health.ts", v)
		}
	}
	for _, v := range []string{"connectivity", "backend", "provider"} {
		if !strings.Contains(ts, `"`+v+`"`) {
			t.Errorf("health.Class value %q missing from health.ts", v)
		}
	}
	if !strings.Contains(ts, `"`+health.EventChanged+`"`) {
		t.Errorf("health.EventChanged %q missing from health.ts", health.EventChanged)
	}

	checkWireShape(t, ts, "CheckStatus", health.CheckStatus{
		Name: "api", Class: health.ClassBackend, State: health.StateDown,
		Critical: true, Err: "boom", CheckedAt: time.Now(), Latency: time.Second,
	})
	checkWireShape(t, ts, "Snapshot", health.Snapshot{
		Overall: health.StateDown, Offline: true,
		Checks: []health.CheckStatus{{Name: "api", Class: health.ClassBackend, State: health.StateDown}},
	})

	checkJSONFields(t, ts, "HealthChangedPayload", reflect.TypeOf(health.ChangedPayload{}))
}

// TestAppearanceTypeParity checks appearance.ts against appearance/appearance.go.
func TestAppearanceTypeParity(t *testing.T) {
	ts := readFile(t, "types/appearance.ts")

	for _, v := range []appearance.Mode{appearance.ModeSystem, appearance.ModeLight, appearance.ModeDark} {
		if !strings.Contains(ts, `"`+string(v)+`"`) {
			t.Errorf("appearance.Mode value %q missing from appearance.ts", v)
		}
	}
	if !strings.Contains(ts, `"`+appearance.EventChanged+`"`) {
		t.Errorf("appearance.EventChanged %q missing from appearance.ts", appearance.EventChanged)
	}

	checkJSONFields(t, ts, "AppearanceChangedPayload", reflect.TypeOf(appearance.ChangedPayload{}))
}

// TestI18nTypeParity checks i18n.ts against i18n/localizer.go and i18n/catalog.go.
func TestI18nTypeParity(t *testing.T) {
	ts := readFile(t, "types/i18n.ts")

	if !strings.Contains(ts, `"`+i18n.EventChanged+`"`) {
		t.Errorf("i18n.EventChanged %q missing from i18n.ts", i18n.EventChanged)
	}
	// CLDR plural categories Localizer.Catalog() can emit (i18n/catalog.go's
	// pluralForms map) must all be representable in CatalogPluralEntry.
	for _, category := range []string{"zero", "one", "two", "few", "many", "other"} {
		if !strings.Contains(ts, category) {
			t.Errorf("CLDR plural category %q missing from i18n.ts", category)
		}
	}

	checkJSONFields(t, ts, "I18nChangedPayload", reflect.TypeOf(i18n.ChangedPayload{}))
}

// TestPermissionsTypeParity checks permissions.ts against permissions/permissions.go.
func TestPermissionsTypeParity(t *testing.T) {
	ts := readFile(t, "types/permissions.ts")

	for _, v := range []permissions.Kind{permissions.Notifications, permissions.Accessibility, permissions.FullDiskAccess} {
		if !strings.Contains(ts, `"`+string(v)+`"`) {
			t.Errorf("permissions.Kind value %q missing from permissions.ts", v)
		}
	}
	for _, v := range []permissions.Status{permissions.StatusGranted, permissions.StatusDenied, permissions.StatusNotDetermined, permissions.StatusUnsupported} {
		if !strings.Contains(ts, `"`+string(v)+`"`) {
			t.Errorf("permissions.Status value %q missing from permissions.ts", v)
		}
	}
	if !strings.Contains(ts, `"`+permissions.EventChanged+`"`) {
		t.Errorf("permissions.EventChanged %q missing from permissions.ts", permissions.EventChanged)
	}

	checkJSONFields(t, ts, "PermissionsChangedPayload", reflect.TypeOf(permissions.ChangedPayload{}))
}

// TestFirstrunTypeParity checks firstrun.ts against firstrun/firstrun.go.
func TestFirstrunTypeParity(t *testing.T) {
	ts := readFile(t, "types/firstrun.ts")

	for _, v := range []firstrun.Kind{firstrun.Fresh, firstrun.Upgrade, firstrun.Downgrade, firstrun.Same} {
		if !strings.Contains(ts, `"`+string(v)+`"`) {
			t.Errorf("firstrun.Kind value %q missing from firstrun.ts", v)
		}
	}
	if !strings.Contains(ts, `"`+firstrun.EventTransition+`"`) {
		t.Errorf("firstrun.EventTransition %q missing from firstrun.ts", firstrun.EventTransition)
	}

	checkJSONFields(t, ts, "FirstrunTransitionPayload", reflect.TypeOf(firstrun.TransitionPayload{}))
}

// TestWindowstateTypeParity checks windowstate.ts against windowstate/windowstate.go.
func TestWindowstateTypeParity(t *testing.T) {
	ts := readFile(t, "types/windowstate.ts")

	checkJSONFields(t, ts, "Geometry", reflect.TypeOf(windowstate.Geometry{}))

	for _, name := range []string{windowstate.EventRestored, windowstate.EventError} {
		if !strings.Contains(ts, `"`+name+`"`) {
			t.Errorf("windowstate event %q missing from windowstate.ts", name)
		}
	}
	checkJSONFields(t, ts, "WindowstateRestoredPayload", reflect.TypeOf(windowstate.RestoredPayload{}))
	checkJSONFields(t, ts, "WindowstateErrorPayload", reflect.TypeOf(windowstate.ErrorPayload{}))
}

// TestDiagnosticsTypeParity checks diagnostics.ts against diagnostics/diagnostics.go.
func TestDiagnosticsTypeParity(t *testing.T) {
	ts := readFile(t, "types/diagnostics.ts")

	checkJSONFields(t, ts, "SystemInfo", reflect.TypeOf(diagnostics.SystemInfo{}))

	for _, name := range []string{diagnostics.EventBundleCreated, diagnostics.EventBundleSubmitted} {
		if !strings.Contains(ts, `"`+name+`"`) {
			t.Errorf("diagnostics event %q missing from diagnostics.ts", name)
		}
	}
	checkJSONFields(t, ts, "DiagnosticsBundleCreatedPayload", reflect.TypeOf(diagnostics.BundleCreatedPayload{}))
	checkJSONFields(t, ts, "DiagnosticsBundleSubmittedPayload", reflect.TypeOf(diagnostics.BundleSubmittedPayload{}))
}

// checkJSONFields verifies that every exported JSON-tagged field in a Go struct
// appears in the TypeScript interface definition.
// hasTSProperty reports whether ifaceBlock declares a property literally
// named name.
//
// A bare strings.Contains is not sufficient and was the reason this guard
// was quietly weaker than it looked: "overallTYPO: HealthState" contains
// "overall", so a renamed field — the most likely kind of drift — passed.
// Requiring the name to be followed by an optional "?" and a colon, at a
// property position, makes a rename fail the way a deletion already did.
func hasTSProperty(ifaceBlock, name string) bool {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\??\s*:`)
	return re.MatchString(ifaceBlock)
}

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
		if !hasTSProperty(ifaceBlock, jsonName) {
			t.Errorf("%s: JSON field %q (Go: %s) missing from TypeScript interface", interfaceName, jsonName, field.Name)
		}
	}
}

// checkWireShape verifies that the JSON keys actually produced by
// marshaling value all appear in the TypeScript interface named
// interfaceName. Unlike checkJSONFields (which reads struct tags via
// reflection), this checks the real encoding/json output — the only
// reliable check for a Go type that carries no `json:"..."` tags at all
// (see health.Snapshot / health.CheckStatus). value should have every
// field populated with a non-zero value so omitempty fields are not
// silently skipped and the check stays honest.
func checkWireShape(t *testing.T, ts, interfaceName string, value any) {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("%s: marshal: %v", interfaceName, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("%s: marshaled value is not a JSON object: %v", interfaceName, err)
	}

	ifaceBlock := extractInterface(ts, interfaceName)
	if ifaceBlock == "" {
		t.Fatalf("interface %s not found in TypeScript source", interfaceName)
	}

	for key := range fields {
		if !hasTSProperty(ifaceBlock, key) {
			t.Errorf("%s: JSON field %q missing from TypeScript interface", interfaceName, key)
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
