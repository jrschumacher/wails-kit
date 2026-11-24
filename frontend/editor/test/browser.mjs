// Browser checks for the editors, on the review pages in demo/ (index.html
// and themed.html for the code editor, markdown.html for the Markdown
// editor), which load the prebuilt dist/ modules as an app vendors them. Run with: npm run test:browser
// It needs playwright-core (a devDependency) and Chrome: set CHROME_PATH, or
// have Chrome at /usr/bin/google-chrome or the macOS default. Without Chrome
// it skips, unless CI is set, when it fails.
//
// It serves the frontend/ folder, since the pages link @wails-kit/ui's
// stylesheet from ../../ui/. Any request outside this server, any console
// error or warning (a CSP violation among them) and any failed request fails
// the check it happens in. Each check runs on its own page and reports on its
// own line. Waits are for conditions, not for time.
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
const types = { '.html': 'text/html', '.js': 'text/javascript', '.mjs': 'text/javascript', '.css': 'text/css' };
const server = http.createServer(async (req, res) => {
  try {
    const pathname = new URL(req.url, 'http://localhost').pathname;
    if (pathname.includes('..') || !/^\/(editor|ui)\//.test(pathname) || pathname.includes('/node_modules/')) { res.writeHead(404).end(); return; }
    const data = await readFile(path.join(root, pathname));
    res.setHeader('Content-Type', types[path.extname(pathname)] || 'application/octet-stream');
    res.end(data);
  } catch { res.writeHead(404).end(); }
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
const origin = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.launch({ executablePath: chrome, headless: true });
const mod = process.platform === 'darwin' ? 'Meta' : 'Control';

async function open(url = '/editor/demo/index.html', options = {}) {
  const page = await browser.newPage({ viewport: { width: 1100, height: 800 }, colorScheme: 'light', ...options });
  const problems = [];
  page.on('pageerror', e => problems.push(`pageerror: ${e.message}`));
  page.on('console', m => { if (m.type() === 'error' || m.type() === 'warning') problems.push(`console ${m.type()}: ${m.text()}`); });
  page.on('request', r => { if (!r.url().startsWith(origin + '/') && !r.url().startsWith('data:')) problems.push(`network: ${r.url()}`); });
  page.on('requestfailed', r => problems.push(`failed: ${r.url()}`));
  page.on('response', r => { if (r.status() >= 400) problems.push(`HTTP ${r.status()}: ${r.url()}`); });
  await page.goto(origin + url);
  await page.waitForFunction(() => window.demoReady === true);
  await page.evaluate(() => document.fonts.ready);
  // Helpers in the page: a token's colour, an element's colours, WCAG contrast.
  await page.evaluate(() => {
    const probe = document.createElement('span');
    probe.className = 'wk-test-probe';
    document.body.append(probe);
    window.t = {
      colour(value) { probe.style.color = value; return getComputedStyle(probe).color; },
      rgb: c => c.match(/[\d.]+/g).slice(0, 3).map(Number),
      contrast(a, b) {
        const lum = c => { const [r, g, bl] = window.t.rgb(c).map(x => { x /= 255; return x <= .03928 ? x / 12.92 : ((x + .055) / 1.055) ** 2.4; }); return .2126 * r + .7152 * g + .0722 * bl; };
        const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p);
        return (x + .05) / (y + .05);
      },
    };
  });
  return { page, problems };
}
const until = (page, fn, arg) => page.waitForFunction(fn, arg, { timeout: 3000 });
const value = (page, id) => page.evaluate(i => document.getElementById(i).value, id);
const focusIn = (page, id) => page.evaluate(i => document.getElementById(i).contains(document.activeElement), id);
const setTheme = (page, theme) => page.evaluate(v => { if (v) document.documentElement.dataset.theme = v; else delete document.documentElement.dataset.theme; }, theme);
// A new editor on the page, from the dist module, in #demo-scratch.
const make = (page, spec) => page.evaluate(async s => {
  const { wkCodeEditor } = await import('/editor/dist/code.js');
  let host = document.getElementById('demo-scratch');
  if (!host) { host = document.createElement('div'); host.id = 'demo-scratch'; document.getElementById('demo').append(host); }
  window.calls = [];
  const el = wkCodeEditor({ ...s, onchange: v => window.calls.push(['onchange', v]) });
  el.addEventListener('wk-change', e => window.calls.push(['wk-change', e.detail.value]));
  host.replaceChildren(el);
  return el.id;
}, spec);

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

const NEUTRAL = { light: { bg: 'rgb(255, 255, 255)', accent: 'rgb(61, 68, 82)' }, dark: { bg: 'rgb(38, 42, 49)', accent: 'rgb(195, 201, 212)' } };
const EXAMPLE = { light: { bg: 'rgb(255, 255, 255)', accent: 'rgb(31, 107, 85)' }, dark: { bg: 'rgb(38, 42, 49)', accent: 'rgb(127, 207, 180)' } };

try {
  await check('the demo pages load from the local server with no errors, CSP violations or network (light and dark; neutral and themed)', async () => {
    for (const url of ['/editor/demo/index.html', '/editor/demo/themed.html']) {
      for (const scheme of ['light', 'dark']) {
        const { page, problems } = await open(url, { colorScheme: scheme });
        const look = await page.evaluate(() => ({
          editors: document.querySelectorAll('.wk-code-editor > .cm-editor').length,
          // CodeMirror's own styles applied (they come from an adopted sheet, not a <style> element).
          display: getComputedStyle(document.querySelector('.cm-editor')).display,
          adopted: document.adoptedStyleSheets.length,
          styleTags: document.querySelectorAll('style').length,
          font: getComputedStyle(document.querySelector('.cm-scroller')).fontFamily,
          overflow: document.documentElement.scrollWidth > innerWidth,
        }));
        assert.equal(look.editors, 14, `${url} ${scheme}`);
        assert.equal(look.display, 'flex');
        assert.ok(look.adopted >= 1);
        assert.equal(look.styleTags, 0);
        assert.match(look.font, /^ui-monospace,/);
        assert.equal(look.overflow, false);
        assert.deepEqual(problems, [], `${url} (${scheme})`);
        await page.close();
      }
    }
  });

  await check('theme: the editor takes the kit tokens and follows data-theme and an app theme', async () => {
    for (const [url, want] of [['/editor/demo/index.html', NEUTRAL], ['/editor/demo/themed.html', EXAMPLE]]) {
      for (const [scheme, attr, appearance] of [['light', null, 'light'], ['dark', null, 'dark'], ['dark', 'light', 'light'], ['light', 'dark', 'dark']]) {
        const { page, problems } = await open(url, { colorScheme: scheme });
        await setTheme(page, attr);
        await page.click('#demo-editor .cm-content');
        const look = await page.evaluate(() => {
          const editor = document.querySelector('#demo-editor .cm-editor');
          const s = getComputedStyle(editor);
          const gutter = getComputedStyle(editor.querySelector('.cm-gutters'));
          const cursor = editor.querySelector('.cm-cursor');
          const sel = getComputedStyle(document.documentElement).getPropertyValue('--wk-color-selection');
          return {
            bg: s.backgroundColor, color: s.color, outline: `${s.outlineStyle} ${s.outlineColor}`,
            gutter: gutter.color, gutterImage: gutter.backgroundImage,
            cursor: cursor && getComputedStyle(cursor).borderLeftColor,
            selection: window.t.colour(sel), text: window.t.colour('var(--wk-color-text)'), secondary: window.t.colour('var(--wk-color-text-secondary)'),
          };
        });
        const w = want[appearance];
        const label = `${url} system ${scheme}, data-theme ${attr}`;
        assert.equal(look.bg, w.bg, label);
        assert.equal(look.color, look.text, label);
        assert.equal(look.outline, `solid ${w.accent}`, label);
        assert.equal(look.cursor, w.accent, label);
        assert.equal(look.gutter, look.secondary, label);
        assert.match(look.gutterImage, /linear-gradient/);
        // The selection layer uses the (theme's) selection token.
        await page.keyboard.press(`${mod}+a`);
        await until(page, () => document.querySelector('#demo-editor .cm-selectionBackground'));
        const selBg = await page.evaluate(() => getComputedStyle(document.querySelector('#demo-editor .cm-selectionBackground')).backgroundColor);
        assert.equal(selBg, look.selection, label);
        assert.deepEqual(problems, []);
        await page.close();
      }
    }
  });

  await check('syntax colours and gutter numbers reach 4.5:1 on the editor, in light and dark; each syntax token is used', async () => {
    for (const scheme of ['light', 'dark']) {
      const { page, problems } = await open('/editor/demo/index.html', { colorScheme: scheme });
      const result = await page.evaluate(() => {
        const out = {};
        const spans = [...document.querySelectorAll('.wk-code-editor .cm-line span')];
        for (const name of ['keyword', 'string', 'number', 'comment', 'type', 'function']) {
          const colour = window.t.colour(`var(--wk-color-code-${name})`);
          const span = spans.find(s => getComputedStyle(s).color === colour);
          const bg = span && getComputedStyle(span.closest('.cm-editor')).backgroundColor;
          out[name] = span ? window.t.contrast(colour, bg) : 0;
        }
        const editor = document.querySelector('#demo-editor .cm-editor');
        const g = editor.querySelector('.cm-lineNumbers .cm-gutterElement:not(.cm-activeLineGutter):nth-child(3)');
        // The gutter's ground: the fill laid over the control colour.
        const fill = window.t.colour('var(--wk-color-fill)').match(/[\d.]+/g).map(Number);
        const control = window.t.rgb(getComputedStyle(editor).backgroundColor);
        const a = fill[3] ?? 1;
        const ground = `rgb(${control.map((c, i) => Math.round(fill[i] * a + c * (1 - a))).join(', ')})`;
        out.gutter = window.t.contrast(getComputedStyle(g).color, ground);
        return out;
      });
      for (const [name, ratio] of Object.entries(result)) {
        assert.ok(ratio >= 4.5, `${scheme}: ${name} is ${ratio.toFixed(2)}:1 (0 means no span has the colour)`);
      }
      assert.deepEqual(problems, []);
      await page.close();
    }
  });

  await check('typing, undo and redo; the value property', async () => {
    const { page, problems } = await open();
    const before = await value(page, 'demo-editor');
    await page.click('#demo-editor .cm-content');
    await page.keyboard.press(`${mod}+End`);
    await page.keyboard.type('\n// typed');
    assert.equal(await value(page, 'demo-editor'), `${before}\n// typed`);
    assert.equal(await page.textContent('#demo-mirror'), `${before.length + 9} characters`);
    await page.keyboard.press(`${mod}+z`);
    assert.equal(await value(page, 'demo-editor'), before);
    await page.keyboard.press(`${mod}+Shift+z`);
    assert.equal(await value(page, 'demo-editor'), `${before}\n// typed`);
    await page.keyboard.press(`${mod}+z`);
    await page.keyboard.press(`${mod}+y`);
    assert.equal(await value(page, 'demo-editor'), `${before}\n// typed`);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('onchange and wk-change fire on each edit with the text; setting value replaces it and fires neither', async () => {
    const { page, problems } = await open();
    await make(page, { id: 'scratch', label: 'Scratch', value: 'a' });
    await page.click('#scratch .cm-content');
    await page.keyboard.press('End');
    await page.keyboard.type('bc');
    assert.deepEqual(await page.evaluate(() => window.calls), [['wk-change', 'ab'], ['onchange', 'ab'], ['wk-change', 'abc'], ['onchange', 'abc']]);
    // The demo's document-level listener sees the event bubble.
    assert.deepEqual(await page.evaluate(() => window.kitEvents.at(-1)), { type: 'wk-change', target: 'scratch', value: 'abc' });
    await page.evaluate(() => { window.calls = []; document.getElementById('scratch').value = 'new\ntext'; });
    assert.equal(await value(page, 'scratch'), 'new\ntext');
    assert.equal(await page.evaluate(() => document.querySelectorAll('#scratch .cm-line').length), 2);
    assert.deepEqual(await page.evaluate(() => window.calls), []);
    // A load is not undoable.
    await page.click('#scratch .cm-content');
    await page.keyboard.press(`${mod}+z`);
    assert.equal(await value(page, 'scratch'), 'new\ntext');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('a throwing onchange does not swallow wk-change, which fires first', async () => {
    const { page, problems } = await open();
    const order = await page.evaluate(async () => {
      const { wkCodeEditor } = await import('/editor/dist/code.js');
      const order = [];
      const el = wkCodeEditor({ label: 'Throws', value: 'a', onchange: () => { order.push('onchange'); throw new Error('app bug'); } });
      el.addEventListener('wk-change', () => order.push('wk-change'));
      document.getElementById('demo').append(el);
      window.order = order;
      return el.isConnected;
    });
    assert.equal(order, true);
    await page.click('.wk-code-editor >> nth=-1');
    await page.keyboard.type('b');
    assert.deepEqual(await page.evaluate(() => window.order), ['wk-change', 'onchange']);
    // CodeMirror catches the callback's error and logs it; nothing else goes wrong.
    assert.ok(problems.length > 0 && problems.every(m => m.includes('app bug')), problems.join('\n'));
    await page.close();
  });

  await check('line numbers line up: each gutter row is as tall as its line, for editors scrolled into view', async () => {
    for (const scheme of ['light', 'dark']) {
      const { page, problems } = await open('/editor/demo/index.html', { colorScheme: scheme });
      const ids = await page.evaluate(() => [...document.querySelectorAll('.wk-code-editor')].filter(e => e.querySelector('.cm-lineNumbers')).map(e => e.id));
      assert.ok(ids.length >= 10);
      for (const id of ids) {
        await page.locator(`#${id}`).scrollIntoViewIfNeeded();
        // An editor measures itself once it is in view (CodeMirror's design); wait for that.
        await until(page, i => {
          const el = document.getElementById(i);
          // The gutter's first row is a hidden spacer that sizes its width.
          const rows = [...el.querySelectorAll('.cm-lineNumbers .cm-gutterElement')].filter(g => getComputedStyle(g).visibility !== 'hidden');
          const lines = [...el.querySelectorAll('.cm-line')];
          return rows.length === lines.length && rows.every((g, k) => Math.abs(g.getBoundingClientRect().height - lines[k].getBoundingClientRect().height) < 0.5);
        }, id).catch(async () => {
          const got = await page.evaluate(i => {
            const el = document.getElementById(i);
            return { rows: [...el.querySelectorAll('.cm-lineNumbers .cm-gutterElement')].filter(g => getComputedStyle(g).visibility !== 'hidden').map(g => g.getBoundingClientRect().height), lines: [...el.querySelectorAll('.cm-line')].map(l => l.getBoundingClientRect().height) };
          }, id);
          assert.fail(`${scheme} #${id}: gutter rows ${JSON.stringify(got.rows)} vs lines ${JSON.stringify(got.lines)}`);
        });
      }
      assert.deepEqual(problems, []);
      await page.close();
    }
  });

  await check('the current line number stands out only while the editor has focus', async () => {
    const { page, problems } = await open();
    const look = () => page.evaluate(() => {
      const g = document.querySelector('#demo-editor .cm-activeLineGutter');
      const other = document.querySelector('#demo-editor .cm-lineNumbers .cm-gutterElement:not(.cm-activeLineGutter):nth-child(3)');
      return { active: getComputedStyle(g).color, other: getComputedStyle(other).color, weight: getComputedStyle(g).fontWeight, otherWeight: getComputedStyle(other).fontWeight, text: window.t.colour('var(--wk-color-text)') };
    });
    let l = await look();
    assert.equal(l.active, l.other, 'the active line number stands out without focus');
    assert.equal(l.weight, l.otherWeight);
    await page.click('#demo-editor .cm-content');
    l = await look();
    assert.equal(l.active, l.text);
    assert.equal(l.weight, l.otherWeight, 'the active line number is bold');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('readOnly blocks edits but keeps focus, selection and find, without Replace', async () => {
    const { page, problems } = await open();
    const before = await value(page, 'demo-readonly');
    assert.equal(await page.getAttribute('#demo-readonly .cm-content', 'aria-readonly'), 'true');
    await page.click('#demo-readonly .cm-content');
    assert.equal(await focusIn(page, 'demo-readonly'), true);
    await page.keyboard.type('xyz');
    await page.keyboard.press('Backspace');
    await page.keyboard.press('Enter');
    await page.keyboard.press(`${mod}+a`);
    await page.keyboard.press('Delete');
    assert.equal(await value(page, 'demo-readonly'), before);
    assert.equal(await page.evaluate(() => window.kitEvents.length), 0);
    await page.keyboard.press(`${mod}+f`);
    await until(page, () => document.querySelector('#demo-readonly .wk-code-search'));
    assert.equal(await page.locator('#demo-readonly .wk-code-search [name=replace]').count(), 0);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('accessibility: a labelled multi-line textbox; the label is required; the gutters are hidden', async () => {
    const { page, problems } = await open();
    const attrs = await page.evaluate(() => {
      const c = document.querySelector('#demo-editor .cm-content');
      return { role: c.getAttribute('role'), multi: c.getAttribute('aria-multiline'), label: c.getAttribute('aria-label'), gutters: document.querySelector('#demo-editor .cm-gutters').getAttribute('aria-hidden') };
    });
    assert.deepEqual(attrs, { role: 'textbox', multi: 'true', label: 'TypeScript source', gutters: 'true' });
    const cdp = await page.context().newCDPSession(page);
    const { nodes } = await cdp.send('Accessibility.getFullAXTree');
    await cdp.detach();
    assert.ok(nodes.some(n => !n.ignored && n.role?.value === 'textbox' && n.name?.value === 'TypeScript source'), 'no textbox named "TypeScript source" in the accessibility tree');
    const errors = await page.evaluate(async () => {
      const { wkCodeEditor } = await import('/editor/dist/code.js');
      const out = [];
      for (const spec of [{}, { label: '  ' }, { label: 'x', language: 'cobol' }]) {
        try { wkCodeEditor(spec); out.push('no error'); } catch (e) { out.push(e.constructor.name); }
      }
      return out;
    });
    assert.deepEqual(errors, ['TypeError', 'TypeError', 'TypeError']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('focus: Tab leaves the editor and Shift+Tab comes back; with tabIndents, Tab indents and Escape then Tab leaves', async () => {
    const { page, problems } = await open();
    await page.click('#demo-editor .cm-content');
    const before = await value(page, 'demo-editor');
    await page.keyboard.press('Tab');
    assert.equal(await focusIn(page, 'demo-editor'), false, 'Tab stayed in the editor');
    assert.equal(await value(page, 'demo-editor'), before, 'Tab changed the text');
    await page.keyboard.press('Shift+Tab');
    assert.equal(await focusIn(page, 'demo-editor'), true, 'Shift+Tab did not come back');
    // tabIndents: the wrapped Markdown editor.
    const md = await value(page, 'demo-wrap');
    await page.click('#demo-wrap .cm-line >> nth=0');
    await page.keyboard.press('Home');
    await page.keyboard.press('Tab');
    assert.equal(await focusIn(page, 'demo-wrap'), true);
    assert.notEqual(await value(page, 'demo-wrap'), md, 'Tab did not indent');
    await page.keyboard.press('Shift+Tab');
    assert.equal(await value(page, 'demo-wrap'), md, 'Shift+Tab did not dedent');
    await page.keyboard.press('Escape');
    await page.keyboard.press('Tab');
    assert.equal(await focusIn(page, 'demo-wrap'), false, 'Escape then Tab stayed in the editor');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('find: Mod-F opens the kit search panel, finds and replaces; Escape closes it and returns to the text', async () => {
    const { page, problems } = await open();
    await page.click('#demo-editor .cm-content');
    await page.keyboard.press(`${mod}+f`);
    await until(page, () => document.activeElement?.matches('#demo-editor .wk-code-search input.wk-field[name=search]'));
    const panel = await page.evaluate(() => {
      const p = document.querySelector('#demo-editor .wk-code-search');
      return {
        role: p.getAttribute('role'), label: p.querySelector('[name=search]').getAttribute('aria-label'),
        buttons: [...p.querySelectorAll('button')].map(b => `${b.className.includes('wk-button') ? 'kit' : 'other'}:${b.getAttribute('aria-label') || b.textContent}`),
        checks: [...p.querySelectorAll('label.wk-check > input.wk-checkbox')].length,
      };
    });
    assert.deepEqual(panel, { role: 'search', label: 'Find', buttons: ['kit:Next', 'kit:Previous', 'kit:Select all matches', 'kit:Close find', 'kit:Replace', 'kit:Replace all'], checks: 3 });
    assert.equal(await page.textContent('#demo-editor .wk-code-search [aria-label="Select all matches"]'), 'All');
    await page.keyboard.type('handler');
    await until(page, () => document.querySelectorAll('#demo-editor .cm-searchMatch').length === 3);
    await page.keyboard.press('Enter');
    await until(page, () => document.querySelectorAll('#demo-editor .cm-searchMatch-selected').length === 1);
    await page.fill('#demo-editor [name=replace]', 'onEvent');
    await page.click('#demo-editor .wk-code-search button:text-is("Replace all")');
    assert.equal((await value(page, 'demo-editor')).match(/handler/g), null);
    await page.focus('#demo-editor [name=search]');
    await page.keyboard.press('Escape');
    await until(page, () => !document.querySelector('#demo-editor .wk-code-search'));
    assert.equal(await page.evaluate(() => document.activeElement.classList.contains('cm-content')), true);
    // Setting value while the panel has focus closes it and leaves focus in the text, not on <body>.
    await page.keyboard.press(`${mod}+f`);
    await until(page, () => document.activeElement?.matches('#demo-editor [name=search]'));
    await page.evaluate(() => { document.getElementById('demo-editor').value = 'loaded'; });
    assert.equal(await page.evaluate(() => document.activeElement.matches('#demo-editor .cm-content')), true);
    // Not when focus was elsewhere.
    await page.focus('#demo-theme input');
    await page.evaluate(() => { document.getElementById('demo-editor').value = 'loaded again'; });
    assert.equal(await page.evaluate(() => document.activeElement.matches('#demo-theme input')), true);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('forced colours: native selection and caret in system colours; the selection stays visible', async () => {
    const { page, problems } = await open('/editor/demo/index.html', { forcedColors: 'active' });
    assert.equal(await page.evaluate(() => matchMedia('(forced-colors: active)').matches), true);
    await page.click('#demo-editor .cm-content');
    const look = await page.evaluate(() => ({
      layer: document.querySelectorAll('#demo-editor .cm-selectionLayer').length,
      caret: getComputedStyle(document.querySelector('#demo-editor .cm-content')).caretColor,
      canvasText: window.t.colour('CanvasText'),
      ring: getComputedStyle(document.querySelector('#demo-editor .cm-editor')).outlineStyle,
    }));
    assert.equal(look.layer, 0, 'CodeMirror still draws its own selection');
    assert.equal(look.caret, look.canvasText);
    assert.equal(look.ring, 'solid');
    const editor = page.locator('#demo-editor .cm-content');
    const plain = await editor.screenshot({ caret: 'hide' });
    await page.keyboard.press(`${mod}+a`);
    assert.ok((await page.evaluate(() => getSelection().toString())).includes('typed event'));
    const selected = await editor.screenshot({ caret: 'hide' });
    assert.ok(!plain.equals(selected), 'the selection does not show in forced colours');
    // Leaving forced colours brings CodeMirror's drawn selection back.
    await page.emulateMedia({ forcedColors: 'none' });
    await until(page, () => document.querySelectorAll('#demo-editor .cm-selectionLayer').length === 1);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('highlighter: diff lines take the editor\'s syntax colours, the language from the path; unknown files stay plain', async () => {
    const { page, problems } = await open();
    const look = await page.evaluate(() => {
      const lines = [...document.querySelectorAll('#demo-highlight .demo-highlight-line')];
      const colourOf = (line, text) => { const s = [...line.querySelectorAll('span:not(.demo-highlight-marker)')].find(x => x.textContent === text); return s && getComputedStyle(s).color; };
      return {
        text: lines.map(l => l.textContent.slice(1).trimEnd()),
        func: colourOf(lines[0], 'func'), keyword: window.t.colour('var(--wk-color-code-keyword)'),
        str: colourOf(lines[1], '"not started"'), string: window.t.colour('var(--wk-color-code-string)'),
        comment: colourOf(lines[1], '// TODO'), commentToken: window.t.colour('var(--wk-color-code-comment)'),
        tsType: colourOf(lines[3], 'string'), type: window.t.colour('var(--wk-color-code-type)'),
        plainSpans: lines[5].querySelectorAll('span:not(.demo-highlight-marker)').length,
      };
    });
    assert.equal(look.text[0], 'func (s *Service) Start(ctx context.Context) error {');
    assert.equal(look.func, look.keyword);
    assert.equal(look.str, look.string);
    assert.equal(look.comment, look.commentToken);
    assert.equal(look.tsType, look.type);
    assert.equal(look.plainSpans, 0);
    const api = await page.evaluate(async () => {
      const { highlighter, languageForPath, languageName, languages } = await import('/editor/dist/code.js');
      // YAML: the key and the comment (a plain scalar such as 3 is untagged in the grammar).
      const frag = highlighter('yml')(null, 'retries: 3 # x');
      const auto = highlighter()({ oldPath: 'a/b.json' }, '{"a": 1}');
      let unknown;
      try { highlighter('cobol'); } catch (e) { unknown = e.constructor.name; }
      return {
        fragment: frag instanceof DocumentFragment, text: frag.textContent, spans: frag.querySelectorAll('span').length,
        auto: auto.querySelectorAll('span').length > 0, empty: highlighter()({ path: 'x.go' }, '').childNodes.length, unknown,
        paths: ['src/app.tsx', 'main.go', 'README.md', '.bashrc', 'Makefile', 'x.unknown'].map(languageForPath),
        name: languageName('YML'), languages,
      };
    });
    assert.deepEqual(api, {
      fragment: true, text: 'retries: 3 # x', spans: 2, auto: true, empty: 0, unknown: 'TypeError',
      paths: ['typescript', 'go', 'markdown', 'shell', null, null], name: 'yaml',
      languages: ['javascript', 'typescript', 'go', 'json', 'yaml', 'markdown', 'shell', 'css', 'html'],
    });
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('another language through a stream parser; options: no line numbers, wrapping, placeholder, tab size', async () => {
    const { page, problems } = await open();
    const look = await page.evaluate(async () => {
      const { wkCodeEditor } = await import('/editor/dist/code.js');
      const parser = { name: 'words', token(stream) { if (stream.match(/^\d+/)) return 'number'; stream.next(); return null; } };
      const el = wkCodeEditor({ label: 'Words', language: parser, value: 'abc 123', lineNumbers: false, wrap: true, tabSize: 2, placeholder: 'x' });
      document.getElementById('demo').append(el);
      const num = [...el.querySelectorAll('.cm-line span')].find(s => s.textContent === '123');
      const empty = wkCodeEditor({ label: 'Empty', placeholder: 'Write a note…' });
      document.getElementById('demo').append(empty);
      return {
        num: num && getComputedStyle(num).color, token: window.t.colour('var(--wk-color-code-number)'),
        gutters: el.querySelectorAll('.cm-gutters').length, wrapping: el.querySelector('.cm-content').classList.contains('cm-lineWrapping'),
        tab: getComputedStyle(el.querySelector('.cm-content')).tabSize, placeholder: empty.querySelector('.cm-placeholder')?.textContent,
      };
    });
    assert.equal(look.num, look.token);
    assert.equal(look.gutters, 0);
    assert.equal(look.wrapping, true);
    assert.equal(look.tab, '2');
    assert.equal(look.placeholder, 'Write a note…');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('destroy() tears the editor down and leaves the element empty', async () => {
    const { page, problems } = await open();
    await make(page, { id: 'scratch', label: 'Scratch', value: 'x' });
    const after = await page.evaluate(() => {
      const el = document.getElementById('scratch');
      el.destroy(); el.destroy();
      // After destroy, setting value and focus() do nothing (and do not throw).
      el.value = 'later';
      el.focus();
      return { children: el.childElementCount, editors: document.querySelectorAll('#scratch .cm-editor').length, value: el.value, focus: document.activeElement === document.body };
    });
    assert.deepEqual(after, { children: 0, editors: 0, value: 'x', focus: true });
    assert.deepEqual(problems, []);
    await page.close();
  });

  // ---- The Markdown editor (demo/markdown.html, dist/markdown.js) ------
  const MD = '/editor/demo/markdown.html';
  // Puts the comment box's text, focuses it with the cursor at the end, and
  // selects its last `select` characters (Shift+Left), or all of it (true).
  const mdSet = async (page, text, select = 0) => {
    await page.evaluate(t => { const el = document.getElementById('demo-comment'); el.value = t; el.focus(); }, text);
    await page.keyboard.press(`${mod}+End`);
    if (select === true) await page.keyboard.press(`${mod}+a`);
    else for (let i = 0; i < select; i++) await page.keyboard.press('Shift+ArrowLeft');
  };
  const mdValue = page => value(page, 'demo-comment');
  const mdSelected = page => page.evaluate(() => getSelection().toString());

  await check('markdown: the page loads with no errors, CSP violations or network; styles adopted, tokens used (light and dark)', async () => {
    for (const scheme of ['light', 'dark']) {
      const { page, problems } = await open(MD, { colorScheme: scheme });
      const look = await page.evaluate(() => {
        const md = document.getElementById('demo-rendered');
        const keyword = [...md.querySelectorAll('.wk-code span')].find(s => s.textContent === 'func');
        return {
          editors: document.querySelectorAll('.wk-markdown-editor').length,
          styleTags: document.querySelectorAll('style').length,
          adopted: document.adoptedStyleSheets.length,
          color: getComputedStyle(md).color, text: window.t.colour('var(--wk-color-text)'),
          keyword: keyword && getComputedStyle(keyword).color, keywordToken: window.t.colour('var(--wk-color-code-keyword)'),
          table: md.querySelector('table').className, th: getComputedStyle(md.querySelector('th')).color, secondary: window.t.colour('var(--wk-color-text-secondary)'),
          link: getComputedStyle(md.querySelector('a.wk-link')).color, accent: window.t.colour('var(--wk-color-accent)'),
          imgs: document.querySelectorAll('.wk-markdown img').length,
          margin: getComputedStyle(md.querySelector('p')).marginBottom,
          count: document.querySelector('#demo-comment .wk-markdown-editor-count').textContent,
          overflow: document.documentElement.scrollWidth > innerWidth,
        };
      });
      assert.equal(look.editors, 4);
      assert.equal(look.styleTags, 0);
      assert.ok(look.adopted >= 1);
      assert.equal(look.color, look.text, scheme);
      assert.equal(look.keyword, look.keywordToken, `${scheme}: code blocks are not highlighted`);
      assert.equal(look.table, 'wk-table');
      assert.equal(look.th, look.secondary);
      assert.equal(look.link, look.accent);
      assert.equal(look.imgs, 0, 'a remote image rendered as <img>');
      assert.notEqual(look.margin, '0px', 'paragraphs have no space between them');
      assert.match(look.count, /^\d+ \/ 1000$/);
      assert.equal(look.overflow, false);
      assert.deepEqual(problems, [], scheme);
      await page.close();
    }
  });

  await check('markdown: Write and Preview switch by keyboard (wk-tabs); the preview renders the current text; toolbar only in Write', async () => {
    const { page, problems } = await open(MD);
    await mdSet(page, '**bold** text');
    await page.focus('#demo-comment [role=tab]:text-is("Write")');
    await page.keyboard.press('ArrowRight');
    const look = await page.evaluate(() => {
      const el = document.getElementById('demo-comment');
      const tab = el.querySelector('[role=tab][aria-selected=true]');
      const panel = document.getElementById(tab.getAttribute('aria-controls'));
      return {
        mode: el.mode, tab: tab.textContent, focused: document.activeElement === tab, html: panel.querySelector('.wk-markdown').innerHTML,
        writeHidden: el.querySelector('.wk-code-editor').closest('[role=tabpanel]').hidden, toolbar: el.querySelector('[role=toolbar]').hidden,
        events: window.kitEvents.filter(e => e.target === 'demo-comment').map(e => [e.type, e.detail.mode]),
      };
    });
    assert.deepEqual(look, { mode: 'preview', tab: 'Preview', focused: true, html: '<p><strong>bold</strong> text</p>', writeHidden: true, toolbar: true, events: [['wk-mode', 'preview']] });
    await page.keyboard.press('ArrowLeft');
    assert.equal(await page.evaluate(() => document.getElementById('demo-comment').mode), 'write');
    assert.equal(await page.evaluate(() => document.querySelector('#demo-comment [role=toolbar]').hidden), false);
    // Tab goes from the tab into the toolbar (one stop), then the text.
    await page.keyboard.press('Tab');
    assert.equal(await page.evaluate(() => document.activeElement.getAttribute('aria-label')), 'Bold');
    await page.keyboard.press('ArrowRight');
    assert.equal(await page.evaluate(() => document.activeElement.getAttribute('aria-label')), 'Italic');
    await page.keyboard.press('Tab');
    assert.equal(await page.evaluate(() => document.activeElement.matches('#demo-comment .cm-content')), true);
    // An empty text previews as "Nothing to preview"; mode set by the app fires nothing.
    await page.evaluate(() => { window.kitEvents = []; const el = document.getElementById('demo-comment'); el.value = ''; el.mode = 'preview'; });
    assert.equal(await page.textContent('#demo-comment .wk-markdown-editor-preview'), 'Nothing to preview');
    assert.deepEqual(await page.evaluate(() => window.kitEvents), []);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('markdown: the shortcuts and the toolbar wrap the selection (bold, italic, code, link, list, quote)', async () => {
    const { page, problems } = await open(MD);
    const cases = [
      [`${mod}+b`, 'hello **world**', 'world'],
      [`${mod}+i`, 'hello _world_', 'world'],
      [`${mod}+e`, 'hello `world`', 'world'],
      [`${mod}+k`, 'hello [world](url)', 'url'],
    ];
    for (const [key, want, selected] of cases) {
      await mdSet(page, 'hello world', 5);
      await page.keyboard.press(key);
      assert.equal(await mdValue(page), want, key);
      assert.equal(await mdSelected(page), selected, key);
    }
    // Pressing it again unwraps.
    await mdSet(page, 'hello world', 5);
    await page.keyboard.press(`${mod}+b`);
    await page.keyboard.press(`${mod}+b`);
    assert.equal(await mdValue(page), 'hello world');
    // A selected URL becomes the link's target.
    await mdSet(page, 'see https://example.com/x', 21);
    await page.keyboard.press(`${mod}+k`);
    assert.equal(await mdValue(page), 'see [](https://example.com/x)');
    // Lines: the toolbar buttons, and focus stays in the text.
    await mdSet(page, 'one\ntwo', true);
    await page.click('#demo-comment [data-tool=list]');
    assert.equal(await mdValue(page), '- one\n- two');
    await page.click('#demo-comment [data-tool=list]');
    assert.equal(await mdValue(page), 'one\ntwo');
    await page.click('#demo-comment [data-tool=quote]');
    assert.equal(await mdValue(page), '> one\n> two');
    assert.equal(await page.evaluate(() => document.activeElement.matches('#demo-comment .cm-content')), true);
    await mdSet(page, 'a\nb', true);
    await page.click('#demo-comment [data-tool=code]');
    assert.equal(await mdValue(page), '```\na\nb\n```');
    await mdSet(page, 'x', 1);
    await page.click('#demo-comment [data-tool=bold]');
    assert.equal(await mdValue(page), '**x**');
    const labels = await page.evaluate(() => [...document.querySelectorAll('#demo-comment [role=toolbar] button')].map(b => [b.getAttribute('aria-label'), b.getAttribute('aria-keyshortcuts')]));
    const m = mod === 'Meta' ? 'Meta' : 'Control';
    assert.deepEqual(labels, [['Bold', `${m}+B`], ['Italic', `${m}+I`], ['Code', `${m}+E`], ['Link', `${m}+K`], ['Bulleted list', null], ['Quote', null]]);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('markdown: Mod-Enter and the submit action submit; empty and too long show an error until fixed; a pending onsubmit keeps it busy', async () => {
    const { page, problems } = await open(MD);
    const submits = () => page.evaluate(() => window.kitEvents.filter(e => e.type === 'wk-submit').map(e => e.detail.value));
    // The error is a polite live region from the start, empty, so a later message is announced.
    const region = await page.evaluate(() => { const e = document.querySelector('#demo-comment .wk-markdown-editor-error'); return { role: e.getAttribute('role'), text: e.textContent, shown: e.getClientRects().length > 0 || getComputedStyle(e).display !== 'none' }; });
    assert.deepEqual(region, { role: 'status', text: '', shown: true });
    await mdSet(page, 'Looks good');
    await page.evaluate(() => { window.kitEvents = []; });
    await page.keyboard.press(`${mod}+Enter`);
    assert.deepEqual(await submits(), ['Looks good']);
    assert.equal(await mdValue(page), 'Looks good', 'Mod-Enter inserted a line');
    await page.click('#demo-comment button:text-is("Comment")');
    assert.deepEqual(await submits(), ['Looks good', 'Looks good']);
    // Empty: an error on the text, focus in it, and no submit.
    await mdSet(page, '  ');
    await page.focus('#demo-comment [role=tab]:text-is("Write")');
    await page.keyboard.press('ArrowRight');
    await page.click('#demo-comment button:text-is("Comment")');
    const err = await page.evaluate(() => {
      const c = document.querySelector('#demo-comment .cm-content');
      const e = document.querySelector('#demo-comment .wk-markdown-editor-error');
      return { text: e.textContent, invalid: c.getAttribute('aria-invalid'), described: c.getAttribute('aria-describedby').split(' ').includes(e.id), focus: document.activeElement === c, mode: document.getElementById('demo-comment').mode };
    });
    assert.deepEqual(err, { text: 'Write something first.', invalid: 'true', described: true, focus: true, mode: 'write' });
    assert.equal((await submits()).length, 2);
    // Typing fixes it, and the error goes.
    await page.keyboard.type('ok');
    assert.equal(await page.textContent('#demo-comment .wk-markdown-editor-error'), '');
    assert.equal(await page.getAttribute('#demo-comment .cm-content', 'aria-invalid'), null);
    // Too long.
    await mdSet(page, 'x'.repeat(1003));
    await page.keyboard.press(`${mod}+Enter`);
    assert.equal(await page.textContent('#demo-comment .wk-markdown-editor-error'), 'Shorten this by 3 characters.');
    // Mod-Enter in the text leaves focus there: the live region carries the message to a screen reader.
    assert.equal(await page.evaluate(() => document.activeElement.matches('#demo-comment .cm-content')), true);
    const cdp = await page.context().newCDPSession(page);
    const { nodes } = await cdp.send('Accessibility.getFullAXTree');
    await cdp.detach();
    const live = nodes.find(n => !n.ignored && n.role?.value === 'status' && (n.properties || []).some(p => p.name === 'live' && p.value.value === 'polite'));
    assert.ok(live, 'no polite status region in the accessibility tree');
    await page.keyboard.press('Backspace');
    await page.keyboard.press('Backspace');
    assert.equal(await page.textContent('#demo-comment .wk-markdown-editor-error'), 'Shorten this by 1 character.');
    await page.keyboard.press('Backspace');
    assert.equal(await page.textContent('#demo-comment .wk-markdown-editor-error'), '');
    assert.equal((await submits()).length, 2);
    // Busy while onsubmit's promise is pending: a second submit does nothing.
    await page.evaluate(() => { window.pendingSubmit = new Promise(r => { window.finishSubmit = r; }); });
    await mdSet(page, 'Ship it');
    await page.keyboard.press(`${mod}+Enter`);
    await page.keyboard.press(`${mod}+Enter`);
    // force: Playwright waits for an aria-disabled button; the press must do nothing.
    await page.click('#demo-comment button:text-is("Comment")', { force: true });
    const busy = await page.evaluate(() => {
      const b = [...document.querySelectorAll('#demo-comment button')].find(x => x.textContent === 'Comment');
      return { busy: document.getElementById('demo-comment').hasAttribute('aria-busy'), aria: b.getAttribute('aria-disabled'), data: b.hasAttribute('data-busy'), disabled: b.disabled, cancel: [...document.querySelectorAll('#demo-comment button')].find(x => x.textContent === 'Cancel').getAttribute('aria-disabled') };
    });
    // Busy is on the submit control (aria-disabled, data-busy), not aria-busy on the editor.
    assert.deepEqual(busy, { busy: false, aria: 'true', data: true, disabled: false, cancel: null });
    assert.equal((await submits()).length, 3);
    await page.evaluate(() => window.finishSubmit());
    await until(page, () => !document.querySelector('#demo-comment [data-submit]').hasAttribute('data-busy'));
    assert.equal(await page.getAttribute('#demo-comment [data-submit]', 'aria-disabled'), null);
    await page.focus('#demo-comment .cm-content');
    await page.keyboard.press(`${mod}+Enter`);
    assert.equal((await submits()).length, 4);
    // A cancelled wk-submit stops onsubmit.
    const cancelled = await page.evaluate(() => {
      const el = document.getElementById('demo-comment');
      el.addEventListener('wk-submit', e => e.preventDefault(), { once: true });
      return el.submit();
    });
    assert.equal(cancelled, false);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('markdown: the count updates as the person types and marks a text over the limit', async () => {
    const { page, problems } = await open(MD);
    await mdSet(page, 'abc');
    assert.equal(await page.textContent('#demo-comment .wk-markdown-editor-count'), '3 / 1000');
    await page.keyboard.type('de');
    assert.equal(await page.textContent('#demo-comment .wk-markdown-editor-count'), '5 / 1000');
    const counted = await page.evaluate(() => document.querySelector('#demo-comment .cm-content').getAttribute('aria-describedby').split(' ').includes(document.querySelector('#demo-comment .wk-markdown-editor-count').id));
    assert.equal(counted, true);
    await mdSet(page, 'y'.repeat(1000));
    assert.equal(await page.evaluate(() => document.querySelector('#demo-comment .wk-markdown-editor-count').hasAttribute('data-over')), false);
    await page.keyboard.type('z');
    const over = await page.evaluate(() => {
      const c = document.querySelector('#demo-comment .wk-markdown-editor-count');
      return { text: c.textContent, over: c.hasAttribute('data-over'), colour: getComputedStyle(c).color, token: window.t.colour('var(--wk-color-attention-text)') };
    });
    assert.deepEqual(over, { text: '1001 / 1000', over: true, colour: over.token, token: over.token });
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('markdown: preview and rendered links go to onlink and never navigate; unsafe links are text; images do not load', async () => {
    const { page, problems } = await open(MD);
    const url = page.url();
    await page.evaluate(() => { document.getElementById('demo-comment').mode = 'preview'; });
    await page.click('#demo-comment .wk-markdown a:text-is("the guide")');
    await page.click('#demo-rendered a:text-is("https://example.com/faq")');
    await page.click('#demo-rendered a:text-is("A remote chart")', { button: 'middle' });
    await page.click('#demo-rendered a:text-is("A remote chart")', { modifiers: [mod] });
    assert.deepEqual(await page.evaluate(() => window.links), ['https://example.com/guide', 'https://example.com/faq', 'https://example.com/chart.png']);
    assert.equal(page.url(), url);
    assert.equal(page.context().pages().length, 1, 'a link opened a window');
    const look = await page.evaluate(() => [...document.querySelectorAll('.wk-markdown a')].map(a => [a.getAttribute('href'), a.getAttribute('rel'), a.getAttribute('target')]));
    for (const [href, rel, target] of look) {
      assert.match(href, /^https:\/\/example\.com\//);
      assert.equal(rel, 'noopener noreferrer');
      assert.equal(target, null);
    }
    // An editor whose preview has images, from a host not allowed and from the page's own origin: none load.
    const imgs = await page.evaluate(async () => {
      const { wkMarkdownEditor } = await import('/editor/dist/markdown.js');
      const el = wkMarkdownEditor({ label: 'Images', value: `![a](https://example.com/a.png) ![b](${location.origin}/editor/demo/missing.png) <img src="https://example.com/raw.png">`, mode: 'preview' });
      document.getElementById('demo').append(el);
      return { imgs: el.querySelectorAll('img').length, links: el.querySelectorAll('.wk-markdown a').length };
    });
    assert.deepEqual(imgs, { imgs: 0, links: 2 });
    // problems records any request outside the page's server, and any failed one.
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('markdown: disabled blocks edits, the toolbar and submit; enabling it again restores them', async () => {
    const { page, problems } = await open(MD);
    const el = '#demo-md-disabled';
    const look = () => page.evaluate(s => {
      const root = document.querySelector(s);
      return {
        aria: root.querySelector('.cm-content').getAttribute('aria-disabled'), data: root.hasAttribute('data-disabled'), editable: root.querySelector('.cm-content').getAttribute('contenteditable'),
        tools: [...root.querySelectorAll('[role=toolbar] button')].every(b => b.disabled), action: root.querySelector('.wk-markdown-editor-actions button').disabled,
      };
    }, el);
    assert.deepEqual(await look(), { aria: 'true', data: true, editable: 'false', tools: true, action: true });
    const before = await value(page, 'demo-md-disabled');
    await page.click(`${el} .cm-content`, { force: true });
    await page.keyboard.type('x');
    await page.keyboard.press(`${mod}+b`);
    assert.equal(await value(page, 'demo-md-disabled'), before);
    assert.equal(await page.evaluate(s => document.querySelector(s).submit(), el), false);
    // The preview is still there to read.
    await page.click(`${el} [role=tab]:text-is("Preview")`);
    assert.equal(await page.evaluate(s => document.querySelector(`${s} .wk-markdown strong`).textContent, el), 'locked');
    await page.evaluate(s => { const r = document.querySelector(s); r.disabled = false; r.mode = 'write'; }, el);
    assert.deepEqual(await look(), { aria: null, data: false, editable: 'true', tools: false, action: false });
    await page.click(`${el} .cm-content`);
    await page.keyboard.press(`${mod}+End`);
    await page.keyboard.type('!');
    assert.equal(await value(page, 'demo-md-disabled'), `${before}!`);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('markdown: the API: events and callbacks, value and mode set quietly, label required, destroy, auto-grow', async () => {
    const { page, problems } = await open(MD);
    const result = await page.evaluate(async () => {
      const { wkMarkdownEditor } = await import('/editor/dist/markdown.js');
      const calls = [];
      const el = wkMarkdownEditor({ label: 'Note', minHeight: 60, maxHeight: '120px', onchange: v => calls.push(['onchange', v]), onmode: m => calls.push(['onmode', m]), onsubmit: v => calls.push(['onsubmit', v]) });
      for (const t of ['wk-change', 'wk-mode', 'wk-submit']) el.addEventListener(t, e => calls.push([t, e.detail.value ?? e.detail.mode]));
      document.getElementById('demo').append(el);
      const box = el.querySelector('.wk-code-editor');
      const small = box.getBoundingClientRect().height;
      el.value = 'a\n'.repeat(40);
      await new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)));
      const big = box.getBoundingClientRect().height;
      const scrolls = el.querySelector('.cm-scroller').scrollHeight > el.querySelector('.cm-scroller').clientHeight;
      el.mode = 'preview';
      el.value = 'x';
      const quiet = calls.length;
      const errors = [];
      for (const spec of [{}, { label: ' ' }]) { try { wkMarkdownEditor(spec); errors.push('none'); } catch (e) { errors.push(e.constructor.name); } }
      el.destroy(); el.destroy();
      el.value = 'later';
      return { small, big, scrolls, quiet, errors, children: el.childElementCount, value: el.value, submit: el.submit() };
    });
    assert.equal(Math.round(result.small), 60);
    assert.equal(Math.round(result.big), 120);
    assert.equal(result.scrolls, true);
    assert.equal(result.quiet, 0, 'setting value or mode fired something');
    assert.deepEqual(result.errors, ['TypeError', 'TypeError']);
    assert.equal(result.children, 0);
    assert.equal(result.submit, false);
    // Typing fires wk-change then onchange; the tab fires wk-mode then onmode; Mod-Enter wk-submit then onsubmit.
    await page.evaluate(async () => {
      const { wkMarkdownEditor } = await import('/editor/dist/markdown.js');
      window.order = [];
      const el = wkMarkdownEditor({ id: 'demo-order', label: 'Order', onchange: () => window.order.push('onchange'), onmode: m => window.order.push(`onmode:${m}`), onsubmit: () => window.order.push('onsubmit') });
      for (const t of ['wk-change', 'wk-mode', 'wk-submit']) el.addEventListener(t, e => window.order.push(`${t}:${JSON.stringify(e.detail)}`));
      document.getElementById('demo').append(el);
    });
    await page.click('#demo-order .cm-content');
    await page.keyboard.type('a');
    await page.keyboard.press(`${mod}+Enter`);
    await page.click('#demo-order [role=tab]:text-is("Preview")');
    assert.deepEqual(await page.evaluate(() => window.order), ['wk-change:{"value":"a"}', 'onchange', 'wk-submit:{"value":"a"}', 'onsubmit', 'wk-mode:{"mode":"preview"}', 'onmode:preview']);
    // The tabs' own wk-change does not escape the editor.
    assert.equal(await page.evaluate(() => window.kitEvents.some(e => e.type === 'wk-change' && e.detail.index !== undefined)), false);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('markdown: forced colours: borders, the selected tab and the focus ring stay visible', async () => {
    const { page, problems } = await open(MD, { forcedColors: 'active' });
    await page.evaluate(() => { document.getElementById('demo-comment').mode = 'preview'; });
    const look = await page.evaluate(() => {
      const canvasText = window.t.colour('CanvasText');
      const pv = getComputedStyle(document.querySelector('#demo-comment .wk-markdown-editor-preview'));
      const code = getComputedStyle(document.querySelector('#demo-rendered p code'));
      const quote = getComputedStyle(document.querySelector('#demo-rendered blockquote'));
      return { canvasText, preview: pv.borderTopColor, code: `${code.borderTopStyle} ${code.borderTopColor}`, quote: quote.borderLeftColor };
    });
    assert.equal(look.preview, look.canvasText);
    // Over the limit is not shown by colour alone.
    await page.evaluate(() => { const el = document.getElementById('demo-comment'); el.value = 'x'.repeat(1001); });
    assert.equal(await page.evaluate(() => getComputedStyle(document.querySelector('#demo-comment .wk-markdown-editor-count')).textDecorationLine), 'underline');
    assert.equal(look.code, `solid ${look.canvasText}`);
    assert.equal(look.quote, look.canvasText);
    await page.click('#demo-comment [role=tab]:text-is("Write")');
    await page.click('#demo-comment .cm-content');
    assert.equal(await page.evaluate(() => getComputedStyle(document.querySelector('#demo-comment .cm-editor')).outlineStyle), 'solid');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('markdown: hostile text previews as its source, quickly, with no error, and wk-mode still fires', async () => {
    const { page, problems } = await open(MD);
    for (const [label, text] of [
      ['nested quotes', '>'.repeat(2000) + ' x'],
      ['emphasis markers', '*'.repeat(5000) + 'a' + '*'.repeat(5000)],
      ['quoted list items', '> - '.repeat(5000)],
      ['deep list', Array.from({ length: 1000 }, (_, i) => `${'  '.repeat(i)}- x`).join('\n')],
    ]) {
      await page.evaluate(t => { window.kitEvents = []; const el = document.getElementById('demo-comment'); el.mode = 'write'; el.value = t; }, text);
      const start = Date.now();
      await page.click('#demo-comment [role=tab]:text-is("Preview")');
      const took = Date.now() - start;
      const look = await page.evaluate(() => ({
        source: document.querySelector('#demo-comment .wk-markdown-editor-preview > .wk-markdown-source')?.textContent.length,
        whiteSpace: getComputedStyle(document.querySelector('#demo-comment .wk-markdown-editor-preview > *')).whiteSpace,
        modes: window.kitEvents.filter(e => e.type === 'wk-mode').map(e => e.detail.mode),
      }));
      assert.deepEqual(look, { source: text.length, whiteSpace: 'pre-wrap', modes: ['preview'] }, label);
      assert.ok(took < 3000, `${label}: ${took} ms`);
    }
    // Typing the text back to shallow Markdown turns the Write tab's highlighting on again.
    await page.evaluate(() => { const el = document.getElementById('demo-comment'); el.mode = 'write'; el.value = `${'> '.repeat(40)}x`; });
    assert.equal(await page.evaluate(() => document.querySelectorAll('#demo-comment .cm-line span').length), 0);
    await page.click('#demo-comment .cm-content');
    await page.keyboard.press(`${mod}+a`);
    await page.keyboard.type('# Title');
    await until(page, () => document.querySelectorAll('#demo-comment .cm-line span').length > 0);
    assert.deepEqual(problems, []);
    // wk-mode fires even when drawing the preview fails (here, a failing DOM call).
    await page.evaluate(() => {
      window.kitEvents = [];
      const preview = document.querySelector('#demo-comment .wk-markdown-editor-preview');
      preview.replaceChildren = () => { throw new Error('drawing failed'); };
    });
    await page.click('#demo-comment [role=tab]:text-is("Preview")');
    assert.deepEqual(await page.evaluate(() => window.kitEvents.filter(e => e.type === 'wk-mode').map(e => e.detail.mode)), ['preview']);
    assert.ok(problems.length === 1 && problems[0].includes('drawing failed'), problems.join('\n'));
    await page.close();
  });

  await check('markdown: Escape and the cancel action cancel (wk-cancel, then oncancel), except in find, on a selection, or while composing', async () => {
    const { page, problems } = await open(MD);
    const cancels = () => page.evaluate(() => window.kitEvents.filter(e => e.type === 'wk-cancel' && e.target === 'demo-comment').length);
    await mdSet(page, 'draft');
    await page.evaluate(() => { window.kitEvents = []; });
    await page.keyboard.press('Escape');
    assert.equal(await cancels(), 1);
    assert.equal(await mdValue(page), '', 'oncancel did not run');
    // With a selection, the first Escape collapses it.
    await mdSet(page, 'draft', 3);
    await page.keyboard.press('Escape');
    assert.equal(await cancels(), 1);
    await page.keyboard.press('Escape');
    assert.equal(await cancels(), 2);
    // In the find panel, Escape closes it; a second Escape in the text cancels.
    await mdSet(page, 'draft');
    await page.keyboard.press(`${mod}+f`);
    await until(page, () => document.activeElement?.matches('#demo-comment [name=search]'));
    await page.keyboard.press('Escape');
    await until(page, () => !document.querySelector('#demo-comment .cm-panel'));
    assert.equal(await cancels(), 2);
    assert.equal(await mdValue(page), 'draft');
    // Not while an input method is composing.
    const composing = await page.evaluate(() => {
      const c = document.querySelector('#demo-comment .cm-content');
      const e = new KeyboardEvent('keydown', { key: 'Escape', isComposing: true, bubbles: true, cancelable: true });
      c.dispatchEvent(e);
      return e.defaultPrevented;
    });
    assert.equal(composing, false);
    assert.equal(await cancels(), 2);
    // From the tabs too; and the Cancel action.
    await page.focus('#demo-comment [role=tab]:text-is("Write")');
    await page.keyboard.press('Escape');
    assert.equal(await cancels(), 3);
    await page.click('#demo-comment button:text-is("Cancel")');
    assert.equal(await cancels(), 4);
    // Without oncancel, Escape is left alone (a dialog around the editor can use it).
    const free = await page.evaluate(async () => {
      const { wkMarkdownEditor } = await import('/editor/dist/markdown.js');
      const el = wkMarkdownEditor({ label: 'Free' });
      document.getElementById('demo').append(el);
      let cancelled = 0;
      el.addEventListener('wk-cancel', () => cancelled++);
      const e = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
      el.querySelector('.cm-content').dispatchEvent(e);
      return { prevented: e.defaultPrevented, cancelled };
    });
    assert.deepEqual(free, { prevented: false, cancelled: 0 });
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('markdown: disabling keeps focus in the editor (from an action or the toolbar); a cancel action stays usable', async () => {
    const { page, problems } = await open(MD);
    for (const target of ['#demo-comment [data-submit]', '#demo-comment [data-tool=bold]']) {
      await page.evaluate(() => { const el = document.getElementById('demo-comment'); el.disabled = false; el.mode = 'write'; });
      await page.focus(target);
      await page.evaluate(() => { document.getElementById('demo-comment').disabled = true; });
      assert.equal(await page.evaluate(() => document.getElementById('demo-comment').contains(document.activeElement)), true, `${target}: focus went to ${await page.evaluate(() => document.activeElement.tagName)}`);
    }
    assert.equal(await page.evaluate(() => [...document.querySelectorAll('#demo-comment button')].find(b => b.textContent === 'Cancel').disabled), false);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('markdown: renderMarkdownElement returns a styled .wk-markdown element', async () => {
    const { page, problems } = await open(MD);
    const look = await page.evaluate(async () => {
      const { renderMarkdownElement, MAX_MARKDOWN_LENGTH, MAX_MARKDOWN_DEPTH } = await import('/editor/dist/markdown.js');
      const el = renderMarkdownElement('**hi** `x`');
      document.getElementById('demo').append(el);
      return { tag: el.tagName, cls: el.className, html: el.innerHTML, colour: getComputedStyle(el).color, text: window.t.colour('var(--wk-color-text)'), limits: [MAX_MARKDOWN_LENGTH, MAX_MARKDOWN_DEPTH] };
    });
    assert.deepEqual(look, { tag: 'DIV', cls: 'wk-markdown', html: '<p><strong>hi</strong> <code>x</code></p>', colour: look.text, text: look.text, limits: [100000, 32] });
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('markdown: hostile emphasis text does not throw from the editor (built with it, set, pasted), in time; under the limit it stays highlighted', async () => {
    const { page, problems } = await open(MD);
    const cases = [
      ['a 40,000-marker run', '*'.repeat(20000) + 'a' + '*'.repeat(20000)],
      ['nested emphasis pairs', '*a '.repeat(20000)],
      ['nested strong pairs', '**a '.repeat(15000)],
      ['nested links', '[a '.repeat(5000) + 'b' + '](x)'.repeat(5000)],
      // A fence in a quote ends at a line without the quote's marker: the scan must not read it as code.
      ['after a fence in a quote', '> ```\n   ' + '*'.repeat(20000) + 'a' + '*'.repeat(20000)],
      // A delimiter row with more cells than its header is no table: one paragraph of 33,600 markers.
      ['a fake table', 'a|b\n|-|-|-|\n' + Array(12).fill('| ' + '**a '.repeat(700)).join('\n') + '\n| b\n' + Array(12).fill('| ' + ' a**'.repeat(700)).join('\n')],
    ];
    for (const [label, text] of cases) {
      for (const how of ['spec', 'setter', 'paste']) {
        const r = await page.evaluate(async ([v, how]) => {
          const { wkMarkdownEditor } = await import('/editor/dist/markdown.js');
          const start = performance.now();
          try {
            const ed = wkMarkdownEditor({ label: 'Hostile', value: how === 'spec' ? v : '' });
            document.getElementById('demo').replaceChildren(ed);
            if (how === 'setter') ed.value = v;
            if (how === 'paste') {
              const c = ed.querySelector('.cm-content');
              c.focus();
              const dt = new DataTransfer();
              dt.setData('text/plain', v);
              c.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }));
            }
            await new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)));
            ed.mode = 'preview';
            return { length: ed.value.length, source: !!ed.querySelector('.wk-markdown-editor-preview > .wk-markdown-source'), ms: performance.now() - start };
          } catch (e) { return { threw: String(e).slice(0, 120) }; }
        }, [text, how]);
        assert.equal(r.threw, undefined, `${label} (${how}) threw`);
        assert.deepEqual([r.length, r.source], [text.length, true], `${label} (${how})`);
        assert.ok(r.ms < 1500, `${label} (${how}): ${Math.round(r.ms)} ms`);
      }
    }
    // Just under the delimiter limit: highlighted in Write, rendered in Preview.
    const under = await page.evaluate(async () => {
      const { wkMarkdownEditor, MAX_MARKDOWN_DELIMITERS } = await import('/editor/dist/markdown.js');
      const n = MAX_MARKDOWN_DELIMITERS / 2;
      const ed = wkMarkdownEditor({ label: 'Under', value: '*a '.repeat(n) + 'b' + ' a*'.repeat(n) });
      document.getElementById('demo').replaceChildren(ed);
      await new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)));
      const spans = ed.querySelectorAll('.cm-line span').length;
      ed.mode = 'preview';
      return { spans, ems: ed.querySelectorAll('.wk-markdown em').length };
    });
    assert.ok(under.spans > 0, 'no highlighting under the limit');
    assert.ok(under.ems > 0, 'no emphasis rendered under the limit');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('markdown: the editor is a named group; focus parked on it while disabled goes back to the text on enable; a repeated error is announced again', async () => {
    const { page, problems } = await open(MD);
    const root = await page.evaluate(() => { const r = document.getElementById('demo-comment'); return [r.getAttribute('role'), r.getAttribute('aria-label'), r.getAttribute('tabindex')]; });
    assert.deepEqual(root, ['group', 'Comment', null]);
    await page.focus('#demo-comment [data-submit]');
    await page.evaluate(() => { document.getElementById('demo-comment').disabled = true; });
    assert.equal(await page.evaluate(() => document.activeElement.id), 'demo-comment');
    await page.evaluate(() => { document.getElementById('demo-comment').disabled = false; });
    const back = await page.evaluate(() => ({ text: document.activeElement.matches('#demo-comment .cm-content'), tabindex: document.getElementById('demo-comment').getAttribute('tabindex') }));
    assert.deepEqual(back, { text: true, tabindex: null });
    // The same error twice: the region empties and refills, so a screen reader hears it again.
    await mdSet(page, '');
    await page.keyboard.press(`${mod}+Enter`);
    await until(page, () => document.querySelector('#demo-comment .wk-markdown-editor-error').textContent === 'Write something first.');
    await page.evaluate(() => {
      window.seen = [];
      const e = document.querySelector('#demo-comment .wk-markdown-editor-error');
      new MutationObserver(() => window.seen.push(e.textContent)).observe(e, { childList: true, characterData: true, subtree: true });
    });
    await page.keyboard.press(`${mod}+Enter`);
    await until(page, () => window.seen.length >= 2);
    assert.deepEqual(await page.evaluate(() => window.seen), ['', 'Write something first.']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('markdown: a stack overflow the scan misses (forced in the parser) leaves the text, without highlighting, when built, set or pasted; a new value highlights again', async () => {
    const { page, problems } = await open(MD);
    for (const how of ['spec', 'setter', 'paste']) {
      const r = await page.evaluate(async how => {
        const { wkMarkdownEditor } = await import('/editor/dist/markdown.js');
        // The first charCodeAt on a string holding the marker throws a RangeError,
        // as an overflow would. The scan uses no charCodeAt; Lezer's parser does.
        const mark = '\u2063overflow\u2063', text = `# Title ${mark} *x*`;
        const native = String.prototype.charCodeAt;
        let fired = false;
        const arm = () => { String.prototype.charCodeAt = function (i) { if (!fired && this.includes(mark)) { fired = true; throw new RangeError('Maximum call stack size exceeded'); } return native.call(this, i); }; };
        try {
          let ed;
          if (how === 'spec') { arm(); ed = wkMarkdownEditor({ label: 'Forced', value: text }); document.getElementById('demo').replaceChildren(ed); }
          else { ed = wkMarkdownEditor({ label: 'Forced', value: '' }); document.getElementById('demo').replaceChildren(ed); arm(); }
          if (how === 'setter') ed.value = text;
          if (how === 'paste') {
            const c = ed.querySelector('.cm-content');
            c.focus();
            const dt = new DataTransfer();
            dt.setData('text/plain', text);
            c.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }));
          }
          String.prototype.charCodeAt = native;
          await new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)));
          const out = { fired, kept: ed.value === text, spans: ed.querySelectorAll('.cm-line span').length };
          ed.value = '# Fine *now*';
          await new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)));
          out.again = ed.querySelectorAll('.cm-line span').length > 0;
          return out;
        } catch (e) { String.prototype.charCodeAt = native; return { threw: String(e), fired }; }
      }, how);
      assert.deepEqual(r, { fired: true, kept: true, spans: 0, again: true }, how);
    }
    // Any other RangeError is not an overflow: it is thrown as it is, with no retry.
    const other = await page.evaluate(async () => {
      const { wkMarkdownEditor } = await import('/editor/dist/markdown.js');
      const mark = '\u2063other\u2063';
      const native = String.prototype.charCodeAt;
      let calls = 0;
      String.prototype.charCodeAt = function (i) { if (this.includes(mark)) { calls++; throw new RangeError('not an overflow'); } return native.call(this, i); };
      const out = [];
      try { wkMarkdownEditor({ label: 'Other', value: `# ${mark}` }); out.push('built'); } catch (e) { out.push(e.message); }
      const ed = wkMarkdownEditor({ label: 'Other', value: 'fine' });
      try { ed.value = `# ${mark}`; out.push('set'); } catch (e) { out.push(e.message); }
      String.prototype.charCodeAt = native;
      out.push(ed.value, calls);
      return out;
    });
    assert.deepEqual(other, ['not an overflow', 'not an overflow', 'fine', 2]);
    assert.deepEqual(problems, []);
    await page.close();
  });
} finally { await browser.close(); await new Promise(resolve => server.close(resolve)); }

if (failures.length) {
  console.log(`Editor browser checks: ${failures.length} failed.`);
  process.exit(1);
}
console.log('Editor browser checks passed.');
