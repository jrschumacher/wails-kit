# Notice

A strip across the top of a view that names one problem or pending thing, says why in a line, and offers the action that fixes it.

## When to use

- **Attention** (`data-tone="attention"`): HQ's strip when jobs can't run: "HQ can't reach GitHub.", "No machine can take Linux x64 jobs.", "No machine serves Linux ARM64 jobs." These set the health line to "Jobs can't run".
- **Neutral** (default): something waits on a person but nothing is broken: "Linux ARM64 jobs are waiting for macbookpro, which you're using." Sets the health line to "Busy".
- **Inline** (`wk-notice--inline`): a one-line pending item with its action at the end: "1 invite waiting to be used" with Cancel invite; the status window's "Settings saved" note with "Review Settings".
- One strip at a time: the highest-priority problem wins and the rest are counted ("and 2 more problems").

## What you supply

- The `warn` glyph inline (`currentColor`), `aria-hidden="true"`; omit it for inline notices.
- `wk-notice-body`: a bold first sentence that names what is wrong and where, with a full stop; then one `<p>` with the cause and what happens next.
- `wk-notice-actions`: at most one primary Button that fixes it, and optionally a Link for the next step ("See queued runs on GitHub").

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-notice` | Row: glyph, body, actions; `wk-color-surface`, 1px `wk-color-hairline-strong`, `wk-radius-group`; glyph in `wk-color-text-secondary`. |
| `data-tone="attention"` | `wk-color-attention-fill` ground, `wk-color-attention` border and glyph. |
| `wk-notice-body` | The bold title and a secondary `<p>`. |
| `wk-notice-actions` | Right-aligned column of actions. |
| `wk-notice--inline` | One wrapping line, actions at the end in a row. |

## Do and don't

- Do say what failed and what Muster does next ("Trying again at 2:18 pm.").
- Do label an uncertain count ("that count may be low").
- Don't use red for something that is only waiting on a person; an offline laptop or a machine on break is neutral.
- Don't stack notices; don't use one for success.

## Replaces

Design page: `.attn`, `.attn.ink`, `.attn.ord`, `.attn .t`, `.attn .acts`, `.attn .more`, `.pending`, `.prob`, `.assume`. App: `.save-notice`, `#join-notice`.
