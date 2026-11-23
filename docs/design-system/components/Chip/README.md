# Chip

A small pill of one or two words that labels something: a job type, a count, the admin's access, a note such as "update available".

## When to use

- **Default**: a label that belongs to the thing beside it: job types in Repos ("Any Linux", "Linux ARM64"), "update available" after an app version, a role ("Lender").
- **Count** (`wk-chip--count`): a number at the end of a sidebar item ("Machines 2", "Repos 6").
- **Attention** (`wk-chip--attention`): how many things need someone, on the sidebar's Now item. Red, so only for problems that set HQ's health to "Jobs can't run".
- **With a dot**: a StatusDot (`data-size="sm"`) first, then the item and its state in words: one chip per reviewer run in Peanut ("codex-high · 3 findings").
- **Muted** (`wk-chip--muted`): a dashed chip that says something is absent ("No reviewer runs yet").
- **Access** (`wk-chip--access`): in the toolbar, what this admin may do: "View only" with the eye glyph, "Can manage" with the lock glyph and `data-level="manage"`.

## What you supply

A `<span>` (or a `<button>` when it filters) with the words; a `title` that shows the exact source where useful (`runs-on: [linux, ARM64]`); an `aria-label` on a bare number ("1 needs attention").

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-chip` | 20px pill, `wk-radius-pill`, `wk-color-hairline-strong` outline, `wk-color-text-secondary`, `wk-font-size-xs` semibold. |
| `wk-chip--count` | No outline, `wk-color-fill-strong` ground, tabular figures. |
| `wk-chip--attention` | `wk-color-attention` fill, `wk-color-on-attention` text. |
| `.wk-chip > .wk-dot` | A small dot leading the words, pulled 3px toward the edge. |
| `wk-chip--muted` | Dashed outline, medium weight. |
| `wk-chip--access` | Leading 11px glyph; `data-level="manage"` reads in `wk-color-text`. |

## Do and don't

- Do keep chips to one or two words.
- Don't use a chip as a button for a primary action.
- Don't invent tones: the accent never fills a chip.
- Known contrast gap: `wk-color-text-secondary` on a count chip inside the sidebar is 4.26:1 in light (3.6:1 on a selected item). The source pair is kept; see the token's usage note.

## Replaces

Design page: `.chip-s` (job type, 4px radius), `.badge-ink` ("update available"), `.state-pill` ("Lender", "Can manage"), `.perm` and `.perm.mg` (access), `.sb .it .cnt` and `.cnt.att`, `.slice` (presentation only, not carried into the product).
