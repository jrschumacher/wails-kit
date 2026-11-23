// @wails-kit/editor/markdown: a Markdown editor like a pull request's comment
// box: Write (CodeMirror in Markdown mode) and Preview tabs, an optional
// formatting toolbar, a character count and the buttons under it; and
// renderMarkdown(), the preview's safe renderer, for showing posted text
// read-only under the same rules. dist/markdown.js is this entry; it shares
// CodeMirror (dist/chunk-*.js) with the code editor.
//
// The tabs are @wails-kit/ui's <wk-tabs>: the page loads @wails-kit/ui's
// script (kit.js, or dist/wails-kit-ui.js), which defines it.
import { EditorSelection, EditorState, Compartment, Prec } from '@codemirror/state';
import { EditorView, keymap } from '@codemirror/view';
import { StyleModule } from 'style-mod';
import { createCodeEditor } from '../shared/code-editor.js';
import { renderMarkdown, safeUrl, mountPreviewStyles } from './render.js';
import { editorStyles } from './style-spec.js';

export { renderMarkdown };

const SVG = 'http://www.w3.org/2000/svg';
const MODES = ['write', 'preview'];
const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent || '');

let warnedTabs = false;
function warnWithoutTabs() {
  if (warnedTabs || typeof customElements === 'undefined' || customElements.get('wk-tabs')) return;
  warnedTabs = true;
  console.warn('@wails-kit/editor: <wk-tabs> is not defined, so the Markdown editor\'s Write and Preview tabs do not switch. Load @wails-kit/ui\'s script (kit.js) on the page.');
}

let editorModule = null;
function mountStyles() {
  mountPreviewStyles();
  editorModule ??= new StyleModule(editorStyles);
  StyleModule.mount(document, editorModule);
}

let uid = 0;
function freshId(prefix) {
  let id;
  do id = `${prefix}-${++uid}`; while (document.getElementById(id));
  return id;
}

function el(tag, attrs = {}, children = []) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) if (v != null && v !== false) node.setAttribute(k, v === true ? '' : String(v));
  node.append(...children);
  return node;
}

function icon(d) {
  const svg = document.createElementNS(SVG, 'svg');
  for (const [k, v] of Object.entries({ viewBox: '0 0 16 16', fill: 'none', stroke: 'currentColor', 'stroke-width': '1.5', 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'aria-hidden': 'true' })) svg.setAttribute(k, v);
  const path = document.createElementNS(SVG, 'path');
  path.setAttribute('d', d);
  svg.append(path);
  return svg;
}

// ---- Formatting commands (CodeMirror commands: view → handled) ----------

// Wraps each selection in mark (**, _, `), or unwraps it when the mark is
// already around it. An empty selection gets the pair with the cursor inside.
function wrap(mark) {
  return view => {
    const { state } = view;
    if (state.readOnly) return false;
    const n = mark.length;
    view.dispatch(state.update(state.changeByRange(r => {
      if (state.sliceDoc(r.from - n, r.from) === mark && state.sliceDoc(r.to, r.to + n) === mark) {
        return { changes: [{ from: r.from - n, to: r.from }, { from: r.to, to: r.to + n }], range: EditorSelection.range(r.from - n, r.to - n) };
      }
      return { changes: [{ from: r.from, insert: mark }, { from: r.to, insert: mark }], range: EditorSelection.range(r.from + n, r.to + n) };
    }), { scrollIntoView: true, userEvent: 'input.format' }));
    return true;
  };
}

// Inline code, or a fenced block when the selection spans lines.
function code(view) {
  const { state } = view;
  if (state.readOnly) return false;
  if (!state.selection.ranges.some(r => state.sliceDoc(r.from, r.to).includes('\n'))) return wrap('`')(view);
  view.dispatch(state.update(state.changeByRange(r => {
    const open = (r.from > 0 && state.sliceDoc(r.from - 1, r.from) !== '\n' ? '\n' : '') + '```\n';
    const text = state.sliceDoc(r.from, r.to);
    const close = (text.endsWith('\n') ? '' : '\n') + '```';
    return { changes: { from: r.from, to: r.to, insert: open + text + close }, range: EditorSelection.range(r.from + open.length, r.from + open.length + text.length) };
  }), { scrollIntoView: true, userEvent: 'input.format' }));
  return true;
}

// [text](url) with "url" selected to type over; a selected URL becomes the
// link's target with the cursor in its text.
function link(view) {
  const { state } = view;
  if (state.readOnly) return false;
  view.dispatch(state.update(state.changeByRange(r => {
    const text = state.sliceDoc(r.from, r.to);
    if (text && !text.includes('\n') && safeUrl(text.trim())) {
      return { changes: { from: r.from, to: r.to, insert: `[](${text.trim()})` }, range: EditorSelection.cursor(r.from + 1) };
    }
    const insert = `[${text}](url)`;
    const at = r.from + text.length + 3;
    return { changes: { from: r.from, to: r.to, insert }, range: text ? EditorSelection.range(at, at + 3) : EditorSelection.cursor(r.from + 1) };
  }), { scrollIntoView: true, userEvent: 'input.format' }));
  return true;
}

// Puts prefix ("- ", "> ") before each selected line, or takes it off when
// every one has it.
function prefixLines(prefix) {
  return view => {
    const { state } = view;
    if (state.readOnly) return false;
    const lines = new Map();
    for (const r of state.selection.ranges) {
      for (let pos = r.from; pos <= r.to;) {
        const line = state.doc.lineAt(pos);
        lines.set(line.number, line);
        pos = line.to + 1;
      }
    }
    const all = [...lines.values()];
    const has = all.every(l => l.text.startsWith(prefix));
    const changes = state.changes(all.map(l => (has ? { from: l.from, to: l.from + prefix.length } : { from: l.from, insert: prefix })));
    view.dispatch({ changes, selection: state.selection.map(changes, 1), scrollIntoView: true, userEvent: 'input.format' });
    return true;
  };
}

const TOOLS = [
  { name: 'bold', label: 'Bold', key: 'B', run: wrap('**'), icon: 'M4.5 2.75h4.25a2.6 2.6 0 0 1 0 5.2H4.5zM4.5 7.95h5a2.65 2.65 0 0 1 0 5.3h-5z' },
  { name: 'italic', label: 'Italic', key: 'I', run: wrap('_'), icon: 'M7 2.75h5M4 13.25h5M10 2.75 6.5 13.25' },
  { name: 'code', label: 'Code', key: 'E', run: code, icon: 'M5.5 4.5 2 8l3.5 3.5M10.5 4.5 14 8l-3.5 3.5' },
  { name: 'link', label: 'Link', key: 'K', run: link, icon: 'M6.75 9.25a2.75 2.75 0 0 0 3.9 0l2-2a2.75 2.75 0 0 0-3.9-3.9l-.6.6M9.25 6.75a2.75 2.75 0 0 0-3.9 0l-2 2a2.75 2.75 0 0 0 3.9 3.9l.6-.6' },
  { name: 'list', label: 'Bulleted list', run: prefixLines('- '), icon: 'M6.5 4h7M6.5 8h7M6.5 12h7M2.75 4h.01M2.75 8h.01M2.75 12h.01' },
  { name: 'quote', label: 'Quote', run: prefixLines('> '), icon: 'M3 3.5v9M6.5 5h7M6.5 8h7M6.5 11h5' },
];

const lengthOf = value => value.length;

/**
 * Builds a Markdown editor. Returns a <div class="wk-markdown-editor"> with
 * `value`, `mode` and `disabled` (get and set), `focus()`, `submit()` and
 * `destroy()`. It fires bubbling events: `wk-change` ({ value }) on each edit,
 * `wk-mode` ({ mode }) when the person switches tabs, and `wk-submit`
 * ({ value }, cancelable) before onsubmit.
 */
export function wkMarkdownEditor(spec = {}) {
  const label = typeof spec.label === 'string' ? spec.label.trim() : '';
  if (!label) throw new TypeError('wkMarkdownEditor: label (the accessible name) is required');
  const maxLength = Number.isInteger(spec.maxLength) && spec.maxLength > 0 ? spec.maxLength : null;
  const required = spec.required !== false;
  const renderOptions = { onlink: url => spec.onlink?.(url), imageHosts: spec.imageHosts };
  warnWithoutTabs();
  mountStyles();

  let disabled = !!spec.disabled;
  let busy = false;
  let destroyed = false;
  let mode = 'write';
  let error = null;

  const root = el('div', { class: 'wk-markdown-editor', id: spec.id || null });
  const base = root.id || freshId('wk-markdown-editor');
  const ids = { write: `${base}-write`, preview: `${base}-preview`, count: `${base}-count`, error: `${base}-error` };

  // The text: the code editor's core in Markdown mode, with the formatting
  // keys, Mod-Enter to submit, and the disabled state.
  const state = new Compartment();
  const stateFor = () => {
    const described = [maxLength ? ids.count : null, error ? ids.error : null].filter(Boolean).join(' ');
    return [
    // First, so these win over the core's own readOnly and editable (each
    // facet takes its first value).
    EditorState.readOnly.of(disabled),
    EditorView.editable.of(!disabled),
    EditorView.contentAttributes.of({
      ...(described ? { 'aria-describedby': described } : {}),
      ...(error ? { 'aria-invalid': 'true' } : {}),
      ...(disabled ? { 'aria-disabled': 'true' } : {}),
    }),
    ];
  };
  const { el: text, view } = createCodeEditor({
    label,
    value: spec.value,
    language: 'markdown',
    lineNumbers: false,
    wrap: true,
    placeholder: spec.placeholder,
  }, {
    name: 'wkMarkdownEditor',
    extensions: () => [
      state.of(stateFor()),
      Prec.high(keymap.of([
        { key: 'Mod-Enter', run: () => { submit(); return true; } },
        ...TOOLS.filter(t => t.key).map(t => ({ key: `Mod-${t.key.toLowerCase()}`, run: t.run })),
      ])),
    ],
    onedit: value => {
      update();
      root.dispatchEvent(new CustomEvent('wk-change', { bubbles: true, detail: { value } }));
      spec.onchange?.(value);
    },
  });
  if (spec.minHeight) text.style.minHeight = String(spec.minHeight);
  if (spec.maxHeight) text.style.maxHeight = String(spec.maxHeight);
  const refresh = () => view.dispatch({ effects: state.reconfigure(stateFor()) });

  // Tabs.
  const tab = (name, text) => el('button', { type: 'button', role: 'tab', id: `${ids[name]}-tab`, 'aria-controls': ids[name], 'aria-selected': String(name === 'write'), tabindex: name === 'write' ? '0' : '-1' }, [text]);
  const tabs = { write: tab('write', 'Write'), preview: tab('preview', 'Preview') };
  const tablist = el('div', { class: 'wk-tablist', role: 'tablist', 'aria-label': label }, [tabs.write, tabs.preview]);
  const preview = el('div', { class: 'wk-markdown wk-markdown-editor-preview' });
  const panels = {
    // The text inside is the stop; the panel itself is not.
    write: el('div', { role: 'tabpanel', id: ids.write, 'aria-labelledby': tabs.write.id, tabindex: '-1' }, [text]),
    preview: el('div', { role: 'tabpanel', id: ids.preview, 'aria-labelledby': tabs.preview.id, tabindex: '0', hidden: true }, [preview]),
  };

  // Toolbar: quiet icon buttons, one tab stop, arrow keys between them.
  let toolbar = null;
  const toolButtons = [];
  if (spec.toolbar !== false) {
    toolbar = el('div', { class: 'wk-markdown-editor-toolbar', role: 'toolbar', 'aria-label': 'Formatting', 'aria-controls': ids.write });
    for (const t of TOOLS) {
      const keys = t.key ? `${isMac ? 'Meta' : 'Control'}+${t.key}` : null;
      const b = el('button', {
        type: 'button', class: 'wk-button wk-button--quiet wk-button--icon', 'aria-label': t.label, 'data-tool': t.name,
        'aria-keyshortcuts': keys, title: t.key ? `${t.label} (${isMac ? '⌘' : 'Ctrl+'}${t.key})` : t.label, tabindex: toolButtons.length ? '-1' : '0',
      }, [icon(t.icon)]);
      // Keep focus (and the selection) in the text.
      b.addEventListener('mousedown', e => e.preventDefault());
      b.addEventListener('click', () => { if (disabled) return; t.run(view); view.focus(); });
      toolButtons.push(b);
      toolbar.append(b);
    }
    toolbar.addEventListener('keydown', e => {
      const at = toolButtons.indexOf(e.target);
      if (at < 0) return;
      const to = { ArrowRight: at + 1, ArrowLeft: at - 1, Home: 0, End: toolButtons.length - 1 }[e.key];
      if (to === undefined) return;
      e.preventDefault();
      const next = toolButtons[(to + toolButtons.length) % toolButtons.length];
      for (const b of toolButtons) b.tabIndex = b === next ? 0 : -1;
      next.focus();
    });
  }
  const tabsEl = el('wk-tabs', {}, [tablist, ...(toolbar ? [toolbar] : []), panels.write, panels.preview]);

  // Footer: the count, the error and the actions.
  const count = maxLength ? el('span', { class: 'wk-markdown-editor-count', id: ids.count }) : null;
  const errorEl = el('p', { class: 'wk-markdown-editor-error', id: ids.error, hidden: true });
  const status = el('div', { class: 'wk-markdown-editor-status' }, [...(count ? [count] : []), errorEl]);
  const actionButtons = [];
  for (const a of Array.isArray(spec.actions) ? spec.actions : []) {
    const b = el('button', { type: 'button', class: `wk-button${a.primary ? ' wk-button--primary' : ''}`, 'data-submit': a.submit ? '' : null }, [String(a.label ?? '')]);
    b.addEventListener('click', () => {
      if (b.getAttribute('aria-disabled') === 'true') return;
      if (a.submit) submit(); else a.onclick?.(root);
    });
    actionButtons.push(b);
  }
  const foot = el('div', { class: 'wk-markdown-editor-foot' }, [status, ...(actionButtons.length ? [el('div', { class: 'wk-markdown-editor-actions' }, actionButtons)] : [])]);
  root.append(tabsEl, foot);

  // ---- State ----------------------------------------------------------
  const problem = value => {
    if (required && !value.trim()) return 'Write something first.';
    const over = maxLength ? lengthOf(value) - maxLength : 0;
    return over > 0 ? `Shorten this by ${over} character${over === 1 ? '' : 's'}.` : null;
  };
  function setError(next) {
    if (next === error) return;
    error = next;
    errorEl.textContent = next || '';
    errorEl.hidden = !next;
    refresh();
  }
  // After each edit or load: the count, and a shown error re-checked.
  function update() {
    const value = view.state.doc.toString();
    if (count) {
      const n = lengthOf(value);
      count.textContent = `${n} / ${maxLength}`;
      count.toggleAttribute('data-over', n > maxLength);
    }
    if (error) setError(problem(value));
    if (mode === 'preview') showPreview();
  }
  function showPreview() {
    const value = view.state.doc.toString();
    preview.replaceChildren(value.trim() ? renderMarkdown(value, renderOptions) : el('p', { class: 'wk-markdown-editor-empty' }, ['Nothing to preview']));
  }
  function setDisabled() {
    // Not aria-disabled: the tabs and the preview stay usable.
    root.toggleAttribute('data-disabled', disabled);
    for (const b of [...toolButtons, ...actionButtons]) b.disabled = disabled;
    refresh();
  }
  function setBusy(on) {
    busy = on;
    root.toggleAttribute('aria-busy', on);
    for (const b of actionButtons.filter(b => b.hasAttribute('data-submit'))) {
      b.toggleAttribute('data-busy', on);
      if (on) b.setAttribute('aria-disabled', 'true'); else b.removeAttribute('aria-disabled');
    }
  }

  // Shows a mode. wk-tabs selects the tab and panel when it is wired; before
  // that (the editor not yet in the document) the attributes are set here,
  // and wk-tabs keeps them when it wires.
  function show(next) {
    const index = MODES.indexOf(next);
    if (index < 0 || next === mode) return false;
    if (next === 'preview') {
      // The preview starts as tall as the text was, so the page does not jump.
      const height = text.getBoundingClientRect().height;
      if (height) preview.style.minHeight = `${Math.round(height)}px`;
    }
    mode = next;
    const WkTabs = customElements.get('wk-tabs');
    if (WkTabs && tabsEl instanceof WkTabs && tabsEl.isConnected) tabsEl.selectedIndex = index;
    else {
      for (const m of MODES) {
        tabs[m].setAttribute('aria-selected', String(m === next));
        tabs[m].tabIndex = m === next ? 0 : -1;
        panels[m].hidden = m !== next;
      }
    }
    if (toolbar) toolbar.hidden = next !== 'write';
    if (next === 'preview') showPreview(); else preview.replaceChildren();
    return true;
  }
  tabsEl.addEventListener('wk-change', e => {
    if (e.target !== tabsEl) return;
    // The tabs' own event stops here: the editor's wk-change is about the text.
    e.stopPropagation();
    const next = MODES[e.detail.index];
    show(next);
    root.dispatchEvent(new CustomEvent('wk-mode', { bubbles: true, detail: { mode: next } }));
    spec.onmode?.(next);
  });

  function submit() {
    if (destroyed || disabled || busy) return false;
    const value = view.state.doc.toString();
    const why = problem(value);
    setError(why);
    if (why) {
      show('write');
      view.focus();
      return false;
    }
    if (!root.dispatchEvent(new CustomEvent('wk-submit', { bubbles: true, cancelable: true, detail: { value } }))) return false;
    const result = spec.onsubmit?.(value);
    if (result && typeof result.then === 'function') {
      setBusy(true);
      const done = () => { if (!destroyed) setBusy(false); };
      result.then(done, done);
    }
    return true;
  }

  Object.defineProperties(root, {
    value: {
      get: () => view.state.doc.toString(),
      set: next => {
        if (destroyed) return;
        text.value = next;
        setError(null);
        update();
      },
      enumerable: true,
    },
    mode: {
      get: () => mode,
      set: next => { if (!destroyed && MODES.includes(next)) show(next); },
      enumerable: true,
    },
    disabled: {
      get: () => disabled,
      set: next => { if (!destroyed && !!next !== disabled) { disabled = !!next; setDisabled(); } },
      enumerable: true,
    },
    focus: { value: () => { if (destroyed) return; show('write'); view.focus(); } },
    submit: { value: () => submit() },
    destroy: {
      value: () => {
        if (destroyed) return;
        destroyed = true;
        text.destroy();
        root.replaceChildren();
      },
    },
  });

  if (spec.mode === 'preview') show('preview');
  setDisabled();
  update();
  return root;
}
