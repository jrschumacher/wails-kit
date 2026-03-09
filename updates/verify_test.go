package updates

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrschumacher/wails-kit/events"
)

func generateTestKeys(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return pub, priv
}

func TestVerifySignatureValid(t *testing.T) {
	pub, priv := generateTestKeys(t)

	dir := t.TempDir()
	assetPath := filepath.Join(dir, "app")
	content := []byte("binary content")
	if err := os.WriteFile(assetPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	sig := ed25519.Sign(priv, content)
	sigPath := filepath.Join(dir, "app.sig")
	if err := os.WriteFile(sigPath, sig, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := verifySignature(pub, assetPath, sigPath); err != nil {
		t.Fatalf("expected valid signature, got: %v", err)
	}
}

func TestVerifySignatureInvalid(t *testing.T) {
	pub, priv := generateTestKeys(t)

	dir := t.TempDir()
	assetPath := filepath.Join(dir, "app")
	if err := os.WriteFile(assetPath, []byte("binary content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Sign different content
	sig := ed25519.Sign(priv, []byte("different content"))
	sigPath := filepath.Join(dir, "app.sig")
	if err := os.WriteFile(sigPath, sig, 0o644); err != nil {
		t.Fatal(err)
	}

	err := verifySignature(pub, assetPath, sigPath)
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
	_ = pub
}

func TestVerifySignatureWrongKey(t *testing.T) {
	_, priv := generateTestKeys(t)
	otherPub, _ := generateTestKeys(t)

	dir := t.TempDir()
	content := []byte("binary content")
	assetPath := filepath.Join(dir, "app")
	if err := os.WriteFile(assetPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	sig := ed25519.Sign(priv, content)
	sigPath := filepath.Join(dir, "app.sig")
	if err := os.WriteFile(sigPath, sig, 0o644); err != nil {
		t.Fatal(err)
	}

	err := verifySignature(otherPub, assetPath, sigPath)
	if err == nil {
		t.Fatal("expected error for wrong key")
	}
}

func TestVerifySignatureBadSize(t *testing.T) {
	pub, _ := generateTestKeys(t)

	dir := t.TempDir()
	assetPath := filepath.Join(dir, "app")
	if err := os.WriteFile(assetPath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	sigPath := filepath.Join(dir, "app.sig")
	if err := os.WriteFile(sigPath, []byte("short"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := verifySignature(pub, assetPath, sigPath)
	if err == nil {
		t.Fatal("expected error for bad signature size")
	}
}

func TestVerifySignatureMissingAsset(t *testing.T) {
	pub, _ := generateTestKeys(t)

	dir := t.TempDir()
	sigPath := filepath.Join(dir, "app.sig")
	if err := os.WriteFile(sigPath, make([]byte, ed25519.SignatureSize), 0o644); err != nil {
		t.Fatal(err)
	}

	err := verifySignature(pub, filepath.Join(dir, "nonexistent"), sigPath)
	if err == nil {
		t.Fatal("expected error for missing asset file")
	}
}

func TestVerifySignatureMissingSigFile(t *testing.T) {
	pub, _ := generateTestKeys(t)

	dir := t.TempDir()
	assetPath := filepath.Join(dir, "app")
	if err := os.WriteFile(assetPath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := verifySignature(pub, assetPath, filepath.Join(dir, "nonexistent.sig"))
	if err == nil {
		t.Fatal("expected error for missing sig file")
	}
}

func TestDownloadVerifiesSignature(t *testing.T) {
	pub, priv := generateTestKeys(t)

	assetContent := []byte("binary-content-v2")
	sig := ed25519.Sign(priv, assetContent)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v2.0.0",
				Assets: []Asset{
					{Name: "app_darwin_arm64.tar.gz", BrowserDownloadURL: "/download/app"},
					{Name: "app_darwin_arm64.tar.gz.sig", BrowserDownloadURL: "/download/app.sig"},
				},
			})
		case "/download/app":
			_, _ = w.Write(assetContent)
		case "/download/app.sig":
			_, _ = w.Write(sig)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Fix asset download URLs to point to test server
	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithPublicKey(pub),
		WithAssetPattern("app_{os}_{arch}"),
		WithAppName("test-verify"),
	)
	if err != nil {
		t.Fatal(err)
	}
	svc.github.apiURL = srv.URL

	// Patch asset URLs after check
	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil {
		t.Fatal("expected release")
	}
	// Rewrite download URLs to point at test server
	for i := range rel.Assets {
		rel.Assets[i].BrowserDownloadURL = srv.URL + rel.Assets[i].BrowserDownloadURL
	}

	path, err := svc.DownloadUpdate(context.Background())
	if err != nil {
		t.Fatalf("expected successful download, got: %v", err)
	}
	defer func() { _ = os.Remove(path) }()

	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, assetContent) {
		t.Error("downloaded content mismatch")
	}
}

func TestDownloadFailsOnBadSignature(t *testing.T) {
	pub, _ := generateTestKeys(t)
	_, wrongPriv := generateTestKeys(t)

	assetContent := []byte("binary-content-v2")
	sig := ed25519.Sign(wrongPriv, assetContent) // signed with wrong key

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v2.0.0",
				Assets: []Asset{
					{Name: "app_darwin_arm64.tar.gz", BrowserDownloadURL: "/download/app"},
					{Name: "app_darwin_arm64.tar.gz.sig", BrowserDownloadURL: "/download/app.sig"},
				},
			})
		case "/download/app":
			_, _ = w.Write(assetContent)
		case "/download/app.sig":
			_, _ = w.Write(sig)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	mem := events.NewMemoryEmitter()
	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithPublicKey(pub),
		WithAssetPattern("app_{os}_{arch}"),
		WithAppName("test-verify-bad"),
		WithEmitter(events.NewEmitter(mem)),
	)
	if err != nil {
		t.Fatal(err)
	}
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := range rel.Assets {
		rel.Assets[i].BrowserDownloadURL = srv.URL + rel.Assets[i].BrowserDownloadURL
	}

	_, err = svc.DownloadUpdate(context.Background())
	if err == nil {
		t.Fatal("expected error for bad signature")
	}

	// Should emit error event with verify code
	evts := mem.Events()
	foundVerifyError := false
	for _, e := range evts {
		if e.Name == EventError {
			foundVerifyError = true
		}
	}
	if !foundVerifyError {
		t.Error("expected error event for verification failure")
	}
}

func TestDownloadFailsOnMissingSigAsset(t *testing.T) {
	pub, _ := generateTestKeys(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v2.0.0",
				Assets: []Asset{
					{Name: "app_darwin_arm64.tar.gz", BrowserDownloadURL: "/download/app"},
					// No .sig asset
				},
			})
		case "/download/app":
			_, _ = w.Write([]byte("binary"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithPublicKey(pub),
		WithAssetPattern("app_{os}_{arch}"),
		WithAppName("test-verify-missing"),
	)
	if err != nil {
		t.Fatal(err)
	}
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := range rel.Assets {
		rel.Assets[i].BrowserDownloadURL = srv.URL + rel.Assets[i].BrowserDownloadURL
	}

	_, err = svc.DownloadUpdate(context.Background())
	if err == nil {
		t.Fatal("expected error when .sig asset is missing")
	}
}

func TestDownloadSkipVerification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v2.0.0",
				Assets: []Asset{
					{Name: "app_darwin_arm64.tar.gz", BrowserDownloadURL: "/download/app"},
				},
			})
		case "/download/app":
			_, _ = w.Write([]byte("binary"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithSkipVerification(),
		WithAssetPattern("app_{os}_{arch}"),
		WithAppName("test-skip-verify"),
	)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatalf("expected download to succeed with skip verification: %v", err)
	}
	defer func() { _ = os.Remove(path) }()
}

func TestDownloadNoKeyNoVerification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v2.0.0",
				Assets: []Asset{
					{Name: "app_darwin_arm64.tar.gz", BrowserDownloadURL: "/download/app"},
				},
			})
		case "/download/app":
			_, _ = w.Write([]byte("binary"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// No public key, no skip — verification is a no-op
	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithAssetPattern("app_{os}_{arch}"),
		WithAppName("test-no-key"),
	)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatalf("expected download to succeed without key: %v", err)
	}
	defer func() { _ = os.Remove(path) }()
}
