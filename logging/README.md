# logging

OS-aware structured logging with file rotation and sensitive field redaction. Built on `slog` with JSON output.

## Usage

```go
import "github.com/jrschumacher/wails-kit/logging"

err := logging.Init(&logging.Config{
    AppName:       "my-app",
    Level:         "info",         // debug, info, warn, error
    AddSource:     true,
    MaxSize:       100,            // MB per file
    MaxAge:        7,              // days
    MaxBackups:    10,
    Compress:      true,
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
| `AppName` | string | required | App name for log directory |
| `Level` | string | `"info"` | Minimum log level: debug, info, warn, error |
| `AddSource` | bool | `false` | Include source file/line in log entries |
| `MaxSize` | int | `100` | Max size in MB before rotation |
| `MaxAge` | int | `7` | Max age in days before cleanup |
| `MaxBackups` | int | `10` | Max number of old log files to keep |
| `Compress` | bool | `false` | Compress rotated log files |
| `SensitiveKeys` | []string | `nil` | Field names to redact in output |

## Sensitive field redaction

Configured field names are replaced with `[REDACTED:N chars]` in log output:

```go
logging.Init(&logging.Config{
    SensitiveKeys: []string{"password", "token"},
})

logging.Info("auth", "password", "secret123")
// Output: {"msg":"auth","password":"[REDACTED:9 chars]"}
```

## Multi-writer output

Logs are written to both stdout and the log file simultaneously.

## File rotation

Powered by [lumberjack](https://github.com/natefinsh/lumberjack.v2):

- Rotates when file exceeds `MaxSize` MB
- Removes files older than `MaxAge` days
- Keeps at most `MaxBackups` old files
- Optionally compresses rotated files with gzip
