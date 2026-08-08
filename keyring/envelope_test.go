package keyring

// These tests must never touch the real OS keyring or the network. Every case
// backs the envelope store with NewMemoryStore() and a t.TempDir() config
// directory. Do not substitute NewOSStore here, even "just to check" — a real
// Keychain entry has been created by accident in this project before.

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
	kiterrors "github.com/jrschumacher/wails-kit/v2/errors"
)

// newTestEnvelope returns a store backed by an in-memory keyring and a temp
// config dir, plus the keyring and the config dir for direct inspection.
func newTestEnvelope(t *testing.T) (*EnvelopeStore, *MemoryStore, string) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "config")
	kr := NewMemoryStore()
	s := newTestEnvelopeWith(t, cfg, kr)
	return s, kr, cfg
}

// newTestEnvelopeWith builds a store over an explicit config dir and keyring,
// so a test can construct a second, cold-cache store over the same state.
func newTestEnvelopeWith(t *testing.T, cfg string, kr Store) *EnvelopeStore {
	t.Helper()
	dirs := appdirs.New("wails-kit-envelope-test", appdirs.WithConfigDir(cfg))
	s, err := NewEnvelopeStore(dirs, kr)
	if err != nil {
		t.Fatalf("NewEnvelopeStore: %v", err)
	}
	return s
}

func mustSet(t *testing.T, s *EnvelopeStore, k, v string) {
	t.Helper()
	if err := s.Set(k, v); err != nil {
		t.Fatalf("Set(%q): %v", k, err)
	}
}

func mustGet(t *testing.T, s *EnvelopeStore, k string) string {
	t.Helper()
	v, err := s.Get(k)
	if err != nil {
		t.Fatalf("Get(%q): %v", k, err)
	}
	return v
}

// readRaw returns the parsed on-disk file.
func readRaw(t *testing.T, s *EnvelopeStore) envelopeFile {
	t.Helper()
	data, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatalf("read secrets file: %v", err)
	}
	var f envelopeFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("parse secrets file: %v", err)
	}
	return f
}

func writeRaw(t *testing.T, s *EnvelopeStore, f envelopeFile) {
	t.Helper()
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(s.Path(), data, 0o600); err != nil {
		t.Fatalf("write secrets file: %v", err)
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	s, _, _ := newTestEnvelope(t)

	if _, err := s.Get("absent"); err != ErrNotFound {
		t.Fatalf("Get on empty store = %v, want ErrNotFound", err)
	}
	if s.Has("absent") {
		t.Fatal("Has on empty store = true")
	}

	mustSet(t, s, "llm.anthropic.secret", "sk-ant-test-value")
	mustSet(t, s, "llm.openai.secret", "sk-openai-test-value")

	if got := mustGet(t, s, "llm.anthropic.secret"); got != "sk-ant-test-value" {
		t.Fatalf("Get = %q", got)
	}
	if !s.Has("llm.openai.secret") {
		t.Fatal("Has = false for a key that was set")
	}

	// Overwrite.
	mustSet(t, s, "llm.anthropic.secret", "sk-ant-rotated")
	if got := mustGet(t, s, "llm.anthropic.secret"); got != "sk-ant-rotated" {
		t.Fatalf("Get after overwrite = %q", got)
	}

	keys, err := s.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	want := []string{"llm.anthropic.secret", "llm.openai.secret"}
	if fmt.Sprint(keys) != fmt.Sprint(want) {
		t.Fatalf("Keys = %v, want %v", keys, want)
	}

	if err := s.Delete("llm.openai.secret"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if s.Has("llm.openai.secret") {
		t.Fatal("Has = true after Delete")
	}
	// Deleting an absent key is not an error.
	if err := s.Delete("llm.openai.secret"); err != nil {
		t.Fatalf("Delete absent: %v", err)
	}

	// Empty values round-trip.
	mustSet(t, s, "empty", "")
	if got := mustGet(t, s, "empty"); got != "" {
		t.Fatalf("empty value = %q", got)
	}

	// A cold store over the same dir and keyring sees the same data.
	s2 := newTestEnvelopeWith(t, filepath.Dir(s.Path()), s.kr)
	if got := mustGet(t, s2, "llm.anthropic.secret"); got != "sk-ant-rotated" {
		t.Fatalf("cold store Get = %q", got)
	}
}

// TestEnvelopeLargeSecret is the entire justification for this package:
// Windows Credential Manager caps CredentialBlob at 2560 bytes, so a secret
// larger than that cannot live in the OS keyring directly.
func TestEnvelopeLargeSecret(t *testing.T) {
	s, kr, _ := newTestEnvelope(t)

	const winCredentialBlobLimit = 2560

	sizes := []int{
		winCredentialBlobLimit + 1, // one byte past the Windows limit
		16 * 1024,                  // a service-account JSON / cert bundle
		1 << 20,                    // 1 MiB, well past any keyring
	}

	for _, n := range sizes {
		t.Run(fmt.Sprintf("%dB", n), func(t *testing.T) {
			raw := make([]byte, n)
			if _, err := rand.Read(raw); err != nil {
				t.Fatalf("rand: %v", err)
			}
			// Base64 so the value is a valid string, as Store requires,
			// and still larger than the limit.
			secret := base64.StdEncoding.EncodeToString(raw)
			if len(secret) <= winCredentialBlobLimit {
				t.Fatalf("test secret is %d bytes, not past the %d-byte limit", len(secret), winCredentialBlobLimit)
			}

			key := fmt.Sprintf("big.%d", n)
			mustSet(t, s, key, secret)
			if got := mustGet(t, s, key); got != secret {
				t.Fatalf("large secret did not round-trip (got %d bytes, want %d)", len(got), len(secret))
			}
		})
	}

	// The keyring itself only ever holds the 32-byte wrapping key, base64'd.
	raw, err := kr.Get(DefaultEnvelopeKeyName)
	if err != nil {
		t.Fatalf("wrapping key: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("wrapping key not base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("wrapping key is %d bytes, want 32", len(decoded))
	}
	if len(raw) > winCredentialBlobLimit {
		t.Fatalf("keyring payload is %d bytes, must stay under %d", len(raw), winCredentialBlobLimit)
	}
}

func TestEnvelopeNoPlaintextOnDisk(t *testing.T) {
	s, _, _ := newTestEnvelope(t)
	const marker = "UNIQUE-PLAINTEXT-MARKER-4f3a9c"
	mustSet(t, s, "secret", marker)

	data, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if bytes.Contains(data, []byte(marker)) {
		t.Fatal("plaintext marker found in secrets file")
	}
	// The key name is not secret and is intentionally in the clear (it is the
	// AEAD associated data), so assert that rather than accidentally relying
	// on it.
	if !bytes.Contains(data, []byte(`"secret"`)) {
		t.Fatal("expected entry key name to be present in the clear")
	}
}

func TestEnvelopeOnDiskFormat(t *testing.T) {
	s, _, _ := newTestEnvelope(t)
	mustSet(t, s, "a", "value-a")

	f := readRaw(t, s)
	if f.Version != 1 {
		t.Fatalf("version = %d, want 1", f.Version)
	}
	if f.Alg != "xchacha20poly1305" {
		t.Fatalf("alg = %q", f.Alg)
	}
	e, ok := f.Entries["a"]
	if !ok {
		t.Fatal("entry a missing")
	}
	nonce, err := base64.StdEncoding.DecodeString(e.Nonce)
	if err != nil {
		t.Fatalf("nonce not base64: %v", err)
	}
	if len(nonce) != 24 {
		t.Fatalf("nonce is %d bytes, want 24 (XChaCha20 extended nonce)", len(nonce))
	}
	if _, err := base64.StdEncoding.DecodeString(e.CT); err != nil {
		t.Fatalf("ct not base64: %v", err)
	}

	// Every write uses a fresh nonce, and writing one entry must not disturb
	// another entry's ciphertext.
	mustSet(t, s, "b", "value-b")
	f2 := readRaw(t, s)
	if f2.Entries["a"] != e {
		t.Fatal("writing entry b rewrote entry a")
	}

	mustSet(t, s, "a", "value-a")
	f3 := readRaw(t, s)
	if f3.Entries["a"].Nonce == e.Nonce {
		t.Fatal("nonce reused across writes of the same key")
	}
}

func TestEnvelopePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}
	s, _, cfg := newTestEnvelope(t)
	mustSet(t, s, "a", "value")

	fi, err := os.Stat(s.Path())
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("secrets file mode = %#o, want 0600", perm)
	}

	di, err := os.Stat(cfg)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Fatalf("config dir mode = %#o, want 0700", perm)
	}

	// No temp files left behind after a successful write.
	ents, err := os.ReadDir(cfg)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestEnvelopeUsesConfigDirNotDataDir(t *testing.T) {
	base := t.TempDir()
	cfg := filepath.Join(base, "config")
	data := filepath.Join(base, "data")
	dirs := appdirs.New("wails-kit-envelope-test",
		appdirs.WithConfigDir(cfg),
		appdirs.WithDataDir(data),
	)
	s, err := NewEnvelopeStore(dirs, NewMemoryStore())
	if err != nil {
		t.Fatalf("NewEnvelopeStore: %v", err)
	}
	mustSet(t, s, "a", "value")

	if filepath.Dir(s.Path()) != cfg {
		t.Fatalf("secrets file is at %s, want it under the config dir %s", s.Path(), cfg)
	}
	if _, err := os.Stat(filepath.Join(data, DefaultEnvelopeFileName)); err == nil {
		t.Fatal("secrets file was written under the data dir")
	}
}

func TestEnvelopeWrongKey(t *testing.T) {
	s, kr, cfg := newTestEnvelope(t)
	mustSet(t, s, "a", "value-a")
	before := readRaw(t, s)

	// Replace the wrapping key with a different, valid 32-byte key.
	other := make([]byte, 32)
	if _, err := rand.Read(other); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if err := kr.Set(DefaultEnvelopeKeyName, base64.StdEncoding.EncodeToString(other)); err != nil {
		t.Fatalf("kr.Set: %v", err)
	}

	cold := newTestEnvelopeWith(t, cfg, kr)
	_, err := cold.Get("a")
	if err == nil {
		t.Fatal("Get with the wrong wrapping key returned no error")
	}
	if !kiterrors.IsCode(err, ErrEnvelopeDecrypt) {
		t.Fatalf("code = %q, want %q (%v)", kiterrors.GetCode(err), ErrEnvelopeDecrypt, err)
	}

	// The data must survive: recovery is possible if the real key comes back.
	if after := readRaw(t, cold); after.Entries["a"] != before.Entries["a"] {
		t.Fatal("a failed decrypt mutated the secrets file")
	}
}

func TestEnvelopeInvalidKeyMaterial(t *testing.T) {
	cases := map[string]string{
		"not base64": "!!!not-base64!!!",
		"too short":  base64.StdEncoding.EncodeToString(make([]byte, 16)),
		"too long":   base64.StdEncoding.EncodeToString(make([]byte, 64)),
	}
	for name, val := range cases {
		t.Run(name, func(t *testing.T) {
			s, kr, cfg := newTestEnvelope(t)
			mustSet(t, s, "a", "value-a")
			if err := kr.Set(DefaultEnvelopeKeyName, val); err != nil {
				t.Fatalf("kr.Set: %v", err)
			}
			cold := newTestEnvelopeWith(t, cfg, kr)
			_, err := cold.Get("a")
			if !kiterrors.IsCode(err, ErrEnvelopeKeyInvalid) {
				t.Fatalf("code = %q, want %q (%v)", kiterrors.GetCode(err), ErrEnvelopeKeyInvalid, err)
			}
		})
	}
}

func TestEnvelopeKeyMissingButFilePresent(t *testing.T) {
	s, kr, cfg := newTestEnvelope(t)
	mustSet(t, s, "a", "value-a")
	before, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if err := kr.Delete(DefaultEnvelopeKeyName); err != nil {
		t.Fatalf("kr.Delete: %v", err)
	}
	cold := newTestEnvelopeWith(t, cfg, kr)

	// Reads must fail loudly rather than report "not found" or an empty value.
	if _, err := cold.Get("a"); !kiterrors.IsCode(err, ErrEnvelopeKeyMissing) {
		t.Fatalf("Get code = %q, want %q (%v)", kiterrors.GetCode(err), ErrEnvelopeKeyMissing, err)
	}
	// Writes must not silently mint a fresh key, which would orphan every
	// existing entry.
	if err := cold.Set("b", "value-b"); !kiterrors.IsCode(err, ErrEnvelopeKeyMissing) {
		t.Fatalf("Set code = %q, want %q (%v)", kiterrors.GetCode(err), ErrEnvelopeKeyMissing, err)
	}
	if err := cold.Rotate(); !kiterrors.IsCode(err, ErrEnvelopeKeyMissing) {
		t.Fatalf("Rotate code = %q, want %q (%v)", kiterrors.GetCode(err), ErrEnvelopeKeyMissing, err)
	}

	after, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("secrets file was modified after the wrapping key went missing")
	}
	if kr.Has(DefaultEnvelopeKeyName) {
		t.Fatal("a replacement wrapping key was generated over existing data")
	}

	// Delete does not need the key: it discards ciphertext rather than
	// decrypting it, so entries stay removable.
	if err := cold.Delete("a"); err != nil {
		t.Fatalf("Delete without a wrapping key: %v", err)
	}
	if cold.Has("a") {
		t.Fatal("entry survived Delete")
	}
}

func TestEnvelopeCorruptFile(t *testing.T) {
	t.Run("not json", func(t *testing.T) {
		s, _, _ := newTestEnvelope(t)
		mustSet(t, s, "a", "value-a")
		if err := os.WriteFile(s.Path(), []byte("{not json"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := s.Get("a"); !kiterrors.IsCode(err, ErrEnvelopeCorrupt) {
			t.Fatalf("code = %q, want %q (%v)", kiterrors.GetCode(err), ErrEnvelopeCorrupt, err)
		}
		// A corrupt file must not be silently replaced by a write, either.
		if err := s.Set("b", "value-b"); !kiterrors.IsCode(err, ErrEnvelopeCorrupt) {
			t.Fatalf("Set code = %q, want %q (%v)", kiterrors.GetCode(err), ErrEnvelopeCorrupt, err)
		}
	})

	t.Run("truncated", func(t *testing.T) {
		s, _, _ := newTestEnvelope(t)
		mustSet(t, s, "a", "value-a")
		data, err := os.ReadFile(s.Path())
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if err := os.WriteFile(s.Path(), data[:len(data)/2], 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := s.Get("a"); !kiterrors.IsCode(err, ErrEnvelopeCorrupt) {
			t.Fatalf("code = %q, want %q (%v)", kiterrors.GetCode(err), ErrEnvelopeCorrupt, err)
		}
	})

	t.Run("unsupported version", func(t *testing.T) {
		s, _, _ := newTestEnvelope(t)
		mustSet(t, s, "a", "value-a")
		f := readRaw(t, s)
		f.Version = 99
		writeRaw(t, s, f)
		if _, err := s.Get("a"); !kiterrors.IsCode(err, ErrEnvelopeCorrupt) {
			t.Fatalf("code = %q, want %q (%v)", kiterrors.GetCode(err), ErrEnvelopeCorrupt, err)
		}
	})

	t.Run("unsupported alg", func(t *testing.T) {
		s, _, _ := newTestEnvelope(t)
		mustSet(t, s, "a", "value-a")
		f := readRaw(t, s)
		f.Alg = "aes-256-gcm"
		writeRaw(t, s, f)
		if _, err := s.Get("a"); !kiterrors.IsCode(err, ErrEnvelopeCorrupt) {
			t.Fatalf("code = %q, want %q (%v)", kiterrors.GetCode(err), ErrEnvelopeCorrupt, err)
		}
	})

	t.Run("bad entry encoding", func(t *testing.T) {
		s, _, _ := newTestEnvelope(t)
		mustSet(t, s, "a", "value-a")
		mustSet(t, s, "b", "value-b")
		f := readRaw(t, s)
		e := f.Entries["a"]
		e.CT = "!!!not-base64!!!"
		f.Entries["a"] = e
		writeRaw(t, s, f)

		// The damaged entry surfaces as an error...
		if _, err := s.Get("a"); !kiterrors.IsCode(err, ErrEnvelopeCorrupt) {
			t.Fatalf("code = %q, want %q (%v)", kiterrors.GetCode(err), ErrEnvelopeCorrupt, err)
		}
		// ...and is not skipped during rotation, which would drop it forever.
		if err := s.Rotate(); err == nil {
			t.Fatal("Rotate silently proceeded past a corrupt entry")
		}
		// The healthy sibling is untouched by the failed rotation.
		if got := mustGet(t, s, "b"); got != "value-b" {
			t.Fatalf("sibling entry = %q", got)
		}
	})

	t.Run("tampered ciphertext", func(t *testing.T) {
		s, _, _ := newTestEnvelope(t)
		mustSet(t, s, "a", "value-a")
		f := readRaw(t, s)
		ct, err := base64.StdEncoding.DecodeString(f.Entries["a"].CT)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		ct[0] ^= 0xff
		f.Entries["a"] = envelopeEntry{Nonce: f.Entries["a"].Nonce, CT: base64.StdEncoding.EncodeToString(ct)}
		writeRaw(t, s, f)

		if _, err := s.Get("a"); !kiterrors.IsCode(err, ErrEnvelopeDecrypt) {
			t.Fatalf("code = %q, want %q (%v)", kiterrors.GetCode(err), ErrEnvelopeDecrypt, err)
		}
	})

	t.Run("bad nonce length", func(t *testing.T) {
		s, _, _ := newTestEnvelope(t)
		mustSet(t, s, "a", "value-a")
		f := readRaw(t, s)
		f.Entries["a"] = envelopeEntry{
			Nonce: base64.StdEncoding.EncodeToString(make([]byte, 12)), // GCM-sized
			CT:    f.Entries["a"].CT,
		}
		writeRaw(t, s, f)
		if _, err := s.Get("a"); !kiterrors.IsCode(err, ErrEnvelopeCorrupt) {
			t.Fatalf("code = %q, want %q (%v)", kiterrors.GetCode(err), ErrEnvelopeCorrupt, err)
		}
	})
}

// TestEnvelopeEntryRelocation proves the key name is bound as AEAD associated
// data: a ciphertext copied under a different entry name must not open.
func TestEnvelopeEntryRelocation(t *testing.T) {
	s, _, _ := newTestEnvelope(t)
	mustSet(t, s, "admin.token", "privileged-value")
	mustSet(t, s, "guest.token", "unprivileged-value")

	f := readRaw(t, s)
	f.Entries["guest.token"] = f.Entries["admin.token"]
	writeRaw(t, s, f)

	if _, err := s.Get("guest.token"); !kiterrors.IsCode(err, ErrEnvelopeDecrypt) {
		t.Fatalf("relocated entry opened, or wrong code %q (%v)", kiterrors.GetCode(err), err)
	}
}

func TestEnvelopeRotate(t *testing.T) {
	s, kr, cfg := newTestEnvelope(t)

	want := map[string]string{
		"a":     "value-a",
		"b":     strings.Repeat("x", 5000), // past the Windows blob limit
		"c":     "",
		"d.e.f": "nested.key.name",
	}
	for k, v := range want {
		mustSet(t, s, k, v)
	}

	beforeKey, err := kr.Get(DefaultEnvelopeKeyName)
	if err != nil {
		t.Fatalf("kr.Get: %v", err)
	}
	beforeFile := readRaw(t, s)

	if err := s.Rotate(); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	afterKey, err := kr.Get(DefaultEnvelopeKeyName)
	if err != nil {
		t.Fatalf("kr.Get: %v", err)
	}
	if afterKey == beforeKey {
		t.Fatal("wrapping key unchanged after Rotate")
	}
	decoded, err := base64.StdEncoding.DecodeString(afterKey)
	if err != nil || len(decoded) != 32 {
		t.Fatalf("rotated key is not 32 base64 bytes: %v (%d)", err, len(decoded))
	}

	afterFile := readRaw(t, s)
	if len(afterFile.Entries) != len(beforeFile.Entries) {
		t.Fatalf("entry count changed: %d -> %d", len(beforeFile.Entries), len(afterFile.Entries))
	}
	for k, be := range beforeFile.Entries {
		ae, ok := afterFile.Entries[k]
		if !ok {
			t.Fatalf("entry %q lost during rotation", k)
		}
		if ae.CT == be.CT {
			t.Fatalf("entry %q ciphertext unchanged after rotation", k)
		}
		if ae.Nonce == be.Nonce {
			t.Fatalf("entry %q nonce reused after rotation", k)
		}
	}

	// Readable through the hot store...
	for k, v := range want {
		if got := mustGet(t, s, k); got != v {
			t.Fatalf("after rotate Get(%q) = %q (%d bytes), want %d bytes", k, got, len(got), len(v))
		}
	}
	// ...and through a cold one that loads the new key from the keyring.
	cold := newTestEnvelopeWith(t, cfg, kr)
	for k, v := range want {
		if got := mustGet(t, cold, k); got != v {
			t.Fatalf("cold after rotate Get(%q) mismatch", k)
		}
	}

	// Writes keep working after rotation.
	mustSet(t, s, "post", "post-rotate")
	if got := mustGet(t, newTestEnvelopeWith(t, cfg, kr), "post"); got != "post-rotate" {
		t.Fatalf("post-rotate write = %q", got)
	}

	// No temp files left behind.
	ents, err := os.ReadDir(cfg)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind after rotation: %s", e.Name())
		}
	}
}

func TestEnvelopeRotateEmptyStore(t *testing.T) {
	s, kr, _ := newTestEnvelope(t)
	if err := s.Rotate(); err != nil {
		t.Fatalf("Rotate on an empty store: %v", err)
	}
	if !kr.Has(DefaultEnvelopeKeyName) {
		t.Fatal("Rotate did not establish a wrapping key")
	}
	mustSet(t, s, "a", "value-a")
	if got := mustGet(t, s, "a"); got != "value-a" {
		t.Fatalf("Get = %q", got)
	}
}

// TestEnvelopeConcurrent must be run under -race.
func TestEnvelopeConcurrent(t *testing.T) {
	s, _, _ := newTestEnvelope(t)

	// Seed a key so readers have something real to decrypt, and so the
	// wrapping key exists before the goroutines race for it.
	mustSet(t, s, "seed", "seed-value")

	// Kept modest on purpose: every write rewrites and fsyncs the whole file,
	// so total work is quadratic in the entry count. Large values are covered
	// by TestEnvelopeLargeSecret; this test is about races, not throughput.
	const workers = 8
	const iterations = 10

	var wg sync.WaitGroup
	errCh := make(chan error, workers*iterations*4)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := fmt.Sprintf("w%d.k%d", w, i)
				val := fmt.Sprintf("value-%d-%d-%s", w, i, strings.Repeat("p", 300))

				if err := s.Set(key, val); err != nil {
					errCh <- fmt.Errorf("Set(%q): %w", key, err)
					continue
				}
				got, err := s.Get(key)
				if err != nil {
					errCh <- fmt.Errorf("Get(%q): %w", key, err)
					continue
				}
				if got != val {
					errCh <- fmt.Errorf("Get(%q) mismatch", key)
				}
				if v, err := s.Get("seed"); err != nil || v != "seed-value" {
					errCh <- fmt.Errorf("seed read: %v / %q", err, v)
				}
				if _, err := s.Keys(); err != nil {
					errCh <- fmt.Errorf("Keys: %w", err)
				}
				s.Has(key)
			}
		}(w)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
	if t.Failed() {
		return
	}

	// Read-modify-write under the lock means no write is lost.
	keys, err := s.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if want := workers*iterations + 1; len(keys) != want {
		t.Fatalf("final key count = %d, want %d (a concurrent write was lost)", len(keys), want)
	}
}

// TestEnvelopeConcurrentRotate exercises rotation racing ordinary traffic.
func TestEnvelopeConcurrentRotate(t *testing.T) {
	s, _, _ := newTestEnvelope(t)
	for i := 0; i < 10; i++ {
		mustSet(t, s, fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 128)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			if err := s.Rotate(); err != nil {
				errCh <- fmt.Errorf("Rotate: %w", err)
			}
		}
	}()

	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				k := fmt.Sprintf("k%d", i%10)
				v, err := s.Get(k)
				if err != nil {
					errCh <- fmt.Errorf("Get(%q): %w", k, err)
					continue
				}
				if v != fmt.Sprintf("v%d", i%10) {
					errCh <- fmt.Errorf("Get(%q) = %q", k, v)
				}
				if err := s.Set(fmt.Sprintf("w%d.%d", w, i), "x"); err != nil {
					errCh <- fmt.Errorf("Set: %w", err)
				}
			}
		}(w)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func TestNewEnvelopeStoreValidation(t *testing.T) {
	dirs := appdirs.New("wails-kit-envelope-test", appdirs.WithConfigDir(t.TempDir()))

	if _, err := NewEnvelopeStore(nil, NewMemoryStore()); err == nil {
		t.Fatal("nil appdirs accepted")
	}
	if _, err := NewEnvelopeStore(dirs, nil); err == nil {
		t.Fatal("nil keyring accepted")
	}
	if _, err := NewEnvelopeStore(dirs, NewMemoryStore(), WithEnvelopeFileName("")); err == nil {
		t.Fatal("empty file name accepted")
	}
	if _, err := NewEnvelopeStore(dirs, NewMemoryStore(), WithEnvelopeFileName(filepath.Join("sub", "s.json"))); err == nil {
		t.Fatal("file name with a path separator accepted")
	}
	if _, err := NewEnvelopeStore(dirs, NewMemoryStore(), WithEnvelopeKeyName("")); err == nil {
		t.Fatal("empty key name accepted")
	}

	s, err := NewEnvelopeStore(dirs, NewMemoryStore(),
		WithEnvelopeFileName("custom.json"),
		WithEnvelopeKeyName("custom.key"),
	)
	if err != nil {
		t.Fatalf("NewEnvelopeStore: %v", err)
	}
	if filepath.Base(s.Path()) != "custom.json" {
		t.Fatalf("path = %s", s.Path())
	}

	if err := s.Set("", "v"); err == nil {
		t.Fatal("empty entry key accepted")
	}
}

// TestEnvelopeIsStore is a compile-and-behavior check that EnvelopeStore drops
// in wherever a keyring Store is expected, including the JSON helpers.
func TestEnvelopeIsStore(t *testing.T) {
	s, _, _ := newTestEnvelope(t)
	var store Store = s

	type creds struct {
		Token string   `json:"token"`
		Blob  string   `json:"blob"`
		Scope []string `json:"scope"`
	}
	in := creds{Token: "t", Blob: strings.Repeat("z", 4000), Scope: []string{"a", "b"}}
	if err := SetJSON(store, "oauth", in); err != nil {
		t.Fatalf("SetJSON: %v", err)
	}
	var out creds
	if err := GetJSON(store, "oauth", &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if out.Token != in.Token || out.Blob != in.Blob || len(out.Scope) != 2 {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

// TestEnvelopeKeyListerTypeAssertion exercises the documented extension
// pattern: a caller holding a plain Store type-asserts to KeyLister to get
// enumeration, and MemoryStore (which does not implement it) is correctly
// rejected by the same assertion.
func TestEnvelopeKeyListerTypeAssertion(t *testing.T) {
	s, _, _ := newTestEnvelope(t)
	mustSet(t, s, "a", "1")
	mustSet(t, s, "b", "2")

	var store Store = s
	lister, ok := store.(KeyLister)
	if !ok {
		t.Fatal("EnvelopeStore does not implement KeyLister")
	}
	keys, err := lister.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("Keys() = %v, want [a b]", keys)
	}

	var plain Store = NewMemoryStore()
	if _, ok := plain.(KeyLister); ok {
		t.Fatal("MemoryStore unexpectedly implements KeyLister")
	}
}
