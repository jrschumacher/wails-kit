// wkToast: a brief outcome with nowhere inline to show it (BEHAVIOUR.md,
// "Notifications inside the window" and "Building blocks › Toast").
//
//   wkToast({ words: 'Conversation archived', action: { label: 'Undo', run } })
//     -> { element, dismiss() }
//
// One at a time, at the bottom centre of the window, above any footer bar
// (wk-footer-bar). It shows for timings.toast (6 s), or
// timings.toastWithAction (10 s) with an action, and the clock runs only
// while the window is focused and the pointer and focus are outside it. A
// newer toast replaces it at once. It isn't a live region: its words go to
// the window's polite announcer. F6 moves focus to its action and back to
// where it was; Esc dismisses it while focus is inside; focus goes back
// when it closes. An action must also exist elsewhere (Undo in Edit > Undo,
// Show in the list), because the toast goes away by itself.
//
// An action from wkUndo.action() (it carries undoId) closes the toast once
// the app's undo stack moves past it, from Edit > Undo or anywhere else.

import { timings, motion, reducedMotion } from './motion.js';
import { announce } from './announce.js';

let toastShown = null;   // the toast on screen
let toastSeq = 0;
// Whether the window has focus: from the document at load, then window
// focus and blur (document.hasFocus() can't be watched).
let toastWindowFocused = typeof document !== 'undefined' && typeof document.hasFocus === 'function' ? document.hasFocus() : true;
let toastListening = false;

// The open modal on top, where the toast must sit to be reachable; else
// the body.
function toastHome() {
  // The announcer keeps the modals in the order they opened; else the last
  // open one in the document.
  const stack = (document.wkAnnouncer?.wkModals || []).filter(d => d.isConnected && d.matches(':modal'));
  return stack.pop() || [...document.querySelectorAll('dialog')].filter(d => d.matches(':modal')).pop() || document.body;
}

// Bottom offset: above the footer bars that sit along the bottom of the
// window, in the toast's home.
function toastPlace(t) {
  const vh = document.documentElement.clientHeight;
  let top = vh;
  for (const bar of t.home.querySelectorAll('.wk-footer-bar')) {
    const r = bar.getBoundingClientRect();
    if (r.height > 0 && r.bottom >= vh - 1 && r.top < top) top = r.top;
  }
  t.el.style.setProperty('--wk-toast-above', `${Math.max(0, Math.round(vh - top))}px`);
}

function toastListen() {
  if (toastListening) return;
  toastListening = true;
  window.addEventListener('focus', () => { toastWindowFocused = true; toastShown?.tick(); });
  window.addEventListener('blur', () => { toastWindowFocused = false; toastShown?.tick(); });
  window.addEventListener('resize', () => { if (toastShown) toastPlace(toastShown); });
  // F6 (and Shift+F6) moves between the toast's action and where focus was.
  window.addEventListener('keydown', e => {
    if (e.key !== 'F6' || e.defaultPrevented || e.ctrlKey || e.altKey || e.metaKey) return;
    const t = toastShown;
    if (!t || !t.actionEl) return;
    toastRehome(t);
    const was = document.activeElement;
    if (t.el.contains(was)) toastReturn(t);
    else { t.actionEl.focus(); if (document.activeElement === t.actionEl) t.back = was; }
    // Only a key that moved focus is used; otherwise F6 goes on to the page.
    if (document.activeElement !== was) e.preventDefault();
  });
  // A modal opening or closing makes the toast's place inert or takes it
  // away: it follows the top-most modal, as the announcer does.
  const onDialog = () => { if (toastShown) toastRehome(toastShown); };
  document.addEventListener('toggle', onDialog, true);
  document.addEventListener('close', onDialog, true);
  new MutationObserver(onDialog).observe(document.documentElement, { subtree: true, childList: true, attributes: true, attributeFilter: ['open'] });
  document.addEventListener('wk-undo-change', e => {
    const t = toastShown;
    const id = t?.undoId;
    if (!id) return;
    const top = e.detail?.undo?.id ?? null;
    if (top === id) t.seen = true;
    else if (t.seen) t.dismiss();
  });
}

// Moves the toast into the top-most open modal, or back to the body, keeping
// focus on its action if it had it.
function toastRehome(t) {
  if (t.gone) return;
  const home = toastHome();
  if (t.el.parentNode === home && t.el.isConnected) return;
  const had = t.el.contains(document.activeElement) ? document.activeElement : null;
  t.home = home;
  home.append(t.el);
  toastPlace(t);
  if (had && had.isConnected) had.focus({ preventScroll: true });
}

// Puts focus back where it was before it came to the toast.
function toastReturn(t) {
  const back = t.back;
  t.back = null;
  if (back && back !== document.body && back.isConnected && typeof back.focus === 'function') back.focus({ preventScroll: true });
  else if (t.el.contains(document.activeElement)) document.activeElement.blur();
}

function wkToast({ words, action } = {}) {
  const text = String(words ?? '').replace(/\s+/g, ' ').trim();
  if (!text) throw new TypeError('wkToast: words are required');
  if (action && (typeof action.run !== 'function' || !action.label)) throw new TypeError('wkToast: an action needs a label and run()');
  toastListen();

  const prev = toastShown;
  const hadFocus = Boolean(prev && prev.el.contains(document.activeElement));
  const back = prev?.back ?? null;
  if (prev) prev.remove();

  const el = document.createElement('div');
  el.className = 'wk-toast';
  el.setAttribute('role', 'group');
  el.dataset.wkToast = '';
  const said = document.createElement('span');
  said.className = 'wk-toast-words';
  said.id = `wk-toast-words-${++toastSeq}`;
  said.textContent = text;
  el.setAttribute('aria-labelledby', said.id);
  el.append(said);

  const t = {
    el, home: toastHome(), back, undoId: action?.undoId ?? null, seen: false,
    remaining: action ? timings.toastWithAction : timings.toast,
    timer: 0, startedAt: 0, hovered: false, focused: false, gone: false, actionEl: null,
  };
  if (t.undoId) t.seen = action.current?.() === t.undoId;

  if (action) {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'wk-button wk-button--small wk-toast-action';
    b.textContent = action.label;
    b.addEventListener('click', () => {
      if (t.gone) return;
      t.dismiss();
      action.run();
    });
    el.append(b);
    t.actionEl = b;
  }

  // The clock runs only while the window is focused and the pointer and
  // focus are outside the toast.
  t.tick = () => {
    if (t.gone) return;
    const run = toastWindowFocused && !t.hovered && !t.focused;
    if (run && !t.timer) {
      t.startedAt = Date.now();
      t.timer = setTimeout(() => { t.timer = 0; t.remaining = 0; t.dismiss(); }, t.remaining);
    } else if (!run && t.timer) {
      clearTimeout(t.timer);
      t.timer = 0;
      t.remaining = Math.max(0, t.remaining - (Date.now() - t.startedAt));
    }
  };
  el.addEventListener('pointerenter', () => { t.hovered = true; t.tick(); });
  el.addEventListener('pointerleave', () => { t.hovered = false; t.tick(); });
  el.addEventListener('focusin', e => {
    // Reached with Tab (or a click): focus goes back where it came from.
    if (!t.focused && e.relatedTarget && !el.contains(e.relatedTarget) && !t.back) t.back = e.relatedTarget;
    t.focused = true;
    t.tick();
  });
  el.addEventListener('focusout', e => {
    if (el.contains(e.relatedTarget)) return;
    t.focused = false;
    t.tick();
  });
  el.addEventListener('keydown', e => {
    if (e.key !== 'Escape' || e.defaultPrevented) return;
    e.preventDefault();
    e.stopPropagation();
    t.dismiss();
  });

  // Taken off the screen at once (a newer toast), with no exit.
  t.remove = () => {
    if (t.gone) return;
    t.gone = true;
    clearTimeout(t.timer);
    if (toastShown === t) toastShown = null;
    el.remove();
  };
  // Dismissed: focus goes back first, then it leaves, in about two-thirds of
  // the time it took to enter (a fade of at most 100 ms with reduced motion).
  t.dismiss = () => {
    if (t.gone) return;
    const inside = el.contains(document.activeElement);
    t.gone = true;
    clearTimeout(t.timer);
    if (toastShown === t) toastShown = null;
    if (inside) toastReturn(t);
    el.inert = true;
    el.dataset.leaving = '';
    const leave = reducedMotion() ? motion.durationFast : Math.round(motion.durationBase * 2 / 3);
    setTimeout(() => el.remove(), leave);
  };

  t.home.append(el);
  toastPlace(t);
  toastShown = t;
  // Focus was in the toast it replaced: the new action takes it, or it goes
  // back where it was.
  if (hadFocus) {
    if (t.actionEl) t.actionEl.focus({ preventScroll: true });
    else toastReturn(t);
  }
  announce(text);
  t.tick();
  return { element: el, dismiss: () => t.dismiss() };
}

export { wkToast };
