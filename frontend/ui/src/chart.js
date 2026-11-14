// wk-chart: bars, stacked bars, and a line with a band, drawn as
// plain SVG with no dependencies. Every chart carries a legend, a 2 px gap between
// touching marks, hover and keyboard readout, and a table view that shows the same
// numbers as a kit table. Colours come from the kit's tokens through data-tone
// ("1" to "5" are --wk-color-chart-N, "neutral" is a secondary ink), so a chart
// follows light and dark with the page.
//
// The layout rules are plain functions with no DOM (niceTicks, labelStep, runs,
// roundedTop, roundedEnd, tableModel, readout), so node can test them
// (test/chart.test.mjs). wkChart builds the figure.
//
//   wkChart({
//     id: 'jobs',                       // unique on the page; names the controls
//     kind: 'bars' | 'line',
//     label: 'Jobs per day',            // the chart's title and accessible name
//     summary: '14 days, 134 jobs',     // optional: read after the title
//     caption: 'UTC',                   // optional: small text beside the title
//     categories: [{ label: 'Sat 8', name: 'Saturday 8 Nov', shaded: true }],
//     series: [{ key, label, tone: 1, values: [3, 0, null] }],  // null is "no data"
//     band: { key, label, tone: 1, values: [...] },  // line only: from the line up to these
//     stacked: false, orientation: 'vertical' | 'horizontal',   // bars only
//     format: v => String(v), ticks: max => [0, 5, 10], integer: false,
//     legendValues: false,              // show each series' total in the legend
//     shadeLabel: 'Weekend',            // names the shaded categories in the legend
//     categoryHeader: 'Day',            // the table's first column
//     view: 'chart' | 'table', onview: view => {},
//     selection: -1, onselection: index => {}, // category index, or -1 for none
//     retainSelectionOnBlur: false,    // keep the readout when focus leaves the plot
//   }) -> <figure class="wk-chart">
const SVG = 'http://www.w3.org/2000/svg';
const GAP = 2;          // the surface between touching marks
const BAR_MAX = 24;     // a bar is never thicker than this
const RADIUS = 4;       // the data end of a bar
const CHAR = 6.4;       // an estimate of a label's width per character at 11 px
const HEIGHT = 220;

// ---- Rules with no DOM ---------------------------------------------------

/** Round tick values from 0 up to at least `max`, about `count` of them. `integer` keeps whole steps. */
export function niceTicks(max, count = 4, integer = false) {
  if (!(max > 0)) return [0, 1];
  const raw = max / count, mag = 10 ** Math.floor(Math.log10(raw));
  let step = [1, 2, 5, 10].map(m => m * mag).find(s => s >= raw);
  if (integer) step = Math.max(1, Math.ceil(step));
  const n = Math.ceil(max / step - 1e-9);
  return Array.from({ length: n + 1 }, (_, i) => Math.round(i * step * 1e9) / 1e9);
}

/** Show every k-th label so labels `width` px wide fit in bands `band` px apart. */
export function labelStep(band, width) { return Math.max(1, Math.ceil((width + 6) / Math.max(band, 1))); }

/** The runs of consecutive non-null values: [[index, value], ...] for each, so a gap breaks the line. */
export function runs(values) {
  const out = [];
  let run = null;
  values.forEach((v, i) => {
    if (v == null || Number.isNaN(v)) { run = null; return; }
    if (!run) out.push(run = []);
    run.push([i, v]);
  });
  return out;
}

const n2 = v => Math.round(v * 100) / 100;
// A box with its top corners rounded (a column's data end); r 0 is a plain box.
export function roundedTop(x, y, w, h, r = RADIUS) {
  r = Math.min(r, w / 2, h);
  return `M${n2(x)} ${n2(y + h)}V${n2(y + r)}${r ? `a${r} ${r} 0 0 1 ${r} ${-r}H${n2(x + w - r)}a${r} ${r} 0 0 1 ${r} ${r}` : `H${n2(x + w)}`}V${n2(y + h)}z`;
}
// A box with its right corners rounded (a row's data end).
export function roundedEnd(x, y, w, h, r = RADIUS) {
  r = Math.min(r, h / 2, w);
  return `M${n2(x)} ${n2(y)}H${n2(x + w - r)}${r ? `a${r} ${r} 0 0 1 ${r} ${r}V${n2(y + h - r)}a${r} ${r} 0 0 1 ${-r} ${r}` : `V${n2(y + h)}`}H${n2(x)}z`;
}

const stackedTotal = spec => spec.stacked && spec.series.length > 1;
const categoryName = (spec, c) => c.name || c.label || spec.label;

/** The table view's content: the same numbers as the chart, as header and rows of text. */
export function tableModel(spec) {
  const fmt = spec.tableFormat || spec.format || String;
  const cols = spec.series.map(s => ({ label: s.label, values: s.values }));
  if (spec.band) cols.push({ label: spec.band.label, values: spec.band.values });
  const cell = v => v == null ? '–' : fmt(v);
  const head = [spec.categoryHeader || '', ...cols.map(c => c.label)];
  if (stackedTotal(spec)) head.push('Total');
  const rows = spec.categories.map((c, i) => {
    const row = [categoryName(spec, c), ...cols.map(col => cell(col.values[i]))];
    if (stackedTotal(spec)) row.push(fmt(spec.series.reduce((sum, s) => sum + (s.values[i] || 0), 0)));
    return row;
  });
  return { head, rows };
}

/** What the tooltip and the live readout say for category i: a title and one row per series. */
export function readout(spec, i) {
  const fmt = spec.format || String;
  const c = spec.categories[i];
  const rows = [];
  for (const s of spec.series) if (s.values[i] != null && !(spec.stacked && !(s.values[i] > 0))) rows.push({ tone: s.tone, shape: spec.kind === 'line' ? 'line' : 'bar', label: s.label, text: fmt(s.values[i]) });
  if (spec.band && spec.band.values[i] != null) rows.push({ tone: spec.band.tone, shape: 'band', label: spec.band.label, text: fmt(spec.band.values[i]) });
  const title = categoryName(spec, c) + (c.shaded && spec.shadeLabel ? ` (${spec.shadeLabel.toLowerCase()})` : '');
  const text = rows.length ? rows.map(r => `${r.label} ${r.text}`).join(', ') : 'No data';
  return { title, rows, text: `${title}: ${text}` };
}

// ---- Drawing -------------------------------------------------------------
let sequence = 0;
function svgEl(tag, attrs, parent) {
  const el = document.createElementNS(SVG, tag);
  for (const [name, value] of Object.entries(attrs || {})) if (value != null && value !== false) el.setAttribute(name, value);
  parent?.append(el);
  return el;
}
function htmlEl(tag, cls, text, parent) {
  const el = document.createElement(tag);
  if (cls) el.className = cls;
  if (text != null) el.textContent = text;
  parent?.append(el);
  return el;
}
const sum = list => list.reduce((a, b) => a + (b || 0), 0);
const clip = (text, chars) => text.length > chars ? `${text.slice(0, Math.max(chars - 1, 1))}…` : text;

// Each draw returns the <svg> and how to place the tooltip for a category.
function drawColumns(spec, width) {
  const fmt = spec.format || String, n = spec.categories.length, line = spec.kind === 'line';
  const totals = spec.categories.map((_, i) => {
    const vals = spec.series.map(s => s.values[i] || 0);
    return spec.stacked ? sum(vals) : Math.max(0, ...vals, spec.band?.values[i] || 0);
  });
  const ticks = (spec.ticks || (max => niceTicks(max, 4, spec.integer)))(Math.max(0, ...totals));
  const top = ticks.at(-1) || 1;
  const m = { l: Math.max(...ticks.map(t => fmt(t).length)) * CHAR + 14, r: 8, t: 8, b: 24 };
  const pw = Math.max(width - m.l - m.r, 10), ph = HEIGHT - m.t - m.b, band = pw / Math.max(n, 1);
  const y = v => m.t + ph - (v / top) * ph;
  const svg = svgEl('svg', { width, height: HEIGHT, viewBox: `0 0 ${width} ${HEIGHT}`, 'aria-hidden': 'true', focusable: 'false' });
  spec.categories.forEach((c, i) => { if (c.shaded) svgEl('rect', { class: 'wk-chart-shade', x: n2(m.l + i * band), y: m.t, width: n2(band), height: ph }, svg); });
  const active = svgEl('rect', { class: 'wk-chart-active', y: m.t, width: n2(band), height: ph, hidden: '' }, svg);
  for (const t of ticks) {
    svgEl('line', { class: t === 0 ? 'wk-chart-axis' : 'wk-chart-grid', x1: m.l, x2: m.l + pw, y1: n2(y(t)), y2: n2(y(t)) }, svg);
    svgEl('text', { class: 'wk-chart-tick', x: m.l - 8, y: n2(y(t)), 'text-anchor': 'end', 'dominant-baseline': 'central' }, svg).textContent = fmt(t);
  }
  const step = labelStep(band, Math.max(0, ...spec.categories.map(c => c.label.length)) * CHAR);
  spec.categories.forEach((c, i) => {
    if (i % step === 0) svgEl('text', { class: 'wk-chart-tick', x: n2(m.l + (i + .5) * band), y: HEIGHT - 8, 'text-anchor': 'middle' }, svg).textContent = c.label;
  });
  const dots = [];
  if (!line) {
    const k = spec.stacked ? 1 : spec.series.length;
    const bw = Math.max(2, Math.min(BAR_MAX, (band * (spec.stacked ? 1 : .8) - GAP * (spec.stacked ? 2 : k + 1)) / k));
    spec.categories.forEach((_, i) => {
      if (spec.stacked) {
        let acc = 0;
        const segments = [];
        for (const s of spec.series) { const v = s.values[i]; if (v > 0) { segments.push({ s, from: y(acc), to: y(acc + v) }); acc += v; } }
        segments.forEach((g, j) => {
          const x = m.l + i * band + (band - bw) / 2;
          const bottom = g.from - (j > 0 ? GAP : 0), h = Math.max(bottom - g.to, 1.5);
          svgEl('path', { class: 'wk-chart-bar', 'data-tone': g.s.tone, d: roundedTop(x, bottom - h, bw, h, j === segments.length - 1 ? RADIUS : 0) }, svg);
        });
      } else {
        const group = k * bw + (k - 1) * GAP, x0 = m.l + i * band + (band - group) / 2;
        spec.series.forEach((s, j) => {
          const v = s.values[i];
          if (!(v > 0)) return;
          const h = Math.max(y(0) - y(v), 1.5);
          svgEl('path', { class: 'wk-chart-bar', 'data-tone': s.tone, d: roundedTop(x0 + j * (bw + GAP), y(0) - h, bw, h) }, svg);
        });
      }
    });
  } else {
    const x = i => m.l + (i + .5) * band;
    if (spec.band) {
      for (const run of runs(spec.band.values.map((v, i) => v != null && spec.series[0].values[i] != null ? v : null))) {
        if (run.length === 1) {
          const [i, v] = run[0], xx = n2(x(i)), yy = n2(y(v));
          // A one-point band has no area. Show its range and cap the upper value.
          svgEl('path', { class: 'wk-chart-line wk-chart-band-single', 'data-tone': spec.band.tone,
            d: `M${xx} ${n2(y(spec.series[0].values[i]))}V${yy}M${n2(xx - 8)} ${yy}H${n2(xx + 8)}` }, svg);
          continue;
        }
        const up = run.map(([i, v]) => `${n2(x(i))} ${n2(y(v))}`);
        const down = run.map(([i]) => `${n2(x(i))} ${n2(y(spec.series[0].values[i]))}`).reverse();
        svgEl('path', { class: 'wk-chart-band', 'data-tone': spec.band.tone, d: `M${up.join('L')}L${down.join('L')}z` }, svg);
      }
    }
    spec.series.forEach(s => {
      for (const run of runs(s.values)) {
        if (run.length > 1) svgEl('path', { class: 'wk-chart-line', 'data-tone': s.tone, d: `M${run.map(([i, v]) => `${n2(x(i))} ${n2(y(v))}`).join('L')}` }, svg);
        else svgEl('circle', { class: 'wk-chart-dot', 'data-tone': s.tone, cx: n2(x(run[0][0])), cy: n2(y(run[0][1])), r: 4 }, svg);
      }
      dots.push(svgEl('circle', { class: 'wk-chart-dot', 'data-tone': s.tone, r: 4, hidden: '' }, svg));
    });
  }
  return {
    svg,
    index: (px, py) => Math.min(n - 1, Math.max(0, Math.floor((px - m.l) / band))),
    show(i) {
      active.removeAttribute('hidden'); active.setAttribute('x', n2(m.l + i * band));
      dots.forEach((d, j) => {
        const v = spec.series[j].values[i];
        d.toggleAttribute('hidden', v == null);
        if (v != null) { d.setAttribute('cx', n2(m.l + (i + .5) * band)); d.setAttribute('cy', n2(y(v))); }
      });
      return { x: m.l + (i + .5) * band, y: m.t };
    },
    hide() { active.setAttribute('hidden', ''); dots.forEach(d => d.setAttribute('hidden', '')); },
    horizontal: false,
  };
}

function drawRows(spec, width) {
  const fmt = spec.format || String, n = spec.categories.length;
  const single = n === 1 && spec.stacked;
  const bh = single ? 28 : BAR_MAX, rowH = single ? 28 : bh + 10, height = n * rowH;
  const labelled = spec.categories.some(c => c.label);
  const labelChars = Math.max(0, ...spec.categories.map(c => c.label.length));
  const labelW = labelled ? Math.min(width * .5, labelChars * CHAR + 16) : 0;
  const totals = spec.categories.map((_, i) => spec.stacked ? sum(spec.series.map(s => s.values[i])) : Math.max(0, ...spec.series.map(s => s.values[i] || 0)));
  const max = Math.max(...totals, 0) || 1;
  const valueW = spec.stacked ? 0 : Math.max(...spec.series.flatMap(s => s.values.map(v => fmt(v || 0).length)), 1) * CHAR + 12;
  const pw = Math.max(width - labelW - valueW, 10);
  const svg = svgEl('svg', { width, height, viewBox: `0 0 ${width} ${height}`, 'aria-hidden': 'true', focusable: 'false' });
  const active = svgEl('rect', { class: 'wk-chart-active', x: 0, width, height: rowH, hidden: '' }, svg);
  spec.categories.forEach((c, i) => {
    const cy = i * rowH + rowH / 2, yy = cy - bh / 2;
    if (labelled) svgEl('text', { class: 'wk-chart-tick', x: labelW - 8, y: n2(cy), 'text-anchor': 'end', 'dominant-baseline': 'central' }, svg).textContent = clip(c.label, Math.floor((labelW - 12) / CHAR));
    if (spec.stacked) {
      const segments = spec.series.filter(s => s.values[i] > 0);
      let x = labelW;
      segments.forEach((s, j) => {
        const w = Math.max((s.values[i] / max) * pw - (j > 0 ? GAP : 0), 1.5);
        svgEl('path', { class: 'wk-chart-bar', 'data-tone': s.tone, d: (j === segments.length - 1 ? roundedEnd : (a, b, c2, d) => `M${a} ${b}h${n2(c2)}v${d}h${n2(-c2)}z`)(n2(x + (j > 0 ? GAP : 0)), n2(yy), n2(w), bh) }, svg);
        x += (s.values[i] / max) * pw;
      });
    } else {
      spec.series.forEach(s => {
        const v = s.values[i] || 0;
        if (v > 0) svgEl('path', { class: 'wk-chart-bar', 'data-tone': s.tone, d: roundedEnd(labelW, yy, Math.max((v / max) * pw, 1.5), bh) }, svg);
        svgEl('text', { class: 'wk-chart-value', x: n2(labelW + (v / max) * pw + 6), y: n2(cy), 'dominant-baseline': 'central' }, svg).textContent = fmt(v);
      });
    }
  });
  return {
    svg,
    index: (px, py) => Math.min(n - 1, Math.max(0, Math.floor(py / rowH))),
    show(i) { active.removeAttribute('hidden'); active.setAttribute('y', i * rowH); return { x: labelW + pw / 2, y: i * rowH + rowH }; },
    hide() { active.setAttribute('hidden', ''); },
    horizontal: true,
  };
}

function legend(spec) {
  const fmt = spec.format || String;
  const list = htmlEl('ul', 'wk-chart-legend');
  const entry = (shape, tone, text) => {
    const li = htmlEl('li', 'wk-chart-legend-item', null, list);
    const key = htmlEl('span', 'wk-chart-key', null, li);
    key.dataset.shape = shape; if (tone != null) key.dataset.tone = tone; key.setAttribute('aria-hidden', 'true');
    htmlEl('span', null, text, li);
  };
  for (const s of spec.series) entry(spec.kind === 'line' ? 'line' : 'bar', s.tone, spec.legendValues ? `${s.label} ${fmt(sum(s.values))}` : s.label);
  if (spec.band) entry('band', spec.band.tone, spec.band.label);
  if (spec.shadeLabel && spec.categories.some(c => c.shaded)) entry('shade', null, spec.shadeLabel);
  return list;
}

// Detached plots must not be retained by the shared ResizeObserver. Keep their
// callbacks weakly, so charts made before attachment (or reattached later) can
// resume observing without keeping discarded charts alive.
const sized = new WeakMap(); // plot → its redraw
const observing = new Set(); // only connected plots; swept on every DOM mutation
let sizeObserver = null, connectionObserver = null;
function syncSize(plot) {
  if (!sized.has(plot)) return;
  if (plot.isConnected) {
    if (!observing.has(plot)) {
      observing.add(plot);
      sizeObserver.observe(plot);
    }
  } else if (observing.delete(plot)) sizeObserver.unobserve(plot);
}
function syncSizesIn(node) {
  if (node.nodeType !== 1) return;
  syncSize(node);
  node.querySelectorAll('.wk-chart-plot').forEach(syncSize);
}
function watchSize(plot, redraw) {
  if (typeof ResizeObserver !== 'function') return null;
  sizeObserver ??= new ResizeObserver(entries => { for (const e of entries) if (e.target.isConnected) sized.get(e.target)?.(); });
  connectionObserver ??= new MutationObserver(records => {
    for (const observed of observing) if (!observed.isConnected) syncSize(observed);
    for (const record of records) {
      record.removedNodes.forEach(syncSizesIn);
      record.addedNodes.forEach(syncSizesIn);
    }
  });
  connectionObserver.observe(document, { childList: true, subtree: true });
  sized.set(plot, redraw);
  syncSize(plot);
  return plot;
}
function unwatchSize(plot) {
  sized.delete(plot);
  if (observing.delete(plot)) sizeObserver.unobserve(plot);
}

/** Builds the chart's <figure>. It redraws itself when its width changes. */
export function wkChart(spec) {
  const id = spec.id || `wk-chart-${++sequence}`;
  let view = spec.view === 'table' ? 'table' : 'chart';
  const fig = htmlEl('figure', 'wk-chart');
  fig.id = id; fig.setAttribute('aria-labelledby', `${id}-title`);
  const head = htmlEl('div', 'wk-chart-head', null, fig);
  const title = htmlEl('h3', 'wk-chart-title', spec.label, head);
  title.id = `${id}-title`;
  if (spec.caption) htmlEl('span', 'wk-chart-caption', spec.caption, title);
  const toggle = htmlEl('div', 'wk-segmented wk-chart-view', null, head);
  toggle.setAttribute('role', 'radiogroup'); toggle.setAttribute('aria-label', `${spec.label} view`);
  for (const [value, text] of [['chart', 'Chart'], ['table', 'Table']]) {
    const label = htmlEl('label', null, null, toggle), input = htmlEl('input', null, null, label);
    input.id = `${id}-view-${value}`; input.type = 'radio'; input.name = `${id}-view`; input.value = value; input.checked = value === view;
    htmlEl('span', null, text, label);
    input.addEventListener('change', () => { if (input.checked) { view = value; spec.onview?.(value); paint(); } });
  }
  fig.append(legend(spec));
  const body = htmlEl('div', 'wk-chart-body', null, fig);
  const status = htmlEl('p', 'wk-chart-readout', null, fig);
  status.setAttribute('role', 'status');
  let observed = null, current = null, width = 0, selection = Number.isInteger(spec.selection) ? spec.selection : -1, activeIndex = selection;
  const empty = spec.categories.length === 0 || spec.series.length === 0;

  function tooltip(plot, tip, at, i) {
    const r = readout(spec, i);
    tip.replaceChildren();
    htmlEl('p', 'wk-chart-tip-title', r.title, tip);
    for (const row of r.rows) {
      const line = htmlEl('p', 'wk-chart-tip-row', null, tip);
      const key = htmlEl('span', 'wk-chart-key', null, line);
      key.dataset.shape = row.shape; key.dataset.tone = row.tone; key.setAttribute('aria-hidden', 'true');
      htmlEl('strong', null, row.text, line);
      htmlEl('span', 'wk-chart-tip-name', row.label, line);
    }
    tip.removeAttribute('hidden');
    const flip = at.x > plot.clientWidth / 2;
    tip.style.left = flip ? 'auto' : `${Math.round(at.x + 12)}px`;
    tip.style.right = flip ? `${Math.round(plot.clientWidth - at.x + 12)}px` : 'auto';
    tip.style.top = `${Math.round(Math.max(0, at.y))}px`;
    status.textContent = r.text;
  }

  function drawChart(plot, w) {
    width = w;
    const drawn = (spec.orientation === 'horizontal' && spec.kind !== 'line' ? drawRows : drawColumns)(spec, w);
    current = drawn;
    plot.querySelector('svg')?.remove();
    plot.prepend(drawn.svg);
    const tip = plot.querySelector('.wk-chart-tip');
    const activate = i => { activeIndex = i; selection = i; spec.onselection?.(i); tooltip(plot, tip, drawn.show(i), i); };
    const clear = () => { activeIndex = -1; selection = -1; spec.onselection?.(-1); drawn.hide(); tip.setAttribute('hidden', ''); status.textContent = ''; };
    drawn.svg.addEventListener('pointermove', e => { const box = drawn.svg.getBoundingClientRect(); activate(drawn.index(e.clientX - box.left, e.clientY - box.top)); });
    drawn.svg.addEventListener('pointerleave', () => { if (document.activeElement !== plot) clear(); });
    plot.onkeydown = e => {
      const n = spec.categories.length, next = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
      if (e.key === 'Escape' && activeIndex >= 0) { clear(); e.preventDefault(); e.stopPropagation(); }
      else if (next || e.key === 'Home' || e.key === 'End') {
        e.preventDefault();
        activate(e.key === 'Home' ? 0 : e.key === 'End' ? n - 1 : Math.min(n - 1, Math.max(0, activeIndex < 0 ? (next > 0 ? 0 : n - 1) : activeIndex + next)));
      }
    };
    if (!spec.retainSelectionOnBlur) plot.onblur = clear;
    if (activeIndex >= 0) activate(Math.min(activeIndex, spec.categories.length - 1));
  }

  function paint() {
    if (observed) unwatchSize(observed); observed = null; current = null; activeIndex = selection; status.textContent = '';
    body.replaceChildren();
    if (view === 'table') {
      const { head: header, rows } = tableModel(spec);
      const scroll = htmlEl('div', 'wk-chart-scroll', null, body);
      scroll.tabIndex = 0; scroll.setAttribute('role', 'region'); scroll.setAttribute('aria-label', `${spec.label}, table`); scroll.id = `${id}-table`;
      const table = htmlEl('table', 'wk-table', null, scroll);
      const tr = htmlEl('tr', null, null, htmlEl('thead', null, null, table));
      header.forEach((text, i) => { const th = htmlEl('th', i ? 'wk-num' : null, text, tr); th.scope = 'col'; });
      const tbody = htmlEl('tbody', null, null, table);
      for (const row of rows) {
        const r = htmlEl('tr', null, null, tbody);
        row.forEach((text, i) => { const cell = htmlEl(i ? 'td' : 'th', i ? 'wk-num' : null, text, r); if (!i) cell.scope = 'row'; });
      }
      return;
    }
    if (empty) { htmlEl('p', 'wk-chart-none', spec.emptyText || 'No data', body); return; }
    const plot = htmlEl('div', 'wk-chart-plot', null, body);
    plot.id = `${id}-plot`; plot.tabIndex = 0; plot.setAttribute('role', 'group');
    plot.setAttribute('aria-label', `${spec.label}${spec.summary ? `, ${spec.summary}` : ''}. Use the arrow keys to read each value.`);
    htmlEl('div', 'wk-chart-tip', null, plot).setAttribute('hidden', '');
    drawChart(plot, Math.round(plot.clientWidth) || 640);
    observed = watchSize(plot, () => {
      const w = Math.round(plot.clientWidth);
      if (w > 0 && w !== width) drawChart(plot, w);
    });
  }
  paint();
  return fig;
}
