package updates

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jrschumacher/wails-kit/events"
	"github.com/jrschumacher/wails-kit/settings"
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

