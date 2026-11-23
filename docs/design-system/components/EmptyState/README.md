# EmptyState

What a view shows when it has nothing to list yet or cannot show live data: a title, one or two lines, and the action that changes it.

## When to use

- A fresh HQ: "No machines yet", with "Invite a machine".
- History before there is enough record: "Keeping the log", with the date recording began and a count.
- HQ unreachable: "Can't reach HQ", with "Show last report". Never stale numbers presented as live.
- Not for errors with details (ErrorBox) or problems that block jobs (Notice).

## What you supply

- Optionally the warden mark as an `<img>` (`assets/Logos/warden.svg`, and `warden-dark.svg` in the dark theme), `alt=""`, 56px. Use it for first-run and "keeping the log" states, not for failures.
- A `wk-empty-title`: what is true now, in the same words as the state list where one applies.
- One or two `<p>` lines: why, and what happens next without the person ("Muster tries again on its own.").
- At most one Button: primary when it starts something (Invite a machine), default otherwise.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-empty` | Centred column that fills its pane, 6px gaps, 40px × 20px padding. |
| `wk-empty > img`, `wk-empty > svg` | 56px mark. |
| `wk-empty-title` | `wk-font-size-xl`, weight 650. |
| `wk-empty > p` | `wk-color-text-secondary`, at most 46 characters a line. |
| `wk-empty > .wk-button` | `wk-space-3` above. |

## Do and don't

- Do read as working, not broken: say what is being kept or waited for.
- Do show a locked Button with a line under it when the person lacks access ("Ask the HQ owner to invite a machine.").
- Don't show empty tables, zeros or charts with no data.

## Replaces

Design page: `.empty`, `.empty .stamp`, `.empty svg.spot`, `.empty .nb`. App: the roster screen (`.roster`) layout.
