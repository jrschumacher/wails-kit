# Menu

A short list of commands on one item or one window: a context menu on a list row, a sort menu from a button, the actions in a tray Popover.

## When to use

- **Context menu** (`wk-context-menu` around the target): right-click, Shift-F10 or the menu key on a List row or Table row.
- **Button menu**: a `wk-button` with `aria-haspopup="menu"` and `aria-expanded` opens it below.
- **Inline** (`wk-menu--inline`): the same items laid out in place, in a Popover.

## What you supply

- `<div class="wk-menu" role="menu" aria-label>` holding `role="menuitem"` buttons, or `<a role="menuitem">` for a URL; `menuitemradio` and `menuitemcheckbox` with `aria-checked` for choices.
- `wk-menu-shortcut` at the end of an item for its key ("⌘O" on macOS, "Ctrl+O" elsewhere).
- `role="separator"` between groups; `wk-menu-heading` to name the target when it is not obvious.
- An item that does not apply now: `aria-disabled="true"`, kept in place so the menu's shape stays the same. Leave it out only when it can never apply to this item.
- Behaviour from the `wk-menu` / `wk-context-menu` custom elements: focus moves into the menu, arrows move, Esc closes and returns focus.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-menu` | Floating column, 180–320px, `wk-color-surface`, `wk-radius-group`, `wk-shadow-popover`. |
| `[role^="menuitem"]` | 24px row; focus fills it with `wk-color-accent`, text `wk-color-on-accent`. |
| `aria-checked="true"` | A check mark in the leading gutter. |
| `wk-menu-shortcut` | Trailing, secondary. |
| `wk-menu--inline` | No float, border or shadow: the items in place. |

## Do and don't

- Do end an item that asks for more with an ellipsis ("Check PR now…").
- Do keep menus to about eight items.
- Don't nest submenus.
