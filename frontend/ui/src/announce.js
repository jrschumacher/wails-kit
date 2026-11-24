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
// to <body> when it closes (or is removed). They move when a dialog opens or
// closes, not while words wait to be said.

const JOIN_WINDOW = 500;
const JOIN_MAX = 2_000;
const ANNOUNCER = 'data-wk-announcer';

// The regions' element: one per document, kept on the document so every
// copy of the kit (and a re-home after the app removed it) uses the same
// element, with the words it was about to say.
function hostOf() {
  let host = document.wkAnnouncer || document.querySelector(`[${ANNOUNCER}]`);
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
  document.wkAnnouncer = host;
  return host;
}
const regionOf = (host, kind) => host.querySelector(`[data-wk-live="${kind}"]`);
// The messages waiting to go out live on the regions' element, so every
// copy of the kit in the window joins into the same ones.
const queuesOf = host => (host.wkQueues ??= { polite: null, assertive: null });

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

// Sets a region's words on the next frame, after clearing it, so the change
// is heard: for the same words again, and for a region just put back in the
// page. A hidden window may not run frames, so a timer backs the frame up;
// whichever comes first sets them, unless newer words came in between.
function setNextFrame(r, text) {
  r.textContent = '';
  const token = (r.wkToken = {});
  const set = () => { if (r.wkToken === token) { r.wkToken = null; r.textContent = text; } };
  if (typeof requestAnimationFrame === 'function') requestAnimationFrame(set);
  setTimeout(set, 100);
}

// Puts the element where a screen reader can hear it: in the top-most open
// modal, else in <body>. Moving with a dialog opening or closing clears the
// regions first, so the last words aren't read again where they land. If
// the element left the page (removed with a modal, or by a framework that
// owns <body>), it goes back at once, and words said in the last 2 s are set
// again on the next frame, since they may not have been heard.
function settle() {
  const host = hostOf();
  const home = topModal(host) || document.body || document.documentElement;
  if (host.parentNode === home) return host;
  const lost = !host.isConnected;
  for (const kind of ['polite', 'assertive']) {
    const r = regionOf(host, kind);
    const text = r.wkToken ? null : r.textContent;
    r.wkToken = null;
    r.textContent = '';
    if (lost && text && Date.now() - (r.wkSaidAt || 0) < JOIN_MAX) r.wkAgain = text;
  }
  home.append(host);
  // Put back after leaving the page: the next words wait a frame. (Its first
  // placement, at load, is long before anything is said.)
  host.wkFresh = lost && Boolean(host.wkPlaced);
  host.wkPlaced = true;
  for (const kind of ['polite', 'assertive']) {
    const r = regionOf(host, kind);
    if (r.wkAgain) { setNextFrame(r, r.wkAgain); r.wkAgain = null; host.wkFresh = false; }
  }
  return host;
}

// Called when a dialog opens (open true) or closes. wk-dialog calls it; an
// observer on every dialog's open attribute catches a plain <dialog>.
function placeAnnouncer(dialog, open) {
  if (typeof document === 'undefined') return;
  const host = hostOf();
  const stack = (host.wkModals ??= []);
  if (dialog) {
    const at = stack.indexOf(dialog);
    if (at >= 0) stack.splice(at, 1);
    if (open && dialog.matches(':modal')) stack.push(dialog);
  }
  settle();
}

const join = list => list.reduce((text, w) => (text ? `${text}${/[.!?…:]$/.test(text) ? ' ' : '. '}${w}` : w), '');

function flush(kind) {
  const host = settle();
  const queues = queuesOf(host);
  const q = queues[kind];
  queues[kind] = null;
  if (!q) return;
  clearTimeout(q.timer);
  const r = regionOf(host, kind);
  const text = join(q.words);
  r.wkSaidAt = Date.now();
  // Just put back in the page, or the same words again: clear, then set
  // them on the next frame.
  if (host.wkFresh || r.textContent === text) { host.wkFresh = false; setNextFrame(r, text); return; }
  r.wkToken = null;
  r.textContent = text;
}

function announce(words, { assertive = false } = {}) {
  const text = String(words ?? '').replace(/\s+/g, ' ').trim();
  if (!text || typeof document === 'undefined') return;
  const kind = assertive ? 'assertive' : 'polite';
  const queues = queuesOf(settle());
  const now = Date.now();
  let q = queues[kind];
  if (!q) q = queues[kind] = { words: [], first: now, timer: 0 };
  if (q.words[q.words.length - 1] !== text) q.words.push(text);
  clearTimeout(q.timer);
  q.timer = setTimeout(() => flush(kind), Math.max(0, Math.min(JOIN_WINDOW, q.first + JOIN_MAX - now)));
}

// In the page from load: now if the body exists, else as soon as it does.
if (typeof document !== 'undefined') {
  if (document.body) settle();
  else document.addEventListener('DOMContentLoaded', () => settle(), { once: true });
  const onDialog = e => {
    if (e.target instanceof HTMLDialogElement) placeAnnouncer(e.target, e.type === 'toggle' ? e.newState === 'open' : false);
  };
  document.addEventListener('toggle', onDialog, true);
  document.addEventListener('close', onDialog, true);
  // toggle and close are queued as tasks of their own, so something else can
  // run while the regions still sit in a dialog that has closed (and hides
  // them). A dialog's open attribute changes with it, and an observer hears
  // that in the same task; it also hears the element leave the page with a
  // modal that was removed, and puts it back at once.
  new MutationObserver(records => {
    for (const r of records) if (r.type === 'attributes' && r.target instanceof HTMLDialogElement) placeAnnouncer(r.target, r.target.open);
    if (document.wkAnnouncer && !document.wkAnnouncer.isConnected) settle();
  }).observe(document.documentElement, { subtree: true, childList: true, attributes: true, attributeFilter: ['open'] });
}

export { announce, placeAnnouncer };
