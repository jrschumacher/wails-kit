# SettingsTabs

The Settings window's pane buttons along the top, each a 22px glyph over its name: General, Lending, HQ, About.

## When to use

Only at the top of the Settings window (Muster › Settings…, ⌘,). It opens on the last pane used. Elsewhere, use SegmentedControl for a few views of one thing.

## What you supply

A `role="tablist"` container with `aria-label="Settings"` and one `<button class="wk-settings-tab" role="tab">` per pane: the glyph (`i-gen`, `i-lend`, `i-hq`, `i-about`) inline with `currentColor`, then the pane name. `aria-selected` and roving `tabindex` as for any tab list; `aria-controls` points at the pane. A pane that holds a field to review gets `data-attention` and an `aria-describedby` that says so.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-settings-tabs` | Centred row, `wk-space-1` gaps, a `wk-color-hairline` rule under it. |
| `wk-settings-tab` | Glyph over a `wk-font-size-xs` name, at least 60px, `wk-radius-control`; hover `wk-color-fill`. |
| `aria-selected="true"` | `wk-color-fill-strong` ground, glyph in `wk-color-accent`. |
| `data-attention` | A 7px `wk-color-attention` dot at the top right. |

## Do and don't

- Do keep pane names to one word.
- Don't add a pane for a single setting; put it in General.
- Don't use the attention dot for anything but a field that needs review.

## Replaces

Design page: `.setw .stb`, `.panes`, `.pane`, `.pane[aria-current]`. App: `.settings-tabs`, `.settings-tab`, `.settings-tab.attention`.
