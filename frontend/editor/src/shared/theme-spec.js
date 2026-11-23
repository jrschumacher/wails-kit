// The editors' look, as plain data: CodeMirror theme rules and the syntax
// colours, every colour a var(--wk-…) token from @wails-kit/ui, so light,
// dark and an app's theme follow without a rebuild. It imports nothing, so
// the node tests read it directly (test/static.test.mjs checks that no rule
// hard-codes a colour, outside the forced-colours block's system colours).
//
// Every selector starts with "&.cm-editor": "&" is the theme's own class, and
// the extra class lifts the rule over CodeMirror's "&light" base rules.

const fillOverControl = {
  backgroundColor: 'var(--wk-color-control)',
  backgroundImage: 'linear-gradient(var(--wk-color-fill), var(--wk-color-fill))',
};

export const editorTheme = {
  '&.cm-editor': {
    color: 'var(--wk-color-text)',
    backgroundColor: 'var(--wk-color-control)',
    border: 'var(--wk-hairline-width) solid var(--wk-color-control-border)',
    borderRadius: 'var(--wk-radius-control)',
    height: '100%',
    overflow: 'hidden',
  },
  // The kit's focus ring, on the whole editor (gutters and panels too), as a
  // field draws it.
  '&.cm-editor.cm-focused': {
    outline: 'var(--wk-focus-width) solid var(--wk-color-accent)',
    outlineOffset: '0',
  },
  '&.cm-editor .cm-scroller': {
    fontFamily: 'var(--wk-font-family-mono)',
    fontSize: 'var(--wk-font-size-sm)',
    lineHeight: 'var(--wk-line-height)',
  },
  '&.cm-editor .cm-content': { caretColor: 'var(--wk-color-accent)', padding: 'var(--wk-space-1) 0' },
  '&.cm-editor .cm-line': { padding: '0 var(--wk-space-2)' },
  '&.cm-editor .cm-cursor, &.cm-editor .cm-dropCursor': {
    borderLeft: '2px solid var(--wk-color-accent)',
    marginLeft: '-1px',
  },
  // CodeMirror draws the selection itself; native selection (forced colours)
  // takes the same token.
  '&.cm-editor .cm-selectionBackground, &.cm-editor.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground': {
    background: 'var(--wk-color-selection)',
  },
  '&.cm-editor .cm-content ::selection': { backgroundColor: 'var(--wk-color-selection)' },
  // Gutters: secondary text on a fill, laid over the control colour so text
  // scrolled sideways does not show through.
  '&.cm-editor .cm-gutters': {
    ...fillOverControl,
    color: 'var(--wk-color-text-secondary)',
    border: 'none',
    borderRight: 'var(--wk-hairline-width) solid var(--wk-color-hairline)',
  },
  '&.cm-editor .cm-lineNumbers .cm-gutterElement': { padding: '0 var(--wk-space-2) 0 var(--wk-space-3)', minWidth: '2ch' },
  '&.cm-editor .cm-activeLineGutter': { backgroundColor: 'transparent', color: 'var(--wk-color-text)' },
  '&.cm-editor .cm-placeholder': { color: 'var(--wk-color-text-secondary)' },
  '&.cm-editor.cm-focused .cm-matchingBracket': {
    backgroundColor: 'var(--wk-color-fill-strong)',
    outline: 'var(--wk-hairline-width) solid var(--wk-color-hairline-strong)',
  },
  '&.cm-editor.cm-focused .cm-nonmatchingBracket': {
    backgroundColor: 'var(--wk-color-attention-fill)',
    color: 'var(--wk-color-attention-text)',
  },
  '&.cm-editor .cm-searchMatch': {
    backgroundColor: 'var(--wk-color-selection)',
    outline: 'var(--wk-hairline-width) solid var(--wk-color-control-border)',
  },
  '&.cm-editor .cm-searchMatch.cm-searchMatch-selected': {
    backgroundColor: 'var(--wk-color-fill-strong)',
    outline: 'var(--wk-hairline-width) solid var(--wk-color-accent)',
  },
  '&.cm-editor .cm-selectionMatch': { backgroundColor: 'var(--wk-color-fill-strong)' },
  // Panels (the search panel): the kit's surface, with kit fields and buttons.
  '&.cm-editor .cm-panels': { backgroundColor: 'var(--wk-color-surface)', color: 'var(--wk-color-text)' },
  '&.cm-editor .cm-panels.cm-panels-top': { borderBottom: 'var(--wk-hairline-width) solid var(--wk-color-hairline)' },
  '&.cm-editor .cm-panels.cm-panels-bottom': { borderTop: 'var(--wk-hairline-width) solid var(--wk-color-hairline)' },
  '&.cm-editor .wk-code-search': {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: 'var(--wk-space-2)',
    padding: 'var(--wk-space-2)',
    fontFamily: 'var(--wk-font-family)',
    fontSize: 'var(--wk-font-size-sm)',
  },
  '&.cm-editor .wk-code-search-row': { display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 'var(--wk-space-2)', flexBasis: '100%' },
  '&.cm-editor .wk-code-search .wk-field': {
    minHeight: 'var(--wk-control-height-small)',
    padding: '1px 6px',
    fontSize: 'var(--wk-font-size-sm)',
    width: '14em',
    maxWidth: '100%',
  },
  '&.cm-editor .wk-code-search .wk-check': { fontSize: 'var(--wk-font-size-sm)' },
  '&.cm-editor .wk-code-search-close': { marginLeft: 'auto' },
  // Forced colours: the system draws the selection (the editor switches to
  // native selection) and the caret; the focused editor keeps a ring.
  '@media (forced-colors: active)': {
    '&.cm-editor .cm-content': { caretColor: 'CanvasText' },
    '&.cm-editor .cm-cursor, &.cm-editor .cm-dropCursor': { borderLeftColor: 'CanvasText' },
    '&.cm-editor.cm-focused': { outlineColor: 'Highlight' },
    '&.cm-editor .cm-gutters': { borderRightColor: 'CanvasText' },
    '&.cm-editor .cm-searchMatch, &.cm-editor.cm-focused .cm-matchingBracket': { outlineColor: 'CanvasText' },
    '&.cm-editor .cm-searchMatch.cm-searchMatch-selected': { outline: '2px solid Highlight' },
  },
};

// Syntax colours: [the Lezer tags (as names, resolved in theme.js), the style].
// A name like "function(variableName)" applies a tag modifier.
export const syntax = [
  [['keyword', 'controlKeyword', 'operatorKeyword', 'definitionKeyword', 'moduleKeyword', 'modifier', 'self'], { color: 'var(--wk-color-code-keyword)' }],
  [['string', 'special(string)', 'character', 'regexp', 'escape', 'attributeValue', 'url'], { color: 'var(--wk-color-code-string)' }],
  [['number', 'integer', 'float', 'bool', 'null', 'atom', 'unit', 'color'], { color: 'var(--wk-color-code-number)' }],
  [['comment', 'lineComment', 'blockComment', 'docComment', 'meta', 'processingInstruction'], { color: 'var(--wk-color-code-comment)', fontStyle: 'italic' }],
  [['typeName', 'className', 'namespace', 'tagName', 'standard(typeName)'], { color: 'var(--wk-color-code-type)' }],
  [['function(variableName)', 'function(propertyName)', 'function(definition(variableName))', 'macroName', 'attributeName', 'definition(propertyName)', 'labelName'], { color: 'var(--wk-color-code-function)' }],
  [['heading'], { color: 'var(--wk-color-code-keyword)', fontWeight: 'var(--wk-font-weight-semibold)' }],
  [['strong'], { fontWeight: 'var(--wk-font-weight-bold)' }],
  [['emphasis'], { fontStyle: 'italic' }],
  [['strikethrough'], { textDecoration: 'line-through' }],
  [['link'], { color: 'var(--wk-color-code-string)', textDecoration: 'underline' }],
  [['monospace'], { color: 'var(--wk-color-code-string)' }],
  [['invalid'], { color: 'var(--wk-color-attention-text)' }],
];
