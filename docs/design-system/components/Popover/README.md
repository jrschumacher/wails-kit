# Popover

A panel that opens from the menu bar or tray icon: the app's state at a glance, its one switch, a few live items and its menu.

## When to use

For the tray (Windows, Linux) or menu bar (macOS) window of an app that runs in the background: Peanut's tray. It is a small window of its own, 280–320px wide, closed by Esc or a click elsewhere. Anything longer than a glance opens the main window.

## What you supply

- A `wk-popover` with `role="dialog"` and the app's name as its label.
- `wk-popover-header`: the app name and its connection or state as a small StatusDot with words.
- One or more `wk-popover-section`s: a switch row, up to three live items, a short message.
- The app's actions as a Menu laid out in place (`wk-menu wk-menu--inline`) with shortcuts.
- Optionally a `wk-popover-footer` with a caption and one small button.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-popover` | `wk-color-surface`, `wk-color-hairline-strong` edge, `wk-radius-window`, `wk-shadow-popover`. |
| `wk-popover-header` | Row, `wk-space-3` × 14px padding, rule below. |
| `wk-popover-section` | 10px × 14px padding; rules between sections. |
| `wk-popover-footer` | Row on `wk-color-bg`, rule above. |

## Do and don't

- Do say the state in words in the header; never colour alone.
- Do keep forms in the popover short (two fields) because it cannot host a Sheet.
- Don't scroll the popover; cap its list and link to the main window.
