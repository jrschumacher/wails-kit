package diagnostics

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrschumacher/wails-kit/v2/firstrun"
)

func TestBundleIncludesFirstrun(t *testing.T) {
	outputDir := t.TempDir()

	frSvc, err := firstrun.New(
		firstrun.WithStoragePath(filepath.Join(t.TempDir(), "firstrun.json")),
		firstrun.WithVersion("2.3.0"),
		firstrun.WithBaselineVersion("2.1.0"),
	)
	if err != nil {
		t.Fatalf("firstrun.New: %v", err)
	}

	svc, err := NewService(
		WithAppName("test-app"),
		WithFirstRun(frSvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	path, err := svc.CreateBundle(context.Background(), outputDir)
	if err != nil {
		t.Fatal(err)
	}

	files := readZipFiles(t, path)
	data, ok := files["firstrun.json"]
	if !ok {
		t.Fatal("missing firstrun.json")
	}

	manifest := string(files["manifest.txt"])
	if !strings.Contains(manifest, "firstrun.json") {
		t.Error("manifest should list firstrun.json")
	}

	var payload firstrun.TransitionPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal firstrun.json: %v", err)
	}
	// No stamp has ever been written for this fresh firstrun.Service, and a
	// baseline was set — Detect reports Upgrade from baseline to current
	// (see firstrun.Service.Detect / WithBaselineVersion).
	if payload.Kind != firstrun.Upgrade {
		t.Errorf("expected kind %q, got %q", firstrun.Upgrade, payload.Kind)
	}
	if payload.Previous != "v2.1.0" {
		t.Errorf("expected previous v2.1.0, got %q", payload.Previous)
	}
	if payload.Current != "v2.3.0" {
		t.Errorf("expected current v2.3.0, got %q", payload.Current)
	}
}

func TestBundleIncludesFirstrun_Fresh(t *testing.T) {
	outputDir := t.TempDir()

	frSvc, err := firstrun.New(
		firstrun.WithStoragePath(filepath.Join(t.TempDir(), "firstrun.json")),
		firstrun.WithVersion("1.0.0"),
	)
	if err != nil {
		t.Fatalf("firstrun.New: %v", err)
	}

	svc, err := NewService(
		WithAppName("test-app"),
		WithFirstRun(frSvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	path, err := svc.CreateBundle(context.Background(), outputDir)
	if err != nil {
		t.Fatal(err)
	}

	files := readZipFiles(t, path)
	var payload firstrun.TransitionPayload
	if err := json.Unmarshal(files["firstrun.json"], &payload); err != nil {
		t.Fatalf("unmarshal firstrun.json: %v", err)
	}
	if payload.Kind != firstrun.Fresh {
		t.Errorf("expected kind %q, got %q", firstrun.Fresh, payload.Kind)
	}
	if payload.Previous != "" {
		t.Errorf("expected empty previous for a fresh install, got %q", payload.Previous)
	}
	if payload.Current != "v1.0.0" {
		t.Errorf("expected current v1.0.0, got %q", payload.Current)
	}
}

func TestBundleWithoutFirstrun_OmitsFile(t *testing.T) {
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
	if _, ok := files["firstrun.json"]; ok {
		t.Error("firstrun.json should be entirely absent when WithFirstRun is not configured")
	}
	manifest := string(files["manifest.txt"])
	if strings.Contains(manifest, "firstrun.json") {
		t.Error("manifest should not mention firstrun.json when WithFirstRun is not configured")
	}
}
