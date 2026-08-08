# wails-kit v2 — Roadmap and Implementation Plan

Status: proposed. Audience: coding agents executing work packages, plus one senior Go
engineer (Ryan) reviewing. Backwards compatibility with wails-kit v1.x or with Prune's
current integration is **explicitly not required**. This is a clean break.

Every claim about Wails v3 in this document was verified against the module cache:
`v3.0.0-alpha.64` (current pin) and `v3.0.0-beta.4`, both at
`/Users/ryan/go/pkg/mod/github.com/wailsapp/wails/`.

---

## 1. Vision and scope

### What v2 is

wails-kit v2 is the infrastructure layer that lets one developer stand up a new Wails v3
desktop app in under an hour, with secure defaults on by default (opt-out, not opt-in).
Two reference consumers define the envelope:

1. **Prune** (repo `aboldnewlook/workctl`) — a task/work-log app with a Cobra CLI, a
   Bubbletea TUI, and a Wails GUI sharing one Go core. It needs settings, secrets, i18n,
   appearance, LLM config, and eventually a background runner. Its flat files live in git
   and get read by LLMs, so anything the kit persists into a Prune workspace must be
   genuinely human-readable.
2. **A plain "reskinned web app" shell** — a webview around a hosted product. It needs
   window state, appearance, updates, deep links, crash reporting, and backend-health
   checks. It never needs LLM config or a job queue.

Both must be first-class. Any design that only works for one is wrong.

### What v2 deliberately isn't

- Not a UI framework. The kit ships schemas, catalogs, snapshots, and events; frontends
  render them. The only shipped frontend code is headless TypeScript (types + logic).
- Not a durable-execution engine. The runner is a durable *queue* with worker ticking
  (Watermill-inspired), not Temporal. Desktop apps don't need replayable workflow
  histories.
- Not a general Go utility library. The inclusion criteria below still gate everything.
- Not multi-app orchestration, telemetry pipelines, or licensing/payments.

### Amended philosophy (replaces the "Philosophy" sections of CLAUDE.md and README.md)

> wails-kit provides **desktop app infrastructure** — the plumbing that every Wails app
> needs but shouldn't rewrite. Each package must meet these criteria:
>
> 1. **Desktop-specific or Wails-specific** — solves a problem unique to desktop apps or
>    Wails integration. Generic Go libraries (HTTP clients, data processing) belong in
>    standalone repos.
> 2. **Reduces real boilerplate** — eliminates >50 lines of repeated setup that multiple
>    apps would otherwise copy-paste.
> 3. **Infrastructure, not business logic** — foundational services (storage, config,
>    lifecycle, OS integration), not application features (chat UI, domain models).
>
> **The honest carve-out:** `settings/templates/anyllm` is the one deliberate exception
> to rule 1. LLM provider configuration is not desktop-specific, but wiring provider /
> model / API-key fields into the settings schema is boilerplate every LLM-enabled app of
> ours repeats identically, and the secret-handling half of it *is* desktop-specific
> (OS keyring). It lives in the repo as a **nested Go module** so its SDK dependency
> (`any-llm-go`) is never paid by apps that don't import it. We name the exception rather
> than pretending the criteria have no exceptions. Any future carve-out must be argued
> the same way: named, justified, isolated in its own module.
>
> **Ask before adding:** "Would a Wails app author write this themselves, and would it
> look roughly the same every time?" If yes, it belongs in the kit.

---

## 2. Architecture decisions

### AD-1: Bootstrap is a wired-components constructor, not a lifecycle-owning composition root

**The open question, decided.** Two candidate shapes:

- **(a) Composition root**: `kit.Run(...)` builds and owns the `*application.App`, runs
  the event loop, returns when the app exits.
- **(b) Convenience constructor**: `kit.New(...)` returns a `*kit.Kit` of wired
  components (dirs, logging, settings, keyring, events, health, appearance, updates,
  lifecycle, i18n, …); the developer creates their own `application.App` and attaches the
  kit to it with one call.

**Decision: (b), a two-phase constructor — `kit.New` (Wails-free) then
`wailsbridge.Attach` (Wails glue).**

Rationale, in order of weight:

1. **Prune is a three-headed app.** `prune` the CLI and `prune tui` need the exact same
   wired spine — appdirs, settings, keyring/envelope, logging, i18n, errors — with *no*
   `application.App` in the process. A composition root that returns a configured
   `*application.App` cannot serve two of Prune's three entry points. The web-shell app
   doesn't force this, but Prune does, and the kit must serve both.
2. **Wails v3 is still a beta-moving target.** If the kit owns `application.New`, it must
   either proxy every `application.Options` field (a wrapper that chases upstream churn
   every beta) or accept the options struct and become a thin pass-through that owns
   nothing of value. Attaching to a developer-owned app means Wails API churn lands in
   exactly one adapter package.
3. **Escape hatches stay free.** Custom asset middleware, multiple windows, systray-only
   modes, single-instance handling — all remain plain Wails code the developer writes,
   with the kit indifferent.
4. **The speed goal is met by the template, not the constructor.** The composition root's
   only real advantage is fewer lines in `main.go`. The `wails3` template (WP-32) ships
   that `main.go` — roughly 40 lines the developer never types. We get composition-root
   ergonomics without composition-root ownership.

The tradeoff accepted: the developer *can* mis-wire things a composition root would have
made impossible (e.g., forgetting `Attach`). Mitigation: `Attach` is one call, the
template always includes it, and `kit.Kit.Start` fails loudly if lifecycle-critical
services are missing their backends.

Shape (signatures are the contract; bodies are indicative):

```go
// package kit — imports NO wails code.
type AppInfo struct {
    Name    string // e.g. "prune" — drives appdirs, keyring service name, log paths
    ID      string // bundle id, e.g. "com.aboldnewlook.prune"
    Version string // semver, injected at build time
}

type Kit struct {
    Info       AppInfo
    Dirs       *appdirs.Dirs
    Logger     *slog.Logger
    Events     *events.Emitter
    Keyring    keyring.Store        // EnvelopeStore over OS keyring by default
    Settings   *settings.Service
    I18n       *i18n.Localizer
    Appearance *appearance.Service
    Health     *health.Registry
    Updates    *updates.Service     // nil when WithoutUpdates()
    FirstRun   *firstrun.Service
    Diag       *diagnostics.Service
    Lifecycle  *lifecycle.Manager
}

func New(info AppInfo, opts ...Option) (*Kit, error)
func (k *Kit) Start(ctx context.Context) error // lifecycle.Startup in dependency order
func (k *Kit) Close() error                    // lifecycle.Shutdown, reverse order

// Options are opt-OUT and additive:
func WithoutUpdates() Option
func WithoutHealth() Option
func WithoutDiagnostics() Option
func WithSettingsGroup(g settings.Group) Option
func WithGitHubRepo(owner, repo string) Option        // enables updates
func WithHealthCheck(c health.Check) Option
func WithLocales(fsys fs.FS) Option
func WithFirstRunHook(h firstrun.Hook) Option
func WithKeyring(store keyring.Store) Option          // override envelope default
```

```go
// package wailsbridge (dir kit/wailsbridge) — the ONE bootstrap package importing wails/v3.
func Attach(k *kit.Kit, app *application.App, opts ...Option) error
// Attach wires: events backend -> app.EmitEvent; appearance source -> ThemeChanged
// events + app.Env.IsDarkMode(); shortcuts menu -> app.SetMenu; registers binding
// services (settings.Binding, i18n.Binding, health.Binding, updates, diagnostics,
// permissions) via app-side registration.

func ManageWindow(k *kit.Kit, win *application.WebviewWindow, opts ...windowstate.Option) error
func Services(k *kit.Kit) []application.Service // for apps that prefer Options.Services
```

CLI/TUI entry points call `kit.New` + `kit.Start` and never import `wailsbridge`.

### AD-2: v2 targets Wails `v3.0.0-beta.4`

The kit currently pins `alpha.64`; upstream is at `beta.4` (both in the local module
cache). v2 pins **beta.4** and tracks betas thereafter.

Verified migration cost — low. Every API surface the kit touches exists in beta.4:

- `Menu.AddRole` (`pkg/application/menu.go:108`) — `shortcuts` compiles unchanged.
- `WebviewWindow.Position/SetPosition/Size/SetSize/IsMinimised/IsMaximised/IsFullscreen`
  (`webview_window.go:413,714,730,738,758,1046,1058`) and `GetScreen()` (`:1354`).
- `App.Screen *ScreenManager` with `GetAll/GetPrimary/GetByID`
  (`application.go:358`, `screenmanager.go:399–424`) — needed by `windowstate`.
- `events.Common.ThemeChanged`, per-platform mapping to
  `AppleInterfaceThemeChangedNotification` / `SystemThemeChanged` (dbus on Linux —
  `application_linux_dbus.go`, new since alpha.64), `ApplicationEventContext.IsDarkMode()`,
  `EnvironmentManager.IsDarkMode()` — needed by `appearance`.
- `pkg/services/notifications` with `CheckNotificationAuthorization` /
  `RequestNotificationAuthorization` — **corrected during WP-01 execution**: the
  bindable surface is on the exported `*notifications.NotificationService` at
  `pkg/services/notifications/notifications.go:263,267`. The previously cited
  `notifications_darwin.go:90,108` are methods on the *unexported* `darwinNotifier`
  and are not reachable by a consumer. WP-21 must target the exported service. — needed
  by `permissions`.
- `SingleInstanceOptions` and `events.Common.ApplicationOpenedWithFile` — deep-link /
  open-with plumbing for the template.

beta.4 also adds autostart and a global-shortcut manager; both are candidate template
options, neither is required by any v2 package. The bump is one `go.mod` edit plus a full
`go build ./... && go test ./...`; it lands first (WP-01) so everything else builds
against the real target.

### AD-3: Module structure — one module, one nested module, split-publishing dies

- **Root module path becomes `github.com/jrschumacher/wails-kit/v2`** (current tags are
  v1.3.0, so Go semantic-import-versioning requires the `/v2` suffix). Every import in
  the repo is rewritten once in WP-01. This is also the mechanical marker of the clean
  break: v1 consumers cannot accidentally pick up v2.
- **`settings/templates/anyllm` becomes a nested module** with its own `go.mod`
  (`module github.com/jrschumacher/wails-kit/settings/templates/anyllm/v2`), tagged
  `settings/templates/anyllm/v2.x.y`. It is the only package allowed to depend on
  `any-llm-go`. A committed `go.work` at the repo root keeps local dev seamless.
- **The pure schema half of LLM settings moves into the main module** as
  `settings/templates/llmconfig` (no SDK imports — just `settings.Group` construction and
  value readers). The nested `anyllm` module depends on `llmconfig` and adds client
  construction + model enumeration. See WP-07.
- **The `abnl.dev` split-module scheme is removed**: delete `split-modules.json`,
  `.github/scripts/publish-split-modules.sh`, the CI publish step, and the vanity-URL
  README section. It is broken today (TLS cert on `abnl.dev`) and, more importantly,
  unnecessary: Go module pruning already keeps unimported packages' deps out of consumer
  builds (measured: importing 7 kit packages adds only 4 modules to a consumer). The
  one dependency heavy enough to matter (`any-llm-go` and its transitive SDK zoo —
  anthropic, openai, genai, grpc, ollama…) is exactly what the nested module isolates.
  With that isolated, `wails/v3` itself is the only big remaining `require`, and every
  consumer is a Wails app anyway.

### AD-4: Where the Wails dependency may appear

Today only `shortcuts` imports `wails/v3`, hidden behind an undocumented
`(darwin||linux||windows) && wails` build tag (`shortcuts/apply.go:1`) — a known defect:
`Apply` silently doesn't exist unless you know the magic tag.

**v2 policy: no build tags. A package either may import `wails/v3` plainly or must not
import it at all.**

| May import `wails/v3` | Must stay Wails-free |
|---|---|
| `shortcuts`, `windowstate`, `permissions`, `kit/wailsbridge` | `appdirs`, `keyring`, `settings` (+`cli`, `templates/*`), `database`, `state`, `errors`, `events`, `logging`, `lifecycle`, `updates`, `diagnostics`, `i18n`, `appearance`, `health`, `firstrun`, `runner`, `semver`, `kit` |

Rationale: the left column's entire purpose is Wails integration; wrapping those behind
interfaces would be abstraction theater. The right column must run in Prune's CLI/TUI
processes (or is pure logic), so a Wails import there is a build-graph bug. Packages that
need OS/GUI signals but live on the right (e.g. `appearance` needs the OS theme) define a
small source interface and receive the Wails-backed implementation from `wailsbridge`.
`events` keeps its `Backend` interface — that insulation survives and is the model.

Enforcement is mechanical and part of CI (WP-01):

```sh
# every package outside the allowlist must not import wails
go list -deps -json ./... | jq -r 'select(.ImportPath|startswith("github.com/jrschumacher/wails-kit")) | select(.Imports[]?|startswith("github.com/wailsapp")) | .ImportPath' \
  | grep -vE '/(shortcuts|windowstate|permissions|kit/wailsbridge)$' && exit 1 || true
```

### AD-5: i18n — Go owns the catalog; everything hydrates from it

Decisive reason (settled): Prune's CLI and TUI cannot be served by a frontend-only
catalog. Scope is wider than error messages — the settings schema's `Label`/
`Description`/`Placeholder` fields are likely the largest body of translatable strings,
plus `settings/cli` output, permission prompt copy, update/menu strings.

Design (package `i18n`, full spec in WP-10):

```go
// Text is a translatable string: a stable key plus the built-in (English) fallback.
// It is declared inline at the call/definition site so code stays readable and the
// catalog can be extracted mechanically.
type Text struct {
    Key   string // "wailskit.settings.appearance.theme.label" | app-defined keys
    Other string // fallback / source-language text
}
func T(key, other string) Text // sugar

type Localizer struct{ /* ... */ }
func New(opts ...Option) (*Localizer, error)
func WithCatalog(fsys fs.FS) Option          // locales/<bcp47>.json, mergeable; later wins
func WithLocale(tag string) Option           // default: OS locale, fallback chain via x/text/language
func (l *Localizer) Locale() string
func (l *Localizer) SetLocale(tag string) error         // emits i18n:changed
func (l *Localizer) T(t Text, args ...any) string        // fmt-style args
func (l *Localizer) TN(t Text, n int, args ...any) string // CLDR plurals via golang.org/x/text
func (l *Localizer) Catalog() map[string]any             // resolved strings + plural-form objects, for hydration
```

How it threads through the kit:

- **`errors`**: `RegisterMessages` changes to `map[Code]i18n.Text`; `UserError` stores the
  `Text`, and `GetUserMessage(err)` resolves through a package-level localizer installed
  via `errors.SetLocalizer(*i18n.Localizer)` (nil localizer → `Text.Other`). Errors are
  frequently created in `init()` before any localizer exists, so resolution happens at
  *read* time, not construction time.
- **`settings`**: `Field.Label/Description/Placeholder` and `Group.Label` change from
  `string` to `i18n.Text` (and `SelectOption.Label`). `GetSchema()` takes the service's
  localizer and returns a **resolved** schema (plain strings) so no frontend change in
  rendering logic is needed; the frontend refetches on `i18n:changed`.
- **Frontend**: an `i18n.Binding` Wails service exposes `GetCatalog/GetLocale/SetLocale`;
  `@wails-kit/i18n` (TS) resolves plurals client-side with `Intl.PluralRules`.
- **CLI/TUI**: consume `Localizer` directly.

Because this changes the public shape of `errors` and `settings`, i18n lands **before**
those APIs are declared frozen (see §7 sequencing — this is why i18n moved up relative to
the discussed order).

Kit message keys are namespaced `wailskit.<package>.<ident>`. Kit packages embed their
own `locales/en.json`; the localizer merges kit catalogs with app catalogs (app wins on
key collision). v2 ships `en` only, plus the full machinery; that is enough to prove the
plumbing without inventing translations nobody reviews.

### AD-6: Agent files are per-package `AGENTS.md`

Every package ships an `AGENTS.md` (defined precisely in §5). Why this form and not the
alternatives:

- **Claude Code skill**: harness-specific; Ryan's global instructions explicitly target
  multiple agents (Claude Code, Codex, Copilot, Cursor, Pi). `AGENTS.md` is the emerging
  cross-tool convention and is auto-discovered by several harnesses.
- **Structured frontmatter / JSON manifest**: optimizes for tooling that doesn't exist;
  agents read prose+signatures better than schemas, and the file must stay pleasant for
  the human too.
- **Fatter README**: READMEs are user documentation (how to *use* the package). The agent
  file is contributor documentation (how to *change* it safely: invariants, landmines,
  test doubles, file map). Mixing them bloats both audiences' signal.

### AD-7: Prune's `upgrade-wails-kit` branch is **kept**

The branch (worktree at `/Users/ryan/Projects/aboldnewlook/workctl-upgrade-wails-kit`,
pinned to kit `v1.3.0`) already adopted `shortcuts`, `appdirs`, `logging`, and `errors`.
Those four APIs survive into v2 with changes that are mechanical from the consumer side:
import path gains `/v2`; `shortcuts` *loses* the build tag (a simplification for the
branch); `errors.RegisterMessages` calls gain `i18n.T(...)` wrappers; `appdirs`/`logging`
are unchanged. Discarding the branch would throw away real integration work to avoid a
sed pass. Decision: keep it, rebase onto Prune main, switch to a local `replace`
directive at integration checkpoint IC-1, and update call sites incrementally per
checkpoint. Nothing in it becomes load-bearing for the kit's design — if a checkpoint
shows an adoption pattern fighting the v2 API, the v2 API wins and the branch adapts.

### AD-8: `keyring.EnvelopeStore` ships as-is; the migration follow-up is dropped

The uncommitted `keyring/envelope.go` + `envelope_test.go` are part of v2's starting
state: XChaCha20-Poly1305 envelope encryption, wrapping key in the OS keyring, entries in
`secrets.json` under `dirs.Config()`, strict refuse-don't-reset failure policy, key
rotation. It becomes the **default** secrets backend wired by `kit.New` (one keychain
prompt per app instead of N; no size limits). The previously planned
migration-from-per-item-keychain follow-up is dropped — no v1 installs need migrating
under the clean-break constraint. WP-02 commits it, documents it, and adds the one gap
(`Keys()` exists on `EnvelopeStore` but not on the `Store` interface — it stays off the
interface; callers that need enumeration type-assert an optional `KeyLister`).

### AD-9: How packages compose (unchanged pattern, restated as law)

Functional options; concrete service structs; optional `*events.Emitter` (nil → drop);
optional settings integration via exported `SettingsGroup()` + read-at-call-time;
`errors.Code` + `RegisterMessages` in `init()`; stdlib-first (<200 LOC rule). New in v2:
optional `*i18n.Localizer` via `WithLocalizer`, and every package that polls or probes
must register with `health` rather than embedding its own connectivity logic.

---

## 3. Package inventory

Disposition of every existing package, and every new one, against the inclusion criteria.

### Existing

| Package | Disposition | Justification / v2 changes |
|---|---|---|
| `appdirs` | **Keep as-is** | Leaf, zero deps, correct. No changes. |
| `keyring` | **Change** | Commit `EnvelopeStore` (AD-8); README + AGENTS.md; envelope becomes kit default. |
| `settings` | **Change (heavy)** | Defect fixes (WP-03): binding-surface split so `GetSecret` is not webview-callable, validate effective state not submitted payload, reject non-string password input instead of deleting the secret, fire `onChange` outside the mutex, fsync on save. Then i18n schema change (WP-12). |
| `settings/cli` | **Change** | Localize output via `i18n` (WP-22). |
| `settings/templates/anyllm` | **Rewrite → split** | Schema half moves to `settings/templates/llmconfig` (main module, no SDK); client half stays at `settings/templates/anyllm` as a nested module. Hardcoded model registry becomes configurable options with built-ins as defaults (WP-07). |
| `database` | **Change** | Fix: pragmas applied once to the pool, not per-connection — foreign keys are silently OFF on the second pooled connection. Fix by encoding pragmas into the modernc DSN (`?_pragma=...`) so every connection gets them (WP-04). |
| `diagnostics` | **Change (small)** | Fold `health.Snapshot()` and `firstrun.Info` into the bundle; localize user-visible strings. Core stays. |
| `errors` | **Change** | i18n integration (WP-11): `Text`-based messages, read-time resolution. |
| `events` | **Keep** | `Backend` interface is the model for Wails insulation. No structural change. |
| `lifecycle` | **Change** | Fix: duplicate service names accepted silently; rollback path ignores configured timeouts (WP-04). |
| `logging` | **Change** | Fix: redaction doesn't recurse into `slog.Group` attrs; docs contradict code (WP-06). |
| `shortcuts` | **Change** | Delete the `wails` build tag (AD-4); menu item labels become `i18n.Text`; joins the Wails-importing allowlist (WP-06 + WP-31). |
| `state` | **Change** | Fix: emits events while holding its lock (WP-06). Gains a `windowstate`/`firstrun` consumer role. |
| `updates` | **Change** | Security fixes (WP-05): downgrade attack (a compromised/rolled-back release feed can push an older version — enforce `NewerThan(current)` unless an explicit `AllowDowngrade` option is set, and verify signatures before apply); Linux `/tmp` TOCTOU (stage downloads in `dirs.Temp()` 0700 with `O_EXCL`, never world-writable `/tmp`); README signing instructions that cannot produce a valid signature get rewritten against what `verify.go` actually checks. `version.go` extracted to new leaf package `semver`. |
| `taskfiles` | **Keep** | Orthogonal to code; template references it. |
| `frontend/types`, `frontend/settings` | **Change** | Un-`private`, publish to npm, regenerate types for the v2 schema (i18n-resolved), add `@wails-kit/i18n` (WP-33). |
| `abnl.dev` split publishing | **Remove** | AD-3. Broken and unnecessary. |

### New

| Package | Criteria check (one line) |
|---|---|
| `i18n` | Desktop apps ship to OS locales; every app would hand-roll catalog+plural+hydration identically (AD-5). |
| `appearance` | OS dark/light detection + user override + one resolved source of truth is identical in every app; kills Prune's documented three-layer background-color seam. |
| `windowstate` | Geometry persistence with multi-monitor clamping is absent from Wails v3 (verified: no persistence in `pkg/application`; `WindowDidMove/WindowDidResize` events and `ScreenManager` exist as raw material) and every app needs the same ~200 lines. |
| `health` | Registry of endpoint checks with failure-mode classification; every kit package and app otherwise embeds its own connectivity check. |
| `permissions` | check → request → "open System Settings" flows are pure OS integration, identical per app; macOS notifications/accessibility/full-disk-access first. |
| `firstrun` | Fresh-install vs upgrade vs downgrade detection with ordered hooks; identical in every app; semver already in-kit. |
| `semver` | Extraction of `updates/version.go` so `firstrun` (and apps) don't import the updater to compare versions. Leaf, zero deps. |
| `runner` (+ `runner/flatfile`, `runner/sqlitestore`) | Durable background queue with sleep/wake and retry semantics is desktop infrastructure every non-trivial app rewrites badly; profiles make it one-line adoptable. |
| `kit`, `kit/wailsbridge` | The bootstrap (AD-1) — the highest-leverage piece for "create apps quickly". |
| `template/` | `wails3 init -t` custom template generating a working app (menu, settings, theming, updates, crash reporting) — the other half of the bootstrap. |
| `examples/` | One runnable example per package (deliverables standard, §5). |

---

## 4. Deliverables standard

**Every work package that touches a Go package must leave behind all five:**

1. **Code** following AD-9 conventions.
2. **Tests** — table-driven where natural; test doubles from the kit
   (`events.NewMemoryEmitter()` wrapped in `events.NewEmitter`, `keyring.NewMemoryStore()`,
   `t.TempDir()`, `net/http/httptest`); race-clean (`go test -race`).
3. **Package `README.md`** — user documentation: what/why, quickstart, options table,
   events emitted, error codes, settings group (if any).
4. **Runnable example** at `examples/<package>/main.go` — a `main` package that compiles
   with `go build ./examples/...` and demonstrates the primary flow. GUI-dependent
   packages (`windowstate`, `shortcuts`, `permissions`, `wailsbridge`) may require a
   display to *run* but must still *build* headlessly.
5. **`AGENTS.md`** — the agent file, per the template below.

### The agent file, precisely

An **agent file** is a ≤150-line `AGENTS.md` at the package root whose job is: *a
competent agent with zero repo context can extend or modify this package correctly after
reading only this file plus the files it points to.* It is contributor-facing (invariants
and landmines), not user-facing (that's the README). It must be updated in the same PR as
any change that invalidates it — an out-of-date agent file is a review-blocking defect.

Template (copy verbatim, fill every section; delete a section only if truly empty):

```markdown
# <package> — agent notes

## Purpose
One paragraph. What this package owns and what it explicitly does not own.

## Public API (load-bearing signatures)
```go
// The signatures a consumer compiles against. Keep exact. ~10–25 lines.
```

## Invariants (do not break)
- Bullet list of properties that MUST survive any change, each with the one-line reason.
  e.g. "Never emit events while holding s.mu — deadlocks re-entrant listeners."
  e.g. "Secrets never touch settings.json — GetValues masks, SetValues diverts to keyring."

## Dependencies & insulation
- Kit packages used and why. Whether wails/v3 import is allowed here (AD-4 allowlist).

## Extension points
- The intended seams: options to add, interfaces to implement, where new features go.

## Testing
- The doubles to use, what `go test ./<pkg>/ -race` must cover, any manual verification
  that automated tests cannot do (e.g. "theme change needs a real macOS session").

## File map
- one line per file: `service.go — options + constructor + bindings`

## Landmines
- Known sharp edges an agent will otherwise rediscover the hard way.
```

**Root-file ownership rule (collision avoidance):** package WPs may only modify files
inside their owned directories plus create `examples/<pkg>/`. Root `README.md`,
`CLAUDE.md`, `docs/architecture.md`, `go.mod`, `go.work`, and CI config are owned
exclusively by WP-01 (Phase 0) and WP-90 (final docs pass). If a package WP needs a root
change, it records the need in its final report and WP-90 applies it.

---

## 5. Work packages

Conventions for every WP below:

- **Owns** = exclusive write access. Two WPs marked `[P]` (parallel-safe) never share a
  path.
- **Verify** = commands that must pass, run from the repo root, before the WP is done.
  Baseline for every WP (stated once here, implied everywhere):
  `go build ./... && go vet ./... && go test -race ./<owned>/... && golangci-lint run ./<owned>/...`
  plus `go test ./...` (whole repo) as the final gate.
- The executing agent reads this document, the owned packages' current source, and the
  relevant Wails source under
  `/Users/ryan/go/pkg/mod/github.com/wailsapp/wails/v3@v3.0.0-beta.4/` — nothing else is
  assumed.

**Errata — corrections found while executing WP-01. Trust these over the text above.**

- **`./...` is not just kit packages.** `frontend/{settings,types}/node_modules/flatted/golang/pkg/flatted`
  are real Go packages inside the module. Harmless for build/test, but any WP that
  *walks the repo* — notably WP-10's catalog-extraction lint helper — must exclude
  `node_modules` or it will scan vendored JS-adjacent Go source.
- **Import-existence greps must be anchored to the import path, not the bare string.**
  `grep -rn "wails-kit\""` matches the string literal `appName = "wails-kit"` at
  `updates/service.go:286` and yields a false positive on a correct tree. Use
  `grep -rn '"github.com/jrschumacher/wails-kit' --include="*.go" . | grep -v '/v2'`.
  Do **not** filter out `_test.go` — test files are the ones most likely to be missed.
- **Any `go list`-based Wails-import check must pass `-tags wails`** while
  `shortcuts/apply.go` remains behind its build tag. Without the tag, `go list -deps ./...`
  reports zero Wails importers repo-wide and the check passes while proving nothing.
  The shipped `.github/scripts/check-wails-imports.sh` does this and was verified in
  both directions against a planted violation. Allowlist matching is exact full-path
  equality, not an unanchored suffix — a suffix pattern would silently exempt a future
  `.../foo/shortcuts`.
- **WP-01 owned the import-path rewrite across `**/*.md` as well as `**/*.go`.** 13
  package READMEs carried import examples. Package WPs inherit corrected READMEs and
  should not re-fix them.
- **Wails `webview_window.go` line numbers in AD-2 are mis-paired** (cosmetic; all
  symbols exist). Actual: `SetSize:413, IsMinimised:714, IsMaximised:730, Size:738,
  IsFullscreen:758, Position:1046, SetPosition:1058`.
- **Release versioning — DECIDED.** `.release-please-manifest.json` still reads
  `{".": "1.3.0"}` and would compute a 1.x tag that contradicts the `/v2` import path.
  Release automation only runs on `main`, and v2 development happens on the `v2`
  branch against a local `replace` directive (OQ-1), so nothing fires during the
  phases. **Before merging `v2` to `main`:** tag `v2.0.0` manually and seed the
  manifest to `2.0.0` so subsequent automation computes 2.0.x/2.1.0. This is a
  cross-cutting release task, not a package WP — see §Cross-cutting.
- **`go.work.sum` is gitignored** (regenerates on build; committing it means a
  permanently dirty `git status`). `go.work` itself is committed.

### Phase 0 — Foundations (serialize WP-01; then WP-02…WP-07 in parallel)

---

#### WP-01 — Module hygiene: `/v2` path, beta.4, kill split publishing  `[serialized: lands first]`

**Scope.** (1) Rewrite module path to `github.com/jrschumacher/wails-kit/v2` and update
every import in the repo. (2) Bump `github.com/wailsapp/wails/v3` to `v3.0.0-beta.4`;
fix any compile fallout (expected: none — AD-2 verified the touched surfaces). (3) Delete
the split-module scheme: `split-modules.json`, `.github/scripts/publish-split-modules.sh`,
its CI step, the "Selective imports via vanity URL" README section, the "Split module
publishing" section of `docs/architecture.md`. (4) Add committed `go.work` (root module
now; `settings/templates/anyllm` added by WP-07). (5) Replace the Philosophy sections of
`CLAUDE.md` and root `README.md` with the amended text from §1 verbatim. (6) Add the
AD-4 Wails-import CI check. (7) Commit the currently-uncommitted `go.mod` change
(`golang.org/x/crypto` promoted to direct) — it belongs with WP-02's envelope commit but
the file is owned here; coordinate by landing WP-01 with the dependency line intact.

**Owns.** `go.mod`, `go.sum`, `go.work` (new), `CLAUDE.md`, `README.md`,
`docs/architecture.md`, `.github/**`, `split-modules.json` (delete), plus the mechanical
import-path rewrite across all `*.go` (other Phase-0 WPs start after this merges).

**Depends.** Nothing.

**Deliverables.** Compiling repo on beta.4 under `/v2`; CI green including the new
import-policy check; docs updated.

**Verify.**
```sh
grep -rn "wails-kit\"" --include="*.go" . | grep -v "/v2" | grep -v _test && exit 1 || true
grep -n "beta.4" go.mod
test ! -f split-modules.json
go build ./... && go test ./... && golangci-lint run ./...
```

---

#### WP-02 — keyring: land `EnvelopeStore`  `[P]`

**Scope.** Commit `keyring/envelope.go` + `keyring/envelope_test.go` as-is (review for
lint only; the design is settled — AD-8). **Drop** any migration-from-per-item-keychain
work. Add: README section (size-limit rationale, threat-model honesty note from the file
header, rotation, failure policy — do **not** market it as "more secure"); optional
`KeyLister` interface (`Keys() ([]string, error)`) documented as a type-assertion
extension, `Store` interface unchanged; `examples/keyring/main.go` using `MemoryStore` +
envelope in a temp dir; `AGENTS.md`.

**Owns.** `keyring/`, `examples/keyring/`.

**Depends.** WP-01.

**Verify.**
```sh
go test -race ./keyring/...
go build ./examples/keyring/
test -f keyring/AGENTS.md
```

---

#### WP-03 — settings: defect burn-down + binding split  `[P]`

**Scope.** Five known defects, one structural change:

1. **`GetSecret` is webview-callable.** Wails binds every exported method of a registered
   service, so registering `*settings.Service` exposes `GetSecret` to frontend JS.
   Fix structurally: introduce `settings.Binding` — a separate struct with exactly
   `GetSchema`, `GetValues`, `SetValues` — as the only thing ever registered with Wails
   (`wailsbridge` uses it; document it in the README as *the* rule). `Service.GetSecret`
   remains for backend use.
   ```go
   type Binding struct{ svc *Service }
   func (s *Service) Binding() *Binding
   func (b *Binding) GetSchema() Schema
   func (b *Binding) GetValues() (map[string]any, error)
   func (b *Binding) SetValues(values map[string]any) ([]ValidationError, error)
   ```
2. **Validates the submitted payload, not effective state.** `SetValues` currently runs
   `Validate(s.schema, values)` on the partial submission; a partial update can pass
   validation while leaving effective state invalid (or fail because required-but-
   unchanged fields are absent). Fix: load current values, overlay the submission, run
   validation on the merged map, persist only submitted non-secret keys.
3. **Non-string password input deletes the secret.** In `SetValues`,
   `str, _ := v.(string)` turns `42` into `""` which hits the clear-secret branch. Fix:
   non-string values for password fields return a `ValidationError`, never mutate the
   keyring.
4. **`onChange` fires under `s.mu`.** A listener that calls back into the service
   deadlocks. Fix: build the notify snapshot under the lock, release, then invoke
   callbacks.
5. **`Save` lacks fsync.** `settings/store.go` writes tmp+rename without `File.Sync()`;
   crash can leave a zero-length rename winner. Fix: sync before rename (mirror
   `keyring/envelope.go` `writeTempLocked`, including best-effort dir sync).

Also: `AGENTS.md`, README updates, `examples/settings/main.go` (headless: define groups,
set/get, show masking).

**Owns.** `settings/` (root only — not `cli/`, not `templates/`), `examples/settings/`.

**Depends.** WP-01. **Coordinates with** WP-12 (i18n schema change comes later; do not
pre-empt it).

**Verify.**
```sh
go test -race ./settings/
# regression tests exist and are named:
go test ./settings/ -run 'TestBindingSurface|TestValidateEffectiveState|TestPasswordNonString|TestOnChangeNoLock|TestSaveDurability' -v
```

---

#### WP-04 — database + lifecycle defect fixes  `[P]`

**Scope, database.** Pragmas are currently applied once to the pool; the second pooled
connection has `foreign_keys` OFF (SQLite pragmas are per-connection). Fix: build the
modernc DSN with `_pragma=` query parameters so the driver applies them on every new
connection (e.g. `file:app.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)`),
keeping `WithPragmas` semantics (merge with defaults, empty string disables). Add the
regression test: open pool with `MaxOpenConns(2)`, hold one conn, verify
`PRAGMA foreign_keys` == 1 on a second concurrent conn.

**Scope, lifecycle.** (a) `WithService` with a duplicate name must make `NewManager`
return `lifecycle_duplicate_service` (new code) instead of silently accepting. (b) The
rollback path after a failed startup currently ignores per-service/global shutdown
timeouts — apply the same timeout machinery used in `Shutdown`.

Both packages: `AGENTS.md`, README touch-ups, `examples/database/`, `examples/lifecycle/`.

**Owns.** `database/`, `lifecycle/`, `examples/database/`, `examples/lifecycle/`.

**Depends.** WP-01.

**Verify.**
```sh
go test -race ./database/ ./lifecycle/
go test ./database/ -run TestForeignKeysEveryConnection -v
go test ./lifecycle/ -run 'TestDuplicateServiceName|TestRollbackHonorsTimeout' -v
```

---

#### WP-05 — updates security hardening + semver extraction  `[P]`

**Scope.**
1. **Downgrade attack.** `CheckForUpdate`/`ApplyUpdate` must refuse any release not
   strictly `NewerThan` the current version unless `WithAllowDowngrade()` is set, and the
   refusal must happen again at apply time (not only at check time — the release object
   could be swapped between calls).
2. **Linux `/tmp` TOCTOU.** Downloads/extractions must stage exclusively inside
   `dirs.Temp()` (0700, owned) using `O_CREATE|O_EXCL`; never a world-writable shared
   `/tmp` path an attacker can pre-create or swap between verify and apply. Verify then
   rename within the private dir.
3. **Signing docs.** `updates/README.md` signing instructions currently cannot produce a
   signature `verify.go` accepts. Read `verify.go`, decide the canonical flow (see open
   question OQ-4 for minisign-vs-cosign; default: match whatever `verify.go` implements
   today, fix the docs to it, add an end-to-end test that generates a key, signs a fake
   asset, and verifies via the documented commands).
4. **Extract `updates/version.go` → new package `semver/`** (`ParseVersion`, `Version`,
   `Compare`, `NewerThan`); `updates` imports it. Leaf package, zero deps, own
   README/AGENTS.md/tests (move `version_test.go`).

**Owns.** `updates/`, `semver/` (new), `examples/updates/`.

**Depends.** WP-01.

**Verify.**
```sh
go test -race ./updates/ ./semver/
go test ./updates/ -run 'TestRefuseDowngrade|TestApplyRechecksVersion|TestStagingDirPrivate|TestSigningDocsFlow' -v
```

---

#### WP-06 — logging, state, shortcuts small fixes  `[P]`

**Scope.**
- `logging`: redaction must recurse into `slog.Group` attrs (walk `slog.Value` of kind
  Group recursively in the handler, not just top-level keys); reconcile README with code
  (agent: diff every documented behavior against `logger.go`/`redact.go` and fix
  whichever is wrong — prefer fixing docs unless code behavior is clearly a bug).
- `state`: `Save` currently emits its change event while holding the store lock —
  re-entrant listeners deadlock. Emit after unlock (snapshot first).
- `shortcuts`: delete the `(darwin||linux||windows) && wails` build tag from `apply.go`
  so `Apply` always exists (AD-4); update README (remove any tag instructions); note that
  label localization arrives in WP-31.

**Owns.** `logging/`, `state/`, `shortcuts/`, `examples/logging/`, `examples/state/`.

**Depends.** WP-01.

**Verify.**
```sh
go test -race ./logging/ ./state/ ./shortcuts/
go test ./logging/ -run TestRedactGroupRecursion -v
go test ./state/ -run TestEmitOutsideLock -v
grep -n "go:build" shortcuts/apply.go && exit 1 || true
```

---

#### WP-07 — LLM settings split: `llmconfig` + nested `anyllm` module  `[P after WP-01]`

**Scope.** Split `settings/templates/anyllm` per AD-3:

- **`settings/templates/llmconfig`** (main module, imports only `settings` + `i18n`
  later): provider/model/API-key `settings.Group` construction and typed readers.
  ```go
  type Provider struct {
      ID     string                  // "anthropic"
      Label  string                  // becomes i18n.Text in WP-12 pass
      Models []settings.SelectOption // defaults; overridable
  }
  func Builtin() []Provider // current hardcoded registry, exported as data
  func New(opts ...Option) (settings.Group, *Config)
  func WithProviders(ids ...string) Option
  func WithProvider(p Provider) Option              // add/override incl. custom model lists
  func WithModels(providerID string, models []settings.SelectOption) Option // override built-ins
  func WithDefaultProvider(id string) Option
  type Config struct{ /* reads provider/model/base-url from a settings.Service, secret via GetSecret */ }
  func (c *Config) Selection(svc *settings.Service) (providerID, modelID, apiKey string, err error)
  ```
  Hardcoded model lists become `Builtin()` defaults that `WithModels`/`WithProvider`
  override — the current frozen-in-amber registry (`anyllm.go` registry map) is the
  defect being fixed.
- **`settings/templates/anyllm`** (nested module, own `go.mod`, module path
  `github.com/jrschumacher/wails-kit/settings/templates/anyllm/v2`): depends on
  `llmconfig` + `any-llm-go`; provides `BuildProvider(svc *settings.Service, cfg *llmconfig.Config) (anyllm.Provider, string, error)`
  and live model enumeration where the SDK supports it. Root `go.mod` **drops**
  `any-llm-go` entirely — that is the point.
- Add the nested module to `go.work`. Document the release tagging scheme
  (`settings/templates/anyllm/v2.x.y`) in the package README.

**Owns.** `settings/templates/**` (both dirs), `go.work` (adding one `use` line —
coordinate: WP-07 is the only Phase-0 WP allowed to touch `go.work`, WP-01 creates it),
root `go.mod` (removal of `any-llm-go` require — single-line change; land after other
Phase-0 WPs to avoid conflicts, or rebase), `examples/llmconfig/`.

**Depends.** WP-01 (must); land last in Phase 0 (touches root files).

**Verify.**
```sh
grep -n "any-llm-go" go.mod && exit 1 || true
test -f settings/templates/anyllm/go.mod
go build ./... && go test ./settings/templates/llmconfig/
cd settings/templates/anyllm && go build ./... && go test ./... && cd -
go work sync && git diff --exit-code go.work
```

---

#### WP-08 — `diagnostics`: audit `webhook.go`, make submission consent-gated  `[P]`

**Scope.** Resolves OQ-6 (decided: both audit *and* opt-in). Two halves.

*(a) Security audit of `diagnostics/webhook.go`.* This file uploads support bundles to a
remote endpoint and has never been reviewed. Bundles contain logs; for Prune that means
LLM prompt traffic, i.e. user content. It is the highest-risk path in the kit after
`updates`. Audit specifically: TLS enforcement (reject plaintext `http://` destinations
outside localhost); what a hostile or misconfigured endpoint can extract; redaction
coverage on the upload path versus the on-disk bundle path — the review found bundle
redaction solid (`diagnostics.go:335-354`) but the *upload* path was never checked
independently; retry/backoff behaviour and whether a failing endpoint can cause unbounded
retries or leak via error strings; request timeouts; whether response bodies are
logged. Fix what you find; if a finding needs a design decision rather than a fix,
report it rather than guessing.

*(b) Consent gating.* Submission must never happen on a default path. Add a consent
setting exposed as a `SettingsGroup()` (the pattern in `docs/settings-integration.md`),
defaulting to **off**. `Submit` returns a typed refusal error when consent is absent —
not a silent no-op, because a silent no-op is indistinguishable from a broken uploader.
Expose consent state so the later crash-sentinel work (Phase 2) composes with it rather
than reimplementing it. Also fix the bundle file mode: `diagnostics.go:199` uses
`os.Create` (0644); it must be 0600, since users are told the path and will move the
file into Downloads.

**Owns.** `diagnostics/`, `examples/diagnostics/`.

**Depends.** WP-01. Independent of WP-02…WP-07 — no shared paths.

**Deliverables.** Audited + fixed `webhook.go`; consent gate with settings group; 0600
bundles; tests covering consent-absent refusal, consent-present submission against
`httptest`, plaintext-endpoint rejection, and redaction on the upload path; README
documenting the consent model and stating plainly that log *content* redaction remains
the consuming app's responsibility; `AGENTS.md`.

**Verify.**
```sh
go test -race ./diagnostics/...
go build ./examples/diagnostics/
# no test may reach the network:
go test ./diagnostics/... -run . -v 2>&1 | grep -iE "dial tcp|no such host" && exit 1 || true
```

---

### Phase 1 — The GUI-shell spine (i18n first, then heavy parallelism)

---

#### WP-10 — `i18n` core  `[serialized: its API gates WP-11..13, 15]`

**Scope.** Implement AD-5's `i18n` package on `golang.org/x/text` (`language` matching +
`plural` CLDR rules; do **not** hand-roll plural logic). Catalog format:
`locales/<bcp47>.json`, values either `"string"` or `{"one": "...", "other": "..."}`
plural objects; `fmt`-style verbs in messages. OS locale detection (env `LC_ALL/LANG` on
unix, `GetUserDefaultLocaleName` deferred — start with env + explicit option; note
limitation in README). `Localizer.Catalog()` returns the merged, locale-resolved map for
frontend hydration; `SetLocale` re-resolves and emits `i18n:changed` via optional
emitter. A `Binding` struct (same pattern as settings — WP-03) exposes
`GetCatalog/GetLocale/SetLocale` for Wails registration. Kit settings group
(`SettingsGroup()`) offering a locale picker (options = locales present in the merged
catalog, `"system"` default). Extraction lint helper: a `go test`-run check in the
package that walks the repo for `i18n.T(` literals with keys prefixed `wailskit.` and
verifies each key exists in that package's `locales/en.json` (keeps catalogs honest;
wired into CI by WP-90).

**Owns.** `i18n/` (new), `examples/i18n/`.

**Depends.** WP-01.

**Verify.**
```sh
go test -race ./i18n/
go test ./i18n/ -run 'TestPluralCLDR|TestCatalogMergeAppWins|TestFallbackToOther|TestSetLocaleEmits' -v
go build ./examples/i18n/
```

---

#### WP-11 — `errors` × i18n  `[P]`

**Scope.** Per AD-5: `RegisterMessages(map[Code]i18n.Text)`; `UserError` carries the
`Text` (keep the `UserMsg string` JSON field as the *resolved* value at marshal/read
time); `GetUserMessage` resolves via package-level `SetLocalizer` (nil-safe →
`Text.Other`); default messages table becomes `i18n.Text` with `wailskit.errors.*` keys +
`errors/locales/en.json`. Update every kit package's `init()` registration
(`database`, `diagnostics`, `keyring` envelope codes, `lifecycle`, `settings`, `state`,
`updates`) — mechanical, but those files are owned by this WP for the registration lines
only; **conflict rule:** this WP may only touch `RegisterMessages` call sites and
`locales/` files outside `errors/`.

**Owns.** `errors/`, `*/locales/en.json` (new files), `RegisterMessages` call sites
repo-wide, `examples/errors/`.

**Depends.** WP-10, and all Phase-0 WPs merged (so call sites are stable).

**Verify.**
```sh
go test -race ./errors/
go build ./...   # every RegisterMessages call site compiles
go test ./errors/ -run 'TestResolveAtReadTime|TestNilLocalizerFallsBack' -v
```

---

#### WP-12 — `settings` × i18n  `[P]`

**Scope.** Schema fields `Group.Label`, `Field.Label/Description/Placeholder`,
`SelectOption.Label` change `string` → `i18n.Text`. `settings.NewService` gains
`WithLocalizer(*i18n.Localizer)`. `GetSchema()` (and `Binding.GetSchema`) returns a
**resolved** schema — JSON shape unchanged from the frontend's point of view (labels are
plain strings on the wire); resolution happens at call time so an `i18n:changed` →
refetch cycle just works. `Validate` error messages become `i18n.Text`. Update
`updates.SettingsGroup()` and any other in-kit group definitions (same narrow-touch
conflict rule as WP-11: group-definition literals only).

**Owns.** `settings/` (root), `updates/settings.go` (group literal), `frontend` untouched
(WP-33 handles TS).

**Depends.** WP-10, WP-03. **Not** parallel with another WP touching `settings/` root
(none in this phase).

**Verify.**
```sh
go test -race ./settings/
go test ./settings/ -run 'TestSchemaResolvesLocale|TestSchemaWireShapeUnchanged' -v
```

---

#### WP-13 — `appearance` (new)  `[P]`

**Scope.** One resolved source of truth for dark/light:

```go
package appearance

type Mode string   // "system" | "light" | "dark"  (user preference)
type Theme string  // "light" | "dark"             (resolved truth)

// Source is the OS signal. Wails-free here; wailsbridge supplies the real one
// (Env.IsDarkMode() + events.Common.ThemeChanged → context IsDarkMode()).
type Source interface {
    IsDark() bool
    Subscribe(func(dark bool)) (cancel func())
}

type Service struct{ /* ... */ }
func NewService(opts ...Option) *Service
func WithSource(s Source) Option
func WithSettings(svc *settings.Service) Option // reads/watches "appearance.mode"
func WithEmitter(e *events.Emitter) Option
func (s *Service) Mode() Mode
func (s *Service) SetMode(m Mode) error   // persists via settings when wired
func (s *Service) Resolved() Theme
func SettingsGroup() settings.Group       // the theme picker (system/light/dark), i18n labels
```

Events: `appearance:changed` payload `{mode, resolved}` — emitted on OS flip (when
mode=system) and on user override. Resolution rule: `mode != system → mode`, else
`Source.IsDark()`. Nil source → light, documented. The **frontend decides how to apply
it** (CSS class/vars); README shows the recommended pattern including setting the Wails
window background at startup from `Resolved()` — this is what kills Prune's three-layer
background-color seam (Prune `CLAUDE.md` "Background Color Consistency": Wails
`BackgroundColour`, base CSS, and theme CSS must all agree; with appearance as the single
truth, the template wires all three from one value).

**Owns.** `appearance/` (new), `examples/appearance/`.

**Depends.** WP-10 (labels), WP-03 (settings group registration pattern). Source wiring
to Wails arrives in WP-31; this package tests with a fake `Source`.

**Verify.**
```sh
go test -race ./appearance/
go test ./appearance/ -run 'TestResolveOverridesSystem|TestSystemFlipEmits|TestPersistViaSettings' -v
```

---

#### WP-14 — `windowstate` (new)  `[P]`

**Scope.** Geometry persistence with multi-monitor clamping. Verified absent from Wails
v3 (beta.4 persists nothing; raw material exists: `WindowDidMove`/`WindowDidResize`
window events, `WebviewWindow.Position/Size/SetPosition/SetSize/IsMinimised/IsMaximised`,
`App.Screen.GetAll()` returning `Screen{Bounds, WorkArea, IsPrimary, ID}`).

```go
package windowstate // Wails-importing (AD-4 allowlist)

type Geometry struct {
    X, Y, W, H int    `json:"x","y","w","h"`
    Maximised  bool   `json:"maximised"`
    ScreenID   string `json:"screenId"`
}

type Manager struct{ /* ... */ }
func Manage(app *application.App, win *application.WebviewWindow, opts ...Option) (*Manager, error)
func WithName(name string) Option            // multi-window: state key per window; default "main"
func WithStore(st *state.Store[Geometry]) Option // default: state.New with app name
func WithDebounce(d time.Duration) Option    // default 500ms
func (m *Manager) Restore()                  // call before window.Show()
func (m *Manager) Close()                    // flush pending save
```

Behavior contract:
- Save on `WindowDidMove`/`WindowDidResize`, debounced; **never save while
  `IsMinimised()`**; save maximised flag rather than maximised geometry (restore
  pre-maximise bounds + re-maximise).
- Restore clamping: saved geometry is applied only if ≥ 50% of its area intersects some
  screen's `WorkArea` from `app.Screen.GetAll()`; otherwise center on the primary screen
  at saved size clamped to that work area. **Never restore onto a detached display.**
- Persistence via `state.Store[Geometry]` (which is why WP-06's emit-under-lock fix
  precedes this).

Clamping math is pure and unit-tested against fabricated `Screen` slices; the
Wails-touching glue is thin. Manual verification recorded in AGENTS.md (unplug-monitor
scenario).

**Owns.** `windowstate/` (new), `examples/windowstate/`.

**Depends.** WP-01 (beta.4), WP-06 (state fix).

**Verify.**
```sh
go test -race ./windowstate/
go test ./windowstate/ -run 'TestClampDetachedDisplay|TestClampPartialOverlap|TestDebounce|TestSkipWhileMinimised' -v
go build ./examples/windowstate/
```

---

#### WP-15 — `health` (new)  `[P]`

**Scope.** The connectivity registry that replaces every ad-hoc "are we online" check.

```go
package health

type Class string
const (
    ClassConnectivity Class = "connectivity" // is there a network at all
    ClassBackend      Class = "backend"      // your own API — remedy: status page, retry
    ClassProvider     Class = "provider"     // third party — remedy: different messaging
)

type State string // "unknown" | "healthy" | "degraded" | "down"

type Probe interface{ Probe(ctx context.Context) error }
func HTTPProbe(url string, opts ...HTTPOption) Probe // GET, expect <400, timeout opt

type Check struct {
    Name     string        // unique; Register errors on duplicates
    Class    Class
    Critical bool          // participates in overall app health
    Interval time.Duration // 0 → registry default (60s)
    Probe    Probe
}

type CheckStatus struct {
    Name      string; Class Class; State State; Critical bool
    Err       string; CheckedAt time.Time; Latency time.Duration
}
type Snapshot struct {
    Overall State                  // see suppression rule below
    Offline bool                   // connectivity class is down
    Checks  []CheckStatus
}

type Registry struct{ /* ... */ }
func New(opts ...Option) *Registry
func WithEmitter(e *events.Emitter) Option
func WithDefaultInterval(d time.Duration) Option
func WithConnectivityProbe(p Probe) Option // default: HTTP 204 probe, URL configurable
func (r *Registry) Register(c Check) (remove func(), err error)
func (r *Registry) Snapshot() Snapshot
func (r *Registry) Trigger(names ...string)      // manual re-check now
func (r *Registry) Start(ctx context.Context)    // ticker loop; Stop via ctx
func (r *Registry) Binding() *Binding            // GetSnapshot/Trigger for the webview
```

Failure-mode classification (the point of the package): when the connectivity check is
down, `Snapshot.Offline=true` and backend/provider checks are reported as
`state=unknown` rather than `down` — "your API is down" must not be claimed when the
laptop's Wi-Fi is off. Overall = worst state among *critical* checks after suppression.
Events: `health:changed` emitted only on state transitions (not every tick), payload =
the changed `CheckStatus` + new `Snapshot.Overall`. A built-in default connectivity check
registers automatically (removable). Other kit packages register their own behind the
scenes: `updates` registers `github-releases` (ClassProvider) when constructed with a
registry option (`updates.WithHealth(r)` — small addition to `updates/`, allowed here
under the narrow-touch rule: one option + one Register call); `anyllm`/`llmconfig`
documents the pattern for provider endpoints. Cadence is user-configurable via
`SettingsGroup()` (check interval; i18n labels).

**Owns.** `health/` (new), `updates/health.go` (new file only), `examples/health/`.

**Depends.** WP-10 (labels). Parallel-safe with WP-05 (different files in `updates/` —
WP-05 owns existing files, WP-15 adds `updates/health.go`; if WP-05 is in flight, land
this file after it merges).

**Verify.**
```sh
go test -race ./health/
go test ./health/ -run 'TestOfflineSuppressesBlame|TestTransitionEventsOnly|TestManualTrigger|TestDuplicateName' -v
```

---

#### IC-1 — Prune integration checkpoint (after WP-10…15)

See §6 for the mechanics. Adopt in Prune's GUI: appearance (delete the three-layer
background hack), windowstate on the main window, settings binding split, tag-free
shortcuts. Adopt in CLI/TUI: i18n localizer for error output.

---

### Phase 2 — Transitions and OS integration

---

#### WP-20 — `firstrun` (new)  `[P]`

**Scope.**

```go
package firstrun

type Kind string
const (
    Fresh     Kind = "fresh"
    Upgrade   Kind = "upgrade"
    Downgrade Kind = "downgrade"
    Same      Kind = "same"
)

type Info struct {
    Kind     Kind
    Previous semver.Version // zero for Fresh
    Current  semver.Version
}

type Hook struct {
    Name string
    When func(Info) bool                       // helpers below
    Run  func(context.Context, Info) error
}
func OnFresh() func(Info) bool
func OnUpgrade() func(Info) bool
func OnDowngrade() func(Info) bool
func OnUpgradeThrough(version string) func(Info) bool // Previous < v <= Current

type Service struct{ /* ... */ }
func New(opts ...Option) (*Service, error) // WithAppName/WithDirs, WithVersion(string), WithEmitter, WithHooks(...)
func (s *Service) Detect() (Info, error)   // reads version stamp via state.Store
func (s *Service) Run(ctx context.Context) (Info, error)
```

Contract: hooks run **in registration order**, filtered by `When`; the version stamp
(`state.Store[stamp]` in `dirs.Data()`) is written **only after all matching hooks
succeed** — a failed hook reruns next launch (hooks must be idempotent; documented
loudly in README + AGENTS.md). Downgrade runs hooks matching `OnDowngrade` (typically:
warn, or refuse via returned error the app surfaces). Events:
`firstrun:transition {kind, previous, current}`. Uses `semver` (WP-05 extraction), not
`updates`.

**Owns.** `firstrun/` (new), `examples/firstrun/`.

**Depends.** WP-05 (semver), WP-06 (state).

**Verify.**
```sh
go test -race ./firstrun/
go test ./firstrun/ -run 'TestFreshVsUpgradeVsDowngrade|TestStampOnlyAfterSuccess|TestUpgradeThroughBoundary|TestHookOrder' -v
```

---

#### WP-21 — `permissions` (new)  `[P]`

**Scope.** check → request → "denied, open System Settings", macOS first.

```go
package permissions // Wails-importing (AD-4 allowlist: notifications service)

type Kind string
const (
    Notifications  Kind = "notifications"
    Accessibility  Kind = "accessibility"
    FullDiskAccess Kind = "full_disk_access"
)

type Status string // "granted" | "denied" | "not_determined" | "unsupported"

type Service struct{ /* ... */ }
func NewService(opts ...Option) *Service // WithEmitter, WithLocalizer
func (s *Service) Check(k Kind) Status
func (s *Service) Request(ctx context.Context, k Kind) (Status, error)
func (s *Service) OpenSystemSettings(k Kind) error
func (s *Service) Binding() *Binding // Check/Request/OpenSystemSettings for the webview
```

macOS implementations:
- Notifications: wrap `wails/v3/pkg/services/notifications`
  `CheckNotificationAuthorization` / `RequestNotificationAuthorization` (verified in
  beta.4).
- Accessibility: `AXIsProcessTrustedWithOptions` via cgo (darwin file); `Request` passes
  the prompt option; there is no programmatic grant — `Request` returning
  `not_determined`/`denied` is expected, pair with `OpenSystemSettings`.
- Full Disk Access: no API; `Check` probes a canary path (e.g. read
  `~/Library/Safari` metadata — pick and document one stable canary), `Request` is
  `OpenSystemSettings` + returns `not_determined`.
- `OpenSystemSettings`: `x-apple.systempreferences:com.apple.preference.security?Privacy_<pane>`
  URLs via `open`.
- Linux/Windows: all kinds return `Unsupported` in v2 except Windows notifications if the
  Wails service supports it cheaply (agent judgment; `Unsupported` is acceptable).

Prompt copy (the "why we're asking" strings apps show) ships as `i18n.Text` constants so
permission dialogs are consistent across apps. Automated tests cover the state machine
with a fake platform layer; real-OS behavior goes in AGENTS.md manual checklist.

**Owns.** `permissions/` (new), `examples/permissions/`.

**Depends.** WP-01 (beta.4), WP-10 (copy).

**Verify.**
```sh
go test -race ./permissions/
GOOS=linux go build ./permissions/   # unsupported path compiles cross-platform
go build ./examples/permissions/
```

---

#### WP-22 — `settings/cli` + `diagnostics` localization pass  `[P]`

**Scope.** `settings/cli`: all human-facing output through the localizer
(`WithLocalizer`); machine output (`--json`) unchanged. `diagnostics`: bundle gains
`health.json` (from a `WithHealth(*health.Registry)` option) and `firstrun.json`
(transition info); localize user-facing strings; verify panic/webhook paths still hold
(files exist: `panic.go`, `webhook.go` — no redesign, just the additive options).

**Owns.** `settings/cli/`, `diagnostics/`, `examples/diagnostics/`.

**Depends.** WP-10..12, WP-15, WP-20.

**Verify.**
```sh
go test -race ./settings/cli/ ./diagnostics/
go test ./diagnostics/ -run 'TestBundleIncludesHealth|TestBundleIncludesFirstrun' -v
```

---

#### IC-2 — Prune integration checkpoint (after Phase 2)

First-run hooks (Prune workspace init as an `OnFresh` hook), notifications permission
flow, localized CLI output.

---

### Phase 3 — Bootstrap, template, frontend

---

#### WP-30 — `kit` core (new)  `[serialized within phase: API gates 31/32]`

**Scope.** Implement AD-1's `kit.New`/`Start`/`Close` exactly as specified. Wiring rules
(the value of the package — encode them, test them):

- `appdirs.New(info.Name)` + `EnsureAll()`; logging init to `dirs.Log()` with redaction
  defaults (`password`, `token`, `api_key`, `secret`); `events.NewEmitter` with a nil-safe
  backend placeholder that buffers-or-drops until `wailsbridge.Attach` (decide: drop, and
  document — CLI mode never attaches).
- Keyring default: `NewEnvelopeStore(dirs, NewOSStore(info.Name, WithEnvPrefix(strings.ToUpper(info.Name))))`.
- Settings: envelope keyring, app groups + kit groups (appearance, updates, health,
  i18n locale) in a fixed order; localizer wired.
- Health: registry with default connectivity check; updates registers its GitHub check
  when enabled.
- Lifecycle: registers the startable services (health ticker, updates scheduler) with
  correct dependency ordering; `Start`/`Close` delegate to it.
- FirstRun: `Detect` during `New` (cheap), hooks run inside `Start` before dependent
  services.
- Secure defaults are **on**: updates enabled iff `WithGitHubRepo` given; diagnostics
  panic-capture on; everything else opt-out via `Without*`.

`kit` imports no Wails code (compile-guard: the AD-4 CI check covers it).

**Owns.** `kit/` (new, excluding `wailsbridge/`), `examples/kit-headless/` (CLI-style
usage: settings get/set + secret + health snapshot, no GUI).

**Depends.** Everything in Phases 0–2 except WP-14/21 (GUI packages — not imported by
core).

**Verify.**
```sh
go test -race ./kit/
go build ./examples/kit-headless/ && ./examplesbin-smoke 2>/dev/null || go run ./examples/kit-headless/ --help
go list -deps ./kit/ | grep wailsapp && exit 1 || true
```

---

#### WP-31 — `kit/wailsbridge` + shortcuts localization  `[after WP-30]`

**Scope.** Implement `Attach`, `ManageWindow`, `Services` per AD-1: events backend →
`app.EmitEvent`; appearance `Source` implementation from
`application.Get().Env.IsDarkMode()` + `events.Common.ThemeChanged` subscription
(context `IsDarkMode()`); shortcuts menu build + `app.SetMenu`; binding registrations
(settings/i18n/health/permissions/updates/diagnostics). Set window background color from
`appearance.Resolved()` at attach time (the third layer of Prune's seam). In
`shortcuts`: menu item labels → `i18n.Text` (Settings…, standard roles keep native
labels), `WithLocalizer` option.

**Owns.** `kit/wailsbridge/` (new), `shortcuts/` (labels change), `examples/kit-gui/`
(full miniature app: window + menu + settings page served from embedded assets).

**Depends.** WP-30, WP-13, WP-14, WP-15, WP-21.

**Verify.**
```sh
go test -race ./kit/... ./shortcuts/
go build ./examples/kit-gui/
```

---

#### WP-32 — `wails3` project template  `[after WP-31]`

**Scope.** A `template/` directory consumable by `wails3 init -t` (verify the invocation
form against the wails3 CLI in the module cache — `v3/internal/commands` — and document
the exact command; if local-path templates are awkward, a thin git-repo layout under
`template/` that `wails3 init -t <path-or-url>` accepts). Generated app contains:
`main.go` composition root (~40 lines: `kit.New`, `application.New`, `wailsbridge.Attach`,
`ManageWindow`, menu, run); embedded frontend (vanilla TS + Vite — no framework
lock-in) with settings page rendered from `GetSchema`, theme wiring from
`appearance:changed`, update toast from `updates:*` events, health status widget;
`Taskfile.yml` including kit `taskfiles`; `.wails-kit.yml`; README. Include a CI smoke
job **in wails-kit's CI** that generates a project from the template into a temp dir,
`go mod edit -replace`s the kit to the checkout, and `go build`s it (frontend `npm ci &&
npm run build` included).

**Owns.** `template/` (new), `.github/workflows/` template-smoke job (root-file
exception, granted to this WP for that one file).

**Depends.** WP-31, WP-33 (template consumes published TS packages — during CI smoke,
use `file:` refs into `frontend/`).

**Verify.**
```sh
# the CI smoke, runnable locally:
wails3 init -t ./template -n /tmp/kit-smoke-app   # exact flag form per wails3 docs — agent verifies
cd /tmp/kit-smoke-app && go mod edit -replace github.com/jrschumacher/wails-kit/v2=$OLDPWD && go mod tidy && go build ./...
```

---

#### WP-33 — Frontend packages: publish + v2 types  `[P with WP-30]`

**Scope.** `frontend/types`: regenerate for the v2 wire schema (resolved-label schema,
health `Snapshot`, appearance payloads, i18n catalog types, firstrun/permissions
payloads). `frontend/settings`: keep headless logic in sync with `settings/validate.go`
(conditions, dynamic options, validation). New `frontend/i18n` (`@wails-kit/i18n`):
catalog consumption + `Intl.PluralRules` plural resolution + `i18n:changed` refetch
helper. Remove `"private": true` from all three; add npm publish workflow (CI, on
release, `npm publish --access public`; provenance on). Scope name per OQ-2 — until
resolved, keep `@wails-kit/*` and gate publish on the org existing.

**Owns.** `frontend/**`, npm publish workflow file.

**Depends.** WP-12 (schema shape), WP-15 (snapshot shape).

**Verify.**
```sh
cd frontend && npm ci && npm test --workspaces && npm run build --workspaces
node -e "const p=require('./types/package.json'); if(p.private) process.exit(1)"
```

---

#### IC-3 — Prune full adoption + green-field smoke (after Phase 3)

Prune adopts `kit.New`/`wailsbridge.Attach` and deletes its hand-rolled composition;
separately, a brand-new app is generated from the template and must build+run in under
ten minutes of human time. This checkpoint is the v2 release gate.

---

### Phase 4 — Background runner

---

#### WP-40 — `runner` core: profiles, worker, flat-file store  `[serialized start of phase]`

**Scope.** Durable queue + worker ticking, Watermill-inspired (message/handler
separation, middleware-style retry), **not** durable execution.

```go
package runner

type Persistence int
const (
    PersistNone Persistence = iota // in-memory
    PersistStore                   // via Store
)

type Profile struct {
    Name          string
    Persistence   Persistence
    MaxAttempts   int           // 0 = unlimited (durable), N = drop after N (best-effort)
    Backoff       func(attempt int) time.Duration // default: exp, 1s..5m, jitter
    DeadLetter    bool          // failed-terminal jobs retained in "dead" state vs deleted
    ResumeOnWake  bool          // re-tick immediately after sleep/wake gap detection
    MaxQueueDepth int           // backpressure: Enqueue errors beyond this; 0 = unbounded
    Retention     time.Duration // completed-job retention in the store; 0 = delete on success
}
func Ephemeral() Profile  // in-memory, dropped on exit; MaxAttempts=3, no dead-letter
func Durable() Profile    // persisted, at-least-once + idempotency keys, retry w/ backoff, dead-letter
func BestEffort() Profile // persisted, dropped after N failures, no dead-letter
// All fields overridable: p := runner.Durable(); p.MaxAttempts = 10

type JobState string // "pending" | "running" | "done" | "failed" | "dead"
type Job struct {
    ID             string          `json:"id"`
    Type           string          `json:"type"`
    Payload        json.RawMessage `json:"payload"` // stored as raw JSON — human-readable, never base64
    State          JobState        `json:"state"`
    Attempts       int             `json:"attempts"`
    IdempotencyKey string          `json:"idempotency_key,omitempty"`
    EnqueuedAt     time.Time       `json:"enqueued_at"`
    NextRunAt      time.Time       `json:"next_run_at"`
    LastError      string          `json:"last_error,omitempty"`
}

type Handler func(ctx context.Context, job Job) error

type Store interface {
    Append(Job) error
    Update(Job) error
    Due(now time.Time, limit int) ([]Job, error)
    Sweep(retention time.Duration, now time.Time) error
    Close() error
}

type Queue struct{ /* ... */ }
func New(p Profile, opts ...Option) (*Queue, error)
func WithStore(s Store) Option    // required when Persistence == PersistStore
func WithEmitter(e *events.Emitter) Option
func WithWorkers(n int) Option    // default 1 — desktop, not a server
func WithClock(c func() time.Time) Option // tests
func (q *Queue) Handle(jobType string, h Handler)
func (q *Queue) Enqueue(ctx context.Context, jobType string, payload any, opts ...EnqueueOption) (id string, err error)
func WithIdempotencyKey(k string) EnqueueOption // duplicate key while pending/running → returns existing id
func WithDelay(d time.Duration) EnqueueOption
func (q *Queue) Start(ctx context.Context) error // tick loop; sleep/wake detected via monotonic gap > 2×tick
func (q *Queue) Close() error                    // drain running, persist pending
```

Semantics to test explicitly: at-least-once (crash between handler success and Update →
re-run; idempotency key is the app's dedup tool and the docs say so plainly);
backpressure (`ErrQueueFull`); wake-after-sleep triggers immediate due-scan when
`ResumeOnWake`; dead-letter jobs queryable (`q.Dead()`), re-enqueueable.

**`runner/flatfile`** ships in this WP: JSONL, one job-state record per line,
append-only with periodic compaction (rewrite keeping latest record per live job +
retention window). **Human-readability is a hard requirement** (Prune keeps it in git and
runs Claude over it): real field names as in the `Job` JSON tags above, payload inline as
raw JSON, RFC3339 timestamps, **no base64 anywhere**, stable field order via a custom
marshal. A committed `testdata/queue.jsonl` golden file guards the format; the
acceptance test greps it for `"type":` and rejects any `base64`/opaque blob patterns.
Compaction is deterministic (sorted by enqueue time) so git diffs stay reviewable.

Events: `runner:job_done`, `runner:job_failed`, `runner:job_dead` (transition-only).

**Owns.** `runner/`, `runner/flatfile/`, `examples/runner/`.

**Depends.** Phase 0 only (errors/events). Deliberately last in priority: a web-shell
app never needs it (agreed demotion — no disagreement).

**Verify.**
```sh
go test -race ./runner/...
go test ./runner/ -run 'TestAtLeastOnceRedelivery|TestIdempotencyKeyDedup|TestBackpressure|TestWakeRescan|TestDeadLetter|TestProfileOverrides' -v
go test ./runner/flatfile/ -run 'TestGoldenHumanReadable|TestCompactionDeterministic|TestCrashMidAppendRecovers' -v
grep -E 'base64|"data":' runner/flatfile/testdata/queue.jsonl && exit 1 || true
```

---

#### WP-41 — `runner/sqlitestore`  `[after WP-40]`

**Scope.** `Store` implementation on `database` (kit package — reuses the fixed
per-connection pragmas), goose migration for the jobs table, index on
`(state, next_run_at)`. Contract tests shared with flatfile: extract a
`storetest.Run(t, func() Store)` suite in WP-40 and run it here verbatim.

**Owns.** `runner/sqlitestore/`.

**Depends.** WP-40, WP-04.

**Verify.**
```sh
go test -race ./runner/sqlitestore/
```

---

#### IC-4 — Prune background jobs (after WP-41)

Prune moves its background work (e.g. git sync / agent runs) onto `Durable()` + flatfile
in the workspace repo; reviews the JSONL in git and confirms an LLM can read it.

---

### Cross-cutting

#### WP-90 — Root docs + CI consolidation  `[last]`

**Scope.** Rewrite root `README.md` package list; rewrite `docs/architecture.md`
(dependency graph including new packages, AD-4 table, nested-module note,
`docs/settings-integration.md` refresh); update `CLAUDE.md` scopes list and conventions
(add `i18n.Text` and health-registration to the patterns section); wire the i18n
catalog-lint (WP-10) and template smoke (WP-32) into CI; final `go work sync`,
release-please config for `/v2` + nested module.

**Owns.** All root docs, CI config. **Depends.** Everything.

**Verify.** `task check` green; every package listed in README exists and vice versa
(scripted check); `git grep -l "abnl.dev"` returns nothing.

---

## 6. Validation strategy — Prune as the proving ground

**Mechanics.** Prune = repo `aboldnewlook/workctl`; the existing worktree
`/Users/ryan/Projects/aboldnewlook/workctl-upgrade-wails-kit` (branch
`upgrade-wails-kit`, currently on kit v1.3.0 with uncommitted app changes) is the
integration sandbox — kept, per AD-7. At IC-1 it gets:

```sh
cd /Users/ryan/Projects/aboldnewlook/workctl-upgrade-wails-kit
go mod edit -droprequire github.com/jrschumacher/wails-kit
go mod edit -require=github.com/jrschumacher/wails-kit/v2@v2.0.0-alpha \
            -replace=github.com/jrschumacher/wails-kit/v2=/Users/ryan/Projects/aboldnewlook/wails-kit
grep -rl 'jrschumacher/wails-kit"' --include='*.go' . | xargs sed -i '' 's#jrschumacher/wails-kit#jrschumacher/wails-kit/v2#g'
go mod tidy && go build ./...
```

The `replace` against the local checkout is the standing state for all checkpoints —
this is the "local snapshot" mode Ryan asked for. No kit tags are cut until IC-3 passes.

**When to integrate.** At the four checkpoints (IC-1…IC-4), not per-WP. Rationale:
per-WP integration thrashes Prune with churn from APIs that WP-12/WP-31 are about to
touch again; per-phase integration is soon enough to catch "feels wrong in practice"
before the next phase builds on it. Each checkpoint is a dispatched task with its own
report; kit API changes it motivates are filed as amendments to this document before the
next phase starts.

**What "working out well" means — concrete pass criteria per checkpoint:**

1. **Net deletion in Prune.** Each adopted capability deletes more app code than it adds
   (measure with `git diff --stat`). If adopting a kit package *grows* Prune, the kit API
   is wrong.
2. **No adapter shims.** Prune needs no wrapper/adapter over a kit API longer than ~20
   lines. A shim is a design smell to fix kit-side.
3. **Behavior checklist** (manual, per checkpoint):
   - IC-1: toggle OS dark mode with app open → window + UI follow with no flash; set
     override to light → OS toggle no longer affects app; move window to external
     monitor, quit, unplug monitor, relaunch → window appears on the laptop screen;
     kill Wi-Fi → status shows offline, not "backend down".
   - IC-2: delete the version stamp → fresh-install hooks run once, not twice; downgrade
     the stamp → downgrade path fires; notifications permission round-trip incl. the
     "open System Settings" path after denial.
   - IC-3: Prune `main.go` composition ≤ ~60 lines; `prune` CLI and TUI run with zero
     Wails symbols in the binary (`go version -m` / `nm` spot check); template app builds
     and runs green-field.
   - IC-4: pull the plug mid-job → job re-runs after relaunch exactly once
     (idempotency); `queue.jsonl` diff in git is human-reviewable.
4. **CLI/TUI parity.** Everything Wails-free (per AD-4's table) is exercised at least
   once from a non-GUI Prune entry point.

---

## 7. Sequencing

### Dependency graph (WP granularity)

```
Phase 0:  WP-01 ──┬─▶ WP-02 keyring        ─┐
                  ├─▶ WP-03 settings fixes  │
                  ├─▶ WP-04 db+lifecycle    ├─(all parallel)
                  ├─▶ WP-05 updates+semver  │
                  ├─▶ WP-06 log/state/short │
                  └─▶ WP-07 llm split (lands last: touches root files)

Phase 1:  WP-10 i18n ──┬─▶ WP-11 errors×i18n      ─┐
          (serialized)  ├─▶ WP-12 settings×i18n    │ parallel
                        ├─▶ WP-13 appearance       │ (WP-14 needs only 01+06;
                        └─▶ WP-15 health           │  may start with WP-10)
          WP-06 ─────────▶ WP-14 windowstate      ─┘
          ── IC-1 ──

Phase 2:  WP-05,06 ─▶ WP-20 firstrun   ─┐
          WP-01,10 ─▶ WP-21 permissions ├─ parallel
          WP-10..15,20 ─▶ WP-22 cli/diag localization
          ── IC-2 ──

Phase 3:  (0–2) ─▶ WP-30 kit core ─▶ WP-31 wailsbridge ─▶ WP-32 template
          WP-12,15 ─▶ WP-33 frontend  (parallel with WP-30/31)
          ── IC-3 = v2.0.0 release gate ──

Phase 4:  WP-40 runner+flatfile ─▶ WP-41 sqlitestore ─▶ IC-4
          (WP-40 may start any time after Phase 0 if agent capacity allows)

Final:    WP-90 docs/CI (after everything)
```

### What ships first / independent usefulness

- After **Phase 0**: v2 is already a strictly better v1 — every known defect fixed,
  beta.4, sane module story. Prune's branch could adopt it with only the import rewrite.
- After **Phase 1 + IC-1**: the web-shell app's top needs (appearance, window state,
  health) exist — a green-field shell is viable without the bootstrap by hand-wiring.
- After **Phase 3 + IC-3**: **tag `v2.0.0`.** The bootstrap+template goal ("stand up an
  app in under an hour") is met; runner intentionally trails the release.
- **Phase 4** ships as `v2.1.0`.

### Where this refines the agreed priority order (and why)

Agreed order was: appearance → window state → health → i18n → first-run → permissions →
bootstrap+template → runner. Two refinements, one demotion kept:

1. **A defect phase (Phase 0) precedes everything.** The reviews' defect list touches
   `settings`, `state`, `updates`, `lifecycle`, `database` — packages that appearance,
   windowstate, firstrun, and the bootstrap all build on. Fixing them after building on
   them means fixing them twice.
2. **i18n moves ahead of appearance/health** (from 4th to the head of Phase 1). Settled
   reasoning in AD-5: i18n changes the *types* in `settings.Field` and
   `errors.RegisterMessages`; appearance and health both declare settings groups and
   error messages. Landing them before the i18n types exist means every one of those
   literals is written twice. Since WP-10 is a leaf package with no in-kit dependencies,
   serializing it first costs a few days of wall-clock, not parallelism — WP-13/14/15
   still run concurrently right behind it (and WP-14 doesn't even need it).
3. **Runner stays last** — agreed and kept: the web-shell consumer never needs it, and
   nothing else depends on it.

---

## 8. Open questions (for Ryan — do not guess in implementation)

- **OQ-1: v2 release cadence during the phases.** Tag `v2.0.0-alpha.N` per phase for
  Prune pinning convenience, or stay on the `replace` directive until `v2.0.0`? Plan
  assumes `replace`-only (simplest); pre-releases are cheap to add.
- **OQ-2: npm scope.** Is `@wails-kit` registrable/owned? Fallbacks: `@aboldnewlook/*`
  or `@jrschumacher/*`. WP-33 gates publish on this answer; everything else proceeds.
- **OQ-3: template distribution.** In-repo `template/` consumed by path/URL vs a
  dedicated `wails-kit-template` repo (cleaner `wails3 init -t github.com/...` UX, but a
  second repo to version). Plan assumes in-repo; WP-32 flags if the wails3 CLI makes
  remote-repo layout materially better.
- **OQ-4: update signing scheme. — DECIDED: minisign.**
  Rationale: the verifier ships inside every consumer desktop app, so verification
  weight and offline behaviour dominate. Sigstore/cosign's keyless model does remove
  the lose-the-private-key-and-strand-every-install failure mode, but `sigstore-go`
  pulls TUF/Rekor/Fulcio and expects network reachability to verify — unacceptable
  in an updater that must work offline and must never add flaky failure modes.
  minisign is Ed25519 underneath (so this is an incremental change to `verify.go`,
  not a rewrite), has a small pure-Go verifier, a stable documented file format, and
  a correct upstream signing recipe. That last point fixes the *actual* observed
  failure: the hand-rolled `openssl` recipe in `updates/README.md` cannot produce a
  valid signature, and owning a bespoke recipe is what put us there. Key-loss risk
  remains — WP-05 must document offline key backup in the strongest terms.
  Affects `taskfiles` release flow.
- **OQ-5: locale sourcing on macOS.** Env-var detection (WP-10) is wrong for GUI-launched
  apps on macOS (no `LANG`). Options: `defaults read -g AppleLocale`, cgo
  `NSLocale`, or a `wailsbridge`-provided source. Ship env+option first; decide the
  darwin-correct source before v2.0.0.
- **OQ-6: `diagnostics` webhook. — DECIDED: audit in Phase 0, and make it opt-in.**
  Both, not either. It uploads support bundles to a remote endpoint, and bundles
  contain logs — for Prune that means LLM prompt traffic, i.e. user content. That
  makes it the highest-risk path in the kit after `updates`, and it has never been
  reviewed. The audit is one file, so it is cheap. Separately, it must not sit on any
  default path: crash reporting that uploads without explicit consent is a trust
  failure, not a configuration preference. Add a Phase 0 work package covering (a) a
  security audit of `webhook.go` — TLS enforcement, redaction coverage, retry/backoff
  behaviour, what a hostile or misconfigured endpoint can extract — and (b) making
  submission consent-gated, with the consent state exposed as a settings field so the
  later crash-sentinel work composes with it.
- **OQ-7: deep links.** The web-shell list includes deep links; beta.4 provides
  `ApplicationOpenedWithFile` + single-instance args but URL-scheme *registration* is
  OS-packaging territory (Info.plist / registry). Is a `deeplink` helper package in scope
  for v2.1, or is template-level plumbing (WP-32 wires the events to a callback) enough
  for v2.0? Plan assumes template-level only.
- **OQ-8: runner flat-file compaction vs git.** Compaction rewrites history lines, which
  churns git diffs. Alternatives: compact only on explicit `q.Compact()` (Prune calls it
  at chosen commit points) vs automatic. Plan assumes automatic with deterministic
  ordering; Prune's IC-4 verdict decides.
