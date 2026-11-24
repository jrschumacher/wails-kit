// Static checks for the component gallery: the server serves only what the
// gallery shows, the pages keep the strict CSP and load nothing from the
// network, the page list is whole, and the helpers that show an example's
// source and a design-system card work.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { resolve } from '../serve.mjs';
import { dedent, body } from '../source.js';
import { parseCard } from '../card.js';
import { pages, groups } from '../pages/index.js';

const here = fileURLToPath(new URL('../', import.meta.url));
const frontend = path.join(here, '..');
const CSP = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; form-action 'none'";
const sources = () => [
  'index.html', 'gallery.js', 'gallery.css', 'card.js', 'source.js', 'serve.mjs',
  ...readdirSync(path.join(here, 'pages')).map(f => `pages/${f}`),
].map(f => [f, readFileSync(path.join(here, f), 'utf8')]);

test('the server maps the four packages and the design-system cards, and refuses the rest', () => {
  assert.equal(resolve('/gallery/'), path.join(frontend, 'gallery', 'index.html'));
  assert.equal(resolve('/ui/src/ui.css'), path.join(frontend, 'ui', 'src', 'ui.css'));
  assert.equal(resolve('/editor/dist/code.js'), path.join(frontend, 'editor', 'dist', 'code.js'));
  assert.equal(resolve('/diagram/dist/diagram.js'), path.join(frontend, 'diagram', 'dist', 'diagram.js'));
  assert.equal(resolve('/design-system/Button/README.md'), path.join(frontend, '..', 'docs', 'design-system', 'components', 'Button', 'README.md'));
  for (const refused of ['/', '/settings/package.json', '/types/index.ts', '/ui/../../go.mod', '/ui/node_modules/playwright-core/package.json',
    '/gallery/node_modules/x.js', '/design-system/Button/preview.html', '/design-system/../README.md', '/design-system/a-b/README.md', '/ui\\..\\x', '/package.json']) {
    assert.equal(resolve(refused), null, refused);
  }
});

test('the page keeps the demo pages\' strict CSP, with no inline script or style', () => {
  const html = readFileSync(path.join(here, 'index.html'), 'utf8');
  assert.ok(html.includes(`<meta http-equiv="Content-Security-Policy" content="${CSP}">`));
  assert.doesNotMatch(html, /<script(?![^>]*\bsrc=)[^>]*>/);
  assert.doesNotMatch(html, /<style/);
  for (const [file, text] of sources()) {
    // A style attribute would be blocked: properties are set through the CSSOM.
    assert.doesNotMatch(text, /\sstyle="/, file);
    assert.doesNotMatch(text, /\son[a-z]+="/, file);
  }
});

test('nothing in the gallery loads from the network', () => {
  for (const [file, text] of sources()) {
    assert.doesNotMatch(text, /(src|href)=["']?(https?:)?\/\//i, file);
    assert.doesNotMatch(text, /import\s*\(?\s*['"]https?:/, file);
    assert.doesNotMatch(text, /url\(\s*['"]?(https?:)?\/\//i, file);
    assert.doesNotMatch(text, /fetch\(\s*['"`]https?:/, file);
  }
});

test('every page has a unique id, a known group and at least one example with something to show', () => {
  const ids = pages.map(p => p.id);
  assert.equal(new Set(ids).size, ids.length);
  assert.ok(!ids.includes('all') && !ids.includes(''));
  for (const page of pages) {
    assert.match(page.id, /^[a-z][a-z0-9-]*$/);
    assert.ok(groups.some(g => g.id === page.group), `${page.id}: group ${page.group}`);
    assert.ok(page.examples.length > 0, page.id);
    for (const ex of page.examples) assert.ok(ex.title && (ex.html || ex.build), `${page.id}: ${ex.title}`);
  }
  for (const g of groups) assert.ok(pages.some(p => p.group === g.id), `group ${g.id} is empty`);
  // A card a page names exists.
  for (const page of pages.filter(p => p.card)) {
    readFileSync(path.join(frontend, '..', 'docs', 'design-system', 'components', page.card, 'README.md'));
  }
});

test('an example\'s source is shown without its indent or its signature', () => {
  assert.equal(dedent('\n    <div>\n      <b>x</b>\n    </div>\n'), '<div>\n  <b>x</b>\n</div>');
  assert.equal(dedent('<div class="g-row">\n  <a>one</a>\n</div>'), '<div class="g-row">\n  <a>one</a>\n</div>');
  const obj = {
    after(el, { kit }) {
      const x = (a, b) => { return a + b; };
      el.textContent = String(x(1, kit));
    },
    async build(host) {
      host.append(await Promise.resolve('x'));
    },
  };
  assert.equal(body(obj.after), 'const x = (a, b) => { return a + b; };\nel.textContent = String(x(1, kit));');
  assert.equal(body(obj.build), "host.append(await Promise.resolve('x'));");
  assert.equal(body(async (host, { kit }) => { host.append(kit); }), 'host.append(kit);');
});

test('a design-system card gives its "When to use" and "Do and don\'t" sections', () => {
  const card = parseCard(readFileSync(path.join(frontend, '..', 'docs', 'design-system', 'components', 'Button', 'README.md'), 'utf8'));
  assert.deepEqual(card.map(s => s.title), ['When to use', "Do and don't"]);
  assert.ok(card[0].blocks.some(b => b.kind === 'list' && b.items.length >= 3));
  assert.deepEqual(parseCard('# X\n\n## Classes\n\n- a\n\n## When to use\n\nOne line\nwraps.\n\n- b\n- c\n'), [
    { title: 'When to use', blocks: [{ kind: 'p', text: 'One line wraps.' }, { kind: 'list', items: ['b', 'c'] }] },
  ]);
});
