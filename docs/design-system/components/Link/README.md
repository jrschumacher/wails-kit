# Link

Accent-coloured text that goes somewhere or shows something, as an `<a>` or a text `<button>`: one look for every link in Muster.

## When to use

- Moving to another view, window or site: "Open HQ", "View run", "See queued runs on GitHub", "All machines".
- A small action that belongs to a line of text or a cell: "Why?", "Change", the SlotCell's "Stop…".
- Showing or hiding a section in place: "Advanced", "Where do I get a join code?" (add `aria-expanded`).
- Use a Button when the action changes state for the whole view (Take a break, Leave HQ…).

## What you supply

An `<a class="wk-link" href>` for a URL, or `<button class="wk-link" type="button">` for an in-app action. It inherits the font size of its context; set the context to `wk-font-size-sm` for a cell or footer link. External links open in the browser through the app.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-link` | `wk-color-accent` text, no underline; underline on hover, 2px offset. |
| `aria-expanded="false"` / `"true"` | Adds a leading chevron that points right when closed and down when open. Pair with `aria-controls`. |
| `disabled` (button) | `wk-color-text-secondary`, no hover. |

## Do and don't

- Do write the destination or the action, in sentence case, no trailing period.
- Do end a link that asks first with an ellipsis ("Stop…").
- Don't underline links at rest, and don't colour them anything but `wk-color-accent`.
- Don't use a link for a destructive action without a confirming Alert.

## Replaces

Seven link styles on the design page: `a`, `.slotstop`, `.qlink`, `.foot a`, `.foot .disc` (with `.chev`), `.jc-link a`, `.nt a`, `.sec .head a`. In the app: `.slotstop`, `.sheet-row` (the footer's "This machine" row) and `.disclosure`.
