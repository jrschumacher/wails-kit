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
const types = { '.html': 'text/html', '.js': 'text/javascript', '.mjs': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png' };
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
  await check('system font: a wk-root page, from src/ and from dist/, sets its text in system-ui and loads no font file, even for extended Latin', async () => {
    for (const url of ['/test/page/src.html', '/test/page/dist.html']) {
      const { page, problems } = await open(url);
      const fonts = await page.evaluate(async () => {
        const p = document.createElement('p');
        p.textContent = 'Łódź, Kraków, Þórsmörk';
        document.body.append(p);
        await document.fonts.ready;
        const body = getComputedStyle(document.body);
        const resources = performance.getEntriesByType('resource').map(e => new URL(e.name).pathname).filter(n => !/\.(css|m?js)$/.test(n));
        return { faces: document.fonts.size, resources, family: body.fontFamily, features: body.fontFeatureSettings };
      });
      assert.match(fonts.family, /^system-ui,/, `${url}: the body's font`);
      assert.equal(fonts.features, 'normal', `${url}: no font features`);
      assert.equal(fonts.faces, 0, `${url}: no @font-face`);
      assert.deepEqual(fonts.resources, [], `${url}: loads nothing but the stylesheet and the module`);
      // problems include any request that left the local server.
      assert.deepEqual(problems, [], url);
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
