package diagnostics

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrschumacher/wails-kit/events"
	"github.com/jrschumacher/wails-kit/keyring"
	"github.com/jrschumacher/wails-kit/settings"
)

func TestNewService(t *testing.T) {
	t.Run("requires app name", func(t *testing.T) {
		_, err := NewService()
		if err == nil {
			t.Fatal("expected error for missing app name")
		}
	})

	t.Run("creates with app name", func(t *testing.T) {
		svc, err := NewService(WithAppName("test-app"))
		if err != nil {
			t.Fatal(err)
		}
		if svc.appName != "test-app" {
			t.Fatalf("expected app name test-app, got %s", svc.appName)
		}
	})

	t.Run("defaults max log size", func(t *testing.T) {
		svc, err := NewService(WithAppName("test-app"))
		if err != nil {
			t.Fatal(err)
		}
		if svc.maxLogSize != defaultMaxLogSize {
			t.Fatalf("expected default max log size %d, got %d", defaultMaxLogSize, svc.maxLogSize)
		}
	})
}

func TestGetSystemInfo(t *testing.T) {
	svc, err := NewService(
		WithAppName("test-app"),
		WithVersion("1.2.3"),
	)
	if err != nil {
		t.Fatal(err)
	}

	info := svc.GetSystemInfo()
	if info.AppName != "test-app" {
		t.Errorf("expected app name test-app, got %s", info.AppName)
	}
	if info.AppVersion != "1.2.3" {
		t.Errorf("expected version 1.2.3, got %s", info.AppVersion)
	}
	if info.NumCPU == 0 {
		t.Error("expected non-zero CPU count")
	}
	if info.OS == "" {
		t.Error("expected non-empty OS")
	}
}

func TestCreateBundle(t *testing.T) {
	t.Run("basic bundle with system info", func(t *testing.T) {
		outputDir := t.TempDir()
		svc, err := NewService(
			WithAppName("test-app"),
			WithVersion("0.1.0"),
		)
		if err != nil {
			t.Fatal(err)
		}

		path, err := svc.CreateBundle(context.Background(), outputDir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(filepath.Base(path), "diagnostics-test-app-") {
			t.Errorf("unexpected bundle name: %s", filepath.Base(path))
		}
		if !strings.HasSuffix(path, ".zip") {
			t.Error("expected .zip extension")
		}

		// Verify zip contents
		files := readZipFiles(t, path)
		if _, ok := files["system.json"]; !ok {
			t.Error("missing system.json")
		}
		if _, ok := files["manifest.txt"]; !ok {
			t.Error("missing manifest.txt")
		}

		// Verify system info content
		var info SystemInfo
		if err := json.Unmarshal(files["system.json"], &info); err != nil {
			t.Fatal(err)
		}
		if info.AppName != "test-app" {
			t.Errorf("expected app name test-app, got %s", info.AppName)
		}
		if info.AppVersion != "0.1.0" {
			t.Errorf("expected version 0.1.0, got %s", info.AppVersion)
		}
	})

	t.Run("bundle with logs", func(t *testing.T) {
		logDir := t.TempDir()
		outputDir := t.TempDir()

		// Create some fake log files
		if err := os.WriteFile(filepath.Join(logDir, "app.log"), []byte("current log line\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(logDir, "app-2026-03-07.log.gz"), []byte("compressed log"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(logDir, "unrelated.txt"), []byte("not a log"), 0600); err != nil {
			t.Fatal(err)
		}

		svc, err := NewService(
			WithAppName("test-app"),
			WithLogDir(logDir),
		)
		if err != nil {
			t.Fatal(err)
		}

		path, err := svc.CreateBundle(context.Background(), outputDir)
		if err != nil {
			t.Fatal(err)
		}

		files := readZipFiles(t, path)

		if _, ok := files["logs/app.log"]; !ok {
			t.Error("missing logs/app.log")
		}
		if _, ok := files["logs/app-2026-03-07.log.gz"]; !ok {
			t.Error("missing logs/app-2026-03-07.log.gz")
		}
		if _, ok := files["logs/unrelated.txt"]; ok {
			t.Error("should not include non-log files")
		}

		// Check manifest lists log files
		manifest := string(files["manifest.txt"])
		if !strings.Contains(manifest, "logs/app.log") {
			t.Error("manifest should list logs/app.log")
		}
	})

	t.Run("bundle with sanitized settings", func(t *testing.T) {
		outputDir := t.TempDir()
		settingsPath := filepath.Join(t.TempDir(), "settings.json")

		settingsSvc := settings.NewService(
			settings.WithStorePath(settingsPath),
			settings.WithKeyring(keyring.NewMemoryStore()),
			settings.WithGroup(settings.Group{
				Key:   "general",
				Label: "General",
				Fields: []settings.Field{
					{Key: "general.name", Type: settings.FieldText, Label: "Name"},
					{Key: "general.api_key", Type: settings.FieldPassword, Label: "API Key"},
				},
			}),
		)

		// Set some values
		if _, err := settingsSvc.SetValues(map[string]any{
			"general.name":    "My App",
			"general.api_key": "sk-secret-123",
		}); err != nil {
			t.Fatal(err)
		}

		svc, err := NewService(
			WithAppName("test-app"),
			WithSettings(settingsSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		path, err := svc.CreateBundle(context.Background(), outputDir)
		if err != nil {
			t.Fatal(err)
		}

		files := readZipFiles(t, path)
		if _, ok := files["settings.json"]; !ok {
			t.Fatal("missing settings.json")
		}

		var vals map[string]any
		if err := json.Unmarshal(files["settings.json"], &vals); err != nil {
			t.Fatal(err)
		}

		if vals["general.name"] != "My App" {
			t.Errorf("expected name 'My App', got %v", vals["general.name"])
		}
		if vals["general.api_key"] != "[REDACTED]" {
			t.Errorf("expected api_key to be redacted, got %v", vals["general.api_key"])
		}
	})

	t.Run("log size cap", func(t *testing.T) {
		logDir := t.TempDir()
		outputDir := t.TempDir()

		// Create a log file larger than our cap
		bigContent := strings.Repeat("x", 200)
		if err := os.WriteFile(filepath.Join(logDir, "big.log"), []byte(bigContent), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(logDir, "small.log"), []byte("small"), 0600); err != nil {
			t.Fatal(err)
		}

		svc, err := NewService(
			WithAppName("test-app"),
			WithLogDir(logDir),
			WithMaxLogSize(100), // 100 bytes cap
		)
		if err != nil {
			t.Fatal(err)
		}

		path, err := svc.CreateBundle(context.Background(), outputDir)
		if err != nil {
			t.Fatal(err)
		}

		files := readZipFiles(t, path)

		// small.log should be included (5 bytes < 100 byte cap)
		if _, ok := files["logs/small.log"]; !ok {
			t.Error("expected small.log to be included")
		}
		// big.log should be excluded (200 bytes > 100 byte cap)
		if _, ok := files["logs/big.log"]; ok {
			t.Error("expected big.log to be excluded due to size cap")
		}
	})

	t.Run("emits bundle created event", func(t *testing.T) {
		outputDir := t.TempDir()
		mem := events.NewMemoryEmitter()
		emitter := events.NewEmitter(mem)

		svc, err := NewService(
			WithAppName("test-app"),
			WithEmitter(emitter),
		)
		if err != nil {
			t.Fatal(err)
		}

		path, err := svc.CreateBundle(context.Background(), outputDir)
		if err != nil {
			t.Fatal(err)
		}

		if mem.Count() != 1 {
			t.Fatalf("expected 1 event, got %d", mem.Count())
		}

		last := mem.Last()
		if last.Name != EventBundleCreated {
			t.Errorf("expected event %s, got %s", EventBundleCreated, last.Name)
		}

		payload, ok := last.Data.(BundleCreatedPayload)
		if !ok {
			t.Fatal("expected BundleCreatedPayload")
		}
		if payload.Path != path {
			t.Errorf("expected path %s, got %s", path, payload.Path)
		}
		if payload.Size == 0 {
			t.Error("expected non-zero bundle size")
		}
	})

	t.Run("nonexistent log dir is not an error", func(t *testing.T) {
		outputDir := t.TempDir()
		svc, err := NewService(
			WithAppName("test-app"),
			WithLogDir("/nonexistent/path"),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, err = svc.CreateBundle(context.Background(), outputDir)
		if err != nil {
			t.Fatal("nonexistent log dir should not cause error")
		}
	})
}

func TestCustomCollectors(t *testing.T) {
	t.Run("includes collector output in bundle", func(t *testing.T) {
		outputDir := t.TempDir()
		svc, err := NewService(
			WithAppName("test-app"),
			WithCustomCollector("db-version.json", func(ctx context.Context) ([]byte, error) {
				return []byte(`{"version": "3.42.0"}`), nil
			}),
			WithCustomCollector("routes.json", func(ctx context.Context) ([]byte, error) {
				return []byte(`["/api/v1/users"]`), nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}

		path, err := svc.CreateBundle(context.Background(), outputDir)
		if err != nil {
			t.Fatal(err)
		}

		files := readZipFiles(t, path)
		if data, ok := files["collectors/db-version.json"]; !ok {
			t.Error("missing collectors/db-version.json")
		} else if string(data) != `{"version": "3.42.0"}` {
			t.Errorf("unexpected collector content: %s", data)
		}

		if _, ok := files["collectors/routes.json"]; !ok {
			t.Error("missing collectors/routes.json")
		}

		// Check manifest
		manifest := string(files["manifest.txt"])
		if !strings.Contains(manifest, "collectors/db-version.json") {
			t.Error("manifest should list collector files")
		}
	})

	t.Run("skips failed collectors", func(t *testing.T) {
		outputDir := t.TempDir()
		svc, err := NewService(
			WithAppName("test-app"),
			WithCustomCollector("good.json", func(ctx context.Context) ([]byte, error) {
				return []byte(`"ok"`), nil
			}),
			WithCustomCollector("bad.json", func(ctx context.Context) ([]byte, error) {
				return nil, fmt.Errorf("collector failed")
			}),
		)
		if err != nil {
			t.Fatal(err)
		}

		path, err := svc.CreateBundle(context.Background(), outputDir)
		if err != nil {
			t.Fatal(err)
		}

		files := readZipFiles(t, path)
		if _, ok := files["collectors/good.json"]; !ok {
			t.Error("expected good.json to be included")
		}
		if _, ok := files["collectors/bad.json"]; ok {
			t.Error("expected bad.json to be excluded")
		}
	})
}

func TestSanitizeSettings(t *testing.T) {
	schema := settings.Schema{
		Groups: []settings.Group{
			{
				Key:   "test",
				Label: "Test",
				Fields: []settings.Field{
					{Key: "name", Type: settings.FieldText, Label: "Name"},
					{Key: "secret", Type: settings.FieldPassword, Label: "Secret"},
					{Key: "toggle", Type: settings.FieldToggle, Label: "Toggle"},
				},
			},
		},
	}

	values := map[string]any{
		"name":   "foo",
		"secret": "••••••••",
		"toggle": true,
	}

	result := sanitizeSettings(schema, values)

	if result["name"] != "foo" {
		t.Errorf("expected name foo, got %v", result["name"])
	}
	if result["secret"] != "[REDACTED]" {
		t.Errorf("expected secret [REDACTED], got %v", result["secret"])
	}
	if result["toggle"] != true {
		t.Errorf("expected toggle true, got %v", result["toggle"])
	}
}

func TestIsLogFile(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"app.log", true},
		{"app-2026-03-07.log", true},
		{"app.log.gz", true},
		{"app-2026-03-07.log.gz", true},
		{"settings.json", false},
		{"readme.txt", false},
		{"app.log.bak", false},
	}

	for _, tt := range tests {
		if got := isLogFile(tt.name); got != tt.expected {
			t.Errorf("isLogFile(%q) = %v, want %v", tt.name, got, tt.expected)
		}
	}
}

// readZipFiles reads all files from a zip archive into a map.
func readZipFiles(t *testing.T, path string) map[string][]byte {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	files := make(map[string][]byte)
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := readAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[f.Name] = data
	}
	return files
}

func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 1024)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			return buf, err
		}
	}
}
