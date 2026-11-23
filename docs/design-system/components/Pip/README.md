# Pip

A small square per slot: filled while the slot runs a job, an outline while it is free, hatched while it cannot be used.

## When to use

Wherever slots are counted: the head of each SlotCell, the Busy column of HQ's machine tables, a machine page's header. Pips are square so they never read as StatusDots, which are round.

## What you supply

One `<span class="mu-pip">` per slot, inside `<span class="mu-pips" aria-hidden="true">`, followed by the count in words ("1 of 2", "none"). The words carry the meaning for screen readers.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `mu-pips` | A row of pips, 3px apart. |
| `mu-pip` | Free: 10px (`wk-dot-size`) square, `wk-radius-small`, 1.5px outline in `wk-color-text-secondary`. |
| `data-state="busy"` | Filled with `wk-color-text`. A job starting counts as busy. |
| `data-state="off"` | Hatched in `wk-color-text-secondary`: the machine is on break or off duty. |

## Do and don't

- Do show every slot, always; a slot never disappears when it is free.
- Do write the count beside the pips.
- Don't colour pips with the accent or a state colour: busy is ink.
- Don't scale pips up for emphasis (the design's 13px and 16px "big pips" fold into the one size).

## Replaces

Design page: `.pips`, `.pip`, `.pip.b`, `.pip.off`, `.big-pips .pip`. App: `.pip` and `.slot[data-phase] .pip`.
