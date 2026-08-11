package updates

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func newTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

func TestLatestRelease(t *testing.T) {
	release := Release{
		TagName:    "v1.2.3",
		Name:       "Release v1.2.3",
		Body:       "Bug fixes",
		Draft:      false,
		Prerelease: false,
		HTMLURL:    "https://github.com/owner/repo/releases/tag/v1.2.3",
		Assets: []Asset{
			{Name: "app_darwin_arm64.tar.gz", Size: 1024, BrowserDownloadURL: "https://example.com/app.tar.gz"},
		},
	}

	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("missing Accept header")
		}
		_ = json.NewEncoder(w).Encode(release)
	})
	defer srv.Close()

	g := &GitHubSource{
		owner:  "owner",
		repo:   "repo",
		apiURL: srv.URL,
	}

	got, err := g.LatestRelease(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TagName != "v1.2.3" {
		t.Errorf("got tag %q, want %q", got.TagName, "v1.2.3")
	}
	if got.Version.Major != 1 || got.Version.Minor != 2 || got.Version.Patch != 3 {
		t.Errorf("got version %+v, want 1.2.3", got.Version)
	}
	if len(got.Assets) != 1 {
		t.Errorf("got %d assets, want 1", len(got.Assets))
	}
}

func TestLatestReleaseWithToken(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("got auth %q, want %q", auth, "Bearer test-token")
		}
		_ = json.NewEncoder(w).Encode(Release{TagName: "v1.0.0"})
	})
	defer srv.Close()

	g := &GitHubSource{
		owner:  "owner",
		repo:   "repo",
		apiURL: srv.URL,
		token:  "test-token",
	}

	_, err := g.LatestRelease(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLatestReleaseNotFound(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	g := &GitHubSource{owner: "owner", repo: "repo", apiURL: srv.URL}
	_, err := g.LatestRelease(context.Background(), false)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestLatestReleaseRateLimit(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	defer srv.Close()

	g := &GitHubSource{owner: "owner", repo: "repo", apiURL: srv.URL}
	_, err := g.LatestRelease(context.Background(), false)
	if err == nil {
		t.Fatal("expected error for rate limit")
	}
}

func TestDownloadAsset(t *testing.T) {
	content := []byte("binary-content-here")
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "19")
		_, _ = w.Write(content)
	})
	defer srv.Close()

	g := &GitHubSource{apiURL: srv.URL}
	asset := &Asset{
		Name:               "app_darwin_arm64",
		Size:               19,
		BrowserDownloadURL: srv.URL + "/download",
	}

	var buf bytes.Buffer
	var lastDownloaded int64
	err := g.DownloadAsset(context.Background(), asset, &buf, func(downloaded, total int64) {
		lastDownloaded = downloaded
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.String() != "binary-content-here" {
		t.Errorf("got %q, want %q", buf.String(), "binary-content-here")
	}
	if lastDownloaded != 19 {
		t.Errorf("final progress: got %d, want 19", lastDownloaded)
	}
}

func TestFindAsset(t *testing.T) {
	release := &Release{
		Assets: []Asset{
			{Name: "app_linux_amd64.tar.gz"},
			{Name: "app_darwin_arm64.tar.gz"},
			{Name: "app_windows_amd64.zip"},
		},
	}

	// FindAsset uses runtime.GOOS/GOARCH so we test with a pattern
	// that matches a known asset
	asset, err := FindAsset(release, "app_{os}_{arch}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asset == nil {
		t.Fatal("expected to find an asset")
	}
}

func TestFindAssetPrefersExactArchiveOverAdjacentArtifacts(t *testing.T) {
	pattern := "app_{os}_{arch}"
	candidate := buildCandidateNames(pattern, runtime.GOOS, runtime.GOARCH)[0]
	release := &Release{
		Assets: []Asset{
			{Name: candidate + "_debug.tar.gz"},
			{Name: candidate + ".tar.gz.sig"},
			{Name: candidate + ".tar.gz"},
		},
	}

	asset, err := FindAsset(release, pattern)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asset == nil {
		t.Fatal("expected to find an asset")
	}
	if asset.Name != candidate+".tar.gz" {
		t.Fatalf("got %q, want exact archive match", asset.Name)
	}
}

func TestFindAssetNotFound(t *testing.T) {
	release := &Release{
		TagName: "v1.0.0",
		Assets: []Asset{
			{Name: "app_freebsd_mips.tar.gz"},
		},
	}

	_, err := FindAsset(release, "app_{os}_{arch}")
	if err == nil {
		t.Fatal("expected error when no matching asset")
	}
}

func TestBuildCandidateNames(t *testing.T) {
	candidates := buildCandidateNames("myapp_{os}_{arch}", "darwin", "arm64")

	// Should include darwin_arm64 and macos_arm64 and mac_arm64, plus aarch64 variants
	found := map[string]bool{}
	for _, c := range candidates {
		found[c] = true
	}

	if !found["myapp_darwin_arm64"] {
		t.Error("missing myapp_darwin_arm64")
	}
	if !found["myapp_macos_arm64"] {
		t.Error("missing myapp_macos_arm64")
	}
	if !found["myapp_darwin_aarch64"] {
		t.Error("missing myapp_darwin_aarch64")
	}
}

func TestLatestReleaseIncludingPrereleases(t *testing.T) {
	releases := []Release{
		{TagName: "v1.0.0", Draft: false, Prerelease: false},
		{TagName: "v2.0.0-beta.1", Draft: false, Prerelease: true},
		{TagName: "v0.9.0", Draft: false, Prerelease: false},
		{TagName: "v3.0.0", Draft: true, Prerelease: false}, // draft, should be skipped
	}

	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/releases" {
			_ = json.NewEncoder(w).Encode(releases)
			return
		}
		http.NotFound(w, r)
	})
	defer srv.Close()

	g := &GitHubSource{owner: "owner", repo: "repo", apiURL: srv.URL}

	got, err := g.LatestRelease(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// v2.0.0-beta.1 is the newest non-draft release
	if got.TagName != "v2.0.0-beta.1" {
		t.Errorf("got tag %q, want %q", got.TagName, "v2.0.0-beta.1")
	}
}

func TestLatestReleaseStableIgnoresPrereleases(t *testing.T) {
	// When includePrereleases=false, it uses /releases/latest which
	// GitHub already filters to stable releases only
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/releases/latest" {
			_ = json.NewEncoder(w).Encode(Release{TagName: "v1.0.0"})
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
		http.NotFound(w, r)
	})
	defer srv.Close()

	g := &GitHubSource{owner: "owner", repo: "repo", apiURL: srv.URL}

	got, err := g.LatestRelease(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TagName != "v1.0.0" {
		t.Errorf("got tag %q, want %q", got.TagName, "v1.0.0")
	}
}

func TestBuildCandidateNamesNoPattern(t *testing.T) {
	candidates := buildCandidateNames("", "linux", "amd64")

	found := map[string]bool{}
	for _, c := range candidates {
		found[c] = true
	}

	if !found["linux_amd64"] {
		t.Error("missing linux_amd64")
	}
	if !found["linux-amd64"] {
		t.Error("missing linux-amd64")
	}
	if !found["linux_x86_64"] {
		t.Error("missing linux_x86_64")
	}
}

// TestDownloadAssetRejectsOversizedResponse is the regression test for the
// missing download size cap: a compromised or malicious feed that streams
// more bytes than the release metadata declared must be cut off rather than
// buffered to EOF, which would let it fill the disk before signature
// verification ever runs.
func TestDownloadAssetRejectsOversizedResponse(t *testing.T) {
	// The server ignores the declared size and just keeps writing.
	oversized := bytes.Repeat([]byte("x"), 1024)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(oversized)
	})
	defer srv.Close()

	g := &GitHubSource{apiURL: srv.URL}
	asset := &Asset{
		Name:               "app",
		Size:               100, // much smaller than what the server actually sends
		BrowserDownloadURL: srv.URL + "/download",
	}

	var buf bytes.Buffer
	err := g.DownloadAsset(context.Background(), asset, &buf, nil)
	if err == nil {
		t.Fatal("expected error when the response exceeds the declared asset size")
	}
	if buf.Len() > int(asset.Size)+1 {
		t.Errorf("wrote %d bytes to dest, expected at most %d (size cap + 1 to detect overflow)", buf.Len(), asset.Size+1)
	}
}

func TestDownloadAssetAcceptsExactDeclaredSize(t *testing.T) {
	content := bytes.Repeat([]byte("y"), 256)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	})
	defer srv.Close()

	g := &GitHubSource{apiURL: srv.URL}
	asset := &Asset{
		Name:               "app",
		Size:               int64(len(content)),
		BrowserDownloadURL: srv.URL + "/download",
	}

	var buf bytes.Buffer
	if err := g.DownloadAsset(context.Background(), asset, &buf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), content) {
		t.Error("content mismatch for a download exactly matching the declared size")
	}
}

// TestDownloadAssetPrefersAPIURLForPrivateRepos confirms the fix for the
// suspected private-repo bug: browser_download_url requires a browser
// session and 404s for API/token-authenticated requests against a private
// repo, so DownloadAsset must use the API asset endpoint (Asset.URL) with
// an "Accept: application/octet-stream" header and the configured token
// whenever the release metadata provides it.
func TestDownloadAssetPrefersAPIURLForPrivateRepos(t *testing.T) {
	content := []byte("private-repo-binary")
	var hitAPIEndpoint, hitBrowserURL bool

	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/assets/12345":
			hitAPIEndpoint = true
			if got := r.Header.Get("Accept"); got != "application/octet-stream" {
				t.Errorf("missing/wrong Accept header on API asset endpoint: %q", got)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer priv-token" {
				t.Errorf("missing/wrong Authorization header on API asset endpoint: %q", got)
			}
			_, _ = w.Write(content)
		case "/download/should-not-be-used":
			hitBrowserURL = true
			http.NotFound(w, r) // simulates the real-world 404 for private repos
		default:
			http.NotFound(w, r)
		}
	})
	defer srv.Close()

	g := &GitHubSource{apiURL: srv.URL, token: "priv-token"}
	asset := &Asset{
		Name:               "app",
		Size:               int64(len(content)),
		BrowserDownloadURL: srv.URL + "/download/should-not-be-used",
		URL:                srv.URL + "/repos/owner/repo/releases/assets/12345",
	}

	var buf bytes.Buffer
	if err := g.DownloadAsset(context.Background(), asset, &buf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), content) {
		t.Error("content mismatch")
	}
	if !hitAPIEndpoint {
		t.Error("expected DownloadAsset to use the API asset endpoint (Asset.URL)")
	}
	if hitBrowserURL {
		t.Error("expected DownloadAsset not to fall back to BrowserDownloadURL when Asset.URL is set")
	}
}

// TestDownloadAssetFallsBackToBrowserURLWhenAPIURLUnset covers
// hand-constructed Asset values (as used throughout the rest of this test
// suite, and plausible for a custom Source implementation) that never set
// URL — DownloadAsset must still work against BrowserDownloadURL alone.
func TestDownloadAssetFallsBackToBrowserURLWhenAPIURLUnset(t *testing.T) {
	content := []byte("public-repo-binary")
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(content)
	})
	defer srv.Close()

	g := &GitHubSource{apiURL: srv.URL}
	asset := &Asset{
		Name:               "app",
		Size:               int64(len(content)),
		BrowserDownloadURL: srv.URL + "/download",
	}

	var buf bytes.Buffer
	if err := g.DownloadAsset(context.Background(), asset, &buf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), content) {
		t.Error("content mismatch")
	}
}
