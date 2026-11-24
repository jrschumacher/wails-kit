// wkUndo: the frontend of the app's undo stack, which lives on the Go side
// (the shortcuts package's UndoStack; BEHAVIOUR.md, "Undo, confirm or
// neither").
//
//   wkUndo.connect({ on, bindings })   on(name, listener) subscribes to a Go
//                                      event and may return an unsubscribe;
//                                      bindings is the stack's Bindings()
//   wkUndo.state()                     { undo, redo, busy, textFocus }
//   wkUndo.undo(), wkUndo.redo()       the app's stack, from a button
//   wkUndo.action(id?)                 a toast's Undo: { label, run, undoId }
//   wkUndo.words                       the kit's words, for localising
//
// Connected, it reports text focus to Go (a field, a contenteditable, or
// an editor host such as the kit's CodeMirror and ProseMirror editors), so
// Edit > Undo and Redo undo typing first: Go sends undo:text (as the DOM
// event wk-undo-text to the window in front), the field or editor gets a
// cancelable beforeinput historyUndo/historyRedo, and then execCommand. A
// plain field with nothing to undo hands over to the app's stack; an editor
// never does. A failed undo is said in words through the announcer. Every state from Go fires
// wk-undo-change on the document (detail: the state), for the app to show
// what came back.

import { announce } from './announce.js';

const undoWords = {
  undo: 'Undo',
  busy: 'An undo is already under way',
  couldNot: (op, name) => `Couldn’t ${op} ${name}`,
};

const undoEmpty = Object.freeze({ undo: null, redo: null, busy: false, textFocus: false });
let undoState = undoEmpty;
let undoConn = null;

// Editors that keep their own undo history: the kit's code, Markdown and
// rich-text editors (CodeMirror and ProseMirror), and any element the app
// marks with data-wk-undo.
const UNDO_HOSTS = '.cm-editor, .ProseMirror, .wk-code-editor, .wk-markdown-editor, .wk-richtext-editor, [data-wk-undo]';

// Where Edit > Undo goes first: a plain field (the browser's typing undo),
// an editor host (its own history; never the app's stack), or nowhere.
function undoTarget(el) {
  if (!el || el === document.body || el === document.documentElement) return null;
  if (el instanceof HTMLTextAreaElement) return !el.readOnly && !el.disabled ? { kind: 'field', el } : null;
  if (el instanceof HTMLInputElement) {
    return !el.readOnly && !el.disabled && /^(text|search|url|tel|email|password|number|)$/.test(el.type) ? { kind: 'field', el } : null;
  }
  if (el.isContentEditable) return { kind: 'host', el };
  const host = el.closest?.(UNDO_HOSTS);
  if (host) return { kind: 'host', el: host.querySelector('[contenteditable="true"]') || host };
  return null;
}
const undoTextField = el => Boolean(undoTarget(el));

// The error code of a rejected binding: Wails passes the Go error's JSON,
// on the error itself, its cause, or as its message.
function undoCode(err) {
  for (const v of [err, err?.cause]) {
    if (v && typeof v === 'object' && typeof v.code === 'string' && v.code.startsWith('undo_')) return v.code;
    if (typeof v === 'string' || typeof v?.message === 'string') {
      try { const parsed = JSON.parse(typeof v === 'string' ? v : v.message); if (parsed?.code) return parsed.code; } catch { /* not JSON */ }
    }
  }
  return '';
}

function undoTake(state) {
  undoState = Object.freeze({ ...undoEmpty, ...(state || {}) });
  document.dispatchEvent(new CustomEvent('wk-undo-change', { detail: undoState }));
}

// Calls a binding; a failure of the action is said assertively (the person
// pressed it and waits on it). Nothing to undo, a stale toast and a busy
// stack say nothing: there is no failure to report.
async function undoCall(name, op, args = [], what = '', pressed = false) {
  const fn = undoConn?.bindings?.[name];
  if (typeof fn !== 'function') return false;
  const label = what || (op === 'redo' ? undoState.redo?.name : undoState.undo?.name) || '';
  try {
    await fn(...args);
    return true;
  } catch (err) {
    const code = undoCode(err);
    // A toast's Undo pressed while another undo runs: say why nothing happened.
    if (code === 'undo_busy' && pressed) announce(undoWords.busy);
    else if (!['undo_empty', 'undo_stale', 'undo_busy'].includes(code)) announce(undoWords.couldNot(op, label).trim(), { assertive: true });
    return false;
  }
}

// Edit > Undo or Redo with text focus, in the focused window only. First
// the field or editor gets a cancelable beforeinput (historyUndo or
// historyRedo): an editor with its own history (CodeMirror, ProseMirror)
// takes it there. Otherwise a plain field undoes its typing with
// execCommand, and with none to undo, the app's stack goes. An editor host
// never falls back to the app's stack: its undo stays in the editor.
function undoText(op) {
  if (!document.hasFocus()) return; // another window's field
  const target = undoTarget(document.activeElement);
  if (target) {
    const ev = new InputEvent('beforeinput', { inputType: op === 'redo' ? 'historyRedo' : 'historyUndo', bubbles: true, cancelable: true });
    target.el.dispatchEvent(ev);
    if (ev.defaultPrevented) return;
    if (target.kind === 'host') { document.execCommand(op); return; }
    const before = target.el.value;
    document.execCommand(op);
    if (target.el.value !== before) return;
  }
  undoCall(op === 'redo' ? 'Redo' : 'Undo', op);
}

function undoConnect({ on, bindings = {} } = {}) {
  if (typeof on !== 'function') throw new TypeError('wkUndo.connect: on must be a function');
  undoConn?.disconnect();
  const conn = { bindings, seen: false, offs: [], focus: null };
  conn.offs.push(on('undo:changed', state => { conn.seen = true; undoTake(state); }));
  conn.offs.push(on('undo:text', p => undoText(p?.op === 'redo' ? 'redo' : 'undo')));
  // The Go side's Apply sends undo:text to one window's page as a DOM event.
  const onDomText = e => undoText(e.detail?.op === 'redo' ? 'redo' : 'undo');
  document.addEventListener('wk-undo-text', onDomText);
  conn.offs.push(on('undo:failed', p => announce(undoWords.couldNot(p?.op === 'redo' ? 'redo' : 'undo', p?.name || '').trim(), { assertive: true })));
  // Text focus, reported when it changes.
  // A window coming to the front reports again, so the menu follows the
  // window in front.
  const report = force => {
    const now = document.hasFocus() && undoTextField(document.activeElement);
    if (now === conn.focus && force !== true) return;
    conn.focus = now;
    Promise.resolve().then(() => bindings.SetTextFocus?.(now)).catch(() => {});
  };
  const later = e => queueMicrotask(() => report(e?.type === 'focus' && e.target === window));
  document.addEventListener('focusin', later, true);
  document.addEventListener('focusout', later, true);
  window.addEventListener('blur', later);
  window.addEventListener('focus', later);
  conn.disconnect = () => {
    for (const off of conn.offs) if (typeof off === 'function') off();
    document.removeEventListener('wk-undo-text', onDomText);
    document.removeEventListener('focusin', later, true);
    document.removeEventListener('focusout', later, true);
    window.removeEventListener('blur', later);
    window.removeEventListener('focus', later);
    if (undoConn !== conn) return;
    undoConn = null;
    undoTake(undoEmpty);
  };
  undoConn = conn;
  report();
  // A reloaded window asks for the stack as it is, unless an event has
  // already brought it.
  Promise.resolve().then(() => bindings.State?.()).then(s => { if (!conn.seen && undoConn === conn && s) undoTake(s); }, () => {});
  return conn.disconnect;
}

// A toast's Undo for the action with this ID (Push's result on the Go side),
// or else the most recent action. It undoes only that action: once the
// stack has moved past it, it does nothing and its toast closes.
function undoAction(id) {
  const undoId = id || undoState.undo?.id || null;
  const name = undoState.undo?.id === undoId ? undoState.undo.name : '';
  return {
    label: undoWords.undo,
    undoId,
    current: () => undoState.undo?.id ?? null,
    run: () => (undoId ? undoCall('UndoID', 'undo', [undoId], name, true) : Promise.resolve(false)),
  };
}

const wkUndo = {
  connect: undoConnect,
  state: () => undoState,
  undo: () => undoCall('Undo', 'undo'),
  redo: () => undoCall('Redo', 'redo'),
  action: undoAction,
  words: undoWords,
};

export { wkUndo };
