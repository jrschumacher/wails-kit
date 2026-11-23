# ErrorBox

What failed, in one box: a title that names the failure, the copy icon button, the facts (how often, when, the next try) and the raw message in the machine's own words.

## When to use

Wherever an error has details a person may need to send on: under the StatusHeadline in Needs attention ("Can't start jobs."), in HQ's Start health card on a machine page, in Settings when saving fails. The same component on the machine and in HQ.

## What you supply

- `wk-errorbox-head`: a `<span>` naming the failure in plain words ("Couldn't download the job image"; in history, "Last failure: couldn't download the job image"), then a `wk-copy` button (`aria-label="Copy error details"`, `title="Copy"`) with the `i-copy` glyph.
- A FactsList (`wk-facts`) with the streak and times: "Failed" / "5 starts in a row, last at 2:13 pm", "Next try" / "2:18 pm", or "When".
- A `<pre>` with the raw error exactly as the machine reported it.
- `role="group"` and an `aria-label`.

The copy button copies the whole box as text, plus the app version and machine name.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-errorbox` | Up to 400px wide; `wk-color-surface`, 1px `wk-color-hairline-strong`, `wk-radius-group`. |
| `wk-errorbox-head` | Semibold title row over a `wk-color-hairline` rule; the copy button at the end. |
| `wk-errorbox > .wk-facts` | Facts with tight 3px rows. |
| `wk-errorbox > pre` | `wk-font-family-mono` `wk-font-size-sm` in `wk-color-text-secondary`, wraps, at most 96px then scrolls, selectable. |

## Do and don't

- Do say what failed, not that "something went wrong".
- Do keep the raw message verbatim; don't paraphrase it.
- Don't add a separate "Copy details" button: the icon in the corner is the copy action.
- Don't colour the box red; the StatusHeadline or Notice above carries the state.

## Replaces

Design page: `.errbox`, `.errbox .eh`, `.errbox .copy`, `.errbox dl`, `.errbox pre`, `.card .errbox`. App: `#status-error`, `.copy-details` buttons, the Copy details flow in `detailsText`.
