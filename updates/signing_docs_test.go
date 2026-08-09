package updates

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	minisign "github.com/jedisct1/go-minisign"
)

// TestSigningDocsFlow is the end-to-end proof for the README's signing
// recipe: it runs the exact commands the README documents against the real
// minisign CLI (not a Go re-implementation), then verifies the result with
// the same go-minisign library the running service uses. This is the test
// the security review asked for: the previous README recipe (openssl +
// "$(cat binary)") could never produce a signature verify.go would accept,
// and that gap is exactly why the package moved to minisign. This test
// fails if the documented commands ever stop producing a signature this
// package's verifier accepts.
//
// It shells out to the "minisign" binary and is skipped when that binary
// isn't on PATH — it doesn't hit the network, so it's safe to run, but a
// missing external tool shouldn't fail unrelated CI runs. Install it with
// "brew install minisign" (macOS) or see https://jedisct1.github.io/minisign/
// for other platforms.
func TestSigningDocsFlow(t *testing.T) {
	minisignBin, err := exec.LookPath("minisign")
	if err != nil {
		t.Skip("minisign CLI not found on PATH; skipping end-to-end signing-recipe test (install: brew install minisign)")
	}

	dir := t.TempDir()
	pubPath := filepath.Join(dir, "minisign.pub")
	secPath := filepath.Join(dir, "minisign.key")
	assetPath := filepath.Join(dir, "myapp_darwin_arm64.tar.gz")
	sigPath := assetPath + ".minisig"

	// Step 1 of the README recipe: generate a keypair. -W skips the
	// password prompt — fine for this test, but production keys should be
	// password-protected (see README "Generating a keypair").
	run(t, minisignBin, "-G", "-W", "-f", "-p", pubPath, "-s", secPath)

	if err := os.WriteFile(assetPath, []byte("fake release asset content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Step 2 of the README recipe: sign the release asset.
	run(t, minisignBin, "-S", "-s", secPath, "-m", assetPath, "-t", "myapp v1.2.3")

	// Step 3 of the README recipe (what a user/CI would run to sanity
	// check the signature): verify with the CLI itself.
	run(t, minisignBin, "-V", "-p", pubPath, "-m", assetPath)

	// The actual assertion that matters for this package: the exact bytes
	// the documented recipe produces must verify through go-minisign, the
	// library verify.go actually uses at runtime.
	pubText, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := parseMinisignPublicKey(string(pubText))
	if err != nil {
		t.Fatalf("parseMinisignPublicKey rejected the CLI-generated public key: %v", err)
	}

	if err := verifySignature(pub, assetPath, sigPath); err != nil {
		t.Fatalf("verifySignature rejected a signature produced by the documented recipe: %v", err)
	}

	// Negative control: a tampered asset must still be rejected.
	if err := os.WriteFile(assetPath, []byte("tampered content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifySignature(pub, assetPath, sigPath); err == nil {
		t.Fatal("expected verifySignature to reject a tampered asset")
	}

	// Sanity: go-minisign's own entry point agrees.
	sig, err := minisign.NewSignatureFromFile(sigPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pub.VerifyFromFile(assetPath, sig); err == nil {
		t.Fatal("expected go-minisign to also reject the tampered asset")
	}
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}
