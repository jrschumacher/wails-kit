# Theming and tone

These rules belong to the kit (`wk-`, the future `@wails-kit/ui`), not to any one app. Muster and Peanut each ship one theme file on the same base; this section says what a theme may change, what the kit works out for it, and how the state tones are used.

## What a theme sets

A theme is one small stylesheet loaded after the kit. It sets custom properties on `:root, [data-theme="light"]` and again on `[data-theme="dark"]`, and nothing else: no selectors for components, no new classes.

**Identity (required, about ten values).**

| Token | Per theme | What it is |
|---|---|---|
| `wk-color-accent`, `wk-color-on-accent` | yes | The app's colour: primary button, links, focus ring, selected glyph. |
| `wk-color-working` | yes | Something is running now. |
| `wk-color-attention`, `wk-color-attention-text` | yes | A person should act. |
| `wk-font-family` | no | Inter by default; a theme may only add fallbacks. |
| `wk-radius-control`, `wk-radius-group`, `wk-radius-window` | no | Corner rounding, within 2px of the kit's 6, 8 and 10. |

**Grounds (optional).** A theme may tint the neutrals so its windows feel like its own: `wk-color-bg`, `wk-color-sidebar`, `wk-color-surface`, `wk-color-control`, `wk-color-hairline`, `wk-color-hairline-strong`, `wk-color-control-border`, `wk-color-text`, `wk-color-text-secondary`. Set all of them or none, so no ground is left from the other palette. Keep the hue shift small (a warm or cool bias, not a colour), and re-check: text and secondary text at 4.5:1 or better on all four grounds, control borders at 3:1 on the window.

**Derived (do not set).** The kit works out four colours from the ones above:

| Token | Light | Dark |
|---|---|---|
| `wk-color-selection` | accent at 12% | accent at 16% |
| `wk-color-chart-1` | accent | accent |
| `wk-color-destructive-text` | attention text | attention text |
| `wk-color-attention-fill` | attention at 8% | attention at 12% |

The kit does this with `color-mix()` inside `@supports`. Where `color-mix()` is missing, the literals in `tokens.json` apply instead; they are Muster's colours, so a theme that must look right on old engines repeats its own four values inside `@supports not (color: color-mix(in srgb, red, red)) { … }`.

## Where color-mix() works

Wails uses the operating system's web view, so support follows the OS, not the app.

| Platform | Web view | color-mix() from | In practice |
|---|---|---|---|
| macOS | WKWebView (system WebKit, updated with Safari) | Safari 16.2, December 2022 | macOS 13 Ventura and later; macOS 11 Big Sur and 12 Monterey once Safari is updated to 16.2 or later. macOS 10.15 Catalina stops at Safari 15.6 and gets the fallback. |
| Windows | WebView2 (Chromium) | Chromium 111, March 2023 | The Evergreen runtime updates itself, so every supported Windows 10 and 11 machine has it. Only an app that pins a Fixed Version runtime older than 111 would fall back. |
| Linux | WebKitGTK | 2.40, March 2023 | **2.40 is the minimum for a desktop window.** It is also the first release with the WebKitGTK 6.0 API that a GTK 4 build of Wails already needs. On anything older, run the app headless (its daemon only) instead of falling back. |

Custom properties do not fall back on their own: a `var()` that resolves to an unsupported value makes the whole declaration invalid, not the earlier value. That is why the derivation sits inside `@supports` rather than relying on the cascade.

## State tones

The kit has exactly three tones. Every state in every app is one of them, and its StatusDot shape and words carry the state; the tone only adds urgency.

| Tone | Token | Means | Examples |
|---|---|---|---|
| Neutral | `wk-color-text` | Nothing for anyone to do, whatever the state. | Done, idle, queued, on break, off duty, offline, skipped, backing off |
| Working | `wk-color-working` (gold to amber) | Something is happening right now and will finish on its own. | Running, posting, standing down |
| Attention | `wk-color-attention` (red) | A person should look or act. | Stalled, failed, can't start jobs, no machine for a job type |

Rules:

- **There is no warning tone.** Yellow and orange read as "warning" in most software, so the kit does not use them for anything a person must notice. If it needs a person, it is red. If it does not, it is neutral and the words say what is going on ("Backing off until 11:05", "Retrying").
- **Working is motion, not a caution.** The gold or amber mark only ever sits on something that is running now, beside a word that says so. Never use it for "expiring soon", "degraded", "almost full" or "needs review".
- **Red when a person is the next step.** A failure the system retries by itself is neutral ("Retrying"). A failure that stays failed until someone acts is red. A state that needs a decision (a stalled job to retry) is red.
- **Tell working and attention apart by lightness as well as hue.** In each theme the two must differ by at least 1.5:1 in contrast with each other, and each must reach 3:1 as a mark on the window. Shape still carries the state for anyone who cannot tell them apart.
- **Keep chart oranges and golds away from dots.** Chart series 3 and 5 are gold and orange; never draw a chart next to a working or attention dot without a legend.
- **No other colour carries state.** Not the accent, not a chart colour, not green for "success". Success is the neutral dot and the word "Done".

## The two themes today

| | Muster | Peanut |
|---|---|---|
| Accent | Steel blue `#2f4f8f` / `#8fb2f0` | Gallery green `#2d6a45` / `#84c99a` |
| Working | Gold `#c9961a` / `#e2b23e` (2.6:1, known gap) | Amber `#b4740f` / `#e4a94a` (3.7:1) |
| Attention | Red `#c8402f` / `#ef6a5e` | Deeper red `#a9372a` / `#e8604f` |
| Grounds | Kit neutrals (cool) | Warm neutrals |
| Radii | 6 · 8 · 10 | 7 · 10 · 12 |
