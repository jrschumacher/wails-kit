// Writes the editor's review page: the code editor in each language it
// bundles and each state it can show, and the highlighter on diff lines.
// Used by index.html (no theme) and themed.html (ui/theme/example.css). It
// loads the prebuilt dist/code.js, as an app vendors it.
import { wkCodeEditor, highlighter, languages } from '../dist/code.js';

const themed = document.body.dataset.demo === 'themed';
const pages = [['index.html', 'Neutral'], ['themed.html', 'Example theme'], ['markdown.html', 'Markdown editor']];
const here = location.pathname.split('/').pop() || 'index.html';
const other = pages.filter(([href]) => href !== here).map(([href, text]) => `<a class="wk-link" href="${href}">${text}</a>`).join('');

const samples = {
  go: `package main

import "fmt"

// Greeting says hello to name.
type Greeting struct{ Name string }

func (g Greeting) String() string {
	return fmt.Sprintf("hello, %s (%d)", g.Name, 42)
}`,
  typescript: `// A typed event, as the kit emits them.
interface Change { value: string; at: number }

export function onChange(el: HTMLElement, fn: (c: Change) => void): () => void {
  const handler = (e: Event) => fn({ value: (e as CustomEvent).detail.value, at: Date.now() });
  el.addEventListener('wk-change', handler);
  return () => el.removeEventListener('wk-change', handler);
}`,
  javascript: `const total = items.reduce((sum, { price }) => sum + price, 0);
if (total > 100 && !discounted) console.log(\`total: \${total}\`, /\\d+/.test("12"));`,
  json: `{
  "name": "example",
  "port": 8080,
  "debug": false,
  "tags": ["a", "b", null]
}`,
  yaml: `# Build matrix
version: 3
tasks:
  check:
    desc: Run all checks
    cmds: [lint, test]
    parallel: true`,
  markdown: `# Release notes

Some *emphasis*, **strong** text and \`inline code\`, plus [a link](https://example.com).

\`\`\`go
func main() { println("fenced code is highlighted") }
\`\`\``,
  shell: `#!/usr/bin/env bash
set -euo pipefail
# Vendor the editor from a release tag.
for f in code.js code.d.ts THIRD-PARTY-LICENSES.txt; do
  cp "$checkout/frontend/editor/dist/$f" vendor/wails-kit-editor/
done
echo "done: $?"`,
  css: `/* A theme file */
:root { --wk-color-accent: #1f6b55; }
.app-header > h1:hover { font-weight: 600; margin: 0 auto; }`,
  html: `<!doctype html>
<html lang="en" class="wk-root">
  <link rel="stylesheet" href="vendor/wails-kit-ui/wails-kit-ui.css">
  <script type="module">import { wkCodeEditor } from './vendor/wails-kit-editor/code.js';</script>
</html>`,
};

const section = (id, title, note = '') => `<section class="demo-section" id="${id}" aria-labelledby="${id}-h">
  <h2 class="wk-heading" id="${id}-h">${title}</h2>${note ? `<p class="wk-caption demo-note">${note}</p>` : ''}<div class="demo-slot" id="${id}-slot"></div></section>`;

document.getElementById('demo').innerHTML = `
<header class="demo-head">
  <div><h1 class="wk-title">Code editor</h1>
  <p class="wk-caption">${themed ? 'With the example theme (ui/theme/example.css).' : 'Neutral: the kit\'s stylesheet only, no theme.'} Follows the system appearance unless an appearance is picked here (it sets &lt;html data-theme&gt;).</p></div>
  <nav class="demo-nav demo-row">${other}
    <div class="wk-segmented" role="radiogroup" aria-label="Appearance" id="demo-theme">
      <label><input type="radio" name="demo-theme" value="" checked><span>System</span></label>
      <label><input type="radio" name="demo-theme" value="light"><span>Light</span></label>
      <label><input type="radio" name="demo-theme" value="dark"><span>Dark</span></label>
    </div>
  </nav>
</header>
${section('editable', 'Editable', 'TypeScript. Tab moves focus on; Mod-F finds; Mod-Z and Mod-Shift-Z undo and redo. Its value is shown below it on each change.')}
${section('states', 'States', 'Read-only (Go); an empty editor with a placeholder; wrapping with Tab indenting (Escape, then Tab, moves focus on); no line numbers.')}
${section('languages', 'Languages', `Every bundled language: ${languages.join(', ')}.`)}
${section('highlighter', 'Highlighter', 'highlighter() colours single lines, as the diff view\'s highlight(file, line) hook: the language comes from each file\'s path.')}
${section('events', 'Events', 'wk-change events fired by the editors above.')}
`;

const slot = id => document.getElementById(`${id}-slot`);

const main = wkCodeEditor({ id: 'demo-editor', label: 'TypeScript source', language: 'typescript', value: samples.typescript });
const mirror = document.createElement('pre');
mirror.id = 'demo-mirror';
mirror.className = 'wk-caption demo-mirror';
mirror.textContent = `${main.value.length} characters`;
main.addEventListener('wk-change', e => { mirror.textContent = `${e.detail.value.length} characters`; });
slot('editable').append(main, mirror);

const grid = document.createElement('div');
grid.className = 'demo-grid';
grid.append(
  wkCodeEditor({ id: 'demo-readonly', label: 'Go source (read-only)', language: 'go', value: samples.go, readOnly: true }),
  wkCodeEditor({ id: 'demo-empty', label: 'Notes', placeholder: 'Write a note…' }),
  wkCodeEditor({ id: 'demo-wrap', label: 'Markdown, wrapped', language: 'markdown', wrap: true, tabIndents: true, value: samples.markdown }),
  wkCodeEditor({ id: 'demo-plain', label: 'Shell, no line numbers', language: 'shell', lineNumbers: false, value: samples.shell }),
);
slot('states').append(grid);

const langs = document.createElement('div');
langs.className = 'demo-grid';
for (const name of languages) {
  const figure = document.createElement('figure');
  figure.className = 'demo-figure';
  const caption = document.createElement('figcaption');
  caption.className = 'wk-caption';
  caption.textContent = name;
  figure.append(caption, wkCodeEditor({ id: `demo-lang-${name}`, label: `${name} sample`, language: name, value: samples[name] }));
  langs.append(figure);
}
slot('languages').append(langs);

const highlight = highlighter();
const diff = [
  [{ path: 'service/main.go' }, 'func (s *Service) Start(ctx context.Context) error {', ' '],
  [{ path: 'service/main.go' }, '	return errors.New("not started") // TODO', '-'],
  [{ path: 'service/main.go' }, '	return s.run(ctx, 3)', '+'],
  [{ path: 'web/src/app.ts' }, "const label: string = 'Settings';", '+'],
  [{ path: 'config.yaml' }, 'retries: 3 # per request', ' '],
  [{ path: 'notes.txt' }, 'plain text in no known language', ' '],
];
const pre = document.createElement('pre');
pre.id = 'demo-highlight';
pre.className = 'demo-highlight';
pre.tabIndex = 0;
pre.setAttribute('aria-label', 'Highlighted diff lines');
for (const [file, text, marker] of diff) {
  const row = document.createElement('span');
  row.className = 'demo-highlight-line';
  const mark = document.createElement('span');
  mark.className = 'demo-highlight-marker';
  mark.setAttribute('aria-hidden', 'true');
  mark.textContent = marker;
  row.append(mark, highlight(file, { kind: 'context', text }), '\n');
  pre.append(row);
}
slot('highlighter').append(pre);

// Demo-only behaviour: the appearance picker and the event log.
document.getElementById('demo-theme').addEventListener('change', e => {
  if (e.target.value) document.documentElement.dataset.theme = e.target.value;
  else delete document.documentElement.dataset.theme;
});
const log = document.createElement('pre');
log.id = 'demo-log';
log.className = 'demo-log';
log.setAttribute('aria-live', 'polite');
slot('events').append(log);
window.kitEvents = [];
document.addEventListener('wk-change', e => {
  if (!e.target.closest('.wk-code-editor')) return;
  window.kitEvents.push({ type: e.type, target: e.target.id, value: e.detail.value });
  log.textContent = `${e.type} on #${e.target.id}: ${e.detail.value.length} characters\n${log.textContent}`.split('\n').slice(0, 6).join('\n');
});
window.demoReady = true;
