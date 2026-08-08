# keyring — agent notes

## Purpose

`keyring` provides credential storage behind one `Store` interface: `OSStore`
(thin wrapper over the platform credential manager, optional env-var fallback
for headless/CI) and `EnvelopeStore` (a single XChaCha20-Poly1305-encrypted
`secrets.json`, wrapped by one 32-byte key held in an underlying `Store`). It
owns secret persistence only — not *which* fields are secrets (that's
`settings` via `FieldPassword`), not passphrase-based key derivation (there
is no passphrase; the wrapping key is random, generated once), and not
migration from a pre-envelope layout (dropped per AD-8 — v2 has no v1
installs to migrate).

## Public API (load-bearing signatures)

```go
type Store interface {
    Set(key, value string) error
    Get(key string) (string, error)
    Delete(key string) error
    Has(key string) bool
}

type KeyLister interface{ Keys() ([]string, error) } // optional; see Landmines

func NewOSStore(serviceName string, opts ...OSStoreOption) *OSStore
func NewMemoryStore() *MemoryStore // test double; never touches the OS keychain

func NewEnvelopeStore(dirs *appdirs.Dirs, kr Store, opts ...EnvelopeStoreOption) (*EnvelopeStore, error)
func (s *EnvelopeStore) Path() string
func (s *EnvelopeStore) Keys() ([]string, error) // KeyLister
func (s *EnvelopeStore) Rotate() error
```

## Invariants (do not break)

- `Store` stays exactly four methods. `Keys` is deliberately not on it — most
  OS keyrings can't enumerate portably; enumeration is the opt-in `KeyLister`
  extension, satisfied only by `EnvelopeStore`.
- `EnvelopeStore` caches only the wrapping key + `cipher.AEAD` (`s.key`,
  `s.aead`); every `Get`/`Set`/`Delete`/`Keys` reads the file fresh under
  `s.mu`. No decrypted entry is held in memory across calls.
- Every write goes temp-file → `Chmod 0600` → write → `Sync` → close →
  `rename` → best-effort `syncDir`. Don't skip `Sync` before rename — that's
  what makes a crash leave either the old file or the new one, never a
  truncated one.
- The entry name is bound as AEAD associated data in `seal`/`open`, so
  relocating one entry's ciphertext under a different key name fails
  decryption instead of silently succeeding. Preserve this if the format
  changes.
- On any wrapping-key or decrypt/parse failure: **refuse, never silently
  reset or regenerate**. A "helpful" reset destroys data that may still be
  recoverable. Keep `envelope.go`'s header comment and the README "Failure
  policy" section in sync with actual behavior.
- Never soften the "not more secure" language in `envelope.go`'s header or
  the README. The wrapping key sits in the same OS keyring any per-item
  secret would; a same-user process that reads one can read the other.
- No test may construct an `OSStore` or reach the network — a real Keychain
  entry was created by accident in this project's history (see the header
  comment in `envelope_test.go`). Back every test with `NewMemoryStore()` and
  `t.TempDir()`.

## Dependencies & insulation

- `appdirs` — `secrets.json` lives in `dirs.Config()`, not `dirs.Data()`.
- `errors` (`kiterrors`) — every failure is a `*kiterrors.UserError` with an
  `ErrEnvelope*` code, registered via `RegisterMessages` in `init()`.
- `golang.org/x/crypto/chacha20poly1305` — the only cipher. Don't add a
  second construction.
- `github.com/zalando/go-keyring` — confined to `os_keyring.go`.
- No `wails/v3` import; this package is Wails-free per the AD-4 allowlist.

## Extension points

- New backends implement `Store`, and optionally `KeyLister` if enumeration
  is cheap. Follow `EnvelopeStore`'s compile-time assertions
  (`var _ Store = ...`, `var _ KeyLister = ...`) as the pattern.
- `EnvelopeStoreOption` is the functional-options seam for construction
  knobs; add new options there, not a new constructor.
- A future on-disk format bump goes through `envelopeFormatVersion` /
  `envelopeAlg` — `readFileLocked` already rejects anything unrecognized, so
  a bump is additive.

## Testing

- Doubles: `NewMemoryStore()` + `appdirs.New(name, appdirs.WithConfigDir(t.TempDir()))`.
  Never `NewOSStore` in a test.
- `go test -race ./keyring/...` must stay green. Preserve the adversarial
  shapes: entry-relocation (`TestEnvelopeEntryRelocation`), tampered
  ciphertext, wrong-key, corrupt/truncated file, concurrent
  `Set`/`Get`/`Rotate` (`TestEnvelopeConcurrent*`), and the `KeyLister`
  type-assertion (`TestEnvelopeKeyListerTypeAssertion`).
- Not automatable: a real OS keychain unlock prompt per platform. Out of
  scope for this package's suite by design — verify manually via
  `examples/keyring` swapped to `NewOSStore` if ever needed, then revert.

## File map

- `keyring.go` — `Store`, `KeyLister`, `ErrNotFound`, `SetJSON`/`GetJSON`,
  env-var key mangling.
- `memory.go` — `MemoryStore`, the test double.
- `os_keyring.go` — `OSStore`, the real OS-credential-manager backend.
- `envelope.go` — `EnvelopeStore`: construction, CRUD, `Rotate`, file/crypto
  internals. Read the package doc comment first — it's the design rationale.
- `envelope_test.go` — adversarial suite; header documents the
  no-OS-keyring/no-network rule.
- `keyring_test.go` — `MemoryStore`/JSON-helper/env-fallback tests.
- `README.md` — quickstart, size-limit rationale, threat model, rotation
  semantics, failure policy, `KeyLister` usage.

## Landmines

- `Keys()` exists on `EnvelopeStore` but is intentionally absent from
  `Store`. Don't "fix" this by adding it to `Store` — it would make `Store`
  unimplementable against real OS keyrings (no portable list-by-service call
  in `go-keyring`).
- `Has` does not decrypt and can't distinguish a healthy entry from a corrupt
  one — a damaged file reports `false`, not an error. Use `Get` when the
  difference matters.
- Rotation's key-swap-then-rename order leaves a narrow window: if the
  process crashes between the keyring `Set` and a failing `rename`, and the
  best-effort old-key restore in `Rotate` also fails, the *new* key ends up
  in the keyring pointing at the *old* (unrotated) file. Documented, not
  silently accepted — see `Rotate`'s doc comment and the README's "Rotation"
  section. Don't attempt full atomicity; it can't be done cheaply across a
  keyring and a filesystem.
- Every write re-serializes and fsyncs the *whole* `secrets.json` — cost is
  O(total bytes stored), not O(bytes changed). Fine for a few dozen
  credentials; wrong for a hot or large-N workload.
- The migration-from-per-item-keychain follow-up from earlier planning is
  deliberately **gone**, not deferred (AD-8). Don't resurrect it without a
  fresh decision from the roadmap owner.
