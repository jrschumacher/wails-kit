package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoverAndLog(t *testing.T) {
	t.Run("captures panic to crash log", func(t *testing.T) {
		logDir := t.TempDir()
		svc, err := NewService(
			WithAppName("test-app"),
			WithLogDir(logDir),
		)
		if err != nil {
			t.Fatal(err)
		}

		// Simulate a panic in a goroutine
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer RecoverAndLog(svc)()
			panic("something went wrong")
		}()
		<-done

		// Find the crash log
		entries, err := os.ReadDir(logDir)
		if err != nil {
			t.Fatal(err)
		}

		var crashFile string
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "crash-") && strings.HasSuffix(e.Name(), ".log") {
				crashFile = e.Name()
				break
			}
		}
		if crashFile == "" {
			t.Fatal("expected crash log file to be created")
		}

		content, err := os.ReadFile(filepath.Join(logDir, crashFile))
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(content), "panic: something went wrong") {
			t.Errorf("crash log should contain panic message, got: %s", content)
		}
		if !strings.Contains(string(content), "goroutine") {
			t.Errorf("crash log should contain stack trace, got: %s", content)
		}
	})

	t.Run("no-op when no panic", func(t *testing.T) {
		logDir := t.TempDir()
		svc, err := NewService(
			WithAppName("test-app"),
			WithLogDir(logDir),
		)
		if err != nil {
			t.Fatal(err)
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			defer RecoverAndLog(svc)()
			// no panic
		}()
		<-done

		entries, err := os.ReadDir(logDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("expected no crash log files, got %d", len(entries))
		}
	})

	t.Run("no-op when no log dir", func(t *testing.T) {
		svc, err := NewService(WithAppName("test-app"))
		if err != nil {
			t.Fatal(err)
		}

		// Should not panic even without a log dir
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer RecoverAndLog(svc)()
			panic("no log dir")
		}()
		<-done
	})

	t.Run("crash logs included in bundle", func(t *testing.T) {
		logDir := t.TempDir()
		outputDir := t.TempDir()

		// Write a crash log manually
		crashContent := "panic: test crash\n\ngoroutine 1 [running]:\n..."
		if err := os.WriteFile(filepath.Join(logDir, "crash-2026-03-09T12-00-00.log"), []byte(crashContent), 0600); err != nil {
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
		if _, ok := files["logs/crash-2026-03-09T12-00-00.log"]; !ok {
			t.Error("expected crash log to be included in bundle")
		}
	})
}
