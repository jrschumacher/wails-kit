# @wails-kit/types

TypeScript types for wails-kit's Wails v3 bridge — the wire shapes that
cross from Go to the webview via bound service methods and `events:on`
payloads. No runtime logic beyond a handful of event-name string constants;
almost everything here is `interface`/`type` declarations.

## Install

```bash
npm install @wails-kit/types
```

## What's covered

| File                | Go source                                              | Covers |
|---------------------|---------------------------------------------------------|--------|
| `settings.ts`       | `settings/schema.go` (`ResolvedSchema` family)           | Settings schema: `Schema`, `Group`, `Field`, `SelectOption`, `DynamicOptions`, `Condition`, `Validation` |
| `health.ts`         | `health/health.go`, `health/binding.go`                  | `Snapshot`, `CheckStatus`, `HealthClass`, `HealthState` — **see the header comment**, this one has a known wire-format quirk |
| `appearance.ts`     | `appearance/appearance.go`                                | `AppearanceMode`, `Theme`, `AppearanceChangedPayload` |
| `i18n.ts`           | `i18n/localizer.go`, `i18n/catalog.go`                    | `Catalog`, `CatalogEntry`, `CatalogPluralEntry`, `I18nChangedPayload` |
| `permissions.ts`    | `permissions/permissions.go`                              | `PermissionKind`, `PermissionStatus`, `PermissionsChangedPayload` |
| `firstrun.ts`       | `firstrun/firstrun.go`                                    | `FirstrunKind`, `FirstrunTransitionPayload` |
| `windowstate.ts`    | `windowstate/windowstate.go`                               | `Geometry`, `WindowstateRestoredPayload`, `WindowstateErrorPayload` |
| `diagnostics.ts`    | `diagnostics/diagnostics.go`                                | `SystemInfo`, `DiagnosticsBundleCreatedPayload`, `DiagnosticsBundleSubmittedPayload` |
| `events.ts`         | every package's `Event*` constant + `*Payload` struct       | Event name constants, payload interfaces, and the combined `EventMap` |
| `errors.ts`         | `errors/errors.go`, `updates/service.go`                    | `ErrorCode` (a curated subset — see below), `UserError` |

## Usage

```ts
import type { Schema, CheckStatus } from "@wails-kit/types";
import { HealthChanged, UpdateAvailable, type EventMap } from "@wails-kit/types";

// Typed event subscription, e.g. wrapping wailsjs/runtime's EventsOn:
function on<K extends keyof EventMap>(name: K, handler: (payload: EventMap[K]) => void) {
  EventsOn(name, handler);
}

on(HealthChanged, ({ check, overall }) => { /* check: CheckStatus */ });
```

## `ErrorCode` is a curated subset, not every kit error code

`ErrorCode` lists the core `errors` package codes plus `updates`' codes
(`update_verify`, `update_managed`, ...) — the ones a generic frontend is
expected to branch on (e.g. driving a "reinstall" or "use your package
manager" message). Every other kit package registers its own error codes
(`health_duplicate_check`, `appearance_invalid_mode`,
`permissions_unsupported`, `firstrun_downgrade_refused`,
`windowstate_save`, `diagnostics_consent_required`, `state_load`, ...) and
none of those are enumerated here — `UserError.code` on the Go side is an
open `type Code string`, not a closed set, and most of those errors are
handled by displaying `UserError.userMsg` (already localized) rather than
branching on the code. If your app needs to branch on a specific
non-core/non-updates code, treat `ErrorCode` as non-exhaustive and widen
the type at the call site rather than waiting for it to be added here.

## Known wire-format quirk: `health.Snapshot` / `health.CheckStatus`

Every other resolved type in this package (`settings.ts`'s `ResolvedSchema`
family, `appearance.ChangedPayload`, `permissions.ChangedPayload`, ...) is
backed by a Go struct with explicit lowerCamelCase `json:"..."` tags. As of
this writing, `health.Snapshot` and `health.CheckStatus`
(`health/health.go`) carry **no** JSON tags at all, so Go's default
marshaling uses the exact Go field names — `Overall`, `Offline`, `Checks`,
`Name`, `Class`, `State`, `Critical`, `Err`, `CheckedAt`, `Latency` — not
the kit's usual camelCase convention. `Snapshot` and `CheckStatus` in
`health.ts` intentionally match those real wire bytes (verified against
`encoding/json` output, not guessed), so the types stay honest about what
actually crosses the bridge today. `health/health.go` is outside this
package's ownership; fixing it (adding `json:"..."` tags) is a breaking
wire-format change that must land together with an update to `health.ts`.

## Design

- **Type-only where possible** — the only runtime values exported are
  event-name string constants (`SettingsChanged`, `HealthChanged`, ...).
- **`"types"` resolves to source (`index.ts`), not `dist/`** — this lets
  monorepo-internal consumers (`@wails-kit/settings`, `@wails-kit/i18n` via
  a `file:` dependency) typecheck without requiring this package to be
  built first, and gives real npm consumers accurate, always-in-sync
  types (TypeScript is happy to use a `.ts` file directly as a `types`
  target). `"main"`/`"module"` point at the compiled `dist/index.js` for
  the runtime event-name constants.
- **Relative imports carry an explicit `.js` extension** (e.g.
  `from "./events.js"`) even though the source files are `.ts` — required
  for the compiled output to resolve correctly under plain Node ESM, and
  understood identically by TypeScript's `bundler`/`nodenext` module
  resolution when reading the `.ts` source directly.
- **Kept in sync with Go via `frontend/types_test.go`** — a Go test in the
  parent `frontend` package that checks these interfaces against the real
  Go structs (via reflection over JSON tags, or — for `health.Snapshot`/
  `CheckStatus` specifically — against the real `encoding/json.Marshal`
  output). Run `go test ./frontend/...` after changing either side.
