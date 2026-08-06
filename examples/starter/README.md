# starter

A minimal but real Wails v3 application wiring together the three pieces a new
wails-kit project needs:

- `settings.NewService` — schema-driven settings with keyring-backed secrets
- `llm.LLMSettingsGroup()` — provider and model selection, contributed to that
  same schema
- `app.Updater.Init` — Wails v3's own auto-updater

## Running it

```bash
cd examples/starter
go mod download
go run .
```

Or with the Wails CLI, for hot reload and packaging:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.4
wails3 dev
```

Try, in order:

1. Open **LLM** → pick a provider, paste an API key, **Save settings**. Reload:
   the key field shows the sentinel, not your key. The real value went to the OS
   keyring.
2. Press **LLM status**. It resolves the provider from settings, builds a real
   client, and reports `provider / model` — never the key.
3. Clear the **Display name** field and save. The service rejects the whole
   submission and nothing is written.
4. Press **Check for updates**. It fails, loudly and correctly: the checked-in
   public key is a placeholder and the repository does not exist. Failing closed
   is the intended behaviour.

## This example is its own Go module

`go.mod` here declares `github.com/jrschumacher/wails-kit/examples/starter` and
requires Wails. **The wails-kit root module does not depend on Wails, and must
not.** That is the whole reason this directory is a separate module: a consumer
running `go get github.com/jrschumacher/wails-kit` gets the settings and LLM
packages without a beta desktop framework in their dependency graph.

To confirm the invariant still holds, from the repo root:

```bash
go list -m all | grep wailsapp   # must print nothing
```

## What each file demonstrates

### `main.go` — wiring

- **`settings.NewService` error handling.** It fails when the config directory
  cannot be resolved or the schema is malformed. Both are fatal and both belong
  at startup, not at first use.
- **`svc.Corruption()`.** Not an error — the service works fine — but it means
  the user's saved settings were unreadable and have been reset to defaults, so
  it is reported rather than swallowed. A real app surfaces this in the UI.
- **Binding `svc.Bindings()`, never `svc`.** Wails binds *every* exported method
  of a bound type. `settings.Service` exposes `GetSecret` and
  `GetValuesWithSecrets`, which return real API keys — binding it would publish
  both to the webview and undo the entire point of the keyring. `Bindings`
  exposes exactly `GetSchema`, `GetValues`, `SetValues`, `Corruption`. **This
  example is what people copy, so it models the safe form.**
- **Blank imports for provider registration.** `llm/anthropic` and `llm/openai`
  register themselves from `init`. Without the imports the settings UI still
  lists both providers, but `NewProviderFromValues` cannot build either.
- **`settings.WithOnChange` rebuilding the LLM provider.** The manager is
  declared before `NewService` and captured by the closure, because the service
  needs the hook and the hook needs the manager.
- **`app.Updater.Init`.** Configured from the update settings group. `Init` may
  be called once per process, so channel and auto-check changes take effect on
  next launch.

### `updatesettings.go` — settings ↔ updater composition

An ordinary `settings.Group` for update preferences: an auto-check toggle, a
channel select, and computed current-version / last-checked fields.

Note what is **not** there: a "Check Now" button. `settings.FieldType` is
exactly `text`, `password`, `select`, `toggle`, `computed`, `number` — every one
a value the service persists. Actions are not settings, so the button lives in
the app's own UI and calls `AppService.CheckForUpdates`.

Also note `update.lastChecked`: computed fields are never persisted (`SetValues`
strips and recomputes them), so it is backed by an in-memory `atomic.Int64` and
resets to "Never" on each launch. Persisting it would need a real writable
field, which the user would then see as an editable control.

### `appservice.go` — the app's own bound API

Same rule as `settings.Service`: every exported method is published to the
webview, so nothing here may return a secret. It holds a `*settings.Service` and
a `*llm.ProviderManager` — both of which *can* reach real API keys — but the
fields are unexported and no method hands them out. `LLMStatus` returns provider
and model names only.

### `assets/index.html` — the frontend

Deliberately minimal: one hand-written file, no build step, no npm. The thing
being demonstrated is the wiring, not the UI.

It renders the schema returned by `GetSchema` — honouring `condition`
(show-when), `dynamicOptions` (options that depend on another field), computed
fields as read-only, and password fields as the sentinel round-trip — then posts
back through `SetValues` and displays any `ValidationError`s by field.

Methods are called by fully-qualified name via
`Call.ByName('<package path>.<Type>.<Method>')`, which needs no code generation.
A real app would run `wails3 generate bindings` and get typed wrappers instead.

### `update-public-key.pem` — the trust anchor

A real, well-formed ed25519 public key whose private half was never saved. The
example therefore **fails closed**: nothing can produce a signature it accepts.

Generate your own before shipping anything:

```bash
openssl genpkey -algorithm ed25519 -out update-private-key.pem
openssl pkey -in update-private-key.pem -pubout -out update-public-key.pem
```

Then back the private key up offline and never commit it. Losing it, or shipping
a public key that does not match it, permanently strands every installed copy of
your app with no remote fix. See [`docs/auto-update.md`](../../docs/auto-update.md).

## Not included

This is a wiring example, not a project template.

- No `build/` directory, `Taskfile.yml` or `Info.plist` — run `wails3 init` for
  those. `docs/auto-update.md` §6 covers the version fields you must keep in
  sync once you have them.
- No release workflow — `docs/auto-update.md` §7 has one, including the macOS
  codesigning and notarization steps.
- No generated bindings — see above.
