# StatusHeadline

The state of this machine in one headline, its StatusDot and one line, with at most the action that state offers; the top of the status window.

## When to use

At the top of the lender's status window, always. HQ's health line ("All clear", "Busy", "Jobs can't run") uses the same parts with `mu-status--start`. One per window.

## What you supply

- The **title**: the state's name, exactly as HQ prints it ("On duty", "Standing down", "On break", "Off duty", "Needs attention", "Can't reach HQ", "Removed from HQ").
- The **dot** for that state (see StatusDot), `aria-hidden="true"`.
- **One line** that says what is happening now, as a sentence with a full stop: "Running 1 job.", "Waiting for jobs from HQ.", "Can't start jobs.", "HQ took this machine off duty at 1:10 pm."
- Optionally the **action** for that state as a default Button ("Take a break", "Stop now", "Quit Muster"); a state that offers a way forward puts its primary first ("Join another HQ").
- Wrap the title and line in an `aria-live="polite"` region so a state change is announced once.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `mu-status` | Centred column. |
| `mu-status-title` | 24px (`wk-font-size-xl`), weight 650, tight leading. |
| `mu-status-row` | Dot and line on one row, `wk-space-2` apart. |
| `mu-status-line` | `wk-font-size-md`, at most 46 characters a line. |
| `mu-status-actions` | Column of buttons under the line, 14px below. |
| `data-tone="attention"` | Title in `wk-color-attention-text`. Only for Needs attention. |
| `mu-status--start` | Left-aligned (HQ's health line). |

## Do and don't

- Do use the same words on the machine and in HQ.
- Do put details below the headline in their own component (an ErrorBox for Needs attention, the SlotCell grid for On duty).
- Don't add a sub line or a reassurance ("You don't need to do anything"). The headline and its one line are the whole message.
- Don't put more than one secondary action under it.

## Replaces

Design page: `.state`, `.state.md`, `.state.alert`, `.lamp-row`, `.line`, `.sub` (dropped), `.quiet` as the action, `.health`, `.health .stamp`, `.health .why`, `.dhead .stamp`/`.stline`. App: `.hero`, `.headline`, `.lamp-row`, `.status-line`, `.hint` (dropped), `#status-action`, `#status-secondary`.
