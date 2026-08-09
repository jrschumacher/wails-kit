package diagnostics

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

// TestValidateWebhookURL is a pure unit test (no network) of the TLS/
// localhost enforcement policy: audit finding — the original webhook.go had
// no scheme check at all, so a misconfigured http:// endpoint would silently
// receive bundle contents (and any bearer token) in the clear.
func TestValidateWebhookURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"https to arbitrary host is allowed", "https://support.example.com/upload", false},
		{"http to localhost hostname is allowed", "http://localhost:8080/upload", false},
		{"http to 127.0.0.1 is allowed", "http://127.0.0.1:8080/upload", false},
		{"http to ::1 is allowed", "http://[::1]:8080/upload", false},
		{"http to a public host is rejected", "http://support.example.com/upload", true},
		{"non-http(s) scheme is rejected", "ftp://example.com/upload", true},
		{"malformed URL is rejected", "://not-a-url", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWebhookURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWebhookURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
			if err != nil && !errors.IsCode(err, ErrBundleSubmit) {
				t.Errorf("expected ErrBundleSubmit code, got: %v", err)
			}
		})
	}
}

// TestSubmitRejectsPlaintextEndpoint is the failing-first regression test
// for the same finding at the Submit entry point: consent is granted, but
// the destination is a plaintext http:// URL to a non-localhost host, so no
// network call should ever be attempted.
func TestSubmitRejectsPlaintextEndpoint(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "bundle.zip")
	if err := os.WriteFile(bundlePath, []byte("fake zip content"), 0o600); err != nil {
		t.Fatal(err)
	}

	settingsSvc := newTestSettingsService(t)
	if _, err := settingsSvc.SetValues(map[string]any{SettingConsent: true}); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(WithAppName("test-app"), WithSettings(settingsSvc))
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Submit(context.Background(), bundlePath, "http://attacker.example.com/upload")
	if err == nil {
		t.Fatal("expected Submit to reject a plaintext non-localhost endpoint")
	}
	if !errors.IsCode(err, ErrBundleSubmit) {
		t.Errorf("expected ErrBundleSubmit code, got: %v", err)
	}
}

// TestSubmitBundleDoesNotFollowRedirects is the audit's redirect finding:
// the original client used the default net/http redirect policy, which
// would follow a 3xx response anywhere — including back down to plaintext
// http:// — silently bypassing validateWebhookURL and re-sending the bearer
// token wherever the redirect pointed. A compromised or misconfigured
// endpoint could exploit this. It must now surface as a single, non-retried
// failure instead of being followed.
func TestSubmitBundleDoesNotFollowRedirects(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "bundle.zip")
	if err := os.WriteFile(bundlePath, []byte("fake zip content"), 0o600); err != nil {
		t.Fatal(err)
	}

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		// Redirect target is intentionally unreachable/bogus — if the
		// client ever tried to follow it, this test would either hang or
		// leak a network dial attempt, which the no-network gate catches.
		http.Redirect(w, r, "http://127.0.0.1:1/elsewhere", http.StatusFound)
	}))
	defer srv.Close()

	svc, err := NewService(WithAppName("test-app"), WithWebhookMaxRetries(3))
	if err != nil {
		t.Fatal(err)
	}

	err = svc.submitBundle(context.Background(), bundlePath, srv.URL)
	if err == nil {
		t.Fatal("expected an error for an unfollowed redirect response")
	}
	if attempts.Load() != 1 {
		t.Errorf("expected exactly 1 attempt (redirect treated as non-retryable), got %d", attempts.Load())
	}
}

// TestUploadPathRedactsSecrets is the redaction-coverage finding: the prior
// review confirmed the on-disk bundle redacts password fields, but nobody
// had verified that the bytes actually placed on the wire during upload
// carry the same guarantee — SubmitBundle just streams whatever file is at
// bundlePath, so this checks the real, wire-level content, not the on-disk
// zip.
func TestUploadPathRedactsSecrets(t *testing.T) {
	outputDir := t.TempDir()
	storePath := filepath.Join(t.TempDir(), "settings.json")
	settingsSvc := settings.NewService(
		settings.WithStorePath(storePath),
		settings.WithKeyring(keyring.NewMemoryStore()),
		settings.WithGroup(SettingsGroup()),
		settings.WithGroup(settings.Group{
			Key:   "general",
			Label: i18n.Text{Other: "General"},
			Fields: []settings.Field{
				{Key: "general.name", Type: settings.FieldText, Label: i18n.Text{Other: "Name"}},
				{Key: "general.api_key", Type: settings.FieldPassword, Label: i18n.Text{Other: "API Key"}},
			},
		}),
	)

	const secret = "sk-super-secret-token"
	if _, err := settingsSvc.SetValues(map[string]any{
		"general.name":    "My App",
		"general.api_key": secret,
		SettingConsent:    true,
	}); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(WithAppName("test-app"), WithSettings(settingsSvc))
	if err != nil {
		t.Fatal(err)
	}

	bundlePath, err := svc.CreateBundle(context.Background(), outputDir)
	if err != nil {
		t.Fatal(err)
	}

	var receivedBody []byte
	var receivedContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := svc.Submit(context.Background(), bundlePath, srv.URL); err != nil {
		t.Fatalf("expected successful submit, got: %v", err)
	}

	// Parse the multipart body the server actually received off the wire —
	// this is the upload path, independent of the on-disk file.
	_, params, err := mime.ParseMediaType(receivedContentType)
	if err != nil {
		t.Fatal(err)
	}
	mr := multipart.NewReader(bytes.NewReader(receivedBody), params["boundary"])
	part, err := mr.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	zipBytes, err := io.ReadAll(part)
	if err != nil {
		t.Fatal(err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatal(err)
	}
	var settingsJSON []byte
	for _, f := range zr.File {
		if f.Name == "settings.json" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			settingsJSON, err = io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if settingsJSON == nil {
		t.Fatal("settings.json missing from the bundle received over the wire")
	}
	if strings.Contains(string(settingsJSON), secret) {
		t.Fatalf("secret leaked into the bytes actually uploaded: %s", settingsJSON)
	}
	if !strings.Contains(string(settingsJSON), "[REDACTED]") {
		t.Fatalf("expected redacted marker in uploaded settings.json, got: %s", settingsJSON)
	}
}
