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

	"github.com/jrschumacher/wails-kit/appdirs"
	"gopkg.in/natefinch/lumberjack.v2"
)

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
	Stdout        bool     // Also write to stdout (default: true)
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
	if config == nil {
		config = &Config{AppName: "app"}
	}
	if config.AppName == "" {
		config.AppName = "app"
	}

	dir := config.LogDir
	if dir == "" {
		dirs := appdirs.New(config.AppName)
		dir = dirs.Log()
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("logging: create dir: %w", err)
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

	logFile := &lumberjack.Logger{
		Filename:   filepath.Join(dir, "app.log"),
		MaxSize:    maxSize,
		MaxAge:     maxAge,
		MaxBackups: maxBackups,
		Compress:   compress,
		LocalTime:  true,
	}

	var writer io.Writer = logFile
	if config.Stdout {
		writer = io.MultiWriter(os.Stdout, logFile)
	}

	level := parseLevel(config.Level)

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
			if err := Init(&Config{AppName: "app", Stdout: true}); err != nil {
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
