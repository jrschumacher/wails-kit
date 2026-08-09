// Command diagnostics-example demonstrates diagnostics.Service: building a
// support bundle (including the optional health.json/firstrun.json
// content) and the consent-gated Submit flow.
//
// It runs an httptest.Server as a stand-in "support endpoint" so this
// example never touches the network — safe to run repeatedly and safe to
// run in CI. In a real app, WithSettings would be wired to the app's real
// *settings.Service (persisted, not in-memory), and the webhook URL would
// be your actual support endpoint (https://, per the TLS policy documented
// in the package README). The health check registered below uses a fake,
// in-process Probe for the same reason — no real network dependency.
package main

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	kiterrors "github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/firstrun"
	"github.com/jrschumacher/wails-kit/v2/health"
	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/settings"

	"github.com/jrschumacher/wails-kit/v2/diagnostics"
)

// failingProbe is a health.Probe that never touches the network. Its error
// deliberately looks like a real net/http failure (it embeds a URL with a
// query-string token) so the example can demonstrate that diagnostics
// never repeats that text into health.json — see writeHealth/WithHealth's
// doc comment in diagnostics/health.go and the README "Security" section.
type failingProbe struct{}

func (failingProbe) Probe(context.Context) error {
	// The embedded, capitalized `Get "...": ...` text is deliberately in
	// the shape net/http's *url.Error renders on a real request failure —
	// see the type doc comment above. The fmt.Errorf wrapper (rather than
	// errors.New) keeps the overall message lowercase-leading, per
	// convention, while still faithfully reproducing that shape as data.
	return fmt.Errorf("probe failed: %s", `Get "https://internal-status.example.corp/health?token=sekrit": dial tcp: no such host`)
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	tmp, err := os.MkdirTemp("", "wails-kit-diagnostics-example-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	// A stand-in "support endpoint". In a real app this is your actual
	// webhook URL — diagnostics.Submit refuses plaintext http:// to
	// anything other than localhost, so this httptest server (which is
	// http://127.0.0.1:<port>) is allowed without any special-casing.
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 0)
		p := make([]byte, 4096)
		for {
			n, err := r.Body.Read(p)
			buf = append(buf, p[:n]...)
			if err != nil {
				break
			}
		}
		received = buf
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// settings.Service holds the consent toggle diagnostics.SettingsGroup
	// defines. A real app registers this group alongside its other settings
	// groups (see docs/settings-integration.md) and passes the same service
	// to both settings.Binding (for the frontend) and diagnostics.WithSettings.
	settingsSvc := settings.NewService(
		settings.WithStoragePath(filepath.Join(tmp, "settings.json")),
		settings.WithKeyring(keyring.NewMemoryStore()),
		settings.WithGroup(diagnostics.SettingsGroup()),
	)

	// A health.Registry with one intentionally-failing check, registered
	// via WithHealth so CreateBundle includes health.json. The registered
	// check's error text contains a fake internal URL with a token in its
	// query string, purely to demonstrate that health.json never repeats
	// it (see failingProbe above and README "Security").
	healthReg := health.New(health.WithoutDefaultConnectivityCheck())
	if _, err := healthReg.Register(health.Check{
		Name:     "internal-status",
		Class:    health.ClassBackend,
		Critical: true,
		Probe:    failingProbe{},
	}); err != nil {
		return fmt.Errorf("register health check: %w", err)
	}
	healthReg.Trigger("internal-status") // resolve it once, synchronously

	// A firstrun.Service reporting an upgrade transition, registered via
	// WithFirstRun so CreateBundle includes firstrun.json. No stamp has
	// been written yet for this fresh temp-dir store; WithBaselineVersion
	// tells Detect to treat that as "was on 0.9.0" rather than "fresh
	// install" — see firstrun.WithBaselineVersion.
	firstrunSvc, err := firstrun.New(
		firstrun.WithStoragePath(filepath.Join(tmp, "firstrun.json")),
		firstrun.WithVersion("1.0.0"),
		firstrun.WithBaselineVersion("0.9.0"),
	)
	if err != nil {
		return fmt.Errorf("new firstrun service: %w", err)
	}

	diagSvc, err := diagnostics.NewService(
		diagnostics.WithAppName("diagnostics-example"),
		diagnostics.WithVersion("1.0.0"),
		diagnostics.WithSettings(settingsSvc),
		diagnostics.WithHealth(healthReg),
		diagnostics.WithFirstRun(firstrunSvc),
	)
	if err != nil {
		return fmt.Errorf("new diagnostics service: %w", err)
	}

	bundlePath, err := diagSvc.CreateBundle(context.Background(), tmp)
	if err != nil {
		return fmt.Errorf("create bundle: %w", err)
	}
	fmt.Println("bundle created at:", bundlePath)

	if err := printBundleFile(bundlePath, "health.json"); err != nil {
		return err
	}
	if err := printBundleFile(bundlePath, "firstrun.json"); err != nil {
		return err
	}

	// Submission consent defaults to off. Submit refuses with a typed
	// error rather than silently doing nothing — a silent no-op here would
	// be indistinguishable from a broken uploader.
	err = diagSvc.Submit(context.Background(), bundlePath, srv.URL)
	if err == nil {
		return errors.New("expected Submit to refuse before consent is granted")
	}
	if !kiterrors.IsCode(err, diagnostics.ErrConsentRequired) {
		return fmt.Errorf("expected ErrConsentRequired, got: %w", err)
	}
	fmt.Println("Submit refused without consent, as expected:", kiterrors.GetUserMessage(err))

	// The user opts in — this is the only thing that turns Submit from a
	// refusal into an actual upload.
	if _, err := settingsSvc.SetValues(map[string]any{diagnostics.SettingConsent: true}); err != nil {
		return fmt.Errorf("grant consent: %w", err)
	}
	fmt.Println("consent granted:", diagSvc.SubmissionConsent())

	if err := diagSvc.Submit(context.Background(), bundlePath, srv.URL); err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	fmt.Printf("bundle submitted: %d bytes received by the support endpoint\n", len(received))

	return nil
}

// printBundleFile reads and prints one file's contents from the bundle zip
// at bundlePath, for demonstration purposes only (a real app doesn't need
// to do this — it's just so `go run` shows what WithHealth/WithFirstRun
// actually produced).
func printBundleFile(bundlePath, name string) error {
	r, err := zip.OpenReader(bundlePath)
	if err != nil {
		return fmt.Errorf("open bundle: %w", err)
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open %s in bundle: %w", name, err)
		}
		defer func() { _ = rc.Close() }()

		data, err := io.ReadAll(rc)
		if err != nil {
			return fmt.Errorf("read %s from bundle: %w", name, err)
		}
		fmt.Printf("--- %s ---\n%s\n", name, data)
		return nil
	}
	return fmt.Errorf("%s not found in bundle", name)
}
