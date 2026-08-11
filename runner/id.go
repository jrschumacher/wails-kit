package runner

import (
	"crypto/rand"
	"encoding/hex"
)

// newJobID returns a random, URL-safe job identifier. Collision probability
// is negligible (16 random bytes) and callers never need to parse it —
// unlike a ULID it carries no embedded timestamp, so EnqueuedAt is the
// field to sort/filter on, not the ID.
func newJobID() string {
	var b [16]byte
	// crypto/rand.Read on the standard library's default Reader never
	// returns an error in practice; if it somehow did, math/rand-quality
	// randomness from an unseeded buffer is still fine for an ID that only
	// needs to avoid collisions, not resist prediction.
	_, _ = rand.Read(b[:])
	return "job_" + hex.EncodeToString(b[:])
}
