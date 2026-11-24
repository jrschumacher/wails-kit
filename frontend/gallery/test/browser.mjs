// Browser checks for the component gallery. Run with: npm run test:browser
// It needs playwright-core (a devDependency) and Chrome: set CHROME_PATH, or
// have Chrome at /usr/bin/google-chrome or the macOS default. Without Chrome
// it skips, unless CI is set, when it fails.
//
// It uses the gallery's own server (serve.mjs). Any request outside that
// server, any console error or warning (a CSP violation among them), any
// securitypolicyviolation event and any failed request fails the check it
// happens in. Each check reports on its own line. Waits are for conditions:
// a page is ready when <html data-gallery-ready> names its hash, and a "Try
// it" run has settled when window.galleryPending is back to 0.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { browser, origin, open as openPage, check, finish, ready, go, settled, CLOCK_T0, pauseClock } from './helpers.mjs';
const { pages, groups } = await import('../pages/index.js');
const routes = ['#/', ...pages.map(p => `#/${p.id}`)];

// Every class the page ever had on an element, and every CSP violation.
const watch = () => {
  window.galleryClasses = new Set();
  window.galleryCsp = [];
  window.galleryDiagrams = [];
  document.addEventListener('securitypolicyviolation', e => window.galleryCsp.push(`${e.violatedDirective} ${e.blockedURI}`));
  document.addEventListener('wk-render', e => { if (e.detail?.type) window.galleryDiagrams.push(e.detail.type); }, true);
  // The gallery's own frame (its sidebar, toolbar, notices and panes) uses
  // kit classes too; only what the pages draw counts, and <html class="wk-root">.
  const frame = '#gallery-nav, #gallery-toolbar, #gallery-forced, .gallery-app, .gallery-app > .wk-pane-list, #gallery-main, .gallery-skip';
  const note = el => {
    if (!el.classList || (el !== document.documentElement && (el.matches(frame) || el.closest('#gallery-nav, #gallery-toolbar, #gallery-forced')))) return;
    for (const c of el.classList) window.galleryClasses.add(c);
  };
  new MutationObserver(records => {
    for (const r of records) {
      if (r.type === 'attributes') note(r.target);
      for (const n of r.addedNodes) if (n.nodeType === 1) { note(n); n.querySelectorAll('[class]').forEach(note); }
    }
  }).observe(document, { subtree: true, childList: true, attributes: true, attributeFilter: ['class'] });
};

const open = (path = '/gallery/', options = {}) => openPage(path, { ...options, init: [watch] });
const until = (page, fn, arg) => page.waitForFunction(fn, arg, { timeout: 5000 });
const active = page => page.evaluate(() => {
  const a = document.activeElement;
  return !a || a === document.body ? 'BODY' : a.id || a.textContent.trim();
});

// Interactive nodes in the accessibility tree that have no name.
const INTERACTIVE = new Set(['button', 'link', 'textbox', 'searchbox', 'checkbox', 'radio', 'switch', 'combobox', 'slider', 'spinbutton', 'tab', 'menuitem', 'menuitemcheckbox', 'menuitemradio', 'option', 'listbox', 'menu', 'radiogroup', 'tablist', 'treeitem', 'gridcell']);
async function unnamed(page) {
  const cdp = await page.context().newCDPSession(page);
  const { nodes } = await cdp.send('Accessibility.getFullAXTree');
  await cdp.detach();
  return nodes.filter(n => !n.ignored && INTERACTIVE.has(n.role?.value) && !String(n.name?.value ?? '').trim())
    .map(n => `${n.role.value} (backend node ${n.backendDOMNodeId})`);
}

try {
  const variants = [
    ['light', '/gallery/', { colorScheme: 'light' }],
    ['dark', '/gallery/', { colorScheme: 'dark' }],
    ['example theme', '/gallery/?theme=example', { colorScheme: 'light' }],
    ['example theme, dark', '/gallery/?theme=example&appearance=dark', { colorScheme: 'light' }],
    ['forced colours', '/gallery/', { colorScheme: 'light', forcedColors: 'active' }],
  ];
  for (const [label, path, options] of variants) {
    await check(`every gallery page in ${label}: no console errors, no CSP violations, nothing from outside the server, unique ids`, async () => {
      const { page, problems } = await open(path, options);
      const bad = [];
      for (const hash of [...routes, '#/all']) {
        await go(page, hash);
        const ids = await page.evaluate(() => {
          const seen = new Map();
          for (const el of document.querySelectorAll('[id]')) seen.set(el.id, (seen.get(el.id) || 0) + 1);
          return [...seen].filter(([, n]) => n > 1).map(([id]) => id);
        });
        if (ids.length) bad.push(`${hash}: duplicate ids ${ids.join(', ')}`);
        const csp = await page.evaluate(() => window.galleryCsp.splice(0));
        if (csp.length) bad.push(`${hash}: CSP ${csp.join('; ')}`);
        if (problems.length) bad.push(`${hash}: ${problems.splice(0).join('; ')}`);
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false, `${hash}: the window scrolls sideways`);
      }
      if (label === 'forced colours') assert.equal(await page.isVisible('#gallery-forced-on'), true);
      else assert.equal(await page.isVisible('#gallery-forced-on'), false);
      assert.deepEqual(bad, []);
      await page.close();
    });
  }

  await check('every interactive element on every page has an accessible name', async () => {
    const { page, problems } = await open();
    const bad = [];
    for (const hash of routes) {
      await go(page, hash);
      const missing = await unnamed(page);
      if (missing.length) {
        const names = await page.evaluate(() => [...document.querySelectorAll('button, a[href], input, select, textarea, [role="button"], [role="tab"], [role="option"], [role^="menuitem"], [role="slider"]')]
          .filter(el => !el.closest('[hidden], [aria-hidden="true"]') && !(el.getAttribute('aria-label') || el.textContent.trim() || el.labels?.length || el.getAttribute('aria-labelledby') || el.title || el.placeholder))
          .map(el => el.outerHTML.slice(0, 120)));
        bad.push(`${hash}: ${missing.join(', ')} ${names.join(' | ')}`);
      }
    }
    assert.deepEqual(bad, []);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('every sidebar entry has its page, addressable by hash, with examples and their markup', async () => {
    const { page, problems } = await open();
    const entries = await page.evaluate(() => [...document.querySelectorAll('#gallery-nav a.wk-sidebar-item')].map(a => [a.getAttribute('href'), a.textContent.trim()]));
    assert.equal(entries.length, pages.length + 2);
    assert.deepEqual(entries.map(e => e[0]).slice(2), pages.map(p => `#/${p.id}`));
    const groupNames = await page.evaluate(() => [...document.querySelectorAll('#gallery-nav .wk-sidebar-heading')].map(h => h.textContent));
    assert.deepEqual(groupNames, groups.map(g => g.title));
    for (const [href, text] of entries.slice(2)) {
      await go(page, href);
      const shown = await page.evaluate(() => ({
        title: document.getElementById('gallery-title').textContent,
        current: document.querySelector('#gallery-nav [aria-current="page"]')?.getAttribute('href'),
        examples: [...document.querySelectorAll('.gallery-example')].map(s => ({
          filled: s.querySelector('.gallery-specimen').children.length > 0,
          code: [...s.querySelectorAll('.gallery-code .wk-code pre')].map(p => p.textContent.length),
        })),
        documentTitle: document.title,
        firstHeading: document.querySelector('h1, h2, h3, h4, h5, h6')?.id,
      }));
      assert.equal(shown.title, text, href);
      assert.equal(shown.current, href);
      assert.equal(shown.documentTitle, `${text} · Component gallery`);
      // The page's own h1 is the first heading: the frame has none before it.
      assert.equal(shown.firstHeading, 'gallery-title', href);
      assert.ok(shown.examples.length > 0, `${href} has no examples`);
      for (const ex of shown.examples) {
        assert.ok(ex.filled, `${href}: an empty example`);
        assert.ok(ex.code.length > 0 && ex.code.every(n => n > 0), `${href}: an example without its markup or script`);
      }
    }
    // A page loads straight from its address, and an unknown one says so.
    const direct = await open('/gallery/#/range');
    assert.equal(await direct.page.textContent('#gallery-title'), 'Range');
    assert.ok(await direct.page.$('#g-ranges .wk-range'));
    assert.deepEqual(direct.problems, []);
    await direct.page.close();
    await go(page, '#/no-such-thing');
    assert.equal(await page.textContent('#gallery-title'), 'No such page');
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('a page with a design-system card shows its guidance, as text', async () => {
    const { page, problems } = await open('/gallery/#/button');
    await until(page, () => document.querySelectorAll('.gallery-card li').length > 0);
    const card = await page.evaluate(() => ({
      headings: [...document.querySelectorAll('.gallery-card-heading')].map(h => h.textContent),
      code: document.querySelectorAll('.gallery-card code').length,
      scripts: document.querySelectorAll('.gallery-card script, .gallery-card [onerror]').length,
    }));
    assert.deepEqual(card.headings, ['When to use', "Do and don't"]);
    assert.ok(card.code > 0);
    assert.equal(card.scripts, 0);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('every component class in the kit\'s stylesheets appears on some gallery page', async () => {
    // Every .wk-* class components.css and base.css style, after each page
    // has drawn and each of its "Try it" buttons has run.
    const css = ['components.css', 'base.css'].map(f => readFileSync(new URL(`../../ui/src/${f}`, import.meta.url), 'utf8')).join('\n');
    const wanted = [...new Set(css.replace(/\/\*[\s\S]*?\*\//g, '').match(/\.wk-[a-z0-9-]+/g).map(c => c.slice(1)))];
    const { page, problems } = await open();
    for (const hash of routes) {
      await go(page, hash);
      for (const button of await page.$$('[data-gallery-try]')) {
        if (!(await button.isVisible()) || await button.getAttribute('aria-disabled') === 'true') continue;
        await button.click();
        await settled(page);
        // A try that opened a modal (the task list) is closed again.
        if (await page.evaluate(() => Boolean(document.querySelector('dialog[open]')))) {
          await page.keyboard.press('Escape');
          await until(page, () => !document.querySelector('dialog[open]'));
        }
      }
      // The chart's readout shows on the arrow keys; the diff's comment box on c.
      const plot = await page.$('.wk-chart-plot[tabindex="0"]');
      if (plot) { await plot.focus(); await page.keyboard.press('ArrowRight'); }
      // The + button follows the pointer over a line of a diff that takes
      // comments; c on a focused line opens the comment box.
      const line = await page.$('#g-review .wk-diff-line:not(.wk-diff-hunk) >> nth=4');
      if (line) {
        await line.hover();
        await until(page, () => document.querySelector('#g-review .wk-diff-add-comment'));
        await line.evaluate(el => el.focus());
        await page.keyboard.press('c');
        await until(page, () => document.querySelector('#g-review .wk-diff-compose'));
      }
    }
    const seen = await page.evaluate(() => [...window.galleryClasses]);
    const missing = wanted.filter(c => !seen.includes(c));
    assert.deepEqual(missing, []);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('the appearance switch sets <html data-theme>, keeps it in the address, and is read back from it', async () => {
    const { page, problems } = await open();
    const bg = () => page.evaluate(() => getComputedStyle(document.querySelector('.wk-root')).backgroundColor);
    assert.equal(await bg(), 'rgb(252, 252, 251)');
    await page.click('#gallery-appearance label:nth-child(3)');
    assert.equal(await page.getAttribute('html', 'data-theme'), 'dark');
    assert.equal(await bg(), 'rgb(24, 26, 31)');
    assert.match(page.url(), /appearance=dark/);
    await page.click('#gallery-appearance label:nth-child(1)');
    assert.equal(await page.getAttribute('html', 'data-theme'), null);
    assert.doesNotMatch(page.url(), /appearance=/);
    await page.close();
    const back = await open('/gallery/?appearance=light#/chip', { colorScheme: 'dark' });
    assert.equal(await back.page.getAttribute('html', 'data-theme'), 'light');
    assert.equal(await back.page.isChecked('#gallery-appearance input[value="light"]'), true);
    assert.equal(await back.page.textContent('#gallery-title'), 'Chip');
    assert.deepEqual([...problems, ...back.problems], []);
    await back.page.close();
  });

  await check('the theme switch links the theme file and the page follows it', async () => {
    const { page, problems } = await open('/gallery/#/tokens');
    const token = name => page.evaluate(n => getComputedStyle(document.documentElement).getPropertyValue(n).trim(), name);
    const accent = await token('--wk-color-accent');
    const shown = () => page.evaluate(() => [...document.querySelectorAll('.g-swatch')].find(s => s.textContent.startsWith('color-bg'))?.querySelector('.wk-small').textContent);
    assert.equal(await page.getAttribute('#gallery-theme', 'href'), null);
    await page.selectOption('#gallery-theme-select', 'warm-grounds');
    await until(page, () => getComputedStyle(document.documentElement).getPropertyValue('--wk-color-bg').trim() === '#fbfaf7');
    assert.equal(await page.getAttribute('#gallery-theme', 'href'), '../ui/test/fixtures/warm-grounds.css');
    assert.notEqual(await token('--wk-color-accent'), accent);
    assert.match(page.url(), /theme=warm-grounds/);
    // The tokens page redraws its values for the new theme.
    await until(page, () => [...document.querySelectorAll('.g-swatch')].some(s => s.textContent === 'color-bg#fbfaf7'));
    assert.equal(await shown(), '#fbfaf7');
    await page.selectOption('#gallery-theme-select', '');
    await until(page, () => getComputedStyle(document.documentElement).getPropertyValue('--wk-color-accent').trim() !== '#2d6a45');
    assert.equal(await token('--wk-color-accent'), accent);
    assert.doesNotMatch(page.url(), /theme=/);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('text size zooms the pages, and the forced-colours help opens in place', async () => {
    const { page, problems } = await open('/gallery/#/button');
    const zoom = () => page.evaluate(() => getComputedStyle(document.getElementById('gallery-main')).zoom);
    assert.equal(await zoom(), '1');
    await page.selectOption('#gallery-size', '150');
    assert.equal(await zoom(), '1.5');
    assert.match(page.url(), /size=150/);
    await page.selectOption('#gallery-size', '125');
    assert.equal(await zoom(), '1.25');
    assert.equal(await page.isVisible('#gallery-forced-help'), false);
    await page.click('#gallery-forced-toggle');
    assert.equal(await page.getAttribute('#gallery-forced-toggle', 'aria-expanded'), 'true');
    assert.equal(await page.isVisible('#gallery-forced-help'), true);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('"Kit from dist/" loads the prebuilt files and nothing from src/', async () => {
    const page = await browser.newPage();
    const urls = [];
    page.on('request', r => urls.push(new URL(r.url()).pathname));
    await page.goto(`${origin}/gallery/#/menu`);
    await ready(page, '#/menu');
    assert.ok(urls.includes('/ui/src/index.js'));
    await page.selectOption('#gallery-source', 'dist');
    await page.waitForURL(/source=dist/);
    await ready(page, '#/menu');
    const after = urls.slice(urls.lastIndexOf('/gallery/'));
    assert.ok(after.includes('/ui/dist/wails-kit-ui.js') && after.includes('/ui/dist/wails-kit-ui.css'), after.join(' '));
    assert.equal(after.some(u => u.startsWith('/ui/src/')), false, after.join(' '));
    // The kit from dist works: the menu opens.
    await page.click('#g-menu > button');
    await until(page, () => !document.querySelector('#g-menu [role="menu"]').hidden);
    await page.close();
  });

  await check('the gallery\'s own navigation works from the keyboard', async () => {
    const { page, problems } = await open();
    // The skip link is the first tab stop and goes to the page.
    await page.keyboard.press('Tab');
    assert.equal(await active(page), 'Skip to the component');
    await page.keyboard.press('Tab');
    assert.equal(await active(page), 'gallery-filter');
    await page.keyboard.press('ArrowDown');
    assert.equal(await active(page), 'Overview');
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('ArrowDown');
    assert.equal(await active(page), pages[0].title);
    await page.keyboard.press('End');
    assert.equal(await active(page), pages[pages.length - 1].title);
    await page.keyboard.press('Home');
    assert.equal(await active(page), 'Overview');
    await page.keyboard.press('ArrowUp');
    assert.equal(await active(page), 'gallery-filter');
    // Filter, then Down and Enter open the page and focus its title.
    await page.keyboard.type('range');
    assert.deepEqual(await page.evaluate(() => [...document.querySelectorAll('#gallery-nav li:not([hidden]) a')].map(a => a.textContent.trim())), ['Range']);
    await page.keyboard.press('ArrowDown');
    assert.equal(await active(page), 'Range');
    await page.keyboard.press('Enter');
    await ready(page, '#/range');
    assert.equal(await active(page), 'gallery-title');
    assert.equal(await page.getAttribute('#gallery-nav a[href="#/range"]', 'aria-current'), 'page');
    await page.fill('#gallery-filter', 'zzz');
    assert.equal(await page.isVisible('#gallery-filter-none'), true);
    await page.fill('#gallery-filter', '');
    assert.equal(await page.evaluate(() => document.querySelectorAll('#gallery-nav li[hidden], #gallery-nav [hidden].wk-sidebar-heading').length), 0);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('the skip link moves focus to the page and leaves the address alone', async () => {
    const { page, problems } = await open('/gallery/#/range');
    await page.focus('.gallery-skip');
    await page.keyboard.press('Enter');
    await until(page, () => document.activeElement?.id === 'gallery-main');
    assert.equal(new URL(page.url()).hash, '#/range');
    assert.equal(await page.textContent('#gallery-title'), 'Range');
    assert.equal(await page.title(), 'Range · Component gallery');
    // A hash that isn't a page ("#gallery-main" typed in) doesn't route either.
    await page.evaluate(() => { location.hash = '#gallery-main'; });
    await page.evaluate(() => new Promise(r => setTimeout(r, 300)));
    assert.equal(await page.textContent('#gallery-title'), 'Range');
    assert.ok(await page.$('#g-ranges .wk-range'));
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('leaving a page cancels what its "Try it" started: no task runs on, nothing stays pending', async () => {
    const { page, problems } = await open('/gallery/#/task');
    for (const s of ['done', 'long', 'parts', 'queued']) await page.click(`tr[data-scenario="${s}"] [data-gallery-try]`);
    await until(page, () => document.querySelector('tr[data-scenario="long"] .wk-task-state'));
    assert.ok(await page.evaluate(() => window.galleryPending) > 0);
    // Everything the announcer is given from here on.
    await page.evaluate(() => {
      window.said = [];
      const regions = () => document.querySelectorAll('[data-wk-announcer] [aria-live], [data-wk-announcer][aria-live]');
      new MutationObserver(() => { for (const r of regions()) if (r.textContent.trim()) window.said.push(r.textContent.trim()); })
        .observe(document.documentElement, { subtree: true, childList: true, characterData: true });
    });
    await go(page, '#/task-list');
    await settled(page);
    await until(page, () => document.querySelector('#g-tasks-footer .wk-task-summary')?.hidden === true);
    await page.click('#g-tasks-open');
    await until(page, () => document.querySelector('dialog[open]'));
    assert.deepEqual(await page.evaluate(() => [...document.querySelectorAll('dialog[open] .wk-task-row')].map(r => r.textContent)), []);
    await page.keyboard.press('Escape');
    // The cancel was the gallery's, not the person's: nothing says "Cancelled".
    await page.evaluate(() => new Promise(r => setTimeout(r, 2600)));
    assert.deepEqual(await page.evaluate(() => window.said.filter(w => /cancel/i.test(w))), []);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('a design-system card is drawn as text: markup in it shows literally', async () => {
    const page = await browser.newPage();
    await page.route('**/design-system/Button/README.md', route => route.fulfill({
      contentType: 'text/markdown',
      body: '# Button\n\n## When to use\n\n- <img src=x onerror="window.pwned=1"> and **<b>x</b>** and `<i>c</i>`\n\n## Do and don\'t\n\nPlain <script>window.pwned=2</script> text.\n',
    }));
    await page.goto(`${origin}/gallery/#/button`);
    await ready(page, '#/button');
    await until(page, () => document.querySelectorAll('.gallery-card li').length > 0);
    const card = await page.evaluate(() => ({
      item: document.querySelector('.gallery-card li').textContent,
      para: [...document.querySelectorAll('.gallery-card p')].map(p => p.textContent),
      images: document.querySelectorAll('.gallery-card img').length,
      scripts: document.querySelectorAll('.gallery-card script').length,
      nested: document.querySelectorAll('.gallery-card b b, .gallery-card i').length,
      bold: document.querySelector('.gallery-card li b')?.textContent,
      pwned: window.pwned,
    }));
    assert.equal(card.item, '<img src=x onerror="window.pwned=1"> and <b>x</b> and <i>c</i>');
    assert.ok(card.para.includes('Plain <script>window.pwned=2</script> text.'));
    assert.equal(card.bold, '<b>x</b>');
    assert.deepEqual([card.images, card.scripts, card.nested, card.pwned], [0, 0, 0, undefined]);
    await page.close();
  });

  await check('the Diagram page draws all six diagram types', async () => {
    const { page, problems } = await open('/gallery/#/diagram');
    const types = await page.evaluate(async () => [...(await import('/diagram/dist/diagram.js')).diagramTypes]);
    assert.equal(types.length, 6);
    const drawn = await page.evaluate(() => [...new Set(window.galleryDiagrams)].sort());
    assert.deepEqual(drawn, [...types].sort());
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('"Try it" runs the lifecycle: busy at once, the spinner from 400 ms, then back to rest', async () => {
    const { page, problems } = await open('/gallery/#/busy');
    await page.click('#g-busy-slow');
    assert.equal(await page.getAttribute('#g-busy-slow', 'aria-disabled'), 'true');
    assert.equal(await page.evaluate(() => document.getElementById('g-busy-slow').hasAttribute('data-busy')), true);
    assert.equal(await page.$('#g-busy-slow .wk-spinner'), null);
    await until(page, () => document.querySelector('#g-busy-slow .wk-spinner'));
    await settled(page);
    assert.equal(await page.evaluate(() => document.getElementById('g-busy-slow').hasAttribute('data-busy')), false);
    assert.equal(await page.textContent('#g-busy-words'), 'Exported');
    // The quick one never shows a spinner.
    await page.evaluate(() => {
      window.spun = false;
      new MutationObserver(() => { if (document.querySelector('#g-busy-quick .wk-spinner')) window.spun = true; }).observe(document.getElementById('g-busy-quick'), { childList: true, subtree: true });
    });
    await page.click('#g-busy-quick');
    await settled(page);
    assert.equal(await page.evaluate(() => window.spun), false);
    // A row that fails shows the attention dot and its words.
    await go(page, '#/task');
    await page.click('tr[data-scenario="failed"] [data-gallery-try]');
    await settled(page);
    assert.match(await page.textContent('tr[data-scenario="failed"] td'), /Couldn’t export/);
    assert.ok(await page.$('tr[data-scenario="failed"] td .wk-dot[data-tone="attention"]'));
    assert.deepEqual(problems, []);
    await page.close();
  });
  // ---- The lifecycle's "Try it" rows, on the fake clock --------------------
  // Each opens the page on the fake clock (from CLOCK_T0), stops it at a
  // fixed point with pauseClock, then moves it with page.clock.runFor.
  const onClock = async path => {
    const r = await open(path, { clock: true });
    await pauseClock(r.page);
    await r.page.evaluate(() => { window.taskStates = []; document.addEventListener('wk-task', e => window.taskStates.push(`${e.detail.state}`)); });
    return r;
  };
  const row = s => `tr[data-scenario="${s}"]`;
  // A press, as a click: Playwright's own click waits for animation frames,
  // which a paused clock holds back.
  const press = (page, sel) => page.locator(sel).evaluate(el => { el.focus(); el.click(); });
  const cell = (page, s) => page.locator(`${row(s)} td`).first();

  await check('Task page: work that might be long keeps Stop\'s space from the start and shows it at 10 s, with no layout shift', async () => {
    const { page, problems } = await onClock('/gallery/#/task');
    await press(page, `${row('maybe')} [data-gallery-try]`);
    const stop = page.locator(`${row('maybe')} .wk-task-stop`);
    const tryBox = () => page.locator(`${row('maybe')} [data-gallery-try]`).boundingBox();
    const rowBox = () => page.locator(row('maybe')).boundingBox();
    await page.clock.runFor(500);
    assert.equal(await stop.count(), 1, 'the space for Stop is kept from the start');
    assert.equal(await stop.isVisible(), false);
    assert.match(await cell(page, 'maybe').textContent(), /^Syncing/);
    const before = [await tryBox(), await rowBox()];
    await page.clock.runFor(9_400);
    assert.equal(await stop.isVisible(), false, 'Stop is hidden before 10 s');
    await page.clock.runFor(200);
    assert.equal(await stop.isVisible(), true, 'Stop shows at 10 s');
    assert.equal(await stop.textContent(), 'Stop');
    assert.deepEqual([await tryBox(), await rowBox()], before, 'nothing moved when Stop appeared');
    await page.clock.runFor(6_000);
    assert.deepEqual(await page.evaluate(() => window.taskStates), ['running', 'done']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('Task page: Stop… asks with an Alert; Keep keeps it running, Stop stops it and keeps what finished', async () => {
    const { page, problems } = await onClock('/gallery/#/task');
    await press(page, `${row('asks')} [data-gallery-try]`);
    const stop = page.locator(`${row('asks')} .wk-task-stop`);
    const alert = page.locator('#g-stop-alert dialog');
    await page.clock.runFor(2_600);
    assert.equal(await stop.isVisible(), true, 'work expected to be long shows Stop at once');
    assert.equal(await stop.textContent(), 'Stop…');
    await stop.evaluate(el => { el.focus(); el.click(); });
    await page.clock.runFor(10);
    assert.equal(await alert.isVisible(), true, 'Stop… opens its Alert');
    assert.match(await page.textContent('#g-stop-alert-body'), /^\d+ of 40 files are copied; the rest won’t be\.$/);
    assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Keep copying');
    await press(page, '#g-stop-alert button[value="keep"]');
    // The dialog's close event is a task of its own: wait for it, then move the clock.
    await until(page, () => !document.querySelector('#g-stop-alert dialog').open);
    await page.clock.runFor(1_000);
    assert.equal(await alert.isVisible(), false);
    assert.deepEqual(await page.evaluate(() => window.taskStates), ['running'], 'Keep copying keeps it running');
    await stop.evaluate(el => { el.focus(); el.click(); });
    await page.clock.runFor(10);
    await press(page, '#g-stop-alert button[value="stop"]');
    // The dialog's close event is a task of its own: wait for it, then move the clock.
    await until(page, () => !document.querySelector('#g-stop-alert dialog').open);
    await page.clock.runFor(1_000);
    assert.deepEqual(await page.evaluate(() => window.taskStates), ['running', 'stopped']);
    assert.match(await cell(page, 'asks').textContent(), /^Stopped · \d+ of 40 files$/);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('Task page: Esc cancels short work while focus is on its control, and not from elsewhere', async () => {
    const { page, problems } = await onClock('/gallery/#/task');
    const button = page.locator(`${row('esc')} [data-gallery-try]`);
    await button.evaluate(el => { el.focus(); el.click(); });
    await page.clock.runFor(1_000);
    assert.match(await cell(page, 'esc').textContent(), /^Checking/);
    assert.equal(await button.evaluate(b => b === document.activeElement), true);
    await page.keyboard.press('Escape');
    await page.clock.runFor(1_000);
    assert.deepEqual(await page.evaluate(() => window.taskStates.splice(0)), ['running', 'cancelled']);
    assert.equal(await cell(page, 'esc').textContent(), 'Ready', 'Cancel puts the row back');
    // With focus elsewhere, Esc leaves the work alone.
    await button.evaluate(el => { el.focus(); el.click(); });
    await page.clock.runFor(500);
    await page.evaluate(() => document.activeElement.blur());
    await page.keyboard.press('Escape');
    await page.clock.runFor(4_000);
    assert.deepEqual(await page.evaluate(() => window.taskStates), ['running', 'done']);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('Task page: work with no progress times out (here after 5 s) and fails with the attention dot', async () => {
    const { page, problems } = await onClock('/gallery/#/task');
    await press(page, `${row('timeout')} [data-gallery-try]`);
    await page.clock.runFor(4_900);
    assert.deepEqual(await page.evaluate(() => window.taskStates), ['running']);
    assert.match(await cell(page, 'timeout').textContent(), /^Exporting/);
    await page.clock.runFor(200);
    await page.clock.runFor(600);
    assert.deepEqual(await page.evaluate(() => window.taskStates), ['running', 'failed']);
    assert.equal(await cell(page, 'timeout').textContent(), 'Timed out after 5 s');
    assert.ok(await page.$(`${row('timeout')} td .wk-dot[data-tone="attention"]`));
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('Skeleton page: the app\'s loading line shows at 3 s and goes with the skeleton', async () => {
    const { page, problems } = await onClock('/gallery/#/skeleton');
    const line = page.locator('#g-skeleton-line');
    await press(page, '#g-skeleton-long');
    await page.clock.runFor(2_900);
    assert.equal(await page.locator('#g-skeleton-table .wk-skeleton').count() > 0, true, 'the skeleton shows');
    assert.equal(await line.isVisible(), false, 'no line before 3 s');
    await page.clock.runFor(200);
    assert.equal(await line.isVisible(), true, 'the line at 3 s');
    assert.equal(await line.textContent(), 'Fetching runs from server.example…');
    await page.clock.runFor(2_500);
    assert.equal(await line.isVisible(), false, 'the line goes with the skeleton');
    assert.equal(await page.locator('#g-skeleton-table .wk-skeleton').count(), 0);
    assert.equal(await page.locator('#g-skeleton-table tbody tr').count(), 3);
    // A fast load never shows it.
    await press(page, '#g-skeleton-slow');
    await page.clock.runFor(4_000);
    assert.equal(await line.isVisible(), false);
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('App undo page: the toast\'s Undo and Edit > Undo share the stack; with text focus, Edit > Undo undoes the typing first', async () => {
    const { page, problems } = await open('/gallery/#/undo');
    const names = () => page.evaluate(() => [...document.querySelectorAll('#g-undo-list li')].map(li => li.textContent));
    const undoItem = page.locator('#g-edit-undo');
    assert.equal(await undoItem.textContent(), 'Edit > Undo');
    assert.equal(await undoItem.getAttribute('aria-disabled'), 'true');
    await page.click('#g-undo-archive');
    assert.deepEqual((await names())[0], 'Lunch on Friday');
    await until(page, () => document.querySelector('.wk-toast:not([data-leaving]) .wk-toast-action'));
    assert.equal(await undoItem.textContent(), 'Edit > Undo Archive');
    // The toast's Undo undoes it.
    await page.click('.wk-toast-action');
    await until(page, () => document.querySelector('#g-undo-list li').textContent === 'Plans for May');
    assert.equal(await page.textContent('#g-edit-redo'), 'Edit > Redo Archive');
    // Archived again; typing in the field, Edit > Undo undoes the typing first.
    await page.click('#g-undo-archive');
    await page.fill('#g-undo-field', '');
    await page.focus('#g-undo-field');
    await page.keyboard.type('abc');
    await until(page, () => document.getElementById('g-edit-undo').textContent === 'Edit > Undo Typing');
    await undoItem.click();
    assert.equal(await page.inputValue('#g-undo-field'), '');
    assert.equal((await names())[0], 'Lunch on Friday', 'the archive stays');
    // Without text focus, Edit > Undo undoes the archive, and its toast closes.
    await page.evaluate(() => document.activeElement.blur());
    await until(page, () => document.getElementById('g-edit-undo').textContent === 'Edit > Undo Archive');
    await undoItem.click();
    await until(page, () => document.querySelector('#g-undo-list li').textContent === 'Plans for May');
    await until(page, () => !document.querySelector('.wk-toast:not([data-leaving])'));
    // Leaving the page disconnects the stand-in stack.
    await go(page, '#/toast');
    assert.deepEqual(await page.evaluate(async () => (await import('/ui/src/index.js')).wkUndo.state()), { undo: null, redo: null, busy: false, textFocus: false });
    assert.deepEqual(problems, []);
    await page.close();
  });

  await check('Announcer page: words go to the regions; while a modal is open they sit inside it, and go back when it closes', async () => {
    const { page, problems } = await open('/gallery/#/announcer');
    const where = () => page.evaluate(() => (document.wkAnnouncer.parentNode === document.getElementById('g-plain-modal') ? 'modal' : document.wkAnnouncer.parentNode.tagName));
    assert.equal(await where(), 'BODY');
    await page.click('#g-say');
    await until(page, () => document.getElementById('g-said').textContent.startsWith('polite: Saved'));
    await page.click('#g-say-modal');
    assert.equal(await page.evaluate(() => document.getElementById('g-plain-modal').matches(':modal')), true);
    assert.equal(await where(), 'modal');
    assert.equal(await page.textContent('#g-announcer-home'), 'the modal');
    await until(page, () => document.getElementById('g-said').textContent.startsWith('polite: Said from inside the modal'));
    await page.click('#g-plain-close');
    await until(page, () => document.wkAnnouncer.parentNode === document.body);
    assert.equal(await page.textContent('#g-announcer-home'), 'the page');
    assert.deepEqual(problems, []);
    await page.close();
  });
} finally {
  await finish();
}
