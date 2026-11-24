// The component gallery: a sidebar of components grouped like the design
// system's cards, one page per component addressed by its hash (#/button),
// an overview (#/) and every page at once (#/all). The toolbar picks the
// appearance, the theme, the text size and where the kit comes from (src/
// or dist/); each choice is kept in the query, so a link opens the gallery
// as it was. The pages are in pages/*.js; each example's markup is shown
// beside it, from the same string that drew it.
import { pages, groups } from './pages/index.js';
import { renderCard } from './card.js';
import { dedent, body } from './source.js';

const params = new URLSearchParams(location.search);
const source = params.get('source') === 'dist' ? 'dist' : 'src';
export const themes = [
  ['', 'No theme', null],
  ['example', 'Example theme', '../ui/theme/example.css'],
  ['warm-grounds', 'Fixture: warm grounds', '../ui/test/fixtures/warm-grounds.css'],
  ['gold-working', 'Fixture: gold working', '../ui/test/fixtures/gold-working.css'],
  ['low-text', 'Fixture: low text', '../ui/test/fixtures/low-text.css'],
  ['strong-secondary', 'Fixture: strong secondary', '../ui/test/fixtures/strong-secondary.css'],
];
const sizes = ['100', '125', '150'];
const pick = (name, allowed, fallback) => (allowed.includes(params.get(name)) ? params.get(name) : fallback);
const state = {
  appearance: pick('appearance', ['light', 'dark'], ''),
  theme: pick('theme', themes.map(t => t[0]), ''),
  size: pick('size', sizes, '100'),
};

// The kit's stylesheet and the theme go before gallery.css, so the gallery's
// own layout rules come last.
const own = document.querySelector('link[href="gallery.css"]');
const sheet = (id, href) => new Promise(resolve => {
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.id = id;
  link.href = href;
  link.addEventListener('load', resolve, { once: true });
  link.addEventListener('error', resolve, { once: true });
  own.before(link);
});
await sheet('gallery-kit', source === 'dist' ? '../ui/dist/wails-kit-ui.css' : '../ui/src/ui.css');
const themeLink = document.createElement('link');
themeLink.rel = 'stylesheet';
themeLink.id = 'gallery-theme';
own.before(themeLink);
const kit = await import(source === 'dist' ? '../ui/dist/wails-kit-ui.js' : '../ui/src/index.js');

const nav = document.getElementById('gallery-nav');
const toolbar = document.getElementById('gallery-toolbar');
const main = document.getElementById('gallery-main');
const forced = document.getElementById('gallery-forced');

// ---- The query: appearance, theme, size and source ------------------------
function writeQuery() {
  const q = new URLSearchParams();
  if (source === 'dist') q.set('source', 'dist');
  if (state.theme) q.set('theme', state.theme);
  if (state.appearance) q.set('appearance', state.appearance);
  if (state.size !== '100') q.set('size', state.size);
  const search = q.toString();
  history.replaceState(null, '', `${location.pathname}${search ? `?${search}` : ''}${location.hash}`);
}
function applyAppearance() {
  if (state.appearance) document.documentElement.dataset.theme = state.appearance;
  else delete document.documentElement.dataset.theme;
}
async function applyTheme() {
  const href = themes.find(t => t[0] === state.theme)[2];
  if (!href) { themeLink.removeAttribute('href'); return; }
  if (themeLink.getAttribute('href') === href) return;
  await new Promise(resolve => {
    themeLink.addEventListener('load', resolve, { once: true });
    themeLink.addEventListener('error', resolve, { once: true });
    themeLink.href = href;
  });
}
// Text size: the kit's sizes are in px, as in a desktop window, so the
// specimens are zoomed, as the web view's own zoom does.
function applySize() {
  if (state.size === '100') main.style.removeProperty('zoom');
  else main.style.setProperty('zoom', String(Number(state.size) / 100));
  document.documentElement.dataset.gallerySize = state.size;
}
applyAppearance();
await applyTheme();
applySize();

// ---- Toolbar ----------------------------------------------------------------
toolbar.innerHTML = `
  <p class="wk-toolbar-title">Component gallery <span class="wk-toolbar-subtitle" id="gallery-source-words"></span></p>
  <span class="wk-toolbar-spacer"></span>
  <div class="wk-segmented" role="radiogroup" aria-label="Appearance" id="gallery-appearance">
    <label><input type="radio" name="gallery-appearance" value=""><span>System</span></label>
    <label><input type="radio" name="gallery-appearance" value="light"><span>Light</span></label>
    <label><input type="radio" name="gallery-appearance" value="dark"><span>Dark</span></label>
  </div>
  <label class="gallery-control"><span class="wk-caption">Theme</span>
    <select class="wk-select" id="gallery-theme-select">${themes.map(([v, label]) => `<option value="${v}">${label}</option>`).join('')}</select></label>
  <label class="gallery-control"><span class="wk-caption">Text size</span>
    <select class="wk-select" id="gallery-size">${sizes.map(s => `<option value="${s}">${s}%</option>`).join('')}</select></label>
  <label class="gallery-control"><span class="wk-caption">Kit from</span>
    <select class="wk-select" id="gallery-source"><option value="src">src/</option><option value="dist">dist/ (prebuilt)</option></select></label>
  <button class="wk-link" type="button" aria-expanded="false" aria-controls="gallery-forced-help" id="gallery-forced-toggle">Forced colours</button>`;
document.getElementById('gallery-source-words').textContent = source === 'dist' ? 'the prebuilt dist/ files' : 'the kit from src/';
for (const input of toolbar.querySelectorAll('[name="gallery-appearance"]')) input.checked = input.value === state.appearance;
const themeSelect = document.getElementById('gallery-theme-select');
const sizeSelect = document.getElementById('gallery-size');
const sourceSelect = document.getElementById('gallery-source');
themeSelect.value = state.theme;
sizeSelect.value = state.size;
sourceSelect.value = source;
document.getElementById('gallery-appearance').addEventListener('change', e => {
  state.appearance = e.target.value;
  applyAppearance();
  writeQuery();
});
themeSelect.addEventListener('change', async () => {
  state.theme = themeSelect.value;
  writeQuery();
  await applyTheme();
  document.dispatchEvent(new CustomEvent('gallery-theme', { detail: { theme: state.theme } }));
});
sizeSelect.addEventListener('change', () => { state.size = sizeSelect.value; applySize(); writeQuery(); });
// The kit's module can't be swapped in a page that has defined its custom
// elements, so the source switch reloads with the other one.
sourceSelect.addEventListener('change', () => {
  const q = new URLSearchParams(location.search);
  if (sourceSelect.value === 'dist') q.set('source', 'dist'); else q.delete('source');
  location.search = q.toString();
});

// Forced colours can't be simulated by a class: the browser or the system
// turns it on. The gallery says how, and says when it is on.
forced.innerHTML = `
  <div class="wk-notice wk-notice--inline gallery-forced-on" id="gallery-forced-on" role="status" hidden><div class="wk-notice-body">Forced colours are on: every page shows the kit's forced-colours rules.</div></div>
  <div class="wk-notice gallery-forced-help" id="gallery-forced-help" hidden><div class="wk-notice-body"><b>Seeing forced colours</b>
    <p>A page can't turn forced colours on for itself. On Windows, turn on a contrast theme (Settings, Accessibility, Contrast themes). In Chrome or Edge on any system, open the developer tools, then Rendering, and set "Emulate CSS media feature forced-colors" to active. The gallery follows it at once.</p></div></div>`;
const forcedToggle = document.getElementById('gallery-forced-toggle');
forcedToggle.addEventListener('click', () => {
  const open = forcedToggle.getAttribute('aria-expanded') !== 'true';
  forcedToggle.setAttribute('aria-expanded', String(open));
  document.getElementById('gallery-forced-help').hidden = !open;
});
const forcedQuery = matchMedia('(forced-colors: active)');
const showForced = () => { document.getElementById('gallery-forced-on').hidden = !forcedQuery.matches; };
forcedQuery.addEventListener('change', showForced);
showForced();

// ---- Sidebar ------------------------------------------------------------------
const esc = s => String(s).replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]);
nav.innerHTML = `
  <div class="wk-search gallery-filter"><input class="wk-field" type="search" id="gallery-filter" placeholder="Filter" aria-label="Filter components" aria-controls="gallery-nav-lists"></div>
  <div id="gallery-nav-lists">
  <ul><li><a class="wk-sidebar-item" href="#/"><span class="wk-sidebar-label">Overview</span></a></li>
  <li><a class="wk-sidebar-item" href="#/all"><span class="wk-sidebar-label">All on one page</span></a></li></ul>
  ${groups.map(g => `<p class="wk-sidebar-heading" id="gallery-group-${g.id}">${esc(g.title)}</p>
  <ul aria-labelledby="gallery-group-${g.id}">${pages.filter(p => p.group === g.id).map(p => `<li><a class="wk-sidebar-item" href="#/${p.id}"><span class="wk-sidebar-label">${esc(p.title)}</span></a></li>`).join('')}</ul>`).join('')}
  </div>
  <p class="wk-small gallery-none" id="gallery-filter-none" hidden>No component matches.</p>
  <div class="wk-sidebar-footer">${pages.length} pages · ${source === 'dist' ? 'dist/' : 'src/'}</div>`;
const links = () => [...nav.querySelectorAll('a.wk-sidebar-item')].filter(a => !a.closest('li').hidden);
const filter = document.getElementById('gallery-filter');
filter.addEventListener('input', () => {
  const words = filter.value.trim().toLowerCase();
  for (const a of nav.querySelectorAll('a.wk-sidebar-item')) a.closest('li').hidden = Boolean(words) && !a.textContent.toLowerCase().includes(words);
  for (const ul of nav.querySelectorAll('ul[aria-labelledby]')) {
    const empty = ![...ul.children].some(li => !li.hidden);
    ul.hidden = empty;
    document.getElementById(ul.getAttribute('aria-labelledby')).hidden = empty;
  }
  document.getElementById('gallery-filter-none').hidden = links().length > 0;
});
// Up and Down move between the visible entries, Home and End jump; Down
// from the filter goes to the first entry. Tab still reaches each one.
nav.addEventListener('keydown', e => {
  if (e.altKey || e.ctrlKey || e.metaKey) return;
  const all = links();
  if (e.target === filter) {
    if (e.key === 'ArrowDown' && all.length) { e.preventDefault(); all[0].focus(); }
    return;
  }
  const at = all.indexOf(e.target);
  if (at < 0) return;
  const to = { ArrowDown: Math.min(at + 1, all.length - 1), ArrowUp: at - 1, Home: 0, End: all.length - 1 }[e.key];
  if (to === undefined) return;
  e.preventDefault();
  if (to < 0) filter.focus(); else all[to].focus();
});

// ---- Pages ----------------------------------------------------------------------
const copyGlyph = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><rect x="5.5" y="5.5" width="8" height="8" rx="1.6"/><path d="M10.5 3.5v-.2a1.3 1.3 0 0 0-1.3-1.3H3.8a1.3 1.3 0 0 0-1.3 1.3v5.4a1.3 1.3 0 0 0 1.3 1.3h.2"/></svg>';
const codeBlock = (label, text) => {
  const box = document.createElement('div');
  box.className = 'wk-code wk-code--scroll';
  const pre = document.createElement('pre');
  pre.tabIndex = 0;
  pre.setAttribute('role', 'region');
  pre.setAttribute('aria-label', label);
  pre.textContent = text;
  const copy = document.createElement('button');
  copy.className = 'wk-copy';
  copy.type = 'button';
  copy.title = 'Copy';
  copy.setAttribute('aria-label', `Copy: ${label}`);
  copy.innerHTML = copyGlyph;
  box.append(pre, copy);
  return box;
};

// Pending "Try it" runs: tests wait for none to be left.
window.galleryPending = 0;
// Runs a "Try it" press: the gallery counts the run until it settles.
async function track(promise) {
  window.galleryPending++;
  try { return await promise; } finally { window.galleryPending--; }
}
// What a drawn page's examples get. When the page goes, its waits settle at
// once (so a run never hangs), any wait asked for afterwards settles at once
// too, and every task it started is cancelled quietly, with its parts: nothing it
// started runs on in the task list or the summary.
function pageContext() {
  const cleanups = [];
  const waits = new Set();
  let alive = true;
  const own = handle => {
    // Quietly: the person didn't ask for this cancel, so nothing says it.
    cleanups.push(() => handle.cancelled({ quiet: true }));
    return new Proxy(handle, { get: (h, key) => (key === 'part' ? spec => own(h.part(spec)) : h[key]) });
  };
  return {
    kit,
    source,
    track,
    get alive() { return alive; },
    cleanup(fn) { cleanups.push(fn); },
    later(ms, fn) {
      if (!alive) return Promise.resolve();
      return new Promise(resolve => {
        const settle = () => { clearTimeout(timer); waits.delete(settle); resolve(alive ? fn?.() : undefined); };
        const timer = setTimeout(settle, ms);
        waits.add(settle);
      });
    },
    task: spec => own(kit.wkTask(spec)),
    end() {
      alive = false;
      for (const fn of cleanups.splice(0).reverse()) { try { fn(); } catch { /* the page is going */ } }
      for (const settle of [...waits]) settle();
    },
  };
}
let ctx = pageContext();

function renderExample(page, ex, index, depth) {
  const id = `${page.id}-ex-${index}`;
  const section = document.createElement('section');
  section.className = 'gallery-example';
  section.id = id;
  section.setAttribute('aria-labelledby', `${id}-h`);
  section.innerHTML = `<h${depth} class="wk-subheading gallery-example-title" id="${id}-h">${esc(ex.title)}</h${depth}>${ex.note ? `<p class="wk-caption gallery-note">${ex.note}</p>` : ''}`;
  const specimen = document.createElement('div');
  specimen.className = `gallery-specimen${ex.wide ? ' gallery-specimen--wide' : ''}`;
  if (ex.html) specimen.innerHTML = ex.html;
  section.append(specimen);
  const parts = [];
  if (ex.html) parts.push(['Markup', dedent(ex.html)]);
  if (ex.build) parts.push(['Script', body(ex.build)]);
  if (ex.after) parts.push([ex.html ? 'Script' : 'Script', body(ex.after)]);
  if (parts.length) {
    const details = document.createElement('details');
    details.className = 'gallery-code';
    const summary = document.createElement('summary');
    summary.className = 'wk-link';
    summary.textContent = parts.map(p => p[0]).join(' and ');
    details.append(summary, ...parts.map(([kind, text]) => codeBlock(`${kind}: ${ex.title}`, text)));
    section.append(details);
  }
  return { section, specimen, run: async () => {
    if (ex.build) await ex.build(specimen, ctx);
    if (ex.after) {
      const done = await ex.after(specimen, ctx);
      if (typeof done === 'function') ctx.cleanup(done);
    }
  } };
}

function renderPage(page, { all = false } = {}) {
  const depth = all ? 2 : 1;
  const article = document.createElement('article');
  article.className = 'gallery-page';
  article.id = `page-${page.id}`;
  article.dataset.page = page.id;
  const group = groups.find(g => g.id === page.group);
  article.innerHTML = `<header class="gallery-page-head">
    <p class="wk-caption">${esc(group.title)}</p>
    <h${depth} class="${all ? 'wk-title-2' : 'wk-title'}" ${all ? `id="page-${page.id}-title"` : 'id="gallery-title" tabindex="-1"'}>${all ? `<a class="wk-link" href="#/${page.id}">${esc(page.title)}</a>` : esc(page.title)}</h${depth}>
    ${page.intro ? `<p class="gallery-intro">${page.intro}</p>` : ''}</header>`;
  if (!all) article.setAttribute('aria-labelledby', 'gallery-title');
  else article.setAttribute('aria-labelledby', `page-${page.id}-title`);
  const runs = page.examples.map((ex, i) => {
    const r = renderExample(page, ex, i, depth + 1);
    article.append(r.section);
    return r.run;
  });
  if (page.card && !all) {
    const guide = document.createElement('section');
    guide.className = 'gallery-card';
    guide.setAttribute('aria-labelledby', `${page.id}-card-h`);
    guide.innerHTML = `<h${depth + 1} class="wk-subheading" id="${page.id}-card-h">From the design system's ${esc(page.card)} card</h${depth + 1}>`;
    article.append(guide);
    runs.push(() => renderCard(guide, page.card, depth + 2));
  }
  return { article, runs };
}

function overview() {
  const article = document.createElement('article');
  article.className = 'gallery-page';
  article.dataset.page = 'overview';
  article.setAttribute('aria-labelledby', 'gallery-title');
  article.innerHTML = `<header class="gallery-page-head"><h1 class="wk-title" id="gallery-title" tabindex="-1">Overview</h1>
    <p class="gallery-intro">Every component of @wails-kit/ui, @wails-kit/editor and @wails-kit/diagram, in each state it can show. Pick one on the left, or see <a class="wk-link" href="#/all">all on one page</a>. Each example's markup or script is under it, ready to copy; "Try it" buttons run the behaviour in <code>BEHAVIOUR.md</code> with its real timings.</p></header>
    <section class="gallery-example" aria-labelledby="overview-toolbar-h"><h2 class="wk-subheading" id="overview-toolbar-h">The toolbar</h2>
    <dl class="wk-facts gallery-facts"><dt>Appearance</dt><dd>System follows the system; Light and Dark set <code>&lt;html data-theme&gt;</code>, as an app's own picker does.</dd>
    <dt>Theme</dt><dd>None, <code>theme/example.css</code>, or a fixture theme from <code>test/fixtures/</code> (the gold working tone and low text fail the theme check on purpose).</dd>
    <dt>Text size</dt><dd>Zooms the pages, as the web view's zoom does: the kit's sizes are in px.</dd>
    <dt>Kit from</dt><dd><code>src/</code> (the ES modules) or the prebuilt <code>dist/</code> files an app vendors.</dd>
    <dt>Forced colours</dt><dd>Turned on by the system or the browser, not by the page; the gallery says how.</dd></dl></section>
    <section class="gallery-example" aria-labelledby="overview-pages-h"><h2 class="wk-subheading" id="overview-pages-h">The packages' own review pages</h2>
    <ul class="gallery-links">
      <li><a class="wk-link" href="../ui/demo/index.html">@wails-kit/ui, every component on one page</a></li>
      <li><a class="wk-link" href="../ui/demo/themed.html">@wails-kit/ui with the example theme</a></li>
      <li><a class="wk-link" href="../ui/demo/dist.html">@wails-kit/ui from dist/</a></li>
      <li><a class="wk-link" href="../ui/demo/settings.html">The settings window</a></li>
      <li><a class="wk-link" href="../editor/demo/index.html">The code editor</a>, <a class="wk-link" href="../editor/demo/markdown.html">the Markdown editor</a> and <a class="wk-link" href="../editor/demo/richtext.html">the rich-text editor</a></li>
      <li><a class="wk-link" href="../diagram/demo/index.html">Diagrams</a></li>
    </ul></section>`;
  return article;
}

// Scrolls the sidebar so its current entry shows. By hand: scrollIntoView
// moves the sequential focus starting point (Chrome 153), so the first Tab
// would skip the skip link and the filter.
function reveal(link) {
  const top = link.getBoundingClientRect().top - nav.getBoundingClientRect().top + nav.scrollTop;
  if (top < nav.scrollTop) nav.scrollTop = top;
  else if (top + link.offsetHeight > nav.scrollTop + nav.clientHeight) nav.scrollTop = top + link.offsetHeight - nav.clientHeight;
}

let rendering = Promise.resolve();
// Only "#/…" names a page; any other hash (an in-page link) is left alone.
const pageHash = () => (/^#\/|^#?$/.test(location.hash) ? location.hash.replace(/^#\/?/, '') : null);
async function route({ focus = false } = {}) {
  const hash = pageHash();
  ctx.end();
  ctx = pageContext();
  main.replaceChildren();
  main.scrollTop = 0;
  let runs = [];
  let title;
  if (!hash) {
    main.append(overview());
    title = 'Overview';
  } else if (hash === 'all') {
    const head = document.createElement('header');
    head.className = 'gallery-page-head';
    head.innerHTML = '<h1 class="wk-title" id="gallery-title" tabindex="-1">All on one page</h1><p class="gallery-intro">Every page below the next, for visual review. Each title opens its own page.</p>';
    main.append(head);
    for (const page of pages) {
      const r = renderPage(page, { all: true });
      main.append(r.article);
      runs.push(...r.runs);
    }
    title = 'All on one page';
  } else {
    const page = pages.find(p => p.id === hash);
    if (page) {
      const r = renderPage(page);
      main.append(r.article);
      runs = r.runs;
      title = page.title;
    } else {
      main.innerHTML = '<article class="gallery-page" aria-labelledby="gallery-title"><h1 class="wk-title" id="gallery-title" tabindex="-1">No such page</h1><p class="gallery-intro">There is no component at this address. See the <a class="wk-link" href="#/">overview</a>.</p></article>';
      title = 'No such page';
    }
  }
  document.title = `${title} · Component gallery`;
  for (const a of nav.querySelectorAll('a.wk-sidebar-item')) {
    if (a.getAttribute('href') === `#/${hash}`) { a.setAttribute('aria-current', 'page'); reveal(a); } else a.removeAttribute('aria-current');
  }
  if (focus) document.getElementById('gallery-title').focus({ preventScroll: true });
  for (const run of runs) await run();
  // Steppers drawn by a page start with their arrows in step.
  kit.syncSteppers?.(main);
}
async function go(options) {
  document.documentElement.removeAttribute('data-gallery-ready');
  rendering = rendering.then(() => route(options)).catch(e => { console.error(e); });
  await rendering;
  document.documentElement.dataset.galleryReady = location.hash || '#/';
}
window.addEventListener('hashchange', () => { if (pageHash() !== null) go({ focus: true }); });
// The skip link moves focus to the page without changing the address.
document.querySelector('.gallery-skip').addEventListener('click', e => {
  e.preventDefault();
  main.focus();
});
await go();
