package updates

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/semver"
	"github.com/jrschumacher/wails-kit/v2/settings"
)

func TestNewServiceRequiresRepo(t *testing.T) {
	_, err := NewService(WithCurrentVersion("v1.0.0"))
	if err == nil {
		t.Fatal("expected error without repo")
	}
}

func TestNewServiceRequiresVersion(t *testing.T) {
	_, err := NewService(WithGitHubRepo("owner", "repo"))
	if err == nil {
		t.Fatal("expected error without version")
	}
}

func TestCheckForUpdateNewer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{
			TagName: "v2.0.0",
			Body:    "New features",
			HTMLURL: "https://github.com/owner/repo/releases/tag/v2.0.0",
		})
	}))
	defer srv.Close()

	mem := events.NewMemoryEmitter()
	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithEmitter(events.NewEmitter(mem)),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel == nil {
		t.Fatal("expected release, got nil")
	}
	if rel.Version.Major != 2 {
		t.Errorf("got major %d, want 2", rel.Version.Major)
	}

	// Check that EventAvailable was emitted
	evts := mem.Events()
	if len(evts) != 1 {
		t.Fatalf("got %d events, want 1", len(evts))
	}
	if evts[0].Name != EventAvailable {
		t.Errorf("got event %q, want %q", evts[0].Name, EventAvailable)
	}
}

func TestCheckForUpdateUpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{TagName: "v1.0.0"})
	}))
	defer srv.Close()

	mem := events.NewMemoryEmitter()
	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithEmitter(events.NewEmitter(mem)),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel != nil {
		t.Error("expected nil release when up-to-date")
	}

	// No event should be emitted
	if mem.Count() != 0 {
		t.Errorf("expected no events, got %d", mem.Count())
	}
}

func TestCheckForUpdateOlder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{TagName: "v0.9.0"})
	}))
	defer srv.Close()

	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel != nil {
		t.Error("expected nil when remote is older")
	}
}

func TestCheckForUpdateError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	mem := events.NewMemoryEmitter()
	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithEmitter(events.NewEmitter(mem)),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc.github.apiURL = srv.URL

	_, err = svc.CheckForUpdate(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}

	// Should emit error event
	evts := mem.Events()
	if len(evts) != 1 {
		t.Fatalf("got %d events, want 1", len(evts))
	}
	if evts[0].Name != EventError {
		t.Errorf("got event %q, want %q", evts[0].Name, EventError)
	}
	payload, ok := evts[0].Data.(ErrorPayload)
	if !ok {
		t.Fatalf("expected ErrorPayload, got %T", evts[0].Data)
	}
	if payload.Code != ErrUpdateCheck {
		t.Fatalf("got code %q, want %q", payload.Code, ErrUpdateCheck)
	}
	if payload.Message != "Unable to check for updates. Please try again later." {
		t.Fatalf("got message %q", payload.Message)
	}
}

func TestDownloadUpdateWithoutCheck(t *testing.T) {
	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc.DownloadUpdate(context.Background())
	if err == nil {
		t.Fatal("expected error when no check performed")
	}
}

func TestApplyUpdateWithoutDownload(t *testing.T) {
	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = svc.ApplyUpdate(context.Background())
	if err == nil {
		t.Fatal("expected error when no download performed")
	}
}

func TestGetCurrentVersion(t *testing.T) {
	svc, err := NewService(
		WithCurrentVersion("v1.2.3"),
		WithGitHubRepo("owner", "repo"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v := svc.GetCurrentVersion(); v != "v1.2.3" {
		t.Errorf("got %q, want %q", v, "v1.2.3")
	}
}

func TestSettingsGroup(t *testing.T) {
	g := SettingsGroup()
	if g.Key != "updates" {
		t.Errorf("got key %q, want %q", g.Key, "updates")
	}
	if len(g.Fields) != 3 {
		t.Errorf("got %d fields, want 3", len(g.Fields))
	}
}

func TestCheckForUpdateWithSettingsPrereleases(t *testing.T) {
	// Serve prereleases on /releases and stable on /releases/latest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases":
			_ = json.NewEncoder(w).Encode([]Release{
				{TagName: "v2.0.0-beta.1", Draft: false, Prerelease: true},
				{TagName: "v1.0.0", Draft: false, Prerelease: false},
			})
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{TagName: "v1.0.0"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Create a settings service with include_prereleases=true
	tmpDir := t.TempDir()
	settingsSvc := settings.NewService(
		settings.WithStorePath(filepath.Join(tmpDir, "settings.json")),
		settings.WithGroup(SettingsGroup()),
	)
	// Set include_prereleases to true
	_, _ = settingsSvc.SetValues(map[string]any{
		SettingCheckFrequency:     "daily",
		SettingAutoDownload:       false,
		SettingIncludePrereleases: true,
	})

	svc, err := NewService(
		WithCurrentVersion("v0.9.0"),
		WithGitHubRepo("owner", "repo"),
		WithSettings(settingsSvc),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel == nil {
		t.Fatal("expected release")
	}
	// Should find the prerelease as the latest
	if rel.TagName != "v2.0.0-beta.1" {
		t.Errorf("got tag %q, want %q", rel.TagName, "v2.0.0-beta.1")
	}
}

func TestCheckForUpdateWithoutSettingsFallsBackToOption(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only stable endpoint should be hit
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			t.Errorf("expected /releases/latest, got %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(Release{TagName: "v1.0.0"})
	}))
	defer srv.Close()

	// No settings, includePrereleases defaults to false
	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel != nil {
		t.Error("expected nil when up-to-date")
	}
}

func TestCheckForUpdateSettingsOverridesOption(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases":
			_ = json.NewEncoder(w).Encode([]Release{
				{TagName: "v2.0.0-rc.1", Draft: false, Prerelease: true},
				{TagName: "v1.0.0", Draft: false, Prerelease: false},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Option says false, but settings says true — settings wins
	tmpDir := t.TempDir()
	settingsSvc := settings.NewService(
		settings.WithStorePath(filepath.Join(tmpDir, "settings.json")),
		settings.WithGroup(SettingsGroup()),
	)
	_, _ = settingsSvc.SetValues(map[string]any{
		SettingCheckFrequency:     "daily",
		SettingAutoDownload:       false,
		SettingIncludePrereleases: true,
	})

	svc, err := NewService(
		WithCurrentVersion("v0.9.0"),
		WithGitHubRepo("owner", "repo"),
		WithIncludePrereleases(false), // static says no
		WithSettings(settingsSvc),     // settings says yes — wins
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel == nil {
		t.Fatal("expected release")
	}
	if rel.TagName != "v2.0.0-rc.1" {
		t.Errorf("got tag %q, want %q", rel.TagName, "v2.0.0-rc.1")
	}
}

func TestWithIncludePrereleasesWithoutSettings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases":
			_ = json.NewEncoder(w).Encode([]Release{
				{TagName: "v2.0.0-alpha.1", Draft: false, Prerelease: true},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := NewService(
		WithCurrentVersion("v0.9.0"),
		WithGitHubRepo("owner", "repo"),
		WithIncludePrereleases(true), // static option, no settings
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel == nil {
		t.Fatal("expected release")
	}
	if rel.TagName != "v2.0.0-alpha.1" {
		t.Errorf("got tag %q, want %q", rel.TagName, "v2.0.0-alpha.1")
	}
}

// TestRefuseDowngrade reproduces the downgrade-attack scenario from the
// security review: a user on v2.0.0 sees "v2.1.0 available", the feed then
// rolls back to an older release (this repo has literally done this — "fix:
// revert v2.0.0 release to v1.3.0") — and a later check must not silently
// overwrite the cache with the older, still-legitimately-signed release.
func TestRefuseDowngrade(t *testing.T) {
	var call int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 1 {
			_ = json.NewEncoder(w).Encode(Release{TagName: "v2.1.0"})
			return
		}
		// The feed rolled back.
		_ = json.NewEncoder(w).Encode(Release{TagName: "v1.3.0"})
	}))
	defer srv.Close()

	svc, err := NewService(
		WithCurrentVersion("v2.0.0"),
		WithGitHubRepo("owner", "repo"),
	)
	if err != nil {
		t.Fatal(err)
	}
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil || rel.TagName != "v2.1.0" {
		t.Fatalf("expected v2.1.0 to be reported available, got %+v", rel)
	}
	if cached := svc.GetLatestRelease(); cached == nil || cached.TagName != "v2.1.0" {
		t.Fatalf("expected v2.1.0 to be cached, got %+v", cached)
	}

	// A second check sees the rolled-back feed. It must report "no
	// update" and must NOT overwrite the cache with the older release.
	rel2, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel2 != nil {
		t.Fatalf("expected nil release after rollback, got %+v", rel2)
	}
	if stillCached := svc.GetLatestRelease(); stillCached == nil || stillCached.TagName != "v2.1.0" {
		t.Fatalf("cache was overwritten by a non-newer release: %+v", stillCached)
	}

	// Whitebox: even if an older release is force-set into the cache
	// (simulating any future code path that bypasses the newness gate in
	// CheckForUpdate), DownloadUpdate must independently refuse it.
	older, err := semver.ParseVersion("v1.3.0")
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.latestRelease = &Release{TagName: "v1.3.0", Version: older}
	svc.mu.Unlock()

	if _, err := svc.DownloadUpdate(context.Background()); err == nil {
		t.Fatal("expected DownloadUpdate to refuse a non-newer cached release")
	}
}

// TestApplyRechecksVersion whiteboxes a downloaded update whose recorded
// version is not newer than current — as if the check at download time
// were bypassed by a future refactor, or DownloadUpdate and ApplyUpdate
// were separated further than they are today. ApplyUpdate must refuse
// independently rather than trusting that Download already checked.
func TestApplyRechecksVersion(t *testing.T) {
	svc, err := NewService(
		WithCurrentVersion("v2.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithSkipVerification(),
	)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	assetPath := filepath.Join(dir, "app")
	if err := os.WriteFile(assetPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldVersion, err := semver.ParseVersion("v1.3.0")
	if err != nil {
		t.Fatal(err)
	}

	svc.mu.Lock()
	svc.downloadPath = assetPath
	svc.downloadVersion = oldVersion
	svc.downloadDir = dir
	svc.mu.Unlock()

	if err := svc.ApplyUpdate(context.Background()); err == nil {
		t.Fatal("expected ApplyUpdate to refuse applying a non-newer version")
	}
}

// TestAllowDowngradeBypassesGuard confirms WithAllowDowngrade is the only
// way past the newness guard, and that it must be explicitly opted into.
func TestAllowDowngradeBypassesGuard(t *testing.T) {
	svc, err := NewService(
		WithCurrentVersion("v2.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithSkipVerification(),
		WithAllowDowngrade(),
	)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	assetPath := filepath.Join(dir, "app")
	if err := os.WriteFile(assetPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldVersion, err := semver.ParseVersion("v1.3.0")
	if err != nil {
		t.Fatal(err)
	}

	svc.mu.Lock()
	svc.downloadPath = assetPath
	svc.downloadVersion = oldVersion
	svc.downloadDir = dir
	svc.mu.Unlock()

	if err := svc.ApplyUpdate(context.Background()); err != nil {
		t.Fatalf("expected WithAllowDowngrade to permit applying an older version, got: %v", err)
	}
}

// TestStagingDirPrivate verifies DownloadUpdate stages the asset under the
// app's private cache directory rather than the shared OS temp dir (Linux's
// /tmp is world-writable, mode 1777), and that the staging directory itself
// carries private (0700-class) permissions.
func TestStagingDirPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits not meaningful on Windows")
	}

	assetContent := []byte("binary")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v2.0.0",
				Assets: []Asset{
					{Name: "app", Size: int64(len(assetContent)), BrowserDownloadURL: "/download/app"},
				},
			})
		case "/download/app":
			_, _ = w.Write(assetContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithSkipVerification(),
		WithAssetPattern("app"),
		WithAppName("test-staging-private"),
	)
	if err != nil {
		t.Fatal(err)
	}
	dirs := svc.appDirs()
	t.Cleanup(func() { _ = os.RemoveAll(dirs.Cache()) })
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := range rel.Assets {
		rel.Assets[i].BrowserDownloadURL = srv.URL + rel.Assets[i].BrowserDownloadURL
	}

	path, err := svc.DownloadUpdate(context.Background())
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	if !strings.HasPrefix(path, dirs.Cache()) {
		t.Errorf("staged download %q is not under the private cache dir %q", path, dirs.Cache())
	}
	if strings.HasPrefix(path, os.TempDir()) {
		t.Errorf("staged download %q must not be under the shared OS temp dir %q", path, os.TempDir())
	}

	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Errorf("staging dir %q is not private: mode %04o", filepath.Dir(path), info.Mode().Perm())
	}
}

// TestEnsurePrivateDirRejectsLoosePermissions is the direct regression test
// for the Linux /tmp TOCTOU: os.MkdirAll succeeds silently on a directory
// that already exists with the wrong permissions (as a local attacker who
// pre-created it would leave it), so ensurePrivateDir must catch that after
// the fact rather than trusting MkdirAll's success.
func TestEnsurePrivateDirRejectsLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits not meaningful on Windows")
	}
	dir := filepath.Join(t.TempDir(), "preexisting")
	if err := os.Mkdir(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDir(dir); err == nil {
		t.Fatal("expected ensurePrivateDir to reject a pre-existing world-writable directory")
	}
}

func TestEnsurePrivateDirAcceptsFreshDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fresh")
	if err := ensurePrivateDir(dir); err != nil {
		t.Fatalf("expected ensurePrivateDir to accept a freshly created private dir: %v", err)
	}
}
