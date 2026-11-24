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
mkdir -p vendor/wails-kit-ui
cp "$checkout/frontend/ui/dist/wails-kit-ui.css" vendor/wails-kit-ui/
cp "$checkout/frontend/ui/dist/wails-kit-ui.js" vendor/wails-kit-ui/
# Optional TypeScript declarations, named to match the vendored JS module.
cp "$checkout/frontend/ui/src/index.d.ts" vendor/wails-kit-ui/wails-kit-ui.d.ts
printf 'Source: https://github.com/jrschumacher/wails-kit\nTag: %s\nCommit: %s\n' \
  "$tag" "$(git -C "$checkout" rev-parse HEAD)" > vendor/wails-kit-ui/SOURCES.md
(cd vendor/wails-kit-ui && shasum -a 256 wails-kit-ui.css wails-kit-ui.js wails-kit-ui.d.ts > SHA256SUMS)
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
anything else, fonts included. The first line of each file names the package
version; the tag, commit and checksums recorded above identify the exact
vendored release.

## Font

The kit sets text in the platform's own UI font and bundles none:
`--wk-font-family` is `system-ui`, then the named system fonts for engines
that lack it (`-apple-system`, `BlinkMacSystemFont`, `"Segoe UI"`,
`"Noto Sans"`, `Cantarell`, `Ubuntu`, `"Helvetica Neue"`, `Arial`), then
`sans-serif`. That is San Francisco on macOS, Segoe UI on Windows and the
desktop's font on Linux, so a kit app reads like the other apps on the
machine, and no font file is shipped, licensed or loaded. On a Windows CJK
locale, `system-ui` is the locale's UI font (such as Yu Gothic UI or
Microsoft YaHei UI), which also draws Latin text; that is intended. Code,
keys and other monospace text use `--wk-font-family-mono` (`ui-monospace`
first), as before.

System fonts differ in their metrics, so the same size can render slightly
wider or narrower, and wrap differently, on each system. The type scale (11
to 24px) and the line heights are the same everywhere; leave room in fixed
widths rather than fitting text to one system's font.

The font is part of the theme contract: a theme keeps `system-ui` first and
may only add fallbacks after it, such as a font for a script the system font
lacks, which the app bundles with an `@font-face` in its theme
([THEMING.md](THEMING.md)). An app that needs another font sets
`--wk-font-family` in its own stylesheet, outside its theme, ships the font
itself and never loads it from the network.

**Differs from the design system.** The design system (`docs/design-system/`,
a read-only export) names Inter with its `cv11` and `ss03` features. The kit
deliberately uses the system UI font instead, and has no `--wk-font-features`
token.

## Light and dark

The page follows the system appearance. `<html data-theme="light">` or
`data-theme="dark"` overrides it, for the tokens and for `color-scheme`. Every
colour token has a light value on `:root` and a dark value under
`@media (prefers-color-scheme: dark)` on `:root:not([data-theme="light"])`,
repeated on `:root[data-theme="dark"]`.

Increased contrast is followed live too: under `prefers-contrast: more`, one block
at the end of `tokens.css` mixes the lines, control outlines, secondary text and the
fill tints from the theme's `text` and `bg`, in either appearance and any theme.

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
(a green accent, an amber working tone, rounder corners), shown in the
[component gallery](../gallery/README.md) with its Theme switch.

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
| Button | `wk-button` with `--primary`, `--destructive`, `--quiet`, `--small`, `--icon`; locked with `aria-disabled="true"` and `data-locked` (stays focusable, lock glyph first, `title="Needs manage access"`); busy with `aria-disabled="true"` and `data-busy` (see [Busy, spinner and progress](#busy-spinner-and-progress)) |
| Link | `wk-link` (an `aria-expanded` link draws a disclosure chevron) |
| Field | `wk-label`, `wk-field`, `wk-help`, `wk-error`; search field `wk-search` |
| Checkbox, radio, switch | `wk-check`, `wk-checkbox`, `wk-radio`, `wk-switch` (`role="switch"`); `wk-check--row`, `wk-check--top` |
| Select | `wk-select` |
| Filter buttons, segmented control | `wk-filters` / `wk-filter`, `wk-segmented` |
| Chip | `wk-chip` (a `<button class="wk-chip">` filters, with hover and disabled states; a leading `wk-dot data-size="sm"` then the item and its state in words), `--count`, `--attention`, `--muted` (dashed, for an absence), `--access` (what the person may do: a glyph first, then "View only" or "Can manage" with `data-level="manage"`, in the text colour) |
| Table | `wk-table` (`--zebra` tints every other row); `wk-table-name`, the row's name as a `<button>` that opens the item; `wk-table-actions`, a cell's row actions on one line; `tr[data-muted]`, a row that no longer counts (secondary text, its name struck) |
| List | `wk-list`, `wk-list-row` (`aria-selected` or `aria-current`), `wk-list-heading` (its count in a `<span>`). Selectable rows of any height. The app lays out each row and moves focus: one row `tabindex="0"`, the rest `-1`; Up and Down move, Home and End jump, Space or a click selects (the gallery's List page shows it). |
| Sidebar, toolbar, footer bar | `wk-sidebar` (`wk-sidebar-detail`: short trailing text on an item, "Running"), `wk-toolbar` (`wk-toolbar-subtitle` after the title: what the view counts over, "last 14 days"; keep a space before it, `Now <span class="wk-toolbar-subtitle">`, so the text reads "Now last 14 days" when copied or styles are off), `wk-footer-bar` |
| Three-pane layout | `wk-three-pane`, `wk-pane-list`, `wk-pane-inspector`; `wk-pane-scroll`, a pane body that scrolls under a fixed toolbar (give it `tabindex="0"` and a name) |
| Tabs | `wk-tablist` inside `<wk-tabs>` |
| Menu, context menu, tooltip | `wk-menu` and its items (`<button>`, or `<a href>` for a URL), `wk-menu--inline` (the items laid out in place, as in a popover), `wk-tooltip` |
| Dialog, sheet, popover surface | `wk-dialog` (`--alert`, sheets), `wk-popover` |
| Settings group, row and tabs | `wk-settings-heading`, `wk-settings-group`, `wk-settings-row`, `wk-settings-label`, `wk-settings-help`, `wk-settings-note`, `wk-settings-tabs` / `wk-settings-tab` (`data-attention`), `wk-settings-pending` (a check's words), `wk-settings-status` (the window's Notice region) |
| Panel, facts, section heading | `wk-panel` (`--rows`, `--dashed`), `wk-facts`, `wk-section-heading` |
| Status dot | `wk-dot` with `data-shape` (`filled`, `ringed`, `half`, `hollow`, `dashed`, `bar`), `data-tone` (`neutral`, `working`, `attention`) and `data-size="sm"`. It has no state names: the app maps its own states onto shapes and tones. The filled working dot at the default size has a halo (running), its gap drawn in `--wk-dot-ground`; set that on the dot when it sits on something other than `wk-color-bg`. |
| Utilities | `wk-visually-hidden`: read by assistive technology, not drawn |
| Empty state, notice, error box | `wk-empty`, `wk-notice`, `wk-errorbox` |
| Stat, code block, copy button | `wk-stat`, `wk-code`, `wk-copy` (copies by itself with `data-wk-copy`, `data-wk-copy-from` or inside a `wk-code`; "Copied" in `wk-copy-tip`) |
| Toast, skeleton rows | `wk-toast` (`wk-toast-words`, `wk-toast-action`), built by `wkToast()`; `wk-skeleton` rows with `wk-skeleton-bar` cells, built by `wkSkeleton()` |
| Stepper | `wk-stepper` (an input and two arrow buttons; see `syncSteppers`) |
| Range | `wk-range` on `<input type="range">`, drawn the same on WebKit and Chromium. The app sets `--wk-range-value` to the filled share (`el.style.setProperty('--wk-range-value', '60%')`, which a Content-Security-Policy allows where a `style` attribute is blocked) and updates it on `input`; show the value beside it in an `<output>`. |
| Chart | `wk-chart`, built by `wkChart()` |
| Diff | `wk-diff`, built by `wkDiff()` |
| Spinner, progress bar | `wk-spinner`, `wk-progress`; see [Busy, spinner and progress](#busy-spinner-and-progress) |

The [component gallery](../gallery/README.md) shows each component on its own
page, in each state it can show, with its markup, "Try it" buttons for its
behaviour, and switches for the appearance, the theme, the text size and
`src/` or `dist/`: `npm run gallery` in `frontend/gallery`. Its browser checks
are the kit's component checks.

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
| `announce(words, { assertive? })`, `placeAnnouncer(dialog?, open?)` | says words through the window's announcer; see [Tasks and the announcer](#tasks-and-the-announcer) |
| `wkTask(spec)`, `wkTask.follow`, `wkTask.connect`, `wkTask.summary`, `wkTask.showList` | the action lifecycle: the delays, the busy control, the indicator, Stop, Esc, announcements, Go tasks and the task list; see [Tasks and the announcer](#tasks-and-the-announcer) |
| `timings`, `motion`, `reducedMotion()` | the action lifecycle's timings and the motion values, for scripts; see [Timings and motion](#timings-and-motion) |
| `wkChart(spec)` | bars, stacked bars, or a line with a band, as plain SVG, with a legend, hover and keyboard readout, and a Chart/Table switch; colours from `--wk-color-chart-1` to `-5` (`tone` 1 to 5 or `"neutral"`) |
| `niceTicks`, `labelStep`, `runs`, `roundedTop`, `roundedEnd`, `tableModel`, `readout` | the chart's layout rules, with no DOM |
| `wkDiff(spec)` | a read-only diff of one or more files, unified or split, with collapsible files, word marks, keyboard navigation and "Show more" context; see [Diff](#diff); with review comments, see [Review comments](#review-comments) |
| `parseUnifiedDiff`, `diffSplitRows`, `diffWordRanges` | the diff's parser, split-view pairing and word marks, with no DOM |
| `diffCommentPlaces(file, threads)` | where review threads go in a parsed file (under which line, or outdated), with no DOM |
| `wkToast({ words, action? })` | a brief outcome at the bottom of the window, one at a time; see [Toast, undo, copy and skeleton rows](#toast-undo-copy-and-skeleton-rows) |
| `wkUndo.connect`, `wkUndo.action`, `wkUndo.undo`, `wkUndo.redo`, `wkUndo.state` | the frontend of the Go undo stack (`shortcuts.UndoStack`): text focus, Edit > Undo for typing, a toast's Undo |
| `wkCopy(button, text)` | copies, then shows "Copied" above a `wk-copy` for 2 s and says it once; `data-wk-copy` and `data-wk-copy-from` copy with no script |
| `wkSkeleton(tbody, options?)` | skeleton rows for a table that loads, from 400 ms, for at least 500 ms |
| `wkChart.ready(fig)` | resolves when a loading chart (`loading: 'Fetching runs'`) may be replaced |

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
`aria: 'now'` when the work pauses or changes stage. [`wkTask`](#tasks-and-the-announcer)
does so on waiting, pause and each change of stage. `text` replaces the default words ("Step 2 of 3 · 12 of 40 files").
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

## Tasks and the announcer

[`BEHAVIOUR.md`](BEHAVIOUR.md#the-action-lifecycle) is the spec. `wkTask` runs
the lifecycle of one action; the app writes the words, reports progress and
does (and stops) the work.

### The announcer

```js
import { announce } from '@wails-kit/ui';
announce('Link copied');                                 // polite
announce('Couldn’t save the file', { assertive: true }); // only for a failure the person waits on
```

Importing the kit puts two visually hidden regions in the page, before
anything is said: `aria-live="polite"` and `aria-live="assertive"`, both
`aria-atomic="true"` (`<div class="wk-announcer wk-visually-hidden"
data-wk-announcer>`). Messages within 500 ms of each other are joined into one
("Link copied. Comment posted"): a message goes out 500 ms after the last one,
and never more than 2 s after the first. Words the region already holds are
said again (it is cleared and set on the next frame). Each window has one: a
second copy of the kit (`src/` and `dist/` together) uses the same regions and
joins into the same messages. It is one element per window
(`document.wkAnnouncer`): if it leaves the page (removed by the app, or with a
modal it sat in), the same element is put back in `<body>` at once, keeping
the words it was about to say; words it said in the last 2 s are set again on
the next frame. A modal `<dialog>` makes the rest of the page inert, live
regions too, so while one is open the regions sit inside the top-most modal
and go back to `<body>` when it closes, cleared first so the last words aren't
read again where they land. `wk-dialog` moves them, and so does any `<dialog>`
in the document when its `open` attribute changes (watched in the same task,
not on the later `toggle` or `close` event); `placeAnnouncer(dialog, open)`
does it by hand for a modal opened some other way. A modal opened inside a
shadow root, such as another component's, needs `placeAnnouncer(dialog, open)`
when it opens and closes, because the kit can't see into shadow roots. Never
send a countdown, a progress value or streamed output; wkTask, the toast and
the settings window send their words here instead of having live regions of
their own.

The first copy of the kit in the window also sets `document.wkAnnounce` and
`document.wkTask` (its `announce` and `wkTask`), for opt-in packages that use
them without bundling the kit, such as `@wails-kit/editor`'s busy submit.
The announcer is shared by every copy; the task list is not: each copy of the
kit keeps its own, and `document.wkTask` is the first copy's, so work started
through it joins that copy's list (and summary). Load one copy of the kit's
module per window.

### wkTask

```js
import { wkTask } from '@wails-kit/ui';

button.addEventListener('click', async () => {
  const task = wkTask({ control: button, words: 'Exporting', expect: 'maybe-long', stop: 'cancel', bar });
  try {
    for await (const n of exportFiles({ signal: task.signal })) task.progress({ done: n, total: 40, unit: 'files' });
    await task.done({ words: 'Report exported' });  // resolves when the indicator goes: draw the result here
    showResult();
  } catch (error) {
    if (!task.signal.aborted) task.fail({ words: 'Couldn’t export the report', error, retry: () => button.click() });
  }
});
```

| Option | Default | What it does |
|---|---|---|
| `control` | none | The element, or elements, that started it. At once: `aria-disabled="true"` and `data-busy` (it keeps focus, its name and its pressed face; kit.js drops a second press). At the end each goes back to the `aria-disabled` it had, so a locked control stays locked. With no `target`, a `wk-button` control shows the spinner at 400 ms. |
| `target` | none | The element that shows the state, such as a row's state cell. From 400 ms, a working dot and the words (`wk-task-state`) take its content's place; Cancel puts the content back, and Done does unless `visible: false`. Failed shows the attention dot and the words; Stopped shows the words. |
| `bar` | none | A `wk-progress` the app placed where the work shows. Hidden until 400 ms, indeterminate until the work reports a total, hidden again at the end. |
| `words` | required | What it does ("Exporting"); announced when the indicator appears. |
| `expect` | `'short'` | `'maybe-long'` keeps the Stop button's space from the start and shows it at 10 s; `'long'` shows it at once. |
| `stop` | `false` | `'cancel'`, `'stop'`, `'stop-asks'` (labelled "Stop…"), `'pause'` (Pause and Resume), `'unsafe'` (the words add "This can't be stopped" at 10 s) or `false`. |
| `onStop`, `onPause`, `onResume` | none | The app stops, pauses or resumes the work. Resolving `false` leaves the task as it is (the person said no to "Stop…", or the app will report the end itself); otherwise the kit reports Cancelled, Stopped, Paused or Running. Without `onStop`, Stop aborts `task.signal`. While one runs, the Stop button is busy. |
| `timeout` | `timings.timeout` (2 min) | Time without a report before it fails ("Timed out after 2 min", or `timeoutWords`), aborting `task.signal` with a `TimeoutError` and calling `onTimeout`. `false` for none. Off while queued, paused or stalled. |
| `waitingWords` | the current words | After 30 s without a report it goes to Waiting with these words, and back to Running (with its words) on the next report. The switch doesn't restart the timeout. |
| `queued` | none | Start Queued with these words: no indicator and no clocks, Cancel at once; `task.running()` or a progress report starts it. |
| `scope` | the Sheet it is in, else the controls and the target's row | Where Esc applies, and where focus must still be for a failure to be assertive. |
| `esc` | `stop === 'cancel'` and `expect !== 'long'` | Esc (and ⌘. on macOS only) cancels it while focus is in `scope`. The kit listens after the page, so an Escape a menu, a tooltip or the app already used (and prevented) is left alone, and so is one while a Stop is under way. In a Sheet the first Esc cancels the work and the Sheet stays open; a second closes it. |
| `stopIn` | after the last control, else in the target | Where the Stop button goes. |
| `subject`, `kind`, `id` | | Shown in the task list (`subject`, else the words). `id` defaults to `wk-task-<n>`. |
| `listed` | `true` | `false` keeps it out of the task list: work the window shows in place and never leaves waiting there (the settings window's checks). |
| `unit`, `summary` | | Work in parts: see below. |

The handle: `progress({ done, total, unit, step, steps, stage })`, `setWords(words)`,
`waiting(words, { timeout })` (a named wait; `timeout: false` turns it off
while it waits), `paused(words)`, `resumed(words)`, `stalled(words)`,
`queued(words)`, `running(words)`, `done({ words, visible, show, showLabel })`
(or `done(words)`), `fail({ words, error, retry })`, `stopped(words)` (default
"Stopped · 12 of 40 files"), `cancelled({ quiet })`, `stop()` (as the Stop button),
`dismiss()` (takes an ended task, such as a failed one, out of the task list),
`part(spec)`, and `id`, `state` and `signal` (aborted on Cancel, Stop, a
timeout or a failure). `done` and `fail` need words (`TypeError` without):
the outcome is announced, unless `quiet: true` (for a caller that says it
itself, or a cancel the person didn't ask for).

**The clocks.** No indicator before 400 ms; once shown it stays at least
500 ms, so `done`, `fail`, `stopped` and `cancelled` wait for that, then in one
step remove the indicator and the Stop button, return the controls to rest,
announce, fire `wk-task`, and resolve their promise: draw the result in that
`.then`. The clocks count from the press; in a hidden window they run on
(sleep and wake are the Go side's).

**Announcements.** Once each: the words when the indicator appears (work
done sooner announces only its outcome), each change of stage ("Step 2 of 3 ·
Uploading"), new words on Waiting, Paused and Resume, and the end. Never a
count. A failure is assertive only while focus is still in `scope`; otherwise
it is polite and, when the window has a task list and the failure has
`retry`, adds "Retry is in Tasks.". Cancelled is announced only once the
indicator had shown. Focus never moves when work ends, or as the indicator
redraws (the target's line changes in place); if it was on the Stop button, it
goes to the control, else to the target's row, made focusable for it.

**The bar's value** follows `setProgress`'s 5 s cap, and is set at once
(`aria: 'now'`) on Waiting, Paused, Resume and each change of stage, so it
never stays behind the fill while the work stands still.

**Work in parts.** One task, a part per item, each in its own row; one
summary when the last part ends.

```js
const review = wkTask({ control: runButton, words: 'Running reviewers', unit: 'reviewers' });
const parts = reviewers.map(r => review.part({ target: cellOf(r), words: 'Reviewing' }));
// each part: parts[i].done('Passed') / parts[i].fail('Couldn’t run')
// → "2 of 3 reviewers finished · 1 failed", announced once; the task is Done only if every part is
```

Parts don't announce, have their own clocks and Stop mode (the task's, unless
given), and count toward the task's bar ("1 of 3 reviewers"). Once it has
parts, the task's own Waiting switch and timeout are off: its parts keep
theirs. Stop on the task
stops every part (unless it has its own `onStop`). `summary(counts)` replaces
the words; the `wk-task` detail carries `parts: [{ id, state, words }]`. The
task ends a microtask after its last part, so parts added in the same step as
one that ends at once still count. Add every part before any `await`: once a
microtask has passed with every part ended, the task is over.

**Events.** Every change of state fires a bubbling `wk-task` on the target,
else the first control, else the document (also when they have left the
page): `detail: { id, state, words, ...payload }`, where `state` is one of
the [Go states](../../tasks/README.md#states).

### Go tasks: wkTask.connect and wkTask.follow

The [`tasks`](../../tasks/README.md) service keeps the clocks, the words and
the state; the kit draws them. Connect once per window, with the Wails
runtime's events and the service's `Bindings()`:

```js
import { Events } from '@wailsio/runtime';
import * as Tasks from '../bindings/github.com/jrschumacher/wails-kit/tasks/bindings.js';

wkTask.connect({
  subscribe: listener => Events.On('tasks:changed', e => listener(e.data)),
  bindings: Tasks,                     // Tasks, Get, Stop, Pause, Resume, Retry, Dismiss, DismissFinished
  confirmStop: snapshot => askStop(snapshot),  // optional: before a "stop-asks" task stops; resolve true to stop
});

const id = await App.StartExport();
wkTask.follow(id, { control: exportButton, target, bar });
```

`connect` asks for `Tasks()` once (a reloaded window draws the tasks as they
are) unless an event comes first, and returns a function that disconnects;
disconnecting ends every followed task quietly, so its controls go back to
rest. A binding call that fails (Stop, Retry, Dismiss…) is said in words
("Couldn't stop the task"), assertively while focus is in the task list.
`follow(id, { control, target, bar, scope, stopIn, expect })` draws the task
as `wkTask` would, from its snapshots: the indicator from 400 ms after
`startedAt`, Stop from 10 s (`expect` defaults to `'maybe-long'` when it can
be stopped), Waiting, Paused, Resume, Stalled and the end from the Go state. A
failure's words are the app's (`words`), else `error.userMsg`, and it offers
Retry unless its `retry` is `unsafe`. Stop, Pause and Resume call the
bindings; the next snapshot changes the state. A task already finished when
it is first seen is drawn without being announced. A task that isn't in the
list is asked for once with `Get`; if it isn't there, the control is left
alone. It returns `{ id, state, stop(), unfollow() }`, and ends at the task's
first end (a retried task runs on in the task list).

### The summary and the task list

```js
footerBar.append(wkTask.summary());    // or the toolbar, beside the connection state
Events.On('menu:tasks', () => wkTask.showList());  // Window > Tasks
```

`wkTask.summary()` returns a `<button class="wk-task-summary">`: a small dot
and the tasks by state ("2 tasks running · 1 paused · 1 needs you · 1
failed"; Waiting counts as running), shown while two or more tasks are under
way or any has failed or is stalled (the working dot, or the attention dot),
and while the list it opened is open or it has focus. It isn't a live region.
Pressing it opens the list. `button.dispose()` stops it updating; one taken out
of the page stops at the next change (make a new one to put back). A failure
announced away from its control adds "Retry is in Tasks." only while the
window has a task list: a summary in the page, or the list opened.

`wkTask.showList({ opener })` opens the task list, a Sheet (`wk-dialog`) of
the window's tasks: the Go side's (from `connect`), then the page's own
top-level tasks while they run. Each row is a dot, the subject (else the
words), the state's words with the stage, the count and "Next try in 8 s",
then its actions: Cancel, Stop, Stop…, Pause or Resume while it runs (Retry
now while an automatic retry waits), and Retry and Dismiss once it ends. A
task's parts sit under it. A failed task stays until it is retried or
dismissed; a page task that fails stays too, and its Retry runs the `retry`
passed to `fail`. Clear finished calls `DismissFinished`. Rows update in
place, moving no element that has focus; a row that goes while focus is in it hands focus to the next. Focus
starts on the list, not on an action; Esc or Done closes it, and focus
returns to the summary or wherever it was.

### The kit's words

`wkTask.words` holds the words the kit writes itself (Cancel, Stop, Stop…,
Pause, Resume, Paused, Stopped, Cancelled, Failed, "This can't be stopped", "Timed out
after 2 min", "12 of 40 files", "Step 2 of 3", the parts summary, Tasks, Retry,
Dismiss, Clear finished, Done, "3 tasks running", "1 queued", "1 paused",
"1 needs you", "1 failed", the "Couldn't stop the task" family, "Next try in
8 s"). Replace them to write another language; the functions take the
numbers.

### Classes

| Class | What it is |
|---|---|
| `wk-task-stop` | the Cancel, Stop or Pause button (a small `wk-button`); `data-reserved` keeps its space without drawing it |
| `wk-task-state` (`data-state`), `wk-task-words` | the dot and words in a target |
| `wk-task-show` | the Show link after done words that aren't visible |
| `wk-task-summary`, `wk-task-summary-words` | the summary button |
| `wk-task-list`, `wk-task-rows`, `wk-task-row` (`data-state`), `wk-task-text`, `wk-task-title`, `wk-task-line`, `wk-task-actions`, `wk-task-parts`, `wk-task-none` | the task list |
| `wk-announcer` | the announcer's regions |

In forced colours a failed or stalled task's words are underlined, and the
summary is drawn as a button.

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

## Toast, undo, copy and skeleton rows

[`BEHAVIOUR.md`](BEHAVIOUR.md) is the spec: "Notifications inside the window", "Undo,
confirm or neither", "Patterns › Copy to clipboard" and "Loading content".

### Toast

```js
import { wkToast } from '@wails-kit/ui';
wkToast({ words: 'Link copied' });
const toast = wkToast({ words: 'Conversation archived', action: wkUndo.action(id) });
toast.dismiss();
```

One `<div class="wk-toast" role="group">` at a time, at the bottom centre of the window,
16 px above a `wk-footer-bar` along the bottom. While a modal is open it sits in the
top-most modal, and it moves when one opens or closes (as the announcer does), so it
is never stranded or left behind an inert page. A move doesn't replay its entry, and
the document is watched for modals only while a toast is shown. It shows for `timings.toast` (6 s), or `timings.toastWithAction` (10 s) with an
action, and its clock runs only while the window is focused and the pointer and focus
are outside it; it runs on from where it stopped. A newer toast replaces it at once. It
isn't a live region: its words go to the polite announcer once. It never takes focus
when it appears. **F6** (or Shift+F6) moves focus to its action and back to where focus
was; Tab reaches it too. F6 is only used when it moves focus; otherwise it reaches the
page. **Esc** dismisses it while focus is inside (and only the toast: not
a dialog around it or a task that started in the page), and focus goes back; so does pressing the action, which then runs. If a newer toast replaces it while
focus is inside, the new action takes focus (or focus goes back, with no action). An
action needs a `label` and `run()`, and must also exist elsewhere (Undo in Edit > Undo,
Show in the list): the toast goes away by itself. It enters with a short rise and fade
(`--wk-duration-base`) and leaves with a fade in two-thirds of that; with reduced motion
it only fades, in 100 ms. In forced colours its border is `CanvasText`. Never put an
error the person must act on in a toast.

### App undo and redo

The stack lives on the Go side: [`shortcuts.UndoStack`](../../shortcuts/README.md#app-undo-and-redo)
puts it behind Edit > Undo and Redo, named for the action ("Undo Archive").

```js
import { Events } from '@wailsio/runtime';
import { wkUndo, wkToast } from '@wails-kit/ui';
wkUndo.connect({ on: (name, fn) => Events.On(name, e => fn(e.data)), bindings: UndoBindings });

const id = await Archive(conversation.id);              // the Go method pushes and returns the ID
wkToast({ words: 'Conversation archived', action: wkUndo.action(id) });
document.addEventListener('wk-undo-change', e => showWhatCameBack(e.detail));
```

- `connect({ on, bindings })`: `on(name, listener)` subscribes to `undo:changed`,
  `undo:text` and `undo:failed` and may return an unsubscribe; `bindings` is the stack's
  `Bindings()` (`State`, `Undo`, `Redo`, `UndoID`, `SetTextFocus`). It returns a
  function that disconnects.
- **Text focus.** Connected, `wkUndo` reports text focus to Go, and again whenever the
  window comes to the front. Text focus means an editable `input` or `textarea`,
  editable content (a `contenteditable`, which is where the kit's code, Markdown and
  rich-text editors take typing), or an element the app marks `data-wk-undo`. Focus
  elsewhere in an editor (a read-only editor, marked `aria-readonly`; the rich-text
  toolbar; the Markdown Write tab) isn't text focus: Edit > Undo there is the app's, with its "Undo Archive" label.
- **Edit > Undo and Redo with text focus.** Go sends `undo:text`, which arrives in the
  window in front as the DOM event `wk-undo-text`. In the focused window only:
  - The field or editor first gets a cancelable `beforeinput` with `inputType`
    `historyUndo` or `historyRedo`. An editor with its own history (CodeMirror,
    ProseMirror) takes it there.
  - Otherwise a plain field undoes its typing with `execCommand`; when that changes
    nothing, the app's stack goes instead.
  - Editable content (and `data-wk-undo`) never falls back to the app's stack: undo in
    an editor stays in the editor.
- **Clicking the menu with the mouse.** On Windows and Linux, clicking the menu bar may
  take focus from the web view first. The page then reports no text focus, so Edit >
  Undo clicked with the mouse may undo the app's action rather than the typing. The
  shortcut isn't affected. This hasn't been checked on real hardware.
- `action(id?)`: a toast's Undo for the action with this ID (else the most recent). It
  undoes only that action (`UndoID`), so the toast's Undo and Edit > Undo agree; once
  the stack moves past it (Edit > Undo undid it, or another action came), its toast
  closes. A toast replaced by a newer one doesn't take the Undo with it: Edit > Undo
  still has it. Pressed while another undo runs, it says "An undo is already under way"
  (`wkUndo.words.busy`); pressed after the stack moved on, it says nothing.
- `undo()`, `redo()`: the app's stack, for a button.
- `state()`: `{ undo, redo, busy, textFocus }`, each of `undo` and `redo` an
  `{ id, name, label }` or null. Every state from Go also fires `wk-undo-change` on the
  document, for the app to show what came back.
- A failed undo is said in words, assertively: "Couldn’t undo Archive"
  (`wkUndo.words.couldNot(op, name)`). Nothing to undo, a stale toast and a busy stack
  say nothing.

### Copy feedback

```html
<button class="wk-copy" type="button" aria-label="Copy join code" data-wk-copy="K7-42"><svg …></svg></button>
<button class="wk-copy" type="button" aria-label="Copy the token" data-wk-copy-from="token-field"><svg …></svg></button>
<div class="wk-code"><code>npm i @wails-kit/ui</code><button class="wk-copy" type="button" aria-label="Copy command"><svg …></svg></button></div>
```

The kit copies on a click: `data-wk-copy`'s text, the text (or value) of the element
`data-wk-copy-from` names, or, inside a `wk-code`, the `<code>` or `<pre>` beside it.
Otherwise the app calls `wkCopy(button, text)` (it resolves to whether it copied).
"Copied" shows for `timings.copied` (2 s) in a `wk-tooltip wk-copy-tip` above the button
(below it when there's no room), hidden from assistive technology, and goes to the
polite announcer once per press. The button's accessible name and glyph don't change.
On failure it shows and says "Couldn’t copy", assertively while focus is still on the
button. Esc hides the tip while focus or the pointer is on the button; elsewhere Esc is
left to the page (a dialog, a task's Cancel). A busy copy button (`data-busy` with `aria-disabled`) does
nothing. `wkCopy.write` is the clipboard writer (`navigator.clipboard.writeText`);
replace it with Wails's `Clipboard.SetText` where the web view's clipboard isn't
allowed. A writer that rejects or resolves to `false` (as `Clipboard.SetText` does on
failure) didn't copy. An app that copies a `wk-copy` itself, with `wkCopy(button, text)`
from its own click handler, on a button the kit would copy from too (one inside a
`wk-code`, say), calls `preventDefault()` on that click so the kit doesn't copy as
well. `wkCopy.words` holds the words. Each press fires `wk-copy` on the button with
`detail: { ok }` (never the text, which may be a secret).

### Skeleton rows

```js
import { wkSkeleton } from '@wails-kit/ui';
const loading = wkSkeleton(table.tBodies[0]);
const runs = await fetchRuns();
await loading.done(() => drawRuns(table.tBodies[0], runs)); // swapped in one step
```

From the call, the table (or `region`) has `aria-busy="true"` until `done()`. At the
indicator delay (400 ms) the body's rows give way to `<tr class="wk-skeleton"
aria-hidden="true">` rows, one cell per header column (or `columns`), each holding a
still `wk-skeleton-bar` in `--wk-color-fill`, at the real row height: as many rows as
the body had, else as many as the last load drew, else enough to fill the window below
it, up to 12 (or `rows`). `done(draw)` takes them out, calls `draw` in the same step and
resolves; once they have shown, it waits for the indicator minimum (500 ms). Done
within 400 ms, no rows ever show. `cancel()` puts the old rows back. If focus was in the rows that give way, it
moves to the table (made focusable with `tabindex="-1"` until focus leaves), not the
page. Keep headings,
toolbars and filters real and usable. In forced colours each bar is outlined in
`GrayText`.

### Loading chart

`wkChart({ …spec, loading: 'Fetching runs' })` keeps the chart's frame, title, view
switch and, when the series are known (their labels, with no values), its legend; the
figure has `aria-busy="true"`. The plot keeps the chart's height and shows a spinner
(`role="progressbar"`, named by the words) from 400 ms. There's no skeleton for a
chart. When the data arrives, `await wkChart.ready(fig)` (at once if the spinner hasn't
shown, else once it has shown for 500 ms), then replace the figure with the drawn
chart.

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
  stays and a note says "Couldn’t load lines", which the window's
  [announcer](#the-announcer) says (assertively while focus is still on the
  button); while it loads the button is busy (`aria-disabled` and `data-busy`),
  never disabled, so it keeps focus. An empty
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
  which runs after every build of a file's rows. For review comments, use
  the options below instead.

### Review comments

Comments on lines of a diff, as in a pull-request review: threads under
their lines, a "+" button to add one, and a box to write it in. The diff
draws them and keeps open boxes; the app owns the threads and posts them.

```js
const diff = wkDiff({
  id: 'review', diff: patchText,
  comments: threads,                          // the threads, below
  canComment: (file, side, line) => true,     // or true / false; default true when oncomment is set
  oncomment: ({ path, side, line, endLine, body }) => api.post(...),  // a promise, or nothing
  onreply: (threadId, body) => api.reply(...),                       // a promise, or nothing
  onresolve: (threadId, resolved) => api.resolve(...),  // a promise makes Resolve busy until it settles
  onretry: (threadId, entry) => api.retry(...),   // Retry on an entry you marked with error
  commentBox: spec => myEditor(spec),         // optional: the box (below); default a textarea
  commentPlaceholder: 'Leave a comment',      // the box's placeholder (a reply's is "Reply")
  renderBody: entry => renderMarkdown(entry.body),  // optional: default plain text
  author: { name: 'Ana', avatar: 'data:…' },  // who a comment being posted shows as (default "You")
  now: () => Date.now(),                      // the clock for "3 minutes ago"
});
// Later, when threads change (a comment lands, a thread is resolved):
diff.setComments(threads);
// When the diff goes away: closes open boxes (destroying injected editors) and stops its clock.
diff.destroy();
```

- **Threads.** `{ id, path, side: 'old'|'new', line, endLine?, resolved?,
  outdated?, entries: [{ author, avatar?, time, body, bodyHtml?, pending?,
  error? }] }`. `path` is the file's path (or its old path, for a rename);
  `line` and `endLine` are numbers on `side` (a range runs from `line` to
  `endLine`, and the thread sits under `endLine`). `time` is a `Date`, an
  ISO string or milliseconds, shown as relative time ("3 hours ago",
  "yesterday", in the user's locale) in a `<time>` whose `datetime` is
  exact, redrawn with the threads and once a minute. `avatar` is drawn only
  for an `http(s)` or `data:image/` URL, and a redraw reuses its `<img>`.
  A thread's `path` matches a file's path before another file's old path.
  A thread whose line
  (or range) is not shown in one hunk on its side, or which says
  `outdated: true`, is **outdated** and listed at the top of its file with
  "Outdated · line 14". Threads for files not in the diff are not shown.
  `diffCommentPlaces(file, threads)` returns the placement without the DOM.
- **Drawing.** A thread is a `role="region"` named by file and line
  ("Comments on server/server.go, lines 18 to 20"; "old line 13" on the old
  side). Each comment reads as author, time, then body. Then Reply (with
  `onreply`) and Resolve or Unresolve (with `onresolve`). A resolved thread
  collapses to one disclosure, "Resolved · 3 comments", that expands it
  again (and stays expanded through a view switch). In the unified view a
  thread spans the row; in the split view it sits under the side it belongs
  to. Each file header counts its comments ("2 comments"). Bodies are plain
  text (line breaks kept) unless `renderBody(entry)` returns a node; the
  kit never inserts `bodyHtml` itself, so your `renderBody` decides how to
  sanitise it.
- **Adding a comment.** With `oncomment`, line rows take focus and a
  "+" button (`wk-diff-add-comment`) appears in the gutter of the line
  under the pointer or with focus, with an `aria-label` naming the file and
  line. Click it for one line, or drag it across lines for a range;
  Shift-click a second line number to select from the first. On a focused
  line, `c` opens a box (for the selection, if the line is in one). A range
  stays within one hunk and on one side: added and context lines are on the
  new side, removed lines on the old; in the split view, the side of the
  half. The box opens in a row under the line (or range), focused. Escape or
  Cancel closes it and returns focus to the line; Ctrl+Enter or Cmd+Enter,
  or the Comment button, sends it (not while an input method is composing).
  Several boxes can be open at once, and
  each keeps its draft through a view switch. `canComment(file, side,
  line)` returning false leaves a line without the button.
- **Keyboard.** With commenting on, the arrow keys move between lines and
  hunk headers in a file; in the split view, left and right move between the
  halves of a line (the focus ring marks the half). Shift with Up or Down
  selects lines (announced: "Lines 3 to 5 selected"), and Escape clears the
  selection. Anywhere in the diff,
  `N` and `P` move to the next or previous thread (in expanded files),
  alongside `n`/`p` and `]`/`[`.
- **Pending and failed.** When `oncomment` or `onreply` returns a promise,
  the comment shows at once as pending: secondary text, with a working dot
  and "Posting…" after `timings.indicatorDelay` (400 ms); once shown, the
  dot stays at least `timings.indicatorMin` (500 ms). When the promise
  resolves, the copy stays, no longer pending, until your comments hold it,
  checked then and at each `setComments`, or for 10 s at most. They hold
  it when a thread (for a reply, an entry) has the id the promise resolved
  with (`{ id }`, or the id itself), or, with no id, when an entry has the
  same text and author (so set `author` to match your own comments).
  When it rejects, the comment shows "Couldn't post the comment" with the
  error in an ErrorBox (`wk-errorbox`, with a copy button), Retry (sends it
  again) and Edit (puts the text back in a box: a new one, or added to the
  end of a box already open there; if that box's `value` can't be set, the
  failed comment stays). Outcomes, like selections, are said politely by the
  window's [announcer](#the-announcer); the diff has no live region of its own.
  If the callback returns nothing, the app shows the comment itself; mark an
  entry `pending: true` to show it pending (the dot shows at once), or
  `error: 'raw message'` to show the ErrorBox, with Retry calling
  `onretry(threadId, entry)`.
- **Resolving.** When `onresolve` returns a promise, Resolve (or Unresolve)
  is busy until it settles: `data-busy` and `aria-disabled="true"` at once
  (it keeps focus and its name, and another press does nothing), and a
  `wk-spinner` after `timings.indicatorDelay` that stays at least
  `timings.indicatorMin`. Update the thread with `setComments`; a rejection
  is announced ("Couldn't resolve the comments").
- **Review mode and layout.** With comments or commenting on, the diff has
  `data-review`, and each file's body is an inline-size container, so a
  comment row keeps the file's width while the code scrolls sideways. The
  body then takes its width from its parent: inside a shrink-to-fit parent
  (an `inline-block`, a float, an `auto` grid track), give the diff a
  width. Without review, the diff sizes as before.
- **Redrawing.** A box opening or closing, a thread expanding, a comment
  being sent and a resolve redraw only the row they are in; `setComments`
  redraws the threads of every built file. A reply box with text outlives
  its thread: when `setComments` drops the thread, it stays, listed as
  outdated, until the reply is sent or cancelled (an empty one goes with
  the thread). After `destroy()`, a resolve still in flight draws nothing.
  Focus stays where it was: in an
  open box, on the same button of the same thread, else on the next
  thread, the file's toggle, or the diff.
- **The comment box.** `commentBox(spec)` makes the box, so an app can use
  an editor (the Markdown editor in `@wails-kit/editor`, say) while this
  package stays dependency-free. It gets

  ```ts
  { value: string,        // the text to start with (a draft put back by Edit)
    placeholder: string,  // "Leave a comment", or "Reply"
    label: string,        // the accessible name the box must carry
    onSubmit: () => void, // call to send (from your own Mod-Enter, say)
    onCancel: () => void } // call to close (from your own Escape)
  ```

  and returns `{ element, value, focus(), destroy() }`: `element` goes in
  the frame above Cancel and Comment (which the diff draws); `value` is
  read when the comment is sent; `focus()` is called when the box opens or
  is reopened; `destroy()` when it closes. `focus` and `destroy` are
  optional, and returning a bare element (with `value` and `focus`) also
  works. The element is moved, not rebuilt, when the view switches, so an
  editor keeps its state. If the box doesn't handle Escape or Ctrl/Cmd+Enter
  (it didn't call `preventDefault()`), the diff does. If the factory
  throws, the default box is used: a `wk-field` textarea
  (`wk-diff-compose-field`).
- **With `@wails-kit/editor`'s Markdown editor.** An adapter of a few
  lines; this package imports nothing from the editor. The editor's element
  has `value`, `focus()` and `destroy()`, so it is returned as is; leave
  out its `actions`, since the diff draws Cancel and Comment.

  ```js
  import { wkMarkdownEditor, renderMarkdown } from './vendor/wails-kit-editor/markdown.js';

  const open = url => Browser.OpenURL(url);
  wkDiff({
    // …
    commentBox: ({ value, placeholder, label, onSubmit, onCancel }) =>
      wkMarkdownEditor({ value, placeholder, label, onlink: open, onsubmit: onSubmit, oncancel: onCancel }),
    renderBody: entry => {
      const body = document.createElement('div');
      body.className = 'wk-markdown';
      body.append(renderMarkdown(entry.body, { onlink: open }));
      return body;   // or renderMarkdownElement(entry.body, { onlink: open }), where the editor has it
    },
  });
  ```

  The diff closes the box as soon as the comment is sent and shows it as
  pending itself, so `onSubmit` returns nothing. Keys typed in the box
  (including in a disabled editor, `contenteditable="false"`) never reach
  the diff's single-key shortcuts.
- **Classes** (proposed for the design system): `wk-diff-comments` (the
  row under a line), `wk-diff-outdated`, `wk-diff-thread` (with
  `data-side`, `data-resolved`, `data-outdated`, `data-local` while
  posting), `wk-diff-thread-head`, `wk-diff-thread-toggle`,
  `wk-diff-thread-body`, `wk-diff-comment-list`, `wk-diff-comment` (with
  `data-state="pending"` or `"error"`), `wk-diff-comment-head`,
  `wk-diff-avatar`, `wk-diff-author`, `wk-diff-time`, `wk-diff-comment-body`,
  `wk-diff-comment-state`, `wk-diff-errorbox`, `wk-diff-comment-actions`,
  `wk-diff-thread-actions`, `wk-diff-compose`, `wk-diff-compose-field`,
  `wk-diff-compose-actions`, `wk-diff-add-comment`,
  `wk-diff-comment-count`; selected lines carry `data-selected`, and a
  focused split row `data-focus-side`. In forced colours the selection is a
  `Highlight` outline, the + button a `ButtonText` border on `ButtonFace`,
  and pending text `GrayText`.

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
import { GetSchema, GetValues, SetValues, CheckValue, CheckFailures } from './bindings/.../service';

const el = document.querySelector('wk-settings');
el.schema = await GetSchema();
el.values = await GetValues();
el.check = CheckValue; // for fields with a back-end check
el.errors = (await CheckFailures()) ?? [];
el.addEventListener('wk-settings-change', async e => {
  if (e.detail.checked) return; // CheckValue saved it already
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
| `check` | optional `(key, values) => Promise<ValidationError[] \| null>` for fields with a `check` in the schema: the `CheckValue` binding. Every value of such a field goes to it, an empty one included. No errors means the value was saved; errors on the field are its failure; errors only on other fields mean it wasn't saved (they show on their fields, and this one says "Your … wasn't saved."). Without it, those fields apply on change like any other |

| Method | |
|---|---|
| `reveal(key)` | selects the field's tab, opens Advanced if it's there, and focuses its control; `false` when the field isn't shown. Use it to open Settings on a field whose check failed |

| Attribute | |
|---|---|
| `label` | the tab list's accessible name (default "Settings") |

**Event** `wk-settings-change` (bubbles), when a value applies (a control's
`change`: a text field on blur or Enter; for a checked field, once its check
passes): `detail` is `{ key, value, values, errors, checked }`; `checked` is
true when the value passed a back-end check, which has saved it. `value` has the schema's type (a boolean for a toggle, a number or
`null` for a number field, a string otherwise); `values` holds every field;
`errors` is what `validate` returned (`[]` without it). Without `validate`,
the shown error on the changed field clears and the others stay until the app
sets `errors`.

**Validation timing** follows [BEHAVIOUR.md](BEHAVIOUR.md#forms-and-validation):

- A field is checked when it commits, not on each keystroke. Once it shows an
  error, it is re-checked as the person types (with `validate`, or for an empty
  required field), so the error clears as soon as it's fixed.
- An empty required field shows "<label> is required" from the moment the window
  opens. The window's Notice, between the tabs and the pane (under the heading
  with one group), counts the visible fields with an error: "2 settings need
  attention." It is empty when nothing needs attention. Its words go to the
  window's [announcer](#the-announcer) when the count changes, not when the
  window is drawn (where the Notice is read with it). While the person types,
  the field and its tab follow each keystroke, but the Notice's count waits
  until typing stops (500 ms) and its words change only when the count does.
- A field with a back-end check (`check: { pending }` in the schema, and the
  `check` property set) applies only once the check passes. The check runs on
  commit and, once the field shows an error, 500 ms after typing stops. Each
  check is a [`wkTask`](#wktask) whose target is the field's
  `wk-settings-pending` line: its words (`pending`, else "Checking…") show there,
  with the working dot, after 400 ms, are announced then, and stay at least
  500 ms; the row has `data-pending`, and the control is described by the words,
  meanwhile. The check runs with no timeout (`CheckValue` carries on in Go). When
  it ends, the words go and its outcome goes to the window's announcer, in the
  same step as the value applies or the field shows its error: "Projects folder
  checked", or the failure's message. The task ends Done either way (the field
  shows a failure), and checks are not in the task list (`listed: false`).
  Typing is never blocked, and the person's text stays after a failure. A check
  overtaken by a newer one is ignored, its answer neither applied nor said:
  before its words show, its task is cancelled; once they show, the newer check
  keeps them, so they don't go and come back; and once its answer is in (its
  words held for their 500 ms), every newer check's words wait until they have
  gone, as they do after a shown check abandoned for the window's own error. A check abandoned because the window's own validation rejects the value,
  or because a new `schema` or `icons` redraws the window, is cancelled without
  saying "Cancelled". In a modal Sheet the announcer is inside it, so all this
  is heard. An error with
  code `check` (a failure, or one from `CheckFailures()`) stays on its field until
  that field commits or is checked again, even when the app sets `errors` or
  another field changes; when the app sets `values` with a new value for that
  field, it goes, with any unapplied text.
- A value the window's own validation rejects (on commit, or a live draft once
  typing stops) still goes to `check`, and the reply is ignored: `CheckValue`
  validates it, saves nothing, and overtakes the check of the value it replaced,
  so that check can't save it.
- A check that outlives the window carries on in Go (`CheckValue`): if it fails,
  `settings:check_failed` lets the main window show a Notice with "Open Settings";
  Settings then shows `CheckFailures()` and calls `reveal(key)`. See the
  [settings package](../../settings/README.md#back-end-checks).

The rules the window follows are exported with no DOM, for tests and other
renderers: `settingsModel(schema, values, errors)` (with `attention`, the
count for the Notice), `conditionMet`,
`resolveOptions`, `fieldValue`, `readValue`, `displayValue`,
`dependentResets`, `errorsByField` and `SECRET_MASK`.

The gallery's Settings window page shows the window on a sample schema.

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
| `dist/wails-kit-ui.css`, `dist/wails-kit-ui.js` | the prebuilt files, committed and checked against `src/` |
| `scripts/build.mjs` | builds `dist/` with no dependencies |
| `test/` | the checks |

## Develop

```sh
npm ci
npm run build          # after any change in src/
npm test               # static checks: node --test
npm run test:browser   # the package's files on their own, in Chrome
```

`npm test` checks that every `var(--wk-…)` used is defined, that every colour
token has light and dark values (and the same values in both dark blocks),
that themes set only theme tokens and meet the contrast rules, that colours in
component rules come from tokens, that the code syntax colours reach 4.5:1 on
the control colour, that every selector is scoped, that nothing
loads from outside the package or names an app, the chart's layout rules, the
diff's parser, pairing and word marks, where review threads go, the contrast of
text on the diff tints, the settings window's rules, and that `dist/` matches a fresh build.

`npm run test:browser` loads the package's files on their own, as an app
vendors them, with `playwright-core` and Chrome (set `CHROME_PATH`, or have it
at `/usr/bin/google-chrome` or the macOS default; without Chrome it skips,
unless `CI` is set). Its server serves this folder and nothing else, so it
checks what must stay in the package: that `src/` and `dist/` load nothing
from outside it and load no font file, that their text is in `system-ui`,
and that the prebuilt module exports what `src/index.js`
exports and defines the same elements.

The components are checked in the [component gallery](../gallery/README.md)'s
browser tests (`frontend/gallery`, `npm run test:browser`), on its pages: the
WAI-ARIA keyboard patterns of every custom element, focus return, imperative
construction and clean-up, forced colours, the chart, the diff (lines in both
views, keyboard, copy, forced colours, a 5,000-line diff, review comments), the
settings window, `wkTask`, the announcer, the task list, the toast, app undo,
copy feedback and skeleton rows. The kit targets WebKit (Wails on macOS) as
well as WebView2; these checks run in Chrome, so check WebKit by hand on a Mac
for a change in behaviour.

Chart callers can preserve the active category across replacement by retaining the index reported by `onselection(index)` and passing it as `selection` to the next `wkChart` call. `-1` means no selection. Pointer and keyboard interaction, clearing, and restoring a selection may invoke the callback; an out-of-range positive index is clamped to the last category. Callers manage focus when replacing a chart.

Each chart uses its `id` (or a generated ID) to name its focusable plot (`<id>-plot`), view radios (`<id>-view-chart` and `<id>-view-table`), and table (`<id>-table`). A caller replacing a chart can restore focus to the corresponding ID. By default, blurring the plot clears selection; set `retainSelectionOnBlur: true` to retain it. Escape still clears selection, and pointer leave clears it when the plot is not focused.
