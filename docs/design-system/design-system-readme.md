The **Wails Kit Design System** is the shared base for desktop apps built with Wails: the `wk-` tokens and components, used by Muster and Peanut and headed for `@wails-kit/ui`. Kit-wide rules for themes and state tones are in **Theming and tone**; each app adds a theme file and its own components. The sections below are Muster's brand book, the first app on the kit.

Muster lends spare time on people's own machines to their GitHub Actions jobs. Two windows carry it: the **status window** on every lending machine, and the **HQ window** for whoever runs the fleet. Build both from these tokens and components; set `data-theme="light"` or `"dark"` on `<html>` to follow the system appearance, and put `class="wk-root"` on the root so the base rules apply.

## Voice and copy

- Write in sentence case everywhere: titles, buttons, menu items, headings ("Take a break", "Invite a machine", "Running and waiting").
- Give each state one headline and one line, nothing more. The headline is the state's name ("On duty"); the line says what is happening now, as a sentence with a full stop ("Running 1 job.", "Waiting for jobs from HQ.", "Can't start jobs.").
- Never add a reassurance sub line ("You don't need to do anything", "Everything is fine"). If the person has nothing to do, the headline already says so.
- Use the same words on the machine and in HQ. A machine's StatusHeadline and its row in HQ name the same state; HQ may add a coarse reason after a dot ("On break · you're using it", "Off duty · by macbookpro at 1:10 pm").
- Say what failed, in plain words, then what Muster does next: "Couldn't download the job image" · "Next try 2:18 pm". Put the raw message in an ErrorBox and offer the copy icon button in its corner; never "Something went wrong".
- Say what a number counts from when it is not obvious ("jobs since HQ started at 8:02 am") and label a doubtful count ("that count may be low").
- Address the person as "you" and the machine as "this Mac" or "this machine". Name machines and repos as they are (`macbookpro`, `specgraph`, `jrschumacher/laurel`).
- Words to avoid in the product: "shut down", "killed", "terminated", "runner", "score", issue numbers, ADR references. Say "took off duty", "stopped", "removed", "machine", "job".
- End a label that opens a confirmation with an ellipsis ("Stop…", "Leave HQ…", "Revoke…"). Ask an Alert's question as a question ("Stop this job?") and make the safe choice the default ("Keep running").
- Durations: waits read "34 s", "3 h 12 m"; elapsed time in tables is `m:ss`; on a slot cell it is "3 min" or "just started".
- No emoji, no exclamation marks.

## Colour

- Build every surface from four grounds: `wk-color-bg` for windows, `wk-color-sidebar` for the HQ sidebar, `wk-color-surface` for raised boxes (Panel, SlotCell, ErrorBox, sheets, alerts), `wk-color-control` for button and field faces.
- Set text in `wk-color-text`; set labels, help and captions in `wk-color-text-secondary`. Both read at 4.5:1 or better on all four grounds in both themes.
- Draw rules with `wk-color-hairline`; use `wk-color-hairline-strong` for table header rules, Chip outlines, the free slot's dashed outline and the sheet's indent rule; outline controls with `wk-color-control-border`.
- Tint with `wk-color-fill` at rest and `wk-color-fill-strong` on hover or press; mark quiet selection with `wk-color-selection`.
- Spend `wk-color-accent` sparingly: the primary Button, Link text, the focus ring, the selected sidebar glyph, gauge lines (`wk-color-chart-1`) and the HeatMap. One primary button per view. Put `wk-color-on-accent` on any accent fill.
- The kit-wide rules for themes and for state tones (red, gold, and why there is no orange or yellow warning) are in **Theming and tone**; this section is how Muster applies them.
- Let state colours carry state and nothing else. `wk-color-working` (gold) means a job is running or finishing. `wk-color-attention` (red) means someone must act. Never use either for decoration, branding or emphasis.
- Use red only when a person can fix something: Needs attention, "HQ can't reach GitHub", "No machine can take Linux x64 jobs". A machine on break, off duty or offline is neutral.
- Write attention words in `wk-color-attention-text`; put an attention Notice on `wk-color-attention-fill`; label destructive buttons in `wk-color-destructive-text`.
- Order chart series `wk-color-chart-1` to `wk-color-chart-5` by join order; series 3 matches the working gold, so keep it away from a working dot. Charts always carry a legend.
- Known gaps, kept from the source: `wk-color-working` as a mark is 2.6:1 on the light window (the dot's shape and its words carry the state), and secondary text in a count Chip on the sidebar is 4.26:1 in light (3.6:1 on a selected item).

## Type

- Set everything in Inter (`wk-font-family`), bundled in `fonts/` as one variable file per script (weights 100–900); the system UI font is the fallback. Turn on Inter's `cv11` and `ss03` features (the base rule does this).
- Use the kit's six sizes and nothing between them: `wk-font-size-xs` 11, `-sm` 12, `-base` 13, `-md` 15, `-lg` 20, `-xl` 24.
- Use the type styles by name: `wk-title` for the StatusHeadline and EmptyState titles; `wk-heading` for sheet, dialog and toolbar titles; `wk-subheading` for SectionHeading; `wk-body` for everything else; `wk-caption` for help and captions; `wk-small` for chips and sidebar labels; `wk-figure` for large numbers; `wk-mono` (`wk-font-family-mono`) for codes, hosts, commands and raw errors.
- Use weights 400, 500 (buttons, labels, tabs), 600 (headings, figures, selected items) and 650 for the two big titles; 700 only for the toolbar title.
- Set numbers that change or line up in tabular figures (`wk-num`).
- Do not scale type for emphasis inside a view; hierarchy comes from weight, colour and space.

## Spacing and radii

- Space on the 4px scale: `wk-space-1` 4 inside a pair, `-2` 8 between related items, `-3` 12 for Panel padding and between groups, `-4` 16 for sheet and toolbar padding, `-5` 24 between sections, `-6` 32 for outer margins.
- Size controls with `wk-control-height` (28px) and `wk-control-height-small` (22px). There is no large button.
- Round corners with five radii: `wk-radius-small` 3 (checkboxes, Pips, HeatMap cells), `wk-radius-control` 6 (buttons, fields, sidebar items), `wk-radius-group` 8 (Panel, SlotCell, ErrorBox, Notice, Stat tile), `wk-radius-window` 10 (windows, sheet bottom corners, Alert), `wk-radius-pill` (Chips, switches).
- Draw every border and rule at `wk-hairline-width` (1px). Separate with hairlines and space, not shadows; keep `wk-shadow-dialog` for Sheet and Alert, `wk-shadow-popover` for menus and tooltips, `wk-shadow-control` and `wk-shadow-raised` for controls.
- Draw focus as a solid 2px `wk-color-accent` ring (`wk-focus-width`) at `wk-focus-offset`.

## Status language

Every machine is in exactly one state. Each state has one name, one line, and one StatusDot shape; the shape alone tells the state, so no state rides on colour.

| State | StatusDot | Line on the machine | In HQ |
|---|---|---|---|
| On duty, running | filled, `working` (halo at md) | "Running 1 job." | On duty since 1:32 pm |
| On duty, ready | filled, neutral | "Waiting for jobs from HQ." | On duty since 1:32 pm |
| Standing by | ringed, neutral | On duty, as usual | Standing by · no jobs it can run |
| Standing down | half, `working` | "Finishing 1 job, then it's all yours." | Standing down · finishing 1 job |
| On break | half, neutral | "You're using this machine." | On break since 12:40 pm |
| Off duty | bar, neutral | "HQ took this machine off duty at 1:10 pm." | Off duty · by macbookpro at 1:10 pm |
| Needs attention | filled, `attention` | "Can't start jobs." | Can't start jobs since 1:58 pm |
| Can't reach HQ | hollow, neutral | (headline "Can't reach HQ") | Offline since 12:40 pm |
| Removed | dashed, neutral | "This machine no longer lends to hq.schumacher.home." | Removed 3 Nov |

- Use a md dot (`wk-dot-size`, 10px) beside headlines, health lines and table states; use a sm dot (`wk-dot-size-small`, 8px) in the sidebar and beside the toolbar's connection words.
- Keep dots still. The halo on the running dot is the only emphasis; there is no pulse.
- Count slots with Pips: filled while busy, outlined while free, hatched while the machine is on break or off duty. Show every slot, always.
- Map states to dots in one place (the app's `lampDot`), never per screen.

## Layout

- **Status window**: one centred column. The StatusHeadline sits at the top; under it the SlotCell grid (two slots side by side, three or four as a 2×2 grid, more wrapping in pairs), then the "Muster in total" line, then the state's one action as a default Button. A footer bar along the bottom edge holds Links ("Details", "Open HQ"). Needs attention replaces the grid with an ErrorBox. Fixed size, no toolbar.
- **Sheet**: "This machine" drops from the status window's top edge as a Sheet: header, a body that scrolls on its own (HeatMap, Stat tiles, SectionHeadings with indented text, a FactsList), and a footer bar with "Done" at the right. The window behind keeps its live state.
- **HQ window**: Sidebar (220px) on the left, the main pane on the right. The main pane starts with a Toolbar (sidebar button, section title, connection, access Chip, the section's one action), then the content: a health line (StatusHeadline, start-aligned), at most one Notice, a ruled row of Stats, then SectionHeadings over Tables. Below about 900px the sidebar hides and tables drop low-priority columns. Opens at 1100 × 760; down to 720 × 520.
- **Settings window**: SettingsTabs along the top (General, Lending, HQ, About), then Panels of CheckRows, a `wk-settings-note` under a group, and the identity and "Leave HQ…" along the bottom.
- Show a locked Button (lock glyph, "Needs manage access") to a view-only admin rather than hiding the action.
- When there is nothing to show, use an EmptyState; never empty tables or stale numbers presented as live.

## Iconography

- Use the glyph set in `assets/Icons/`: sidebar glyphs (`i-now`, `i-hist`, `i-mach`, `i-repo`, `i-mac`) at 16px; Settings pane glyphs (`i-gen`, `i-lend`, `i-hq`, `i-about`) at 22px; access glyphs (`i-lock`, `i-eye`) at 11–12px; Stat tile glyphs (`s-jobs`, `s-time`, `s-fast`, `s-cal`, `s-slots`, `s-ok`) at 18px; `warn` for Notices at 20px; `i-copy` for the copy button at 16px.
- Draw glyphs as outlines with a 1.4–1.5 stroke (1.2–1.3 for the 12px access glyphs, 2 for `warn`), round caps where the source has them, no fills except the few solid parts the source draws.
- Inline them as `<svg>` with `stroke="currentColor"` (and `fill="currentColor"` for solid parts) so they take the text, secondary or accent colour of their place. The files in `assets/Icons/` are the same drawings as standalone SVGs in the light theme's ink (`#1b1f27`, `wk-color-text`), because an `<img>` cannot inherit colour; use them as images only on a light ground.
- The menu bar glyph is a single-colour template whose shape changes per state: `tmpl` (on duty), `tmpl-idle` (standing down, on break), `tmpl-off` (off duty), `tmpl-alert` (needs attention), `tmpl-lost` (can't reach HQ, removed).
- Use the warden mark (`assets/Logos/warden.svg`, `warden-dark.svg` in the dark theme) only in first-run and empty states, at 56px; use `appicon.png` in Alerts at 54px. Do not recolour either.
- Use no emoji and no icon font.
