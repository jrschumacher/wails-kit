# Table

A dense native table with hairline rows: HQ's lists of machines, running jobs, repos and history figures.

## When to use

For items with the same columns: Running and waiting, Machines, Repos, a machine's shared-job speeds, the "Show as a table" view under every chart. For a handful of label–value facts, use FactsList.

## What you supply

A `<table class="wk-table">` with `<thead>` and `<tbody>`. Put numbers in `wk-num` cells (right-aligned, tabular). Name cells that open an item hold a `<button class="wk-table-name">`. A State cell holds a StatusDot in a `wk-dot-label` with the state's words; a Busy cell holds Pips and "1 of 2". Row actions go in a `wk-table-actions` span: a Link ("View run") and a small Button ("Stop…", locked for view-only admins). Owner prefixes and waiting rows use `wk-secondary`. Wrap a wide table in a container with `overflow-x: auto` and drop low-priority columns at narrow widths.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-table` | Full width, 6px × 10px cells, `wk-font-size-base`; header in `wk-font-size-sm` semibold secondary over a `wk-color-hairline-strong` rule; rows split by `wk-color-hairline`. |
| `wk-table--zebra` | Even rows on `wk-color-fill`. Optional; use for long numeric tables. |
| `wk-num` | Right-aligned tabular figures, no wrap. |
| `tr[aria-selected="true"]` or `tr[aria-current]` | Selected row on `wk-color-selection`. |
| `tr[data-muted]` | A row that no longer counts (a removed machine): secondary text, its `wk-table-name` struck through. |
| `wk-table-name` | The item's name as a semibold text button; underline on hover. |
| `wk-table-actions` | Inline row actions, 6px apart. |

## Do and don't

- Do give elapsed times as `m:ss` in tables and waits as "34 s", "3 h 12 m".
- Do keep StatusDots static in tables; only the lender window shows the live state.
- Don't fill a selected row with the accent; selection is the quiet `wk-color-selection`.
- Don't hide a removed machine immediately: show it muted with the date.

## Replaces

Design page: `.nt` (with `.n`, `.repo`, `.o`, `.sm`, `.st`, `tr.revoked`, `tr.sel`, `.wide`), `.mname`, `.rowacts`, `.legend-t`, `.kbdt`, `.fail-list`. The round-3 zebra (`.nt tbody tr:nth-child(even)`) becomes the optional modifier.
