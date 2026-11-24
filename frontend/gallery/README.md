# Component gallery

Every component of [`@wails-kit/ui`](../ui/README.md),
[`@wails-kit/editor`](../editor/README.md) and
[`@wails-kit/diagram`](../diagram/README.md), in each state it can show, one page
per component. It is a review tool for people building on the kit and for the
kit's own browser tests; it isn't published, and no app ships it.

```sh
cd frontend/gallery
npm run gallery          # Component gallery: http://127.0.0.1:5178/gallery/
```

`PORT=8080 npm run gallery` picks another port. The server needs no
dependencies; `npm ci` is only for the tests.

## What it shows

- **A sidebar** of the components, grouped like the design system's cards:
  Foundations, Actions, Forms, Containers, Status, Overlays, Layout, Data and
  Editors, with a filter field. Up and Down move between entries, Home and End
  jump, and Down from the filter goes to the first; Tab reaches each one too.
- **One page per component**, addressed by its hash (`#/list`, `#/range`), so a
  test or a person can link straight to it. Each example draws the component and
  shows its markup or script under it, from the same source that drew it, in a
  `wk-code` block with a copy button. Where the design system has a card for the
  component (`docs/design-system/components/<Name>/README.md`), the page ends
  with its "When to use" and "Do and don't".
- **"Try it" buttons** for the behaviour in [`BEHAVIOUR.md`](../ui/BEHAVIOUR.md),
  with the kit's real timings: a busy button, a bar a task moves, the states of
  the action lifecycle, each on a row (quiet wait, working, done, measured
  progress, long with Stop, a timed long wait that shows Stop at 10 s, Stop…
  with its confirming Alert, Esc cancelling short work, paused, waiting,
  stalled, queued, failed, the timeout's failure (shortened to 5 s), cancelled,
  stopped, work in parts), the task list and its summary, toasts with and
  without Undo, app undo (`wkUndo`, with a stand-in for the Go stack and for
  Edit > Undo and Redo), the announcer and `placeAnnouncer`, copy feedback (and
  a copy that fails), skeleton rows with the app's 3 s loading line, and a
  loading chart. Leaving a page cancels, quietly, whatever its "Try it" buttons
  started. Time left ("About 3 min left") isn't shown: `wkTask` doesn't draw it
  yet (BEHAVIOUR.md, Known gaps).
- **`#/all`**, every page one below the other, for visual review. The editor's
  and diagram's own demo pages are linked from the overview.

## The toolbar

Each choice is kept in the address (`?appearance=dark&theme=example&size=125`),
so a link opens the gallery as it was.

| Control | What it does |
|---|---|
| Appearance | System, Light or Dark: sets `<html data-theme>`, as an app's own picker does. |
| Theme | None, `ui/theme/example.css`, or one of the fixture themes in `ui/test/fixtures/` (the gold working tone and low text fail the theme check on purpose). |
| Text size | 100%, 125% or 150%. The kit's sizes are in px, as in a desktop window, so this zooms the page (CSS `zoom`), as the web view's own zoom does. |
| Kit from | `src/` (the ES modules) or the prebuilt `dist/` files an app vendors. It reloads the page: a page that has defined the kit's custom elements can't swap them. |
| Forced colours | A page can't turn forced colours on for itself, and a class that imitates it would test nothing. The button says how to turn it on (a Windows contrast theme, or DevTools' Rendering panel); while it's on, a notice says so. |

## How it's built

Static pages, no dependencies and no build step, under the demo pages' strict
Content-Security-Policy (`default-src 'self'`, no inline script or style): custom
properties are set through the CSSOM.

| File | |
|---|---|
| `index.html` | The shell: the sidebar, the toolbar and the page. |
| `gallery.js` | Links the kit (from the query), draws the toolbar and the sidebar, and routes by hash. |
| `pages/*.js` | The pages, one file per group. A page is `{ id, title, group, card?, intro?, examples }`; an example is `{ title, note?, html?, build?(host, ctx), after?(host, ctx) }`. |
| `source.js`, `card.js` | An example's source as shown, and a design-system card drawn as text. |
| `serve.mjs` | The server, also used by the browser tests. |

To add a component, add a page to its group's file in `pages/`. Ids in an
example's markup start with `g-` and are unique across the gallery, since
`#/all` draws every page at once.

### Why its own folder and server

The gallery combines three packages, and each package's own test server only
serves itself (`@wails-kit/ui` refuses any path outside the package). Rather than
widen one package's server, the gallery has its own: `serve.mjs` serves
`/ui/`, `/editor/` and `/diagram/` from the packages' folders, `/gallery/`
itself, and `/design-system/<Name>/README.md` from `docs/design-system/`, and
refuses everything else: `..`, dotfiles, `node_modules/`, a file whose real
path (after symlinks) leaves the folder the request named (`/ui/…` only from
`frontend/ui/`) or lands in a `node_modules/` or dot folder there, and any
other path. It answers only
requests whose `Host` is `127.0.0.1` or `localhost` on its port. The editors
and diagrams load from the packages' prebuilt `dist/` modules, as an app vendors
them.

## Tests

```sh
npm ci
npm test                                            # the server's paths, the CSP, the page list, the source helpers
CHROME_PATH=/usr/bin/google-chrome npm run test:browser
CHECK='^menu' npm run test:browser                  # only the checks whose names match
```

`npm run test:browser` runs two files (`test/run.mjs` runs both, and fails if
either failed), which share `test/helpers.mjs` (the
server, Chrome, a page that records every problem, and the fake clock:
`open(path, { clock: true })` starts the page's timers at a fixed
`CLOCK_T0`, `pauseClock(page)` stops them a minute on, and the check moves
them with `page.clock.runFor`):

- **`test/browser.mjs`, the gallery itself.** It loads every page in light,
  dark, the example theme (light and dark) and forced colours, and fails on
  any console error or warning, any CSP violation, any request outside the
  local server, any duplicate id and any sideways scroll. It also checks that
  every interactive element on every page has an accessible name; that every
  sidebar entry has its page, with examples and their markup; that every
  `.wk-*` class in `ui/src/components.css` and `ui/src/base.css` appears on
  some page once its "Try it" buttons have run; the appearance, theme,
  text-size and source switches; the gallery's keyboard navigation; and the
  "Try it" runs: on the fake clock, Stop at 10 s, Stop… and its Alert, Esc,
  the timeout and the loading line; on the real clock (waiting for
  conditions), the busy button, app undo and the announcer.
- **`test/components.mjs`, the kit's components**, on the gallery's pages:
  the checks that were `@wails-kit/ui`'s own browser tests on its demo pages.
  The WAI-ARIA keyboard patterns of every custom element, focus return,
  imperative construction and clean-up, forced colours, themes and increased
  contrast, the chart, the diff and its review comments, the settings window,
  `wkTask`, the announcer, the task list, the toast, app undo, copy feedback
  and skeleton rows. `@wails-kit/ui` keeps only what must stay in the package
  (its files on their own, that it loads no font file, `dist/`'s exports).
