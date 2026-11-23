// The Markdown editor's and preview's look, as plain data for style-mod
// (mounted as a stylesheet adopted by the document, so a strict CSP allows
// it). Every colour is a var(--wk-…) token from @wails-kit/ui; every selector
// is scoped under .wk-markdown or .wk-markdown-editor. It imports nothing, so
// the node tests read it directly.

const forced = '@media (forced-colors: active)';
const flow = 'var(--wk-space-3)';
const blocks = 'p, h1, h2, h3, h4, h5, h6, ul, ol, blockquote, hr, pre, div, table';

// Rendered Markdown: put renderMarkdown's fragment in an element with the
// class wk-markdown.
export const previewStyles = {
  '.wk-markdown': {
    color: 'var(--wk-color-text)',
    fontSize: 'var(--wk-font-size-base)',
    fontWeight: 'var(--wk-font-weight-regular)',
    lineHeight: 'var(--wk-line-height)',
    overflowWrap: 'anywhere',
  },
  // :is() gives these an element's weight on top of the class, so a page's
  // own `p { margin: 0 }` style resets do not flatten the text.
  [`.wk-markdown :is(${blocks})`]: { margin: `0 0 ${flow}` },
  [`.wk-markdown :is(${blocks}):last-child`]: { marginBottom: '0' },
  '.wk-markdown :is(h1, h2, h3, h4, h5, h6)': {
    marginTop: 'var(--wk-space-4)',
    lineHeight: 'var(--wk-line-height-tight)',
    fontWeight: 'var(--wk-font-weight-semibold)',
  },
  '.wk-markdown > :first-child, .wk-markdown blockquote > :first-child, .wk-markdown li > :first-child': { marginTop: '0' },
  '.wk-markdown h1': { fontSize: 'var(--wk-font-size-xl)' },
  '.wk-markdown h2': { fontSize: 'var(--wk-font-size-lg)' },
  '.wk-markdown h3': { fontSize: 'var(--wk-font-size-md)' },
  '.wk-markdown h4, .wk-markdown h5, .wk-markdown h6': { fontSize: 'var(--wk-font-size-base)' },
  '.wk-markdown h6': { color: 'var(--wk-color-text-secondary)' },
  '.wk-markdown ul, .wk-markdown ol': { paddingLeft: 'var(--wk-space-5)' },
  '.wk-markdown li + li': { marginTop: '2px' },
  '.wk-markdown li > :is(ul, ol)': { margin: '2px 0 0' },
  '.wk-markdown li.wk-markdown-task': { listStyle: 'none' },
  '.wk-markdown li.wk-markdown-task > .wk-checkbox': { margin: '0 var(--wk-space-1) 0 calc(-1 * var(--wk-space-4))', verticalAlign: 'middle' },
  '.wk-markdown blockquote': {
    marginLeft: '0',
    marginRight: '0',
    padding: '0 0 0 var(--wk-space-3)',
    borderLeft: '3px solid var(--wk-color-hairline-strong)',
    color: 'var(--wk-color-text-secondary)',
  },
  '.wk-markdown hr': { height: '0', border: 'none', borderTop: 'var(--wk-hairline-width) solid var(--wk-color-hairline-strong)' },
  // Inline code; code blocks are the kit's wk-code.
  '.wk-markdown :not(pre) > code': {
    padding: '1px 4px',
    borderRadius: 'var(--wk-radius-small)',
    backgroundColor: 'var(--wk-color-fill-strong)',
    fontFamily: 'var(--wk-font-family-mono)',
    fontSize: '.9em',
  },
  '.wk-markdown .wk-code > pre': { margin: '0', overflowX: 'auto', whiteSpace: 'pre', overflowWrap: 'normal', color: 'var(--wk-color-text)' },
  '.wk-markdown .wk-markdown-table': { overflowX: 'auto' },
  '.wk-markdown .wk-table': { width: 'auto', minWidth: '50%' },
  '.wk-markdown .wk-table th, .wk-markdown .wk-table td': { border: 'var(--wk-hairline-width) solid var(--wk-color-hairline)' },
  '.wk-markdown [data-align="center"]': { textAlign: 'center' },
  '.wk-markdown [data-align="right"]': { textAlign: 'right' },
  '.wk-markdown img': { maxWidth: '100%' },
  // Text past renderMarkdown's limits, shown as written.
  '.wk-markdown .wk-markdown-source': { whiteSpace: 'pre-wrap' },
  '.wk-markdown a.wk-link': { textDecoration: 'underline' },
  // Forced colours drop backgrounds: inline code keeps an edge instead.
  // Borders, text and links take system colours by themselves.
  [forced]: {
    '.wk-markdown :not(pre) > code': { border: '1px solid CanvasText' },
  },
};

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
