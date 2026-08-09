# i18n

Go-owned translation catalog for wails-kit apps. Not just error strings and not just
the GUI: Prune's Cobra CLI and Bubbletea TUI need localized output too, and a
frontend-only catalog can't reach either of them — see
[`docs/v2-roadmap.md`](../docs/v2-roadmap.md) AD-5 for the full rationale. CLDR plural
rules come from `golang.org/x/text` — this package never hand-rolls `%d items`-style
pluralization, which is simply wrong for most of the world's languages.

## Usage

```go
import "github.com/jrschumacher/wails-kit/v2/i18n"

//go:embed locales/*.json
var appCatalog embed.FS

var Greeting = i18n.T("myapp.example.greeting", "Hello, %s!")

l, err := i18n.New(i18n.WithCatalog(appCatalog))
if err != nil {
    log.Fatal(err)
}

fmt.Println(l.T(Greeting, "Ryan")) // "Hello, Ryan!" (or catalog translation)
```

A `Text` is a stable catalog key plus the built-in (English/source-language) fallback,
declared inline at the call site so code stays readable without a catalog lookup —
`i18n.T("myapp.example.greeting", "Hello, %s!")`. Kit packages use keys namespaced
`wailskit.<package>.<ident>`; apps use whatever namespace they like (not `wailskit.`,
which is reserved and lint-checked — see Extraction lint below).

## Catalog format

One JSON file per locale, at `locales/<bcp47>.json` in whatever `fs.FS` you pass to
`WithCatalog` (an `embed.FS` in the common case). Each value is either a plain string
or a plural object:

```json
{
  "myapp.example.greeting": "Hello, %s!",
  "myapp.example.file_count": {
    "one": "%d file",
    "other": "%d files"
  }
}
```

Plural objects use CLDR category names: `zero`, `one`, `two`, `few`, `many`, `other`.
`other` is required; the rest are whatever the target language's cardinal rule
actually uses (English only ever needs `one`/`other`; Polish needs `one`/`few`/`many`/
`other`; Arabic needs all six). Messages use `fmt`-style verbs (`%s`, `%d`, ...).

Multiple `WithCatalog` sources merge in the order given, per key per locale — **later
wins**. The kit's own embedded catalog is always merged first, so an app's own
`WithCatalog` overrides a kit string on key collision without needing to know which
kit package owns it.

## Plain vs. plural

```go
l.T(Greeting, "Ryan")              // fmt.Sprintf if args given; the literal string if not
l.TN(FileCount, n)                 // n is always the first fmt arg — "%d file(s)"
l.TN(FileCount, n, extraArg)       // n first, then any additional args
```

`T` with zero args returns the resolved string unchanged, rather than always running
it through `fmt.Sprintf` — a message containing a literal `%` (e.g. "100% off") is
never misinterpreted as a broken verb.

## Locale resolution

Highest wins ([OQ-5](../docs/v2-roadmap.md), decided):

1. explicit `WithLocale` option
2. settings override, if wired via `WithSettings` (skipped when the value is
   `"system"`, the locale picker's default — meaning "keep resolving lower tiers")
3. `LC_ALL` / `LC_MESSAGES` / `LANG`
4. OS source — `defaults read -g AppleLanguages` on darwin (see Landmines); on
   Linux/Windows the env vars above already *are* the OS source, so this tier is a
   no-op there today (`GetUserDefaultLocaleName` on Windows is a documented,
   not-yet-built follow-up)
5. configured default (`WithDefault`, else English)

Resolved **once**, at `New`, and cached — nothing here re-execs or re-reads settings
on every lookup. `SetLocale(tag)` is the only way to change it afterward; it takes the
same precedence an explicit `WithLocale` would have, and emits `i18n:changed`
(`EventChanged`) via the optional `*events.Emitter` from `WithEmitter` — but only when
the locale actually changes.

A resolved locale doesn't need a catalog of its own: `T`/`TN` look up a key against
the resolved locale, then its ancestor locales (`fr-CA` → `fr`), then the configured
default, before finally falling back to the `Text`'s own built-in string. Missing a
whole locale's catalog degrades gracefully; missing a single key degrades gracefully
too.

## Frontend hydration

`Localizer.Catalog()` returns the merged, locale-resolved catalog as `map[string]any`
— plain strings stay plain strings, plural entries come back as
`map[string]string` keyed by CLDR category, so `@wails-kit/i18n` (TypeScript) can
resolve plurals client-side with `Intl.PluralRules` rather than needing Go for every
render.

## Registering with Wails — use `Binding()`

Same pattern `settings` uses ([`settings/README.md`](../settings/README.md)): register
`l.Binding()` with Wails, not `*Localizer` directly, so the registered surface stays
exactly `GetCatalog`, `GetLocale`, `SetLocale` regardless of what backend-only methods
`Localizer` grows later.

## Settings integration

`l.LocaleOptions()` returns the raw data for a locale picker: `"system"` (default)
first, then every locale present in `l`'s merged catalog, each labeled with an
English display name via `golang.org/x/text/language/display`.
[`settings.LocaleGroup(l)`](../settings/README.md#locale-picker) turns that into a
registerable `settings.Group` with one select field. Register it with your
`settings.Service` the same way you'd register any other group; wire the same service
back into `i18n.New` via `WithSettings` to close the loop (persisted choice →
resolution tier 2 → picker reflects it on next launch).

This package used to build the `settings.Group` itself (`Localizer.SettingsGroup()`,
WP-10). WP-12 moved that assembly into package `settings`: once `settings.Field`
carries `i18n.Text` and `settings.Service` accepts a `*Localizer`
(`settings.WithLocalizer`), `settings` imports `i18n` — so `i18n` can no longer import
`settings` too without an import cycle. `LocaleOptions()` is the cycle-free split:
this package owns catalog-derived data, `settings` owns assembling it into its own
types.

## Extraction lint

`go test ./i18n/ -run TestCatalogKeysExist` walks the whole repository for
`i18n.T("wailskit.…", …)` call sites and verifies each such key exists in the
`locales/en.json` of the package directory that declares it. It's how kit packages
stay honest about their own catalogs as they add strings over time; it does not (and
cannot, without knowing an app's own catalog layout) check app-defined keys. Wired
into repo-wide CI separately.

## Events

| Event | Payload | When |
|---|---|---|
| `i18n:changed` | `ChangedPayload{Locale string}` | `SetLocale` resolves to a different locale than before |

## Errors

`New` fails only on a malformed catalog (bad JSON, or a plural object missing the
required `"other"` form, or with an unrecognized category name) — never on an
unresolvable locale, which always falls through to the configured default instead.
`SetLocale` fails only if given a string that doesn't parse as a BCP-47 tag at all.
