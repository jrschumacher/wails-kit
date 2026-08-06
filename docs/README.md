# wails-kit documentation

Guides for building a Wails v3 desktop application on top of `wails-kit`.

## Guides

| Document | What it covers |
|---|---|
| [`auto-update.md`](auto-update.md) | Wiring Wails v3's built-in `app.Updater`: provider config, ed25519 signing, appcast XML, the GitHub Actions release + notarization workflow, platform limits, and the failure modes that are silent. |

## Examples

| Example | What it demonstrates |
|---|---|
| [`../examples/starter/`](../examples/starter/) | A minimal but real Wails v3 app wiring `settings.NewService`, `llm.LLMSettingsGroup()` and `app.Updater.Init` together. Its own Go module. |

## Package reference

The packages document themselves. Start with the package comments:

- `settings` — schema-driven settings, keyring-backed secrets
  (`settings/doc.go`)
- `llm` — LLM provider management, settings-driven
  (`llm/doc.go`)

```
go doc github.com/jrschumacher/wails-kit/settings
go doc github.com/jrschumacher/wails-kit/llm
```

## Invariants

These hold across the repository. Breaking one is a bug, not a preference.

1. **The root module has no Wails dependency.** `go list -m all` in the repo
   root must return zero `wailsapp` lines. Anything needing Wails belongs in
   `examples/`, which is a separate module.
2. **Bind `svc.Bindings()`, never `settings.Service`.** Wails binds every
   exported method of a bound type, and the Service exposes `GetSecret` and
   `GetValuesWithSecrets`. `Bindings` exposes exactly `GetSchema`, `GetValues`,
   `SetValues` and `Corruption`.
3. **Real secrets never cross the bridge.** `GetValues` substitutes
   `settings.SecretSentinel` for stored password fields; only Go-side code calls
   `GetValuesWithSecrets` or `GetSecret`.
4. **Check `Service.Corruption()` after `NewService`.** A non-nil result means
   the user's settings file was unreadable and was quarantined and reset. It is
   not an error, but it must never be silent.
