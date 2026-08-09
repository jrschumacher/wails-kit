package updates

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"

	minisign "github.com/jedisct1/go-minisign"
)

// generateMinisignKeys creates an Ed25519 key pair and wraps the public
// half as a minisign.PublicKey using the legacy (non-prehashed) "Ed"
// signature algorithm. Using the legacy algorithm keeps these unit tests
// independent of blake2b and the external minisign binary; the
// interoperability with the real CLI-generated, prehashed ("ED") format is
// covered separately by TestSigningDocsFlow, which shells out to the real
// tool when it's available.
func generateMinisignKeys(t *testing.T) (pub minisign.PublicKey, priv ed25519.PrivateKey, pubFileText string) {
	t.Helper()

	rawPub, rawPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	var keyID [8]byte
	if _, err := rand.Read(keyID[:]); err != nil {
		t.Fatalf("generate key id: %v", err)
	}

	blob := make([]byte, 0, 42)
	blob = append(blob, 'E', 'd')
	blob = append(blob, keyID[:]...)
	blob = append(blob, rawPub...)

	pubFileText = "untrusted comment: minisign public key TEST\n" + base64.StdEncoding.EncodeToString(blob) + "\n"

	pub, err = minisign.DecodePublicKey(pubFileText)
	if err != nil {
		t.Fatalf("decode generated public key: %v", err)
	}
	return pub, rawPriv, pubFileText
}

// signMinisignFile signs message with priv under pub's key ID (legacy,
// non-prehashed "Ed" algorithm) and returns the resulting .minisig file
// text, matching the format minisign.DecodeSignature parses.
func signMinisignFile(priv ed25519.PrivateKey, pub minisign.PublicKey, message []byte, trustedComment string) string {
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
