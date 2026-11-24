// Browser checks for the editors, on the review pages in demo/ (index.html
// and themed.html for the code editor, markdown.html for the Markdown
// editor, richtext.html for the rich-text editor), which load the prebuilt dist/ modules as an app vendors them. Run with: npm run test:browser
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
// CHECK=text runs only the checks whose name includes it.
async function check(name, fn) {
  if (process.env.CHECK && !name.includes(process.env.CHECK)) return;
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

  // ---- The rich-text editor (demo/richtext.html, dist/richtext.js) ------
  const RT = '/editor/demo/richtext.html';
  // Puts an editor's value, focuses its text with the cursor at the end, and
  // selects its last `select` characters (Shift+Left), or all of it (true).
  const rtSet = async (page, text, select = 0, id = 'demo-rich') => {
    await page.evaluate(([t, i]) => { const el = document.getElementById(i); el.value = t; el.focus(); }, [text, id]);
    await page.keyboard.press(`${mod}+End`);
    if (select === true) await page.keyboard.press(`${mod}+a`);
    else for (let i = 0; i < select; i++) await page.keyboard.press('Shift+ArrowLeft');
  };
  const rtValue = (page, id = 'demo-rich') => value(page, id);
  const rtFocused = (page, id = 'demo-rich') => page.evaluate(i => document.activeElement === document.querySelector(`#${i} .wk-richtext-editor-content`), id);
  const pressed = (page, id = 'demo-rich') => page.evaluate(i => Object.fromEntries([...document.querySelectorAll(`#${i} [role=toolbar] button[aria-pressed]`)].map(b => [b.dataset.tool, b.getAttribute('aria-pressed') === 'true'])), id);
  const pressedOn = async (page, id) => Object.entries(await pressed(page, id)).filter(([, v]) => v).map(([k]) => k);

  await check('richtext: the page loads with no errors, CSP violations or network; styles adopted, tokens used (light and dark)', async () => {
    for (const scheme of ['light', 'dark']) {
      const { page, problems } = await open(RT, { colorScheme: scheme });
      const look = await page.evaluate(() => {
        const content = document.querySelector('#demo-rich .wk-richtext-editor-content');
        const s = getComputedStyle(content);
        return {
          editors: document.querySelectorAll('.wk-richtext-editor').length,
          styleTags: document.querySelectorAll('style').length,
          adopted: document.adoptedStyleSheets.length,
          role: content.getAttribute('role'), multiline: content.getAttribute('aria-multiline'), name: content.getAttribute('aria-label'),
          bg: s.backgroundColor, control: window.t.colour('var(--wk-color-control)'),
          color: s.color, text: window.t.colour('var(--wk-color-text)'),
          border: s.borderTopColor, controlBorder: window.t.colour('var(--wk-color-control-border)'),
          whiteSpace: s.whiteSpace,
          link: getComputedStyle(content.querySelector('a.wk-link')).color, accent: window.t.colour('var(--wk-color-accent)'),
          code: getComputedStyle(content.querySelector('.wk-code > pre')).fontFamily,
          heading: getComputedStyle(content.querySelector('h2')).fontSize, lg: getComputedStyle(document.documentElement).getPropertyValue('--wk-font-size-lg').trim(),
          imgs: document.querySelectorAll('.wk-richtext-editor img:not(.ProseMirror-separator)').length,
          overflow: document.documentElement.scrollWidth > innerWidth,
        };
      });
      assert.equal(look.editors, 6);
      assert.equal(look.styleTags, 0);
      assert.ok(look.adopted >= 1);
      assert.deepEqual([look.role, look.multiline, look.name], ['textbox', 'true', 'Release notes']);
      assert.equal(look.bg, look.control, scheme);
      assert.equal(look.color, look.text, scheme);
      assert.equal(look.border, look.controlBorder, scheme);
      assert.equal(look.whiteSpace, 'break-spaces', 'ProseMirror\'s structural rules are missing');
      assert.equal(look.link, look.accent, scheme);
      assert.match(look.code, /^ui-monospace,/);
      assert.equal(look.heading, look.lg);
      assert.equal(look.imgs, 0, 'the raw <img> in the sample became an image');
      assert.equal(look.overflow, false);
      assert.deepEqual(problems, [], scheme);
      await page.close();
    }
  });

  await check('richtext: typing with marks by shortcut and toolbar (bold, italic, code), and the block buttons, write Markdown', async () => {
    const { page, problems } = await open(RT);
    await rtSet(page, '');
    await page.keyboard.type('plain ');
    for (const [key, text] of [[`${mod}+b`, 'bold'], [`${mod}+i`, 'italic'], [`${mod}+e`, 'code']]) {
      await page.keyboard.press(key);
      await page.keyboard.type(text);
      await page.keyboard.press(key);
      await page.keyboard.type(' ');
    }
    assert.equal(await rtValue(page), 'plain **bold** _italic_ `code`');
    // On a selection, the shortcut and the toolbar toggle it.
    await rtSet(page, 'hello world', 5);
    await page.keyboard.press(`${mod}+b`);
    assert.equal(await rtValue(page), 'hello **world**');
    await page.click('#demo-rich [data-tool=bold]');
    assert.equal(await rtValue(page), 'hello world');
    await page.click('#demo-rich [data-tool=italic]');
    assert.equal(await rtValue(page), 'hello _world_');
    assert.equal(await rtFocused(page), true, 'a toolbar button took focus from the text');
    // Blocks, from the toolbar and the list shortcuts.
    for (const [tool, want] of [['heading1', '# line'], ['heading2', '## line'], ['bulletList', '- line'], ['orderedList', '1. line'], ['quote', '> line'], ['codeBlock', '```\nline\n```']]) {
      await rtSet(page, 'line');
      await page.click(`#demo-rich [data-tool=${tool}]`);
      assert.equal(await rtValue(page), want, tool);
      await page.click(`#demo-rich [data-tool=${tool}]`);
      assert.equal(await rtValue(page), 'line', `${tool} again`);
    }
    await rtSet(page, 'line');
    await page.keyboard.press(`${mod}+Shift+8`);
    assert.equal(await rtValue(page), '- line');
    await page.keyboard.press(`${mod}+Shift+7`);
    assert.equal(await rtValue(page), '1. line');
    await rtSet(page, 'above');
    await page.click('#demo-rich [data-tool=rule]');
    await page.keyboard.type('below');
    assert.equal(await rtValue(page), 'above\n\n---\n\nbelow');
    // Line breaks, list indent and outdent.
    await rtSet(page, '- one');
    await page.keyboard.press('Enter');
    await page.keyboard.type('two');
    await page.keyboard.press(`${mod}+]`);
    await page.keyboard.press('Shift+Enter');
    await page.keyboard.type('more');
    assert.equal(await rtValue(page), '- one\n  - two\\\n    more');
    await page.keyboard.press(`${mod}+[`);
    assert.equal(await rtValue(page), '- one\n- two\\\n  more');
    const labels = await page.evaluate(() => [...document.querySelectorAll('#demo-rich [role=toolbar] button')].map(b => [b.getAttribute('aria-label'), b.getAttribute('aria-keyshortcuts')]));
    const m = mod === 'Meta' ? 'Meta' : 'Control';
    assert.deepEqual(labels, [['Bold', `${m}+B`], ['Italic', `${m}+I`], ['Code', `${m}+E`], ['Link', `${m}+K`], ['Heading 1', null], ['Heading 2', null],
      ['Bulleted list', `${m}+Shift+8`], ['Numbered list', `${m}+Shift+7`], ['Quote', null], ['Code block', null], ['Horizontal rule', null]]);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: Markdown input rules (# , ## , - , 1. , > , ``` and `code`), and Backspace undoes one', async () => {
    const { page, problems } = await open(RT);
    for (const [typed, want] of [
      ['# Title', '# Title'], ['## Sub', '## Sub'], ['- item', '- item'], ['* item', '- item'], ['1. item', '1. item'], ['3. item', '3. item'],
      ['> quoted', '> quoted'], ['```go code', '```go\ncode\n```'], ['run `go test` now', 'run `go test` now'],
    ]) {
      await rtSet(page, '');
      await page.keyboard.type(typed);
      assert.equal(await rtValue(page), want, typed);
    }
    // A fence line then Enter starts a code block too.
    await rtSet(page, '');
    await page.keyboard.type('```js');
    await page.keyboard.press('Enter');
    await page.keyboard.type('x');
    assert.equal(await rtValue(page), '```js\nx\n```');
    // Backspace right after a rule fired puts the typed text back.
    await rtSet(page, '');
    await page.keyboard.type('# ');
    await page.keyboard.press('Backspace');
    assert.equal(await rtValue(page), '\\#');
    // The rules do nothing inside a code block.
    await rtSet(page, '```\n\n```');
    await page.keyboard.type('# not a heading');
    assert.equal(await rtValue(page), '```\n# not a heading\n```');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: undo and redo', async () => {
    const { page, problems } = await open(RT);
    await rtSet(page, 'start');
    await page.keyboard.type(' more');
    await page.keyboard.press(`${mod}+b`);
    await page.keyboard.type('bold');
    assert.equal(await rtValue(page), 'start more**bold**');
    await page.keyboard.press(`${mod}+z`);
    assert.equal(await rtValue(page), 'start');
    await page.keyboard.press(`${mod}+Shift+z`);
    assert.equal(await rtValue(page), 'start more**bold**');
    await page.keyboard.press(`${mod}+z`);
    await page.keyboard.press(`${mod}+y`);
    assert.equal(await rtValue(page), 'start more**bold**');
    // Setting value starts a fresh history: Mod-Z does not undo a load.
    await rtSet(page, 'loaded');
    await page.keyboard.press(`${mod}+z`);
    assert.equal(await rtValue(page), 'loaded');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: the toolbar\'s aria-pressed follows the selection', async () => {
    const { page, problems } = await open(RT);
    await rtSet(page, '## Head\n\nplain **bold _both_** [link](https://example.com/) `code`\n\n- item\n\n1. num\n\n> quote\n\n```\nblock\n```');
    const to = async (text, offset = 1) => {
      await page.evaluate(([t, o]) => {
        const content = document.querySelector('#demo-rich .wk-richtext-editor-content');
        const walker = document.createTreeWalker(content, NodeFilter.SHOW_TEXT);
        let n;
        while ((n = walker.nextNode())) if (n.data.includes(t)) break;
        getSelection().collapse(n, n.data.indexOf(t) + o);
      }, [text, offset]);
    };
    // ProseMirror reads the DOM selection on selectionchange: wait for the toolbar to follow.
    const follows = want => until(page, w => JSON.stringify([...document.querySelectorAll('#demo-rich [role=toolbar] button[aria-pressed=true]')].map(b => b.dataset.tool)) === JSON.stringify(w), want)
      .catch(async () => assert.fail(`pressed ${JSON.stringify(await pressedOn(page, 'demo-rich'))}, want ${JSON.stringify(want)}`));
    for (const [text, want] of [
      ['Head', ['heading2']], ['plain', []], ['bold', ['bold']], ['both', ['bold', 'italic']], ['link', ['link']], ['code', ['code']],
      ['item', ['bulletList']], ['num', ['orderedList']], ['quote', ['quote']], ['block', ['codeBlock']],
    ]) {
      await to(text);
      await follows(want);
    }
    // A stored mark shows too: Mod-B with nothing selected.
    await to('plain', 5);
    await page.keyboard.press(`${mod}+b`);
    assert.deepEqual(await pressedOn(page, 'demo-rich'), ['bold']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: the link popover sets, checks, edits, opens and removes a link; Escape closes it back to the opener', async () => {
    const { page, problems } = await open(RT);
    const pop = () => page.evaluate(() => {
      const p = document.querySelector('#demo-rich .wk-richtext-editor-link');
      const input = p.querySelector('input');
      return { open: !p.hidden, role: p.getAttribute('role'), focused: document.activeElement === input, value: input.value, invalid: input.getAttribute('aria-invalid'), error: p.querySelector('.wk-richtext-editor-link-error').textContent, expanded: document.querySelector('#demo-rich [data-tool=link]').getAttribute('aria-expanded') };
    });
    await rtSet(page, 'see the guide', 9);
    await page.keyboard.press(`${mod}+k`);
    assert.deepEqual(await pop(), { open: true, role: 'dialog', focused: true, value: '', invalid: null, error: '', expanded: 'true' });
    // An unsafe or empty address is refused with an error; the field keeps focus.
    await page.keyboard.type('javascript:alert(1)');
    await page.keyboard.press('Enter');
    let p = await pop();
    assert.equal(p.open, true);
    assert.equal(p.invalid, 'true');
    assert.match(p.error, /web or email address/);
    assert.equal(p.focused, true);
    assert.equal(await rtValue(page), 'see the guide');
    // A bare domain gets https://.
    await page.fill('#demo-rich .wk-richtext-editor-link input', 'example.com/guide');
    await page.keyboard.press('Enter');
    assert.equal(await rtValue(page), 'see [the guide](https://example.com/guide)');
    assert.equal((await pop()).open, false);
    assert.equal(await rtFocused(page), true, 'focus did not return to the text');
    // Editing: the cursor in the link, the toolbar button: the popover shows its address, Open and Remove.
    await page.keyboard.press('End');
    await page.keyboard.press('ArrowLeft');
    await page.keyboard.press('ArrowLeft');
    await page.click('#demo-rich [data-tool=link]');
    p = await pop();
    assert.equal(p.value, 'https://example.com/guide');
    await page.click('#demo-rich .wk-richtext-editor-link button:text-is("Open")');
    assert.deepEqual(await page.evaluate(() => window.links), ['https://example.com/guide']);
    // Escape closes it and focus goes back to the opener (the toolbar button), and does not cancel the editor.
    await page.focus('#demo-rich .wk-richtext-editor-link input');
    await page.keyboard.press('Escape');
    assert.equal((await pop()).open, false);
    assert.equal(await page.evaluate(() => document.activeElement.dataset.tool), 'link');
    assert.equal(await page.evaluate(() => window.kitEvents.some(e => e.type === 'wk-cancel')), false, 'Escape in the popover cancelled the editor');
    // Remove.
    await page.keyboard.press('Enter');
    await page.click('#demo-rich .wk-richtext-editor-link button:text-is("Remove")');
    assert.equal(await rtValue(page), 'see the guide');
    // With nothing selected, the address becomes the link's text.
    await rtSet(page, 'at ');
    await page.keyboard.press(`${mod}+k`);
    await page.keyboard.type('a@example.com');
    await page.keyboard.press('Enter');
    await page.keyboard.type(' now');
    assert.equal(await rtValue(page), 'at a@example.com now');
    assert.equal(await page.evaluate(() => document.querySelector('#demo-rich a').getAttribute('href')), 'mailto:a@example.com');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: links never navigate: Mod-click and read-only clicks go to onlink; a plain click only places the cursor', async () => {
    const { page, problems } = await open(RT);
    const url = page.url();
    await page.click('#demo-rich a:text-is("the guide")');
    assert.deepEqual(await page.evaluate(() => window.links), []);
    await page.click('#demo-rich a:text-is("the guide")', { modifiers: [mod] });
    await page.click('#demo-rich a:text-is("the guide")', { button: 'middle' });
    await page.click('#demo-rich-readonly a');
    // Keyboard: in the read-only text, links are reachable by Tab.
    await page.focus('#demo-rich-readonly a');
    await page.keyboard.press('Enter');
    assert.deepEqual(await page.evaluate(() => window.links), ['https://example.com/guide', 'https://example.com/read', 'https://example.com/read']);
    assert.equal(page.url(), url);
    assert.equal(page.context().pages().length, 1, 'a link opened a window');
    for (const [href, rel, target] of await page.evaluate(() => [...document.querySelectorAll('.wk-richtext-editor a')].map(a => [a.getAttribute('href'), a.getAttribute('rel'), a.getAttribute('target')]))) {
      assert.match(href, /^(https:\/\/example\.com\/|mailto:)/);
      assert.equal(rel, 'noopener noreferrer');
      assert.equal(target, null);
    }
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: pasted HTML is cleaned to the supported features: no scripts, handlers, images, frames or unsafe links, and nothing loads', async () => {
    const { page, problems } = await open(RT);
    const paste = (id, html, text = '') => page.evaluate(([i, h, t]) => {
      const content = document.querySelector(`#${i} .wk-richtext-editor-content`);
      content.focus();
      const data = new DataTransfer();
      data.setData('text/html', h);
      data.setData('text/plain', t);
      content.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }));
    }, [id, html, text]);
    await rtSet(page, '');
    await paste('demo-rich', '<h3 onclick="window.pwned=1">Title</h3><p>a <b>bold</b> <span style="font-weight:700">heavy</span> <i>it</i> <code>c</code>'
      + '<script>window.pwned=1</script><img src="https://example.com/pixel.png" onerror="window.pwned=1"><svg onload="window.pwned=1"><circle r="1"></circle></svg>'
      + ' <a href="javascript:window.pwned=1">bad</a> <a href="https://example.com/ok" onclick="window.pwned=1" target="_blank">ok</a> <a href="data:text/html,x">data</a></p>'
      + '<iframe src="https://example.com/frame"></iframe><style>body{display:none}</style><table><tr><td>cell</td></tr></table>'
      + '<ul><li>one</li><li>two</li></ul><blockquote>q</blockquote><pre><code class="language-go">x := 1</code></pre><hr><form action="https://example.com"><input value="in"></form>');
    assert.equal(await rtValue(page), '## Title\n\na **bold** **heavy** _it_ `c` bad [ok](https://example.com/ok) data\n\ncell\n\n- one\n- two\n\n> q\n\n```go\nx := 1\n```\n\n---');
    const dom = await page.evaluate(() => {
      const content = document.querySelector('#demo-rich .wk-richtext-editor-content');
      const tags = new Set([...content.querySelectorAll('*')].map(e => e.localName));
      const attrs = new Set([...content.querySelectorAll('*')].flatMap(e => [...e.attributes].map(a => a.name)));
      return { tags: [...tags].sort(), attrs: [...attrs].sort(), pwned: window.pwned ?? null, bodyHidden: getComputedStyle(document.body).display === 'none' };
    });
    for (const tag of dom.tags) assert.ok(['p', 'h1', 'h2', 'strong', 'em', 'code', 'a', 'br', 'ul', 'ol', 'li', 'blockquote', 'div', 'pre', 'hr', 'img'].includes(tag), tag);
    for (const attr of dom.attrs) assert.ok(['class', 'href', 'rel', 'data-language', 'data-tight', 'start', 'contenteditable', 'draggable', 'alt', 'mark-placeholder', 'data-placeholder'].includes(attr), attr);
    assert.equal(dom.pwned, null);
    assert.equal(dom.bodyHidden, false);
    // In an editor with fewer features, pasted HTML is cut down to them.
    await rtSet(page, '', 0, 'demo-rich-few');
    await paste('demo-rich-few', '<h1>Head</h1><p><b>b</b> <i>i</i> <code>c</code> <a href="https://example.com/">l</a></p><ol><li>n</li></ol><blockquote>q</blockquote>');
    assert.equal(await rtValue(page, 'demo-rich-few'), 'Head\n\n**b** _i_ c [l](https://example.com/)\n\n- n\n\nq');
    // Pasted HTML nested past the Markdown depth limit goes in as its text.
    await rtSet(page, '');
    await paste('demo-rich', '<blockquote>'.repeat(40) + '<p>deep</p><p>text</p>' + '</blockquote>'.repeat(40));
    assert.equal(await rtValue(page), 'deep\n\ntext');
    assert.equal(await page.evaluate(() => document.querySelectorAll('#demo-rich blockquote').length), 0);
    // Plain text pastes as text: Markdown in it is not read.
    await rtSet(page, '');
    await paste('demo-rich', '', '**not bold**\n# not a heading');
    assert.equal(await rtValue(page), '\\*\\*not bold\\*\\*\n\n\\# not a heading');
    // problems records any request outside the page's server.
    assert.deepEqual(problems, []);
    await page.close();
  });

  // Hostile HTML for the two checks below: each vector, if any document ran
  // or loaded it, would add its name to window.__pwned.
  const PWN = name => `(window.__pwned=(window.__pwned||[]).concat('${name}'))`;
  const HOSTILE = [
    `<p onclick="${PWN('p')}">text<img src="x" onerror="${PWN('img')}"><img src="https://example.com/i.png" onerror="${PWN('img-remote')}"></p>`,
    `<svg onload="${PWN('svg')}"><image href="x" onerror="${PWN('svg-image')}"></image></svg>`,
    `<details open ontoggle="${PWN('details')}"><summary>s</summary>d</details>`,
    `<video><source src="x" onerror="${PWN('video')}"></video><audio src="x" onerror="${PWN('audio')}"></audio>`,
    `<iframe srcdoc="<script>parent.__pwned=['iframe']</script>"></iframe><object data="x" onerror="${PWN('object')}"></object>`,
    `<input autofocus onfocus="${PWN('input')}"><body onload="${PWN('body')}"><style onload="${PWN('style')}">p{}</style>`,
    `<script>${PWN('script')}</script><a href="javascript:${PWN('href')}" onmouseover="${PWN('a')}">link</a>`,
    // A stray "<", a dropped tag, then the rest of a tag: must not join into <img onerror>.
    `<<img>img src=x onerror="${PWN('joined')}"><<img>svg onload="${PWN('joined-svg')}">`,
  ].join('');
  // Waits until a live image the page adds after the hostile HTML has failed
  // to load: the hostile images (were any live) would have failed by then.
  const settle = page => page.evaluate(() => new Promise(resolve => {
    const img = new Image();
    img.addEventListener('error', () => requestAnimationFrame(() => requestAnimationFrame(resolve)), { once: true });
    img.src = 'data:,';
  }));
  // Drops HTML onto the text, inside it (where ProseMirror takes the drop).
  const dropHtml = (page, id, html) => page.evaluate(([i, h]) => {
    const content = document.querySelector(`#${i} .wk-richtext-editor-content`);
    const box = content.getBoundingClientRect();
    const data = new DataTransfer();
    data.setData('text/html', h);
    content.dispatchEvent(new DragEvent('drop', { dataTransfer: data, bubbles: true, cancelable: true, clientX: box.left + 20, clientY: box.top + 15 }));
  }, [id, html]);
  const pasteHtml = (page, id, html) => page.evaluate(([i, h]) => {
    const content = document.querySelector(`#${i} .wk-richtext-editor-content`);
    content.focus();
    const data = new DataTransfer();
    data.setData('text/html', h);
    content.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }));
  }, [id, html]);

  await check('richtext: untrusted HTML reaches no parser with a handler, resource URL or inline style (paste and format html)', async () => {
    const { page, problems } = await open(RT);
    // Record every HTML string parsed outside the page: DOMParser, and innerHTML on another document's elements.
    await page.evaluate(() => {
      window.parsed = [];
      const parse = DOMParser.prototype.parseFromString;
      DOMParser.prototype.parseFromString = function (text, type) { window.parsed.push(String(text)); return parse.call(this, text, type); };
      const inner = Object.getOwnPropertyDescriptor(Element.prototype, 'innerHTML');
      Object.defineProperty(Element.prototype, 'innerHTML', { ...inner, set(v) { if (this.ownerDocument !== document) window.parsed.push(String(v)); inner.set.call(this, v); } });
    });
    await rtSet(page, '');
    await pasteHtml(page, 'demo-rich', HOSTILE);
    await page.evaluate(h => { document.getElementById('demo-rich-html').value = h; }, HOSTILE);
    // The joined "<img" stays text.
    assert.match(await rtValue(page), /^text\n\nsdlink\\<img src=x onerror="\(window\.\\_\\_pwned=.*\\<svg onload=/);
    // A drop reaches the same parser and goes in.
    const before = await page.evaluate(() => window.parsed.length);
    await dropHtml(page, 'demo-rich', `<p>DROPPED</p>${HOSTILE}`);
    assert.ok(await page.evaluate(n => window.parsed.length > n, before), 'the drop reached no parser');
    assert.match(await rtValue(page), /DROPPED/, 'the drop inserted nothing');
    const parsed = await page.evaluate(() => window.parsed);
    assert.ok(parsed.length >= 3, `only ${parsed.length} parses seen`);
    // Every "<" a parser is handed opens a tag the cleaner wrote (text has &lt;):
    // check each tag's name and attributes. Escaped text may read "onerror=".
    for (const html of parsed) {
      for (const tag of html.match(/<[^>]*>?/g) || []) {
        assert.match(tag, /^<\/?[a-z][a-z0-9]*(?:-[a-z0-9]+)*(?: [a-z-]+="[^"<>]*")*>$/, `${tag} in ${html}`);
        assert.doesNotMatch(tag, /^<\/?(img|svg|image|iframe|object|script|style|video|audio|source|input)(?=[\s>])/, `${tag} in ${html}`);
        for (const [, name, value] of tag.matchAll(/ ([a-z-]+)="([^"]*)"/g)) {
          assert.ok(['href', 'start', 'class', 'data-language', 'data-wk-style'].includes(name), `${tag} in ${html}`);
          assert.doesNotMatch(value, /javascript:/i, `${tag} in ${html}`);
        }
      }
    }
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: hostile HTML runs nothing on a page with no CSP, and raises no CSP violation on the strict page', async () => {
    for (const url of ['/editor/test/fixtures/no-csp.html', RT]) {
      const { page, problems } = await open(url);
      await page.evaluate(() => { window.violations = []; document.addEventListener('securitypolicyviolation', e => window.violations.push(`${e.violatedDirective}: ${e.sample || e.blockedURI}`)); });
      await rtSet(page, '');
      await pasteHtml(page, 'demo-rich', HOSTILE);
      await page.evaluate(h => { document.getElementById('demo-rich-html').value = h; }, HOSTILE);
      // Drop it too, inside the text.
      await dropHtml(page, 'demo-rich', `<p>DROPPED</p>${HOSTILE}`);
      assert.match(await rtValue(page), /DROPPED/, `${url}: the drop inserted nothing`);
      await settle(page);
      assert.deepEqual(await page.evaluate(() => ({ pwned: window.__pwned ?? null, violations: window.violations })), { pwned: null, violations: [] }, url);
      assert.deepEqual(problems, [], url);
      await page.close();
    }
  });

  await check('richtext: an edit that would take the Markdown past the delimiter or length limit is refused, with a message; the value still loads as Markdown', async () => {
    const { page, problems } = await open(RT);
    const errorText = () => page.textContent('#demo-rich .wk-richtext-editor-error');
    // 1,999 escaped asterisks are under the limit; typed ones stop at 2,000.
    await rtSet(page, '\\*'.repeat(1999));
    await page.keyboard.type('***');
    assert.equal(await rtValue(page), '\\*'.repeat(2000));
    assert.match(await errorText(), /more formatting in one paragraph than the editor can save/);
    // The next edit that fits clears the message.
    await page.keyboard.type('x');
    assert.equal(await errorText(), '');
    // Bold words: the one that would pass the limit is refused (Mod-B on a selection).
    await rtSet(page, Array.from({ length: 500 }, (_, i) => `**w${i}**`).join(' ') + ' last more');
    const before = await rtValue(page);
    await page.keyboard.press('Shift+ArrowLeft');
    await page.keyboard.press('Shift+ArrowLeft');
    await page.keyboard.press(`${mod}+b`);
    assert.equal(await rtValue(page), before);
    assert.match(await errorText(), /more formatting in one paragraph/);
    const r = await page.evaluate(async () => {
      const { wkRichTextEditor } = await import('/editor/dist/richtext.js');
      return wkRichTextEditor({ label: 'Again', value: document.getElementById('demo-rich').value }).hasAttribute('data-source');
    });
    assert.equal(r, false, 'the value went past the limits');
    // Length: at the limit, typing is refused.
    await page.evaluate(() => { document.getElementById('demo-rich').value = 'x'.repeat(100000); });
    await page.click('#demo-rich .wk-richtext-editor-content');
    await page.keyboard.press(`${mod}+End`);
    await page.keyboard.type('y');
    assert.equal((await rtValue(page)).length, 100000);
    assert.match(await errorText(), /too long to save/);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: HTML nested too deep loads as its text a line per block, not run together', async () => {
    const { page, problems } = await open(RT);
    const out = await page.evaluate(() => {
      const el = document.getElementById('demo-rich-html');
      el.value = '<blockquote>'.repeat(40) + '<p>one</p><p>two <b>bold</b></p><ul><li>three</li><li>four<br>five</li></ul><h4>six</h4>' + '</blockquote>'.repeat(40);
      return el.value;
    });
    assert.equal(out, '<p>one</p><p>two bold</p><p>three</p><p>four</p><p>five</p><p>six</p>');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: Escape in a modal dialog closes it when the editor has no oncancel; with oncancel it cancels', async () => {
    const { page, problems } = await open(RT);
    await page.evaluate(async () => {
      const { wkRichTextEditor } = await import('/editor/dist/richtext.js');
      for (const [id, cancels] of [['demo-dialog-plain', false], ['demo-dialog-cancel', true]]) {
        const dialog = document.createElement('dialog');
        dialog.id = id;
        dialog.append(wkRichTextEditor({ label: 'In a dialog', value: 'text', ...(cancels ? { oncancel: () => { window.cancelled = true; } } : {}) }));
        document.body.append(dialog);
      }
    });
    for (const id of ['demo-dialog-plain', 'demo-dialog-cancel']) {
      await page.evaluate(i => document.getElementById(i).showModal(), id);
      await page.click(`#${id} .wk-richtext-editor-content`);
      await page.keyboard.press('Escape');
    }
    assert.deepEqual(await page.evaluate(() => [document.getElementById('demo-dialog-plain').open, window.cancelled ?? false]), [false, true]);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: HTML format: in and out sanitised; hostile and too-deep HTML loads as text', async () => {
    const { page, problems } = await open(RT);
    const out = await page.evaluate(async () => {
      const el = document.getElementById('demo-rich-html');
      const first = el.value;
      el.value = '<p onclick="window.pwned=1">x<img src="https://example.com/i.png" onerror="window.pwned=1"><script>window.pwned=1</script> <a href="javascript:window.pwned=1">bad</a> <a href="https://example.com/a?b=1&amp;c=&quot;2&quot;">ok</a> <span style="font-weight:bold">heavy</span></p><h4>h</h4><style>body{display:none}</style>';
      const cleaned = el.value;
      const t0 = performance.now();
      el.value = '<blockquote>'.repeat(3000) + 'deep' + '</blockquote>'.repeat(3000);
      const deep = { value: el.value, ms: performance.now() - t0, quotes: el.querySelectorAll('blockquote').length };
      return { first, cleaned, deep, pwned: window.pwned ?? null };
    });
    assert.equal(out.first, '<p>Some <strong>HTML</strong> with <a href="https://example.com/h" rel="noopener noreferrer">a link</a>.</p><ul><li>one</li><li>two</li></ul>');
    assert.equal(out.cleaned, '<p>x bad <a href="https://example.com/a?b=1&amp;c=%222%22" rel="noopener noreferrer">ok</a> <strong>heavy</strong></p><h2>h</h2>');
    assert.equal(out.deep.value, '<p>deep</p>');
    assert.equal(out.deep.quotes, 0);
    assert.ok(out.deep.ms < 2000, `${out.deep.ms} ms`);
    assert.equal(out.pwned, null);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: hostile Markdown loads as its source, quickly, with formatting off; it goes out as it came; a normal value turns formatting back on', async () => {
    const { page, problems } = await open(RT);
    for (const [label, text] of [
      ['nested quotes', '>'.repeat(2000) + ' x'],
      ['emphasis markers', '*'.repeat(5000) + 'a' + '*'.repeat(5000)],
      ['a deep list', Array.from({ length: 200 }, (_, i) => `${'  '.repeat(i)}- level ${i}`).join('\n')],
    ]) {
      for (const how of ['spec', 'setter']) {
        const r = await page.evaluate(async ([t, h]) => {
          const { wkRichTextEditor } = await import('/editor/dist/richtext.js');
          const start = performance.now();
          let ed;
          if (h === 'spec') ed = wkRichTextEditor({ id: 'demo-hostile', label: 'Hostile', value: t });
          else { ed = wkRichTextEditor({ id: 'demo-hostile', label: 'Hostile', value: 'fine' }); ed.value = t; }
          document.getElementById('demo').append(ed);
          const ms = performance.now() - start;
          const out = { ms, same: ed.value === t, source: ed.hasAttribute('data-source'), tools: [...ed.querySelectorAll('[role=toolbar] button')].every(b => b.getAttribute('aria-disabled') === 'true') };
          ed.remove();
          return out;
        }, [text, how]);
        assert.ok(r.ms < 2000, `${label} (${how}): ${Math.round(r.ms)} ms`);
        assert.deepEqual({ ...r, ms: 0 }, { ms: 0, same: true, source: true, tools: true }, `${label} (${how})`);
      }
    }
    // In source mode the shortcuts and input rules do nothing; typing still works and goes out as written.
    await page.evaluate(() => { const el = document.getElementById('demo-rich'); el.value = '*'.repeat(5000); });
    await page.click('#demo-rich .wk-richtext-editor-content');
    await page.keyboard.press(`${mod}+End`);
    await page.keyboard.press('Enter');
    await page.keyboard.type('# x');
    await page.keyboard.press(`${mod}+b`);
    await page.keyboard.type('y');
    assert.equal(await rtValue(page), `${'*'.repeat(5000)}\n# xy`);
    await page.evaluate(() => { document.getElementById('demo-rich').value = '**ok**'; });
    assert.equal(await page.evaluate(() => document.getElementById('demo-rich').hasAttribute('data-source')), false);
    assert.equal(await page.evaluate(() => document.querySelector('#demo-rich [data-tool=bold]').getAttribute('aria-disabled')), null);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: no edit nests lists or quotes past the Markdown depth limit, so the value always loads again as Markdown', async () => {
    const { page, problems } = await open(RT);
    await rtSet(page, '- a');
    for (let i = 0; i < 40; i++) {
      await page.keyboard.press('Enter');
      await page.keyboard.type(`l${i}`);
      await page.keyboard.press(`${mod}+]`);
    }
    for (let i = 0; i < 40; i++) await page.click('#demo-rich [data-tool=quote]');
    const r = await page.evaluate(() => {
      const el = document.getElementById('demo-rich');
      return { value: el.value, lists: el.querySelectorAll('ul').length, quotes: el.querySelectorAll('blockquote').length };
    });
    assert.ok(r.lists > 5, `only ${r.lists} levels`);
    // The value loads as Markdown again (not as its source).
    const again = await page.evaluate(async v => {
      const { wkRichTextEditor } = await import('/editor/dist/richtext.js');
      return wkRichTextEditor({ label: 'Again', value: v }).hasAttribute('data-source');
    }, r.value);
    assert.equal(again, false, `${r.lists} lists and ${r.quotes} quotes went past the limit`);
    assert.ok(r.value.includes('l39'), 'typing stopped');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: keyboard only: Tab to the toolbar, arrows between its buttons, Tab to the text and on; Alt+F10 back to the toolbar', async () => {
    const { page, problems } = await open(RT);
    await page.focus('#demo-theme input:checked');
    await page.keyboard.press('Tab');
    const active = () => page.evaluate(() => document.activeElement.dataset.tool || document.activeElement.className || document.activeElement.textContent);
    assert.equal(await active(), 'bold');
    await page.keyboard.press('ArrowRight');
    assert.equal(await active(), 'italic');
    await page.keyboard.press('End');
    assert.equal(await active(), 'rule');
    await page.keyboard.press('ArrowRight');
    assert.equal(await active(), 'bold', 'arrows wrap');
    await page.keyboard.press('ArrowLeft');
    await page.keyboard.press('Home');
    await page.keyboard.press('ArrowRight');
    // One tab stop: the last button used.
    assert.deepEqual(await page.evaluate(() => [...document.querySelectorAll('#demo-rich [role=toolbar] button')].filter(b => b.tabIndex === 0).map(b => b.dataset.tool)), ['italic']);
    await page.keyboard.press('Tab');
    assert.equal(await rtFocused(page), true);
    // Select the line and make it italic from the toolbar by keyboard; focus comes back to the text.
    await page.evaluate(() => { document.getElementById('demo-rich').value = 'plain words'; });
    await page.keyboard.press(`${mod}+End`);
    await page.keyboard.press('Shift+Home');
    await page.keyboard.press('Alt+F10');
    assert.equal(await active(), 'italic');
    await page.keyboard.press('Enter');
    assert.equal(await rtFocused(page), true);
    assert.equal(await rtValue(page), '_plain words_');
    // Tab leaves the text (no trap): on to the actions.
    await page.keyboard.press('Tab');
    assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Cancel');
    await page.keyboard.press('Shift+Tab');
    assert.equal(await rtFocused(page), true);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: submitting: an empty text shows an error until fixed; Mod-Enter and Save submit; a pending onsubmit keeps Save busy; Escape cancels', async () => {
    const { page, problems } = await open(RT);
    const look = () => page.evaluate(() => {
      const el = document.getElementById('demo-rich');
      const content = el.querySelector('.wk-richtext-editor-content');
      const save = el.querySelector('[data-submit]');
      return { error: el.querySelector('.wk-richtext-editor-error').textContent, role: el.querySelector('.wk-richtext-editor-error').getAttribute('role'), invalid: content.getAttribute('aria-invalid'), described: content.getAttribute('aria-describedby'), busy: save.hasAttribute('data-busy'), ariaDisabled: save.getAttribute('aria-disabled'), disabled: save.disabled };
    });
    await rtSet(page, '');
    await page.evaluate(() => { window.kitEvents = []; });
    await page.keyboard.press(`${mod}+Enter`);
    let l = await look();
    assert.deepEqual({ ...l, described: !!l.described }, { error: 'Write something first.', role: 'status', invalid: 'true', described: true, busy: false, ariaDisabled: null, disabled: false });
    assert.equal(await rtFocused(page), true);
    // Typing it right clears the error.
    await page.keyboard.type('Ready');
    l = await look();
    assert.equal(l.error, '');
    assert.equal(l.invalid, null);
    await page.keyboard.press(`${mod}+Enter`);
    await page.click('#demo-rich [data-submit]');
    assert.deepEqual(await page.evaluate(() => window.kitEvents.filter(e => e.type === 'wk-submit').map(e => e.detail.value)), ['Ready', 'Ready']);
    assert.equal(await rtValue(page), 'Ready', 'Mod-Enter changed the text');
    // Busy while onsubmit's promise is pending: focusable, aria-disabled, data-busy; another submit does nothing.
    await page.evaluate(() => { window.kitEvents = []; window.pendingSubmit = new Promise(r => { window.settle = r; }); });
    await page.click('#demo-rich [data-submit]');
    l = await look();
    assert.deepEqual([l.busy, l.ariaDisabled, l.disabled], [true, 'true', false]);
    // (aria-disabled: Playwright would wait for it to be enabled.)
    await page.click('#demo-rich [data-submit]', { force: true });
    assert.equal(await page.evaluate(() => document.getElementById('demo-rich').submit()), false);
    assert.equal(await page.evaluate(() => window.kitEvents.filter(e => e.type === 'wk-submit').length), 1);
    await page.evaluate(() => window.settle());
    await until(page, () => !document.querySelector('#demo-rich [data-submit]').hasAttribute('data-busy'));
    // A cancelled wk-submit does not reach onsubmit.
    await page.evaluate(() => { window.pendingSubmit = undefined; document.getElementById('demo-rich').addEventListener('wk-submit', e => e.preventDefault(), { once: true }); });
    assert.equal(await page.evaluate(() => document.getElementById('demo-rich').submit()), false);
    // Escape in the text cancels: wk-cancel, then oncancel.
    await page.evaluate(() => { window.kitEvents = []; });
    await page.click('#demo-rich .wk-richtext-editor-content');
    await page.keyboard.press('Escape');
    assert.deepEqual(await page.evaluate(() => window.kitEvents.map(e => e.type)), ['wk-cancel']);
    assert.match(await page.textContent('#demo-log'), /^cancel/);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: disabled blocks edits, the toolbar and submit and keeps focus in the editor; enabling restores them; read-only has no toolbar', async () => {
    const { page, problems } = await open(RT);
    const id = 'demo-rich-disabled';
    const look = () => page.evaluate(i => {
      const root = document.getElementById(i);
      const content = root.querySelector('.wk-richtext-editor-content');
      return { aria: content.getAttribute('aria-disabled'), editable: content.getAttribute('contenteditable'), data: root.hasAttribute('data-disabled'), tools: [...root.querySelectorAll('[role=toolbar] button')].every(b => b.disabled), action: root.querySelector('[data-submit]').disabled };
    }, id);
    assert.deepEqual(await look(), { aria: 'true', editable: 'false', data: true, tools: true, action: true });
    const before = await rtValue(page, id);
    await page.click(`#${id} .wk-richtext-editor-content`, { force: true });
    await page.keyboard.type('x');
    await page.keyboard.press(`${mod}+b`);
    assert.equal(await rtValue(page, id), before);
    assert.equal(await page.evaluate(i => document.getElementById(i).submit(), id), false);
    await page.evaluate(i => { document.getElementById(i).disabled = false; }, id);
    assert.deepEqual(await look(), { aria: null, editable: 'true', data: false, tools: false, action: false });
    await page.click(`#${id} .wk-richtext-editor-content`);
    await page.keyboard.press(`${mod}+End`);
    await page.keyboard.type('!');
    assert.equal(await rtValue(page, id), `${before}!`);
    // Disabling while a toolbar button has focus parks focus on the editor (a named group); enabling sends it back to the text.
    await page.focus(`#${id} [data-tool=bold]`);
    await page.evaluate(i => { document.getElementById(i).disabled = true; }, id);
    assert.deepEqual(await page.evaluate(i => [document.activeElement.id, document.activeElement.getAttribute('role'), document.activeElement.getAttribute('aria-label')], id), [id, 'group', 'Locked notes']);
    await page.evaluate(i => { document.getElementById(i).disabled = false; }, id);
    assert.equal(await rtFocused(page, id), true);
    // Read-only: no toolbar, a focusable read-only textbox, no edits.
    const ro = await page.evaluate(() => {
      const root = document.getElementById('demo-rich-readonly');
      const content = root.querySelector('.wk-richtext-editor-content');
      return { toolbar: root.querySelector('[role=toolbar]').hidden, readonly: content.getAttribute('aria-readonly'), tabindex: content.getAttribute('tabindex'), editable: content.getAttribute('contenteditable') };
    });
    assert.deepEqual(ro, { toolbar: true, readonly: 'true', tabindex: '0', editable: 'false' });
    const text = await rtValue(page, 'demo-rich-readonly');
    await page.focus('#demo-rich-readonly .wk-richtext-editor-content');
    await page.keyboard.type('x');
    await page.keyboard.press(`${mod}+b`);
    assert.equal(await rtValue(page, 'demo-rich-readonly'), text);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: the API: events and callbacks in order, value set quietly, options checked, placeholder, destroy', async () => {
    const { page, problems } = await open(RT);
    const result = await page.evaluate(async () => {
      const { wkRichTextEditor, FEATURES } = await import('/editor/dist/richtext.js');
      const calls = [];
      const el = wkRichTextEditor({ label: 'Note', placeholder: 'Say something', minHeight: 60, maxHeight: '120px', onchange: v => calls.push(['onchange', v]) });
      el.addEventListener('wk-change', e => calls.push(['wk-change', e.detail.value]));
      document.getElementById('demo').append(el);
      const content = el.querySelector('.wk-richtext-editor-content');
      const placeholder = getComputedStyle(content.querySelector('p'), '::before').content;
      const small = content.getBoundingClientRect().height;
      el.value = 'a\n\n'.repeat(40);
      const big = content.getBoundingClientRect().height;
      const quiet = calls.length;
      const errors = [];
      for (const spec of [{}, { label: ' ' }, { label: 'x', format: 'rtf' }, { label: 'x', features: ['bold', 'tables'] }]) {
        try { wkRichTextEditor(spec); errors.push('none'); } catch (e) { errors.push(e.constructor.name); }
      }
      el.destroy(); el.destroy();
      el.value = 'later';
      return { placeholder, aria: content.getAttribute('aria-placeholder'), small, big, quiet, errors, children: el.childElementCount, submit: el.submit(), features: FEATURES, format: el.format };
    });
    assert.equal(result.placeholder, '"Say something"');
    assert.equal(result.aria, 'Say something');
    assert.equal(Math.round(result.small), 60);
    assert.equal(Math.round(result.big), 120);
    assert.equal(result.quiet, 0, 'setting value fired something');
    assert.deepEqual(result.errors, ['TypeError', 'TypeError', 'TypeError', 'TypeError']);
    assert.equal(result.children, 0);
    assert.equal(result.submit, false);
    assert.equal(result.format, 'markdown');
    assert.deepEqual(result.features, ['bold', 'italic', 'code', 'link', 'heading', 'bulletList', 'orderedList', 'quote', 'codeBlock', 'rule']);
    // Typing fires wk-change then onchange, with the Markdown.
    await page.evaluate(async () => {
      const { wkRichTextEditor } = await import('/editor/dist/richtext.js');
      window.order = [];
      const el = wkRichTextEditor({ id: 'demo-order', label: 'Order', onchange: v => window.order.push(`onchange:${v}`), onsubmit: () => window.order.push('onsubmit') });
      for (const t of ['wk-change', 'wk-submit']) el.addEventListener(t, e => window.order.push(`${t}:${JSON.stringify(e.detail)}`));
      document.getElementById('demo').append(el);
    });
    await page.click('#demo-order .wk-richtext-editor-content');
    await page.keyboard.press(`${mod}+b`);
    await page.keyboard.type('a');
    await page.keyboard.press(`${mod}+Enter`);
    assert.deepEqual(await page.evaluate(() => window.order), ['wk-change:{"value":"**a**"}', 'onchange:**a**', 'wk-submit:{"value":"**a**"}', 'onsubmit']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('richtext: forced colours: the box edge, the focus ring, a pressed button and the placeholder stay visible', async () => {
    const { page, problems } = await open(RT, { forcedColors: 'active' });
    await rtSet(page, '**bold**');
    await page.keyboard.press('ArrowLeft');
    const look = await page.evaluate(() => {
      const content = document.querySelector('#demo-rich .wk-richtext-editor-content');
      const s = getComputedStyle(content);
      const b = getComputedStyle(document.querySelector('#demo-rich [data-tool=bold]'));
      const empty = getComputedStyle(document.querySelector('#demo-rich-empty .wk-richtext-editor-content p'), '::before');
      return {
        canvasText: window.t.colour('CanvasText'), highlight: window.t.colour('Highlight'), highlightText: window.t.colour('HighlightText'), gray: window.t.colour('GrayText'),
        border: `${s.borderTopStyle} ${s.borderTopColor}`, outline: `${s.outlineStyle} ${s.outlineColor}`, pressedBg: b.backgroundColor, pressedText: b.color,
        placeholder: empty.color, separator: getComputedStyle(document.querySelector('#demo-rich .wk-richtext-editor-separator')).borderLeftColor,
      };
    });
    assert.equal(look.border, `solid ${look.canvasText}`);
    assert.equal(look.outline, `solid ${look.highlight}`);
    assert.equal(look.pressedBg, look.highlight);
    assert.equal(look.pressedText, look.highlightText);
    assert.equal(look.placeholder, look.gray);
    assert.equal(look.separator, look.canvasText);
    assert.deepEqual(problems, []);
    await page.close();
  });
} finally { await browser.close(); await new Promise(resolve => server.close(resolve)); }

if (failures.length) {
  console.log(`Editor browser checks: ${failures.length} failed.`);
  process.exit(1);
}
console.log('Editor browser checks passed.');
