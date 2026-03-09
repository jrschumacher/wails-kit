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

## Multi-window support

Register windows to send targeted events:

```go
emitter.RegisterWindow("preferences", prefsBackend)

// Target a specific window
emitter.EmitTo("preferences", events.SettingsChanged, payload)

// Broadcast to all windows (default backend)
emitter.Emit(events.SettingsChanged, payload)
```

## Event history and replay

Enable history to replay recent events to late-joining windows:

```go
emitter := events.NewEmitter(backend, events.WithHistory(100))

// Emit some events...
emitter.Emit(events.SettingsChanged, payload)

// Later, when a new window joins:
emitter.RegisterWindow("preferences", prefsBackend)
emitter.Replay("preferences", events.SettingsChanged) // sends most recent
emitter.ReplayAll("preferences")                       // sends latest of each event name
```

History uses a ring buffer — older events are evicted when the buffer is full. Both `Emit` and `EmitTo` events are recorded.

## Middleware

Middleware processes broadcast events (`Emit`) before they reach the backend. Targeted events (`EmitTo`) bypass middleware.

### Debounce

Delays emission until no new events arrive for the specified duration. Only the last payload is emitted.

```go
emitter := events.NewEmitter(backend,
    events.WithDebounce(events.SettingsChanged, 100*time.Millisecond),
)
```

### Throttle

Allows at most one event per duration (leading edge). Events within the throttle window are dropped.

```go
emitter := events.NewEmitter(backend,
    events.WithThrottle("mouse:move", 16*time.Millisecond),
)
```

### Batching

Collects events until `maxSize` is reached or the duration elapses, then emits them as a single `[]any` payload.

```go
emitter := events.NewEmitter(backend,
    events.WithBatching("log:entry", 500*time.Millisecond, 50),
)
```

### Cleanup

Call `Close()` when shutting down to flush pending debounced and batched events and stop async handler goroutines:

```go
emitter.Close()
```

## Scoped emitters

Multi-tab and multi-pane apps can scope events so subscribers only receive
events from a specific scope:

```go
// Create a scoped emitter for a specific tab
tabEmitter := emitter.Scope("tab:abc123")
tabEmitter.Emit("stream:delta", payload)

// Subscribe to events from a specific scope only
unsub := events.OnScoped(emitter, "tab:abc123", "stream:delta", func(p Payload) {
    // only receives from tab:abc123
})
defer unsub()

// Subscribe to all scopes of an event (including unscoped)
unsub = events.On(emitter, "stream:delta", func(p Payload) {
    // receives from all scopes and unscoped emits
})
defer unsub()
```

Scoped events are emitted to the backend with the wire name `@scope/eventName`.
Use `events.ScopedName(scope, name)` and `events.ParseScopedName(wireName)` to
construct and parse scoped event names.

## Async emission

By default, handlers run synchronously on the emitting goroutine. For hot
paths (e.g., LLM streaming), enable async delivery so slow handlers don't
block the emitter:

```go
emitter := events.NewEmitter(backend, events.WithAsync(100))
defer emitter.Close() // stops handler goroutines
```

Each handler gets a dedicated goroutine with a buffered channel. When the
buffer is full, events are dropped to avoid blocking the emitter.

## Typed subscriptions

Register type-safe handlers and unsubscribe when done:

```go
unsub := events.On(emitter, events.SettingsChanged, func(p events.SettingsChangedPayload) {
    // handle
})
unsub() // unsubscribe
```

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

mem.Events()              // []Record — all emitted events
mem.Last()                // *Record — most recent event
mem.Count()               // int — number of events
mem.Clear()               // reset
mem.Broadcasts()          // []Record — non-targeted events only
mem.EventsFor("main")    // []Record — events for a specific window
mem.WaitFor("name", 1*time.Second) // block until event is emitted
```

Each `Record` has `Name string`, `Data any`, and `WindowID string` (empty for broadcasts) fields.

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
