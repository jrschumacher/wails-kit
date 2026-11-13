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
    const data = await readFile(path.join(root, pathname));
    res.setHeader('Content-Type', types[path.extname(pathname)] || 'application/octet-stream');
    res.end(data);
  } catch { res.writeHead(404).end(); }
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
const origin = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.launch({ executablePath: chrome, headless: true });

async function open(url = '/demo/index.html', { ready, ...options } = {}) {
  const page = await browser.newPage({ viewport: { width: 1100, height: 800 }, colorScheme: 'light', ...options });
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
        if (url === '/demo/themed.html') assert.match(font, /^"?Avenir Next"?,/);
        else assert.match(font, /^system-ui,/);
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

  await check('data-theme wins over the system appearance, for the tokens, color-scheme and the backdrop', async () => {
    for (const url of ['/demo/index.html', '/demo/themed.html']) {
      for (const [scheme, attr, want] of [['dark', 'light', 'light'], ['light', 'dark', 'dark'], ['dark', null, 'dark'], ['light', null, 'light']]) {
        const { page, problems } = await open(url, { colorScheme: scheme });
        await page.evaluate(a => { if (a) document.documentElement.dataset.theme = a; else delete document.documentElement.dataset.theme; }, attr);
        const look = await page.evaluate(() => {
          const root = getComputedStyle(document.documentElement);
          const d = document.querySelector('#demo-dialog dialog'); d.showModal();
          const backdrop = getComputedStyle(d, '::backdrop').backgroundColor; d.close();
          return { bg: root.backgroundColor, scheme: root.colorScheme, fill: root.getPropertyValue('--wk-color-attention-fill').trim(), backdrop };
        });
        const dark = want === 'dark';
        assert.equal(look.bg, dark ? 'rgb(24, 26, 31)' : 'rgb(252, 252, 251)', `${url} system ${scheme}, data-theme ${attr}`);
        assert.equal(look.scheme, want, `${url} color-scheme, system ${scheme}, data-theme ${attr}`);
        assert.equal(look.backdrop, dark ? 'rgba(0, 0, 0, 0.5)' : 'rgba(15, 20, 30, 0.28)');
        if (url === '/demo/themed.html') assert.equal(look.fill.replace(/\s+/g, ''), dark ? 'rgba(238,107,95,.12)' : 'rgba(196,55,43,.08)');
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
    const closed = await chevron('button.wk-link[aria-expanded=false]'), open_ = await chevron('button.wk-link[aria-expanded=true]');
    assert.notEqual(closed.content, 'none'); assert.notEqual(closed.transform, open_.transform);
    assert.equal(await page.locator('a.wk-link').first().evaluate(el => getComputedStyle(el, '::before').content), 'none');
    for (const sel of ['.wk-codeblock--scroll > pre', '.wk-errorbox > pre', '#demo-long-sheet .wk-dialog-scroll']) {
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
} finally { await browser.close(); await new Promise(resolve => server.close(resolve)); }

if (failures.length) {
  console.log(`Kit browser checks: ${failures.length} failed.`);
  process.exit(1);
}
console.log('Kit browser checks passed.');
