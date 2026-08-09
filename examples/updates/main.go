// Command updates-example demonstrates the updates.Service check/download/
// apply flow — including minisign signature verification — against a local
// httptest server standing in for the GitHub Releases API. It needs no real
// network access and never touches the running example binary, so it's
// safe to run repeatedly and safe to run in CI.
//
// A real app:
//   - embeds its minisign public key via go:embed instead of generating one
//     at runtime (see updates/README.md, "Generating a keypair");
//   - does not set WithApplier, letting ApplyUpdate replace the running
//     executable directly via the default atomic-rename strategy;
//   - does not set WithGitHubAPIURL, letting it default to the real GitHub
//     API.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	minisign "github.com/jedisct1/go-minisign"

	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/updates"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dir, err := os.MkdirTemp("", "updates-example-*")
	if err != nil {
		return fmt.Errorf("create scratch dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	pub, priv, pubFileText := generateDemoKeypair()

	// A bare binary asset (no archive extension) — DownloadUpdate handles
	// .tar.gz/.zip archives too (see README "Archive extraction"), but a
	// plain binary keeps this example focused on the check/verify/apply
	// flow rather than archive layout.
	const assetName = "myapp_demo"
	assetContent := []byte("pretend this is a compiled binary, v2.0.0")
	sigText := signDemoAsset(priv, pub, assetContent, "myapp v2.0.0")

	srv := newFakeGitHubServer(assetName, assetContent, sigText)
	defer srv.Close()

	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)

	// Stand in for "the currently installed app" — ApplyUpdate normally
	// replaces os.Executable() directly; this example redirects that via
	// a custom Applier so it never touches the example binary itself.
	installedPath := filepath.Join(dir, "installed-app")
	if err := os.WriteFile(installedPath, []byte("pretend this is v1.0.0"), 0o755); err != nil {
		return err
	}

	svc, err := updates.NewService(
		updates.WithCurrentVersion("v1.0.0"),
		updates.WithGitHubRepo("exampleorg", "myapp"),
		updates.WithGitHubAPIURL(srv.URL), // point at the local stand-in
		updates.WithAssetPattern(assetName),
		updates.WithBinaryName(assetName),
		updates.WithAppName("updates-example"),
		updates.WithEmitter(emitter),
		updates.WithPublicKey(pubFileText),
		updates.WithApplier(installToPath{path: installedPath}),
	)
	if err != nil {
		return fmt.Errorf("new service: %w", err)
	}

	ctx := context.Background()

	fmt.Println("checking for updates...")
	rel, err := svc.CheckForUpdate(ctx)
	if err != nil {
		return fmt.Errorf("check for update: %w", err)
	}
	if rel == nil {
		fmt.Println("already up to date")
		return nil
	}
	fmt.Printf("update available: %s\n%s\n", rel.Version, rel.Body)

	fmt.Println("downloading and verifying signature...")
	if _, err := svc.DownloadUpdate(ctx); err != nil {
		return fmt.Errorf("download update: %w", err)
	}
	fmt.Println("download verified")

	fmt.Println("applying update (re-verifies the signature immediately before extraction)...")
	if err := svc.ApplyUpdate(ctx); err != nil {
		return fmt.Errorf("apply update: %w", err)
	}

	installed, err := os.ReadFile(installedPath)
	if err != nil {
		return err
	}
	fmt.Printf("installed binary now reads: %q\n", installed)
	fmt.Printf("events emitted: %d\n", mem.Count())
	for _, e := range mem.Events() {
		fmt.Printf("  - %s\n", e.Name)
	}

	return nil
}

// installToPath is a minimal updates.Applier that copies the verified new
// binary to path instead of replacing the current process's own
// executable — see the package doc comment.
type installToPath struct{ path string }

func (a installToPath) Apply(newPath, _ string) error {
	data, err := os.ReadFile(newPath)
	if err != nil {
		return err
	}
	return os.WriteFile(a.path, data, 0o755)
}

// newFakeGitHubServer stands in for the subset of the GitHub Releases API
// that updates.Service calls: /releases/latest and asset downloads.
func newFakeGitHubServer(assetName string, assetContent []byte, sigText string) *httptest.Server {
	mux := http.NewServeMux()

	// srv.URL doesn't exist until httptest.NewServer returns, but asset
	// URLs in the JSON response need to be absolute (real GitHub responses
	// are). The handler closure only reads srv.URL when handling a
	// request, which happens strictly after the assignment below.
	var srv *httptest.Server

	mux.HandleFunc("/repos/exampleorg/myapp/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v2.0.0",
			"body":     "Example release notes.",
			"html_url": "https://github.com/exampleorg/myapp/releases/tag/v2.0.0",
			"assets": []map[string]any{
				{"name": assetName, "size": len(assetContent), "browser_download_url": srv.URL + "/dl/asset"},
				{"name": assetName + ".minisig", "size": len(sigText), "browser_download_url": srv.URL + "/dl/sig"},
			},
		})
	})
	mux.HandleFunc("/dl/asset", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(assetContent) })
	mux.HandleFunc("/dl/sig", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(sigText)) })

	srv = httptest.NewServer(mux)
	return srv
}

// generateDemoKeypair generates an ephemeral Ed25519 key pair wrapped as a
// minisign key, using the legacy (non-prehashed) signature algorithm so
// this example needs no dependency beyond crypto/ed25519. A real release
// pipeline uses the actual "minisign" CLI (see README) and never generates
// keys at runtime.
func generateDemoKeypair() (pub minisign.PublicKey, priv ed25519.PrivateKey, pubFileText string) {
	rawPub, rawPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatalf("generate key: %v", err)
	}
	var keyID [8]byte
	if _, err := rand.Read(keyID[:]); err != nil {
		log.Fatalf("generate key id: %v", err)
	}

	blob := make([]byte, 0, 42)
	blob = append(blob, 'E', 'd')
	blob = append(blob, keyID[:]...)
	blob = append(blob, rawPub...)
	pubFileText = "untrusted comment: minisign public key DEMO\n" + base64.StdEncoding.EncodeToString(blob) + "\n"

	pub, err = minisign.DecodePublicKey(pubFileText)
	if err != nil {
		log.Fatalf("decode generated public key: %v", err)
	}
	return pub, rawPriv, pubFileText
}

// signDemoAsset signs message under pub's key ID and returns the resulting
// .minisig file text (legacy, non-prehashed "Ed" algorithm — see
// generateDemoKeypair).
func signDemoAsset(priv ed25519.PrivateKey, pub minisign.PublicKey, message []byte, trustedComment string) string {
	sig := ed25519.Sign(priv, message)

	sigBlob := make([]byte, 0, 74)
	sigBlob = append(sigBlob, 'E', 'd')
	sigBlob = append(sigBlob, pub.KeyId[:]...)
	sigBlob = append(sigBlob, sig...)

	globalMsg := make([]byte, 0, len(sig)+len(trustedComment))
	globalMsg = append(globalMsg, sig...)
	globalMsg = append(globalMsg, []byte(trustedComment)...)
	globalSig := ed25519.Sign(priv, globalMsg)

	return "untrusted comment: signature from minisign secret key\n" +
		base64.StdEncoding.EncodeToString(sigBlob) + "\n" +
		"trusted comment: " + trustedComment + "\n" +
		base64.StdEncoding.EncodeToString(globalSig) + "\n"
}
