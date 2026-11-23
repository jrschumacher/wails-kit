# CheckRow

A label with its control: a checkbox, radio or switch beside its words, or a settings row with the words at the start and the switch, stepper or menu at the end, help underneath.

## When to use

- **Settings row** (`wk-settings-row` + `wk-check--row`) inside a Panel `--rows`: every setting in General and Lending ("Open at login", "Only when plugged in", "Jobs at once", "Public repositories").
- **Checkbox** (`wk-check`): a choice made once in a sheet ("Allow public repositories on it"); read-only scopes shown disabled and checked ("See machines, jobs, history and repos").
- **Radio** (`wk-check` with `wk-radio`): one of a few modes ("When I'm away", "Always, even while I'm using it") in a `<fieldset>` with a `<legend>`.

## What you supply

- A `<label class="wk-check">` wrapping the input and its words, so the whole row is the hit area.
- For a settings row: `<div class="wk-settings-row">` around a `<label class="wk-check wk-check--row">` holding `wk-settings-label` (the words, an optional `<small class="wk-settings-help">`) then the control; a long help line or a `wk-error` goes after the label inside the row.
- For a checkbox with a help line: `wk-check--top` and a `wk-check-text` column.
- Switches are `<input type="checkbox" role="switch" class="wk-switch">`; menus are `<select class="wk-select">`; numbers are a Stepper.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-check` | Inline row, `wk-space-2` gap; disabled inputs grey the words. |
| `wk-check--row` | Full-width, space-between: words, then control. |
| `wk-check--top` | Box aligned with the first line of a two-line label. |
| `wk-check-text` | Words and a help line, stacked. |
| `wk-checkbox`, `wk-radio`, `wk-switch` | 14px box, 14px radio, 34 × 20px switch; checked in `wk-color-accent` with `wk-color-on-accent` marks. |
| `wk-select` | 28px menu with a drawn chevron. |
| `wk-settings-row` | At least 40px, `wk-space-2` block padding; rows after the first get a `wk-color-hairline` rule. |
| `wk-settings-label`, `wk-settings-help`, `wk-settings-note`, `wk-settings-heading` | Words, secondary help (`wk-font-size-sm`), a note under a group, a heading over a group. |

## Do and don't

- Do phrase a switch as the thing that is on ("Open at login"), not as a question.
- Do put the reason a control is limited in the help line ("HQ may allow fewer.").
- Don't use a checkbox for an action; use a Button.
- Don't use a switch inside a sheet that has Cancel; a sheet's choices apply on its primary button, so use a checkbox there.

## Replaces

Design page: `.ncheck`, `.setbody .grp .row`, `.setbody .soft`. App: `.setting`, `.setting > .row`, `.radios`, `.stack`, `.refuse`.
