# appdirs

OS-standard application directory paths for config, data, cache, log, and temp categories. Provides a single source of truth for "where does this file go?" across macOS, Linux, and Windows.

## Usage

```go
import "github.com/jrschumacher/wails-kit/appdirs"

dirs := appdirs.New("my-app")

dirs.Config()  // settings, preferences
dirs.Data()    // database, user content
dirs.Cache()   // ephemeral cached data
dirs.Log()     // log files
dirs.Temp()    // temporary working files

// Create all directories at startup
if err := dirs.EnsureAll(); err != nil {
    log.Fatal(err)
}

// Clean temp directory on startup
if err := dirs.CleanTemp(); err != nil {
    log.Fatal(err)
}
```

## OS paths

### macOS

| Category | Path |
|----------|------|
| Config | `~/Library/Application Support/{app}/` |
| Data | `~/Library/Application Support/{app}/` |
| Cache | `~/Library/Caches/{app}/` |
| Log | `~/Library/Logs/{app}/` |
| Temp | `$TMPDIR/{app}/` |

### Linux (XDG)

| Category | Path |
|----------|------|
| Config | `$XDG_CONFIG_HOME/{app}/` (default `~/.config/{app}/`) |
| Data | `$XDG_DATA_HOME/{app}/` (default `~/.local/share/{app}/`) |
| Cache | `$XDG_CACHE_HOME/{app}/` (default `~/.cache/{app}/`) |
| Log | `$XDG_STATE_HOME/{app}/` (default `~/.local/state/{app}/`) |
| Temp | `/tmp/{app}/` |

### Windows

| Category | Path |
|----------|------|
| Config | `%APPDATA%/{app}/` |
| Data | `%APPDATA%/{app}/` |
| Cache | `%LOCALAPPDATA%/{app}/cache/` |
| Log | `%LOCALAPPDATA%/{app}/logs/` |
| Temp | `%TEMP%/{app}/` |

## Options

Override any directory for testing or non-standard deployments:

```go
dirs := appdirs.New("my-app",
    appdirs.WithConfigDir("/custom/config"),
    appdirs.WithDataDir("/custom/data"),
    appdirs.WithCacheDir("/custom/cache"),
    appdirs.WithLogDir("/custom/log"),
    appdirs.WithTempDir("/custom/temp"),
)
```

## Methods

| Method | Description |
|--------|-------------|
| `Config()` | Config directory path |
| `Data()` | Data directory path |
| `Cache()` | Cache directory path |
| `Log()` | Log directory path |
| `Temp()` | Temp directory path |
| `EnsureAll()` | Create all directories with 0700 permissions |
| `CleanTemp()` | Remove temp contents and recreate empty |
