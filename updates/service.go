package updates

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	minisign "github.com/jedisct1/go-minisign"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/semver"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

// Event names.
const (
	EventAvailable   = "updates:available"
	EventDownloading = "updates:downloading"
	EventReady       = "updates:ready"
	EventError       = "updates:error"
	EventManaged     = "updates:managed"
)

// Error codes.
const (
	ErrUpdateCheck    errors.Code = "update_check"
	ErrUpdateDownload errors.Code = "update_download"
	ErrUpdateApply    errors.Code = "update_apply"
	ErrUpdateVerify   errors.Code = "update_verify"
	ErrUpdateManaged  errors.Code = "update_managed"
)

func init() {
	errors.RegisterMessages(map[errors.Code]i18n.Text{
		ErrUpdateCheck:    i18n.T("wailskit.updates.errors.check", "Unable to check for updates. Please try again later."),
		ErrUpdateDownload: i18n.T("wailskit.updates.errors.download", "Failed to download the update. Please try again."),
		ErrUpdateApply:    i18n.T("wailskit.updates.errors.apply", "Failed to install the update. Please try again."),
		ErrUpdateVerify:   i18n.T("wailskit.updates.errors.verify", "Update signature verification failed. The download may be corrupted or tampered with."),
		ErrUpdateManaged:  i18n.T("wailskit.updates.errors.managed", "This app is managed by a package manager. Please update through your package manager instead."),
	})
}

// Event payloads.
type (
	AvailablePayload struct {
		Version      string `json:"version"`
		ReleaseNotes string `json:"releaseNotes"`
		ReleaseURL   string `json:"releaseUrl"`
	}

	DownloadingPayload struct {
		Version    string  `json:"version"`
		Progress   float64 `json:"progress"`
		Downloaded int64   `json:"downloaded"`
		Total      int64   `json:"total"`
	}

	ReadyPayload struct {
		Version string `json:"version"`
	}

	ErrorPayload struct {
		Message string      `json:"message"`
		Code    errors.Code `json:"code"`
	}

	ManagedPayload struct {
		Method       InstallMethod `json:"method"`
		Instructions string        `json:"instructions"`
	}
)

// Service manages update checking, downloading, and applying.
type Service struct {
	currentVersion semver.Version
	// currentVersionRaw and currentVersionErr let NewService distinguish
	// "WithCurrentVersion was never called" from "WithCurrentVersion was
	// called with a string that failed to parse" — WithCurrentVersion has
	// no error return, so it can't surface a parse failure itself. Without
	// this, an invalid version silently leaves currentVersion at its zero
	// value and NewService reports the misleading "current version is
	// required" instead of the actual parse error.
	currentVersionRaw  string
	currentVersionErr  error
	github             *GitHubSource
	emitter            *events.Emitter
	applier            Applier
	settings           *settings.Service
	appName            string
	assetPattern       string
	binaryName         string
	includePrereleases bool
	publicKeyText      string
	publicKey          *minisign.PublicKey
	skipVerification   bool
	allowDowngrade     bool

	mu sync.Mutex
	// latestRelease is populated only when it is strictly newer than
	// currentVersion (see CheckForUpdate) — a compromised or rolled-back
	// feed must never be able to poison this cache with an older,
	// legitimately-signed release. See AGENTS.md, "Downgrade attack".
	latestRelease *Release
	// downloadPath, downloadSigPath, downloadVersion, and downloadDir
	// describe the most recently completed DownloadUpdate. ApplyUpdate
	// re-validates all of them (newness + signature) immediately before
	// extraction, as defense in depth against the staging directory being
	// tampered with between the two — separate, user-triggered — calls.
	downloadPath    string
	downloadSigPath string
	downloadVersion semver.Version
	downloadDir     string
}

type ServiceOption func(*Service)

// WithEmitter sets the event emitter for update notifications.
func WithEmitter(e *events.Emitter) ServiceOption {
	return func(s *Service) {
		s.emitter = e
	}
}

// WithCurrentVersion sets the current app version for comparison. A version
// that fails to parse is not silently dropped: NewService reports the parse
// error explicitly rather than the misleading "current version is required"
// (which is reserved for WithCurrentVersion never having been called at all).
func WithCurrentVersion(version string) ServiceOption {
	return func(s *Service) {
		s.currentVersionRaw = version
		v, err := semver.ParseVersion(version)
		if err != nil {
			s.currentVersionErr = err
			return
		}
		s.currentVersion = v
	}
}

// WithGitHubRepo sets the GitHub owner/repo to check for releases.
func WithGitHubRepo(owner, repo string) ServiceOption {
	return func(s *Service) {
		if s.github == nil {
			s.github = &GitHubSource{}
		}
		s.github.owner = owner
		s.github.repo = repo
	}
}

// WithGitHubToken sets a token for accessing private repos.
func WithGitHubToken(token string) ServiceOption {
	return func(s *Service) {
		if s.github == nil {
			s.github = &GitHubSource{}
		}
		s.github.token = token
	}
}

// WithGitHubAPIURL overrides the GitHub API base URL (default
// "https://api.github.com"). Set this for GitHub Enterprise Server, or to
// point at a local stand-in server in tests and examples.
func WithGitHubAPIURL(url string) ServiceOption {
	return func(s *Service) {
		if s.github == nil {
			s.github = &GitHubSource{}
		}
		s.github.apiURL = url
	}
}

// WithHTTPClient sets a custom HTTP client for API requests.
func WithHTTPClient(client *http.Client) ServiceOption {
	return func(s *Service) {
		if s.github == nil {
			s.github = &GitHubSource{}
		}
		s.github.client = client
	}
}

// WithApplier overrides the default binary replacement strategy.
func WithApplier(a Applier) ServiceOption {
	return func(s *Service) {
		s.applier = a
	}
}

// WithAssetPattern sets the pattern for matching release assets.
// Use {os} and {arch} as placeholders (e.g., "myapp_{os}_{arch}").
func WithAssetPattern(pattern string) ServiceOption {
	return func(s *Service) {
		s.assetPattern = pattern
	}
}

// WithBinaryName sets the name of the binary inside an archive.
// If unset, the first executable file in the archive is used.
func WithBinaryName(name string) ServiceOption {
	return func(s *Service) {
		s.binaryName = name
	}
}

// WithSettings optionally connects the update service to a settings service.
// When set, CheckForUpdate reads the include_prereleases setting at call time.
// The app is still responsible for reading check_frequency and auto_download
// to decide when to call CheckForUpdate and DownloadUpdate.
func WithSettings(svc *settings.Service) ServiceOption {
	return func(s *Service) {
		s.settings = svc
	}
}

// WithAppName sets the application name, used for app-namespaced staging
// directories.
func WithAppName(name string) ServiceOption {
	return func(s *Service) {
		s.appName = name
	}
}

// WithIncludePrereleases sets whether to include pre-release versions.
// This is the static fallback; if WithSettings is also provided, the
// settings value takes precedence.
func WithIncludePrereleases(include bool) ServiceOption {
	return func(s *Service) {
		s.includePrereleases = include
	}
}

// WithPublicKey sets the minisign public key used to verify update
// signatures. Accepts either the full contents of a minisign public key
// file (typically embedded via go:embed) or just the bare base64-encoded
// key. When set, each downloaded asset must have a corresponding
// "<asset>.minisig" signature file in the release; DownloadUpdate verifies
// it after download, and ApplyUpdate verifies it again immediately before
// extraction. The key is parsed (and rejected if malformed) at
// NewService time rather than at first use — see AGENTS.md, "Fail-open
// default".
func WithPublicKey(minisignPublicKey string) ServiceOption {
	return func(s *Service) {
		s.publicKeyText = minisignPublicKey
	}
}

// WithSkipVerification disables signature verification.
// This is intended for local development and testing only.
// A warning is logged when this option is active.
func WithSkipVerification() ServiceOption {
	return func(s *Service) {
		s.skipVerification = true
	}
}

// WithAllowDowngrade permits DownloadUpdate and ApplyUpdate to proceed
// against a release that is not strictly newer than the current version.
// Off by default: without it, both calls refuse a non-newer release even
// if the release object being acted on was somehow swapped after
// CheckForUpdate ran (e.g. a rolled-back feed) — signatures alone don't
// prevent installing an older, legitimately-signed build with known,
// already-patched vulnerabilities. Only enable this for an explicit,
// user-initiated "reinstall" or "downgrade" flow. See AGENTS.md,
// "Downgrade attack".
func WithAllowDowngrade() ServiceOption {
	return func(s *Service) {
		s.allowDowngrade = true
	}
}

// NewService creates a new update service.
func NewService(opts ...ServiceOption) (*Service, error) {
	s := &Service{}
	for _, opt := range opts {
		opt(s)
	}

	if s.github == nil || s.github.owner == "" || s.github.repo == "" {
		return nil, fmt.Errorf("updates: GitHub repo is required (use WithGitHubRepo)")
	}
	if s.currentVersionRaw == "" {
		return nil, fmt.Errorf("updates: current version is required (use WithCurrentVersion)")
	}
	if s.currentVersionErr != nil {
		return nil, fmt.Errorf("updates: invalid current version %q: %w", s.currentVersionRaw, s.currentVersionErr)
	}
	if s.applier == nil {
		s.applier = defaultApplier{}
	}
	if s.publicKeyText != "" {
		pk, err := parseMinisignPublicKey(s.publicKeyText)
		if err != nil {
			return nil, fmt.Errorf("updates: %w", err)
		}
		s.publicKey = &pk
	}

	// Verification must be either configured or explicitly declined. This
	// package replaces the user's running binary; "no key configured, so
	// silently apply unverified updates" is not a defensible default, and a
	// constructor that already hard-fails on a missing repo or version
	// should not treat missing verification as a soft warning. See
	// AGENTS.md, "Fail-open default".
	if s.publicKey == nil && !s.skipVerification {
		return nil, fmt.Errorf("updates: signature verification is required — call WithPublicKey (recommended) or WithSkipVerification (dev/test only, logs a warning on every use)")
	}

	// Best-effort: remove leftover per-download staging directories left by
	// a crashed process between DownloadUpdate and ApplyUpdate/cleanup, or
	// by a prior process instance that never finished. A sweep failure must
	// not prevent constructing the service. See AGENTS.md / README, "Staging
	// directory cleanup".
	s.sweepStaleDownloads()

	return s, nil
}

// sweepStaleDownloads removes leftover "dl-*" per-download staging
// directories under the app's update cache directory
// (appdirs.Cache()/updates/). Every such directory is, by definition, stale
// at NewService time: this process has not created one yet, so any that
// exist were left behind by a crashed or otherwise-abandoned previous run
// (DownloadUpdate normally removes its own directory on failure or after a
// successful ApplyUpdate — see AGENTS.md, "Staging directory cleanup").
// Best-effort: errors are logged, not returned, since a failed sweep should
// not block constructing the service.
func (s *Service) sweepStaleDownloads() {
	stageRoot := filepath.Join(s.appDirs().Cache(), "updates")
	entries, err := os.ReadDir(stageRoot)
	if err != nil {
		// Most commonly: the directory doesn't exist yet. Nothing to sweep.
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "dl-") {
			continue
		}
		path := filepath.Join(stageRoot, e.Name())
		if err := os.RemoveAll(path); err != nil {
			slog.Warn("updates: failed to remove stale staging directory", "path", path, "error", err)
		}
	}
}

// CheckForUpdate checks GitHub for a newer version.
// Returns the release if available, nil if up-to-date.
func (s *Service) CheckForUpdate(ctx context.Context) (*Release, error) {
	includePre := s.includePrereleases
	if s.settings != nil {
		if values, err := s.settings.GetValues(); err == nil {
			if v, ok := values[SettingIncludePrereleases].(bool); ok {
				includePre = v
			}
		}
	}

	rel, err := s.github.LatestRelease(ctx, includePre)
	if err != nil {
		s.emitError(ErrUpdateCheck, err)
		return nil, errors.Wrap(ErrUpdateCheck, "check for update", err)
	}

	newer := rel.Version.NewerThan(s.currentVersion)
	if !newer && !s.allowDowngrade {
		// Do not cache a non-newer release: a rolled-back or compromised
		// feed must not be able to overwrite a previously cached, genuinely
		// newer release with an older one. See AGENTS.md, "Downgrade
		// attack".
		return nil, nil
	}

	s.mu.Lock()
	s.latestRelease = rel
	s.mu.Unlock()

	if newer {
		s.emit(EventAvailable, AvailablePayload{
			Version:      rel.Version.String(),
			ReleaseNotes: rel.Body,
			ReleaseURL:   rel.HTMLURL,
		})
	}

	return rel, nil
}

// DownloadUpdate downloads the latest release asset for the current platform.
// Returns the path to the downloaded file.
func (s *Service) DownloadUpdate(ctx context.Context) (string, error) {
	s.mu.Lock()
	rel := s.latestRelease
	s.mu.Unlock()

	if rel == nil {
		return "", errors.Newf(ErrUpdateDownload, "no update available; call CheckForUpdate first")
	}

	// Defense in depth: re-check newness here too, in case the cached
	// release was set by a caller other than CheckForUpdate (e.g. a
	// future API extension) or the check happened long enough ago that
	// re-validating is cheap insurance. See AGENTS.md, "Downgrade attack".
	if !s.allowDowngrade && !rel.Version.NewerThan(s.currentVersion) {
		return "", errors.Newf(ErrUpdateDownload, "refusing to download %s: not newer than current version %s", rel.Version, s.currentVersion)
	}

	asset, err := FindAsset(rel, s.assetPattern)
	if err != nil {
		s.emitError(ErrUpdateDownload, err)
		return "", errors.Wrap(ErrUpdateDownload, "find platform asset", err)
	}

	// Stage the download in a private, per-attempt directory under the
	// app's cache dir — never the shared OS temp dir. appdirs.Temp()
	// resolves to a subdirectory of os.TempDir(), which is world-writable
	// on Linux (/tmp is mode 1777); a local attacker can pre-create that
	// subdirectory and own it before this process ever runs. appdirs.Cache()
	// lives under the user's home directory instead. See AGENTS.md,
	// "Linux /tmp TOCTOU".
	dirs := s.appDirs()
	stageRoot := filepath.Join(dirs.Cache(), "updates")
	if err := ensurePrivateDir(stageRoot); err != nil {
		s.emitError(ErrUpdateDownload, err)
		return "", errors.Wrap(ErrUpdateDownload, "prepare staging directory", err)
	}

	// A previous DownloadUpdate call that was never applied (or that failed
	// after creating its directory but is being retried) left its staging
	// directory behind — s.downloadDir is about to be overwritten below, so
	// this is the last point at which anything still references it. Remove
	// it now rather than leaking it; best effort, since a failure here
	// should not block downloading the new update. See AGENTS.md, "Staging
	// directory cleanup".
	s.mu.Lock()
	previousDownloadDir := s.downloadDir
	s.mu.Unlock()
	if previousDownloadDir != "" {
		if err := os.RemoveAll(previousDownloadDir); err != nil {
			slog.Warn("updates: failed to remove previous staging directory", "path", previousDownloadDir, "error", err)
		}
	}

	downloadDir, err := os.MkdirTemp(stageRoot, "dl-*")
	if err != nil {
		s.emitError(ErrUpdateDownload, err)
		return "", errors.Wrap(ErrUpdateDownload, "create download directory", err)
	}

	assetPath := filepath.Join(downloadDir, asset.Name)
	tmpFile, err := os.OpenFile(assetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = os.RemoveAll(downloadDir)
		s.emitError(ErrUpdateDownload, err)
		return "", errors.Wrap(ErrUpdateDownload, "create download file", err)
	}

	version := rel.Version.String()
	err = s.github.DownloadAsset(ctx, asset, tmpFile, func(downloaded, total int64) {
		var progress float64
		if total > 0 {
			progress = float64(downloaded) / float64(total)
		}
		s.emit(EventDownloading, DownloadingPayload{
			Version:    version,
			Progress:   progress,
			Downloaded: downloaded,
			Total:      total,
		})
	})

	_ = tmpFile.Close()

	if err != nil {
		_ = os.RemoveAll(downloadDir)
		s.emitError(ErrUpdateDownload, err)
		return "", errors.Wrap(ErrUpdateDownload, "download update", err)
	}

	// Verify the downloaded asset's signature.
	var sigPath string
	switch {
	case s.skipVerification:
		slog.Warn("updates: signature verification skipped — do not use in production")
	case s.publicKey == nil:
		slog.Warn("updates: no public key configured — downloaded update will not be signature-verified")
	default:
		sigPath, err = s.downloadSignature(ctx, rel, asset, downloadDir)
		if err != nil {
			_ = os.RemoveAll(downloadDir)
			s.emitError(ErrUpdateVerify, err)
			return "", errors.Wrap(ErrUpdateVerify, "download update signature", err)
		}
		if err := verifySignature(*s.publicKey, assetPath, sigPath); err != nil {
			_ = os.RemoveAll(downloadDir)
			s.emitError(ErrUpdateVerify, err)
			return "", errors.Wrap(ErrUpdateVerify, "verify update signature", err)
		}
	}

	s.mu.Lock()
	s.downloadPath = assetPath
	s.downloadSigPath = sigPath
	s.downloadVersion = rel.Version
	s.downloadDir = downloadDir
	s.mu.Unlock()

	s.emit(EventReady, ReadyPayload{Version: version})

	return assetPath, nil
}

// ApplyUpdate applies a previously downloaded update to the running binary.
// Returns an ErrUpdateManaged error if the app was installed via a package
// manager (e.g., Homebrew Cask). In that case, an updates:managed event is
// emitted with instructions for the user.
func (s *Service) ApplyUpdate(_ context.Context) error {
	// Check for managed installs before attempting anything
	if method := DetectInstallMethod(); method != InstallDirect {
		instructions := method.UpdateInstructions(s.appName)
		s.emit(EventManaged, ManagedPayload{
			Method:       method,
			Instructions: instructions,
		})
		return errors.Newf(ErrUpdateManaged, "%s", instructions)
	}

	s.mu.Lock()
	downloadPath := s.downloadPath
	sigPath := s.downloadSigPath
	downloadVersion := s.downloadVersion
	downloadDir := s.downloadDir
	s.mu.Unlock()

	if downloadPath == "" {
		return errors.Newf(ErrUpdateApply, "no downloaded update; call DownloadUpdate first")
	}

	// Defense in depth: DownloadUpdate and ApplyUpdate are separate,
	// user-triggered steps with an unbounded gap between them. Re-check
	// newness here even though DownloadUpdate already checked it — the
	// whole point is not to trust a single check-then-act window.
	// See AGENTS.md, "Downgrade attack".
	if !s.allowDowngrade && !downloadVersion.NewerThan(s.currentVersion) {
		return errors.Newf(ErrUpdateApply, "refusing to apply %s: not newer than current version %s", downloadVersion, s.currentVersion)
	}

	// Defense in depth: re-verify the signature immediately before
	// extraction, not just at download time. This closes the window where
	// a local attacker who gains write access to the staging directory
	// between DownloadUpdate and ApplyUpdate could swap the file.
	// See AGENTS.md, "Linux /tmp TOCTOU".
	if !s.skipVerification && s.publicKey != nil {
		if sigPath == "" {
			return errors.Newf(ErrUpdateVerify, "no signature recorded for the downloaded update; refusing to apply")
		}
		if err := verifySignature(*s.publicKey, downloadPath, sigPath); err != nil {
			s.emitError(ErrUpdateVerify, err)
			return errors.Wrap(ErrUpdateVerify, "re-verify update signature before apply", err)
		}
	}

	// Extract the archive (if applicable) into a private subdirectory of
	// the same staging directory used for the download.
	extractDir, err := extractArchive(downloadPath, downloadDir)
	if err != nil {
		s.emitError(ErrUpdateApply, err)
		return errors.Wrap(ErrUpdateApply, "extract update", err)
	}
	defer func() { _ = os.RemoveAll(extractDir) }()

	// Find the binary in the extracted archive
	binaryPath, err := findBinary(extractDir, s.binaryName)
	if err != nil {
		s.emitError(ErrUpdateApply, err)
		return errors.Wrap(ErrUpdateApply, "find binary in update", err)
	}

	// Get the current executable path
	currentExe, err := os.Executable()
	if err != nil {
		return errors.Wrap(ErrUpdateApply, "determine current executable", err)
	}

	// Apply the update
	if err := s.applier.Apply(binaryPath, currentExe); err != nil {
		s.emitError(ErrUpdateApply, err)
		return errors.Wrap(ErrUpdateApply, "apply update", err)
	}

	// Clean up the whole staging directory for this download (asset,
	// signature, and any extracted files).
	_ = os.RemoveAll(downloadDir)
	s.mu.Lock()
	s.downloadPath = ""
	s.downloadSigPath = ""
	s.downloadDir = ""
	s.downloadVersion = semver.Version{}
	s.mu.Unlock()

	return nil
}

// GetCurrentVersion returns the current app version string.
func (s *Service) GetCurrentVersion() string {
	return s.currentVersion.String()
}

// GetLatestRelease returns the cached latest release from the last check.
func (s *Service) GetLatestRelease() *Release {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latestRelease
}

// appDirs returns the appdirs.Dirs for this service's app name, defaulting
// to "wails-kit" when unset (matching the pre-v2 behavior).
func (s *Service) appDirs() *appdirs.Dirs {
	appName := s.appName
	if appName == "" {
		appName = "wails-kit"
	}
	return appdirs.New(appName)
}

// downloadSignature fetches the minisign detached signature for asset
// (named "<asset.Name>.minisig" in the release) into dir and returns its
// path.
func (s *Service) downloadSignature(ctx context.Context, rel *Release, asset *Asset, dir string) (string, error) {
	sigAssetName := asset.Name + ".minisig"
	var sigAsset *Asset
	for i := range rel.Assets {
		if rel.Assets[i].Name == sigAssetName {
			sigAsset = &rel.Assets[i]
			break
		}
	}
	if sigAsset == nil {
		return "", fmt.Errorf("signature file %q not found in release %s", sigAssetName, rel.TagName)
	}

	sigPath := filepath.Join(dir, sigAssetName)
	sigFile, err := os.OpenFile(sigPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create signature file: %w", err)
	}
	if err := s.github.DownloadAsset(ctx, sigAsset, sigFile, nil); err != nil {
		_ = sigFile.Close()
		return "", fmt.Errorf("download signature: %w", err)
	}
	_ = sigFile.Close()

	return sigPath, nil
}

func (s *Service) emit(name string, data any) {
	if s.emitter != nil {
		s.emitter.Emit(name, data)
	}
}

func (s *Service) emitError(code errors.Code, err error) {
	s.emit(EventError, ErrorPayload{
		Message: errors.New(code, "", nil).UserMsg,
		Code:    code,
	})
}
