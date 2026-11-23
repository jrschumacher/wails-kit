// Browser checks for the code editor, on the review pages in demo/
// (index.html, themed.html), which load the prebuilt dist/code.js as an app
// vendors it. Run with: npm run test:browser
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
    assert.deepEqual(await page.evaluate(() => window.calls), [['onchange', 'ab'], ['wk-change', 'ab'], ['onchange', 'abc'], ['wk-change', 'abc']]);
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
    assert.deepEqual(panel, { role: 'search', label: 'Find', buttons: ['kit:Next', 'kit:Previous', 'kit:All', 'kit:Close', 'kit:Replace', 'kit:Replace all'], checks: 3 });
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
      return { children: el.childElementCount, editors: document.querySelectorAll('#scratch .cm-editor').length };
    });
    assert.deepEqual(after, { children: 0, editors: 0 });
    assert.deepEqual(problems, []);
    await page.close();
  });
} finally { await browser.close(); await new Promise(resolve => server.close(resolve)); }

if (failures.length) {
  console.log(`Editor browser checks: ${failures.length} failed.`);
  process.exit(1);
}
console.log('Editor browser checks passed.');
