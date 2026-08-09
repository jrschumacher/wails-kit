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

// Load returns zero value if file doesn't exist
s, err := store.Load()

// Save with atomic write (tmp + rename)
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

Both events are emitted **after** the internal lock is released, so a
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
- **Atomic writes** — write-to-tmp + rename prevents corruption
- **Mutex-protected** — safe for single-process concurrent access
