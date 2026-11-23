# FactsList

Label and value pairs in two aligned columns: how a machine is set up, when access was issued, what an error box knows.

## When to use

For a handful of facts that each have a short name: "Set up as" in the "This machine" sheet, HQ's machine Setup card, the HQ access group in Settings, the About pane, and inside an ErrorBox ("Failed", "Next try"). For many rows or comparable values across items, use a Table.

## What you supply

A `<dl class="wk-facts">` with `<dt>`/`<dd>` pairs. Labels are nouns in sentence case without a colon ("Job types", "Last used"); values are plain text and may hold a Chip ("update available") or `wk-mono` for identifiers.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-facts` | Grid: labels auto width, values fill the rest; `wk-space-1` row gap, `wk-space-3` column gap, `wk-font-size-base`. |
| `dt` | `wk-color-text-secondary`. |
| `dd` | `wk-color-text`; long values wrap anywhere. |

## Do and don't

- Do keep labels to one or two words.
- Do write values in the words of the rest of the app ("2 × 5 CPU · 6 GiB", "Refused by this Mac").
- Don't put actions in a value; put them in a Panel row.

## Replaces

Design page: `.facts`, `.errbox dl`, `.verify .k` pairs. App: `.factsheet` (gap 6px 18px), `.wk-settings-group > .factsheet`.
