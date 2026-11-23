# CodeBlock

Monospace text the person copies or reads exactly: a join code, a command to run on HQ's console, the recent log; optionally with the copy icon button.

## When to use

- A single value to hand on: the invite's join code (with copy).
- Commands to type elsewhere: `muster admin add macbookpro --manage`, `brew install colima docker`.
- The Troubleshooting log in Settings › About (`wk-code--scroll`).
- Inline code inside a sentence is a plain `<code>`, not a CodeBlock.

## What you supply

A `<div class="wk-code">` holding a `<code>` (one value) or a `<pre>` (lines, kept as typed), and optionally a `<button class="wk-copy" aria-label="Copy …" title="Copy">` with the `i-copy` glyph inline. The copy button copies the whole text. Comments in commands use the shell's `#`.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-code` | Row: text then the copy button; `wk-color-fill` ground, 1px `wk-color-control-border`, `wk-radius-control`, `wk-space-2` × 10px padding. |
| `wk-code > pre`, `wk-code > code` | `wk-font-family-mono`, `wk-font-size-sm`, wraps anywhere, selectable. |
| `wk-code--scroll` | The `<pre>` stops at 200px and scrolls. |
| `wk-copy` | 28 × 26px icon button, `wk-color-text-secondary` glyph, `wk-color-fill` on hover. Shared with ErrorBox. |

## Do and don't

- Do give the copy button a specific `aria-label` ("Copy join code").
- Do keep the block in the theme's colours; there is no dark terminal block.
- Don't truncate a code; let it wrap.

## Replaces

Design page: `.codebox` (with a "Copy" push button), `.term` (fixed dark terminal, `#1d2129` on `#e6e1d3`). App: `.logs`, the join form's monospace field read-out, `.chip code`.
