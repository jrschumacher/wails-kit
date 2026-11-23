// @wails-kit/editor/code: a code editor on CodeMirror 6, themed by the kit's
// --wk-* tokens, and the same syntax colours as a highlighter for read-only
// code (the diff view's highlight hook). dist/code.js is this entry with
// CodeMirror bundled; it loads nothing at run time.
import { EditorState, Compartment } from '@codemirror/state';
import {
  EditorView, keymap, lineNumbers, highlightActiveLineGutter, highlightSpecialChars,
  drawSelection, placeholder as placeholderText,
} from '@codemirror/view';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { indentOnInput, bracketMatching, syntaxHighlighting, indentUnit } from '@codemirror/language';
import { search, searchKeymap } from '@codemirror/search';
import { highlightCode } from '@lezer/highlight';
import { StyleModule } from 'style-mod';
import { theme, highlightStyle } from '../shared/theme.js';
import { createSearchPanel } from '../shared/search-panel.js';
import { languageExtension, languageForPath, languageName, resolve, languages } from './languages.js';

export { languages, languageForPath, languageName };

// CodeMirror's styles reach the page as a stylesheet adopted by the
// document (see scripts/build.mjs). Without adoptedStyleSheets (WebKit before
// Safari 16.4) they fall back to a <style> element, which a strict CSP
// blocks, and the editor renders broken: say so once.
let warned = false;
function warnWithoutAdoptedSheets() {
  if (warned || typeof document === 'undefined' || 'adoptedStyleSheets' in document) return;
  warned = true;
  console.warn('@wails-kit/editor: this browser has no document.adoptedStyleSheets, so the editor\'s styles need a CSP that allows inline styles. It needs WebView2, Safari 16.4+ (macOS 13.3) or WebKitGTK 2.40+.');
}

const forcedColors = () => typeof matchMedia === 'function' && matchMedia('(forced-colors: active)').matches;

/**
 * Builds a code editor. Returns a <div class="wk-code-editor"> holding it,
 * with `value` (get and set), `focus()` and `destroy()`; it fires a bubbling
 * `wk-change` event ({ detail: { value } }) on each edit. After destroy(),
 * setting `value` and focus() do nothing.
 */
export function wkCodeEditor(spec = {}) {
  const label = typeof spec.label === 'string' ? spec.label.trim() : '';
  if (!label) throw new TypeError('wkCodeEditor: label (the accessible name) is required');
  const tabSize = Number.isInteger(spec.tabSize) && spec.tabSize > 0 ? spec.tabSize : 4;
  const language = languageExtension(spec.language);
  const usesTabs = typeof spec.language === 'string' && languageName(spec.language) === 'go';
  const readOnly = !!spec.readOnly;
  warnWithoutAdoptedSheets();

  const el = document.createElement('div');
  el.className = 'wk-code-editor';
  if (spec.id) el.id = spec.id;

  // In forced colours the system draws a native selection and caret, which
  // stay visible in every contrast theme; elsewhere CodeMirror draws them in
  // the kit's colours.
  const selection = new Compartment();
  const selectionFor = forced => (forced ? [] : drawSelection());

  const extensions = () => [
    spec.lineNumbers === false ? [] : [lineNumbers(), highlightActiveLineGutter()],
    highlightSpecialChars(),
    history(),
    selection.of(selectionFor(forcedColors())),
    indentOnInput(),
    bracketMatching(),
    syntaxHighlighting(highlightStyle),
    search({ top: true, createPanel: createSearchPanel }),
    keymap.of([
      ...(spec.tabIndents ? [indentWithTab] : []),
      ...defaultKeymap, ...historyKeymap, ...searchKeymap,
    ]),
    EditorState.tabSize.of(tabSize),
    indentUnit.of(usesTabs ? '\t' : ' '.repeat(tabSize)),
    EditorState.readOnly.of(readOnly),
    EditorView.contentAttributes.of({ 'aria-label': label, ...(readOnly ? { 'aria-readonly': 'true' } : {}) }),
    spec.wrap ? EditorView.lineWrapping : [],
    spec.placeholder ? placeholderText(String(spec.placeholder)) : [],
    language,
    theme,
    EditorView.updateListener.of(update => {
      // Setting value replaces the state (setState), which reports no update.
      if (!update.docChanged) return;
      const value = update.state.doc.toString();
      // The event first, so a throwing callback cannot swallow it.
      el.dispatchEvent(new CustomEvent('wk-change', { bubbles: true, detail: { value } }));
      spec.onchange?.(value);
    }),
  ];

  const view = new EditorView({ parent: el, state: EditorState.create({ doc: String(spec.value ?? ''), extensions: extensions() }) });

  const media = typeof matchMedia === 'function' ? matchMedia('(forced-colors: active)') : null;
  const onForced = e => view.dispatch({ effects: selection.reconfigure(selectionFor(e.matches)) });
  media?.addEventListener('change', onForced);

  let destroyed = false;
  Object.defineProperties(el, {
    value: {
      get: () => view.state.doc.toString(),
      set: next => {
        next = String(next ?? '');
        if (destroyed || next === view.state.doc.toString()) return;
        // New content: replace the document, keep the cursor in range, and
        // start the undo history afresh (Mod-Z does not undo a load). The new
        // state closes an open find panel; focus that was in the editor (the
        // panel included) goes back to the text, not to <body>.
        const hadFocus = el.contains(document.activeElement);
        const head = Math.min(view.state.selection.main.head, next.length);
        view.setState(EditorState.create({ doc: next, selection: { anchor: head }, extensions: extensions() }));
        if (hadFocus) view.focus();
      },
      enumerable: true,
    },
    focus: { value: () => { if (!destroyed) view.focus(); } },
    destroy: {
      value: () => {
        if (destroyed) return;
        destroyed = true;
        media?.removeEventListener('change', onForced);
        view.destroy();
        el.replaceChildren();
      },
    },
  });
  return el;
}

/**
 * The diff view's highlight hook: highlighter(language) returns
 * highlight(file, line) → DocumentFragment, colouring one line's text with
 * the editor's syntax colours. With no language it picks one from the file's
 * path (file.path, else file.oldPath); a line in no known language comes back
 * as plain text. `line` is a diff line ({ text }) or a string. Each line is
 * highlighted on its own, so a construct that spans lines (a block comment)
 * colours only the line it starts on.
 */
export function highlighter(language) {
  const fixed = language == null || language === '' ? null : resolve(language);
  if (language != null && language !== '' && !fixed) throw new TypeError(`highlighter: unknown language "${language}"; bundled: ${languages.join(', ')}`);
  let mounted = false;
  return function highlight(file, line) {
    const text = typeof line === 'string' ? line : String(line?.text ?? '');
    const fragment = document.createDocumentFragment();
    const support = fixed ?? resolve(languageForPath(file?.path || file?.oldPath));
    if (!support || !text) { if (text) fragment.append(text); return fragment; }
    if (!mounted) { StyleModule.mount(document, highlightStyle.module); mounted = true; }
    const tree = support.language.parser.parse(text);
    highlightCode(text, tree, highlightStyle, (part, classes) => {
      if (!classes) { fragment.append(part); return; }
      const span = document.createElement('span');
      span.className = classes;
      span.textContent = part;
      fragment.append(span);
    }, () => fragment.append('\n'));
    return fragment;
  };
}
