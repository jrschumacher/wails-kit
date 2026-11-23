# StatusDot

One small round mark per machine state: the shape names the state, the tone says whether it is working or needs someone; the same dot appears on the machine and in HQ.

## When to use

Beside every state name: the StatusHeadline, HQ's health line, the State column of a Table, sidebar machine items, the toolbar's connection words. Never alone: a dot always sits next to the words for its state (`aria-hidden="true"` on the dot).

## What you supply

A `<span class="wk-dot">` with `data-shape`, `data-tone` and `data-size` for the state. Map states to dots with one function (the app's `lampDot` in `ui-model.mjs`), never per screen.

| State | `data-shape` | `data-tone` |
|---|---|---|
| On duty, running a job | (none) or `filled` | `working` (gets a halo at md) |
| On duty, waiting for jobs | (none) | (none) |
| Standing by (on duty, no jobs it can run) | `ringed` | (none) |
| Standing down | `half` | `working` |
| On break | `half` | (none) |
| Off duty | `bar` | (none) |
| Can't reach HQ (HQ says Offline) | `hollow` | (none) |
| Removed | `dashed` | (none) |
| Needs attention (Can't start jobs) | (none) | `attention` |

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-dot` | 10px (`wk-dot-size`), 1.5px ring and fill in `wk-color-text`. |
| `data-size="sm"` | 8px (`wk-dot-size-small`): sidebar items, toolbar connection words. No halo. |
| `data-tone="working"` / `"attention"` | `wk-color-working` / `wk-color-attention`. |
| `data-shape` | `filled` (default), `ringed`, `half`, `hollow`, `dashed`, `bar`. |
| `--wk-dot-ground` | Set it to the surface behind a halo dot when that is not `wk-color-bg` (a dot inside a Panel: `wk-color-surface`). |
| `wk-dot-label` | Wraps a dot and its words on one line, `wk-space-2` apart. After an sm dot the words are secondary and `wk-font-size-sm`. |

## Do and don't

- Do keep shapes: state never rides on colour alone, and forced-colours mode draws every dot in one system colour with its shape intact.
- Do use the working tone only while a job runs or finishes; ready-and-waiting is neutral.
- Don't animate the dot. The design's pulse is gone; the halo marks a running job.
- Don't use red for anything a lender cannot fix: Off duty, On break and Offline are neutral.
- Don't size dots by hand: md (10px) or sm (8px).

## Replaces

Design page: `.lamp` with `.on`, `.work`, `.pulse`, `.hollow`, `.red`, `.half`, `.bar`, `.x`, `.dot`, `.drain`; `.sb .it .dlamp`; `.conn .d` and `.conn .d.off`; `.health .lamp`, `.nt .st .lamp`, `.legend-t .lamp`. App: `.lamp` (11px) and its working ring rule.
