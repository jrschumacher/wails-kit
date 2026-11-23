# SlotCell

One cell per slot of this machine, always shown: a busy cell names its job and draws that slot's CPU, memory, network and disk over the last five minutes; a free cell waits.

## When to use

On the lender's status window, under the StatusHeadline, while the machine is On duty or Standing down. Not in HQ: HQ counts slots with Pips in its tables.

## What you supply

- A `<ul class="mu-slots">` with `data-count` (the number of slots) and an `aria-label` that says the whole grid in words ("2 slots: 1 running, 1 free. Resource use for the last 5 minutes").
- One `<li class="mu-slot" data-phase>` per slot, in a stable order: a running job keeps its cell when others change.
- **Running or starting**: a head (busy Pip, `<b>repo</b> · job`, and the elapsed time "3 min", or "Starting"), the four gauge rows once samples exist, and a foot with a Link "Stop…" that asks in an Alert first.
- **Free**: an outline Pip, "Free", and one centred line: "Waiting for a job" while on duty, "Not taking new jobs" while standing down.
- **Stopped**: the struck job (`<s>laurel · build</s>`) and who stopped it ("Stopped by HQ just now. Free again.", "Stopped by you") for about ten seconds, then Free.
- Under the grid, the total line: "Muster in total", then CPU and memory against what the machine lends and free disk.

Gauges: each row is a key, an inline `<svg viewBox="0 0 120 20" preserveAspectRatio="none">` holding an area path (`mu-gauge-area`) and a line path (`mu-gauge-line`, `vector-effect="non-scaling-stroke"`), and the latest value. Values are Muster's job containers only, never the whole machine.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `mu-slots` | Two columns, `wk-space-2` gap, at most 460px; `data-count="1"` spans one cell across. Three or four slots make a 2×2 grid; more wrap in pairs. |
| `mu-slot` | `wk-color-surface`, 1px `wk-color-hairline`, `wk-radius-group`. |
| `data-phase="running"`, `"starting"` | Busy cell. |
| `data-phase="free"`, `"stopped"` | Dashed `wk-color-hairline-strong` outline on the window ground. |
| `mu-slot-head`, `mu-slot-label`, `mu-slot-time` | The head row; the repo name gives way (ellipsis) when the cell is narrow; the time never does. |
| `mu-gauges`, `mu-gauge-key`, `mu-gauge-value` | Three-column grid, sparklines 16px tall in `wk-color-chart-1` (line 1.5px, area at 12%). |
| `mu-slot-foot` | Right-aligned Stop link at `wk-font-size-sm`. |
| `mu-slot-note` | The free or stopped line. |
| `mu-slot-total` | The total line under the grid, its first item in `wk-color-text`. |

## Do and don't

- Do keep every slot on screen; emptying, never hiding, so the grid never changes shape.
- Do say "Stop…" with the ellipsis; the Alert's default is "Keep running".
- Don't link to the run from the lender window; logs stay with HQ.
- Don't show the gauges on a free cell.

## Replaces

Design page: `.slots`, `.slot`, `.slot.busy`, `.slot.free`, `.slot.free.stopped`, `.slot .sh`, `.lbl`, `.t`, `.slot .sg` with `.k`/`.v`, `.rspark`, `.sfoot`, `.slotstop`, `.mtotal`; also the earlier `.gauges`, `.jobs`/`.jobcard`, `.jobline` and `.slotline` faces it superseded. App: `.slots`, `.slot`, `.slot-text`, `.sg`, `.sfoot`, `.slotstop`, `.mtotal`, `.mcaption`.
