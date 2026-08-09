# updates — agent notes

## Purpose

GitHub Releases–based self-update: check for a newer release, download the
matching asset, verify its minisign signature, and swap the running binary or
`.app` bundle. It owns the update *mechanism*. It does not own release
publishing, signing (that is the release engineer's job, documented in the
README), version parsing (`semver`), or the decision of when to prompt a user —
the app drives that via `CheckForUpdate`/`DownloadUpdate`/`ApplyUpdate`.

**This is the highest-stakes package in the kit.** It replaces the user's
running binary. A defect here is arbitrary code execution on every install, so
prefer refusing to update over updating optimistically.

## Public API (load-bearing signatures)

```go
func NewService(opts ...ServiceOption) (*Service, error)

func WithCurrentVersion(version string) ServiceOption
func WithGitHubRepo(owner, repo string) ServiceOption
func WithGitHubToken(token string) ServiceOption
func WithPublicKey(minisignPublicKey string) ServiceOption // .pub contents or bare base64
func WithSkipVerification() ServiceOption                  // dev only; logs a warning
func WithAllowDowngrade() ServiceOption                    // testing/rollback only
func WithIncludePrereleases(include bool) ServiceOption
func WithApplier(a Applier) ServiceOption

func (s *Service) CheckForUpdate(ctx context.Context) (*Release, error)
func (s *Service) DownloadUpdate(ctx context.Context) (string, error)
func (s *Service) ApplyUpdate(_ context.Context) error
func (s *Service) GetCurrentVersion() string
```

## Invariants (do not break)

- **Never install a version that is not newer than the current one** unless
  `WithAllowDowngrade` is set. Newness is re-checked in `DownloadUpdate` and
  `ApplyUpdate`, not only at check time. The original bug cached a release
  before the newness test, so a feed rollback could serve a legitimately-signed
  old build with known-patched vulnerabilities. Signatures do not prevent this.
- **Verify after download *and* again immediately before extraction.** The two
  are separate user-triggered steps with an unbounded gap; verifying once
  leaves a swap window.
- **Never stage downloads in a shared temp directory.** `os.TempDir()/{app}` on
  Linux is world-writable-parent, and `MkdirAll` succeeds silently on a
  pre-existing directory owned by someone else. Staging goes in a user-private
  location.
- **`minisign.PublicKey.Verify` returns `(bool, error)`. A `false` with a nil
  error is a verification failure.** Treating it as success is the single most
  likely way to introduce a silent fail-open here.
- **A missing `.minisig` asset is a hard failure**, never "unsigned, therefore
  acceptable."
- **Public keys are length-validated at construction.** A wrong-length key
  reaching the Ed25519 primitive panics at update time — the worst possible
  moment.
- **Downloads are size-capped against the API-reported asset size**, and
  extraction is capped independently. A hostile feed must not be able to fill
  the disk before verification rejects it.

## Dependencies & insulation

`semver` (version precedence), `events` (progress/error emission), `settings`
(optional, for update preferences), `errors` (user-facing codes),
`github.com/jedisct1/go-minisign` (verification). **No `wails/v3` import** — not
on the AD-4 allowlist, and must stay off it; update logic must remain usable
from a CLI with no GUI in the process.

## Extension points

- `Applier` is the seam for platform-specific install behaviour — implement it
  rather than branching inside the service.
- Non-GitHub release sources: extract a provider interface alongside
  `github.go`. The verification and apply paths are already source-agnostic.

## Testing

`go test ./updates/ -race`. **No test may touch the network** — everything goes
through `httptest`. Signing helpers live in `minisign_testutil_test.go`.

`TestVerifySignatureRealMinisignCLI` is an interoperability golden test holding
fixtures produced by the actual minisign 0.12 CLI. Every other signature test
signs with our own Go helpers, which proves self-consistency but not that we can
read what the tool emits — and the README instructs release engineers to use the
tool. Do not regenerate those fixtures casually; their value is provenance.

Manual verification automated tests cannot do: an end-to-end release against a
real GitHub repo, and the macOS `.app` bundle swap under Gatekeeper (quarantine
xattr behaviour after replacement is unverified — see Landmines).

## File map

- `service.go` — options, constructor, check/download/apply orchestration
- `github.go` — release listing and asset download
- `verify.go` — minisign public-key parsing and signature verification
- `apply.go` — archive extraction and binary/bundle replacement
- `staging_unix.go` / `staging_windows.go` — `verifyPrivateDir`; this is the
  code backing the "never stage in a shared temp directory" invariant above
- `managed.go` — detection of package-manager-managed installs (notify-only)
- `crossdev_unix.go` / `crossdev_windows.go` — cross-device rename fallback
- `settings.go` — update-preference settings group

## Landmines

- **Gatekeeper quarantine after bundle swap is unverified.** Replacing a
  `.app` on macOS may leave it in a state Gatekeeper re-evaluates on next
  launch. Nothing here tests that; a real signed-and-notarized release is the
  only way to find out.
- **`WithSkipVerification` and an unset public key are different failure
  modes** and both log warnings. Neither should ever be the default in a
  shipped app; do not "simplify" them into a single silent path.
- **The trusted comment is signed; the untrusted comment is not.** Displaying
  the untrusted one as though it were verified is an easy and dangerous mistake.
- Losing the signing private key permanently strands every installed copy —
  there is no remote fix, because the fix would need to be signed. The README
  says this loudly; keep it that way.
