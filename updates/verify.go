package updates

import (
	"crypto/ed25519"
	"fmt"
	"os"
)

// verifySignature reads the asset file at assetPath, reads the detached
// Ed25519 signature from sigPath, and verifies the signature against the
// given public key. Returns nil on success.
func verifySignature(publicKey ed25519.PublicKey, assetPath, sigPath string) error {
	message, err := os.ReadFile(assetPath)
	if err != nil {
		return fmt.Errorf("read asset for verification: %w", err)
	}

	sig, err := os.ReadFile(sigPath)
	if err != nil {
		return fmt.Errorf("read signature file: %w", err)
	}

	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature size: got %d bytes, want %d", len(sig), ed25519.SignatureSize)
	}

	if !ed25519.Verify(publicKey, message, sig) {
		return fmt.Errorf("signature verification failed: binary may have been tampered with")
	}

	return nil
}
