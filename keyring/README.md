# keyring

OS keyring credential storage with environment variable fallback. Wraps the system keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service) behind a simple `Store` interface.

## Usage

```go
import "github.com/jrschumacher/wails-kit/keyring"

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

## Integration with settings

The settings package uses keyring internally for `FieldPassword` fields. Pass a keyring store when creating the settings service:

```go
svc := settings.NewService(
    settings.WithAppName("my-app"),
    settings.WithKeyring(store),
)
```
