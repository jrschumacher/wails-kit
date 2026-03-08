# Settings integration

This document describes how packages integrate with the `settings` package to provide user-configurable behavior.

## Overview

The settings system has two sides:

1. **Schema definition** — packages export a `SettingsGroup()` function that returns field definitions
2. **Value consumption** — packages read settings values at call time via `*settings.Service`

## Defining a settings group

Each package that needs user configuration exports a `SettingsGroup()`:

```go
package updates

import "github.com/jrschumacher/wails-kit/settings"

const (
    SettingCheckFrequency     = "updates.check_frequency"
    SettingAutoDownload       = "updates.auto_download"
    SettingIncludePrereleases = "updates.include_prereleases"
)

func SettingsGroup() settings.Group {
    return settings.Group{
        Key:   "updates",
        Label: "Updates",
        Fields: []settings.Field{
            {
                Key:     SettingCheckFrequency,
                Type:    settings.FieldSelect,
                Label:   "Check for updates",
                Default: "daily",
                Options: []settings.SelectOption{
                    {Label: "On startup", Value: "startup"},
                    {Label: "Daily", Value: "daily"},
                    {Label: "Weekly", Value: "weekly"},
                    {Label: "Never", Value: "never"},
                },
            },
            // ...
        },
    }
}
```

### Key naming convention

Setting keys use `{package}.{field}` format:

- `updates.check_frequency`
- `updates.auto_download`
- `llm.provider`
- `llm.model`
- `llm.anthropic.baseURL`

This prevents collisions when multiple packages register groups.

## Consuming settings

Packages accept `*settings.Service` via an optional `WithSettings` option:

```go
func WithSettings(svc *settings.Service) ServiceOption {
    return func(s *Service) {
        s.settings = svc
    }
}
```

Values are read at call time, not cached:

```go
func (s *Service) CheckForUpdate(ctx context.Context) (*Release, error) {
    includePre := s.includePrereleases // static fallback
    if s.settings != nil {
        if values, err := s.settings.GetValues(); err == nil {
            if v, ok := values[SettingIncludePrereleases].(bool); ok {
                includePre = v
            }
        }
    }
    // ...
}
```

This ensures changes to settings take effect immediately without needing a reload mechanism.

## App-owned vs library-owned settings

Some settings are read by the library (e.g., `include_prereleases` affects which GitHub API endpoint is called). Others are purely for the app to read and act on:

| Setting | Owner | Why |
|---------|-------|-----|
| `updates.include_prereleases` | Library | Changes API behavior |
| `updates.check_frequency` | App | Library doesn't poll; app decides when to check |
| `updates.auto_download` | App | Library doesn't auto-download; app decides |
| `llm.provider` | Library | Determines which provider factory to use |

The library provides the settings fields for all of these (so they appear in the UI), but only reads the ones it owns. The app reads the rest.

## Frontend rendering

Settings groups are rendered by the frontend from the schema. The frontend doesn't need to know which package defined a group — it just renders the fields:

```
for each group in schema.groups:
  render group heading (group.label)
  for each field in group.fields:
    if field.condition: check visibility
    if field.advanced: hide behind toggle
    render appropriate input for field.type
```

## Existing settings groups

| Package | Group key | Fields |
|---------|-----------|--------|
| `llm` | `llm` | provider, model, per-provider API config |
| `updates` | `updates` | check_frequency, auto_download, include_prereleases |
