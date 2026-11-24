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
  the action lifecycle a page can show in seconds, each on a row (quiet wait, working, done, measured progress,
  long with Stop, paused, waiting, stalled, queued, failed, cancelled, stopped,
  work in parts), the task list and its summary, toasts with and without Undo,
  copy feedback (and a copy that fails), skeleton rows and a loading chart.
- **`#/all`**, every page one below the other, for visual review. The kit's own
  review pages (`ui/demo/index.html` and the editor's and diagram's demo pages)
  are linked from the overview.

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
refuses everything else (`..`, `node_modules/`, any other path). The editors
and diagrams load from the packages' prebuilt `dist/` modules, as an app vendors
them.

## Tests

```sh
npm ci
npm test                                            # the server's paths, the CSP, the page list, the source helpers
CHROME_PATH=/usr/bin/google-chrome npm run test:browser
```

The browser tests load every page in light, dark, the example theme (light and
dark) and forced colours, and fail on any console error or warning, any CSP
violation, any request outside the local server, any duplicate id and any
sideways scroll. They also check that every interactive element on every page
has an accessible name; that every sidebar entry has its page, with examples
and their markup; that every `.wk-*` class in `ui/src/components.css` and
`ui/src/base.css` appears on some page once its "Try it" buttons have run; the
appearance, theme, text-size and source switches; the gallery's keyboard
navigation; and a "Try it" run's timings.
