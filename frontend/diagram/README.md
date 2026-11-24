# @wails-kit/diagram

Opt-in diagrams for Wails desktop apps: diagrams written as text in
[Mermaid](https://mermaid.js.org/) (MIT) syntax, drawn read-only in the kit's
look, with pan and zoom. It builds on [`@wails-kit/ui`](../ui/README.md): the
diagram's colours and type come from the `--wk-*` tokens, so light, dark and
an app's theme apply without extra work, and its buttons and error box are
the kit's.

`@wails-kit/ui` stays dependency-free; Mermaid is large, so it lives here and
loads only when a diagram first renders.

| Module | Size (minified) | Gzipped | What it is |
|---|---|---|---|
| `dist/diagram.js` | 25 KB | 9 KB | `wkDiagram()`, the pan and zoom frame and its stylesheet. Loads `mermaid.js` on first use. |
| `dist/mermaid.js` | 1.23 MB | 344 KB | Mermaid 12.3.0 and its libraries, holding only the six diagram types below and the dagre layout. |

The diagram types: **flowchart**, **sequence**, **state**, **class**,
**entity-relationship** and **Gantt**. Any other type Mermaid knows (pie,
mindmap, git graph, …) is refused with a clear message in the error box; it is
not in the bundle.

## Use it

The package is not published to npm. Like `@wails-kit/ui`, vendor the prebuilt
files from a pinned repository release tag that contains
`frontend/diagram/dist/` (see the [ui README](../ui/README.md#use-it) for the
reasoning). The diagram expects `@wails-kit/ui`'s stylesheet on the page, so
vendor that too. Keep `diagram.js` and `mermaid.js` side by side: `diagram.js`
imports `./mermaid.js` relative to itself.

```sh
tag='REPLACE_WITH_RELEASE_TAG'
checkout=$(mktemp -d)
git clone --depth 1 --branch "$tag" https://github.com/jrschumacher/wails-kit.git "$checkout"
mkdir -p vendor/wails-kit-diagram
cp "$checkout/frontend/diagram/dist/diagram.js" \
   "$checkout/frontend/diagram/dist/mermaid.js" \
   "$checkout/frontend/diagram/dist/diagram.d.ts" \
   "$checkout/frontend/diagram/dist/THIRD-PARTY-LICENSES.txt" vendor/wails-kit-diagram/
printf 'Source: https://github.com/jrschumacher/wails-kit\nTag: %s\nCommit: %s\n' \
  "$tag" "$(git -C "$checkout" rev-parse HEAD)" > vendor/wails-kit-diagram/SOURCES.md
(cd vendor/wails-kit-diagram && shasum -a 256 diagram.js mermaid.js diagram.d.ts THIRD-PARTY-LICENSES.txt > SHA256SUMS)
```

Commit the vendored files with the app, `THIRD-PARTY-LICENSES.txt` included:
it carries the licence of every package bundled in `mermaid.js`, as their
licences require, including the two Mermaid's own build inlines (js-yaml and
fastdom, kept in `scripts/licences/`). The libraries' `@license` banners
(DOMPurify's copyright among them) are kept at the end of `mermaid.js`.

```html
<html class="wk-root">
  <link rel="stylesheet" href="vendor/wails-kit-ui/wails-kit-ui.css">
  <script type="module" src="app.js"></script>
  …
  <section id="release"></section>
```

```js
// app.js: a module file, as a strict CSP allows (no inline scripts).
import { wkDiagram } from './vendor/wails-kit-diagram/diagram.js';

const diagram = wkDiagram({
  label: 'Release flow',
  source: `flowchart LR
    push[Push a tag] --> ci{Checks pass?}
    ci -->|yes| build[Build]
    ci -->|no| fix[Fix it]`,
});
document.querySelector('#release').append(diagram);
```

### Content-Security-Policy

It works under the kit's strict policy, `default-src 'self'` with no
`'unsafe-inline'` or `'unsafe-eval'`, with no CSP violations; the browser
checks hold it to that on pages with that policy. How:

- **No eval.** `mermaid.js` has no `eval`, `new Function`, `fetch` or worker.
  The build leaves out `d3-fetch` and `d3-dsv` (which hold `fetch()` and
  `new Function()`; Mermaid does not call them for these diagrams) and
  patches lodash's `Function('return this')()` global lookup to `globalThis`.
- **No inline styles.** Mermaid styles a diagram with a `<style>` element in
  the SVG and with `style` attributes, both of which such a policy blocks.
  The build patches Mermaid and d3 (see `scripts/build.mjs` and
  `src/mermaid/csp.js`): the diagram's rules go into a constructable
  stylesheet adopted by the document, and style attributes are set through
  the CSSOM (`el.style.cssText`), which a CSP does not block. Mermaid
  sanitizes its markup with DOMPurify, whose parser document shares the
  page's CSP, so style attributes travel through it as `data-wk-style` and go
  back through the CSSOM once the SVG is in the page. The build fails if a
  line it patches changes in a new Mermaid.
- **Mermaid's own guards.** `securityLevel: 'strict'` (DOMPurify, no click
  callbacks), `htmlLabels: false` (labels are SVG text, no `foreignObject`),
  and the kit's theme, look, layout, fonts and `themeCSS` are secure keys, so
  a diagram's front matter or `%%{init}%%` directive cannot change them.
- **Nothing loads.** No fonts, icons or images are fetched:
  - links (`click` targets) are removed from the SVG, and every `href` that
    is not a fragment goes;
  - every CSS declaration that names a `url()` other than a fragment is
    dropped, whole (in Mermaid's rules, in style attributes and in a
    diagram's `style`, `classDef` and config such as `actorFontFamily`), as
    are `@import` and `@font-face`;
  - so is every presentation attribute (`fill`, `stroke`, `filter`, `mask`,
    `marker-end`, `cursor`, …) that names one, while drawing and in the
    finished SVG, before it enters the page;
  - shapes that load a picture are refused with a message in the error box:
    the image shape (`A@{ img: … }`, which Mermaid fetches before any
    sanitizing) and the icon shapes (`A@{ icon: … }`, drawn through
    `innerHTML` with style attributes).
- **Lazy loading** uses `import('./mermaid.js')`, a same-origin module, which
  `script-src 'self'` allows.

Saving a PNG draws the SVG through an `<img>` from a `data:` URL, so it needs
`img-src data:` (the kit's demo pages allow it). Without it, Save PNG says it
couldn't, and `toPNG()` rejects.

### Browser floor

Constructable stylesheets (`document.adoptedStyleSheets`): WebView2, Safari
16.4 (macOS 13.3) and WebKitGTK 2.40. On an older web view the styles fall
back to `<style>` elements, which a strict CSP blocks, and the diagram warns
once in the console. The diagram's styles are adopted by the document, so put
it in the light DOM, not in a shadow root.

## `wkDiagram(spec)`

Returns a `<div class="wk-diagram">`: a toolbar, the viewport with the
diagram, and the text alternative. It renders straight away; append it
wherever you like.

| Option | Type | Default | What it does |
|---|---|---|---|
| `label` | string | required | The accessible name: the SVG's and the viewport's. |
| `source` | string | `''` | The Mermaid text. |
| `description` | string | the source | A text alternative, in a `<details>` below the diagram. |
| `id` | string | none | The element's id. Without one the element has none (its parts get generated ids; Mermaid's render ids start `wk-mermaid-`). |
| `filename` | string | the label as a slug | The saved file's name, without its extension. |
| `onerror(failure)` | function | | Called when the diagram can't be drawn (the error box shows either way). |
| `onexport({ format, blob, filename })` | function | | Called by Save SVG and Save PNG instead of downloading the file: an app saves it through its Go side. |

| Property or method | |
|---|---|
| `source` | Get or set the text; setting it re-renders. |
| `ready` | A promise that settles when the diagram shows the current source in the current appearance and theme (drawn, or the error box shown). Read straight after a theme change, it checks the tokens itself rather than waiting for the next frame's check. |
| `type` | The diagram's type (`'flowchart'`, `'sequence'`, `'state'`, `'class'`, `'er'`, `'gantt'`) or `null`. |
| `zoom` | The zoom level; 1 is 100%. |
| `fit()`, `zoomTo(scale)`, `zoomBy(factor)` | Pan and zoom from code. The zoom runs from 10% to 800%. |
| `toSVG()` | A standalone SVG string: the sanitized drawing with its styles in a `<style>` element, a `<title>` (the label) and a `<desc>` (the description or source). No scripts, links or external references. |
| `toPNG(scale = 2)` | A promise for a PNG `Blob` on the diagram's ground. |
| `copySource()` | Copies the source; resolves `false` if it couldn't. |
| `destroy()` | Removes the diagram and its stylesheet; the element is left empty. Optional: a diagram taken out of the document lets go of its stylesheet and its theme listener on its own, and takes them up again if it is put back. |

| Event (bubbles) | `detail` |
|---|---|
| `wk-render` | `{ type }`: a diagram was drawn. |
| `wk-error` | `{ message, line, kind }`: it couldn't be. `kind` is `'syntax'` (Mermaid's parse or draw failed; `line` is the source line when Mermaid gives one), `'unsupported'` (a type the kit doesn't draw, or no type) or `'load'` (`mermaid.js` didn't load). |

Other exports: `preloadDiagrams()` loads Mermaid ahead of the first diagram;
`diagramTypes`; `timings`, `@wails-kit/ui`'s lifecycle timings, which the
diagram uses and bundles from the kit's `src/motion.js`; and the theme builder, `buildTheme(get)`, with
`TEXT_PAIRS` and `MARK_PAIRS`, the pairs it holds to 4.5:1 and 3:1.

### Pan and zoom

The diagram starts fitted to its box, centred, never above 100%, and follows
its container's size while it stays fitted; once you pan or zoom, a resize
keeps your view.

| Input | Does |
|---|---|
| Drag (mouse, pen or one finger) | Pans. |
| Wheel, trackpad pinch, two-finger pinch | Zooms around the pointer. |
| Arrow keys (Shift: further) | Pan, when the diagram has focus. |
| `+` / `-` | Zoom in and out, when the diagram has focus. |
| `0` | Fits, when the diagram has focus. |
| Toolbar | Zoom out, zoom in, Fit, 100%, and the zoom level. |

The viewport takes the wheel, so a diagram in a scrolling page catches wheel
scrolling while the pointer is over it.

### Loading and errors

It follows [BEHAVIOUR.md](../ui/BEHAVIOUR.md). The frame shows at once.
A render that ends within the quiet wait (400 ms) shows no indicator; a
longer one shows the kit's `wk-spinner` in the viewport (`role="progressbar"`, "Drawing the
diagram"), kept for at least 500 ms, and after 3 s a line saying what is
loading ("Loading the diagram library…" on first use). Re-rendering a diagram
already shown (a new source or theme) keeps it until the new one replaces it,
with the spinner in the corner; a render that replaces another while the
spinner shows keeps it, and its 500 ms still counts from when it appeared.
Screen readers hear "Drawing the diagram" once, when the spinner appears, and
then "Diagram drawn" or "Couldn't draw the diagram" (a status region). Mermaid
loads once per page, however many
diagrams there are; a failed load is tried again on the next render.

A diagram that can't be drawn shows the kit's error box in place of the
diagram, never a blank box: what went wrong in words ("Couldn't draw the
diagram", "Can't draw this kind of diagram"), the line, and Mermaid's message
(with its pointer to the column) to read or copy. The zoom and save buttons
are disabled until a diagram is drawn.

Copy source says "Source copied" (or "Couldn't copy") beside the button for
2 s, in a status region.

### Theme

Mermaid's `base` theme, with its `themeVariables` built from the tokens at
render time (read with `getComputedStyle`, so an app theme and `data-theme`
apply). Every colour is composited onto the diagram's ground as an opaque
`#rrggbb`.

| Part | Tokens |
|---|---|
| Ground | `--wk-color-surface` |
| Text | `--wk-color-text` |
| Nodes, actors, states | `--wk-color-fill-strong` over the ground, outlined in `--wk-color-field-border` |
| Lines and arrows | `--wk-color-text-secondary` |
| Clusters, alternate rows, Gantt sections | `--wk-color-fill`; hairlines `--wk-color-hairline-strong` |
| Highlights (sequence activations, active Gantt tasks, today's line) | `--wk-color-accent` and a tint of it |
| Categorical fills (notes, class and ER accents, Gantt tasks) | `--wk-color-chart-1` to `-5`, tinted into the ground |
| Type | `--wk-font-family` at `--wk-font-size-base` |

Every text and ground pair reaches 4.5:1 and every line 3:1, in light and
dark, with the kit's tokens and with a theme that keeps to
[THEMING.md](../ui/THEMING.md) (`test/theme.test.mjs` checks the kit, the
example theme and a tinted-grounds theme; the browser checks measure the
page's own tokens and the drawn labels).

Diagrams re-render when the appearance or theme changes: `data-theme`,
`class` or `style` on `<html>`; `data-theme` on any other element (a themed
region); a stylesheet `<link>` added, removed or re-pointed (an app swapping
its theme file; checked once it has loaded) or a `<style>` added or removed;
the system's light or dark setting, forced colours or increased contrast.
Each re-renders only if its tokens changed. A diagram out of the document at
the time re-renders when it is put back.

In forced colours, the diagram is drawn in `CanvasText` on `Canvas`: grounds,
lines, outlines and text.

### Accessibility

The viewport carries the diagram for assistive technology: a focusable
group with the role description "diagram", named by `label` and described
(`aria-describedby`) by the text alternative (`description`, or else the
source itself, in a `<details>` below the diagram that anyone can open) and
by how to pan and zoom. The SVG inside is `aria-hidden`, so the name is given
once and Mermaid's drawn text isn't read as loose fragments. The error box
and the progress indicator inside the viewport stay reachable. Toolbar
buttons have names ("Zoom in", "Fit to view", "Save as PNG"). A saved SVG is
`role="img"` with a `<title>` and `<desc>`.

## Layout

`.wk-diagram` is a flex column, 400px tall by default; set its height (or
`flex` in a layout) and the viewport takes what the toolbar and the text
alternative leave. The viewport is at least 160px tall.

## Develop

```sh
npm ci
npm run build          # dist/ from src/ and node_modules
npm test               # dist/ is current, bundle and licence checks, theme contrast, static rules
npm run test:browser   # every diagram type, errors, theme, pan and zoom, export, forced colours, in Chrome
```

`npm run test:browser` serves `frontend/` (the demo pages link
`@wails-kit/ui`'s stylesheet), opens `demo/index.html` and `demo/themed.html`
under their strict CSP, and fails on any console error, CSP violation, failed
request or request outside the server. It needs Chrome (`CHROME_PATH`, or the
default install path); without it, it skips, unless `CI` is set. `WK_CHECK=<text>` runs only the checks whose name contains the text.

The demo pages are the gallery's: each diagram type, a syntax error and an
unsupported type, with a light, dark and system switch. Serve `frontend/`
(for example `npx serve .`) and open `/diagram/demo/index.html`.

After a Mermaid update (Dependabot opens one weekly), run `npm run build` and
commit `dist/`; the build check fails until then. If the build fails on a
patch, a line it patches has changed: read the new Mermaid code, update
`PATCHES` (or `STYLE_CALLS`, `REFUSED_SHAPES`, `INLINED`) in `scripts/build.mjs`, and run the browser
checks, which catch any CSP violation that remains.
