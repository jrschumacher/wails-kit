// Editors: @wails-kit/editor's code, Markdown and rich-text editors, from
// its prebuilt dist/ modules, as an app vendors them.
const code = () => import('../../editor/dist/code.js');
const markdown = () => import('../../editor/dist/markdown.js');
const richtext = () => import('../../editor/dist/richtext.js');

const notes = `## Release notes

This **mostly** works, but _one_ thing: \`Start\` returns before the listener is ready.

- [x] Read the diff
- [ ] Run the tests

\`\`\`go
func (s *Service) Start(ctx context.Context) error {
	return s.run(ctx, 3)
}
\`\`\`

> Quoted from the description.`;

export default [
  {
    id: 'code-editor', title: 'Code editor', group: 'editors',
    intro: '<code>wkCodeEditor(spec)</code> on CodeMirror 6. Tab moves focus on (unless <code>tabIndents</code>); Mod-F finds; Mod-Z and Mod-Shift-Z undo and redo. It fires <code>wk-change</code>.',
    examples: [{
      title: 'Editable',
      async build(host) {
        const { wkCodeEditor } = await code();
        host.append(wkCodeEditor({ id: 'g-code', label: 'TypeScript source', language: 'typescript', value: "export function onChange(el: HTMLElement, fn: (v: string) => void): () => void {\n  const handler = (e: Event) => fn((e as CustomEvent).detail.value);\n  el.addEventListener('wk-change', handler);\n  return () => el.removeEventListener('wk-change', handler);\n}" }));
      },
    }, {
      title: 'Read-only, empty, wrapped and without line numbers',
      async build(host) {
        const { wkCodeEditor } = await code();
        const grid = document.createElement('div');
        grid.className = 'g-grid-wide';
        grid.append(
          wkCodeEditor({ id: 'g-code-readonly', label: 'Go source (read-only)', language: 'go', readOnly: true, value: 'package main\n\nimport "fmt"\n\nfunc main() {\n\tfmt.Println("hello")\n}' }),
          wkCodeEditor({ id: 'g-code-empty', label: 'Notes', placeholder: 'Write a note…' }),
          wkCodeEditor({ id: 'g-code-wrap', label: 'Markdown, wrapped', language: 'markdown', wrap: true, tabIndents: true, value: 'A long line that wraps at the edge of the editor rather than scrolling sideways, as prose should.' }),
          wkCodeEditor({ id: 'g-code-plain', label: 'Shell, no line numbers', language: 'shell', lineNumbers: false, value: '#!/usr/bin/env bash\nset -euo pipefail\necho "done: $?"' }),
        );
        host.append(grid);
      },
    }, {
      title: 'The highlighter on its own',
      note: 'highlighter() colours single lines; the diff view takes it.',
      async build(host) {
        const { highlighter } = await code();
        const highlight = highlighter();
        const box = document.createElement('div');
        box.className = 'wk-code wk-code--scroll';
        const pre = document.createElement('pre');
        pre.tabIndex = 0;
        pre.setAttribute('role', 'region');
        pre.setAttribute('aria-label', 'Highlighted lines');
        for (const [path, text] of [['main.go', 'func (s *Service) Start(ctx context.Context) error {'], ['app.ts', "const label: string = 'Settings';"], ['config.yaml', 'retries: 3 # per request']]) {
          pre.append(highlight({ path }, { text }), '\n');
        }
        box.append(pre);
        host.append(box);
      },
    }],
  },
  {
    id: 'markdown-editor', title: 'Markdown editor', group: 'editors',
    intro: '<code>wkMarkdownEditor(spec)</code>: a comment box with Write and Preview tabs, a toolbar, a count and its actions. Mod-B, Mod-I, Mod-E and Mod-K format; Mod-Enter submits; Escape cancels. Its busy submit is a <code>wkTask</code> (through <code>document.wkTask</code>): "Sending…" from 400 ms, then "Sent"; this one\'s stand-in server takes 1.2 s. <code>renderMarkdown()</code> draws a posted comment with no raw HTML, safe links and no remote images.',
    examples: [{
      title: 'A comment box',
      async build(host) {
        const { wkMarkdownEditor } = await markdown();
        host.append(wkMarkdownEditor({
          id: 'g-md', label: 'Comment', value: notes, maxLength: 1000, placeholder: 'Leave a comment', onlink: () => {},
          actions: [{ label: 'Cancel', cancel: true }, { label: 'Comment', primary: true, submit: true }],
          oncancel: editor => { editor.value = ''; },
          onsubmit: () => new Promise(resolve => setTimeout(resolve, 1200)),
        }));
      },
    }, {
      title: 'Empty, disabled, and opened on its preview',
      async build(host) {
        const { wkMarkdownEditor } = await markdown();
        const grid = document.createElement('div');
        grid.className = 'g-grid-wide';
        grid.append(
          wkMarkdownEditor({ id: 'g-md-empty', label: 'Reply', placeholder: 'Write a reply…', actions: [{ label: 'Reply', primary: true, submit: true }], onlink: () => {} }),
          wkMarkdownEditor({ id: 'g-md-disabled', label: 'Locked comment', value: 'Comments are **locked** on this thread.', disabled: true, actions: [{ label: 'Comment', primary: true, submit: true }], onlink: () => {} }),
          wkMarkdownEditor({ id: 'g-md-opened', label: 'Description', value: notes, mode: 'preview', toolbar: false, onlink: () => {} }),
        );
        host.append(grid);
      },
    }, {
      title: 'A rendered comment',
      async build(host) {
        const { renderMarkdown } = await markdown();
        const posted = document.createElement('div');
        posted.className = 'wk-markdown';
        posted.append(renderMarkdown(notes, { onlink: () => {} }));
        host.append(posted);
      },
    }],
  },
  {
    id: 'richtext-editor', title: 'Rich-text editor', group: 'editors',
    intro: '<code>wkRichTextEditor(spec)</code> on ProseMirror, reading and writing Markdown (or HTML). Mod-B, Mod-I, Mod-E and Mod-K format; Markdown shortcuts work as you type; Mod-Enter submits; Escape cancels. Its busy submit is a <code>wkTask</code> too: "Sending…" from 400 ms.',
    examples: [{
      title: 'Every feature',
      async build(host) {
        const { wkRichTextEditor } = await richtext();
        host.append(wkRichTextEditor({
          id: 'g-rich', label: 'Release notes', value: notes.replace(/- \[[ x]\] /g, '- '), placeholder: 'Write the notes', onlink: () => {},
          actions: [{ label: 'Cancel', cancel: true }, { label: 'Save', primary: true, submit: true }],
          onsubmit: () => new Promise(resolve => setTimeout(resolve, 1200)),
        }));
      },
    }, {
      title: 'Empty, read-only, disabled and with fewer features',
      async build(host) {
        const { wkRichTextEditor } = await richtext();
        const grid = document.createElement('div');
        grid.className = 'g-grid-wide';
        grid.append(
          wkRichTextEditor({ id: 'g-rich-empty', label: 'Reply', placeholder: 'Write a reply…', onlink: () => {} }),
          wkRichTextEditor({ id: 'g-rich-readonly', label: 'Description', value: 'A **read-only** text with `code`.', readOnly: true, onlink: () => {} }),
          wkRichTextEditor({ id: 'g-rich-disabled', label: 'Locked notes', value: 'Notes are **locked** for this release.', disabled: true, actions: [{ label: 'Save', primary: true, submit: true }], onlink: () => {} }),
          wkRichTextEditor({ id: 'g-rich-few', label: 'Short note', features: ['bold', 'italic', 'link', 'bulletList'], value: '**Bold** and a list:\n\n- one', onlink: () => {} }),
        );
        host.append(grid);
      },
    }],
  },
];
