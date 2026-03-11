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

### Advanced fields

Fields marked `Advanced: true` should be rendered behind a "Show advanced" toggle in the frontend.

## Behaviors

- **Atomic writes** — writes to `.tmp` then renames, preventing corruption on crash
- **Schema migration** — unknown keys in saved files are stripped on load
- **File permissions** — directories `0700`, settings file `0600`
- **Defaults** — schema-defined defaults are applied when a key has no saved value

## Frontend contract

The frontend calls three Wails bindings:

1. **`GetSchema()`** — returns JSON describing all fields, types, options, conditions
2. **`GetValues()`** — returns current values with defaults and computed fields
3. **`SetValues(values)`** — validates, saves, triggers `onChange` callbacks

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
