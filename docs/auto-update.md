# Auto-update for Wails v3 applications

**Audience: a coding agent setting up a new Wails v3 project.** Follow this
top to bottom and you end with a signed, notarized, auto-updating macOS app,
plus working Windows and Linux builds.

Verified against `wailsapp/wails` at tag **`v3.0.0-beta.4`**. Every API
signature, field name, event string and exit code below was read out of that
source tree. Where behaviour is surprising, the surprise is stated rather than
smoothed over.

---

## 0. The decision this document encodes

**wails-kit ships no update code.** No Sparkle CGO bridge, no wrapper package,
no `wailskit/update` shim. You wire `app.Updater` directly.

Why: Wails v3 already ships `v3/pkg/updater`, and it is *framework-integrated*,
not a library you bolt on.

- `application.New` constructs the updater and assigns it to `app.Updater`
  (`v3/pkg/application/application.go:518`).
- `application.New` also calls `updater.HandleHelperMode()` at process start
  (`application.go:48`), which is what makes restart-into-the-new-version work
  with zero wiring on your side.

A wrapper in wails-kit would add a beta Wails dependency to **every** consumer's
`go.mod` in order to re-export an API that already needs no wiring. So the kit
stays Wails-free, and the deliverable is this document plus
[`examples/starter/`](../examples/starter/).

**Invariant: the wails-kit root module MUST NOT depend on
`github.com/wailsapp/wails/v3`.** `go list -m all` in the repo root must return
zero `wailsapp` lines. `examples/starter/` is a separate module for exactly
this reason.

---

## 1. Requirements

| Requirement | Value |
|---|---|
| Wails | `github.com/wailsapp/wails/v3 v3.0.0-beta.4` or later |
| Go | `1.25.0` or later (Wails v3's own `go.mod` declares `go 1.25.0`) |
| Codesigning + notarization | **macOS runner + paid Apple Developer account.** Irreducible. See §7. |
| Update artifact | One archive or binary per platform/arch, published where a provider can find it |

Add to your app module (not to wails-kit):

```
go get github.com/wailsapp/wails/v3@v3.0.0-beta.4
```

---

## 2. The one-way door: signing keys

Read this before writing any code.

The public key you compile into your binary is the **only** trust anchor for
update verification. `updater.Config.PublicKey` is pinned at build time and the
release feed has no say in it — a feed cannot substitute its own key. That is
the entire security property, and it is also the trap:

> **Losing the private key, or shipping a public key that does not match it,
> permanently strands every installed copy of your application.** There is no
> remote fix. Those installs will reject every future update, forever, because
> the only thing that could authorize a new key is a signed update they will not
> accept. Recovery means telling every user to download and reinstall by hand.

Consequences, all mandatory:

1. **Back the private key up offline**, in at least two places, before you ship
   a single build. A password manager entry and a printed/paper copy is not
   excessive.
2. **Never commit the private key.** Add `*-private-key.pem` to `.gitignore`
   now, not later.
3. **Store it in CI as an encrypted secret**, never as a repo file.
4. **Verify the pair before the first release.** Sign a throwaway artifact and
   verify it with the public key you are about to compile in. Do it once; it
   costs a minute and it is the only check that catches a mismatched pair before
   users have it.
5. **Do not rotate casually.** Rotating means shipping a release signed with the
   *old* key whose new binary contains the *new* public key, waiting for that
   release to reach effectively everyone, and only then signing with the new key.
   Rotate too fast and you strand whoever skipped the bridging release.

### Key generation

```bash
# Private key — back this up offline, never commit it.
openssl genpkey -algorithm ed25519 -out update-private-key.pem

# Public key — this one gets compiled into the app.
openssl pkey -in update-private-key.pem -pubout -out update-public-key.pem
```

`Config.PublicKey` accepts either form (`parseEd25519Public`, `verify.go:126`):

- a raw 32-byte ed25519 public key, or
- PKIX DER, or PEM-wrapped PKIX — which is what the command above produces.

PEM is the practical choice: embed the file and pass the bytes.

```go
//go:embed update-public-key.pem
var updatePublicKey []byte
```

---

## 3. Wiring the updater

`app.Updater` exists as soon as `application.New` returns. Configure it with
`Init`, which may be called **once per process** — a second call returns
`updater.ErrAlreadyConfigured` (`updater.go:108`).

### 3a. GitHub Releases provider

```go
package main

import (
	_ "embed"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

//go:embed update-public-key.pem
var updatePublicKey []byte

// MUST equal CFBundleShortVersionString and CFBundleVersion. See §8.
const version = "1.2.3"

func configureUpdater(app *application.App) error {
	provider, err := github.New(github.Config{
		Repository:    "owner/repo",     // required, "owner/repo" form
		Token:         "",               // required for private repos; raises rate limit otherwise
		Prerelease:    false,            // true walks /releases (includes prereleases)
		BaseURL:       "",               // GitHub Enterprise: "https://<host>/api/v3"
		ChecksumAsset: "checksums.txt",  // sibling asset parsed into Release.Verification
		// AssetMatcher: nil -> github.DefaultAssetMatcher
		// HTTPClient:   nil -> 30s-timeout client
	})
	if err != nil {
		return err
	}

	return app.Updater.Init(updater.Config{
		CurrentVersion: version,                          // required
		Providers:      []updater.Provider{provider},     // required, tried in order
		PublicKey:      updatePublicKey,                  // the only trust anchor
		CheckInterval:  6 * time.Hour,                    // 0 disables background polling
		Platform:       "",                               // "" -> runtime.GOOS
		Arch:           "",                               // "" -> runtime.GOARCH
		Channel:        "",                               // "" -> provider default
		Window:         nil,                              // nil -> built-in update window
	})
}
```

`updater.Config` field-by-field (`v3/pkg/updater/config.go`):

| Field | Type | Notes |
|---|---|---|
| `CurrentVersion` | `string` | **Required.** Same string you tag releases with, no `v` prefix. |
| `Providers` | `[]Provider` | **Required, non-empty, no nil entries.** Tried in order; a provider returning `(nil, nil)` means "up to date" and stops the chain. Fallback is for *unreachable*, not *disagreeing*. |
| `PublicKey` | `[]byte` | Unset ⇒ digest-only releases still install, but any release carrying a signature is **rejected**. |
| `CheckInterval` | `time.Duration` | Non-zero starts a background poller that runs `CheckAndInstall` each tick. |
| `Platform`, `Arch` | `string` | Default to `runtime.GOOS` / `runtime.GOARCH`. |
| `Channel` | `string` | Passed to the provider. The appcast provider filters `sparkle:channel` on it. |
| `Window` | `WindowOption` | `nil` ⇒ built-in window. See §3c. |

#### DefaultAssetMatcher: your asset filenames are load-bearing

`github.DefaultAssetMatcher` (`providers/github/github.go:395`) selects a
release asset by **lowercased substring match on the filename**. It requires
the name to contain:

- `req.Platform`, which is `runtime.GOOS` — literally **`darwin`**, `windows`,
  `linux`. Not `macos`, not `mac`, not `osx`.
- `req.Arch`, which is `runtime.GOARCH` — `arm64`, `amd64`, `386`. Aliases are
  accepted: `x86_64`/`x64` for `amd64`, `aarch64` for `arm64`.

It skips assets ending `.sig`, `.asc`, checksum-looking names
(`.sha256`, `.sha512`, `.sums`, `.checksum`, `.checksums`), and installer names
(`*-installer.*`, `*_installer.*`, `installer.exe`).

If nothing matches, `Check` returns an error naming the release and platform. So:

```
myapp-darwin-arm64.zip     ✓
myapp-darwin-amd64.zip     ✓
myapp-windows-amd64.zip    ✓
myapp-linux-amd64.tar.gz   ✓
MyApp-macOS-arm64.zip      ✗  "macos" is not "darwin" — never matches
MyApp-1.2.3.dmg            ✗  no platform, no arch
```

Pick names that satisfy the matcher, or supply your own `AssetMatcher`.

### 3b. Appcast (Sparkle 2) provider

```go
import "github.com/wailsapp/wails/v3/pkg/updater/providers/appcast"

provider, err := appcast.New(appcast.Config{
	URL:     "https://updates.example.com/appcast.xml", // required
	Channel: "",                                        // "" matches every item
	// HTTPClient: nil -> 30s client wrapped to strip Authorization across hosts
})
```

Minimal, complete appcast feed. The provider parses exactly these
(`appcast.go:210-253`):

```xml
<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel>
    <title>MyApp</title>
    <item>
      <title>Version 1.2.3</title>
      <description><![CDATA[<h3>Fixed</h3><ul><li>Crash on launch</li></ul>]]></description>
      <pubDate>Mon, 03 Aug 2026 12:00:00 +0000</pubDate>
      <sparkle:shortVersionString>1.2.3</sparkle:shortVersionString>
      <sparkle:version>1230</sparkle:version>
      <sparkle:channel>stable</sparkle:channel>
      <sparkle:os>macos</sparkle:os>
      <enclosure
        url="https://downloads.example.com/MyApp-1.2.3-darwin-arm64.zip"
        length="18234881"
        type="application/octet-stream"
        sparkle:edSignature="BASE64_SIGNATURE_HERE" />
    </item>
  </channel>
</rss>
```

Element semantics as implemented:

- Version comes from `sparkle:shortVersionString`, falling back to
  `sparkle:version`.
- `sparkle:channel` — an item with **no** channel never matches a configured
  `Channel`. Items opt *in*. Leave `Config.Channel` empty to accept everything.
- `sparkle:os` may sit on `<item>` or on `<enclosure>`. `macos`/`mac`/`osx` all
  map to `darwin`; anything else must equal `runtime.GOOS` exactly.
- Ties in version keep document order (first wins), matching Sparkle.
- `<enclosure url>` is the download. Its filename and extension become
  `Artifact.Filename` / `Artifact.Filetype`.
- `sparkle:edSignature` populates `Release.Verification` with
  `SignatureAlgo: "ed25519"`. Base64, standard or raw (unpadded) — both decode.
- **`sparkle:dsaSignature` (Sparkle 1 DSA) is not supported.** Rotate to EdDSA.

> ⚠️ **Signature payload differs from Sparkle's own tooling.** Wails computes a
> streaming SHA-256 of the artifact during download and the raw `ed25519`
> verifier checks the signature against **that digest**, not against the file
> bytes (`verify.go:112`, `download.go:53`). Sparkle's `sign_update` signs the
> **file bytes** with pure Ed25519. A feed signed by Sparkle's tooling will
> therefore fail verification here. Generate `sparkle:edSignature` as
> `ed25519(sha256(artifact))` — see §5 — or use `ed25519ph`, where Wails streams
> SHA-512 and verifies with `crypto.SHA512` prehash, matching the standard
> Ed25519ph construction. If you are migrating an existing Sparkle feed, this is
> the thing that will bite you.

Other providers, same shape: `providers/endpoint` (your own JSON manifest) and
`providers/keygen` (keygen.sh).

### 3c. The update window

`Config.Window` is a closed interface with exactly three inhabitants
(`window.go`):

| Value | Behaviour |
|---|---|
| `nil` or `&updater.BuiltinWindow{}` | Framework's default update window, complete and functional. Opens at 348×161 and grows to 520×540 when a release is found. |
| `updater.WindowNone` | Headless. Nothing opens; subscribe to events and build your own UI. |
| `updater.BYOWindow(w)` | Drive a window you created yourself. |

`BuiltinWindow` fields: `HTML` (replaces the template entirely), `CSS`
(appended to the default template), `Options` (`Title`, `Width`, `Height`,
`Frameless`, `AlwaysOnTop`, `DisableResize`, `InitialHTML`).

> `config.go` carries a stale comment saying `Window` is "not yet wired in v1".
> It is wired: `CheckAndInstall` opens a session from it
> (`updater.go`, `window_lifecycle.go`). Trust the code, not that comment.

### 3d. Methods and events

```go
app.Updater.Init(cfg)                     // once per process; ErrAlreadyConfigured after
app.Updater.Check(ctx)                    // (*Release, nil) | (nil, nil)=up-to-date | (nil, err)
app.Updater.DownloadAndInstall(ctx)       // download + verify + stage
app.Updater.CheckAndInstall(ctx)          // window + Check + DownloadAndInstall — the button you want
app.Updater.Restart(ctx)                  // spawn helper, swap, relaunch; ErrNotReady if nothing staged
app.Updater.State() updater.State
app.Updater.CurrentVersion() string
app.Updater.DownloadedPath() string
app.Updater.SkipVersion(v string)         // subsequent Checks treat v as "no update"
app.Updater.SkippedVersion() string
app.Updater.StopPeriodicCheck()           // cancels the CheckInterval poller, blocks until stopped
```

States: `unconfigured`, `idle`, `checking`, `up-to-date`, `available`,
`downloading`, `verifying`, `installing`, `ready`, `error`.

Events — subscribe in Go with `app.Event.On(name, func(*application.CustomEvent))`,
in JS with `Events.On(name, cb)` after `import { Events, Updater } from "@wailsio/runtime"`:

| Constant | String | Payload |
|---|---|---|
| `EventCheckStarted` | `wails:updater:check-started` | nil |
| `EventUpdateAvailable` | `wails:updater:update-available` | `*Release` |
| `EventNoUpdate` | `wails:updater:no-update` | nil |
| `EventDownloadStarted` | `wails:updater:download-started` | `*Release` |
| `EventDownloadProgress` | `wails:updater:download-progress` | `Progress{written,total,rate,provider}` |
| `EventDownloadComplete` | `wails:updater:download-complete` | `*Release` (before verification) |
| `EventVerifying` | `wails:updater:verifying` | `*Release` |
| `EventInstalling` | `wails:updater:installing` | `*Release` |
| `EventUpdateReady` | `wails:updater:update-ready` | `*Release` |
| `EventError` | `wails:updater:error` | `ErrorInfo{stage,message,provider}` |
| `EventMeta` | `wails:updater:meta` | `Meta{currentVersion,skippedVersion}` |

User actions the window emits back: `wails:updater:user:install`, `:restart`,
`:skip`, `:remind`, `:cancel`.

---

## 4. Verification: non-negotiable, and what each mistake does

`runVerification` (`verify.go:73`) is the whole contract:

```
Verification == nil                                → nothing checked, artifact installs
Digest present, mismatch                           → "updater: digest mismatch", update aborts
Signature present, SignatureAlgo empty             → "signature present but signatureAlgo missing", FAILS CLOSED
Signature present, Config.PublicKey empty          → "signature requires a public key but none configured", FAILS CLOSED
Signature present, key present, verify fails       → "<algo> signature did not verify", update aborts
```

Supported `SignatureAlgo` values (`verifierRegistry`, `verify.go:58`):

| Algo | Digest streamed | Signature over | Key format |
|---|---|---|---|
| `ed25519` | SHA-256 (or `DigestAlgo`) | the **digest** | raw 32 bytes, or PKIX DER/PEM |
| `ed25519ph` | SHA-512, forced | SHA-512 digest, `crypto.SHA512` prehash | same |
| `ecdsa-p256` | SHA-256 (or `DigestAlgo`) | the digest | PKIX DER/PEM, P-256 only |

`ecdsa-p256` accepts raw `r||s` (64 bytes) or ASN.1 DER signatures.

### Misconfigurations and their runtime behaviour

| What you got wrong | What actually happens |
|---|---|
| No `PublicKey`, feed ships signatures | Every update rejected at the verify stage. `EventError{stage:"verify"}`. Users silently stop receiving updates. |
| No `PublicKey`, feed ships only digests | Updates install. Integrity checked, **authenticity not**. Anyone who can rewrite your feed or MITM your CDN can ship arbitrary code. Do not ship this. |
| `PublicKey` set, feed ships nothing | `Verification == nil` ⇒ nothing is checked and the artifact installs. **The pinned key is not a floor.** Verify your feed actually carries signatures. |
| Public key does not match private key | Every update rejected forever. See §2. |
| `signatureAlgo` omitted from a manifest carrying a signature | Fails closed, by design — a garbled field must not silently downgrade to digest-only. |
| Sparkle-tool-generated `edSignature` | Fails to verify: Sparkle signs file bytes, Wails verifies over the digest. See §3b. |

**Rule: set `PublicKey`, and make every published release carry a signature.**
The one combination that silently installs unauthenticated code is "key
configured, feed carries no `Verification`".

---

## 5. Signing an artifact

Ed25519 over the SHA-256 digest, matching the `ed25519` verifier:

```bash
#!/usr/bin/env bash
# sign-artifact.sh <artifact> <private-key.pem>
set -euo pipefail
artifact="$1"; key="$2"

# The updater verifies the signature against the SHA-256 digest of the bytes,
# not against the bytes themselves. Sign the raw digest.
openssl dgst -sha256 -binary "$artifact" > "$artifact.sha256.bin"
openssl pkeyutl -sign -inkey "$key" -rawin -in "$artifact.sha256.bin" \
  -out "$artifact.sig"

# base64 for sparkle:edSignature / a JSON manifest
base64 < "$artifact.sig" | tr -d '\n'
echo
```

For the GitHub provider's `ChecksumAsset`, publish a `checksums.txt` in the
familiar `sha256sum` format alongside the release:

```bash
shasum -a 256 myapp-darwin-arm64.zip myapp-windows-amd64.zip > checksums.txt
```

That gives integrity. It does **not** give authenticity — pair it with a
signature that verifies under your pinned key.

---

## 6. Version numbers must match, or nothing works

Three places must agree, exactly:

1. `updater.Config.CurrentVersion` in Go.
2. `CFBundleShortVersionString` in `build/darwin/Info.plist`.
3. `CFBundleVersion` in `build/darwin/Info.plist`.

In a `wails3 init` project, both plist keys are generated from `info.version` in
`build/config.yml`:

```yaml
# build/config.yml
info:
  companyName: "My Company"
  productName: "My Product"
  productIdentifier: "com.mycompany.myproduct"
  version: "1.2.3"   # ← regenerates both plist keys
```

After editing it: `wails3 task common:update:build-assets` (this **overwrites**
hand edits to the generated assets, so make version changes here, not in the
plist).

Resulting `build/darwin/Info.plist` fragment:

```xml
<key>CFBundleShortVersionString</key>
    <string>1.2.3</string>
<key>CFBundleVersion</key>
    <string>1.2.3</string>
```

### Why the mismatch is nasty

Nothing reads the plist back to cross-check `CurrentVersion`. The updater
compares `Config.CurrentVersion` against the feed and nothing else. So:

- **`CurrentVersion` lower than what actually shipped** → every check finds an
  "update", downloads it, installs it, restarts — and the new process reports
  the same low version. The app updates itself in a loop, forever, and nothing
  logs an error.
- **`CurrentVersion` higher than the feed's newest** → `semver.IsNewer` is false,
  the provider returns `(nil, nil)`, and the app reports "up to date" forever.
  A genuinely available update is never offered, and again nothing logs.

Both are silent. **Generate the Go constant from `build/config.yml`** so all
three genuinely share one source. Checking the generated file in makes local
builds work with no extra step, and CI regenerates and diffs it:

```bash
#!/usr/bin/env bash
# scripts/gen-version.sh — run after editing build/config.yml.
set -euo pipefail
version=$(grep -E '^\s+version:' build/config.yml | head -1 | sed -E 's/.*"(.*)".*/\1/')
cat > version_gen.go <<EOF
// Code generated by scripts/gen-version.sh. DO NOT EDIT.
package main

// version is generated from build/config.yml, which also generates
// CFBundleShortVersionString and CFBundleVersion. One source of truth.
const version = "$version"
EOF
gofmt -w version_gen.go
```

CI then fails loudly on drift rather than shipping a silent mismatch:

```bash
./scripts/gen-version.sh && git diff --exit-code version_gen.go
```

`-ldflags "-X main.version=$VERSION"` also works, but the Wails build tasks own
`BUILD_FLAGS` (see `build/darwin/Taskfile.yml`), so overriding it from CI means
reproducing the production flags by hand. Generating a file avoids that.

---

## 7. Release workflow (GitHub Actions)

### The friction you cannot remove

macOS codesigning and notarization require:

- a **macOS runner** — `codesign`, `xcrun notarytool`, and `xcrun stapler` are
  macOS-only tools; and
- a **paid Apple Developer Program account** ($99/yr) for a Developer ID
  Application certificate.

This is true regardless of which updater you use — Sparkle, Wails, or a
hand-rolled one. It is Gatekeeper policy, not a framework choice. Self-hosted
Linux runners can build and release everything else; the macOS signing job
cannot move off macOS.

If you do not sign and notarize, macOS Gatekeeper will refuse to open the
downloaded app, and — because the updater swaps the bundle on disk — an
unsigned or broken-signature replacement can leave users with an app that
stops launching after an "update". This step is not optional for a shipped
macOS app.

### Required secrets

| Secret | What it is |
|---|---|
| `MACOS_CERTIFICATE` | Developer ID Application `.p12`, base64-encoded |
| `MACOS_CERTIFICATE_PWD` | Password for that `.p12` |
| `KEYCHAIN_PASSWORD` | Any random string; names a temporary keychain |
| `APPLE_ID` | Apple ID email for notarization |
| `APPLE_APP_PASSWORD` | App-specific password from appleid.apple.com |
| `APPLE_TEAM_ID` | 10-character team ID |
| `UPDATE_PRIVATE_KEY` | The ed25519 private key PEM from §2 |

### `.github/workflows/release.yml`

```yaml
name: release

on:
  push:
    tags: ['v*']

permissions:
  contents: write

jobs:
  # macOS MUST run on a macOS runner. Self-hosted runners are fine for the
  # other platforms, but codesign/notarytool/stapler do not exist elsewhere.
  macos:
    runs-on: macos-14
    strategy:
      matrix:
        arch: [arm64, amd64]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.25' }

      - name: Resolve version
        id: v
        run: echo "version=${GITHUB_REF_NAME#v}" >> "$GITHUB_OUTPUT"

      - name: Build .app bundle
        env:
          VERSION: ${{ steps.v.outputs.version }}
        run: |
          go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.4
          # One source of truth: the tag drives build/config.yml, which
          # regenerates Info.plist, which gen-version.sh mirrors into Go.
          # See §6 — a mismatch here is silent in both directions.
          sed -i '' "s/^  version: .*/  version: \"$VERSION\"/" build/config.yml
          wails3 task common:update:build-assets
          ./scripts/gen-version.sh
          git diff --exit-code version_gen.go || \
            echo "version_gen.go regenerated for $VERSION"
          # ARCH is the task variable; setting GOARCH in the environment does
          # NOT work, because darwin:build sets GOARCH from ARCH itself.
          # darwin:package builds with PRODUCTION=true and emits bin/MyApp.app.
          wails3 task darwin:package ARCH=${{ matrix.arch }}

      - name: Import signing certificate
        env:
          CERT: ${{ secrets.MACOS_CERTIFICATE }}
          CERT_PWD: ${{ secrets.MACOS_CERTIFICATE_PWD }}
          KEYCHAIN_PWD: ${{ secrets.KEYCHAIN_PASSWORD }}
        run: |
          echo "$CERT" | base64 --decode > cert.p12
          security create-keychain -p "$KEYCHAIN_PWD" build.keychain
          security default-keychain -s build.keychain
          security unlock-keychain -p "$KEYCHAIN_PWD" build.keychain
          security import cert.p12 -k build.keychain -P "$CERT_PWD" \
            -T /usr/bin/codesign
          security set-key-partition-list -S apple-tool:,apple:,codesign: \
            -s -k "$KEYCHAIN_PWD" build.keychain
          rm cert.p12

      - name: Codesign
        env:
          TEAM_ID: ${{ secrets.APPLE_TEAM_ID }}
        run: |
          # darwin:create:app:bundle ad-hoc signs the bundle
          # (`codesign --force --deep --sign -`). That signature is NOT valid
          # for distribution, so this step must run after packaging and must
          # pass --force to replace it.
          #
          # --options runtime (hardened runtime) is REQUIRED for notarization.
          # --deep is deprecated for signing; sign nested code first if any.
          codesign --force --options runtime --timestamp \
            --sign "Developer ID Application: ($TEAM_ID)" \
            bin/MyApp.app
          codesign --verify --deep --strict --verbose=2 bin/MyApp.app

      - name: Notarize and staple
        env:
          APPLE_ID: ${{ secrets.APPLE_ID }}
          APPLE_APP_PASSWORD: ${{ secrets.APPLE_APP_PASSWORD }}
          TEAM_ID: ${{ secrets.APPLE_TEAM_ID }}
        run: |
          # notarytool wants an archive, not a bundle directory.
          ditto -c -k --keepParent bin/MyApp.app notarize.zip
          xcrun notarytool submit notarize.zip \
            --apple-id "$APPLE_ID" \
            --password "$APPLE_APP_PASSWORD" \
            --team-id "$TEAM_ID" \
            --wait
          # Staple the ticket to the BUNDLE, then re-zip. Stapling the zip does
          # nothing; a user offline on first launch needs the ticket in-bundle.
          xcrun stapler staple bin/MyApp.app
          xcrun stapler validate bin/MyApp.app

      - name: Package update artifact
        env:
          VERSION: ${{ steps.v.outputs.version }}
        run: |
          # Filename MUST contain "darwin" and the GOARCH for
          # github.DefaultAssetMatcher to select it. See §3a.
          ditto -c -k --keepParent bin/MyApp.app \
            "myapp-darwin-${{ matrix.arch }}.zip"

      - name: Sign update artifact
        env:
          KEY: ${{ secrets.UPDATE_PRIVATE_KEY }}
        run: |
          printf '%s' "$KEY" > private.pem
          f="myapp-darwin-${{ matrix.arch }}.zip"
          openssl dgst -sha256 -binary "$f" > "$f.digest"
          openssl pkeyutl -sign -inkey private.pem -rawin -in "$f.digest" \
            -out "$f.sig"
          shasum -a 256 "$f" > "$f.sha256"
          rm private.pem "$f.digest"

      - uses: actions/upload-artifact@v4
        with:
          name: macos-${{ matrix.arch }}
          path: |
            myapp-darwin-*.zip
            myapp-darwin-*.sig
            myapp-darwin-*.sha256

  # Everything that is not macOS can run wherever you like, including
  # self-hosted runners.
  others:
    strategy:
      matrix:
        include:
          - { os: windows-latest, goos: windows, arch: amd64, ext: zip }
          - { os: ubuntu-latest,  goos: linux,   arch: amd64, ext: tar.gz }
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.25' }
      - name: Build and package
        shell: bash
        run: |
          V="${GITHUB_REF_NAME#v}"
          go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.4
          sed -i.bak "s/^  version: .*/  version: \"$V\"/" build/config.yml
          wails3 task common:update:build-assets
          ./scripts/gen-version.sh
          wails3 task package
          # Filename must contain the GOOS and GOARCH.
          name="myapp-${{ matrix.goos }}-${{ matrix.arch }}"
          if [ "${{ matrix.ext }}" = "zip" ]; then
            7z a "$name.zip" ./bin/*
          else
            tar czf "$name.tar.gz" -C bin .
          fi
      - uses: actions/upload-artifact@v4
        with: { name: ${{ matrix.goos }}-${{ matrix.arch }}, path: myapp-* }

  publish:
    needs: [macos, others]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with: { path: dist, merge-multiple: true }
      - name: Build checksums.txt
        run: |
          cd dist
          # Exclude sidecars so the manifest lists only real artifacts.
          shasum -a 256 $(ls | grep -Ev '\.(sig|sha256|txt)$') > checksums.txt
      - uses: softprops/action-gh-release@v2
        with:
          files: dist/*
          fail_on_unmatched_files: true
```

Windows codesigning (`signtool`, an EV or OV certificate) is a separate concern
with the same shape. Unsigned Windows binaries trigger SmartScreen warnings but
do still run and do still self-update, so it is a UX problem rather than a
functional one.

---

## 8. Platform reality: who can actually self-update

The swap is performed by a helper process (`updater/helper.go`). `Restart`
re-execs the current binary with `WAILS_UPDATER_HELPER=1` and supporting env
vars; `updater.HandleHelperMode()` at process start detects that, waits up to
30s for the parent PID to exit, backs up the target, replaces it (Unix: unlink
+ rename; Windows: move-aside), restores the original file mode, relaunches, and
cleans up. There is **no privilege elevation anywhere in that path.**

That single fact determines the platform matrix:

| Platform / install location | Self-update? | Why |
|---|---|---|
| macOS `.app` in `/Applications` or `~/Applications` | ✅ | The bundle is user-writable in normal installs. `bundleTarget` walks the path to the `.app` and replaces the whole bundle (`updater_darwin.go`). |
| macOS `.app` under `/Applications` on a managed/MDM Mac | ⚠️ | Fails if the admin has locked it. Degrade to notify-only. |
| Windows, per-user install (`%LOCALAPPDATA%`) | ✅ | Writable without elevation. |
| **Windows, MSI install under `Program Files`** | ❌ | Needs admin. The helper has no elevation path; the swap fails and the backup is restored. |
| Windows Store / MSIX | ❌ | Store-managed. |
| Linux, tarball or AppImage in `$HOME` | ✅ | Writable. |
| **Linux, apt / rpm / dnf install** | ❌ | Binary lives under `/usr/bin`, owned by root and by the package manager. |
| **Linux, Flatpak / Snap** | ❌ | Sandboxed and store-managed. |

### Detecting it, and degrading

Probe writability of the swap target before offering an in-place update:

```go
import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// canSelfUpdate reports whether this process could replace its own
// executable (or, on macOS, its .app bundle) without elevation.
func canSelfUpdate() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return false
	}

	target := exe
	if runtime.GOOS == "darwin" {
		// Mirror updater.bundleTarget: replace the whole .app, not the binary.
		parts := strings.Split(filepath.Clean(exe), string(os.PathSeparator))
		for i, p := range parts {
			if strings.HasSuffix(p, ".app") {
				target = string(os.PathSeparator) + filepath.Join(parts[1:i+1]...)
				break
			}
		}
	}

	if runtime.GOOS == "linux" && isPackageManaged(target) {
		return false
	}

	// The helper renames within the parent directory, so that is what must be
	// writable — not the file itself.
	probe, err := os.CreateTemp(filepath.Dir(target), ".update-probe-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	probe.Close()
	os.Remove(name)
	return true
}

// isPackageManaged reports whether target looks like it came from a distro
// package or a sandboxed store, both of which forbid self-replacement.
func isPackageManaged(target string) bool {
	if os.Getenv("FLATPAK_ID") != "" || os.Getenv("SNAP") != "" ||
		os.Getenv("container") == "flatpak" {
		return true
	}
	for _, prefix := range []string{"/usr/", "/opt/", "/snap/", "/var/lib/flatpak/"} {
		if strings.HasPrefix(target, prefix) {
			return true
		}
	}
	return false
}
```

Then degrade to **notify-only**: keep checking (so users learn a release exists)
but never download or install.

```go
if canSelfUpdate() {
	cfg.CheckInterval = 6 * time.Hour
	err = app.Updater.Init(cfg)
} else {
	// Headless: the updater still drives Check, but nothing opens and
	// nothing installs. Poll yourself and show your own "a new version is
	// available, download it here" affordance.
	cfg.Window = updater.WindowNone
	cfg.CheckInterval = 0
	err = app.Updater.Init(cfg)

	go func() {
		rel, err := app.Updater.Check(context.Background())
		if err != nil || rel == nil {
			return
		}
		notifyUserToDownload(rel.Version) // your UI; do NOT call DownloadAndInstall
	}()
}
```

Never call `DownloadAndInstall` on a non-self-updating install: the download
succeeds, verification succeeds, `Restart` spawns a helper, the app quits, the
swap fails, the backup is restored — and the user watches their app close and
reopen at the same version with no explanation.

---

## 9. Silent failure modes checklist

Work through these before shipping.

### Version mismatch
Covered in §6. Silent in both directions. **Derive all three from one variable.**

### Unsigned feed item
`Verification == nil` means nothing is checked, even with `PublicKey` set. Add a
CI assertion that every published release carries a `.sig` and that the feed
references it.

### The feed URL is permanent
`github.Config.Repository`, `appcast.Config.URL`, `endpoint` URLs — all are
compiled into shipped binaries. **A binary in the field will never look
anywhere else.** Moving the feed silently strands every install that predates
the move. Front it with a stable redirect (`updates.example.com/appcast.xml` →
wherever) from day one, and treat that hostname as permanent infrastructure.
Renaming the GitHub repo is the same mistake in a different costume — GitHub's
redirect covers the web UI and git, but do not bet your update channel on it.

### Gatekeeper re-evaluation after the swap
On macOS the helper replaces the whole `.app` bundle and relaunches with `open`.
The replacement is a fresh bundle that Gatekeeper evaluates on first launch. If
it is unsigned, signed with a different identity, or notarized-but-unstapled and
the machine is offline, launch is refused — and the old bundle is gone. Symptom:
"the update killed my app."

- Sign **and** notarize **and** `xcrun stapler staple` the `.app` **before**
  zipping it for release. Stapling puts the ticket inside the bundle so an
  offline first launch still passes.
- Keep the signing identity stable across releases.
- `ditto -c -k --keepParent` preserves bundle structure and extended attributes;
  plain `zip -r` can mangle symlinks inside a `.app`. Use `ditto`.
- The `com.apple.quarantine` xattr is applied by the *browser* to user
  downloads. The updater's own download does not carry it, so a swapped bundle
  is not quarantined — the risk is the signature, not the xattr. Verify with
  `spctl -a -vvv -t install /Applications/MyApp.app` on a clean machine.

### The helper leaves a log — read it
`Restart` writes to `$TMPDIR/wails-update-<pid>.log`. When an update "did
nothing", that file has the answer. Exit codes from `runHelperSwap`:

| Code | Meaning |
|---|---|
| 10 | target path does not exist |
| 11 | staged artifact does not exist |
| 12 | backup failed |
| 13 | all 20 swap attempts failed; backup restored |
| 14 | swap failed **and** restore failed (bad state) |
| 15 | relaunch failed; backup restored |
| 16 | relaunch failed **and** restore failed |
| 17 | parent did not exit within 30s — swap aborted deliberately |

Code 17 usually means a webview dialog blocked shutdown. Proceeding would launch
a second instance on macOS or grind against the file lock on Windows, so the
helper refuses.

### Archive artifacts are unpacked, plain binaries are not
`DownloadAndInstall` extracts `.zip` / `.tar.gz` before staging, so the helper
gets a real binary or `.app` to rename into place. Ship the macOS `.app` inside
a `.zip`. Shipping a `.dmg` will not work: it is not an archive the updater
unpacks, and the helper would try to replace your bundle directory with a disk
image file.

### A provider returning `(nil, nil)` short-circuits the chain
Multiple providers are a *fallback for unreachable*, not a vote. The first
provider to answer "up to date" ends the check — a later provider that knows
about a newer release is never consulted. Order accordingly.

---

## 10. Settings integration

Update preferences compose with the kit's `settings` package as an ordinary
`settings.Group`. This adds **no** Wails dependency to wails-kit: the group is
plain data, and the code that reads it into `updater.Config` lives in your app.

Complete working version:
[`examples/starter/updatesettings.go`](../examples/starter/updatesettings.go).

```go
package main

import (
	"sync/atomic"
	"time"

	"github.com/jrschumacher/wails-kit/settings"
)

const (
	KeyAutoCheck      = "update.autoCheck"
	KeyChannel        = "update.channel"
	KeyCurrentVersion = "update.currentVersion"
	KeyLastChecked    = "update.lastChecked"
)

const (
	ChannelStable = "stable"
	ChannelBeta   = "beta"
)

type UpdatePrefs struct {
	version     string
	lastChecked atomic.Int64 // Unix seconds; 0 == never
}

func NewUpdatePrefs(version string) *UpdatePrefs { return &UpdatePrefs{version: version} }

func (p *UpdatePrefs) MarkChecked(t time.Time) { p.lastChecked.Store(t.Unix()) }

func (p *UpdatePrefs) Group() settings.Group {
	return settings.Group{
		Key:   "update",
		Label: "Updates",
		Fields: []settings.Field{
			{
				Key:         KeyAutoCheck,
				Type:        settings.FieldToggle,
				Label:       "Check for updates automatically",
				Description: "Takes effect after the next restart.",
				Default:     true,
			},
			{
				Key:         KeyChannel,
				Type:        settings.FieldSelect,
				Label:       "Update channel",
				Description: "Beta receives prereleases. Takes effect after the next restart.",
				Default:     ChannelStable,
				Options: []settings.SelectOption{
					{Label: "Stable", Value: ChannelStable},
					{Label: "Beta", Value: ChannelBeta},
				},
			},
			{Key: KeyCurrentVersion, Type: settings.FieldComputed, Label: "Current version"},
			{Key: KeyLastChecked, Type: settings.FieldComputed, Label: "Last checked"},
		},
		ComputeFuncs: map[string]settings.ComputeFunc{
			KeyCurrentVersion: func(map[string]any) any { return p.version },
			KeyLastChecked: func(map[string]any) any {
				sec := p.lastChecked.Load()
				if sec == 0 {
					return "Never"
				}
				return time.Unix(sec, 0).Format(time.RFC1123)
			},
		},
	}
}
```

Wiring it to `updater.Config`:

```go
svc, err := settings.NewService(
	settings.WithAppName("myapp"),
	settings.WithGroup(llm.LLMSettingsGroup()),
	settings.WithGroup(prefs.Group()),
)
if err != nil {
	return err
}
if c := svc.Corruption(); c != nil {
	// Not an error, but never silent: the user's settings were unreadable
	// and have been reset. Tell them, and where the old file went.
	warnUser(c.Path, c.QuarantinePath)
}

values, err := svc.GetValues()
if err != nil {
	return err
}

cfg := updater.Config{
	CurrentVersion: version,
	Providers:      []updater.Provider{provider},
	PublicKey:      updatePublicKey,
	Channel:        channelOf(values),
}
if autoCheckEnabled(values) {
	cfg.CheckInterval = 6 * time.Hour
}
if err := app.Updater.Init(cfg); err != nil {
	return err
}

// The updater emits this for both manual and background checks.
app.Event.On(updater.EventCheckStarted, func(*application.CustomEvent) {
	prefs.MarkChecked(time.Now())
})
```

Constraints worth knowing before you design the UI:

- **There is no button field type.** `settings.FieldType` is exactly `text`,
  `password`, `select`, `toggle`, `computed`, `number` — every one of them a
  value the service persists. "Check Now" is therefore your app's own UI
  element calling `app.Updater.CheckAndInstall` through a bound method. See
  `AppService.CheckForUpdates` in the starter example.
- **Computed fields are never persisted.** `SetValues` strips them and
  recomputes rather than trusting a round-tripped value. `update.lastChecked`
  backed by an in-memory `atomic.Int64` therefore resets to "Never" on each
  launch. Persisting it would require a real writable field, which the user
  would see as an editable control.
- **Every `FieldComputed` needs a `ComputeFuncs` entry in the same group**, and
  every `ComputeFuncs` key must be declared as a field in that group.
  `ValidateSchema` rejects both mistakes at `NewService` time, not at runtime.
- **`Init` is once per process.** Channel and auto-check changes take effect on
  next launch. Say so in the field `Description` (both examples above do)
  instead of letting the user wonder.
- **Bind `svc.Bindings()`, never `svc`.** Wails binds every exported method, and
  `settings.Service` exposes `GetSecret` and `GetValuesWithSecrets`.

---

## 11. Setup checklist

```
[ ] go get github.com/wailsapp/wails/v3@v3.0.0-beta.4
[ ] openssl genpkey -algorithm ed25519 -out update-private-key.pem
[ ] openssl pkey -in update-private-key.pem -pubout -out update-public-key.pem
[ ] Add *-private-key.pem to .gitignore
[ ] Back the private key up offline, in two places
[ ] Add UPDATE_PRIVATE_KEY to CI secrets
[ ] //go:embed update-public-key.pem into the app
[ ] One version variable drives: build/config.yml info.version -> Info.plist
    (both keys) -> generated version_gen.go -> updater.Config.CurrentVersion
[ ] wails3 task common:update:build-assets after any version change
[ ] CI asserts version_gen.go is regenerated and unchanged
[ ] Release asset filenames contain runtime.GOOS ("darwin", not "macos") and
    runtime.GOARCH
[ ] macOS: Developer ID cert + notarization on a macOS runner; stapler staple
    the .app BEFORE zipping
[ ] Publish a signature for every artifact; publish checksums.txt
[ ] Feed URL is a stable hostname you control forever
[ ] canSelfUpdate() gate, notify-only fallback for Program Files / apt / Flatpak
[ ] Verify the key pair against a throwaway artifact before the first release
[ ] Test the full loop: install vN, publish vN+1, check, install, restart,
    confirm the running version is vN+1
```

---

## Reference: upstream source

Everything above was verified against `wailsapp/wails` at `v3.0.0-beta.4`:

| Topic | File |
|---|---|
| `Config` fields and validation | `v3/pkg/updater/config.go` |
| `Updater` methods, `Init`, `Restart` | `v3/pkg/updater/updater.go` |
| `Release`, `Artifact`, `Verification`, `Provider` | `v3/pkg/updater/types.go` |
| Event name constants | `v3/pkg/updater/events.go` |
| Verification rules and algorithms | `v3/pkg/updater/verify.go` |
| Streaming digest during download | `v3/pkg/updater/download.go` |
| Helper-mode swap, exit codes | `v3/pkg/updater/helper.go` |
| macOS `.app` bundle resolution | `v3/pkg/updater/updater_darwin.go` |
| Window options | `v3/pkg/updater/window.go`, `window_lifecycle.go` |
| GitHub provider, `DefaultAssetMatcher` | `v3/pkg/updater/providers/github/github.go` |
| Appcast provider, Sparkle XML mapping | `v3/pkg/updater/providers/appcast/appcast.go` |
| `app.Updater` construction, `HandleHelperMode` | `v3/pkg/application/application.go` |
| `Services` binding, method FQNs | `v3/pkg/application/services.go`, `bindings.go` |
| Generated `Info.plist` / `build/config.yml` | `v3/examples/badge/build/` |
