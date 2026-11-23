# Logos

The shipped files from the desktop app, copied verbatim.

- `warden.svg`: the Laptop Warden mark for light grounds, transparent background, fixed colours (red `#b5352b`, paper `#ebe0c4`, ink `#1f2a3a`). The status window's header and first-run EmptyStates use it at 20–56px. Do not recolour it.
- `warden-dark.svg`: the same mark for dark grounds (red `#e0675c`, paper `#1a2230`, ink `#ece2c8`). Swap to it under `data-theme="dark"` (or a `<picture>` with `prefers-color-scheme: dark`).
- `appicon.png`: the 64 × 64 app icon with its own tile (fixed colours, the same in both themes): Alerts at 54px, notifications, the Dock.

These colours predate the neutral palette in `tokens.json`; the mark keeps them until it is redrawn.
