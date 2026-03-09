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
    diagnostics.WithCustomCollector("db.json", dbCollector), // optional: custom data
    diagnostics.WithWebhookToken("token"),      // optional: bearer auth for webhook
    diagnostics.WithWebhookTimeout(30*time.Second), // optional: webhook timeout (default 30s)
    diagnostics.WithWebhookMaxRetries(3),       // optional: webhook retries (default 3)
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

### Submit bundle via webhook

```go
err := svc.SubmitBundle(ctx, bundlePath, "https://support.example.com/diagnostics")
```

Sends a multipart POST with the zip bundle. Includes `X-App-Name` and `X-App-Version` headers, and optional `Authorization: Bearer <token>` if configured. Retries with exponential backoff on 5xx errors; fails immediately on 4xx errors.

### Custom collectors

Register functions that contribute arbitrary data to the bundle:

```go
svc, _ := diagnostics.NewService(
    diagnostics.WithAppName("my-app"),
    diagnostics.WithCustomCollector("db-version.json", func(ctx context.Context) ([]byte, error) {
        version, err := db.QueryVersion(ctx)
        if err != nil {
            return nil, err
        }
        return json.Marshal(map[string]string{"version": version})
    }),
    diagnostics.WithCustomCollector("feature-flags.json", func(ctx context.Context) ([]byte, error) {
        return json.Marshal(featureFlags)
    }),
)
```

Each collector's output is written to `collectors/{name}` in the zip. Failed collectors are silently skipped.

### Panic capture

Wrap goroutines with `RecoverAndLog` to capture panics as crash logs:

```go
go func() {
    defer diagnostics.RecoverAndLog(diagSvc)()
    // ... work that may panic ...
}()
```

On panic, writes a `crash-{timestamp}.log` file to the log directory containing the panic message and stack trace. These crash logs are automatically included in the next bundle created by `CreateBundle`.

**Limitations:** Only captures panics in goroutines that explicitly use this helper. Does not capture panics in the main goroutine or goroutines started by third-party libraries.

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
├── collectors/
│   └── db-version.json   # Output from custom collectors
└── logs/
    ├── app.log               # Current log file
    ├── crash-2026-03-08T12-00-00.log  # Panic crash log
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
| `diagnostics:bundle_submitted` | `BundleSubmittedPayload{Path, StatusCode}` | Bundle successfully submitted via webhook |

## Error codes

| Code | User message |
|------|-------------|
| `diagnostics_bundle` | Failed to create the diagnostics bundle. Please try again. |
| `diagnostics_logs` | Failed to collect log files for the diagnostics bundle. |
| `diagnostics_submit` | Failed to submit the diagnostics bundle. Please try again. |

## Example: full integration

```go
func setupDiagnostics(dirs *appdirs.Dirs, settingsSvc *settings.Service, emitter *events.Emitter) *diagnostics.Service {
    svc, err := diagnostics.NewService(
        diagnostics.WithAppName("my-app"),
        diagnostics.WithVersion(version),
        diagnostics.WithDirs(dirs),
        diagnostics.WithSettings(settingsSvc),
        diagnostics.WithEmitter(emitter),
        diagnostics.WithWebhookToken(os.Getenv("SUPPORT_TOKEN")),
        diagnostics.WithCustomCollector("db-version.json", func(ctx context.Context) ([]byte, error) {
            return json.Marshal(map[string]string{"sqlite": db.Version()})
        }),
    )
    if err != nil {
        log.Fatal(err)
    }
    return svc
}
```
