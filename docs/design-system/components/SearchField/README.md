# SearchField

A text field with a magnifier that filters the list or table in the same view as you type.

## When to use

In a Toolbar, before the view's action, when a list is long enough to scan: Peanut's jobs, Muster's machines. It filters what is already loaded; it never searches a server.

## What you supply

- `<span class="wk-search"><input class="wk-field" type="search" placeholder="Filter …" aria-label="…"></span>`.
- A status line (`role="status"`) that says how many items match ("3 of 44 match"), in the toolbar subtitle or under the field.
- ⌘F (Ctrl+F) focuses it; Esc clears it.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-search` | Wrapper that draws the magnifier in `wk-color-text-secondary`. |
| `.wk-toolbar .wk-search` | 160px wide. |

## Do and don't

- Do name what it filters in the placeholder ("Filter jobs").
- Do match on the words a person sees (title, repository, number, reviewer).
- Don't hide the match count; an empty list without it looks broken.
