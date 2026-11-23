# HeatMap

A year of days as a grid of small squares, one hue in five steps, so a lender sees at a glance when their machine served most.

## When to use

At the top of the "This machine" sheet, under the SectionHeading "How this Mac helped" with its aside ("since it joined, 14 Mar"). Pair it with the Stat tiles that answer in words, and offer the same numbers as a table ("View by month") for keyboard and screen-reader users.

## What you supply

- `mu-heatmap-months`: a `<span>` per month start, placed with `style="grid-column:N"` (N = its week column, 1 to 53), `aria-hidden="true"`.
- `mu-heatmap-grid`: 53 × 7 = 371 `<i>` cells in column order (week by week, Monday first), each with `data-level` and a `title` ("Mon 9 Nov: 4 jobs"). Give the grid `role="img"` and an `aria-label` that summarises the year in a sentence.
- `mu-heatmap-legend`: "Fewer jobs", five `<i data-level="0..4">`, "More".

Levels: `0` no jobs, `1` 1 to 4, `2` 5 to 8, `3` 9 to 12, `4` 13 or more. `none` marks days before the machine joined; `future` holds the place of days after today.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `mu-heatmap` | Column: months, grid, legend; `wk-font-size-xs` secondary text. |
| `mu-heatmap-grid` | 53 fluid columns (`minmax(0, 1fr)`), 2px gaps, square cells, `wk-radius-small`. It always fits its container and never scrolls sideways. |
| `data-level="0"` to `"4"` | `wk-color-accent` at `mu-heat-0` … `mu-heat-4` opacity. |
| `data-level="none"` | An empty square outlined in `wk-color-hairline`. |
| `data-level="future"` | Invisible. |

## Do and don't

- Do use one hue: the accent. Steps are opacity, never more colours.
- Do keep the legend; colour alone does not tell the count.
- Don't animate or add a tooltip layer of your own; the cell `title` is the tooltip.
- Don't use it for hours of a week: that strip is a separate HQ chart.

## Replaces

Design page: `.cal-wrap`, `.cal-months`, `.cal`, `.cal i`, `.cal i.pre`, `.cal-key` (both the fixed 7px grid and the round-8 fluid override). The hourly `.heat` strip on the machine page is not folded in.
