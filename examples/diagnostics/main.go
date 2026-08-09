// Command diagnostics-example demonstrates diagnostics.Service: building a
// support bundle and the consent-gated Submit flow.
//
// It runs an httptest.Server as a stand-in "support endpoint" so this
// example never touches the network — safe to run repeatedly and safe to
// run in CI. In a real app, WithSettings would be wired to the app's real
// *settings.Service (persisted, not in-memory), and the webhook URL would
// be your actual support endpoint (https://, per the TLS policy documented
// in the package README).
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	kiterrors "github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/settings"

	"github.com/jrschumacher/wails-kit/v2/diagnostics"
)

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

	diagSvc, err := diagnostics.NewService(
		diagnostics.WithAppName("diagnostics-example"),
		diagnostics.WithVersion("1.0.0"),
		diagnostics.WithSettings(settingsSvc),
	)
	if err != nil {
		return fmt.Errorf("new diagnostics service: %w", err)
	}

	bundlePath, err := diagSvc.CreateBundle(context.Background(), tmp)
	if err != nil {
		return fmt.Errorf("create bundle: %w", err)
	}
	fmt.Println("bundle created at:", bundlePath)

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
