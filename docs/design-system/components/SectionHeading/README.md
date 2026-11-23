# SectionHeading

The one heading style inside a window, sheet or card: semibold body-size words, optionally followed by an aside that says what the section counts and a link at the end.

## When to use

Above every group of content below a window's title: "Running and waiting", "Machines", "Now", "Set up as", "Start health", "Docker". It is the only heading level inside a view; sheet and dialog titles use `wk-dialog-title`, the toolbar uses `wk-toolbar-title`.

## What you supply

An `<h2>`–`<h4>` (whatever the document outline needs) with `class="wk-section-heading"` and the section name in sentence case. Optionally:
- a `<span class="wk-section-heading-aside">` with the range or source ("last 14 days", "since it joined, 14 Mar", "23 jobs ran here and on unraid");
- a Link at the end ("All machines").

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-section-heading` | `wk-font-size-base`, `wk-font-weight-semibold`, flex row: words, aside, then anything pushed to the end. |
| `wk-section-heading-aside` | `wk-font-size-sm`, regular, `wk-color-text-secondary`. |
| `wk-section-heading--rule` | A `wk-color-hairline` rule above, 10px clear: starts a card on HQ's machine page. |

## Do and don't

- Do use the same style at every depth; hierarchy comes from spacing and containers, not size.
- Don't end a heading with a colon or a full stop.
- Don't use the toolbar title or the StatusHeadline title as a section heading.

## Replaces

Design page: `.content h4` and `h4 .aside`, `.card h5` and `h5 .aside`, `.mine h6` and `h6 .aside`, `.sec .head`, `.card` (its top rule), `.chart .ct`, `.menu-col h5`. App: `.wk-subheading` in the sheet, `.wk-settings-heading`, `.step`.
