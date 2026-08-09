package updates

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jrschumacher/wails-kit/v2/events"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVerifySignatureValid(t *testing.T) {
	pub, priv, _ := generateMinisignKeys(t)

	dir := t.TempDir()
	assetPath := filepath.Join(dir, "app")
	content := []byte("binary content")
	writeFile(t, assetPath, string(content))

	sigPath := filepath.Join(dir, "app.minisig")
	writeFile(t, sigPath, signMinisignFile(priv, pub, content, "test asset"))

	if err := verifySignature(pub, assetPath, sigPath); err != nil {
		t.Fatalf("expected valid signature, got: %v", err)
	}
}

func TestVerifySignatureTamperedContent(t *testing.T) {
	pub, priv, _ := generateMinisignKeys(t)

	dir := t.TempDir()
	assetPath := filepath.Join(dir, "app")
	writeFile(t, assetPath, "binary content")

	sigPath := filepath.Join(dir, "app.minisig")
	// Sign different content than what's on disk.
	writeFile(t, sigPath, signMinisignFile(priv, pub, []byte("different content"), "test asset"))

	err := verifySignature(pub, assetPath, sigPath)
	if err == nil {
		t.Fatal("expected error for tampered content")
	}
}

func TestVerifySignatureTamperedTrustedComment(t *testing.T) {
	// The trusted comment is itself authenticated via minisign's global
	// signature. Flipping it after signing must be caught even though the
	// per-file signature bytes are untouched.
	pub, priv, _ := generateMinisignKeys(t)

	dir := t.TempDir()
	content := []byte("binary content")
	assetPath := filepath.Join(dir, "app")
	writeFile(t, assetPath, string(content))

	sig := signMinisignFile(priv, pub, content, "original comment")
	tampered := bytes.ReplaceAll([]byte(sig), []byte("original comment"), []byte("attacker comment"))

	sigPath := filepath.Join(dir, "app.minisig")
	writeFile(t, sigPath, string(tampered))

	err := verifySignature(pub, assetPath, sigPath)
	if err == nil {
		t.Fatal("expected error for tampered trusted comment")
	}
}

func TestVerifySignatureWrongKey(t *testing.T) {
	signingPub, signingPriv, _ := generateMinisignKeys(t)
	otherPub, _, _ := generateMinisignKeys(t)

	dir := t.TempDir()
	content := []byte("binary content")
	assetPath := filepath.Join(dir, "app")
	writeFile(t, assetPath, string(content))

	sigPath := filepath.Join(dir, "app.minisig")
	writeFile(t, sigPath, signMinisignFile(signingPriv, signingPub, content, "asset"))

	err := verifySignature(otherPub, assetPath, sigPath)
	if err == nil {
		t.Fatal("expected error for wrong key")
	}
}

func TestVerifySignatureBadSize(t *testing.T) {
	pub, _, _ := generateMinisignKeys(t)

	dir := t.TempDir()
	assetPath := filepath.Join(dir, "app")
	writeFile(t, assetPath, "content")
	sigPath := filepath.Join(dir, "app.minisig")
	writeFile(t, sigPath, "not a valid minisig file\n")

	err := verifySignature(pub, assetPath, sigPath)
	if err == nil {
		t.Fatal("expected error for malformed signature file")
	}
}

func TestVerifySignatureMissingAsset(t *testing.T) {
	pub, priv, _ := generateMinisignKeys(t)

	dir := t.TempDir()
	sigPath := filepath.Join(dir, "app.minisig")
	writeFile(t, sigPath, signMinisignFile(priv, pub, []byte("content"), "asset"))

	err := verifySignature(pub, filepath.Join(dir, "nonexistent"), sigPath)
	if err == nil {
		t.Fatal("expected error for missing asset file")
	}
}

func TestVerifySignatureMissingSigFile(t *testing.T) {
	pub, _, _ := generateMinisignKeys(t)

	dir := t.TempDir()
	assetPath := filepath.Join(dir, "app")
	writeFile(t, assetPath, "content")

	err := verifySignature(pub, assetPath, filepath.Join(dir, "nonexistent.minisig"))
	if err == nil {
		t.Fatal("expected error for missing sig file")
	}
}

func TestParseMinisignPublicKeyFullFile(t *testing.T) {
	_, _, pubFileText := generateMinisignKeys(t)

	pk, err := parseMinisignPublicKey(pubFileText)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pk.SignatureAlgorithm != [2]byte{'E', 'd'} {
		t.Errorf("unexpected signature algorithm: %v", pk.SignatureAlgorithm)
	}
}

func TestParseMinisignPublicKeyBareBase64(t *testing.T) {
	_, _, pubFileText := generateMinisignKeys(t)
	lines := splitLines(pubFileText)
	if len(lines) < 2 {
		t.Fatal("expected at least 2 lines in generated pub file")
	}

	pk, err := parseMinisignPublicKey(lines[1])
	if err != nil {
		t.Fatalf("unexpected error for bare base64 key: %v", err)
	}
	if pk.SignatureAlgorithm != [2]byte{'E', 'd'} {
		t.Errorf("unexpected signature algorithm: %v", pk.SignatureAlgorithm)
	}
}

func TestParseMinisignPublicKeyEmpty(t *testing.T) {
	if _, err := parseMinisignPublicKey(""); err == nil {
		t.Fatal("expected error for empty key")
	}
	if _, err := parseMinisignPublicKey("   \n  "); err == nil {
		t.Fatal("expected error for whitespace-only key")
	}
}

func TestParseMinisignPublicKeyGarbage(t *testing.T) {
	if _, err := parseMinisignPublicKey("not a key at all"); err == nil {
		t.Fatal("expected error for garbage key")
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func TestDownloadVerifiesSignature(t *testing.T) {
	pub, priv, pubFileText := generateMinisignKeys(t)

	assetContent := []byte("binary-content-v2")
	sig := signMinisignFile(priv, pub, assetContent, "v2.0.0")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v2.0.0",
				Assets: []Asset{
					{Name: fmt.Sprintf("app_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH), Size: int64(len(assetContent)), BrowserDownloadURL: "/download/app"},
					{Name: fmt.Sprintf("app_%s_%s.tar.gz.minisig", runtime.GOOS, runtime.GOARCH), Size: int64(len(sig)), BrowserDownloadURL: "/download/app.minisig"},
				},
			})
		case "/download/app":
			_, _ = w.Write(assetContent)
		case "/download/app.minisig":
			_, _ = w.Write([]byte(sig))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithPublicKey(pubFileText),
		WithAssetPattern("app_{os}_{arch}"),
		WithAppName("test-verify"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(svc.appDirs().Cache()) })
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil {
		t.Fatal("expected release")
	}
	for i := range rel.Assets {
		rel.Assets[i].BrowserDownloadURL = srv.URL + rel.Assets[i].BrowserDownloadURL
	}

	path, err := svc.DownloadUpdate(context.Background())
	if err != nil {
		t.Fatalf("expected successful download, got: %v", err)
	}

	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, assetContent) {
		t.Error("downloaded content mismatch")
	}
}

func TestDownloadFailsOnBadSignature(t *testing.T) {
	_, _, pubFileText := generateMinisignKeys(t)
	_, wrongPriv, wrongPubText := generateMinisignKeys(t)
	wrongPub, err := parseMinisignPublicKey(wrongPubText)
	if err != nil {
		t.Fatal(err)
	}

	assetContent := []byte("binary-content-v2")
	sig := signMinisignFile(wrongPriv, wrongPub, assetContent, "v2.0.0") // signed with wrong key

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v2.0.0",
				Assets: []Asset{
					{Name: fmt.Sprintf("app_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH), Size: int64(len(assetContent)), BrowserDownloadURL: "/download/app"},
					{Name: fmt.Sprintf("app_%s_%s.tar.gz.minisig", runtime.GOOS, runtime.GOARCH), Size: int64(len(sig)), BrowserDownloadURL: "/download/app.minisig"},
				},
			})
		case "/download/app":
			_, _ = w.Write(assetContent)
		case "/download/app.minisig":
			_, _ = w.Write([]byte(sig))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	mem := events.NewMemoryEmitter()
	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithPublicKey(pubFileText),
		WithAssetPattern("app_{os}_{arch}"),
		WithAppName("test-verify-bad"),
		WithEmitter(events.NewEmitter(mem)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(svc.appDirs().Cache()) })
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
	_, _, pubFileText := generateMinisignKeys(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v2.0.0",
				Assets: []Asset{
					{Name: fmt.Sprintf("app_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH), BrowserDownloadURL: "/download/app"},
					// No .minisig asset
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
		WithPublicKey(pubFileText),
		WithAssetPattern("app_{os}_{arch}"),
		WithAppName("test-verify-missing"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(svc.appDirs().Cache()) })
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
		t.Fatal("expected error when .minisig asset is missing")
	}
}

func TestDownloadSkipVerification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v2.0.0",
				Assets: []Asset{
					{Name: fmt.Sprintf("app_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH), BrowserDownloadURL: "/download/app"},
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
	t.Cleanup(func() { _ = os.RemoveAll(svc.appDirs().Cache()) })
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := range rel.Assets {
		rel.Assets[i].BrowserDownloadURL = srv.URL + rel.Assets[i].BrowserDownloadURL
	}

	_, err = svc.DownloadUpdate(context.Background())
	if err != nil {
		t.Fatalf("expected download to succeed with skip verification: %v", err)
	}
}

func TestDownloadNoKeyNoVerification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v2.0.0",
				Assets: []Asset{
					{Name: fmt.Sprintf("app_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH), BrowserDownloadURL: "/download/app"},
				},
			})
		case "/download/app":
			_, _ = w.Write([]byte("binary"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// No public key, no skip — verification is skipped but a warning is logged.
	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithAssetPattern("app_{os}_{arch}"),
		WithAppName("test-no-key"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(svc.appDirs().Cache()) })
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := range rel.Assets {
		rel.Assets[i].BrowserDownloadURL = srv.URL + rel.Assets[i].BrowserDownloadURL
	}

	_, err = svc.DownloadUpdate(context.Background())
	if err != nil {
		t.Fatalf("expected download to succeed without key: %v", err)
	}
}

func TestNewServiceRejectsInvalidPublicKey(t *testing.T) {
	_, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithPublicKey("not a valid minisign key"),
	)
	if err == nil {
		t.Fatal("expected NewService to reject a malformed public key")
	}
}

func TestApplyReVerifiesSignatureBeforeExtraction(t *testing.T) {
	pub, priv, pubFileText := generateMinisignKeys(t)

	assetContent := []byte("binary-content-v2")
	sig := signMinisignFile(priv, pub, assetContent, "v2.0.0")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v2.0.0",
				Assets: []Asset{
					{Name: "app", Size: int64(len(assetContent)), BrowserDownloadURL: "/download/app"},
					{Name: "app.minisig", Size: int64(len(sig)), BrowserDownloadURL: "/download/app.minisig"},
				},
			})
		case "/download/app":
			_, _ = w.Write(assetContent)
		case "/download/app.minisig":
			_, _ = w.Write([]byte(sig))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := NewService(
		WithCurrentVersion("v1.0.0"),
		WithGitHubRepo("owner", "repo"),
		WithPublicKey(pubFileText),
		WithAssetPattern("app"),
		WithBinaryName("app"),
		WithAppName("test-reverify-apply"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(svc.appDirs().Cache()) })
	svc.github.apiURL = srv.URL

	rel, err := svc.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := range rel.Assets {
		rel.Assets[i].BrowserDownloadURL = srv.URL + rel.Assets[i].BrowserDownloadURL
	}

	downloadPath, err := svc.DownloadUpdate(context.Background())
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	// Simulate a local attacker swapping the staged asset after
	// verification but before Apply — this is exactly the TOCTOU window
	// the re-verify-before-extract defense closes.
	if err := os.WriteFile(downloadPath, []byte("tampered-content"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = svc.ApplyUpdate(context.Background())
	if err == nil {
		t.Fatal("expected ApplyUpdate to reject a tampered staged file")
	}
}

// TestVerifySignatureRealMinisignCLI is an interoperability golden test.
//
// Every other test in this file signs with our own Go helpers, which proves the
// verifier is self-consistent but not that it can read what the actual minisign
// tool emits. Since the README instructs release engineers to sign with the
// minisign CLI, self-consistency is the wrong thing to prove — a divergence
// between our encoder and the real format would leave every test green while
// every real release failed to verify.
//
// The fixtures below were produced by minisign 0.12:
//
//	printf 'pretend release tarball\n' > myapp_darwin_arm64.tar.gz
//	minisign -G -W -p minisign.pub -s minisign.key
//	minisign -S -s minisign.key -m myapp_darwin_arm64.tar.gz -t "myapp 2.0.0"
//
// Do not regenerate them casually: their value is that they came from the tool,
// not from this package.
func TestVerifySignatureRealMinisignCLI(t *testing.T) {
	const (
		cliPublicKey = "untrusted comment: minisign public key AE7C49152F6CAACE\n" +
			"RWTOqmwvFUl8riCrT4qLxxOT0F+5agcs4oCVu7x2ePvYdtzMDdVIpEE8\n"

		cliSignature = "untrusted comment: signature from minisign secret key\n" +
			"RUTOqmwvFUl8rijq2cVPvPFGxwietJYO2rUDJZQgBMXKk7FFi94iXP2uURYCVI8DLEeEL8k8OJMiI6hsnWBOC/kATMj4wZANjAc=\n" +
			"trusted comment: myapp 2.0.0\n" +
			"F34+D6c/luIC88b6uX6wnOG3Tberb7yeBahjrhuCY6EZK1P7PQj/cmhQ/rdIh5o4ePaNcVdFWSatLp7t8wt1Bg==\n"

		cliArtifact = "pretend release tarball\n"
	)

	pub, err := parseMinisignPublicKey(cliPublicKey)
	if err != nil {
		t.Fatalf("parse CLI-generated public key: %v", err)
	}

	dir := t.TempDir()
	assetPath := filepath.Join(dir, "myapp_darwin_arm64.tar.gz")
	sigPath := assetPath + ".minisig"
	writeFile(t, assetPath, cliArtifact)
	writeFile(t, sigPath, cliSignature)

	if err := verifySignature(pub, assetPath, sigPath); err != nil {
		t.Fatalf("failed to verify a signature produced by the real minisign CLI: %v", err)
	}

	// The same fixtures must fail on a single flipped byte. Without this, the
	// test above could pass against a verifier that accepts anything.
	writeFile(t, assetPath, "pretend release tarbalL\n")
	if err := verifySignature(pub, assetPath, sigPath); err == nil {
		t.Fatal("tampered artifact verified against a real CLI signature; verifier accepts anything")
	}
}
