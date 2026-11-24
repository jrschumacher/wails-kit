// Skeleton rows: a table that loads shows still rows in the shape of its
// content (BEHAVIOUR.md, "Loading content" and "Building blocks › Skeleton
// rows"), after the indicator delay (400 ms), for at least the indicator
// minimum (500 ms) once shown.
//
//   const loading = wkSkeleton(tbody, { rows, columns, region });
//   const data = await fetchRuns();
//   await loading.done(() => drawRows(tbody, data)); // swapped in one step
//
// From the call, the region (the table, or `region`) has aria-busy="true"
// until done(). At 400 ms the body's rows give way to
// <tr class="wk-skeleton" aria-hidden="true">, one cell per column (the
// header's, or `columns`), each holding a still bar, at the real row height:
// as many rows as the body had, or as last time, or enough to fill the
// window below the body, up to 12 (or `rows`). done(draw) takes them out,
// calls draw in the same step and resolves; if they haven't shown yet, at
// once, with no indicator. cancel() takes them out and puts the old rows
// back.

import { timings } from './motion.js';

const SKELETON_MAX = 12;
const skeletonLast = new WeakMap(); // body -> its row count after the last load

function skeletonColumns(body) {
  const table = body.closest('table');
  const head = table?.tHead?.rows[0] || body.rows[0];
  if (!head) return 1;
  return [...head.cells].reduce((n, c) => n + (c.colSpan || 1), 0) || 1;
}

function skeletonRow(columns) {
  const tr = document.createElement('tr');
  tr.className = 'wk-skeleton';
  tr.setAttribute('aria-hidden', 'true');
  for (let i = 0; i < columns; i++) {
    const td = tr.insertCell();
    const bar = document.createElement('span');
    bar.className = 'wk-skeleton-bar';
    td.append(bar);
  }
  return tr;
}

function wkSkeleton(body, { rows, columns, region } = {}) {
  if (body instanceof HTMLTableElement) body = body.tBodies[0] || body.createTBody();
  if (!(body instanceof HTMLTableSectionElement)) throw new TypeError('wkSkeleton: pass a <tbody> or a <table>');
  const busy = region || body.closest('table') || body;
  busy.setAttribute('aria-busy', 'true');
  let shownAt = -1, kept = null, ended = false, added = [];

  const show = () => {
    const cols = columns || skeletonColumns(body);
    const first = skeletonRow(cols);
    kept = [...body.childNodes];
    body.replaceChildren(first);
    let n = rows || kept.filter(r => r.nodeName === 'TR').length || skeletonLast.get(body) || 0;
    if (!n) {
      const h = first.getBoundingClientRect().height || 1;
      const below = document.documentElement.clientHeight - first.getBoundingClientRect().top;
      n = Math.floor(below / h);
    }
    n = Math.max(1, Math.min(SKELETON_MAX, n));
    added = [first];
    for (let i = 1; i < n; i++) added.push(body.appendChild(skeletonRow(cols)));
    shownAt = Date.now();
  };
  const timer = setTimeout(show, timings.indicatorDelay);

  const finish = (draw, restore) => {
    if (ended) return Promise.resolve();
    ended = true;
    clearTimeout(timer);
    const swap = () => {
      for (const r of added) r.remove();
      if (restore && kept) body.append(...kept);
      busy.removeAttribute('aria-busy');
      if (typeof draw === 'function') draw();
      if (!restore) queueMicrotask(() => { if (body.rows.length) skeletonLast.set(body, body.rows.length); });
    };
    const wait = shownAt < 0 ? 0 : shownAt + timings.indicatorMin - Date.now();
    if (wait <= 0) { swap(); return Promise.resolve(); }
    return new Promise(resolve => setTimeout(() => { swap(); resolve(); }, wait));
  };

  return {
    done: draw => finish(draw, false),
    cancel: () => finish(null, true),
    get shown() { return shownAt >= 0 && !ended; },
  };
}

export { wkSkeleton };
