# Sheet

A modal panel that drops from the top edge of its window: a title, a body that scrolls on its own, and a footer bar with the primary action at the right.

## When to use

For a task or a closer look that belongs to the window behind it, which stays live: "This machine" from the status window's footer; "Invite a machine" and "Send this code" in HQ. A question with two or three answers is an Alert, not a sheet.

## What you supply

A native `<dialog class="wk-dialog wk-dialog--sheet" aria-labelledby>` opened with `showModal()`, closed by the primary button or Esc.
- Short sheets: `wk-dialog-title`, `wk-dialog-body` paragraphs, then `wk-dialog-actions` (an optional `wk-dialog-start` button such as "Make a new code" at the left, Cancel, then the primary).
- Long sheets: `wk-dialog-header` (the title), `wk-dialog-scroll` (the body: SectionHeadings, Stats, a HeatMap, FactsLists), `wk-dialog-footer` (the primary, "Done", `autofocus`). Only the middle scrolls; header and footer span the sheet.
- Plain text under a heading inside the body goes in `wk-dialog-indent`, so it reads as belonging to that heading.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-dialog` | Up to 440px, 20px padding, `wk-color-surface`, `wk-radius-window`, `wk-shadow-dialog`. |
| `wk-dialog--sheet` | No top margin, square top corners. With a `wk-dialog-scroll` child it becomes a column with no padding. |
| `wk-dialog-title` | `wk-font-size-md` semibold. |
| `wk-dialog-header` | `wk-space-4` padding, `wk-space-2` below. |
| `wk-dialog-scroll` | Flexible, scrolls vertically, `wk-space-2` gaps, `wk-space-4` side padding. |
| `wk-dialog-footer` | Right-aligned buttons over a `wk-color-hairline` rule. |
| `wk-dialog-indent` | 10px in, behind a 2px `wk-color-hairline-strong` rule. |
| `wk-dialog-actions` | Right-aligned row, 18px above. |
| `::backdrop` | A fixed dim (literal: WebKit 13 does not pass custom properties to `::backdrop`). |

## Do and don't

- Do make the primary the safe, expected close ("Done"); Return and Esc work.
- Do keep the window behind the sheet live; never condense its state into the sheet.
- Don't nest sheets; don't put a sheet over an Alert.

## Replaces

Design page: `.sheet`, `.sheet h4`, `.sheet .acts`, `.sheet .acts .left`, `.sheet .soft`, `.mine-sheet` and its round-9 header/scroll/footer rules, `.mine`, `.scrim`, `.sheet-host`. App: `.machine` dialog, `.sheet-head` (Get Docker's header-with-Done), `.acts`.
