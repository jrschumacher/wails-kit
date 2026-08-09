// Command kit-headless-example demonstrates kit.New/Start/Close the way a
// Cobra CLI or Bubbletea TUI entry point would use them: no
// *application.App, no window, no import of anything under
// github.com/wailsapp/wails anywhere in this binary's dependency graph
// (verify with `go list -deps ./examples/kit-headless/ | grep wailsapp` —
// it prints nothing). This is the whole point of AD-1 (docs/v2-roadmap.md):
// Prune's CLI and TUI need the identical wired spine a GUI entry point
// gets, minus the GUI.
//
// It walks through: constructing a Kit with a couple of components
// customized, reading/writing settings (including a password field, to
// show the keyring round-trip and masking), Start (which runs firstrun
// hooks and starts the health ticker), a manual health check, retargeting
// the appearance Source the way kit/wailsbridge (WP-31) will, and Close.
//
// It uses a throwaway temp directory and an in-memory keyring, so this
// example never touches a real config directory or OS keychain — safe to
// run repeatedly and safe to run in CI.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
	"github.com/jrschumacher/wails-kit/v2/health"
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/kit"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

// apiKeyLabel and apiKeyGroup demonstrate an app registering its own
// settings group alongside the kit's built-in ones — this is what
// kit.WithSettingsGroup is for. The field is a password: its value never
// touches settings.json (see settings/AGENTS.md); it goes through
// Kit.Keyring instead, and GetValues always masks it.
var apiKeyLabel = i18n.T("example.kit_headless.api_key.label", "Example API key")

var apiKeyGroup = settings.Group{
	Key:   "example",
	Label: i18n.T("example.kit_headless.group.label", "Example"),
	Fields: []settings.Field{
		{Key: "example.api_key", Type: settings.FieldPassword, Label: apiKeyLabel},
	},
}

// fakeThemeSource is a stand-in for the live Wails-backed appearance.Source
// kit/wailsbridge.Attach wires in for a real GUI app (WP-31). A headless
// process has no OS theme signal to offer at all — this shows the seam
// (Kit.SetAppearanceSource) without pretending to have one.
type fakeThemeSource struct{ dark bool }

func (f *fakeThemeSource) IsDark() bool                         { return f.dark }
func (f *fakeThemeSource) Subscribe(func(bool)) (cancel func()) { return func() {} }

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	tmp, err := os.MkdirTemp("", "wails-kit-kit-headless-example-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	dirs := appdirs.New("kit-headless-example",
		appdirs.WithConfigDir(tmp+"/config"),
		appdirs.WithDataDir(tmp+"/data"),
		appdirs.WithCacheDir(tmp+"/cache"),
		appdirs.WithLogDir(tmp+"/log"),
		appdirs.WithTempDir(tmp+"/temp"),
	)

	k, err := kit.New(
		kit.AppInfo{Name: "kit-headless-example", ID: "dev.wails-kit.examples.kit-headless", Version: "1.0.0"},
		kit.WithDirs(dirs),
		kit.WithKeyring(keyring.NewMemoryStore()), // never a real OS keychain in an example
		kit.WithSettingsGroup(apiKeyGroup),
		// No real network in this example: disable the built-in default
		// connectivity check and register a fake, always-healthy one
		// instead (the same pattern examples/health/main.go uses).
		kit.WithHealthOptions(health.WithoutDefaultConnectivityCheck()),
		kit.WithHealthCheck(health.Check{
			Name:     "example-backend",
			Class:    health.ClassBackend,
			Critical: true,
			Interval: 200 * time.Millisecond,
			Probe:    health.ProbeFunc(func(context.Context) error { return nil }),
		}),
	)
	if err != nil {
		if comp, ok := kit.GetComponent(err); ok {
			return fmt.Errorf("kit.New: component %q failed: %w", comp, err)
		}
		return fmt.Errorf("kit.New: %w", err)
	}

	fmt.Println("wired components (no application.App anywhere in this process):")
	fmt.Printf("  config dir: %s\n", k.Dirs.Config())
	fmt.Printf("  log dir:    %s\n", k.Dirs.Log())
	fmt.Printf("  locale:     %s\n", k.I18n.Locale())

	if err := demoSettings(k); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := k.Start(ctx); err != nil {
		return fmt.Errorf("k.Start: %w", err)
	}
	defer func() {
		if err := k.Close(); err != nil {
			log.Printf("k.Close: %v", err)
		}
	}()
	fmt.Println("\nStart: firstrun hooks ran, health ticker is running")

	if err := demoHealth(k); err != nil {
		return err
	}

	demoAppearance(k)

	return nil
}

func demoSettings(k *kit.Kit) error {
	fmt.Println("\nsettings groups registered (kit-internal + the app's own \"example\" group):")
	for _, g := range k.Settings.GetSchema().Groups {
		fmt.Printf("  - %s\n", g.Key)
	}

	if _, err := k.Settings.SetValues(map[string]any{
		"example.api_key": "sk-example-not-a-real-key",
	}); err != nil {
		return fmt.Errorf("SetValues: %w", err)
	}

	values, err := k.Settings.GetValues()
	if err != nil {
		return fmt.Errorf("GetValues: %w", err)
	}
	fmt.Printf("\nGetValues masks the password field: example.api_key = %q\n", values["example.api_key"])

	secret, err := k.Settings.GetSecret("example.api_key")
	if err != nil {
		return fmt.Errorf("GetSecret: %w", err)
	}
	fmt.Printf("GetSecret (backend-only, never webview-callable — see settings.Binding) returns the real value: %q\n", secret)

	return nil
}

func demoHealth(k *kit.Kit) error {
	if k.Health == nil {
		return nil
	}
	// Give the ticker a moment, then trigger an immediate re-check for a
	// deterministic demo instead of a sleep-and-hope.
	k.Health.Trigger("example-backend")
	snap := k.Health.Snapshot()
	fmt.Printf("\nhealth snapshot: overall=%s offline=%v\n", snap.Overall, snap.Offline)
	for _, c := range snap.Checks {
		fmt.Printf("  - %-16s class=%-10s state=%s\n", c.Name, c.Class, c.State)
	}
	return nil
}

func demoAppearance(k *kit.Kit) {
	fmt.Printf("\nappearance before any Source is wired: resolved=%s (dynamicSource's zero value has none — see kit/AGENTS.md)\n", k.Appearance.Resolved())

	// This is the seam kit/wailsbridge.Attach (WP-31) uses in a real GUI
	// app, retargeting the same already-constructed Appearance service at a
	// live Wails-backed Source instead of a fake one.
	k.SetAppearanceSource(&fakeThemeSource{dark: true})
	fmt.Printf("appearance after SetAppearanceSource(dark): resolved=%s\n", k.Appearance.Resolved())
}
