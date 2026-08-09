package updates

import (
	"fmt"
	"strings"

	minisign "github.com/jedisct1/go-minisign"
)

// parseMinisignPublicKey accepts either the full contents of a minisign
// public key file (two lines: an "untrusted comment: ..." line followed by
// the base64-encoded key) or just the bare base64-encoded key on its own.
// Apps typically embed the whole .pub file via go:embed and pass its
// contents directly.
func parseMinisignPublicKey(raw string) (minisign.PublicKey, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return minisign.PublicKey{}, fmt.Errorf("empty public key")
	}
	if strings.Contains(trimmed, "\n") {
		pk, err := minisign.DecodePublicKey(trimmed)
		if err != nil {
			return minisign.PublicKey{}, fmt.Errorf("decode minisign public key file: %w", err)
		}
		return pk, nil
	}
	pk, err := minisign.NewPublicKey(trimmed)
	if err != nil {
		return minisign.PublicKey{}, fmt.Errorf("decode minisign public key: %w", err)
	}
	return pk, nil
}

// verifySignature verifies the minisign detached signature at sigPath
// against the asset at assetPath, using the given public key. It checks
// both the per-file signature and minisign's global signature over the
// trusted comment. Returns nil only when both are valid.
//
// go-minisign's Verify returns (bool, error); a false result with a nil
// error is treated as failure just like a non-nil error — never trust the
// bare bool alone for a security decision (see AGENTS.md).
func verifySignature(publicKey minisign.PublicKey, assetPath, sigPath string) error {
	sig, err := minisign.NewSignatureFromFile(sigPath)
	if err != nil {
		return fmt.Errorf("read signature file: %w", err)
	}

	ok, err := publicKey.VerifyFromFile(assetPath, sig)
	if err != nil {
		return fmt.Errorf("verify signature: %w", err)
	}
	if !ok {
		return fmt.Errorf("signature verification failed: binary may have been tampered with")
	}

	return nil
}
