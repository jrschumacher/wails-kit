# Theming @wails-kit/ui

Every app on the kit ships one theme file on the same base. This page says what
a theme may change, what the kit works out for it, and how the state tones are
used. It follows the Wails Kit Design System's `theming.md`
([`docs/design-system/`](../../docs/design-system/)), with one change: text
fields and similar controls have their own border (see the field border, below).

## What a theme is

One small stylesheet, linked after the kit's. It sets custom properties on
`:root` (light), again under `@media (prefers-color-scheme: dark)` on
`:root:not([data-theme="light"])`, and again on `:root[data-theme="dark"]`, with
the same values in the two dark blocks. Nothing else: no component selectors, no
new classes. [`theme/theme.css`](theme/theme.css) lists every token a theme may
set, holding the kit's defaults; copy it and change the values.
[`theme/example.css`](theme/example.css) is a worked example.

## Must set: identity

| Token | What it is |
|---|---|
| `--wk-color-accent`, `--wk-color-on-accent` | The app's colour: primary button, links, focus ring, selected glyph. Text on the accent reaches 4.5:1. |
| `--wk-color-working` | Something is running now. |
| `--wk-color-attention`, `--wk-color-attention-text` | A person should act; attention words and field errors. |

Each has a light and a dark value.

## May set

**Grounds: all ten or none.** A theme may tint the neutrals so its windows feel
like its own: `--wk-color-bg`, `--wk-color-sidebar`, `--wk-color-surface`,
`--wk-color-control`, `--wk-color-hairline`, `--wk-color-hairline-strong`,
`--wk-color-control-border`, `--wk-color-field-border`, `--wk-color-text` and
`--wk-color-text-secondary`. Set all ten or none, so no ground is left from the
other palette. Keep the hue shift small (a warm or cool bias, not a colour).
Then text and secondary text must reach 4.5:1 on all four grounds (`bg`,
`sidebar`, `surface`, `control`), and the field border 3:1 on `bg` and `surface`.

**Font fallbacks.** `--wk-font-family` may add fallbacks after the kit's first
family, never replace it. A font the theme names is the app's to bundle: an
`@font-face` in the theme pointing at a file the app ships. Never load a font
from the network.

**Radii.** `--wk-radius-control`, `--wk-radius-group` and `--wk-radius-window`,
each within 2px of the kit's (5, 7 and 10px).

## Must not set: derived

The kit works out four colours from the ones above:

| Token | Light | Dark |
|---|---|---|
| `--wk-color-selection` | accent at 12% | accent at 16% |
| `--wk-color-chart-1` | accent | accent |
| `--wk-color-destructive-text` | attention text | attention text |
| `--wk-color-attention-fill` | attention at 8% | attention at 12% |

It does this with `color-mix()` inside
`@supports (color: color-mix(in srgb, red, blue))`. Custom properties don't fall
back on their own: a `var()` that resolves to an unsupported value makes the
whole declaration invalid, not the earlier value. That is why the derivation
sits inside `@supports` rather than relying on the cascade.

Where `color-mix()` is missing, the kit's literals apply. They are the kit's own
neutral defaults. A theme that must look right on such an engine may repeat its
own four values, exactly as the kit would derive them, inside
`@supports not (color: color-mix(in srgb, red, blue)) { … }`, in the same three
blocks. Only those four go there.

## Held by the kit

Every other token stays the kit's. That includes the fills, `on-attention`, the
switch thumb, the tooltip colours, chart series 2 to 5, and
`--wk-color-text-secondary-on-selected`, the darker secondary text on a selected
row or a count chip. The check below measures them against the theme's colours
all the same.

## The field border

`--wk-color-control-border` is soft (about 1.5:1). It outlines buttons and
segmented controls, which their face fill, shadow and label already set apart.
Text fields, selects, search fields, steppers, checkboxes, radios and an off
switch are known only by their outline, so WCAG 1.4.11 applies to them. They use
`--wk-color-field-border`, at 3:1 or more on `bg` and `surface`: by default
`#868c96` in light (3.3:1 on the window, 3.4:1 on a surface) and `#707682` in
dark (3.8:1 and 3.5:1). The control border has no minimum.

## Where color-mix() works

Wails uses the operating system's web view, so support follows the OS, not the
app.

| Platform | Web view | `color-mix()` from | In practice |
|---|---|---|---|
| macOS | WKWebView (system WebKit, updated with Safari) | Safari 16.2, December 2022 | macOS 13 Ventura and later; macOS 11 and 12 once Safari is 16.2 or later. macOS 10.15 stops at Safari 15.6 and gets the fallback. |
| Windows | WebView2 (Chromium) | Chromium 111, March 2023 | The Evergreen runtime updates itself, so every supported Windows 10 and 11 machine has it. Only an app that pins a Fixed Version runtime older than 111 falls back. |
| Linux | WebKitGTK | 2.40, March 2023 | **2.40 is the minimum for a desktop window.** It is also the first release with the WebKitGTK 6.0 API that a GTK 4 build of Wails needs. On anything older, run the app headless (its daemon only) instead of falling back. |

## State tones

The kit has exactly three tones. Every state in every app is one of them. Its
status dot's shape and its words carry the state; the tone only adds urgency.

| Tone | Token | Means | Examples |
|---|---|---|---|
| Neutral | `--wk-color-text` | Nothing for anyone to do, whatever the state. | Done, idle, queued, paused, offline, skipped, backing off |
| Working | `--wk-color-working` | Something is happening right now and will finish on its own. | Running, syncing, finishing |
| Attention | `--wk-color-attention` | A person should look or act. | Stalled, failed, can't start, needs a decision |

Rules:

- **There is no warning tone.** Yellow and orange read as "warning" in most
  software, so the kit doesn't use them for anything a person must notice. If it
  needs a person, it is attention. If it doesn't, it is neutral, and the words
  say what is going on ("Backing off until 11:05", "Retrying").
- **Working is motion, not a caution.** The working colour only ever sits on
  something that is running now, beside a word that says so. Never use it for
  "expiring soon", "degraded", "almost full" or "needs review".
- **Attention when a person is the next step.** A failure the system retries by
  itself is neutral ("Retrying"). A failure that stays failed until someone acts
  is attention. A state that needs a decision is attention.
- **Tell working and attention apart by lightness as well as hue.** In each
  appearance the two must differ by at least 1.5:1 in contrast with each other,
  and each must reach 3:1 as a mark on the window. Shape still carries the state
  for anyone who can't tell them apart.
- **Keep chart colours that look like a tone away from dots.** Never draw a chart
  next to a working or attention dot without a legend.
- **No other colour carries state.** Not the accent, not a chart colour, not
  green for "success". Success is the neutral dot and the word "Done".

## Checking a theme

[`test/theme-check.mjs`](test/theme-check.mjs) is this page as code. The
package's tests run it on the kit's defaults, on `theme/theme.css`, on
`theme/example.css` and on a theme that tints all ten grounds
(`test/fixtures/warm-grounds.css`). `checkTheme(themeCss, kitCss)` returns the
problems, `[]` when the theme keeps the contract. It fails when a theme:

- sets a derived colour, a token outside the lists above, or a colour without a
  dark value, or leaves out an identity colour;
- sets some grounds but not all ten;
- has a radius more than 2px from the kit's, or replaces the kit's first font;
- has text or secondary text under 4.5:1 on any of the four grounds, in either
  appearance;
- has a field border under 3:1 on `bg` or `surface`;
- has working and attention closer than 1.5:1 to each other, or either under
  3:1 on the window;
- has text on the accent, or the kit's secondary text on a selected row or count
  chip, under 4.5:1.

Each problem names its rule and appearance (`working-on-bg/light`). A known gap
is recorded as a named exception, never by loosening the rule:
`checkTheme(css, kit, { exceptions: { 'working-on-bg/light': 'why' } })`. An
exception that no longer matches a problem is reported too. The tests show one,
for a gold working tone at 2.6:1 on the light window
(`test/fixtures/gold-working.css`), a gap the design system records.
