// Writes the rich-text editor's review page: an editor with every feature
// (and its Markdown beside it), an empty one, a read-only and a disabled one,
// one in HTML, and one with a few features. It loads the prebuilt
// dist/richtext.js, as an app vendors it.
import { wkRichTextEditor } from '../dist/richtext.js';

const sample = `## Release notes

This **mostly** works, but _one_ thing: \`Start\` returns before the listener is ready. See [the guide](https://example.com/guide) or https://example.com/faq.

- Read the diff
- Run the tests
  - unit
  - browser

1. Build
2. Ship

> Quoted from the description.

\`\`\`go
func (s *Service) Start(ctx context.Context) error {
	return s.run(ctx, 3)
}
\`\`\`

---

<img src=x onerror=alert(1)> stays text, and so does [this](javascript:alert(1)).`;

const nav = [['index.html', 'Code editor'], ['markdown.html', 'Markdown editor']].map(([href, text]) => `<a class="wk-link" href="${href}">${text}</a>`).join('');
const section = (id, title, note = '') => `<section class="demo-section" id="${id}" aria-labelledby="${id}-h">
  <h2 class="wk-heading" id="${id}-h">${title}</h2>${note ? `<p class="wk-caption demo-note">${note}</p>` : ''}<div class="demo-slot" id="${id}-slot"></div></section>`;

document.getElementById('demo').innerHTML = `
<header class="demo-head">
  <div><h1 class="wk-title">Rich-text editor</h1>
  <p class="wk-caption">Neutral: the kit's stylesheet only, no theme. Follows the system appearance unless an appearance is picked here (it sets &lt;html data-theme&gt;).</p></div>
  <nav class="demo-nav demo-row">${nav}
    <div class="wk-segmented" role="radiogroup" aria-label="Appearance" id="demo-theme">
      <label><input type="radio" name="demo-theme" value="" checked><span>System</span></label>
      <label><input type="radio" name="demo-theme" value="light"><span>Light</span></label>
      <label><input type="radio" name="demo-theme" value="dark"><span>Dark</span></label>
    </div>
  </nav>
</header>
${section('every', 'Every feature', 'Mod-B, Mod-I, Mod-E and Mod-K format the selection; Mod-Shift-8 and Mod-Shift-7 make lists; # , - , 1. , &gt; and backticks as you type. Mod-Enter submits; Escape cancels. Its Markdown is below.')}
${section('states', 'States', 'Empty with a placeholder; read-only; disabled.')}
${section('formats', 'HTML and fewer features', 'One that reads and writes HTML; one with bold, italic, links and bulleted lists only.')}
${section('events', 'Events', 'Events and link clicks from the editors above. Links go to onlink; the page never navigates.')}
`;
const slot = id => document.getElementById(`${id}-slot`);

const log = document.createElement('pre');
log.id = 'demo-log';
log.className = 'demo-log';
log.setAttribute('aria-live', 'polite');
const note = line => { log.textContent = `${line}\n${log.textContent}`.split('\n').slice(0, 6).join('\n'); };
window.kitEvents = [];
window.links = [];
const onlink = url => { window.links.push(url); note(`link: ${url}`); };

const mirror = document.createElement('pre');
mirror.id = 'demo-mirror';
mirror.className = 'demo-highlight demo-mirror';
mirror.tabIndex = 0;
mirror.setAttribute('aria-label', 'Markdown');

// A test sets window.pendingSubmit to a promise to see the busy state.
const every = wkRichTextEditor({
  id: 'demo-rich', label: 'Release notes', value: sample, placeholder: 'Write the notes', onlink,
  actions: [
    { label: 'Cancel', cancel: true },
    { label: 'Save', primary: true, submit: true },
  ],
  onchange: value => { mirror.textContent = value; },
  oncancel: () => note('cancel'),
  onsubmit: value => { note(`submitted ${value.length} characters`); return window.pendingSubmit; },
});
mirror.textContent = every.value;
slot('every').append(every, mirror);

const grid = document.createElement('div');
grid.className = 'demo-grid';
grid.append(
  wkRichTextEditor({ id: 'demo-rich-empty', label: 'Reply', placeholder: 'Write a reply…', onlink }),
  wkRichTextEditor({ id: 'demo-rich-readonly', label: 'Description', value: 'A **read-only** text with [a link](https://example.com/read) and `code`.', readOnly: true, onlink }),
  wkRichTextEditor({ id: 'demo-rich-disabled', label: 'Locked notes', value: 'Notes are **locked** for this release.', disabled: true, actions: [{ label: 'Save', primary: true, submit: true }], onlink }),
);
slot('states').append(grid);

const formats = document.createElement('div');
formats.className = 'demo-grid';
formats.append(
  wkRichTextEditor({ id: 'demo-rich-html', label: 'HTML note', format: 'html', value: '<p>Some <strong>HTML</strong> with <a href="https://example.com/h">a link</a>.</p><ul><li>one</li><li>two</li></ul>', onlink }),
  wkRichTextEditor({ id: 'demo-rich-few', label: 'Short note', features: ['bold', 'italic', 'link', 'bulletList'], value: '# A heading here is plain text\n\n**Bold** and a list:\n\n- one', onlink }),
);
slot('formats').append(formats);

slot('events').append(log);
for (const type of ['wk-change', 'wk-submit', 'wk-cancel']) {
  document.addEventListener(type, e => {
    if (!e.target.classList?.contains('wk-richtext-editor')) return;
    window.kitEvents.push({ type, target: e.target.id, detail: e.detail });
    note(`${type} on #${e.target.id}: ${JSON.stringify(e.detail).slice(0, 60)}`);
  });
}
document.getElementById('demo-theme').addEventListener('change', e => {
  if (e.target.value) document.documentElement.dataset.theme = e.target.value;
  else delete document.documentElement.dataset.theme;
});
window.demoReady = true;
