# Range

A slider for a value where the rough amount matters more than the exact number: a reviewer's target share.

## When to use

For a share, a weight or a level with a sensible range. When the exact number matters (slots, jobs at once), use Stepper.

## What you supply

- `<input class="wk-range" type="range" min max step value aria-label>`.
- `--wk-range-value` set to the filled share of the track (`style="--wk-range-value: 60%"`), updated on `input`. The track cannot know its value from CSS alone.
- The value in words beside it, in an `<output>` ("60%").

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-range` | 4px track in `wk-color-fill-strong`, filled with `wk-color-accent` up to `--wk-range-value`; 16px thumb in `wk-color-thumb` with `wk-shadow-raised`. |
| `disabled` | 45% opacity. |

## Do and don't

- Do show the number beside the slider; the track alone is not precise enough.
- Do let arrow keys step it (native behaviour) and keep `step` meaningful (5 for percentages).
- Don't use a slider for a choice between named options; use SegmentedControl.
