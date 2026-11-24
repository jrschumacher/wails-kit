// Browser checks that must stay in the package: its files on their own, as
// an app vendors them. The server serves this folder and nothing else (it
// refuses ".."), plus two pages it writes itself under the demo pages'
// strict CSP: /test/page/src.html (src/ui.css and src/index.js) and
// /test/page/dist.html (the prebuilt dist/ files). Every component is drawn
// and checked in the component gallery (frontend/gallery, test/components.mjs).
// Run with: npm run test:browser
// It needs playwright-core (a devDependency) and Chrome: set CHROME_PATH, or
// have Chrome at /usr/bin/google-chrome or the macOS default. Without Chrome
// it skips, unless CI is set, when it fails.
import assert from 'node:assert/strict';
import http from 'node:http';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';
import path from 'node:path';
const require = createRequire(import.meta.url);
import { existsSync } from 'node:fs';
const { chromium } = require(process.argv[2] || 'playwright-core');
const root = fileURLToPath(new URL('../', import.meta.url));
const chrome = [process.env.CHROME_PATH, '/usr/bin/google-chrome', '/usr/bin/google-chrome-stable', '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'].find(p => p && existsSync(p));
if (!chrome) {
  console.log('test/browser.mjs: Chrome not found; set CHROME_PATH. Browser checks skipped.');
  process.exit(process.env.CI ? 1 : 0);
}
const CSP = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; form-action 'none'";
// A page that loads the kit from src/ or dist/, relative to itself, as a vendored copy is.
const page = from => `<!doctype html>
<html lang="en" class="wk-root">
<head>
  <meta charset="utf-8">
  <meta http-equiv="Content-Security-Policy" content="${CSP}">
  <link rel="icon" href="data:,">
  <title>@wails-kit/ui from ${from}/</title>
  <link rel="stylesheet" href="${from === 'dist' ? '../../dist/wails-kit-ui.css' : '../../src/ui.css'}">
  <script type="module" src="${from === 'dist' ? '../../dist/wails-kit-ui.js' : '../../src/index.js'}"></script>
</head>
<body><main><h1 class="wk-title">The kit from ${from}/</h1><p>Text in the kit's font.</p></main></body>
</html>`;
const types = { '.html': 'text/html', '.js': 'text/javascript', '.mjs': 'text/javascript', '.css': 'text/css', '.woff2': 'font/woff2', '.svg': 'image/svg+xml', '.png': 'image/png' };
const server = http.createServer(async (req, res) => {
  try {
    const pathname = new URL(req.url, 'http://localhost').pathname;
    if (pathname.includes('..')) { res.writeHead(404).end(); return; }
    const own = /^\/test\/page\/(src|dist)\.html$/.exec(pathname);
    const data = own ? page(own[1]) : await readFile(path.join(root, pathname));
    res.setHeader('Content-Type', types[path.extname(pathname)] || 'application/octet-stream');
    res.end(data);
  } catch { res.writeHead(404).end(); }
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
const origin = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.launch({ executablePath: chrome, headless: true });

// Opens a page; problems gets every request outside the server, console
// error or warning, failed request and HTTP error.
async function open(url, options = {}) {
  const page = await browser.newPage({ viewport: { width: 1100, height: 800 }, colorScheme: 'light', ...options });
  const problems = [];
  page.on('pageerror', e => problems.push(`pageerror: ${e.message}`));
  page.on('console', m => { if (m.type() === 'error' || m.type() === 'warning') problems.push(`console ${m.type()}: ${m.text()}`); });
  page.on('request', r => { if (!r.url().startsWith(origin + '/') && !r.url().startsWith('data:')) problems.push(`network: ${r.url()}`); });
  page.on('requestfailed', r => problems.push(`failed: ${r.url()}`));
  page.on('response', r => { if (r.status() >= 400) problems.push(`HTTP ${r.status()}: ${r.url()}`); });
  await page.goto(origin + url);
  await page.waitForFunction(() => customElements.get('wk-dialog'));
  await page.evaluate(() => document.fonts.ready);
  return { page, problems };
}

const failures = [];
async function check(name, fn) {
  try {
    await fn();
    console.log(`ok    ${name}`);
  } catch (e) {
    failures.push(name);
    console.log(`FAIL  ${name}\n${String(e.stack || e).split('\n').slice(0, 12).join('\n')}`);
  }
}

try {
  await check('Inter: a wk-root page with no network loads the bundled Inter, from src/ and from dist/, with cv11 and ss03 on; latin-ext loads only when used', async () => {
    for (const url of ['/test/page/src.html', '/test/page/dist.html']) {
      const { page, problems } = await open(url);
      const fonts = await page.evaluate(async () => {
        await document.fonts.ready;
        const loaded = () => [...document.fonts].filter(f => f.family.replace(/"/g, '') === 'Inter' && f.status === 'loaded').length;
        const before = loaded();
        const files = () => performance.getEntriesByType('resource').map(e => new URL(e.name).pathname).filter(n => n.endsWith('.woff2')).sort();
        const fetchedBefore = files();
        const check = document.fonts.check('13px Inter');
        const root = getComputedStyle(document.querySelector('.wk-root'));
        const p = document.createElement('p');
        p.textContent = 'Łódź, Kraków, Þórsmörk';
        document.body.append(p);
        await document.fonts.load('13px Inter', p.textContent);
        const after = loaded();
        p.remove();
        return { check, before, after, fetchedBefore, fetchedAfter: files(), features: root.fontFeatureSettings, family: root.fontFamily };
      });
      assert.equal(fonts.check, true, `${url}: document.fonts.check('13px Inter')`);
      assert.equal(fonts.before, 1, `${url}: only the latin file loads for latin text`);
      assert.equal(fonts.after, 2, `${url}: the latin-ext file loads for "Łódź"`);
      // The package's own files, not a font installed on the machine.
      const dir = url.includes('dist') ? '/dist/fonts/' : '/fonts/';
      assert.deepEqual(fonts.fetchedBefore, [`${dir}Inter-latin.woff2`]);
      assert.deepEqual(fonts.fetchedAfter, [`${dir}Inter-latin-ext.woff2`, `${dir}Inter-latin.woff2`]);
      assert.equal(fonts.features, '"cv11", "ss03"');
      assert.match(fonts.family, /^Inter,/);
      // problems include any request that left the local server.
      assert.deepEqual(problems, [], url);
      await page.close();
    }
  });

  await check('Inter: the bundled files carry cv11 and ss03, so --wk-font-features changes what renders', async () => {
    for (const file of ['/fonts/Inter-latin.woff2', '/dist/fonts/Inter-latin.woff2']) {
      const { page, problems } = await open('/test/page/src.html');
      // Loaded under its own family name, so an Inter installed on the
      // machine can't stand in for the bundled file.
      await page.evaluate(async file => {
        const face = new FontFace('WkFeatureCheck', `url(${file})`);
        document.fonts.add(await face.load());
        const box = document.createElement('div');
        box.id = 'feature-check';
        box.style.cssText = 'position: fixed; top: 0; left: 0; background: #fff; color: #000; z-index: 99';
        for (const [id, text, features] of [['a', 'aaaaaaaaaa', 'normal'], ['a-cv11', 'aaaaaaaaaa', '"cv11"'], ['q', ',,“”,,', 'normal'], ['q-ss03', ',,“”,,', '"ss03"']]) {
          const s = document.createElement('span');
          s.id = `feature-${id}`;
          s.style.cssText = `display: inline-block; font: 40px WkFeatureCheck; font-feature-settings: ${features}; white-space: pre; padding: 4px`;
          s.textContent = text;
          box.append(s, document.createElement('br'));
        }
        document.body.append(box);
      }, file);
      const width = id => page.locator(`#feature-${id}`).evaluate(el => el.getBoundingClientRect().width);
      // cv11, the single-storey a, has its own advance width.
      assert.notEqual(await width('a-cv11'), await width('a'), `${file}: cv11 changes a run of "a"`);
      // ss03, round quotes and commas: commas keep their width, so compare pixels.
      const shot = id => page.locator(`#feature-${id}`).screenshot();
      assert.ok(!(await shot('q-ss03')).equals(await shot('q')), `${file}: ss03 changes quotes and commas`);
      assert.deepEqual(problems, [], file);
      await page.close();
    }
  });

  await check('dist/wails-kit-ui.js exports what src/index.js exports, and defines the same elements', async () => {
    const { page, problems } = await open('/test/page/dist.html');
    const keys = await page.evaluate(async () => {
      const dist = await import('/dist/wails-kit-ui.js');
      return { dist: Object.keys(dist).sort(), elements: ['wk-dialog', 'wk-menu', 'wk-context-menu', 'wk-tabs', 'wk-tooltip', 'wk-settings'].map(n => Boolean(customElements.get(n))) };
    });
    const src = await import('node:fs').then(fs => [...fs.readFileSync(new URL('../src/index.js', import.meta.url), 'utf8').matchAll(/^export\s*\{([^}]*)\}/gm)].flatMap(m => m[1].split(',').map(n => n.trim()).filter(Boolean)).sort());
    assert.deepEqual(keys.dist, src);
    assert.deepEqual(keys.elements, [true, true, true, true, true, true]);
    assert.deepEqual(problems, []);
    await page.close();
  });
} finally { await browser.close(); await new Promise(resolve => server.close(resolve)); }

if (failures.length) {
  console.log(`Kit browser checks: ${failures.length} failed.`);
  process.exit(1);
}
console.log('Kit browser checks passed.');
