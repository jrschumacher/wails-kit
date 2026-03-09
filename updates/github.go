package updates

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
	"unicode"
)

// Release represents a GitHub release.
type Release struct {
	TagName    string  `json:"tag_name"`
	Name       string  `json:"name"`
	Body       string  `json:"body"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	HTMLURL    string  `json:"html_url"`
	Assets     []Asset `json:"assets"`
	Version    Version `json:"-"`
}

// Asset represents a downloadable file from a release.
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
}

// GitHubSource fetches release info from the GitHub Releases API.
type GitHubSource struct {
	owner  string
	repo   string
	client *http.Client
	token  string
	apiURL string // defaults to "https://api.github.com"
}

// LatestRelease fetches the latest release. When includePrereleases is false,
// it uses the /releases/latest endpoint (which excludes drafts and prereleases).
// When true, it lists recent releases and returns the newest by semver.
func (g *GitHubSource) LatestRelease(ctx context.Context, includePrereleases bool) (*Release, error) {
	base := g.apiURL
	if base == "" {
		base = "https://api.github.com"
	}

	if !includePrereleases {
		return g.fetchLatestStable(ctx, base)
	}
	return g.fetchLatestIncludingPrereleases(ctx, base)
}

func (g *GitHubSource) fetchLatestStable(ctx context.Context, base string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", base, g.owner, g.repo)
	rel, err := g.fetchRelease(ctx, url)
	if err != nil {
		return nil, err
	}
	return rel, nil
}

func (g *GitHubSource) fetchLatestIncludingPrereleases(ctx context.Context, base string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=10", base, g.owner, g.repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	g.setHeaders(req)

	resp, err := g.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch releases: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkResponse(resp, g.owner, g.repo); err != nil {
		return nil, err
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode releases: %w", err)
	}

	var best *Release
	for i := range releases {
		rel := &releases[i]
		if rel.Draft {
			continue
		}
		v, err := ParseVersion(rel.TagName)
		if err != nil {
			continue
		}
		rel.Version = v
		if best == nil || v.NewerThan(best.Version) {
			best = rel
		}
	}

	if best == nil {
		return nil, fmt.Errorf("no releases found for %s/%s", g.owner, g.repo)
	}
	return best, nil
}

func (g *GitHubSource) fetchRelease(ctx context.Context, url string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	g.setHeaders(req)

	resp, err := g.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkResponse(resp, g.owner, g.repo); err != nil {
		return nil, err
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}

	v, err := ParseVersion(rel.TagName)
	if err != nil {
		return nil, fmt.Errorf("parse release tag %q: %w", rel.TagName, err)
	}
	rel.Version = v

	return &rel, nil
}

func (g *GitHubSource) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
}

func checkResponse(resp *http.Response, owner, repo string) error {
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("no releases found for %s/%s", owner, repo)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("GitHub API rate limit exceeded (status %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}
	return nil
}

// FindAsset selects the asset matching the current OS and architecture.
// It uses the assetPattern template with {os} and {arch} placeholders,
// falling back to common naming conventions.
func FindAsset(release *Release, pattern string) (*Asset, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Build candidate names from the pattern
	candidates := buildCandidateNames(pattern, goos, goarch)
	var fuzzy *Asset

	for i := range release.Assets {
		asset := &release.Assets[i]
		lower := strings.ToLower(asset.Name)
		if isSignatureLikeAsset(lower) {
			continue
		}
		base := stripAssetExtensions(lower)
		for _, candidate := range candidates {
			lowerCandidate := strings.ToLower(candidate)
			if lower == lowerCandidate || base == lowerCandidate {
				return asset, nil
			}
			if fuzzy == nil && (tokenContains(base, lowerCandidate) || tokenContains(lower, lowerCandidate)) {
				fuzzy = asset
			}
		}
	}
	if fuzzy != nil {
		return fuzzy, nil
	}

	return nil, fmt.Errorf("no asset found for %s/%s in release %s (tried patterns: %v)",
		goos, goarch, release.TagName, candidates)
}

// DownloadAsset downloads an asset, reporting progress via the callback.
func (g *GitHubSource) DownloadAsset(ctx context.Context, asset *Asset, dest io.Writer, progress func(downloaded, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := g.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("download asset: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)
	lastReport := time.Time{}

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := dest.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("write downloaded data: %w", writeErr)
			}
			downloaded += int64(n)
			if progress != nil && time.Since(lastReport) > 250*time.Millisecond {
				progress(downloaded, total)
				lastReport = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read download stream: %w", readErr)
		}
	}

	// Final progress report
	if progress != nil {
		progress(downloaded, total)
	}

	return nil
}

func (g *GitHubSource) httpClient() *http.Client {
	if g.client != nil {
		return g.client
	}
	return http.DefaultClient
}

// buildCandidateNames generates asset name patterns for matching.
func buildCandidateNames(pattern, goos, goarch string) []string {
	// OS name variants
	osNames := []string{goos}
	switch goos {
	case "darwin":
		osNames = append(osNames, "macos", "mac")
	case "windows":
		osNames = append(osNames, "win")
	}

	// Arch name variants
	archNames := []string{goarch}
	switch goarch {
	case "amd64":
		archNames = append(archNames, "x86_64", "x64")
	case "arm64":
		archNames = append(archNames, "aarch64")
	case "386":
		archNames = append(archNames, "i386", "x86")
	}

	var candidates []string
	if pattern != "" {
		// Use the pattern with substitutions
		for _, os := range osNames {
			for _, arch := range archNames {
				name := strings.ReplaceAll(pattern, "{os}", os)
				name = strings.ReplaceAll(name, "{arch}", arch)
				candidates = append(candidates, name)
			}
		}
	} else {
		// Default: try "{os}_{arch}" and "{os}-{arch}"
		for _, os := range osNames {
			for _, arch := range archNames {
				candidates = append(candidates, os+"_"+arch)
				candidates = append(candidates, os+"-"+arch)
			}
		}
	}

	return candidates
}

func isSignatureLikeAsset(name string) bool {
	for _, suffix := range []string{".sig", ".sha256", ".sha512", ".checksums", ".checksum"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func stripAssetExtensions(name string) string {
	for _, suffix := range []string{
		".tar.gz",
		".tgz",
		".zip",
		".tar",
		".gz",
		".bz2",
		".xz",
		".dmg",
		".pkg",
		".msi",
		".exe",
		".appimage",
	} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return name
}

func tokenContains(name, candidate string) bool {
	for start := strings.Index(name, candidate); start >= 0; {
		end := start + len(candidate)
		beforeOK := start == 0 || !isAlphaNumeric(rune(name[start-1]))
		afterOK := end == len(name) || !isAlphaNumeric(rune(name[end]))
		if beforeOK && afterOK {
			return true
		}
		next := strings.Index(name[start+1:], candidate)
		if next < 0 {
			break
		}
		start += next + 1
	}
	return false
}

func isAlphaNumeric(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
