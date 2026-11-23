# @wails-kit/ui

Base UI for Wails desktop apps: design tokens in light and dark, a type scale,
CSS components on native elements, a few custom elements where behaviour needs
them, charts, a diff view, a small theme file an app overrides, and a settings window drawn
from wails-kit's [`settings`](../../settings/README.md) schema.

The kit is one design of its own on every platform. It does not imitate macOS
or Windows controls; the system keeps the window frame, menus, the tray icon,
alerts, shortcuts and the light or dark appearance.

- **CSS first.** Most components are a class on a native element, so they keep
  native keyboard and screen-reader behaviour.
- **Custom elements only where behaviour needs them,** each wrapping a native
  building block (`<dialog>`, a `role="menu"` list). Light DOM, no shadow DOM,
  no customized built-ins (`is="…"`, which WebKit lacks), no popover API.
- **Data in through attributes and properties, out through bubbling `wk-*` DOM events.**
- **No runtime dependencies and no build step required:** one CSS file and one
  ES module. TypeScript types are included (`src/index.d.ts`).
- **Prefixes:** classes `wk-`, custom properties `--wk-`, elements `wk-`.
- **Scoped:** every rule is scoped to a `wk-` class or element, and the page
  rules (font, background, focus ring) apply only inside `.wk-root`, so linking
  the stylesheet never restyles a host app.

## Use it

The UI is not published to npm. Vendor the prebuilt files from a pinned
repository release tag for both bundled and unbundled apps. Choose a tag that
contains `frontend/ui/dist/`; until the first such release, this UI is unreleased.
Do not substitute a moving branch such as `main` for the tag.

Run this from the app's frontend directory, replacing the tag placeholder:

```sh
tag='REPLACE_WITH_RELEASE_TAG'
checkout=$(mktemp -d)
git clone --depth 1 --branch "$tag" https://github.com/jrschumacher/wails-kit.git "$checkout"
mkdir -p vendor/wails-kit-ui/fonts
cp "$checkout/frontend/ui/dist/wails-kit-ui.css" vendor/wails-kit-ui/
cp "$checkout/frontend/ui/dist/wails-kit-ui.js" vendor/wails-kit-ui/
# Inter and its licence: the stylesheet loads them from fonts/ beside it.
cp "$checkout/frontend/ui/dist/fonts/"* vendor/wails-kit-ui/fonts/
# Optional TypeScript declarations, named to match the vendored JS module.
cp "$checkout/frontend/ui/src/index.d.ts" vendor/wails-kit-ui/wails-kit-ui.d.ts
printf 'Source: https://github.com/jrschumacher/wails-kit\nTag: %s\nCommit: %s\n' \
  "$tag" "$(git -C "$checkout" rev-parse HEAD)" > vendor/wails-kit-ui/SOURCES.md
(cd vendor/wails-kit-ui && shasum -a 256 wails-kit-ui.css wails-kit-ui.js wails-kit-ui.d.ts fonts/* > SHA256SUMS)
```

Commit the vendored files, `SOURCES.md` and `SHA256SUMS` with the app. To update,
repeat with a newer release tag and review the file and checksum changes. The
temporary checkout can be deleted after copying. Neither the files nor the
commands require npm installation or a local kit build.

### With a bundler (Vite and the like)

Import the vendored files using paths relative to your entry module:

```js
import './vendor/wails-kit-ui/wails-kit-ui.css'; // tokens, components, settings window
import './theme.css';                          // your theme, after the kit's stylesheet
import { wkChart, WkSettings } from './vendor/wails-kit-ui/wails-kit-ui.js';
```

The JavaScript import also defines the `wk-*` elements. Put `class="wk-root"`
on `<html>` (or on the container that holds kit components).

### Without a bundler

Use the same vendored files, served with the app's frontend assets:

```html
<html lang="en" class="wk-root">
<head>
  <link rel="stylesheet" href="vendor/wails-kit-ui/wails-kit-ui.css">
  <link rel="stylesheet" href="theme.css">
  <script type="module" src="vendor/wails-kit-ui/wails-kit-ui.js"></script>
</head>
```

```js
import { wkChart } from './vendor/wails-kit-ui/wails-kit-ui.js';
```

`dist/wails-kit-ui.js` exports exactly what the package entry exports, and
`dist/wails-kit-ui.css` is `src/ui.css` with its imports inlined. Neither loads
anything else, except the stylesheet's two font files from `fonts/` beside
it. The first line of each file names the package version; the tag, commit and
checksums recorded above identify the exact vendored release.

## Font

The kit sets everything in [Inter](https://rsms.me/inter/), and the package
includes it: two variable woff2 files (weights 100 to 900), one for Latin
(`Inter-latin.woff2`, 67 KB) and one for the rest of the Latin script
(`Inter-latin-ext.woff2`, 87 KB). Each has an `@font-face` with a
`unicode-range`, so a file loads only when the page shows one of its
characters, and `font-display: swap`. Nothing comes from the network: desktop
apps can't rely on a font CDN at run time. The faces are in `src/base.css`,
pointing at the package's `fonts/` folder; `dist/wails-kit-ui.css` points at
its own copy in `dist/fonts/`. A bundler copies whichever the stylesheet you
import names. `--wk-font-features` turns on Inter's `cv11` (single-storey a)
and `ss03` (round quotes and commas), and the base rule applies it. The files
are subset from upstream Inter 4.1 with its OpenType features kept; see
[Rebuilding the font files](#rebuilding-the-font-files).

Inter is © The Inter Project Authors, under the SIL Open Font License 1.1
(`fonts/OFL-Inter.txt`, also in `dist/fonts/`). The licence lets an app bundle
and ship the font; keep the licence file with it.

Inter is part of the theme contract: a theme may add fallbacks after it but
not replace it ([THEMING.md](THEMING.md)). An app that still needs another
font sets `--wk-font-family` to a stack without Inter in its own stylesheet,
outside its theme, and sets `--wk-font-features: normal` too, since `cv11`
and `ss03` are Inter's and may mean something else, or nothing, in another
font. An `@font-face` that no element uses downloads nothing, so the files
then never load, and when vendoring you can leave out `fonts/`. The kit is
designed and tested in Inter, so expect small differences in widths.

### Rebuilding the font files

The files come from `InterVariable.ttf` (the roman variable font) in
[Inter 4.1](https://github.com/rsms/inter/releases/download/v4.1/Inter-4.1.zip),
with fontTools 4.68 and brotli. The optical-size axis is pinned at 14, its
default, leaving the weight axis (100 to 900); each file keeps every OpenType
layout feature and drops hinting. The unicode ranges are the ones in the
`@font-face` rules in `src/base.css`.

```sh
fonttools varLib.instancer InterVariable.ttf opsz=14 -o Inter-opsz14.ttf
latin='U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+02C6,U+02DA,U+02DC,U+0304,U+0308,U+0329,U+2000-206F,U+20AC,U+2122,U+2191,U+2193,U+2212,U+2215,U+FEFF,U+FFFD'
ext='U+0100-02BA,U+02BD-02C5,U+02C7-02CC,U+02CE-02D7,U+02DD-02FF,U+0304,U+0308,U+0329,U+1D00-1DBF,U+1E00-1E9F,U+1EF2-1EFF,U+2020,U+20A0-20AB,U+20AD-20C0,U+2113,U+2C60-2C7F,U+A720-A7FF'
pyftsubset Inter-opsz14.ttf --unicodes="$latin" --layout-features='*' \
  --flavor=woff2 --no-hinting --output-file=fonts/Inter-latin.woff2
pyftsubset Inter-opsz14.ttf --unicodes="$ext" --layout-features='*' \
  --flavor=woff2 --no-hinting --output-file=fonts/Inter-latin-ext.woff2
cp LICENSE.txt fonts/OFL-Inter.txt  # from the same zip
npm run build                       # copies them to dist/fonts/
```

The browser checks confirm that `cv11` and `ss03` change what the bundled file
renders.

## Light and dark

The page follows the system appearance. `<html data-theme="light">` or
`data-theme="dark"` overrides it, for the tokens and for `color-scheme`. Every
colour token has a light value on `:root` and a dark value under
`@media (prefers-color-scheme: dark)` on `:root:not([data-theme="light"])`,
repeated on `:root[data-theme="dark"]`.

## The theme file

An app makes the kit its own with one theme file, linked after the kit's
stylesheet. [THEMING.md](THEMING.md) is the contract:

| | Tokens |
|---|---|
| Must set (identity) | `--wk-color-accent`, `--wk-color-on-accent`, `--wk-color-working`, `--wk-color-attention`, `--wk-color-attention-text` |
| May set (grounds, all ten or none) | `--wk-color-bg`, `-sidebar`, `-surface`, `-control`, `-hairline`, `-hairline-strong`, `-control-border`, `-field-border`, `-text`, `-text-secondary` |
| May set | `--wk-font-family` (fallbacks after the kit's first family); `--wk-radius-control`, `-group`, `-window` (within 2px of the kit's) |
| Must not set (derived) | `--wk-color-selection`, `-chart-1`, `-destructive-text`, `-attention-fill`: the kit derives them from the accent and attention with `color-mix()` inside `@supports` |

Each colour needs a light value and a dark value, written in the two dark
blocks with the same values. [`theme/theme.css`](theme/theme.css) lists them
with the kit's defaults: copy it, change the values, and link it after the
kit's stylesheet. [`theme/example.css`](theme/example.css) is an example theme
(a green accent, an amber working tone, rounder corners), shown on
`demo/themed.html`.

There are three state tones (neutral, working, attention) and no warning tone.
Text fields, selects, checkboxes and the like are outlined in
`--wk-color-field-border` (3:1 or more on the window); buttons keep the soft
`--wk-color-control-border`. `color-mix()` needs Safari 16.2 or later (on
macOS 11 and later), WebView2 Chromium 111 or WebKitGTK 2.40; below that the
kit's literals apply, and on older Linux the app runs headless.

[`theme/check.mjs`](theme/check.mjs) ships with the package and checks a theme
against the contract: its blocks, the tokens it sets (opaque `#rrggbb`
colours), all-or-none grounds, radii, and contrast (text 4.5:1 on every ground,
the field border 3:1, working and attention 3:1 on the window and 1.5:1 apart).
The package's tests run it on the kit, both theme files in `theme/` and a
tinted-grounds fixture. An app runs it on its own theme, with no dependencies:

```sh
node node_modules/@wails-kit/ui/theme/check.mjs path/to/theme.css
```

It exits 1 when there are problems. As a module it is
`@wails-kit/ui/theme/check` (`checkTheme`).

The code syntax colours, `--wk-color-code-keyword`, `-string`, `-number`,
`-comment`, `-type` and `-function`, are kit-held: a theme does not set them.
[`@wails-kit/editor`](../editor/README.md)'s code editor and highlighter use
them, and each reaches 4.5:1 on `--wk-color-control` in light and dark.

## Components

CSS (`src/components.css`): each is a class on a native element; its states
come from native attributes (`disabled`, `checked`, `aria-pressed`,
`aria-selected`, `aria-expanded`, `aria-invalid`, …).

| Component | Classes |
|---|---|
| Type scale | `wk-title`, `wk-title-2`, `wk-heading`, `wk-subheading`, `wk-body`, `wk-caption`, `wk-small`, `wk-secondary`, `wk-num`, `wk-mono`, `wk-figure` |
| Button | `wk-button` with `--primary`, `--destructive`, `--quiet`, `--small`, `--icon`, `--locked`; busy with `aria-disabled="true"` and `data-busy` (see [Busy, spinner and progress](#busy-spinner-and-progress)) |
| Link | `wk-link` (an `aria-expanded` link draws a disclosure chevron) |
| Field | `wk-label`, `wk-field`, `wk-help`, `wk-error`; search field `wk-search` |
| Checkbox, radio, switch | `wk-check`, `wk-checkbox`, `wk-radio`, `wk-switch` (`role="switch"`); `wk-check--row`, `wk-check--top` |
| Select | `wk-select` |
| Filter buttons, segmented control | `wk-filters` / `wk-filter`, `wk-segmented` |
| Chip | `wk-chip` (a `<button class="wk-chip">` filters, with hover and disabled states; a leading `wk-dot data-size="sm"` then the item and its state in words), `--count`, `--attention`, `--muted` (dashed, for an absence), `--access` (what the person may do: a glyph first, then "View only" or "Can manage" with `data-level="manage"`, in the text colour) |
| Table | `wk-table` (`--zebra` tints every other row); `wk-table-name`, the row's name as a `<button>` that opens the item; `wk-table-actions`, a cell's row actions on one line; `tr[data-muted]`, a row that no longer counts (secondary text, its name struck) |
| List | `wk-list`, `wk-list-row` (`aria-selected` or `aria-current`), `wk-list-heading` (its count in a `<span>`). Selectable rows of any height. The app lays out each row and moves focus: one row `tabindex="0"`, the rest `-1`; Up and Down move, Home and End jump, Space or a click selects (the demo page shows it). |
| Sidebar, toolbar, footer bar | `wk-sidebar` (`wk-sidebar-detail`: short trailing text on an item, "Running"), `wk-toolbar` (`wk-toolbar-subtitle` after the title: what the view counts over, "last 14 days"; keep a space before it, `Now <span class="wk-toolbar-subtitle">`, so the text reads "Now last 14 days" when copied or styles are off), `wk-footer-bar` |
| Three-pane layout | `wk-three-pane`, `wk-pane-list`, `wk-pane-inspector`; `wk-pane-scroll`, a pane body that scrolls under a fixed toolbar (give it `tabindex="0"` and a name) |
| Tabs | `wk-tablist` inside `<wk-tabs>` |
| Menu, context menu, tooltip | `wk-menu` and its items (`<button>`, or `<a href>` for a URL), `wk-menu--inline` (the items laid out in place, as in a popover), `wk-tooltip` |
| Dialog, sheet, popover surface | `wk-dialog` (`--alert`, sheets), `wk-popover` |
| Settings group, row and tabs | `wk-settings-heading`, `wk-settings-group`, `wk-settings-row`, `wk-settings-label`, `wk-settings-help`, `wk-settings-note`, `wk-settings-tabs` / `wk-settings-tab` (`data-attention`) |
| Panel, facts, section heading | `wk-panel` (`--rows`, `--dashed`), `wk-facts`, `wk-section-heading` |
| Status dot | `wk-dot` with `data-shape` (`filled`, `ringed`, `half`, `hollow`, `dashed`, `bar`), `data-tone` (`neutral`, `working`, `attention`) and `data-size="sm"`. It has no state names: the app maps its own states onto shapes and tones. The filled working dot at the default size has a halo (running), its gap drawn in `--wk-dot-ground`; set that on the dot when it sits on something other than `wk-color-bg`. |
| Utilities | `wk-visually-hidden`: read by assistive technology, not drawn |
| Empty state, notice, error box | `wk-empty`, `wk-notice`, `wk-errorbox` |
| Stat, code block, copy button | `wk-stat`, `wk-code`, `wk-copy` |
| Stepper | `wk-stepper` (an input and two arrow buttons; see `syncSteppers`) |
| Range | `wk-range` on `<input type="range">`, drawn the same on WebKit and Chromium. The app sets `--wk-range-value` to the filled share (`el.style.setProperty('--wk-range-value', '60%')`, which a Content-Security-Policy allows where a `style` attribute is blocked) and updates it on `input`; show the value beside it in an `<output>`. |
| Chart | `wk-chart`, built by `wkChart()` |
| Diff | `wk-diff`, built by `wkDiff()` |
| Spinner, progress bar | `wk-spinner`, `wk-progress`; see [Busy, spinner and progress](#busy-spinner-and-progress) |

`demo/index.html` shows every component in each state it can show without a
pointer, the tokens and every status dot. Serve the package folder (for
example `npx serve .`) and open `/demo/index.html`.

### Custom elements and functions

| Export | What it is |
|---|---|
| `<wk-dialog>` (`WkDialog`) | wraps a native `<dialog>` and opens it modally; focus returns to the opener; `open` attribute; `[data-wk-close]` buttons set `returnValue`; `light-dismiss` attribute; events `wk-open`, `wk-close`. Any element with `data-wk-open="<id>"` opens the dialog with that id. |
| `<wk-menu>` (`WkMenu`) | a WAI-ARIA menu button: a `<button>` followed by a `[role=menu]`; event `wk-select`, `wk-toggle`. Around a `wk-menu--inline` with no button, the same keys in place: one tab stop, arrows, Home, End and typeahead move, Enter and Space activate (`wk-select`); it never opens or closes, and leaves Escape and Tab to the page. A link item (`<a href role="menuitem">`) follows its link on Enter or Space. |
| `<wk-context-menu for="id">` (`WkContextMenu`) | the same menu, opened by right-click, Shift+F10 or the Menu key; `detail.context` is the element it opened on |
| `<wk-tabs>` (`WkTabs`) | WAI-ARIA tabs with automatic activation; event `wk-change` |
| `<wk-tooltip>` (`WkTooltip`) | WAI-ARIA tooltip on focus and after a hover delay (`delay="ms"`); event `wk-toggle` |
| `syncSteppers(root?)` | redraws stepper arrows after the app sets a value itself |
| `setProgress(bar, { done, total, unit?, text? }, { aria?, clock? })` | makes a `wk-progress` determinate and moves it, with its value attributes changing at most every 5 s; see [Busy, spinner and progress](#busy-spinner-and-progress) |
| `timings`, `motion`, `reducedMotion()` | the action lifecycle's timings and the motion values, for scripts; see [Timings and motion](#timings-and-motion) |
| `wkChart(spec)` | bars, stacked bars, or a line with a band, as plain SVG, with a legend, hover and keyboard readout, and a Chart/Table switch; colours from `--wk-color-chart-1` to `-5` (`tone` 1 to 5 or `"neutral"`) |
| `niceTicks`, `labelStep`, `runs`, `roundedTop`, `roundedEnd`, `tableModel`, `readout` | the chart's layout rules, with no DOM |
| `wkDiff(spec)` | a read-only diff of one or more files, unified or split, with collapsible files, word marks, keyboard navigation and "Show more" context; see [Diff](#diff) |
| `parseUnifiedDiff`, `diffSplitRows`, `diffWordRanges` | the diff's parser, split-view pairing and word marks, with no DOM |

Every element works when the app builds the DOM imperatively: it may be
connected before its children exist, moved, or removed while open, and it
removes everything it added to the document when it is disconnected.

## Busy, spinner and progress

[`BEHAVIOUR.md`](BEHAVIOUR.md) says when each of these shows; this is the
markup.

**Busy control.** While a control's action runs, give it
`aria-disabled="true"` and `data-busy` at once, on the press; never
`disabled`, which drops focus. It keeps its focus, its accessible name, its
width and its pressed face (`--wk-color-fill-strong`), with the `progress`
cursor, not the 45% disabled look. The kit drops a click, from the pointer
or the keyboard, when the control pressed (the nearest `button`, `a[href]`,
`input`, `select`, `summary`, or element with role `button`, `link`,
`menuitem…`, `switch`, `checkbox`, `radio`, `tab` or `option`, to the click)
itself has both `data-busy` and `aria-disabled="true"`, so a second press
never reaches the app. Other controls inside a busy region (a Stop button in
a busy row) still work, and `data-busy` without `aria-disabled` blocks
nothing. The kit listens in the capture phase on `window`: the app's
listeners on the document or below never see the dropped click, but a
capture listener on `window` registered before the kit loaded still does. Styled for
`wk-button` (every variant), `wk-filter`, `button.wk-chip`, `wk-copy` and
`button.wk-link`. Remove both attributes when the work ends.

**Spinner.** After the indicator delay (400 ms), a spinner takes the
label's place in the button that started the work:

```html
<button class="wk-button" type="button" aria-disabled="true" data-busy>
  <span class="wk-spinner" aria-hidden="true"></span>Export
</button>
```

It sits over the label, which keeps its place (so the width holds) and stays
in the accessible name; the label's text is painted transparent rather than
hidden with `visibility: hidden`, which would drop it from the name. Icons in
the button are hidden. 14 px, 12 px in a `wk-button--small`; drawn with a
border in `currentColor`. On its own, name the work:
`<span class="wk-spinner" role="progressbar" aria-label="Fetching runs">`. A
spinner keeps turning under reduced motion.

**Progress bar.** 4 px, a `--wk-color-working` fill on a
`--wk-color-fill-strong` track, for panels, sheets and standalone regions; in
a table or list row, progress is words beside the dot, with no bar. With no
value attributes it is indeterminate: a sweep, or under reduced motion a slow
fade in place.

```html
<div class="wk-progress" role="progressbar" aria-label="Copying files"></div>
<span class="wk-caption">12 of 40 files</span>
```

```js
import { setProgress } from '@wails-kit/ui';
setProgress(bar, { done: 12, total: 40, unit: 'files' });
// aria-valuemin="0" aria-valuemax="40" aria-valuenow="12" aria-valuetext="12 of 40 files"
setProgress(bar, { done: 13, total: 40, unit: 'files' });         // within 5 s: the fill alone
setProgress(bar, { done: 40, total: 40, unit: 'files' });         // finished: both, at once
```

`setProgress` moves the fill on every call: it sets `--wk-progress-value`
(the filled share, through the CSSOM, which a Content-Security-Policy
allows). The four value attributes change together, and at most every
`timings.progressValueEvery` (5 s), so a screen reader isn't flooded: a call
within 5 s of the last change moves the fill only, and the next call after
that sets them. They change at once on the first call, when `done` reaches
`total`, when `total` or `unit` differs from the last values set (so they
never describe another total), and when `clock` reads earlier than the last
set. New `text` alone waits for the next due update, so words that embed the
count ("Step 2 of 3 · 12 of 40 files") can't defeat the cap; pass `aria:
'now'` when the words must change at once. Nothing fires later by itself: if the updates stop inside the
window, the attributes stay behind the fill until the next call, so pass
`aria: 'now'` when the work pauses or changes stage. The task helper
(`wkTask`, #142) does so on waiting, pause and each change of stage. `text` replaces the default words ("Step 2 of 3 · 12 of 40 files").
Options: `aria: 'now'` sets them whatever the cap (a change of stage),
`aria: false` never does (except on the first call, since a bar without
`aria-valuenow` is indeterminate), and `clock` returns milliseconds for the
cap (default `performance.now`). A `total` that isn't a positive number
throws a `RangeError`. Progress only moves forward, and a bar never goes back
to indeterminate: start a new bar instead. An app that sets
`--wk-progress-value` itself sets the attributes too.

In forced colours the fill is `Highlight` inside a `CanvasText` outline, the
spinner keeps its border, and a button with a spinner is drawn in
`ButtonFace` and `ButtonText` so its label stays hidden.

## Timings and motion

The timings in [`BEHAVIOUR.md`](BEHAVIOUR.md#timings), in milliseconds, for
the task helper, toasts and the app's own code:

```js
import { timings, motion, reducedMotion } from '@wails-kit/ui';
timings.indicatorDelay;   // 400: no indicator for work that ends sooner
timings.indicatorMin;     // 500: an indicator, once shown, stays this long
// stopDelay 10_000, waiting 30_000, timeout 120_000, estimateStable 5_000,
// loadingLine 3_000, recheckDelay 500, toast 6_000, toastWithAction 10_000,
// copied 2_000, progressValueEvery 5_000,
// backoff [2_000, 4_000, 8_000, 16_000, 30_000] (the last repeats),
// backoffJitter 0.2, wakeSettle 2_000
motion;                   // { durationFast: 100, durationBase: 200, durationSlow: 300, easeOut, easeIn }
reducedMotion();          // true while the system asks for reduced motion
```

Both objects are frozen. In CSS the same motion values are tokens:

| Token | Value | For |
|---|---|---|
| `--wk-duration-fast` | 100ms | state changes: hover, press, a switch's thumb |
| `--wk-duration-base` | 200ms | things appearing or moving: a menu, a toast, a row |
| `--wk-duration-slow` | 300ms | large surfaces: a sheet |
| `--wk-ease-out` | `cubic-bezier(.2, 0, 0, 1)` | things entering |
| `--wk-ease-in` | `cubic-bezier(.4, 0, 1, 1)` | things leaving |

Every transition in the kit uses a duration token. Under
`prefers-reduced-motion: reduce`, transitions are instant or a fade of 100 ms
at most and nothing slides; a component that animates from script checks
`reducedMotion()`.

## Diff

`wkDiff(spec)` returns a `<section class="wk-diff">`: a focusable region with
a Unified/Split switch and one card per file. It builds plain DOM and has no
dependencies.

```js
import { wkDiff } from './vendor/wails-kit-ui/wails-kit-ui.js';

pane.append(wkDiff({
  id: 'changes',                 // names the parts: changes-file-0, changes-file-0-toggle, …
  label: 'Changes',              // the region's accessible name
  diff: patchText,               // git's unified diff (one or more files), or parsed files
  view: 'unified',               // or 'split'; onview(view) reports the switch
  wrap: false,                   // unified: wrap long lines, or scroll the file sideways
  collapseAbove: 400,            // files with more lines start collapsed,
  collapseCharsAbove: 200000,    // and files with more characters
  generated: file => file.path.endsWith('.lock'),     // these start collapsed too
  headingLevel: 3,               // each file's header is a heading of this level (2 to 6)
  emptyText: 'No changes',       // all a diff with no files shows
  getContext: (file, range) => readLines(file.path, range.newStart, range.newEnd),
  highlight: (file, line) => null,                     // a DocumentFragment, or null for plain text
  onrender: (fileElement, file) => {},                 // after a file's rows are (re)built
}));
```

- **Input.** A unified diff string (git format or `diff -u`, LF or CRLF), or
  files already parsed: `{ path, oldPath, status, hunks: [{ header, lines:
  [{ kind: 'context'|'add'|'del', old, new, text }] }] }`, where `status` is
  `modified`, `added`, `deleted`, `renamed`, `copied` or `binary`. Counts and
  hunk ranges are filled in when missing; lines without `old`/`new` are
  numbered from the hunk's start (context takes both, `del` the old number,
  `add` the new), and a line of any other kind is shown as context.
  `parseUnifiedDiff(text)` returns the
  same shape, with `binary`, `oldMode`/`newMode`, `additions`/`deletions` and a
  `noNewline` flag on a line that ended its file without a newline. Paths come
  without git's prefixes, whichever it wrote (`a/` `b/`, `--no-prefix`,
  `diff.mnemonicPrefix`, `--src-prefix`/`--dst-prefix`); in a plain diff,
  a first folder that differs between the `---` and `+++` paths of the same
  file is taken as the prefix. A hunk whose header claims more lines than
  follow ends at the next file. Combined diffs (`diff --cc`, from a merge)
  are not supported: such a file shows "No changes to show".
- **Per file.** A header (a `wk-link` disclosure naming the path, `old → new`
  for a rename, status chips, `+n −m` counts in tabular figures). Large,
  generated and binary files start collapsed (a parsed file's `collapsed`
  overrides this; its `generated` marks it generated); rows are built when a
  file is first expanded. The empty text is all an empty diff shows. A binary file
  shows "Binary file not shown."
- **Lines.** Old and new line numbers in a gutter; added and removed rows are
  tinted and carry a `+` or `−` marker, and each row has a screen-reader label,
  the same in both views ("Added 14:", "Removed 13:", "Line 10:", or "Line
  10, now 12:" for an unchanged line whose number moved). When removed lines are followed by added ones,
  they pair up and the changed words are marked (`<mark class="wk-diff-word">`;
  not for a line over 10,000 characters).
  The split view puts old beside new, row by row, and always wraps long lines
  so its halves stay aligned.
- **Context.** With `getContext(file, { oldStart, oldEnd, newStart, newEnd })`
  (1-based, inclusive; returns the lines as strings, or a promise of them),
  each hunk header offers "Show more" for the unchanged lines above it,
  `contextStep` (20) at a time. If the callback throws or rejects, the button
  stays and a note says "Couldn’t load lines" (announced politely); an empty
  result removes the button. Lines that arrive after a view switch go into
  the view then shown. A wrapped line hangs under its own indent.
- **Highlighting** is not built in: `highlight(file, line)` may return a node
  (a `DocumentFragment` of spans) to use as the line's code. Word marks are
  laid over it when its text is the line's text. If it throws, that line is
  plain text.
- **Keyboard.** While focus is in the diff, `n` and `p` move to the next or
  previous hunk (in expanded files), `]` and `[` to the next or previous file.
  Copying copies the code alone: the gutter, the markers and the labels are
  not selectable, and in the split view only the side the selection starts on
  is copied.
- **Colours.** `--wk-color-diff-add` and `--wk-color-diff-del` tint rows;
  `--wk-color-diff-add-strong` and `--wk-color-diff-del-strong` mark changed
  words. Removed uses the attention hue, added a blue rather than a green, so
  the pair stays distinct with red-green colour blindness; text reaches 4.5:1
  on every tint (checked by `npm test`). In forced colours the tints become a
  bar at the start of the row, solid for added and dashed for removed, and the
  markers stay.
- **Hooks for comments.** Each file card has `data-path`. A unified row has
  `data-kind` and its `data-old` and `data-new` numbers (one of them for an
  added or removed line); a split half has `data-kind`, `data-side` (`old` or
  `new`) and only its own side's number (`data-old` on the old half,
  `data-new` on the new). A file's rows are built lazily and rebuilt on a
  view switch, so anything an app adds among them belongs in `onrender`,
  which runs after every build of a file's rows.

## Settings window

`<wk-settings>` draws a settings window from the schema that
`settings.Service.GetSchema()` returns, with the kit's settings tabs, panels
and rows. With more than one group it shows one tab per group; with one, a
heading. Fields marked `advanced` sit behind an "Advanced" disclosure.

| Field type | Control |
|---|---|
| `text` | `wk-field` text input, with the placeholder |
| `password` | `wk-field` password input; it shows the secret mask when a secret is stored and sends the mask back (no change) unless the person types a new value; focusing it selects the mask |
| `select` | `wk-select`; `dynamicOptions` follow their controlling field, and when it changes, a value no longer offered moves to the first option (`""` when none is offered, and the select is disabled) |
| `toggle` | `wk-switch` (`role="switch"`), the whole row its label |
| `number` | `wk-stepper`, with `validation.min` and `max` as its limits |
| `computed` | read-only text |

A field with a `condition` shows only while the condition is met. A field's
`description` is its help text, read with the control.

```js
import { WkSettings } from '@wails-kit/ui'; // or from the dist module
import { GetSchema, GetValues, SetValues } from './bindings/.../service';

const el = document.querySelector('wk-settings');
el.schema = await GetSchema();
el.values = await GetValues();
el.addEventListener('wk-settings-change', async e => {
  const errors = await SetValues(e.detail.values); // []ValidationError or null
  el.errors = errors ?? [];
});
```

| Property | |
|---|---|
| `schema` | the settings schema; setting it rebuilds the window |
| `values` | the current values (`GetValues()`); setting it updates the controls in place |
| `errors` | `ValidationError[]` to show (from `SetValues`); each shows under its field, marks it `aria-invalid`, and puts the attention dot on its group's tab |
| `icons` | `{ [groupKey]: svg markup or a Node }`, a 22 px glyph above each tab's name |
| `validate` | optional `(schema, values) => ValidationError[]` run on each change, such as `validate` from `@wails-kit/settings`; its errors replace the shown ones |

| Attribute | |
|---|---|
| `label` | the tab list's accessible name (default "Settings") |

**Event** `wk-settings-change` (bubbles), when a control commits a value
(`change`: a text field on blur or Enter): `detail` is `{ key, value, values,
errors }`. `value` has the schema's type (a boolean for a toggle, a number or
`null` for a number field, a string otherwise); `values` holds every field;
`errors` is what `validate` returned (`[]` without it). Without `validate`,
the shown error on the changed field clears and the others stay until the app
sets `errors`.

The rules the window follows are exported with no DOM, for tests and other
renderers: `settingsModel(schema, values, errors)`, `conditionMet`,
`resolveOptions`, `fieldValue`, `readValue`, `displayValue`,
`dependentResets`, `errorsByField` and `SECRET_MASK`.

`demo/settings.html` shows the window on a sample schema.

## Layout

| Path | |
|---|---|
| `src/ui.css` | the stylesheet entry: `tokens.css`, `base.css`, `components.css`, `settings.css` |
| `src/index.js` | the module entry: `kit.js` (custom elements, the busy control's press guard and `setProgress`), `motion.js` (timings and motion values, no DOM), `chart.js`, `diff.js`, `settings-model.js`, `settings.js` |
| `src/index.d.ts` | TypeScript types |
| `theme/theme.css`, `theme/example.css` | the theme file and an example theme |
| `theme/check.mjs` | the theme check, shipped: checks a theme against `THEMING.md` |
| `THEMING.md` | the theme contract |
| `BEHAVIOUR.md` | how components behave over time: the action lifecycle and its timings, feedback, undo and confirm, loading, retries, motion, accessibility and desktop behaviour |
| `fonts/` | Inter (two variable woff2 files) and its licence, SIL OFL 1.1 |
| `dist/wails-kit-ui.css`, `dist/wails-kit-ui.js`, `dist/fonts/` | the prebuilt files and a copy of the fonts, committed and checked against `src/` and `fonts/` |
| `demo/` | review pages: `index.html`, `themed.html`, `dist.html`, `settings.html` |
| `scripts/build.mjs` | builds `dist/` with no dependencies |
| `test/` | the checks |

## Develop

```sh
npm ci
npm run build          # after any change in src/
npm test               # static checks: node --test
npm run test:browser   # custom elements and the settings window, in Chrome
```

`npm test` checks that every `var(--wk-…)` used is defined, that every colour
token has light and dark values (and the same values in both dark blocks),
that themes set only theme tokens and meet the contrast rules, that colours in
component rules come from tokens, that the code syntax colours reach 4.5:1 on
the control colour, that every selector is scoped, that nothing
loads from outside the package or names an app, the chart's layout rules, the
diff's parser, pairing and word marks, the contrast of text on the diff tints, the
settings window's rules, and that `dist/` matches a fresh build.

`npm run test:browser` drives the review pages with `playwright-core` and
Chrome (set `CHROME_PATH`, or have it at `/usr/bin/google-chrome` or the macOS
default; without Chrome it skips, unless `CI` is set). It covers the
WAI-ARIA keyboard patterns of every custom element, focus return, imperative
construction and clean-up, forced colours, the chart, the diff (lines in both
views, keyboard, copy, forced colours, a 5,000-line diff), the prebuilt module's
exports, and the settings window. The kit targets WebKit (Wails on macOS) as
well as WebView2; these checks run in Chrome, so check WebKit by hand on a Mac
for a change in behaviour.

Chart callers can preserve the active category across replacement by retaining the index reported by `onselection(index)` and passing it as `selection` to the next `wkChart` call. `-1` means no selection. Pointer and keyboard interaction, clearing, and restoring a selection may invoke the callback; an out-of-range positive index is clamped to the last category. Callers manage focus when replacing a chart.

Each chart uses its `id` (or a generated ID) to name its focusable plot (`<id>-plot`), view radios (`<id>-view-chart` and `<id>-view-table`), and table (`<id>-table`). A caller replacing a chart can restore focus to the corresponding ID. By default, blurring the plot clears selection; set `retainSelectionOnBlur: true` to retain it. Escape still clears selection, and pointer leave clears it when the plot is not focused.
