# settings/cli

Headless/CLI adapter for the settings package. Uses the same schema that drives the Wails frontend to provide non-interactive settings management for CI/scripting.

## Usage

```go
import (
    "github.com/jrschumacher/wails-kit/v2/settings"
    settingscli "github.com/jrschumacher/wails-kit/v2/settings/cli"
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

`Show`, `Get`, and `Set` all accept the same options:

```go
settingscli.Show(svc, settingscli.WithOutput(os.Stderr))
```

### Localization

`WithLocalizer` resolves this package's own human-facing strings — the
`"(not set)"` placeholder and error text from `Show`/`Get`/`Set` — through an
`*i18n.Localizer`:

```go
l, _ := i18n.New(i18n.WithLocale("fr"), i18n.WithCatalog(appCatalog))
settingscli.Show(svc, settingscli.WithLocalizer(l))
```

Without it, every string falls back to the English baked into this package,
same as `permissions.WithLocalizer` and `settings.WithLocalizer`.

Schema labels (`[General]`, field labels in `Get`'s select-with-label output)
are **not** localized by this option — they arrive already resolved on
`SettingsProvider.GetSchema()`'s `ResolvedSchema`, via the localizer wired
into the underlying `*settings.Service` with `settings.WithLocalizer`
(a separate, independent localizer — the two don't have to be the same
instance, though in practice they usually are).

`WithLocalizer` also never touches the machine-readable value tokens this
package emits and parses: `formatValue`'s `"true"`/`"false"` toggle output
and default number/string formatting, and `coerceValue`'s accepted input
alphabet (`"true"`/`"1"`/`"yes"`/`"y"`/`"on"` and their false-ish
counterparts). Those stay fixed across every locale so `Get`'s output can
round-trip back through `Set`, and so an app building its own
machine-readable (e.g. `--json`) mode on top of `SettingsProvider.GetValues`
never has to worry about locale-dependent formatting from this package.

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
    GetSchema() settings.ResolvedSchema
    GetValues() (map[string]any, error)
    SetValues(values map[string]any) ([]settings.ValidationError, error)
}
```

This is satisfied by `*settings.Service`.
