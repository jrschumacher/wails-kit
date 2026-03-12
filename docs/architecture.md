# Architecture

wails-kit is a Go module providing reusable infrastructure for Wails v3 desktop apps. Each package is independently importable and follows consistent patterns.

## Package dependency graph

```
┌──────────┐     ┌──────────┐     ┌──────────┐
│ settings │────▶│ keyring  │     │ logging  │
└────┬─────┘     └──────────┘     └────┬─────┘
     │                                 │
     └──────────────┐  ┌───────────────┘
                    ▼  ▼
                  ┌──────────┐
                  │ appdirs  │  (leaf — no kit dependencies)
                  └──────────┘
                       ▲
┌──────────┐     ┌─────┘
│ updates  │────▶│
└────┬──┬──┘     │
     │  │        │
     │  └────────┘
     ▼
┌──────────┐     ┌──────────┐
│ events   │     │  errors  │
└──────────┘     └──────────┘
     ▲                ▲
┌────┴────────────────┴────┐
│       lifecycle          │
└──────────────────────────┘

┌──────────┐────▶ appdirs, errors, events
│ database │
└──────────┘

┌─────────────┐
│ diagnostics │──▶ appdirs, settings, events, errors (all optional except errors)
└─────────────┘

┌─────────────┐────▶ events (optional)
│  shortcuts  │────▶ wails/v3 (native menus)
└─────────────┘

┌──────────┐────▶ appdirs, errors, events (optional)
│  state   │
└──────────┘
```

- `errors`, `events`, and `appdirs` are leaf packages with no kit dependencies
- `keyring` is a leaf package
- `settings/cli` depends on `settings` (CLI adapter, no additional external deps)
- `settings` depends on `keyring` for password field storage and `appdirs` for config paths
- `updates` depends on `errors`, `events`, and `appdirs`; optionally depends on `settings`
- `database` depends on `appdirs`, `errors`, and `events`
- `lifecycle` depends on `errors` and `events`; manages startup/shutdown ordering of any services
- `diagnostics` depends on `errors`; optionally depends on `appdirs`, `settings`, and `events`
- `logging` depends on `appdirs` for log directory paths
- `shortcuts` depends on `events` (optional) and `wails/v3` for native menu APIs
- `state` depends on `appdirs` and `errors`; optionally depends on `events`
- `settings/templates/anyllm` depends on `settings` and `any-llm-go` (external); provides LLM provider settings integration

### Frontend packages

```
┌───────────────────┐
│ @wails-kit/settings │──▶ @wails-kit/types
└───────────────────┘
┌───────────────────┐
│ @wails-kit/types  │     (leaf — no kit dependencies)
└───────────────────┘
```

- `@wails-kit/types` (`frontend/types`) — TypeScript type definitions mirroring Go schema types
- `@wails-kit/settings` (`frontend/settings`) — headless settings logic: validation, condition evaluation, dynamic option resolution. Mirrors `settings/validate.go`

## Design patterns

### Functional options

Every service uses the functional options pattern:

```go
type ServiceOption func(*Service)

func WithFoo(value string) ServiceOption {
    return func(s *Service) {
        s.foo = value
    }
}

func NewService(opts ...ServiceOption) *Service {
    s := &Service{}
    for _, opt := range opts {
        opt(s)
    }
    // apply defaults
    return s
}
```

This keeps constructors stable as new options are added.

### Event emission

Packages emit events through `*events.Emitter`, accepted via `WithEmitter`. The emitter is always optional — if nil, events are silently dropped.

```go
func (s *Service) emit(name string, data any) {
    if s.emitter != nil {
        s.emitter.Emit(name, data)
    }
}
```

Event names use `package:action` format (e.g., `updates:available`, `settings:changed`). Payloads are typed structs with JSON tags.

### Error handling

Packages use `errors.Code` constants and `errors.Wrap`/`errors.New` to create structured errors with both technical and user-facing messages. Error codes are registered in `init()`.

```go
const ErrMyOperation errors.Code = "my_operation"

func init() {
    errors.RegisterMessages(map[errors.Code]string{
        ErrMyOperation: "Something went wrong. Please try again.",
    })
}
```

### Optional dependencies

When a package can optionally integrate with another (e.g., `updates` with `settings`), it:

1. Accepts the dependency via a `With*` option
2. Checks for nil before using it
3. Falls back to static configuration when not provided

This keeps packages independently usable.

### Testing

- `events.MemoryEmitter` captures emitted events for assertions
- `keyring.NewMemoryStore()` replaces OS keyring in tests
- `net/http/httptest` mocks external APIs
- `t.TempDir()` for file-based tests

## Settings integration

Packages that provide user-configurable behavior export a `SettingsGroup()` function returning a `settings.Group`. The app composes these when creating the settings service:

```go
svc := settings.NewService(
    settings.WithAppName("my-app"),
    settings.WithGroup(updates.SettingsGroup()),
    settings.WithGroup(myAppSettingsGroup()),
)
```

The settings service owns persistence and validation. Packages read values from settings at call time rather than caching them, so changes take effect immediately.

## Split module publishing

Development uses a single `go.mod` monorepo. On release, a CI pipeline publishes per-package Go modules to [`jrschumacher/wails-kit-pub`](https://github.com/jrschumacher/wails-kit-pub) with vanity import paths:

```go
import "abnl.dev/wails-kit/appdirs"    // only pulls appdirs — no SQLite, no SDKs
import "abnl.dev/wails-kit/database"   // pulls goose + sqlite, nothing else
```

Each package gets its own `go.mod` with only its direct dependencies, tagged as `{pkg}/v{version}` (e.g., `appdirs/v1.2.0`). The publish pipeline is defined in `.github/scripts/publish-split-modules.sh`. Packages and their dependencies are auto-detected from the source code at publish time — `split-modules.json` only contains the vanity domain and pub repo config.

## Adding a new package

When adding a new package to wails-kit:

1. **Create the package directory** with implementation and tests
2. **Follow the patterns**: functional options, optional emitter/settings, error codes with `init()` registration
3. **Keep dependencies minimal**: prefer stdlib, avoid adding external deps for <200 lines of logic
4. **Add a package README** documenting usage, options, events, and error codes
5. **Update the root README** with a summary section linking to the package README
6. **Update this architecture doc** with the new package's position in the dependency graph
7. **Add the package name as a conventional commit scope** in `.github/workflows/ci.yml` and `CLAUDE.md`
8. **No config needed for split modules** — packages and deps are auto-detected at publish time
