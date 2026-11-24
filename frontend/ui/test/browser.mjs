// Browser checks for the kit's custom elements and the settings window, on
// the review pages in demo/ (index.html, themed.html, dist.html,
// settings.html). Run with: npm run test:browser
// It needs playwright-core (a devDependency) and Chrome: set CHROME_PATH, or
// have Chrome at /usr/bin/google-chrome or the macOS default. Without Chrome
// it skips, unless CI is set, when it fails.
//
// Each check runs on its own page and reports on its own line, so one
// failure does not hide the rest. Nothing waits for wall-clock time: every
// wait is for a condition (an event, an element's state).
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
const types = { '.html': 'text/html', '.js': 'text/javascript', '.mjs': 'text/javascript', '.css': 'text/css', '.woff2': 'font/woff2', '.svg': 'image/svg+xml', '.png': 'image/png' };
const server = http.createServer(async (req, res) => {
  try {
    const pathname = new URL(req.url, 'http://localhost').pathname;
    if (pathname.includes('..')) { res.writeHead(404).end(); return; }
    // /editor/ serves @wails-kit/editor's built modules (../editor/dist), for checks with the kit's editors.
    const file = pathname.startsWith('/editor/') ? path.join(root, '..', 'editor', 'dist', pathname.slice('/editor/'.length)) : path.join(root, pathname);
    const data = await readFile(file);
    res.setHeader('Content-Type', types[path.extname(pathname)] || 'application/octet-stream');
    res.end(data);
  } catch { res.writeHead(404).end(); }
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
const origin = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.launch({ executablePath: chrome, headless: true });

async function open(url = '/demo/index.html', { ready, clock, ...options } = {}) {
  const page = await browser.newPage({ viewport: { width: 1100, height: 800 }, colorScheme: 'light', ...options });
  // clock: the page's timers run on Playwright's fake clock (page.clock), for timing checks.
  if (clock) await page.clock.install();
  const problems = [];
  page.on('pageerror', e => problems.push(`pageerror: ${e.message}`));
  page.on('console', m => { if (m.type() === 'error' || m.type() === 'warning') problems.push(`console ${m.type()}: ${m.text()}`); });
  page.on('request', r => { if (!r.url().startsWith(origin + '/') && !r.url().startsWith('data:')) problems.push(`network: ${r.url()}`); });
  page.on('requestfailed', r => problems.push(`failed: ${r.url()}`));
  page.on('response', r => { if (r.status() >= 400) problems.push(`HTTP ${r.status()}: ${r.url()}`); });
  await page.goto(origin + url);
  if (ready === false) return { page, problems };
  await page.waitForFunction(() => customElements.get('wk-dialog') && document.querySelector('#demo-values dt'));
  await page.evaluate(() => document.fonts.ready);
  return { page, problems };
}
// The focused element: its id when the page gave it one, else its text, or
// BODY when nothing has focus.
const active = page => page.evaluate(() => {
  const a = document.activeElement;
  if (!a || a === document.body) return 'BODY';
  return a.id && !a.id.startsWith('wk-') ? a.id : a.textContent.trim();
});
const events = (page, type) => page.evaluate(t => window.kitEvents.filter(e => e.type === t), type);
// A dialog's close event is queued, so wait for the count first.
const closes = async (page, n) => { await page.waitForFunction(m => window.kitEvents.filter(e => e.type === 'wk-close').length === m, n); return events(page, 'wk-close'); };
const isOpen = (page, id) => page.evaluate(i => document.getElementById(i).open, id);
// Waits for a condition, failing fast instead of after Playwright's 30 s.
const until = (page, fn, arg) => page.waitForFunction(fn, arg, { timeout: 3000 });
// Event listeners on the objects the expressions name, from the DevTools protocol.
async function listeners(page, expressions) {
  const cdp = await page.context().newCDPSession(page);
  const out = {};
  for (const expression of expressions) {
    const { result } = await cdp.send('Runtime.evaluate', { expression });
    out[expression] = (await cdp.send('DOMDebugger.getEventListeners', { objectId: result.objectId })).listeners.map(l => `${l.type}${l.useCapture ? '/capture' : ''}`).sort();
  }
  await cdp.detach();
  return out;
}
async function axTree(page) {
  const cdp = await page.context().newCDPSession(page);
  const { nodes } = await cdp.send('Accessibility.getFullAXTree');
  await cdp.detach();
  return nodes.filter(n => !n.ignored);
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
  await check('review pages load locally with no errors (light and dark; neutral, themed and prebuilt)', async () => {
    for (const url of ['/demo/index.html', '/demo/themed.html', '/demo/dist.html']) {
      for (const scheme of ['light', 'dark']) {
        const { page, problems } = await open(url, { colorScheme: scheme });
        const font = await page.evaluate(() => getComputedStyle(document.body).fontFamily);
        // The example theme keeps the kit's font.
        assert.match(font, /^Inter,/);
        const bg = await page.evaluate(() => getComputedStyle(document.querySelector('.wk-root')).backgroundColor);
        assert.equal(bg, scheme === 'light' ? 'rgb(252, 252, 251)' : 'rgb(24, 26, 31)');
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false);
        // The pages link only to each other.
        assert.deepEqual(await page.evaluate(() => [...document.querySelectorAll('a[href]')].map(a => a.getAttribute('href')).filter(h => !h.startsWith('#') && !/^[\w-]+\.html$/.test(h))), []);
        assert.deepEqual(problems, [], `${url} (${scheme})`);
        await page.close();
      }
    }
  });

  await check('increased contrast is followed live, in light and dark, and wins over a theme file', async () => {
    for (const url of ['/demo/index.html', '/demo/themed.html']) {
      for (const scheme of ['light', 'dark']) {
        const { page, problems } = await open(url, { colorScheme: scheme });
        // A field's border, and what the contrast block should make it: the
        // theme's text mixed into its bg (set through the CSSOM: the demo's
        // CSP allows no inline style).
        const look = () => page.evaluate(() => {
          const field = document.querySelector('.wk-field');
          const probe = document.createElement('i');
          probe.style.setProperty('color', 'color-mix(in srgb, var(--wk-color-text) 70%, var(--wk-color-bg))');
          probe.style.setProperty('background-color', 'var(--wk-color-text-secondary)');
          document.body.append(probe);
          const p = getComputedStyle(probe);
          const out = { field: getComputedStyle(field).borderTopColor, mixed: p.color, secondary: p.backgroundColor };
          probe.remove();
          return out;
        });
        const usual = await look();
        assert.notEqual(usual.field, usual.mixed, `${url} (${scheme}): already the contrast value`);
        await page.emulateMedia({ contrast: 'more' });
        const more = await look();
        assert.equal(more.field, more.mixed, `${url} (${scheme}): the field border follows increased contrast`);
        assert.notEqual(more.secondary, usual.secondary, `${url} (${scheme}): secondary text unchanged`);
        await page.emulateMedia({ contrast: 'no-preference' });
        assert.deepEqual(await look(), usual, `${url} (${scheme}): back to the usual values`);
        assert.deepEqual(problems, []);
        await page.close();
      }
    }
  });

  await check('data-theme wins over the system appearance, for the tokens, color-scheme and the backdrop', async () => {
    for (const url of ['/demo/index.html', '/demo/themed.html']) {
      for (const [scheme, attr, want] of [['dark', 'light', 'light'], ['light', 'dark', 'dark'], ['dark', null, 'dark'], ['light', null, 'light']]) {
        const { page, problems } = await open(url, { colorScheme: scheme });
        await page.evaluate(a => { if (a) document.documentElement.dataset.theme = a; else delete document.documentElement.dataset.theme; }, attr);
        const look = await page.evaluate(() => {
          const root = getComputedStyle(document.documentElement);
          const d = document.querySelector('#demo-dialog dialog'); d.showModal();
          const backdrop = getComputedStyle(d, '::backdrop').backgroundColor; d.close();
          const probe = document.createElement('i');
          probe.style.backgroundColor = 'var(--wk-color-attention-fill)';
          document.body.append(probe);
          const fill = getComputedStyle(probe).backgroundColor;
          probe.remove();
          return { bg: root.backgroundColor, scheme: root.colorScheme, fill, backdrop };
        });
        const dark = want === 'dark';
        assert.equal(look.bg, dark ? 'rgb(24, 26, 31)' : 'rgb(252, 252, 251)', `${url} system ${scheme}, data-theme ${attr}`);
        assert.equal(look.scheme, want, `${url} color-scheme, system ${scheme}, data-theme ${attr}`);
        assert.equal(look.backdrop, dark ? 'rgba(0, 0, 0, 0.5)' : 'rgba(15, 20, 30, 0.28)');
        // Derived from the theme's attention: #ee6b5f at 12% in dark, #bc3529 at 8% in light.
        if (url === '/demo/themed.html') assert.equal(look.fill.replace(/\s+/g, ''), dark ? 'color(srgb0.9333330.4196080.372549/0.12)' : 'color(srgb0.7372550.2078430.160784/0.08)');
        assert.deepEqual(problems, []);
        await page.close();
      }
    }
  });

  await check('tabs: the WAI-ARIA tabs pattern, automatic activation', async () => {
    const { page, problems } = await open();
    const tab = name => page.locator('#demo-tabs [role=tab]', { hasText: name });
    assert.equal(await tab('General').getAttribute('tabindex'), '0');
    assert.equal(await tab('Sharing').getAttribute('tabindex'), '-1');
    await tab('General').focus();
    await page.keyboard.press('ArrowRight');
    assert.equal(await active(page), 'Sharing');
    assert.equal(await tab('Sharing').getAttribute('aria-selected'), 'true');
    assert.equal(await tab('General').getAttribute('aria-selected'), 'false');
    assert.equal(await page.locator('#demo-tabs [role=tabpanel]:not([hidden])').textContent(), 'The Sharing pane.');
    const panelId = await tab('Sharing').getAttribute('aria-controls');
    assert.equal(await page.locator(`#${panelId}`).getAttribute('aria-labelledby'), await tab('Sharing').getAttribute('id'));
    await page.keyboard.press('ArrowRight'); // skips the disabled tab
    assert.equal(await active(page), 'About');
    await page.keyboard.press('ArrowRight'); // wraps
    assert.equal(await active(page), 'General');
    await page.keyboard.press('ArrowLeft');
    assert.equal(await active(page), 'About');
    await page.keyboard.press('Home');
    assert.equal(await active(page), 'General');
    await page.keyboard.press('End');
    assert.equal(await active(page), 'About');
    await tab('Disabled').click({ force: true });
    assert.equal(await tab('About').getAttribute('aria-selected'), 'true');
    await tab('Sharing').click();
    assert.deepEqual((await events(page, 'wk-change')).map(e => e.index), [1, 3, 0, 3, 0, 3, 1]);
    await page.keyboard.press('Tab'); // one tab stop: on to the panel
    assert.equal(await page.evaluate(() => document.activeElement.getAttribute('role')), 'tabpanel');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('menu button: the WAI-ARIA menu button and menu patterns', async () => {
    const { page, problems } = await open();
    const menuButton = page.locator('#demo-menu > button');
    const menu = page.locator('#demo-menu [role=menu]');
    assert.equal(await menuButton.getAttribute('aria-haspopup'), 'menu');
    assert.equal(await menuButton.getAttribute('aria-expanded'), 'false');
    await menuButton.focus();
    await page.keyboard.press('ArrowDown');
    assert.equal(await menu.isVisible(), true);
    assert.equal(await menuButton.getAttribute('aria-expanded'), 'true');
    assert.equal(await active(page), 'Copy detailsCtrl+C');
    await page.keyboard.press('ArrowUp'); // wraps to the last item
    assert.equal(await active(page), 'Remove…');
    await page.keyboard.press('Home');
    await page.keyboard.press('ArrowDown');
    assert.equal(await active(page), 'Open in browser');
    await page.keyboard.press('ArrowDown'); // a disabled item takes focus
    assert.equal(await active(page), 'Pause (disabled)');
    await page.keyboard.press('Enter'); // but does nothing
    assert.equal(await menu.isVisible(), true);
    await page.keyboard.press('c'); // type-ahead
    assert.equal(await active(page), 'Compact rows');
    await page.keyboard.press(' ');
    assert.equal(await menu.isVisible(), false);
    assert.equal(await active(page), 'Actions');
    assert.equal(await menuButton.getAttribute('aria-expanded'), 'false');
    assert.equal(await page.locator('#demo-menu [data-value=compact]').getAttribute('aria-checked'), 'false');
    const selects = await events(page, 'wk-select');
    assert.deepEqual(selects.at(-1), { type: 'wk-select', target: 'demo-menu', value: 'compact', checked: false });
    await page.keyboard.press('ArrowUp');
    assert.equal(await active(page), 'Remove…');
    await page.keyboard.press('Escape');
    assert.equal(await menu.isVisible(), false);
    assert.equal(await active(page), 'Actions');
    assert.equal((await events(page, 'wk-select')).length, selects.length, 'Escape selected something');
    await page.keyboard.press('Enter'); // Enter on the button opens it
    assert.equal(await active(page), 'Copy detailsCtrl+C');
    await page.keyboard.press('Tab'); // Tab closes it and moves on
    assert.equal(await menu.isVisible(), false);
    assert.equal(await active(page), 'demo-context-target');
    await menuButton.click();
    assert.equal(await menu.isVisible(), true);
    await page.locator('#demo-menu [data-value=open]').click();
    assert.equal((await events(page, 'wk-select')).at(-1).value, 'open');
    await menuButton.click();
    await page.mouse.click(5, 5); // a click outside closes it
    assert.equal(await menu.isVisible(), false);
    // The open menu stays inside the window.
    await page.setViewportSize({ width: 1100, height: 400 });
    await menuButton.scrollIntoViewIfNeeded();
    await page.evaluate(() => { const b = document.querySelector('#demo-menu > button'); scrollBy(0, b.getBoundingClientRect().bottom - innerHeight + 20); });
    await menuButton.click();
    const box = await menu.boundingBox();
    assert.ok(box && box.y >= 0 && box.y + box.height <= 400, `menu at ${JSON.stringify(box)} leaves the window`);
    await page.keyboard.press('Escape');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('menu: a scroll that started before it opened does not close it; a real scroll or window blur closes it and focus returns to the button', async () => {
    const { page, problems } = await open();
    // Scroll to the top, then bring the menu into view and wait for that scroll.
    await page.evaluate(() => scrollTo(0, 0));
    await page.evaluate(() => new Promise(resolve => { addEventListener('scroll', resolve, { once: true }); document.getElementById('demo-menu').scrollIntoView({ block: 'center' }); }));
    // A scroll whose event is still on its way when the menu opens.
    const late = await page.evaluate(() => new Promise(resolve => {
      scrollBy(0, 30);
      const m = document.getElementById('demo-menu');
      m.querySelector('button').focus({ preventScroll: true });
      m.show();
      document.addEventListener('scroll', () => resolve({ open: m.open, active: document.activeElement.textContent.trim() }), { once: true, capture: true });
    }));
    assert.deepEqual(late, { open: true, active: 'Copy detailsCtrl+C' }, 'the late scroll event closed the menu');
    // The same for the context menu, opened from the keyboard.
    await page.keyboard.press('Escape');
    const lateContext = await page.evaluate(() => new Promise(resolve => {
      scrollBy(0, -30);
      const t = document.getElementById('demo-context-target');
      t.focus({ preventScroll: true });
      t.dispatchEvent(new KeyboardEvent('keydown', { key: 'F10', shiftKey: true, bubbles: true, cancelable: true }));
      const c = document.getElementById('demo-context');
      document.addEventListener('scroll', () => resolve({ open: c.open, active: document.activeElement.textContent.trim() }), { once: true, capture: true });
    }));
    assert.deepEqual(lateContext, { open: true, active: 'Rename' }, 'the late scroll event closed the context menu');
    await page.keyboard.press('Escape');
    assert.equal(await active(page), 'demo-context-target');
    // A real scroll moves the button away from the menu: it closes, and focus
    // goes back to the button rather than to <body>.
    await page.locator('#demo-menu > button').focus();
    await page.keyboard.press('ArrowDown');
    assert.equal(await isOpen(page, 'demo-menu'), true);
    await page.mouse.move(5, 5);
    await page.mouse.wheel(0, 120);
    await until(page, () => !document.getElementById('demo-menu').open);
    assert.equal(await active(page), 'Actions');
    // Window blur closes it too, with focus left on the button.
    await page.keyboard.press('ArrowDown');
    assert.equal(await active(page), 'Copy detailsCtrl+C');
    await page.evaluate(() => dispatchEvent(new Event('blur')));
    assert.equal(await isOpen(page, 'demo-menu'), false);
    assert.equal(await active(page), 'Actions');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('context menu: right-click, Shift+F10, focus back where it was', async () => {
    const { page, problems } = await open();
    const menuButton = page.locator('#demo-menu > button');
    const target = page.locator('#demo-context-target');
    const ctx = page.locator('#demo-context [role=menu]');
    await target.focus();
    await page.keyboard.press('Shift+F10');
    await ctx.waitFor({ state: 'visible' });
    assert.equal(await active(page), 'Rename');
    await page.keyboard.press('Escape');
    assert.equal(await ctx.isVisible(), false);
    assert.equal(await active(page), 'demo-context-target');
    await menuButton.focus();
    const tb = await target.boundingBox();
    await page.mouse.click(tb.x + 30, tb.y + 10, { button: 'right' });
    assert.equal(await ctx.isVisible(), true);
    const cb = await ctx.boundingBox();
    assert.ok(Math.abs(cb.x - (tb.x + 30)) <= 1 && Math.abs(cb.y - (tb.y + 10)) <= 1, 'the context menu opens at the pointer');
    await page.keyboard.press('End');
    assert.equal(await active(page), 'Large');
    await page.keyboard.press('Enter');
    assert.equal(await ctx.isVisible(), false);
    assert.equal(await page.locator('#demo-context [data-value=large]').getAttribute('aria-checked'), 'true');
    assert.equal(await page.locator('#demo-context [data-value=small]').getAttribute('aria-checked'), 'false');
    assert.deepEqual((await events(page, 'wk-select')).at(-1), { type: 'wk-select', target: 'demo-context', value: 'large', checked: true, context: 'demo-context-target' });
    // The right-click focused the (focusable) target, so focus returns there.
    assert.equal(await active(page), 'demo-context-target');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('menu items: the check mark is not part of the name; the checked state is exposed', async () => {
    const { page, problems } = await open();
    await page.locator('#demo-menu > button').focus();
    await page.keyboard.press('ArrowDown');
    const items = (await axTree(page)).filter(n => n.role?.value === 'menuitemcheckbox');
    assert.equal(items.length, 1);
    assert.equal(items[0].name?.value, 'Compact rows');
    assert.equal(items[0].properties.find(p => p.name === 'checked')?.value.value, 'true');
    await page.keyboard.press('Escape');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('tooltip: shows on focus and after a delay on hover; Escape hides it', async () => {
    const { page, problems } = await open();
    const tipTrigger = page.locator('#demo-tooltip > button');
    const tip = page.locator('#demo-tooltip [role=tooltip]');
    assert.equal(await tipTrigger.getAttribute('aria-describedby'), await tip.getAttribute('id'));
    assert.equal(await tip.isVisible(), false);
    await tipTrigger.focus();
    assert.equal(await tip.isVisible(), true);
    const [tipBox, triggerBox] = [await tip.boundingBox(), await tipTrigger.boundingBox()];
    assert.ok(tipBox.y + tipBox.height <= triggerBox.y, 'the tooltip sits above its trigger');
    await page.keyboard.press('Escape');
    assert.equal(await tip.isVisible(), false);
    assert.equal(await active(page), 'Hover or focus me');
    await page.keyboard.press('Tab');
    assert.equal(await tip.isVisible(), false);
    // A delay far longer than the check takes, so "not yet" is certain.
    await page.evaluate(() => document.getElementById('demo-tooltip').setAttribute('delay', '60000'));
    await tipTrigger.hover();
    assert.equal(await tip.isVisible(), false, 'the tooltip waits before showing on hover');
    await page.mouse.move(5, 5);
    await page.evaluate(() => document.getElementById('demo-tooltip').setAttribute('delay', '0'));
    await tipTrigger.hover();
    await tip.waitFor({ state: 'visible' });
    await page.mouse.move(5, 5);
    assert.equal(await tip.isVisible(), false);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('tooltip: stays open while the pointer moves onto it (WCAG 1.4.13); with a menu open, the first Escape closes the menu', async () => {
    const { page, problems } = await open();
    const trigger = page.locator('#demo-tooltip > button');
    const tip = page.locator('#demo-tooltip [role=tooltip]');
    await page.evaluate(() => document.getElementById('demo-tooltip').setAttribute('delay', '0'));
    await trigger.scrollIntoViewIfNeeded();
    const g = await trigger.boundingBox();
    await page.mouse.move(g.x + g.width / 2, g.y + g.height / 2);
    await tip.waitFor({ state: 'visible' });
    const t = await tip.boundingBox();
    await page.mouse.move(g.x + g.width / 2, g.y + 1);
    await page.mouse.move(t.x + t.width / 2, t.y + t.height / 2, { steps: 8 });
    assert.equal(await tip.isVisible(), true, 'moving onto the tooltip hid it');
    await page.keyboard.press('Escape');
    assert.equal(await tip.isVisible(), false, 'Escape did not dismiss the hovered tooltip');
    await page.mouse.move(5, 5);
    // A menu open with keyboard focus in it, and the tooltip shown by hover.
    await page.locator('#demo-menu > button').focus();
    await page.keyboard.press('ArrowDown');
    await page.mouse.move(g.x + g.width / 2, g.y + g.height / 2);
    await tip.waitFor({ state: 'visible' });
    assert.equal(await isOpen(page, 'demo-menu'), true);
    await page.keyboard.press('Escape');
    assert.equal(await isOpen(page, 'demo-menu'), false, 'the first Escape did not close the menu');
    assert.equal(await active(page), 'Actions');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('dialog: native modal, focus return, return values, events, attribute; a sheet and a question do not light-dismiss', async () => {
    const { page, problems } = await open();
    const opener = page.locator('#demo-dialog-open');
    const dialog = page.locator('#demo-dialog dialog');
    await opener.focus();
    await page.keyboard.press('Enter');
    assert.equal(await dialog.isVisible(), true);
    assert.equal(await page.locator('#demo-dialog').getAttribute('open'), '');
    assert.equal(await page.evaluate(() => document.querySelector('#demo-dialog dialog').matches(':modal')), true);
    assert.equal(await active(page), 'Cancel'); // autofocus
    assert.equal(await dialog.getAttribute('aria-labelledby'), await page.locator('#demo-dialog .wk-dialog-title').getAttribute('id'));
    await page.keyboard.press('Tab');
    assert.equal(await active(page), 'Delete');
    await page.keyboard.press('Escape');
    await closes(page, 1);
    assert.equal(await dialog.isVisible(), false);
    assert.equal(await page.locator('#demo-dialog').getAttribute('open'), null);
    assert.equal(await active(page), 'demo-dialog-open');
    await opener.click();
    await page.locator('#demo-dialog [value=remove]').click();
    assert.deepEqual((await closes(page, 2)).map(e => e.returnValue), ['', 'remove']);
    assert.equal(await active(page), 'demo-dialog-open');
    assert.equal((await events(page, 'wk-open')).length, 2);
    await opener.click(); // a question: a click on the backdrop leaves it open
    await page.mouse.click(5, 5);
    assert.equal(await dialog.isVisible(), true);
    await page.keyboard.press('Escape');
    await closes(page, 3);
    await page.evaluate(() => document.getElementById('demo-dialog').setAttribute('open', '')); // the attribute drives it too
    assert.equal(await dialog.isVisible(), true);
    await page.evaluate(() => document.getElementById('demo-dialog').removeAttribute('open'));
    assert.equal(await dialog.isVisible(), false);
    await closes(page, 4);
    // The sheet hangs from the top edge and has no light dismiss.
    await page.locator('#demo-sheet-open').click();
    const sheet = page.locator('#demo-sheet dialog');
    assert.equal((await sheet.boundingBox()).y, 0);
    await page.mouse.click(5, 700);
    assert.equal(await sheet.isVisible(), true);
    await page.locator('#demo-sheet [value=rename]').click();
    assert.equal((await closes(page, 5)).at(-1).returnValue, 'rename');
    assert.equal(await active(page), 'demo-sheet-open');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('dialog light-dismiss: a click on the backdrop closes it, a drag that starts inside it does not', async () => {
    const { page, problems } = await open();
    const info = page.locator('#demo-info dialog');
    await page.locator('#demo-info-open').click();
    assert.equal(await info.isVisible(), true);
    const body = await page.locator('#demo-info .wk-dialog-body').boundingBox();
    await page.mouse.move(body.x + 20, body.y + 5);
    await page.mouse.down();
    await page.mouse.move(5, 5, { steps: 5 });
    await page.mouse.up();
    assert.equal(await info.isVisible(), true, 'a drag from inside the dialog to the backdrop closed it');
    // Down on the backdrop, up inside: not a backdrop click either.
    await page.mouse.move(5, 5);
    await page.mouse.down();
    await page.mouse.move(body.x + 20, body.y + 5, { steps: 5 });
    await page.mouse.up();
    assert.equal(await info.isVisible(), true, 'a drag from the backdrop into the dialog closed it');
    await page.mouse.click(5, 5);
    assert.equal(await info.isVisible(), false);
    await closes(page, 1);
    assert.equal(await active(page), 'demo-info-open');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('dialog focus return does not rely on the browser: an opener the click did not focus gets focus back', async () => {
    const { page, problems } = await open();
    // WebKit does not focus a clicked button; preventing mousedown's default
    // does the same in Chromium, so the browser's own restore goes to <body>.
    await page.evaluate(() => {
      const keep = e => e.preventDefault();
      document.getElementById('demo-dialog-open').addEventListener('mousedown', keep);
      const b = document.createElement('button');
      b.id = 'script-open'; b.type = 'button'; b.className = 'wk-button'; b.textContent = 'Script opener';
      b.addEventListener('mousedown', keep);
      b.addEventListener('click', () => document.getElementById('demo-dialog').show());
      document.getElementById('demo-dialog-open').after(b);
      document.activeElement.blur();
    });
    for (const id of ['demo-dialog-open', 'script-open']) {
      assert.equal(await active(page), 'BODY');
      await page.locator(`#${id}`).click();
      assert.equal(await active(page), 'Cancel');
      await page.keyboard.press('Escape');
      await until(page, () => !document.getElementById('demo-dialog').open);
      await until(page, i => document.activeElement?.id === i, id).catch(() => {});
      assert.equal(await active(page), id);
      await page.evaluate(() => document.activeElement.blur());
    }
    assert.deepEqual(problems, []);
    await page.close();
  });

  // A container at the end of the page for elements a check builds itself.
  const box = page => page.evaluate(() => {
    const el = document.createElement('div');
    el.id = 'late';
    document.getElementById('demo').append(el);
  });
  const rightClick = async (page, id) => {
    await page.locator(`#${id}`).scrollIntoViewIfNeeded();
    const b = await page.locator(`#${id}`).boundingBox();
    await page.mouse.click(b.x + 10, b.y + 10, { button: 'right' });
  };
  const target = (page, ...ids) => page.evaluate(list => {
    for (const id of list) {
      const el = document.createElement('div'); el.id = id; el.tabIndex = 0; el.textContent = id; el.className = 'demo-target';
      document.getElementById('late').append(el);
    }
  }, ids);

  await check('built imperatively: elements connected before their children wire themselves when the children arrive', async () => {
    const { page, problems } = await open();
    await box(page);
    await page.evaluate(() => {
      const make = (tag, id) => { const el = document.createElement(tag); el.id = id; document.getElementById('late').append(el); return el; };
      const t = make('wk-tabs', 'late-tabs');
      const m = make('wk-menu', 'late-menu');
      const d = make('wk-dialog', 'late-dialog');
      const tt = make('wk-tooltip', 'late-tooltip');
      t.innerHTML = '<div role="tablist"><button role="tab" type="button">A</button><button role="tab" type="button">B</button></div><div role="tabpanel">a</div><div role="tabpanel">b</div>';
      m.innerHTML = '<button type="button">Late</button><div role="menu" class="wk-menu"><button role="menuitem" type="button">One</button><button role="menuitem" type="button">Two</button></div>';
      tt.innerHTML = '<button type="button">Tip</button><span role="tooltip" class="wk-tooltip">Late tip</span>';
      d.innerHTML = '<dialog class="wk-dialog"><p>Late</p><button type="button" data-wk-close>Close</button></dialog>';
      d.setAttribute('open', '');
    });
    await until(page, () => {
      const t = document.getElementById('late-tabs'), m = document.getElementById('late-menu'), tt = document.getElementById('late-tooltip');
      return t.querySelector('[role=tab]').getAttribute('aria-selected') === 'true' && t.querySelectorAll('[role=tabpanel]')[1].hidden
        && m.querySelector('button').getAttribute('aria-haspopup') === 'menu' && m.querySelector('[role=menu]').hidden
        && tt.querySelector('button').getAttribute('aria-describedby') === tt.querySelector('[role=tooltip]').id && tt.querySelector('[role=tooltip]').hidden
        && document.getElementById('late-dialog').open;
    });
    await page.keyboard.press('Escape');
    await until(page, () => !document.getElementById('late-dialog').open);
    await page.locator('#late-tabs [role=tab]', { hasText: 'B' }).click();
    assert.equal(await page.locator('#late-tabs [role=tabpanel]:not([hidden])').textContent(), 'b');
    await page.locator('#late-menu > button').focus();
    await page.keyboard.press('ArrowDown');
    assert.equal(await active(page), 'One');
    await page.keyboard.press('Escape');
    assert.equal(await active(page), 'Late');
    // Children replaced later are wired in their place, once.
    await page.evaluate(() => { document.getElementById('late-menu').innerHTML = '<button type="button">Again</button><div role="menu" class="wk-menu" hidden><button role="menuitem" type="button">Three</button></div>'; });
    await until(page, () => document.querySelector('#late-menu > button').getAttribute('aria-haspopup') === 'menu');
    await page.locator('#late-menu > button').click();
    assert.equal(await isOpen(page, 'late-menu'), true, 'a click on the new button did not open the menu (or opened and closed it)');
    await page.keyboard.press('Escape');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('built imperatively: a context menu finds a for= target added later, and follows a changed for=', async () => {
    const { page, problems } = await open();
    await box(page);
    await page.evaluate(() => {
      const c = document.createElement('wk-context-menu'); c.id = 'late-context'; c.setAttribute('for', 'late-target');
      c.innerHTML = '<div role="menu" class="wk-menu" hidden><button role="menuitem" type="button">Late item</button></div>';
      document.getElementById('late').append(c);
    });
    await target(page, 'late-target', 'late-target-2');
    await rightClick(page, 'late-target');
    assert.equal(await isOpen(page, 'late-context'), true, 'a for= target added later does not open the menu');
    await page.keyboard.press('Escape');
    await page.evaluate(() => document.getElementById('late-context').setAttribute('for', 'late-target-2'));
    await rightClick(page, 'late-target');
    assert.equal(await isOpen(page, 'late-context'), false, 'the old for= target still opens the menu');
    await rightClick(page, 'late-target-2');
    assert.equal(await isOpen(page, 'late-context'), true, 'the new for= target does not open the menu');
    await page.keyboard.press('Escape');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('built imperatively: removing open menus leaves no listeners behind and puts focus back on the context target', async () => {
    const { page, problems } = await open();
    await box(page);
    await target(page, 'late-target');
    const watched = ['document', 'window', 'document.getElementById("late-target")', 'document.getElementById("demo-context-target")'];
    const before = await listeners(page, watched);
    await page.evaluate(() => {
      const c = document.createElement('wk-context-menu'); c.id = 'late-context'; c.setAttribute('for', 'late-target');
      c.innerHTML = '<div role="menu" class="wk-menu" hidden><button role="menuitem" type="button">Late item</button></div>';
      document.getElementById('late').append(c);
    });
    await page.locator('#late-target').focus();
    await page.keyboard.press('Shift+F10');
    assert.equal(await active(page), 'Late item');
    await page.evaluate(() => document.getElementById('late-context').remove());
    assert.equal(await active(page), 'late-target');
    await page.locator('#demo-menu > button').focus();
    await page.keyboard.press('ArrowDown');
    await page.evaluate(() => document.getElementById('demo-menu').remove());
    await page.locator('#demo-context-target').focus();
    await page.keyboard.press('Shift+F10');
    assert.equal(await active(page), 'Rename');
    await page.evaluate(() => document.getElementById('demo-context').remove());
    assert.equal(await active(page), 'demo-context-target');
    const after = await listeners(page, watched);
    // The demo's own context menu listened at the document; it is gone too.
    const gone = new Set(['contextmenu', 'keydown']);
    assert.deepEqual(after.window, before.window, 'window listeners left behind');
    assert.deepEqual(after.document, before.document.filter(t => !gone.has(t)), 'document listeners left behind');
    for (const t of watched.slice(2)) assert.deepEqual(after[t], [], `${t} listeners left behind`);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('built imperatively: a moved menu, tooltip and dialog keep one set of listeners and still work', async () => {
    const { page, problems } = await open();
    await box(page);
    const watched = ['document.querySelector("#demo-menu > button")', 'document.getElementById("demo-tooltip")', 'document.querySelector("#demo-dialog > dialog")'];
    const once = await listeners(page, watched);
    await page.evaluate(() => {
      const late = document.getElementById('late');
      for (const id of ['demo-menu', 'demo-tooltip', 'demo-dialog']) { const el = document.getElementById(id); late.append(el); late.append(el); }
    });
    assert.deepEqual(await listeners(page, watched), once, 'moving changed the listeners');
    await page.locator('#demo-menu > button').click();
    assert.equal(await isOpen(page, 'demo-menu'), true, 'a moved menu does not open on click');
    await page.keyboard.press('Escape');
    assert.equal(await active(page), 'Actions');
    await page.locator('#demo-tooltip > button').focus();
    assert.equal(await page.locator('#demo-tooltip [role=tooltip]').isVisible(), true, 'a moved tooltip does not show');
    await page.locator('#demo-dialog-open').click();
    assert.equal(await isOpen(page, 'demo-dialog'), true, 'a moved dialog does not open');
    await page.keyboard.press('Escape');
    await until(page, () => !document.getElementById('demo-dialog').open);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('built imperatively: generated ids are free in the document', async () => {
    const { page, problems } = await open();
    await box(page);
    const ids = await page.evaluate(() => {
      for (const prefix of ['wk-tab', 'wk-tabpanel', 'wk-menu', 'wk-tooltip']) {
        for (let i = 0; i < 1000; i++) { const s = document.createElement('span'); s.id = `${prefix}-${i}`; s.hidden = true; document.body.append(s); }
      }
      const t = document.createElement('wk-tabs');
      t.innerHTML = '<div role="tablist"><button role="tab" type="button">A</button></div><div role="tabpanel">a</div>';
      document.getElementById('late').append(t);
      return [t.querySelector('[role=tab]').id, t.querySelector('[role=tabpanel]').id];
    });
    for (const id of ids) assert.equal(await page.evaluate(i => document.querySelectorAll(`[id="${i}"]`).length, id), 1, `${id} is used twice`);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('forced colours: the six dot shapes stay distinct in every tone; the focused menu item stands out', async () => {
    const { page, problems } = await open('/demo/index.html', { forcedColors: 'active', reducedMotion: 'reduce' });
    assert.equal(await page.evaluate(() => matchMedia('(forced-colors: active)').matches), true);
    const shapes = ['filled', 'ringed', 'half', 'hollow', 'dashed', 'bar'];
    await page.locator('#dots').scrollIntoViewIfNeeded();
    for (const tone of ['neutral', 'working', 'attention']) {
      const shots = [];
      for (const shape of shapes) shots.push(await page.locator(`.demo-dot-large[data-shape=${shape}][data-tone=${tone}]`).screenshot());
      for (let i = 0; i < shapes.length; i++) {
        for (let j = i + 1; j < shapes.length; j++) {
          assert.ok(!shots[i].equals(shots[j]), `${shapes[i]} and ${shapes[j]} (${tone}) look the same in forced colours`);
        }
      }
    }
    await page.locator('#demo-menu > button').focus();
    await page.keyboard.press('ArrowDown');
    const item = page.locator('#demo-menu [data-value=open]');
    const unfocused = await item.screenshot();
    await page.keyboard.press('ArrowDown');
    assert.equal(await active(page), 'Open in browser');
    const focused = await item.screenshot();
    assert.ok(!focused.equals(unfocused), 'the focused menu item looks like the others in forced colours');
    await page.keyboard.press('Escape');
    // A link item is drawn as the button items, not in LinkText.
    const ink = await page.locator('#demo-inline [role=menu]').evaluate(m => {
      const probe = c => { const e = document.createElement('i'); e.style.forcedColorAdjust = 'none'; e.style.color = c; document.body.append(e); const v = getComputedStyle(e).color; e.remove(); return v; };
      return { link: getComputedStyle(m.querySelector('a')).color, button: getComputedStyle(m.querySelector('[data-value=settings]')).color, canvasText: probe('CanvasText'), linkText: probe('LinkText') };
    });
    assert.notEqual(ink.canvasText, ink.linkText);
    assert.equal(ink.link, ink.canvasText, 'a link item is CanvasText');
    assert.equal(ink.link, ink.button, 'a link item looks like a button item');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('long sheet: hidden while closed; open, a fixed header and footer bar, and only the middle scrolls', async () => {
    const { page, problems } = await open('/demo/index.html', { viewport: { width: 1100, height: 420 } });
    const sheet = page.locator('#demo-long-sheet dialog');
    assert.equal(await sheet.isVisible(), false);
    await page.locator('#demo-long-sheet-open').click();
    assert.equal(await sheet.isVisible(), true);
    assert.equal(await active(page), 'Done');
    const look = () => page.evaluate(() => {
      const d = document.querySelector('#demo-long-sheet dialog'), s = d.querySelector('.wk-dialog-scroll');
      const top = el => Math.round(el.getBoundingClientRect().top);
      return { clips: getComputedStyle(d).overflow, overflowing: s.scrollHeight > s.clientHeight, header: top(d.querySelector('.wk-dialog-header')), footer: top(d.querySelector('.wk-dialog-footer')), inside: d.getBoundingClientRect().bottom >= d.querySelector('.wk-dialog-footer').getBoundingClientRect().bottom - 1 };
    });
    const before = await look();
    assert.equal(before.clips, 'hidden'); assert.equal(before.overflowing, true); assert.equal(before.inside, true);
    await page.evaluate(() => { document.querySelector('#demo-long-sheet .wk-dialog-scroll').scrollTop = 1e4; });
    const after = await look();
    assert.equal(after.header, before.header); assert.equal(after.footer, before.footer);
    await page.keyboard.press('Escape');
    await until(page, () => !document.querySelector('#demo-long-sheet dialog').open);
    assert.equal(await sheet.isVisible(), false);
    assert.deepEqual(problems, []);
    await page.close();
  });


  await check('switch and segmented control: native keyboard behaviour', async () => {
    const { page, problems } = await open();
    const sw = page.locator('#demo-switch');
    assert.equal(await sw.getAttribute('role'), 'switch');
    assert.equal(await sw.isChecked(), false);
    await sw.focus();
    await page.keyboard.press(' ');
    assert.equal(await sw.isChecked(), true);
    await page.keyboard.press(' ');
    assert.equal(await sw.isChecked(), false);
    const radio = v => page.locator(`#demo-segmented input[value=${v}]`);
    await radio('day').focus();
    await page.keyboard.press('ArrowRight');
    assert.equal(await radio('week').isChecked(), true);
    await page.keyboard.press('ArrowRight');
    await page.keyboard.press('ArrowRight');
    assert.equal(await radio('day').isChecked(), true);
    assert.equal(await radio('year').isChecked(), false);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('stepper: the arrows step the value, fire input and change, stop at the limits and keep focus', async () => {
    const { page, problems } = await open();
    await page.evaluate(() => { window.stepperEvents = []; for (const t of ['input', 'change']) document.getElementById('demo-stepper').addEventListener(t, e => window.stepperEvents.push(e.type)); });
    const stepper = page.locator('#demo-stepper').locator('xpath=ancestor::span[contains(@class,"wk-stepper")]');
    const more = stepper.getByRole('button', { name: 'More' }), fewer = stepper.getByRole('button', { name: 'Fewer' });
    const input = page.locator('#demo-stepper');
    // The browser's own spinner is gone: the arrows are the stepper's.
    assert.equal(await input.evaluate(el => getComputedStyle(el).appearance), 'textfield');
    await more.click();
    assert.equal(await input.inputValue(), '3');
    assert.deepEqual(await page.evaluate(() => window.stepperEvents), ['input', 'change']);
    await fewer.click(); await fewer.click();
    assert.equal(await input.inputValue(), '1');
    // At min: Fewer disables and the keyboard stays on the stepper.
    assert.equal(await fewer.isDisabled(), true); assert.equal(await more.isDisabled(), false);
    assert.equal(await page.evaluate(() => document.activeElement.id), 'demo-stepper');
    await fewer.click({ force: true }).catch(() => {});
    assert.equal(await input.inputValue(), '1');
    // At max: More disables; typing a value moves the arrows too.
    await input.fill('7'); await more.click();
    assert.equal(await input.inputValue(), '8'); assert.equal(await more.isDisabled(), true);
    await input.fill('4');
    assert.equal(await more.isDisabled(), false);
    // An off-grid value steps to the grid, as the native spinner does: 2.5 → 3 up, → 2 down.
    await input.fill('2.5'); await more.click(); assert.equal(await input.inputValue(), '3');
    await input.fill('2.5'); await fewer.click(); assert.equal(await input.inputValue(), '2');
    // A text field steps the same way, from its own limits.
    const text = page.locator('#demo-stepper-text');
    await text.locator('xpath=ancestor::span[contains(@class,"wk-stepper")]').getByRole('button', { name: 'More' }).click();
    assert.equal(await text.inputValue(), '4');
    // The Up key is the field's own; the arrows are real buttons in the tab order.
    await input.focus(); await page.keyboard.press('Tab');
    assert.equal(await page.evaluate(() => document.activeElement.getAttribute('aria-label')), 'More');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('link with aria-expanded draws a chevron that turns down when open; scrolling regions take focus', async () => {
    const { page, problems } = await open();
    const chevron = sel => page.locator(sel).evaluate(el => ({ content: getComputedStyle(el, '::before').content, transform: getComputedStyle(el, '::before').transform }));
    const closed = await chevron('#buttons button.wk-link[aria-expanded=false]'), open_ = await chevron('#buttons button.wk-link[aria-expanded=true]');
    assert.notEqual(closed.content, 'none'); assert.notEqual(closed.transform, open_.transform);
    assert.equal(await page.locator('a.wk-link').first().evaluate(el => getComputedStyle(el, '::before').content), 'none');
    for (const sel of ['.wk-code--scroll > pre', '.wk-errorbox > pre', '#demo-long-sheet .wk-dialog-scroll']) {
      const el = page.locator(sel).first();
      assert.equal(await el.getAttribute('tabindex'), '0', sel);
      assert.ok(await el.getAttribute('aria-label') || await el.getAttribute('aria-labelledby'), `${sel} has no name`);
    }
    const alertBox = await page.locator('.demo-static .wk-dialog--alert').evaluate(el => ({ width: el.getBoundingClientRect().width, align: getComputedStyle(el).textAlign, dir: getComputedStyle(el.querySelector('.wk-dialog-actions')).flexDirection, icon: el.querySelector(':scope > svg').getBoundingClientRect().width }));
    assert.deepEqual(alertBox, { width: 280, align: 'center', dir: 'column', icon: 54 });
    assert.deepEqual(problems, []);
    await page.close();
  });
  await check('chart: legend, 2 px gaps, token colours in light and dark, weekend shading, a gap breaks the line, tooltip, keyboard and table view', async () => {
    const probe = (page, token) => page.evaluate(tok => { const s = document.createElement('span'); s.style.color = `var(${tok})`; document.body.append(s); const c = getComputedStyle(s).color; s.remove(); return c; }, token);
    const fills = page => page.locator('#demo-stacked .wk-chart-bar').evaluateAll(els => els.map(el => getComputedStyle(el).fill));
    const light = await open('/demo/index.html', { colorScheme: 'light' });
    const { page, problems } = light;
    // A legend names every series and the shading; each key has the tone of its series.
    assert.deepEqual(await page.locator('#demo-stacked .wk-chart-legend-item').allTextContents(), ['Alder', 'Birch', 'Cedar', 'Other', 'Weekend']);
    for (const id of ['demo-line', 'demo-split', 'demo-rows']) assert.ok(await page.locator(`#${id} .wk-chart-legend-item`).count() >= 1, `${id} has a legend`);
    assert.deepEqual(await page.locator('#demo-split .wk-chart-legend-item').allTextContents(), ['Alder 52%', 'Birch 31%', 'Cedar 17%'], 'a split bar prints each part in its legend');
    // Weekends are shaded: two a week over 14 days.
    assert.equal(await page.locator('#demo-stacked .wk-chart-shade').count(), 4);
    // Stacked segments are 2 px apart; bars are never thicker than 24 px.
    const column = await page.locator('#demo-stacked .wk-chart-bar').evaluateAll(els => {
      const boxes = els.map(el => el.getBoundingClientRect()).filter(b => Math.abs(b.left - els[0].getBoundingClientRect().left) < 1).sort((a, b) => a.top - b.top);
      return { gaps: boxes.slice(1).map((b, i) => Math.round((b.top - boxes[i].bottom) * 10) / 10), widths: els.map(el => el.getBoundingClientRect().width) };
    });
    assert.ok(column.gaps.length >= 3 && column.gaps.every(g => Math.abs(g - 2) < .6), `gaps ${column.gaps}`);
    assert.ok(column.widths.every(w => w <= 24.5), 'bars are at most 24 px wide');
    // Colours come from the tokens, and change with the appearance.
    const lightFills = await fills(page);
    assert.ok(lightFills.includes(await probe(page, '--wk-color-chart-1')) && lightFills.includes(await probe(page, '--wk-color-chart-2')) && lightFills.includes(await probe(page, '--wk-color-chart-4')));
    assert.equal(await page.locator('#demo-stacked [data-tone="3"]').count(), 0, 'series 3 is not used for the stack');
    const dark = await open('/demo/index.html', { colorScheme: 'dark' });
    const darkFills = await fills(dark.page);
    assert.ok(darkFills.includes(await probe(dark.page, '--wk-color-chart-1')));
    assert.notDeepEqual(darkFills, lightFills);
    assert.equal(await dark.page.locator('#demo-stacked .wk-chart-tick').first().evaluate(el => getComputedStyle(el).fill), await probe(dark.page, '--wk-color-text-secondary'));
    assert.equal(await dark.page.locator('#demo-stacked').evaluate(el => getComputedStyle(el).backgroundColor), await probe(dark.page, '--wk-color-surface'));
    assert.deepEqual(dark.problems, []); await dark.page.close();
    // A gap in the data breaks the line: three runs, three bands.
    assert.equal(await page.locator('#demo-line .wk-chart-line').count(), 3);
    assert.equal(await page.locator('#demo-line .wk-chart-band').count(), 3);
    // Hover shows every series at that day, the value first.
    const svg = page.locator('#demo-stacked svg').first();
    await svg.scrollIntoViewIfNeeded();
    const box = await svg.boundingBox();
    await page.mouse.move(box.x + box.width * .3, box.y + 60);
    const tip = page.locator('#demo-stacked .wk-chart-tip');
    await tip.waitFor({ state: 'visible' });
    assert.match(await tip.textContent(), /Alder/);
    assert.equal(await tip.locator('.wk-chart-tip-row strong').first().evaluate(el => /^\d+$/.test(el.textContent)), true, 'the value leads');
    await page.mouse.move(0, 0);
    await tip.waitFor({ state: 'hidden' });
    // The keyboard reads the same: the plot takes focus, the arrows move, Escape clears.
    const plot = page.locator('#demo-stacked .wk-chart-plot');
    assert.equal(await plot.getAttribute('tabindex'), '0');
    assert.match(await plot.getAttribute('aria-label'), /^Jobs per day, 14 days\./);
    await plot.focus(); await page.keyboard.press('ArrowRight'); await page.keyboard.press('ArrowRight');
    assert.match(await page.locator('#demo-stacked .wk-chart-readout').textContent(), /^Tuesday 4 Nov: /);
    await page.keyboard.press('Escape');
    assert.equal(await page.locator('#demo-stacked .wk-chart-readout').textContent(), '');
    // The table view has the same numbers in a kit table, in a named, focusable region; the radio stays where it was.
    await page.locator('#demo-stacked .wk-chart-view input[value=table]').check({ force: true });
    const region = page.locator('#demo-stacked .wk-chart-scroll');
    assert.equal(await region.getAttribute('tabindex'), '0');
    assert.equal(await region.getAttribute('aria-label'), 'Jobs per day, table');
    assert.equal(await page.locator('#demo-stacked table.wk-table tbody tr').count(), 14);
    assert.deepEqual(await page.locator('#demo-stacked table thead th').allTextContents(), ['Day', 'Alder', 'Birch', 'Cedar', 'Other', 'Total']);
    assert.equal(await page.locator('#demo-stacked svg').count(), 0, 'the chart gives way to the table');
    await page.locator('#demo-stacked .wk-chart-view input[value=chart]').check({ force: true });
    assert.equal(await page.locator('#demo-stacked svg').count(), 1);
    // A chart can start as a table.
    assert.equal(await page.locator('#demo-rows table.wk-table tbody tr').count(), 3);
    assert.equal(await page.locator('#demo-rows .wk-chart-view input[value=table]').isChecked(), true);
    assert.deepEqual(problems, []);
    await page.close();
  });
  await check('chart: isolated band remains visible and accessible across gaps', async () => {
    const { page, problems } = await open();
    await page.evaluate(async () => {
      const { wkChart } = await import('../src/index.js');
      document.body.append(wkChart({ id: 'isolated', kind: 'line', label: 'Wait',
        categories: ['Empty', 'Isolated', 'Gap', 'Pair A', 'Pair B'].map(label => ({ label })),
        series: [{ label: 'p50', tone: 1, values: [null, 10, null, 12, 15] }],
        band: { label: 'p90', tone: 1, values: [null, 30, null, 25, 35] } }));
      document.body.append(wkChart({ id: 'close-band', kind: 'line', label: 'Close values',
        categories: ['Equal', 'Gap', 'Near'].map(label => ({ label })),
        series: [{ label: 'p50', tone: 1, values: [10, null, 10] }],
        band: { label: 'p90', tone: 1, values: [10, null, 10.01] } }));
    });
    const chart = page.locator('#isolated');
    const interval = chart.locator('.wk-chart-band-single');
    assert.equal(await interval.count(), 1, 'one visible interval for the isolated bucket');
    const geometry = await interval.evaluate(el => ({ height: el.getBBox().height, stroke: getComputedStyle(el).stroke, width: parseFloat(getComputedStyle(el).strokeWidth) }));
    assert.ok(geometry.height > 0 && geometry.stroke !== 'none' && geometry.width > 0, 'p50 to p90 interval is painted');
    assert.equal(await chart.locator('.wk-chart-band').count(), 1, 'the multi-point band remains');
    const caps = await page.locator('#close-band .wk-chart-band-single').evaluateAll(els => els.map(el => ({ width: el.getBBox().width, stroke: getComputedStyle(el).stroke })));
    assert.equal(caps.length, 2, 'equal and nearly equal values have caps');
    assert.ok(caps.every(cap => cap.width >= 16 && cap.stroke !== 'none'), 'caps extend visibly beyond the median dot');
    const plot = chart.locator('.wk-chart-plot');
    assert.equal(await plot.getAttribute('role'), 'group');
    assert.match(await plot.getAttribute('aria-label'), /Wait.*arrow keys/);
    await plot.focus(); await page.keyboard.press('Home');
    assert.equal(await chart.locator('[role=status]').textContent(), 'Empty: No data');
    await page.keyboard.press('ArrowRight');
    assert.equal(await chart.locator('[role=status]').textContent(), 'Isolated: p50 10, p90 30');
    await page.keyboard.press('ArrowRight');
    assert.equal(await chart.locator('[role=status]').textContent(), 'Gap: No data');
    await chart.locator('input[value=table]').check({ force: true });
    assert.deepEqual(await chart.locator('tbody tr').nth(1).locator('th, td').allTextContents(), ['Isolated', '10', '30']);
    assert.deepEqual(problems, []); await page.close();
  });
  await check('chart: stable focus and opt-in selection survive replacements', async () => {
    const { page, problems } = await open('/demo/index.html');
    await page.evaluate(async () => {
      const { wkChart } = await import('../src/index.js');
      const host = document.createElement('div'); document.body.append(host);
      let selection = -1, view = 'chart';
      window.replaceChart = value => {
        const focused = document.activeElement?.id;
        host.replaceChildren(wkChart({ id: 'persistent', kind: 'bars', label: 'Snapshot',
          categories: ['First', 'Second', 'Third'].map(label => ({ label })),
          series: [{ label: 'Count', values: [1, value, 9] }], selection, view,
          retainSelectionOnBlur: true, onselection: i => { selection = i; }, onview: v => { view = v; } }));
        document.getElementById(focused)?.focus();
      };
      window.replaceChart(2);
    });
    await page.locator('#persistent-plot').focus();
    await page.keyboard.press('ArrowRight'); await page.keyboard.press('ArrowRight');
    assert.equal(await page.locator('#persistent [role=status]').textContent(), 'Second: Count 2');
    for (const value of [3, 4]) {
      await page.evaluate(v => window.replaceChart(v), value);
      assert.equal(await page.evaluate(() => document.activeElement.id), 'persistent-plot');
      assert.equal(await page.locator('#persistent [role=status]').textContent(), `Second: Count ${value}`);
    }
    await page.keyboard.press('ArrowRight');
    assert.equal(await page.locator('#persistent [role=status]').textContent(), 'Third: Count 9');
    await page.locator('#persistent-view-table').focus();
    assert.equal(await page.locator('#persistent [role=status]').textContent(), 'Third: Count 9', 'opt-in retains on blur');
    await page.keyboard.press('Space');
    await page.evaluate(() => window.replaceChart(5));
    assert.equal(await page.evaluate(() => document.activeElement.id), 'persistent-view-table');
    assert.equal(await page.locator('#persistent-table').count(), 1);
    await page.locator('#persistent-view-chart').check({ force: true });
    await page.locator('#persistent-plot').focus(); await page.keyboard.press('Escape');
    assert.equal(await page.locator('#persistent [role=status]').textContent(), '');
    // Existing consumers still clear on blur by default.
    await page.locator('#demo-stacked .wk-chart-plot').focus(); await page.keyboard.press('Home');
    assert.notEqual(await page.locator('#demo-stacked [role=status]').textContent(), '');
    await page.locator('#persistent-view-chart').focus();
    assert.equal(await page.locator('#demo-stacked [role=status]').textContent(), '');
    assert.deepEqual(problems, []); await page.close();
  });
  await check('chart: selection survives snapshot replacement and can be cleared', async () => {
    const { page, problems } = await open('/demo/index.html');
    await page.evaluate(async () => {
      const { wkChart } = await import('../src/index.js');
      const host = document.createElement('div'); host.id = 'snapshot-host'; document.body.append(host);
      let selection = -1;
      window.snapshot = value => {
        host.replaceChildren(wkChart({ id: 'snapshot', kind: 'bars', label: 'Snapshot',
          categories: [{ label: 'First' }, { label: 'Second' }],
          series: [{ label: 'Count', values: [1, value] }], selection,
          onselection: i => { selection = i; window.reported.push(i); } }));
      };
      window.reported = [];
      window.selectedIndex = () => selection;
      window.snapshot(2);
    });
    const plot = page.locator('#snapshot .wk-chart-plot');
    await plot.focus();
    await page.keyboard.press('End');
    assert.equal(await page.evaluate(() => window.selectedIndex()), 1);
    for (const value of [3, 4]) {
      await page.evaluate(v => window.snapshot(v), value);
      await plot.focus(); // the consumer restores focus after replacing the element
      assert.equal(await page.locator('#snapshot .wk-chart-readout').textContent(), `Second: Count ${value}`);
      assert.equal(await page.locator('#snapshot .wk-chart-active').getAttribute('hidden'), null);
      assert.equal(await page.evaluate(() => window.selectedIndex()), 1);
    }
    // Replacing the focused chart is not the person leaving it: the old
    // chart's blur (Chrome fires it as the element is removed) must not
    // report -1 in between, even for a moment (#137).
    assert.deepEqual(await page.evaluate(() => window.reported.filter(i => i !== 1)), [], 'nothing but the kept selection was reported');
    await page.keyboard.press('Escape');
    assert.equal(await page.evaluate(() => window.selectedIndex()), -1);
    assert.equal(await page.locator('#snapshot .wk-chart-readout').textContent(), '');
    await page.evaluate(() => window.snapshot(5));
    assert.equal(await page.locator('#snapshot .wk-chart-readout').textContent(), '');
    // Pointer selection reports the category as well as keyboard selection.
    const svg = page.locator('#snapshot svg');
    await svg.hover(); // Wait until the replacement chart is visible and stable.
    const box = await svg.boundingBox();
    await page.mouse.move(box.x + box.width * .75, box.y + box.height / 2);
    assert.equal(await page.evaluate(() => window.selectedIndex()), 1);
    await page.evaluate(() => {
      for (const view of ['table', 'chart']) {
        const input = document.querySelector(`#snapshot .wk-chart-view input[value=${view}]`);
        input.checked = true; input.dispatchEvent(new Event('change'));
      }
    });
    assert.equal(await page.locator('#snapshot .wk-chart-readout').textContent(), 'Second: Count 5');
    assert.deepEqual(problems, []);
    await page.close();
  });
  await check('chart: replacement, final removal, delayed attachment and reattachment manage observation', async () => {
    const { page, problems } = await open('/demo/index.html');
    const result = await page.evaluate(async () => {
      // Count observed plots on the prototype: the kit's observer may exist before this patch.
      const P = ResizeObserver.prototype, { observe, unobserve } = P, seen = new Set();
      P.observe = function (el, o) { if (el.classList?.contains('wk-chart-plot')) seen.add(el); return observe.call(this, el, o); };
      P.unobserve = function (el) { seen.delete(el); return unobserve.call(this, el); };
      const { wkChart } = await import(document.body.dataset.module || '../src/index.js');
      const host = document.createElement('div');
      document.body.append(host);
      const spec = { id: 'leak', kind: 'bars', label: 'Leak', categories: [{ label: 'a' }, { label: 'b' }], series: [{ key: 's', label: 'S', tone: 1, values: [1, 2] }] };
      const settle = async () => { for (let i = 0; i < 3; i++) await new Promise(requestAnimationFrame); };
      for (let i = 0; i < 6; i++) { host.replaceChildren(wkChart(spec)); await settle(); }
      const replaced = seen.size;
      host.replaceChildren();
      await settle();
      const removed = seen.size;
      const chart = wkChart(spec);
      // Let the old implementation's initial cleanup tick run before attachment.
      await new Promise(r => setTimeout(r, 20));
      const detached = seen.size;
      host.style.width = '410px';
      host.append(chart);
      await settle();
      const attached = seen.size;
      host.style.width = '270px';
      await settle();
      const plot = chart.querySelector('.wk-chart-plot');
      const resized = { plot: plot.clientWidth, svg: plot.querySelector('svg').getBoundingClientRect().width };
      host.remove();
      host.replaceChildren();
      await settle();
      const removedHost = seen.size;
      host.append(chart);
      document.body.append(host);
      await settle();
      return { replaced, removed, detached, attached, resized, removedHost, reattached: seen.size };
    });
    assert.equal(result.replaced, 1, 'only the chart in the page is observed');
    assert.equal(result.removed, 0, 'removing the last chart unobserves its plot without another paint');
    assert.equal(result.detached, 0, 'a detached chart is not retained by ResizeObserver');
    assert.equal(result.attached, 1, 'a chart attached after the initial tick becomes observed');
    assert.ok(result.resized.plot > 0 && Math.abs(result.resized.svg - result.resized.plot) <= .5, 'a delayed chart redraws after a later resize');
    assert.equal(result.removedHost, 0, 'removing a containing subtree unobserves the plot');
    assert.equal(result.reattached, 1, 'reattaching a containing subtree resumes observation');
    assert.deepEqual(problems, []);
    await page.close();
  });
  await check('chart: redraws to its width, and stays inside a narrow page', async () => {
    const { page, problems } = await open('/demo/index.html', { viewport: { width: 380, height: 800 } });
    const w = await page.locator('#demo-stacked svg').evaluate(el => ({ svg: el.getBoundingClientRect().width, plot: el.parentElement.getBoundingClientRect().width }));
    assert.ok(w.svg <= w.plot + .5, `chart ${w.svg} fits its ${w.plot} px`);
    assert.ok(await page.locator('#demo-charts *').evaluateAll(els => els.every(el => el.getBoundingClientRect().right <= window.innerWidth + .5)), 'nothing in a chart reaches past the page');
    assert.ok(await page.locator('#demo-stacked .wk-chart-tick').count() < 20, 'labels thin out on a narrow chart');
    await page.setViewportSize({ width: 1000, height: 800 });
    await page.waitForFunction(() => document.querySelector('#demo-stacked svg').getBoundingClientRect().width > 600);
    assert.deepEqual(problems, []);
    await page.close();
  });
  // ---- Diff (wk-diff) ----
  // The lines a diff shows, as [kind, old, new, code]: a unified row is one
  // line; a split row is its old half, then its new half unless that is the
  // same context line.
  const diffLines = (page, id) => page.locator(`#${id}`).evaluate(root => {
    const out = [];
    for (const row of root.querySelectorAll('.wk-diff-line')) {
      const halves = row.classList.contains('wk-diff-pair') ? [...row.children] : [row];
      for (const h of halves) {
        if (h.dataset.kind === 'empty' || (h.dataset.side === 'new' && h.dataset.kind === 'context')) continue;
        const nums = [...h.querySelectorAll('.wk-diff-num')].map(n => n.textContent);
        const old = h.dataset.side ? (h.dataset.side === 'old' ? nums[0] : '') : nums[0];
        const neu = h.dataset.side ? (h.dataset.side === 'new' ? nums[0] : (h.dataset.kind === 'context' ? row.lastElementChild.querySelector('.wk-diff-num').textContent : '')) : nums[1];
        out.push([h.dataset.kind, old, neu, h.querySelector('.wk-diff-code').textContent]);
      }
    }
    return out;
  });

  await check('diff: unified and split show the same lines with the right numbers, markers, labels and word marks; chips, counts and collapsed files', async () => {
    const { page, problems } = await open();
    const unified = await diffLines(page, 'demo-diff');
    await page.locator('#demo-diff-split').scrollIntoViewIfNeeded();
    const split = await diffLines(page, 'demo-diff-split');
    // The split specimen holds the first two files of the unified one.
    assert.ok(split.length > 20);
    const sorted = list => list.map(l => JSON.stringify(l)).sort();
    assert.deepEqual(sorted(split), sorted(unified.slice(0, split.length)), 'split and unified show the same lines');
    // Switching the view rebuilds the same lines.
    await page.locator('#demo-diff-split-view-unified').check({ force: true });
    assert.deepEqual(await diffLines(page, 'demo-diff-split'), unified.slice(0, split.length));
    await page.locator('#demo-diff-split-view-split').check({ force: true });
    // Line numbers follow the hunk headers.
    assert.deepEqual(unified.slice(0, 5), [
      ['context', '11', '11', 'type Server struct {'], ['context', '12', '12', '\taddr    string'], ['del', '13', '', '\ttimeout time.Duration'],
      ['add', '', '13', '\ttimeout  time.Duration'], ['add', '', '14', '\tmaxBytes int64'],
    ]);
    assert.deepEqual(unified.find(l => l[3] === '\t}()'), ['context', '35', '38', '\t}()']);
    // Markers and screen-reader labels, not colour alone.
    const first = page.locator('#demo-diff .wk-diff-line[data-kind=del]').first();
    assert.equal(await first.locator('.wk-diff-mark').textContent(), '−');
    assert.equal(await page.locator('#demo-diff .wk-diff-line[data-kind=add] .wk-diff-mark').first().textContent(), '+');
    assert.equal(await first.locator('.wk-diff-sr').textContent(), 'Removed 13: ');
    const tree = await axTree(page);
    const names = tree.map(n => n.name?.value?.trim()).filter(Boolean);
    assert.ok(names.includes('Added 13:') && names.includes('Removed 13:'), `each line's kind reaches the accessibility tree: ${names.filter(n => /^(Added|Removed) /.test(n)).slice(0, 4)}`);
    assert.ok(!names.includes('\u2212'), 'the markers are hidden from the accessibility tree');
    // Word marks: only what changed, on the stronger tint.
    const marks = await page.locator('#demo-diff-file-1 mark.wk-diff-word').allTextContents();
    assert.deepEqual(marks, ['Serving', 'Running the server', 'five', 'ten']);
    const probe = token => page.evaluate(tok => { const s = document.createElement('span'); s.style.backgroundColor = `var(${tok})`; document.body.append(s); const c = getComputedStyle(s).backgroundColor; s.remove(); return c; }, token);
    const bg = loc => loc.evaluate(el => getComputedStyle(el).backgroundColor);
    assert.equal(await bg(page.locator('#demo-diff-file-1 .wk-diff-line[data-kind=del] mark').first()), await probe('--wk-color-diff-del-strong'));
    assert.equal(await bg(page.locator('#demo-diff-file-1 .wk-diff-line[data-kind=add] mark').first()), await probe('--wk-color-diff-add-strong'));
    assert.equal(await bg(first), await probe('--wk-color-diff-del'));
    assert.equal(await bg(page.locator('#demo-diff .wk-diff-line[data-kind=add]').first()), await probe('--wk-color-diff-add'));
    // Headers: a rename reads old → new, chips and counts; binary and generated files start collapsed.
    const heads = await page.locator('#demo-diff .wk-diff-file').evaluateAll(files => files.map(f => ({
      path: f.querySelector('.wk-diff-toggle').textContent, open: f.querySelector('.wk-diff-toggle').getAttribute('aria-expanded'),
      chips: [...f.querySelectorAll('.wk-diff-chip')].map(c => c.textContent), counts: f.querySelector('.wk-diff-counts [aria-hidden]')?.textContent ?? null,
      rows: f.querySelectorAll('.wk-diff-line').length,
    })));
    assert.deepEqual(heads, [
      { path: 'server/server.go', open: 'true', chips: [], counts: '+10−7', rows: 28 },
      { path: 'docs/serve.md →  renamed to docs/running.md', open: 'true', chips: ['renamed'], counts: '+2−2', rows: 6 },
      { path: 'assets/icon.png', open: 'false', chips: ['binary'], counts: null, rows: 0 },
      { path: 'data/index.lock.json', open: 'false', chips: ['generated'], counts: '+300−300', rows: 0 },
      { path: 'server/limits.go', open: 'true', chips: ['added'], counts: '+4−0', rows: 4 },
    ]);
    assert.equal(await page.locator('#demo-diff-file-3 .wk-diff-counts .wk-diff-sr').textContent(), '300 lines added, 300 lines removed');
    // Expanding builds the rows then; collapsing hides them.
    await page.locator('#demo-diff-file-2-toggle').click();
    assert.equal(await page.locator('#demo-diff-file-2 .wk-diff-note').textContent(), 'Binary file not shown.');
    await page.locator('#demo-diff-file-3-toggle').click();
    assert.equal(await page.locator('#demo-diff-file-3 .wk-diff-line').count(), 603);
    await page.locator('#demo-diff-file-3-toggle').click();
    assert.equal(await page.locator('#demo-diff-file-3-body').isVisible(), false);
    // Show more fetches the hidden context above a hunk and numbers it.
    const more = page.locator('#demo-diff-file-0 .wk-diff-more').first();
    assert.equal(await more.textContent(), 'Show 10 hidden lines');
    await more.click();
    await until(page, () => !document.querySelector('#demo-diff-file-0 .wk-diff-hunk[data-hunk="0"] .wk-diff-more'));
    const top = (await diffLines(page, 'demo-diff')).slice(0, 2);
    assert.deepEqual(top, [['context', '1', '1', 'package server'], ['context', '2', '2', '']]);
    assert.equal(await active(page), (await page.locator('#demo-diff-file-0 .wk-diff-hunk').first().textContent()), 'focus moves to the hunk when its button goes');
    const second = page.locator('#demo-diff-file-0 .wk-diff-more');
    assert.equal(await second.textContent(), 'Show 8 hidden lines');
    await second.click();
    await until(page, () => !document.querySelector('#demo-diff-file-0 .wk-diff-more'));
    assert.deepEqual((await diffLines(page, 'demo-diff')).find(l => l[1] === '20'), ['context', '20', '21', '}']);
    // Gutters, markers and labels cannot be selected; long lines scroll in the unified view and wrap in the split one.
    const look = await page.evaluate(() => {
      const css = sel => getComputedStyle(document.querySelector(sel));
      return {
        select: ['.wk-diff-num', '.wk-diff-mark', '.wk-diff-sr'].map(s => css(`#demo-diff ${s}`).userSelect),
        code: css('#demo-diff .wk-diff-code').userSelect,
        unified: css('#demo-diff .wk-diff-code').whiteSpace, split: css('#demo-diff-split .wk-diff-code').whiteSpace,
      };
    });
    assert.deepEqual(look, { select: ['none', 'none', 'none'], code: 'auto', unified: 'pre', split: 'pre-wrap' });
    // Dark has its own tints.
    const dark = await open('/demo/index.html', { colorScheme: 'dark' });
    const darkAdd = await dark.page.locator('#demo-diff .wk-diff-line[data-kind=add]').first().evaluate(el => getComputedStyle(el).backgroundColor);
    assert.notEqual(darkAdd, await probe('--wk-color-diff-add'));
    assert.deepEqual(dark.problems, []); await dark.page.close();
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff: n and p move between hunks, ] and [ between files; copying takes the code without gutters or markers', async () => {
    const { page, problems } = await open();
    const diff = page.locator('#demo-diff');
    assert.equal(await diff.getAttribute('tabindex'), '0');
    assert.equal(await diff.getAttribute('aria-label'), 'Changes in this example');
    assert.equal(await diff.getAttribute('aria-keyshortcuts'), 'n p ] [');
    await diff.focus();
    const hunk = () => page.evaluate(() => { const a = document.activeElement; return a.classList.contains('wk-diff-hunk') ? `${a.closest('.wk-diff-file').id}#${a.dataset.hunk}` : a.id || a.tagName; });
    await page.keyboard.press('n'); assert.equal(await hunk(), 'demo-diff-file-0#0');
    await page.keyboard.press('n'); assert.equal(await hunk(), 'demo-diff-file-0#1');
    await page.keyboard.press('n'); assert.equal(await hunk(), 'demo-diff-file-1#0');
    await page.keyboard.press('n'); assert.equal(await hunk(), 'demo-diff-file-4#0', 'collapsed files are skipped');
    await page.keyboard.press('n'); assert.equal(await hunk(), 'demo-diff-file-4#0', 'the last hunk stays');
    await page.keyboard.press('p'); assert.equal(await hunk(), 'demo-diff-file-1#0');
    await page.keyboard.press('['); assert.equal(await hunk(), 'demo-diff-file-1-toggle');
    await page.keyboard.press('['); assert.equal(await hunk(), 'demo-diff-file-0-toggle');
    await page.keyboard.press(']'); await page.keyboard.press(']'); assert.equal(await hunk(), 'demo-diff-file-2-toggle', 'files move through collapsed ones too');
    await page.keyboard.press('n'); assert.equal(await hunk(), 'demo-diff-file-4#0');
    // The keys do nothing with a modifier.
    await page.keyboard.press('Control+p'); assert.equal(await hunk(), 'demo-diff-file-4#0');
    // Copy: a selection across rows gives the code only, one line each.
    const copied = id => page.evaluate(id => {
      const rows = [...document.querySelectorAll(`#${id}-file-0 .wk-diff-line`)].slice(1, 5);
      const range = document.createRange();
      range.setStart(rows[0], 0); range.setEnd(rows.at(-1), rows.at(-1).childNodes.length);
      const sel = getSelection(); sel.removeAllRanges(); sel.addRange(range);
      const data = new DataTransfer();
      const e = new ClipboardEvent('copy', { clipboardData: data, bubbles: true, cancelable: true });
      document.getElementById(id).querySelector('.wk-diff-line').dispatchEvent(e);
      return { text: data.getData('text/plain'), handled: e.defaultPrevented, native: sel.toString() };
    }, id);
    const u = await copied('demo-diff');
    assert.ok(u.handled);
    assert.equal(u.text, '\taddr    string\n\ttimeout time.Duration\n\ttimeout  time.Duration\n\tmaxBytes int64');
    for (const glyph of ['−', 'Added', 'Removed', '13']) assert.ok(!u.native.includes(glyph), `the browser's own selection leaves out ${glyph}`);
    // Split: only the side the selection starts on.
    const sp = await copied('demo-diff-split');
    assert.equal(sp.text, '\taddr    string\n\ttimeout time.Duration\n\tmux     *http.ServeMux');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff: forced colours keep added and removed apart, by marker and by bar', async () => {
    const { page, problems } = await open('/demo/index.html', { forcedColors: 'active' });
    const look = await page.evaluate(() => {
      const row = sel => { const el = document.querySelector(`#demo-diff ${sel}`); const cs = getComputedStyle(el); return { style: cs.borderInlineStartStyle, width: cs.borderInlineStartWidth, mark: el.querySelector('.wk-diff-mark').textContent }; };
      const word = getComputedStyle(document.querySelector('#demo-diff mark.wk-diff-word')).textDecorationLine;
      const probe = document.createElement('span'); probe.style.backgroundColor = 'Canvas'; document.body.append(probe);
      const canvas = getComputedStyle(probe).backgroundColor; probe.remove();
      const marks = [...document.querySelectorAll('mark.wk-diff-word')].map(m => getComputedStyle(m).backgroundColor);
      return { canvas, marks: [...new Set(marks)], count: marks.length, add: row('.wk-diff-line[data-kind=add]'), del: row('.wk-diff-line[data-kind=del]'), half: getComputedStyle(document.querySelector('#demo-diff-split .wk-diff-half[data-kind=del]')).borderInlineStartStyle, word };
    });
    const { canvas, marks, count, ...rest } = look;
    assert.ok(count > 10, `${count} word marks`);
    assert.deepEqual(marks, [canvas], 'every word mark, added or removed, is on Canvas in forced colours');
    assert.deepEqual(rest, { add: { style: 'solid', width: '4px', mark: '+' }, del: { style: 'dashed', width: '4px', mark: '−' }, half: 'dashed', word: 'underline' });
    const shot = sel => page.locator(sel).first().screenshot();
    await page.locator('#demo-diff-file-1').scrollIntoViewIfNeeded();
    // Two rows with the same text apart from their kind still look different.
    const [del, add] = await Promise.all([shot('#demo-diff-file-1 .wk-diff-line[data-kind=del] .wk-diff-mark'), shot('#demo-diff-file-1 .wk-diff-line[data-kind=add] .wk-diff-mark')]);
    assert.ok(!del.equals(add));
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff: parsed input with unknown kinds and no line numbers; highlight that throws; an empty diff; onrender', async () => {
    const { page, problems } = await open();
    const r = await page.evaluate(async () => {
      const { wkDiff } = await import('../src/index.js');
      const out = {};
      const t = performance.now();
      const parsed = [{ path: 'x.txt', hunks: [{ header: '@@ -5,3 +5,3 @@', lines: [{ kind: 'context', text: 'a' }, { kind: 'added', text: 'odd' }, { kind: 'del', text: 'b' }, { kind: 'add', text: 'c' }] }] }];
      for (const view of ['unified', 'split']) {
        const el = wkDiff({ id: `kinds-${view}`, diff: parsed, view });
        out[view] = [...el.querySelectorAll('.wk-diff-sr')].filter(x => !x.closest('[aria-hidden]')).map(x => x.textContent).filter(x => /^(Line|Added|Removed) /.test(x)); // what a screen reader reaches
      }
      out.fast = performance.now() - t < 1000;
      const lit = wkDiff({ id: 'lit', diff: 'diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n-foo bar\n+foo baz\n', highlight: (f, l) => { if (l.kind === 'del') throw new Error('boom'); const s = document.createElement('b'); s.textContent = l.text; return s; } });
      out.lit = [...lit.querySelectorAll('.wk-diff-code')].map(c => [c.textContent, c.querySelectorAll('b').length, c.querySelectorAll('mark').length]);
      const shared = document.createElement('span'); shared.className = 'lit'; shared.textContent = 'same';
      const frag = document.createDocumentFragment(); frag.append(Object.assign(document.createElement('i'), { textContent: 'frag' }));
      const reuse = wkDiff({ id: 'reuse', diff: 'diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1,3 +1,3 @@\n a\n-b\n+c\n', highlight: (f, l) => l.kind === 'add' ? frag : shared });
      out.reuse = [...reuse.querySelectorAll('.wk-diff-code')].map(c => c.textContent);
      const placed = document.createElement('b'); placed.textContent = 'on page'; document.body.append(placed);
      const onPage = wkDiff({ id: 'onpage', diff: 'diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n-x\n+y\n', highlight: () => placed });
      out.onPage = [placed.parentNode === document.body, ...[...onPage.querySelectorAll('.wk-diff-code')].map(c => c.textContent)];
      placed.remove();
      const empty = wkDiff({ id: 'none', diff: '', emptyText: 'Nothing changed' });
      out.empty = [empty.textContent, empty.querySelectorAll('.wk-diff-head, .wk-segmented').length, empty.hasAttribute('tabindex')];
      const calls = [];
      const big = 'diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1,500 +1,500 @@\n' + Array.from({ length: 500 }, (_, i) => ` line ${i}`).join('\n') + '\n';
      const wide = 'diff --git a/m.js b/m.js\n--- a/m.js\n+++ b/m.js\n@@ -1 +1 @@\n-' + 'a'.repeat(120000) + '\n+' + 'b'.repeat(120000) + '\n';
      const two = wkDiff({ id: 'hooks', diff: big + wide + 'diff --git a/s b/s\n--- a/s\n+++ b/s\n@@ -1 +1 @@\n-x\n+y\n', onrender: (box, file) => calls.push(`${box.id}:${file.path}`) });
      document.body.append(two);
      out.collapsedByChars = two.querySelector('#hooks-file-1-toggle').getAttribute('aria-expanded');
      out.afterBuild = [...calls];
      two.querySelector('#hooks-file-0-toggle').click();
      out.afterExpand = [...calls];
      two.querySelector('#hooks-view-split').click();
      out.afterSwitch = [...calls];
      two.remove();
      return out;
    });
    assert.ok(r.fast);
    assert.deepEqual(r.unified, ['Line 5: ', 'Line 6: ', 'Removed 7: ', 'Added 7: '], 'an unknown kind reads as context; numbers come from the header');
    assert.deepEqual(r.split, r.unified);
    assert.deepEqual(r.lit, [['foo bar', 0, 1], ['foo baz', 1, 1]], 'a highlighter that throws leaves plain text (with its word marks) for that line');
    assert.deepEqual(r.reuse, ['same', 'same', 'frag'], 'a node returned for every line is copied, not moved');
    assert.deepEqual(r.onPage, [true, 'on page', 'on page'], 'a node already on the page stays there; the line gets a copy');
    assert.deepEqual(r.empty, ['Nothing changed', 0, false]);
    assert.equal(r.collapsedByChars, 'false', 'a file over 200,000 characters starts collapsed');
    assert.deepEqual(r.afterBuild, ['hooks-file-2:s']);
    assert.deepEqual(r.afterExpand, ['hooks-file-2:s', 'hooks-file-0:a']);
    assert.deepEqual(r.afterSwitch.slice(2).sort(), ['hooks-file-0:a', 'hooks-file-2:s']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff: Show more keeps working through a view switch, keeps its button on errors, and goes on an empty result', async () => {
    const { page, problems } = await open();
    const r = await page.evaluate(async () => {
      const { wkDiff } = await import('../src/index.js');
      const text = 'diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -30,3 +30,3 @@\n a\n-b\n+c\n d\n';
      const source = Array.from({ length: 100 }, (_, i) => `line ${i + 1}`);
      const tick = () => new Promise(res => setTimeout(res, 0));
      const out = {};
      // A view switch while the lines load: they land in the view shown then.
      let release;
      const race = wkDiff({ id: 'race', diff: text, getContext: (f, range) => new Promise(res => { release = () => res(source.slice(range.oldStart - 1, range.oldEnd)); }) });
      document.body.append(race);
      race.querySelector('.wk-diff-more').click(); await tick();
      race.querySelector('#race-view-split').click(); await tick();
      out.pendingDisabled = race.querySelector('.wk-diff-more').getAttribute('aria-disabled');
      release(); await tick(); await tick();
      out.split = [...race.querySelectorAll('.wk-diff-half[data-side=old]')].map(h => h.dataset.old).join(',');
      out.label = race.querySelector('.wk-diff-more')?.textContent;
      race.querySelector('#race-view-unified').click();
      out.unified = [...race.querySelectorAll('.wk-diff-line')].map(h => h.dataset.old ?? '-').join(',');
      // Errors: rejected, then thrown; the button stays, with a note; a retry that works clears it.
      for (const kind of ['rejects', 'throws']) {
        let fail = true;
        const el = wkDiff({ id: `err-${kind}`, diff: text, getContext: (f, range) => {
          if (fail) { if (kind === 'throws') throw new Error('x'); return Promise.reject(new Error('net')); }
          return source.slice(range.oldStart - 1, range.oldEnd);
        } });
        document.body.append(el);
        el.querySelector('.wk-diff-more').click(); await tick(); await tick();
        const more = el.querySelector('.wk-diff-more'), note = el.querySelector('.wk-diff-more-note');
        out[kind] = { button: Boolean(more), disabled: more?.getAttribute('aria-disabled'), note: note.textContent, live: note.getAttribute('aria-live'), color: getComputedStyle(note).color };
        fail = false;
        more.click(); await tick(); await tick();
        out[kind].retry = [el.querySelector('.wk-diff-more-note').textContent, el.querySelectorAll('.wk-diff-line').length];
        el.remove();
      }
      const probe = document.createElement('span'); probe.style.color = 'var(--wk-color-text-secondary)'; document.body.append(probe);
      out.secondary = getComputedStyle(probe).color; probe.remove();
      const empty = wkDiff({ id: 'empty-ctx', diff: text, getContext: () => [] });
      document.body.append(empty);
      empty.querySelector('.wk-diff-more').focus(); empty.querySelector('.wk-diff-more').click(); await tick(); await tick();
      out.empty = [empty.querySelectorAll('.wk-diff-more').length, document.activeElement.classList.contains('wk-diff-hunk')];
      return out;
    });
    const range = (a, b) => Array.from({ length: b - a + 1 }, (_, i) => a + i);
    assert.equal(r.pendingDisabled, 'true', 'the rebuilt button waits for the pending load');
    assert.equal(r.split, range(10, 32).join(','), 'the loaded lines appear in the view shown when they arrive');
    assert.equal(r.label, 'Show 9 hidden lines');
    assert.equal(r.unified, [...range(10, 31), '-', 32].join(','));
    for (const kind of ['rejects', 'throws']) {
      assert.deepEqual(r[kind], { button: true, disabled: null, note: 'Couldn’t load lines', live: null, color: r.secondary, retry: ['', 24] }, kind);
    }
    assert.deepEqual(r.empty, [0, true], 'an empty result removes the button and focus goes to the hunk');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff: a wrapped line hangs under its indent, and copying still gives its own whitespace', async () => {
    const { page, problems } = await open('/demo/index.html', { viewport: { width: 700, height: 900 } });
    const r = await page.evaluate(async () => {
      const { wkDiff } = await import('../src/index.js');
      const long = 'word '.repeat(40).trim();
      const el = wkDiff({ id: 'hang', wrap: true, diff: `diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1,2 +1,2 @@\n \t\t${long}\n-  ${long}\n+  ${long}!\n` });
      document.body.prepend(el);
      const codes = [...el.querySelectorAll('.wk-diff-code')];
      const geometry = codes.map(c => {
        const rects = [...(() => { const r = document.createRange(); r.selectNodeContents(c); return r.getClientRects(); })()];
        const lines = [...new Set(rects.map(x => Math.round(x.top)))].length;
        const range = document.createRange(); range.setStart(c.firstChild.nodeType === 3 ? c.firstChild : c.querySelector('mark')?.firstChild || c.firstChild, 0); range.collapse(true);
        return { indent: c.style.getPropertyValue('--wk-diff-indent'), lines, padding: parseFloat(getComputedStyle(c).paddingInlineStart), textIndent: parseFloat(getComputedStyle(c).textIndent) };
      });
      const range = document.createRange(); range.selectNodeContents(el.querySelector('.wk-diff-rows'));
      const sel = getSelection(); sel.removeAllRanges(); sel.addRange(range);
      const data = new DataTransfer();
      el.querySelector('.wk-diff-line').dispatchEvent(new ClipboardEvent('copy', { clipboardData: data, bubbles: true, cancelable: true }));
      return { geometry, copied: data.getData('text/plain'), long };
    });
    assert.deepEqual(r.geometry.map(g => g.indent), ['8', '2', '2'], 'tabs count to the next multiple of 4');
    for (const g of r.geometry) {
      assert.ok(g.lines > 1, 'the line wraps');
      assert.ok(g.padding > 0 && Math.abs(g.padding + g.textIndent) < .5, 'the first line starts at the edge and the rest hang under the indent');
    }
    assert.equal(r.copied, `\t\t${r.long}\n  ${r.long}\n  ${r.long}!`);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff: a split-view selection that starts on a header copies one side only', async () => {
    const { page, problems } = await open();
    const r = await page.evaluate(() => {
      const copy = (start, so, end, eo) => {
        const range = document.createRange(); range.setStart(start, so); range.setEnd(end, eo);
        const sel = getSelection(); sel.removeAllRanges(); sel.addRange(range);
        const data = new DataTransfer();
        document.querySelector('#demo-diff-split .wk-diff-line').dispatchEvent(new ClipboardEvent('copy', { clipboardData: data, bubbles: true, cancelable: true }));
        return data.getData('text/plain');
      };
      const split = document.getElementById('demo-diff-split');
      const rows = [...split.querySelectorAll('#demo-diff-split-file-0 .wk-diff-line')];
      const oldCode = rows[2].querySelector('[data-side=old] .wk-diff-code');
      return {
        toNew: copy(split.querySelector('.wk-diff-hunk'), 0, rows[3], rows[3].childNodes.length),
        toOld: copy(split.querySelector('.wk-diff-file-head'), 0, oldCode, oldCode.childNodes.length),
      };
    });
    assert.equal(r.toNew, 'type Server struct {\n\taddr    string\n\ttimeout  time.Duration\n\tmaxBytes int64', 'it ends on the new side');
    assert.equal(r.toOld, 'type Server struct {\n\taddr    string\n\ttimeout time.Duration', 'it ends on the old side');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff: a 5,000-line diff builds and expands within a budget, and stays inside a narrow page', async () => {
    const { page, problems } = await open('/demo/index.html', { viewport: { width: 380, height: 800 } });
    const timing = await page.evaluate(async () => {
      const { wkDiff } = await import('../src/index.js');
      const body = [];
      for (let i = 1; i <= 2500; i++) body.push(`-\tvalue_${i} := compute(${i}, "old")`, `+\tvalue_${i} := compute(${i}, "new", true)`);
      const text = `diff --git a/big.go b/big.go\n--- a/big.go\n+++ b/big.go\n@@ -1,2500 +1,2500 @@\n${body.join('\n')}\n`;
      let t = performance.now();
      const el = wkDiff({ id: 'big', diff: text, collapseAbove: 400 });
      document.body.append(el); void el.offsetHeight;
      const build = performance.now() - t;
      const collapsed = el.querySelector('.wk-diff-toggle').getAttribute('aria-expanded');
      t = performance.now();
      el.querySelector('.wk-diff-toggle').click(); void el.offsetHeight;
      const expand = performance.now() - t;
      return { build, expand, collapsed, rows: el.querySelectorAll('.wk-diff-line').length, marks: el.querySelectorAll('mark').length,
        page: [el, ...document.querySelectorAll('#demo-diff, #demo-diff .wk-diff-file, #demo-diff .wk-diff-body, #big .wk-diff-file, #big .wk-diff-body')].every(x => x.getBoundingClientRect().right <= innerWidth + .5), scrolls: el.querySelector('.wk-diff-body').scrollWidth > el.querySelector('.wk-diff-body').clientWidth };
    });
    assert.equal(timing.collapsed, 'false', 'a large file starts collapsed');
    assert.equal(timing.rows, 5000);
    assert.equal(timing.marks, 7500, 'three changed words in each pair');
    assert.ok(timing.build < 200, `built in ${timing.build.toFixed(0)} ms`);
    assert.ok(timing.expand < 2500, `expanded 5,000 rows in ${timing.expand.toFixed(0)} ms`);
    assert.ok(timing.page, 'no diff reaches past the page');
    assert.ok(timing.scrolls, 'long lines scroll inside the file');
    assert.deepEqual(problems, []);
    await page.close();
  });

  // ---- Diff review comments ----
  // A small diff for the comment checks, at the top of the page: new lines
  // 1-7 in the first hunk (3 and 4 added, old 3 removed), 21-23 in the second.
  const mountReview = (page, extra = {}) => page.evaluate(async extra => {
    const { wkDiff } = await import('../src/index.js');
    const text = 'diff --git a/x/f.go b/x/f.go\n--- a/x/f.go\n+++ b/x/f.go\n@@ -1,6 +1,7 @@\n a1\n a2\n-a3\n+b3\n+b4\n a4\n a5\n a6\n@@ -20,3 +21,3 @@\n c20\n-c21\n+d22\n c22\n';
    window.rv = { log: [], pending: [] };
    const commentBox = extra.injected ? spec => {
      window.rv.log.push(['make', spec.label, spec.value, spec.placeholder]);
      const element = document.createElement('div');
      element.className = 'stub-box';
      const area = document.createElement('textarea');
      area.className = 'wk-field'; area.value = spec.value; area.setAttribute('aria-label', spec.label);
      // The box handles Escape and Ctrl+Enter itself, as an editor would.
      area.addEventListener('keydown', e => {
        // swallow: the box takes Escape for itself (closing its own popup, say) and does not cancel.
        if (e.key === 'Escape' && extra.swallow) { e.preventDefault(); window.rv.log.push(['box-own-escape']); return; }
        if (e.key === 'Escape') { e.preventDefault(); window.rv.log.push(['box-cancel']); spec.onCancel(); }
        if (e.key === 'Enter' && e.ctrlKey) { e.preventDefault(); window.rv.log.push(['box-submit']); spec.onSubmit(); }
      });
      element.append(area);
      // A disabled editor's surface: contenteditable="false", still focusable.
      const frozen = document.createElement('div');
      frozen.className = 'stub-frozen'; frozen.contentEditable = 'false'; frozen.tabIndex = 0; frozen.textContent = 'read-only';
      element.append(frozen);
      if (extra.bare) { element.focus = () => area.focus(); element.destroy = () => window.rv.log.push(['destroy', spec.label]); Object.defineProperty(element, 'value', { get: () => area.value }); return element; }
      return { element, get value() { return area.value; }, focus: () => area.focus(), destroy: () => window.rv.log.push(['destroy', spec.label]) };
    } : extra.throwing ? () => { throw new Error('no editor'); } : undefined;
    const diff = wkDiff({
      id: 'rv', label: 'Review', diff: extra.text || text, view: extra.view, comments: extra.comments, commentBox,
      now: () => window.rvNow ?? Date.parse('2026-11-23T12:00:00Z'), author: { name: 'Me', avatar: extra.avatar },
      canComment: extra.cannot ? (file, side, line) => !(side === 'new' && line === 6) : undefined,
      oncomment: c => { window.rv.log.push(['comment', c]); return extra.sync ? undefined : new Promise((resolve, reject) => window.rv.pending.push({ resolve, reject, c })); },
      onreply: (id, body) => { window.rv.log.push(['reply', id, body]); return extra.sync ? undefined : new Promise((resolve, reject) => window.rv.pending.push({ resolve, reject })); },
      onresolve: (id, resolved) => { window.rv.log.push(['resolve', id, resolved]); if (extra.asyncResolve) return new Promise((resolve, reject) => window.rv.pending.push({ resolve, reject })); },
    });
    window.rv.diff = diff;
    document.getElementById('demo').prepend(diff);
  }, extra);
  const rvLog = page => page.evaluate(() => window.rv.log);
  // What has focus in the review diff: a line as side:number, a box by its label, else its text.
  const rvFocus = page => page.evaluate(() => {
    const a = document.activeElement;
    if (a.classList.contains('wk-diff-line')) return a.classList.contains('wk-diff-pair') ? `${a.dataset.focusSide}:${a.querySelector(`[data-side="${a.dataset.focusSide}"]`).dataset[a.dataset.focusSide]}` : `${a.dataset.kind === 'del' ? 'old' : 'new'}:${a.dataset.kind === 'del' ? a.dataset.old : a.dataset.new}`;
    return a.getAttribute('aria-label') || a.textContent.trim();
  });
  const boxes = page => page.locator('#rv .wk-diff-compose').evaluateAll(fs => fs.map(f => [f.querySelector('[aria-label]').getAttribute('aria-label'), f.querySelector('textarea').value, [...f.closest('.wk-diff-comments').previousElementSibling.querySelectorAll('.wk-diff-code')].map(c => c.textContent).join('|')]));

  await check('diff comments: the + button on hover and focus opens a box under the line; c opens one from the keyboard; Escape cancels and returns focus', async () => {
    const { page, problems } = await open();
    await mountReview(page);
    const rv = page.locator('#rv');
    assert.equal(await rv.getAttribute('aria-keyshortcuts'), 'n p ] [ N P c');
    // Hover: the button moves to the line under the pointer, named by file and line.
    await rv.locator('.wk-diff-line[data-new="5"]').hover();
    const plus = rv.locator('.wk-diff-add-comment');
    assert.equal(await plus.count(), 1);
    assert.equal(await plus.evaluate(b => b.parentElement.dataset.new), '5');
    assert.equal(await plus.getAttribute('aria-label'), 'Comment on x/f.go, line 5');
    assert.equal(await plus.getAttribute('tabindex'), '-1');
    await rv.locator('.wk-diff-line[data-old="3"]').hover();
    assert.equal(await plus.getAttribute('aria-label'), 'Comment on x/f.go, old line 3', 'a removed line takes a comment on the old side');
    await rv.locator('.wk-diff-line[data-new="5"]').hover();
    await plus.click();
    assert.equal(await rvFocus(page), 'Comment on x/f.go, line 5', 'the box opens focused');
    assert.deepEqual(await boxes(page), [['Comment on x/f.go, line 5', '', 'a4']], 'the box sits in a row under its line');
    assert.equal(await rv.locator('.wk-diff-compose textarea').getAttribute('class'), 'wk-field wk-diff-compose-field');
    assert.deepEqual(await rv.locator('.wk-diff-compose button').allTextContents(), ['Cancel', 'Comment']);
    await page.keyboard.type('typed');
    await page.keyboard.press('Escape');
    assert.equal(await rv.locator('.wk-diff-compose').count(), 0);
    assert.equal(await rvFocus(page), 'new:5', 'Escape returns focus to the line');
    // Keyboard: lines take focus; arrows move, the + button follows focus, c opens a box.
    const five = await rv.locator('.wk-diff-line[data-new="5"] .wk-diff-code').boundingBox();
    await page.mouse.move(five.x + 40, five.y + five.height / 2);
    await page.keyboard.press('ArrowDown');
    assert.equal(await rvFocus(page), 'new:6');
    assert.equal(await plus.evaluate(b => b.parentElement.dataset.new), '6', 'the + button shows on the focused line');
    // A mouse event at the same point (the browser sends one when the page under a
    // still pointer changes) leaves the button with the keyboard.
    await page.evaluate(([x, y]) => document.elementFromPoint(x, y).dispatchEvent(new MouseEvent('mousemove', { bubbles: true, clientX: x, clientY: y })), [five.x + 40, five.y + five.height / 2]);
    assert.equal(await plus.evaluate(b => b.parentElement.dataset.new), '6', 'a pointer that did not move does not take the button');
    await page.keyboard.press('ArrowUp'); await page.keyboard.press('ArrowUp');
    assert.equal(await rvFocus(page), 'new:4');
    await page.keyboard.press('c');
    assert.equal(await rvFocus(page), 'Comment on x/f.go, line 4');
    await page.keyboard.press('Escape');
    assert.equal(await rvFocus(page), 'new:4');
    // From a hunk header, ArrowDown reaches the first line; ArrowUp goes back.
    await rv.locator('.wk-diff-hunk').first().focus();
    await page.keyboard.press('ArrowDown');
    assert.equal(await rvFocus(page), 'new:1');
    await page.keyboard.press('ArrowUp');
    assert.equal(await page.evaluate(() => document.activeElement.dataset.hunk), '0');
    await page.keyboard.press('ArrowDown');
    // Cancel by button returns focus too; the screen-reader hint names the keys.
    await page.keyboard.press('c');
    await rv.locator('.wk-diff-compose button', { hasText: 'Cancel' }).click();
    assert.equal(await rvFocus(page), 'new:1');
    assert.match(await page.locator('#rv-keys').textContent(), /N or P for the next or previous comment thread.*press c to comment/);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments: a range by dragging the + button, by Shift-clicking line numbers and by Shift with the arrow keys; a range stays in one hunk; canComment', async () => {
    const { page, problems } = await open();
    await mountReview(page, { cannot: true });
    const rv = page.locator('#rv');
    const row = n => rv.locator(`.wk-diff-line[data-new="${n}"]`);
    // Drag from line 1 to line 4.
    await row(1).hover();
    const b = await rv.locator('.wk-diff-add-comment').boundingBox();
    const to = await row(4).boundingBox();
    await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2);
    await page.mouse.down();
    await page.mouse.move(b.x + b.width / 2, to.y + to.height / 2, { steps: 4 });
    assert.equal(await rv.locator('[data-selected]').count(), 5, 'the rows dragged over are marked (old 3 lies between)');
    await page.mouse.up();
    assert.equal(await rvFocus(page), 'Comment on x/f.go, lines 1 to 4');
    assert.equal(await rv.locator('[data-selected]').count(), 0);
    await page.keyboard.press('Escape');
    assert.equal(await rvFocus(page), 'new:4', 'Escape returns to the range’s last line');
    // Shift-click line numbers: 2, then 5.
    await row(2).locator('.wk-diff-num').nth(1).click();
    await row(5).locator('.wk-diff-num').nth(1).click({ modifiers: ['Shift'] });
    assert.equal(await rv.locator('[data-selected]').count(), 5);
    await page.keyboard.press('c');
    assert.equal(await rvFocus(page), 'Comment on x/f.go, lines 2 to 5');
    // A click on the + inside a selection comments on the selection.
    await page.keyboard.press('Escape');
    await row(1).locator('.wk-diff-num').nth(1).click();
    await row(2).locator('.wk-diff-num').nth(1).click({ modifiers: ['Shift'] });
    await row(2).hover();
    await rv.locator('.wk-diff-add-comment').click();
    assert.equal(await rvFocus(page), 'Comment on x/f.go, lines 1 to 2');
    await page.keyboard.press('Escape');
    // Shift with the arrows, then c; Escape on the line clears a selection.
    await row(1).focus();
    await page.keyboard.press('Shift+ArrowDown'); await page.keyboard.press('Shift+ArrowDown');
    assert.equal(await rv.locator('[data-selected]').count(), 3);
    await page.keyboard.press('Escape');
    assert.equal(await rv.locator('[data-selected]').count(), 0);
    await row(1).focus();
    await page.keyboard.press('Shift+ArrowDown');
    await page.keyboard.press('c');
    assert.equal(await rvFocus(page), 'Comment on x/f.go, lines 1 to 2');
    await page.keyboard.press('Escape');
    // The selection stops at the hunk header.
    await row(7).focus();
    await page.keyboard.press('Shift+ArrowDown');
    assert.equal(await rvFocus(page), 'new:7');
    // canComment: line 6 takes no comment: no + button and c does nothing.
    await row(6).hover();
    assert.equal(await rv.locator('.wk-diff-add-comment').count(), 0);
    await row(6).focus();
    await page.keyboard.press('c');
    assert.equal(await rvFocus(page), 'new:6');
    assert.equal(await rv.locator('.wk-diff-compose').count(), 0);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments: several boxes keep their drafts through a view switch; the split view puts each under its side; left and right change side', async () => {
    const { page, problems } = await open();
    await mountReview(page);
    const rv = page.locator('#rv');
    for (const n of [2, 4]) {
      await rv.locator(`.wk-diff-line[data-new="${n}"]`).focus();
      await page.keyboard.press('c');
      await page.keyboard.type(`draft ${n}`);
    }
    await rv.locator('.wk-diff-line[data-old="3"]').focus();
    await page.keyboard.press('c');
    await page.keyboard.type('on the old side');
    const before = await boxes(page);
    assert.deepEqual(before.map(b => b.slice(0, 2)), [['Comment on x/f.go, line 2', 'draft 2'], ['Comment on x/f.go, old line 3', 'on the old side'], ['Comment on x/f.go, line 4', 'draft 4']]);
    await page.locator('#rv-view-split').check({ force: true });
    assert.deepEqual((await boxes(page)).map(b => b.slice(0, 2)).sort(), before.map(b => b.slice(0, 2)).sort(), 'drafts survive the switch');
    // Split: each box is in its side's column, under the pair holding its line.
    const columns = await rv.locator('.wk-diff-compose').evaluateAll(fs => fs.map(f => {
      const pair = f.closest('.wk-diff-comments').previousElementSibling;
      const half = pair.querySelector(`[data-side="${f.dataset.side}"]`);
      const a = f.getBoundingClientRect(), h = half.getBoundingClientRect();
      return [f.querySelector('textarea').value, f.dataset.side, half.dataset[f.dataset.side], Math.abs(a.left - h.left) < 20 && a.right <= h.right + 20];
    }));
    assert.deepEqual(columns.sort(), [['draft 2', 'new', '2', true], ['draft 4', 'new', '4', true], ['on the old side', 'old', '3', true]]);
    // In the split view, left and right move between the halves of a pair.
    await rv.locator('.wk-diff-pair').nth(2).focus();
    assert.equal(await rvFocus(page), 'new:3');
    await page.keyboard.press('ArrowLeft');
    assert.equal(await rvFocus(page), 'old:3');
    assert.equal(await rv.locator('.wk-diff-add-comment').evaluate(b => b.parentElement.dataset.side), 'old');
    const ring = await page.evaluate(() => getComputedStyle(document.activeElement.querySelector('[data-side="old"]')).outlineStyle);
    assert.equal(ring, 'solid', 'the focused half carries the ring');
    await page.keyboard.press('c');
    assert.equal(await rvFocus(page), 'Comment on x/f.go, old line 3', 'the box already open takes focus again');
    assert.equal(await rv.locator('.wk-diff-compose').count(), 3);
    await page.locator('#rv-view-unified').check({ force: true });
    assert.deepEqual(await boxes(page), before, 'and back in the unified view');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments: threads under their lines in both views, counts, resolve and expand, outdated at the top, N and P', async () => {
    const { page, problems } = await open();
    const entry = (author, time, body) => ({ author, time, body });
    await mountReview(page, { sync: true, comments: [
      { id: 't-new', path: 'x/f.go', side: 'new', line: 4, entries: [entry('ana', '2026-11-23T09:00:00Z', 'first\nsecond line'), entry('sam', '2026-11-23T11:58:00Z', 'reply')] },
      { id: 't-old', path: 'x/f.go', side: 'old', line: 3, entries: [entry('sam', '2026-11-22T10:00:00Z', 'why remove a3?')] },
      { id: 't-range', path: 'x/f.go', side: 'new', line: 21, endLine: 23, entries: [entry('ana', '2026-11-23T11:59:50Z', 'range')] },
      { id: 't-done', path: 'x/f.go', side: 'new', line: 1, resolved: true, entries: [entry('ana', '2026-11-20T10:00:00Z', 'a'), entry('sam', '2026-11-20T11:00:00Z', 'b'), entry('ana', '2026-11-20T12:00:00Z', 'c')] },
      { id: 't-gone', path: 'x/f.go', side: 'new', line: 40, entries: [entry('sam', '2026-10-01T10:00:00Z', 'old news')] },
      { id: 't-flag', path: 'x/f.go', side: 'new', line: 2, outdated: true, entries: [entry('sam', '2026-10-01T10:00:00Z', 'marked outdated')] },
      { id: 't-other', path: 'elsewhere.go', side: 'new', line: 1, entries: [entry('sam', '2026-10-01T10:00:00Z', 'not in this diff')] },
    ] });
    const rv = page.locator('#rv');
    const where = () => rv.locator('.wk-diff-thread').evaluateAll(ts => ts.map(t => {
      const host = t.closest('.wk-diff-comments');
      const anchor = host ? host.previousElementSibling : null;
      const cell = anchor && (anchor.classList.contains('wk-diff-pair') ? anchor.querySelector(`[data-side="${t.dataset.side}"]`) : anchor);
      return [t.dataset.thread, host ? `${t.dataset.side}:${cell.dataset[t.dataset.side]}` : t.closest('.wk-diff-outdated') ? 'outdated' : '?'];
    }));
    const unified = await where();
    assert.deepEqual(unified, [['t-gone', 'outdated'], ['t-flag', 'outdated'], ['t-done', 'new:1'], ['t-old', 'old:3'], ['t-new', 'new:4'], ['t-range', 'new:23']]);
    assert.equal(await rv.locator('.wk-diff-body > .wk-diff-outdated').evaluate(o => o === o.parentElement.firstElementChild), true, 'outdated threads are listed at the top of the file');
    assert.equal(await rv.locator('.wk-diff-comment-count').textContent(), '9 comments');
    assert.equal(await rv.locator('.wk-diff-comment-count').isVisible(), true);
    // Regions named by file and line; each comment read with its author and time.
    const thread = id => rv.locator(`.wk-diff-thread[data-thread="${id}"]`);
    assert.equal(await thread('t-new').getAttribute('role'), 'region');
    assert.equal(await thread('t-new').getAttribute('aria-label'), 'Comments on x/f.go, line 4');
    assert.equal(await thread('t-old').getAttribute('aria-label'), 'Comments on x/f.go, old line 3');
    assert.equal(await thread('t-range').getAttribute('aria-label'), 'Comments on x/f.go, lines 21 to 23');
    assert.equal(await thread('t-gone').getAttribute('aria-label'), 'Outdated comments on x/f.go, line 40');
    assert.equal(await thread('t-gone').locator('.wk-diff-thread-head').textContent(), 'Outdated · line 40');
    assert.deepEqual(await thread('t-new').locator('.wk-diff-comment').evaluateAll(cs => cs.map(c => c.textContent)), ['ana, 3 hours ago: first\nsecond line', 'sam, 2 minutes ago: reply']);
    assert.equal(await thread('t-range').locator('time').textContent(), 'just now');
    assert.equal(await thread('t-old').locator('time').textContent(), 'yesterday');
    assert.equal(await thread('t-new').locator('time').first().getAttribute('datetime'), '2026-11-23T09:00:00.000Z');
    // Resolved: one line until expanded.
    const toggle = thread('t-done').locator('[data-action=toggle]');
    assert.equal(await toggle.textContent(), 'Resolved · 3 comments');
    assert.equal(await toggle.getAttribute('aria-expanded'), 'false');
    assert.equal(await thread('t-done').locator('.wk-diff-comment').first().isVisible(), false);
    await toggle.click();
    assert.equal(await thread('t-done').locator('[data-action=toggle]').getAttribute('aria-expanded'), 'true');
    assert.equal(await thread('t-done').locator('.wk-diff-comment').first().isVisible(), true);
    assert.equal(await rvFocus(page), 'Resolved · 3 comments', 'focus stays on the toggle');
    // Split: the same lines, each thread in its side's column.
    await page.locator('#rv-view-split').check({ force: true });
    assert.deepEqual(await where(), unified);
    const col = id => thread(id).evaluate(t => { const r = t.getBoundingClientRect(), body = t.closest('.wk-diff-body').getBoundingClientRect(); return r.left - body.left < body.width / 2 ? 'left' : 'right'; });
    assert.deepEqual([await col('t-old'), await col('t-new'), await col('t-done')], ['left', 'right', 'right']);
    assert.equal(await thread('t-done').locator('[data-action=toggle]').getAttribute('aria-expanded'), 'true', 'an expanded thread stays expanded through a view switch');
    // Resolve and Reply call the app; setComments redraws, keeping focus on the thread.
    await thread('t-new').locator('[data-action=resolve]').click();
    assert.deepEqual((await rvLog(page)).at(-1), ['resolve', 't-new', true]);
    await page.evaluate(() => { const c = window.rv.diff; c.setComments([{ id: 't-new', path: 'x/f.go', side: 'new', line: 4, resolved: true, entries: [{ author: 'ana', time: 0, body: 'x' }] }]); });
    assert.equal(await rv.locator('.wk-diff-thread').count(), 1);
    assert.equal(await rv.locator('.wk-diff-comment-count').textContent(), '1 comment');
    assert.equal(await rvFocus(page), 'Resolved · 1 comment', 'focus moves from Resolve to the thread’s toggle');
    await page.evaluate(() => window.rv.diff.setComments([]));
    assert.equal(await rv.locator('.wk-diff-comment-count').isVisible(), false);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments: N and P step through threads, skipping collapsed files', async () => {
    const { page, problems } = await open();
    const e = { author: 'ana', time: 0, body: 'x' };
    await mountReview(page, { comments: [
      { id: 'a', path: 'x/f.go', side: 'new', line: 2, entries: [e] }, { id: 'b', path: 'x/f.go', side: 'new', line: 22, entries: [e] }, { id: 'c', path: 'x/f.go', side: 'new', line: 90, entries: [e] },
    ] });
    const rv = page.locator('#rv');
    await rv.focus();
    const at = () => page.evaluate(() => document.activeElement.dataset.thread || document.activeElement.className);
    await page.keyboard.press('Shift+N'); assert.equal(await at(), 'c', 'the outdated thread at the top comes first');
    await page.keyboard.press('Shift+N'); assert.equal(await at(), 'a');
    await page.keyboard.press('Shift+N'); assert.equal(await at(), 'b');
    await page.keyboard.press('Shift+N'); assert.equal(await at(), 'b');
    await page.keyboard.press('Shift+P'); assert.equal(await at(), 'a');
    await page.locator('#rv-file-0-toggle').click();
    await rv.focus();
    await page.keyboard.press('Shift+N'); assert.equal(await at(), 'wk-diff', 'threads in a collapsed file are skipped');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments: a comment posts as pending, then lands or fails with an ErrorBox, Retry and Edit; replies too', async () => {
    const { page, problems } = await open();
    await mountReview(page, { comments: [{ id: 't1', path: 'x/f.go', side: 'new', line: 7, entries: [{ author: 'ana', time: 0, body: 'hello' }] }] });
    const rv = page.locator('#rv');
    await rv.locator('.wk-diff-line[data-new="5"]').focus();
    await page.keyboard.press('c');
    await page.keyboard.type('  ');
    await page.keyboard.press('Control+Enter');
    assert.equal(await rv.locator('.wk-diff-compose').count(), 1, 'an empty comment is not sent');
    await page.keyboard.type('looks wrong');
    await page.keyboard.press('Control+Enter');
    assert.deepEqual((await rvLog(page)).at(-1), ['comment', { path: 'x/f.go', side: 'new', line: 5, endLine: 5, body: '  looks wrong' }]);
    const local = rv.locator('.wk-diff-thread[data-local]');
    assert.equal(await local.locator('.wk-diff-comment').getAttribute('data-state'), 'pending');
    assert.equal(await local.locator('.wk-diff-author').textContent(), 'Me');
    assert.equal(await rvFocus(page), 'Comments on x/f.go, line 5', 'focus moves to the pending comment');
    // The working dot shows only after 400 ms.
    assert.equal(await local.locator('.wk-diff-comment-state').isVisible(), false);
    await until(page, () => document.querySelector('#rv .wk-diff-comment-state')?.offsetParent);
    assert.equal(await local.locator('.wk-diff-comment-state').textContent(), 'Posting…');
    assert.equal(await local.locator('.wk-dot').getAttribute('data-tone'), 'working');
    const secondary = await page.evaluate(() => { const p = document.createElement('span'); p.style.color = 'var(--wk-color-text-secondary)'; document.querySelector('#rv').append(p); const c = getComputedStyle(p).color; p.remove(); return c; });
    assert.equal(await local.locator('.wk-diff-comment-body').evaluate(b => getComputedStyle(b).color), secondary);
    // It fails: an ErrorBox with the raw error, Retry and Edit; announced politely.
    await page.evaluate(() => window.rv.pending.shift().reject(new Error('HTTP 500: upstream timed out')));
    await until(page, () => document.querySelector('#rv .wk-diff-comment[data-state="error"]'));
    assert.equal(await local.locator('.wk-errorbox-head > span').textContent(), 'Couldn’t post the comment');
    assert.equal(await local.locator('.wk-errorbox').getAttribute('role'), 'group');
    assert.equal(await local.locator('.wk-errorbox pre').textContent(), 'HTTP 500: upstream timed out');
    assert.equal(await local.locator('.wk-copy').getAttribute('aria-label'), 'Copy error details');
    // Said by the window's announcer, not a region of the diff's own.
    assert.equal(await rv.locator('[aria-live]').count(), 0);
    await until(page, () => document.querySelector('[data-wk-announcer] [aria-live=polite]').textContent.includes('Couldn’t post the comment'));
    assert.equal(await rvFocus(page), 'Comments on x/f.go, line 5', 'focus stays on the thread');
    // Retry: pending again, then it lands: the app adds the thread and resolves.
    await local.locator('[data-action=retry]').click();
    assert.equal((await rvLog(page)).filter(l => l[0] === 'comment').length, 2);
    assert.equal(await local.locator('.wk-diff-comment').getAttribute('data-state'), 'pending');
    await page.evaluate(() => {
      const c = window.rv.pending.shift();
      window.rv.diff.setComments([{ id: 't1', path: 'x/f.go', side: 'new', line: 7, entries: [{ author: 'ana', time: 0, body: 'hello' }] }, { id: 't2', path: 'x/f.go', side: 'new', line: 5, entries: [{ author: 'Me', time: 0, body: c.c.body }] }]);
      c.resolve();
    });
    await until(page, () => !document.querySelector('#rv .wk-diff-thread[data-local]'));
    assert.equal(await rv.locator('.wk-diff-thread[data-thread="t2"] .wk-diff-comment-body').textContent(), '  looks wrong');
    await until(page, () => document.querySelector('[data-wk-announcer] [aria-live=polite]').textContent.includes('Comment posted'));
    assert.equal(await rvFocus(page), 'Comments on x/f.go, line 5', 'focus goes to the thread that landed in its place');
    // Edit after a failure puts the text back in a box.
    await rv.locator('.wk-diff-line[data-new="6"]').focus();
    await page.keyboard.press('c');
    await page.keyboard.type('second try');
    await rv.locator('.wk-diff-compose button', { hasText: 'Comment' }).click();
    await page.evaluate(() => window.rv.pending.shift().reject('offline'));
    await until(page, () => document.querySelector('#rv [data-action="edit"]'));
    assert.equal(await local.locator('.wk-errorbox pre').textContent(), 'offline');
    await local.locator('[data-action=edit]').click();
    assert.equal(await local.count(), 0);
    assert.equal(await rvFocus(page), 'Comment on x/f.go, line 6');
    assert.equal(await rv.locator('.wk-diff-compose textarea').inputValue(), 'second try');
    await page.keyboard.press('Escape');
    // A reply: Reply opens a box in the thread; Escape returns to Reply; a failed reply says so.
    const t1 = rv.locator('.wk-diff-thread[data-thread="t1"]');
    await t1.locator('[data-action=reply]').click();
    assert.equal(await rvFocus(page), 'Reply to the comments on x/f.go');
    assert.equal(await t1.locator('[data-action=reply]').count(), 0, 'Reply hides while its box is open');
    await page.keyboard.press('Escape');
    assert.equal(await rvFocus(page), 'Reply');
    await page.keyboard.press('Enter');
    await page.keyboard.type('a reply');
    await page.keyboard.press('Control+Enter');
    assert.deepEqual((await rvLog(page)).at(-1), ['reply', 't1', 'a reply']);
    assert.deepEqual(await t1.locator('.wk-diff-comment').evaluateAll(cs => cs.map(c => [c.querySelector('.wk-diff-comment-body').textContent, c.dataset.state || ''])), [['hello', ''], ['a reply', 'pending']]);
    await page.evaluate(() => window.rv.pending.shift().reject(new Error('nope')));
    await until(page, () => document.querySelector('#rv .wk-diff-thread[data-thread="t1"] [data-state="error"]'));
    assert.equal(await t1.locator('.wk-errorbox-head > span').textContent(), 'Couldn’t post the reply');
    await t1.locator('[data-action=edit]').click();
    assert.equal(await t1.locator('textarea').inputValue(), 'a reply');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments: an injected box is used through its interface, and a factory that throws falls back to the textarea', async () => {
    const { page, problems } = await open();
    await mountReview(page, { injected: true, sync: true });
    const rv = page.locator('#rv');
    await rv.locator('.wk-diff-line[data-new="2"]').focus();
    await page.keyboard.press('c');
    assert.deepEqual((await rvLog(page))[0], ['make', 'Comment on x/f.go, line 2', '', 'Leave a comment']);
    assert.equal(await rv.locator('.wk-diff-compose > .stub-box').count(), 1, 'the box’s element is in the frame');
    assert.deepEqual(await rv.locator('.wk-diff-compose button').allTextContents(), ['Cancel', 'Comment'], 'the view draws the buttons');
    assert.equal(await rvFocus(page), 'Comment on x/f.go, line 2', 'focus() put focus in the box');
    // The box cancels itself (onCancel); destroy runs; focus returns to the line.
    await page.keyboard.press('Escape');
    assert.deepEqual((await rvLog(page)).slice(1), [['box-cancel'], ['destroy', 'Comment on x/f.go, line 2']]);
    assert.equal(await rvFocus(page), 'new:2');
    // Its draft survives a view switch, and the box submits itself (onSubmit); value is read.
    await page.keyboard.press('c');
    await page.keyboard.type('from the editor');
    await page.locator('#rv-view-split').check({ force: true });
    assert.equal(await rv.locator('.stub-box textarea').inputValue(), 'from the editor');
    await rv.locator('.stub-box textarea').focus();
    await page.keyboard.press('Control+Enter');
    const log = await rvLog(page);
    assert.deepEqual(log.slice(-3), [['box-submit'], ['destroy', 'Comment on x/f.go, line 2'], ['comment', { path: 'x/f.go', side: 'new', line: 2, endLine: 2, body: 'from the editor' }]]);
    assert.equal(log.filter(l => l[0] === 'comment').length, 1, 'submitted once, though the frame also handles Ctrl+Enter');
    assert.equal(await rvFocus(page), 'new:2', 'a comment the app takes at once returns focus to the line');
    // Keys in a box go to the box: in its contenteditable="false" part (a disabled
    // editor) n, p, N, ] and c do nothing to the diff.
    await page.keyboard.press('c');
    await rv.locator('.stub-frozen').focus();
    for (const key of ['n', 'p', 'Shift+N', ']', 'c', 'ArrowDown']) await page.keyboard.press(key);
    assert.equal(await page.evaluate(() => document.activeElement.className), 'stub-frozen');
    assert.equal(await rv.locator('.wk-diff-compose').count(), 1);
    // So do the box's own buttons.
    await rv.locator('.wk-diff-compose [data-action=cancel]').focus();
    await page.keyboard.press('n');
    assert.equal(await page.evaluate(() => document.activeElement.dataset.action), 'cancel');
    await page.close();
    // A factory that returns a bare element with value, focus() and destroy() (an editor element).
    const bare = await open();
    await mountReview(bare.page, { injected: true, bare: true, sync: true });
    await bare.page.locator('#rv .wk-diff-line[data-new="3"]').focus();
    await bare.page.keyboard.press('c');
    assert.equal(await bare.page.evaluate(() => document.activeElement.getAttribute('aria-label')), 'Comment on x/f.go, line 3');
    await bare.page.keyboard.type('bare');
    await bare.page.locator('#rv .wk-diff-compose [data-action=submit]').click();
    assert.deepEqual((await bare.page.evaluate(() => window.rv.log)).slice(-2), [['destroy', 'Comment on x/f.go, line 3'], ['comment', { path: 'x/f.go', side: 'new', line: 3, endLine: 3, body: 'bare' }]]);
    assert.deepEqual(bare.problems, []);
    await bare.page.close();
    // A factory that throws: the default textarea.
    const second = await open();
    await mountReview(second.page, { throwing: true });
    await second.page.locator('#rv .wk-diff-line[data-new="2"]').focus();
    await second.page.keyboard.press('c');
    assert.equal(await second.page.locator('#rv .wk-diff-compose textarea.wk-diff-compose-field').count(), 1);
    assert.deepEqual([...problems, ...second.problems], []);
    await second.page.close();
  });

  await check('diff comments: the demo shows open, resolved, pending, failed and outdated threads with the default and an injected box, in light and dark', async () => {
    for (const scheme of ['light', 'dark']) {
      const { page, problems } = await open('/demo/index.html', { colorScheme: scheme });
      const look = await page.evaluate(() => ['demo-review', 'demo-review-split'].map(id => {
        const d = document.getElementById(id);
        return {
          count: d.querySelector('.wk-diff-comment-count').textContent,
          resolved: d.querySelectorAll('.wk-diff-thread[data-resolved]').length,
          pending: d.querySelectorAll('[data-state="pending"]').length,
          error: d.querySelectorAll('[data-state="error"] .wk-errorbox').length,
          outdated: d.querySelectorAll('.wk-diff-outdated .wk-diff-thread').length,
          surface: getComputedStyle(d.querySelector('.wk-diff-thread')).backgroundColor,
        };
      }));
      const probe = await page.evaluate(() => { const p = document.createElement('span'); p.style.background = 'var(--wk-color-surface)'; document.body.append(p); const c = getComputedStyle(p).backgroundColor; p.remove(); return c; });
      for (const l of look) assert.deepEqual(l, { count: '8 comments', resolved: 1, pending: 1, error: 1, outdated: 1, surface: probe });
      await page.locator('#demo-review-split .wk-diff-line[data-new]').first().focus().catch(() => {});
      await page.locator('#demo-review-split .wk-diff-pair').nth(3).focus();
      await page.keyboard.press('c');
      assert.equal(await page.locator('#demo-review-split .wk-diff-compose .demo-comment-box').count(), 1, 'the split demo uses the injected box');
      await page.locator('#demo-review .wk-diff-pair, #demo-review .wk-diff-line').nth(3).focus();
      await page.keyboard.press('c');
      assert.equal(await page.locator('#demo-review .wk-diff-compose textarea.wk-diff-compose-field').count(), 1, 'the unified demo the default box');
      assert.deepEqual(problems, []);
      await page.close();
    }
  });

  await check('diff comments: forced colours draw the selection, the + button and a pending comment with system colours', async () => {
    const { page, problems } = await open('/demo/index.html', { forcedColors: 'active' });
    await page.locator('#demo-review .wk-diff-line[data-new="11"]').hover();
    await page.locator('#demo-review .wk-diff-line[data-new="11"] .wk-diff-num').nth(1).click();
    await page.locator('#demo-review .wk-diff-line[data-new="12"] .wk-diff-num').nth(1).click({ modifiers: ['Shift'] });
    const look = await page.evaluate(() => {
      const sel = getComputedStyle(document.querySelector('#demo-review [data-selected]'));
      const plus = getComputedStyle(document.querySelector('#demo-review .wk-diff-add-comment'));
      const pending = getComputedStyle(document.querySelector('#demo-review [data-state="pending"] .wk-diff-comment-body'));
      const sys = c => { const p = document.createElement('span'); p.style.color = c; document.body.append(p); const v = getComputedStyle(p).color; p.remove(); return v; };
      return { sel: [sel.outlineStyle, sel.outlineColor === sys('Highlight')], plus: [plus.borderTopStyle, plus.color === sys('ButtonText'), plus.backgroundColor === sys('ButtonFace')], pending: pending.color === sys('GrayText') };
    });
    assert.deepEqual(look, { sel: ['solid', true], plus: ['solid', true, true], pending: true });
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments (review fixes): a diff without review keeps its shrink-to-fit width; review mode makes the body a container', async () => {
    const { page, problems } = await open();
    const r = await page.evaluate(async () => {
      const { wkDiff } = await import('../src/index.js');
      const text = 'diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1,3 +1,3 @@\n context line that is fairly long so it has some width\n-old\n+new\n';
      const out = {};
      for (const [name, extra] of [['plain', {}], ['review', { oncomment: () => {} }]]) {
        const wrap = document.createElement('div');
        wrap.className = 'demo-shrink';
        wrap.style.setProperty('display', 'inline-block');
        document.getElementById('demo').prepend(wrap);
        const d = wkDiff({ id: `fit-${name}`, diff: text, ...extra });
        wrap.append(d);
        const body = d.querySelector('.wk-diff-body');
        out[name] = { review: d.hasAttribute('data-review'), container: getComputedStyle(body).containerType, width: Math.round(body.getBoundingClientRect().width), code: Math.round(d.querySelector('.wk-diff-code').scrollWidth) };
        wrap.remove();
      }
      return out;
    });
    assert.deepEqual([r.plain.review, r.plain.container], [false, 'normal']);
    assert.ok(r.plain.width > r.plain.code, `the plain diff sizes to its content in an inline-block (${r.plain.width} px for ${r.plain.code} px of code)`);
    assert.deepEqual([r.review.review, r.review.container], [true, 'inline-size']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments (review fixes): Escape and Ctrl+Enter while an input method composes, and keys a box already handled, leave the box alone; focus stays in a box across setComments', async () => {
    const { page, problems } = await open();
    await mountReview(page, { comments: [{ id: 't1', path: 'x/f.go', side: 'new', line: 2, entries: [{ author: 'ana', time: 0, body: 'x' }] }] });
    const rv = page.locator('#rv');
    await rv.locator('.wk-diff-line[data-new="4"]').focus();
    await page.keyboard.press('c');
    await page.keyboard.type('かな');
    const composing = key => page.evaluate(k => {
      const ta = document.activeElement;
      const e = new KeyboardEvent('keydown', { key: k, ctrlKey: k === 'Enter', bubbles: true, cancelable: true, isComposing: true, keyCode: 229 });
      ta.dispatchEvent(e);
      return document.querySelectorAll('#rv .wk-diff-compose').length;
    }, key);
    assert.equal(await composing('Escape'), 1, 'Escape during composition does not cancel');
    assert.equal(await composing('Enter'), 1, 'Ctrl+Enter during composition does not send');
    assert.equal((await rvLog(page)).filter(l => l[0] === 'comment').length, 0);
    // A redraw from the app keeps focus and the caret in the box.
    await page.evaluate(() => window.rv.diff.setComments([{ id: 't1', path: 'x/f.go', side: 'new', line: 2, entries: [{ author: 'ana', time: 0, body: 'x' }, { author: 'sam', time: 0, body: 'y' }] }]));
    assert.equal(await rvFocus(page), 'Comment on x/f.go, line 4');
    await page.keyboard.type('!');
    assert.equal(await rv.locator('.wk-diff-compose textarea').inputValue(), 'かな!');
    assert.equal(await rv.locator('.wk-diff-thread .wk-diff-comment').count(), 2);
    await page.close();
    // A box that handles Escape itself (preventDefault) keeps the frame from cancelling.
    const second = await open();
    await mountReview(second.page, { injected: true, swallow: true, sync: true });
    await second.page.locator('#rv .wk-diff-line[data-new="4"]').focus();
    await second.page.keyboard.press('c');
    await second.page.keyboard.press('Escape');
    assert.equal(await second.page.locator('#rv .wk-diff-compose').count(), 1, 'the frame leaves an Escape the box handled');
    assert.deepEqual((await second.page.evaluate(() => window.rv.log)).at(-1), ['box-own-escape']);
    assert.deepEqual([...problems, ...second.problems], []);
    await second.page.close();
  });

  await check('diff comments (review fixes): Edit with a box already open adds the text to it, or keeps the failed comment when the box can’t take it', async () => {
    for (const injected of [false, true]) {
      const { page, problems } = await open();
      await mountReview(page, { injected });
      const rv = page.locator('#rv');
      const box = () => rv.locator('.wk-diff-compose textarea');
      await rv.locator('.wk-diff-line[data-new="4"]').focus();
      await page.keyboard.press('c');
      await page.keyboard.type('FIRST');
      await page.keyboard.press('Control+Enter');
      await page.evaluate(() => window.rv.pending.shift().reject(new Error('boom')));
      await until(page, () => document.querySelector('#rv [data-action="edit"]'));
      await rv.locator('.wk-diff-line[data-new="4"]').focus();
      await page.keyboard.press('c');
      await page.keyboard.type('SECOND');
      await rv.locator('[data-action=edit]').click();
      if (!injected) {
        assert.equal(await box().inputValue(), 'SECOND\n\nFIRST', 'the failed text is added to the open box');
        assert.equal(await rv.locator('[data-state="error"]').count(), 0, 'and the failed comment goes');
      } else {
        assert.equal(await box().inputValue(), 'SECOND', 'a box whose value can’t be set keeps its own text');
        assert.equal(await rv.locator('[data-state="error"] .wk-diff-comment-body').textContent(), 'FIRST', 'and the failed comment stays, with its text');
      }
      assert.equal(await page.evaluate(() => document.activeElement.tagName), 'TEXTAREA', 'focus goes to the box');
      assert.deepEqual(problems, []);
      await page.close();
    }
  });

  await check('diff comments (review fixes): in the split view the + button is outside anything aria-hidden; a context line is read once', async () => {
    const { page, problems } = await open();
    await mountReview(page, { view: 'split' });
    const r = await page.evaluate(() => {
      const pair = document.querySelector('#rv .wk-diff-pair');
      pair.focus();
      const plus = document.querySelector('#rv .wk-diff-add-comment');
      const newHalf = pair.querySelector('[data-side="new"]');
      return {
        plusHidden: Boolean(plus.closest('[aria-hidden="true"]')), side: plus.parentElement.dataset.side,
        halfHidden: newHalf.getAttribute('aria-hidden'), partsHidden: [...newHalf.children].filter(c => c !== plus).every(c => c.getAttribute('aria-hidden') === 'true'),
        read: [...document.querySelectorAll('#rv .wk-diff-sr')].filter(x => !x.closest('[aria-hidden]')).map(x => x.textContent).filter(x => /^Line 1/.test(x)),
      };
    });
    assert.deepEqual(r, { plusHidden: false, side: 'new', halfHidden: null, partsHidden: true, read: ['Line 1: '] });
    const tree = await axTree(page);
    assert.ok(tree.some(n => n.role?.value === 'button' && n.name?.value === 'Comment on x/f.go, line 1'), 'the + button is in the accessibility tree');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments (review fixes): opening a box or toggling a thread redraws only its row; 5,000 lines with 200 threads answer c within 100 ms', async () => {
    const { page, problems } = await open();
    await mountReview(page, { comments: [
      { id: 'a', path: 'x/f.go', side: 'new', line: 1, entries: [{ author: 'ana', time: 0, body: 'x' }] },
      { id: 'b', path: 'x/f.go', side: 'new', line: 5, resolved: true, entries: [{ author: 'ana', time: 0, body: 'y' }] },
    ] });
    const same = await page.evaluate(() => {
      const a = document.querySelector('#rv .wk-diff-thread[data-thread="a"]');
      const row = document.querySelector('#rv .wk-diff-line[data-new="3"]');
      row.focus(); row.dispatchEvent(new KeyboardEvent('keydown', { key: 'c', bubbles: true, cancelable: true }));
      const afterOpen = a.isConnected;
      document.querySelector('#rv .wk-diff-thread[data-thread="b"] [data-action="toggle"]').click();
      return [afterOpen, a.isConnected, document.querySelector('#rv .wk-diff-thread[data-thread="b"] [data-action="toggle"]').getAttribute('aria-expanded')];
    });
    assert.deepEqual(same, [true, true, 'true'], 'thread a is the same element after a box opens and b toggles');
    const r = await page.evaluate(async () => {
      const { wkDiff } = await import('../src/index.js');
      const lines = [];
      for (let i = 1; i <= 2500; i++) lines.push(`-\tvalue_${i} := compute(${i}, "old")`, `+\tvalue_${i} := compute(${i}, "new", true)`);
      const text = `diff --git a/big.go b/big.go\n--- a/big.go\n+++ b/big.go\n@@ -1,2500 +1,2500 @@\n${lines.join('\n')}\n`;
      const threads = [];
      for (let k = 0; k < 200; k++) threads.push({ id: `t${k}`, path: 'big.go', side: k % 2 ? 'old' : 'new', line: 1 + k * 12, entries: [{ author: 'a', time: 0, body: `comment ${k}` }, { author: 'b', time: 0, body: 'reply' }] });
      const d = wkDiff({ id: 'big', diff: text, collapseAbove: 10000, collapseCharsAbove: 1e9, comments: threads, oncomment: () => {}, onreply: () => {}, onresolve: () => {} });
      document.getElementById('demo').prepend(d);
      const row = d.querySelector('.wk-diff-line[data-new="2400"]');
      row.focus();
      const times = [];
      for (const n of [2400, 1200, 30]) {
        const target = d.querySelector(`.wk-diff-line[data-new="${n}"]`);
        target.focus();
        const t = performance.now();
        target.dispatchEvent(new KeyboardEvent('keydown', { key: 'c', bubbles: true, cancelable: true }));
        void d.offsetHeight;
        times.push(performance.now() - t);
      }
      const t = performance.now();
      for (let i = 0; i < 20; i++) d.querySelector('.wk-diff-line[data-new="100"]').dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true }));
      const arrow = (performance.now() - t) / 20;
      return { threads: d.querySelectorAll('.wk-diff-thread').length, boxes: d.querySelectorAll('.wk-diff-compose').length, times, arrow };
    });
    assert.equal(r.threads, 200);
    assert.equal(r.boxes, 3);
    for (const t of r.times) assert.ok(t < 100, `c answered in ${t.toFixed(1)} ms`);
    assert.ok(r.arrow < 20, `an arrow key took ${r.arrow.toFixed(1)} ms`);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments (review fixes): a comment that lands before setComments stays, as posted, until setComments; the working dot keeps its minimum', async () => {
    const { page, problems } = await open();
    await mountReview(page);
    const rv = page.locator('#rv');
    await rv.locator('.wk-diff-line[data-new="1"]').focus();
    await page.keyboard.press('c');
    await page.keyboard.type('LATE');
    await page.keyboard.press('Control+Enter');
    await page.evaluate(() => window.rv.pending.shift().resolve());
    await until(page, () => document.querySelector('[data-wk-announcer] [aria-live=polite]').textContent.includes('Comment posted'));
    const local = rv.locator('.wk-diff-thread[data-local]');
    assert.equal(await local.count(), 1, 'the copy stays until the app redraws');
    assert.equal(await local.locator('.wk-diff-comment').getAttribute('data-state'), null, 'no longer pending');
    assert.equal(await local.locator('.wk-diff-comment-body').textContent(), 'LATE');
    await page.evaluate(() => window.rv.diff.setComments([{ id: 'real', path: 'x/f.go', side: 'new', line: 1, entries: [{ author: 'Me', time: 0, body: 'LATE' }] }]));
    assert.equal(await local.count(), 0, 'and goes when the app’s comment arrives');
    assert.equal(await rv.locator('.wk-diff-thread').count(), 1);
    // The dot, once shown, stays timings.indicatorMin before the outcome replaces it.
    await rv.locator('.wk-diff-line[data-new="2"]').focus();
    await page.keyboard.press('c');
    await page.keyboard.type('slow');
    await page.keyboard.press('Control+Enter');
    await until(page, () => document.querySelector('#rv .wk-diff-comment-state:not([hidden])'));
    const held = await page.evaluate(async () => {
      const { timings } = await import('../src/index.js');
      const t = performance.now();
      window.rv.pending.shift().reject(new Error('no'));
      await new Promise(r => { const wait = () => document.querySelector('#rv [data-state="error"]') ? r() : requestAnimationFrame(wait); wait(); });
      return { took: performance.now() - t, min: timings.indicatorMin };
    });
    assert.ok(held.took >= held.min - 120, `the error replaced the dot after ${Math.round(held.took)} ms`);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments (review fixes): Resolve is busy while onresolve’s promise runs, with a spinner after the delay', async () => {
    const { page, problems } = await open();
    await mountReview(page, { asyncResolve: true, comments: [{ id: 't1', path: 'x/f.go', side: 'new', line: 2, entries: [{ author: 'ana', time: 0, body: 'x' }] }] });
    const rv = page.locator('#rv');
    const button = () => rv.locator('[data-action=resolve]');
    await button().click();
    assert.deepEqual([await button().getAttribute('data-busy'), await button().getAttribute('aria-disabled'), await button().locator('.wk-spinner').count()], ['', 'true', 0], 'busy at once, no spinner yet');
    assert.equal(await rvFocus(page), 'Resolve', 'it keeps focus and its name');
    await button().click({ force: true });
    assert.equal((await rvLog(page)).filter(l => l[0] === 'resolve').length, 1, 'a second press does nothing');
    await until(page, () => document.querySelector('#rv [data-action="resolve"] .wk-spinner'));
    assert.equal(await button().locator('.wk-spinner').getAttribute('aria-hidden'), 'true');
    // The app redraws as resolved, then resolves the promise.
    await page.evaluate(() => { window.rv.diff.setComments([{ id: 't1', path: 'x/f.go', side: 'new', line: 2, resolved: true, entries: [{ author: 'ana', time: 0, body: 'x' }] }]); window.rv.pending.shift().resolve(); });
    await until(page, () => !document.querySelector('#rv [data-busy]'));
    assert.equal(await rv.locator('.wk-diff-thread[data-resolved]').count(), 1);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments (review fixes): focus moves on when setComments removes a focused thread: the next thread, else the file’s toggle', async () => {
    const { page, problems } = await open();
    const t = (id, line) => ({ id, path: 'x/f.go', side: 'new', line, entries: [{ author: 'ana', time: 0, body: id }] });
    await mountReview(page, { comments: [t('gone1', 90), t('gone2', 91), t('here', 2)] });
    const at = () => page.evaluate(() => document.activeElement.dataset.thread || document.activeElement.id || document.activeElement.className);
    await page.locator('#rv .wk-diff-thread[data-thread="gone1"] [data-action="reply"]').focus();
    await page.evaluate(() => window.rv.diff.setComments([{ id: 'gone2', path: 'x/f.go', side: 'new', line: 91, entries: [{ author: 'ana', time: 0, body: 'x' }] }, { id: 'here', path: 'x/f.go', side: 'new', line: 2, entries: [{ author: 'ana', time: 0, body: 'x' }] }]));
    assert.equal(await at(), 'gone2', 'an outdated thread that goes hands focus to the next thread');
    await page.evaluate(() => window.rv.diff.setComments([]));
    assert.equal(await at(), 'rv-file-0-toggle', 'with no thread left, to the file’s toggle');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments (review fixes): unique thread ids, exact paths first, announced selections, destroy(), safe avatars, times redrawn in the user’s locale', async () => {
    const { page, problems } = await open('/demo/index.html', { locale: 'de-DE' });
    await page.route('https://avatars.example/**', route => route.fulfill({ status: 200, contentType: 'image/png', body: Buffer.from('') }));
    const e = (body, extra = {}) => ({ author: 'ana', time: '2026-11-23T09:00:00Z', body, ...extra });
    const text = 'diff --git a/old.txt b/new.txt\nsimilarity index 90%\nrename from old.txt\nrename to new.txt\n--- a/old.txt\n+++ b/new.txt\n@@ -1,2 +1,2 @@\n-a\n+b\n c\ndiff --git a/old.txt b/old.txt\nnew file mode 100644\n--- /dev/null\n+++ b/old.txt\n@@ -0,0 +1 @@\n+fresh\n';
    await mountReview(page, { injected: true, text, avatar: 'javascript:alert(1)', comments: [
      { id: 'a.b', path: 'new.txt', side: 'new', line: 1, resolved: true, entries: [e('one', { avatar: 'https://avatars.example/a.png' })] },
      { id: 'a_b', path: 'new.txt', side: 'new', line: 2, resolved: true, entries: [e('two', { avatar: 'javascript:alert(1)' })] },
      { id: 'exact', path: 'old.txt', side: 'new', line: 1, entries: [e('on the new file', { avatar: 'data:image/png;base64,iVBORw0KGgo=' })] },
    ] });
    const rv = page.locator('#rv');
    const ids = await rv.locator('.wk-diff-thread-body[id]').evaluateAll(xs => xs.map(x => x.id));
    assert.equal(new Set(ids).size, 2, `thread ids are unique: ${ids}`);
    assert.deepEqual(await rv.locator('.wk-diff-comment-count').allTextContents(), ['2 comments', '1 comment'], 'a thread on old.txt belongs to the file now called old.txt');
    assert.equal(await rv.locator('#rv-file-1 .wk-diff-thread').getAttribute('data-thread'), 'exact');
    const srcs = await rv.locator('img.wk-diff-avatar').evaluateAll(xs => xs.map(x => x.getAttribute('src').slice(0, 11)));
    assert.deepEqual(srcs.sort(), ['data:image/', 'https://ava'], 'a javascript: avatar is not drawn');
    // Locale: the user's, for relative time and the title.
    const first = rv.locator('.wk-diff-thread[data-thread="exact"] time');
    assert.equal(await first.textContent(), 'vor 3 Stunden');
    await page.evaluate(() => { window.rvNow = Date.parse('2026-11-23T15:00:00Z'); });
    await rv.locator('.wk-diff-thread[data-thread="a.b"] [data-action=toggle]').click();
    assert.equal(await rv.locator('.wk-diff-thread[data-thread="exact"] time').textContent(), 'vor 3 Stunden', 'a toggle elsewhere redraws only its own row');
    // Selections are announced, and the same words twice still change the region.
    // Through the window's announcer: count each time the words are set.
    await page.evaluate(() => {
      const r = document.querySelector('[data-wk-announcer] [aria-live=polite]');
      window.selSaid = 0;
      new MutationObserver(() => { if (r.textContent === 'Lines 1 to 2 selected') window.selSaid++; }).observe(r, { childList: true, characterData: true, subtree: true });
    });
    await rv.locator('#rv-file-0 .wk-diff-line[data-new="1"]').focus();
    await page.keyboard.press('Shift+ArrowDown');
    await until(page, () => window.selSaid === 1);
    assert.equal(await page.evaluate(() => document.querySelector('[data-wk-announcer] [aria-live=polite]').getAttribute('aria-atomic')), 'true');
    await page.keyboard.press('Escape'); await page.keyboard.press('ArrowUp'); await page.keyboard.press('Shift+ArrowDown');
    await until(page, () => window.selSaid === 2);
    assert.equal(await page.evaluate(() => document.querySelector('[data-wk-announcer] [aria-live=polite]').textContent), 'Lines 1 to 2 selected', 'repeated words re-announce');
    await page.keyboard.press('Escape');
    // destroy(): open injected boxes are destroyed; setComments then does nothing.
    await rv.locator('#rv-file-1 .wk-diff-line[data-new="1"]').focus();
    await page.keyboard.press('c');
    await page.evaluate(() => window.rv.diff.destroy());
    assert.deepEqual((await rvLog(page)).at(-1), ['destroy', 'Comment on old.txt, line 1']);
    assert.equal(await rv.locator('.wk-diff-compose').count(), 0);
    await page.evaluate(() => window.rv.diff.setComments([]));
    assert.equal(await rv.locator('.wk-diff-thread').count(), 3);
    assert.deepEqual(problems.filter(p => !p.includes('avatars.example')), []);
    await page.close();
  });

  await check('diff comments (review fixes): setComments redraws relative times and keeps avatar images; a reply box for a thread that went is destroyed', async () => {
    const { page, problems } = await open();
    await mountReview(page, { injected: true, sync: true, comments: [] });
    const r = await page.evaluate(() => {
      const entry = { author: 'ana', time: '2026-11-23T11:00:00Z', body: 'x', avatar: 'data:image/png;base64,iVBORw0KGgo=' };
      const threads = [{ id: 't', path: 'x/f.go', side: 'new', line: 2, entries: [entry] }, { id: 'u', path: 'x/f.go', side: 'new', line: 5, entries: [{ author: 'sam', time: 0, body: 'y' }] }];
      window.rv.diff.setComments(threads);
      const img = document.querySelector('#rv img.wk-diff-avatar');
      const before = document.querySelector('#rv time').textContent;
      window.rvNow = Date.parse('2026-11-23T14:00:00Z');
      document.querySelector('#rv .wk-diff-thread[data-thread="u"] [data-action="reply"]').click();
      window.rv.diff.setComments(threads.slice());
      const out = { before, after: document.querySelector('#rv time').textContent, sameImg: document.querySelector('#rv img.wk-diff-avatar') === img };
      window.rv.diff.setComments([threads[0]]);
      out.log = window.rv.log.slice(-1);
      out.boxes = document.querySelectorAll('#rv .wk-diff-compose').length;
      return out;
    });
    assert.deepEqual(r, { before: '1 hour ago', after: '3 hours ago', sameImg: true, log: [['destroy', 'Reply to the comments on x/f.go']], boxes: 0 });
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments (final round): a posted copy the app never shows goes after 10 s on the fake clock', async () => {
    const { page, problems } = await open('/demo/index.html', { clock: true });
    await mountReview(page);
    const rv = page.locator('#rv');
    await rv.locator('.wk-diff-line[data-new="1"]').focus();
    await page.keyboard.press('c');
    await page.keyboard.type('never shown');
    await page.keyboard.press('Control+Enter');
    // The clock stands still from here, before the working dot shows; the test moves it.
    await page.clock.pauseAt(await page.evaluate(() => Date.now() + 10));
    await page.evaluate(async () => { window.rv.pending.shift().resolve(); for (let i = 0; i < 5; i++) await null; });
    const local = rv.locator('.wk-diff-thread[data-local]');
    assert.equal(await local.locator('.wk-diff-comment').getAttribute('data-state'), null, 'posted, no longer pending');
    await page.clock.runFor(9_999);
    assert.equal(await local.count(), 1, 'still there at 9,999 ms');
    await page.clock.runFor(1);
    assert.equal(await local.count(), 0, 'gone at 10 s');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments (final round): a posted copy goes only when the app’s comments hold it, by the resolved id or by text and author', async () => {
    const { page, problems } = await open();
    await mountReview(page);
    const rv = page.locator('#rv');
    const local = rv.locator('.wk-diff-thread[data-local]');
    const post = async (line, text) => { await rv.locator(`.wk-diff-line[data-new="${line}"]`).focus(); await page.keyboard.press('c'); await page.keyboard.type(text); await page.keyboard.press('Control+Enter'); };
    // Unrelated threads arrive while it is pending: the copy stays after it resolves.
    await post(1, 'mine');
    await page.evaluate(() => window.rv.diff.setComments([{ id: 'other', path: 'x/f.go', side: 'new', line: 5, entries: [{ author: 'sam', time: 0, body: 'not mine' }] }]));
    await page.evaluate(async () => { window.rv.pending.shift().resolve(); for (let i = 0; i < 5; i++) await null; });
    assert.equal(await local.count(), 1, 'kept: setComments did not hold it');
    await page.evaluate(() => window.rv.diff.setComments([{ id: 'other', path: 'x/f.go', side: 'new', line: 5, entries: [{ author: 'sam', time: 0, body: 'not mine' }] }]));
    assert.equal(await local.count(), 1, 'still kept after another unrelated setComments');
    await page.evaluate(() => window.rv.diff.setComments([{ id: 'mine', path: 'x/f.go', side: 'new', line: 1, entries: [{ author: 'Me', time: 0, body: 'mine' }] }]));
    assert.equal(await local.count(), 0, 'gone once a thread holds the same text and author');
    // Resolved with an id: the thread with that id covers it, whatever its text.
    await post(2, 'by id');
    await page.evaluate(async () => { window.rv.pending.shift().resolve({ id: 'server-7' }); for (let i = 0; i < 5; i++) await null; });
    assert.equal(await local.count(), 1);
    await page.evaluate(() => window.rv.diff.setComments([{ id: 'server-7', path: 'x/f.go', side: 'new', line: 2, entries: [{ author: 'Ana (edited)', time: 0, body: 'by id, reformatted' }] }]));
    assert.equal(await local.count(), 0, 'the thread with the resolved id holds it');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('diff comments (final round): a reply box with text outlives its thread, listed as outdated, until sent or cancelled; a resolve after destroy() draws nothing', async () => {
    const { page, problems } = await open();
    const t = { id: 'gone', path: 'x/f.go', side: 'new', line: 2, entries: [{ author: 'ana', time: 0, body: 'x' }] };
    await mountReview(page, { injected: true, sync: true, asyncResolve: true, comments: [t] });
    const rv = page.locator('#rv');
    await rv.locator('[data-action=reply]').click();
    await page.keyboard.type('half a reply');
    await page.evaluate(() => window.rv.diff.setComments([]));
    const kept = rv.locator('.wk-diff-outdated .wk-diff-thread[data-thread="gone"]');
    assert.equal(await kept.count(), 1, 'the thread is kept, outdated');
    assert.equal(await kept.locator('textarea').inputValue(), 'half a reply');
    assert.equal(await kept.locator('[data-action=resolve]').count(), 0, 'a thread that has gone offers no Resolve');
    assert.equal(await rvFocus(page), 'Reply to the comments on x/f.go', 'focus stays in the box');
    assert.equal((await rvLog(page)).filter(l => l[0] === 'destroy').length, 0);
    // Another redraw keeps it; Cancel lets it go.
    await page.evaluate(() => window.rv.diff.setComments([]));
    assert.equal(await kept.count(), 1);
    await kept.locator('[data-action=cancel]').click();
    assert.equal(await rv.locator('.wk-diff-thread').count(), 0);
    assert.equal(await rvFocus(page), 'x/f.go', 'focus moves to the file’s toggle');
    // An empty reply box goes with its thread.
    await page.evaluate(t => window.rv.diff.setComments([t]), t);
    await rv.locator('[data-action=reply]').click();
    await page.evaluate(() => window.rv.diff.setComments([]));
    assert.equal(await rv.locator('.wk-diff-thread').count(), 0);
    // Sending from a kept box: the reply goes to the app and the kept thread goes.
    await page.evaluate(t => window.rv.diff.setComments([t]), t);
    await rv.locator('[data-action=reply]').click();
    await page.keyboard.type('sent anyway');
    await page.evaluate(() => window.rv.diff.setComments([]));
    await kept.locator('[data-action=submit]').click();
    assert.deepEqual((await rvLog(page)).filter(l => l[0] === 'reply').at(-1), ['reply', 'gone', 'sent anyway']);
    assert.equal(await rv.locator('.wk-diff-thread').count(), 0);
    // A resolve in flight when the diff is destroyed draws nothing when it settles.
    await page.evaluate(t => window.rv.diff.setComments([t]), t);
    await rv.locator('[data-action=resolve]').click();
    const after = await page.evaluate(async () => {
      const button = document.querySelector('#rv [data-action="resolve"]');
      window.rv.diff.destroy();
      window.rv.pending.shift().resolve();
      for (let i = 0; i < 5; i++) await null;
      return [button.isConnected, button.hasAttribute('data-busy')];
    });
    assert.deepEqual(after, [true, true], 'the destroyed diff is left as it was');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('footer bar: a hairline above, the first link at the start, the last at the end, a hidden one leaves the rest at the start', async () => {
    const { page, problems } = await open();
    const [two, one] = await page.locator('#footer-bar .wk-footer-bar').evaluateAll(bars => bars.map(b => {
      const box = b.getBoundingClientRect();
      const links = [...b.querySelectorAll('.wk-link')].filter(l => l.offsetParent).map(l => l.getBoundingClientRect());
      return { top: getComputedStyle(b).borderTopStyle, atStart: links[0].left - box.left < 24, atEnd: box.right - links.at(-1).right < 24, count: links.length };
    }));
    assert.deepEqual(two, { top: 'solid', atStart: true, atEnd: true, count: 2 });
    assert.deepEqual(one, { top: 'solid', atStart: true, atEnd: false, count: 1 });
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('Inter: a wk-root page with no network loads the bundled Inter, from src/ and from dist/, with cv11 and ss03 on; latin-ext loads only when used', async () => {
    for (const url of ['/demo/index.html', '/demo/dist.html']) {
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

  await check('derived colours: a theme that changes only the accent and attention changes selection, chart-1, destructive text and attention fill with them, in light and dark', async () => {
    for (const scheme of ['light', 'dark']) {
      const { page, problems } = await open('/demo/index.html', { colorScheme: scheme });
      // A theme in a stylesheet object (the page's CSP blocks inline <style>).
      const read = () => page.evaluate(() => {
        const probe = (prop, token) => { const e = document.createElement('i'); e.style[prop] = `var(${token})`; document.body.append(e); const v = getComputedStyle(e)[prop]; e.remove(); return v; };
        return { selection: probe('backgroundColor', '--wk-color-selection'), chart: probe('color', '--wk-color-chart-1'), destructive: probe('color', '--wk-color-destructive-text'), fill: probe('backgroundColor', '--wk-color-attention-fill') };
      });
      const before = await read();
      await page.evaluate(() => {
        const sheet = new CSSStyleSheet();
        const theme = '--wk-color-accent: #ff0000; --wk-color-attention: #0000ff; --wk-color-attention-text: #00aa00;';
        sheet.replaceSync(`:root { ${theme} } @media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) { ${theme} } } :root[data-theme="dark"] { ${theme} }`);
        document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
      });
      const after = await read();
      const dark = scheme === 'dark';
      assert.deepEqual(after, {
        selection: `color(srgb 1 0 0 / ${dark ? 0.16 : 0.12})`,
        chart: 'rgb(255, 0, 0)',
        destructive: 'rgb(0, 170, 0)',
        fill: `color(srgb 0 0 1 / ${dark ? 0.12 : 0.08})`,
      }, scheme);
      assert.notDeepEqual(after, before);
      assert.deepEqual(problems, []);
      await page.close();
    }
  });

  await check('Inter: the bundled files carry cv11 and ss03, so --wk-font-features changes what renders', async () => {
    for (const file of ['/fonts/Inter-latin.woff2', '/dist/fonts/Inter-latin.woff2']) {
      const { page, problems } = await open();
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

  await check('field border: a text field, select, checkbox, radio, off switch and stepper outline reach 3:1 on the window, in light and dark; a button keeps the soft border', async () => {
    for (const scheme of ['light', 'dark']) {
      const { page, problems } = await open('/demo/index.html', { colorScheme: scheme });
      const ratios = await page.evaluate(() => {
        const rgb = c => c.match(/[\d.]+/g).slice(0, 3).map(Number);
        const lum = c => { const [r, g, b] = rgb(c).map(x => { x /= 255; return x <= .03928 ? x / 12.92 : ((x + .055) / 1.055) ** 2.4; }); return .2126 * r + .7152 * g + .0722 * b; };
        const ratio = (a, b) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + .05) / (y + .05); };
        const bg = getComputedStyle(document.documentElement).backgroundColor;
        const of = sel => { const el = document.querySelector(sel); return el ? +ratio(getComputedStyle(el).borderTopColor, bg).toFixed(2) : null; };
        return {
          field: of('input.wk-field:not([readonly]):not(:disabled)'), select: of('.wk-select:not(:disabled)'), checkbox: of('.wk-checkbox:not(:checked):not(:disabled)'),
          radio: of('.wk-radio:not(:checked):not(:disabled)'), switch: of('.wk-switch:not(:checked):not(:disabled)'), stepper: of('.wk-stepper-arrows'), button: of('.wk-button:not(.wk-button--primary):not(.wk-button--quiet)'),
        };
      });
      for (const [what, r] of Object.entries(ratios)) {
        assert.ok(r !== null, `${what} is on the page`);
        if (what === 'button') assert.ok(r < 3, `${scheme}: the button keeps the soft border (${r}:1)`);
        else assert.ok(r >= 3, `${scheme}: the ${what} outline is ${r}:1 on the window`);
      }
      assert.deepEqual(problems, []);
      await page.close();
    }
  });

  await check('status dot: the running dot (filled, working, default size) has a halo in --wk-dot-ground, which follows wk-color-bg in light and dark; an outline in forced colours', async () => {
    for (const scheme of ['light', 'dark']) {
      const { page, problems } = await open('/demo/index.html', { colorScheme: scheme });
      const look = await page.evaluate(() => {
        const dot = attrs => { const d = document.createElement('span'); d.className = 'wk-dot'; for (const [k, v] of Object.entries(attrs)) d.setAttribute(k, v); document.querySelector('#dots').append(d); const s = getComputedStyle(d); const out = { shadow: s.boxShadow, ground: s.getPropertyValue('--wk-dot-ground').trim() }; d.remove(); return out; };
        return { bg: getComputedStyle(document.documentElement).backgroundColor, running: dot({ 'data-tone': 'working' }), filled: dot({ 'data-tone': 'working', 'data-shape': 'filled' }), small: dot({ 'data-tone': 'working', 'data-size': 'sm' }), half: dot({ 'data-tone': 'working', 'data-shape': 'half' }), neutral: dot({}) };
      });
      assert.ok(look.running.ground, '--wk-dot-ground is set on the dot');
      // Two rings: the gap in the page background, then the dot's colour.
      const rings = look.running.shadow.split(/,(?![^(]*\))/).map(x => x.trim());
      assert.equal(rings.length, 2, look.running.shadow);
      assert.ok(rings[0].startsWith(look.bg), `${scheme}: the halo's gap is the background (${rings[0]} vs ${look.bg})`);
      assert.equal(look.filled.shadow, look.running.shadow);
      for (const plain of ['small', 'half', 'neutral']) assert.equal(look[plain].shadow, 'none', `${plain} has no halo`);
      assert.deepEqual(problems, []);
      await page.close();
    }
    const { page, problems } = await open('/demo/index.html', { forcedColors: 'active' });
    const forced = await page.locator('#demo-zebra .wk-dot[data-tone="working"]').evaluate(d => { const s = getComputedStyle(d); return { shadow: s.boxShadow, outline: s.outlineStyle, wide: parseFloat(s.outlineWidth) >= 1 }; });
    assert.deepEqual(forced, { shadow: 'none', outline: 'solid', wide: true });
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('table: zebra rows, a muted row (secondary text, name struck), the name button and row actions; a visually hidden header keeps its name', async () => {
    const { page, problems } = await open();
    const look = await page.locator('#demo-zebra').evaluate(t => {
      const probe = (prop, token) => { const e = document.createElement('i'); e.style[prop] = `var(${token})`; t.append(e); const v = getComputedStyle(e)[prop]; e.remove(); return v; };
      const rows = [...t.tBodies[0].rows];
      const name = rows[0].querySelector('.wk-table-name');
      const hidden = t.querySelector('.wk-visually-hidden').getBoundingClientRect();
      const actions = rows[0].querySelector('.wk-table-actions');
      return {
        fill: probe('backgroundColor', '--wk-color-fill'), selection: probe('backgroundColor', '--wk-color-selection'), secondary: probe('color', '--wk-color-text-secondary'), text: probe('color', '--wk-color-text'),
        bgs: rows.map(r => getComputedStyle(r.cells[0]).backgroundColor),
        muted: { color: getComputedStyle(rows[2].cells[2]).color, line: getComputedStyle(rows[2].querySelector('.wk-table-name')).textDecorationLine },
        name: { color: getComputedStyle(name).color, weight: getComputedStyle(name).fontWeight, border: getComputedStyle(name).borderTopStyle, bg: getComputedStyle(name).backgroundColor },
        hidden: [hidden.width, hidden.height], actionsOneLine: actions.getBoundingClientRect().height < 30,
      };
    });
    assert.notEqual(look.selection, look.fill);
    assert.deepEqual(look.bgs, ['rgba(0, 0, 0, 0)', look.fill, 'rgba(0, 0, 0, 0)', look.selection], 'every other row is tinted, and the selected even row keeps the selection tint');
    assert.deepEqual(look.muted, { color: look.secondary, line: 'line-through' });
    assert.deepEqual(look.name, { color: look.text, weight: '600', border: 'none', bg: 'rgba(0, 0, 0, 0)' });
    assert.deepEqual(look.hidden, [1, 1]);
    assert.ok(look.actionsOneLine, 'row actions stay on one line');
    const ax = await axTree(page);
    assert.ok(ax.some(n => n.role?.value === 'columnheader' && n.name?.value === 'Actions'), 'the hidden header is still named');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('list: the selected row carries the selection tint and darker secondary text, rows keep their own height, forced colours mark the selected row', async () => {
    const { page, problems } = await open();
    const read = p => p.locator('#list .wk-list').first().evaluate(list => {
      const probe = (prop, token) => { const e = document.createElement('i'); e.style[prop] = `var(${token})`; list.append(e); const v = getComputedStyle(e)[prop]; e.remove(); return v; };
      const rows = [...list.querySelectorAll('.wk-list-row')];
      const row = r => ({ bg: getComputedStyle(r).backgroundColor, meta: getComputedStyle(r.querySelector('.wk-secondary')).color, height: r.getBoundingClientRect().height });
      return { rows: rows.map(row), selection: probe('backgroundColor', '--wk-color-selection'), onSelected: probe('color', '--wk-color-text-secondary-on-selected'), secondary: probe('color', '--wk-color-text-secondary'), bullets: getComputedStyle(list).listStyleType };
    });
    const light = await read(page);
    assert.equal(light.bullets, 'none');
    assert.equal(light.rows[0].bg, light.selection, 'the selected row has the selection tint');
    assert.equal(light.rows[0].meta, light.onSelected, 'secondary text on the selected row is the darker step');
    assert.notEqual(light.rows[1].bg, light.selection);
    assert.equal(light.rows[1].meta, light.secondary);
    assert.ok(light.rows[0].height > light.rows[1].height, 'a row with chips is taller than one without');
    assert.equal(await page.locator('#list .wk-list-heading > span').first().evaluate(s => getComputedStyle(s).fontVariantNumeric), 'tabular-nums');
    assert.deepEqual(problems, []);
    await page.close();
    const forced = await open('/demo/index.html', { forcedColors: 'active', reducedMotion: 'reduce' });
    const shots = await forced.page.locator('#list .wk-list').first().evaluate(list => [...list.querySelectorAll('.wk-list-row')].map(r => getComputedStyle(r).backgroundColor));
    assert.notEqual(shots[0], shots[1], 'the selected row looks like the others in forced colours');
    assert.deepEqual(forced.problems, []);
    await forced.page.close();
  });

  await check('list: Up and Down move focus between rows (one tab stop), Home and End jump, Space selects', async () => {
    const { page, problems } = await open();
    const list = page.locator('#list .wk-list').first();
    const state = () => list.evaluate(l => [...l.querySelectorAll('.wk-list-row')].map(r => ({ tab: r.tabIndex, sel: r.getAttribute('aria-selected'), focus: r === document.activeElement, bg: getComputedStyle(r).backgroundColor })));
    await list.locator('.wk-list-row').first().focus();
    await page.keyboard.press('ArrowDown');
    let rows = await state();
    assert.deepEqual(rows.map(r => r.focus), [false, true, false]);
    assert.deepEqual(rows.map(r => r.tab), [-1, 0, -1], 'only the focused row is in the tab order');
    assert.deepEqual(rows.map(r => r.sel), ['true', 'false', 'false'], 'moving does not select');
    await page.keyboard.press(' ');
    rows = await state();
    assert.deepEqual(rows.map(r => r.sel), ['false', 'true', 'false'], 'Space selects the focused row');
    const selection = await list.evaluate(l => { const e = document.createElement('i'); e.style.backgroundColor = 'var(--wk-color-selection)'; l.append(e); const v = getComputedStyle(e).backgroundColor; e.remove(); return v; });
    assert.equal(rows[1].bg, selection, 'the newly selected row takes the selection tint');
    assert.notEqual(rows[0].bg, selection);
    await page.keyboard.press('End');
    assert.deepEqual((await state()).map(r => r.focus), [false, false, true]);
    await page.keyboard.press('ArrowDown');
    assert.deepEqual((await state()).map(r => r.focus), [false, false, true], 'Down stops at the last row');
    await page.keyboard.press('Home');
    await page.keyboard.press('ArrowUp');
    assert.deepEqual((await state()).map(r => r.focus), [true, false, false]);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('pane scroll: the list scrolls under a fixed toolbar, takes focus, and its focus ring sits inside; toolbar subtitle and sidebar detail', async () => {
    const { page, problems } = await open();
    const pane = page.locator('#demo-pane-scroll');
    const before = await page.locator('#layout .wk-toolbar').boundingBox();
    const scrolls = await pane.evaluate(p => { const can = p.scrollHeight > p.clientHeight; p.scrollTop = 200; return { can, top: p.scrollTop }; });
    assert.ok(scrolls.can && scrolls.top > 0, 'the pane body scrolls');
    assert.deepEqual(await page.locator('#layout .wk-toolbar').boundingBox(), before, 'the toolbar stays put');
    await page.locator('#layout .wk-toolbar .wk-button', { hasText: 'New' }).focus();
    await page.keyboard.press('Tab');
    assert.equal(await active(page), 'demo-pane-scroll', 'Tab reaches the pane body');
    assert.deepEqual(await pane.evaluate(p => [p.matches(':focus-visible'), getComputedStyle(p).outlineOffset]), [true, '-2px']);
    const bits = await page.evaluate(() => {
      const sub = getComputedStyle(document.querySelector('#layout .wk-toolbar-subtitle'));
      const detail = document.querySelector('#layout .wk-sidebar-detail');
      const item = detail.closest('.wk-sidebar-item').getBoundingClientRect(), d = detail.getBoundingClientRect();
      const muted = getComputedStyle(document.querySelector('#chips .wk-chip--muted'));
      const dotChip = document.querySelector('#chips .wk-chip > .wk-dot');
      return { sub: [sub.fontSize, sub.fontWeight], detailAtEnd: item.right - d.right < 12, detailSize: getComputedStyle(detail).fontSize, muted: [muted.borderTopStyle, muted.fontWeight], dotMargin: getComputedStyle(dotChip).marginLeft };
    });
    assert.deepEqual(bits, { sub: ['12px', '400'], detailAtEnd: true, detailSize: '11px', muted: ['dashed', '500'], dotMargin: '-3px' });
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('inline menu: in place and always shown, one tab stop, arrow keys move through every item (a link too), Enter follows the link', async () => {
    const { page, problems } = await open();
    const menu = page.locator('#demo-inline [role=menu]');
    const look = await menu.evaluate(m => {
      const s = getComputedStyle(m), a = getComputedStyle(m.querySelector('a'));
      return { hidden: m.hidden, position: s.position, shadow: s.boxShadow, border: s.borderTopWidth, bg: s.backgroundColor, underline: a.textDecorationLine, tabs: [...m.querySelectorAll('[role^=menuitem]')].map(i => i.tabIndex) };
    });
    assert.deepEqual(look, { hidden: false, position: 'static', shadow: 'none', border: '0px', bg: 'rgba(0, 0, 0, 0)', underline: 'none', tabs: [0, -1, -1, -1] });
    // Tab reaches the menu once, on its first item.
    await page.locator('#demo-context-target').focus();
    const at = async () => (await active(page)).replace(/(F1|Ctrl\+.)$/, '');
    await page.keyboard.press('Tab'); // the tooltip's button
    await page.keyboard.press('Tab');
    assert.equal(await at(), 'Help');
    await page.keyboard.press('Tab');
    assert.equal(await active(page), 'demo-dialog-open', 'the menu is one tab stop');
    await page.keyboard.press('Shift+Tab');
    assert.equal(await at(), 'Help');
    const order = [await at()];
    for (let i = 0; i < 4; i++) { await page.keyboard.press('ArrowDown'); order.push(await at()); }
    assert.deepEqual(order, ['Help', 'Settings…', 'Check for updates (disabled)', 'Quit', 'Help'], 'Down moves through every item and wraps');
    await page.keyboard.press('ArrowUp');
    assert.equal(await at(), 'Quit');
    await page.keyboard.press('Home');
    assert.equal(await at(), 'Help');
    await page.keyboard.press('End');
    assert.equal(await at(), 'Quit');
    assert.deepEqual(await menu.evaluate(m => [...m.querySelectorAll('[role^=menuitem]')].map(i => i.tabIndex)), [-1, -1, -1, 0], 'the focused item is the tab stop');
    // Escape is left to the page: the menu stays, and the page sees the key.
    await page.evaluate(() => { window.escapes = []; document.addEventListener('keydown', e => { if (e.key === 'Escape') window.escapes.push(e.defaultPrevented); }); });
    await page.keyboard.press('Escape');
    assert.deepEqual(await page.evaluate(() => window.escapes), [false], 'Escape reaches the document, not prevented');
    assert.equal(await at(), 'Quit');
    assert.equal(await menu.evaluate(m => m.hidden), false);
    // Activating an item keeps the menu in place; a disabled one does nothing.
    await page.keyboard.press('ArrowUp');
    await page.keyboard.press('Enter');
    await page.keyboard.press('ArrowUp');
    await page.keyboard.press('Enter');
    let selects = (await events(page, 'wk-select')).filter(e => e.target === 'demo-inline');
    assert.deepEqual(selects.map(e => e.value), ['settings']);
    assert.equal(await menu.evaluate(m => m.hidden), false);
    // The link item: Enter follows its link and reports the selection.
    await page.keyboard.press('Home');
    await page.keyboard.press('Enter');
    await until(page, () => location.hash === '#menus');
    selects = (await events(page, 'wk-select')).filter(e => e.target === 'demo-inline');
    assert.deepEqual(selects.map(e => e.value), ['settings', 'help']);
    // An item added later, with the tab stop in place: still one tab stop.
    const tabs = () => menu.evaluate(m => [...m.querySelectorAll('[role^=menuitem]')].map(i => i.tabIndex));
    assert.deepEqual(await tabs(), [0, -1, -1, -1]);
    await menu.evaluate(m => { const b = document.createElement('button'); b.setAttribute('role', 'menuitem'); b.textContent = 'Later'; m.append(b); });
    await until(page, () => document.querySelector('#demo-inline [role^=menuitem]:last-child').tabIndex === -1);
    assert.deepEqual(await tabs(), [0, -1, -1, -1, -1], 'the new item is not a second tab stop');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('menu: a link item in a floating menu follows its link on Enter; a disabled link does not', async () => {
    const { page, problems } = await open();
    await page.evaluate(() => {
      const menu = document.querySelector('#demo-menu [role=menu]');
      menu.insertAdjacentHTML('afterbegin', '<a role="menuitem" href="#tabs" data-value="go">Go to tabs</a><a role="menuitem" href="#dialogs" aria-disabled="true" data-value="nope">Disabled link</a>');
    });
    await page.locator('#demo-menu > button').focus();
    await page.keyboard.press('ArrowDown');
    assert.equal(await active(page), 'Go to tabs');
    assert.equal(await page.locator('#demo-menu a').first().evaluate(a => getComputedStyle(a).textDecorationLine), 'none');
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('Enter');
    assert.notEqual(await page.evaluate(() => location.hash), '#dialogs');
    // A click on the disabled link does not follow it either, and the menu stays open.
    const hash = await page.evaluate(() => location.hash);
    await page.locator('#demo-menu a[aria-disabled]').click({ force: true });
    assert.equal(await page.evaluate(() => location.hash), hash, 'a click on a disabled link follows it');
    assert.deepEqual(await events(page, 'wk-select'), []);
    assert.equal(await isOpen(page, 'demo-menu'), true, 'a click on a disabled link closes the menu');
    await page.keyboard.press('ArrowUp');
    await page.keyboard.press('Enter');
    await until(page, () => location.hash === '#tabs');
    assert.equal(await page.evaluate(() => document.querySelector('#demo-menu [role=menu]').hidden), true, 'the menu closed');
    assert.deepEqual((await events(page, 'wk-select')).map(e => e.value), ['go']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('inline menu: hiding the tab stop passes it on; in a hidden section it still has exactly one', async () => {
    const { page, problems } = await open();
    const menu = page.locator('#demo-inline [role=menu]');
    const tabs = () => menu.evaluate(m => [...m.querySelectorAll('[role^=menuitem]')].map(i => [i.dataset.value, i.tabIndex]));
    assert.deepEqual(await tabs(), [['help', 0], ['settings', -1], ['update', -1], ['quit', -1]]);
    await menu.evaluate(m => { m.querySelector('[data-value=help]').hidden = true; });
    await until(page, () => document.querySelector('#demo-inline [data-value=settings]').tabIndex === 0);
    assert.deepEqual(await tabs(), [['help', -1], ['settings', 0], ['update', -1], ['quit', -1]], 'the next item takes the tab stop');
    // Tab still reaches the menu, once.
    await page.locator('#demo-context-target').focus();
    await page.keyboard.press('Tab'); // the tooltip's button
    await page.keyboard.press('Tab');
    assert.equal(await active(page), 'Settings…Ctrl+,');
    await page.keyboard.press('Tab');
    assert.equal(await active(page), 'demo-dialog-open');
    // Shown again, it stays out of the tab order: one tab stop.
    await menu.evaluate(m => { m.querySelector('[data-value=help]').hidden = false; });
    await page.evaluate(() => new Promise(requestAnimationFrame));
    assert.deepEqual(await tabs(), [['help', -1], ['settings', 0], ['update', -1], ['quit', -1]]);

    // An inline menu built inside a hidden section: one tab stop, ready for
    // when the section shows.
    const late = await page.evaluate(() => {
      const section = document.createElement('div');
      section.hidden = true;
      section.innerHTML = '<wk-menu id="late-inline"><div role="menu" class="wk-menu wk-menu--inline" aria-label="Late"><button role="menuitem" type="button">One</button><button role="menuitem" type="button">Two</button><button role="menuitem" type="button">Three</button></div></wk-menu>';
      document.body.append(section);
      const tabs = () => [...section.querySelectorAll('[role=menuitem]')].map(i => i.tabIndex);
      const hidden = tabs();
      section.hidden = false;
      return { hidden, shown: tabs() };
    });
    assert.deepEqual(late, { hidden: [0, -1, -1], shown: [0, -1, -1] });
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('range: forced colours draw the track in CanvasText, the thumb apart from it, and a disabled range in GrayText', async () => {
    const { page, problems } = await open('/demo/index.html', { forcedColors: 'active', reducedMotion: 'reduce' });
    assert.equal(await page.evaluate(() => matchMedia('(forced-colors: active)').matches), true);
    const ranges = page.locator('#demo-ranges .wk-range');
    await ranges.first().scrollIntoViewIfNeeded();
    const system = await page.evaluate(() => Object.fromEntries(['Canvas', 'CanvasText', 'GrayText'].map(c => {
      const e = document.createElement('i'); e.style.forcedColorAdjust = 'none'; e.style.color = c; document.body.append(e);
      const v = getComputedStyle(e).color.match(/\d+/g).slice(0, 3).map(Number); e.remove(); return [c, v];
    })));
    // Pixels of a range with its value at 50%: the track at the middle row
    // near each end, the thumb's centre, and the column through the thumb.
    const sample = async range => {
      await range.evaluate(r => { r.value = (Number(r.min) + Number(r.max)) / 2; });
      const png = (await range.screenshot()).toString('base64');
      return page.evaluate(async png => {
        const img = new Image();
        img.src = `data:image/png;base64,${png}`;
        await img.decode();
        const c = document.createElement('canvas');
        c.width = img.width; c.height = img.height;
        const g = c.getContext('2d');
        g.drawImage(img, 0, 0);
        const data = g.getImageData(0, 0, img.width, img.height).data;
        const at = (x, y) => [...data.slice((y * img.width + x) * 4, (y * img.width + x) * 4 + 3)];
        const mid = Math.floor(img.height / 2), cx = Math.round(img.width / 2);
        const column = [];
        for (let y = 0; y < img.height; y++) column.push(at(cx, y));
        return { trackStart: at(4, mid), trackEnd: at(img.width - 5, mid), thumb: at(cx, mid), column, canvas: at(4, 0) };
      }, png);
    };
    const near = (a, b, d = 40) => Math.abs(a[0] - b[0]) + Math.abs(a[1] - b[1]) + Math.abs(a[2] - b[2]) < d;
    const lum = ([r, g, b]) => .2126 * r + .7152 * g + .0722 * b;
    const on = await sample(ranges.first());
    assert.ok(lum(system.Canvas) > lum(system.CanvasText) + 100, 'a light Canvas behind dark CanvasText');
    assert.ok(near(on.canvas, system.Canvas), `the background is Canvas (${on.canvas})`);
    assert.ok(near(on.trackStart, system.CanvasText), `the track is CanvasText at the start (${on.trackStart})`);
    assert.ok(near(on.trackEnd, system.CanvasText), `the track is CanvasText at the end (${on.trackEnd})`);
    assert.ok(lum(on.thumb) - lum(on.trackStart) > 100, `the thumb (${on.thumb}) stands apart from the track (${on.trackStart})`);
    // The thumb's ring shows above the track, against Canvas.
    const above = on.column.slice(0, Math.floor(on.column.length / 2) - 3);
    assert.ok(above.some(p => lum(p) < lum(system.Canvas) - 100), 'the thumb has a dark edge above the track');
    const off = await sample(ranges.nth(2));
    assert.equal(await ranges.nth(2).evaluate(r => r.disabled), true);
    assert.ok(near(off.trackStart, system.GrayText, 24), `a disabled track is GrayText (${off.trackStart}, GrayText ${system.GrayText})`);
    assert.ok(lum(off.thumb) - lum(off.trackStart) > 60, `a disabled thumb (${off.thumb}) stands apart from its track (${off.trackStart})`);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('range: --wk-range-value: 30% fills 30% of the track in the accent; input updates it', async () => {
    const { page, problems } = await open();
    const range = page.locator('#demo-ranges .wk-range').nth(1);
    // The thumb sits at the value: move the value to 0 so it does not cover
    // the end of the fill, which follows --wk-range-value alone.
    await range.evaluate(r => { r.value = 0; r.style.setProperty('--wk-range-value', '30%'); });
    const png = (await range.screenshot()).toString('base64');
    const accent = await range.evaluate(r => { const e = document.createElement('i'); e.style.color = 'var(--wk-color-accent)'; r.parentElement.append(e); const v = getComputedStyle(e).color; e.remove(); return v.match(/\d+/g).map(Number); });
    const fill = await page.evaluate(async ({ png, accent }) => {
      const img = new Image();
      img.src = `data:image/png;base64,${png}`;
      await img.decode();
      const c = document.createElement('canvas');
      c.width = img.width; c.height = img.height;
      const g = c.getContext('2d');
      g.drawImage(img, 0, 0);
      const y = Math.floor(img.height / 2);
      const row = g.getImageData(0, y, img.width, 1).data;
      let last = -1;
      for (let x = 0; x < img.width; x++) {
        const [r, gg, b] = row.slice(x * 4, x * 4 + 3);
        if (Math.abs(r - accent[0]) + Math.abs(gg - accent[1]) + Math.abs(b - accent[2]) < 24) last = x;
      }
      return { last, width: img.width };
    }, { png, accent });
    const share = (fill.last + 1) / fill.width;
    assert.ok(Math.abs(share - .3) < .02, `the fill ends at ${(share * 100).toFixed(1)}% of the track`);
    // The demo sets the share on input, as an app does.
    await range.evaluate(r => { r.value = 70; r.dispatchEvent(new Event('input', { bubbles: true })); });
    assert.equal(await range.evaluate(r => [r.style.getPropertyValue('--wk-range-value'), r.nextElementSibling.textContent].join(' ')), '70% 70%');
    assert.deepEqual(problems, []);
    await page.close();
  });

  // ---- Busy controls, the spinner and the progress bar (BEHAVIOUR.md) ----
  await check('busy button: keeps focus and its accessible name, ignores a second press (pointer and keys), and does not look disabled', async () => {
    const { page, problems } = await open();
    // What the task helper will do: busy at once on the press, the spinner
    // later (the test adds it itself rather than wait for the delay).
    await page.evaluate(() => {
      const b = document.createElement('button');
      b.className = 'wk-button';
      b.type = 'button';
      b.id = 'test-busy';
      b.textContent = 'Export';
      document.getElementById('demo-busy').append(b);
      window.presses = 0;
      b.addEventListener('click', () => { window.presses++; b.setAttribute('aria-disabled', 'true'); b.dataset.busy = ''; });
      document.addEventListener('click', e => { if (e.target === b) window.presses += 100; });
    });
    const b = page.locator('#test-busy');
    const width = await b.evaluate(el => el.getBoundingClientRect().width);
    await b.click();
    assert.equal(await page.evaluate(() => window.presses), 101, 'the first press reaches the app');
    // A second press, by pointer and by keyboard, does nothing; focus stays.
    // (force: Playwright would otherwise wait for aria-disabled to go.)
    await b.click({ force: true });
    await page.keyboard.press('Enter');
    await page.keyboard.press('Space');
    assert.equal(await page.evaluate(() => window.presses), 101);
    assert.equal(await active(page), 'test-busy');
    const look = sel => page.evaluate(s => {
      const cs = getComputedStyle(document.querySelector(s));
      return { opacity: cs.opacity, cursor: cs.cursor, face: cs.backgroundImage, shadow: cs.boxShadow };
    }, sel);
    const busy = await look('#test-busy');
    assert.equal(busy.opacity, '1');
    assert.equal(busy.cursor, 'progress');
    assert.match(busy.face, /linear-gradient/, 'the pressed face');
    assert.notEqual(busy.shadow, 'none');
    // A busy button looks nothing like a disabled or a merely aria-disabled one.
    assert.equal(await page.evaluate(() => getComputedStyle(document.querySelector('#buttons button[disabled]')).opacity), '0.45');
    assert.equal(await page.evaluate(() => getComputedStyle(document.querySelector('#buttons .wk-button--locked')).opacity), '0.45');
    // The spinner takes the label's place: the width holds, the label is
    // not painted, the spinner is centred, and the name stays.
    await b.evaluate(el => { const s = document.createElement('span'); s.className = 'wk-spinner'; s.setAttribute('aria-hidden', 'true'); el.prepend(s); });
    assert.equal(await b.evaluate(el => el.getBoundingClientRect().width), width);
    assert.equal(await b.evaluate(el => getComputedStyle(el).webkitTextFillColor), 'rgba(0, 0, 0, 0)');
    const spin = await b.evaluate(el => {
      const s = el.querySelector('.wk-spinner').getBoundingClientRect();
      const r = el.getBoundingClientRect();
      return { dx: Math.abs((s.left + s.width / 2) - (r.left + r.width / 2)), dy: Math.abs((s.top + s.height / 2) - (r.top + r.height / 2)), w: el.querySelector('.wk-spinner').offsetWidth };
    });
    assert.ok(spin.dx < 1 && spin.dy < 1, `the spinner is centred: ${JSON.stringify(spin)}`);
    assert.equal(spin.w, 14);
    const named = (await axTree(page)).filter(n => n.role?.value === 'button' && n.name?.value === 'Export');
    assert.ok(named.length >= 2, 'the busy buttons keep their accessible names');
    await b.click({ force: true });
    assert.equal(await page.evaluate(() => window.presses), 101, 'a press on the spinner does nothing either');
    assert.equal(await active(page), 'test-busy');
    // The static specimens: a small button's spinner is 12 px.
    assert.equal(await page.evaluate(() => document.querySelector('#demo-busy .wk-button--small > .wk-spinner').offsetWidth), 12);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('busy: a busy link-button, filter or chip ignores presses too; a busy link does not navigate', async () => {
    const { page, problems } = await open();
    const out = await page.evaluate(() => {
      const host = document.createElement('div');
      host.innerHTML = '<button class="wk-filter" type="button" aria-disabled="true" data-busy>F</button><a class="wk-button" href="#elsewhere" aria-disabled="true" data-busy><span>Go</span></a>';
      document.getElementById('busy').append(host);
      let n = 0;
      host.addEventListener('click', () => n++);
      for (const el of host.children) el.click();
      host.querySelector('a span').click();
      return { n, hash: location.hash };
    });
    assert.deepEqual(out, { n: 0, hash: '' });
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('busy: only the pressed control counts; a control inside a [data-busy] container still works, data-busy without aria-disabled is not blocked, and a busy ARIA switch, tab or menu item is', async () => {
    const { page, problems } = await open();
    const out = await page.evaluate(() => {
      const host = document.createElement('div');
      host.innerHTML = '<div data-busy aria-disabled="true"><button class="wk-button" type="button" id="t-stop">Stop</button><span id="t-text">Removing…</span></div>'
        + '<button class="wk-button" type="button" data-busy id="t-pressed">Pressed only</button>'
        + '<div role="menu"><button role="menuitem" type="button" data-busy aria-disabled="true" id="t-item">Item</button></div>'
        + '<div role="switch" aria-checked="false" tabindex="0" data-busy aria-disabled="true" id="t-switch"><span id="t-switch-knob">Sync</span></div>'
        + '<div role="tablist"><div role="tab" tabindex="0" data-busy aria-disabled="true" id="t-tab"><span id="t-tab-label">Tab</span></div></div>';
      document.getElementById('busy').append(host);
      const hits = window.hits = {};
      host.addEventListener('click', e => { const id = e.target.closest('[id]').id; hits[id] = (hits[id] || 0) + 1; });
      for (const id of ['t-stop', 't-text', 't-pressed', 't-item', 't-switch', 't-switch-knob', 't-tab-label']) document.getElementById(id).click();
      return hits;
    });
    assert.deepEqual(out, { 't-stop': 1, 't-text': 1, 't-pressed': 1 });
    // From the keyboard too: the Stop button in a busy row works.
    await page.focus('#t-stop');
    await page.keyboard.press('Enter');
    assert.equal(await page.evaluate(() => window.hits['t-stop']), 2);
    assert.equal(await active(page), 't-stop');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('progress bar: setProgress keeps aria-valuenow and aria-valuetext in step, changes them at most every 5 s (aria: now and done always), moves the fill on every call, and an indeterminate bar has no value', async () => {
    const { page, problems } = await open();
    const ax = async name => (await axTree(page)).find(n => n.role?.value === 'progressbar' && n.name?.value === name);
    const state = () => page.evaluate(() => {
      const bar = document.getElementById('demo-progress');
      const a = n => bar.getAttribute(`aria-value${n}`);
      // The fill eases to its new share; read where it ends.
      for (const t of bar.getAnimations()) t.finish();
      return { min: a('min'), max: a('max'), now: a('now'), text: a('text'), fill: getComputedStyle(bar).backgroundSize.split(',')[0].trim(), height: bar.getBoundingClientRect().height };
    });
    assert.deepEqual(await state(), { min: '0', max: '40', now: '12', text: '12 of 40 files', fill: '30% 100%', height: 4 });
    // The accessibility tree carries the value (DevTools leaves valuetext empty).
    const node = await ax('Copying files');
    assert.deepEqual([node.value?.value, node.properties.find(p => p.name === 'valuemax')?.value.value], [12, 40]);
    await page.evaluate(async () => {
      const { setProgress } = await import('/src/index.js');
      setProgress(document.getElementById('demo-progress'), { done: 20, total: 40, unit: 'files' }, { aria: false });
    });
    const moved = await state();
    assert.equal(moved.fill, '50% 100%', 'the fill moves');
    assert.deepEqual([moved.now, moved.text], ['12', '12 of 40 files'], 'the value attributes wait');
    await page.evaluate(async () => {
      const { setProgress } = await import('/src/index.js');
      setProgress(document.getElementById('demo-progress'), { done: 21, total: 40, text: 'Step 2 of 3 · 21 of 40 files' }, { aria: 'now' });
    });
    const both = await state();
    assert.deepEqual([both.now, both.text, both.fill], ['21', 'Step 2 of 3 · 21 of 40 files', '52.5% 100%']);
    // A first call with { aria: false } still makes the bar determinate.
    const first = await page.evaluate(async () => {
      const { setProgress } = await import('/src/index.js');
      const bar = document.createElement('div');
      bar.className = 'wk-progress';
      bar.setAttribute('role', 'progressbar');
      document.getElementById('busy').append(bar);
      setProgress(bar, { done: 3, total: 4 }, { aria: false });
      let threw = false;
      try { setProgress(bar, { done: 1, total: 0 }); } catch (e) { threw = e instanceof RangeError; }
      return [bar.getAttribute('aria-valuenow'), bar.getAttribute('aria-valuetext'), threw];
    });
    assert.deepEqual(first, ['3', '3 of 4', true]);
    // The value attributes change at most every timings.progressValueEvery
    // (an injected clock, no waiting): the fill moves on every call; the
    // first call, a finished one and { aria: 'now' } always set them.
    const capped = await page.evaluate(async () => {
      const { setProgress, timings } = await import('/src/index.js');
      const bar = document.createElement('div');
      bar.className = 'wk-progress';
      document.getElementById('busy').append(bar);
      let t = 1000;
      const clock = () => t;
      const step = (done, at, aria) => {
        t = at;
        setProgress(bar, { done, total: 10 }, aria === undefined ? { clock } : { clock, aria });
        return [bar.getAttribute('aria-valuenow'), bar.style.getPropertyValue('--wk-progress-value')];
      };
      return {
        every: timings.progressValueEvery,
        steps: [
          step(1, 1000),                 // first: set
          step(2, 2000),                 // within 5 s: fill only
          step(3, 5999),                 // still within
          step(4, 6000),                 // 5 s after the last set: set
          step(5, 6001, 'now'),          // forced
          step(6, 6002, false),          // never
          step(10, 6003),                // done >= total: set
        ],
      };
    });
    // A new total or unit inside the window, and a clock that reads earlier
    // than the last set, change the attributes at once; new words alone wait
    // for the next due update, so words that embed the count can't defeat
    // the cap.
    const changed = await page.evaluate(async () => {
      const { setProgress } = await import('/src/index.js');
      const bar = document.createElement('div');
      bar.className = 'wk-progress';
      document.getElementById('busy').append(bar);
      let t = 0;
      const set = (p, at) => { t = at; setProgress(bar, p, { clock: () => t }); return [bar.getAttribute('aria-valuemax'), bar.getAttribute('aria-valuenow'), bar.getAttribute('aria-valuetext'), bar.style.getPropertyValue('--wk-progress-value')]; };
      return [
        set({ done: 1, total: 4 }, 0),
        set({ done: 1, total: 10 }, 100),
        set({ done: 2, total: 10 }, 200),
        set({ done: 2, total: 10, unit: 'files' }, 300),
        set({ done: 3, total: 10, unit: 'files', text: 'Step 2 of 3' }, 400),
        set({ done: 4, total: 10, unit: 'files', text: 'Step 2 of 3' }, 500),
        set({ done: 5, total: 10, unit: 'files', text: 'Step 2 of 3' }, 50),
      ];
    });
    assert.deepEqual(changed, [
      ['4', '1', '1 of 4', '25%'],                    // first
      ['10', '1', '1 of 10', '10%'],                  // a new total: at once
      ['10', '1', '1 of 10', '20%'],                  // within the window: fill only
      ['10', '2', '2 of 10 files', '20%'],            // a new unit: at once
      ['10', '2', '2 of 10 files', '30%'],            // new words alone: they wait for the next due update
      ['10', '2', '2 of 10 files', '40%'],            // still waiting: fill only
      ['10', '5', 'Step 2 of 3', '50%'],              // a clock reading earlier than the last set: at once, with the new words
    ]);
    assert.equal(capped.every, 5000);
    assert.deepEqual(capped.steps, [['1', '10%'], ['1', '20%'], ['1', '30%'], ['4', '40%'], ['5', '50%'], ['5', '60%'], ['10', '100%']]);
    const sweep = await page.evaluate(() => {
      const bar = document.getElementById('demo-progress-sweep');
      return { values: [...bar.attributes].filter(a => a.name.startsWith('aria-value')).length, animation: getComputedStyle(bar, '::before').animationName };
    });
    assert.deepEqual(sweep, { values: 0, animation: 'wk-progress-sweep' });
    const indeterminate = (await axTree(page)).filter(n => n.role?.value === 'progressbar' && n.name?.value === 'Fetching runs');
    assert.equal(indeterminate.length, 2, 'the standalone spinner and the indeterminate bar');
    for (const n of indeterminate) assert.equal(n.value, undefined);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('reduced motion: the spinner still turns, the indeterminate bar fades in place, and nothing slides', async () => {
    const { page, problems } = await open('/demo/index.html', { reducedMotion: 'reduce' });
    const out = await page.evaluate(() => {
      const running = document.getAnimations().filter(a => a.playState === 'running');
      const names = running.map(a => a.animationName);
      // Only the spinner's turn may transform; nothing translates or scales.
      const moving = running.filter(a => a.animationName !== 'wk-spin' && a.effect.getKeyframes().some(k => k.transform && k.transform !== 'none')).map(a => a.animationName);
      const sweep = getComputedStyle(document.getElementById('demo-progress-sweep'), '::before');
      const sw = document.querySelector('.wk-switch');
      return {
        spinners: running.filter(a => a.animationName === 'wk-spin').length,
        spinnersInPage: document.querySelectorAll('.wk-spinner').length,
        fade: names.includes('wk-progress-fade'), sweep: names.includes('wk-progress-sweep'),
        width: sweep.width === getComputedStyle(document.getElementById('demo-progress-sweep')).width,
        moving, bar: getComputedStyle(document.getElementById('demo-progress')).transitionDuration,
        switch: [getComputedStyle(sw).transitionDuration, getComputedStyle(sw, '::before').transitionDuration],
      };
    });
    assert.equal(out.spinners, out.spinnersInPage, 'every spinner turns');
    assert.ok(out.spinners > 0);
    assert.deepEqual([out.fade, out.sweep, out.width], [true, false, true]);
    assert.deepEqual(out.moving, []);
    assert.equal(out.bar, '0s');
    assert.deepEqual(out.switch, ['0s', '0s']);
    // Without reduced motion: the switch uses --wk-duration-fast (100 ms).
    const normal = await open();
    assert.equal(await normal.page.evaluate(() => getComputedStyle(document.querySelector('.wk-switch')).transitionDuration), '0.1s, 0.1s');
    assert.deepEqual(problems, []);
    assert.deepEqual(normal.problems, []);
    await page.close();
    await normal.page.close();
  });

  await check('forced colours: the bar is Highlight in a CanvasText outline, the spinner keeps its border, and a busy button\'s label stays hidden under its spinner', async () => {
    const { page, problems } = await open('/demo/index.html', { forcedColors: 'active', reducedMotion: 'reduce' });
    const out = await page.evaluate(() => {
      const bar = getComputedStyle(document.getElementById('demo-progress'));
      const sweep = getComputedStyle(document.getElementById('demo-progress-sweep'), '::before');
      const spinner = getComputedStyle(document.getElementById('demo-spinner'));
      return { outline: bar.outlineStyle, fill: bar.backgroundImage, sweep: sweep.backgroundColor, spinner: [spinner.borderTopStyle, spinner.borderTopColor !== 'rgba(0, 0, 0, 0)', spinner.borderRightColor] };
    });
    assert.equal(out.outline, 'solid');
    assert.match(out.fill, /linear-gradient/);
    assert.notEqual(out.sweep, 'rgba(0, 0, 0, 0)');
    assert.deepEqual(out.spinner, ['solid', true, 'rgba(0, 0, 0, 0)']);
    // The label is not painted: the button looks the same with no text.
    const button = page.locator('#demo-busy .wk-button:not(.wk-button--small):not(.wk-button--primary):not(.wk-button--quiet):has(> .wk-spinner)');
    await page.evaluate(() => document.querySelectorAll('.wk-spinner').forEach(s => { s.style.animation = 'none'; }));
    const withLabel = await button.screenshot();
    await button.evaluate(el => { const w = el.getBoundingClientRect().width; el.lastChild.textContent = ''; el.style.width = `${w}px`; });
    const without = await button.screenshot();
    assert.equal(Buffer.compare(withLabel, without), 0, 'the label shows through in forced colours');
    // The focus ring of a button with a spinner is Highlight, not the accent
    // (it opts out of forced colours); a busy button with no spinner yet
    // still differs from one at rest, by a Highlight border.
    const marks = await page.evaluate(() => {
      const probe = document.createElement('i');
      probe.style.color = 'Highlight';
      document.body.append(probe);
      const highlight = getComputedStyle(probe).color;
      probe.remove();
      const spinning = document.querySelector('#demo-busy .wk-button--primary');
      spinning.focus();
      const pressed = [...document.querySelectorAll('#demo-busy .wk-button')].find(b => b.textContent === 'Pressed');
      const rest = document.getElementById('demo-busy-try');
      return {
        highlight, focusVisible: spinning.matches(':focus-visible'), ring: getComputedStyle(spinning).outlineColor, ringStyle: getComputedStyle(spinning).outlineStyle,
        busy: getComputedStyle(pressed).borderTopColor, rest: getComputedStyle(rest).borderTopColor,
      };
    });
    assert.ok(marks.focusVisible);
    assert.deepEqual([marks.ring, marks.ringStyle], [marks.highlight, 'solid']);
    assert.equal(marks.busy, marks.highlight);
    assert.notEqual(marks.rest, marks.highlight);
    assert.deepEqual(problems, []);
    await page.close();
  });

  // ---- The prebuilt module and the settings window ----
  await check('dist/wails-kit-ui.js exports what src/index.js exports, and defines the same elements', async () => {
    const { page, problems } = await open('/demo/dist.html');
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

  const openSettings = async (options = {}) => {
    const r = await open('/demo/settings.html', { ...options, ready: false });
    await r.page.waitForFunction(() => document.querySelector('#demo-settings .wk-settings-row'));
    // A fake clock stands still from here; the test moves it.
    if (options.clock) await r.page.clock.pauseAt(await r.page.evaluate(() => Date.now() + 1000));
    // Everything the window's announcer says from here, in order: window.heard, [politeness, words].
    await r.page.evaluate(() => {
      window.heard = [];
      const host = document.querySelector('[data-wk-announcer]');
      const last = new Map();
      new MutationObserver(() => {
        for (const reg of host.querySelectorAll('[aria-live]')) {
          const t = reg.textContent;
          if (t && t !== last.get(reg)) window.heard.push([reg.getAttribute('aria-live'), t]);
          last.set(reg, t);
        }
      }).observe(host, { subtree: true, childList: true, characterData: true });
    });
    return r;
  };
  const row = (page, key) => page.locator(`#demo-settings .wk-settings-row[data-key="${key}"]`);
  const lastEvent = page => page.evaluate(() => window.settingsEvents.at(-1));

  await check('settings window: one tab per group, a row per field, the right control per type, in light and dark', async () => {
    for (const scheme of ['light', 'dark']) {
      const { page, problems } = await openSettings({ colorScheme: scheme });
      const tabs = page.locator('#demo-settings [role=tab]');
      assert.deepEqual(await tabs.allTextContents(), ['General', 'Provider']);
      assert.equal(await tabs.first().getAttribute('aria-selected'), 'true');
      assert.equal(await page.locator('#demo-settings [role=tabpanel]:visible').count(), 1);
      const control = key => row(page, key).evaluate(r => {
        const c = r.querySelector('input, select, output');
        return [c.tagName.toLowerCase(), c.type || '', c.className, c.getAttribute('role')];
      });
      assert.deepEqual(await control('general.name'), ['input', 'text', 'wk-field wk-settings-control', null]);
      assert.deepEqual(await control('general.launch'), ['input', 'checkbox', 'wk-switch', 'switch']);
      assert.deepEqual(await control('general.updates'), ['select', 'select-one', 'wk-select', null]);
      assert.deepEqual(await control('general.copies'), ['input', 'number', 'wk-field wk-num', null]);
      assert.deepEqual(await control('general.config'), ['output', 'output', 'wk-settings-value', null]);
      assert.equal(await row(page, 'general.launch').locator('input').isChecked(), true);
      assert.equal(await row(page, 'general.updates').locator('select').inputValue(), 'weekly');
      assert.equal(await row(page, 'general.copies').locator('input').inputValue(), '2');
      assert.equal(await row(page, 'general.config').locator('output').textContent(), '~/.config/example/settings.json');
      // Every control has an accessible name from its label.
      const names = await page.evaluate(() => [...document.querySelectorAll('#demo-settings .wk-settings-row')].map(r => {
        const c = r.querySelector('input, select, output');
        const by = c.getAttribute('aria-labelledby');
        return by ? document.getElementById(by).textContent : c.labels?.[0]?.textContent.trim();
      }));
      assert.ok(names.every(Boolean), `unnamed controls: ${names}`);
      // The description is read with the control.
      const help = await row(page, 'general.name').locator('input').getAttribute('aria-describedby');
      assert.equal(await page.locator(`[id="${help}"]`).textContent(), 'Shown to people you share with.');
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false);
      assert.deepEqual(problems, []);
      await page.close();
    }
  });

  await check('settings window: conditions, dynamic options and the reset of a dependent select', async () => {
    const { page, problems } = await openSettings();
    await page.locator('#demo-settings [role=tab]', { hasText: 'Provider' }).click();
    assert.equal(await row(page, 'provider.key').isVisible(), true);
    assert.equal(await row(page, 'provider.port').isVisible(), false);
    assert.equal(await row(page, 'provider.key').locator('input').inputValue(), '•'.repeat(8));
    assert.deepEqual(await row(page, 'provider.model').locator('option').allTextContents(), ['Alpha Small', 'Alpha Large']);
    await row(page, 'provider.name').locator('select').selectOption('local');
    assert.equal(await row(page, 'provider.key').isVisible(), false);
    assert.equal(await row(page, 'provider.port').isVisible(), true);
    const model = row(page, 'provider.model').locator('select');
    assert.equal(await model.isDisabled(), true);
    assert.deepEqual(await model.locator('option').allTextContents(), ['None available']);
    let e = await lastEvent(page);
    assert.equal(e.key, 'provider.name');
    assert.equal(e.values['provider.model'], '');
    await row(page, 'provider.name').locator('select').selectOption('beta');
    assert.equal(await model.inputValue(), 'b-1');
    e = await lastEvent(page);
    assert.equal(e.values['provider.model'], 'b-1');
    assert.equal(e.values['provider.key'], '•'.repeat(8), 'an untouched secret goes back as the mask');
    // The first visible row draws no hairline above it.
    await row(page, 'provider.name').locator('select').selectOption('local');
    const borders = await page.evaluate(() => [...document.querySelectorAll('#demo-settings [data-group=provider] > .wk-panel > .wk-settings-row')].filter(r => r.checkVisibility()).map(r => getComputedStyle(r).borderTopStyle));
    assert.deepEqual(borders, ['none', 'solid', 'solid']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('settings window: typed values, errors from the app, the attention dot, and the advanced section', async () => {
    const { page, problems } = await openSettings();
    const name = row(page, 'general.name').locator('input');
    await name.fill('');
    await name.press('Tab');
    let e = await lastEvent(page);
    assert.deepEqual([e.key, e.value], ['general.name', '']);
    await until(page, () => document.querySelector('[data-key="general.name"] .wk-error:not([hidden])'));
    assert.equal(await name.getAttribute('aria-invalid'), 'true');
    const error = row(page, 'general.name').locator('.wk-error');
    assert.equal(await error.textContent(), 'Display name is required');
    assert.ok((await name.getAttribute('aria-describedby')).split(' ').includes(await error.getAttribute('id')));
    assert.equal(await page.locator('#demo-settings [role=tab]', { hasText: 'General' }).getAttribute('data-attention'), '');
    await name.fill('Ana');
    await name.press('Tab');
    assert.equal(await name.getAttribute('aria-invalid'), null);
    assert.equal(await error.isVisible(), false);
    assert.equal(await page.locator('#demo-settings [role=tab]', { hasText: 'General' }).getAttribute('data-attention'), null);
    // A toggle sends a boolean, a stepper a number.
    await row(page, 'general.launch').locator('label').click();
    e = await lastEvent(page);
    assert.deepEqual([e.key, e.value], ['general.launch', false]);
    await row(page, 'general.copies').locator('button[aria-label=More]').click();
    e = await lastEvent(page);
    assert.deepEqual([e.key, e.value], ['general.copies', 3]);
    // Arrow keys move between the panes; the advanced fields open on demand.
    await page.locator('#demo-settings [role=tab][aria-selected=true]').focus();
    await page.keyboard.press('ArrowRight');
    assert.equal(await page.locator('#demo-settings [role=tab]', { hasText: 'Provider' }).getAttribute('aria-selected'), 'true');
    const advanced = page.locator('#demo-settings .wk-settings-advanced');
    assert.equal(await row(page, 'provider.endpoint').isVisible(), false);
    await advanced.click();
    assert.equal(await advanced.getAttribute('aria-expanded'), 'true');
    assert.equal(await row(page, 'provider.endpoint').isVisible(), true);
    // A secret: focusing the mask selects it, so typing replaces it.
    const key = row(page, 'provider.key').locator('input');
    await key.focus();
    await page.keyboard.type('new-key');
    await key.press('Tab');
    e = await lastEvent(page);
    assert.deepEqual([e.key, e.value], ['provider.key', 'new-key']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('settings window: a schema set again rebuilds it; one group has a heading and no tabs; none shows an empty state', async () => {
    const { page, problems } = await openSettings();
    const look = await page.evaluate(() => {
      const el = document.getElementById('demo-settings');
      const out = {};
      el.schema = { groups: [{ key: 'only', label: 'Only', fields: [{ key: 'x', type: 'text', label: 'X' }] }] };
      out.one = { tabs: el.querySelectorAll('[role=tab]').length, heading: el.querySelector('.wk-settings-heading')?.textContent, rows: el.querySelectorAll('.wk-settings-row').length };
      el.schema = { groups: [] };
      out.none = el.querySelector('.wk-empty')?.textContent;
      return out;
    });
    assert.deepEqual(look, { one: { tabs: 0, heading: 'Only', rows: 1 }, none: 'There are no settings.' });
    assert.deepEqual(problems, []);
    await page.close();
  });

  // ---- Settings window: validation timing, back-end checks and the Notice (BEHAVIOUR.md "Forms and validation") ----
  // A check the test answers: window.checks holds each call's key, values and resolve.
  const manualChecks = page => page.evaluate(() => {
    window.checks = [];
    document.getElementById('demo-settings').check = (key, values) => new Promise(resolve => window.checks.push({ key, values, resolve }));
  });
  const answer = (page, i, errors) => page.evaluate(([n, errs]) => window.checks[n].resolve(errs), [i, errors]);
  // The pending words shown beside a field (wkTask's), and what the window's
  // announcer has said since the last call (its messages go out after 500 ms:
  // on the fake clock, said() moves it on first).
  const pendingText = (page, key) => row(page, key).locator('.wk-settings-pending').evaluate(p => p.textContent);
  const said = async (page, { clock = true } = {}) => {
    if (clock) await page.clock.runFor(600);
    return page.evaluate(() => window.heard.splice(0).map(h => h[1]));
  };
  const eventCount = page => page.evaluate(() => window.settingsEvents.length);

  await check('settings window: an error shows when the field is left, then clears (and returns) as the person types', async () => {
    const { page, problems } = await openSettings();
    const name = row(page, 'general.name').locator('input');
    const error = row(page, 'general.name').locator('.wk-error');
    // Typing a value that will fail shows nothing until the field is left.
    const tooLong = 'a'.repeat(41);
    await page.evaluate(() => { document.getElementById('demo-settings').validate = (schema, values) => String(values['general.name'] ?? '').length > 40 ? [{ field: 'general.name', message: 'Display name must be at most 40 characters', code: 'max_length' }] : []; });
    await name.fill(tooLong);
    assert.equal(await name.getAttribute('aria-invalid'), null, 'no error while typing before the field shows one');
    await name.press('Tab');
    assert.equal(await error.textContent(), 'Display name must be at most 40 characters');
    // Once shown, each keystroke re-checks the draft: the error clears with no change event.
    const before = await eventCount(page);
    await name.focus();
    await page.keyboard.press('End');
    await page.keyboard.press('Backspace');
    assert.equal(await name.getAttribute('aria-invalid'), null);
    assert.equal(await error.isVisible(), false);
    assert.equal(await page.locator('#demo-settings [role=tab]', { hasText: 'General' }).getAttribute('data-attention'), null);
    assert.equal(await eventCount(page), before, 'the value applies on change, not on each keystroke');
    await page.keyboard.type('b');
    assert.equal(await error.textContent(), 'Display name must be at most 40 characters', 'and it comes back while the field is still being re-checked');
    await page.keyboard.press('Backspace');
    await name.press('Tab');
    const e = await lastEvent(page);
    assert.deepEqual([e.key, e.value], ['general.name', 'a'.repeat(40)]);
    assert.equal(await error.isVisible(), false);
    // Without validate, an empty required field's error clears as soon as something is typed.
    await page.evaluate(() => { document.getElementById('demo-settings').validate = null; });
    await name.fill('');
    await name.press('Tab');
    assert.equal(await error.textContent(), 'Display name is required');
    await name.focus();
    await page.keyboard.type('A');
    assert.equal(await name.getAttribute('aria-invalid'), null);
    await page.keyboard.press('Backspace');
    assert.equal(await error.textContent(), 'Display name is required');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('settings window: a back-end check shows its words beside the field after 400 ms, for at least 500 ms, and never blocks typing', async () => {
    const { page, problems } = await openSettings({ clock: true });
    await manualChecks(page);
    const folder = row(page, 'general.folder').locator('input');
    const before = await eventCount(page);
    await folder.fill('/work');
    await folder.press('Tab');
    assert.equal(await page.evaluate(() => window.checks.length), 1);
    assert.deepEqual(await page.evaluate(() => [window.checks[0].key, window.checks[0].values['general.folder'], window.checks[0].values['general.name']]), ['general.folder', '/work', 'Sam']);
    assert.equal(await pendingText(page, 'general.folder'), '', 'nothing in the quiet wait');
    await page.clock.runFor(399);
    assert.equal(await pendingText(page, 'general.folder'), '');
    await page.clock.runFor(1);
    assert.equal(await pendingText(page, 'general.folder'), 'Checking folder…');
    assert.equal(await row(page, 'general.folder').getAttribute('data-pending'), '');
    const pending = row(page, 'general.folder').locator('.wk-settings-pending');
    // wkTask's indicator: a working dot and the words in its target, no live region of its own.
    assert.equal(await pending.getAttribute('role'), null);
    assert.equal(await pending.locator('.wk-task-state[data-state=running] > .wk-dot[data-tone=working][aria-hidden=true]').count(), 1);
    assert.ok((await folder.getAttribute('aria-describedby')).split(' ').includes(await pending.getAttribute('id')));
    // Its words go to the announcer when they show (after the announcer's 500 ms).
    await page.clock.runFor(1);
    await page.clock.runFor(499);
    assert.deepEqual(await page.evaluate(() => window.heard.splice(0).map(h => h[1])), ['Checking folder…']);
    assert.equal(await folder.isEditable(), true, 'typing is never blocked');
    // The check passes 500 ms after the words appeared: they go, and the value applies, at once.
    await answer(page, 0, null);
    await until(page, () => window.settingsEvents.at(-1)?.key === 'general.folder');
    assert.equal(await pendingText(page, 'general.folder'), '');
    assert.deepEqual(await said(page), ['Projects folder checked']);
    // A check that passes 100 ms after its words appeared: they stay until 500 ms, then the value applies in the same step.
    await folder.fill('/work2');
    await folder.press('Tab');
    await page.clock.runFor(500);
    assert.equal(await pendingText(page, 'general.folder'), 'Checking folder…');
    const applied = await eventCount(page);
    await answer(page, 1, null);
    await page.clock.runFor(399);
    assert.equal(await pendingText(page, 'general.folder'), 'Checking folder…');
    assert.equal(await eventCount(page), applied, 'the result waits for the indicator minimum');
    await page.clock.runFor(1);
    assert.equal(await pendingText(page, 'general.folder'), '');
    assert.equal(await row(page, 'general.folder').getAttribute('data-pending'), null);
    // The outcome is announced in words, by the window's announcer; nothing stays beside the field.
    assert.equal(await pending.evaluate(p => p.childNodes.length), 0);
    assert.equal((await folder.getAttribute('aria-describedby') || '').split(' ').includes(await pending.getAttribute('id')), false);
    const e = await lastEvent(page);
    assert.deepEqual([e.key, e.value, e.checked], ['general.folder', '/work2', true]);
    assert.equal(await page.evaluate(() => document.getElementById('demo-settings').values['general.folder']), '/work2');
    assert.deepEqual(await said(page), ['Checking folder…', 'Projects folder checked']);
    // A quick check shows no words at all, and applies at once; only its outcome is announced.
    await folder.fill('/quick');
    await folder.press('Tab');
    await page.clock.runFor(200);
    await answer(page, 2, []);
    await until(page, () => window.settingsEvents.at(-1).value === '/quick');
    assert.equal(await pendingText(page, 'general.folder'), '');
    await page.clock.runFor(1000);
    assert.equal(await pendingText(page, 'general.folder'), '', 'the indicator never appears after the result');
    assert.deepEqual(await said(page), ['Projects folder checked']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('settings window: a value applies only once its check passes; after a failure it re-checks 500 ms after typing stops', async () => {
    const { page, problems } = await openSettings({ clock: true });
    await manualChecks(page);
    const folder = row(page, 'general.folder').locator('input');
    const error = row(page, 'general.folder').locator('.wk-error');
    const before = await eventCount(page);
    await folder.fill('nowhere');
    await folder.press('Tab');
    await answer(page, 0, [{ field: 'general.folder', message: 'There is no folder at this path. Choose a folder that exists.', code: 'check' }]);
    await until(page, () => !document.querySelector('[data-key="general.folder"] .wk-error').hidden);
    assert.equal(await error.textContent(), 'There is no folder at this path. Choose a folder that exists.');
    // The failure is announced by the window's announcer, with the Notice's new count (joined, as they come together).
    assert.deepEqual(await said(page), ['There is no folder at this path. Choose a folder that exists. 1 setting needs attention.']);
    assert.equal(await eventCount(page), before, 'a value that failed its check is not applied');
    assert.equal(await page.evaluate(() => document.getElementById('demo-settings').values['general.folder']), '~/Projects');
    assert.equal(await folder.inputValue(), 'nowhere', 'the person\'s text stays');
    // Something else changing redraws the window, and the draft and its error stay.
    await row(page, 'general.launch').locator('label').click();
    assert.equal(await folder.inputValue(), 'nowhere');
    assert.equal(await folder.getAttribute('aria-invalid'), 'true');
    // Typing re-checks with the back end 500 ms after the last keystroke, not on each one.
    await folder.focus();
    await page.keyboard.press('Control+A');
    await page.keyboard.type('/ho');
    await page.clock.runFor(300);
    await page.keyboard.type('me');
    await page.clock.runFor(499);
    assert.equal(await page.evaluate(() => window.checks.length), 1);
    await page.clock.runFor(1);
    assert.equal(await page.evaluate(() => window.checks.length), 2);
    assert.equal(await page.evaluate(() => window.checks[1].values['general.folder']), '/home');
    // A check overtaken by a newer one applies nothing, even when it answers.
    await page.keyboard.type('s');
    await page.clock.runFor(500);
    assert.equal(await page.evaluate(() => window.checks[2]?.values['general.folder']), '/homes');
    await answer(page, 1, []);
    await page.clock.runFor(1000);
    assert.equal(await page.evaluate(() => window.settingsEvents.some(x => x.key === 'general.folder')), false, 'the overtaken check applied nothing');
    await answer(page, 2, []);
    await until(page, () => window.settingsEvents.at(-1)?.key === 'general.folder');
    const e = await lastEvent(page);
    assert.deepEqual([e.value, e.checked], ['/homes', true]);
    assert.equal(await folder.getAttribute('aria-invalid'), null);
    // Passed: typing again checks on leaving the field, not as the person types.
    await page.keyboard.type('/x');
    await page.clock.runFor(2000);
    assert.equal(await page.evaluate(() => window.checks.length), 3);
    // A check that fails to run says so on the field.
    await page.evaluate(() => { document.getElementById('demo-settings').check = () => Promise.reject(new Error('binding gone')); });
    await folder.press('Tab');
    await until(page, () => !document.querySelector('[data-key="general.folder"] .wk-error').hidden);
    assert.equal(await error.textContent(), 'Couldn\'t check your projects folder. Edit it to check again.');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('settings window: in a modal Sheet, the check\'s words and outcome reach the announcer inside it, in the accessibility tree; an overtaken check is cancelled, and one whose words show hands them on', async () => {
    const { page, problems } = await openSettings({ clock: true });
    await manualChecks(page);
    await page.evaluate(() => {
      const d = document.createElement('dialog');
      d.className = 'wk-dialog wk-dialog--sheet';
      d.id = 'settings-sheet';
      document.body.append(d);
      d.append(document.getElementById('demo-settings'));
      d.showModal();
      window.taskStates = [];
      document.addEventListener('wk-task', e => window.taskStates.push(e.detail.state));
    });
    await page.clock.runFor(10);
    assert.equal(await page.evaluate(() => document.getElementById('settings-sheet').contains(document.querySelector('[data-wk-announcer]'))), true);
    // Words in a live region of the accessibility tree (the field's own error shows the same words, outside one).
    const inTree = async words => {
      const nodes = await axTree(page);
      const byId = new Map(nodes.map(n => [n.nodeId, n]));
      const live = n => (n.properties || []).some(p => p.name === 'live');
      return nodes.some(n => {
        if (!String(n.name?.value ?? '').includes(words)) return false;
        for (let m = n; m; m = byId.get(m.parentId)) if (live(m)) return true;
        return false;
      });
    };
    const folder = row(page, 'general.folder').locator('input');
    // Overtaken in the quiet wait: cancelled, with nothing said and nothing shown.
    await folder.fill('/a');
    await folder.press('Tab');
    await page.clock.runFor(200);
    await folder.fill('/b');
    await folder.press('Tab');
    assert.deepEqual(await page.evaluate(() => window.taskStates.splice(0)), ['running', 'cancelled', 'running']);
    // Overtaken once its words show: the newer check keeps them; they don't go and come back.
    await page.clock.runFor(400);
    assert.equal(await pendingText(page, 'general.folder'), 'Checking folder…');
    await folder.fill('/c');
    await folder.press('Tab');
    assert.equal(await pendingText(page, 'general.folder'), 'Checking folder…');
    assert.deepEqual(await page.evaluate(() => window.taskStates.splice(0)), []);
    await answer(page, 1, []);
    await page.clock.runFor(1000);
    assert.equal(await pendingText(page, 'general.folder'), 'Checking folder…', 'the overtaken check\'s answer is ignored');
    await answer(page, 2, [{ field: 'general.folder', message: 'There is no folder at this path. Choose a folder that exists.', code: 'check' }]);
    await until(page, () => !document.querySelector('[data-key="general.folder"] .wk-error').hidden);
    assert.equal(await pendingText(page, 'general.folder'), '');
    assert.deepEqual(await page.evaluate(() => window.taskStates.splice(0)), ['done']);
    await page.clock.runFor(600);
    assert.equal(await inTree('There is no folder at this path.'), true, 'the outcome is not inert under the modal');
    assert.deepEqual(problems, []);
    await page.close();
  });

  // The folder field's pending line: [its words, the row's data-pending, the control described by them, task lines in it].
  const pendingState = page => page.evaluate(() => {
    const p = document.querySelector('#demo-settings [data-key="general.folder"] .wk-settings-pending');
    const control = p.closest('.wk-settings-row').querySelector('input');
    return [p.textContent, p.closest('.wk-settings-row').hasAttribute('data-pending'), (control.getAttribute('aria-describedby') || '').split(' ').includes(p.id), p.querySelectorAll('.wk-task-state').length];
  });
  const watchTasks = page => page.evaluate(() => { window.taskStates = []; document.addEventListener('wk-task', e => window.taskStates.push(e.detail.state)); });

  await check('settings window: checks committed while an answered one holds its words wait for it, so the words never stick; an overtaken check\'s answer is not announced', async () => {
    const { page, problems } = await openSettings({ clock: true });
    await manualChecks(page);
    await watchTasks(page);
    const folder = row(page, 'general.folder').locator('input');
    await folder.fill('/a');
    await folder.press('Tab');
    await page.clock.runFor(410);
    await answer(page, 0, []);
    await page.clock.runFor(10);
    await folder.fill('/b');
    await folder.press('Tab');
    await page.clock.runFor(10);
    await folder.fill('/c');
    await folder.press('Tab');
    await page.clock.runFor(600);
    assert.deepEqual(await said(page), ['Checking folder…'], '/a was overtaken in its hold: its answer is not said');
    await page.clock.runFor(400);
    assert.deepEqual(await pendingState(page), ['Checking folder…', true, true, 1], '/c shows its own words once /a\'s have gone');
    await answer(page, 2, []);
    await page.clock.runFor(1500);
    assert.deepEqual(await pendingState(page), ['', false, false, 0], 'nothing left beside the field');
    assert.equal(await page.evaluate(() => document.getElementById('demo-settings').values['general.folder']), '/c');
    assert.deepEqual(await said(page, { clock: false }).then(w => w.filter(x => x !== 'Checking folder…')), ['Projects folder checked']);
    assert.deepEqual(await page.evaluate(() => window.taskStates), ['running', 'done', 'running', 'done']);
    // /b's answer, overtaken before it came, changes nothing.
    await answer(page, 1, []);
    await page.clock.runFor(1000);
    assert.deepEqual(await pendingState(page), ['', false, false, 0]);
    assert.deepEqual(await said(page), []);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('settings window: a check abandoned for the window\'s own error, or by a new schema, is cancelled without saying "Cancelled"; checks stay out of the task list', async () => {
    const { page, problems } = await openSettings({ clock: true });
    await manualChecks(page);
    await watchTasks(page);
    await page.evaluate(() => { document.getElementById('demo-settings').validate = (s, v) => (String(v['general.folder']).length > 6 ? [{ field: 'general.folder', message: 'Keep it short.', code: 'max_length' }] : []); });
    const folder = row(page, 'general.folder').locator('input');
    await folder.fill('/abc');
    await folder.press('Tab');
    await page.clock.runFor(600);
    assert.equal(await page.evaluate(() => document.wkTask.showList().querySelectorAll('.wk-task-row').length), 0, 'not in the task list');
    await page.evaluate(() => document.querySelector('.wk-task-list').close());
    await said(page);
    await folder.fill('/abcdefghij');
    await folder.press('Tab');
    await page.clock.runFor(600);
    assert.deepEqual(await pendingState(page), ['', false, false, 0]);
    assert.equal(await row(page, 'general.folder').locator('.wk-error').textContent(), 'Keep it short.');
    assert.deepEqual(await said(page), ['1 setting needs attention.'], 'the Notice\'s count, and no "Cancelled"');
    assert.deepEqual(await page.evaluate(() => window.taskStates.splice(0)), ['running', 'cancelled']);
    // A new schema (or icons) while a check shows its words: cancelled, quietly.
    await folder.fill('/x');
    await folder.press('Tab');
    await page.clock.runFor(600);
    await said(page);
    await page.evaluate(() => { const el = document.getElementById('demo-settings'); el.icons = { ...el.icons }; });
    await page.clock.runFor(600);
    assert.deepEqual(await page.evaluate(() => window.taskStates.splice(0)), ['running', 'cancelled']);
    assert.deepEqual(await said(page), []);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('settings window: an empty required setting shows its error on open, and the window\'s Notice counts what needs attention', async () => {
    const { page, problems } = await openSettings({ clock: true });
    assert.equal(await page.locator('#demo-settings .wk-settings-status').textContent(), '', 'nothing needs attention at first');
    assert.equal(await page.locator('#demo-settings .wk-settings-status').isVisible(), false);
    const look = await page.evaluate(async () => {
      const { schema } = await import('./settings-demo.js');
      const el = document.createElement('wk-settings');
      el.id = 'fresh';
      el.values = { 'general.name': '', 'provider.name': 'alpha', 'provider.key': '' };
      el.schema = schema;
      document.querySelector('.demo-settings-window').replaceChildren(el);
      const status = el.querySelector('.wk-settings-status');
      return {
        error: el.querySelector('[data-key="general.name"] .wk-error').textContent,
        invalid: el.querySelector('[data-key="general.name"] input').getAttribute('aria-invalid'),
        tabs: [...el.querySelectorAll('[role=tab]')].map(t => t.hasAttribute('data-attention')),
        role: status.getAttribute('role'),
        notice: status.textContent,
        tone: status.querySelector('.wk-notice').dataset.tone,
        place: [status.previousElementSibling.getAttribute('role'), status.nextElementSibling.getAttribute('role')],
      };
    });
    // The Notice is no live region of its own: its count goes to the window's announcer, not when the window is drawn.
    assert.deepEqual(look, { error: 'Display name is required', invalid: 'true', tabs: [true, false], role: null, notice: '1 setting needs attention.', tone: 'attention', place: ['tablist', 'tabpanel'] });
    assert.deepEqual(await said(page), []);
    const status = page.locator('#fresh .wk-settings-status');
    assert.equal(await status.isVisible(), true);
    await page.evaluate(() => { document.getElementById('fresh').errors = [{ field: 'general.copies', message: 'Keep at most 8 copies', code: 'max' }]; });
    assert.equal(await status.textContent(), '2 settings need attention.');
    assert.deepEqual(await said(page), ['2 settings need attention.']);
    // While the person types, the field's error clears at once, but the
    // Notice waits until typing stops, and changes (announces) only when the count does.
    await page.evaluate(() => {
      window.noticeChanges = 0;
      new MutationObserver(m => { window.noticeChanges += m.length; }).observe(document.querySelector('#fresh .wk-settings-status'), { subtree: true, childList: true, characterData: true });
    });
    const name = page.locator('#fresh [data-key="general.name"] input');
    await name.focus();
    await page.keyboard.type('Ana');
    assert.equal(await name.getAttribute('aria-invalid'), null);
    assert.equal(await status.textContent(), '2 settings need attention.', 'not on each keystroke');
    assert.equal(await page.evaluate(() => window.noticeChanges), 0);
    await page.clock.runFor(499);
    assert.equal(await status.textContent(), '2 settings need attention.');
    await page.clock.runFor(1);
    assert.equal(await status.textContent(), '1 setting needs attention.', 'the count follows once typing stops');
    const changes = await page.evaluate(() => window.noticeChanges);
    assert.ok(changes > 0);
    await page.keyboard.type('x');
    await page.keyboard.press('Backspace');
    await page.clock.runFor(500);
    assert.equal(await page.evaluate(() => window.noticeChanges), changes, 'an unchanged count leaves the region alone');
    assert.deepEqual(await said(page), ['1 setting needs attention.'], 'announced once, when the count changed');
    await page.evaluate(() => { document.getElementById('fresh').errors = []; });
    assert.equal(await status.textContent(), '');
    assert.equal(await status.isVisible(), false);
    // One group: the Notice sits under the heading.
    const single = await page.evaluate(() => {
      const el = document.getElementById('fresh');
      el.schema = { groups: [{ key: 'only', label: 'Only', fields: [{ key: 'x', type: 'text', label: 'X', validation: { required: true } }] }] };
      const status = el.querySelector('.wk-settings-status');
      return [status.previousElementSibling.className, status.textContent];
    });
    assert.deepEqual(single, ['wk-settings-heading', '1 setting needs attention.']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('settings window: reveal(key) opens on a field (its tab, the Advanced section) and focuses it', async () => {
    const { page, problems } = await openSettings();
    const r = await page.evaluate(() => {
      const el = document.getElementById('demo-settings');
      el.errors = [{ field: 'provider.endpoint', message: 'The endpoint must be https', code: 'check' }];
      el.expand('provider', false);
      return [el.reveal('provider.endpoint'), el.reveal('provider.port'), el.reveal('nope')];
    });
    assert.deepEqual(r, [true, false, false], 'a hidden or unknown field can\'t be revealed');
    assert.equal(await page.locator('#demo-settings [role=tab]', { hasText: 'Provider' }).getAttribute('aria-selected'), 'true');
    assert.equal(await page.locator('#demo-settings .wk-settings-advanced').getAttribute('aria-expanded'), 'true');
    assert.equal(await page.evaluate(() => document.activeElement.closest('.wk-settings-row')?.dataset.key), 'provider.endpoint');
    assert.equal(await row(page, 'provider.endpoint').locator('.wk-error').textContent(), 'The endpoint must be https');
    assert.deepEqual(problems, []);
    await page.close();
  });


  await check('settings window: a value passes its check and applies while another required field is empty; a reply that didn\'t save it doesn\'t apply it', async () => {
    const { page, problems } = await openSettings({ clock: true });
    const name = row(page, 'general.name').locator('input');
    await name.fill('');
    await name.press('Tab');
    assert.equal(await row(page, 'general.name').locator('.wk-error').textContent(), 'Display name is required');
    // The demo's stand-in saves the one value, like CheckValue: no errors.
    const folder = row(page, 'general.folder').locator('input');
    await folder.fill('/work');
    await folder.press('Tab');
    await page.clock.runFor(600);
    await page.clock.runFor(400);
    await until(page, () => window.settingsEvents.at(-1)?.key === 'general.folder');
    const e = await lastEvent(page);
    assert.deepEqual([e.value, e.checked], ['/work', true]);
    assert.equal(await row(page, 'general.name').locator('.wk-error').textContent(), 'Display name is required', 'the other field keeps its error');
    // A reply with errors for other fields says the value wasn't saved: it doesn't apply.
    await manualChecks(page);
    const before = await eventCount(page);
    await folder.fill('/other');
    await folder.press('Tab');
    await answer(page, 0, [{ field: 'general.name', message: 'Display name is required', code: 'required' }]);
    await until(page, () => !document.querySelector('[data-key="general.folder"] .wk-error').hidden);
    assert.equal(await row(page, 'general.folder').locator('.wk-error').textContent(), 'Your projects folder wasn\'t saved. Fix the other settings, then enter it again.');
    assert.equal(await eventCount(page), before);
    assert.equal(await page.evaluate(() => document.getElementById('demo-settings').values['general.folder']), '/work');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('settings window: a check failure the app passes in stays when another field changes, until its own field commits', async () => {
    const { page, problems } = await openSettings({ clock: true });
    await manualChecks(page);
    // As when Settings reopens on CheckFailures(): an error, no draft.
    await page.evaluate(() => { document.getElementById('demo-settings').errors = [{ field: 'general.folder', message: 'There is no folder at this path. Choose a folder that exists.', code: 'check' }]; });
    const error = row(page, 'general.folder').locator('.wk-error');
    await row(page, 'general.launch').locator('label').click();
    assert.equal((await lastEvent(page)).key, 'general.launch');
    assert.equal(await error.textContent(), 'There is no folder at this path. Choose a folder that exists.', 'the app\'s reply to the other change keeps it');
    await page.evaluate(() => { document.getElementById('demo-settings').validate = () => []; });
    await row(page, 'general.copies').locator('button[aria-label=More]').click();
    assert.equal(await error.isVisible(), true, 'and so does validate on another change');
    const folder = row(page, 'general.folder').locator('input');
    await folder.fill('/fixed');
    await folder.press('Tab');
    await answer(page, 0, []);
    await until(page, () => window.settingsEvents.at(-1)?.key === 'general.folder');
    assert.equal(await error.isVisible(), false);
    assert.deepEqual(problems, []);
    await page.close();
  });


  await check('settings window: a value the window\'s own validation rejects still goes to the back end, so an older check can\'t save the value it replaced', async () => {
    const { page, problems } = await openSettings({ clock: true });
    await manualChecks(page);
    await page.evaluate(() => { document.getElementById('demo-settings').validate = (schema, values) => String(values['general.folder'] ?? '').length > 12 ? [{ field: 'general.folder', message: 'Use 12 characters or fewer', code: 'max_length' }] : []; });
    const folder = row(page, 'general.folder').locator('input');
    const error = row(page, 'general.folder').locator('.wk-error');
    await folder.fill('/good');
    await folder.press('Tab');
    // While /good is being checked, the person commits a value the window rejects itself.
    await folder.fill('/a-very-long-path');
    await folder.press('Tab');
    assert.equal(await error.textContent(), 'Use 12 characters or fewer');
    await until(page, () => window.checks.length === 2);
    assert.equal(await page.evaluate(() => window.checks[1].values['general.folder']), '/a-very-long-path', 'the back end hears of the new value, and overtakes its check of /good');
    // The window ignores both replies: /good was abandoned, and the new value's errors are shown already.
    await answer(page, 0, []);
    await answer(page, 1, [{ field: 'general.folder', message: 'General folder must be at most 12 characters', code: 'max_length' }]);
    await page.clock.runFor(1000);
    assert.equal(await page.evaluate(() => window.settingsEvents.some(e => e.key === 'general.folder')), false);
    assert.equal(await error.textContent(), 'Use 12 characters or fewer');
    // A live re-check of an invalid draft goes to the back end too, once typing stops.
    await folder.focus();
    await page.keyboard.press('End');
    await page.keyboard.type('xyz');
    await page.clock.runFor(499);
    assert.equal(await page.evaluate(() => window.checks.length), 2);
    await page.clock.runFor(1);
    assert.equal(await page.evaluate(() => window.checks.length), 3);
    assert.equal(await page.evaluate(() => window.checks[2].values['general.folder']), '/a-very-long-pathxyz');
    assert.equal(await error.textContent(), 'Use 12 characters or fewer');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('settings window: a check failure the window kept goes when the app sets a new value for that field', async () => {
    const { page, problems } = await openSettings();
    const error = row(page, 'general.folder').locator('.wk-error');
    const look = await page.evaluate(() => {
      const el = document.getElementById('demo-settings');
      const out = [];
      el.errors = [{ field: 'general.folder', message: 'There is no folder at this path. Choose a folder that exists.', code: 'check' }];
      el.errors = []; // an empty CheckFailures() keeps it: it is about the field, not the app's last reply
      out.push(el.errors.length);
      el.values = { ...el.values }; // the same value keeps it
      out.push(el.errors.length);
      el.values = { ...el.values, 'general.folder': '/saved/elsewhere' };
      out.push(el.errors.length);
      return out;
    });
    assert.deepEqual(look, [1, 1, 0]);
    assert.equal(await error.isVisible(), false);
    assert.equal(await row(page, 'general.folder').locator('input').inputValue(), '/saved/elsewhere');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('settings window: in forced colours, the pending words and their dot, and the Notice\'s outline and words, take system colours', async () => {
    const { page, problems } = await openSettings({ forcedColors: 'active', clock: true });
    await manualChecks(page);
    const folder = row(page, 'general.folder').locator('input');
    await folder.fill('/f');
    await folder.press('Tab');
    await page.clock.runFor(400);
    await page.evaluate(() => { document.getElementById('demo-settings').errors = [{ field: 'general.copies', message: 'x', code: 'max' }]; });
    const look = await page.evaluate(() => {
      const probe = c => { const e = document.createElement('i'); e.style.forcedColorAdjust = 'none'; e.style.color = c; document.body.append(e); const v = getComputedStyle(e).color; e.remove(); return v; };
      const dot = getComputedStyle(document.querySelector('[data-key="general.folder"] .wk-settings-pending .wk-dot'));
      const words = getComputedStyle(document.querySelector('[data-key="general.folder"] .wk-settings-pending'));
      const notice = getComputedStyle(document.querySelector('#demo-settings .wk-settings-status .wk-notice'));
      const bold = getComputedStyle(document.querySelector('#demo-settings .wk-settings-status b'));
      return {
        canvasText: probe('CanvasText'), canvas: probe('Canvas'),
        dot: [dot.borderTopStyle, dot.borderTopColor, dot.outlineStyle, dot.outlineColor],
        words: words.color,
        // The system keeps a background's alpha: compare its colour channels.
        notice: [notice.borderTopStyle, notice.borderTopWidth !== '0px', notice.borderTopColor, notice.backgroundColor.match(/\d+/g).slice(0, 3).join()],
        bold: bold.color,
      };
    });
    const { canvasText } = look;
    const canvas = look.canvas.match(/\d+/g).slice(0, 3).join();
    assert.notEqual(canvasText, look.canvas);
    assert.deepEqual(look.dot, ['solid', canvasText, 'solid', canvasText], 'the working dot is drawn in CanvasText, with its ring as an outline');
    assert.equal(look.words, canvasText, 'the pending words read as text');
    assert.deepEqual(look.notice, ['solid', true, canvasText, canvas], 'the Notice keeps a CanvasText outline on Canvas');
    assert.equal(look.bold, canvasText);
    assert.deepEqual(problems, []);
    await page.close();
  });
  // ---- wkTask, the announcer and the task list (BEHAVIOUR.md) ------------
  // These checks run on Playwright's fake clock: the page's timers, Date and
  // performance.now move only when the check says so (run(ms)).
  const T0 = Date.parse('2026-01-05T10:00:00Z');
  async function openTasks(url = '/demo/index.html') {
    const page = await browser.newPage({ viewport: { width: 1100, height: 800 }, colorScheme: 'light' });
    const problems = [];
    page.on('pageerror', e => problems.push(`pageerror: ${e.message}`));
    page.on('console', m => { if (m.type() === 'error' || m.type() === 'warning') problems.push(`console ${m.type()}: ${m.text()}`); });
    page.on('request', r => { if (!r.url().startsWith(origin + '/') && !r.url().startsWith('data:')) problems.push(`network: ${r.url()}`); });
    await page.clock.install({ time: T0 });
    await page.goto(origin + url);
    await page.waitForFunction(() => customElements.get('wk-dialog') && document.querySelector('#demo-values dt'));
    await page.clock.pauseAt(T0 + 60_000);
    await page.evaluate(async () => {
      window.kit = await import('/src/index.js');
      // Everything the live regions say, in order: [politeness, words].
      window.said = [];
      const host = document.querySelector('[data-wk-announcer]');
      const last = new Map();
      new MutationObserver(() => {
        for (const r of document.querySelectorAll('[data-wk-announcer] [aria-live]')) {
          const t = r.textContent;
          if (t && t !== last.get(r)) window.said.push([r.getAttribute('aria-live'), t]);
          last.set(r, t);
        }
      }).observe(host.parentNode, { subtree: true, childList: true, characterData: true });
      window.taskEvents = [];
      document.addEventListener('wk-task', e => window.taskEvents.push(`${e.detail.id}:${e.detail.state}`));
      const box = document.createElement('div');
      box.id = 'tbox';
      document.querySelector('main').prepend(box);
    });
    const run = ms => page.clock.runFor(ms);
    const said = () => page.evaluate(() => window.said.splice(0));
    const taskEvents = () => page.evaluate(() => window.taskEvents.splice(0));
    return { page, problems, run, said, taskEvents };
  }

  await check('announcer: two regions in the page from load; messages within 500 ms are joined; the same words again are heard again; one per window', async () => {
    const { page, problems, run, said } = await openTasks();
    const regions = await page.evaluate(() => [...document.querySelectorAll('[data-wk-announcer] [aria-live]')].map(r => [r.getAttribute('aria-live'), r.getAttribute('aria-atomic'), r.textContent]));
    assert.deepEqual(regions, [['polite', 'true', ''], ['assertive', 'true', '']]);
    assert.equal(await page.evaluate(() => { const r = document.querySelector('[data-wk-announcer]').getBoundingClientRect(); return r.width <= 1 && r.height <= 1; }), true, 'visually hidden');
    await page.evaluate(() => kit.announce('Link copied'));
    await run(300);
    await page.evaluate(() => { kit.announce('Comment posted'); kit.announce('Comment posted'); kit.announce('Couldn’t save', { assertive: true }); });
    await run(499);
    assert.deepEqual(await said(), [], 'nothing until 500 ms after the last');
    await run(1);
    assert.deepEqual(await said(), [['polite', 'Link copied. Comment posted'], ['assertive', 'Couldn’t save']]);
    // Apart: one each, and the same words twice are said twice.
    await page.evaluate(() => kit.announce('Saved'));
    await run(600);
    await page.evaluate(() => kit.announce('Saved'));
    await run(600);
    assert.deepEqual(await said(), [['polite', 'Saved'], ['polite', 'Saved']]);
    // A stream of messages still goes out within 2 s of the first.
    await page.evaluate(() => { window.n = 0; window.iv = setInterval(() => kit.announce(`m${++window.n}`), 300); });
    await run(2300);
    await page.evaluate(() => clearInterval(window.iv));
    assert.deepEqual(await said(), [['polite', 'm1. m2. m3. m4. m5. m6. m7']]);
    await run(1000);
    await said();
    // A second copy of the kit (dist/) shares the regions; regions the app
    // removed come back.
    const count = await page.evaluate(async () => {
      const dist = await import('/dist/wails-kit-ui.js');
      dist.announce('From dist');
      kit.announce('From src');
      return document.querySelectorAll('[data-wk-announcer]').length;
    });
    assert.equal(count, 1);
    await run(600);
    assert.deepEqual(await said(), [['polite', 'From dist. From src']]);
    await page.evaluate(() => { document.querySelector('[data-wk-announcer]').remove(); kit.announce('Back'); });
    await run(600);
    assert.deepEqual(await page.evaluate(() => [document.querySelectorAll('[data-wk-announcer]').length, document.querySelector('[data-wk-announcer] [aria-live="polite"]').textContent]), [1, 'Back']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('wkTask: listed false keeps a task out of the list; cancelled({ quiet }) and done({ quiet }) say nothing; dismiss() takes a failed task out of the list', async () => {
    const { page, problems, run, said } = await openTasks();
    await page.evaluate(() => {
      window.hidden = kit.wkTask({ words: 'Checking', listed: false });
      window.shown = kit.wkTask({ words: 'Exporting' });
      window.summary = kit.wkTask.summary();
      document.getElementById('tbox').append(window.summary);
    });
    await run(500);
    assert.equal(await page.evaluate(() => window.summary.textContent), '1 task running');
    await page.evaluate(() => { hidden.cancelled({ quiet: true }); shown.done({ words: 'Exported', quiet: true }); });
    await run(1000);
    assert.deepEqual(await said(), [['polite', 'Checking. Exporting']], 'the starts only: neither end is announced');
    await page.evaluate(() => { window.f = kit.wkTask({ words: 'Saving' }); f.fail('Couldn’t save'); });
    await run(1000);
    assert.equal(await page.evaluate(() => window.summary.textContent), '1 failed');
    await page.evaluate(() => f.dismiss());
    await run(10);
    assert.equal(await page.evaluate(() => window.summary.hidden || window.summary.textContent === ''), true, 'the failure is gone from the list');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('wkTask: no indicator for work under 400 ms; busy at once; a locked control stays locked; done always announces', async () => {
    const { page, problems, run, said, taskEvents } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<button class="wk-button" type="button" id="t-go">Export</button><button class="wk-button wk-button--locked" type="button" id="t-locked" aria-disabled="true">Invite</button>';
      window.t = kit.wkTask({ id: 'q', control: [document.getElementById('t-go'), document.getElementById('t-locked')], words: 'Exporting' });
    });
    const look = () => page.evaluate(() => ['t-go', 't-locked'].map(id => { const b = document.getElementById(id); return [b.getAttribute('aria-disabled'), b.hasAttribute('data-busy'), Boolean(b.querySelector('.wk-spinner'))]; }));
    assert.deepEqual(await look(), [['true', true, false], ['true', true, false]], 'busy at once, no spinner');
    await run(399);
    assert.deepEqual((await look())[0], ['true', true, false], 'no spinner in the quiet wait');
    await page.evaluate(() => { t.done('Report exported'); });
    assert.deepEqual(await look(), [[null, false, false], ['true', false, false]], 'back to rest; the locked control keeps aria-disabled');
    await run(1000);
    assert.deepEqual(await said(), [['polite', 'Report exported']], 'no start announcement, the outcome once');
    assert.deepEqual(await taskEvents(), ['q:running', 'q:done']);
    assert.equal(await page.evaluate(() => { try { kit.wkTask({ words: 'x' }).done({}); } catch (e) { return e instanceof TypeError; } return false; }), true, 'done() without words throws');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('wkTask: the indicator shows at 400 ms and stays at least 500 ms; the result swaps in that step; focus never moves', async () => {
    const { page, problems, run, said } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<button class="wk-button" type="button" id="t-go">Export</button>';
      document.getElementById('t-go').focus();
      window.t = kit.wkTask({ control: document.getElementById('t-go'), words: 'Exporting' });
    });
    const spinner = () => page.evaluate(() => Boolean(document.querySelector('#t-go > .wk-spinner[aria-hidden="true"]')));
    await run(399);
    assert.equal(await spinner(), false);
    await run(1);
    assert.equal(await spinner(), true, 'the spinner at 400 ms');
    await run(100);
    // Done 100 ms after it showed: held until 500 ms after.
    await page.evaluate(() => { window.drawn = null; t.done('Exported').then(d => { window.drawn = [Boolean(document.querySelector('#t-go > .wk-spinner')), d.state]; }); });
    await run(399);
    assert.equal(await spinner(), true, 'held');
    assert.equal(await page.evaluate(() => window.drawn), null);
    await run(1);
    assert.deepEqual(await page.evaluate(() => window.drawn), [false, 'done'], 'the promise resolves as the spinner goes');
    assert.equal(await active(page), 't-go', 'focus stays on the control');
    await run(1000);
    assert.deepEqual(await said(), [['polite', 'Exporting'], ['polite', 'Exported']], 'the start when the indicator appears, then the end');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('wkTask: Stop shows at 10 s in space kept for it, with no layout shift; Waiting at 30 s; the timeout fails it and aborts its signal', async () => {
    const { page, problems, run, said, taskEvents } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<div id="t-row"><button class="wk-button" type="button" id="t-go">Sync</button><span id="t-after">after</span></div><div class="wk-progress" role="progressbar" aria-label="Syncing" id="t-bar"></div>';
      window.t = kit.wkTask({ id: 'long', control: document.getElementById('t-go'), bar: document.getElementById('t-bar'), words: 'Syncing', expect: 'maybe-long', stop: 'cancel', waitingWords: 'Waiting for the server…' });
    });
    const where = () => page.evaluate(() => {
      const stop = document.querySelector('.wk-task-stop');
      return { after: document.getElementById('t-after').getBoundingClientRect().left, go: document.getElementById('t-go').getBoundingClientRect().left, stop: stop && getComputedStyle(stop).visibility, label: stop?.textContent, bar: document.getElementById('t-bar').hidden };
    });
    const start = await where();
    assert.equal(start.stop, 'hidden', 'the space is kept from the start');
    assert.equal(start.bar, true, 'the bar waits for the indicator');
    await run(400);
    assert.equal((await where()).bar, false, 'an indeterminate bar at 400 ms');
    assert.equal(await page.evaluate(() => document.getElementById('t-bar').hasAttribute('aria-valuenow')), false);
    await page.evaluate(() => t.progress({ done: 3, total: 10, unit: 'files' }));
    await run(9600 - 1);
    assert.equal((await where()).stop, 'hidden');
    await run(1);
    const at10 = await where();
    assert.deepEqual([at10.stop, at10.label, at10.after, at10.go], ['visible', 'Cancel', start.after, start.go], 'Cancel at 10 s; nothing moved');
    // Waiting: 30 s after the last report (at 400 ms).
    await run(30_400 - 10_000 - 1);
    assert.equal(await page.evaluate(() => t.state), 'running');
    await run(1);
    assert.equal(await page.evaluate(() => t.state), 'waiting');
    // Progress brings it back, with its words.
    await page.evaluate(() => t.progress({ done: 4, total: 10, unit: 'files' }));
    assert.equal(await page.evaluate(() => t.state), 'running');
    await run(120_000 - 1);
    assert.equal(await page.evaluate(() => [t.state, t.signal.aborted].join()), 'waiting,false');
    await run(1);
    assert.equal(await page.evaluate(() => [t.state, t.signal.aborted, t.signal.reason?.name].join()), 'failed,true,TimeoutError');
    const look = await page.evaluate(() => ({ stop: document.querySelectorAll('.wk-task-stop').length, busy: document.getElementById('t-go').hasAttribute('data-busy'), bar: document.getElementById('t-bar').hidden }));
    assert.deepEqual(look, { stop: 0, busy: false, bar: true });
    await run(1000);
    assert.deepEqual(await said(), [['polite', 'Syncing'], ['polite', 'Waiting for the server…'], ['polite', 'Waiting for the server…'], ['polite', 'Timed out after 2 min']]);
    assert.deepEqual(await taskEvents(), ['long:running', 'long:waiting', 'long:running', 'long:waiting', 'long:failed']);
    assert.deepEqual(problems, []);
    await page.close();
  });
  await check('wkTask: a target row shows a working dot and words in place of its state; Cancel puts it back; Stop keeps what finished; onStop false keeps it running', async () => {
    const { page, problems, run, said } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<table class="wk-table"><tbody>'
        + ['a', 'b', 'c'].map(k => `<tr id="row-${k}"><th>${k}</th><td id="cell-${k}"><span class="wk-dot" aria-hidden="true"></span>Ready</td></tr>`).join('') + '</tbody></table>';
      window.asked = 0;
      window.ta = kit.wkTask({ id: 'a', target: document.getElementById('cell-a'), words: 'Copying', expect: 'long', stop: 'cancel' });
      window.tb = kit.wkTask({ id: 'b', target: document.getElementById('cell-b'), words: 'Copying', expect: 'long', stop: 'stop' });
      window.tc = kit.wkTask({ id: 'c', target: document.getElementById('cell-c'), words: 'Copying', expect: 'long', stop: 'stop-asks', onStop: () => { window.asked++; return false; } });
    });
    const cell = k => page.evaluate(id => {
      const c = document.getElementById(`cell-${id}`), stop = c.querySelector('.wk-task-stop');
      return { text: c.textContent, dot: c.querySelector('.wk-dot') ? c.querySelector('.wk-dot').dataset.tone ?? 'neutral' : null, stop: stop && [stop.textContent, getComputedStyle(stop).visibility] };
    }, k);
    assert.deepEqual(await cell('a'), { text: 'ReadyCancel', dot: 'neutral', stop: ['Cancel', 'visible'] }, 'expected long: Cancel at once; the row keeps its state through the quiet wait');
    await run(400);
    assert.deepEqual(await cell('a'), { text: 'CopyingCancel', dot: 'working', stop: ['Cancel', 'visible'] });
    await page.evaluate(() => { tb.progress({ done: 12, total: 40, unit: 'files' }); tc.progress({ done: 1, total: 2 }); });
    assert.deepEqual(await cell('b'), { text: 'Copying · 12 of 40 filesStop', dot: 'working', stop: ['Stop', 'visible'] });
    assert.deepEqual(await cell('c'), { text: 'Copying · 1 of 2Stop…', dot: 'working', stop: ['Stop…', 'visible'] });
    for (const k of ['a', 'b', 'c']) await page.$eval(`#cell-${k} .wk-task-stop`, b => b.click());
    await run(500);
    assert.deepEqual(await cell('a'), { text: 'Ready', dot: 'neutral', stop: null }, 'Cancel: the state from before');
    assert.deepEqual(await cell('b'), { text: 'Stopped · 12 of 40 files', dot: null, stop: null }, 'Stop: what finished stays');
    assert.equal(await page.evaluate(() => [window.asked, tc.state].join()), '1,running', 'a Stop… the person declined');
    assert.equal(await page.evaluate(() => [ta.signal.aborted, tb.signal.aborted, tc.signal.aborted].join()), 'true,true,false');
    // A failure in a row: the attention dot and the words, in place.
    await page.evaluate(() => { tc.fail({ words: 'Couldn’t copy', error: new Error('EACCES') }); });
    assert.deepEqual(await cell('c'), { text: 'Couldn’t copy', dot: 'attention', stop: null });
    await run(1000);
    assert.deepEqual(await said(), [['polite', 'Copying'], ['polite', 'Cancelled. Stopped · 12 of 40 files. Couldn’t copy']]);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('wkTask: progress never ticks in a live region; a change of stage is announced once and sets the bar\'s value at once, as waiting and pause do', async () => {
    const { page, problems, run, said } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<button class="wk-button" type="button" id="t-go">Upload</button><div class="wk-progress" role="progressbar" aria-label="Uploading" id="t-bar"></div>';
      window.t = kit.wkTask({ control: document.getElementById('t-go'), bar: document.getElementById('t-bar'), words: 'Uploading', expect: 'maybe-long', stop: 'pause', onPause: () => {}, onResume: () => {} });
    });
    const value = () => page.evaluate(() => document.getElementById('t-bar').getAttribute('aria-valuetext'));
    await run(400);
    await page.evaluate(() => t.progress({ done: 1, total: 40, unit: 'files', step: 1, steps: 2, stage: 'Copying' }));
    assert.equal(await value(), 'Step 1 of 2 · Copying · 1 of 40 files');
    for (let i = 2; i <= 6; i++) { await run(500); await page.evaluate(n => t.progress({ done: n, total: 40, unit: 'files', step: 1, steps: 2, stage: 'Copying' }), i); }
    assert.equal(await value(), 'Step 1 of 2 · Copying · 1 of 40 files', 'at most every 5 s');
    // A new stage: the value and the words at once, and announced once.
    await run(100);
    await page.evaluate(() => t.progress({ done: 7, total: 40, unit: 'files', step: 2, steps: 2, stage: 'Checking' }));
    assert.equal(await value(), 'Step 2 of 2 · Checking · 7 of 40 files');
    await page.evaluate(() => t.progress({ done: 8, total: 40, unit: 'files', step: 2, steps: 2, stage: 'Checking' }));
    // Pause: the value catches up with the fill at once; the button reads Resume.
    await run(100);
    await page.evaluate(() => t.progress({ done: 9, total: 40, unit: 'files', step: 2, steps: 2, stage: 'Checking' }));
    assert.equal(await value(), 'Step 2 of 2 · Checking · 7 of 40 files');
    await run(10_000);
    await page.$eval('.wk-task-stop', b => b.click());
    assert.equal(await page.evaluate(() => [t.state, document.querySelector('.wk-task-stop').textContent].join()), 'paused,Resume');
    assert.equal(await value(), 'Step 2 of 2 · Checking · 9 of 40 files');
    // Paused: no timeout.
    await run(300_000);
    assert.equal(await page.evaluate(() => t.state), 'paused');
    await page.$eval('.wk-task-stop', b => b.click());
    assert.equal(await page.evaluate(() => [t.state, document.querySelector('.wk-task-stop').textContent].join()), 'running,Pause');
    await page.evaluate(() => t.progress({ done: 10, total: 40, unit: 'files', step: 2, steps: 2, stage: 'Checking' }));
    await run(600);
    // App-reported waiting with a named wait sets the value at once too.
    await page.evaluate(() => t.waiting('Waiting for GitHub…', { timeout: false }));
    assert.equal(await value(), 'Step 2 of 2 · Checking · 10 of 40 files');
    await run(300_000);
    assert.equal(await page.evaluate(() => t.state), 'waiting', 'no timeout while it waits on something named');
    await page.evaluate(() => { t.done('Uploaded'); });
    await run(1000);
    const all = await said();
    assert.deepEqual(all, [['polite', 'Uploading. Step 1 of 2 · Copying'], ['polite', 'Step 2 of 2 · Checking'], ['polite', 'Paused · Step 2 of 2 · Checking'], ['polite', 'Uploading · Step 2 of 2 · Checking'], ['polite', 'Waiting for GitHub… · Step 2 of 2 · Checking'], ['polite', 'Uploaded']]);
    assert.ok(all.every(([, w]) => !/\d+ of 40/.test(w)), 'no count in a live region');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('wkTask: a failure is assertive only while focus is still in scope; focus never moves on done or fail; the summary for parts', async () => {
    const { page, problems, run, said, taskEvents } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<button class="wk-button" type="button" id="t-go">Save</button><input class="wk-field" id="t-else"><ul id="t-parts">'
        + ['r1', 'r2', 'r3'].map(k => `<li><span id="p-${k}">Idle</span></li>`).join('') + '</ul><button class="wk-button" type="button" id="t-review">Run reviewers</button>';
      document.getElementById('t-go').focus();
      window.t1 = kit.wkTask({ control: document.getElementById('t-go'), words: 'Saving' });
      window.t2 = kit.wkTask({ control: document.getElementById('t-go'), words: 'Saving' });
    });
    await run(400);
    // Still on the control: assertive. Moved away: polite.
    await page.evaluate(() => { t1.fail({ words: 'Couldn’t save the file' }); });
    await run(500);
    await page.focus('#t-else');
    await page.evaluate(() => { t2.fail({ words: 'Couldn’t save the copy' }); });
    await run(1000);
    assert.equal(await active(page), 't-else', 'focus stays where the person put it');
    const s = await said();
    assert.deepEqual(s.filter(([k]) => k === 'assertive'), [['assertive', 'Couldn’t save the file']]);
    assert.deepEqual(s.filter(([k]) => k === 'polite').map(x => x[1]).join(' / '), 'Saving / Couldn’t save the copy');
    assert.equal(await page.evaluate(() => document.getElementById('t-go').hasAttribute('data-busy')), false, 'both returned the control to rest');
    await taskEvents();
    // Parts: one task; each part shows its own state; one summary at the end.
    await page.focus('#t-review');
    await page.evaluate(() => {
      window.review = kit.wkTask({ id: 'review', control: document.getElementById('t-review'), words: 'Running reviewers', unit: 'reviewers' });
      window.parts = ['r1', 'r2', 'r3'].map(k => review.part({ target: document.getElementById(`p-${k}`), words: 'Reviewing' }));
    });
    await run(400);
    assert.deepEqual(await page.evaluate(() => ['r1', 'r2', 'r3'].map(k => document.getElementById(`p-${k}`).textContent)), ['Reviewing', 'Reviewing', 'Reviewing']);
    await page.evaluate(() => { parts[0].done('Passed'); parts[1].fail('Couldn’t run'); });
    await run(1000);
    assert.equal(await page.evaluate(() => review.state), 'running');
    assert.deepEqual(await page.evaluate(() => ['r1', 'r2'].map(k => document.getElementById(`p-${k}`).textContent)), ['Idle', 'Couldn’t run']);
    await page.evaluate(() => { parts[2].done('Passed'); });
    await run(1000);
    assert.equal(await page.evaluate(() => review.state), 'failed');
    const words = await said();
    assert.deepEqual(words, [['polite', 'Running reviewers'], ['assertive', '2 of 3 reviewers finished · 1 failed']], 'the start and one summary; no part announces itself');
    assert.equal(await active(page), 't-review');
    assert.deepEqual(await taskEvents(), ['review:running', 'review-1:running', 'review-2:running', 'review-3:running', 'review-1:done', 'review-2:failed', 'review-3:done', 'review:failed']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('wkTask: Esc and ⌘. cancel short work while focus is in scope; in a Sheet the first Esc cancels the work and the Sheet stays', async () => {
    const { page, problems, run } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<input class="wk-field" id="t-field"><button class="wk-button" type="button" id="t-check">Check</button><input class="wk-field" id="t-else">'
        + '<wk-dialog id="t-sheet"><dialog class="wk-dialog wk-dialog--sheet"><h2 class="wk-dialog-title">Add repo</h2><input class="wk-field" id="t-in"><button class="wk-button" type="button" id="t-add">Add</button></dialog></wk-dialog>';
      window.c1 = kit.wkTask({ control: document.getElementById('t-check'), words: 'Checking token', stop: 'cancel' });
      window.c2 = kit.wkTask({ control: document.getElementById('t-check'), words: 'Checking token', stop: 'stop' });
    });
    await page.focus('#t-else');
    await page.keyboard.press('Escape');
    assert.equal(await page.evaluate(() => c1.state), 'running', 'focus outside the scope: Esc does nothing');
    await page.focus('#t-check');
    // ⌘. is macOS's: elsewhere Meta+. is the system's (the emoji picker on Windows).
    await page.keyboard.press('Meta+Period');
    assert.deepEqual(await page.evaluate(() => [c1.state, c2.state]), ['running', 'running'], 'Meta+. does nothing off macOS');
    await page.evaluate(() => Object.defineProperty(navigator, 'userAgentData', { get: () => ({ platform: 'macOS' }), configurable: true }));
    await page.keyboard.press('Meta+Period');
    assert.deepEqual(await page.evaluate(() => [c1.state, c2.state]), ['cancelled', 'running'], '⌘. cancels on macOS; a Stop task is not cancelled by a key');
    await page.evaluate(() => { c2.cancelled(); });
    await page.evaluate(() => { document.getElementById('t-sheet').show(); window.c3 = kit.wkTask({ control: document.getElementById('t-add'), words: 'Adding', stop: 'cancel' }); });
    await page.focus('#t-in');
    await page.keyboard.press('Escape');
    await run(10);
    assert.deepEqual(await page.evaluate(() => [c3.state, document.getElementById('t-sheet').open]), ['cancelled', true]);
    await page.keyboard.press('Escape');
    await run(10);
    assert.equal(await page.evaluate(() => document.getElementById('t-sheet').open), false, 'the second Esc closes the Sheet');
    assert.deepEqual(problems, []);
    await page.close();
  });
  // A fake Go tasks service: tasks:changed goes to the listeners wkTask.connect
  // subscribed, and the bindings record their calls.
  const fakeTasks = page => page.evaluate(() => {
    window.listeners = [];
    window.calls = [];
    window.initial = window.initial || [];
    const rec = name => async (...args) => { window.calls.push([name, ...args].join(':')); };
    window.bindings = {
      Tasks: async () => window.initial,
      Get: async id => { window.calls.push(`Get:${id}`); throw new Error('tasks_not_found'); },
      Stop: rec('Stop'), Pause: rec('Pause'), Resume: rec('Resume'), Retry: rec('Retry'), Dismiss: rec('Dismiss'), DismissFinished: rec('DismissFinished'),
    };
    window.disconnect = kit.wkTask.connect({ subscribe: fn => { window.listeners.push(fn); return () => {}; }, bindings: window.bindings });
    window.snap = o => ({ id: 't1', parent: '', subject: '', kind: 'export', state: 'running', done: 0, total: 0, unit: '', step: 0, steps: 0, stage: '', words: 'Exporting', stop: 'stop', error: null, retry: 'manual', nextTry: null, startedAt: Date.now(), updatedAt: Date.now(), ...o });
    window.send = tasks => window.listeners.forEach(fn => fn({ tasks }));
  });

  await check('wkTask.follow: draws a Go task from its snapshots (indicator, Stop, waiting, failure); a task finished before a reload is not announced; an unknown one leaves the control alone', async () => {
    const { page, problems, run, said, taskEvents } = await openTasks();
    await page.evaluate(() => {
      window.initial = [{ id: 'old', parent: '', subject: '', kind: 'sync', state: 'done', done: 0, total: 0, unit: '', step: 0, steps: 0, stage: '', words: 'Synced', stop: false, error: null, retry: 'manual', nextTry: null, startedAt: 1, updatedAt: 2 }];
      document.getElementById('tbox').innerHTML = '<button class="wk-button" type="button" id="t-go">Export</button><button class="wk-button" type="button" id="t-sync">Sync</button><button class="wk-button" type="button" id="t-ghost">Ghost</button>';
    });
    await fakeTasks(page);
    await run(10);
    // Finished before this window loaded: drawn as it is, quietly.
    await page.evaluate(() => { window.fo = kit.wkTask.follow('old', { control: document.getElementById('t-sync') }); });
    assert.deepEqual(await page.evaluate(() => [fo.state, document.getElementById('t-sync').hasAttribute('data-busy')]), ['done', false]);
    await page.evaluate(() => { send([window.initial[0], snap({})]); window.f = kit.wkTask.follow('t1', { control: document.getElementById('t-go') }); });
    assert.equal(await page.evaluate(() => document.getElementById('t-go').getAttribute('aria-disabled')), 'true');
    await run(400);
    assert.equal(await page.evaluate(() => Boolean(document.querySelector('#t-go > .wk-spinner'))), true);
    await page.evaluate(() => send([window.initial[0], snap({ done: 3, total: 10, unit: 'files', updatedAt: Date.now() })]));
    await run(9600);
    assert.equal(await page.evaluate(() => getComputedStyle(document.querySelector('.wk-task-stop')).visibility), 'visible', 'Stop at 10 s');
    await page.$eval('.wk-task-stop', b => b.click());
    await run(10);
    assert.deepEqual(await page.evaluate(() => [window.calls.join(), f.state]), ['Stop:t1', 'running'], 'Stop asks the Go side; its snapshot ends the task');
    await page.evaluate(() => send([window.initial[0], snap({ state: 'waiting', words: 'Waiting for GitHub…', done: 3, total: 10, unit: 'files' })]));
    assert.equal(await page.evaluate(() => f.state), 'waiting');
    // The app's words, not the error code's generic message.
    await page.evaluate(() => send([window.initial[0], snap({ state: 'failed', words: 'Couldn’t export the report', error: { code: 'x', userMsg: 'Couldn’t finish this', message: 'EIO' } })]));
    await run(1000);
    assert.deepEqual(await page.evaluate(() => [f.state, document.getElementById('t-go').hasAttribute('data-busy'), document.querySelectorAll('.wk-task-stop').length]), ['failed', false, 0]);
    // An id the Go side doesn't know: asked once, then left alone.
    await page.evaluate(() => { window.fg = kit.wkTask.follow('nope', { control: document.getElementById('t-ghost') }); });
    await run(10);
    assert.deepEqual(await page.evaluate(() => [fg.state, document.getElementById('t-ghost').hasAttribute('data-busy'), window.calls.includes('Get:nope')]), [null, false, true]);
    assert.deepEqual(await said(), [['polite', 'Exporting'], ['polite', 'Waiting for GitHub… Couldn’t export the report']]);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('task list: the summary shows for two running tasks or a failure; it opens the list, whose rows stop, retry and dismiss by keyboard; a failed task stays; focus returns', async () => {
    const { page, problems, run, said } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<div class="wk-footer-bar" id="t-foot"><span>Ready</span></div><button class="wk-button" type="button" id="t-save">Save</button>';
    });
    await fakeTasks(page);
    await page.evaluate(() => { window.sum = kit.wkTask.summary(); document.getElementById('t-foot').append(sum); });
    const look = () => page.evaluate(() => ({ hidden: sum.hidden, text: sum.textContent, tone: sum.querySelector('.wk-dot').dataset.tone ?? null }));
    await page.evaluate(() => send([snap({ id: 'g1', subject: 'report.pdf' })]));
    await run(1);
    assert.equal((await look()).hidden, true, 'one task shows where it started');
    // A page task joins the count only once its indicator shows: work that
    // ends in the quiet wait never flashes the summary.
    await page.evaluate(() => { window.pt = kit.wkTask({ id: 'p1', words: 'Saving' }); });
    await run(399);
    assert.equal((await look()).hidden, true);
    await run(1);
    assert.deepEqual(await look(), { hidden: false, text: '2 tasks running', tone: 'working' });
    await page.evaluate(() => { pt.cancelled(); });
    await run(1100);
    assert.equal((await look()).hidden, true);
    await said();
    await page.evaluate(() => send([snap({ id: 'g1', subject: 'report.pdf', done: 12, total: 40, unit: 'files' }), snap({ id: 'g2', subject: 'notes.md', stop: 'pause' }), snap({ id: 'g3', state: 'done', words: 'Synced', stop: false })]));
    await run(1);
    assert.deepEqual(await look(), { hidden: false, text: '2 tasks running', tone: 'working' });
    // A page task that fails stays in the list until retried or dismissed.
    await page.evaluate(() => {
      window.retried = 0;
      const t = kit.wkTask({ id: 'k1', subject: 'settings.json', control: document.getElementById('t-save'), words: 'Saving' });
      t.fail({ words: 'Couldn\u2019t save the file', retry: () => { window.retried++; } });
    });
    await run(120_000);
    assert.deepEqual(await look(), { hidden: false, text: '2 tasks running · 1 failed', tone: 'attention' });
    assert.deepEqual(await said(), [['polite', 'Couldn\u2019t save the file. Retry is in Tasks.']], 'a failure away from its control says where Retry is');
    await page.focus('#t-save');
    await page.keyboard.press('Shift+Tab');
    await page.$eval('.wk-task-summary', b => b.focus());
    await page.keyboard.press('Enter');
    await run(10);
    const rows = () => page.evaluate(() => [...document.querySelectorAll('.wk-task-list .wk-task-rows > li')].map(li => [li.dataset.id, li.querySelector('.wk-task-title').textContent, li.querySelector('.wk-task-line').hidden ? '' : li.querySelector('.wk-task-line').textContent, [...li.querySelectorAll(':scope > .wk-task-actions > button')].map(b => b.textContent).join('|')]));
    assert.equal(await page.evaluate(() => document.querySelector('.wk-task-list').open), true);
    assert.equal(await page.evaluate(() => document.activeElement.classList.contains('wk-dialog-scroll')), true, 'focus goes to the list, not to an action');
    assert.deepEqual(await rows(), [
      ['g1', 'report.pdf', 'Exporting · 12 of 40 files', 'Stop'],
      ['g2', 'notes.md', 'Exporting', 'Pause'],
      ['g3', 'Synced', '', 'Dismiss'],
      ['k1', 'settings.json', 'Couldn\u2019t save the file', 'Retry|Dismiss'],
    ]);
    // By keyboard: Tab reaches the first row's action.
    await page.keyboard.press('Tab');
    assert.equal(await active(page), 'Stop');
    await page.keyboard.press('Enter');
    await run(10);
    for (const name of ['Pause', 'Dismiss']) await page.$eval(`.wk-task-list button[data-action="${name.toLowerCase()}"]`, b => b.click());
    await run(10);
    assert.deepEqual(await page.evaluate(() => window.calls), ['Stop:g1', 'Pause:g2', 'Dismiss:g3']);
    // The Go side drops g3: the list follows, in place.
    await page.evaluate(() => send([snap({ id: 'g1', subject: 'report.pdf', done: 12, total: 40, unit: 'files' }), snap({ id: 'g2', subject: 'notes.md', stop: 'pause', state: 'paused', words: 'Paused' })]));
    await run(10);
    assert.deepEqual((await rows()).map(r => r[0] + ':' + r[3]), ['g1:Stop', 'g2:Resume', 'k1:Retry|Dismiss']);
    // Retry on the page's task: the app's retry runs, the row goes, and
    // focus passes to the next row rather than being lost.
    await page.$eval('.wk-task-list li[data-id="k1"] button[data-action="retry"]', b => b.focus());
    await page.keyboard.press('Enter');
    await run(10);
    assert.equal(await page.evaluate(() => window.retried), 1);
    assert.deepEqual((await rows()).map(r => r[0]), ['g1', 'g2']);
    assert.equal(await page.evaluate(() => document.querySelector('.wk-task-list').contains(document.activeElement) && document.activeElement !== document.body), true, 'focus stays in the list');
    // Clear finished: unavailable (aria-disabled, still focusable) while nothing has finished.
    const clear = () => page.$eval('.wk-task-list button[data-action="clear"]', b => { b.click(); return b.getAttribute('aria-disabled'); });
    assert.equal(await clear(), 'true');
    await run(10);
    assert.equal(await page.evaluate(() => window.calls.includes('DismissFinished')), false);
    await page.evaluate(() => send([snap({ id: 'g1', subject: 'report.pdf', state: 'done', words: 'Exported', stop: false })]));
    await run(10);
    assert.equal(await clear(), 'false');
    await run(10);
    assert.equal(await page.evaluate(() => window.calls.includes('DismissFinished')), true);
    await page.keyboard.press('Escape');
    await run(10);
    assert.equal(await page.evaluate(() => document.querySelector('.wk-task-list').open), false);
    assert.equal(await page.evaluate(() => document.activeElement === sum), true, 'focus returns to the summary');
    // With nothing left to show, it stays until focus leaves it.
    assert.equal((await look()).hidden, false);
    await page.focus('#t-save');
    await run(10);
    assert.equal((await look()).hidden, true);
    // Window > Tasks opens it with no summary press.
    await page.evaluate(() => kit.wkTask.showList());
    assert.equal(await page.evaluate(() => document.querySelector('.wk-task-list').open), true);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('task list and task states: forced colours keep failed words apart and draw the summary as a button', async () => {
    const { page, problems, run } = await openTasks();
    await page.emulateMedia({ forcedColors: 'active' });
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<span id="t-cell">Ready</span><div class="wk-footer-bar" id="t-foot"></div>';
      const sum = kit.wkTask.summary(); document.getElementById('t-foot').append(sum);
      const t = kit.wkTask({ target: document.getElementById('t-cell'), words: 'Copying' });
      t.fail('Couldn\u2019t copy');
    });
    await run(10);
    const look = await page.evaluate(() => ({
      words: getComputedStyle(document.querySelector('#t-cell .wk-task-words')).textDecorationLine,
      dot: getComputedStyle(document.querySelector('#t-cell .wk-dot')).borderTopStyle,
      summary: getComputedStyle(document.querySelector('.wk-task-summary')).forcedColorAdjust,
    }));
    assert.deepEqual(look, { words: 'underline', dot: 'solid', summary: 'none' });
    assert.deepEqual(problems, []);
    await page.close();
  });
  await check('diff: Show more is busy, not disabled, while it loads: focus stays and a second press does nothing; a failure goes to the announcer', async () => {
    const { page, problems, run, said } = await openTasks();
    await page.evaluate(() => {
      const text = 'diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -30,3 +30,3 @@\n a\n-b\n+c\n d\n';
      window.asks = 0;
      const d = kit.wkDiff({ id: 'busy-diff', diff: text, getContext: () => { window.asks++; return new Promise((res, rej) => { window.reject = rej; }); } });
      document.getElementById('tbox').append(d);
    });
    await page.focus('#busy-diff .wk-diff-more');
    await page.keyboard.press('Enter');
    await page.keyboard.press('Enter');
    const look = await page.evaluate(() => { const b = document.querySelector('#busy-diff .wk-diff-more'); return [b.disabled, b.getAttribute('aria-disabled'), b.hasAttribute('data-busy'), document.activeElement === b, window.asks]; });
    assert.deepEqual(look, [false, 'true', true, true, 1]);
    await page.evaluate(() => window.reject(new Error('net')));
    await run(600);
    assert.deepEqual(await said(), [['assertive', 'Couldn\u2019t load lines']], 'assertive: focus is still on the button');
    assert.equal(await page.evaluate(() => document.activeElement === document.querySelector('#busy-diff .wk-diff-more')), true);
    // Pressed again and left: the failure is polite.
    await page.keyboard.press('Enter');
    await page.evaluate(() => { document.activeElement.blur(); window.reject(new Error('net')); });
    await run(600);
    assert.deepEqual(await said(), [['polite', 'Couldn\u2019t load lines']]);
    assert.deepEqual(problems, []);
    await page.close();
  });
  await check('announcer: while a modal Sheet is open the regions are inside it, so the accessibility tree still has what is said; they go back when it closes', async () => {
    const { page, problems, run } = await openTasks();
    const heard = async text => (await axTree(page)).some(n => String(n.name?.value ?? '').includes(text));
    await page.evaluate(() => kit.wkTask.showList());
    await run(100);
    assert.equal(await page.evaluate(() => document.querySelector('.wk-task-list').contains(document.querySelector('[data-wk-announcer]'))), true);
    await page.evaluate(() => kit.announce('Exported the report'));
    await run(600);
    assert.equal(await heard('Exported the report'), true, 'not inert under the modal');
    // Closed by the script: the regions are back in the same task, before the
    // dialog's close event (queued as a task of its own) has fired.
    assert.equal(await page.evaluate(async () => {
      const d = document.querySelector('.wk-task-list');
      let closeFired = false;
      d.addEventListener('close', () => { closeFired = true; }, { once: true });
      d.close();
      await Promise.resolve();
      return [closeFired, document.querySelector('[data-wk-announcer]').parentNode === document.body].join();
    }), 'false,true');
    // Closed by Esc: the browser may close it a task later; wait for that.
    await page.evaluate(() => kit.wkTask.showList());
    await run(10);
    assert.equal(await page.evaluate(() => document.querySelector('.wk-task-list').contains(document.querySelector('[data-wk-announcer]'))), true);
    await page.keyboard.press('Escape');
    for (let i = 0; i < 100 && await page.evaluate(() => document.querySelector('.wk-task-list').open); i++) await new Promise(res => setTimeout(res, 20));
    assert.equal(await page.evaluate(() => document.querySelector('[data-wk-announcer]').parentNode === document.body), true);
    await page.evaluate(() => kit.announce('Back in the window'));
    await run(600);
    assert.equal(await heard('Back in the window'), true);
    // A plain <dialog> the app opens itself: the toggle event moves them too.
    await page.evaluate(() => { const d = document.createElement('dialog'); d.id = 't-plain'; d.innerHTML = '<p>Plain</p>'; document.body.append(d); d.showModal(); });
    await run(10);
    await page.evaluate(() => kit.announce('Said in a plain dialog'));
    await run(600);
    assert.equal(await heard('Said in a plain dialog'), true);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('wkTask: focus on a Stop button in a target, or on a task list action, survives progress redraws; a target row without a focusable control keeps focus at the end', async () => {
    const { page, problems, run } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<div id="t-row"><span id="t-cell">Idle</span></div><button class="wk-button" type="button" id="t-two">Two</button>';
      window.t = kit.wkTask({ target: document.getElementById('t-cell'), words: 'Copying', stop: 'stop', expect: 'long' });
      window.t2 = kit.wkTask({ control: document.getElementById('t-two'), words: 'Syncing', stop: 'stop', expect: 'long' });
    });
    await run(450);
    await page.$eval('#t-cell .wk-task-stop', b => b.focus());
    await page.evaluate(() => { t.progress({ done: 1, total: 4, unit: 'files' }); t.setWords('Copying files'); t.progress({ done: 2, total: 4, unit: 'files', step: 1, steps: 2, stage: 'Copying' }); });
    assert.equal(await page.evaluate(() => document.activeElement?.classList.contains('wk-task-stop')), true, 'focus stays on Stop in the target');
    await page.evaluate(() => kit.wkTask.showList());
    await run(50);
    await page.$eval('.wk-task-list li[data-id="' + await page.evaluate(() => t.id) + '"] button[data-action="stop"]', b => b.focus());
    await page.evaluate(() => { t.progress({ done: 3, total: 4, unit: 'files', step: 2, steps: 2, stage: 'Checking' }); t2.progress({ done: 1, total: 2 }); });
    await run(10);
    assert.equal(await page.evaluate(() => document.activeElement?.dataset.action), 'stop', 'focus stays on the row action');
    await page.keyboard.press('Escape');
    await run(10);
    // Stop in a target whose row has no focusable control: the end moves focus to the row, not the page.
    await page.$eval('#t-cell .wk-task-stop', b => b.focus());
    await page.evaluate(() => { t.done('Copied'); });
    await run(600);
    assert.equal(await page.evaluate(() => document.activeElement !== document.body && document.getElementById('t-cell').contains(document.activeElement) || document.activeElement === document.getElementById('t-cell')), true);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('wkTask: a shared control keeps its spinner until its last task ends; a task in parts doesn\'t time out while its parts report; a part that ends at once doesn\'t end the task', async () => {
    const { page, problems, run } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<button class="wk-button" type="button" id="t-go">Go</button><button class="wk-button" type="button" id="t-run">Run</button>';
      window.a = kit.wkTask({ control: document.getElementById('t-go'), words: 'A' });
      window.b = kit.wkTask({ control: document.getElementById('t-go'), words: 'B' });
    });
    await run(450);
    await page.evaluate(() => { a.done('A done'); });
    await run(600);
    const shared = () => page.evaluate(() => { const g = document.getElementById('t-go'); return [g.getAttribute('aria-disabled'), g.querySelectorAll('.wk-spinner').length]; });
    assert.deepEqual(await shared(), ['true', 1], 'B still runs: busy, one spinner');
    await page.evaluate(() => { b.done('B done'); });
    await run(600);
    assert.deepEqual(await shared(), [null, 0]);
    await page.evaluate(() => {
      window.whole = kit.wkTask({ control: document.getElementById('t-run'), words: 'Running reviewers', unit: 'reviewers' });
      window.ps = [whole.part({ words: 'A' }), whole.part({ words: 'B' })];
    });
    for (let i = 0; i < 15; i++) { await run(10_000); await page.evaluate(n => { ps[0].progress({ done: n, total: 20 }); ps[1].progress({ done: n, total: 20 }); }, i); }
    assert.equal(await page.evaluate(() => whole.state), 'running', '150 s of progress in its parts');
    const sync = await page.evaluate(() => {
      const t = kit.wkTask({ words: 'Removing', unit: 'items' });
      const p1 = t.part({ words: 'a' });
      p1.done('a removed');
      let threw = null;
      try { t.part({ words: 'b' }).done('b removed'); } catch (e) { threw = e.message; }
      return new Promise(res => queueMicrotask(() => queueMicrotask(() => res([threw, t.state]))));
    });
    assert.deepEqual(sync, [null, 'done']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('wkTask: Esc goes to an open menu inside the scope first; Esc while a Stop is under way is not swallowed', async () => {
    const { page, problems, run } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<wk-dialog id="t-sheet"><dialog class="wk-dialog wk-dialog--sheet"><h2 class="wk-dialog-title">Sheet</h2>'
        + '<wk-menu id="t-menu"><button class="wk-button" type="button" id="t-more">More</button><div role="menu" class="wk-menu" hidden><button role="menuitem" type="button">One</button></div></wk-menu>'
        + '<button class="wk-button" type="button" id="t-chk">Check</button></dialog></wk-dialog>';
      document.getElementById('t-sheet').show();
      window.t = kit.wkTask({ control: document.getElementById('t-chk'), words: 'Checking', stop: 'cancel', onStop: () => new Promise(() => {}) });
    });
    await page.$eval('#t-more', b => { b.focus(); b.click(); });
    await run(10);
    await page.keyboard.press('Escape');
    await run(10);
    assert.deepEqual(await page.evaluate(() => [t.state, document.getElementById('t-menu').open, document.querySelector('#t-sheet dialog').open]), ['running', false, true], 'the menu closes; the work and the Sheet stay');
    // First Esc: Stop starts (onStop never settles); the second isn't held by it.
    await page.keyboard.press('Escape');
    await run(10);
    assert.deepEqual(await page.evaluate(() => [t.state, document.querySelector('#t-sheet dialog').open]), ['running', true]);
    await page.keyboard.press('Escape');
    await run(10);
    assert.equal(await page.evaluate(() => document.querySelector('#t-sheet dialog').open), false);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('wkTask.follow: a resumed task reads Pause again and is announced; an unsafe failure has no Retry hint; disconnect() puts followed controls back; a failed binding is said in words', async () => {
    const { page, problems, run, said } = await openTasks();
    await page.evaluate(() => { document.getElementById('tbox').innerHTML = '<button class="wk-button" type="button" id="t-go">Export</button><button class="wk-button" type="button" id="t-two">Two</button>'; });
    await fakeTasks(page);
    await page.evaluate(() => {
      window.sum = kit.wkTask.summary(); document.getElementById('tbox').append(sum);
      send([snap({ stop: 'pause' })]);
      window.f = kit.wkTask.follow('t1', { control: document.getElementById('t-go'), expect: 'long' });
    });
    await run(500);
    await page.evaluate(() => send([snap({ stop: 'pause', state: 'paused', words: 'Paused · 3 of 9' })]));
    await run(10);
    assert.equal(await page.evaluate(() => document.querySelector('.wk-task-stop').textContent), 'Resume');
    await run(600);
    await said();
    await page.evaluate(() => send([snap({ stop: 'pause', state: 'running', words: 'Exporting' })]));
    await run(600);
    assert.equal(await page.evaluate(() => [f.state, document.querySelector('.wk-task-stop').textContent].join()), 'running,Pause');
    assert.deepEqual(await said(), [['polite', 'Exporting']]);
    // An unsafe failure: no Retry, so no "Retry is in Tasks."
    await page.evaluate(() => send([snap({ state: 'failed', words: 'Couldn’t confirm the comment was posted', retry: 'unsafe', error: { code: 'x', userMsg: 'x', message: 'x' } })]));
    await run(600);
    assert.deepEqual(await said(), [['polite', 'Couldn’t confirm the comment was posted']]);
    // disconnect() ends what it follows, quietly.
    await page.evaluate(() => { send([snap({ id: 't2' })]); window.f2 = kit.wkTask.follow('t2', { control: document.getElementById('t-two') }); });
    await run(1000);
    await said();
    await page.evaluate(() => disconnect());
    await run(600);
    assert.deepEqual(await page.evaluate(() => { const b = document.getElementById('t-two'); return [b.getAttribute('aria-disabled'), b.querySelectorAll('.wk-spinner').length, f2.state]; }), [null, 0, 'cancelled']);
    assert.deepEqual(await said(), []);
    // A binding that fails is said in words.
    await fakeTasks(page);
    await page.evaluate(() => { window.bindings.Stop = async () => { throw new Error('boom'); }; send([snap({ id: 't3', subject: 'a' }), snap({ id: 't4', subject: 'b' })]); kit.wkTask.showList(); });
    await run(10);
    await page.$eval('.wk-task-list li[data-id="t3"] button[data-action="stop"]', b => { b.focus(); b.click(); });
    await run(600);
    assert.deepEqual(await said(), [['assertive', 'Couldn’t stop the task']]);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('task list: the summary names running, queued, paused, stalled and failed apart; it can be disposed, and stops when taken out of the page', async () => {
    const { page, problems, run } = await openTasks();
    await fakeTasks(page);
    await page.evaluate(() => { window.sum = kit.wkTask.summary(); document.getElementById('tbox').append(sum); });
    const text = () => page.evaluate(() => [sum.hidden, sum.textContent].join('|'));
    await page.evaluate(() => send([snap({ id: 'a' }), snap({ id: 'b', state: 'paused' })]));
    await run(10);
    assert.equal(await text(), 'false|1 task running · 1 paused');
    await page.evaluate(() => send([snap({ id: 'a', state: 'waiting' }), snap({ id: 'c', state: 'queued' }), snap({ id: 'd', state: 'stalled' }), snap({ id: 'e', state: 'failed' })]));
    await run(10);
    assert.equal(await text(), 'false|1 task running · 1 queued · 1 needs you · 1 failed');
    await page.evaluate(() => sum.remove());
    await page.evaluate(() => send([snap({ id: 'a' })]));
    await run(10);
    await page.evaluate(() => send([snap({ id: 'a' }), snap({ id: 'z' })]));
    await run(10);
    assert.equal(await text(), 'false|1 task running · 1 queued · 1 needs you · 1 failed', 'out of the page: it stopped updating');
    assert.equal(await page.evaluate(() => typeof sum.dispose), 'function');
    // A summary made but never shown is no task list: a failure doesn't send the person to one.
    await page.evaluate(() => { kit.wkTask.summary(); kit.wkTask({ words: 'Saving' }).fail({ words: 'Couldn\u2019t save', retry: () => {} }); });
    await run(600);
    assert.deepEqual(await page.evaluate(() => window.said.splice(0)), [['polite', 'Couldn\u2019t save']]);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('announcer: one element per window; removed with an open modal, it is put back at once with the words it was about to say; moving it clears what was said', async () => {
    const { page, problems, run } = await openTasks();
    const heard = async text => (await axTree(page)).some(n => String(n.name?.value ?? '').includes(text));
    await page.evaluate(() => { window.firstHost = document.querySelector('[data-wk-announcer]'); });
    // Said, then the sheet opens: it lands in the sheet empty, not saying the old words again.
    await page.evaluate(() => kit.announce('Before the sheet'));
    await run(600);
    await page.evaluate(() => kit.wkTask.showList());
    await run(10);
    assert.deepEqual(await page.evaluate(() => { const h = document.querySelector('[data-wk-announcer]'); return [document.querySelector('.wk-task-list').contains(h), h.textContent]; }), [true, '']);
    await page.keyboard.press('Escape');
    for (let i = 0; i < 100 && await page.evaluate(() => document.querySelector('.wk-task-list').open); i++) await new Promise(res => setTimeout(res, 20));
    // A plain modal, words queued, then the modal removed inside the 500 ms.
    await page.evaluate(() => { const d = document.createElement('dialog'); d.id = 't-rm'; document.body.append(d); d.showModal(); });
    await run(10);
    const back = await page.evaluate(async () => {
      kit.announce('Comment posted');
      document.getElementById('t-rm').remove();
      await Promise.resolve();
      const h = document.querySelector('[data-wk-announcer]');
      return [h === window.firstHost, h?.parentNode === document.body, document.querySelectorAll('[data-wk-announcer]').length];
    });
    assert.deepEqual(back, [true, true, 1], 'the same element, back in <body> in the same task');
    await run(600);
    assert.equal(await heard('Comment posted'), true, 'the queued words reach a region in the tree');
    // Words just said, then their modal removed: set again on the next frame in <body>.
    await page.evaluate(() => { const d = document.createElement('dialog'); d.id = 't-rm2'; document.body.append(d); d.showModal(); kit.announce('Report exported'); });
    await run(600);
    await page.evaluate(async () => { document.getElementById('t-rm2').remove(); await Promise.resolve(); });
    await run(100);
    assert.equal(await heard('Report exported'), true);
    assert.deepEqual(problems, []);
    await page.close();
  });

  // ---- Toast, app undo, copy feedback, skeleton rows, loading chart (#144) ----
  // The toast's box and state: [words, action label, leaving, role, live regions inside].
  const toastLook = page => page.evaluate(() => [...document.querySelectorAll('.wk-toast')].map(t => [
    t.querySelector('.wk-toast-words').textContent, t.querySelector('.wk-toast-action')?.textContent ?? null,
    t.hasAttribute('data-leaving'), t.getAttribute('role'), t.querySelectorAll('[aria-live], [role="status"], [role="alert"]').length + (t.hasAttribute('aria-live') ? 1 : 0)]));

  await check('toast: bottom centre above a footer bar, 6 s or 10 s with an action, announced through the announcer once, no live region of its own; a newer one replaces it', async () => {
    const { page, problems, run, said } = await openTasks();
    const liveRegions = () => page.evaluate(() => document.querySelectorAll('[aria-live], [role="status"], [role="alert"]').length);
    const regionsBefore = await liveRegions();
    await page.evaluate(() => kit.wkToast({ words: 'Link copied' }));
    assert.deepEqual(await toastLook(page), [['Link copied', null, false, 'group', 0]]);
    assert.equal(await liveRegions(), regionsBefore, 'no live region added');
    // Where it comes to rest: the entry transition (on the real clock) finished.
    const settle = () => page.evaluate(() => document.querySelector('.wk-toast').getAnimations().forEach(a => a.finish()));
    await settle();
    const box = await page.evaluate(() => { const r = document.querySelector('.wk-toast').getBoundingClientRect(); return { mid: Math.round(r.left + r.width / 2), gap: Math.round(innerHeight - r.bottom) }; });
    assert.deepEqual(box, { mid: 550, gap: 16 }, 'bottom centre');
    await run(500);
    assert.deepEqual(await said(), [['polite', 'Link copied']]);
    await run(5499);
    assert.equal((await toastLook(page))[0][2], false, 'still there at 5999 ms');
    await run(1);
    assert.equal((await toastLook(page))[0][2], true, 'leaving at 6 s');
    assert.match(await page.evaluate(() => getComputedStyle(document.querySelector('.wk-toast')).transitionDuration), /^0\.13\d*s$/, 'it fades out in two-thirds of 200 ms');
    await run(132);
    assert.equal((await toastLook(page)).length, 1, 'fading out');
    await run(1);
    assert.deepEqual(await toastLook(page), [], 'gone two-thirds of 200 ms later');
    // With an action: 10 s.
    await page.evaluate(() => kit.wkToast({ words: 'Conversation archived', action: { label: 'Undo', run() {} } }));
    await run(9999);
    assert.equal((await toastLook(page))[0][2], false, 'still there at 9999 ms');
    await run(1);
    assert.equal((await toastLook(page))[0][2], true, 'leaving at 10 s');
    await run(200);
    // A newer toast replaces it at once; the old one's clock goes with it.
    await page.evaluate(() => { kit.wkToast({ words: 'Comment posted' }); });
    await run(3000);
    await page.evaluate(() => { kit.wkToast({ words: 'Report exported' }); });
    assert.deepEqual(await toastLook(page), [['Report exported', null, false, 'group', 0]]);
    await run(5999);
    assert.equal((await toastLook(page))[0][2], false, 'the new one has its own 6 s');
    await run(1);
    assert.equal((await toastLook(page))[0][2], true);
    await run(200);
    // Above a footer bar along the bottom of the window.
    await page.evaluate(() => {
      const bar = document.createElement('div');
      bar.className = 'wk-footer-bar';
      bar.id = 't-footer';
      bar.textContent = 'Connected';
      for (const [p, v] of [['position', 'fixed'], ['left', '0'], ['right', '0'], ['bottom', '0'], ['height', '30px']]) bar.style.setProperty(p, v);
      document.body.append(bar);
      kit.wkToast({ words: 'Saved' });
    });
    await settle();
    assert.equal(await page.evaluate(() => Math.round(document.getElementById('t-footer').getBoundingClientRect().top - document.querySelector('.wk-toast').getBoundingClientRect().bottom)), 16, 'above the footer bar');
    assert.equal(await page.evaluate(() => { try { kit.wkToast({ words: ' ' }); } catch (e) { return e instanceof TypeError; } return false; }), true, 'words are required');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('toast: its clock pauses while the pointer or focus is on it and while the window isn\'t focused, and runs on from where it stopped', async () => {
    const { page, problems, run } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<button class="wk-button" type="button" id="t-a">Archive</button>';
      document.getElementById('t-a').focus();
      kit.wkToast({ words: 'Conversation archived', action: { label: 'Undo', run() {} } });
    });
    const leaving = async () => (await toastLook(page))[0]?.[2];
    await run(4000);
    // Pointer on it: paused.
    const r = await page.evaluate(() => { const b = document.querySelector('.wk-toast').getBoundingClientRect(); return [b.left + 10, b.top + 10]; });
    await page.mouse.move(r[0], r[1]);
    await run(20_000);
    assert.equal(await leaving(), false, 'paused under the pointer');
    await page.mouse.move(5, 5);
    await run(3000);
    // Window not focused: paused.
    await page.evaluate(() => window.dispatchEvent(new Event('blur')));
    await run(20_000);
    assert.equal(await leaving(), false, 'paused while the window is in the background');
    await page.evaluate(() => window.dispatchEvent(new Event('focus')));
    await run(2000);
    // Focus on it: paused.
    await page.keyboard.press('F6');
    await run(20_000);
    assert.equal(await leaving(), false, 'paused while focus is on it');
    await page.keyboard.press('F6');
    assert.equal(await active(page), 't-a');
    // 4 + 3 + 2 = 9 s ran: 1 s is left.
    await run(999);
    assert.equal(await leaving(), false, 'resumes where it stopped');
    await run(1);
    assert.equal(await leaving(), true, 'gone after 10 s of running');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('toast: it never takes focus; F6 and Shift+F6 move to its action and back; Esc dismisses it and focus returns; a newer toast keeps focus in the toast', async () => {
    const { page, problems, run } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<button class="wk-button" type="button" id="t-a">Archive</button>';
      document.getElementById('t-a').focus();
      window.ran = 0;
      kit.wkToast({ words: 'Conversation archived', action: { label: 'Undo', run() { window.ran++; } } });
    });
    assert.equal(await active(page), 't-a', 'showing it leaves focus where it was');
    await page.keyboard.press('F6');
    assert.equal(await active(page), 'Undo');
    assert.equal(await page.evaluate(() => document.activeElement.closest('.wk-toast').getAttribute('aria-labelledby') === document.querySelector('.wk-toast-words').id), true, 'named by its words');
    await page.keyboard.press('Shift+F6');
    assert.equal(await active(page), 't-a', 'F6 again goes back');
    await page.keyboard.press('F6');
    // A newer toast while focus is in it: its action takes focus; Esc then goes back.
    await page.evaluate(() => kit.wkToast({ words: 'Message moved', action: { label: 'Undo', run() { window.ran += 10; } } }));
    assert.deepEqual([await active(page), (await toastLook(page)).map(t => t[0])], ['Undo', ['Message moved']]);
    await page.keyboard.press('Escape');
    assert.equal(await active(page), 't-a', 'Esc returns focus');
    assert.equal((await toastLook(page))[0][2], true, 'Esc dismisses it');
    await run(200);
    // The action: runs once, the toast closes, focus goes back.
    await page.evaluate(() => kit.wkToast({ words: 'Conversation archived', action: { label: 'Undo', run() { window.ran++; } } }));
    await page.keyboard.press('F6');
    await page.keyboard.press('Enter');
    assert.deepEqual([await page.evaluate(() => window.ran), await active(page), (await toastLook(page))[0][2]], [1, 't-a', true]);
    await run(200);
    // Without an action, F6 is left alone and nothing in it takes focus.
    await page.evaluate(() => kit.wkToast({ words: 'Link copied' }));
    await page.keyboard.press('F6');
    assert.equal(await active(page), 't-a');
    // Esc in the page outside it doesn't dismiss it.
    await page.keyboard.press('Escape');
    assert.equal((await toastLook(page))[0][2], false);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('toast: reduced motion fades in 100 ms with no slide; forced colours keep its outline', async () => {
    const { page, problems, run } = await openTasks();
    const look = () => page.evaluate(() => { const s = getComputedStyle(document.querySelector('.wk-toast')); return [s.transitionProperty, s.transitionDuration, s.transform]; });
    await page.evaluate(() => kit.wkToast({ words: 'Saved' }));
    const [prop, dur] = await look();
    assert.match(prop, /transform/);
    assert.equal(dur.split(', ')[0], '0.2s');
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await page.evaluate(() => kit.wkToast({ words: 'Saved again' }));
    const [rprop, rdur, rtransform] = await look();
    assert.deepEqual([rprop, rdur], ['opacity', '0.1s'], 'only a fade of 100 ms');
    assert.match(rtransform, /^matrix\(1, 0, 0, 1, -?[\d.]+, 0\)$/, 'no vertical offset');
    await page.evaluate(() => document.querySelector('.wk-toast [class]') && kit.wkToast({ words: 'Third' }));
    await page.evaluate(() => window.dispatchEvent(new Event('focus')));
    await run(6000);
    await run(99);
    assert.equal(await page.evaluate(() => document.querySelectorAll('.wk-toast').length), 1);
    await run(1);
    assert.equal(await page.evaluate(() => document.querySelectorAll('.wk-toast').length), 0, 'gone after a 100 ms fade');
    await page.emulateMedia({ reducedMotion: 'no-preference', forcedColors: 'active' });
    await page.evaluate(() => kit.wkToast({ words: 'Saved' }));
    const border = await page.evaluate(() => { const s = getComputedStyle(document.querySelector('.wk-toast')); const p = document.createElement('i'); p.style.setProperty('color', 'CanvasText'); document.body.append(p); const c = getComputedStyle(p).color; p.remove(); return [s.borderTopStyle, s.borderTopColor === c]; });
    assert.deepEqual(border, ['solid', true]);
    assert.deepEqual(problems, []);
    await page.close();
  });

  // A stand-in for the Go UndoStack and its Wails events, in the page.
  async function fakeUndo(page) {
    await page.evaluate(() => {
      const go = window.go = {
        undo: [], redo: [], seq: 0, calls: [], textFocus: false, listeners: {}, failNext: null,
        emit(name, payload) { for (const f of go.listeners[name] || []) f(payload); },
        state() {
          const e = (x, verb) => (x ? { id: x.id, name: x.name, label: `${verb} ${x.name}` } : null);
          return { undo: e(go.undo.at(-1), 'Undo'), redo: e(go.redo.at(-1), 'Redo'), busy: false, textFocus: go.textFocus };
        },
        changed() { go.emit('undo:changed', go.state()); },
        push(name) { const id = `undo-${++go.seq}`; go.undo.push({ id, name }); go.redo = []; go.changed(); return id; },
        doUndo(id) {
          const top = go.undo.at(-1);
          if (!top) throw { code: 'undo_empty' };
          if (id && top.id !== id) throw { code: 'undo_stale' };
          if (go.failNext) { const e = go.failNext; go.failNext = null; throw e; }
          if (go.busyNext) { go.busyNext = false; throw { code: 'undo_busy' }; }
          go.undo.pop(); go.redo.push(top); go.changed();
        },
        // Edit > Undo, as UndoStack.MenuUndo does it.
        menuUndo() {
          if (go.textFocus) { go.emit('undo:text', { op: 'undo' }); return; }
          try { go.doUndo(go.undo.at(-1)?.id); } catch (e) { if (e.code === 'undo_failed') go.emit('undo:failed', { op: 'undo', id: '', name: go.undo.at(-1).name, error: e }); }
        },
      };
      window.disconnectUndo = kit.wkUndo.connect({
        on: (name, fn) => { (go.listeners[name] ??= []).push(fn); return () => { go.listeners[name] = go.listeners[name].filter(f => f !== fn); }; },
        bindings: {
          State: async () => go.state(),
          Undo: async () => { go.calls.push('Undo'); go.doUndo(); },
          Redo: async () => { go.calls.push('Redo'); },
          UndoID: async id => { go.calls.push(`UndoID ${id}`); go.doUndo(id); },
          SetTextFocus: async on => { go.calls.push(`SetTextFocus ${on}`); go.textFocus = on; go.changed(); },
        },
      });
      window.undoChanges = [];
      document.addEventListener('wk-undo-change', e => window.undoChanges.push(e.detail.undo?.label ?? null));
    });
  }

  await check('app undo: the toast\'s Undo and Edit > Undo use the same stack and agree; Edit > Undo closes the toast for the action it undid', async () => {
    const { page, problems, run, said } = await openTasks();
    await fakeUndo(page);
    await run(10);
    // The toast's Undo undoes exactly the action it named.
    await page.evaluate(() => { const id = go.push('Archive'); kit.wkToast({ words: 'Conversation archived', action: kit.wkUndo.action(id) }); });
    assert.deepEqual(await toastLook(page), [['Conversation archived', 'Undo', false, 'group', 0]]);
    await page.$eval('.wk-toast-action', b => { b.focus(); b.click(); });
    await run(10);
    assert.deepEqual(await page.evaluate(() => [go.calls.splice(0).filter(c => !c.startsWith('SetTextFocus')), kit.wkUndo.state().undo, kit.wkUndo.state().redo?.label]), [['UndoID undo-1'], null, 'Redo Archive']);
    await run(200);
    // Edit > Undo undoes the same action the toast offers, and the toast closes.
    await page.evaluate(() => { go.push('Move'); const id = go.push('Archive'); kit.wkToast({ words: 'Conversation archived', action: kit.wkUndo.action(id) }); });
    await page.evaluate(() => go.menuUndo());
    await run(10);
    assert.deepEqual(await page.evaluate(() => [go.undo.map(e => e.name), go.redo.map(e => e.name)]), [['Move'], ['Archive']]);
    assert.equal((await toastLook(page))[0][2], true, 'the toast for the undone action closes');
    await run(200);
    // A toast replaced by a newer one doesn't take its Undo with it: Edit > Undo still has it.
    await page.evaluate(() => { const id = go.push('Archive'); kit.wkToast({ words: 'Conversation archived', action: kit.wkUndo.action(id) }); kit.wkToast({ words: 'Link copied' }); });
    assert.equal(await page.evaluate(() => kit.wkUndo.state().undo.label), 'Undo Archive');
    // A stale toast Undo (the stack moved on) undoes nothing else and says nothing.
    await run(600);
    await said();
    await page.evaluate(() => { window.stale = kit.wkUndo.action(); go.push('Rename'); });
    await page.evaluate(() => window.stale.run());
    await run(600);
    assert.deepEqual(await page.evaluate(() => [go.undo.map(e => e.name), go.calls.splice(0).filter(c => c.startsWith('UndoID'))]), [['Move', 'Archive', 'Rename'], ['UndoID undo-4']]);
    assert.deepEqual(await said(), [], 'a stale toast Undo says nothing');
    // A toast's Undo pressed while an undo runs says why nothing happened.
    await page.evaluate(() => { go.busyNext = true; kit.wkUndo.action().run(); });
    await run(600);
    assert.deepEqual(await said(), [['polite', 'An undo is already under way']]);
    await page.evaluate(() => { go.busyNext = true; kit.wkUndo.undo(); });
    await run(600);
    assert.deepEqual(await said(), [], 'from a button, a busy stack says nothing');
    // A failure is said, assertively; the action stays.
    await page.evaluate(() => { go.failNext = { code: 'undo_failed' }; kit.wkUndo.undo(); });
    await run(600);
    assert.deepEqual(await said(), [['assertive', 'Couldn’t undo Rename']]);
    await page.evaluate(() => { go.failNext = { code: 'undo_failed' }; go.menuUndo(); });
    await run(600);
    assert.deepEqual(await said(), [['assertive', 'Couldn’t undo Rename']], 'from the menu too');
    assert.equal(await page.evaluate(() => window.undoChanges.length > 0), true, 'wk-undo-change for the app');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('app undo: text focus is reported to Go; Edit > Undo undoes typing first, then the app\'s action', async () => {
    const { page, problems, run } = await openTasks();
    await page.evaluate(() => { document.getElementById('tbox').innerHTML = '<input class="wk-field" id="t-name" aria-label="Name"><button class="wk-button" type="button" id="t-b">Other</button>'; });
    await fakeUndo(page);
    await page.evaluate(() => go.push('Archive'));
    await run(10);
    assert.deepEqual(await page.evaluate(() => go.calls.splice(0)), ['SetTextFocus false'], 'reported once on connect');
    await page.focus('#t-name');
    await run(10);
    assert.deepEqual(await page.evaluate(() => go.calls.splice(0)), ['SetTextFocus true']);
    // The window coming to the front reports again, even unchanged, so the
    // menu follows the window in front.
    await page.evaluate(() => window.dispatchEvent(new Event('focus')));
    await run(10);
    assert.deepEqual(await page.evaluate(() => go.calls.splice(0)), ['SetTextFocus true'], 'reported again on window focus');
    await page.keyboard.type('abc');
    await page.evaluate(() => go.menuUndo());
    await run(10);
    assert.deepEqual(await page.evaluate(() => [document.getElementById('t-name').value, go.undo.length, go.calls.slice()]), ['', 1, []], 'the typing goes first');
    // Another window has focus (this one's field still holds it): nothing here.
    await page.evaluate(() => { document.hasFocus = () => false; go.menuUndo(); delete document.hasFocus; });
    await run(10);
    assert.deepEqual(await page.evaluate(() => [go.undo.length, go.calls.slice()]), [1, []], 'an unfocused window does nothing');
    await page.evaluate(() => go.menuUndo());
    await run(10);
    assert.deepEqual(await page.evaluate(() => [go.undo.length, go.calls.splice(0)]), [0, ['Undo']], 'with no typing left, the app\'s action');
    await page.focus('#t-b');
    await run(10);
    assert.deepEqual(await page.evaluate(() => go.calls.splice(0)), ['SetTextFocus false']);
    await page.evaluate(() => window.disconnectUndo());
    assert.equal(await page.evaluate(() => kit.wkUndo.state().undo), null);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('copy: the kit copies; "Copied" shows above the button for 2 s and is announced once; the accessible name and glyph stay; failure says "Couldn\'t copy"', async () => {
    const { page, problems, run, said } = await openTasks();
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'], { origin });
    await page.evaluate(() => {
      document.getElementById('tbox').style.setProperty('padding-top', '60px');
      document.getElementById('tbox').innerHTML = '<button class="wk-copy" type="button" id="t-copy" aria-label="Copy join code" data-wk-copy="JOIN-42"><svg viewBox="0 0 16 16" aria-hidden="true"><rect width="8" height="8"></rect></svg></button>'
        + '<div class="wk-code"><code>npm i @wails-kit/ui</code><button class="wk-copy" type="button" id="t-code" aria-label="Copy command"></button></div>';
      window.copyEvents = [];
      document.addEventListener('wk-copy', e => window.copyEvents.push(e.detail.ok));
    });
    const before = await page.evaluate(() => document.getElementById('t-copy').outerHTML);
    const tip = () => page.evaluate(() => [...document.querySelectorAll('.wk-copy-tip')].map(t => [t.textContent, t.getAttribute('aria-hidden')]));
    await page.$eval('#t-copy', b => { b.focus(); b.click(); });
    await run(10);
    assert.equal(await page.evaluate(() => navigator.clipboard.readText()), 'JOIN-42');
    assert.deepEqual(await tip(), [['Copied', 'true']]);
    assert.equal(await page.evaluate(() => { const t = document.querySelector('.wk-copy-tip').getBoundingClientRect(), b = document.getElementById('t-copy').getBoundingClientRect(); return t.bottom <= b.top && Math.abs((t.left + t.right) / 2 - (b.left + b.right) / 2) < 1; }), true, 'centred above the button');
    assert.equal(await page.evaluate(() => document.getElementById('t-copy').outerHTML), before, 'name, description and glyph unchanged');
    const name = (await axTree(page)).find(n => n.role?.value === 'button' && n.name?.value === 'Copy join code');
    assert.ok(name, 'the accessible name stays');
    await run(600);
    assert.deepEqual(await said(), [['polite', 'Copied']], 'announced once');
    {
      // The tip is hidden from assistive tech: only the announcer says it.
      const cdp = await page.context().newCDPSession(page);
      const { root } = await cdp.send('DOM.getDocument', { depth: -1 });
      const { nodeId } = await cdp.send('DOM.querySelector', { nodeId: root.nodeId, selector: '.wk-copy-tip' });
      assert.equal((await cdp.send('Accessibility.getPartialAXTree', { nodeId, fetchRelatives: false })).nodes[0].ignored, true, 'the tip is hidden from assistive tech');
      await cdp.detach();
    }
    await run(1389);
    assert.equal((await tip()).length, 1, 'still there before 2 s');
    await run(1);
    assert.deepEqual(await tip(), [], 'gone at 2 s');
    // Inside a wk-code, it copies the code beside it.
    await page.$eval('#t-code', b => { b.focus(); b.click(); });
    await run(10);
    assert.equal(await page.evaluate(() => navigator.clipboard.readText()), 'npm i @wails-kit/ui');
    await run(2000);
    // A failure: assertive while focus is still on the button.
    await page.evaluate(() => { window.keepWrite = kit.wkCopy.write; kit.wkCopy.write = () => Promise.reject(new Error('denied')); });
    await said();
    await page.$eval('#t-copy', b => { b.focus(); b.click(); });
    await run(10);
    assert.deepEqual(await tip(), [['Couldn’t copy', 'true']]);
    await run(600);
    assert.deepEqual(await said(), [['assertive', 'Couldn’t copy']]);
    await run(2000);
    // A busy copy button does nothing.
    await page.evaluate(() => { kit.wkCopy.write = window.keepWrite; const b = document.getElementById('t-copy'); b.setAttribute('aria-disabled', 'true'); b.setAttribute('data-busy', ''); });
    await page.$eval('#t-copy', b => { b.focus(); b.click(); });
    await run(10);
    assert.deepEqual(await tip(), []);
    assert.deepEqual(await page.evaluate(() => window.copyEvents), [true, true, false]);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('skeleton rows: only after 400 ms, at the real row height, hidden from assistive tech, with aria-busy on the table; at least 500 ms once shown; the same count next time', async () => {
    const { page, problems, run } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<table class="wk-table" id="t-runs" aria-label="Runs"><thead><tr><th>Name</th><th>State</th><th class="wk-num">Time</th></tr></thead><tbody></tbody></table>'
        + '<table class="wk-table" id="t-real"><thead><tr><th>Name</th><th>State</th><th>Time</th></tr></thead><tbody><tr><td>build</td><td>Done</td><td>0:42</td></tr></tbody></table>';
      window.tbody = document.querySelector('#t-runs tbody');
      window.load = kit.wkSkeleton(window.tbody);
    });
    const look = () => page.evaluate(() => [document.getElementById('t-runs').getAttribute('aria-busy'), tbody.querySelectorAll('tr.wk-skeleton[aria-hidden="true"]').length, tbody.rows.length]);
    assert.deepEqual(await look(), ['true', 0, 0], 'busy at once, no rows');
    await run(399);
    assert.deepEqual(await look(), ['true', 0, 0], 'nothing in the quiet wait');
    await run(1);
    const [busy, n] = await look();
    assert.equal(busy, 'true');
    assert.ok(n >= 1 && n <= 12, `${n} rows, up to 12`);
    const shape = await page.evaluate(() => {
      const row = tbody.rows[0], real = document.querySelector('#t-real tbody tr');
      return [row.cells.length, row.textContent, Math.round(row.getBoundingClientRect().height) === Math.round(real.getBoundingClientRect().height), row.querySelectorAll('.wk-skeleton-bar').length, getComputedStyle(row.querySelector('.wk-skeleton-bar')).animationName];
    });
    assert.deepEqual(shape, [3, '', true, 3, 'none'], 'one still bar per column, no text, the real row height');
    // In the accessibility tree: the table, its header row, and no skeleton row.
    const cdp = await page.context().newCDPSession(page);
    const { root } = await cdp.send('DOM.getDocument', { depth: -1 });
    const axOf = async selector => {
      const { nodeId } = await cdp.send('DOM.querySelector', { nodeId: root.nodeId, selector });
      return (await cdp.send('Accessibility.getPartialAXTree', { nodeId, fetchRelatives: false })).nodes[0];
    };
    assert.equal((await axOf('#t-runs')).role.value, 'table');
    assert.equal((await axOf('#t-runs thead tr')).ignored, false);
    for (const sel of ['#t-runs tbody tr.wk-skeleton', '#t-runs tbody tr.wk-skeleton td', '#t-runs tbody tr.wk-skeleton:last-child td']) assert.equal((await axOf(sel)).ignored, true, `${sel} is hidden from assistive tech`);
    await cdp.detach();
    // Done 200 ms after they showed: they stay until 500 ms, then swap in one step.
    await run(200);
    const done = page.evaluate(() => window.load.done(() => { tbody.innerHTML = '<tr><td>a</td><td>Done</td><td>1:00</td></tr><tr><td>b</td><td>Done</td><td>2:00</td></tr>'; }).then(() => 'resolved'));
    await run(299);
    assert.deepEqual((await look()).slice(0, 2), ['true', n], 'held for the minimum');
    await run(1);
    assert.equal(await done, 'resolved');
    assert.deepEqual(await look(), [null, 0, 2], 'content in, busy gone');
    // Next time: as many rows as there were.
    await page.evaluate(() => { window.load = kit.wkSkeleton(tbody); });
    await run(400);
    assert.deepEqual(await look(), ['true', 2, 2]);
    await page.evaluate(() => { window.load.cancel(); });
    await run(500);
    assert.deepEqual(await page.evaluate(() => [...tbody.rows].map(r => r.cells[0].textContent)), ['a', 'b'], 'cancel puts the rows back');
    // Done within the quiet wait: no rows ever, the content at once.
    await page.evaluate(() => { tbody.replaceChildren(); window.load = kit.wkSkeleton(tbody, { rows: 4 }); });
    await run(100);
    await page.evaluate(() => window.load.done(() => { tbody.innerHTML = '<tr><td>c</td><td>Done</td><td>0:10</td></tr>'; }));
    assert.deepEqual(await look(), [null, 0, 1]);
    await run(1000);
    assert.deepEqual(await look(), [null, 0, 1]);
    // An explicit count.
    await page.evaluate(() => { tbody.replaceChildren(); window.load = kit.wkSkeleton(tbody, { rows: 4 }); });
    await run(400);
    assert.deepEqual(await look(), ['true', 4, 4]);
    // Forced colours: outlined in GrayText.
    await page.emulateMedia({ forcedColors: 'active' });
    const fc = await page.evaluate(() => { const s = getComputedStyle(tbody.querySelector('.wk-skeleton-bar')); const p = document.createElement('i'); p.style.setProperty('color', 'GrayText'); document.body.append(p); const c = getComputedStyle(p).color; p.remove(); return [s.outlineStyle, s.outlineColor === c]; });
    assert.deepEqual(fc, ['solid', true]);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('loading chart: the frame, title and legend stay, aria-busy, the plot keeps its height, a named spinner from 400 ms; ready() holds it 500 ms', async () => {
    const { page, problems, run } = await openTasks();
    await page.evaluate(() => {
      window.fig = kit.wkChart({ id: 't-chart', kind: 'bars', label: 'Jobs per day', loading: 'Fetching runs', series: [{ label: 'Succeeded', tone: 1 }, { label: 'Failed', tone: 2 }] });
      document.getElementById('tbox').append(window.fig);
    });
    const look = () => page.evaluate(() => {
      const s = fig.querySelector('.wk-chart-loading > .wk-spinner');
      return [fig.getAttribute('aria-busy'), fig.querySelector('.wk-chart-title').textContent, [...fig.querySelectorAll('.wk-chart-legend-item')].map(li => li.textContent), Math.round(fig.querySelector('.wk-chart-loading').getBoundingClientRect().height), s.getAttribute('role'), s.getAttribute('aria-label'), getComputedStyle(s).display !== 'none'];
    });
    assert.deepEqual(await look(), ['true', 'Jobs per day', ['Succeeded', 'Failed'], 220, 'progressbar', 'Fetching runs', false]);
    await run(399);
    assert.equal((await look())[6], false, 'no spinner in the quiet wait');
    await run(1);
    assert.equal((await look())[6], true, 'the spinner at 400 ms');
    assert.ok((await axTree(page)).find(n => n.role?.value === 'progressbar' && n.name?.value === 'Fetching runs'));
    await run(100);
    await page.evaluate(() => { window.chartReady = false; kit.wkChart.ready(fig).then(() => { window.chartReady = true; }); });
    await run(399);
    assert.equal(await page.evaluate(() => window.chartReady), false, 'held until the spinner has shown for 500 ms');
    await run(1);
    assert.equal(await page.evaluate(() => window.chartReady), true);
    // Ready before the spinner: at once.
    await page.evaluate(() => { window.fig2 = kit.wkChart({ kind: 'line', label: 'Runs', loading: 'Fetching runs', series: [] }); document.getElementById('tbox').append(window.fig2); });
    assert.equal(await page.evaluate(() => [fig2.querySelector('.wk-chart-legend'), kit.wkChart.ready(fig2) instanceof Promise][0]), null, 'no legend while the series are unknown');
    await run(1000);
    assert.equal(await page.evaluate(() => getComputedStyle(fig2.querySelector('.wk-spinner')).display), 'none', 'ready() before 400 ms: no spinner ever');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('app undo: Edit > Undo and Redo in the kit\'s code and rich-text editors use the editor\'s own history (typing, Enter, Backspace, bold then typing) and never the app\'s action', async () => {
    const { page, problems } = await open('/demo/index.html');
    await page.evaluate(async () => {
      window.kit = await import('/src/index.js');
      const { wkCodeEditor } = await import('/editor/code.js');
      const { wkRichTextEditor } = await import('/editor/richtext.js');
      const box = document.createElement('div');
      box.id = 'ebox';
      document.querySelector('main').prepend(box);
      window.code = wkCodeEditor({ label: 'Code', value: '' });
      window.rich = wkRichTextEditor({ label: 'Notes', value: '' });
      box.append(window.code, window.rich);
      window.go = { listeners: {}, calls: [] };
      kit.wkUndo.connect({
        on: (n, f) => { (go.listeners[n] ??= []).push(f); },
        bindings: { Undo: async () => go.calls.push('Undo'), Redo: async () => go.calls.push('Redo'), SetTextFocus: async v => { go.focus = v; go.calls.push(`focus ${v}`); }, State: async () => null },
      });
      // Edit > Undo as Apply sends it: a DOM event in this window's page.
      window.menu = op => document.dispatchEvent(new CustomEvent('wk-undo-text', { detail: { op } }));
    });
    const cases = [
      ['code: typing', '#ebox .wk-code-editor .cm-content', 'code', '', page => page.keyboard.type('hello')],
      ['code: Enter', '#ebox .wk-code-editor .cm-content', 'code', 'abc', async page => { await page.keyboard.press('ArrowLeft'); await page.keyboard.press('Enter'); }],
      ['code: Backspace', '#ebox .wk-code-editor .cm-content', 'code', 'abc', page => page.keyboard.press('Backspace')],
      ['rich: typing', '#ebox .wk-richtext-editor .ProseMirror', 'rich', '', page => page.keyboard.type('hello')],
      ['rich: Enter', '#ebox .wk-richtext-editor .ProseMirror', 'rich', 'abc', async page => { await page.keyboard.press('ArrowLeft'); await page.keyboard.press('Enter'); }],
      ['rich: Backspace', '#ebox .wk-richtext-editor .ProseMirror', 'rich', 'abc', page => page.keyboard.press('Backspace')],
      ['rich: bold then typing', '#ebox .wk-richtext-editor .ProseMirror', 'rich', 'abc', async page => { await page.keyboard.press('Control+b'); await page.keyboard.type('x'); }],
    ];
    // A plain contenteditable, which has no history of its own: the browser's
    // undo, and still never the app's stack.
    await page.evaluate(() => {
      const ce = document.createElement('div');
      ce.id = 't-ce';
      ce.contentEditable = 'true';
      document.getElementById('ebox').append(ce);
      Object.defineProperty(window, 'ce', { get: () => ({ get value() { return document.getElementById('t-ce').textContent; }, set value(v) { document.getElementById('t-ce').textContent = v; } }) });
    });
    cases.push(['contenteditable: typing', '#t-ce', 'ce', '', page => page.keyboard.type('hello')]);
    for (const [name, sel, which, start, act] of cases) {
      await page.evaluate(([w, v]) => { window[w].value = v; }, [which, start]);
      await page.click(sel);
      await page.keyboard.press('Control+End');
      await page.waitForTimeout(600); // the editors' history groups by time
      const before = await page.evaluate(w => window[w].value, which);
      await act(page);
      const edited = await page.evaluate(w => window[w].value, which);
      assert.notEqual(edited, before, `${name}: the keys changed it`);
      await page.evaluate(() => { go.calls.length = 0; menu('undo'); });
      await page.waitForTimeout(50);
      assert.equal(await page.evaluate(w => window[w].value, which), before, `${name}: Edit > Undo undoes it in the editor`);
      await page.evaluate(() => menu('redo'));
      await page.waitForTimeout(50);
      assert.equal(await page.evaluate(w => window[w].value, which), edited, `${name}: Edit > Redo redoes it`);
      // Nothing left in the editor to undo: still not the app's action.
      await page.evaluate(() => { menu('undo'); menu('undo'); menu('undo'); });
      await page.waitForTimeout(50);
      assert.deepEqual(await page.evaluate(() => go.calls.filter(c => !c.startsWith('focus'))), [], `${name}: the app's stack is never touched`);
    }
    assert.equal(await page.evaluate(() => go.focus), true, 'focus in an editor is text focus');
    // Focus in an editor that isn't on editable content (a read-only code
    // editor, the rich-text toolbar) isn't text focus: Edit > Undo is the app's.
    await page.evaluate(async () => {
      const { wkCodeEditor } = await import('/editor/code.js');
      const ro = wkCodeEditor({ label: 'Read-only', value: 'fixed', readOnly: true });
      ro.id = 't-ro';
      document.getElementById('ebox').append(ro);
    });
    for (const [name, sel] of [['read-only code editor', '#t-ro .cm-content'], ['rich-text toolbar button', '#ebox .wk-richtext-editor-toolbar button']]) {
      await page.evaluate(s => { const el = document.querySelector(s); if (!el.matches('button') && !el.hasAttribute('tabindex')) el.tabIndex = -1; el.focus(); }, sel);
      await page.waitForTimeout(50);
      assert.equal(await page.evaluate(s => document.activeElement === document.querySelector(s), sel), true, `${name}: focused`);
      assert.equal(await page.evaluate(() => go.focus), false, `${name}: no text focus reported`);
      await page.evaluate(() => { go.calls.length = 0; menu('undo'); });
      await page.waitForTimeout(50);
      assert.deepEqual(await page.evaluate(() => go.calls.filter(c => !c.startsWith('focus'))), ['Undo'], `${name}: Edit > Undo goes to the app's stack`);
    }
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('toast: it follows a modal that opens or closes, F6 reaches it there, and Esc in it inside a dialog or a task\'s scope dismisses only the toast; F6 with nowhere to go is left to the page', async () => {
    const { page, problems, run, taskEvents } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<button class="wk-button" type="button" id="t-a">Archive</button><dialog id="t-dlg"><button type="button" id="t-in">Inside</button></dialog>';
    });
    // Made inside a modal, which then closes: back in the body, reachable.
    await page.evaluate(() => { document.getElementById('t-dlg').showModal(); document.getElementById('t-in').focus(); kit.wkToast({ words: 'Saved', action: { label: 'Undo', run() {} } }); });
    assert.equal(await page.evaluate(() => document.querySelector('.wk-toast').parentElement.id), 't-dlg');
    await page.evaluate(() => document.querySelector('.wk-toast').getAnimations().forEach(x => x.finish()));
    await page.evaluate(() => { document.getElementById('t-dlg').close(); document.getElementById('t-a').focus(); });
    await run(10);
    assert.equal(await page.evaluate(() => document.querySelector('.wk-toast').parentElement === document.body), true, 'back in the body when the modal closes');
    assert.deepEqual(await page.evaluate(() => { const t = document.querySelector('.wk-toast'); return [t.getAnimations().length, getComputedStyle(t).opacity]; }), [0, '1'], 'moved without its entry animation again');
    await page.keyboard.press('F6');
    assert.equal(await active(page), 'Undo');
    await page.keyboard.press('F6');
    // Made in the body, then a modal opens: it moves into the modal.
    await page.evaluate(() => { document.getElementById('t-dlg').showModal(); document.getElementById('t-in').focus(); });
    await run(10);
    assert.equal(await page.evaluate(() => document.querySelector('.wk-toast').parentElement.id), 't-dlg', 'in the modal that opened over it');
    await page.keyboard.press('F6');
    assert.equal(await active(page), 'Undo', 'F6 reaches it in the modal');
    // Esc in it dismisses the toast, not the dialog.
    await page.keyboard.press('Escape');
    await run(10);
    assert.deepEqual(await page.evaluate(() => [document.getElementById('t-dlg').open, document.activeElement.id]), [true, 't-in']);
    await page.evaluate(() => document.getElementById('t-dlg').close());
    await run(200);
    // Esc in it doesn't cancel a task whose control had focus.
    await page.evaluate(() => {
      document.getElementById('t-a').focus();
      window.t = kit.wkTask({ id: 'esc', control: document.getElementById('t-a'), words: 'Archiving', stop: 'cancel', onStop() { return true; } });
      kit.wkToast({ words: 'Saved', action: { label: 'Undo', run() {} } });
    });
    await page.keyboard.press('F6');
    await page.keyboard.press('Escape');
    await run(10);
    assert.deepEqual(await taskEvents(), ['esc:running'], 'the task keeps running');
    assert.equal(await active(page), 't-a');
    await page.evaluate(() => window.t.done('Archived'));
    // The document is watched for modals only while a toast is shown.
    await page.evaluate(() => {
      window.mo = { observe: 0, disconnect: 0 };
      const o = MutationObserver.prototype.observe, d = MutationObserver.prototype.disconnect;
      MutationObserver.prototype.observe = function (...a) { if (a[0] === document.documentElement && a[1]?.attributeFilter?.includes('open') && !a[1].attributeFilter.includes('class')) window.mo.observe++; return o.apply(this, a); };
      MutationObserver.prototype.disconnect = function () { window.mo.disconnect++; return d.apply(this); };
      window.moRestore = () => { MutationObserver.prototype.observe = o; MutationObserver.prototype.disconnect = d; };
    });
    await run(20_000);
    const before = await page.evaluate(() => ({ ...window.mo }));
    await page.evaluate(() => kit.wkToast({ words: 'Saved' }));
    await run(6200);
    const after = await page.evaluate(() => { window.moRestore(); return { ...window.mo }; });
    assert.equal(after.observe - before.observe, 1, 'watched while shown');
    assert.ok(after.disconnect - before.disconnect >= 1, 'not watched once it has gone');
    // F6 with nowhere to go (no action, or an action that can't take focus) is left to the page.
    const f6 = async () => {
      await page.evaluate(() => { window.f6 = null; window.addEventListener('keydown', e => { if (e.key === 'F6') window.f6 = e.defaultPrevented; }, { once: true }); });
      await page.keyboard.press('F6');
      return page.evaluate(() => window.f6);
    };
    await page.evaluate(() => kit.wkToast({ words: 'Link copied' }));
    assert.equal(await f6(), false, 'no action');
    await page.evaluate(() => { kit.wkToast({ words: 'Saved', action: { label: 'Undo', run() {} } }).element.inert = true; });
    assert.equal(await f6(), false, 'an action that can\'t take focus');
    assert.equal(await active(page), 't-a');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('copy: Esc hides the tip only from the button; elsewhere it reaches the page; failure is assertive only with focus on the button; a writer resolving false failed; forced colours outline the tip', async () => {
    const { page, problems, run, said } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').style.setProperty('padding-top', '60px');
      document.getElementById('tbox').innerHTML = '<button class="wk-copy" type="button" id="t-copy" aria-label="Copy join code" data-wk-copy="K7"></button><dialog id="t-dlg"><button type="button" id="t-in">Inside</button><button class="wk-copy" type="button" id="t-copy2" aria-label="Copy code" data-wk-copy="K8"></button></dialog>';
      kit.wkCopy.write = async () => {};
    });
    const tips = () => page.evaluate(() => document.querySelectorAll('.wk-copy-tip').length);
    // In a dialog, focus moved on: Esc closes the dialog, as it should.
    await page.evaluate(() => { document.getElementById('t-dlg').showModal(); const b = document.getElementById('t-copy2'); b.focus(); b.click(); });
    await run(10);
    // Focus on the button: Esc hides the tip, and the dialog stays.
    await page.keyboard.press('Escape');
    await run(10);
    assert.deepEqual([await tips(), await page.evaluate(() => document.getElementById('t-dlg').open)], [0, true], 'Esc on the button hides the tip only');
    await page.evaluate(() => document.getElementById('t-copy2').click());
    await run(10);
    await page.evaluate(() => document.getElementById('t-in').focus());
    await page.keyboard.press('Escape');
    await run(10);
    assert.equal(await page.evaluate(() => document.getElementById('t-dlg').open), false, 'Esc away from the button closes the dialog');
    await run(2000);
    // With focus on the button: Esc hides the tip and goes no further.
    await page.$eval('#t-copy', b => { b.focus(); b.click(); });
    await run(10);
    assert.equal(await tips(), 1);
    await page.keyboard.press('Escape');
    assert.equal(await tips(), 0);
    await run(600);
    await said();
    // Failure with focus elsewhere: polite. A writer resolving false failed.
    await page.evaluate(() => { kit.wkCopy.write = async () => false; document.activeElement.blur(); kit.wkCopy(document.getElementById('t-copy'), 'K7'); });
    await run(600);
    assert.deepEqual(await said(), [['polite', 'Couldn’t copy']]);
    assert.equal(await page.evaluate(() => document.querySelector('.wk-copy-tip').textContent), 'Couldn’t copy');
    await run(2000);
    // Forced colours: the tip has a CanvasText border.
    await page.emulateMedia({ forcedColors: 'active' });
    await page.evaluate(() => { kit.wkCopy.write = async () => {}; kit.wkCopy(document.getElementById('t-copy'), 'K7'); });
    await run(10);
    const fc = await page.evaluate(() => { const s = getComputedStyle(document.querySelector('.wk-copy-tip')); const p = document.createElement('i'); p.style.setProperty('color', 'CanvasText'); document.body.append(p); const c = getComputedStyle(p).color; p.remove(); return [s.borderTopStyle, s.borderTopColor === c]; });
    assert.deepEqual(fc, ['solid', true]);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('skeleton rows: focus in the rows that give way moves to the table, not the page', async () => {
    const { page, problems, run } = await openTasks();
    await page.evaluate(() => {
      document.getElementById('tbox').innerHTML = '<table class="wk-table" id="t-runs" aria-label="Runs"><thead><tr><th>Name</th></tr></thead><tbody><tr><td><button type="button" id="t-row">build</button></td></tr></tbody></table>';
      document.getElementById('t-row').focus();
      window.load = kit.wkSkeleton(document.querySelector('#t-runs tbody'));
    });
    await run(400);
    assert.equal(await page.evaluate(() => document.activeElement.id), 't-runs');
    await page.evaluate(() => { window.load.done(() => {}); });
    await run(500);
    await page.evaluate(() => document.getElementById('t-a')?.focus() ?? document.body.focus());
    await page.keyboard.press('Tab');
    assert.equal(await page.evaluate(() => document.getElementById('t-runs').hasAttribute('tabindex')), false, 'tabindex dropped once focus leaves');
    assert.deepEqual(problems, []);
    await page.close();
  });

} finally { await browser.close(); await new Promise(resolve => server.close(resolve)); }

if (failures.length) {
  console.log(`Kit browser checks: ${failures.length} failed.`);
  process.exit(1);
}
console.log('Kit browser checks passed.');
