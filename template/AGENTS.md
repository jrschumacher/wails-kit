# template — agent notes

## Purpose

`template/app/` is a `wails3 init -t` project template: everything under it
is copied — literally, or through Go's `text/template` for any filename
containing `.tmpl` — into every app a developer generates. `template/`
itself (this file, `README.md`) is *not* copied; it's wails-kit's own
contributor documentation about the template. This package owns file
layout, `text/template` substitution, and keeping the generated app buildable
against this repo's current API. It does not own `kit`/`kit/wailsbridge`
(WP-30/31) — it only demonstrates wiring them.

## Public API (load-bearing signatures)

No Go API — the "interface" is the generated file tree and the invocation:

```sh
wails3 init -t <path-to-repo>/template/app -n <name> -d <dir> \
  -mod <go-module-path> -skipgomodtidy
```

Verified against the real `v3.0.0-beta.4` CLI (built locally from the
module cache), run end-to-end: `init` → `go mod edit -replace` →
`go mod tidy` → `npm install` (with `file:` swaps) → `npm run build` →
`go build .`. Don't trust `wails3 --help` from whatever CLI is on `$PATH` —
a different installed version can have a different template contract (see
Landmines).

## Invariants (do not break)

- **`wails3 init -t` must point at `template/app`, never `template/`.**
  `internal/templates.Install` (wails3 CLI) copies *every* file under the
  given root — `README.md`/`AGENTS.md` living at `template/` instead of
  `template/app/` is what keeps them out of generated apps. If you add a
  new top-level doc file for this package, it goes beside these two, not
  inside `app/`.
- **A file named literally `gitignore` (no dot) must exist at `app/`'s
  root.** After extraction, `wails3 init` unconditionally does
  `os.Rename(gitignore, .gitignore)` and *fails the whole command* if the
  source is missing — this isn't conditional on template source the way the
  `frontend/npmrc` → `.npmrc` rename is.
- **Any file needing per-project substitution (`{{.ProjectName}}`, etc.)
  must have `.tmpl` somewhere in its filename** (gosod strips the first
  occurrence at every position, so `Taskfile.tmpl.yml` → `Taskfile.yml`).
  Files without `.tmpl` are byte-copied — this is what lets
  `taskfiles/release.yml` keep its own `{{ }}` Task syntax untouched.
- **Inside a `.tmpl` file, any literal `{{ }}` that must survive into the
  output (Task's own variable syntax, e.g. in `Taskfile.tmpl.yml`) must be
  written as `{{.Opn}}...{{.Cls}}`**, not `{{ }}` directly — gosod runs
  `text/template` over the *whole file* first, so an unescaped `{{.GOOS}}`
  is evaluated against gosod's data (which has no `.GOOS` field) instead of
  being emitted for Task to read later. Verified by generating a real
  project and inspecting the output `Taskfile.yml`.
- **`main.go.tmpl` never imports `keyring.NewMemoryStore()` or otherwise
  overrides `kit.New`'s defaults.** Unlike the examples (which must never
  touch a real OS keychain), a generated app is a real app — it should get
  the real `EnvelopeStore`-backed keyring and real `appdirs` paths.
- **The frontend uses `Call.ByName` + FQN string constants, not generated
  TS bindings.** `wails3 generate bindings` requires an extra CLI step this
  template doesn't wire into the build (see Extension points) — using it by
  default would make the smoke build depend on an unverified pipeline stage.
  Keep FQNs (`"<go-import-path>.<TypeName>.<Method>"`) in sync with
  `kit/wailsbridge/bindings.go` if that file's registered types or method
  names ever change; this template has no compiler to catch drift.

## Dependencies & insulation

Not applicable in the AD-4 sense — this package contains no Go source of
its own, only text (some of it Go source *for the generated app*). The
generated `main.go` imports `kit` and `kit/wailsbridge`, exactly as a real
consumer would.

## Extension points

- **Add a settings field, health check, etc. to the generated app**: edit
  `app/main.go.tmpl` the same way a real consumer would (`kit.WithSettingsGroup`,
  `kit.WithHealthCheck`, ...) — it's the same API as everywhere else in the
  kit.
- **Switch the frontend to generated TS bindings**: add
  `wails3 generate bindings ./...` as a pre-build step in
  `Taskfile.tmpl.yml`'s `build`/`dev` tasks, then import from
  `../bindings/github.com/jrschumacher/wails-kit/v2/<pkg>` instead of using
  `Call.ByName` with FQN strings in `frontend/src/main.ts`. Not done by
  default — see Invariants.
- **New template variant** (e.g. React): a sibling directory next to `app/`
  (e.g. `template/app-react/`), its own `template.yaml` `shortname`, pointed
  at independently via `-t`. Don't branch `app/` internally on a flag —
  `wails3 init` has no such concept.

## Testing

No Go tests (no Go source to test). The only verification that matters is
generating a real project and building it — see the `template-smoke` job
in `.github/workflows/ci.yml` for the exact steps (init → replace → tidy →
npm `file:` swap → install → typecheck → build → `go build .`). Run it by
hand after any change to `app/`, before trusting CI to catch it — locally
you see the actual `wails3 init` table output and any warnings CI truncates.

## File map

- `README.md` — user-facing docs (quickstart, why local-path, pre-publish
  caveats). Not copied into generated apps.
- `AGENTS.md` — this file. Not copied into generated apps.
- `app/template.yaml` — template metadata (`wailsVersion: 3` required);
  deleted from the generated project automatically after extraction.
- `app/gitignore` — see Invariants; becomes the generated app's `.gitignore`.
- `app/go.mod.tmpl`, `app/main.go.tmpl` — the ~40-line composition root
  (AD-1) and its module file, with the pre-publish `replace` guidance.
- `app/Taskfile.tmpl.yml`, `app/taskfiles/release.yml` — dev/build/package
  tasks; the latter is `taskfiles/release.yml` copied verbatim (re-copy on
  update, don't hand-edit the copy — see its own header comment).
- `app/.wails-kit.yml.tmpl` — release config consumed by `taskfiles/release.yml`.
- `app/README.md.tmpl` — becomes the generated app's own README.
- `app/frontend/` — vanilla TS + Vite; `src/main.ts` is the settings
  form + theme + health badge + update toast (see its own header comment).

## Landmines

- **The installed `wails3` CLI on a given machine may not be beta.4.** This
  environment had `alpha.64` on `$PATH`; alpha.64's template contract is
  materially different (JSON `template.json` instead of YAML
  `template.yaml`, no `Opn`/`Cls` escape fields, no `BinaryName`) — testing
  against it would validate the wrong thing. Build the exact pinned CLI from
  the module cache when in doubt: `cd $(go env GOMODCACHE)/github.com/wailsapp/wails/v3@v3.0.0-beta.4 && GOFLAGS=-mod=mod go build -o /tmp/wails3-beta ./cmd/wails3`.
- **`wails3 init` without `-skipgomodtidy` fails today, always, for this
  template** — verified: it exits 1 and leaves a half-initialized directory
  (extracted files present, `gitignore` *not* renamed to `.gitignore`,
  because the automatic `go mod tidy` runs, and fails to fetch, before
  `Init()` reaches the rename step). This will resolve itself once
  wails-kit v2.0.0 is tagged; don't "fix" it by removing the require line
  from `go.mod.tmpl` in the meantime — that just breaks `go build` instead.
- **`go build ./...` at the generated project's root fails on
  `build/ios`** (`function main is undeclared in the main package`) on a
  non-iOS host — this is `wails3`'s own auto-generated mobile scaffold
  (`GenerateBuildAssets`, unconditional, unrelated to this template's
  content), not something this template can fix. Build the app package
  directly: `go build .`.
- **`npm ci` does not apply to a freshly generated project** — there's no
  committed lockfile (nothing generates one), so `npm ci` errors out
  immediately. Use `npm install`.
- **`@wailsio/runtime`'s npm versions don't track the Go SDK's git tags
  1:1** — no `3.0.0-beta.4` exists on npm (it jumps `beta.1` → `beta.5`).
  Pinned to `3.0.0-beta.1` here, not `beta.5`: this environment's npm has a
  `min-release-age=7` policy (a real, working `.npmrc` setting, not
  specific to this template) and `beta.5`'s publish timestamp was inside
  that window at verification time, making `npm install` fail with
  `ETARGET`. `beta.1` cleared it. Re-evaluate the pin if `npm install`
  starts failing for this reason again — it's a timing artifact, not a
  compatibility signal.
