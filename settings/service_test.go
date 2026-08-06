package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// themeOptions is the stock option list for the "theme" select used across
// these fixtures. ValidateSchema rejects a select with no options, and rightly
// so, but spelling them out at every call site would bury what each test is
// actually about.
func themeOptions() []SelectOption {
	return []SelectOption{{Label: "Light", Value: "light"}, {Label: "Dark", Value: "dark"}}
}

// newTestService constructs a Service, failing the test on a construction
// error so call sites stay readable.
func newTestService(t *testing.T, opts ...ServiceOption) *Service {
	t.Helper()
	svc, err := NewService(opts...)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestNewService_WithAppName(t *testing.T) {
	configDir := redirectUserConfigDir(t)

	svc := newTestService(t, WithAppName("testapp"))
	expected := filepath.Join(configDir, "testapp", "settings.json")
	if svc.store.Path() != expected {
		t.Errorf("expected store path %q, got %q", expected, svc.store.Path())
	}
}

func TestNewService_ReturnsErrorWhenConfigDirUnresolvable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unsetting AppData is not a reliable failure trigger on windows")
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	svc, err := NewService(WithAppName("testapp"))
	if err == nil {
		t.Fatalf("expected an error, got service with path %q", svc.store.Path())
	}
	if svc != nil {
		t.Error("expected a nil service alongside the error")
	}
}

// A schema is developer-authored, so a pattern that does not compile is a bug
// to surface at construction rather than an "invalid format" message shown to
// an end user at save time.
func TestNewService_ReturnsErrorOnUncompilablePattern(t *testing.T) {
	svc, err := NewService(
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithGroup(Group{
			Key:   "g",
			Label: "G",
			Fields: []Field{
				{Key: "k", Type: FieldText, Label: "K", Validation: &Validation{Pattern: "([unclosed"}},
			},
		}),
	)
	if err == nil {
		t.Fatal("expected an error for an uncompilable validation pattern")
	}
	if svc != nil {
		t.Error("expected a nil service alongside the error")
	}
	if !strings.Contains(err.Error(), "k") {
		t.Errorf("expected the error to name the offending field, got %v", err)
	}
}

func TestNewService_RegistersDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := newTestService(t,
		WithStorePath(path),
		WithGroup(Group{
			Key:   "general",
			Label: "General",
			Fields: []Field{
				{Key: "theme", Type: FieldSelect, Label: "Theme", Default: "dark", Options: themeOptions()},
				{Key: "lang", Type: FieldSelect, Label: "Language", Default: "en",
					Options: []SelectOption{{Label: "English", Value: "en"}, {Label: "French", Value: "fr"}}},
				{Key: "notes", Type: FieldText, Label: "Notes"},
			},
		}),
	)

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values["theme"] != "dark" {
		t.Errorf("expected default theme=dark, got %v", values["theme"])
	}
	if values["lang"] != "en" {
		t.Errorf("expected default lang=en, got %v", values["lang"])
	}
	if _, ok := values["notes"]; ok {
		t.Errorf("expected notes to not be in defaults (no Default set)")
	}
}

func TestGetSchema_ReturnsAllGroups(t *testing.T) {
	g1 := Group{Key: "g1", Label: "Group 1", Fields: []Field{{Key: "f1", Type: FieldText, Label: "F1"}}}
	g2 := Group{Key: "g2", Label: "Group 2", Fields: []Field{{Key: "f2", Type: FieldToggle, Label: "F2"}}}

	svc := newTestService(t, WithGroup(g1), WithGroup(g2))
	schema := svc.GetSchema()

	if len(schema.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(schema.Groups))
	}
	if schema.Groups[0].Key != "g1" {
		t.Errorf("expected first group key=g1, got %s", schema.Groups[0].Key)
	}
	if schema.Groups[1].Key != "g2" {
		t.Errorf("expected second group key=g2, got %s", schema.Groups[1].Key)
	}
}

func TestGetValues_AppliesComputeFuncs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := newTestService(t,
		WithStorePath(path),
		WithGroup(Group{
			Key:   "info",
			Label: "Info",
			Fields: []Field{
				{Key: "first", Type: FieldText, Label: "First", Default: "John"},
				{Key: "last", Type: FieldText, Label: "Last", Default: "Doe"},
				{Key: "full_name", Type: FieldComputed, Label: "Full Name"},
			},
			ComputeFuncs: map[string]ComputeFunc{
				"full_name": func(values map[string]any) any {
					first, _ := values["first"].(string)
					last, _ := values["last"].(string)
					return first + " " + last
				},
			},
		}),
	)

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values["full_name"] != "John Doe" {
		t.Errorf("expected computed full_name=John Doe, got %v", values["full_name"])
	}
}

func TestSetValues_ValidatesAndSaves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := newTestService(t,
		WithStorePath(path),
		WithGroup(Group{
			Key:   "config",
			Label: "Config",
			Fields: []Field{
				{Key: "name", Type: FieldText, Label: "Name", Validation: &Validation{Required: true}},
			},
		}),
	)

	// Valid save
	errs, err := svc.SetValues(map[string]any{"name": "Alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if errs != nil {
		t.Fatalf("unexpected validation errors: %v", errs)
	}

	// Verify persisted
	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if values["name"] != "Alice" {
		t.Errorf("expected name=Alice after save, got %v", values["name"])
	}
}

func TestSetValues_ReturnsValidationErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := newTestService(t,
		WithStorePath(path),
		WithGroup(Group{
			Key:   "config",
			Label: "Config",
			Fields: []Field{
				{Key: "name", Type: FieldText, Label: "Name", Validation: &Validation{Required: true}},
			},
		}),
	)

	errs, err := svc.SetValues(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 validation error, got %d", len(errs))
	}
	if errs[0].Field != "name" {
		t.Errorf("expected validation error for name, got %s", errs[0].Field)
	}

	// Verify nothing was persisted (file should not exist)
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("expected settings file to not exist after validation failure")
	}
}

func TestSetValues_StripsComputedFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := newTestService(t,
		WithStorePath(path),
		WithGroup(Group{
			Key:   "info",
			Label: "Info",
			Fields: []Field{
				{Key: "first", Type: FieldText, Label: "First"},
				{Key: "display", Type: FieldComputed, Label: "Display"},
			},
			ComputeFuncs: map[string]ComputeFunc{
				"display": func(values map[string]any) any {
					return "computed"
				},
			},
		}),
	)

	_, err := svc.SetValues(map[string]any{"first": "Bob", "display": "should-be-stripped"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Load raw file to verify computed field was not saved
	raw := newTestStore(t, path)
	saved, err := raw.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if _, ok := saved["display"]; ok {
		t.Error("expected computed field 'display' to be stripped from saved data")
	}
	if saved["first"] != "Bob" {
		t.Errorf("expected first=Bob, got %v", saved["first"])
	}
}

func TestWithOnChange_CalledAfterSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	var called bool
	var receivedValues map[string]any

	svc := newTestService(t,
		WithStorePath(path),
		WithGroup(Group{
			Key:   "config",
			Label: "Config",
			Fields: []Field{
				{Key: "key", Type: FieldText, Label: "Key"},
			},
		}),
		WithOnChange(func(values map[string]any) {
			called = true
			receivedValues = values
		}),
	)

	_, err := svc.SetValues(map[string]any{"key": "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected onChange callback to be called")
	}
	if receivedValues["key"] != "value" {
		t.Errorf("expected onChange to receive key=value, got %v", receivedValues["key"])
	}
}

func TestWithOnChange_NotCalledOnValidationFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	var called bool

	svc := newTestService(t,
		WithStorePath(path),
		WithGroup(Group{
			Key:   "config",
			Label: "Config",
			Fields: []Field{
				{Key: "name", Type: FieldText, Label: "Name", Validation: &Validation{Required: true}},
			},
		}),
		WithOnChange(func(values map[string]any) {
			called = true
		}),
	)

	svc.SetValues(map[string]any{})
	if called {
		t.Error("expected onChange NOT to be called on validation failure")
	}
}

func TestMultipleGroups_Compose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := newTestService(t,
		WithStorePath(path),
		WithGroup(Group{
			Key:   "appearance",
			Label: "Appearance",
			Fields: []Field{
				{Key: "theme", Type: FieldSelect, Label: "Theme", Default: "light", Options: themeOptions()},
			},
		}),
		WithGroup(Group{
			Key:   "connection",
			Label: "Connection",
			Fields: []Field{
				{Key: "url", Type: FieldText, Label: "URL", Default: "https://example.com"},
				{Key: "timeout", Type: FieldNumber, Label: "Timeout", Default: float64(30)},
			},
		}),
	)

	schema := svc.GetSchema()
	if len(schema.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(schema.Groups))
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values["theme"] != "light" {
		t.Errorf("expected theme=light, got %v", values["theme"])
	}
	if values["url"] != "https://example.com" {
		t.Errorf("expected url=https://example.com, got %v", values["url"])
	}
	if values["timeout"] != float64(30) {
		t.Errorf("expected timeout=30, got %v", values["timeout"])
	}

	// Save and reload
	_, err = svc.SetValues(map[string]any{
		"theme":   "dark",
		"url":     "https://other.com",
		"timeout": float64(60),
	})
	if err != nil {
		t.Fatalf("save error: %v", err)
	}

	values, err = svc.GetValues()
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}
	if values["theme"] != "dark" {
		t.Errorf("expected theme=dark, got %v", values["theme"])
	}
	if values["timeout"] != float64(60) {
		t.Errorf("expected timeout=60, got %v", values["timeout"])
	}
}

// --- Fix #1: SetValues must merge with existing persisted values ---

func TestSetValues_MergesWithExistingPersistedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := newTestService(t,
		WithStorePath(path),
		WithGroup(Group{
			Key:   "llm",
			Label: "LLM",
			Fields: []Field{
				// No Default: nothing masks the loss if this is clobbered.
				{Key: "llm.model", Type: FieldText, Label: "Model"},
			},
		}),
		WithGroup(Group{
			Key:   "appearance",
			Label: "Appearance",
			Fields: []Field{
				{Key: "appearance.theme", Type: FieldSelect, Label: "Theme", Default: "light", Options: themeOptions()},
			},
		}),
	)

	if _, err := svc.SetValues(map[string]any{"llm.model": "claude-opus-4"}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	// A frontend saving only the appearance group must not erase the rest.
	if _, err := svc.SetValues(map[string]any{"appearance.theme": "dark"}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}
	var onDisk map[string]any
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("unmarshal settings file: %v", err)
	}

	if onDisk["llm.model"] != "claude-opus-4" {
		t.Errorf("expected the previously saved value to survive an unrelated save, on-disk value: %v", onDisk["llm.model"])
	}
	if onDisk["appearance.theme"] != "dark" {
		t.Errorf("expected appearance.theme=dark, got %v", onDisk["appearance.theme"])
	}
}

// The same merge guarantee for a secret, which now lives in the SecretStore
// rather than in settings.json.
func TestSetValues_UnrelatedSaveDoesNotEraseAStoredSecret(t *testing.T) {
	svc, secrets, _ := newSecretService(t)

	if _, err := svc.SetValues(map[string]any{"llm.anthropic.secret": "sk-ant-secret"}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	// A frontend saving only another group must not disturb the secret, and
	// must not need to send the sentinel for a key it never rendered.
	if _, err := svc.SetValues(map[string]any{"llm.model": "claude-opus-4"}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	stored, err := secrets.Get("llm.anthropic.secret")
	if err != nil {
		t.Fatalf("the secret did not survive an unrelated save: %v", err)
	}
	if stored != "sk-ant-secret" {
		t.Errorf("got %q, want %q", stored, "sk-ant-secret")
	}
}

// A guard against the whole suite quietly writing to the developer's real
// Keychain: any Service with a password field must be given a test store.
func TestNoTestUsesTheRealKeyring(t *testing.T) {
	svc, _, _ := newSecretService(t)
	if _, ok := svc.secrets.(*MemorySecretStore); !ok {
		t.Fatalf("the shared secret-service helper must inject a MemorySecretStore, got %T", svc.secrets)
	}
}

func TestSetValues_DoesNotPersistDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := newTestService(t,
		WithStorePath(path),
		WithGroup(Group{
			Key:   "g",
			Label: "G",
			Fields: []Field{
				{Key: "theme", Type: FieldSelect, Label: "Theme", Default: "light", Options: themeOptions()},
				{Key: "name", Type: FieldText, Label: "Name"},
			},
		}),
	)

	if _, err := svc.SetValues(map[string]any{"name": "Alice"}); err != nil {
		t.Fatalf("save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}
	var onDisk map[string]any
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := onDisk["theme"]; ok {
		t.Errorf("merge must not bake schema defaults into the persisted file, got %v", onDisk)
	}
}

// --- Fix #3: Service.mu must actually guard the load-merge-save cycle ---

func TestSetValues_ConcurrentWritesDoNotLoseUpdates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	const n = 20
	fields := make([]Field, 0, n)
	for i := 0; i < n; i++ {
		fields = append(fields, Field{Key: fmt.Sprintf("key%02d", i), Type: FieldText, Label: "K"})
	}

	svc := newTestService(t,
		WithStorePath(path),
		WithGroup(Group{Key: "g", Label: "G", Fields: fields}),
	)

	var wg sync.WaitGroup
	start := make(chan struct{})
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if _, err := svc.SetValues(map[string]any{fmt.Sprintf("key%02d", i): fmt.Sprintf("val%02d", i)}); err != nil {
				errCh <- err
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent SetValues error: %v", err)
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	var missing []string
	for i := 0; i < n; i++ {
		k := fmt.Sprintf("key%02d", i)
		if values[k] != fmt.Sprintf("val%02d", i) {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d of %d concurrent writes were lost: %v", len(missing), n, missing)
	}
}

func TestWithOnChange_CallbackCanReadValuesWithoutDeadlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	var svc *Service
	svc = newTestService(t,
		WithStorePath(path),
		WithGroup(Group{
			Key:    "g",
			Label:  "G",
			Fields: []Field{{Key: "key", Type: FieldText, Label: "Key"}},
		}),
		// Entirely reasonable for a settings framework: react to a change by
		// reading the full current value set.
		WithOnChange(func(values map[string]any) {
			if _, err := svc.GetValues(); err != nil {
				t.Errorf("GetValues from onChange: %v", err)
			}
			svc.GetSchema()
		}),
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := svc.SetValues(map[string]any{"key": "value"}); err != nil {
			t.Errorf("SetValues: %v", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: onChange callback calling GetValues never returned")
	}
}

// --- Fix #6: functional options must be commutative ---

func TestNewService_OptionOrderDoesNotChangeStorePath(t *testing.T) {
	configDir := redirectUserConfigDir(t)

	group := WithGroup(Group{
		Key:    "g",
		Label:  "G",
		Fields: []Field{{Key: "key", Type: FieldText, Label: "Key"}},
	})

	cases := []struct {
		name string
		opts func(path string) []ServiceOption
	}{
		{"path before appName", func(path string) []ServiceOption {
			return []ServiceOption{WithStorePath(path), WithAppName("myapp"), group}
		}},
		{"appName before path", func(path string) []ServiceOption {
			return []ServiceOption{WithAppName("myapp"), WithStorePath(path), group}
		}},
	}

	// One shared path, so both orderings are compared against the same target.
	path := filepath.Join(t.TempDir(), "settings.json")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t, tc.opts(path)...)
			if svc.store.Path() != path {
				t.Fatalf("expected store path %q, got %q", path, svc.store.Path())
			}

			if _, err := svc.SetValues(map[string]any{"key": "value"}); err != nil {
				t.Fatalf("SetValues: %v", err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Errorf("expected settings written to the configured path: %v", err)
			}
			// The discarded-path bug writes to the derived default instead.
			if _, err := os.Stat(filepath.Join(configDir, "myapp")); !os.IsNotExist(err) {
				t.Errorf("expected nothing written under the user config dir, stat err=%v", err)
			}
		})
	}
}

func TestNewService_AppNameStillDerivesDefaultPath(t *testing.T) {
	configDir := redirectUserConfigDir(t)

	svc := newTestService(t, WithAppName("myapp"))
	expected := filepath.Join(configDir, "myapp", "settings.json")
	if svc.store.Path() != expected {
		t.Errorf("expected %q, got %q", expected, svc.store.Path())
	}
}

// --- Fix #7: only schema-declared keys may be persisted ---

func TestSetValues_RejectsKeysNotDeclaredInTheSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := newTestService(t,
		WithStorePath(path),
		WithGroup(Group{
			Key:    "g",
			Label:  "G",
			Fields: []Field{{Key: "known", Type: FieldText, Label: "Known"}},
		}),
	)

	errs, err := svc.SetValues(map[string]any{"known": "ok", "evil.injected": "payload"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 validation error for the undeclared key, got %v", errs)
	}
	if errs[0].Field != "evil.injected" {
		t.Errorf("expected the error to name the offending key, got %q", errs[0].Field)
	}

	// A rejected write must not land, not even partially.
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("expected nothing persisted after rejection, stat err=%v", statErr)
	}
}

func TestSetValues_UndeclaredKeysNeverReachTheConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := newTestService(t,
		WithStorePath(path),
		WithGroup(Group{
			Key:    "g",
			Label:  "G",
			Fields: []Field{{Key: "known", Type: FieldText, Label: "Known"}},
		}),
	)

	if _, err := svc.SetValues(map[string]any{"known": "ok"}); err != nil {
		t.Fatalf("seed save: %v", err)
	}
	if _, err := svc.SetValues(map[string]any{"arbitrary": map[string]any{"junk": true}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var onDisk map[string]any
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := onDisk["arbitrary"]; ok {
		t.Errorf("undeclared key was written to the config file: %v", onDisk)
	}
}

func TestSetValues_ReportsEveryUndeclaredKeyDeterministically(t *testing.T) {
	svc := newTestService(t,
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithGroup(Group{
			Key:    "g",
			Label:  "G",
			Fields: []Field{{Key: "known", Type: FieldText, Label: "Known"}},
		}),
	)

	errs, err := svc.SetValues(map[string]any{"zeta": 1, "alpha": 2, "known": "ok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %v", errs)
	}
	if errs[0].Field != "alpha" || errs[1].Field != "zeta" {
		t.Errorf("expected deterministic (sorted) ordering, got %q then %q", errs[0].Field, errs[1].Field)
	}
}

func TestSetValues_ComputedKeysAreAcceptedAsInput(t *testing.T) {
	// Computed fields are schema-declared: a frontend round-tripping the full
	// value set must not be rejected for echoing them back.
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := newTestService(t,
		WithStorePath(path),
		WithGroup(Group{
			Key:   "info",
			Label: "Info",
			Fields: []Field{
				{Key: "first", Type: FieldText, Label: "First"},
				{Key: "display", Type: FieldComputed, Label: "Display"},
			},
			ComputeFuncs: map[string]ComputeFunc{
				"display": func(values map[string]any) any { return "computed" },
			},
		}),
	)

	errs, err := svc.SetValues(map[string]any{"first": "Bob", "display": "echoed-back"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if errs != nil {
		t.Fatalf("expected computed key to be accepted as input, got %v", errs)
	}
}

// --- Corrupt config recovery (end to end) ---

// The failure mode this guards against: a corrupt settings.json used to make
// every save fail forever, and the only fix was deleting a file a
// non-technical user cannot find. Construction must instead quarantine it,
// report the quarantine, and leave a service that works.
func TestNewService_CorruptConfigIsQuarantinedAndReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	original := []byte(`{"app.theme": "dark", TRUNCATED`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	svc := newTestService(t,
		WithStorePath(path),
		WithSecretStore(NewMemorySecretStore()),
		WithGroup(Group{
			Key:   "app",
			Label: "App",
			Fields: []Field{
				{Key: "app.theme", Type: FieldText, Label: "Theme", Default: "light"},
			},
		}),
	)

	// 1. The caller is informed, without having to call anything else first.
	c := svc.Corruption()
	if c == nil {
		t.Fatal("expected NewService to report the quarantine through Corruption()")
	}
	if c.Path != path {
		t.Errorf("expected Path %q, got %q", path, c.Path)
	}

	// 2. The original bytes are preserved for manual recovery.
	preserved, err := os.ReadFile(c.QuarantinePath)
	if err != nil {
		t.Fatalf("reading %q: %v", c.QuarantinePath, err)
	}
	if !bytes.Equal(preserved, original) {
		t.Errorf("quarantined file was modified: got %q, want %q", preserved, original)
	}

	// 3. Defaults load.
	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if values["app.theme"] != "light" {
		t.Errorf("expected the schema default after reset, got %v", values["app.theme"])
	}

	// 4. Saving works again — the part that was previously an unrecoverable
	//    dead end.
	errs, err := svc.SetValues(map[string]any{"app.theme": "dark"})
	if err != nil {
		t.Fatalf("SetValues after quarantine: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}

	reloaded, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if reloaded["app.theme"] != "dark" {
		t.Errorf("expected the saved value to persist, got %v", reloaded["app.theme"])
	}

	// 5. The same report reaches the frontend, so the UI can show the notice.
	bound := svc.Bindings().Corruption()
	if bound == nil || bound.QuarantinePath != c.QuarantinePath {
		t.Errorf("expected Bindings().Corruption() to mirror the report, got %+v", bound)
	}
}

// A healthy config file must never be reported as corrupt.
func TestNewService_HealthyConfigReportsNoCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"app.theme": "dark"}`), 0600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	svc := newTestService(t,
		WithStorePath(path),
		WithSecretStore(NewMemorySecretStore()),
		WithGroup(Group{
			Key:    "app",
			Label:  "App",
			Fields: []Field{{Key: "app.theme", Type: FieldText, Label: "Theme"}},
		}),
	)

	if c := svc.Corruption(); c != nil {
		t.Errorf("expected no corruption report, got %+v", c)
	}
	if svc.Bindings().Corruption() != nil {
		t.Error("expected Bindings().Corruption() to be nil for a healthy file")
	}
}

// A missing config file is the ordinary first-run state, not corruption.
func TestNewService_MissingConfigIsNotCorruption(t *testing.T) {
	svc := newTestService(t,
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithSecretStore(NewMemorySecretStore()),
	)
	if c := svc.Corruption(); c != nil {
		t.Errorf("expected no corruption report on first run, got %+v", c)
	}
}

// Corruption that appears while the app is running is caught on the next read.
func TestService_CorruptionAfterStartupIsReportedOnNextRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	svc := newTestService(t,
		WithStorePath(path),
		WithSecretStore(NewMemorySecretStore()),
		WithGroup(Group{
			Key:    "app",
			Label:  "App",
			Fields: []Field{{Key: "app.theme", Type: FieldText, Label: "Theme", Default: "light"}},
		}),
	)
	if svc.Corruption() != nil {
		t.Fatal("expected a clean start")
	}

	if err := os.WriteFile(path, []byte("}}garbage"), 0600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	if _, err := svc.GetValues(); err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if svc.Corruption() == nil {
		t.Error("expected corruption appearing after startup to be reported")
	}
}

// --- Validation sees the effective post-save state, not the raw submission ---

// The frontend saves one settings group at a time, so SetValues routinely
// receives a partial map. Validating that map on its own made every Required
// field in every other group look empty, so saving the appearance group failed
// with "Name is required" about a field the user had already filled in and had
// not touched.
func TestSetValues_PartialSaveDoesNotTripRequiredFieldsItDidNotTouch(t *testing.T) {
	svc := newTestService(t,
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithSecretStore(NewMemorySecretStore()),
		WithGroup(Group{Key: "general", Label: "General", Fields: []Field{
			{Key: "general.name", Type: FieldText, Label: "Name", Validation: &Validation{Required: true}},
		}}),
		WithGroup(Group{Key: "appearance", Label: "Appearance", Fields: []Field{
			{Key: "appearance.theme", Type: FieldText, Label: "Theme"},
		}}),
	)

	if errs, err := svc.SetValues(map[string]any{"general.name": "Ryan"}); err != nil || len(errs) > 0 {
		t.Fatalf("seed: errs=%v err=%v", errs, err)
	}

	errs, err := svc.SetValues(map[string]any{"appearance.theme": "dark"})
	if err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("a partial save must not be judged on fields it did not submit, got %v", errs)
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if values["general.name"] != "Ryan" || values["appearance.theme"] != "dark" {
		t.Errorf("expected both groups persisted, got %v", values)
	}
}

// The mirror image: a Condition judged on the partial submission made a visible
// field look hidden, so its rules were skipped entirely and an invalid value
// was saved.
func TestSetValues_ConditionIsJudgedOnTheEffectiveStateNotTheSubmission(t *testing.T) {
	svc := newTestService(t,
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithSecretStore(NewMemorySecretStore()),
		WithGroup(Group{Key: "g", Label: "G", Fields: []Field{
			{Key: "g.mode", Type: FieldSelect, Label: "Mode", Default: "simple",
				Options: []SelectOption{{Label: "Simple", Value: "simple"}, {Label: "Advanced", Value: "advanced"}}},
			{Key: "g.endpoint", Type: FieldText, Label: "Endpoint",
				Condition:  &Condition{Field: "g.mode", Equals: []any{"advanced"}},
				Validation: &Validation{MinLen: 5}},
		}}),
	)

	if errs, err := svc.SetValues(map[string]any{"g.mode": "advanced", "g.endpoint": "https://example.com"}); err != nil || len(errs) > 0 {
		t.Fatalf("seed: errs=%v err=%v", errs, err)
	}

	// g.mode is not in this submission, but it is "advanced" on disk, so the
	// endpoint field is visible and its rules must apply.
	errs, err := svc.SetValues(map[string]any{"g.endpoint": "ab"})
	if err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if len(errs) != 1 || errs[0].Field != "g.endpoint" {
		t.Fatalf("expected the visible field's rules to be enforced, got %v", errs)
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if values["g.endpoint"] != "https://example.com" {
		t.Errorf("a rejected save must not persist, got %v", values["g.endpoint"])
	}
}

// A field hidden by the effective state stays exempt: the user cannot see it,
// so they cannot be asked to fix it.
func TestSetValues_HiddenFieldIsStillExemptFromValidation(t *testing.T) {
	svc := newTestService(t,
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithSecretStore(NewMemorySecretStore()),
		WithGroup(Group{Key: "g", Label: "G", Fields: []Field{
			{Key: "g.mode", Type: FieldSelect, Label: "Mode", Default: "simple",
				Options: []SelectOption{{Label: "Simple", Value: "simple"}, {Label: "Advanced", Value: "advanced"}}},
			{Key: "g.endpoint", Type: FieldText, Label: "Endpoint",
				Condition:  &Condition{Field: "g.mode", Equals: []any{"advanced"}},
				Validation: &Validation{Required: true}},
		}}),
	)

	errs, err := svc.SetValues(map[string]any{"g.mode": "simple"})
	if err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if len(errs) > 0 {
		t.Errorf("a field hidden by its condition must not be required, got %v", errs)
	}
}

// Validation must see recomputed values, not whatever the frontend echoed back
// for a computed field.
func TestSetValues_ValidationSeesRecomputedValues(t *testing.T) {
	svc := newTestService(t,
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithSecretStore(NewMemorySecretStore()),
		WithGroup(Group{
			Key:   "g",
			Label: "G",
			Fields: []Field{
				{Key: "g.mode", Type: FieldText, Label: "Mode"},
				{Key: "g.derived", Type: FieldComputed, Label: "Derived"},
				{Key: "g.gated", Type: FieldText, Label: "Gated",
					Condition:  &Condition{Field: "g.derived", Equals: []any{"on"}},
					Validation: &Validation{MinLen: 5}},
			},
			ComputeFuncs: map[string]ComputeFunc{
				"g.derived": func(values map[string]any) any {
					if values["g.mode"] == "enable" {
						return "on"
					}
					return "off"
				},
			},
		}),
	)

	// A client claiming g.derived is "off" must not thereby switch off the
	// validation gated on it.
	errs, err := svc.SetValues(map[string]any{"g.mode": "enable", "g.derived": "off", "g.gated": "ab"})
	if err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if len(errs) != 1 || errs[0].Field != "g.gated" {
		t.Fatalf("expected validation against the recomputed value, got %v", errs)
	}
}

// An invalid submission must not reach the secret backend either: rejection is
// a complete no-op, not a partial write.
func TestSetValues_InvalidSubmissionNeverWritesASecret(t *testing.T) {
	secrets := NewMemorySecretStore()
	svc := newTestService(t,
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithSecretStore(secrets),
		WithGroup(Group{Key: "g", Label: "G", Fields: []Field{
			{Key: "g.name", Type: FieldText, Label: "Name", Validation: &Validation{Required: true}},
			{Key: "g.key", Type: FieldPassword, Label: "Key"},
		}}),
	)

	errs, err := svc.SetValues(map[string]any{"g.name": "", "g.key": "sk-should-not-be-stored"})
	if err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected the empty required field to be rejected")
	}
	if snap := secrets.Snapshot(); len(snap) != 0 {
		t.Errorf("a rejected submission must not write to the secret store, got %v", snap)
	}
}
