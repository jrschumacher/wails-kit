# Panel

A boxed group on the window ground: a 1px hairline, the group radius and the surface colour, holding rows that belong together.

## When to use

- **Default**: a set of controls or facts about one thing: HQ's Manage box on a machine page, a settings group in the HQ pane, the HQ access summary.
- **Rows** (`wk-panel--rows`): a settings group whose rows are separated by hairlines edge to edge (General, Lending).
- **Dashed** (`wk-panel--dashed`): something to check rather than change: the joiner's "Confirm your HQ" box and the invite's "If they ask, read them this".
- Not for strips that ask for something (use Notice), errors (ErrorBox), slots (SlotCell) or code (CodeBlock).

## What you supply

A container (`<section>` or `<div>`) with an accessible name when it stands alone. Inside: `wk-panel-row` rows (words at the start, the control at the end), `wk-panel-hint` lines under the row they explain, FactsList, or CheckRow settings rows.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-panel` | Column, 10px gap, `wk-space-3` padding, 1px `wk-color-hairline`, `wk-radius-group`, `wk-color-surface`. |
| `wk-panel--rows` | No gap and no block padding: the rows draw their own hairlines. |
| `wk-panel--dashed` | Dashed `wk-color-hairline-strong`, no fill. |
| `wk-panel-row` | Space-between row that wraps; `wk-space-2` × `wk-space-3` gap. |
| `wk-panel-hint` | `wk-font-size-sm`, `wk-color-text-secondary`. |

## Do and don't

- Do keep one subject per Panel and a heading (SectionHeading) above it when there are several.
- Don't nest Panels.
- Don't use a heavier border, a shadow or a tinted fill for emphasis.

## Replaces

Design page: `.manage` and `.manage .row`/`.hint`, `.setbody .grp` and `.grp .row`, `.prob`, `.verify`, `.jobcard`, `.panel`. App: `.wk-settings-group`, `.verify`, `.chip` (a repository row in the join flow), `.slot` boxes outside the slot grid.
