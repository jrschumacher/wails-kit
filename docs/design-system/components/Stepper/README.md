# Stepper

A short number field with up and down arrows beside it, for counts that move by one: slots, jobs at once, minutes, a CPU limit.

## When to use

In a settings row or a Panel row where the value is a small whole number within limits: HQ's Slots for a machine, Lending's "Jobs at once", "When I'm back, finish jobs within (min)", "Not when CPU is above (%)". For free text use a plain field.

## What you supply

A `<span class="wk-stepper">` with:
- an `<input class="wk-field" inputmode="numeric">` carrying `aria-label` (or a `<label>`), and `aria-describedby` pointing at the help or error line;
- a `<span class="wk-stepper-arrows">` with two empty `<button type="button">`s labelled "More" and "Fewer". Disable an arrow at the limit, and say why in the help line ("This Mac offers 2, so HQ can't set more.").

The arrows draw their own chevrons. On an invalid value set `aria-invalid="true"` and show a `wk-error` line under the row.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-stepper` | Field and arrows, 6px apart. |
| `wk-stepper > .wk-field` | 64px, right-aligned tabular figures, `wk-control-height`. |
| `wk-stepper-arrows` | Two stacked 18px buttons in one `wk-color-control-border` box, `wk-radius-control`. |
| `button:disabled` | Secondary colour at 45%. |
| `.wk-field[aria-invalid="true"]` | `wk-color-attention` border. |

## Do and don't

- Do say the limit and who sets it.
- Don't use a stepper for values over a few hundred or with units that change.
- Don't rely on the browser's own number spinner; it differs per platform.

## Replaces

Design page: `.nstep`, `.nstep input`, `.nstep .arrows` (▲▼ text glyphs). App: `.number` and `.inline-number` fields (`type="number"` with the native spinner).
