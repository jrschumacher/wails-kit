# @wails-kit/i18n

Headless TypeScript client for wails-kit's i18n bridge (`i18n/localizer.go`,
`i18n/binding.go`): catalog consumption, CLDR plural resolution via
`Intl.PluralRules`, and an `i18n:changed` refetch helper. Framework-agnostic
— no React/Vue/Svelte dependency, and no `wailsjs/runtime` dependency
either (see "Design" below).

## Install

```bash
npm install @wails-kit/i18n
```

## Usage

### Stateful client (the common case)

```ts
import { createI18nClient, T } from "@wails-kit/i18n";
import { EventsOn, EventsOff } from "../wailsjs/runtime/runtime";
import type * as Binding from "../wailsjs/go/main/I18nBinding"; // or wherever your i18n.Binding is generated

const i18n = createI18nClient({
  fetchCatalog: Binding.GetCatalog,
  fetchLocale: Binding.GetLocale,
  subscribe: (eventName, handler) => {
    EventsOn(eventName, handler);
    return () => EventsOff(eventName);
  },
});

await i18n.ready; // first catalog + locale fetch

const greeting = T("myapp.greeting", "Hello, %s!");
i18n.t(greeting, "Ada"); // -> resolved string, catalog-first, Text.other as fallback

const itemCount = T("myapp.items", "%d items");
i18n.tn(itemCount, 5); // -> CLDR-plural-resolved for i18n.locale()

// Later, e.g. on component teardown:
i18n.destroy();
```

`subscribe` is optional — omit it and call `await i18n.refresh()` yourself
(e.g. after your own locale-change flow) instead of wiring an event bus.

### Pure functions (bring your own state)

If you already have the catalog and locale in your own reactive store (a
Svelte store, a Zustand slice, a React context), skip the client and call
the resolution functions directly:

```ts
import { resolveText, resolvePlural, T } from "@wails-kit/i18n";
import type { Catalog } from "@wails-kit/types";

declare const catalog: Catalog;
declare const locale: string;

resolveText(catalog, T("myapp.greeting", "Hello, %s!"), "Ada");
resolvePlural(catalog, T("myapp.items", "%d items"), 5, locale);
```

## API

| Export | Signature | Notes |
|---|---|---|
| `T(key, other)` | `(key: string, other: string) => Text` | Mirrors Go's `i18n.T` |
| `format(template, args)` | `(template: string, args: readonly unknown[]) => string` | Minimal `fmt.Sprintf` stand-in: `%s`, `%d`, `%f`, `%v`, `%%` |
| `resolveText(catalog, text, ...args)` | `(catalog: Catalog, text: Text, ...args: unknown[]) => string` | Mirrors `Localizer.T` |
| `resolvePlural(catalog, text, n, locale, ...args)` | `(catalog, text, n: number, locale: string, ...args: unknown[]) => string` | Mirrors `Localizer.TN` — **`n` is always the first `format` argument**, then `...args`, so a plural template's first verb should consume the count (e.g. `"%d files in %s"`, not `"%s has %d files"`) |
| `createI18nClient(options)` | see `client.ts` | Stateful catalog cache + optional event-bus subscription |

## Design

- **Catalog-first, `Text.other` as the offline fallback** — every resolver
  takes a `Catalog` (the already-locale-resolved map from
  `i18n.Binding.GetCatalog`) and a `Text` (`{ key, other }`). A missing key
  falls back to `Text.other`, never throws — matching the Go
  `Localizer.T`/`TN` "no localizer/catalog, still sensible English"
  guarantee.
- **`Intl.PluralRules`, not a hand-rolled CLDR table** — plural category
  selection for `resolvePlural` runs entirely client-side against the
  browser/Node's built-in CLDR data, mirroring how the Go side uses
  `golang.org/x/text/feature/plural` rather than hand-rolled rules.
- **No `wailsjs/runtime` dependency** — like `@wails-kit/settings`, this
  package is headless. `createI18nClient`'s `subscribe` option is how you
  hand it your app's actual event transport (typically a thin wrapper
  around `EventsOn`/`EventsOff`); the package itself never imports
  `wailsjs/*`.
- **`"types"` resolves to source (`index.ts`), not `dist/`** — see
  `@wails-kit/types`'s README for the rationale (unblocks monorepo-internal
  typecheck without requiring a prior build).
