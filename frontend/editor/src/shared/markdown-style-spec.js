// Rendered Markdown's look (the class wk-markdown), as plain data for
// style-mod (mounted as a stylesheet adopted by the document, so a strict CSP
// allows it). Shared by the Markdown editor's preview and renderer and the
// rich-text editor's content, so both read alike. Every colour is a
// var(--wk-…) token from @wails-kit/ui; every selector is scoped under
// .wk-markdown. It imports nothing, so the node tests read it directly.

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
