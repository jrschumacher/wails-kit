package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"sync"

	"github.com/zalando/go-keyring"
)

// SecretSentinel is the placeholder GetValues emits in place of a stored
// password value, and the value SetValues interprets as "leave the stored
// secret unchanged".
//
// It exists because a settings page renders every field, and a UI that renders
// a real API key into an <input> hands that key to any XSS or hostile
// dependency in the frontend. The render therefore shows the sentinel, and the
// save posts it straight back; the service recognises it and does not touch the
// stored secret.
//
// The value is chosen so a real user submission cannot collide with it:
//
//   - It is far longer and more structured than anything a person types into a
//     password field, and it embeds a fixed random suffix.
//   - It is deliberately NOT a row of asterisks or bullets. Those are the
//     obvious choice and the wrong one: a user who selects the masked text,
//     copies it and pastes it back would silently keep their old key.
//   - It is printable ASCII rather than something containing NUL or other
//     control characters. A control-character sentinel is harder to collide
//     with, but any frontend sanitiser that strips it would turn a harmless
//     no-op into the destruction of the user's stored key.
//
// If a user does enter this exact string, the effect is benign: their stored
// secret is left as it was.
const SecretSentinel = "__wails-kit:secret-unchanged:6f2a91c4d0e8__"

// Errors returned by every SecretStore implementation. Callers must be able to
// tell these apart: "the user has not set this key yet" is a normal state that
// leads to an empty field, while "the backend is broken" must be reported, not
// papered over. Conflating them makes an app silently prompt for a key the user
// already stored, and then overwrite it.
var (
	// ErrSecretNotFound means the key has no stored value. Get returns it;
	// Delete does not, because deleting an absent key is a no-op.
	ErrSecretNotFound = errors.New("settings: secret not found")

	// ErrSecretStoreUnavailable means the backend could not be reached or used
	// at all — no D-Bus session on headless Linux, a locked or unavailable
	// login keychain, an unreadable or corrupt secrets file. Every returned
	// error wrapping this one also carries backend-specific detail.
	ErrSecretStoreUnavailable = errors.New("settings: secret store unavailable")
)

// SecretStore persists values that must not be written into settings.json.
//
// Implementations must satisfy this contract:
//
//   - Get returns the stored value, or an error wrapping ErrSecretNotFound when
//     the key has never been set. Any other failure wraps
//     ErrSecretStoreUnavailable.
//   - Set creates or replaces the value for key.
//   - Delete removes the value for key and returns nil when the key is already
//     absent. Only a backend failure is an error.
//
// A stored empty string is a stored value, distinct from an absent key. The
// Service maps an empty submission to Delete so the two cannot drift apart in
// practice.
type SecretStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

// --- Keyring (default) ---

// KeyringSecretStore stores secrets in the operating system's credential store:
// the Keychain on macOS, the Credential Manager on Windows, and the Secret
// Service (libsecret/gnome-keyring/kwallet) over D-Bus on Linux.
//
// This is the default because it is the only backend where the secret is
// protected by something other than file permissions, and it requires no setup
// from the user.
//
// It is genuinely unavailable in some environments — a headless Linux CI runner
// with no D-Bus session, a minimal container. There, every operation fails with
// ErrSecretStoreUnavailable rather than falling back to disk. That failure is
// deliberate and loud: see WithPlaintextFileSecrets for the explicit opt-in.
type KeyringSecretStore struct {
	service string
}

// NewKeyringSecretStore returns a store that namespaces its entries under
// service, which should be the application name. Construction performs no I/O,
// so it never prompts and never fails; problems surface on first use.
func NewKeyringSecretStore(service string) *KeyringSecretStore {
	return &KeyringSecretStore{service: service}
}

func (k *KeyringSecretStore) Get(key string) (string, error) {
	v, err := keyring.Get(k.service, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", fmt.Errorf("%w: %q", ErrSecretNotFound, key)
		}
		return "", k.unavailable("read", key, err)
	}
	return v, nil
}

func (k *KeyringSecretStore) Set(key, value string) error {
	if err := keyring.Set(k.service, key, value); err != nil {
		return k.unavailable("write", key, err)
	}
	return nil
}

func (k *KeyringSecretStore) Delete(key string) error {
	if err := keyring.Delete(k.service, key); err != nil {
		// Deleting something that was never stored is the desired end state.
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		return k.unavailable("delete", key, err)
	}
	return nil
}

// unavailable wraps a backend error with the hint a developer needs, because
// the underlying D-Bus or Keychain error alone rarely explains what to do.
func (k *KeyringSecretStore) unavailable(op, key string, err error) error {
	return fmt.Errorf(
		"%w: cannot %s %q in the OS keyring (service %q): %w; "+
			"on a headless or containerised system, opt in explicitly with settings.WithPlaintextFileSecrets",
		ErrSecretStoreUnavailable, op, key, k.service, err,
	)
}

// --- File (explicit opt-in) ---

// FileSecretStore keeps secrets in a JSON file with 0600 permissions.
//
// The contents are PLAINTEXT. This exists only for environments where the OS
// keyring is genuinely unreachable — headless Linux CI with no D-Bus or
// libsecret, some containers — and it is never selected automatically. A silent
// fallback would be the worst outcome available: the user believes their API
// keys are in the Keychain while they sit readable on disk, and get swept into
// every backup and sync.
type FileSecretStore struct {
	path string
	mu   sync.Mutex
}

// NewPlaintextFileSecretStore returns a file-backed store at path.
//
// The name says "plaintext" because a call site is the only place the trade-off
// is visible, and construction logs a warning for the same reason: choosing
// this backend should be hard to do by accident and hard to forget about.
func NewPlaintextFileSecretStore(path string) *FileSecretStore {
	slog.Warn(
		"settings: storing secrets in a plaintext file instead of the OS keyring",
		"path", path,
		"detail", "API keys will be readable by any process running as this user and will be captured by file backups",
	)
	return &FileSecretStore{path: path}
}

func (f *FileSecretStore) Get(key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	secrets, err := f.readLocked()
	if err != nil {
		return "", err
	}
	v, ok := secrets[key]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrSecretNotFound, key)
	}
	return v, nil
}

func (f *FileSecretStore) Set(key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	secrets, err := f.readLocked()
	if err != nil {
		return err
	}
	secrets[key] = value
	return f.writeLocked(secrets)
}

func (f *FileSecretStore) Delete(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	secrets, err := f.readLocked()
	if err != nil {
		return err
	}
	if _, ok := secrets[key]; !ok {
		return nil
	}
	delete(secrets, key)
	return f.writeLocked(secrets)
}

// readLocked decodes the secrets file. A missing file is an empty set; an
// unreadable or malformed one is ErrSecretStoreUnavailable, never
// ErrSecretNotFound — reporting a corrupt file as "no such key" would lead the
// caller to overwrite whatever was still recoverable.
func (f *FileSecretStore) readLocked() (map[string]string, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("%w: cannot read %q: %w", ErrSecretStoreUnavailable, f.path, err)
	}

	var secrets map[string]string
	if err := json.Unmarshal(data, &secrets); err != nil {
		return nil, fmt.Errorf("%w: %q is not valid JSON: %w", ErrSecretStoreUnavailable, f.path, err)
	}
	if secrets == nil {
		secrets = map[string]string{}
	}
	return secrets, nil
}

func (f *FileSecretStore) writeLocked(secrets map[string]string) error {
	data, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: cannot encode secrets: %w", ErrSecretStoreUnavailable, err)
	}
	if err := writeFileAtomic(f.path, data); err != nil {
		return fmt.Errorf("%w: cannot write %q: %w", ErrSecretStoreUnavailable, f.path, err)
	}
	return nil
}

// --- Memory (tests) ---

// MemorySecretStore keeps secrets in memory only.
//
// It is intended for tests: a test suite that exercises password fields against
// the default keyring would write to the developer's real Keychain, and on
// macOS may block on an unlock prompt. Pass one with WithSecretStore.
type MemorySecretStore struct {
	mu      sync.RWMutex
	secrets map[string]string
}

func NewMemorySecretStore() *MemorySecretStore {
	return &MemorySecretStore{secrets: make(map[string]string)}
}

func (m *MemorySecretStore) Get(key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	v, ok := m.secrets[key]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrSecretNotFound, key)
	}
	return v, nil
}

func (m *MemorySecretStore) Set(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.secrets[key] = value
	return nil
}

func (m *MemorySecretStore) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.secrets, key)
	return nil
}

// Snapshot returns a copy of everything stored, for assertions in tests.
func (m *MemorySecretStore) Snapshot() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return maps.Clone(m.secrets)
}
