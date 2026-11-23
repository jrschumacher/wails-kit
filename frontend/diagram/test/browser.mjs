// Browser checks for wkDiagram, on the review pages in demo/ (index.html,
// themed.html), which load the prebuilt dist/diagram.js as an app vendors
// it; it loads dist/mermaid.js on first use. Run with: npm run test:browser
// It needs playwright-core (a devDependency) and Chrome: set CHROME_PATH, or
// have Chrome at /usr/bin/google-chrome or the macOS default. Without Chrome
// it skips, unless CI is set, when it fails.
//
// It serves the frontend/ folder, since the pages link @wails-kit/ui's
// stylesheet from ../../ui/. The pages carry a strict CSP (default-src
// 'self', no 'unsafe-inline' or 'unsafe-eval'). Any request outside this
// server, any console error or warning, any CSP violation and any failed
// request fails the check it happens in. Each check runs on its own page and
// reports on its own line. Waits are for conditions, not for time, except
// where the check is about time (the quiet wait).
import assert from 'node:assert/strict';
import http from 'node:http';
import { readFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';
import path from 'node:path';
const require = createRequire(import.meta.url);
const { chromium } = require(process.argv[2] || 'playwright-core');
const root = fileURLToPath(new URL('../../', import.meta.url)); // frontend/
const chrome = [process.env.CHROME_PATH, '/usr/bin/google-chrome', '/usr/bin/google-chrome-stable', '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'].find(p => p && existsSync(p));
if (!chrome) {
  console.log('test/browser.mjs: Chrome not found; set CHROME_PATH. Browser checks skipped.');
  process.exit(process.env.CI ? 1 : 0);
}
const types = { '.html': 'text/html', '.js': 'text/javascript', '.mjs': 'text/javascript', '.css': 'text/css', '.woff2': 'font/woff2' };
const server = http.createServer(async (req, res) => {
  try {
    const pathname = new URL(req.url, 'http://localhost').pathname;
    if (pathname.includes('..') || !/^\/(diagram|ui)\//.test(pathname) || pathname.includes('/node_modules/')) { res.writeHead(404).end(); return; }
    const data = await readFile(path.join(root, pathname));
    res.setHeader('Content-Type', types[path.extname(pathname)] || 'application/octet-stream');
    res.end(data);
  } catch { res.writeHead(404).end(); }
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
const origin = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.launch({ executablePath: chrome, headless: true });

const TYPES = ['flowchart', 'sequence', 'state', 'class', 'er', 'gantt'];

async function open(url = '/diagram/demo/index.html', options = {}, { wait = true, before } = {}) {
  const context = await browser.newContext({ viewport: { width: 1100, height: 900 }, colorScheme: 'light', ...options });
  const page = await context.newPage();
  const problems = [];
  page.on('pageerror', e => problems.push(`pageerror: ${e.message}`));
  page.on('console', m => { if (m.type() === 'error' || m.type() === 'warning') problems.push(`console ${m.type()}: ${m.text()}`); });
  page.on('request', r => { if (!r.url().startsWith(origin + '/') && !r.url().startsWith('data:') && !r.url().startsWith('blob:')) problems.push(`network: ${r.url()}`); });
  page.on('requestfailed', r => problems.push(`failed: ${r.url()}`));
  page.on('response', r => { if (r.status() >= 400) problems.push(`HTTP ${r.status()}: ${r.url()}`); });
  // CSP violations, recorded in the page from the start.
  await page.addInitScript(() => {
    window.violations = [];
    document.addEventListener('securitypolicyviolation', e => window.violations.push(`${e.violatedDirective}: ${e.blockedURI || e.sample || ''} at ${e.sourceFile}:${e.lineNumber}`));
  });
  if (before) await before(page);
  await page.goto(origin + url);
  if (wait) await page.waitForFunction(() => window.demoReady === true, null, { timeout: 30000 });
  await page.evaluate(() => {
    const probe = document.createElement('span');
    probe.className = 'wk-test-probe';
    probe.hidden = true;
    document.body.append(probe);
    window.t = { colour(value) { probe.style.color = value; return getComputedStyle(probe).color; } };
  });
  const close = async () => {
    const v = await page.evaluate(() => window.violations).catch(() => []);
    problems.push(...v.map(x => `csp: ${x}`));
    await context.close();
    assert.deepEqual(problems, [], 'console errors, CSP violations or network');
  };
  return { page, problems, close, context };
}
const until = (page, fn, arg, timeout = 5000) => page.waitForFunction(fn, arg, { timeout });
const setTheme = (page, theme) => page.evaluate(v => { if (v) document.documentElement.dataset.theme = v; else delete document.documentElement.dataset.theme; }, theme);
// Resolves after the diagram `id` next draws or fails.
const nextRender = (page, id) => page.evaluate(i => new Promise(resolve => {
  const el = document.getElementById(i);
  const done = e => { el.removeEventListener('wk-render', done); el.removeEventListener('wk-error', done); resolve(e.type); };
  el.addEventListener('wk-render', done);
  el.addEventListener('wk-error', done);
}), id);
// A new diagram from the dist module in #demo-scratch.
const make = (page, spec) => page.evaluate(async s => {
  const { wkDiagram } = await import('/diagram/dist/diagram.js');
  let host = document.getElementById('demo-scratch');
  if (!host) { host = document.createElement('div'); host.id = 'demo-scratch'; document.getElementById('demo').prepend(host); }
  window.calls = [];
  const el = wkDiagram({ ...s, onerror: f => window.calls.push(['onerror', f]), onexport: f => window.calls.push(['onexport', f.format, f.filename, f.blob.type, f.blob.size]) });
  el.addEventListener('wk-render', e => window.calls.push(['wk-render', e.detail.type]));
  el.addEventListener('wk-error', e => window.calls.push(['wk-error', e.detail]));
  host.replaceChildren(el);
  await el.ready;
  return el.id;
}, spec);
// The canvas transform as { x, y, k }.
const view = (page, id) => page.evaluate(i => {
  const m = new DOMMatrix(getComputedStyle(document.querySelector(`#${i} .wk-diagram-canvas`)).transform);
  return { x: m.e, y: m.f, k: m.a };
}, id);
const close = (a, b, eps = 0.5) => Math.abs(a - b) <= eps;

const failures = [];
// WK_CHECK=<text> runs only the checks whose name contains it.
async function check(name, fn) {
  if (process.env.WK_CHECK && !name.includes(process.env.WK_CHECK)) return;
  try {
    await fn();
    console.log(`ok    ${name}`);
  } catch (e) {
    failures.push(name);
    console.log(`FAIL  ${name}\n${String(e.stack || e).split('\n').slice(0, 14).join('\n')}`);
  }
}

try {
  await check('the demo pages draw every diagram type under a strict CSP, with no errors, violations or network (light and dark; neutral and themed)', async () => {
    for (const url of ['/diagram/demo/index.html', '/diagram/demo/themed.html']) {
      for (const scheme of ['light', 'dark']) {
        const { page, close: done } = await open(url, { colorScheme: scheme });
        const look = await page.evaluate(types => ({
          csp: document.querySelector('meta[http-equiv="Content-Security-Policy"]').content,
          drawn: types.map(t => document.getElementById(`demo-${t}`).type),
          svgs: [...document.querySelectorAll('.wk-diagram-canvas > svg')].map(s => ({ role: s.getAttribute('role'), label: s.getAttribute('aria-label'), w: s.getBoundingClientRect().width, texts: s.querySelectorAll('text').length })),
          errors: document.querySelectorAll('.wk-diagram-error:not([hidden]) .wk-errorbox').length,
          styleTags: document.querySelectorAll('style').length,
          adopted: document.adoptedStyleSheets.length,
          font: getComputedStyle(document.querySelector('#demo-flowchart .wk-diagram-canvas text')).fontFamily,
          overflow: document.documentElement.scrollWidth > innerWidth,
        }), TYPES);
        assert.doesNotMatch(look.csp, /unsafe/);
        assert.deepEqual(look.drawn, TYPES, `${url} ${scheme}`);
        assert.equal(look.svgs.length, 6);
        for (const s of look.svgs) {
          assert.equal(s.role, 'img');
          assert.ok(s.label && s.w > 50 && s.texts > 2, JSON.stringify(s));
        }
        assert.equal(look.errors, 2);
        assert.equal(look.styleTags, 0, 'a <style> element is in the page');
        assert.ok(look.adopted >= 7, 'the frame sheet and one sheet per diagram are adopted');
        assert.match(look.font, /^Inter,/);
        assert.equal(look.overflow, false);
        await done();
      }
    }
  });

  await check('accessibility: the SVG is an image named by the label and described by the source; the viewport is focusable and says how to pan', async () => {
    const { page, close: done } = await open();
    const a = await page.evaluate(() => {
      const el = document.getElementById('demo-sequence');
      const svg = el.querySelector('svg');
      const desc = document.getElementById(svg.getAttribute('aria-describedby'));
      const vp = el.querySelector('.wk-diagram-viewport');
      return {
        role: svg.getAttribute('role'), label: svg.getAttribute('aria-label'), desc: desc?.textContent, inDetails: desc?.parentElement.localName,
        summary: el.querySelector('summary').textContent, source: el.source,
        tab: vp.tabIndex, vpLabel: vp.getAttribute('aria-label'), help: document.getElementById(vp.getAttribute('aria-describedby'))?.textContent,
        buttons: [...el.querySelectorAll('.wk-diagram-toolbar button')].map(b => b.getAttribute('aria-label') || b.textContent),
      };
    });
    assert.equal(a.role, 'img');
    assert.equal(a.label, 'Update check');
    assert.equal(a.desc, a.source);
    assert.equal(a.inDetails, 'details');
    assert.equal(a.summary, 'Diagram source');
    assert.equal(a.tab, 0);
    assert.equal(a.vpLabel, 'Update check');
    assert.match(a.help, /arrow keys to pan/);
    assert.deepEqual(a.buttons, ['Zoom out', 'Zoom in', 'Fit to view', 'Actual size', 'Copy source', 'Save as SVG', 'Save as PNG']);
    // A description replaces the source as the text alternative.
    await make(page, { id: 'described', label: 'Two steps', description: 'A goes to B.', source: 'flowchart LR\n A --> B' });
    const d = await page.evaluate(() => {
      const svg = document.querySelector('#described svg');
      return { desc: document.getElementById(svg.getAttribute('aria-describedby')).textContent, summary: document.querySelector('#described summary').textContent };
    });
    assert.deepEqual(d, { desc: 'A goes to B.', summary: 'Description' });
    // label is required.
    const thrown = await page.evaluate(async () => { const { wkDiagram } = await import('/diagram/dist/diagram.js'); try { wkDiagram({ source: 'graph TD\n a' }); return null; } catch (e) { return e.message; } });
    assert.match(thrown, /label/);
    await done();
  });

  await check('a syntax error and an unsupported type show the ErrorBox with Mermaid\'s message and line; onerror and wk-error fire', async () => {
    const { page, close: done } = await open();
    const look = await page.evaluate(() => ['demo-syntax', 'demo-unsupported'].map(id => {
      const box = document.querySelector(`#${id} .wk-diagram-error:not([hidden]) .wk-errorbox`);
      return {
        head: box?.querySelector('.wk-errorbox-head > span').textContent,
        facts: box?.querySelector('.wk-facts')?.textContent ?? null,
        pre: box?.querySelector('pre').textContent,
        canvas: document.querySelector(`#${id} .wk-diagram-canvas`).childElementCount,
        disabled: [...document.querySelectorAll(`#${id} .wk-diagram-toolbar button`)].filter(b => b.disabled).map(b => b.textContent),
        state: document.querySelector(`#${id} .wk-diagram-viewport`).dataset.state,
      };
    }));
    assert.equal(look[0].head, 'Couldn’t draw the diagram');
    assert.equal(look[0].facts, 'Line3');
    assert.match(look[0].pre, /Parse error on line 3/);
    assert.equal(look[1].head, 'Can’t draw this kind of diagram');
    assert.equal(look[1].facts, null);
    assert.match(look[1].pre, /"pie" diagrams are not supported/);
    for (const l of look) {
      assert.equal(l.canvas, 0);
      assert.equal(l.state, 'error');
      assert.deepEqual(l.disabled, ['−', '+', 'Fit', '100%', 'Save SVG', 'Save PNG']);
    }
    // On a new diagram: onerror and wk-error, with the line.
    await make(page, { id: 'bad', label: 'Bad', source: 'sequenceDiagram\n  Alice->>Bob: hi\n  Bob-->>: \n' });
    const calls = await page.evaluate(() => window.calls);
    assert.equal(calls.length, 2);
    assert.equal(calls[0][0], 'wk-error');
    assert.equal(calls[1][0], 'onerror');
    assert.deepEqual(calls[0][1], calls[1][1]);
    assert.equal(calls[0][1].kind, 'syntax');
    assert.equal(calls[0][1].line, 3);
    // Mindmap, ELK flowcharts and an empty source are refused with words.
    for (const [source, want] of [['mindmap\n  root', /"mindmap" diagrams are not supported/], ['flowchart-elk TD\n a --> b', /"flowchart-elk" diagrams are not supported/], ['  ', /The diagram is empty/], ['hello', /No diagram type found/]]) {
      await make(page, { id: 'bad', label: 'Bad', source });
      const c = await page.evaluate(() => window.calls[0]);
      assert.equal(c[0], 'wk-error');
      assert.equal(c[1].kind, 'unsupported');
      assert.match(c[1].message, want);
    }
    // A source that sets an ELK layout in its front matter is still drawn with dagre.
    await make(page, { id: 'ok', label: 'Front matter', source: '---\nconfig:\n  layout: elk\n  theme: forest\n  look: handDrawn\n---\nflowchart LR\n a --> b' });
    assert.deepEqual(await page.evaluate(() => window.calls), [['wk-render', 'flowchart']]);
    await done();
  });

  await check('setting source re-renders; the type follows; a fixed error goes', async () => {
    const { page, close: done } = await open();
    await make(page, { id: 'live', label: 'Live', source: 'flowchart LR\n a --> b' });
    for (const [source, type] of [['sequenceDiagram\n A->>B: hi', 'sequence'], ['gantt\n dateFormat YYYY-MM-DD\n section S\n T :2026-01-01, 2d', 'gantt'], ['flowchart LR\n a -->', null], ['classDiagram\n A <|-- B', 'class']]) {
      const next = nextRender(page, 'live');
      await page.evaluate(s => { document.getElementById('live').source = s; }, source);
      assert.equal(await next, type ? 'wk-render' : 'wk-error');
      const look = await page.evaluate(() => ({ type: document.getElementById('live').type, box: !!document.querySelector('#live .wk-diagram-error:not([hidden])'), svg: !!document.querySelector('#live svg[role="img"]') }));
      assert.deepEqual(look, { type, box: !type, svg: !!type });
    }
    await done();
  });

  await check('theme: the diagram takes the kit tokens and re-renders for data-theme, the system setting and an app theme', async () => {
    for (const url of ['/diagram/demo/index.html', '/diagram/demo/themed.html']) {
      const { page, close: done } = await open(url);
      const look = () => page.evaluate(() => {
        const flow = document.querySelector('#demo-flowchart svg');
        const seq = document.querySelector('#demo-sequence svg');
        return {
          text: getComputedStyle(flow.querySelector('.node text, .label text, text')).fill,
          edge: getComputedStyle(flow.querySelector('.flowchart-link, path.edge')).stroke,
          ground: getComputedStyle(document.querySelector('#demo-flowchart .wk-diagram-viewport')).backgroundColor,
          activation: getComputedStyle(seq.querySelector('[class^="activation"]')).stroke,
          want: { text: window.t.colour('var(--wk-color-text)'), edge: window.t.colour('var(--wk-color-text-secondary)'), ground: window.t.colour('var(--wk-color-surface)'), accent: window.t.colour('var(--wk-color-accent)') },
        };
      });
      const expectations = async label => {
        const l = await look();
        assert.equal(l.text, l.want.text, `${url} ${label} text`);
        assert.equal(l.edge, l.want.edge, `${url} ${label} edges`);
        assert.equal(l.ground, l.want.ground, `${url} ${label} ground`);
        assert.equal(l.activation, l.want.accent, `${url} ${label} activation (accent)`);
        return l;
      };
      const light = await expectations('light');
      // data-theme="dark" re-renders every diagram in the dark tokens.
      let next = nextRender(page, 'demo-flowchart');
      await setTheme(page, 'dark');
      assert.equal(await next, 'wk-render');
      await until(page, () => document.getElementById('demo-sequence').ready);
      await page.evaluate(() => Promise.all([...document.querySelectorAll('.wk-diagram')].map(d => d.ready)));
      const dark = await expectations('data-theme dark');
      assert.notEqual(dark.text, light.text);
      // Back to the system setting, which is dark now.
      await setTheme(page, null);
      await page.emulateMedia({ colorScheme: 'dark' });
      await page.evaluate(() => Promise.all([...document.querySelectorAll('.wk-diagram')].map(d => d.ready)));
      await expectations('system dark');
      next = nextRender(page, 'demo-flowchart');
      await page.emulateMedia({ colorScheme: 'light' });
      assert.equal(await next, 'wk-render');
      await page.evaluate(() => Promise.all([...document.querySelectorAll('.wk-diagram')].map(d => d.ready)));
      await expectations('system light');
      if (url.includes('themed')) assert.equal(light.want.accent, 'rgb(31, 107, 85)', 'the example theme\'s accent');
      await done();
    }
  });

  await check('every text and ground pair reaches 4.5:1 and every line 3:1 with the page\'s tokens (light, dark, app theme)', async () => {
    for (const url of ['/diagram/demo/index.html', '/diagram/demo/themed.html']) {
      for (const scheme of ['light', 'dark']) {
        const { page, close: done } = await open(url, { colorScheme: scheme });
        const bad = await page.evaluate(async () => {
          const { buildTheme, TEXT_PAIRS, MARK_PAIRS, contrast } = await import('/diagram/dist/diagram.js');
          const get = name => (name.startsWith('--wk-color-') ? window.t.colour(`var(${name})`) : getComputedStyle(document.documentElement).getPropertyValue(name));
          const { themeVariables: v } = buildTheme(get);
          return [
            ...TEXT_PAIRS.map(([a, b]) => [a, b, contrast(v[a], v[b]), 4.5]),
            ...MARK_PAIRS.map(([a, b]) => [a, b, contrast(v[a], v[b]), 3]),
          ].filter(([, , r, min]) => !(r >= min));
        });
        assert.deepEqual(bad, [], `${url} ${scheme}`);
        // And in the drawn diagrams: every label's fill against the shape it sits on, where that is a filled rect.
        const drawn = await page.evaluate(async () => {
          const { contrast } = await import('/diagram/dist/diagram.js');
          const out = [];
          for (const g of document.querySelectorAll('.wk-diagram-canvas svg g.node')) {
            const shape = g.querySelector('rect, polygon, path');
            const text = g.querySelector('text');
            if (!shape || !text) continue;
            const fill = getComputedStyle(shape).fill, colour = getComputedStyle(text).fill;
            if (!/^rgb\(/.test(fill)) continue;
            out.push([g.closest('.wk-diagram').id, contrast(colour, fill)]);
          }
          return out;
        });
        assert.ok(drawn.length > 10);
        for (const [id, ratio] of drawn) assert.ok(ratio >= 4.5, `${url} ${scheme} ${id}: a node label is ${ratio.toFixed(2)}:1`);
        await done();
      }
    }
  });

  await check('pan and zoom: drag pans; the wheel zooms around the pointer; buttons zoom, fit and show 100%', async () => {
    const { page, close: done } = await open();
    await make(page, { id: 'pz', label: 'Pan and zoom', source: 'flowchart LR\n a[Start] --> b[Middle] --> c[End]' });
    const id = 'pz';
    const box = await page.locator(`#${id} .wk-diagram-viewport`).boundingBox();
    const v0 = await view(page, id);
    // Drag.
    await page.mouse.move(box.x + 100, box.y + 100);
    await page.mouse.down();
    await page.mouse.move(box.x + 160, box.y + 130, { steps: 4 });
    assert.equal(await page.evaluate(i => document.querySelector(`#${i} .wk-diagram-viewport`).hasAttribute('data-dragging'), id), true);
    await page.mouse.up();
    const v1 = await view(page, id);
    assert.ok(close(v1.x - v0.x, 60) && close(v1.y - v0.y, 30) && v1.k === v0.k, JSON.stringify([v0, v1]));
    // Wheel up zooms in around the pointer: the diagram point under it stays put.
    const px = 300, py = 150;
    const before = { x: (px - v1.x) / v1.k, y: (py - v1.y) / v1.k };
    await page.mouse.move(box.x + px, box.y + py);
    await page.mouse.wheel(0, -200);
    await until(page, ([i, k]) => new DOMMatrix(getComputedStyle(document.querySelector(`#${i} .wk-diagram-canvas`)).transform).a > k, [id, v1.k]);
    const v2 = await view(page, id);
    assert.ok(close((px - v2.x) / v2.k, before.x, 1) && close((py - v2.y) / v2.k, before.y, 1), JSON.stringify({ before, v2 }));
    // A pinch on a trackpad (a wheel with ctrlKey) zooms out too.
    await page.keyboard.down('Control');
    await page.mouse.wheel(0, 40);
    await page.keyboard.up('Control');
    await until(page, ([i, k]) => new DOMMatrix(getComputedStyle(document.querySelector(`#${i} .wk-diagram-canvas`)).transform).a < k, [id, v2.k]);
    // Buttons.
    const k = async () => (await view(page, id)).k;
    await page.click(`#${id} button[aria-label="Actual size"]`);
    assert.ok(close(await k(), 1, 1e-6));
    assert.equal(await page.textContent(`#${id} .wk-diagram-zoom`), '100%');
    await page.click(`#${id} button[aria-label="Zoom in"]`);
    assert.ok(close(await k(), 1.25, 1e-6));
    await page.click(`#${id} button[aria-label="Zoom out"]`);
    await page.click(`#${id} button[aria-label="Zoom out"]`);
    assert.ok(close(await k(), 0.8, 1e-6));
    assert.equal(await page.textContent(`#${id} .wk-diagram-zoom`), '80%');
    // The limits disable the buttons.
    await page.evaluate(i => document.getElementById(i).zoomTo(100), id);
    assert.ok(close(await k(), 8, 1e-6));
    assert.equal(await page.isDisabled(`#${id} button[aria-label="Zoom in"]`), true);
    await page.click(`#${id} button[aria-label="Fit to view"]`);
    assert.ok((await k()) <= 1);
    await done();
  });

  await check('pinch with two touch points zooms', async () => {
    const { page, close: done } = await open();
    await make(page, { id: 'pinch', label: 'Pinch', source: 'flowchart LR\n a --> b --> c' });
    const k0 = (await view(page, 'pinch')).k;
    await page.evaluate(() => {
      const vp = document.querySelector('#pinch .wk-diagram-viewport');
      const r = vp.getBoundingClientRect();
      const ev = (type, id, x) => vp.dispatchEvent(new PointerEvent(type, { pointerId: id, pointerType: 'touch', clientX: r.left + x, clientY: r.top + 100, bubbles: true, isPrimary: id === 1 }));
      ev('pointerdown', 1, 200); ev('pointerdown', 2, 300);
      ev('pointermove', 1, 150); ev('pointermove', 2, 350);
      ev('pointerup', 1, 150); ev('pointerup', 2, 350);
    });
    const k1 = (await view(page, 'pinch')).k;
    assert.ok(close(k1 / k0, 2, 0.01), `pinch from 100px to 200px apart: ${k0} -> ${k1}`);
    await done();
  });

  await check('keyboard: with focus, arrow keys pan, + and - zoom, 0 fits; keys elsewhere do nothing', async () => {
    const { page, close: done } = await open();
    await make(page, { id: 'kb', label: 'Keyboard', source: 'flowchart LR\n a --> b --> c' });
    const fitted = await view(page, 'kb');
    await page.focus('#kb .wk-diagram-viewport');
    await page.keyboard.press('ArrowRight');
    await page.keyboard.press('ArrowDown');
    let v = await view(page, 'kb');
    assert.ok(close(v.x, fitted.x - 40) && close(v.y, fitted.y - 40), JSON.stringify([fitted, v]));
    await page.keyboard.press('Shift+ArrowLeft');
    v = await view(page, 'kb');
    assert.ok(close(v.x, fitted.x + 120));
    await page.keyboard.press('+');
    assert.ok(close((await view(page, 'kb')).k, fitted.k * 1.25, 1e-6));
    await page.keyboard.press('-');
    await page.keyboard.press('-');
    assert.ok(close((await view(page, 'kb')).k, fitted.k / 1.25, 1e-6));
    await page.keyboard.press('0');
    assert.deepEqual(await view(page, 'kb'), fitted);
    // Focus on a toolbar button: arrow keys are the page's.
    await page.focus('#kb button[aria-label="Zoom in"]');
    await page.keyboard.press('ArrowRight');
    assert.deepEqual(await view(page, 'kb'), fitted);
    await done();
  });

  await check('fit: starts fitted and centred, never above 100%; follows its container while fitted, not after a pan', async () => {
    const { page, close: done } = await open();
    const geometry = id => page.evaluate(i => {
      const vp = document.querySelector(`#${i} .wk-diagram-viewport`).getBoundingClientRect();
      const s = document.querySelector(`#${i} svg`).getBoundingClientRect();
      return { left: s.left - vp.left, right: vp.right - s.right, top: s.top - vp.top, bottom: vp.bottom - s.bottom };
    }, id);
    // A large diagram: fitted inside the viewport, centred.
    const g = await geometry('demo-state');
    for (const side of Object.values(g)) assert.ok(side >= 15, JSON.stringify(g));
    assert.ok(close(g.left, g.right, 1) || close(g.top, g.bottom, 1), JSON.stringify(g));
    // A small one: 100%, centred.
    await make(page, { id: 'small', label: 'Small', source: 'flowchart LR\n a --> b' });
    assert.equal((await view(page, 'small')).k, 1);
    const s = await geometry('small');
    assert.ok(close(s.left, s.right, 1) && close(s.top, s.bottom, 1), JSON.stringify(s));
    // The container shrinks: the fitted diagram follows.
    const k0 = (await view(page, 'demo-state')).k;
    await page.evaluate(() => { document.getElementById('demo-state').style.height = '240px'; });
    await until(page, k => new DOMMatrix(getComputedStyle(document.querySelector('#demo-state .wk-diagram-canvas')).transform).a < k, k0);
    for (const side of Object.values(await geometry('demo-state'))) assert.ok(side >= 15);
    // After a pan, a resize keeps the view.
    await page.focus('#demo-state .wk-diagram-viewport');
    await page.keyboard.press('ArrowLeft');
    const panned = await view(page, 'demo-state');
    await page.evaluate(() => { document.getElementById('demo-state').style.height = '300px'; });
    await page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))));
    assert.deepEqual(await view(page, 'demo-state'), panned);
    await done();
  });

  await check('export: the SVG is a sanitized copy with its styles and no scripts or external references; the PNG is an image', async () => {
    const { page, close: done } = await open();
    // A source that tries links, a click callback and CSS that loads things.
    await make(page, { id: 'ex', label: 'Export me', source: '%%{init: {"themeCSS": ".node rect { fill: url(evil.png) } @import \'evil.css\';", "fontFamily": "url(evil.woff)"}}%%\nflowchart LR\n a[Start] --> b[End]\n click a "https://example.com" "Go"\n click b call alert(1)\n style a fill:#f9f,stroke:#333\n classDef c stroke-dasharray:4\n class b c' });
    const failed = await page.evaluate(() => window.calls.find(c => c[0] === 'wk-error'));
    assert.equal(failed, undefined);
    const out = await page.evaluate(() => {
      const el = document.getElementById('ex');
      const text = el.toSVG();
      const live = el.querySelector('svg');
      return {
        text,
        liveLinks: live.querySelectorAll('a').length,
        liveUrls: [...live.querySelectorAll('[style]')].map(e => e.getAttribute('style')).filter(s => /url\(/.test(s)),
        sheetUrls: [...document.adoptedStyleSheets].flatMap(s => [...s.cssRules].map(r => r.cssText)).filter(t => /url\((?!["']?#)/.test(t)),
      };
    });
    // Parsed as a file would be, on a blank page (the demo's CSP would
    // report the copy's <style> element and style attributes).
    const blank = await page.context().newPage();
    Object.assign(out, await blank.evaluate(text => {
      const doc = new DOMParser().parseFromString(text, 'image/svg+xml');
      return {
        parseError: !!doc.querySelector('parsererror'), root: doc.documentElement.localName, ns: doc.documentElement.namespaceURI,
        title: doc.querySelector('title')?.textContent, desc: doc.querySelector('desc')?.textContent, style: doc.querySelector('style')?.textContent.length ?? 0,
      };
    }, out.text));
    await blank.close();
    assert.equal(out.parseError, false);
    assert.equal(out.root, 'svg');
    assert.equal(out.ns, 'http://www.w3.org/2000/svg');
    assert.equal(out.title, 'Export me');
    assert.match(out.desc, /flowchart LR/);
    // The description is the source as text; the rest is the drawing.
    const drawing = out.text.replace(/<desc>[\s\S]*<\/desc>/, '');
    assert.ok(out.style > 500, 'the diagram\'s styles travel with it');
    assert.doesNotMatch(drawing, /<script|<foreignObject|<image|<iframe|\son\w+=|javascript:/i);
    assert.doesNotMatch(drawing, /(?:href|src)="(?!#)/i, 'an external reference');
    assert.doesNotMatch(drawing, /url\((?!["']?#)/i, 'a CSS url() that is not a fragment');
    assert.doesNotMatch(drawing, /example\.com|evil\./);
    assert.equal(out.liveLinks, 0);
    assert.deepEqual(out.liveUrls, []);
    assert.deepEqual(out.sheetUrls, []);
    // The buttons hand the file to onexport.
    await page.click('#ex button[aria-label="Save as SVG"]');
    await page.click('#ex button[aria-label="Save as PNG"]');
    await until(page, () => window.calls.filter(c => c[0] === 'onexport').length === 2);
    const exports = await page.evaluate(() => window.calls.filter(c => c[0] === 'onexport'));
    assert.deepEqual(exports.map(e => e.slice(0, 4)), [['onexport', 'svg', 'export-me.svg', 'image/svg+xml'], ['onexport', 'png', 'export-me.png', 'image/png']]);
    assert.ok(exports[1][4] > 1000);
    // toPNG draws at the scale asked for.
    const size = await page.evaluate(async () => {
      const blob = await document.getElementById('ex').toPNG(1);
      const bitmap = await createImageBitmap(blob);
      const svg = document.querySelector('#ex svg');
      return { w: bitmap.width, h: bitmap.height, sw: Math.ceil(svg.viewBox.baseVal.width), sh: Math.ceil(svg.viewBox.baseVal.height) };
    });
    assert.deepEqual([size.w, size.h], [size.sw, size.sh]);
    // Without onexport, Save downloads the file.
    const { context } = await (async () => ({ context: page.context() }))();
    await page.evaluate(async () => {
      const { wkDiagram } = await import('/diagram/dist/diagram.js');
      const el = wkDiagram({ id: 'dl', label: 'Download me', source: 'flowchart LR\n a --> b', filename: 'my-diagram' });
      document.getElementById('demo').prepend(el);
      await el.ready;
    });
    const [download] = await Promise.all([page.waitForEvent('download'), page.click('#dl button[aria-label="Save as SVG"]')]);
    assert.equal(download.suggestedFilename(), 'my-diagram.svg');
    void context;
    await done();
  });

  await check('copy source: the clipboard gets the source and "Source copied" shows for 2 s', async () => {
    const { page, close: done, context } = await open();
    await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin });
    await page.click('#demo-er button:text("Copy source")');
    await until(page, () => document.querySelector('#demo-er .wk-diagram-note').textContent === 'Source copied');
    assert.equal(await page.getAttribute('#demo-er .wk-diagram-note', 'role'), 'status');
    const [clip, source] = await page.evaluate(async () => [await navigator.clipboard.readText(), document.getElementById('demo-er').source]);
    assert.equal(clip, source);
    await until(page, () => document.querySelector('#demo-er .wk-diagram-note').textContent === '', null, 3000);
    await done();
  });

  await check('loading: Mermaid loads once, on first use; a quick render shows no indicator; a slow one shows it after 400 ms, then the loading line after 3 s', async () => {
    const { page, close: done } = await open('/diagram/demo/index.html', {}, {
      wait: false,
      before: async p => {
        p.loads = 0;
        await p.route('**/diagram/dist/mermaid.js', async route => {
          p.loads++;
          await new Promise(r => setTimeout(r, 3600));
          await route.continue();
        });
      },
    });
    await until(page, () => document.querySelectorAll('.wk-diagram').length === 8);
    const status = () => page.evaluate(() => {
      const s = document.querySelector('#demo-flowchart .wk-diagram-status');
      return { hidden: s.hidden, role: s.getAttribute('role'), name: s.getAttribute('aria-label'), line: s.querySelector('span:not(.wk-diagram-spinner)').hidden ? '' : s.textContent, state: document.querySelector('#demo-flowchart .wk-diagram-viewport').dataset.state };
    });
    // The quiet wait.
    assert.deepEqual(await status(), { hidden: true, role: 'progressbar', name: 'Drawing the diagram', line: '', state: 'loading' });
    await until(page, () => !document.querySelector('#demo-flowchart .wk-diagram-status').hidden, null, 1000);
    const t = await page.evaluate(() => performance.now());
    assert.equal((await status()).line, '');
    await until(page, () => !document.querySelector('#demo-flowchart .wk-diagram-status span:not(.wk-diagram-spinner)').hidden, null, 4000);
    assert.equal((await status()).line, 'Loading the diagram library…');
    await page.waitForFunction(() => window.demoReady === true, null, { timeout: 30000 });
    assert.equal((await status()).hidden, true);
    assert.ok(await page.evaluate(() => performance.now()) - t > 500);
    assert.equal(page.loads, 1, 'mermaid.js loads once for eight diagrams');
    // Mermaid is loaded: a new diagram draws within the quiet wait, with no indicator.
    const shown = await page.evaluate(async () => {
      const { wkDiagram } = await import('/diagram/dist/diagram.js');
      const el = wkDiagram({ label: 'Quick', source: 'flowchart LR\n a --> b' });
      document.getElementById('demo').prepend(el);
      let seen = false;
      const obs = new MutationObserver(() => { if (!el.querySelector('.wk-diagram-status').hidden) seen = true; });
      obs.observe(el, { attributes: true, subtree: true });
      const t0 = performance.now();
      await el.ready;
      obs.disconnect();
      return { seen, ms: performance.now() - t0 };
    });
    if (shown.ms < 400) assert.equal(shown.seen, false);
    await done();
  });

  await check('forced colours: the diagram is drawn in CanvasText on Canvas', async () => {
    const { page, close: done } = await open('/diagram/demo/index.html', { forcedColors: 'active' });
    const look = await page.evaluate(() => {
      const canvasText = window.t.colour('CanvasText'), canvas = window.t.colour('Canvas');
      const out = { canvasText, canvas, texts: new Set(), strokes: new Set(), nodeFills: new Set() };
      for (const svg of document.querySelectorAll('.wk-diagram-canvas svg')) {
        for (const t of svg.querySelectorAll('text')) out.texts.add(getComputedStyle(t).fill);
        for (const p of svg.querySelectorAll('.flowchart-link, .messageLine0, .messageLine1, .transition, .relationshipLine')) out.strokes.add(getComputedStyle(p).stroke);
        for (const r of svg.querySelectorAll('g.node > rect, rect.actor')) out.nodeFills.add(getComputedStyle(r).fill);
      }
      out.viewport = getComputedStyle(document.querySelector('.wk-diagram-viewport')).borderTopColor;
      return { ...out, texts: [...out.texts], strokes: [...out.strokes], nodeFills: [...out.nodeFills] };
    });
    assert.deepEqual(look.texts, [look.canvasText]);
    assert.deepEqual(look.strokes, [look.canvasText]);
    assert.deepEqual(look.nodeFills, [look.canvas]);
    assert.equal(look.viewport, look.canvasText);
    // Leaving forced colours re-renders in the tokens.
    const next = nextRender(page, 'demo-flowchart');
    await page.emulateMedia({ forcedColors: 'none' });
    assert.equal(await next, 'wk-render');
    await done();
  });

  await check('destroy() removes the diagram and its stylesheet; a destroyed diagram ignores source', async () => {
    const { page, close: done } = await open();
    await make(page, { id: 'gone', label: 'Gone', source: 'flowchart LR\n a --> b' });
    const after = await page.evaluate(async () => {
      const el = document.getElementById('gone');
      const before = document.adoptedStyleSheets.length;
      el.destroy(); el.destroy();
      el.source = 'flowchart LR\n c --> d';
      await new Promise(r => setTimeout(r, 50));
      return { children: el.childElementCount, sheets: before - document.adoptedStyleSheets.length, source: el.source };
    });
    assert.deepEqual(after, { children: 0, sheets: 1, source: 'flowchart LR\n a --> b' });
    await done();
  });
} finally { await browser.close(); await new Promise(resolve => server.close(resolve)); }

if (failures.length) {
  console.log(`Diagram browser checks: ${failures.length} failed.`);
  process.exit(1);
}
console.log('Diagram browser checks passed.');
