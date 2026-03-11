# settings/cli

Headless/CLI adapter for the settings package. Uses the same schema that drives the Wails frontend to provide non-interactive settings management for CI/scripting.

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

### Get a single value

```go
val, err := settingscli.Get(svc, "llm.provider")
// val = "anthropic (Anthropic)"
```

Password fields are returned masked. Unknown keys return an error.

### Set a single value

```go
settingscli.Set(svc, "llm.provider", "anthropic")
```

Values are coerced to the field's type: `"true"`/`"false"` for toggles, numbers for number fields. All schema validation (required, pattern, min/max, allowed options) is enforced.

### Show all values

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

## Options

`Show` accepts an output option:

```go
settingscli.Show(svc, settingscli.WithOutput(os.Stderr))
```

## Validation

Validation failures return a `*cli.ValidationErrors` error:

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
