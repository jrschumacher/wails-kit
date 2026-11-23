// @wails-kit/diagram: diagrams written as text (Mermaid syntax), drawn
// read-only in the kit's look, with pan and zoom. dist/diagram.js is this
// module; it loads dist/mermaid.js (Mermaid, about 1.2 MB) with import()
// the first time a diagram renders, from beside itself. Nothing loads from
// the network.
import css from './diagram.css';
import { buildTheme, TOKENS } from './theme.js';

export { buildTheme, TEXT_PAIRS, MARK_PAIRS, contrast } from './theme.js';

/** The diagram types the kit draws (Mermaid's keywords in brackets). */
export const diagramTypes = Object.freeze(['flowchart', 'sequence', 'state', 'class', 'er', 'gantt']);

// Timings from @wails-kit/ui's BEHAVIOUR.md.
export const TIMINGS = Object.freeze({ indicatorDelay: 400, indicatorMinimum: 500, loadingLine: 3000, copied: 2000 });

const MIN_ZOOM = 0.1, MAX_ZOOM = 8, STEP = 1.25, PAN = 40, PAD = 16;
const SVG_NS = 'http://www.w3.org/2000/svg';

let loading = null, loaded = false;
// Mermaid, once: the first render loads dist/mermaid.js; a failed load is
// tried again on the next render.
function loadMermaid() {
  loading ??= import('./mermaid.js').then(mod => { loaded = true; return mod; }, err => { loading = null; throw err; });
  return loading;
}

// The frame's stylesheet, adopted once.
let styled = false;
let warned = false;
function adoptStyles() {
  if (styled) return;
  styled = true;
  if ('adoptedStyleSheets' in document && typeof CSSStyleSheet === 'function') {
    const sheet = new CSSStyleSheet();
    sheet.replaceSync(css);
    document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
    return;
  }
  // Safari before 16.4: a <style> element, which a strict CSP blocks.
  const style = document.createElement('style');
  style.textContent = css;
  document.head.append(style);
  if (!warned) {
    warned = true;
    console.warn('@wails-kit/diagram: this browser has no document.adoptedStyleSheets, so diagrams need a CSP that allows inline styles. It needs WebView2, Safari 16.4+ (macOS 13.3) or WebKitGTK 2.40+.');
  }
}

const media = q => (typeof matchMedia === 'function' ? matchMedia(q) : null);

// Every live diagram, for re-rendering when the appearance or theme changes.
const live = new Set();
let watching = false;
let pendingThemeCheck = 0;
function themeChanged() {
  if (pendingThemeCheck) return;
  pendingThemeCheck = requestAnimationFrame(() => {
    pendingThemeCheck = 0;
    for (const d of live) d.themeChanged();
  });
}
function watchTheme() {
  if (watching) return;
  watching = true;
  new MutationObserver(themeChanged).observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme', 'class', 'style'] });
  for (const q of ['(prefers-color-scheme: dark)', '(forced-colors: active)', '(prefers-contrast: more)']) media(q)?.addEventListener('change', themeChanged);
}

// The theme from the tokens where `host` sits, resolved by a probe element.
function readTheme(host) {
  const at = host.isConnected ? host : document.documentElement;
  const probe = document.createElement('span');
  probe.hidden = true;
  at.append(probe);
  const raw = getComputedStyle(probe);
  const get = name => {
    if (!name.startsWith('--wk-color-')) return raw.getPropertyValue(name).trim();
    probe.style.color = `var(${name})`;
    return getComputedStyle(probe).color;
  };
  const resolve = value => { probe.style.color = value; return getComputedStyle(probe).color; };
  try {
    const forced = media('(forced-colors: active)')?.matches ? { canvas: resolve('Canvas'), text: resolve('CanvasText') } : null;
    return buildTheme(get, { forced });
  } finally {
    probe.remove();
  }
}

function svgEl(name, attrs = {}) {
  const el = document.createElementNS(SVG_NS, name);
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
  return el;
}
function copyGlyph() {
  const svg = svgEl('svg', { viewBox: '0 0 16 16', fill: 'none', stroke: 'currentColor', 'stroke-width': '1.3', 'aria-hidden': 'true' });
  svg.append(svgEl('rect', { x: '5.5', y: '5.5', width: '8', height: '8', rx: '1.5' }), svgEl('path', { d: 'M10.5 3.5v-1a1 1 0 0 0-1-1h-6a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h1' }));
  return svg;
}
function button(text, label, onclick) {
  const b = document.createElement('button');
  b.type = 'button';
  b.className = 'wk-button wk-button--small';
  b.textContent = text;
  if (label) b.setAttribute('aria-label', label);
  b.addEventListener('click', onclick);
  return b;
}
const slug = s => s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'diagram';

let counter = 0;

/**
 * Builds a diagram from Mermaid text. Returns a <div class="wk-diagram">
 * with `source` (get and set: setting re-renders), `ready` (a promise that
 * settles once the current source is drawn in the current theme), `zoom`, `fit()`, `zoomTo(scale)`, `zoomBy(factor)`,
 * `toSVG()`, `toPNG(scale)`, `copySource()` and `destroy()`. It fires
 * `wk-render` ({ detail: { type } }) when a diagram is drawn and `wk-error`
 * ({ detail: { message, line, kind } }) when it cannot be.
 */
export function wkDiagram(spec = {}) {
  const label = typeof spec.label === 'string' ? spec.label.trim() : '';
  if (!label) throw new TypeError('wkDiagram: label (the accessible name) is required');
  adoptStyles();
  watchTheme();

  const uid = `wk-diagram-${++counter}`;
  const el = document.createElement('div');
  el.className = 'wk-diagram';
  el.id = spec.id || uid;
  const ids = { help: `${uid}-help`, text: `${uid}-text`, zoom: `${uid}-zoom` };

  // Toolbar: zoom out, zoom in, fit, 100%, the zoom level; copy and export.
  const toolbar = document.createElement('div');
  toolbar.className = 'wk-diagram-toolbar';
  toolbar.setAttribute('role', 'group');
  toolbar.setAttribute('aria-label', `${label}: view`);
  const zoomOut = button('−', 'Zoom out', () => zoomBy(1 / STEP));
  const zoomIn = button('+', 'Zoom in', () => zoomBy(STEP));
  const fitButton = button('Fit', 'Fit to view', () => fit());
  const actual = button('100%', 'Actual size', () => zoomTo(1));
  const level = document.createElement('span');
  level.className = 'wk-diagram-zoom';
  level.id = ids.zoom;
  const end = document.createElement('div');
  end.className = 'wk-diagram-toolbar-end';
  const note = document.createElement('span');
  note.className = 'wk-diagram-note';
  note.setAttribute('role', 'status');
  const copyButton = button('Copy source', null, () => copySource());
  const svgButton = button('Save SVG', 'Save as SVG', () => save('svg'));
  const pngButton = button('Save PNG', 'Save as PNG', () => save('png'));
  end.append(note, copyButton, svgButton, pngButton);
  toolbar.append(zoomOut, zoomIn, fitButton, actual, level, end);
  const viewOnly = [zoomOut, zoomIn, fitButton, actual, svgButton, pngButton];

  // Viewport: the canvas that pans and zooms, the working indicator, the error.
  const viewport = document.createElement('div');
  viewport.className = 'wk-diagram-viewport';
  viewport.tabIndex = 0;
  viewport.setAttribute('role', 'group');
  viewport.setAttribute('aria-label', label);
  viewport.setAttribute('aria-describedby', ids.help);
  const canvas = document.createElement('div');
  canvas.className = 'wk-diagram-canvas';
  const status = document.createElement('div');
  status.className = 'wk-diagram-status';
  status.hidden = true;
  status.setAttribute('role', 'progressbar');
  status.setAttribute('aria-label', 'Drawing the diagram');
  const spinner = document.createElement('span');
  spinner.className = 'wk-diagram-spinner';
  spinner.setAttribute('aria-hidden', 'true');
  const loadingLine = document.createElement('span');
  loadingLine.hidden = true;
  status.append(spinner, loadingLine);
  const errorWrap = document.createElement('div');
  errorWrap.className = 'wk-diagram-error';
  errorWrap.hidden = true;
  viewport.append(canvas, status, errorWrap);

  const help = document.createElement('span');
  help.className = 'wk-visually-hidden';
  help.id = ids.help;
  help.textContent = 'Drag or use the arrow keys to pan. Plus and minus zoom, 0 fits.';

  // The text alternative: the description, else the source.
  const details = document.createElement('details');
  details.className = 'wk-diagram-text';
  const summary = document.createElement('summary');
  const textAlt = document.createElement(spec.description ? 'p' : 'pre');
  textAlt.id = ids.text;
  details.append(summary, textAlt);

  el.append(toolbar, viewport, details, help);

  let source = String(spec.source ?? '');
  let svg = null, sheet = null, type = null, failure = null;
  let view = { x: 0, y: 0, k: 1 }, size = { w: 0, h: 0 }, fitted = true;
  let themeKey = '', destroyed = false, generation = 0, stale = false;
  let current = Promise.resolve();

  function describe() {
    summary.textContent = spec.description ? 'Description' : 'Diagram source';
    textAlt.textContent = spec.description ? String(spec.description) : source;
  }

  // ---- Pan and zoom ------------------------------------------------------
  const clamp = k => Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, k));
  function apply() {
    canvas.style.transform = `translate(${view.x}px, ${view.y}px) scale(${view.k})`;
    level.textContent = svg ? `${Math.round(view.k * 100)}%` : '';
    zoomIn.disabled = !svg || view.k >= MAX_ZOOM;
    zoomOut.disabled = !svg || view.k <= MIN_ZOOM;
  }
  function zoomAt(k, px, py) {
    k = clamp(k);
    view = { x: px - (px - view.x) * (k / view.k), y: py - (py - view.y) * (k / view.k), k };
    fitted = false;
    apply();
  }
  function zoomTo(k) {
    if (!svg) return;
    zoomAt(k, viewport.clientWidth / 2, viewport.clientHeight / 2);
  }
  function zoomBy(f) { if (svg) zoomTo(view.k * f); }
  function panBy(dx, dy) {
    if (!svg) return;
    view = { ...view, x: view.x + dx, y: view.y + dy };
    fitted = false;
    apply();
  }
  // Fits the diagram in the viewport, centred; never above 100%.
  function fit() {
    if (!svg) return;
    const vw = viewport.clientWidth, vh = viewport.clientHeight;
    if (!vw || !vh || !size.w || !size.h) { fitted = true; return; }
    const k = clamp(Math.min((vw - 2 * PAD) / size.w, (vh - 2 * PAD) / size.h, 1));
    view = { x: (vw - size.w * k) / 2, y: (vh - size.h * k) / 2, k };
    fitted = true;
    apply();
  }

  viewport.addEventListener('keydown', e => {
    if (e.target !== viewport || !svg || e.altKey || e.ctrlKey || e.metaKey) return;
    const step = e.shiftKey ? PAN * 4 : PAN;
    const keys = {
      ArrowLeft: () => panBy(step, 0), ArrowRight: () => panBy(-step, 0), ArrowUp: () => panBy(0, step), ArrowDown: () => panBy(0, -step),
      '+': () => zoomBy(STEP), '=': () => zoomBy(STEP), '-': () => zoomBy(1 / STEP), _: () => zoomBy(1 / STEP), 0: () => fit(),
    };
    const act = keys[e.key];
    if (!act) return;
    e.preventDefault();
    act();
  });

  viewport.addEventListener('wheel', e => {
    if (!svg) return;
    e.preventDefault();
    const r = viewport.getBoundingClientRect();
    // A pinch on a trackpad arrives as a wheel with ctrlKey, in small steps.
    const scale = e.deltaMode === 1 ? 16 : e.deltaMode === 2 ? viewport.clientHeight : 1;
    const factor = Math.exp(-e.deltaY * scale * (e.ctrlKey ? 0.01 : 0.002));
    zoomAt(view.k * factor, e.clientX - r.left, e.clientY - r.top);
  }, { passive: false });

  const pointers = new Map();
  let pinch = null;
  const pinchState = () => {
    const [a, b] = [...pointers.values()];
    return { d: Math.hypot(a.x - b.x, a.y - b.y) || 1, x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
  };
  viewport.addEventListener('pointerdown', e => {
    if (!svg || (e.pointerType === 'mouse' && e.button !== 0)) return;
    try { viewport.setPointerCapture(e.pointerId); } catch { /* a synthetic or ended pointer */ }
    pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
    if (pointers.size === 2) pinch = pinchState();
    viewport.toggleAttribute('data-dragging', true);
  });
  viewport.addEventListener('pointermove', e => {
    const p = pointers.get(e.pointerId);
    if (!p) return;
    const dx = e.clientX - p.x, dy = e.clientY - p.y;
    p.x = e.clientX; p.y = e.clientY;
    if (pointers.size === 1) { panBy(dx, dy); return; }
    if (pointers.size === 2 && pinch) {
      const next = pinchState();
      const r = viewport.getBoundingClientRect();
      view = { ...view, x: view.x + (next.x - pinch.x), y: view.y + (next.y - pinch.y) };
      zoomAt(view.k * (next.d / pinch.d), next.x - r.left, next.y - r.top);
      pinch = next;
    }
  });
  const release = e => {
    pointers.delete(e.pointerId);
    pinch = pointers.size === 2 ? pinchState() : null;
    if (!pointers.size) viewport.removeAttribute('data-dragging');
  };
  viewport.addEventListener('pointerup', release);
  viewport.addEventListener('pointercancel', release);

  // Starts fitted; follows its container while fitted. A diagram that
  // missed a theme change while detached renders when it is shown again.
  const resize = typeof ResizeObserver === 'function' ? new ResizeObserver(() => {
    if (stale && el.isConnected) { stale = false; render(); return; }
    if (fitted) fit();
  }) : null;
  resize?.observe(viewport);

  // ---- States --------------------------------------------------------------
  function setState(state) {
    viewport.dataset.state = state;
    for (const b of viewOnly) b.disabled = state !== 'ready';
    if (state === 'ready') apply();
  }

  function showError(err) {
    failure = { message: String(err?.message ?? err), line: Number.isInteger(err?.line) ? err.line : null, kind: err?.kind || 'syntax' };
    canvas.replaceChildren();
    sheet?.remove();
    svg = null; sheet = null; type = null;
    const box = document.createElement('div');
    box.className = 'wk-errorbox';
    const head = document.createElement('div');
    head.className = 'wk-errorbox-head';
    const title = document.createElement('span');
    title.textContent = failure.kind === 'unsupported' ? 'Can’t draw this kind of diagram' : failure.kind === 'load' ? 'Couldn’t load the diagram library' : 'Couldn’t draw the diagram';
    const copy = document.createElement('button');
    copy.type = 'button';
    copy.className = 'wk-copy';
    copy.setAttribute('aria-label', 'Copy error');
    copy.append(copyGlyph());
    copy.addEventListener('click', () => writeClipboard(failure.message, 'Error copied'));
    head.append(title, copy);
    box.append(head);
    if (failure.line != null) {
      const facts = document.createElement('dl');
      facts.className = 'wk-facts';
      const dt = document.createElement('dt');
      dt.textContent = 'Line';
      const dd = document.createElement('dd');
      dd.textContent = String(failure.line);
      facts.append(dt, dd);
      box.append(facts);
    }
    const pre = document.createElement('pre');
    pre.tabIndex = 0;
    pre.textContent = failure.message;
    box.append(pre);
    errorWrap.replaceChildren(box);
    errorWrap.hidden = false;
    setState('error');
    el.dispatchEvent(new CustomEvent('wk-error', { bubbles: true, detail: { ...failure } }));
    try { spec.onerror?.({ ...failure }); } catch (e) { console.error(e); }
  }

  function show(result) {
    const keepView = svg && !fitted ? view : null;
    sheet?.remove();
    ({ svg, sheet, type } = result);
    failure = null;
    errorWrap.hidden = true;
    errorWrap.replaceChildren();
    // The SVG is a picture named by the label, described by the text alternative.
    svg.setAttribute('role', 'img');
    svg.setAttribute('aria-label', label);
    svg.setAttribute('aria-describedby', ids.text);
    svg.removeAttribute('aria-labelledby');
    svg.removeAttribute('aria-roledescription');
    svg.setAttribute('focusable', 'false');
    const vb = svg.viewBox?.baseVal;
    size = { w: vb?.width || parseFloat(svg.getAttribute('width')) || 0, h: vb?.height || parseFloat(svg.getAttribute('height')) || 0 };
    svg.setAttribute('width', String(size.w));
    svg.setAttribute('height', String(size.h));
    svg.style.removeProperty('max-width');
    canvas.replaceChildren(svg);
    setState('ready');
    if (keepView) { view = keepView; apply(); } else fit();
    el.dispatchEvent(new CustomEvent('wk-render', { bubbles: true, detail: { type } }));
  }

  // ---- Rendering -------------------------------------------------------------
  // Quiet wait, then Working (BEHAVIOUR.md): no indicator for a render that
  // ends within 400 ms; once shown, the indicator stays 500 ms; after 3 s a
  // line says what is loading. A diagram already shown stays until the new
  // one replaces it, with the indicator in the corner.
  // The theme where the diagram sits now, and its key ('' if unreadable).
  function themeNow() {
    try {
      const theme = readTheme(el);
      return { theme, key: JSON.stringify(theme) };
    } catch (error) {
      return { error, key: '' };
    }
  }

  function render() {
    const gen = ++generation;
    // The key of the theme this render is for, recorded now: a theme check
    // that runs while it loads compares against it, not an older render's.
    themeKey = themeNow().key;
    describe();
    if (!svg && !failure) setState('loading');
    let shownAt = 0;
    const showTimer = setTimeout(() => {
      shownAt = performance.now();
      status.toggleAttribute('data-over', !!svg);
      status.hidden = false;
    }, TIMINGS.indicatorDelay);
    const lineTimer = setTimeout(() => {
      loadingLine.textContent = loaded ? 'Drawing the diagram…' : 'Loading the diagram library…';
      loadingLine.hidden = !!svg;
    }, TIMINGS.loadingLine);
    const settle = async () => {
      clearTimeout(showTimer);
      clearTimeout(lineTimer);
      if (shownAt) {
        const wait = TIMINGS.indicatorMinimum - (performance.now() - shownAt);
        if (wait > 0) await new Promise(r => setTimeout(r, wait));
      }
    };
    const run = (async () => {
      let mod;
      try {
        mod = await loadMermaid();
      } catch (err) {
        await settle();
        if (gen !== generation || destroyed) return;
        status.hidden = true;
        showError({ message: String(err?.message ?? err), kind: 'load' });
        return;
      }
      let result, error;
      try {
        // Read again after the load: the element may have been connected,
        // or the appearance changed, meanwhile.
        const now = themeNow();
        if (now.error) throw now.error;
        if (gen === generation) themeKey = now.key;
        result = await mod.render(source, now.theme);
      } catch (err) {
        error = err;
      }
      await settle();
      if (gen !== generation || destroyed) { result?.sheet?.remove(); return; }
      status.hidden = true;
      loadingLine.hidden = true;
      if (error) showError(error);
      else show(result);
    })();
    current = run;
    return run;
  }

  // ---- Copy and export -------------------------------------------------------
  let noteTimer = 0;
  function say(text) {
    clearTimeout(noteTimer);
    note.textContent = text;
    noteTimer = setTimeout(() => { note.textContent = ''; }, TIMINGS.copied);
  }
  async function writeClipboard(text, done) {
    try {
      await navigator.clipboard.writeText(text);
      say(done);
      return true;
    } catch {
      say('Couldn’t copy');
      return false;
    }
  }
  function copySource() { return writeClipboard(source, 'Source copied'); }

  // A standalone copy of the SVG: its styles in a <style> element (the copy
  // is never put in this page), a title and description, no ARIA ids that
  // point outside it.
  function toSVG() {
    if (!svg) throw new Error('wkDiagram: there is no diagram to export');
    const copy = svg.cloneNode(true);
    copy.setAttribute('xmlns', SVG_NS);
    copy.removeAttribute('aria-describedby');
    copy.removeAttribute('focusable');
    copy.style.removeProperty('transform');
    const title = document.createElementNS(SVG_NS, 'title');
    title.textContent = label;
    const desc = document.createElementNS(SVG_NS, 'desc');
    desc.textContent = spec.description ? String(spec.description) : source;
    const style = document.createElementNS(SVG_NS, 'style');
    style.textContent = sheet?.sheet ? [...sheet.sheet.cssRules].map(r => r.cssText).join('\n') : '';
    copy.prepend(title, desc, style);
    return new XMLSerializer().serializeToString(copy);
  }

  // A PNG of the diagram at `scale` (default 2) on its ground. It draws the
  // SVG through an <img> from a data: URL, so the page's CSP must allow
  // img-src data:.
  function toPNG(scale = 2) {
    if (!svg) return Promise.reject(new Error('wkDiagram: there is no diagram to export'));
    const text = toSVG();
    const ground = getComputedStyle(viewport).backgroundColor;
    return new Promise((resolve, reject) => {
      const img = new Image();
      img.onload = () => {
        const canvasEl = document.createElement('canvas');
        canvasEl.width = Math.max(1, Math.ceil(size.w * scale));
        canvasEl.height = Math.max(1, Math.ceil(size.h * scale));
        const ctx = canvasEl.getContext('2d');
        ctx.fillStyle = ground;
        ctx.fillRect(0, 0, canvasEl.width, canvasEl.height);
        ctx.drawImage(img, 0, 0, canvasEl.width, canvasEl.height);
        canvasEl.toBlob(blob => (blob ? resolve(blob) : reject(new Error('wkDiagram: the PNG could not be made'))), 'image/png');
      };
      img.onerror = () => reject(new Error('wkDiagram: the SVG could not be drawn as an image; the page’s CSP must allow img-src data:'));
      img.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(text)}`;
    });
  }

  async function save(format) {
    const filename = `${spec.filename || slug(label)}.${format}`;
    let blob;
    try {
      blob = format === 'svg' ? new Blob([toSVG()], { type: 'image/svg+xml' }) : await toPNG();
    } catch (err) {
      say(format === 'svg' ? 'Couldn’t save the SVG' : 'Couldn’t save the PNG');
      console.error(err);
      return;
    }
    if (spec.onexport) { spec.onexport({ format, blob, filename }); return; }
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.hidden = true;
    el.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  const instance = {
    themeChanged() {
      if (destroyed) return;
      if (!el.isConnected) { stale = true; return; }
      if (themeNow().key !== themeKey) render();
    },
  };

  // Settles once the diagram shows the current source in the current theme.
  // An appearance change reaches the theme check on the next frame (media
  // query change events and the requestAnimationFrame behind them), so a
  // caller awaiting `ready` straight after one would otherwise get the
  // render for the old appearance; it re-checks the theme here, and waits
  // again if a newer render started meanwhile.
  async function settled() {
    for (;;) {
      if (!destroyed && el.isConnected && themeNow().key !== themeKey) render();
      const run = current;
      await run;
      if (run === current) return;
    }
  }
  live.add(instance);

  Object.defineProperties(el, {
    source: {
      get: () => source,
      set: next => {
        next = String(next ?? '');
        if (destroyed || next === source) return;
        source = next;
        render();
      },
      enumerable: true,
    },
    ready: { get: () => settled() },
    zoom: { get: () => view.k },
    type: { get: () => type },
    fit: { value: () => fit() },
    zoomTo: { value: k => zoomTo(Number(k)) },
    zoomBy: { value: f => zoomBy(Number(f)) },
    toSVG: { value: toSVG },
    toPNG: { value: toPNG },
    copySource: { value: copySource },
    destroy: {
      value: () => {
        if (destroyed) return;
        destroyed = true;
        generation++;
        live.delete(instance);
        resize?.disconnect();
        clearTimeout(noteTimer);
        sheet?.remove();
        svg = null; sheet = null;
        el.replaceChildren();
      },
    },
  });

  describe();
  setState('loading');
  render();
  return el;
}

/** Loads Mermaid now, ahead of the first diagram (optional). */
export function preloadDiagrams() {
  return loadMermaid().then(() => undefined);
}

export { TOKENS };
