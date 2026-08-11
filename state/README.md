# state — Generic Typed State Persistence

Lightweight typed state persistence to disk. Fills the gap between no persistence and the full `settings` package — for cases where you just need to save/load a struct without schema, validation, or keyring integration.

## Usage

```go
import "github.com/jrschumacher/wails-kit/v2/state"

type WindowState struct {
    Width     int  `json:"width"`
    Height    int  `json:"height"`
    X         int  `json:"x"`
    Y         int  `json:"y"`
    Maximized bool `json:"maximized"`
}

store, err := state.New[WindowState](
    state.WithAppName[WindowState]("my-app"),
    state.WithName[WindowState]("window"), // ~/Library/Application Support/my-app/state/window.json
)
// err is non-nil if neither WithAppName nor WithStoragePath is provided —
// New has nowhere to persist state otherwise.

// Load returns zero value if file doesn't exist. If the file exists but is
// corrupt (fails to parse as JSON), Load quarantines it (renamed aside as
// "<path>.corrupt-<unix-nano>", preserving the bytes) and also returns
// defaults with no error — it never errors forever on a corrupt file.
s, err := store.Load()

// LoadDetailed is Load plus a `recovered` flag that is true only when this
// call just quarantined a corrupt file — use it when "no file" and
// "corrupt file, now reset" must be told apart (see firstrun).
s, recovered, err := store.LoadDetailed()

// Save with a durable atomic write (tmp + fsync + rename)
s.Width = 800
err = store.Save(s)

// Delete the state file
err = store.Delete()
```

## Options

| Option | Description |
|--------|-------------|
| `WithAppName(name)` | Uses `appdirs.Data()` for storage directory |
| `WithName(name)` | Names the state file (e.g., `"window"` → `window.json`). May be combined with `WithAppName` in either order — the path is re-derived from whichever name is current. |
| `WithStoragePath(path)` | Override the full file path |
| `WithEmitter(e)` | Optional event emitter |
| `WithDefaults(T)` | Default value returned when no file exists |

`New` requires at least one of `WithAppName` or `WithStoragePath`; it
returns an error (`state_config`) if neither is provided, since otherwise
there is nowhere to persist state.

## Storage paths

State files are stored in a `state/` subdirectory of the app's data directory:

- **macOS:** `~/Library/Application Support/{app}/state/{name}.json`
- **Linux:** `$XDG_DATA_HOME/{app}/state/{name}.json`
- **Windows:** `%AppData%/{app}/state/{name}.json`

## Events

| Event | Payload | Description |
|-------|---------|-------------|
| `state:loaded` | `StateLoadedPayload{Name}` | Emitted after state is loaded from disk |
| `state:saved` | `StateSavedPayload{Name}` | Emitted after state is saved to disk |
| `state:corrupted` | `StateCorruptedPayload{Name, Path, QuarantinePath, Err}` | Emitted after Load/LoadDetailed quarantines and recovers from a corrupt file — fires in addition to (after) `state:loaded` for that same call |

All events are emitted **after** the internal lock is released, so a
handler is free to call `Load`/`Save` back on the same store without
deadlocking.

## Error codes

| Code | User message |
|------|-------------|
| `state_load` | Failed to load application state. Please try again. |
| `state_save` | Failed to save application state. Please try again. |
| `state_config` | State store is misconfigured. Please contact support. (returned by `New` when neither `WithAppName` nor `WithStoragePath` is set) |

## Design

- **Generic** — `state.New[T]()`, fully type-safe
- **No schema or validation** — that's what `settings` is for
- **No keyring** — not for secrets, just state
- **Atomic, durable writes** — write-to-tmp + fsync + rename prevents corruption and truncated files after a crash
- **Self-healing reads** — a corrupt file is quarantined (bytes preserved) and recovered with defaults instead of erroring forever
- **Mutex-protected** — safe for single-process concurrent access
