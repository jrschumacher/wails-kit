// Command logging-example demonstrates the logging package: file-rotated
// JSON logs, the package-level singleton (Init/Get), and sensitive-field
// redaction that recurses into slog.Group.
//
// It points LogDir at a temp directory so this example never writes into a
// real OS log directory — safe to run repeatedly and safe to run in CI.
package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/jrschumacher/wails-kit/v2/logging"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	tmp, err := os.MkdirTemp("", "wails-kit-logging-example-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	// Init configures the package-level singleton returned by logging.Get().
	// Config{AppName: "..."} alone would resolve Stdout and Compress to
	// true (the documented defaults); here Stdout is turned off so the
	// example's output is just what we print explicitly below, not an
	// interleaved copy of every log line.
	stdoutOff := false
	if err := logging.Init(&logging.Config{
		AppName:       "logging-example",
		LogDir:        tmp,
		Level:         "debug",
		Stdout:        &stdoutOff,
		SensitiveKeys: []string{"password", "token", "api_key"},
	}); err != nil {
		return err
	}

	logging.Info("server started", "port", 8080)
	logging.Debug("cache warmed", "entries", 42)
	logging.Warn("deprecated API used", "endpoint", "/v1/old")

	// Top-level sensitive keys are redacted with a fixed marker that does
	// not leak the secret's length.
	logging.Info("login attempt", "username", "alice", "password", "hunter2")

	// Redaction recurses into slog.Group, so grouped attrs are covered too
	// — this is the WP-06 fix: previously only top-level keys were
	// inspected, so a key placed inside a group silently bypassed
	// redaction entirely.
	logging.Get().Info("llm request",
		slog.Group("auth", "token", "sk-example-not-a-real-token"),
		slog.Group("request", "model", "claude", "prompt_id", "req-123"),
	)

	// WithFields returns a derived logger carrying preset attributes.
	logger := logging.Get().WithFields("component", "sync")
	logger.Info("sync started")

	logging.Error("request failed", errDemo, "path", "/api/data")

	logFile := tmp + "/app.log"
	contents, err := os.ReadFile(logFile)
	if err != nil {
		return err
	}
	log.Printf("wrote %d bytes to %s", len(contents), logFile)
	log.Printf("log contents:\n%s", contents)

	return nil
}

var errDemo = &demoError{"connection refused"}

type demoError struct{ msg string }

func (e *demoError) Error() string { return e.msg }
