# Toolbar

The bar across the top of HQ's main pane: the sidebar button, the section's title, then the connection, the admin's access and the section's one action at the end.

## When to use

Once per HQ view, under the window's title bar. The lender status window has no toolbar.

## What you supply

In order:
1. The sidebar button: a quiet icon Button (`aria-label="Show or hide sidebar"`, ⌃⌘S) at the leading edge; on a machine page, Back and Forward instead or as well.
2. `wk-toolbar-title`: the section ("Now", "Machines", "History", "Repos") or the machine's name; an optional `wk-toolbar-subtitle` says what the view counts ("last 14 days", "6 served", "27 Oct – 9 Nov").
3. `wk-toolbar-spacer`.
4. Optional view controls: a search field (`wk-search`, "Filter"), a SegmentedControl for ranges.
5. The connection in words with an sm StatusDot (`wk-dot-label`): "Connected" (filled), "Reconnecting" (hollow).
6. The access Chip: "View only" or "Can manage".
7. The section's action as a default Button ("Invite a machine"), locked for view-only admins.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-toolbar` | Wrapping row, at least 48px, `wk-space-2` × `wk-space-4` padding, `wk-color-bg` with a `wk-color-hairline` rule under it. |
| `wk-toolbar-title` | `wk-font-size-md`, `wk-font-weight-bold`. |
| `wk-toolbar-subtitle` | `wk-font-size-sm`, regular, secondary, 6px after the title. |
| `wk-toolbar-spacer` | Pushes what follows to the end. |
| `.wk-toolbar .wk-search` | 160px filter field. |

## Do and don't

- Do say the connection in words; never colour alone.
- Do keep one action; more go in the menu bar and the Dock menu.
- Don't put the app name in the toolbar; the window title is the section.

## Replaces

Design page: `.tbar`, `.tbar .ttl` and `.ttl small`, `.tbar .sp`, `.tbar .back`, `.sbtog`, `.conn` and `.conn .d`, `.search`, `.perm`, `.lend .tb` (the lender's title bar is the system's). App: none yet (HQ window not built).
