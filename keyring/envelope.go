package keyring

// Envelope encryption for arbitrarily large secrets.
//
// # Why this exists
//
// OS credential managers impose hard size limits on individual items. Windows
// Credential Manager caps CredentialBlob at 2560 bytes, and libsecret has
// practical limits of its own. Anything larger than a short API key — an OAuth
// token bundle, a service-account JSON, a client certificate and its private
// key — cannot be stored in the OS keyring today. Envelope encryption removes
// the limit: the keyring holds exactly 32 bytes (the wrapping key) and the
// ciphertext lives in a file on disk, so secret size is bounded only by the
// filesystem. It also collapses N keychain unlock prompts into one.
//
// # What this is not
//
// This is NOT a security improvement over storing each secret as its own
// keyring item. Any process running as the user can read the wrapping key out
// of the keyring and decrypt every entry — exactly as it could read each
// keyring item individually today. The threat model is unchanged; only the
// size limit and the number of unlock surfaces change. Do not present this to
// users as "more secure storage".
//
// # Construction
//
// Entries are sealed with XChaCha20-Poly1305 (golang.org/x/crypto). The 24-byte
// extended nonce is the reason for that choice over AES-GCM: random 24-byte
// nonces can be generated indefinitely without meaningful collision risk, while
// GCM's 96-bit nonce space makes random generation unsafe past roughly 2^32
// messages, and nonce reuse in GCM is both silent and catastrophic (it leaks
// the authentication key). Each entry carries its own nonce so writing one
// entry does not re-encrypt the others. The entry's key name is passed as AEAD
// associated data, so a ciphertext cannot be relocated to a different key
// without failing authentication.
//
// # Failure policy
//
// Every failure mode surfaces as an error. If the secrets file exists but the
// wrapping key is missing from the keyring, or the key present does not
// decrypt the entries, the store refuses to open rather than starting fresh.
// A corrupt entry is reported, never skipped. Silently resetting would present
// to the user as "all of my API keys vanished" and would destroy data that is
// otherwise recoverable (from a backup, from another machine's keychain, from
// a Time Machine snapshot).

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	"golang.org/x/crypto/chacha20poly1305"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
	kiterrors "github.com/jrschumacher/wails-kit/v2/errors"
)

// Error codes for the envelope store.
const (
	// ErrEnvelopeKeyMissing means the secrets file exists but no wrapping key
	// was found in the keyring. The data is intact but unreadable.
	ErrEnvelopeKeyMissing kiterrors.Code = "envelope_key_missing"
	// ErrEnvelopeKeyInvalid means the wrapping key in the keyring is not a
	// valid 32-byte base64 value.
	ErrEnvelopeKeyInvalid kiterrors.Code = "envelope_key_invalid"
	// ErrEnvelopeCorrupt means the secrets file could not be parsed or declares
	// a format this build does not understand.
	ErrEnvelopeCorrupt kiterrors.Code = "envelope_corrupt"
	// ErrEnvelopeDecrypt means authenticated decryption failed: the wrapping
	// key is wrong, or the entry has been tampered with or relocated.
	ErrEnvelopeDecrypt kiterrors.Code = "envelope_decrypt"
	// ErrEnvelopeIO means the secrets file could not be read or written.
	ErrEnvelopeIO kiterrors.Code = "envelope_io"
)

func init() {
	kiterrors.RegisterMessages(map[kiterrors.Code]string{
		ErrEnvelopeKeyMissing: "Your saved credentials are locked: the encryption key is missing from the system keychain. Your data has not been deleted. Restore the keychain entry, or reset stored credentials to start over.",
		ErrEnvelopeKeyInvalid: "The credential encryption key in the system keychain is not valid. Your data has not been deleted.",
		ErrEnvelopeCorrupt:    "The stored credentials file is damaged and could not be read. Your data has not been deleted.",
		ErrEnvelopeDecrypt:    "A stored credential could not be decrypted. The encryption key may be from a different installation.",
		ErrEnvelopeIO:         "Failed to read or write stored credentials. Please try again.",
	})
}

// envelopeFormatVersion is the on-disk schema version. It is written from the
// first release so that a future format change can be detected rather than
// mis-parsed.
const envelopeFormatVersion = 1

// envelopeAlg is the algorithm identifier recorded on disk.
const envelopeAlg = "xchacha20poly1305"

// DefaultEnvelopeFileName is the default basename of the secrets file, placed
// in the application config directory.
const DefaultEnvelopeFileName = "secrets.json"

// DefaultEnvelopeKeyName is the default keyring item name holding the 32-byte
// wrapping key (base64-encoded).
const DefaultEnvelopeKeyName = "envelope.wrapping-key.v1"

// wrappingKeySize is the XChaCha20-Poly1305 key size.
const wrappingKeySize = chacha20poly1305.KeySize // 32

// envelopeFile is the on-disk representation.
//
//	{
//	  "v": 1,
//	  "alg": "xchacha20poly1305",
//	  "entries": {
//	    "llm.anthropic.secret": {"nonce": "<base64 24 bytes>", "ct": "<base64>"}
//	  }
//	}
type envelopeFile struct {
	Version int                      `json:"v"`
	Alg     string                   `json:"alg"`
	Entries map[string]envelopeEntry `json:"entries"`
}

// envelopeEntry is one sealed secret. The entry's map key is used as AEAD
// associated data, so ct cannot be moved to a different entry name.
type envelopeEntry struct {
	Nonce string `json:"nonce"` // base64 std, 24 bytes
	CT    string `json:"ct"`    // base64 std, ciphertext||tag
}

// EnvelopeStore stores arbitrarily large secrets in a single encrypted file,
// keyed by a 32-byte wrapping key held in an underlying keyring Store.
//
// It implements Store, so it can be substituted anywhere a keyring Store is
// accepted.
//
// EnvelopeStore is safe for concurrent use. It holds no decrypted entries in
// memory: every operation reads the file, decrypts only what it needs, and
// writes atomically. Only the wrapping key is cached, to avoid an OS keychain
// round trip (and, on some platforms, an unlock prompt) per operation.
//
// Every write serializes and fsyncs the whole file, so a write costs O(total
// bytes stored) regardless of the entry being written. That is the right
// trade for the intended workload — a few dozen credentials, written rarely,
// read at startup — and it is why writes are durable and crash-atomic. It is
// the wrong data structure for thousands of frequently-updated entries.
type EnvelopeStore struct {
	kr      Store
	path    string
	dir     string
	keyName string

	mu   sync.Mutex
	aead cipher.AEAD // cached; nil until the wrapping key is loaded
	key  []byte      // cached copy of the wrapping key, for rotation rollback
}

// compile-time assertion that EnvelopeStore is a drop-in keyring Store.
var _ Store = (*EnvelopeStore)(nil)

// compile-time assertion that EnvelopeStore also implements the optional
// KeyLister extension (see keyring.go).
var _ KeyLister = (*EnvelopeStore)(nil)

// EnvelopeStoreOption configures an EnvelopeStore.
type EnvelopeStoreOption func(*EnvelopeStore)

// WithEnvelopeFileName overrides the basename of the secrets file within the
// application config directory. It may not contain a path separator.
func WithEnvelopeFileName(name string) EnvelopeStoreOption {
	return func(s *EnvelopeStore) { s.path = name }
}

// WithEnvelopeKeyName overrides the keyring item name used for the wrapping
// key. Change this only alongside a deliberate migration: pointing an existing
// secrets file at a different key name makes it undecryptable.
func WithEnvelopeKeyName(name string) EnvelopeStoreOption {
	return func(s *EnvelopeStore) { s.keyName = name }
}

// NewEnvelopeStore creates an envelope-encrypted secret store.
//
// The secrets file is placed in dirs.Config() — deliberately not dirs.Data():
// on Linux these differ, and credentials belong with configuration, which is
// what backup and sync tooling treats as user-owned settings.
//
// kr is the keyring holding the wrapping key; pass NewMemoryStore() in tests so
// that no real OS keychain entry is ever created.
//
// The wrapping key is not touched here. It is generated on first write, or
// loaded on first read, so constructing a store is side-effect free.
func NewEnvelopeStore(dirs *appdirs.Dirs, kr Store, opts ...EnvelopeStoreOption) (*EnvelopeStore, error) {
	if dirs == nil {
		return nil, kiterrors.New(kiterrors.ErrConfigMissing, "keyring: envelope store requires appdirs", nil)
	}
	if kr == nil {
		return nil, kiterrors.New(kiterrors.ErrConfigMissing, "keyring: envelope store requires a backing keyring", nil)
	}

	s := &EnvelopeStore{
		kr:      kr,
		dir:     dirs.Config(),
		path:    DefaultEnvelopeFileName,
		keyName: DefaultEnvelopeKeyName,
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.path == "" || filepath.Base(s.path) != s.path {
		return nil, kiterrors.Newf(kiterrors.ErrConfigInvalid, "keyring: envelope file name %q must be a bare file name", s.path)
	}
	if s.keyName == "" {
		return nil, kiterrors.New(kiterrors.ErrConfigInvalid, "keyring: envelope key name must not be empty", nil)
	}
	s.path = filepath.Join(s.dir, s.path)
	return s, nil
}

// Path returns the absolute path of the secrets file.
func (s *EnvelopeStore) Path() string { return s.path }

// Set encrypts value under key and writes it to the secrets file.
// The wrapping key is generated on the first call if the file does not exist.
func (s *EnvelopeStore) Set(key, value string) error {
	if key == "" {
		return kiterrors.New(kiterrors.ErrValidation, "keyring: envelope key must not be empty", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := s.readFileLocked()
	if err != nil {
		return err
	}
	aead, err := s.loadKeyLocked(len(f.Entries) > 0 || s.fileExists())
	if err != nil {
		return err
	}
	entry, err := seal(aead, key, []byte(value))
	if err != nil {
		return err
	}
	f.Entries[key] = entry
	return s.writeFileLocked(f)
}

// Get decrypts and returns the value stored under key.
// It returns ErrNotFound if the key is absent, and a decryption error — never
// a zero value — if the entry exists but cannot be opened.
func (s *EnvelopeStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := s.readFileLocked()
	if err != nil {
		return "", err
	}
	entry, ok := f.Entries[key]
	if !ok {
		return "", ErrNotFound
	}
	aead, err := s.loadKeyLocked(true)
	if err != nil {
		return "", err
	}
	pt, err := open(aead, key, entry)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// Delete removes the entry for key. Deleting an absent key is not an error.
// Delete does not require the wrapping key: the ciphertext is discarded, not
// decrypted, so entries remain removable even when the key is lost.
func (s *EnvelopeStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := s.readFileLocked()
	if err != nil {
		return err
	}
	if _, ok := f.Entries[key]; !ok {
		return nil
	}
	delete(f.Entries, key)
	return s.writeFileLocked(f)
}

// Has reports whether an entry exists for key. It does not decrypt, so it does
// not touch the keyring and cannot distinguish a healthy entry from a corrupt
// one; a damaged or unreadable file reports false. Use Get when the difference
// matters.
func (s *EnvelopeStore) Has(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := s.readFileLocked()
	if err != nil {
		return false
	}
	_, ok := f.Entries[key]
	return ok
}

// Keys returns the entry names present in the secrets file, sorted. It does
// not decrypt anything. This is the method that satisfies the optional
// KeyLister interface (see keyring.go); Store itself has no Keys method.
func (s *EnvelopeStore) Keys() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := s.readFileLocked()
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(f.Entries))
	for k := range f.Entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

// Rotate generates a fresh wrapping key, re-encrypts every entry under it with
// new nonces, and swaps both the key and the file into place.
//
// Every existing entry must decrypt for rotation to proceed: rotating past an
// entry that cannot be opened would discard it permanently.
//
// The swap is not a single atomic operation — a keyring write and a file rename
// cannot be made atomic with respect to each other. The order is chosen so the
// unrecoverable window is as small as possible: the re-encrypted file is fully
// written and fsynced to a temporary path first, then the new key is stored,
// then the file is renamed. If the rename fails, the previous key is restored
// to the keyring on a best-effort basis so the untouched original file remains
// readable.
func (s *EnvelopeStore) Rotate() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := s.readFileLocked()
	if err != nil {
		return err
	}

	oldAEAD, err := s.loadKeyLocked(len(f.Entries) > 0 || s.fileExists())
	if err != nil {
		return err
	}
	oldKey := s.key

	// Decrypt everything up front so a single bad entry aborts rotation before
	// anything is written.
	plain := make(map[string][]byte, len(f.Entries))
	for k, e := range f.Entries {
		pt, err := open(oldAEAD, k, e)
		if err != nil {
			return kiterrors.Wrap(ErrEnvelopeDecrypt, fmt.Sprintf("keyring: rotate aborted, entry %q does not decrypt", k), err)
		}
		plain[k] = pt
	}

	newKey := make([]byte, wrappingKeySize)
	if _, err := rand.Read(newKey); err != nil {
		return kiterrors.Wrap(kiterrors.ErrInternal, "keyring: generate wrapping key", err)
	}
	newAEAD, err := chacha20poly1305.NewX(newKey)
	if err != nil {
		return kiterrors.Wrap(kiterrors.ErrInternal, "keyring: init cipher", err)
	}

	rotated := newEnvelopeFile()
	for k, pt := range plain {
		entry, err := seal(newAEAD, k, pt)
		if err != nil {
			return err
		}
		rotated.Entries[k] = entry
	}

	tmp, err := s.writeTempLocked(rotated)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp) }() // no-op once renamed

	if err := s.kr.Set(s.keyName, base64.StdEncoding.EncodeToString(newKey)); err != nil {
		return kiterrors.Wrap(ErrEnvelopeIO, "keyring: store rotated wrapping key", err)
	}

	if err := os.Rename(tmp, s.path); err != nil {
		// Put the old key back so the untouched file on disk stays readable.
		if oldKey != nil {
			_ = s.kr.Set(s.keyName, base64.StdEncoding.EncodeToString(oldKey))
		}
		return kiterrors.Wrap(ErrEnvelopeIO, "keyring: swap rotated secrets file", err)
	}
	syncDir(s.dir)

	s.key = newKey
	s.aead = newAEAD
	return nil
}

// --- internals ---

func newEnvelopeFile() *envelopeFile {
	return &envelopeFile{
		Version: envelopeFormatVersion,
		Alg:     envelopeAlg,
		Entries: make(map[string]envelopeEntry),
	}
}

func (s *EnvelopeStore) fileExists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

// readFileLocked loads and validates the secrets file. A missing file yields an
// empty in-memory file; every other failure is an error. It never repairs,
// truncates, or resets.
func (s *EnvelopeStore) readFileLocked() (*envelopeFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return newEnvelopeFile(), nil
		}
		return nil, wrapf(ErrEnvelopeIO, err, "keyring: read %s", s.path)
	}

	var f envelopeFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, wrapf(ErrEnvelopeCorrupt, err, "keyring: parse %s", s.path)
	}
	if f.Version != envelopeFormatVersion {
		return nil, kiterrors.Newf(ErrEnvelopeCorrupt,
			"keyring: %s has unsupported format version %d (this build understands %d)",
			s.path, f.Version, envelopeFormatVersion)
	}
	if f.Alg != envelopeAlg {
		return nil, kiterrors.Newf(ErrEnvelopeCorrupt,
			"keyring: %s uses unsupported algorithm %q (this build understands %q)",
			s.path, f.Alg, envelopeAlg)
	}
	if f.Entries == nil {
		f.Entries = make(map[string]envelopeEntry)
	}
	return &f, nil
}

// loadKeyLocked returns the AEAD for the wrapping key, caching it.
//
// mustExist reports whether losing the key would mean losing data — i.e. there
// is a secrets file with entries. When true, a missing keyring item is a hard
// error. When false, the key is generated and stored on demand.
func (s *EnvelopeStore) loadKeyLocked(mustExist bool) (cipher.AEAD, error) {
	if s.aead != nil {
		return s.aead, nil
	}

	raw, err := s.kr.Get(s.keyName)
	switch {
	case err == nil:
		// fall through to decode
	case isNotFound(err):
		if mustExist {
			return nil, kiterrors.Newf(ErrEnvelopeKeyMissing,
				"keyring: %s exists but wrapping key %q is not in the keyring; refusing to reset (data is intact but unreadable)",
				s.path, s.keyName)
		}
		return s.generateKeyLocked()
	default:
		return nil, wrapf(ErrEnvelopeIO, err, "keyring: read wrapping key %q", s.keyName)
	}

	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, wrapf(ErrEnvelopeKeyInvalid, err, "keyring: wrapping key %q is not valid base64", s.keyName)
	}
	if len(key) != wrappingKeySize {
		return nil, kiterrors.Newf(ErrEnvelopeKeyInvalid,
			"keyring: wrapping key %q is %d bytes, want %d", s.keyName, len(key), wrappingKeySize)
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, kiterrors.Wrap(ErrEnvelopeKeyInvalid, "keyring: init cipher", err)
	}
	s.key = key
	s.aead = aead
	return aead, nil
}

func (s *EnvelopeStore) generateKeyLocked() (cipher.AEAD, error) {
	key := make([]byte, wrappingKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, kiterrors.Wrap(kiterrors.ErrInternal, "keyring: generate wrapping key", err)
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, kiterrors.Wrap(kiterrors.ErrInternal, "keyring: init cipher", err)
	}
	if err := s.kr.Set(s.keyName, base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, wrapf(ErrEnvelopeIO, err, "keyring: store wrapping key %q", s.keyName)
	}
	s.key = key
	s.aead = aead
	return aead, nil
}

// isNotFound reports whether err is the keyring "no such item" sentinel.
func isNotFound(err error) bool {
	return stderrors.Is(err, ErrNotFound)
}

// wrapf builds a kit UserError with a formatted technical message wrapping err.
// The kit errors package has Wrap and Newf but no Wrapf; this is the local
// convenience that keeps the call sites readable.
func wrapf(code kiterrors.Code, err error, format string, args ...any) *kiterrors.UserError {
	return kiterrors.Wrap(code, fmt.Sprintf(format, args...), err)
}

// seal encrypts pt with a fresh random nonce, binding the entry name as
// associated data so the ciphertext cannot be relocated to another key.
func seal(aead cipher.AEAD, name string, pt []byte) (envelopeEntry, error) {
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return envelopeEntry{}, kiterrors.Wrap(kiterrors.ErrInternal, "keyring: generate nonce", err)
	}
	ct := aead.Seal(nil, nonce, pt, []byte(name))
	return envelopeEntry{
		Nonce: base64.StdEncoding.EncodeToString(nonce),
		CT:    base64.StdEncoding.EncodeToString(ct),
	}, nil
}

func open(aead cipher.AEAD, name string, e envelopeEntry) ([]byte, error) {
	nonce, err := base64.StdEncoding.DecodeString(e.Nonce)
	if err != nil {
		return nil, wrapf(ErrEnvelopeCorrupt, err, "keyring: entry %q has invalid nonce encoding", name)
	}
	if len(nonce) != aead.NonceSize() {
		return nil, kiterrors.Newf(ErrEnvelopeCorrupt,
			"keyring: entry %q nonce is %d bytes, want %d", name, len(nonce), aead.NonceSize())
	}
	ct, err := base64.StdEncoding.DecodeString(e.CT)
	if err != nil {
		return nil, wrapf(ErrEnvelopeCorrupt, err, "keyring: entry %q has invalid ciphertext encoding", name)
	}
	pt, err := aead.Open(nil, nonce, ct, []byte(name))
	if err != nil {
		return nil, wrapf(ErrEnvelopeDecrypt, err,
			"keyring: entry %q failed authenticated decryption (wrong wrapping key, tampering, or a relocated entry)", name)
	}
	return pt, nil
}

// writeFileLocked writes f durably and atomically: temp file in the same
// directory, Sync, close, rename.
func (s *EnvelopeStore) writeFileLocked(f *envelopeFile) error {
	tmp, err := s.writeTempLocked(f)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return wrapf(ErrEnvelopeIO, err, "keyring: rename into %s", s.path)
	}
	syncDir(s.dir)
	return nil
}

// writeTempLocked serializes f into a 0600 temp file in the target directory
// and fsyncs it. The caller renames it into place (or removes it).
func (s *EnvelopeStore) writeTempLocked(f *envelopeFile) (string, error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", wrapf(ErrEnvelopeIO, err, "keyring: create %s", s.dir)
	}

	data, err := json.Marshal(f)
	if err != nil {
		return "", kiterrors.Wrap(kiterrors.ErrInternal, "keyring: marshal secrets file", err)
	}

	// Same directory as the target: rename is only atomic within a filesystem.
	tf, err := os.CreateTemp(s.dir, ".secrets-*.tmp")
	if err != nil {
		return "", wrapf(ErrEnvelopeIO, err, "keyring: create temp file in %s", s.dir)
	}
	name := tf.Name()

	// CreateTemp already uses 0600, but state this explicitly: the file holds
	// ciphertext, and the guarantee should not depend on stdlib internals.
	if err := tf.Chmod(0o600); err != nil {
		_ = tf.Close()
		_ = os.Remove(name)
		return "", wrapf(ErrEnvelopeIO, err, "keyring: chmod %s", name)
	}
	if _, err := tf.Write(data); err != nil {
		_ = tf.Close()
		_ = os.Remove(name)
		return "", wrapf(ErrEnvelopeIO, err, "keyring: write %s", name)
	}
	// Sync before rename: a rename that lands before the data does leaves a
	// zero-length secrets file after a crash.
	if err := tf.Sync(); err != nil {
		_ = tf.Close()
		_ = os.Remove(name)
		return "", wrapf(ErrEnvelopeIO, err, "keyring: sync %s", name)
	}
	if err := tf.Close(); err != nil {
		_ = os.Remove(name)
		return "", wrapf(ErrEnvelopeIO, err, "keyring: close %s", name)
	}
	return name, nil
}

// syncDir fsyncs a directory so a rename survives a crash. Best effort: it is
// not supported on Windows, and a failure here costs durability, not
// correctness.
func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
