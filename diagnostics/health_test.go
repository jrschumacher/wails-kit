package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jrschumacher/wails-kit/v2/health"
)

// failingProbe never touches the network — used so the registry's checks
// resolve deterministically without any test dialing out.
type failingProbe struct{ err error }

func (p failingProbe) Probe(context.Context) error { return p.err }

func TestBundleIncludesHealth(t *testing.T) {
	outputDir := t.TempDir()

	reg := health.New(health.WithoutDefaultConnectivityCheck())
	if _, err := reg.Register(health.Check{
		Name:     "backend-api",
		Class:    health.ClassBackend,
		Critical: true,
		Probe:    failingProbe{err: errors.New(`Get "https://internal-api.corp.example.com/v1/health?token=sekrit": dial tcp: no such host`)},
	}); err != nil {
		t.Fatalf("register check: %v", err)
	}
	// Force a probe so the check has a non-unknown status to serialize.
	reg.Trigger("backend-api")

	svc, err := NewService(
		WithAppName("test-app"),
		WithHealth(reg),
	)
	if err != nil {
		t.Fatal(err)
	}

	path, err := svc.CreateBundle(context.Background(), outputDir)
	if err != nil {
		t.Fatal(err)
	}

	files := readZipFiles(t, path)
	data, ok := files["health.json"]
	if !ok {
		t.Fatal("missing health.json")
	}

	manifest := string(files["manifest.txt"])
	if !strings.Contains(manifest, "health.json") {
		t.Error("manifest should list health.json")
	}

	var summary healthSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("unmarshal health.json: %v", err)
	}
	if len(summary.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(summary.Checks))
	}
	got := summary.Checks[0]
	if got.Name != "backend-api" {
		t.Errorf("expected name backend-api, got %s", got.Name)
	}
	if got.State != health.StateDown {
		t.Errorf("expected state down, got %s", got.State)
	}
	if !got.HadError {
		t.Error("expected HadError=true for a failing probe")
	}

	// The privacy-sensitive part of this test: the raw error text (which
	// contains a full URL with a token query parameter) must never appear
	// anywhere in the bundle's health.json bytes.
	raw := string(data)
	if strings.Contains(raw, "token=sekrit") {
		t.Error("health.json leaked the probe error's query string")
	}
	if strings.Contains(raw, "internal-api.corp.example.com") {
		t.Error("health.json leaked the probe error's hostname")
	}
	// Confirm no "err"-ish key ever made it into any check object.
	var generic struct {
		Checks []map[string]json.RawMessage `json:"checks"`
	}
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("unmarshal generic: %v", err)
	}
	for _, c := range generic.Checks {
		if _, ok := c["err"]; ok {
			t.Error(`health.json check object must not contain an "err" field`)
		}
		if _, ok := c["Err"]; ok {
			t.Error(`health.json check object must not contain an "Err" field`)
		}
	}
}

func TestBundleWithoutHealth_OmitsFile(t *testing.T) {
	outputDir := t.TempDir()
	svc, err := NewService(WithAppName("test-app"))
	if err != nil {
		t.Fatal(err)
	}

	path, err := svc.CreateBundle(context.Background(), outputDir)
	if err != nil {
		t.Fatal(err)
	}

	files := readZipFiles(t, path)
	if _, ok := files["health.json"]; ok {
		t.Error("health.json should be entirely absent when WithHealth is not configured")
	}
	manifest := string(files["manifest.txt"])
	if strings.Contains(manifest, "health.json") {
		t.Error("manifest should not mention health.json when WithHealth is not configured")
	}
}
