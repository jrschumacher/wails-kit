# Button

A native `<button>` that does one thing now: default, primary, destructive or quiet, at two heights.

## When to use

- **Default** (`wk-button`) for any action, including the one action under a StatusHeadline ("Take a break", "Stop now", "Quit Muster").
- **Primary** (`wk-button--primary`) for the one default action of a window, sheet or alert ("Done", "Keep running", "Join another HQ"). One per view.
- **Destructive** (`wk-button--destructive`) for an action that loses work or access ("Leave HQ…", "Stop job", "Revoke…"). Never the default; put it before the safe choice.
- **Quiet** (`wk-button--quiet`): borderless, secondary text. For Cancel beside a primary, and icon buttons in a toolbar or window header.
- For navigation inside text, use Link, not a quiet button.

## What you supply

A `<button type="button">` (or `<a>` for a real URL) with a sentence-case label. A label that opens a confirmation ends in an ellipsis ("Stop…", "Leave HQ…"). An icon-only button needs `aria-label`; put the glyph inline as `<svg>` with `stroke="currentColor"`, 14px by default.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-button` | Default: control face, control border, 28px (`wk-control-height`), `wk-radius-control`. |
| `wk-button--primary` | `wk-color-accent` fill, `wk-color-on-accent` label. |
| `wk-button--destructive` | Label in `wk-color-destructive-text`. |
| `wk-button--quiet` | No border, no fill, `wk-color-text-secondary` label; text colour on hover. |
| `wk-button--small` | 22px (`wk-control-height-small`), `wk-font-size-sm`. |
| `wk-button--icon` | Square; combine with `--quiet` and `--small`. |
| `disabled` | 45% opacity, no shadow. |
| `aria-disabled="true"` + `data-locked` | Locked: a manage action shown to a view-only admin. Stays focusable; lock glyph first; `title="Needs manage access"`. |
| `aria-expanded="true"` | Pressed tint while its menu is open. |

## States

Hover and active lay `wk-color-fill-strong` over the face; focus draws the 2px `wk-color-accent` ring at `wk-focus-offset`.

## Do and don't

- Do keep one primary per view, last in the row (macOS order: Cancel, then the default).
- Do stack three choices in an Alert, the safe default last.
- Don't make a button wider or taller for emphasis: there is no large size. Full-width primaries in the join flow use the default height.
- Don't colour a button with a state colour.

## Replaces

Design page: `.nb`, `.nb.def`, `.nb.destr`, `.nb.lg`, `.nb.ring`, `.nb.lockd`, `.quiet` (round 8 restyle as a push button), `.tbar .back button`, `.sbtog`. App: `.status-btn`, `.wide`, `.gear`.
