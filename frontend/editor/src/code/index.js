// @wails-kit/editor/code: a code editor on CodeMirror 6, themed by the kit's
// --wk-* tokens, and the same syntax colours as a highlighter for read-only
// code (the diff view's highlight hook). dist/code.js is this entry; the
// CodeMirror it uses is in dist/chunk-*.js, shared with the other editors.
// Nothing loads at run time beyond those files.
import { createCodeEditor } from '../shared/code-editor.js';

export { highlighter } from '../shared/highlight.js';
export { languages, languageForPath, languageName } from '../shared/languages.js';

/**
 * Builds a code editor. Returns a <div class="wk-code-editor"> holding it,
 * with `value` (get and set), `focus()` and `destroy()`; it fires a bubbling
 * `wk-change` event ({ detail: { value } }) on each edit. After destroy(),
 * setting `value` and focus() do nothing.
 */
export function wkCodeEditor(spec = {}) {
  return createCodeEditor(spec).el;
}
