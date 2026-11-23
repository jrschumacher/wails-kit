# ThreePane

A window body in three columns: a Sidebar of sources, a list of items, and an inspector for the selected item.

## When to use

For a window whose job is to pick one item from many and look at it closely: Peanut's jobs window (agents and repositories, jobs, the selected job). A window with one main view and no selection (Muster's HQ) uses the Sidebar and a main pane without the inspector.

## What you supply

- A container `wk-three-pane` filling the window under the title bar.
- First child: a `wk-sidebar` (`wk-sidebar-width`, 220px).
- Second: a `wk-pane-list` section holding a Toolbar, optional FilterButtons, then a `wk-pane-scroll` body with the List.
- Third: a `wk-pane-inspector` aside (`wk-inspector-width`, 320px). With nothing selected it holds an EmptyState.
- Every column that scrolls gets `tabindex="0"` and a name (`aria-label`), so a keyboard can reach it and a screen reader can say what it is.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-three-pane` | Row of columns, full height; each column scrolls on its own. |
| `wk-pane-list` | Flexible middle column on `wk-color-bg`. |
| `wk-pane-scroll` | A body that scrolls under a fixed toolbar; focus ring drawn inside. |
| `wk-pane-inspector` | Fixed column, `wk-space-4` padding, `wk-color-hairline` rule on its leading edge. |

## Do and don't

- Do keep selection in the list (`aria-selected`) and show it in the inspector; the inspector never opens a second selection.
- Do hide the sidebar first when the window narrows (below about 900px), then the inspector becomes a Sheet.
- Don't put actions for the selected item in the toolbar; they go in the inspector and the item's context Menu.
