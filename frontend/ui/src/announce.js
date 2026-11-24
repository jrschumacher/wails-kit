// The window's announcer (BEHAVIOUR.md, "Announcer"): one polite and one
// assertive live region, visually hidden, in the page from load, because a
// region added together with its text often goes unread.
//
//   announce(words, { assertive = false })
//
// Messages within 500 ms of each other are joined into one ("Link copied.
// Comment posted"), so a screen reader reads them once, in order, instead
// of cutting one off with the next. A message goes out 500 ms after the
// last one, and never more than 2 s after the first. Words the region
// already holds are announced again: it is cleared and set on the next
// frame. Each window (each document) has one announcer, shared by every copy
// of the kit in it (src/ and dist/). Nothing that ticks (a countdown, a
// progress value, streamed output) belongs here.
//
// A modal <dialog> makes everything outside it inert, live regions too, so
// while one is open the regions live inside the top-most modal, and go back
// to <body> when it closes. They move when a dialog opens or closes, not
// while words wait to be said.

const JOIN_WINDOW = 500;
const JOIN_MAX = 2_000;
const ANNOUNCER = 'data-wk-announcer';

const regions = { polite: null, assertive: null };
// The messages waiting to go out live on the regions' element, so every
// copy of the kit in the window joins into the same ones.
const queuesOf = () => {
  const host = regions.polite?.parentNode;
  if (!host) return {};
  return (host.wkQueues ??= { polite: null, assertive: null });
};

// The open modal dialogs, in the order they opened (shared on the regions'
// element by every copy of the kit); the last one still open is on top.
function topModal(host) {
  const stack = (host.wkModals ??= []);
  for (let i = stack.length - 1; i >= 0; i--) {
    if (stack[i].isConnected && stack[i].matches(':modal')) return stack[i];
    stack.splice(i, 1);
  }
  // A modal opened without an event the kit saw: the last in the document.
  return [...document.querySelectorAll('dialog')].filter(d => d.matches(':modal')).pop() || null;
}

// Puts the regions where a screen reader can hear them: called when a dialog
// opens (open true) or closes. wk-dialog calls it; the toggle and close
// events catch a plain <dialog>.
function placeAnnouncer(dialog, open) {
  if (typeof document === 'undefined') return;
  const host = document.querySelector(`[${ANNOUNCER}]`);
  if (!host) return;
  const stack = (host.wkModals ??= []);
  if (dialog) {
    const at = stack.indexOf(dialog);
    if (at >= 0) stack.splice(at, 1);
    if (open && dialog.matches(':modal')) stack.push(dialog);
  }
  region('polite');
}

// The two regions, made once per document and found again if another copy
// of the kit made them first. A region the app removed (a framework that
// owns <body>) is put back.
function region(kind) {
  if (typeof document === 'undefined') return null;
  let host = document.querySelector(`[${ANNOUNCER}]`);
  if (!host) {
    host = document.createElement('div');
    host.setAttribute(ANNOUNCER, '');
    host.className = 'wk-announcer wk-visually-hidden';
    for (const live of ['polite', 'assertive']) {
      const r = document.createElement('div');
      r.setAttribute('aria-live', live);
      r.setAttribute('aria-atomic', 'true');
      r.dataset.wkLive = live;
      host.append(r);
    }
  }
  const home = topModal(host) || document.body || document.documentElement;
  if (host.parentNode !== home) home.append(host);
  regions.polite = host.querySelector('[data-wk-live="polite"]');
  regions.assertive = host.querySelector('[data-wk-live="assertive"]');
  return regions[kind];
}

const join = list => list.reduce((text, w) => (text ? `${text}${/[.!?…:]$/.test(text) ? ' ' : '. '}${w}` : w), '');

function flush(kind) {
  const queues = queuesOf();
  const q = queues[kind];
  queues[kind] = null;
  if (!q) return;
  clearTimeout(q.timer);
  const r = region(kind);
  if (!r) return;
  const text = join(q.words);
  if (r.textContent !== text) { r.textContent = text; return; }
  // The same words again: clear, then set them on the next frame, so the
  // change is heard. A hidden window may not run frames, so a timer backs
  // the frame up; whichever comes first sets them.
  r.textContent = '';
  let done = false;
  const set = () => { if (!done) { done = true; r.textContent = text; } };
  if (typeof requestAnimationFrame === 'function') requestAnimationFrame(set);
  setTimeout(set, 100);
}

function announce(words, { assertive = false } = {}) {
  const text = String(words ?? '').replace(/\s+/g, ' ').trim();
  if (!text || typeof document === 'undefined') return;
  const kind = assertive ? 'assertive' : 'polite';
  region(kind);
  const queues = queuesOf();
  const now = Date.now();
  let q = queues[kind];
  if (!q) q = queues[kind] = { words: [], first: now, timer: 0 };
  if (q.words[q.words.length - 1] !== text) q.words.push(text);
  clearTimeout(q.timer);
  q.timer = setTimeout(() => flush(kind), Math.max(0, Math.min(JOIN_WINDOW, q.first + JOIN_MAX - now)));
}

// In the page from load: now if the body exists, else as soon as it does.
if (typeof document !== 'undefined') {
  if (document.body) region('polite');
  else document.addEventListener('DOMContentLoaded', () => region('polite'), { once: true });
  const onDialog = e => {
    if (e.target instanceof HTMLDialogElement) placeAnnouncer(e.target, e.type === 'toggle' ? e.newState === 'open' : false);
  };
  document.addEventListener('toggle', onDialog, true);
  document.addEventListener('close', onDialog, true);
  // toggle and close are queued as tasks of their own, so something else can
  // run while the regions still sit in a dialog that has closed (and hides
  // them). A dialog's open attribute changes with it, and an observer hears
  // that in the same task.
  new MutationObserver(records => {
    for (const r of records) if (r.target instanceof HTMLDialogElement) placeAnnouncer(r.target, r.target.open);
  }).observe(document.documentElement, { subtree: true, attributes: true, attributeFilter: ['open'] });
}

export { announce, placeAnnouncer };
