# keyring

OS keyring credential storage with environment variable fallback. Wraps the system keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service) behind a simple `Store` interface.

## Usage

```go
import "github.com/jrschumacher/wails-kit/v2/keyring"

// OS keyring with env var fallback
store := keyring.NewOSStore("my-app", keyring.WithEnvPrefix("MYAPP"))

// Basic operations
store.Set("api_key", "sk-abc123")
val, err := store.Get("api_key")
store.Has("api_key")  // true
store.Delete("api_key")

// Store structured data
keyring.SetJSON(store, "oauth_token", myToken)
keyring.GetJSON(store, "oauth_token", &token)
```

## Store interface

```go
type Store interface {
    Get(key string) (string, error)
    Set(key string, value string) error
    Has(key string) bool
    Delete(key string) error
}
```

## Environment variable fallback

When an env prefix is configured, `Get` checks the keyring first. If the key is not found, it checks the environment variable `{PREFIX}_{KEY}` (uppercased, with dots and dashes converted to underscores).

```go
store := keyring.NewOSStore("my-app", keyring.WithEnvPrefix("MYAPP"))
// Get("api_key") checks keyring, then falls back to MYAPP_API_KEY
```

This enables headless/CI operation without an OS keyring.

## JSON helpers

For storing structured data like OAuth tokens:

```go
type Token struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
}

keyring.SetJSON(store, "oauth", Token{AccessToken: "abc", RefreshToken: "def"})

var token Token
keyring.GetJSON(store, "oauth", &token)
```

## Testing

Use the in-memory store for tests:

```go
store := keyring.NewMemoryStore()
```

## EnvelopeStore — encrypted secrets file for large or numerous credentials

`OSStore` puts every secret directly in the OS credential manager, one item per
key. That's the right default for a handful of small values, but it runs into
a hard wall: **Windows Credential Manager caps a single credential's
`CredentialBlob` at 2560 bytes**, and libsecret (Linux) has similarly
uncomfortable practical limits. An OAuth token bundle, a service-account JSON
file, or a client certificate and private key can blow past that. `OSStore`
also means one keychain-unlock prompt *per item* on platforms that prompt.

`EnvelopeStore` solves both problems by storing exactly one thing in the OS
keyring — a 32-byte wrapping key — and keeping every actual secret,
encrypted, in a `secrets.json` file under `dirs.Config()`. Secret size is then
bounded only by the filesystem, and there is one unlock prompt for the whole
app, not one per credential.

```go
dirs := appdirs.New("my-app")
kr := keyring.NewOSStore("my-app") // or keyring.NewMemoryStore() in tests
store, err := keyring.NewEnvelopeStore(dirs, kr)
if err != nil {
    // handle
}

_ = store.Set("llm.anthropic.secret", "sk-...")
val, err := store.Get("llm.anthropic.secret")
```

`EnvelopeStore` implements `Store`, so it drops in anywhere a `Store` is
expected — including as the backing store `settings.WithKeyring` uses for
password fields.

### Threat model — read this before treating it as "more secure"

**Envelope encryption is not a security improvement over storing each secret
as its own OS keyring item.** The wrapping key is itself held in the OS
keyring, unprotected beyond whatever the OS credential manager already does.
Any process running as the same user that could read an individual keyring
item today can read the wrapping key and decrypt every entry in
`secrets.json`. The attack surface for a local attacker is unchanged; only
two things change: the per-item size limit disappears, and there's one
unlock surface instead of N. Do not describe `EnvelopeStore` to end users as
"more secure than the alternative" — it isn't, against the threat that
matters (a malicious or compromised process running as the same user). What
it *does* still protect against is a stolen disk image or a bystander reading
`secrets.json` off a synced folder: the file is ciphertext without the
wrapping key.

### Rotation

`store.Rotate()` generates a fresh wrapping key, decrypts and re-encrypts
every entry under it with new nonces, and swaps both the key and the file
into place. Two properties are correct trade-offs, not bugs, and are worth
understanding before you rely on them:

- **Rotation is not atomic across the keyring write and the file rename.**
  A keyring `Set` and a filesystem `rename` cannot be made atomic with respect
  to each other — they're different subsystems. `Rotate` minimizes the
  unrecoverable window instead of eliminating it: the re-encrypted file is
  fully written and fsynced to a temp path *first*, then the new key is
  stored in the keyring, then the file is renamed into place. If the rename
  fails after the key swap, `Rotate` makes a best-effort attempt to restore
  the old key to the keyring so the untouched original file stays readable.
  The residual risk is that single rename call, which on any real filesystem
  is about as small a window as this kind of swap gets.
- **Every write costs O(total bytes stored), not O(bytes changed).** `Set`,
  `Delete`, and `Rotate` all re-serialize and fsync the *entire* file, every
  time — there's no incremental append or in-place patch. For the intended
  workload (a few dozen credentials, written rarely, read once at startup)
  this is unmeasurable and buys simplicity plus crash-atomicity for free. It
  would be the wrong data structure for thousands of hot, frequently-updated
  entries — don't reach for `EnvelopeStore` for anything shaped like a cache.

### Failure policy — refuse, never reset

Every failure mode surfaces as an error; nothing is silently repaired or
reset. Specifically:

- If `secrets.json` exists but the wrapping key is missing from the OS
  keyring, `Get`/`Set` refuse and return `ErrEnvelopeKeyMissing` rather than
  generating a new key and silently orphaning the existing file. The data is
  intact but unreadable until the keyring entry is restored (or the file is
  deliberately reset by the caller).
- A wrapping key present but not valid 32-byte base64 is
  `ErrEnvelopeKeyInvalid`.
- A `secrets.json` that fails to parse, declares an unsupported format
  version, or names an unsupported algorithm is `ErrEnvelopeCorrupt` — never
  truncated or overwritten by the store itself.
- A ciphertext that fails authenticated decryption (wrong key, tampering, or
  an entry relocated to a different key name — the entry name is bound as
  AEAD associated data specifically to catch that last case) is
  `ErrEnvelopeDecrypt`.

The reasoning: a store that "helpfully" resets on any of these conditions
turns into "all of my API keys vanished" from the user's point of view, and
destroys data that might otherwise be recoverable (a backup, another
machine's keychain, a Time Machine snapshot). Refusing loudly is the safer
failure mode even though it is less convenient in the moment.

### Listing keys — the `KeyLister` extension

`Store` intentionally has no `Keys` method: the OS keyring backends don't
offer a portable way to enumerate items for a service through the underlying
`go-keyring` API, so requiring it on every `Store` would make `Store`
unimplementable against the real OS keyring. `EnvelopeStore` *can* enumerate
cheaply — its entries live in one file it fully controls — so it implements
the optional `KeyLister` interface instead:

```go
type KeyLister interface {
    Keys() ([]string, error)
}
```

Callers that need enumeration type-assert:

```go
if lister, ok := store.(keyring.KeyLister); ok {
    keys, err := lister.Keys() // sorted, does not decrypt
}
```

## Integration with settings

The settings package uses keyring internally for `FieldPassword` fields. Pass a keyring store when creating the settings service:

```go
svc := settings.NewService(
    settings.WithAppName("my-app"),
    settings.WithKeyring(store),
)
```
