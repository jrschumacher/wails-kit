// Writes the Markdown editor's review page: a comment box (toolbar, count,
// Cancel and Comment), an empty and a disabled editor, one opened on its
// preview, and a posted comment rendered read-only. It loads the prebuilt
// dist/markdown.js, as an app vendors it; markdown.html loads
// @wails-kit/ui's kit.js first, which defines <wk-tabs>.
import { wkMarkdownEditor, renderMarkdown } from '../dist/markdown.js';

const sample = `## Review notes

This **mostly** works, but _one_ thing: \`Start\` returns before the ~~server~~ listener is ready.

- [x] Read the diff
- [ ] Run the tests

| Check | Result |
|:--|--:|
| lint | 0 |
| tests | 42 |

\`\`\`go
func (s *Service) Start(ctx context.Context) error {
	return s.run(ctx, 3) // retries
}
\`\`\`

> Quoted from the description.

See [the guide](https://example.com/guide) or <https://example.com/faq>. ![A remote chart](https://example.com/chart.png)

<img src=x onerror=alert(1)> stays text, and so does [this](javascript:alert(1)).`;

const nav = [['index.html', 'Neutral'], ['themed.html', 'Example theme']].map(([href, text]) => `<a class="wk-link" href="${href}">${text}</a>`).join('');
const section = (id, title, note = '') => `<section class="demo-section" id="${id}" aria-labelledby="${id}-h">
  <h2 class="wk-heading" id="${id}-h">${title}</h2>${note ? `<p class="wk-caption demo-note">${note}</p>` : ''}<div class="demo-slot" id="${id}-slot"></div></section>`;

document.getElementById('demo').innerHTML = `
<header class="demo-head">
  <div><h1 class="wk-title">Markdown editor</h1>
  <p class="wk-caption">Neutral: the kit's stylesheet only, no theme. Follows the system appearance unless an appearance is picked here (it sets &lt;html data-theme&gt;).</p></div>
  <nav class="demo-nav demo-row">${nav}
    <div class="wk-segmented" role="radiogroup" aria-label="Appearance" id="demo-theme">
      <label><input type="radio" name="demo-theme" value="" checked><span>System</span></label>
      <label><input type="radio" name="demo-theme" value="light"><span>Light</span></label>
      <label><input type="radio" name="demo-theme" value="dark"><span>Dark</span></label>
    </div>
  </nav>
</header>
${section('comment', 'Comment box', 'Write and Preview tabs (arrow keys switch them); Mod-B, Mod-I, Mod-E and Mod-K format the selection; Mod-Enter comments. Up to 1000 characters.')}
${section('states', 'States', 'Empty with a placeholder; disabled; opened on its preview.')}
${section('rendered', 'Rendered comment', 'renderMarkdown() on its own: a posted comment, read-only, under the same rules (no raw HTML, safe links, no remote images).')}
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

// The comment box. A test sets window.pendingSubmit to a promise to see the busy state.
const comment = wkMarkdownEditor({
  id: 'demo-comment', label: 'Comment', value: sample, maxLength: 1000, placeholder: 'Leave a comment', onlink,
  actions: [
    { label: 'Cancel', onclick: editor => { editor.value = ''; note('cancel'); } },
    { label: 'Comment', primary: true, submit: true },
  ],
  onsubmit: value => { note(`submitted ${value.length} characters`); return window.pendingSubmit; },
});
slot('comment').append(comment);

const grid = document.createElement('div');
grid.className = 'demo-grid';
grid.append(
  wkMarkdownEditor({ id: 'demo-md-empty', label: 'Reply', placeholder: 'Write a reply…', actions: [{ label: 'Reply', primary: true, submit: true }], onlink }),
  wkMarkdownEditor({ id: 'demo-md-disabled', label: 'Locked comment', value: 'Comments are **locked** on this thread.', disabled: true, actions: [{ label: 'Comment', primary: true, submit: true }], onlink }),
  wkMarkdownEditor({ id: 'demo-md-preview', label: 'Description', value: sample, mode: 'preview', toolbar: false, onlink }),
);
slot('states').append(grid);

const posted = document.createElement('div');
posted.id = 'demo-rendered';
posted.className = 'wk-markdown';
posted.append(renderMarkdown(sample, { onlink }));
slot('rendered').append(posted);

slot('events').append(log);
for (const type of ['wk-change', 'wk-mode', 'wk-submit']) {
  document.addEventListener(type, e => {
    if (!e.target.classList?.contains('wk-markdown-editor')) return;
    window.kitEvents.push({ type, target: e.target.id, detail: e.detail });
    note(`${type} on #${e.target.id}: ${JSON.stringify(e.detail).slice(0, 60)}`);
  });
}
document.getElementById('demo-theme').addEventListener('change', e => {
  if (e.target.value) document.documentElement.dataset.theme = e.target.value;
  else delete document.documentElement.dataset.theme;
});
window.demoReady = true;
