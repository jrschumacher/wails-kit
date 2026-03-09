# updates

GitHub Releases-based auto-update mechanism for Wails v3 desktop apps. Zero external dependencies — uses only the Go standard library with an inline semver parser.

## Usage

```go
import "github.com/jrschumacher/wails-kit/updates"

svc, err := updates.NewService(
    updates.WithCurrentVersion("v1.0.0"),      // required
    updates.WithGitHubRepo("myorg", "myapp"),   // required
    updates.WithEmitter(emitter),               // optional: event notifications
    updates.WithSettings(settingsSvc),          // optional: reads include_prereleases from settings
    updates.WithAssetPattern("myapp_{os}_{arch}"), // optional: asset name pattern
    updates.WithBinaryName("myapp"),            // optional: binary name inside archive
    updates.WithGitHubToken(token),             // optional: for private repos
    updates.WithHTTPClient(client),             // optional: custom HTTP client
    updates.WithApplier(customApplier),         // optional: custom binary replacement
    updates.WithIncludePrereleases(false),      // optional: static fallback if no settings
    updates.WithPublicKey(publicKey),           // optional: Ed25519 key for signature verification
    updates.WithSkipVerification(),             // optional: skip verification (dev only)
)
```

### Check for updates

```go
rel, err := svc.CheckForUpdate(ctx)
if rel != nil {
    // A newer version is available
    fmt.Printf("Update available: %s\n", rel.Version)
    fmt.Printf("Release notes:\n%s\n", rel.Body)
}
```

### Download and apply

```go
// Download the platform-appropriate asset
path, err := svc.DownloadUpdate(ctx)

// Replace the running binary (user should restart after)
err = svc.ApplyUpdate(ctx)
```

### With settings integration (optional)

The updates service can optionally integrate with the settings package. When a settings service is provided via `WithSettings`, `CheckForUpdate` reads the `updates.include_prereleases` setting at call time. Without settings, it falls back to the `WithIncludePrereleases` option (default: `false`).

The `check_frequency` and `auto_download` settings are for the **app's** use — the library doesn't poll or auto-download. Your app reads those values and decides when to call `CheckForUpdate` and `DownloadUpdate`.

```go
import "github.com/jrschumacher/wails-kit/settings"

settingsSvc := settings.NewService(
    settings.WithAppName("my-app"),
    settings.WithGroup(updates.SettingsGroup()),
)
```

This adds three settings:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `updates.check_frequency` | select | `"daily"` | How often to check: startup, daily, weekly, never |
| `updates.auto_download` | toggle | `false` | Automatically download updates when found |
| `updates.include_prereleases` | toggle | `false` | Include pre-release versions (advanced) |

Then pass the settings service to the updates service:

```go
updateSvc, err := updates.NewService(
    updates.WithCurrentVersion(version),
    updates.WithGitHubRepo("myorg", "myapp"),
    updates.WithSettings(settingsSvc),  // optional — reads include_prereleases
)
```

The `include_prereleases` setting is read by `CheckForUpdate` at call time. The `check_frequency` and `auto_download` settings are for your app to read and act on — the library does **not** poll or schedule checks automatically.

## Events

All events are emitted through the `events.Emitter` if one is provided via `WithEmitter`.

| Event | Payload | When |
|-------|---------|------|
| `updates:available` | `AvailablePayload{Version, ReleaseNotes, ReleaseURL}` | Newer version found |
| `updates:downloading` | `DownloadingPayload{Version, Progress, Downloaded, Total}` | Download progress (throttled to 250ms) |
| `updates:ready` | `ReadyPayload{Version}` | Download complete, ready to apply |
| `updates:error` | `ErrorPayload{Message, Code}` | Any update operation failed |

## Error codes

| Code | User message |
|------|-------------|
| `update_check` | Unable to check for updates. Please try again later. |
| `update_download` | Failed to download the update. Please try again. |
| `update_apply` | Failed to install the update. Please try again. |
| `update_verify` | Update signature verification failed. The download may be corrupted or tampered with. |

## Signature verification

The updates service supports Ed25519 signature verification to ensure downloaded binaries haven't been tampered with. When a public key is configured, each release asset must have a corresponding `.sig` file (e.g., `myapp_darwin_arm64.tar.gz.sig`) containing the raw Ed25519 signature.

```go
import "crypto/ed25519"

// Embed or load your Ed25519 public key
var publicKey ed25519.PublicKey = ...

svc, err := updates.NewService(
    updates.WithCurrentVersion(version),
    updates.WithGitHubRepo("myorg", "myapp"),
    updates.WithPublicKey(publicKey),
)
```

**Behavior:**
- Verification happens after download, before the update is marked ready
- If the `.sig` asset is missing from the release, the download fails
- If the signature is invalid, the downloaded file is deleted and an `update_verify` error is emitted
- Without `WithPublicKey`, verification is skipped (backward compatible)

**Signing in CI:**

Generate a keypair and sign assets in your release workflow:

```bash
# Generate keypair (one-time)
go run crypto/ed25519/cmd/generate.go  # or use any Ed25519 tool

# Sign an asset
echo -n "$(cat myapp_darwin_arm64.tar.gz)" | \
  openssl pkeyutl -sign -inkey private.pem | \
  dd of=myapp_darwin_arm64.tar.gz.sig
```

**Development mode:**

Use `WithSkipVerification()` during development to bypass signature checks. A warning is logged when active:

```go
svc, err := updates.NewService(
    updates.WithCurrentVersion(version),
    updates.WithGitHubRepo("myorg", "myapp"),
    updates.WithSkipVerification(), // logs warning — do not use in production
)
```

## Asset matching

The service matches release assets to the current platform using `runtime.GOOS` and `runtime.GOARCH`. Set a pattern with `WithAssetPattern` using `{os}` and `{arch}` placeholders:

```go
updates.WithAssetPattern("myapp_{os}_{arch}")
```

The matcher handles common naming variants automatically:

| `runtime.GOOS` | Also matches |
|-----------------|-------------|
| `darwin` | `macos`, `mac` |
| `windows` | `win` |

| `runtime.GOARCH` | Also matches |
|-------------------|-------------|
| `amd64` | `x86_64`, `x64` |
| `arm64` | `aarch64` |
| `386` | `i386`, `x86` |

If no pattern is set, the default matches `{os}_{arch}` and `{os}-{arch}`.

## Archive extraction

Downloaded assets are automatically extracted if they are `.tar.gz`, `.tgz`, or `.zip` archives. The extracted files are searched for the binary — either by the name set via `WithBinaryName` or by finding the first executable file.

Path traversal protection is enforced during extraction.

## Binary replacement

The default applier replaces the binary using an atomic rename strategy:

1. Rename current binary to `{name}.old`
2. Rename new binary to the current path
3. Clean up the `.old` file

If the rename of the new binary fails, the old binary is restored. You can provide a custom `Applier` implementation via `WithApplier` for platform-specific needs (e.g., macOS `.app` bundle replacement).

## Version comparison

Versions follow [Semantic Versioning 2.0.0](https://semver.org/). The inline parser handles:

- Standard versions: `v1.2.3`, `1.2.3`
- Pre-release: `v1.0.0-alpha`, `v1.0.0-beta.1`
- Build metadata (ignored): `v1.0.0+build.123`
- Stable releases are always newer than pre-releases of the same version

## Rate limiting

The GitHub API allows 60 requests/hour for unauthenticated requests. If you need more, provide a token via `WithGitHubToken`. The service surfaces 403/429 responses as `update_check` errors.

## Example: full integration

```go
func setupUpdates(settingsSvc *settings.Service, emitter *events.Emitter) *updates.Service {
    svc, err := updates.NewService(
        updates.WithCurrentVersion(version), // set at build time via ldflags
        updates.WithGitHubRepo("myorg", "myapp"),
        updates.WithEmitter(emitter),
        updates.WithSettings(settingsSvc),              // optional
        updates.WithAssetPattern("myapp_{os}_{arch}"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Check on startup (respecting user's check_frequency setting)
    go func() {
        vals, _ := settingsSvc.GetValues()
        if vals["updates.check_frequency"] == "never" {
            return
        }

        rel, err := svc.CheckForUpdate(context.Background())
        if err != nil {
            log.Printf("update check failed: %v", err)
            return
        }
        if rel == nil {
            return // up to date
        }

        // Auto-download if the user opted in
        if autoDownload, _ := vals["updates.auto_download"].(bool); autoDownload {
            svc.DownloadUpdate(context.Background())
        }
        // Frontend handles updates:available / updates:ready events
    }()

    return svc
}
```

### Without settings

The updates service works without the settings package:

```go
svc, err := updates.NewService(
    updates.WithCurrentVersion(version),
    updates.WithGitHubRepo("myorg", "myapp"),
    updates.WithEmitter(emitter),
    updates.WithIncludePrereleases(false),
)
```
