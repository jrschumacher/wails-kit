# Stat

A figure and the few words that say what it counts ("1 of 2" / "slots in use"), plain, in a tinted tile with a glyph, or in a ruled row of large figures.

## When to use

- **Plain** in a sheet or panel, two up (`wk-stats-grid`): "This machine" › Now.
- **Tile** (`wk-stat--tile`) when four answers belong together and each has a glyph: "This machine" › How this Mac helped (jobs served, machine time, speed, when it serves most), and Now.
- **Row** (`wk-stats-row`) for HQ's headline numbers under the health line: slots busy, jobs waiting, longest wait now, jobs since HQ started.

## What you supply

The figure as text with its unit (`1,438 jobs`, `125 h`, `3<small> / 4</small>`), and a label that finishes the sentence ("served for hq.schumacher.home", "jobs since HQ started at 8:02 am"). A label that is an estimate says so ("jobs waiting · may be low"). In a tile, the glyph from the icon set (`s-jobs`, `s-time`, `s-fast`, `s-cal`, `s-slots`, `s-ok`) inline with `currentColor`, `aria-hidden="true"`.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-stat` | Figure over label. |
| `wk-stat-figure` | `wk-font-size-lg` semibold, tabular figures; a `<small>` unit at `wk-font-size-md` regular, secondary. |
| `wk-stat-label` | `wk-font-size-sm`, `wk-color-text-secondary`. |
| `wk-stat--tile` | `wk-color-fill` ground, 1px `wk-color-hairline`, `wk-radius-group`, glyph on the left. |
| `wk-stat-glyph` | 30px square, `wk-color-selection` ground, glyph in `wk-color-accent`. |
| `wk-stats-grid` | Two equal columns, `wk-space-2` gap. |
| `wk-stats-row` | Auto-fit columns between two hairlines; figures at `wk-font-size-xl`. |

## Do and don't

- Do say what a figure counts from when it is not obvious ("since HQ started at 8:02 am").
- Don't present stale numbers as live: when HQ is unreachable, show the EmptyState instead.
- Don't colour figures by good or bad; state belongs to the StatusDot and Notice.
- Don't add filler stats; every figure answers a question the owner asked.

## Replaces

Design page: `.two-up`, `.stat`, `.mine .stat` with `.gl`, `.figs`, `.fig`, `.fig .v`, `.fig .k`, `.big` (machine page "129 jobs · 11 h 24 m"). App: `.two-up`, `.stat`.
