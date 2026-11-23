# Icons

Muster's glyph set, one file per drawing on the design page. Every file is a standalone SVG drawn in a single mid-tone ink, `#787f8b`, because an `<img>` cannot inherit colour: that ink keeps at least 3.9:1 against `wk-color-bg` and `wk-color-surface` in both the light and the dark theme, so the files read on either ground. In the product, do not use these files: inline the same paths as `<svg>` with `stroke="currentColor"` (and `fill="currentColor"` for solid parts) so they take the text colour of their theme.

## Sidebar (HQ), 16px, 1.4 stroke
- `i-now.svg`: Now (a gauge).
- `i-hist.svg`: History (three bars, solid).
- `i-mach.svg`: Machines (two stacked units).
- `i-repo.svg`: Repos (a book).
- `i-mac.svg`: a Mac (laptop); for This Mac.

## Settings panes, 22px, 1.5 stroke
- `i-gen.svg`: General (a sun-like gear).
- `i-lend.svg`: Lending (a laptop with a check).
- `i-hq.svg`: HQ (a house).
- `i-about.svg`: About (an i in a circle).

## Access and actions
- `i-lock.svg`: 12px, 1.3 stroke. "Can manage" chip and locked buttons.
- `i-eye.svg`: 12px, 1.2 stroke. "View only" chip.
- `i-copy.svg`: 16px, 1.4 stroke. The copy icon button in ErrorBox and CodeBlock.
- `warn.svg`: 24px viewBox drawn at 20px, 2 stroke. The Notice glyph.

## Stat tiles, 18px, 1.5 stroke
- `s-jobs.svg`: jobs served (a check in a circle).
- `s-time.svg`: machine time given (a clock).
- `s-fast.svg`: speed against the pool (a bolt).
- `s-cal.svg`: when it serves most (a calendar).
- `s-slots.svg`: slots in use (four squares, one solid).
- `s-ok.svg`: start health (a shield with a check).

## Menu bar template glyph, 18 × 16, 1.5 stroke
One colour; the macOS menu bar tints it. The shape changes per state.
- `tmpl.svg`: On duty, waiting or running (solid helmet, laptop outline).
- `tmpl-idle.svg`: Standing down, On break (helmet outline only).
- `tmpl-off.svg`: Off duty (outline with a bar through the laptop).
- `tmpl-alert.svg`: Needs attention, Can't start jobs (solid helmet, "!" in the laptop).
- `tmpl-lost.svg`: Can't reach HQ, Removed from HQ (dashed outline).

## The design page's drawings of the marks
