// The Markdown editor's and preview's look, as plain data for style-mod
// (mounted as a stylesheet adopted by the document, so a strict CSP allows
// it). Every colour is a var(--wk-…) token from @wails-kit/ui; every selector
// is scoped under .wk-markdown or .wk-markdown-editor. It imports only plain
// data (the shared previewStyles), so the node tests read it directly.

import { previewStyles } from '../shared/markdown-style-spec.js';

export { previewStyles };

const forced = '@media (forced-colors: active)';

// The editor: Write and Preview tabs with the toolbar beside them, the
// CodeMirror box that grows between a min and max height, the preview, and
// the footer (count, error, actions).
export const editorStyles = {
  '.wk-markdown-editor': { display: 'flex', flexDirection: 'column', gap: 'var(--wk-space-2)', minWidth: '0' },
  '.wk-markdown-editor > wk-tabs': { display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) auto', minWidth: '0' },
  '.wk-markdown-editor > wk-tabs > .wk-tablist': { gridColumn: '1' },
  '.wk-markdown-editor > wk-tabs > .wk-markdown-editor-toolbar': {
    gridColumn: '2',
    display: 'flex',
    alignItems: 'center',
    gap: '2px',
    borderBottom: 'var(--wk-hairline-width) solid var(--wk-color-hairline)',
  },
  '.wk-markdown-editor > wk-tabs > [role="tabpanel"]': { gridColumn: '1 / -1', minWidth: '0', paddingTop: 'var(--wk-space-2)' },
  // The text grows with its content between the min and max height
  // (options minHeight and maxHeight set them on the element), then scrolls.
  '.wk-markdown-editor .wk-code-editor': { display: 'flex', flexDirection: 'column', minHeight: '100px', maxHeight: '360px' },
  '.wk-markdown-editor .wk-code-editor > .cm-editor': { flex: '1 1 auto', minHeight: '0', height: 'auto' },
  '.wk-markdown-editor .wk-code-editor > .cm-editor > .cm-scroller': { overflow: 'auto', fontFamily: 'var(--wk-font-family-mono)' },
  '.wk-markdown-editor-preview': {
    boxSizing: 'border-box',
    padding: 'var(--wk-space-2) var(--wk-space-3)',
    border: 'var(--wk-hairline-width) solid var(--wk-color-hairline)',
    borderRadius: 'var(--wk-radius-control)',
    overflow: 'auto',
  },
  '.wk-markdown-editor-empty': { color: 'var(--wk-color-text-secondary)' },
  '.wk-markdown-editor-foot': { display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 'var(--wk-space-2) var(--wk-space-3)' },
  '.wk-markdown-editor-status': { display: 'flex', flexWrap: 'wrap', alignItems: 'baseline', gap: 'var(--wk-space-3)', flex: '1 1 auto', minWidth: '0', fontSize: 'var(--wk-font-size-sm)' },
  '.wk-markdown-editor-count': { color: 'var(--wk-color-text-secondary)', fontVariantNumeric: 'tabular-nums' },
  '.wk-markdown-editor-count[data-over]': { color: 'var(--wk-color-attention-text)', fontWeight: 'var(--wk-font-weight-semibold)' },
  '.wk-markdown-editor-error': { margin: '0', color: 'var(--wk-color-attention-text)' },
  '.wk-markdown-editor-actions': { display: 'flex', flexWrap: 'wrap', gap: 'var(--wk-space-2)', marginLeft: 'auto' },
  '.wk-markdown-editor[data-disabled] .wk-markdown-editor-preview': { color: 'var(--wk-color-text-secondary)' },
  // Forced colours: the over-limit count loses its colour, so it is
  // underlined as well as bold.
  [forced]: {
    '.wk-markdown-editor-count[data-over]': { textDecoration: 'underline' },
  },
};
