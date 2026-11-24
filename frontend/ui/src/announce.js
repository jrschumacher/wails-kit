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
  if (!host.isConnected) (document.body || document.documentElement).append(host);
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
}

export { announce };
