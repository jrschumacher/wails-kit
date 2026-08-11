# diagnostics

Collects application state, logs, and system info into a shareable zip bundle for crash reporting and user support. Zero external dependencies — uses only the Go standard library.

## Usage

```go
import "github.com/jrschumacher/wails-kit/v2/diagnostics"

svc, err := diagnostics.NewService(
    diagnostics.WithAppName("my-app"),          // required
    diagnostics.WithVersion("1.2.3"),           // optional: app version
    diagnostics.WithDirs(dirs),                 // optional: appdirs for log directory
    diagnostics.WithLogDir(dirs.Log()),         // optional: explicit log directory
    diagnostics.WithSettings(settingsSvc),      // optional: include sanitized settings; also the source of Submit's consent check
    diagnostics.WithEmitter(emitter),           // optional: event notifications
    diagnostics.WithMaxLogSize(10*1024*1024),   // optional: log size cap (default 10MB)
    diagnostics.WithCustomCollector("db.json", dbCollector), // optional: custom data
    diagnostics.WithWebhookToken("token"),      // optional: bearer auth for webhook
    diagnostics.WithWebhookTimeout(30*time.Second), // optional: webhook timeout (default 30s)
    diagnostics.WithWebhookMaxRetries(3),       // optional: webhook retries (default 3)
    diagnostics.WithHealth(healthRegistry),     // optional: include health.json (see "Security" for what's omitted)
    diagnostics.WithFirstRun(firstrunSvc),      // optional: include firstrun.json (version transition info)
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

### Submit bundle via webhook — consent-gated

Submission is opt-in and off by default. Register `diagnostics.SettingsGroup()`
with your app's `settings.Service` (same service you pass to `WithSettings`)
to offer the user a toggle; until they turn it on, `Submit` refuses instead
of silently doing nothing:

```go
svc, _ := diagnostics.NewService(
    diagnostics.WithAppName("my-app"),
    diagnostics.WithSettings(settingsSvc), // same service diagnostics.SettingsGroup() is registered on
)

err := svc.Submit(ctx, bundlePath, "https://support.example.com/diagnostics")
if errors.IsCode(err, diagnostics.ErrConsentRequired) {
    // expected until the user opts in — show them the settings toggle,
    // don't retry silently and don't treat this as a transient failure.
}
```

`Submit` is the **only** exported way to upload a bundle — there is no
consent-bypassing variant, so a "send bundle" button can't accidentally be
wired straight past the gate. It:

1. Checks `svc.SubmissionConsent()` — reads the `diagnostics.submission_consent`
   value from the wired `settings.Service`. No settings service wired, no
   value set, or a non-bool value all mean **no consent**; the default is
   off, not on.
2. If consent is absent, returns an error with code `diagnostics.ErrConsentRequired`
   — check it with `errors.IsCode(err, diagnostics.ErrConsentRequired)`.
   This is deliberately not a silent no-op: a silent no-op here is
   indistinguishable from a broken uploader, which is how crash reporting
   quietly stops reporting and nobody notices.
3. If consent is present, sends a multipart POST with the zip bundle,
   including `X-App-Name` and `X-App-Version` headers and an optional
   `Authorization: Bearer <token>` if `WithWebhookToken` is configured.
   Retries with exponential backoff (capped at 30s) on 5xx and network
   errors; fails immediately, without retrying, on 4xx and on any
   unfollowed redirect response.

`SubmissionConsent() bool` is also exposed directly, so other code — notably
the planned crash-sentinel — can check consent before deciding whether to
even *offer* an automatic submission, instead of re-reading the setting
itself.

#### Webhook destination policy (TLS enforcement)

`Submit` validates the destination before any network call:

- `https://` is always allowed.
- Plaintext `http://` is allowed **only** to `localhost`, `127.0.0.1`, or
  `::1` (useful for pointing at a local test receiver during development).
- Every other `http://` destination, and every non-HTTP(S) scheme, is
  rejected immediately with an `ErrBundleSubmit` error — no request is ever
  sent.

The client also never follows HTTP redirects for a webhook request. A
redirect response is surfaced as a hard failure instead. This closes a
concrete attack: without it, a compromised or merely misconfigured endpoint
could 3xx the upload to an arbitrary destination — including a plaintext
one — silently bypassing the TLS/localhost check above and re-sending the
bearer token wherever the redirect pointed.

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

The frontend can offer a "Create Support Bundle" button that calls `CreateBundle()`, and (once the user has opted in via the settings toggle) a "Send to support" button that calls `Submit()`. Unlike `settings.Service` (which splits off a `Binding` because `GetSecret` must never be webview-callable), `diagnostics.Service` has no method that reads or returns unmasked secrets, so registering the whole service is safe: `Submit` is already consent-gated, and `SubmissionConsent()` only returns a bool.

### Health and first-run info

Two more optional, additive sources of bundle content:

```go
svc, _ := diagnostics.NewService(
    diagnostics.WithAppName("my-app"),
    diagnostics.WithHealth(healthRegistry),   // health.json
    diagnostics.WithFirstRun(firstrunSvc),    // firstrun.json
)
```

- `WithHealth(*health.Registry)` writes `health.json`: the registry's
  current `Snapshot()` at `CreateBundle` time — overall state, per-check
  state/class/criticality/timing. **The per-check error text is
  deliberately omitted** — see "Security" below.
- `WithFirstRun(*firstrun.Service)` writes `firstrun.json`: a fresh,
  side-effect-free `Detect()` call at `CreateBundle` time (no hooks run, no
  stamp written) — the same `{kind, previous, current}` shape as
  `firstrun.TransitionPayload`.

Neither option is required. Without it, the corresponding file is simply
absent from the bundle — never present-but-empty — matching how
`settings.json` only appears when `WithSettings` is configured.

## Bundle contents

```
diagnostics-my-app-2026-03-08T12-00-00.zip
├── manifest.txt      # Lists all files in the bundle for user review
├── system.json       # OS, arch, Go version, app version, CPU count
├── settings.json     # Sanitized settings (passwords redacted)
├── health.json        # Health snapshot (only with WithHealth; error text omitted)
├── firstrun.json       # Version transition info (only with WithFirstRun)
├── collectors/
│   └── db-version.json   # Output from custom collectors
└── logs/
    ├── app.log               # Current log file
    ├── crash-2026-03-08T12-00-00.log  # Panic crash log
    └── app-2026-03-07.log.gz # Recent rotated logs
```

The bundle zip is created with file mode `0600` (owner read/write only), not
the more permissive `0644` a plain `os.Create` would give. The containing
directory `CreateBundle` writes into is `0700`, but that protection doesn't
follow the file: users are told the bundle path and will move it into
Downloads, attach it to an email, etc. The file's own mode is what protects
it once it leaves that directory.

### Settings sanitization

When a settings service is provided, all password fields (identified by `settings.FieldPassword` in the schema) are replaced with `"[REDACTED]"`. All other settings are included as-is to help diagnose configuration issues. This redaction happens once, when `CreateBundle` writes `settings.json` into the zip — see **Security** below for why that guarantee doesn't automatically extend to arbitrary non-password fields, and how `Submit` avoids silently assuming it does.

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
| `diagnostics_consent_required` | Diagnostics submission requires your consent. Enable it in Settings before sharing a bundle. |

## Security

This package uploads support bundles — which can contain application logs —
to a remote endpoint you configure. For an app whose logs include LLM prompt
traffic or other user content, that upload path is one of the highest-risk
pieces of surface in the kit, so it's worth being explicit about what is and
isn't guaranteed.

**What `Submit` guarantees:**

- Nothing is ever uploaded without explicit, currently-active consent (see
  above) — there is no default-on path and no consent-bypassing method.
- The destination must be `https://`, or plaintext `http://` to localhost
  only; every other plaintext destination is refused before any request is
  sent.
- Redirect responses are never followed, so a compromised or misconfigured
  endpoint can't silently redirect the upload (and bearer token) somewhere
  else.
- Response bodies are never logged, and errors returned by `Submit` never
  include response-body or bundle-file content — only status codes and
  transport-level details (e.g. the destination host on a dial failure).
- Retries are bounded (`WithWebhookMaxRetries`, default 3) with capped
  exponential backoff (max 30s per attempt) — a failing endpoint cannot
  cause an unbounded retry loop.

**What it does *not* guarantee — read this before assuming a bundle is safe
to share:**

- Redaction only covers **schema-declared password fields** (`settings.FieldPassword`).
  A secret embedded in a non-password field — a token pasted into a `baseURL`
  text field, for example — is included in `settings.json` as-is. If your
  app might put sensitive values into non-password settings fields, don't
  rely on this package to catch it.
- Redaction of settings values says nothing about **log content**. `logs/*`
  files are included verbatim from the log directory. If your application
  logs secrets, prompts, or other sensitive user content, that content is
  in the bundle and will be uploaded on consent — redacting it is entirely
  the consuming application's responsibility (e.g. via the `logging`
  package's redaction hooks at write time, before it ever reaches disk).
- `Submit` uploads whatever file is at the given `bundlePath` — it doesn't
  verify that path was actually produced by this service's `CreateBundle`.
  Callers that build their own bundle-like file and pass it to `Submit` get
  none of `CreateBundle`'s redaction; only bundles created by this package's
  `CreateBundle` carry that guarantee.

A consumer that treats "consent was granted" as "the bundle is automatically
safe" will eventually be wrong. Treat consent as permission to send *the
bundle this package built*, not as a claim that the bundle is free of
sensitive content.

**`health.json` (`WithHealth`) omits per-check error text, deliberately.**
`health.CheckStatus.Err` is a Go error string, and for an HTTP-based probe
(`health.HTTPProbe`, including the registry's default connectivity check)
a failure surfaces net/http's raw `*url.Error` unmodified — its `Error()`
string is `Get "<url>": <cause>`, i.e. **the full probed URL, including any
query string**, verbatim. Apps commonly register checks against internal or
semi-private endpoints (an internal API's health route, a licensing
server), and a query string can carry a token (e.g. a signed status-page
URL). Repeating that text in a bundle destined for a third-party support
endpoint would leak it straight past every guarantee above — TLS
enforcement and redirect refusal protect the *transport*, not the
*payload*. There is no reliable way to redact an arbitrary Go error string
down to "safe" without a heuristic that silently misses cases (the same
reasoning behind the settings-redaction gap two bullets up), so `health.json`
omits `Err` entirely rather than guess at scrubbing it, keeping only a
`hadError` boolean plus `Name`/`Class`/`State`/`Critical`/`CheckedAt`/
`Latency` (all developer-chosen labels or structural/timing data, never
derived from a probed URL). If you need the actual error text for support
purposes, that's a deliberate choice your app has to make explicitly — e.g.
via `WithCustomCollector`, which makes the same "logs can contain
whatever you put in them" tradeoff exceptions above already make you own.

**`firstrun.json` (`WithFirstRun`) is comparatively low-risk** — it's a
version number and a transition kind (fresh/upgrade/downgrade/same), not an
endpoint or a credential. It is not redacted or gated behind any additional
consent beyond the same `SubmissionConsent()` check that gates everything
else `Submit` sends.

## Example: full integration

```go
func setupDiagnostics(dirs *appdirs.Dirs, settingsSvc *settings.Service, emitter *events.Emitter) *diagnostics.Service {
    // Register the consent toggle alongside your app's other settings groups.
    // (Typically done once, near where settingsSvc itself is constructed.)
    //   settings.WithGroup(diagnostics.SettingsGroup()),

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
