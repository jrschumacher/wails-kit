# template

A [`wails3 init -t`](https://v3.wails.io) project template that generates a
working Wails v3 desktop app wired to wails-kit: a `kit.New` +
`wailsbridge.Attach` composition root, and a vanilla TypeScript + Vite
frontend with a settings page, theme wiring, a health badge, and an update
toast already built. This is the "one command" half of wails-kit's stated
goal — standing up a new app quickly and uniformly (`docs/v2-roadmap.md`,
WP-32); `kit`/`kit/wailsbridge` (WP-30/31) are the other half.

## Layout

```
template/
  README.md   — this file (package docs — not copied into generated apps)
  AGENTS.md    — how to modify this template safely (ditto)
  app/         — the actual wails3 template root — see AGENTS.md for why
                 this is a subdirectory and not template/ itself
```

**Point `wails3 init -t` at `template/app`, not `template/`.** Everything
under `template/app/` is copied into every generated project; `README.md`
and `AGENTS.md` live one level up specifically so they aren't.

## Quickstart

Requires the [`wails3` CLI](https://v3.wails.io) pinned to **v3.0.0-beta.4**
(the same version this repo's `go.mod` requires — a mismatched CLI can
generate a project whose `go.mod` conflicts with what actually built it):

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.4
```

```sh
wails3 init \
  -t /path/to/wails-kit/template/app \
  -n myapp \
  -d . \
  -mod github.com/you/myapp \
  -skipgomodtidy
```

`-skipgomodtidy` is **required today**, not optional: wails-kit v2 has no
tagged release yet, so `wails3 init`'s own automatic `go mod tidy` step
fails trying to fetch an unpublished module (verified — see `AGENTS.md`,
"Landmines"). Then, inside the generated project:

```sh
go mod edit -replace github.com/jrschumacher/wails-kit/v2=/path/to/wails-kit
go mod tidy

cd frontend
npm pkg set dependencies.@wails-kit/types=file:/path/to/wails-kit/frontend/types
npm pkg set dependencies.@wails-kit/settings=file:/path/to/wails-kit/frontend/settings
npm pkg set dependencies.@wails-kit/i18n=file:/path/to/wails-kit/frontend/i18n
npm install
npm run build
cd ..

task build   # or: go build .
```

The generated app's own `README.md` repeats this (it's the first thing a
consumer of the template sees, not just a contributor to wails-kit).

## Why a local-path template, not a `wails3 init -t github.com/...` remote

`docs/v2-roadmap.md`'s OQ-3 asked this to flag if remote-repo layout would
be materially better. It would not, for two reasons discovered while
building this: (1) wails-kit itself is not yet published (no `v2.0.0` tag,
no npm packages — see "Pre-publish state" below), so a separate template
repo would have the exact same unpublished-dependency problem one level
removed, with none of the benefit; (2) `wails3 init -t <local-path>` works
today, verified against the real `v3.0.0-beta.4` CLI (built from the module
cache and exercised end-to-end — see `AGENTS.md`, "Verification"). Revisit
this once wails-kit v2.0.0 is tagged: at that point a dedicated
`wails-kit-template` repo gives a nicer `wails3 init -t github.com/...`
one-liner with no local checkout required, at the cost of a second repo to
keep in sync on every kit API change.

## Pre-publish state (temporary — delete this section once resolved)

Two things this template depends on don't exist yet as fetchable artifacts:

1. **`github.com/jrschumacher/wails-kit/v2` has no Go module tag.** The
   generated `go.mod` requires it, with a `replace`-directive comment
   explaining the fix — see `app/go.mod.tmpl`.
2. **`@wails-kit/types` / `@wails-kit/settings` / `@wails-kit/i18n` are not
   on the npm registry** (WP-33 landed them "prepared but unpublished" — the
   npm scope itself is undecided, `docs/v2-roadmap.md` OQ-2). The generated
   `frontend/package.json` depends on them at `^2.0.0`, which cannot resolve
   until either they're published or the consumer swaps in `file:` refs (see
   Quickstart above, and the CI smoke job in `.github/workflows/ci.yml`,
   which does exactly this).

Once both are resolved: drop the `replace` from `go.mod.tmpl`, bump its
`require` line to the real tag, and drop this section along with the
`npm pkg set` steps from the generated README and the CI job.

## Verification

Run for real against the pinned `v3.0.0-beta.4` CLI (not guessed from
reading the CLI's source) as part of landing this template — see the CI job
`template-smoke` in `.github/workflows/ci.yml` for the automated version of
the same steps, and this WP's final report for the transcript.
