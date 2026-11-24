// What the gallery's browser checks share: the gallery's own server, Chrome,
// a page that records every problem, the fake clock, and a check that
// reports on its own line. test/browser.mjs (the gallery itself) and
// test/components.mjs (the kit's components, on the gallery's pages) use it.
//
// Chrome comes from CHROME_PATH, /usr/bin/google-chrome or the macOS default.
// Without it the checks skip, unless CI is set, when they fail.
import { existsSync } from 'node:fs';
import { createRequire } from 'node:module';
import { createServer, listen } from '../serve.mjs';

const require = createRequire(import.meta.url);
const { chromium } = require(process.argv[2] || 'playwright-core');
const chrome = [process.env.CHROME_PATH, '/usr/bin/google-chrome', '/usr/bin/google-chrome-stable', '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'].find(p => p && existsSync(p));
if (!chrome) {
  console.log(`${process.argv[1]}: Chrome not found; set CHROME_PATH. Browser checks skipped.`);
  process.exit(process.env.CI ? 1 : 0);
}
export const server = createServer();
export const origin = await listen(server);
export const browser = await chromium.launch({ executablePath: chrome, headless: true });

// clock: the page's timers run on Playwright's fake clock (page.clock), for
// timing checks. It starts at CLOCK_T0 and runs with real time until a check
// stops it with pauseClock, at a fixed point a minute on. Never pause at a
// time read from the page (Date.now() + n): the clock keeps running between
// that read and the pause, so under load the point can already be past
// ("Cannot fast-forward to the past"), and the page's own timers can fire
// before the check expects them to.
export const CLOCK_T0 = Date.parse('2026-01-05T09:00:00Z');
export const pauseClock = page => page.clock.pauseAt(CLOCK_T0 + 60_000);

// A gallery page is ready when <html data-gallery-ready> names its hash.
export const ready = (page, hash) => page.waitForFunction(h => document.documentElement.dataset.galleryReady === h, hash, { timeout: 20000 });
export async function go(page, hash) {
  await page.evaluate(h => { location.hash = h; }, hash);
  await ready(page, hash);
}
// A "Try it" run has settled when window.galleryPending is back to 0.
export const settled = page => page.waitForFunction(() => window.galleryPending === 0, null, { timeout: 20000 });

// Opens path on the gallery's server in a new page. Any request outside the
// server, any console error or warning (a CSP violation among them), any
// failed request and any HTTP error goes to problems. init scripts run before
// the page's own; clock installs the fake clock at CLOCK_T0; ready: false
// returns once the document has loaded, without waiting for the gallery;
// blob: true allows blob: URLs too (the gallery's own checks, for the
// editors and diagrams it draws), which the component checks don't.
export async function open(path = '/gallery/', { clock, init = [], ready: wait = true, blob = false, ...options } = {}) {
  const page = await browser.newPage({ viewport: { width: 1200, height: 900 }, colorScheme: 'light', ...options });
  for (const script of init) await page.addInitScript(script);
  if (clock) await page.clock.install({ time: CLOCK_T0 });
  const problems = [];
  page.on('pageerror', e => problems.push(`pageerror: ${e.message}`));
  page.on('console', m => { if (m.type() === 'error' || m.type() === 'warning') problems.push(`console ${m.type()}: ${m.text()}`); });
  page.on('request', r => { if (!r.url().startsWith(`${origin}/`) && !r.url().startsWith('data:') && !(blob && r.url().startsWith('blob:'))) problems.push(`network: ${r.url()}`); });
  page.on('requestfailed', r => problems.push(`failed: ${r.url()}`));
  page.on('response', r => { if (r.status() >= 400) problems.push(`HTTP ${r.status()}: ${r.url()}`); });
  await page.goto(origin + path);
  if (wait && path.startsWith('/gallery/')) await ready(page, new URL(origin + path).hash || '#/');
  return { page, problems };
}

// CHECK=<regular expression> runs only the checks whose names match it.
const only = process.env.CHECK ? new RegExp(process.env.CHECK) : null;
export const failures = [];
export async function check(name, fn) {
  if (only && !only.test(name)) return;
  try {
    await fn();
    console.log(`ok    ${name}`);
  } catch (e) {
    failures.push(name);
    console.log(`FAIL  ${name}\n${String(e.stack || e).split('\n').slice(0, 14).join('\n')}`);
  }
}

// Closes the browser and the server, and fails the run if a check failed.
export async function finish() {
  await browser.close();
  server.close();
  if (failures.length) {
    console.log(`\n${failures.length} failed:\n${failures.map(f => `  ${f}`).join('\n')}`);
    process.exit(1);
  }
}
