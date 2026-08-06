# wails-kit

Reusable Go module for Wails apps. Provides a schema-driven settings framework and LLM provider management.

## Packages

### `settings` — Schema-Driven Settings

The backend defines a settings schema (fields, types, options, visibility conditions) and any frontend renders from it dynamically.

```go
import "github.com/jrschumacher/wails-kit/settings"

svc, err := settings.NewService(
    settings.WithAppName("my-app"),             // persists to the OS config dir
    settings.WithGroup(mySettingsGroup()),       // register settings groups
    settings.WithOnChange(func(v map[string]any) {
        // react to settings changes
    }),
)
if err != nil {
    // the user config dir could not be resolved, or the schema is structurally
    // broken — see "Schema validation" below
    log.Fatal(err)
}
if c := svc.Corruption(); c != nil {
    // the settings file was unparseable and has been moved aside; the app is
    // running on defaults — see "Corrupt config files" below
}

// Register as a Wails service. Bind svc.Bindings(), never svc itself —
// see "Binding to Wails" below.
app := application.New(application.Options{
    Services: []application.Service{
        application.NewService(svc.Bindings()),
    },
})
```

Settings persist to the platform's user config directory:
`~/Library/Application Support/<app>/settings.json` on macOS,
`%AppData%\<app>\settings.json` on Windows, `~/.config/<app>/settings.json` on
Linux. `settings.WithStorePath` overrides it.

**Frontend contract** (4 Wails bindings):
- `GetSchema()` — returns JSON describing all fields, types, options, conditions
- `GetValues()` — returns current values with defaults and computed fields. Password fields are **masked** — see Secrets
- `SetValues(values)` — validates, saves, triggers onChange callbacks. Returns
  `[]ValidationError` (things the user must fix) or an `error` (the save could
  not be attempted); exactly one of the two is ever non-nil
- `Corruption()` — returns the quarantine report, or `null`, so the settings
  page can render the "your settings were reset" notice itself. It carries only
  file paths and a decoding error, never secret material — see Corrupt config files

**Binding to Wails:**

Wails binds *every* exported method of a bound struct. `Service` has
`GetSecret` and `GetValuesWithSecrets` on it, so binding the `Service` directly
publishes the user's raw API keys to the webview and defeats the point of
storing them in the keyring at all.

`svc.Bindings()` returns a narrow struct exposing exactly `GetSchema`,
`GetValues`, `SetValues` and `Corruption`. Bind that; keep the `Service` in Go.

```go
application.NewService(svc.Bindings())  // correct
application.NewService(svc)             // exposes GetSecret to the frontend
```

**Defining a settings group:**

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
        },
    }
}
```

**Schema features:**
- `Field.DynamicOptions` — select options that change based on another field's value
- `Field.Condition` — show/hide field based on another field's value
- `Field.Advanced` — render behind a "Show advanced" toggle
- `Field.Validation` — required, pattern, min/max length, min/max number
- `Group.ComputeFuncs` — server-side computed readonly fields

**Conditions:**

`Condition.Equals` is `[]any`, so a field can be gated on a toggle or a number,
not only on a select:

```go
{
    Key:   "proxy.url",
    Type:  settings.FieldText,
    Label: "Proxy URL",
    Condition: &settings.Condition{
        Field:  "proxy.enabled",
        Equals: []any{true},
    },
}
```

Numbers compare numerically across every Go numeric type, so `[]any{3}` matches
`3`, `int64(3)` and `float64(3)` alike — the same schema behaves identically
whether the value came from Go or across the JSON bridge. Comparison never
crosses kinds: `"true"` does not match `true`. `[]any{nil}` means "while this
field is unset". An empty `Equals` never matches, which would hide the field
forever, so `ValidateSchema` rejects one outright.

### Schema validation

There are two kinds of validation and they have different audiences.

`ValidateSchema(schema)` checks the schema *the developer wrote* and returns a
single joined `error`. Everything it reports is a bug in the application's own
code, so the right response is to fail startup. **`NewService` calls it for
you** — call it directly only from a test that asserts a schema is well-formed
before shipping it.

`Validate(schema, values)` checks the values *the user entered* and returns a
`[]ValidationError` with one entry per problem, addressed by field key so the
frontend can render each one next to its input, or nil when everything passes.
`SetValues` runs it before writing anything.

**If you tracked this module before v0.1.0, note that `NewService` is now
materially stricter**: a sloppy schema that used to start will fail at startup
instead. That is deliberate — every one of these is a control the user would
otherwise never be able to satisfy. The checks are:

- a group or field with an empty key, a duplicate group key, or a duplicate
  field key (the store is one flat map, so two fields sharing a key silently
  share a value)
- a field with no type, or an unknown type
- a `FieldSelect` with neither `Options` nor `DynamicOptions`
- a static select whose `Default` is not one of its own options
- a `FieldPassword` with a `Default` (secrets live in the `SecretStore`, where a
  default would be indistinguishable from a value the user saved — use
  `Service.SetSecret`)
- a `Condition` whose `Field` is empty, points at the field itself, or names a
  field not declared anywhere in the schema; or one with an empty `Equals`,
  which would hide the field forever
- `DynamicOptions` with an empty `DependsOn`, one that names an undeclared
  field, or one that points at the field itself
- a `Validation.Pattern` that does not compile, `MinLen` greater than `MaxLen`,
  or `Min` greater than `Max` — ranges no value can satisfy
- a `FieldComputed` with no entry in its group's `ComputeFuncs`, a nil
  `ComputeFunc`, a compute key not declared as a field in any group, or one
  declared in a *different* group than the `ComputeFuncs` map it appears in

Every problem is reported, not just the first: the returned error joins them.

### Corrupt config files

A `settings.json` containing invalid JSON cannot be recovered by retrying, and
used to make every subsequent save fail — leaving a non-technical user with an
app that would not remember anything and a file they could not find. Instead the
damaged file is renamed to `<path>.corrupt-<timestamp>` (UTC, lexically
sortable), the service continues on schema defaults, and the app stays usable
and saveable.

Because that recovery discards the user's preferences it is reported rather than
logged and forgotten. `NewService` probes the file for exactly this reason, so
check `Corruption()` immediately afterwards:

```go
if c := svc.Corruption(); c != nil {
    ui.Warn(fmt.Sprintf(
        "Your settings could not be read and have been reset to defaults.\n"+
            "The previous file was saved as %s", c.QuarantinePath))
}
```

`CorruptConfig` carries `Path`, `QuarantinePath`, `DetectedAt` and `Reason`
(the JSON decoding error — developer-facing text, not something to show a user
on its own). It is also on `Bindings`, so a settings page can render the notice
itself. It stays accurate for the life of the service: a file corrupted while
the app is running is quarantined on the next read and reported here.

Only malformed JSON is quarantined. A file that cannot be *read* at all —
permission denied, an I/O error — still surfaces as an error from `GetValues`
and `SetValues`, because there the user's settings are intact and destroying
them would be the wrong answer.

### Secrets

Fields of type `settings.FieldPassword` are never written to `settings.json`.
They are routed to a `SecretStore`, which defaults to the OS credential store —
Keychain on macOS, Credential Manager on Windows, Secret Service over D-Bus on
Linux.

**The keyring never silently falls back to disk.** On a headless Linux box or a
minimal container the keyring is genuinely unreachable, and there every
operation fails loudly rather than quietly writing plaintext keys somewhere the
user does not expect. Opt in explicitly if that is what you want:

```go
settings.WithPlaintextFileSecrets("/path/to/secrets.json")  // 0600, PLAINTEXT
settings.WithSecretStore(settings.NewMemorySecretStore())   // tests
```

**The sentinel round-trip.** `GetValues` is the map that crosses the Wails
bridge on every settings render, so a real API key in it is one XSS away from
being exfiltrated. Instead:

- A password field with a stored secret is reported as `settings.SecretSentinel`.
- A password field with nothing stored is omitted entirely.
- `SetValues` treats `SecretSentinel` as "leave the stored secret alone", an
  empty string as "clear it", and anything else as a replacement.

So a settings page renders the sentinel into the input and posts it straight
back, and editing an unrelated field cannot clobber a stored key. The sentinel
is deliberately not a row of asterisks: a user who selected the mask, copied it
and pasted it back would silently keep their old key either way.

**Go-only accessors.** Backend code that must actually *use* a secret:

```go
key, err := svc.GetSecret("llm.anthropic.secret")   // one secret
values, err := svc.GetValuesWithSecrets()           // full map, secrets resolved

// write a secret the user never types — an OAuth token the backend obtained:
err = svc.SetSecret("llm.anthropic.secret", token)
err = svc.DeleteSecret("llm.anthropic.secret")
```

None of these is on `Bindings`, so none is reachable from the frontend. The
results of the first two must not be returned to the webview or passed to
anything that logs them. `WithOnChange` listeners also receive the masked map,
for the same reason. All four reject a key that is not a `FieldPassword`
declared in the schema.

A password field's `Default` is rejected by `ValidateSchema`: the value lives in
the `SecretStore`, where a default would be indistinguishable from a secret the
user actually saved. Use `SetSecret` to seed one. And if the secret backend
cannot be read at all, `GetValues` returns an error rather than reporting the
field as unset — rendering an empty input would tell the user their key was
never saved and invite them to overwrite one that is fine.

### `llm` — LLM Provider Management

Provider interface, factory pattern, and a built-in settings group for LLM configuration.

```go
import (
    "github.com/jrschumacher/wails-kit/llm"
    "github.com/jrschumacher/wails-kit/settings"
    _ "github.com/jrschumacher/wails-kit/llm/anthropic"
    _ "github.com/jrschumacher/wails-kit/llm/openai"
)

svc, err := settings.NewService(
    settings.WithAppName("my-app"),
    settings.WithGroup(llm.LLMSettingsGroup()),  // adds provider/model/advanced fields
)
if err != nil {
    log.Fatal(err)
}

mgr := llm.NewProviderManager(svc)

// Get the provider (lazily built from settings on first use, then cached)
provider, err := mgr.Provider()
if err != nil {
    log.Fatal(err)
}

// Stream a chat. The handler is called synchronously on this goroutine and
// never after StreamChat returns, so it needs no locking.
err = provider.StreamChat(ctx, llm.ChatRequest{
    SystemPrompt: "You are helpful.",
    Messages:     []llm.ChatMessage{{Role: llm.RoleUser, Content: "Hello"}},
}, func(event llm.StreamEvent) {
    switch event.Type {
    case llm.EventDelta:
        fmt.Print(event.Text)
    case llm.EventDone:
        fmt.Println(event.StopReason)
    }
})
if err != nil {
    log.Print(err)
}

// After settings change, reload the provider
if err := mgr.Reload(); err != nil {
    log.Print(err)
}
```

`Role`, `EventType` and `StopReason` are named types over `string`, so untyped
literals stay assignable — `ChatMessage{Role: "user"}` and `case "delta":` still
compile — while the constants give discoverable, misspelling-proof names.

Every stream ends with exactly one terminal event: `EventDone` carrying the
`StopReason` the model actually stopped for (emitted for *every* terminal
condition, not only a natural end of turn, so a consumer keyed on "done" is
never left hanging), or `EventError` carrying the same error `StreamChat`
returns. Before it, any number of `EventDelta` events and at most one
`EventToolUse` carrying fully accumulated tool calls.

**Built-in providers:**
- `llm/anthropic` — Anthropic SDK with Cloudflare AI Gateway support
- `llm/openai` — OpenAI SDK with Cloudflare AI Gateway support
- `llm/mock` — Mock provider for testing

**Registration is by blank import, deliberately.** The `llm` package contains no
provider implementations and imports no vendor SDK; each provider package
registers itself from `init()`. The set of providers a binary can build is
therefore exactly the set of provider packages it imports, so an application that
only ships Anthropic support does not link — or vendor, or audit, or carry CVE
exposure for — the OpenAI SDK, and a third-party provider is a peer of the
built-in ones rather than a fork of them.

The cost is that a missing blank import is a runtime error rather than a compile
error. `NewProvider` names the registered providers in its error message, and
`llm.RegisteredProviders()` reports them directly — useful for a diagnostics
screen, or to check a configured provider name before reaching the factory:

```go
// sorted, and exactly what this binary imported:
// []string{"anthropic", "openai"} for the imports above,
// []string{"anthropic", "mock", "openai"} in a test that also imports llm/mock
llm.RegisteredProviders()
```

**LLM settings group fields:**
- `llm.provider` — provider selection (Anthropic / OpenAI by default)
- `llm.model` — model selection, `DynamicOptions` keyed on `llm.provider`
- per-provider advanced fields, each conditioned on `llm.provider`:
  `llm.<name>.baseURL`, `llm.<name>.secret` (password), `llm.<name>.customModel`,
  and `llm.<name>.apiFormat` for providers that declare `APIFormats`
- `llm.resolvedModelID` — computed, the model ID that would actually be sent

**Customizing the group.** `LLMSettingsGroup` takes options; called with no
arguments it describes the built-in providers exactly as `BuiltinProviderSpecs()`
lists them, with Anthropic as the default provider.

```go
settings.WithGroup(llm.LLMSettingsGroup(
    // restrict the provider select, in the order given. The first surviving
    // provider supplies the default llm.provider, and its first model the
    // default llm.model.
    llm.WithProviders("anthropic"),

    // the built-in model lists are a snapshot and go stale as soon as a vendor
    // ships a new model. WithModels replaces a provider's list;
    // WithExtraModels appends to it, keeping the built-ins and the default.
    llm.WithExtraModels("anthropic",
        settings.SelectOption{Label: "Claude Opus 4.6", Value: "claude-opus-4-6"},
    ),
))
```

Options are applied in order over the built-in list, which matters in one place
only: an option editing a provider added by `WithProvider` must come after it.
`WithModels` and `WithExtraModels` do nothing for a provider that is not
configured. Filtering every provider away leaves an empty provider select, and
`settings.NewService` rejects the resulting schema.

**Adding your own provider.** A provider outside this module participates fully
with no changes to the factory: register an implementation, and add a
`ProviderSpec` to the settings group. The two halves are independent.

```go
// in your provider package
func init() {
    llm.RegisterProvider("acme",
        func(modelID string, config llm.ProviderConfig) llm.Provider {
            return newAcmeProvider(modelID, config)  // your llm.Provider
        },
        // Optional. A TransportResolver lets a provider send its traffic over
        // another registered provider's wire protocol. It is handed the full
        // values map, its own key prefix ("llm.acme.") and the model ID
        // resolved so far, and returns the transport to build and the model ID
        // to give it. Returning the name unchanged means "no redirection".
        llm.WithTransportResolver(
            func(values map[string]any, keyPrefix, modelID string) (string, string) {
                if f, _ := values[keyPrefix+"apiFormat"].(string); f == llm.APIFormatOpenAICompatible {
                    return "openai", "acme/" + modelID
                }
                return "acme", modelID
            },
        ),
    )
}
```

```go
// in your app
settings.WithGroup(llm.LLMSettingsGroup(
    llm.WithProvider(llm.ProviderSpec{
        Name:  "acme",  // must match the RegisterProvider name
        Label: "Acme",
        Models: []settings.SelectOption{
            {Label: "Acme Large", Value: "acme-large"},
        },
        APIFormats: []settings.SelectOption{  // optional; adds llm.acme.apiFormat
            {Label: "Native", Value: "acme-native"},
            {Label: "OpenAI Compatible", Value: llm.APIFormatOpenAICompatible},
        },
    }),
))
```

`WithProvider` replaces an existing settings entry of the same name in place,
keeping its position, and `RegisterProvider` likewise replaces an existing
registration — so an app can substitute its own implementation for a built-in by
registering after that package's `init` has run. (`RegisterProvider` panics on an
empty name or a nil factory, both of which would otherwise fail far from the
mistake.) A provider with no models is legal — it still gets a Custom Model ID
field, so every model is typed by hand.

`llm/anthropic` registers a `TransportResolver` of exactly this shape;
that is how selecting the OpenAI-compatible API format routes Anthropic models
through the OpenAI transport, with no knowledge of Anthropic in `llm` itself.

The API key fields are `FieldPassword`, so they live in the keyring and
`GetValues` masks them. `ProviderManager` reads `GetValuesWithSecrets`
internally and resolves the real key. If you build a provider yourself, feed
`llm.NewProviderFromValues` a map from `GetValuesWithSecrets` — a masked map is
rejected with an error rather than producing a provider that authenticates with
the sentinel.

### `llm/mock` — Test Helper

Useful two ways. Constructed directly it is a hand-configured stub; the zero
value streams one `mock.DefaultResponse` delta followed by a terminal done
event, and `OnStreamChat` replaces that script entirely:

```go
import "github.com/jrschumacher/wails-kit/llm/mock"

p := &mock.Provider{
    Name:  "test",
    Model: "test-model",
    OnStreamChat: func(ctx context.Context, req llm.ChatRequest, handler func(llm.StreamEvent)) error {
        handler(llm.StreamEvent{Type: llm.EventDelta, Text: "test response"})
        handler(llm.StreamEvent{Type: llm.EventDone, StopReason: llm.StopReasonEndTurn})
        return nil
    },
}
```

Imported for effect it registers itself with the factory registry exactly as
`llm/anthropic` and `llm/openai` do, so the whole settings-to-provider path can
be exercised end to end without a network or the OS keyring:

```go
import _ "github.com/jrschumacher/wails-kit/llm/mock"

p, err := llm.NewProviderFromValues(map[string]any{"llm.provider": mock.ProviderName})
```

Registration happens in `init`, so it only affects binaries that import the
package — which for a test-only provider means test binaries.

## Frontend Integration

The schema returned by `GetSchema()` is framework-agnostic JSON. Any frontend renders it with a generic loop:

```
for each group in schema.groups:
  render group heading
  for each field in group.fields:
    if field.advanced: group behind toggle
    if field.condition: check values[condition.field] in condition.equals
    if field.dynamicOptions: lookup options[values[dependsOn]]
    render input for field.type (text/password/select/toggle/number/computed)
```

`condition.equals` is a JSON array of literals — `{"field": "proxy.enabled",
"equals": [true]}` — not an array of strings. Compare with the field's value
without coercing types.

Password fields need no special handling: whatever `GetValues` returned goes
into the input and back out through `SetValues` unchanged. A field carrying the
sentinel round-trips as "unchanged"; a field the user actually edited carries
their new value; clearing the input clears the stored secret.

`Corruption()` returns `null` in the normal case and a `{path, quarantinePath,
detectedAt, reason}` object when the settings file had to be moved aside. Render
it as a dismissible notice on the settings page — `reason` is developer-facing
text and belongs in a details pane, not in the headline.

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `ANTHROPIC_API_KEY` | Anthropic API key (SDK fallback when no secret is in settings) |
| `OPENAI_API_KEY` | OpenAI API key (SDK fallback when no secret is in settings) |
| `CF_AIG_AUTHORIZATION` | Cloudflare AI Gateway token |

Auth precedence is identical in both providers: a configured API key always wins
over the SDK's environment fallback. If no API key is configured but
`CF_AIG_AUTHORIZATION` is set, the gateway supplies the provider credential and
the environment fallback is suppressed for that client — the environment
variable itself is never unset, since that is a process-wide side effect. The
gateway header is applied whenever `CF_AIG_AUTHORIZATION` is set, independently
of provider auth.
