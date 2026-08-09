# updates

GitHub Releases-based auto-update mechanism for Wails v3 desktop apps. This package replaces the user's running binary — it is the highest-stakes package in the kit. Signature verification uses [minisign](https://jedisct1.github.io/minisign/) via the pure-Go [`github.com/jedisct1/go-minisign`](https://github.com/jedisct1/go-minisign) verifier (see "Signature verification" below for why).

Backwards compatibility with wails-kit v1.x is **not** provided — the public key type on `WithPublicKey` and the signature file extension (`.sig` → `.minisig`) both changed. See "Migrating from v1" at the bottom.

## Usage

```go
import "github.com/jrschumacher/wails-kit/v2/updates"

svc, err := updates.NewService(
    updates.WithCurrentVersion("v1.0.0"),          // required
    updates.WithGitHubRepo("myorg", "myapp"),      // required
    updates.WithEmitter(emitter),                  // optional: event notifications
    updates.WithSettings(settingsSvc),             // optional: reads include_prereleases from settings
    updates.WithAssetPattern("myapp_{os}_{arch}"), // optional: asset name pattern
    updates.WithBinaryName("myapp"),               // optional: binary name inside archive
    updates.WithGitHubToken(token),                // optional: for private repos
    updates.WithGitHubAPIURL(url),                 // optional: GitHub Enterprise Server, or a test double
    updates.WithHTTPClient(client),                // optional: custom HTTP client
    updates.WithApplier(customApplier),            // optional: custom binary replacement
    updates.WithIncludePrereleases(false),         // optional: static fallback if no settings
    updates.WithPublicKey(publicKeyText),          // optional: minisign public key for signature verification
    updates.WithSkipVerification(),                // optional: skip verification (dev only)
    updates.WithAllowDowngrade(),                  // optional: permit installing an older release (see "Downgrade protection")
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
import "github.com/jrschumacher/wails-kit/v2/settings"

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
| `updates:managed` | `ManagedPayload{Method, Instructions}` | App installed via package manager; self-update blocked |

## Managed install detection

`ApplyUpdate` detects when the app was installed via a package manager and returns an `ErrUpdateManaged` error instead of attempting to replace the binary. An `updates:managed` event is emitted with instructions for the user.

Currently detected:
- **Homebrew Cask** (macOS) — detects `/usr/local/Caskroom/` and `/opt/homebrew/Caskroom/` paths

The frontend can listen for the `updates:managed` event to show appropriate UI (e.g., "Update via `brew upgrade --cask myapp`" instead of a self-update button).

You can also call `DetectInstallMethod()` directly to check the install method before offering self-update UI at all:

```go
if method := updates.DetectInstallMethod(); method != updates.InstallDirect {
    // Show "update via package manager" UI instead of self-update
    fmt.Println(method.UpdateInstructions("myapp"))
}
```

## Error codes

| Code | User message |
|------|-------------|
| `update_check` | Unable to check for updates. Please try again later. |
| `update_download` | Failed to download the update. Please try again. |
| `update_apply` | Failed to install the update. Please try again. |
| `update_verify` | Update signature verification failed. The download may be corrupted or tampered with. |
| `update_managed` | This app is managed by a package manager. Please update through your package manager instead. |

## Security model

Summary of the defenses this package implements against a compromised/rolled-back release feed and against a local attacker on the same machine:

- **Downgrade protection.** `CheckForUpdate` never caches a release that is not strictly newer than the current version — a rolled-back or compromised feed cannot silently poison a previously cached, genuinely newer release with an older one. `DownloadUpdate` and `ApplyUpdate` each independently re-check newness again before acting, rather than trusting a single check performed earlier. A valid signature does not imply a *current* build: an attacker who can make the feed serve an old, legitimately-signed release with known, already-patched vulnerabilities can trick a user into "upgrading" backwards. Opt out only for an explicit, user-initiated reinstall/downgrade flow via `WithAllowDowngrade()`.
- **Private staging directory.** Downloads are staged under the app's private cache directory (`appdirs.Cache()/updates/`, under the user's home directory — e.g. `~/Library/Caches/<app>` on macOS, `~/.cache/<app>` on Linux) rather than the shared OS temp directory. On Linux, `/tmp` is world-writable (mode `1777`); a local attacker can pre-create a subdirectory there and own it before this process ever runs, and `os.MkdirAll` succeeds silently against a directory that already exists regardless of who created it or with what permissions. This package additionally verifies (on POSIX) that the staging directory is owned by the current user and not group/world-accessible, both when it's created for a download and again for archive extraction.
- **Re-verify immediately before extract.** `DownloadUpdate` and `ApplyUpdate` are separate, user-triggered steps with an unbounded gap between them. `ApplyUpdate` re-verifies the staged file's signature immediately before extracting it, not just once at download time — closing the window where a local attacker who gains write access to the staging directory between the two calls could swap the file.
- **Fail-loud, not fail-open, key handling.** A public key passed to `WithPublicKey` is parsed and validated when you call `NewService`, not lazily on first use — a malformed or wrong-length key is rejected immediately with a clear error instead of surfacing later (or, with the previous raw-`ed25519.PublicKey` implementation, **panicking** on a key of the wrong length, since `ed25519.Verify` panics rather than errors on a bad key length). When no key is configured and `WithSkipVerification` isn't set either, a warning is logged on every download — verification being skipped is never silent.
- **Download size cap.** The GitHub API reports each asset's size in the release metadata (`asset.Size`) before any bytes are downloaded. `DownloadAsset` caps the read at that size; a compromised or malicious feed that tries to stream more than it declared is cut off instead of being buffered to disk indefinitely. (Archive decompression is separately capped at 1 GiB.)
- **Private repos use the API asset endpoint.** Confirmed bug, now fixed: `browser_download_url` requires a browser session and returns 404 for API/token-authenticated requests against a private repository. `DownloadAsset` now uses the GitHub API asset endpoint (`Asset.URL`, present on every asset the API returns) with an `Accept: application/octet-stream` header instead, which works for both private and public repos. `Authorization` is safe to send on that request unconditionally: `net/http` strips it automatically when the API 302-redirects to the actual (unauthenticated, signed, short-lived) storage URL.

None of this protects against a compromised *signing key* — see "Generating a keypair" below for what that means in practice.

## Signature verification

Release assets are verified with [minisign](https://jedisct1.github.io/minisign/).
Each asset must have a matching `.minisig` file in the release — for
`myapp_darwin_arm64.tar.gz`, that is `myapp_darwin_arm64.tar.gz.minisig`.

minisign is Ed25519 underneath, but unlike a hand-rolled scheme it has a defined
file format, a maintained CLI, and a signing procedure that is documented
upstream and can be verified independently of this package. That matters: the
previous version of this section documented an `openssl pkeyutl` recipe that
**could not produce a valid signature** — `$(cat …)` mangles binary data, and
OpenSSL 3.x Ed25519 requires `-rawin`. Nobody noticed because nobody ran it. The
recipe below was executed end to end against minisign 0.12, and there is an
interoperability test (`TestVerifySignatureRealMinisignCLI`) pinning a real
CLI-produced signature so this package can never silently drift from the tool.

```go
//go:embed minisign.pub
var minisignPublicKey string

svc, err := updates.NewService(
    updates.WithCurrentVersion(version),
    updates.WithGitHubRepo("myorg", "myapp"),
    updates.WithPublicKey(minisignPublicKey),
)
```

`WithPublicKey` accepts either the full `.pub` file contents or the bare base64
key line.

**Behavior:**
- Verification happens after download and again immediately before extraction,
  so a file swapped in between the two steps is still caught
- A missing `.minisig` asset fails the download; it is never treated as "unsigned, therefore fine"
- An invalid signature deletes the downloaded file and emits an `update_verify` error
- Without `WithPublicKey`, verification is skipped and a warning is logged

### Generating a keypair

```bash
minisign -G -p minisign.pub -s minisign.key
```

Commit `minisign.pub` (it is public, and embedding it is the point). Store
`minisign.key` offline — a password manager, or an encrypted backup you actually
test restoring.

> **The private key is a one-way door.** Lose it and you cannot ship another
> update to anyone who already installed the app: their copy will reject every
> future release, and there is no remote fix, because the fix would itself need
> to be signed. Ship the wrong public key and the same thing happens
> immediately. Back it up before you cut a single release.

### Signing a release

```bash
minisign -S -s minisign.key -m myapp_darwin_arm64.tar.gz -t "myapp 2.0.0"
```

`-t` sets the trusted comment, which minisign signs separately and this package
verifies. It is authenticated data, so it is safe to display; the *untrusted*
comment is not, and must never be shown as though it were.

Verified output:

```
$ minisign -V -p minisign.pub -m myapp_darwin_arm64.tar.gz
Signature and comment signature verified
Trusted comment: myapp 2.0.0
```

In CI, keep the secret key in a repository secret and write it to a file for the
signing step. A key with no password (`minisign -G -W`) is appropriate for
unattended CI; the protection then comes from the secret store, not the
passphrase.

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
