package diagnostics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/keyring"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

func newTestSettingsService(t *testing.T) *settings.Service {
	t.Helper()
	storePath := filepath.Join(t.TempDir(), "settings.json")
	return settings.NewService(
		settings.WithStorePath(storePath),
		settings.WithKeyring(keyring.NewMemoryStore()),
		settings.WithGroup(SettingsGroup()),
	)
}

func TestSettingsGroup(t *testing.T) {
	group := SettingsGroup()
	if group.Key != "diagnostics" {
		t.Fatalf("expected group key %q, got %q", "diagnostics", group.Key)
	}
	if len(group.Fields) != 1 {
		t.Fatalf("expected exactly one field, got %d", len(group.Fields))
	}
	field := group.Fields[0]
	if field.Key != SettingConsent {
		t.Errorf("expected field key %q, got %q", SettingConsent, field.Key)
	}
	if field.Type != settings.FieldToggle {
		t.Errorf("expected toggle field, got %v", field.Type)
	}
	// The whole point of consent-gating: it must default to off.
	if field.Default != false {
		t.Errorf("expected consent field to default to false, got %v", field.Default)
	}
}

func TestSubmissionConsent(t *testing.T) {
	t.Run("false when no settings service wired", func(t *testing.T) {
		svc, err := NewService(WithAppName("test-app"))
		if err != nil {
			t.Fatal(err)
		}
		if svc.SubmissionConsent() {
			t.Fatal("expected SubmissionConsent to be false with no settings service wired")
		}
	})

	t.Run("defaults to false", func(t *testing.T) {
		settingsSvc := newTestSettingsService(t)
		svc, err := NewService(WithAppName("test-app"), WithSettings(settingsSvc))
		if err != nil {
			t.Fatal(err)
		}
		if svc.SubmissionConsent() {
			t.Fatal("expected SubmissionConsent to default to false")
		}
	})

	t.Run("true once granted", func(t *testing.T) {
		settingsSvc := newTestSettingsService(t)
		svc, err := NewService(WithAppName("test-app"), WithSettings(settingsSvc))
		if err != nil {
			t.Fatal(err)
		}

		if _, err := settingsSvc.SetValues(map[string]any{SettingConsent: true}); err != nil {
			t.Fatal(err)
		}

		if !svc.SubmissionConsent() {
			t.Fatal("expected SubmissionConsent to be true after opting in")
		}
	})
}

// TestSubmitRefusesWithoutConsent is the failing-first regression test for
// the consent gate: Submit must never make a network call, and must return
// a typed (ErrConsentRequired) error, when consent has not been granted.
func TestSubmitRefusesWithoutConsent(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "bundle.zip")
	if err := os.WriteFile(bundlePath, []byte("fake zip content"), 0o600); err != nil {
		t.Fatal(err)
	}

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	settingsSvc := newTestSettingsService(t)
	svc, err := NewService(WithAppName("test-app"), WithSettings(settingsSvc))
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Submit(context.Background(), bundlePath, srv.URL)
	if err == nil {
		t.Fatal("expected Submit to refuse when consent has not been granted")
	}
	if !errors.IsCode(err, ErrConsentRequired) {
		t.Errorf("expected error code %q, got: %v", ErrConsentRequired, err)
	}
	if called {
		t.Fatal("Submit must not make a network call when consent is absent")
	}
}

// TestSubmitRefusesWithoutSettingsService covers the "no settings service
// wired at all" path — the most likely real-world default-path mistake — to
// make sure it also refuses rather than silently succeeding.
func TestSubmitRefusesWithoutSettingsService(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "bundle.zip")
	if err := os.WriteFile(bundlePath, []byte("fake zip content"), 0o600); err != nil {
		t.Fatal(err)
	}

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc, err := NewService(WithAppName("test-app"))
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Submit(context.Background(), bundlePath, srv.URL)
	if err == nil {
		t.Fatal("expected Submit to refuse when no settings service is wired")
	}
	if !errors.IsCode(err, ErrConsentRequired) {
		t.Errorf("expected error code %q, got: %v", ErrConsentRequired, err)
	}
	if called {
		t.Fatal("Submit must not make a network call with no settings service wired")
	}
}

func TestSubmitSucceedsWithConsent(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "bundle.zip")
	if err := os.WriteFile(bundlePath, []byte("fake zip content"), 0o600); err != nil {
		t.Fatal(err)
	}

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	settingsSvc := newTestSettingsService(t)
	if _, err := settingsSvc.SetValues(map[string]any{SettingConsent: true}); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(WithAppName("test-app"), WithSettings(settingsSvc))
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Submit(context.Background(), bundlePath, srv.URL); err != nil {
		t.Fatalf("expected Submit to succeed with consent granted, got: %v", err)
	}
	if !called {
		t.Fatal("expected Submit to reach the webhook server once consent is granted")
	}
}
