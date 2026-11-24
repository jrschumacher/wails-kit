// The rich-text editor's look, as plain data for style-mod (mounted as a
// stylesheet adopted by the document, so a strict CSP allows it). The text
// itself is rendered Markdown's look (the shared previewStyles on
// .wk-markdown), so the two editors read alike; this adds the box, the
// toolbar, the link popover, the footer and ProseMirror's own structural
// rules. Every colour is a var(--wk-…) token from @wails-kit/ui; every
// selector is scoped under .wk-richtext-editor. It imports nothing, so the
// node tests read it directly.

const forced = '@media (forced-colors: active)';
const content = '.wk-richtext-editor .wk-richtext-editor-content';

export const richtextStyles = {
  '.wk-richtext-editor': { position: 'relative', display: 'flex', flexDirection: 'column', gap: 'var(--wk-space-2)', minWidth: '0' },

  // The toolbar: quiet icon buttons in groups, a hairline between groups.
  '.wk-richtext-editor .wk-richtext-editor-toolbar': { display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: '2px' },
  '.wk-richtext-editor .wk-richtext-editor-separator': {
    alignSelf: 'stretch',
    width: '0',
    margin: '4px var(--wk-space-1)',
    borderLeft: 'var(--wk-hairline-width) solid var(--wk-color-hairline-strong)',
  },
  '.wk-richtext-editor .wk-richtext-editor-toolbar .wk-button[aria-pressed="true"]': {
    backgroundColor: 'var(--wk-color-selection)',
    color: 'var(--wk-color-text)',
  },
  '.wk-richtext-editor .wk-richtext-editor-toolbar .wk-button > svg': { width: '16px', height: '16px' },

  // The text: a field's box, growing between the min and max height (the
  // options set them on the element), then scrolling.
  [content]: {
    boxSizing: 'border-box',
    minHeight: '100px',
    maxHeight: '360px',
    overflow: 'auto',
    padding: 'var(--wk-space-2) var(--wk-space-3)',
    border: 'var(--wk-hairline-width) solid var(--wk-color-control-border)',
    borderRadius: 'var(--wk-radius-control)',
    backgroundColor: 'var(--wk-color-control)',
    caretColor: 'var(--wk-color-accent)',
    outline: 'none',
    // ProseMirror's own rules (prosemirror-view/style/prosemirror.css).
    position: 'relative',
    wordWrap: 'break-word',
    whiteSpace: 'break-spaces',
    fontVariantLigatures: 'none',
    fontFeatureSettings: '"liga" 0',
  },
  [`${content}:focus`]: { outline: 'var(--wk-focus-width) solid var(--wk-color-accent)', outlineOffset: '0' },
  [`${content} ::selection`]: { backgroundColor: 'var(--wk-color-selection)' },
  [`${content} pre`]: { whiteSpace: 'pre-wrap' },
  [`${content} li`]: { position: 'relative' },
  // A tight list's items hold paragraphs here: no space between them, as the preview shows it.
  [`${content} [data-tight] > li > p`]: { marginBottom: '0' },
  [`${content}.ProseMirror-hideselection *::selection`]: { background: 'transparent' },
  [`${content}.ProseMirror-hideselection`]: { caretColor: 'transparent' },
  [`${content} .ProseMirror-selectednode`]: { outline: 'var(--wk-focus-width) solid var(--wk-color-accent)' },
  [`${content} img.ProseMirror-separator`]: { display: 'inline', border: 'none', margin: '0' },
  [`${content} .wk-code > pre`]: { whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' },
  // The placeholder, on the empty first paragraph.
  [`${content} .wk-richtext-editor-placeholder::before`]: {
    content: 'attr(data-placeholder)',
    float: 'left',
    height: '0',
    color: 'var(--wk-color-text-secondary)',
    pointerEvents: 'none',
  },
  [`${content}[aria-readonly="true"]`]: { backgroundColor: 'var(--wk-color-bg)' },
  '.wk-richtext-editor[data-disabled] .wk-richtext-editor-content': { color: 'var(--wk-color-text-secondary)', backgroundColor: 'var(--wk-color-bg)' },
  [`${content}[aria-invalid="true"]`]: { borderColor: 'var(--wk-color-attention)' },

  // The link popover, under the text where the link is.
  '.wk-richtext-editor .wk-richtext-editor-link': {
    position: 'absolute',
    zIndex: '10',
    width: 'min(320px, 100%)',
    boxSizing: 'border-box',
  },
  '.wk-richtext-editor .wk-richtext-editor-link[hidden]': { display: 'none' },
  '.wk-richtext-editor .wk-richtext-editor-link .wk-field': { width: '100%', boxSizing: 'border-box' },
  '.wk-richtext-editor .wk-richtext-editor-link-error:empty': { display: 'none' },
  '.wk-richtext-editor .wk-richtext-editor-link-actions': { display: 'flex', flexWrap: 'wrap', gap: 'var(--wk-space-2)', justifyContent: 'flex-end' },

  // The footer: the error and the actions.
  '.wk-richtext-editor .wk-richtext-editor-foot': { display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 'var(--wk-space-2) var(--wk-space-3)' },
  '.wk-richtext-editor .wk-richtext-editor-error': { flex: '1 1 auto', minWidth: '0', margin: '0', fontSize: 'var(--wk-font-size-sm)', color: 'var(--wk-color-attention-text)' },
  '.wk-richtext-editor .wk-richtext-editor-actions': { display: 'flex', flexWrap: 'wrap', gap: 'var(--wk-space-2)', marginLeft: 'auto' },

  // Forced colours: the box keeps its edge and the focus ring, a pressed
  // button is drawn in the selection's system colours, the placeholder is
  // greyed.
  [forced]: {
    [content]: { borderColor: 'CanvasText' },
    [`${content}:focus`]: { outlineColor: 'Highlight' },
    [`${content}[aria-invalid="true"]`]: { borderStyle: 'dashed' },
    [`${content} .wk-richtext-editor-placeholder::before`]: { color: 'GrayText' },
    [`${content} .ProseMirror-selectednode`]: { outlineColor: 'Highlight' },
    '.wk-richtext-editor .wk-richtext-editor-toolbar .wk-button[aria-pressed="true"]': {
      forcedColorAdjust: 'none',
      backgroundColor: 'Highlight',
      color: 'HighlightText',
      borderColor: 'Highlight',
    },
    '.wk-richtext-editor .wk-richtext-editor-separator': { borderLeftColor: 'CanvasText' },
  },
};
