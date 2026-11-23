# Wails Kit Design System (reference export)

A read-only copy of the **Wails Kit Design System**, the design reference for
`@wails-kit/ui`. It is here so that people and agents working in this
repository can build from it without access to the original.

- **Source:** the "Wails Kit Design System" artifact on claude.ai, owned by Ryan
  (private).
- **Exported:** 2026-11-23, from artifact version `1794585489-3694`.
- **Artifact's last change note** (2026-11-13, Ryan): "Renamed to Wails Kit Design
  System; WebKitGTK 2.40 is the Linux minimum, headless below it."

**These files are a reference, not source code.** Don't edit them here: a change
to the design system is made in the artifact and a new export replaces this
folder. The implementation is [`frontend/ui/`](../../frontend/ui/) (`@wails-kit/ui`);
where the two differ, the design system is the newer source and `frontend/ui/`
catches up to it.

## What is here

| Path | What it is |
|---|---|
| `README.md` (this file) | What the export is and how to read it. |
| `design-system.json` | The artifact's manifest: title, asset groups, the upload id of each icon and logo, the last change note. |
| `tokens.json` | Every token, with light and dark values and a usage note (often with measured contrast). |
| `components/bundle.css` | The full stylesheet: the kit's `wk-` sections and Muster's `mu-` sections, the derived colours, utilities and the forced-colours blocks. Component CSS only; values come from the tokens. |
| `theming.md` | Kit-wide rules: what a theme may set, the derived colours, where `color-mix()` works, the three state tones. |
| `design-system-readme.md` | The design system's own README, renamed; see below. |
| `components/<Name>/README.md` | One card per component: when to use it, what you supply, its classes and attributes, do and don't, what it replaces. |
| `components/<Name>/preview.html` | The card's preview page. It expects the tokens and `bundle.css` to be loaded by the design-system viewer, so it does not render on its own. |
| `fonts/` | Inter, one variable woff2 per subset (`Inter-latin.woff2`, `Inter-latin-ext.woff2`, weights 100–900) and its licence (`OFL-Inter.txt`, SIL OFL 1.1). |
| `assets/Icons/*.svg` | Muster's glyph set as standalone SVGs. See `assets/Icons/README.md`. |
| `assets/Logos/*` | Muster's warden mark (light and dark) and app icon. See `assets/Logos/README.md`. |

### The design system's README

The artifact's own README (Muster's brand book on the kit: voice, colour, type,
spacing, status language, layout, iconography) was exported as
`README.md`. To keep this folder's README about the export, it is saved
here as [`design-system-readme.md`](design-system-readme.md), unchanged.

### Uploads in the previews

Preview pages reference uploaded images by their upload id, as
`/_blob/<id>`. `design-system.json` maps each id to a file name under
`assets/`; for example `d2f34b8e9eff15bed515cd1e4ae5a0ff` is
`assets/Logos/appicon.png`, `53865862f19e0ce0130b4047b86227dc` is
`assets/Logos/warden.svg` and `68383adba0f3895abdacc5e2f5d7df4e` is
`assets/Logos/warden-dark.svg`.

## Kit (`wk-`) and Muster's own (`mu-`)

The design system holds two layers, and only the first belongs in
`@wails-kit/ui`:

- **Kit, `wk-`**: app-neutral tokens and components shared by every app on the
  kit (today Muster and Peanut). Alert, Button, CheckRow, Chip, CodeBlock,
  EmptyState, ErrorBox, FactsList, FilterButtons, Link, List, Menu, Notice,
  Panel, Popover, Range, SearchField, SectionHeading, SegmentedControl,
  SettingsTabs, Sheet, Sidebar, Stat, StatusDot, Stepper, Table, ThreePane,
  Toolbar, the utilities, the derived colours and the forced-colours rules.
  These are what `frontend/ui/` implements, with the same class and token
  names.
- **Muster's own, `mu-`**: Pip, SlotCell, StatusHeadline, HeatMap and the
  gauges, with their `--mu-*` tokens. They stay in Muster and are not part of
  `@wails-kit/ui`. Their cards are kept here because the kit cards refer to
  them and because they show how an app builds its own components on the kit.

The voice, status language, layout and iconography in the design system's
README, the icons and the logos are also Muster's. The kit-wide rules are in
`theming.md`.
