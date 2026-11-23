# SegmentedControl

Two to four mutually exclusive choices side by side on a tinted track: native radios, one label each.

## When to use

To switch the view of one thing in place: History's range ("24 hours", "7 days", "14 days"). Not for navigation between sections (Sidebar, SettingsTabs) and not for settings that save (CheckRow radios).

## What you supply

A `<div class="wk-segmented" role="radiogroup" aria-label>` with one `<label><input type="radio" name><span>Words</span></label>` per choice, one `checked`. Disable a choice that has no data yet; once there is enough record, enable it. Arrow keys move between choices natively.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-segmented` | `wk-color-fill-strong` track, 2px padding, `wk-radius-control`. |
| `input + span` | 22px (`wk-control-height-small`) segment, `wk-font-size-sm` medium. |
| `:checked` | `wk-color-control` face with `wk-shadow-raised`. |
| `:focus-visible` | Accent ring on the segment. |
| `:disabled` | Secondary text at 60%. |

## Do and don't

- Do keep labels short and parallel ("7 days", "14 days").
- Don't use more than four segments.
- Don't hide unavailable ranges by striking them through; disable them.

## Replaces

Design page: `.seg` and `.seg button[aria-pressed]` (buttons become radios; the design's 7px radius takes `wk-radius-control`). Kit `.wk-filters`/`.wk-filter` stay in bundle.css but are not used by Muster.
