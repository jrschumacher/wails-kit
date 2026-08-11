package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Error codes for the logging package.
const (
	ErrLoggingInit errors.Code = "logging_init"
)

func init() {
	errors.RegisterMessages(map[errors.Code]i18n.Text{
		ErrLoggingInit: i18n.T("wailskit.logging.errors.init", "Failed to initialize logging."),
	})
}

// Config defines logger configuration.
type Config struct {
	AppName       string   // Used to determine log directory
	Level         string   // "debug", "info", "warn", "error" (default: "info")
	LogDir        string   // Override OS-standard log directory
	MaxSize       int      // Max log file size in MB (default: 100)
	MaxAge        int      // Max age in days (default: 7)
	MaxBackups    int      // Max old log files (default: 10)
	Compress      *bool    // Compress rotated files (default: true); use pointer to distinguish unset from false
	AddSource     bool     // Add source file:line to logs
	SensitiveKeys []string // Field names to redact
	// Stdout also writes logs to stdout in addition to the log file
	// (default: true). Use a pointer to distinguish unset from an
	// explicit false, mirroring Compress.
	Stdout *bool
}

// resolvedConfig holds Config after defaults have been applied. Kept
// separate from directory/writer construction so the defaulting logic can
// be unit tested without touching the filesystem or os.Stdout.
type resolvedConfig struct {
	appName    string
	level      string
	maxSize    int
	maxAge     int
	maxBackups int
	compress   bool
	stdout     bool
}

// resolveDefaults applies Config defaults. nil is treated the same as an
// empty Config.
func resolveDefaults(config *Config) resolvedConfig {
	if config == nil {
		config = &Config{}
	}

	appName := config.AppName
	if appName == "" {
		appName = "app"
	}

	maxSize := config.MaxSize
	if maxSize <= 0 {
		maxSize = 100
	}
	maxAge := config.MaxAge
	if maxAge <= 0 {
		maxAge = 7
	}
	maxBackups := config.MaxBackups
	if maxBackups <= 0 {
		maxBackups = 10
	}

	compress := true
	if config.Compress != nil {
		compress = *config.Compress
	}

	stdout := true
	if config.Stdout != nil {
		stdout = *config.Stdout
	}

	return resolvedConfig{
		appName:    appName,
		level:      config.Level,
		maxSize:    maxSize,
		maxAge:     maxAge,
		maxBackups: maxBackups,
		compress:   compress,
		stdout:     stdout,
	}
}

// Logger wraps slog.Logger.
type Logger struct {
	*slog.Logger
}

var (
	loggerMu      sync.RWMutex
	defaultLogger *Logger
	initOnce      sync.Once
)

// Init initializes the global logger. It is safe to call multiple times;
// each call replaces the previous logger.
func Init(config *Config) error {
	resolved := resolveDefaults(config)
	if config == nil {
		config = &Config{}
	}

	dir := config.LogDir
	if dir == "" {
		dirs := appdirs.New(resolved.appName)
		dir = dirs.Log()
	}
	// 0700: log files are user content (they can carry request payloads,
	// prompt traffic, etc. depending on what the app logs), so the
	// directory follows the same permission convention as appdirs and the
	// rest of the kit rather than the world-readable 0755 default.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.Wrap(ErrLoggingInit, "failed to create log directory", err)
	}

	logFile := &lumberjack.Logger{
		Filename:   filepath.Join(dir, "app.log"),
		MaxSize:    resolved.maxSize,
		MaxAge:     resolved.maxAge,
		MaxBackups: resolved.maxBackups,
		Compress:   resolved.compress,
		LocalTime:  true,
	}

	var writer io.Writer = logFile
	if resolved.stdout {
		writer = io.MultiWriter(os.Stdout, logFile)
	}

	level := parseLevel(resolved.level)

	handlerOpts := &slog.HandlerOptions{
		Level:     level,
		AddSource: config.AddSource,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.String("time", a.Value.Time().Format(time.RFC3339))
			}
			if a.Key == slog.SourceKey {
				if source, ok := a.Value.Any().(*slog.Source); ok {
					source.File = filepath.Base(source.File)
				}
			}
			return a
		},
	}

	var handler slog.Handler = slog.NewJSONHandler(writer, handlerOpts)
	if len(config.SensitiveKeys) > 0 {
		handler = NewRedactingHandler(handler, config.SensitiveKeys)
	}

	logger := &Logger{Logger: slog.New(handler)}

	loggerMu.Lock()
	defaultLogger = logger
	loggerMu.Unlock()

	return nil
}

// Get returns the global logger, initializing with defaults if needed.
func Get() *Logger {
	initOnce.Do(func() {
		loggerMu.RLock()
		initialized := defaultLogger != nil
		loggerMu.RUnlock()
		if !initialized {
			if err := Init(&Config{AppName: "app"}); err != nil {
				loggerMu.Lock()
				defaultLogger = &Logger{Logger: slog.Default()}
				loggerMu.Unlock()
			}
		}
	})
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	return defaultLogger
}

// WithFields returns a new logger with preset fields.
func (l *Logger) WithFields(args ...any) *Logger {
	return &Logger{Logger: l.With(args...)}
}

// Error logs an error message with optional fields.
func (l *Logger) Error(msg string, err error, args ...any) {
	if err != nil {
		args = append(args, "error", err.Error())
	}
	l.logWithCaller(slog.LevelError, msg, args...)
}

func (l *Logger) logWithCaller(level slog.Level, msg string, args ...any) {
	if !l.Enabled(context.Background(), level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)
	if err := l.Handler().Handle(context.Background(), r); err != nil {
		fmt.Fprintf(os.Stderr, "logging error: %v (message: %s)\n", err, msg)
	}
}

// Package-level convenience functions

func Debug(msg string, args ...any) { Get().Debug(msg, args...) }
func Info(msg string, args ...any)  { Get().Info(msg, args...) }
func Warn(msg string, args ...any)  { Get().Warn(msg, args...) }
func Error(msg string, err error, args ...any) {
	Get().Error(msg, err, args...)
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
