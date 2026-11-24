# @wails-kit/editor

Opt-in editors for Wails desktop apps, for the jobs that need a mature editing
library. They build on [`@wails-kit/ui`](../ui/README.md): the same tokens,
`wk-root` and kit controls, with the library themed from the `--wk-*` custom
properties, so light, dark and an app's theme apply without extra work.

`@wails-kit/ui` stays dependency-free; the editors live here so an app pays
only for the editor it imports. Each editor is its own entry and its own
prebuilt module, and importing one never loads another. What the editors
share (CodeMirror, the theme, the languages; marked, the Markdown limits and
the link rules; style-mod) is in shared chunks, `dist/chunk-*.js`, which the
entries import by relative path, so an app that uses two editors loads one
copy of each.

| Entry | Module | Editor |
|---|---|---|
| `@wails-kit/editor/code` | `dist/code.js` | a code editor on [CodeMirror 6](https://codemirror.net/) (MIT) |
| `@wails-kit/editor/markdown` | `dist/markdown.js` | a Markdown editor (Write and Preview) on the same CodeMirror, and a safe Markdown renderer on [marked](https://marked.js.org/)'s parser (MIT) |
| `@wails-kit/editor/richtext` | `dist/richtext.js` | a rich-text (WYSIWYG) editor on [ProseMirror](https://prosemirror.net/) (MIT), reading and writing the Markdown editor's Markdown (or sanitised HTML) |

## Use it

The package is not published to npm. Like `@wails-kit/ui`, vendor the prebuilt
files from a pinned repository release tag that contains
`frontend/editor/dist/` (see the [ui README](../ui/README.md#use-it) for the
reasoning). The editor expects `@wails-kit/ui`'s stylesheet on the page, so
vendor that too.

**Vendor the whole `dist/` folder.** An entry imports the shared
`chunk-*.js` files beside it (`code.js` is a few hundred bytes that import
the chunk holding CodeMirror), so copying `code.js` alone breaks it. The
chunk names carry a content hash and change between releases: replace the
folder, rather than adding files to it.

```sh
tag='REPLACE_WITH_RELEASE_TAG'
checkout=$(mktemp -d)
git clone --depth 1 --branch "$tag" https://github.com/jrschumacher/wails-kit.git "$checkout"
rm -rf vendor/wails-kit-editor
cp -R "$checkout/frontend/editor/dist" vendor/wails-kit-editor
printf 'Source: https://github.com/jrschumacher/wails-kit\nTag: %s\nCommit: %s\n' \
  "$tag" "$(git -C "$checkout" rev-parse HEAD)" > vendor/wails-kit-editor/SOURCES.md
(cd vendor/wails-kit-editor && shasum -a 256 *.js *.d.ts THIRD-PARTY-LICENSES.txt > SHA256SUMS)
```

Commit the vendored files with the app, `THIRD-PARTY-LICENSES.txt` included:
the modules bundle CodeMirror, Lezer, marked and ProseMirror, and their MIT licences
travel with them. The file says which entries load each package.

The modules are minified ES modules with everything bundled. They import only
each other (`./chunk-*.js`, relative), load nothing from the network and start
no workers. The import path of the code editor is unchanged
(`vendor/wails-kit-editor/code.js`).

### Size

Minified, then gzipped, as built from this tree:

| File | Minified | Gzipped | Loaded by |
|---|---|---|---|
| `chunk-*.js` (CodeMirror, Lezer, the theme, the nine languages) | 596 KiB | 209 KiB | code, markdown |
| `chunk-*.js` (marked, the Markdown limits, the link rules, the rendered-Markdown styles) | 54 KiB | 17 KiB | markdown, richtext |
| `chunk-*.js` (style-mod, w3c-keyname) | 5 KiB | 3 KiB | all three |
| `code.js` | under 1 KiB | under 1 KiB | |
| `markdown.js` (the editor, the renderer) | 18 KiB | 7 KiB | |
| `richtext.js` (the editor, ProseMirror) | 245 KiB | 78 KiB | |

So the code editor costs about 601 KiB (212 KiB gzipped), the Markdown editor
about 673 KiB (235 KiB), the rich-text editor about 304 KiB (97 KiB), and all
three together about 918 KiB (313 KiB): each further editor adds only what it
does not share. These are loaded from the app's own files, never the
network.

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

### Browser floor

**`@wails-kit/editor` needs macOS 13.3 or later (Safari 16.4+), WebKitGTK
2.40+ on Linux, or WebView2 on Windows.** That is above `@wails-kit/ui`'s
macOS 13 floor.

The reason is the Content Security Policy. The editor works under the kit
pages' policy, `default-src 'self'` with no `'unsafe-inline'` styles.
CodeMirror styles itself at run time, and the build makes it do that with a
constructable stylesheet adopted by the document (`document.adoptedStyleSheets`)
instead of a `<style>` element, which such a policy blocks. On an older WebKit
without adopted stylesheets, CodeMirror falls back to a `<style>` element;
under a strict CSP that is blocked, CodeMirror's structural CSS is missing and
the editor breaks. `wkCodeEditor`, `wkMarkdownEditor` and `wkRichTextEditor` log a one-time `console.warn` when
`document.adoptedStyleSheets` is unavailable. The build patches style-mod
(CodeMirror's style injector) once, in the shared chunk, so every entry gets
the patch; `npm test` checks it.

### Overriding CodeMirror's styles

CodeMirror's styles arrive as an adopted stylesheet, which comes after the
page's own stylesheets in the cascade. An app rule for a `.cm-*` class
therefore loses to CodeMirror's rule of the same specificity: give it more
specificity than CodeMirror's own (it scopes its rules with generated classes,
such as `.ͼ1.cm-editor .cm-gutters`; for example, prefix yours with
`.wk-code-editor .cm-editor`), or, better, change the look through the
`--wk-*` tokens the theme reads.

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
otherwise grows with its text. An editor measures its line heights when it
first comes into view (CodeMirror's design), so one built off-screen settles
its layout when it is scrolled to. After `destroy()`, setting `value` and
`focus()` do nothing. Setting `value` while focus is in the editor (its find
panel included) leaves focus in the text.

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

**Event** `wk-change` (bubbles) after each edit: `detail` is `{ value }`. It
fires before `onchange` is called. In `code.d.ts` the
returned `CodeEditorElement` types `addEventListener('wk-change', …)` with a
`CodeEditorChangeEvent` (`CustomEvent<{ value: string }>`); the global
`HTMLElementEventMap` is left alone, since other kit elements fire `wk-change`
with other details.

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

## Markdown editor: `wkMarkdownEditor(spec)`

A comment box, as on a pull request: write Markdown, preview it, submit. It
is also the box for review comments on a diff.

```js
import { wkMarkdownEditor, renderMarkdown } from './vendor/wails-kit-editor/markdown.js';

const box = wkMarkdownEditor({
  label: 'Comment',                  // the accessible name; required
  placeholder: 'Leave a comment',
  maxLength: 65536,                  // a live count; a longer text does not submit
  onlink: url => Browser.OpenURL(url),   // preview links: open them outside the webview
  actions: [
    { label: 'Cancel', cancel: true },
    { label: 'Comment', primary: true, submit: true },
  ],
  oncancel: editor => editor.destroy(),   // Cancel, or Escape
  onsubmit: async value => { await Comments.Post(value); box.value = ''; },
});
document.querySelector('#reply').append(box);
```

**The page must load `@wails-kit/ui`'s script** (`kit.js`, or
`dist/wails-kit-ui.js`): the Write and Preview tabs are its `<wk-tabs>`.
Without it the editor logs a one-time `console.warn` and the tabs do not
switch.

It returns a `<div class="wk-markdown-editor">`:

- **Write** and **Preview** tabs (`<wk-tabs>`, the WAI-ARIA tabs: Left and
  Right switch them, Home and End). Preview renders the current text with
  `renderMarkdown` and starts as tall as the text was, so the page does not
  jump.
- In Write, the text: the code editor in Markdown mode, wrapping, without line
  numbers, growing with its content between `minHeight` and `maxHeight`, then
  scrolling. Tab moves focus on, as in the code editor.
- A formatting toolbar beside the tabs (Write only): quiet icon buttons, one
  tab stop, Left and Right between them (the WAI-ARIA toolbar pattern). A
  button acts on the selection and leaves focus in the text.
- Under it, the count (with `maxLength`), the error, and the action buttons.

| Option | | Default |
|---|---|---|
| `label` | the accessible name of the text and of the tab list; **required** (it throws without one) | |
| `value` | the text | `''` |
| `placeholder` | text shown while it is empty | |
| `mode` | the tab it opens on: `'write'` or `'preview'` | `'write'` |
| `toolbar` | the formatting toolbar; the shortcuts work without it | `true` |
| `minHeight`, `maxHeight` | CSS lengths, or numbers of pixels: the text grows between them, then scrolls | `100px`, `360px` |
| `maxLength` | shows `n / max` under the text; past it, the count turns attention-coloured and bold (underlined in forced colours), and submitting says how much to cut | none |
| `required` | an empty (or blank) text does not submit | `true` |
| `disabled` | the text is read-only and the toolbar and actions are disabled (a `cancel` action stays usable); the tabs and the preview still work. Disabling while focus is on a toolbar button or an action moves focus to the editor element (a `role="group"` named by `label`); enabling it again moves that focus back to the text | `false` |
| `actions` | the buttons under it, in order: `{ label, primary?, submit?, cancel?, onclick?(editor) }`. A `submit` action submits, a `cancel` action cancels; the others call `onclick` | none |
| `onchange(value)` | after each edit, not when the app sets `value` | |
| `onsubmit(value)` | on submit. Return a promise to keep the submit actions busy until it settles (below) | |
| `oncancel(editor)` | on cancel: a `cancel` action, `cancel()`, or Escape (below). Without it, Escape is left to the page (a dialog around the editor) | |
| `onmode(mode)` | when the person switches tabs | |
| `onlink(url)` | a click on a preview link (see `renderMarkdown`) | |
| `imageHosts` | as `renderMarkdown`'s | none |
| `id` | the element's id; also the stem of its parts' ids | |

**Properties and methods:** `value` (setting it replaces the text and its undo
history, clears an error and fires nothing), `mode` (`'write'` or
`'preview'`; setting it fires nothing), `disabled`, `focus()` (shows Write and
focuses the text), `submit()` (as the submit action; returns whether it
submitted), `cancel()` and `destroy()` (the element is left empty; then
`value`, `focus()`, `submit()` and `cancel()` do nothing).

**Events** (all bubble from the element; in `markdown.d.ts` the returned
`MarkdownEditorElement` types them):

| Event | `detail` | When |
|---|---|---|
| `wk-change` | `{ value }` | after each edit, before `onchange` |
| `wk-mode` | `{ mode }` | the person switched tabs, before `onmode` (the tabs' own `wk-change` stops at the editor) |
| `wk-submit` | `{ value }` | before `onsubmit`; cancelable: `preventDefault()` stops the submit |
| `wk-cancel` | `null` | on cancel, before `oncancel` |

### Keyboard

| Keys | |
|---|---|
| **Mod-B**, **Mod-I** | bold (`**`), italic (`_`); again to unwrap |
| **Mod-E** | code: `` ` `` around the selection, or a fenced block when it spans lines |
| **Mod-K** | link: `[text](url)` with `url` selected to type over; a selected URL becomes the target |
| toolbar only | bulleted list (`- `) and quote (`> `) on each selected line; again to remove |
| **Mod-Enter** | submit |
| **Escape** | cancel, when `oncancel` is set. Not while an input method is composing or a find panel is open (Escape closes the panel first), and not when the same press collapses a selection; from the text, the tabs, the preview, the toolbar or the actions. It calls `preventDefault()` but does not stop propagation, so an outer listener sees the key with `defaultPrevented` set. It cancels while a submit is pending too: the app decides what that means for the request in flight |

Mod is Cmd on macOS, Ctrl elsewhere. The toolbar's buttons carry the keys in
`aria-keyshortcuts` and their `title`. The code editor's keys (find, undo) work
too.

### Submitting

Submitting (Mod-Enter, a `submit` action or `submit()`) checks the text
first. An empty text (with `required`) says "Write something first."; one past
`maxLength` says "Shorten this by 3 characters.". The message sits under the
text (`aria-describedby`, `aria-invalid`) in a polite live region
(`role="status"`, in the page from the start and empty until needed), so it is
announced even when focus is already in the text (Mod-Enter). Focus goes to
the text (on Write), and the message clears as the person types it right. The submit button stays enabled,
as the [behaviour guidelines](../ui/BEHAVIOUR.md#forms-and-validation) ask.

Then `wk-submit` fires, then `onsubmit(value)`. When `onsubmit` returns a
promise, the submit actions get `aria-disabled="true"` and `data-busy` (they
keep focus and their name; the busy state is on the control, as
[BEHAVIOUR.md](../ui/BEHAVIOUR.md#accessibility) says, not `aria-busy` on the
editor) until it settles, and another submit does nothing. The editor does not clear the
text or report a failure itself: on success the app sets `value = ''` (or
removes the editor), and on failure it shows the error.

### As a comment box on a diff

The editor is a self-contained element with no page-level state, so a diff
view can insert one under a line and remove it again: create it with
`actions` (a `cancel` action and a `submit` one) and `oncancel`, call
`focus()` once it is in the document, post in `onsubmit`, and `destroy()` it
on cancel or when the thread closes. A posted comment's body is
`renderMarkdownElement(body, { onlink })`.

Two things to know when the diff view handles keys itself:

- **Escape** in the editor calls `preventDefault()` and does not stop
  propagation (with `oncancel` set). A diff-level Escape handler (closing a
  thread, leaving a hunk) should skip a keydown whose `defaultPrevented` is
  set, or the press would act twice.
- **Escape cancels while a submit is pending.** The submit actions are busy,
  but `oncancel` still runs: abort the request, or keep the box until it
  settles, as the app needs.

### Classes

Proposals for the design system (it has no Markdown card yet):

| Class | |
|---|---|
| `wk-markdown-editor` | the editor; `data-disabled` while disabled |
| `wk-markdown-editor-toolbar` | the formatting toolbar beside the tabs |
| `wk-markdown-editor-preview` | the preview panel's content (also `wk-markdown`) |
| `wk-markdown-editor-empty` | "Nothing to preview" |
| `wk-markdown-editor-foot`, `-status`, `-count` (`data-over`), `-error`, `-actions` | the footer |
| `wk-markdown` | rendered Markdown |
| `wk-markdown-table`, `wk-markdown-task` | a table's scroll wrapper; a task list item |
| `wk-markdown-source` | text past the renderer's limits, shown as written |

It also uses the kit's `wk-tablist`, `wk-button` (`--quiet`, `--icon`,
`--primary`), `wk-code`, `wk-link`, `wk-table` and `wk-checkbox`. The styles
come from the module as an adopted stylesheet (allowed by the strict CSP),
with every colour a `--wk-*` token, so light, dark, forced colours and an app
theme apply.

## Rendering Markdown: `renderMarkdown(text, options?)`

The preview's renderer on its own, to show posted comments read-only under
the same rules. It returns a `DocumentFragment`; put it in an element with the
class `wk-markdown`, which styles it with the kit's type, `wk-code` for code
blocks, `wk-link` for links and `wk-table` for tables.

```js
const comment = document.createElement('div');
comment.className = 'wk-markdown';
comment.append(renderMarkdown(body, { onlink: url => Browser.OpenURL(url) }));

// The same, in one call: a <div class="wk-markdown"> holding the output.
const body = renderMarkdownElement(text, { onlink: url => Browser.OpenURL(url) });
```

It reads CommonMark with GitHub's tables, task lists (shown as disabled
checkboxes), strikethrough and autolinks, parsed by marked's lexer. The DOM is
built from the parsed tokens with `createElement` and `textContent`, never by
parsing HTML, so the text can come from anyone (a pull request, a review):

- **No raw HTML.** HTML in the text (`<script>`, `<img onerror>`, `<iframe>`,
  SVG, comments) shows as the text it is. The output holds only the elements
  and attributes the renderer makes.
- **Links** go only to absolute `http`, `https` and `mailto` URLs, as the
  browser's URL parser reads them after character references are decoded (so
  `javascript:`, `&#106;avascript:`, `java\tscript:`, `data:`, relative and
  fragment links are refused and render as their text). A link carries
  `rel="noopener noreferrer"` and no `target`; a click (or a middle-click)
  never navigates the webview: it calls `onlink(url)`, which the app opens in
  the system browser. Without `onlink`, a click does nothing.
- **No remote images.** An image would load from its server and tell it the
  text was read, so by default it renders as a link to the image, labelled
  with its alt text. `imageHosts: ['images.example.com']` loads `https`
  images from those hosts (exact names), with `referrerpolicy="no-referrer"`;
  the page's CSP must allow them too.
- **Code blocks** take the editors' syntax colours when their info string
  names a bundled language (`go`, `ts`, `yaml`, …), parsed as one block.

| Option | | Default |
|---|---|---|
| `onlink(url)` | a link's click | none: clicks do nothing |
| `imageHosts` | hosts whose `https` images load | `[]` |
| `highlight` | colour fenced code in a bundled language | `true` |

Named character references cover the common ones (`&amp;`, `&copy;`,
`&mdash;`, …) and every numeric one; an unknown name shows as written.

**Limits.** `renderMarkdown` never throws. marked's parser recurses for
each nested quote, list, emphasis run and bracket, so a few kilobytes of
hostile text can overflow the stack, and deep lists take time quadratic in
their depth. A quick scan (`markdownLimits`, outside fenced code) checks
three limits first, and text past any of them renders as its source, one
`<p class="wk-markdown-source">` that keeps its line breaks, without reaching
the parser:

| Limit | | Value |
|---|---|---|
| `MAX_MARKDOWN_LENGTH` | characters (UTF-16 code units) in the text | 100,000 |
| `MAX_MARKDOWN_DEPTH` | nesting of quotes and lists on a line: the quote and list markers it starts with, plus its list indentation at two columns a level | 32 |
| `MAX_MARKDOWN_DELIMITERS` | emphasis and link delimiters (`*`, `_`, `~`, `[`, `]`) in one run of inline text: a run ends where emphasis cannot carry on, at a blank line, a heading, a new list item or a table row (only under a header row with as many cells as the delimiter row, as GFM makes a table), so a long list or table counts each item or row on its own | 2,000 |
| `MAX_MARKDOWN_BLOCK_DELIMITERS` | the same delimiters between two blank lines or headings, whatever the runs: a backstop that keeps the parser's time bounded (under a second) even if a run reset is wrong. A list or table longer than this with no blank line or heading renders as its source | 8,000 |

The scan reads fences as CommonMark does (a backtick fence's info string has
no backtick; `\r\n`, `\r` and `\n` end lines) and errs on the high side: a
fence in a quote or list item stops hiding its lines at the first line that
does not repeat the quote's markers or is less indented than the fence, and a
list marker that might only continue a paragraph (a number other than 1 after
a paragraph, a marker four columns past the item's content, a marker in a
quote) does not end a run. A 700-item list of release notes or a 600-row
table renders as Markdown. Text the parser still fails on renders as its
source too. The limits are exported.

The editor's Write tab drops Markdown highlighting while the text is past
the depth or delimiter limit, since CodeMirror's Markdown parser and
highlighter recurse too. Under that, a safety net for text the scan misses:
a stack overflow (a `RangeError` whose message mentions the call stack)
while building the editor, setting `value` or applying an edit (typing, a
paste, from the editor's current state) builds the state again with
highlighting off for that text, so nothing throws and no edit is lost. The
editor changes its own state only once that retry has worked; any other
error, or a retry that fails, is thrown as it was. Highlighting comes back
when the app sets a new `value`.

## Rich-text editor: `wkRichTextEditor(spec)`

Bold, lists and links edited in place, with no visible markup: for people
who should not have to write Markdown. Its value is Markdown in the Markdown
editor's dialect, so the two editors (and `renderMarkdown`) can edit and show
the same text; or sanitised HTML.

```js
import { wkRichTextEditor } from './vendor/wails-kit-editor/richtext.js';

const notes = wkRichTextEditor({
  label: 'Release notes',                 // the accessible name; required
  value: markdown,
  placeholder: 'Write the notes',
  onlink: url => Browser.OpenURL(url),     // links open outside the webview
  onchange: value => draft.save(value),
  actions: [
    { label: 'Cancel', cancel: true },
    { label: 'Save', primary: true, submit: true },
  ],
  oncancel: editor => editor.destroy(),
  onsubmit: async value => { await Notes.Save(value); },
});
document.querySelector('#notes').append(notes);
```

It needs `@wails-kit/ui`'s stylesheet on the page, not its script. It
returns a `<div class="wk-richtext-editor">` (a `role="group"` named by
`label`) holding:

- the formatting toolbar: kit quiet icon buttons in groups (marks and link;
  headings; lists, quote and code block; rule) split by separators, with
  `aria-pressed` on each toggle following the selection;
- the text: a `role="textbox"` with `aria-multiline="true"` and the label,
  styled as rendered Markdown (`wk-markdown`: the kit's type, `wk-code` code
  blocks, `wk-link` links) in a field's box that grows between `minHeight`
  and `maxHeight`, then scrolls;
- the link popover (a kit `wk-popover`), opened from the toolbar or Mod-K;
- the footer: the error and the action buttons, as in the Markdown editor.

| Option | | Default |
|---|---|---|
| `label` | the accessible name of the text; **required** (it throws without one) | |
| `value` | the content, in `format` | `''` |
| `format` | `'markdown'` or `'html'`: what `value`, `onchange` and `onsubmit` read and write; another value throws | `'markdown'` |
| `features` | the features to offer, any of `bold`, `italic`, `code`, `link`, `heading` (two levels), `bulletList`, `orderedList`, `quote`, `codeBlock`, `rule`; an unknown name throws. Content the editor does not offer is cut down when loaded or pasted (below) | all |
| `placeholder` | text shown while it is empty (also `aria-placeholder`) | |
| `toolbar` | the formatting toolbar; the shortcuts work without it | `true` |
| `readOnly` | the text can be read, selected, copied and its links followed (Tab reaches them), not changed (`aria-readonly`); no toolbar | `false` |
| `disabled` | the text cannot be changed (`aria-disabled`) and the toolbar and actions are disabled (a `cancel` action stays usable). Disabling while focus is on a toolbar button or an action moves focus to the editor element; enabling it again moves that focus back to the text | `false` |
| `minHeight`, `maxHeight` | CSS lengths, or numbers of pixels | `100px`, `360px` |
| `required` | an empty text does not submit | `true` |
| `actions` | the buttons under it: `{ label, primary?, submit?, cancel?, onclick?(editor) }`, as the Markdown editor's | none |
| `onchange(value)` | after each edit, not when the app sets `value` | |
| `onsubmit(value)` | on submit; return a promise to keep the submit actions busy until it settles | |
| `oncancel(editor)` | on cancel: a `cancel` action, `cancel()`, or Escape in the editor | |
| `onlink(url)` | a link opened by the person (below): an `http`, `https` or `mailto` URL | |
| `id` | the element's id; also the stem of its parts' ids | |

**Properties and methods:** `value` (setting it replaces the content and its
undo history, clears an error and fires nothing), `format` (read-only),
`disabled`, `focus()`, `submit()` (returns whether it submitted), `cancel()`
and `destroy()` (the element is left empty; then `value`, `focus()`,
`submit()` and `cancel()` do nothing). `FEATURES` lists the feature names.

**Events** (bubbling; typed in `richtext.d.ts`): `wk-change` (`{ value }`,
after each edit, before `onchange`), `wk-submit` (`{ value }`, cancelable,
before `onsubmit`) and `wk-cancel` (`null`, before `oncancel`).

Submitting, the error ("Write something first.", in a polite live region
under the text, with `aria-invalid` and `aria-describedby` on it, cleared as
the person types) and the busy submit actions (`aria-disabled="true"` and
`data-busy` while `onsubmit`'s promise is pending) work as in the
[Markdown editor](#submitting).

### Keyboard

| Keys | |
|---|---|
| **Mod-B**, **Mod-I**, **Mod-E** | bold, italic, inline code; on the selection, or for what is typed next |
| **Mod-K** | the link popover |
| **Mod-Shift-8**, **Mod-Shift-7** | bulleted list, numbered list (again to lift the item out) |
| **Mod-]**, **Mod-[** | indent or outdent a list item |
| **Enter**, **Shift-Enter** | a new paragraph or list item; a line break (a new line in a code block) |
| **Mod-Z**, **Mod-Shift-Z** (and **Mod-Y**) | undo, redo |
| **Mod-Enter** | submit (when there is an `onsubmit` or a submit action; otherwise it leaves a code block) |
| **Escape** | cancel, when `oncancel` is set (not while an input method is composing); in the link popover, it closes the popover instead. ProseMirror calls `preventDefault()` on Escape in the text either way, so an outer listener sees `defaultPrevented` set |
| **Alt-F10** | from the text to the toolbar |
| **Tab**, **Shift-Tab** | move focus on and back (toolbar, text, actions); the text never traps it |

In the toolbar (one tab stop, the WAI-ARIA toolbar pattern), Left and Right
move between the buttons and wrap, Home and End go to the first and last,
and Enter or Space applies the button and returns focus to the text. The
buttons carry their keys in `aria-keyshortcuts` and `title`. A key acts on
the selection the person just made, even right after Shift-Arrow.

**Markdown as you type** (for the features offered): `# ` and `## ` make a
heading, `- `, `* ` or `+ ` a bulleted list, `1. ` (any number) a numbered
list, `> ` a quote, ```` ``` ```` (with a language) and a space or Enter a
code block, and `` `text` `` inline code. Backspace straight after one puts
the typed text back. None fire inside a code block.

### Links

The link popover (`role="dialog"`, named "Link") has a field for the address,
Save, Cancel and, on an existing link, Open and Remove. A selection becomes
the link's text; with nothing selected, the address is inserted as the text.
The address must be an `http`, `https` or `mailto` URL (`example.com` gets
`https://`, `a@example.com` gets `mailto:`); anything else shows "Use a web
or email address, such as https://example.com." in a polite live region on
the field (`aria-invalid`), and the field keeps focus. Save and Remove return
focus to the text; Cancel and Escape return it to what opened the popover
(the toolbar button, or the text for Mod-K).

A link never navigates the webview. In the text, a click places the cursor;
Mod-click (or any click while the text is read-only or disabled, or Enter on
a link in a read-only text, or Open in the popover) calls `onlink(url)` with
the safe URL. Links carry `rel="noopener noreferrer"` and no `target`.

### Markdown in and out

Markdown is read by marked's lexer, as `renderMarkdown` reads it, and written
in one spelling per feature, so text in that spelling comes back unchanged:

| Feature | Written as |
|---|---|
| headings | `# Heading`, `## Subheading` (deeper headings load as `##`) |
| bold, italic, code | `**bold**`, `_italic_` (`*italic*` inside a word), `` `code` `` (longer fences around backticks) |
| links | `[text](url)`; a link whose text is its address as the bare address (`https://…`, `www.…`, an e-mail address) or `<url>` |
| lists | `- ` bullets, `1. ` numbers from the list's start; `*` or `1)` for a list straight after one of the same kind; tight lists stay tight unless Markdown cannot keep them so |
| quotes, code blocks, rules | `> `, ```` ``` ```` fences with the language, `---` |
| line breaks | a backslash at the end of the line |
| text | Markdown's characters escaped (`\*`, `\_`, `` \` ``, `\[`, `\#` at a line's start, …), `snake_case` left alone |

Other spellings (`*italic*`, `__bold__`, setext headings, `~~~` or indented
code, `***` rules, two-space line breaks) settle on these after one save. A
soft line break inside a paragraph becomes a space; leading spaces on a line,
empty paragraphs, link titles and character references (written as the
characters) are not kept. What the editor does not offer comes in as text:
a table row by row, a task list's box as `[ ]`, strikethrough as plain text,
an image as a link to it, raw HTML as the text it is. Emphasis that Markdown
cannot spell (opening on punctuation straight after a letter, `x*"y"*`) may
shift after a save; the text itself never changes, and a code span inside a
link that holds a bracket, backslash or backtick goes as plain text. The node
tests check the round trip for every feature and on 3,000 random documents.

**Hostile text.** The rich-text editor loads Markdown under the
[renderer's limits](#rendering-markdown-rendermarkdowntext-options): text past
`MAX_MARKDOWN_LENGTH`, `MAX_MARKDOWN_DEPTH` or the delimiter limits (or that
marked fails on) is not parsed. It loads as its source, a plain paragraph per
line, with the formatting off (the toolbar buttons are `aria-disabled`, the
shortcuts and Markdown rules do nothing, pastes go in as text, and the
element has `data-source`), and `value` gives it back exactly as it came,
edits included. Setting a value within the limits turns formatting back on.
No edit or paste nests quotes and lists deeper than the depth limit allows,
so what the editor writes always loads again as Markdown, in it and in the
Markdown editor (pasted content nested deeper goes in as its text).

### HTML in and out

With `format: 'html'`, `value` is HTML built from the document as a string,
holding only the Markdown preview's elements and attributes: `p`, `h1`,
`h2`, `strong`, `em`, `code`, `a` (`href`: the normalised `http`, `https` or
`mailto` URL; `rel="noopener noreferrer"`), `ul`, `ol` (`start`), `li`,
`blockquote`, `pre` (with `code data-language`), `hr` and `br`; text and
attributes escaped. HTML set as `value` is parsed into an inert document
(`DOMParser`: no script runs, nothing loads) and read through the editor's
schema, like a paste.

### Paste

Pasted (and dropped) HTML is read through the schema, so only the features
offered get in: scripts, styles, event handlers, images, frames, forms and
tables are dropped (their text kept), and a link stays only with an `http`,
`https` or `mailto` address. Bold and italic are kept from `<b>`, `<i>` and
`font-weight` or `font-style` spans (Word, Google Docs). Before the browser
parses pasted HTML, its `style` attributes are renamed and its `<style>`
elements made inert, so the strict CSP logs no violation. Plain text pastes
as text: Markdown in it is not read.

### Why ProseMirror

| | ProseMirror | Lexical |
|---|---|---|
| Size for this feature set (the engine's packages bundled alone with esbuild, minified / gzipped) | 212 KiB / 66 KiB | 435 KiB / 143 KiB |
| Strict CSP | no `eval`, no `<style>` element (it needs its structural CSS, which the editor adopts); inline styles only through the CSSOM | the same, with more CSSOM styling |
| Accessibility | a native `contenteditable` with the platform's caret, selection and IME; the editor adds the textbox role, label and toolbar | the same model |
| Markdown | the editor's own reader and writer on marked, shared with the Markdown editor | its own Markdown transformers (a second parser to keep in step) |

ProseMirror is the smaller, older and more widely used base, takes the
schema the editor needs (so pasted content is cut down by construction), and
leaves Markdown to marked, which the Markdown editor already ships. Of
CodeMirror's packages it shares only `w3c-keyname`.

### Classes

Proposals for the design system (it has no rich-text card yet):

| Class | |
|---|---|
| `wk-richtext-editor` | the editor; `data-disabled` while disabled, `data-source` while it holds text past the limits |
| `wk-richtext-editor-toolbar`, `-separator` | the toolbar and the hairline between its groups |
| `wk-richtext-editor-content` | the text (also `wk-markdown`) |
| `wk-richtext-editor-placeholder` | the empty first paragraph showing the placeholder |
| `wk-richtext-editor-link`, `-link-error`, `-link-actions` | the link popover (also `wk-popover`) |
| `wk-richtext-editor-foot`, `-error`, `-actions` | the footer |

It also uses the kit's `wk-button` (`--quiet`, `--icon`, `--small`,
`--primary`, `--destructive`), `wk-popover`, `wk-label`, `wk-field`,
`wk-error`, `wk-code` and `wk-link`. The styles come from the module as an
adopted stylesheet, every colour a `--wk-*` token. **Forced colours:** the
text box keeps a `CanvasText` edge and a `Highlight` focus ring, a pressed
toolbar button is drawn in `Highlight` and `HighlightText`, separators in
`CanvasText`, and the placeholder in `GrayText`.

## Layout

| Path | |
|---|---|
| `src/code/` | the code editor entry: `index.js`, `index.d.ts` |
| `src/markdown/` | the Markdown editor entry: `index.js` (the editor), `render.js` (the renderer), `style-spec.js` (its styles, plain data checked by the tests), `index.d.ts` |
| `src/richtext/` | the rich-text editor entry: `index.js` (the editor), `schema.js` (the document model per feature set), `markdown.js` and `html.js` (in and out), `limits.js` (the nesting limit), `style-spec.js`, `index.d.ts` |
| `src/shared/` | shared by the editors: the CodeMirror core (`code-editor.js`), the languages, the highlighter, the theme (`theme-spec.js`, plain data checked by the tests; `theme.js`) and the search panel; and, for the Markdown and rich-text editors, the link rules (`url.js`), the Markdown limits scan (`markdown-limits.js`), rendered Markdown's styles (`markdown-style-spec.js`, `markdown-styles.js`) and the adopted-stylesheet warning |
| `dist/` | the prebuilt entries, their shared chunks, their types and `THIRD-PARTY-LICENSES.txt`, committed and checked against a fresh build |
| `demo/` | review pages: `index.html` (no theme), `themed.html` (ui's example theme), `markdown.html` (the Markdown editor), `richtext.html` (the rich-text editor: every feature, empty, read-only, disabled, HTML, fewer features) |
| `scripts/build.mjs` | builds `dist/` with esbuild |
| `test/` | the checks |

A new editor is a folder `src/<name>/` with `index.js` and `index.d.ts`, a
name in `ENTRIES` in `scripts/build.mjs`, and an export `./<name>` in
`package.json`. An entry may import `src/shared/` but not another entry;
esbuild puts the code entries share into `dist/chunk-*.js`.

## Develop

```sh
npm ci
npm run build          # after any change in src/ or a dependency update (removes the files an earlier build made that this one did not)
npm test               # static checks and the build check: node --test
npm run test:browser   # the editor in Chrome
```

Dependencies are build-time only (`devDependencies`), pinned exactly and
locked. Dependabot (`.github/dependabot.yml`) proposes updates weekly, with
CodeMirror and Lezer grouped into one pull request, and ProseMirror's packages
into another. marked is loaded by the Markdown and rich-text entries,
ProseMirror by the rich-text entry only. After an update, rebuild and commit
`dist/`, including the regenerated `THIRD-PARTY-LICENSES.txt`.

`npm test` checks that `dist/` matches a fresh build, that each module imports
only `./chunk-*.js` files the build made and uses no network or worker API,
that the entries share one copy of CodeMirror and the patched style-mod, that
no entry loads another entry's source, that the licence file covers every
bundled package with the entries that load it, the Markdown renderer's
sanitising and rendering (in node, on a small fake DOM), the rich-text
editor's Markdown round trips (each feature, and seeded random documents),
its HTML output's allow-list, links and limits, that the theme
takes every colour from a `--wk-*` token `@wails-kit/ui` defines, that
selectors are scoped, and that nothing names an app.

`npm run test:browser` serves the `frontend/` folder (the demo pages need
`@wails-kit/ui`'s stylesheet beside them) and drives the demo pages with
`playwright-core` and Chrome (set `CHROME_PATH`, or have it at
`/usr/bin/google-chrome` or the macOS default; without Chrome it skips, unless
`CI` is set). It covers typing, undo and redo, read-only, the change events,
the theme in light, dark and the example theme, syntax contrast, Tab and
focus, find, forced colours, the highlighter, no network and no CSP
violations; and for the Markdown editor, Write and Preview by keyboard, the
formatting keys and toolbar, submitting (errors, busy, cancel), the count,
links going to `onlink`, no image requests, disabled, the API and forced
colours; and for the rich-text editor, marks and blocks by toolbar and
shortcut, the Markdown input rules, undo and redo, `aria-pressed` following
the selection, the link popover, links going to `onlink`, paste sanitising,
the HTML format, hostile text, the nesting limit, keyboard-only use,
submitting (errors, busy, cancel), disabled and read-only, the API, and light,
dark and forced colours. `CHECK=richtext npm run test:browser` runs only the
checks whose name holds the text. These run in Chrome; check WebKit by hand on a Mac for a change
in behaviour.
