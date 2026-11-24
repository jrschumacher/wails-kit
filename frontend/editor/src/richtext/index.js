// @wails-kit/editor/richtext: a rich-text (WYSIWYG) editor on ProseMirror.
// Bold, lists and links are edited in place with no visible markup; the
// value goes in and out as Markdown (the Markdown editor's dialect, so both
// can edit the same text) or as sanitised HTML. dist/richtext.js is this
// entry; it shares marked, the limits scan and the link rules with the
// Markdown editor through dist/chunk-*.js, and loads no CodeMirror.
import { EditorState, Plugin, TextSelection } from 'prosemirror-state';
import { EditorView, Decoration, DecorationSet } from 'prosemirror-view';
import { Fragment, Slice } from 'prosemirror-model';
import { keymap } from 'prosemirror-keymap';
import { baseKeymap, toggleMark, setBlockType, wrapIn, lift, chainCommands, exitCode } from 'prosemirror-commands';
import { wrapInList, splitListItem, liftListItem, sinkListItem } from 'prosemirror-schema-list';
import { inputRules, wrappingInputRule, textblockTypeInputRule, InputRule, undoInputRule } from 'prosemirror-inputrules';
import { history, undo, redo } from 'prosemirror-history';
import { StyleModule } from 'style-mod';
import { safeUrl } from '../shared/url.js';
import { warnWithoutAdoptedSheets } from '../shared/adopted.js';
import { mountPreviewStyles } from '../shared/markdown-styles.js';
import { MAX_MARKDOWN_DEPTH } from '../shared/markdown-limits.js';
import { buildSchema, featureSet, linkTarget, languageOf, FEATURES } from './schema.js';
import { parseMarkdown, serializeMarkdown, sourceText } from './markdown.js';
import { serializeHtml, parseHtml, withoutInlineStyles } from './html.js';
import { nesting } from './limits.js';
import { richtextStyles } from './style-spec.js';

export { FEATURES };

const SVG = 'http://www.w3.org/2000/svg';
const FORMATS = ['markdown', 'html'];
const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent || '');

let stylesModule = null;
function mountStyles() {
  mountPreviewStyles();
  stylesModule ??= new StyleModule(richtextStyles);
  StyleModule.mount(document, stylesModule);
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

// ---- Document helpers -----------------------------------------------------

// The nearest ancestor of the selection's start that matches, as { node, pos }.
function ancestor(state, test) {
  const { $from } = state.selection;
  for (let d = $from.depth; d > 0; d--) {
    const node = $from.node(d);
    if (test(node)) return { node, pos: $from.before(d), depth: d };
  }
  return null;
}

function markActive(state, type) {
  const { from, to, empty, $from } = state.selection;
  if (empty) return !!type.isInSet(state.storedMarks || $from.marks());
  let text = false, all = true;
  state.doc.nodesBetween(from, to, node => {
    if (!node.isText) return;
    text = true;
    if (!type.isInSet(node.marks)) all = false;
  });
  return text && all;
}

// The link around the selection's start: { from, to, mark }, or null.
function linkAt(state) {
  const type = state.schema.marks.link;
  if (!type) return null;
  const { $from } = state.selection;
  const mark = type.isInSet($from.marks()) || ($from.nodeAfter && type.isInSet($from.nodeAfter.marks));
  if (!mark) return null;
  // The run of the parent's children carrying this link that holds $from.
  let pos = $from.start(), from = null, found = null;
  $from.parent.forEach(child => {
    const end = pos + child.nodeSize;
    if (mark.isInSet(child.marks)) {
      from ??= pos;
      if (!found && from <= $from.pos && $from.pos <= end) found = { from, to: end, mark };
      else if (found && found.from === from) found.to = end;
    } else from = null;
    pos = end;
  });
  return found;
}

function isEmptyDoc(doc) {
  let empty = true;
  doc.descendants(node => {
    if (!empty) return false;
    if ((node.isText && node.text.trim()) || (node.isLeaf && !node.isText && node.type.name !== 'hard_break')) empty = false;
    return true;
  });
  return empty;
}

// ---- Commands ---------------------------------------------------------------

function toggleList(type) {
  return (state, dispatch) => {
    const { list_item: item, bullet_list: bullet, ordered_list: ordered } = state.schema.nodes;
    const list = ancestor(state, n => n.type === bullet || n.type === ordered);
    if (list?.node.type === type) return liftListItem(item)(state, dispatch);
    if (list) {
      if (dispatch) dispatch(state.tr.setNodeMarkup(list.pos, type, type === ordered ? { order: 1, tight: list.node.attrs.tight } : { tight: list.node.attrs.tight }).scrollIntoView());
      return true;
    }
    return wrapInList(type)(state, dispatch);
  };
}

function toggleBlock(type, attrs) {
  return (state, dispatch) => {
    const { $from } = state.selection;
    const active = $from.parent.type === type && (!attrs || Object.entries(attrs).every(([k, v]) => $from.parent.attrs[k] === v));
    return active ? setBlockType(state.schema.nodes.paragraph)(state, dispatch) : setBlockType(type, attrs)(state, dispatch);
  };
}

function toggleQuote(state, dispatch) {
  const quote = state.schema.nodes.blockquote;
  return ancestor(state, n => n.type === quote) ? lift(state, dispatch) : wrapIn(quote)(state, dispatch);
}

function insertRule(state, dispatch) {
  const { horizontal_rule: hr, paragraph } = state.schema.nodes;
  if (!hr) return false;
  if (dispatch) {
    const tr = state.tr.replaceSelectionWith(hr.create());
    // The cursor goes after the rule, into a new paragraph when no text follows.
    const end = tr.selection.to;
    if (!tr.doc.resolve(end).nodeAfter?.isTextblock) tr.insert(end, paragraph.create());
    tr.setSelection(TextSelection.near(tr.doc.resolve(end + 1)));
    dispatch(tr.scrollIntoView());
  }
  return true;
}

// A line break; a new line in a code block.
function hardBreak(state, dispatch) {
  const { $from } = state.selection;
  if ($from.parent.type.spec.code) {
    if (dispatch) dispatch(state.tr.insertText('\n').scrollIntoView());
    return true;
  }
  if (dispatch) dispatch(state.tr.replaceSelectionWith(state.schema.nodes.hard_break.create()).scrollIntoView());
  return true;
}

// Enter on a line that is only a fence ("```", "```go") makes it a code block.
function fenceOnEnter(state, dispatch) {
  const code = state.schema.nodes.code_block;
  const { $from, empty } = state.selection;
  if (!code || !empty || $from.parent.type.name !== 'paragraph' || $from.parentOffset !== $from.parent.content.size) return false;
  const m = $from.parent.textContent.match(/^```([\w+#.-]*)$/);
  if (!m) return false;
  if (dispatch) {
    const start = $from.start();
    const tr = state.tr.delete(start, $from.pos).setBlockType(start, start, code, { language: languageOf(m[1]) });
    dispatch(tr.scrollIntoView());
  }
  return true;
}

function inputRulesFor(schema) {
  const { nodes, marks } = schema;
  const rules = [];
  if (nodes.heading) rules.push(textblockTypeInputRule(/^(#{1,2})\s$/, nodes.heading, m => ({ level: m[1].length })));
  if (nodes.bullet_list) rules.push(wrappingInputRule(/^\s*([-+*])\s$/, nodes.bullet_list));
  if (nodes.ordered_list) {
    rules.push(wrappingInputRule(/^(\d{1,9})\.\s$/, nodes.ordered_list, m => ({ order: +m[1] }), (m, node) => node.childCount + node.attrs.order === +m[1]));
  }
  if (nodes.blockquote) rules.push(wrappingInputRule(/^\s*>\s$/, nodes.blockquote));
  if (nodes.code_block) rules.push(textblockTypeInputRule(/^```([\w+#.-]*)\s$/, nodes.code_block, m => ({ language: languageOf(m[1]) })));
  if (marks.code) {
    // `text` typed with its closing backtick becomes code.
    rules.push(new InputRule(/(^|[^`])`([^`]+)`$/, (state, match, start, end) => {
      const from = start + match[1].length;
      const code = marks.code.create();
      const tr = state.tr.replaceWith(from, end, schema.text(match[2], code.addToSet(state.doc.resolve(from).marks())));
      return tr.removeStoredMark(code);
    }));
  }
  return inputRules({ rules });
}

// ---- The editor ---------------------------------------------------------------

/**
 * Builds a rich-text editor. Returns a <div class="wk-richtext-editor"> with
 * `value` and `disabled` (get and set), `format`, `focus()`, `submit()`,
 * `cancel()` and `destroy()`. It fires bubbling events: `wk-change`
 * ({ value }) on each edit, `wk-submit` ({ value }, cancelable) before
 * onsubmit, and `wk-cancel` before oncancel.
 */
export function wkRichTextEditor(spec = {}) {
  const label = typeof spec.label === 'string' ? spec.label.trim() : '';
  if (!label) throw new TypeError('wkRichTextEditor: label (the accessible name) is required');
  const format = spec.format ?? 'markdown';
  if (!FORMATS.includes(format)) throw new TypeError(`wkRichTextEditor: format must be 'markdown' or 'html', not ${JSON.stringify(format)}`);
  const features = featureSet(spec.features);
  const schema = buildSchema(features);
  const { nodes, marks } = schema;
  const readOnly = !!spec.readOnly;
  const required = spec.required !== false;
  const placeholder = spec.placeholder ? String(spec.placeholder) : '';
  const onlink = url => { if (url) spec.onlink?.(url); };
  warnWithoutAdoptedSheets();
  mountStyles();

  let disabled = !!spec.disabled;
  let busy = false;
  let destroyed = false;
  let error = null;
  // Text past the Markdown limits loads as its source (markdown.js): plain
  // paragraphs, no formatting, and the value goes out exactly as it came.
  let source = false;

  const root = el('div', { class: 'wk-richtext-editor', id: spec.id || null, role: 'group', 'aria-label': label });
  const base = root.id || freshId('wk-richtext-editor');
  const ids = { content: `${base}-content`, error: `${base}-error`, link: `${base}-link` };
  const canEdit = () => !destroyed && !disabled && !readOnly;
  const canFormat = () => canEdit() && !source;

  const load = value => {
    if (format === 'html') return { doc: parseHtml(schema, value), source: false };
    return parseMarkdown(schema, value);
  };
  const serialize = doc => (source ? sourceText(doc) : format === 'html' ? serializeHtml(doc) : serializeMarkdown(doc));

  // ---- Commands bound to this editor ----
  const guard = (cmd, check = canFormat) => (state, dispatch, view) => check() && cmd(state, dispatch, view);
  const TOOLS = [];
  const tool = (name, feature, label, run, active, keys = null, icon_) => {
    if (features.has(feature)) TOOLS.push({ name, label, run: guard(run), active, keys, icon: icon_ });
  };
  tool('bold', 'bold', 'Bold', marks.strong && toggleMark(marks.strong), s => markActive(s, marks.strong), ['B'],
    'M4.5 2.75h4.25a2.6 2.6 0 0 1 0 5.2H4.5zM4.5 7.95h5a2.65 2.65 0 0 1 0 5.3h-5z');
  tool('italic', 'italic', 'Italic', marks.em && toggleMark(marks.em), s => markActive(s, marks.em), ['I'], 'M7 2.75h5M4 13.25h5M10 2.75 6.5 13.25');
  tool('code', 'code', 'Code', marks.code && toggleMark(marks.code), s => markActive(s, marks.code), ['E'], 'M5.5 4.5 2 8l3.5 3.5M10.5 4.5 14 8l-3.5 3.5');
  tool('link', 'link', 'Link', (state, dispatch) => openLink(null, !dispatch), s => !!linkAt(s), ['K'],
    'M6.75 9.25a2.75 2.75 0 0 0 3.9 0l2-2a2.75 2.75 0 0 0-3.9-3.9l-.6.6M9.25 6.75a2.75 2.75 0 0 0-3.9 0l-2 2a2.75 2.75 0 0 0 3.9 3.9l.6-.6');
  for (const level of [1, 2]) {
    tool(`heading${level}`, 'heading', `Heading ${level}`, nodes.heading && toggleBlock(nodes.heading, { level }),
      s => s.selection.$from.parent.type === nodes.heading && s.selection.$from.parent.attrs.level === level, null,
      level === 1 ? 'M2.5 3.5v9M7.5 3.5v9M2.5 8h5M11 5.25 12.75 4v8.5' : 'M2.5 3.5v9M7.5 3.5v9M2.5 8h5M10.25 6.25a1.75 1.75 0 0 1 3.5 0c0 1.75-3.5 3.25-3.5 6.25h3.5');
  }
  tool('bulletList', 'bulletList', 'Bulleted list', nodes.bullet_list && toggleList(nodes.bullet_list),
    s => ancestor(s, n => n.type === nodes.bullet_list || n.type === nodes.ordered_list)?.node.type === nodes.bullet_list, ['Shift', '8'],
    'M6.5 4h7M6.5 8h7M6.5 12h7M2.75 4h.01M2.75 8h.01M2.75 12h.01');
  tool('orderedList', 'orderedList', 'Numbered list', nodes.ordered_list && toggleList(nodes.ordered_list),
    s => ancestor(s, n => n.type === nodes.bullet_list || n.type === nodes.ordered_list)?.node.type === nodes.ordered_list, ['Shift', '7'],
    'M6.5 4h7M6.5 8h7M6.5 12h7M2.5 2.75l1-.5v3.5M2.25 9.5a1 1 0 0 1 2 .25L2.25 13h2');
  tool('quote', 'quote', 'Quote', nodes.blockquote && toggleQuote, s => !!ancestor(s, n => n.type === nodes.blockquote), null, 'M3 3.5v9M6.5 5h7M6.5 8h7M6.5 11h5');
  tool('codeBlock', 'codeBlock', 'Code block', nodes.code_block && toggleBlock(nodes.code_block), s => s.selection.$from.parent.type === nodes.code_block, null,
    'M2.5 2.75h11v10.5h-11zM6.25 6 4.5 8l1.75 2M9.75 6l1.75 2-1.75 2');
  tool('rule', 'rule', 'Horizontal rule', insertRule, null, null, 'M2 8h12M5 4.75h6M5 11.25h6');
  const byName = Object.fromEntries(TOOLS.map(t => [t.name, t]));
  const run = name => (state, dispatch, view) => (byName[name] ? byName[name].run(state, dispatch, view) : false);

  const actionSpecs = Array.isArray(spec.actions) ? spec.actions : [];
  const submitting = () => !!spec.onsubmit || actionSpecs.some(a => a.submit);

  const keys = {
    'Mod-z': guard(undo, canEdit), 'Shift-Mod-z': guard(redo, canEdit), 'Mod-y': guard(redo, canEdit),
    'Mod-b': run('bold'), 'Mod-i': run('italic'), 'Mod-e': run('code'), 'Mod-k': run('link'),
    'Shift-Mod-8': run('bulletList'), 'Shift-Mod-7': run('orderedList'),
    'Mod-]': guard(nodes.list_item ? sinkListItem(nodes.list_item) : () => false),
    'Mod-[': guard(nodes.list_item ? liftListItem(nodes.list_item) : () => false),
    Enter: guard(chainCommands(fenceOnEnter, nodes.list_item ? splitListItem(nodes.list_item) : () => false)),
    'Shift-Enter': guard(hardBreak, canEdit),
    'Mod-Enter': (state, dispatch, view) => {
      if (submitting()) { submit(); return true; }
      return canEdit() && exitCode(state, dispatch, view);
    },
    Backspace: guard(undoInputRule, canEdit),
    // To the toolbar, from the text (a common editor key; Shift-Tab works too).
    'Alt-F10': () => {
      const stop = toolButtons.find(b => b.tabIndex === 0);
      if (!stop || toolbar.hidden) return false;
      stop.focus();
      return true;
    },
  };
  // The base keys edit: none while read-only or disabled. Escape is left to
  // the editor (cancel) and the page, not ProseMirror's "select parent node".
  const baseKeys = Object.fromEntries(Object.entries(baseKeymap).filter(([k]) => k !== 'Escape').map(([k, cmd]) => [k, guard(cmd, canEdit)]));

  const placeholderPlugin = new Plugin({
    props: {
      decorations(state) {
        const first = state.doc.firstChild;
        if (!placeholder || state.doc.childCount !== 1 || first.type !== nodes.paragraph || first.content.size) return null;
        return DecorationSet.create(state.doc, [Decoration.node(0, first.nodeSize, { class: 'wk-richtext-editor-placeholder', 'data-placeholder': placeholder })]);
      },
    },
  });
  // No edit nests quotes and lists past the Markdown limit (limits.js), so
  // what the editor saves loads again as Markdown.
  const depthGuard = new Plugin({
    filterTransaction(tr, state) {
      if (!tr.docChanged) return true;
      const after = nesting(tr.doc);
      return after <= MAX_MARKDOWN_DEPTH || after <= nesting(state.doc);
    },
  });
  const plugins = () => [
    ...(source ? [] : [inputRulesFor(schema)]),
    keymap(keys),
    keymap(baseKeys),
    history(),
    placeholderPlugin,
    depthGuard,
  ];
  const createState = doc => EditorState.create({ doc, plugins: plugins() });

  // Pasted (and dropped) content: the schema has already cut it down to the
  // features; in source mode, or when it nests too deep, only its text goes in.
  function transformPasted(slice) {
    if (!source && nesting(slice.content) <= MAX_MARKDOWN_DEPTH) return slice;
    const text = slice.content.textBetween(0, slice.content.size, '\n', '\n');
    const lines = text.split('\n').map(line => nodes.paragraph.create(null, line ? schema.text(line) : null));
    return new Slice(Fragment.fromArray(lines), 1, 1);
  }

  // ---- DOM ----
  const loaded = load(spec.value);
  source = loaded.source;
  const contentAttrs = () => {
    const described = error ? ids.error : null;
    return {
      id: ids.content,
      class: 'wk-markdown wk-richtext-editor-content',
      role: 'textbox',
      'aria-multiline': 'true',
      'aria-label': label,
      ...(placeholder ? { 'aria-placeholder': placeholder } : {}),
      ...(readOnly ? { 'aria-readonly': 'true', tabindex: '0' } : {}),
      ...(disabled ? { 'aria-disabled': 'true' } : {}),
      ...(described ? { 'aria-describedby': described } : {}),
      ...(error ? { 'aria-invalid': 'true' } : {}),
    };
  };

  // Toolbar: quiet icon buttons in groups, one tab stop, arrow keys between
  // them (the WAI-ARIA toolbar pattern).
  const toolbar = el('div', { class: 'wk-richtext-editor-toolbar', role: 'toolbar', 'aria-label': 'Formatting', 'aria-controls': ids.content });
  const toolButtons = [];
  const groups = [['bold', 'italic', 'code', 'link'], ['heading1', 'heading2'], ['bulletList', 'orderedList', 'quote', 'codeBlock'], ['rule']];
  for (const group of groups) {
    const tools = group.map(n => byName[n]).filter(Boolean);
    if (!tools.length) continue;
    if (toolButtons.length) toolbar.append(el('div', { class: 'wk-richtext-editor-separator', role: 'separator', 'aria-orientation': 'vertical' }));
    for (const t of tools) {
      const combo = t.keys ? [isMac ? 'Meta' : 'Control', ...t.keys].join('+') : null;
      const shown = t.keys ? `${isMac ? '⌘' : 'Ctrl+'}${t.keys.map(k => (k === 'Shift' ? (isMac ? '⇧' : 'Shift+') : k)).join('')}` : '';
      const b = el('button', {
        type: 'button', class: 'wk-button wk-button--quiet wk-button--icon', 'aria-label': t.label, 'data-tool': t.name,
        'aria-keyshortcuts': combo, title: shown ? `${t.label} (${shown})` : t.label, tabindex: toolButtons.length ? '-1' : '0',
        'aria-pressed': t.active ? 'false' : null, 'aria-haspopup': t.name === 'link' ? 'dialog' : null,
      }, [icon(t.icon)]);
      // Keep focus (and the selection) in the text.
      b.addEventListener('mousedown', e => e.preventDefault());
      b.addEventListener('click', () => {
        if (b.getAttribute('aria-disabled') === 'true') return;
        // The selection the person just made, even if ProseMirror has not read it yet.
        view.domObserver?.flush?.();
        if (t.name === 'link') { openLink(b); return; }
        t.run(view.state, view.dispatch, view);
        view.focus();
      });
      toolButtons.push(b);
      toolbar.append(b);
    }
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
  if (readOnly || spec.toolbar === false || !toolButtons.length) toolbar.hidden = true;

  const view = new EditorView(null, {
    state: createState(loaded.doc),
    attributes: contentAttrs,
    editable: () => canEdit(),
    transformPasted,
    transformPastedHTML: withoutInlineStyles,
    // A key right after a selection change (Shift+Arrow, then Mod-K) acts on
    // the new selection: ProseMirror reads the DOM selection asynchronously,
    // so read it first.
    handleKeyDown(v, e) {
      v.domObserver?.flush?.();
      // ProseMirror calls preventDefault() on every Escape in the text, so
      // the root listener below would skip it: cancel here.
      if (e.key === 'Escape' && spec.oncancel && !e.isComposing && e.keyCode !== 229 && !v.composing) { cancel(); return true; }
      return false;
    },
    dispatchTransaction(tr) {
      if (destroyed) return;
      view.updateState(view.state.apply(tr));
      refreshTools();
      if (tr.docChanged) {
        const value = serialize(view.state.doc);
        if (error) setError(problem());
        root.dispatchEvent(new CustomEvent('wk-change', { bubbles: true, detail: { value } }));
        spec.onchange?.(value);
      }
    },
  });
  // A number is pixels; a string is any CSS length (set through the CSSOM,
  // which a strict CSP allows).
  const length = v => (typeof v === 'number' && Number.isFinite(v) ? `${v}px` : String(v));
  if (spec.minHeight != null && spec.minHeight !== '') view.dom.style.minHeight = length(spec.minHeight);
  if (spec.maxHeight != null && spec.maxHeight !== '') view.dom.style.maxHeight = length(spec.maxHeight);

  // Links never navigate the webview: a click with Mod (or any click while
  // the text cannot be edited) goes to onlink.
  view.dom.addEventListener('click', e => {
    const a = e.target.closest?.('a[href]');
    if (!a || !view.dom.contains(a)) return;
    e.preventDefault();
    if (!view.editable || e.metaKey || e.ctrlKey) onlink(safeUrl(a.getAttribute('href')));
  });
  view.dom.addEventListener('auxclick', e => { if (e.target.closest?.('a[href]')) e.preventDefault(); });

  // ---- The link popover (a kit wk-popover) ----
  const linkInput = el('input', { class: 'wk-field', type: 'url', id: `${ids.link}-url`, autocomplete: 'off', spellcheck: 'false', placeholder: 'https://' });
  const linkError = el('p', { class: 'wk-error wk-richtext-editor-link-error', id: `${ids.link}-error`, role: 'status' });
  const linkButtons = {
    open: el('button', { type: 'button', class: 'wk-button wk-button--small' }, ['Open']),
    remove: el('button', { type: 'button', class: 'wk-button wk-button--small wk-button--destructive' }, ['Remove']),
    cancel: el('button', { type: 'button', class: 'wk-button wk-button--small' }, ['Cancel']),
    save: el('button', { type: 'button', class: 'wk-button wk-button--small wk-button--primary' }, ['Save']),
  };
  const popover = el('div', { class: 'wk-popover wk-richtext-editor-link', id: ids.link, role: 'dialog', 'aria-label': 'Link', hidden: true }, [
    el('div', { class: 'wk-popover-section' }, [el('label', { class: 'wk-label', for: linkInput.id }, ['Link address']), linkInput, linkError]),
    el('div', { class: 'wk-popover-footer wk-richtext-editor-link-actions' }, Object.values(linkButtons)),
  ]);
  let linking = null; // { from, to, link, opener }

  function openLink(opener = null, check = false) {
    if (!canFormat() || !marks.link) return false;
    if (check) return true;
    const { state } = view;
    const link = linkAt(state);
    const { from, to } = link && state.selection.empty ? link : state.selection;
    linking = { from, to, link, opener };
    const selected = state.doc.textBetween(from, to, ' ');
    linkInput.value = link ? link.mark.attrs.href : (safeUrl(selected.trim()) ? selected.trim() : '');
    setLinkError(null);
    linkButtons.open.hidden = !link;
    linkButtons.remove.hidden = !link;
    popover.hidden = false;
    // Under the text where the link is, inside the editor's width.
    const at = view.coordsAtPos(from);
    const box = root.getBoundingClientRect();
    const width = popover.offsetWidth;
    popover.style.top = `${Math.round(at.bottom - box.top + 4)}px`;
    popover.style.left = `${Math.round(Math.max(0, Math.min(at.left - box.left, box.width - width)))}px`;
    linkButtons.open.disabled = !spec.onlink;
    toolButtons.find(b => b.dataset.tool === 'link')?.setAttribute('aria-expanded', 'true');
    linkInput.focus();
    linkInput.select();
    return true;
  }
  // focus: 'text' (after a change), 'opener' (cancelled) or false (focus moved elsewhere).
  function closeLink(focus) {
    if (!linking) return;
    const { opener } = linking;
    linking = null;
    popover.hidden = true;
    setLinkError(null);
    toolButtons.find(b => b.dataset.tool === 'link')?.removeAttribute('aria-expanded');
    if (focus === 'opener' && opener?.isConnected) opener.focus();
    else if (focus) view.focus();
  }
  function setLinkError(message) {
    linkError.textContent = message || '';
    if (message) {
      linkInput.setAttribute('aria-invalid', 'true');
      linkInput.setAttribute('aria-describedby', linkError.id);
    } else {
      linkInput.removeAttribute('aria-invalid');
      linkInput.removeAttribute('aria-describedby');
    }
  }
  // What the person typed as a link target: as written, or with https://
  // (or mailto:) added when it has no scheme.
  function targetOf(text) {
    if (!text) return null;
    const direct = linkTarget(text);
    if (direct || /^[a-z][a-z0-9+.-]*:/i.test(text)) return direct;
    if (/^[^\s@/]+@[^\s@/]+\.[^\s@/]+$/.test(text)) return linkTarget(`mailto:${text}`);
    return /^[^\s/]+\.[^\s/]+/.test(text) ? linkTarget(`https://${text}`) : null;
  }
  function saveLink() {
    if (!linking) return;
    const text = linkInput.value.trim();
    if (!text && linking.link) { removeLink(); return; }
    const href = targetOf(text);
    if (!href) {
      setLinkError(text ? 'Use a web or email address, such as https://example.com.' : 'Enter a link address.');
      linkInput.focus();
      return;
    }
    const { state } = view;
    const size = state.doc.content.size;
    const from = Math.min(linking.from, size), to = Math.min(linking.to, size);
    const mark = marks.link.create({ href });
    let tr = state.tr;
    if (from === to) {
      // No text chosen: the address becomes the link's text.
      const shown = text;
      tr = tr.insert(from, schema.text(shown, mark.addToSet(state.doc.resolve(from).marks())));
      tr = tr.setSelection(TextSelection.create(tr.doc, from + shown.length)).removeStoredMark(marks.link);
    } else {
      tr = tr.removeMark(from, to, marks.link).addMark(from, to, mark);
    }
    view.dispatch(tr.scrollIntoView());
    closeLink('text');
  }
  function removeLink() {
    if (!linking) return;
    const size = view.state.doc.content.size;
    view.dispatch(view.state.tr.removeMark(Math.min(linking.from, size), Math.min(linking.to, size), marks.link));
    closeLink('text');
  }
  linkButtons.save.addEventListener('click', saveLink);
  linkButtons.remove.addEventListener('click', removeLink);
  linkButtons.cancel.addEventListener('click', () => closeLink('opener'));
  linkButtons.open.addEventListener('click', () => { if (linking?.link) onlink(safeUrl(linking.link.mark.attrs.href)); });
  linkInput.addEventListener('keydown', e => {
    if (e.key === 'Enter' && !e.isComposing) { e.preventDefault(); saveLink(); }
  });
  linkInput.addEventListener('input', () => { if (linkError.textContent) setLinkError(null); });
  popover.addEventListener('keydown', e => {
    if (e.key !== 'Escape' || e.isComposing) return;
    // The popover's Escape: it closes, and nothing outside sees the key.
    e.preventDefault();
    e.stopPropagation();
    closeLink('opener');
  });
  // Focus leaving the popover for elsewhere closes it, without moving focus.
  popover.addEventListener('focusout', e => {
    if (linking && e.relatedTarget && !popover.contains(e.relatedTarget)) closeLink(false);
  });

  // ---- Footer: the error and the actions ----
  // A polite live region from the start, so a submit error is announced
  // even when focus does not move (Mod-Enter in the text).
  const errorEl = el('p', { class: 'wk-richtext-editor-error', id: ids.error, role: 'status' });
  const actionButtons = [];
  for (const a of actionSpecs) {
    const b = el('button', { type: 'button', class: `wk-button${a.primary ? ' wk-button--primary' : ''}`, 'data-submit': a.submit ? '' : null, 'data-cancel': !a.submit && a.cancel ? '' : null }, [String(a.label ?? '')]);
    b.addEventListener('click', () => {
      if (b.getAttribute('aria-disabled') === 'true') return;
      if (a.submit) submit();
      else if (a.cancel) cancel();
      else a.onclick?.(root);
    });
    actionButtons.push(b);
  }
  const foot = el('div', { class: 'wk-richtext-editor-foot' }, [errorEl, ...(actionButtons.length ? [el('div', { class: 'wk-richtext-editor-actions' }, actionButtons)] : [])]);
  root.append(toolbar, view.dom, popover, foot);

  // ---- State ----
  function refreshTools() {
    const { state } = view;
    const off = !canFormat();
    for (const b of toolButtons) {
      const t = byName[b.dataset.tool];
      if (t.active) b.setAttribute('aria-pressed', String(!off && !!t.active(state)));
      // In source mode the buttons stay focusable but do nothing.
      if (source && !disabled) b.setAttribute('aria-disabled', 'true'); else b.removeAttribute('aria-disabled');
    }
    root.toggleAttribute('data-source', source);
  }
  const problem = () => (required && isEmptyDoc(view.state.doc) ? 'Write something first.' : null);
  let announce = 0;
  function setError(next, { repeat = false } = {}) {
    if (next === error) {
      if (!repeat || !next) return;
      // The same message again (another submit) is announced again.
      errorEl.textContent = '';
      const turn = ++announce;
      requestAnimationFrame(() => { if (turn === announce && error === next && !destroyed) errorEl.textContent = next; });
      return;
    }
    announce++;
    error = next;
    errorEl.textContent = next || '';
    view.setProps({});
  }
  function setDisabled() {
    root.toggleAttribute('data-disabled', disabled);
    if (disabled) closeLink(false);
    const lost = disabled && [...toolButtons, ...actionButtons].includes(document.activeElement);
    // A cancel action stays usable.
    for (const b of [...toolButtons, ...actionButtons]) b.disabled = disabled && !b.hasAttribute('data-cancel');
    view.setProps({});
    refreshTools();
    // A disabled button drops focus to <body>: keep it in the editor (a named group).
    if (lost && root.isConnected) {
      root.tabIndex = -1;
      HTMLElement.prototype.focus.call(root);
    }
    if (!disabled && root.hasAttribute('tabindex')) {
      const parked = document.activeElement === root;
      root.removeAttribute('tabindex');
      if (parked) view.focus();
    }
  }
  // The busy state lives on the submit controls (BEHAVIOUR.md: aria-disabled
  // and data-busy), not on the editor.
  function setBusy(on) {
    busy = on;
    for (const b of actionButtons.filter(x => x.hasAttribute('data-submit'))) {
      b.toggleAttribute('data-busy', on);
      if (on) b.setAttribute('aria-disabled', 'true'); else b.removeAttribute('aria-disabled');
    }
  }

  function cancel() {
    if (destroyed) return;
    root.dispatchEvent(new CustomEvent('wk-cancel', { bubbles: true }));
    spec.oncancel?.(root);
  }
  // Escape cancels when oncancel is set: not from the link popover (which
  // closes), not while an input method is composing.
  root.addEventListener('keydown', e => {
    if (e.key !== 'Escape' || e.defaultPrevented || !spec.oncancel) return;
    if (e.isComposing || e.keyCode === 229 || view.composing) return;
    e.preventDefault();
    cancel();
  });

  function submit() {
    if (destroyed || disabled || busy) return false;
    const why = problem();
    setError(why, { repeat: true });
    if (why) { view.focus(); return false; }
    const value = serialize(view.state.doc);
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
      get: () => serialize(view.state.doc),
      set: next => {
        if (destroyed) return;
        const hadFocus = view.hasFocus();
        closeLink(false);
        const loadedNext = load(next);
        source = loadedNext.source;
        // New content: a fresh state, so Mod-Z does not undo a load.
        view.updateState(createState(loadedNext.doc));
        setError(null);
        refreshTools();
        if (hadFocus) view.focus();
      },
      enumerable: true,
    },
    format: { get: () => format, enumerable: true },
    disabled: {
      get: () => disabled,
      set: next => { if (!destroyed && !!next !== disabled) { disabled = !!next; setDisabled(); } },
      enumerable: true,
    },
    focus: { value: () => { if (!destroyed) view.focus(); } },
    submit: { value: () => submit() },
    cancel: { value: () => cancel() },
    destroy: {
      value: () => {
        if (destroyed) return;
        destroyed = true;
        view.destroy();
        root.replaceChildren();
      },
    },
  });

  view.dom.addEventListener('focus', refreshTools);
  refreshTools();
  if (disabled) setDisabled();
  return root;
}
