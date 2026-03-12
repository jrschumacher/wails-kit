# Shared Taskfiles

Reusable [Task](https://taskfile.dev/) definitions for wails-kit apps. These handle the build-sign-release workflow so each app only needs a thin root Taskfile.

## Prerequisites

| Tool | Install | Purpose |
|------|---------|---------|
| [Task](https://taskfile.dev/) | `brew install go-task` | Task runner |
| [yq](https://github.com/mikefarah/yq) | `brew install yq` | Reads `.wails-kit.yml` config |
| [gh](https://cli.github.com/) | `brew install gh` | GitHub release uploads |

macOS signing also requires Xcode command-line tools (`xcode-select --install`).

## Setup

1. Copy `.wails-kit.example.yml` to `.wails-kit.yml` in your project root and fill in your values.

2. Copy the release Taskfile into your project:

```sh
mkdir -p taskfiles
cp "$(go env GOMODCACHE)/github.com/jrschumacher/wails-kit@<version>/taskfiles/release.yml" taskfiles/release.yml
```

> **Note:** `wails3 task` uses an embedded Task runner that does not support remote taskfiles. You must copy the file locally.

3. Include the release Taskfile in your project's root `Taskfile.yml`:

```yaml
version: '3'

includes:
  # Wails-generated platform Taskfiles
  common: ./build/Taskfile.yml
  darwin: ./build/darwin/Taskfile.yml
  linux: ./build/linux/Taskfile.yml
  windows: ./build/windows/Taskfile.yml

  # wails-kit shared tasks
  release:
    taskfile: ./taskfiles/release.yml

tasks:
  dev:
    cmds:
      - wails3 dev -config ./build/config.yml
```

## Supported platforms

| OS | Architecture | Archive | Upload | Codesign | Notarize | Homebrew Cask |
|----|-------------|---------|--------|----------|----------|---------------|
| macOS | arm64 | `.zip` | Yes | Yes | Yes | Yes |
| macOS | amd64 | `.zip` | Yes | Yes | Yes | Yes |
| macOS | universal | `.zip` | Yes | Yes | Yes | Yes |
| Linux | amd64 | `.tar.gz` | Yes | — | — | — |
| Linux | arm64 | `.tar.gz` | Yes | — | — | — |
| Windows | amd64 | `.zip` | Yes | — | — | — |
| Windows | arm64 | `.zip` | Yes | — | — | — |

All archives are uploaded to the tap repo's GitHub Releases, which serves as the central artifact store.

### Distribution

The release taskfile handles building, signing, and uploading. How users install the app depends on the platform:

- **macOS** — Homebrew Cask (`brew install --cask app`). The taskfile auto-updates the cask formula after upload.
- **Linux** — Direct download from the GitHub release, or packaged for distro-specific formats (`.deb`, `.rpm`, Flatpak) outside this taskfile.
- **Windows** — Direct download from the GitHub release. Can also be published to [Scoop](https://scoop.sh/) or [winget](https://github.com/microsoft/winget-pkgs) separately.

## Available tasks

### `release` (default)

Builds, signs (macOS), archives, uploads to your Homebrew tap, and updates the cask.

```sh
# Uses latest GitHub release tag, host OS and architecture
task release

# Explicit version
task release VERSION=0.3.0

# Build for a specific architecture
task release TARGET_ARCH=amd64

# Build a macOS universal binary (arm64 + amd64 via lipo)
task release TARGET_ARCH=universal
```

### `release:all`

Builds releases for multiple OS/architecture combinations in sequence.

```sh
# Default targets: darwin/universal, linux/amd64, linux/arm64, windows/amd64
task release:all VERSION=0.3.0

# Custom target list
task release:all VERSION=0.3.0 TARGETS=darwin/arm64,linux/amd64,windows/amd64
```

## Platform details

**macOS (`release-darwin`):**
1. `darwin:package` (Wails-generated — builds binary and creates .app bundle)
2. `sign` — codesign with Developer ID (skipped if `signing.developer_id` is empty)
3. `notarize` — notarytool submit + staple (skipped if `signing.keychain_profile` is empty)
4. Archive as `.zip`
5. Upload to Homebrew tap release
6. Update Homebrew cask formula

When `TARGET_ARCH=universal`, builds both arm64 and amd64, merges with `lipo`, then signs and archives.

**Linux (`release-linux`):**
1. `linux:build` (Wails-generated)
2. Archive as `.tar.gz`
3. Upload to GitHub release

**Windows (`release-windows`):**
1. `windows:build` (Wails-generated)
2. Archive as `.zip`
3. Upload to GitHub release

## Configuration

All project-specific values come from `.wails-kit.yml`:

```yaml
app:
  name: MyApp                      # Used in archive names, signing, cask
  bundle_id: com.example.myapp     # macOS bundle identifier

release:
  github_repo: owner/myapp              # Source of release tags
  tap_github_repo: owner/homebrew-tap   # Receives assets + cask updates

signing:
  developer_id: "Developer ID Application: ..."  # Empty = skip signing
  keychain_profile: AC_PASSWORD                   # Empty = skip notarization
  entitlements: "build/darwin/entitlements.plist"  # Optional — hardened runtime entitlements
```

## How it works

The shared Taskfile reads `.wails-kit.yml` via `yq` at task invocation time. It calls back into the Wails-generated platform tasks (`:darwin:package`, `:linux:build`, `:windows:build`) using Taskfile's root-scoped task references.

Signing and notarization are skipped gracefully when the corresponding config values are empty, so the same Taskfile works in CI (unsigned) and on a developer's machine (signed).
