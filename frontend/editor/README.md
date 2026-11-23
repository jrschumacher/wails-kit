# @wails-kit/editor

Opt-in editors for Wails desktop apps, for the jobs that need a mature editing
library. They build on [`@wails-kit/ui`](../ui/README.md): the same tokens,
`wk-root` and kit controls, with the library themed from the `--wk-*` custom
properties, so light, dark and an app's theme apply without extra work.

`@wails-kit/ui` stays dependency-free; the editors live here so an app pays
only for the editor it imports. Each editor is its own entry and its own
prebuilt module, and importing one never loads another.

| Entry | Module | Editor |
|---|---|---|
| `@wails-kit/editor/code` | `dist/code.js` | a code editor on [CodeMirror 6](https://codemirror.net/) (MIT) |

A Markdown editor (`/markdown`) and a rich-text editor (`/richtext`) are
planned as further entries.

## Use it

The package is not published to npm. Like `@wails-kit/ui`, vendor the prebuilt
files from a pinned repository release tag that contains
`frontend/editor/dist/` (see the [ui README](../ui/README.md#use-it) for the
reasoning). The editor expects `@wails-kit/ui`'s stylesheet on the page, so
vendor that too.

```sh
tag='REPLACE_WITH_RELEASE_TAG'
checkout=$(mktemp -d)
git clone --depth 1 --branch "$tag" https://github.com/jrschumacher/wails-kit.git "$checkout"
mkdir -p vendor/wails-kit-editor
cp "$checkout/frontend/editor/dist/code.js" \
   "$checkout/frontend/editor/dist/code.d.ts" \
   "$checkout/frontend/editor/dist/THIRD-PARTY-LICENSES.txt" vendor/wails-kit-editor/
printf 'Source: https://github.com/jrschumacher/wails-kit\nTag: %s\nCommit: %s\n' \
  "$tag" "$(git -C "$checkout" rev-parse HEAD)" > vendor/wails-kit-editor/SOURCES.md
(cd vendor/wails-kit-editor && shasum -a 256 code.js code.d.ts THIRD-PARTY-LICENSES.txt > SHA256SUMS)
```

Commit the vendored files with the app, `THIRD-PARTY-LICENSES.txt` included:
`code.js` bundles CodeMirror and Lezer, and their MIT licences travel with it.

`dist/code.js` is one minified ES module with everything bundled. It imports
nothing, loads nothing from the network and starts no workers.

```html
<html lang="en" class="wk-root">
<head>
  <link rel="stylesheet" href="vendor/wails-kit-ui/wails-kit-ui.css">
  <link rel="stylesheet" href="theme.css">
</head>
```

```js
import { wkCodeEditor } from './vendor/wails-kit-editor/code.js';

const editor = wkCodeEditor({ label: 'Config', language: 'yaml', value: text, onchange: save });
document.querySelector('#config').append(editor);
```

**Content Security Policy.** The editor works under the kit pages' policy,
`default-src 'self'` with no `'unsafe-inline'` styles. CodeMirror styles
itself at run time; the build makes it do that with a constructable stylesheet
adopted by the document (not a `<style>` element, which such a policy blocks).
That needs WebView2 or Chrome 73+, or WebKit from Safari 16.4 (macOS 13.3).
On older WebKit, CodeMirror falls back to a `<style>` element, which a policy
without `'unsafe-inline'` blocks.

## Code editor: `wkCodeEditor(spec)`

```js
const editor = wkCodeEditor({
  label: 'Script',            // the accessible name; required
  value: 'echo hello',
  language: 'shell',
  onchange: value => {},
});
editor.value;                  // the text
editor.value = 'echo bye';     // replaces the text and the undo history; fires no change
editor.focus();
editor.addEventListener('wk-change', e => e.detail.value);
editor.destroy();              // tears it down; the element is left empty
```

It returns a `<div class="wk-code-editor">` holding the editor. The editor
fills the element's height when the app sets one (`height` in its CSS), and
otherwise grows with its text.

| Option | | Default |
|---|---|---|
| `label` | the accessible name, read by screen readers; **required** (it throws without one) | |
| `value` | the text | `''` |
| `language` | a bundled language name or alias (below), a CodeMirror stream parser, or none for plain text; an unknown name throws | plain text |
| `readOnly` | the text can be selected, searched and copied but not changed (`aria-readonly`); the search panel has no Replace | `false` |
| `lineNumbers` | line numbers in a gutter | `true` |
| `wrap` | wrap long lines instead of scrolling sideways | `false` |
| `tabSize` | columns per tab and per indent (Go indents with tabs) | `4` |
| `placeholder` | text shown while it is empty | |
| `tabIndents` | Tab and Shift-Tab indent instead of moving focus | `false` |
| `onchange(value)` | called after each edit, not when the app sets `value` | |
| `id` | the element's id | |

**Event** `wk-change` (bubbles) after each edit: `detail` is `{ value }`.

### Languages

| Name | Aliases and extensions |
|---|---|
| `javascript` | `js`, `jsx`, `mjs`, `cjs` |
| `typescript` | `ts`, `tsx`, `mts`, `cts` |
| `go` | `golang` |
| `json` | `jsonc`, `json5` |
| `yaml` | `yml` |
| `markdown` | `md`, `mdx` (fenced code in a bundled language is highlighted) |
| `shell` | `sh`, `bash`, `zsh`, `console` |
| `css` | `scss` |
| `html` | `htm`, `xhtml` (with its scripts and styles) |

For another language, pass a CodeMirror stream parser, such as one from
[`@codemirror/legacy-modes`](https://github.com/codemirror/legacy-modes):
`wkCodeEditor({ label: 'Query', language: sql })`. A stream parser is plain
data, so it works with the copy of CodeMirror inside `code.js`. (A CodeMirror
extension from a separate copy of CodeMirror would not, which is why the
editor takes no extensions.)

`languages` lists the bundled names; `languageName(alias)` and
`languageForPath(path)` map an alias or a file path to one (or `null`).

### Keyboard

- **Tab moves focus** out of the editor, so keyboard users are never trapped.
  With `tabIndents: true`, Tab indents instead; press **Escape, then Tab** to
  move focus out (CodeMirror's pattern).
- **Mod-F** opens find and replace (Mod is Cmd on macOS, Ctrl elsewhere);
  Enter and Shift-Enter go to the next and previous match, Escape closes it.
  The panel is drawn with the kit's field, button and checkbox classes.
- **Mod-Z** undoes, **Mod-Shift-Z** (and Mod-Y) redoes; CodeMirror's standard
  editing keys (Mod-] and Mod-[ to indent, Alt-Up and Alt-Down to move lines, …).

### Theme

The editor's colours are `--wk-*` tokens:

| Part | Tokens |
|---|---|
| Text, background, border | `--wk-color-text`, `--wk-color-control`, `--wk-color-control-border`, `--wk-radius-control` |
| Gutters | `--wk-color-text-secondary` on `--wk-color-fill`, a `--wk-color-hairline` edge |
| Selection, cursor, focus ring | `--wk-color-selection`, `--wk-color-accent` (the kit's focus ring on the whole editor) |
| Search panel | `--wk-color-surface`, kit `wk-field`, `wk-button`, `wk-check` |
| Font | `--wk-font-family-mono`, `--wk-font-size-sm`, `--wk-line-height` |
| Syntax | `--wk-color-code-keyword`, `-string`, `-number`, `-comment`, `-type`, `-function` |

The syntax tokens are defined by `@wails-kit/ui` (`src/tokens.css`) in light
and dark, each 4.5:1 or better on `--wk-color-control`. They are kit-held: a
theme file does not set them. An app theme's accent and selection reach the
editor's cursor, focus ring and selection.

**Forced colours** (Windows high contrast): the editor switches to the
system's native selection and caret, which the system draws in its own
contrast colours, and the focus ring takes `Highlight`.

## Highlighter: `highlighter(language?)`

The same syntax colours for read-only code, one line at a time. It returns
the `highlight(file, line) → DocumentFragment` hook that `@wails-kit/ui`'s
diff view takes, so diffs match the editor:

```js
import { highlighter } from './vendor/wails-kit-editor/code.js';

const highlight = highlighter();          // language from each file's path
highlight({ path: 'main.go' }, { text: 'func main() {}' }); // a DocumentFragment of spans
highlighter('yaml')(null, 'retries: 3');  // a fixed language; a line may be a string
```

With no language it picks one per file from `file.path` (else
`file.oldPath`); a file in no bundled language comes back as plain text. Each
line is highlighted on its own, so a construct spanning lines (a block
comment, a template string) is coloured only on the line where it starts.

## Layout

| Path | |
|---|---|
| `src/code/` | the code editor entry: `index.js`, `languages.js`, `index.d.ts` |
| `src/shared/` | shared by the editors: the theme (`theme-spec.js`, plain data checked by the tests; `theme.js`) and the search panel |
| `dist/` | the prebuilt modules, their types and `THIRD-PARTY-LICENSES.txt`, committed and checked against a fresh build |
| `demo/` | review pages: `index.html` (no theme), `themed.html` (ui's example theme) |
| `scripts/build.mjs` | builds `dist/` with esbuild |
| `test/` | the checks |

A new editor is a folder `src/<name>/` with `index.js` and `index.d.ts`, a
name in `ENTRIES` in `scripts/build.mjs`, and an export `./<name>` in
`package.json`. An entry may import `src/shared/` but not another entry.

## Develop

```sh
npm ci
npm run build          # after any change in src/ or a dependency update
npm test               # static checks and the build check: node --test
npm run test:browser   # the editor in Chrome
```

Dependencies are build-time only (`devDependencies`), pinned exactly and
locked; Dependabot proposes updates. After an update, rebuild and commit
`dist/`, including the regenerated `THIRD-PARTY-LICENSES.txt`.

`npm test` checks that `dist/` matches a fresh build, that each module imports
nothing and uses no network or worker API, that entries do not import each
other, that the licence file covers every bundled package, that the theme
takes every colour from a `--wk-*` token `@wails-kit/ui` defines, that
selectors are scoped, and that nothing names an app.

`npm run test:browser` serves the `frontend/` folder (the demo pages need
`@wails-kit/ui`'s stylesheet beside them) and drives the demo pages with
`playwright-core` and Chrome (set `CHROME_PATH`, or have it at
`/usr/bin/google-chrome` or the macOS default; without Chrome it skips, unless
`CI` is set). It covers typing, undo and redo, read-only, the change events,
the theme in light, dark and the example theme, syntax contrast, Tab and
focus, find, forced colours, the highlighter, no network and no CSP
violations. These run in Chrome; check WebKit by hand on a Mac for a change
in behaviour.
