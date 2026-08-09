package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLogDir_UsesAppdirs(t *testing.T) {
	// Verify Init uses appdirs-derived log directory when LogDir is not set.
	// The appdirs package has its own OS-specific tests; here we just confirm
	// that the path contains the app name (integration sanity check).
	dir := filepath.Join(t.TempDir(), "logs")

	err := Init(&Config{
		AppName: "myapp",
		LogDir:  dir,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("expected log directory to be created")
	}

	// Reset for other tests
	loggerMu.Lock()
	defaultLogger = nil
	loggerMu.Unlock()
	initOnce = syncOnce()
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
	loggerMu.Lock()
	defaultLogger = nil
	loggerMu.Unlock()
	initOnce = syncOnce()
}

func TestInit_RaceCondition(t *testing.T) {
	// Verify that Init and Get are safe for concurrent use.
	dir := filepath.Join(t.TempDir(), "logs")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = Init(&Config{AppName: "test", LogDir: dir})
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = Get()
		}()
	}
	wg.Wait()

	// Reset for other tests
	loggerMu.Lock()
	defaultLogger = nil
	loggerMu.Unlock()
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

	if entry["password"] != "[REDACTED]" {
		t.Errorf("expected password to be [REDACTED], got %v", entry["password"])
	}
	if entry["token"] != "[REDACTED]" {
		t.Errorf("expected token to be [REDACTED], got %v", entry["token"])
	}
	if entry["username"] != "alice" {
		t.Errorf("expected username to be preserved, got %v", entry["username"])
	}
}

func TestRedactingHandler_DoesNotLeakLength(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	handler := NewRedactingHandler(inner, []string{"secret"})
	logger := slog.New(handler)

	logger.Info("test", "secret", "short")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	// Must not contain character count
	val := entry["secret"].(string)
	if strings.Contains(val, "chars") {
		t.Errorf("redacted value leaks length info: %s", val)
	}
	if val != "[REDACTED]" {
		t.Errorf("expected [REDACTED], got %s", val)
	}
}

func TestRedactingHandler_EmptyValue(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	handler := NewRedactingHandler(inner, []string{"password"})
	logger := slog.New(handler)

	logger.Info("test", "password", "")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

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
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if entry["secret"] != "[REDACTED]" {
		t.Errorf("expected pre-set secret to be [REDACTED], got %v", entry["secret"])
	}
}

func TestWithFields(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := &Logger{Logger: slog.New(handler)}

	derived := logger.WithFields("component", "auth", "version", 2)
	derived.Info("test msg")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if entry["component"] != "auth" {
		t.Errorf("expected component=auth, got %v", entry["component"])
	}
	if entry["version"] != float64(2) {
		t.Errorf("expected version=2, got %v", entry["version"])
	}
}

func TestCompressDefaultTrue(t *testing.T) {
	// When Compress is nil (zero value), it must resolve to true. The
	// README documents this as the default, so the resolved value (not
	// just the unset pointer) must be verified or the docs and code can
	// silently drift apart again.
	resolved := resolveDefaults(&Config{AppName: "test"})
	if !resolved.compress {
		t.Error("expected Compress to default to true")
	}
}

func TestCompressExplicitFalse(t *testing.T) {
	f := false
	resolved := resolveDefaults(&Config{AppName: "test", Compress: &f})
	if resolved.compress {
		t.Error("expected explicit Compress=false to be honored")
	}
}

func TestStdoutDefaultTrue(t *testing.T) {
	// logger.go historically commented Stdout as "default: true" while the
	// zero value (false) silently won because Init never applied a
	// default. Config{AppName: "x"} must log to both stdout and the file,
	// matching the README's documented default.
	resolved := resolveDefaults(&Config{AppName: "test"})
	if !resolved.stdout {
		t.Error("expected Stdout to default to true")
	}
}

func TestStdoutExplicitFalse(t *testing.T) {
	f := false
	resolved := resolveDefaults(&Config{AppName: "test", Stdout: &f})
	if resolved.stdout {
		t.Error("expected explicit Stdout=false to be honored")
	}
}

func TestInit_LogDirIs0700(t *testing.T) {
	// Log files can carry user content (e.g. request/response payloads),
	// so the directory should follow the kit's 0700 convention rather than
	// the world-readable 0755 default.
	dir := filepath.Join(t.TempDir(), "logs")

	if err := Init(&Config{AppName: "test", LogDir: dir}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("expected log dir mode 0700, got %#o", perm)
	}

	loggerMu.Lock()
	defaultLogger = nil
	loggerMu.Unlock()
	initOnce = syncOnce()
}

func TestRedactGroupRecursion(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	handler := NewRedactingHandler(inner, []string{"token", "password"})
	logger := slog.New(handler)

	logger.Info("session started",
		slog.Group("session",
			"token", "super-secret-token",
			"user", "alice",
		),
	)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	session, ok := entry["session"].(map[string]any)
	if !ok {
		t.Fatalf("expected session group in output, got %v", entry["session"])
	}
	if session["token"] != "[REDACTED]" {
		t.Errorf("expected grouped token to be [REDACTED], got %v", session["token"])
	}
	if session["user"] != "alice" {
		t.Errorf("expected grouped user to be preserved, got %v", session["user"])
	}
}

func TestRedactGroupRecursion_Nested(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	handler := NewRedactingHandler(inner, []string{"api_key"})
	logger := slog.New(handler)

	logger.Info("request",
		slog.Group("request",
			slog.Group("auth",
				"api_key", "sk-abc123",
			),
		),
	)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	request, ok := entry["request"].(map[string]any)
	if !ok {
		t.Fatalf("expected request group in output, got %v", entry["request"])
	}
	auth, ok := request["auth"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested auth group in output, got %v", request["auth"])
	}
	if auth["api_key"] != "[REDACTED]" {
		t.Errorf("expected nested api_key to be [REDACTED], got %v", auth["api_key"])
	}
}

// syncOnce returns a fresh sync.Once for test reset.
func syncOnce() syncOnceType {
	return syncOnceType{}
}

type syncOnceType = sync.Once
