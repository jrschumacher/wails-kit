package updates

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/jrschumacher/wails-kit/appdirs"
	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/events"
	"github.com/jrschumacher/wails-kit/settings"
)

// Event names.
const (
	EventAvailable   = "updates:available"
	EventDownloading = "updates:downloading"
	EventReady       = "updates:ready"
	EventError       = "updates:error"
)

// Error codes.
const (
	ErrUpdateCheck    errors.Code = "update_check"
	ErrUpdateDownload errors.Code = "update_download"
	ErrUpdateApply    errors.Code = "update_apply"
	ErrUpdateVerify   errors.Code = "update_verify"
)

func init() {
	errors.RegisterMessages(map[errors.Code]string{
		ErrUpdateCheck:    "Unable to check for updates. Please try again later.",
		ErrUpdateDownload: "Failed to download the update. Please try again.",
		ErrUpdateApply:    "Failed to install the update. Please try again.",
		ErrUpdateVerify:   "Update signature verification failed. The download may be corrupted or tampered with.",
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
)

// Service manages update checking, downloading, and applying.
type Service struct {
	currentVersion     Version
	github             *GitHubSource
	emitter            *events.Emitter
	applier            Applier
	settings           *settings.Service
	appName            string
	assetPattern       string
	binaryName         string
	includePrereleases bool
	publicKey          ed25519.PublicKey
	skipVerification   bool
	mu                 sync.Mutex
	latestRelease      *Release
	downloadPath       string
}

type ServiceOption func(*Service)

// WithEmitter sets the event emitter for update notifications.
func WithEmitter(e *events.Emitter) ServiceOption {
	return func(s *Service) {
		s.emitter = e
	}
}

// WithCurrentVersion sets the current app version for comparison.
func WithCurrentVersion(version string) ServiceOption {
	return func(s *Service) {
		v, err := ParseVersion(version)
		if err == nil {
			s.currentVersion = v
		}
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

// WithAppName sets the application name, used for app-namespaced temp directories.
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

// WithPublicKey sets the Ed25519 public key used to verify update signatures.
// When set, each downloaded asset must have a corresponding .sig file in the
// release. The signature is verified after download, before the update is applied.
func WithPublicKey(key ed25519.PublicKey) ServiceOption {
	return func(s *Service) {
		s.publicKey = key
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

// NewService creates a new update service.
func NewService(opts ...ServiceOption) (*Service, error) {
	s := &Service{}
	for _, opt := range opts {
		opt(s)
	}

	if s.github == nil || s.github.owner == "" || s.github.repo == "" {
		return nil, fmt.Errorf("updates: GitHub repo is required (use WithGitHubRepo)")
	}
	if s.currentVersion.Raw == "" {
		return nil, fmt.Errorf("updates: current version is required (use WithCurrentVersion)")
	}
	if s.applier == nil {
		s.applier = defaultApplier{}
	}

	return s, nil
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

	s.mu.Lock()
	s.latestRelease = rel
	s.mu.Unlock()

	if !rel.Version.NewerThan(s.currentVersion) {
		return nil, nil
	}

	s.emit(EventAvailable, AvailablePayload{
		Version:      rel.Version.String(),
		ReleaseNotes: rel.Body,
		ReleaseURL:   rel.HTMLURL,
	})

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

	asset, err := FindAsset(rel, s.assetPattern)
	if err != nil {
		s.emitError(ErrUpdateDownload, err)
		return "", errors.Wrap(ErrUpdateDownload, "find platform asset", err)
	}

	// Download to an app-namespaced temp directory
	appName := s.appName
	if appName == "" {
		appName = "wails-kit"
	}
	dirs := appdirs.New(appName)
	tmpDir := dirs.Temp()
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return "", errors.Wrap(ErrUpdateDownload, "create temp dir", err)
	}
	tmpFile, err := os.CreateTemp(tmpDir, "update-*-"+asset.Name)
	if err != nil {
		return "", errors.Wrap(ErrUpdateDownload, "create temp file", err)
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
		_ = os.Remove(tmpFile.Name())
		s.emitError(ErrUpdateDownload, err)
		return "", errors.Wrap(ErrUpdateDownload, "download update", err)
	}

	// Verify the downloaded asset's signature
	if err := s.verifyDownload(ctx, rel, asset, tmpFile.Name()); err != nil {
		_ = os.Remove(tmpFile.Name())
		s.emitError(ErrUpdateVerify, err)
		return "", errors.Wrap(ErrUpdateVerify, "verify update signature", err)
	}

	s.mu.Lock()
	s.downloadPath = tmpFile.Name()
	s.mu.Unlock()

	s.emit(EventReady, ReadyPayload{Version: version})

	return tmpFile.Name(), nil
}

// ApplyUpdate applies a previously downloaded update to the running binary.
func (s *Service) ApplyUpdate(ctx context.Context) error {
	s.mu.Lock()
	downloadPath := s.downloadPath
	s.mu.Unlock()

	if downloadPath == "" {
		return errors.Newf(ErrUpdateApply, "no downloaded update; call DownloadUpdate first")
	}

	// Extract the archive (if applicable)
	extractDir, err := extractArchive(downloadPath)
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

	// Clean up the download
	_ = os.Remove(downloadPath)
	s.mu.Lock()
	s.downloadPath = ""
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

// verifyDownload handles signature verification for a downloaded asset.
// If no public key is configured and skip is not set, this is a no-op.
// If skip is set, a warning is logged.
func (s *Service) verifyDownload(ctx context.Context, rel *Release, asset *Asset, assetPath string) error {
	if s.skipVerification {
		slog.Warn("updates: signature verification skipped — do not use in production")
		return nil
	}
	if s.publicKey == nil {
		return nil
	}

	// Find the corresponding .sig asset
	sigAssetName := asset.Name + ".sig"
	var sigAsset *Asset
	for i := range rel.Assets {
		if rel.Assets[i].Name == sigAssetName {
			sigAsset = &rel.Assets[i]
			break
		}
	}
	if sigAsset == nil {
		return fmt.Errorf("signature file %q not found in release %s", sigAssetName, rel.TagName)
	}

	// Download the signature to a temp file
	sigFile, err := os.CreateTemp("", "update-sig-*")
	if err != nil {
		return fmt.Errorf("create temp sig file: %w", err)
	}
	sigPath := sigFile.Name()
	defer func() { _ = os.Remove(sigPath) }()

	if err := s.github.DownloadAsset(ctx, sigAsset, sigFile, nil); err != nil {
		_ = sigFile.Close()
		return fmt.Errorf("download signature: %w", err)
	}
	_ = sigFile.Close()

	return verifySignature(s.publicKey, assetPath, sigPath)
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
