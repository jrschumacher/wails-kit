package kit

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
	"github.com/jrschumacher/wails-kit/v2/firstrun"
	"github.com/jrschumacher/wails-kit/v2/health"
	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/updates"
)

// testInfo returns a minimal, valid AppInfo for tests.
func testInfo() AppInfo {
	return AppInfo{Name: "kit-test-app", ID: "dev.wails-kit.kit-test", Version: "1.0.0"}
}

// testDirs builds an appdirs.Dirs rooted entirely under t.TempDir(), so
// tests never touch the real OS config/data/cache/log directories.
func testDirs(t *testing.T) *appdirs.Dirs {
	t.Helper()
	root := t.TempDir()
	return appdirs.New("kit-test-app",
		appdirs.WithConfigDir(filepath.Join(root, "config")),
		appdirs.WithDataDir(filepath.Join(root, "data")),
		appdirs.WithCacheDir(filepath.Join(root, "cache")),
		appdirs.WithLogDir(filepath.Join(root, "log")),
		appdirs.WithTempDir(filepath.Join(root, "temp")),
	)
}

// baseOpts is what every test needs regardless of what else it configures: a
// hermetic Dirs, a non-OS keyring (see keyring/AGENTS.md — no test may
// construct a real OSStore), and no real network request from health's
// built-in default connectivity check (see health/AGENTS.md).
func baseOpts(t *testing.T) []Option {
	t.Helper()
	return []Option{
		WithDirs(testDirs(t)),
		WithKeyring(keyring.NewMemoryStore()),
		WithHealthOptions(health.WithoutDefaultConnectivityCheck()),
	}
}

func newTestKit(t *testing.T, opts ...Option) *Kit {
	t.Helper()
	all := append(baseOpts(t), opts...)
	k, err := New(testInfo(), all...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return k
}

// TestNewDefaultWiring covers the "default wiring succeeding" requirement:
// every field is populated, every on-by-default component is present, and
// Updates stays nil because no WithGitHubRepo was given.
func TestNewDefaultWiring(t *testing.T) {
	k := newTestKit(t)

	if k.Info.Name != "kit-test-app" {
		t.Errorf("Info.Name = %q, want %q", k.Info.Name, "kit-test-app")
	}
	if k.Dirs == nil {
		t.Error("Dirs is nil")
	}
	if k.Logger == nil {
		t.Error("Logger is nil")
	}
	if k.Events == nil {
		t.Error("Events is nil")
	}
	if k.Keyring == nil {
		t.Error("Keyring is nil")
	}
	if k.Settings == nil {
		t.Error("Settings is nil")
	}
	if k.I18n == nil {
		t.Error("I18n is nil")
	}
	if k.Appearance == nil {
		t.Error("Appearance is nil")
	}
	if k.Health == nil {
		t.Error("Health is nil (should be on by default)")
	}
	if k.Updates != nil {
		t.Error("Updates is non-nil (should be off without WithGitHubRepo)")
	}
	if k.FirstRun == nil {
		t.Error("FirstRun is nil")
	}
	if k.Diag == nil {
		t.Error("Diag is nil (should be on by default)")
	}
	if k.Lifecycle == nil {
		t.Error("Lifecycle is nil")
	}

	// The schema should carry the kit-internal groups that are always on:
	// locale (i18n), appearance, health, diagnostics. No updates group
	// without WithGitHubRepo.
	schema := k.Settings.GetSchema()
	wantGroups := map[string]bool{"i18n": false, "appearance": false, "health": false, "diagnostics": false}
	for _, g := range schema.Groups {
		if _, ok := wantGroups[g.Key]; ok {
			wantGroups[g.Key] = true
		}
		if g.Key == "updates" {
			t.Error("updates settings group present without WithGitHubRepo")
		}
	}
	for key, found := range wantGroups {
		if !found {
			t.Errorf("settings group %q missing from default schema", key)
		}
	}
}

// TestNewRequiresName covers the one structural validation New does itself
// (everything else is delegated to the component being constructed).
func TestNewRequiresName(t *testing.T) {
	_, err := New(AppInfo{Version: "1.0.0"}, baseOpts(t)...)
	if err == nil {
		t.Fatal("expected error for empty AppInfo.Name")
	}
	if comp, ok := GetComponent(err); !ok || comp != "kit" {
		t.Errorf("GetComponent(err) = (%q, %v), want (\"kit\", true)", comp, ok)
	}
}

// TestWithoutHealth covers one of the "each opt-out actually disabling its
// component" cases.
func TestWithoutHealth(t *testing.T) {
	k := newTestKit(t, WithoutHealth())

	if k.Health != nil {
		t.Error("Health is non-nil despite WithoutHealth")
	}
	for _, g := range k.Settings.GetSchema().Groups {
		if g.Key == "health" {
			t.Error("health settings group present despite WithoutHealth")
		}
	}
}

// TestWithoutDiagnostics covers another opt-out.
func TestWithoutDiagnostics(t *testing.T) {
	k := newTestKit(t, WithoutDiagnostics())

	if k.Diag != nil {
		t.Error("Diag is non-nil despite WithoutDiagnostics")
	}
	for _, g := range k.Settings.GetSchema().Groups {
		if g.Key == "diagnostics" {
			t.Error("diagnostics settings group present despite WithoutDiagnostics")
		}
	}
}

// TestWithoutUpdates covers the explicit-override opt-out: even with a
// GitHub repo configured, WithoutUpdates wins.
func TestWithoutUpdates(t *testing.T) {
	k := newTestKit(t, WithGitHubRepo("acme", "widget"), WithoutUpdates())

	if k.Updates != nil {
		t.Error("Updates is non-nil despite WithoutUpdates")
	}
	for _, g := range k.Settings.GetSchema().Groups {
		if g.Key == "updates" {
			t.Error("updates settings group present despite WithoutUpdates")
		}
	}
}

// TestGitHubRepoEnablesUpdates is the positive case: WithGitHubRepo alone
// turns Updates on and registers its settings group.
func TestGitHubRepoEnablesUpdates(t *testing.T) {
	k := newTestKit(t, WithGitHubRepo("acme", "widget"))

	if k.Updates == nil {
		t.Fatal("Updates is nil despite WithGitHubRepo")
	}
	if k.Updates.GetCurrentVersion() == "" {
		t.Error("Updates current version is empty")
	}

	found := false
	for _, g := range k.Settings.GetSchema().Groups {
		if g.Key == "updates" {
			found = true
		}
	}
	if !found {
		t.Error("updates settings group missing despite WithGitHubRepo")
	}
}

// TestConstructionFailureNamesComponent covers the "construction failure
// naming the responsible component" requirement. An invalid semver version
// makes firstrun.New fail; New must surface that as component "firstrun",
// not an opaque wrapped string.
func TestConstructionFailureNamesComponent(t *testing.T) {
	info := AppInfo{Name: "bad-version-app", Version: "not-a-version"}
	_, err := New(info, baseOpts(t)...)
	if err == nil {
		t.Fatal("expected error for invalid AppInfo.Version")
	}
	comp, ok := GetComponent(err)
	if !ok {
		t.Fatalf("GetComponent(err) ok = false, err = %v", err)
	}
	if comp != "firstrun" {
		t.Errorf("GetComponent(err) = %q, want %q", comp, "firstrun")
	}
}

// TestConstructionFailureNamesUpdatesComponent is a second, independent
// case: updates.NewService itself rejects a malformed public key (passed
// through via WithUpdatesOptions), and New must attribute that to
// "updates" — not "firstrun" or a bare wrapped string.
func TestConstructionFailureNamesUpdatesComponent(t *testing.T) {
	opts := append(baseOpts(t),
		WithGitHubRepo("acme", "widget"),
		WithUpdatesOptions(updates.WithPublicKey("not-a-valid-minisign-key")),
	)
	_, err := New(testInfo(), opts...)
	if err == nil {
		t.Fatal("expected error for malformed updates public key")
	}
	comp, ok := GetComponent(err)
	if !ok || comp != "updates" {
		t.Errorf("GetComponent(err) = (%q, %v), want (\"updates\", true)", comp, ok)
	}
}

// TestStartRunsFirstRunAndLifecycle exercises Start end-to-end: firstrun's
// hook fires exactly once (a Fresh install, since the stamp store is a
// fresh temp dir), and the lifecycle-managed health ticker actually starts
// and probes a registered check (Snapshot moves out of the initial Unknown
// state). The built-in default connectivity check is disabled (baseOpts) so
// this never touches the real network; a fake, always-healthy check stands
// in for "some check exists".
func TestStartRunsFirstRunAndLifecycle(t *testing.T) {
	var hookRan int
	hook := firstrun.Hook{
		Name: "test-hook",
		When: firstrun.OnFresh(),
		Run: func(context.Context, firstrun.Info) error {
			hookRan++
			return nil
		},
	}

	k := newTestKit(t,
		WithFirstRunHook(hook),
		WithHealthCheck(health.Check{
			Name:     "fake",
			Class:    health.ClassBackend,
			Critical: true,
			Interval: 20 * time.Millisecond,
			Probe:    health.ProbeFunc(func(context.Context) error { return nil }),
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := k.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := k.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	if hookRan != 1 {
		t.Errorf("firstrun hook ran %d times, want 1", hookRan)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := k.Health.Snapshot()
		if snap.Overall == health.StateHealthy {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("health snapshot never reported healthy after Start")
}

// TestCloseWithoutStart covers the documented "safe even if Start was never
// called" contract.
func TestCloseWithoutStart(t *testing.T) {
	k := newTestKit(t)
	if err := k.Close(); err != nil {
		t.Errorf("Close without Start: %v", err)
	}
}

// TestNormalizeVersion covers the dev-build case. "dev" is the conventional
// default for an un-ldflagged Go binary, and firstrun requires parseable
// semver — so without this, kit.New fails for every `go run` and `go test`,
// which is exactly where it must not fail. A genuinely malformed version is
// still passed through to fail loudly.
func TestNormalizeVersion(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", "0.0.0-dev"},
		{"dev", "0.0.0-dev"},
		{"1.4.0", "1.4.0"},
		{"v1.4.0", "v1.4.0"},
		{"nonsense", "nonsense"},
	} {
		if got := normalizeVersion(tc.in); got != tc.want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
