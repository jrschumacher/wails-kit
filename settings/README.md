# settings

Schema-driven settings framework for Wails v3 apps. The backend defines a settings schema (fields, types, options, visibility conditions) and any frontend renders from it dynamically.

## Usage

```go
import (
    "github.com/jrschumacher/wails-kit/v2/settings"
    "github.com/jrschumacher/wails-kit/v2/keyring"
)

store := keyring.NewOSStore("my-app", keyring.WithEnvPrefix("MYAPP"))

svc := settings.NewService(
    settings.WithAppName("my-app"),
    settings.WithKeyring(store),
    settings.WithGroup(mySettingsGroup()),
    settings.WithOnChange(func(v map[string]any) {
        // react to settings changes
    }),
)
```

## Registering with Wails — use `Binding()`, never the `Service`

Wails v3 binds **every exported method** of a registered service. `*settings.Service`
exports `GetSecret`, which returns the raw, unmasked value of a password field —
registering `svc` directly publishes every stored API key to frontend JS and defeats
`SecretMask` entirely.

Register `svc.Binding()` instead. `*settings.Binding` exposes exactly `GetSchema`,
`GetValues`, and `SetValues` — the frontend-safe surface — and nothing else:

```go
// Correct — the frontend only ever sees GetSchema/GetValues/SetValues.
app.RegisterService(application.NewService(svc.Binding()))

// WRONG — do not do this. Every exported *Service method becomes callable
// from the webview, including GetSecret.
app.RegisterService(application.NewService(svc))
```

`Service.GetSecret` remains available for backend Go code (e.g. constructing an API
client with the real key). It is not, and must never become, part of `Binding`'s method
set — a reflection test (`TestBindingSurface`) pins that surface so a future change that
widens `Binding` fails a test instead of shipping a leak.

## Storage paths

By default, settings are stored in OS-standard locations using `os.UserConfigDir()`:

| OS | Path |
|----|------|
| macOS | `~/Library/Application Support/{app}/settings.json` |
| Linux | `$XDG_CONFIG_HOME/{app}/settings.json` |
| Windows | `%AppData%/{app}/settings.json` |

### Workspace-local storage

For apps that need settings to live inside a workspace directory (e.g., git-tracked project configs), use `WithStoragePath`:

```go
svc := settings.NewService(
    settings.WithStoragePath(filepath.Join(workspaceDir, "config.json")),
    settings.WithKeyring(store),
    settings.WithGroup(mySettingsGroup()),
)
```

This overrides the default OS path. All the same behaviors apply: durable atomic writes, schema migration, file permissions. Password fields are always stored in the OS keyring — never written to the workspace file.

`WithAppName` and `WithStoragePath` compose regardless of call order — `WithStoragePath`
always wins if both are given, whether it comes first or last. Only the storage *path*
resolution is order-independent this way; other options (`WithGroup`, `WithOnChange`)
are simply additive.

### The default keyring is in-memory — configure one explicitly

If `WithKeyring` is omitted, secret/password fields fall back to an in-memory store:
they work for the life of the process and are **silently lost on restart**. `NewService`
logs a warning (`slog.Warn`) when this happens, because losing a user's saved API keys
on every relaunch is a bad enough surprise that it must never be silent. Pass a
persistent `keyring.Store` — `keyring.NewOSStore(...)` or `keyring.NewEnvelopeStore(...)`
— in any app that isn't a short-lived test or example.

**Git usage notes:**

- Track the settings file if you want config to be portable across clones
- Add it to `.gitignore` if settings are machine-specific
- Password fields are safe either way — they never appear in the JSON file

## Defining groups and fields

```go
func mySettingsGroup() settings.Group {
    return settings.Group{
        Key:   "appearance",
        Label: "Appearance",
        Fields: []settings.Field{
            {
                Key:     "appearance.theme",
                Type:    settings.FieldSelect,
                Label:   "Theme",
                Default: "system",
                Options: []settings.SelectOption{
                    {Label: "System", Value: "system"},
                    {Label: "Light", Value: "light"},
                    {Label: "Dark", Value: "dark"},
                },
            },
            {
                Key:     "appearance.font_size",
                Type:    settings.FieldNumber,
                Label:   "Font Size",
                Default: 14,
                Validation: &settings.Validation{Min: intPtr(8), Max: intPtr(32)},
            },
        },
    }
}
```

## Field types

| Type | Constant | Description |
|------|----------|-------------|
| Text | `FieldText` | Single-line text input |
| Password | `FieldPassword` | Stored in OS keyring, masked in `GetValues` |
| Select | `FieldSelect` | Dropdown with static or dynamic options |
| Toggle | `FieldToggle` | Boolean on/off |
| Number | `FieldNumber` | Numeric input with optional min/max |
| Computed | `FieldComputed` | Read-only, derived from other values server-side |

## Password fields

Password fields are stored in the OS keyring, never in the JSON settings file.

- `GetValues()` returns `"••••••••"` (`settings.SecretMask`) for set passwords, `""` for unset
- `SetValues()` with `settings.SecretMask` is a no-op (user didn't change it)
- `SetValues()` with `""` clears the secret from keyring
- `SetValues()` with any non-string value (e.g. a stray number) is rejected with a
  `ValidationError` (`Code: settings.CodeInvalidType`) — it never falls through to being
  coerced to `""` and deleting the stored secret
- `GetSecret(key)` returns the actual value — available only on `*Service`, never on
  `*Binding` (see [Registering with Wails](#registering-with-wails--use-binding-never-the-service))

## Schema features

### Dynamic options

Select options that change based on another field's value:

```go
settings.Field{
    Key:  "appearance.font_size",
    Type: settings.FieldSelect,
    DynamicOptions: &settings.DynamicOptions{
        DependsOn: "appearance.theme",
        Options: map[string][]settings.SelectOption{
            "compact": {{Label: "Small", Value: "12"}, {Label: "Medium", Value: "14"}},
            "default": {{Label: "Medium", Value: "14"}, {Label: "Large", Value: "16"}},
        },
    },
}
```

### Conditional visibility

Show/hide a field based on another field's value:

```go
settings.Field{
    Key:       "proxy.url",
    Condition: &settings.Condition{Field: "network.proxy_enabled", Equals: []string{"true"}},
}
```

### Validation

```go
settings.Field{
    Validation: &settings.Validation{
        Required: true,
        Pattern:  "^sk-",       // regex
        MinLen:   10,
        MaxLen:   200,
        Min:      intPtr(0),    // for number fields
        Max:      intPtr(100),
    },
}
```

**Validation runs against effective state, not the raw payload.** `SetValues` merges
defaults → persisted values → this submission before validating, then persists only the
submitted (non-secret) keys. This matters for `Condition` and `DynamicOptions`: if a
caller submits `{"api_key": ""}` without resubmitting `provider`, validation still sees
whatever `provider` is *currently* set to (persisted or default) when deciding whether
`api_key`'s condition is met — a partial update can't sneak an invalid or missing
required value past a condition simply by omitting the controlling field.

### Computed fields

Server-side computed read-only fields:

```go
settings.Group{
    Fields: []settings.Field{
        {Key: "resolved_model", Type: settings.FieldComputed, Label: "Resolved Model"},
    },
    ComputeFuncs: map[string]settings.ComputeFunc{
        "resolved_model": func(values map[string]any) any {
            return values["appearance.font_size"]
        },
    },
}
```

### Advanced fields

Fields marked `Advanced: true` should be rendered behind a "Show advanced" toggle in the frontend.

## Behaviors

- **Durable atomic writes** — writes to a temp file in the same directory, `fsync`s it,
  closes it, and only then renames it over the target. A rename that lands before the
  data is flushed can leave a truncated or zero-length file after a crash; `Save` never
  renames without a successful `Sync()` first (mirrors `keyring.EnvelopeStore`'s
  `writeTempLocked`).
- **Schema migration** — unknown keys in saved files are stripped on load
- **File permissions** — directories `0700`, settings file `0600`
- **Defaults** — schema-defined defaults are applied when a key has no saved value
- **Effective-state validation** — see [Validation](#validation) above
- **`onChange` runs outside the lock** — callbacks are invoked after `SetValues`
  releases its internal mutex, so a callback that calls back into the `Service` (e.g.
  `GetValues` or another `SetValues`) never deadlocks

## Frontend contract

Register `svc.Binding()` with Wails (see
[Registering with Wails](#registering-with-wails--use-binding-never-the-service)). The
frontend calls its three methods:

1. **`GetSchema()`** — returns JSON describing all fields, types, options, conditions
2. **`GetValues()`** — returns current values with defaults and computed fields, secrets masked
3. **`SetValues(values)`** — validates against effective state, saves, triggers `onChange` callbacks

Render the schema with a generic loop:

```
for each group in schema.groups:
  render group heading
  for each field in group.fields:
    if field.advanced: group behind toggle
    if field.condition: check values[condition.field] in condition.equals
    if field.dynamicOptions: lookup options[values[dependsOn]]
    render input for field.type
```
