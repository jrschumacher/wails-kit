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
// Connected, it reports text focus to Go, so Edit > Undo and Redo undo
// typing first: Go sends undo:text, and the focused window's field undoes
// its typing; when it has none, the app's stack goes instead. A failed undo
// is said in words through the announcer. Every state from Go fires
// wk-undo-change on the document (detail: the state), for the app to show
// what came back.

import { announce } from './announce.js';

const undoWords = {
  undo: 'Undo',
  couldNot: (op, name) => `Couldn’t ${op} ${name}`,
};

const undoEmpty = Object.freeze({ undo: null, redo: null, busy: false, textFocus: false });
let undoState = undoEmpty;
let undoConn = null;

// A field whose typing the browser can undo.
function undoTextField(el) {
  if (!el || el === document.body) return false;
  if (el.isContentEditable) return true;
  if (el instanceof HTMLTextAreaElement) return !el.readOnly && !el.disabled;
  if (el instanceof HTMLInputElement) {
    return !el.readOnly && !el.disabled && /^(text|search|url|tel|email|password|number|)$/.test(el.type);
  }
  return false;
}

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
async function undoCall(name, op, args = [], what = '') {
  const fn = undoConn?.bindings?.[name];
  if (typeof fn !== 'function') return false;
  const label = what || (op === 'redo' ? undoState.redo?.name : undoState.undo?.name) || '';
  try {
    await fn(...args);
    return true;
  } catch (err) {
    const code = undoCode(err);
    if (!['undo_empty', 'undo_stale', 'undo_busy'].includes(code)) announce(undoWords.couldNot(op, label).trim(), { assertive: true });
    return false;
  }
}

// Edit > Undo or Redo with text focus: the field undoes its typing; with
// none to undo, the app's stack goes.
function undoText(op) {
  if (!document.hasFocus()) return; // another window's field
  const el = document.activeElement;
  if (undoTextField(el)) {
    const before = el.isContentEditable ? el.innerHTML : el.value;
    document.execCommand(op);
    const after = el.isContentEditable ? el.innerHTML : el.value;
    if (after !== before) return;
  }
  undoCall(op === 'redo' ? 'Redo' : 'Undo', op);
}

function undoConnect({ on, bindings = {} } = {}) {
  if (typeof on !== 'function') throw new TypeError('wkUndo.connect: on must be a function');
  undoConn?.disconnect();
  const conn = { bindings, seen: false, offs: [], focus: null };
  conn.offs.push(on('undo:changed', state => { conn.seen = true; undoTake(state); }));
  conn.offs.push(on('undo:text', p => undoText(p?.op === 'redo' ? 'redo' : 'undo')));
  conn.offs.push(on('undo:failed', p => announce(undoWords.couldNot(p?.op === 'redo' ? 'redo' : 'undo', p?.name || '').trim(), { assertive: true })));
  // Text focus, reported when it changes.
  const report = () => {
    const now = document.hasFocus() && undoTextField(document.activeElement);
    if (now === conn.focus) return;
    conn.focus = now;
    Promise.resolve().then(() => bindings.SetTextFocus?.(now)).catch(() => {});
  };
  const later = () => queueMicrotask(report);
  document.addEventListener('focusin', later, true);
  document.addEventListener('focusout', later, true);
  window.addEventListener('blur', later);
  window.addEventListener('focus', later);
  conn.disconnect = () => {
    for (const off of conn.offs) if (typeof off === 'function') off();
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
    run: () => (undoId ? undoCall('UndoID', 'undo', [undoId], name) : Promise.resolve(false)),
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
