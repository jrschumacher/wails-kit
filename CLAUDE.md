# wails-kit — agent instructions

Go toolkit for Wails v3 desktop apps. Module: `github.com/jrschumacher/wails-kit`.

Two packages:

- `settings/` — schema-driven settings with keyring-backed secrets
- `llm/` — LLM provider management driven by those settings

## Read before writing code

- [`docs/`](docs/) — guides. Start at [`docs/README.md`](docs/README.md).
- [`docs/auto-update.md`](docs/auto-update.md) — how a consuming app wires
  Wails v3's `app.Updater`. The kit ships **no** update code; this is the
  deliverable.
- [`examples/starter/`](examples/starter/) — a working app wiring settings, LLM
  and the updater together. Copy from here.
- Package comments (`settings/doc.go`, `llm/doc.go`) are the API reference.

## Invariants — do not break these

1. **The root module must not depend on `github.com/wailsapp/wails/v3`.**
   `go list -m all` in the repo root must return zero `wailsapp` lines. Anything
   needing Wails goes in `examples/`, which is a separate module with its own
   `go.mod`. This is deliberate: consumers get the kit without a beta desktop
   framework in their dependency graph.
2. **Bind `svc.Bindings()`, never `settings.Service`.** Wails binds every
   exported method of a bound type, and the Service exposes `GetSecret` and
   `GetValuesWithSecrets`.
3. **Real secrets never cross the bridge.** `GetValues` masks password fields
   with `settings.SecretSentinel`. Only Go-side code calls
   `GetValuesWithSecrets` or `GetSecret`.
4. **`Service.Corruption()` must be checked after `NewService`** and surfaced to
   the user. It means their settings file was unreadable and was reset.

## Conventions

- Conventional commits (`feat:`, `fix:`, `docs:`, `chore:`).
- `go test ./...` must stay green before any commit.
- `.golangci.yml` is authoritative for lint.
