# settings

Schema-driven settings framework for Wails v3 apps. The backend defines a settings schema (fields, types, options, visibility conditions) and any frontend renders from it dynamically.

## Usage

```go
import (
    "github.com/jrschumacher/wails-kit/settings"
    "github.com/jrschumacher/wails-kit/keyring"
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

Register as a Wails service to expose `GetSchema`, `GetValues`, and `SetValues` as frontend bindings.

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

This overrides the default OS path. All the same behaviors apply: atomic writes, schema migration, file permissions. Password fields are always stored in the OS keyring — never written to the workspace file.

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

- `GetValues()` returns `"••••••••"` for set passwords, `""` for unset
- `SetValues()` with `"••••••••"` is a no-op (user didn't change it)
- `SetValues()` with `""` clears the secret from keyring
- `GetSecret(key)` returns the actual value (backend use only)

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

### Back-end checks

Some values can only be checked by the back end: a token against its service, a path on disk. Give the field a `Check` (the words shown while it runs) and its group a `CheckFuncs` entry:

```go
settings.Group{
    Key: "account",
    Fields: []settings.Field{
        {
            Key:        "account.token",
            Type:       settings.FieldPassword,
            Label:      "API token",
            Validation: &settings.Validation{Required: true},
            Check:      &settings.Check{Pending: "Checking token…"},
        },
    },
    CheckFuncs: map[string]settings.CheckFunc{
        "account.token": func(ctx context.Context, value any, values map[string]any) error {
            if err := client.Verify(ctx, value.(string)); err != nil {
                return errors.New(ErrTokenRejected, "verify token", err) // its registered message shows under the field
            }
            return nil
        },
    },
}
```

A `CheckFunc` must honour `ctx`: `CheckValue` stops waiting when the timeout ends, but a check that ignores `ctx` keeps its goroutine (and whatever it holds) running until it returns.

The settings window calls the `CheckValue(key, values)` binding for such a field instead of applying it: when the person leaves the field, and again 500 ms after they stop typing once it shows an error. `values` holds the window's values: they are context for the check and the field's condition, and only `values[key]` is ever saved. `CheckValue`:

- runs the field's own validation first, and returns its errors without calling the check;
- runs the check under its own timeout (`WithCheckTimeout`, default 2 min), **not** the caller's context, so it carries on when Settings closes before it returns;
- on a pass, saves `values[key]` alone (other fields failing their validation don't stop it) and returns no errors: no errors means the value was saved. An empty value, and a password's `SecretMask`, pass without a check, so clearing a field saves the empty value and drops its failure;
- on a failure, saves nothing, records the failure (`CheckFailures()`), emits `settings:check_failed`, and returns a `ValidationError` with code `check`. The message is the error's `*errors.UserError` user message, else `ErrCheckFailed`'s; a raw error never reaches the person;
- when the key changed while the check ran (a newer `CheckValue` for it, including one whose value failed validation, or a `SetValues` that changed it), saves, records and emits nothing, and returns a `check` error: "This setting changed while it was being checked. Enter it again to check it." Each key has a generation, guarded by the service's lock, that both bump; the save happens under that lock only if the generation is still the check's own.

`CheckFailures()` returns the failures that haven't passed since, in schema order. A `SetValues` that changes the key drops its failure too. When Settings opens, pass them in as its errors and open it on the first one (`<wk-settings>`'s `reveal(key)`).

| Option | |
|---|---|
| `WithEmitter(*events.Emitter)` | emitter for `settings:check_failed` |
| `WithCheckTimeout(d)` | how long a check may run (default `DefaultCheckTimeout`, 2 min) |

**Event** `settings:check_failed` (`EventCheckFailed`), payload `CheckFailedPayload`:

| Field | |
|---|---|
| `key`, `label` | the field |
| `message` | its error, as Settings shows it |
| `notice` | the main window's words: "Your API token wasn't saved." |
| `attention` | true when the field is required: the app can't work without it, so the Notice takes the attention tone; neutral otherwise |

The main window shows a Notice with `notice` and an "Open Settings" action when Settings isn't open; Settings opens on that field with its error.

**Error codes**

| Code | Message |
|---|---|
| `ErrCheckFailed` (`settings_check_failed`) | "Couldn't confirm this value. Check it and enter it again." (a check returned an error that isn't a `*errors.UserError`) |
| `ErrCheckTimeout` (`settings_check_timeout`) | "The check timed out. Enter the value again once the service can be reached." |
| `ErrUnknownSetting` (`settings_unknown_setting`) | "This setting doesn't exist." (returned by `CheckValue` for a key the schema lacks) |

Validation codes: `CodeCheck` (`check`) joins `required`, `pattern`, `min_length`, `max_length`, `min`, `max`, `invalid_type` and `invalid_option`.

### Advanced fields

Fields marked `Advanced: true` should be rendered behind a "Show advanced" toggle in the frontend.

## Behaviors

- **Atomic writes** — writes to `.tmp` then renames, preventing corruption on crash
- **Schema migration** — unknown keys in saved files are stripped on load
- **File permissions** — directories `0700`, settings file `0600`
- **Defaults** — schema-defined defaults are applied when a key has no saved value

## Frontend contract

The frontend calls these Wails bindings:

1. **`GetSchema()`** — returns JSON describing all fields, types, options, conditions
2. **`GetValues()`** — returns current values with defaults and computed fields
3. **`SetValues(values)`** — validates, saves, triggers `onChange` callbacks
4. **`CheckValue(key, values)`** — for a field with a `Check`: checks it with the back end, then saves that one value (see [Back-end checks](#back-end-checks))
5. **`CheckFailures()`** — the back-end checks that failed, for Settings to show when it opens

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
