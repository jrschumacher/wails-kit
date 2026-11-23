# List

A column of selectable rows of any height, for items that do not share columns: a job with a title, a meta line and chips.

## When to use

When each item needs two or three lines and the person picks one to inspect: Peanut's jobs. When items share the same columns and are compared across, use Table. When the list is navigation, use Sidebar.

## What you supply

- `<ul class="wk-list" role="listbox" aria-label="…">` with one `<li class="wk-list-row" role="option" aria-selected>` per item. The app lays out the row's content (Peanut's `pn-job`).
- Roving focus: one row has `tabindex="0"`, the rest `-1`; Up and Down move, Home and End jump, Return opens, Shift-F10 or the menu key opens the row's context Menu.
- A `wk-list-heading` over each group, with its count in a `<span>`.
- A state on a row is a StatusDot followed by the state's word in the row's text; never the dot alone.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-list` | Column of rows, 2px apart, `wk-space-2` side inset. |
| `wk-list-row` | `wk-space-2` × 10px padding, `wk-radius-control`; hover `wk-color-fill`. |
| `aria-selected="true"` | `wk-color-selection` ground. |
| `wk-list-heading` | Group title, `wk-font-size-base` semibold, count in secondary. |

## Do and don't

- Do keep selection quiet: the accent never fills a row.
- Do keep a row's height stable as its state changes; chips wrap, the title truncates with an ellipsis.
- Don't put buttons inside a row; actions live in the inspector and the context Menu.
