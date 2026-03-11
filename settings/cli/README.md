# settings/cli

Headless/CLI adapter for the settings package. Uses the same schema that drives the Wails frontend to provide terminal-based settings management.

## Usage

```go
import (
    "github.com/jrschumacher/wails-kit/settings"
    settingscli "github.com/jrschumacher/wails-kit/settings/cli"
)

svc := settings.NewService(
    settings.WithAppName("my-app"),
    settings.WithGroup(mySettingsGroup()),
)
```

### Interactive configuration

Walks through all schema fields with prompts. Conditions, dynamic options, and validation all apply. Press Enter to keep the current value.

```go
settingscli.Configure(svc)
```

### Set a single value

```go
settingscli.Set(svc, "llm.provider", "anthropic")
```

Values are coerced to the field's type: `"true"`/`"false"` for toggles, numbers for number fields.

### Show current values

Prints all settings grouped by section. Passwords are masked, conditional fields that don't apply are hidden.

```go
settingscli.Show(svc)
```

Output:

```
[Appearance]
  theme = dark (Dark)
  font_size = 14

[Auth]
  api_key = ••••••••
```

### Edit in $EDITOR

Opens settings as JSON in `$EDITOR` (falls back to `vi`). Computed and password fields are excluded from the editable file.

```go
settingscli.Edit(svc)
```

### List all keys

```go
keys := settingscli.Keys(svc)  // sorted alphabetically
```

## Options

All functions accept options for custom I/O:

```go
settingscli.Show(svc,
    settingscli.WithInput(os.Stdin),
    settingscli.WithOutput(os.Stdout),
)
```

## Validation

All schema validation rules (required, pattern, min/max, allowed options) are enforced. Validation failures return a `*cli.ValidationErrors` error:

```go
err := settingscli.Set(svc, "theme", "invalid")
if ve, ok := err.(*settingscli.ValidationErrors); ok {
    for _, e := range ve.Errors {
        fmt.Printf("%s: %s\n", e.Field, e.Message)
    }
}
```

## Interface

The adapter works with any type implementing `cli.SettingsProvider`:

```go
type SettingsProvider interface {
    GetSchema() settings.Schema
    GetValues() (map[string]any, error)
    SetValues(values map[string]any) ([]settings.ValidationError, error)
}
```

This is satisfied by `*settings.Service`.
