# diagnostics

Collects application state, logs, and system info into a shareable zip bundle for crash reporting and user support. Zero external dependencies — uses only the Go standard library.

## Usage

```go
import "github.com/jrschumacher/wails-kit/diagnostics"

svc, err := diagnostics.NewService(
    diagnostics.WithAppName("my-app"),          // required
    diagnostics.WithVersion("1.2.3"),           // optional: app version
    diagnostics.WithDirs(dirs),                 // optional: appdirs for log directory
    diagnostics.WithLogDir(dirs.Log()),         // optional: explicit log directory
    diagnostics.WithSettings(settingsSvc),      // optional: include sanitized settings
    diagnostics.WithEmitter(emitter),           // optional: event notifications
    diagnostics.WithMaxLogSize(10*1024*1024),   // optional: log size cap (default 10MB)
)
```

### Create a support bundle

```go
path, err := svc.CreateBundle(ctx, "/path/to/save/")
// Returns: /path/to/save/diagnostics-my-app-2026-03-08T12-00-00.zip
```

### Get system info (for About screens)

```go
info := svc.GetSystemInfo()
// SystemInfo{OS, Arch, GoVersion, AppName, AppVersion, NumCPU, Timestamp}
```

### Register as a Wails service

```go
app := application.New(application.Options{
    Services: []application.Service{
        application.NewService(diagSvc),
    },
})
```

The frontend can offer a "Create Support Bundle" button that calls `CreateBundle()`.

## Bundle contents

```
diagnostics-my-app-2026-03-08T12-00-00.zip
├── manifest.txt      # Lists all files in the bundle for user review
├── system.json       # OS, arch, Go version, app version, CPU count
├── settings.json     # Sanitized settings (passwords redacted)
└── logs/
    ├── app.log               # Current log file
    └── app-2026-03-07.log.gz # Recent rotated logs
```

### Settings sanitization

When a settings service is provided, all password fields (identified by `settings.FieldPassword` in the schema) are replaced with `"[REDACTED]"`. All other settings are included as-is to help diagnose configuration issues.

### Log collection

- Includes `*.log` and `*.log.gz` files from the log directory
- Newest files are prioritized when the size cap is reached
- Configurable total size cap (default 10MB)
- Non-existent log directory is silently skipped

## Events

| Event | Payload | When |
|-------|---------|------|
| `diagnostics:bundle_created` | `BundleCreatedPayload{Path, Size}` | Bundle zip successfully created |

## Error codes

| Code | User message |
|------|-------------|
| `diagnostics_bundle` | Failed to create the diagnostics bundle. Please try again. |
| `diagnostics_logs` | Failed to collect log files for the diagnostics bundle. |

## Example: full integration

```go
func setupDiagnostics(dirs *appdirs.Dirs, settingsSvc *settings.Service, emitter *events.Emitter) *diagnostics.Service {
    svc, err := diagnostics.NewService(
        diagnostics.WithAppName("my-app"),
        diagnostics.WithVersion(version),
        diagnostics.WithDirs(dirs),
        diagnostics.WithSettings(settingsSvc),
        diagnostics.WithEmitter(emitter),
    )
    if err != nil {
        log.Fatal(err)
    }
    return svc
}
```
