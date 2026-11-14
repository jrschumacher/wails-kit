# @wails-kit/ui

Base UI for Wails desktop apps: design tokens in light and dark, a type scale,
CSS components on native elements, a few custom elements where behaviour needs
them, charts, a small theme file an app overrides, and a settings window drawn
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
anything else. The first line of each file names the package version; the tag,
commit and checksums recorded above identify the exact vendored release.

## Light and dark

The page follows the system appearance. `<html data-theme="light">` or
`data-theme="dark"` overrides it, for the tokens and for `color-scheme`. Every
colour token has a light value on `:root` and a dark value under
`@media (prefers-color-scheme: dark)` on `:root:not([data-theme="light"])`,
repeated on `:root[data-theme="dark"]`.

## The theme file

[`theme/theme.css`](theme/theme.css) lists the values an app sets to make the
kit its own, with the kit's defaults. Copy it into the app, change the values,
and link it after the kit's stylesheet. A theme sets only these:

| Group | Tokens | What they colour |
|---|---|---|
| Accent | `--wk-color-accent`, `--wk-color-on-accent`, `--wk-color-selection` | primary buttons, links, the focus ring, selected rows and tabs |
| Working | `--wk-color-working`, `--wk-color-working-ring` | the "something is happening" tone; the ring is a darker step where working is under 3:1 on the background |
| Attention | `--wk-color-attention`, `--wk-color-attention-text`, `--wk-color-attention-fill`, `--wk-color-destructive-text` | someone must act; destructive buttons |
| Charts | `--wk-color-chart-1`, `--wk-color-chart-3` | the series that usually follow the accent and working |
| Type | `--wk-font-family`, `--wk-font-features` | the font and its OpenType features |
| Corners | `--wk-radius-control`, `--wk-radius-group` | controls; groups and panels |

Each colour needs a light value and a dark value, written in the two dark
blocks with the same values. A font the theme names is the app's to bundle:
add an `@font-face` in the theme that points at a file the app ships, and never
load a font from the network. [`theme/example.css`](theme/example.css) is an
example theme (a green accent, an amber working tone, rounder corners), shown
on `demo/themed.html`.

The package's checks hold every theme in `theme/` to these rules: only theme
tokens, light and dark for each colour, the same values in both dark blocks,
4.5:1 for secondary text on a selection and for text on the accent, and 3:1
for the working ring on the background. An app should check its own theme the
same way.

## Components

CSS (`src/components.css`): each is a class on a native element; its states
come from native attributes (`disabled`, `checked`, `aria-pressed`,
`aria-selected`, `aria-expanded`, `aria-invalid`, …).

| Component | Classes |
|---|---|
| Type scale | `wk-title`, `wk-title-2`, `wk-heading`, `wk-subheading`, `wk-body`, `wk-caption`, `wk-small`, `wk-secondary`, `wk-num`, `wk-mono`, `wk-figure` |
| Button | `wk-button` with `--primary`, `--destructive`, `--quiet`, `--small`, `--icon`, `--locked` |
| Link | `wk-link` (an `aria-expanded` link draws a disclosure chevron) |
| Field | `wk-label`, `wk-field`, `wk-help`, `wk-error`; search field `wk-search` |
| Checkbox, radio, switch | `wk-check`, `wk-checkbox`, `wk-radio`, `wk-switch` (`role="switch"`); `wk-check--row`, `wk-check--top` |
| Select | `wk-select` |
| Filter buttons, segmented control | `wk-filters` / `wk-filter`, `wk-segmented` |
| Chip and pill | `wk-chip`, `wk-pill` |
| Table | `wk-table` |
| Sidebar, toolbar, footer bar | `wk-sidebar`, `wk-toolbar`, `wk-footer-bar` |
| Three-pane layout | `wk-panes`, `wk-pane-list`, `wk-pane-inspector` |
| Tabs | `wk-tablist` inside `<wk-tabs>` |
| Menu, context menu, tooltip | `wk-menu` and its items, `wk-tooltip` |
| Dialog, sheet, popover surface | `wk-dialog` (`--alert`, sheets), `wk-popover` |
| Settings group, row and tabs | `wk-settings-heading`, `wk-settings-group`, `wk-settings-row`, `wk-settings-label`, `wk-settings-help`, `wk-settings-note`, `wk-settings-tabs` / `wk-settings-tab` (`data-attention`) |
| Panel, facts, section heading | `wk-panel` (`--rows`, `--dashed`), `wk-facts`, `wk-section-heading` |
| Status dot | `wk-dot` with `data-shape` (`filled`, `ringed`, `half`, `hollow`, `dashed`, `bar`), `data-tone` (`neutral`, `working`, `attention`) and `data-size="sm"`. It has no state names: the app maps its own states onto shapes and tones. |
| Empty state, notice, error box | `wk-empty`, `wk-notice`, `wk-error-box` |
| Stat, code block, copy button | `wk-stat`, `wk-code`, `wk-copy` |
| Stepper | `wk-stepper` (an input and two arrow buttons; see `syncSteppers`) |
| Chart | `wk-chart`, built by `wkChart()` |

`demo/index.html` shows every component in each state it can show without a
pointer, the tokens and every status dot. Serve the package folder (for
example `npx serve .`) and open `/demo/index.html`.

### Custom elements and functions

| Export | What it is |
|---|---|
| `<wk-dialog>` (`WkDialog`) | wraps a native `<dialog>` and opens it modally; focus returns to the opener; `open` attribute; `[data-wk-close]` buttons set `returnValue`; `light-dismiss` attribute; events `wk-open`, `wk-close`. Any element with `data-wk-open="<id>"` opens the dialog with that id. |
| `<wk-menu>` (`WkMenu`) | a WAI-ARIA menu button: a `<button>` followed by a `[role=menu]`; event `wk-select`, `wk-toggle` |
| `<wk-context-menu for="id">` (`WkContextMenu`) | the same menu, opened by right-click, Shift+F10 or the Menu key; `detail.context` is the element it opened on |
| `<wk-tabs>` (`WkTabs`) | WAI-ARIA tabs with automatic activation; event `wk-change` |
| `<wk-tooltip>` (`WkTooltip`) | WAI-ARIA tooltip on focus and after a hover delay (`delay="ms"`); event `wk-toggle` |
| `syncSteppers(root?)` | redraws stepper arrows after the app sets a value itself |
| `wkChart(spec)` | bars, stacked bars, or a line with a band, as plain SVG, with a legend, hover and keyboard readout, and a Chart/Table switch; colours from `--wk-color-chart-1` to `-5` (`tone` 1 to 5 or `"neutral"`) |
| `niceTicks`, `labelStep`, `runs`, `roundedTop`, `roundedEnd`, `tableModel`, `readout` | the chart's layout rules, with no DOM |

Every element works when the app builds the DOM imperatively: it may be
connected before its children exist, moved, or removed while open, and it
removes everything it added to the document when it is disconnected.

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
| `src/index.js` | the module entry: `kit.js` (custom elements), `chart.js`, `settings-model.js`, `settings.js` |
| `src/index.d.ts` | TypeScript types |
| `theme/theme.css`, `theme/example.css` | the theme file and an example theme |
| `dist/wails-kit-ui.css`, `dist/wails-kit-ui.js` | the prebuilt files, committed and checked against `src/` |
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
component rules come from tokens, that every selector is scoped, that nothing
loads from outside the package or names an app, the chart's layout rules, the
settings window's rules, and that `dist/` matches a fresh build.

`npm run test:browser` drives the review pages with `playwright-core` and
Chrome (set `CHROME_PATH`, or have it at `/usr/bin/google-chrome` or the macOS
default; without Chrome it skips, unless `CI` is set). It covers the
WAI-ARIA keyboard patterns of every custom element, focus return, imperative
construction and clean-up, forced colours, the chart, the prebuilt module's
exports, and the settings window. The kit targets WebKit (Wails on macOS) as
well as WebView2; these checks run in Chrome, so check WebKit by hand on a Mac
for a change in behaviour.

Chart callers can preserve the active category across replacement by retaining the index reported by `onselection(index)` and passing it as `selection` to the next `wkChart` call. `-1` means no selection. Pointer and keyboard interaction, clearing, and restoring a selection may invoke the callback; an out-of-range positive index is clamped to the last category. Callers manage focus when replacing a chart.

Each chart uses its `id` (or a generated ID) to name its focusable plot (`<id>-plot`), view radios (`<id>-view-chart` and `<id>-view-table`), and table (`<id>-table`). A caller replacing a chart can restore focus to the corresponding ID. By default, blurring the plot clears selection; set `retainSelectionOnBlur: true` to retain it. Escape still clears selection, and pointer leave clears it when the plot is not focused.
