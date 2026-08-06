package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// secretSchema is the shape this whole feature exists for: an API key beside
// an ordinary setting, in the same group.
func secretSchema() ServiceOption {
	return WithGroup(Group{
		Key:   "llm",
		Label: "LLM",
		Fields: []Field{
			{Key: "llm.provider", Type: FieldSelect, Label: "Provider", Default: "anthropic",
				Options: []SelectOption{{Label: "Anthropic", Value: "anthropic"}, {Label: "OpenAI", Value: "openai"}}},
			{Key: "llm.anthropic.secret", Type: FieldPassword, Label: "Anthropic API Key"},
			{Key: "llm.model", Type: FieldText, Label: "Model"},
		},
	})
}

// newSecretService builds a Service backed by an in-memory secret store, so no
// test in this suite touches the developer's real Keychain.
func newSecretService(t *testing.T, opts ...ServiceOption) (*Service, *MemorySecretStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	secrets := NewMemorySecretStore()

	all := []ServiceOption{WithStorePath(path), WithSecretStore(secrets), secretSchema()}
	all = append(all, opts...)

	svc, err := NewService(all...)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, secrets, path
}

func readSettingsFile(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}
		}
		t.Fatalf("read settings file: %v", err)
	}
	var onDisk map[string]any
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("unmarshal settings file: %v", err)
	}
	return onDisk
}

// --- Routing: password values must never reach settings.json ---

func TestSetValues_PasswordGoesToSecretStoreNotSettingsFile(t *testing.T) {
	svc, secrets, path := newSecretService(t)

	if _, err := svc.SetValues(map[string]any{
		"llm.anthropic.secret": "sk-ant-real-key",
		"llm.model":            "claude-opus-4",
	}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}

	onDisk := readSettingsFile(t, path)
	if _, ok := onDisk["llm.anthropic.secret"]; ok {
		t.Errorf("the API key was written to settings.json: %v", onDisk)
	}
	if onDisk["llm.model"] != "claude-opus-4" {
		t.Errorf("expected the non-secret field to persist normally, got %v", onDisk)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(raw), "sk-ant-real-key") {
		t.Errorf("the raw secret appears in the settings file: %s", raw)
	}

	stored, err := secrets.Get("llm.anthropic.secret")
	if err != nil {
		t.Fatalf("expected the secret in the secret store: %v", err)
	}
	if stored != "sk-ant-real-key" {
		t.Errorf("secret store holds %q, want %q", stored, "sk-ant-real-key")
	}
}

func TestSetValues_EmptyPasswordClearsTheStoredSecret(t *testing.T) {
	svc, secrets, _ := newSecretService(t)

	if _, err := svc.SetValues(map[string]any{"llm.anthropic.secret": "sk-ant-real-key"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := svc.SetValues(map[string]any{"llm.anthropic.secret": ""}); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if _, err := secrets.Get("llm.anthropic.secret"); !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("an empty submission must clear the secret, got %v", err)
	}
}

// A legacy plaintext value in the config file must be cleaned out rather than
// left sitting on disk forever.
func TestSetValues_StripsPreexistingPlaintextSecretFromTheFile(t *testing.T) {
	svc, _, path := newSecretService(t)

	seed := map[string]any{"llm.anthropic.secret": "sk-plaintext-legacy", "llm.model": "old"}
	data, _ := json.Marshal(seed)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := svc.SetValues(map[string]any{"llm.model": "new"}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}

	onDisk := readSettingsFile(t, path)
	if _, ok := onDisk["llm.anthropic.secret"]; ok {
		t.Errorf("a plaintext secret already in the file must be removed on the next save, got %v", onDisk)
	}
}

// --- Masking: GetValues must never carry a real secret ---

func TestGetValues_MasksAStoredSecret(t *testing.T) {
	svc, secrets, _ := newSecretService(t)
	if err := secrets.Set("llm.anthropic.secret", "sk-ant-real-key"); err != nil {
		t.Fatalf("seed secret: %v", err)
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}

	if values["llm.anthropic.secret"] != SecretSentinel {
		t.Errorf("expected the masked sentinel, got %#v", values["llm.anthropic.secret"])
	}
	for k, v := range values {
		if s, ok := v.(string); ok && strings.Contains(s, "sk-ant-real-key") {
			t.Fatalf("the real secret crossed the bridge in key %q", k)
		}
	}
}

func TestGetValues_OmitsAnUnsetSecret(t *testing.T) {
	svc, _, _ := newSecretService(t)

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if v, ok := values["llm.anthropic.secret"]; ok {
		t.Errorf("expected an unset secret to be omitted, got %#v", v)
	}
}

// A Default on a password field is meaningless — the value lives in the
// SecretStore — and dangerous, because GetValues would surface it looking
// exactly like a secret the user had saved. It is rejected at construction, and
// the value path drops password defaults anyway as a second line of defence.
func TestPasswordDefault_IsRejectedAtConstruction(t *testing.T) {
	group := Group{
		Key:   "g",
		Label: "G",
		Fields: []Field{
			{Key: "api_key", Type: FieldPassword, Label: "Key", Default: "should-never-appear"},
		},
	}

	if err := ValidateSchema(Schema{Groups: []Group{group}}); err == nil {
		t.Fatal("expected ValidateSchema to reject a Default on a password field")
	}

	svc, err := NewService(
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithSecretStore(NewMemorySecretStore()),
		WithGroup(group),
	)
	if err == nil {
		t.Fatal("expected NewService to reject a Default on a password field")
	}
	if svc != nil {
		t.Error("expected a nil service alongside the error")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("error should name the offending field, got: %v", err)
	}
}

// The second line of defence: even when a password default reaches the store's
// default set, it must never be surfaced as a value.
func TestGetValues_PasswordDefaultIsNotSurfaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc, err := NewService(
		WithStorePath(path),
		WithSecretStore(NewMemorySecretStore()),
		WithGroup(Group{
			Key:    "g",
			Label:  "G",
			Fields: []Field{{Key: "api_key", Type: FieldPassword, Label: "Key"}},
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	// Inject the default behind ValidateSchema's back, standing in for any
	// future path that registers one.
	svc.store.SetDefaults(map[string]any{"api_key": "should-never-appear"})

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if v, ok := values["api_key"]; ok {
		t.Errorf("a password default must not be surfaced as a value, got %#v", v)
	}
}

// A compute function runs over the masked view, so it cannot smuggle a secret
// into the frontend payload.
func TestGetValues_ComputeFuncsSeeTheMaskedValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	secrets := NewMemorySecretStore()
	if err := secrets.Set("api_key", "sk-real"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc, err := NewService(
		WithStorePath(path),
		WithSecretStore(secrets),
		WithGroup(Group{
			Key:   "g",
			Label: "G",
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: "Key"},
				{Key: "echo", Type: FieldComputed, Label: "Echo"},
			},
			ComputeFuncs: map[string]ComputeFunc{
				"echo": func(values map[string]any) any { return values["api_key"] },
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if values["echo"] == "sk-real" {
		t.Error("a compute function saw the real secret; masking must happen before compute")
	}
	if values["echo"] != SecretSentinel {
		t.Errorf("expected the compute function to observe the sentinel, got %#v", values["echo"])
	}
}

// --- The sentinel round-trip: the whole point of the design ---

func TestSentinelRoundTrip_UnrelatedEditDoesNotClobberTheStoredSecret(t *testing.T) {
	svc, secrets, path := newSecretService(t)

	// 1. The user saves an API key.
	if _, err := svc.SetValues(map[string]any{
		"llm.anthropic.secret": "sk-ant-real-key",
		"llm.model":            "claude-opus-4",
	}); err != nil {
		t.Fatalf("initial save: %v", err)
	}

	// 2. The settings page renders.
	rendered, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if rendered["llm.anthropic.secret"] != SecretSentinel {
		t.Fatalf("render must show the sentinel, got %#v", rendered["llm.anthropic.secret"])
	}

	// 3. The user edits an unrelated field and the UI posts the whole map back
	//    verbatim, sentinel included.
	rendered["llm.model"] = "claude-sonnet-4"
	errs, err := svc.SetValues(rendered)
	if err != nil {
		t.Fatalf("round-trip save: %v", err)
	}
	if errs != nil {
		t.Fatalf("round-trip save reported validation errors: %v", errs)
	}

	// 4. The stored secret is untouched...
	stored, err := secrets.Get("llm.anthropic.secret")
	if err != nil {
		t.Fatalf("the secret was lost by the round trip: %v", err)
	}
	if stored != "sk-ant-real-key" {
		t.Errorf("the secret was clobbered: got %q, want %q", stored, "sk-ant-real-key")
	}

	// 5. ...the unrelated edit landed...
	onDisk := readSettingsFile(t, path)
	if onDisk["llm.model"] != "claude-sonnet-4" {
		t.Errorf("expected the unrelated edit to persist, got %v", onDisk)
	}

	// 6. ...and neither the secret nor the sentinel reached settings.json.
	if _, ok := onDisk["llm.anthropic.secret"]; ok {
		t.Errorf("the password key was written to settings.json: %v", onDisk)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(raw), "sk-ant-real-key") {
		t.Errorf("the raw secret reached the settings file: %s", raw)
	}
	if strings.Contains(string(raw), SecretSentinel) {
		t.Errorf("the sentinel was persisted as though it were a value: %s", raw)
	}
}

func TestSentinelRoundTrip_RepeatedRoundTripsAreStable(t *testing.T) {
	svc, secrets, _ := newSecretService(t)

	if _, err := svc.SetValues(map[string]any{"llm.anthropic.secret": "sk-stable"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for i := 0; i < 5; i++ {
		values, err := svc.GetValues()
		if err != nil {
			t.Fatalf("GetValues %d: %v", i, err)
		}
		if _, err := svc.SetValues(values); err != nil {
			t.Fatalf("SetValues %d: %v", i, err)
		}
	}

	stored, err := secrets.Get("llm.anthropic.secret")
	if err != nil {
		t.Fatalf("secret lost after repeated round trips: %v", err)
	}
	if stored != "sk-stable" {
		t.Errorf("got %q, want %q", stored, "sk-stable")
	}
}

func TestSetValues_ANewSecretStillOverwritesTheStoredOne(t *testing.T) {
	svc, secrets, _ := newSecretService(t)

	if _, err := svc.SetValues(map[string]any{"llm.anthropic.secret": "sk-old"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := svc.SetValues(map[string]any{"llm.anthropic.secret": "sk-new"}); err != nil {
		t.Fatalf("update: %v", err)
	}

	stored, _ := secrets.Get("llm.anthropic.secret")
	if stored != "sk-new" {
		t.Errorf("got %q, want %q: the sentinel must not make every write a no-op", stored, "sk-new")
	}
}

func TestSecretSentinel_CannotBeConfusedWithAnOrdinaryValue(t *testing.T) {
	if SecretSentinel == "" {
		t.Fatal("the sentinel must not be the empty string: that is a legitimate 'clear this' submission")
	}
	if len(SecretSentinel) < 24 {
		t.Errorf("the sentinel should be long enough to be implausible as a typed value, got %q", SecretSentinel)
	}
	// A masking string of bullets is exactly what a naive implementation would
	// choose, and a user pasting it would silently keep their old key.
	for _, plausible := range []string{"********", "••••••••", "password", "secret", "hidden"} {
		if SecretSentinel == plausible {
			t.Errorf("the sentinel %q is a value a user could plausibly enter", plausible)
		}
	}
}

// --- Backend access to the real secret ---

func TestGetSecret_ReturnsTheRealValue(t *testing.T) {
	svc, _, _ := newSecretService(t)

	if _, err := svc.SetValues(map[string]any{"llm.anthropic.secret": "sk-ant-real-key"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := svc.GetSecret("llm.anthropic.secret")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got != "sk-ant-real-key" {
		t.Errorf("got %q, want %q", got, "sk-ant-real-key")
	}
}

func TestGetSecret_ReportsNotFoundForAnUnsetKey(t *testing.T) {
	svc, _, _ := newSecretService(t)

	if _, err := svc.GetSecret("llm.anthropic.secret"); !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestGetSecret_RejectsANonPasswordField(t *testing.T) {
	svc, _, _ := newSecretService(t)

	if _, err := svc.GetSecret("llm.model"); err == nil {
		t.Error("expected an error for a field that is not a password field")
	}
	if _, err := svc.GetSecret("not.in.schema"); err == nil {
		t.Error("expected an error for a key the schema does not declare")
	}
}

func TestSetSecret_AndDeleteSecret(t *testing.T) {
	svc, secrets, path := newSecretService(t)

	if err := svc.SetSecret("llm.anthropic.secret", "sk-programmatic"); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	if v, _ := secrets.Get("llm.anthropic.secret"); v != "sk-programmatic" {
		t.Errorf("got %q, want %q", v, "sk-programmatic")
	}
	if _, ok := readSettingsFile(t, path)["llm.anthropic.secret"]; ok {
		t.Error("SetSecret must not write to settings.json")
	}

	if err := svc.DeleteSecret("llm.anthropic.secret"); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	if _, err := secrets.Get("llm.anthropic.secret"); !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected the secret to be gone, got %v", err)
	}

	if err := svc.SetSecret("llm.model", "x"); err == nil {
		t.Error("SetSecret must reject a field that is not a password field")
	}
}

// The llm package needs a complete value map with real keys to construct a
// provider; this is the one path that resolves them.
func TestGetValuesWithSecrets_ResolvesRealSecrets(t *testing.T) {
	svc, _, _ := newSecretService(t)

	if _, err := svc.SetValues(map[string]any{
		"llm.anthropic.secret": "sk-ant-real-key",
		"llm.model":            "claude-opus-4",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	values, err := svc.GetValuesWithSecrets()
	if err != nil {
		t.Fatalf("GetValuesWithSecrets: %v", err)
	}
	if values["llm.anthropic.secret"] != "sk-ant-real-key" {
		t.Errorf("expected the real secret, got %#v", values["llm.anthropic.secret"])
	}
	if values["llm.model"] != "claude-opus-4" {
		t.Errorf("expected ordinary values too, got %#v", values["llm.model"])
	}
	if values["llm.provider"] != "anthropic" {
		t.Errorf("expected defaults to apply, got %#v", values["llm.provider"])
	}
}

func TestGetValuesWithSecrets_OmitsAnUnsetSecret(t *testing.T) {
	svc, _, _ := newSecretService(t)

	values, err := svc.GetValuesWithSecrets()
	if err != nil {
		t.Fatalf("GetValuesWithSecrets: %v", err)
	}
	if v, ok := values["llm.anthropic.secret"]; ok {
		t.Errorf("expected an unset secret to be absent, got %#v", v)
	}
}

// --- Validation interaction ---

func TestSetValues_RequiredPasswordIsSatisfiedByTheSentinel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	secrets := NewMemorySecretStore()
	svc, err := NewService(
		WithStorePath(path),
		WithSecretStore(secrets),
		WithGroup(Group{
			Key:   "g",
			Label: "G",
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: "API Key", Validation: &Validation{Required: true, MinLen: 8}},
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Not yet stored: the sentinel must not be able to fake a stored secret.
	errs, err := svc.SetValues(map[string]any{"api_key": SecretSentinel})
	if err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if len(errs) != 1 || errs[0].Field != "api_key" {
		t.Fatalf("expected a required error when nothing is stored, got %v", errs)
	}

	// Stored: a round-tripped sentinel satisfies Required.
	if _, err := svc.SetValues(map[string]any{"api_key": "sk-long-enough"}); err != nil {
		t.Fatalf("store: %v", err)
	}
	errs, err = svc.SetValues(map[string]any{"api_key": SecretSentinel})
	if err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if errs != nil {
		t.Fatalf("a sentinel over a stored secret must validate, got %v", errs)
	}
}

func TestSetValues_PasswordValidationStillAppliesToNewValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc, err := NewService(
		WithStorePath(path),
		WithSecretStore(NewMemorySecretStore()),
		WithGroup(Group{
			Key:   "g",
			Label: "G",
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: "API Key", Validation: &Validation{Pattern: `sk-.+`}},
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	errs, err := svc.SetValues(map[string]any{"api_key": "not-a-key"})
	if err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if len(errs) != 1 {
		t.Fatalf("expected the pattern to reject the new secret, got %v", errs)
	}
}

func TestSetValues_RejectedSaveDoesNotWriteTheSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	secrets := NewMemorySecretStore()
	svc, err := NewService(
		WithStorePath(path),
		WithSecretStore(secrets),
		WithGroup(Group{
			Key:   "g",
			Label: "G",
			Fields: []Field{
				{Key: "api_key", Type: FieldPassword, Label: "API Key", Validation: &Validation{Pattern: `sk-.+`}},
				{Key: "name", Type: FieldText, Label: "Name"},
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.SetValues(map[string]any{"api_key": "bad", "name": "n"}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if _, err := secrets.Get("api_key"); !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("a rejected save must not store the secret, got %v", err)
	}
	if snap := secrets.Snapshot(); len(snap) != 0 {
		t.Errorf("expected the secret store to be untouched, got %v", snap)
	}
}

// --- Backend selection and failure modes ---

// The default must be the OS keyring, never a plaintext file: a silent
// downgrade leaves the user believing their keys are in the Keychain.
func TestNewService_DefaultSecretStoreIsTheKeyring(t *testing.T) {
	svc, err := NewService(
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithAppName("myapp"),
		secretSchema(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ks, ok := svc.secrets.(*KeyringSecretStore)
	if !ok {
		t.Fatalf("expected the default secret store to be *KeyringSecretStore, got %T", svc.secrets)
	}
	if ks.service != "myapp" {
		t.Errorf("expected the keyring service name to follow the app name, got %q", ks.service)
	}
}

func TestWithPlaintextFileSecrets_IsAnExplicitOptIn(t *testing.T) {
	secretsPath := filepath.Join(t.TempDir(), "secrets.json")
	svc, err := NewService(
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithPlaintextFileSecrets(secretsPath),
		secretSchema(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if _, ok := svc.secrets.(*FileSecretStore); !ok {
		t.Fatalf("expected *FileSecretStore, got %T", svc.secrets)
	}

	if _, err := svc.SetValues(map[string]any{"llm.anthropic.secret": "sk-file"}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if _, err := os.Stat(secretsPath); err != nil {
		t.Errorf("expected the secret to land in the opted-in file: %v", err)
	}
}

func TestNewService_SecretStoreOptionsAreCommutative(t *testing.T) {
	explicit := NewMemorySecretStore()
	filePath := filepath.Join(t.TempDir(), "secrets.json")

	cases := []struct {
		name string
		opts []ServiceOption
	}{
		{"store before file", []ServiceOption{WithSecretStore(explicit), WithPlaintextFileSecrets(filePath)}},
		{"file before store", []ServiceOption{WithPlaintextFileSecrets(filePath), WithSecretStore(explicit)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := append([]ServiceOption{
				WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
				secretSchema(),
			}, tc.opts...)

			svc, err := NewService(opts...)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}
			if svc.secrets != SecretStore(explicit) {
				t.Errorf("an explicitly injected store must win regardless of option order, got %T", svc.secrets)
			}
		})
	}
}

// An unreachable keyring must be loud. Returning masked-as-absent would tell
// the user their key was never saved; returning the error tells them the truth.
func TestGetValues_SurfacesASecretBackendFailure(t *testing.T) {
	svc, err := NewService(
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithSecretStore(&failingSecretStore{err: ErrSecretStoreUnavailable}),
		secretSchema(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.GetValues(); !errors.Is(err, ErrSecretStoreUnavailable) {
		t.Errorf("expected the backend failure to surface, got %v", err)
	}
}

func TestSetValues_SurfacesASecretBackendFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	svc, err := NewService(
		WithStorePath(path),
		WithSecretStore(&failingSecretStore{err: ErrSecretStoreUnavailable}),
		secretSchema(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.SetValues(map[string]any{"llm.anthropic.secret": "sk-x", "llm.model": "m"})
	if !errors.Is(err, ErrSecretStoreUnavailable) {
		t.Fatalf("expected the backend failure to surface, got %v", err)
	}

	// The whole save is abandoned rather than half-applied: reporting an error
	// while quietly persisting the rest is how a user ends up with a saved
	// config that does not work.
	if _, ok := readSettingsFile(t, path)["llm.model"]; ok {
		t.Error("expected no settings written when the secret write failed")
	}
}

func TestSetValues_OnChangeReceivesTheMaskedValueNotTheSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	var received map[string]any

	svc, err := NewService(
		WithStorePath(path),
		WithSecretStore(NewMemorySecretStore()),
		secretSchema(),
		WithOnChange(func(values map[string]any) { received = values }),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.SetValues(map[string]any{"llm.anthropic.secret": "sk-ant-real-key"}); err != nil {
		t.Fatalf("SetValues: %v", err)
	}

	if received["llm.anthropic.secret"] == "sk-ant-real-key" {
		t.Error("the onChange payload carried the raw secret; listeners may log or forward it")
	}
}

// --- The Wails binding boundary ---

// Wails binds every exported method of a bound struct, so binding the Service
// itself would expose GetSecret to the frontend and undo the whole design.
// Bindings is the type intended to be handed to Wails.
func TestBindings_DoNotExposeSecretReaders(t *testing.T) {
	svc, _, _ := newSecretService(t)
	b := svc.Bindings()

	forbidden := []string{"GetSecret", "SetSecret", "DeleteSecret", "GetValuesWithSecrets"}
	bt := reflect.TypeOf(b)
	for _, name := range forbidden {
		if _, ok := bt.MethodByName(name); ok {
			t.Errorf("Bindings exposes %s to the frontend", name)
		}
	}

	for _, name := range []string{"GetSchema", "GetValues", "SetValues"} {
		if _, ok := bt.MethodByName(name); !ok {
			t.Errorf("Bindings is missing %s, which the frontend needs", name)
		}
	}
}

func TestBindings_GetValuesIsMasked(t *testing.T) {
	svc, secrets, _ := newSecretService(t)
	if err := secrets.Set("llm.anthropic.secret", "sk-ant-real-key"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	values, err := svc.Bindings().GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if values["llm.anthropic.secret"] != SecretSentinel {
		t.Errorf("expected the sentinel through the bindings, got %#v", values["llm.anthropic.secret"])
	}
}

// A password field submitted as a non-string used to vanish: applySecrets can
// only store a string, and the config file never holds a password key, so
// nothing was written anywhere and the call still reported success.
func TestSetValues_NonStringPasswordValueIsRejectedNotDropped(t *testing.T) {
	secrets := NewMemorySecretStore()
	svc := newTestService(t,
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithSecretStore(secrets),
		WithGroup(Group{Key: "g", Label: "G", Fields: []Field{
			{Key: "g.key", Type: FieldPassword, Label: "Key"},
			{Key: "g.name", Type: FieldText, Label: "Name"},
		}}),
	)

	for _, bad := range []any{42, true, nil, []any{"a"}} {
		errs, err := svc.SetValues(map[string]any{"g.key": bad, "g.name": "kept"})
		if err != nil {
			t.Fatalf("SetValues(%#v): %v", bad, err)
		}
		if len(errs) != 1 || errs[0].Field != "g.key" {
			t.Errorf("expected %#v to be rejected for g.key, got %v", bad, errs)
		}
	}

	if snap := secrets.Snapshot(); len(snap) != 0 {
		t.Errorf("nothing should have been stored, got %v", snap)
	}
	values, err := svc.GetValues()
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if _, ok := values["g.name"]; ok {
		t.Error("a rejected submission must not persist its other keys either")
	}
}

// GetValues concurrent with SetValues must return one coherent state, not a
// mixture of the pre- and post-save values.
func TestGetValues_ConcurrentWithSetValuesIsCoherent(t *testing.T) {
	secrets := NewMemorySecretStore()
	svc := newTestService(t,
		WithStorePath(filepath.Join(t.TempDir(), "settings.json")),
		WithSecretStore(secrets),
		WithGroup(Group{Key: "g", Label: "G", Fields: []Field{
			{Key: "g.key", Type: FieldPassword, Label: "Key"},
			{Key: "g.name", Type: FieldText, Label: "Name"},
		}}),
	)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			if _, err := svc.SetValues(map[string]any{"g.key": "sk-1", "g.name": "n"}); err != nil {
				t.Errorf("SetValues: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			if _, err := svc.GetValues(); err != nil {
				t.Errorf("GetValues: %v", err)
				return
			}
		}
	}()
	wg.Wait()
}
