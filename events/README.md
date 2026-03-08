# events

Type-safe event emission wrapper for Wails v3 apps. Keeps the kit Wails-version-agnostic via a `Backend` interface.

## Usage

```go
import "github.com/jrschumacher/wails-kit/events"

// In your app setup, wrap the Wails app
emitter := events.NewEmitter(events.BackendFunc(func(name string, data any) {
    app.EmitEvent(name, data)
}))

// Emit events
emitter.Emit(events.SettingsChanged, events.SettingsChangedPayload{
    Keys: []string{"appearance.theme"},
})
```

## Backend interface

```go
type Backend interface {
    Emit(name string, data any)
}
```

`BackendFunc` adapts a plain function to the `Backend` interface for convenience.

## Kit-provided events

| Constant | Event name | Payload |
|----------|-----------|---------|
| `SettingsChanged` | `settings:changed` | `SettingsChangedPayload{Keys []string}` |

The `updates` package also emits events through this system. See the [updates README](../updates/README.md) for details.

## Testing

`MemoryEmitter` captures events in memory for test assertions:

```go
mem := events.NewMemoryEmitter()
emitter := events.NewEmitter(mem)

// ... trigger actions ...

mem.Events()  // []Record — all emitted events
mem.Last()    // *Record — most recent event
mem.Count()   // int — number of events
mem.Clear()   // reset
```

Each `Record` has `Name string` and `Data any` fields.

## Frontend pattern

Define matching TypeScript constants and types:

```ts
export const Events = {
    SETTINGS_CHANGED: 'settings:changed',
    UPDATES_AVAILABLE: 'updates:available',
    UPDATES_DOWNLOADING: 'updates:downloading',
    UPDATES_READY: 'updates:ready',
    UPDATES_ERROR: 'updates:error',
} as const

export interface EventMap {
    [Events.SETTINGS_CHANGED]: { keys: string[] }
    [Events.UPDATES_AVAILABLE]: { version: string; releaseNotes: string; releaseUrl: string }
    [Events.UPDATES_DOWNLOADING]: { version: string; progress: number; downloaded: number; total: number }
    [Events.UPDATES_READY]: { version: string }
    [Events.UPDATES_ERROR]: { message: string; code: string }
}
```
