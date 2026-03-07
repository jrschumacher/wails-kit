package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestLogDir_OSSpecific(t *testing.T) {
	dir := logDir("myapp")

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, "Library", "Logs", "myapp")
		if dir != expected {
			t.Errorf("expected %s, got %s", expected, dir)
		}
	case "linux":
		if !strings.Contains(dir, "myapp") {
			t.Errorf("expected path to contain 'myapp', got %s", dir)
		}
	case "windows":
		if !strings.Contains(dir, "myapp") {
			t.Errorf("expected path to contain 'myapp', got %s", dir)
		}
	}
}

func TestInit_CreatesLogDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")

	err := Init(&Config{
		AppName: "test",
		LogDir:  dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("expected log directory to be created")
	}

	// Reset for other tests
	defaultLogger = nil
	initOnce = syncOnce()
}

func TestRedactingHandler(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	handler := NewRedactingHandler(inner, []string{"password", "token"})
	logger := slog.New(handler)

	logger.Info("test",
		"password", "secret123",
		"token", "abc",
		"username", "alice",
	)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if !strings.Contains(entry["password"].(string), "REDACTED") {
		t.Errorf("expected password to be redacted, got %v", entry["password"])
	}
	if !strings.Contains(entry["token"].(string), "REDACTED") {
		t.Errorf("expected token to be redacted, got %v", entry["token"])
	}
	if entry["username"] != "alice" {
		t.Errorf("expected username to be preserved, got %v", entry["username"])
	}
}

func TestRedactingHandler_EmptyValue(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	handler := NewRedactingHandler(inner, []string{"password"})
	logger := slog.New(handler)

	logger.Info("test", "password", "")

	var entry map[string]any
	json.Unmarshal(buf.Bytes(), &entry)

	// Empty values should not be redacted
	if strings.Contains(entry["password"].(string), "REDACTED") {
		t.Error("expected empty password to not be redacted")
	}
}

func TestRedactingHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	handler := NewRedactingHandler(inner, []string{"secret"})

	// Use WithAttrs to add a sensitive pre-set attribute
	derived := handler.WithAttrs([]slog.Attr{slog.String("secret", "value123")})
	logger := slog.New(derived)
	logger.Info("test")

	var entry map[string]any
	json.Unmarshal(buf.Bytes(), &entry)

	if !strings.Contains(entry["secret"].(string), "REDACTED") {
		t.Errorf("expected pre-set secret to be redacted, got %v", entry["secret"])
	}
}

func TestWithFields(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := &Logger{Logger: slog.New(handler)}

	derived := logger.WithFields("component", "auth", "version", 2)
	derived.Info("test msg")

	var entry map[string]any
	json.Unmarshal(buf.Bytes(), &entry)

	if entry["component"] != "auth" {
		t.Errorf("expected component=auth, got %v", entry["component"])
	}
	if entry["version"] != float64(2) {
		t.Errorf("expected version=2, got %v", entry["version"])
	}
}

// syncOnce returns a fresh sync.Once for test reset.
func syncOnce() syncOnceType {
	return syncOnceType{}
}

type syncOnceType = sync.Once
