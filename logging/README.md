# logging

OS-aware structured logging with file rotation and sensitive field redaction. Built on `slog` with JSON output.

## Usage

```go
import "github.com/jrschumacher/wails-kit/v2/logging"

err := logging.Init(&logging.Config{
    AppName:       "my-app",
    Level:         "info",         // debug, info, warn, error
    AddSource:     true,
    MaxSize:       100,            // MB per file
    MaxAge:        7,              // days
    MaxBackups:    10,
    // Compress and Stdout default to true; leave them nil unless you need
    // to turn one off (e.g. Stdout: ptr(false) for a headless service).
    SensitiveKeys: []string{"password", "token", "api_key"},
})

// Package-level convenience functions
logging.Info("server started", "port", 8080)
logging.Error("request failed", err, "path", "/api/data")
logging.Debug("cache hit", "key", "user:123")
logging.Warn("deprecated API used", "endpoint", "/v1/old")

// Logger with preset fields
logger := logging.Get().WithFields("component", "sync", "user_id", "abc")
logger.Info("sync started")
```

## Log paths

OS-standard locations:

| OS | Path |
|----|------|
| macOS | `~/Library/Logs/{app}/` |
| Linux | `$XDG_STATE_HOME/{app}/` (fallback `~/.local/state/{app}/`) |
| Windows | `%LOCALAPPDATA%/{app}/logs/` |

## Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `AppName` | string | `"app"` | App name for log directory |
| `Level` | string | `"info"` | Minimum log level: debug, info, warn, error |
| `AddSource` | bool | `false` | Include source file/line in log entries |
| `MaxSize` | int | `100` | Max size in MB before rotation |
| `MaxAge` | int | `7` | Max age in days before cleanup |
| `MaxBackups` | int | `10` | Max number of old log files to keep |
| `Compress` | `*bool` | `true` | Compress rotated log files. Pointer so `false` can be set explicitly; `nil` (the zero value) resolves to `true`. |
| `SensitiveKeys` | []string | `nil` | Field names to redact in output |
| `Stdout` | `*bool` | `true` | Also write logs to stdout in addition to the log file. Pointer so `false` can be set explicitly; `nil` resolves to `true`. |

An empty `Config{AppName: "x"}` therefore logs to **both** stdout and the
rotating file, with compression on. Set `Stdout: ptr(false)` for a
file-only logger (e.g. a background service with no attached terminal).

## Sensitive field redaction

Configured field names are replaced with the fixed marker `[REDACTED]` in
log output — deliberately **not** `[REDACTED:N chars]` or any other form
that reveals the secret's length, since length alone can narrow a brute
force search:

```go
logging.Init(&logging.Config{
    SensitiveKeys: []string{"password", "token"},
})

logging.Info("auth", "password", "secret123")
// Output: {"msg":"auth","password":"[REDACTED]"}
```

Redaction recurses into `slog.Group`, so grouped attrs are covered too:

```go
logging.Info("session started",
    slog.Group("session", "token", tok, "user", "alice"),
)
// Output: {"msg":"session started","session":{"token":"[REDACTED]","user":"alice"}}
```

Redaction matches on attribute key at any nesting depth — it does not
inspect group names, so a key like `token` is redacted whether it appears
top-level or inside any number of nested groups.

## Multi-writer output

By default (`Stdout` unset or explicitly `true`), logs are written to both
stdout and the log file simultaneously. Set `Stdout` to `false` to write to
the file only.

## File rotation

Powered by [lumberjack](https://github.com/natefinch/lumberjack.v2):

- Rotates when file exceeds `MaxSize` MB
- Removes files older than `MaxAge` days
- Keeps at most `MaxBackups` old files
- Optionally compresses rotated files with gzip
