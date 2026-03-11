# CLAUDE.md

## Project

wails-kit is a reusable Go module for Wails v3 desktop apps. It provides infrastructure packages that apps import as a library.

## Philosophy

wails-kit provides **desktop app infrastructure** — the plumbing that every Wails app needs but shouldn't rewrite. Each package must meet these criteria:

1. **Wails-specific or desktop-specific** — solves a problem unique to desktop apps or Wails integration. Generic Go libraries (AI SDKs, HTTP clients, data processing) belong in standalone repos.
2. **Reduces real boilerplate** — eliminates >50 lines of repeated setup that multiple apps would otherwise copy-paste. A 30-line convenience wrapper doesn't justify a kit package.
3. **Infrastructure, not business logic** — provides foundational services (storage, config, lifecycle, OS integration), not application-level features (chat UI, domain models, workflows).

**Ask before adding:** "Would a Wails app author write this themselves, and would it look roughly the same every time?" If yes, it belongs in the kit. If the implementation varies significantly per app, it belongs in the app.

## Structure

Go module at `github.com/jrschumacher/wails-kit` with these packages:

- `appdirs` — OS-standard application directory paths
- `database` — SQLite database management with goose migrations
- `diagnostics` — Support bundle creation for crash reporting
- `keyring` — OS keyring credential storage
- `settings` — Schema-driven settings framework
- `errors` — User-facing error types
- `events` — Typed event emission
- `logging` — Structured logging with rotation
- `updates` — GitHub Releases-based auto-updates
- `lifecycle` — Service lifecycle manager with dependency ordering
- `shortcuts` — Native menu shortcuts and keyboard accelerators

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

**Scopes** (optional, use the package name): `appdirs`, `database`, `diagnostics`, `keyring`, `settings`, `state`, `errors`, `events`, `lifecycle`, `logging`, `shortcuts`, `updates`

Examples:
- `feat(updates): add GitHub Releases auto-update`
- `fix(keyring): handle missing env prefix`
- `docs: update root README`
- `test(settings): add validation edge cases`

### Adding a new package

1. Create the package directory with implementation and tests
2. Follow the patterns: functional options, optional emitter/settings, error codes with `init()` registration
3. Keep dependencies minimal: prefer stdlib, avoid adding external deps for <200 lines of logic
4. Add a package `README.md` documenting usage, options, events, and error codes — this is the single source of truth for the package
5. Update `README.md` (root) with a one-line summary linking to the package README (no code examples — keep the root README minimal to avoid merge conflicts)
6. Update `docs/architecture.md` with the new package's position in the dependency graph
7. Add the package name as a scope in the **Scopes** list above

### Pre-commit checks

Before committing, always run lint and tests locally to catch issues before CI:

```sh
task check
```

This runs `golangci-lint run ./...` and `go test ./...`. Both must pass before committing and pushing.

### Testing

Run all tests: `go test ./...` or `task test`
Run a single package: `go test ./updates/`

### CI

GitHub Actions using `jrschumacher/go-actions@v3`:
- PR titles validated against conventional commit format
- Test, lint, and security checks on PRs and pushes to main
- Releases via Release Please on main
