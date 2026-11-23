// The editors' CodeMirror core: a themed EditorView in a <div>, with the
// value, focus() and destroy() the kit's editors share. wkCodeEditor is this
// with no internal options; the Markdown editor adds its own key bindings and
// commands through `internal`, which no public entry exposes (an extension
// from another copy of CodeMirror would not work with the bundled one).
import { EditorState, Compartment } from '@codemirror/state';
import {
  EditorView, keymap, lineNumbers, highlightActiveLineGutter, highlightSpecialChars,
  drawSelection, placeholder as placeholderText,
} from '@codemirror/view';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { indentOnInput, bracketMatching, syntaxHighlighting, indentUnit } from '@codemirror/language';
import { search, searchKeymap } from '@codemirror/search';
import { theme, highlightStyle } from './theme.js';
import { createSearchPanel } from './search-panel.js';
import { languageExtension, languageName } from './languages.js';

// CodeMirror's styles reach the page as a stylesheet adopted by the
// document (see scripts/build.mjs). Without adoptedStyleSheets (WebKit before
// Safari 16.4) they fall back to a <style> element, which a strict CSP
// blocks, and the editor renders broken: say so once.
let warned = false;
export function warnWithoutAdoptedSheets() {
  if (warned || typeof document === 'undefined' || 'adoptedStyleSheets' in document) return;
  warned = true;
  console.warn('@wails-kit/editor: this browser has no document.adoptedStyleSheets, so the editor\'s styles need a CSP that allows inline styles. It needs WebView2, Safari 16.4+ (macOS 13.3) or WebKitGTK 2.40+.');
}

const forcedColors = () => typeof matchMedia === 'function' && matchMedia('(forced-colors: active)').matches;

/**
 * Builds a code editor: { el, view }. `el` is a <div class="wk-code-editor">
 * holding it, with `value` (get and set), `focus()` and `destroy()`; it fires
 * a bubbling `wk-change` event ({ detail: { value } }) on each edit. After
 * destroy(), setting `value` and focus() do nothing.
 *
 * internal (for the kit's other editors only):
 *   name          the name in error messages (default "wkCodeEditor")
 *   extensions()  more CodeMirror extensions, called each time the state is
 *                 (re)built, so they can read the caller's current settings
 *   onedit(value) called after each edit instead of the event and onchange
 */
export function createCodeEditor(spec = {}, internal = {}) {
  const name = internal.name || 'wkCodeEditor';
  const label = typeof spec.label === 'string' ? spec.label.trim() : '';
  if (!label) throw new TypeError(`${name}: label (the accessible name) is required`);
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
    internal.extensions ? internal.extensions() : [],
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
      if (internal.onedit) { internal.onedit(value); return; }
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
  return { el, view };
}
