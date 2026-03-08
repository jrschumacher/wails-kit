# CLAUDE.md

## Project

wails-kit is a reusable Go module for Wails v3 desktop apps. It provides infrastructure packages that apps import as a library.

## Structure

Go module at `github.com/jrschumacher/wails-kit` with these packages:

- `keyring` — OS keyring credential storage
- `settings` — Schema-driven settings framework
- `llm` — LLM provider management (with `llm/anthropic`, `llm/openai`, `llm/mock`)
- `errors` — User-facing error types
- `events` — Typed event emission
- `logging` — Structured logging with rotation
- `updates` — GitHub Releases-based auto-updates

## Documentation

- `docs/architecture.md` — Package dependency graph, design patterns, how to add new packages
- `docs/settings-integration.md` — How packages define and consume settings
- Each package has its own `README.md` with detailed usage docs

## Conventions

### Go patterns

- **Functional options**: `type ServiceOption func(*Service)` with `With*` constructors
- **Service structs**: concrete struct + `NewService(opts ...ServiceOption)` constructor
- **Events**: accept `*events.Emitter`, emit named events with typed payload structs
- **Errors**: use `errors.Code`, `errors.New`, `errors.Wrap` from the kit `errors` package; register user-facing messages via `errors.RegisterMessages`
- **Testing**: use `events.MemoryEmitter` and `keyring.NewMemoryStore()` for test doubles; use `net/http/httptest` for HTTP mocking
- **Minimal dependencies**: prefer stdlib over external libraries; avoid adding deps for functionality that can be implemented in <200 lines

### Commits

This project uses **conventional commits**. Format:

```
type(scope): description
```

**Types**: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`

**Scopes** (optional, use the package name): `keyring`, `settings`, `llm`, `errors`, `events`, `logging`, `updates`

Examples:
- `feat(updates): add GitHub Releases auto-update`
- `fix(keyring): handle missing env prefix`
- `docs: update root README`
- `test(llm): add context builder edge cases`

### Testing

Run all tests: `go test ./...`
Run a single package: `go test ./updates/`

### CI

GitHub Actions using `jrschumacher/go-actions@v3`:
- PR titles validated against conventional commit format
- Test, lint, and security checks on PRs and pushes to main
- Releases via Release Please on main
