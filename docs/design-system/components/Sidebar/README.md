# Sidebar

HQ's section list down the left of the window: Now, History, Machines (with each machine under it), Repos, then This Mac, and the HQ's address at the foot.

## When to use

In the HQ window only, `wk-sidebar-width` (220px) wide. Below about 900px it hides; the toolbar's sidebar button, View › Show Sidebar and ⌃⌘S bring it back.

## What you supply

- A `<nav class="wk-sidebar" aria-label="HQ sections">` holding `<ul>` lists of `<a class="wk-sidebar-item">` (or `<button>`).
- Each item: a 16px glyph from the icon set (`i-now`, `i-hist`, `i-mach`, `i-repo`) inline with `currentColor`, a `wk-sidebar-label`, and optionally a count Chip (`wk-chip--count`) or the attention Chip.
- Machines as a `<details>` whose `<summary class="wk-sidebar-item">` carries the disclosure triangle; each machine is a child item in a nested `<ul>` with an sm StatusDot for its state instead of a glyph.
- Optionally a `wk-sidebar-detail` at the end of an item: a short word or age where a count would be wrong ("Removed", "2 min", "Idle").
- `wk-sidebar-heading` for a group ("This Mac"); the owner's own machine sits under it.
- `wk-sidebar-footer`: the HQ address.
- `aria-current="page"` on the selected item.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-sidebar` | Column on `wk-color-sidebar` with a `wk-color-hairline` right edge; `wk-space-3` × `wk-space-2` padding. |
| `wk-sidebar-item` | 28px row, `wk-radius-control`; hover `wk-color-fill-strong`; glyph `wk-color-text-secondary`. |
| `aria-current="page"` | `wk-color-selection` ground, semibold, glyph in `wk-color-accent`. |
| Nested `ul ul` | Child items indented 30px. |
| `summary.wk-sidebar-item` | Disclosure triangle, turned while closed. |
| `wk-sidebar-detail` | Trailing, `wk-font-size-xs` secondary, tabular figures. |
| `wk-sidebar-heading` | `wk-font-size-xs` semibold secondary. |
| `wk-sidebar-footer` | Pinned to the bottom over a hairline, `wk-font-size-xs` secondary. |

## Do and don't

- Do show the machine's live state with its sm dot; dots here never halo or pulse.
- Do count only active machines in the Machines chip; a removed machine stays listed with a dashed dot until it ages out.
- Don't fill the selected item with the accent; selection is quiet.

## Replaces

Design page: `.sb`, `.sb .it`, `.it[aria-current]`, `.it.child`, `.it .lbl`, `.it .cnt`, `.cnt.att`, `.it .dlamp`, `.sb .grp-h`, `.sb .who`, `.sb .disc-tri`, `.sb .sp`.
