# FilterButtons

A row of pill toggles above a list that narrow it by one property, each with its count.

## When to use

Above a List or Table, under the Toolbar, when people switch between a few fixed subsets: Active, Done, All. For one choice among ranges of time, use SegmentedControl; for free text, use SearchField.

## What you supply

- `<div class="wk-filters" role="group" aria-label="…">` holding `<button class="wk-filter" aria-pressed>` buttons.
- Each button: the subset's name and a `wk-filter-count`.
- One-of-many (Active, Done, All): exactly one pressed. Many-of-many (reviewers): any number pressed, none pressed means all.
- `aria-controls` naming the list it filters.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-filters` | Wrapping row, 6px gaps. |
| `wk-filter` | 24px pill, control-border outline, secondary text. |
| `aria-pressed="true"` | `wk-color-selection` ground, text colour, no outline. |
| `wk-filter-count` | Tabular figures in secondary. |
| `disabled` | A subset with nothing in it. |

## Do and don't

- Do keep counts live; they are how a person knows a filter is worth pressing.
- Don't use more than about six; use a Menu instead.
- Don't fill a pressed filter with the accent.
