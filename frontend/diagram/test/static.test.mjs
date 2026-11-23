// Static checks, the same rules @wails-kit/ui holds itself to: colours come
// from --wk-* tokens that ui/src/tokens.css defines, every selector is scoped,
// nothing names an app, and nothing loads from the network.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import path from 'node:path';
import { root } from '../scripts/build.mjs';
import { banned, rules, tokens, splitSelectors } from '../../ui/test/css.mjs';

const read = p => readFileSync(p, 'utf8');
const walk = dir => readdirSync(dir).flatMap(name => {
  const p = path.join(dir, name);
  return statSync(p).isDirectory() ? walk(p) : [p];
});
const rel = p => path.relative(root, p);
const kitTokens = tokens(read(path.join(root, '..', 'ui', 'src', 'tokens.css'))).light;
const frameCss = read(path.join(root, 'src', 'diagram.css'));

test('the frame stylesheet takes colours from tokens (system colours only for forced colours)', () => {
  const all = rules(frameCss);
  assert.ok(all.length > 15);
  for (const r of all) {
    for (const [prop, value] of r.decls) assert.deepEqual(banned(r, value), [], `diagram.css: ${r.selector} { ${prop}: ${value} } hard-codes a colour`);
  }
  // Forced colours: the frame, its focus ring and the diagram's text.
  const forced = all.filter(r => r.forced).map(r => r.selector).join(' ');
  for (const want of ['.wk-diagram-viewport', '.wk-diagram-viewport:focus-visible', '.wk-diagram-canvas text']) assert.ok(forced.includes(want), `no forced-colours rule for ${want}`);
});

test('every frame selector starts with .wk-diagram', () => {
  for (const r of rules(frameCss)) {
    if (r.selector.startsWith('from') || r.selector.startsWith('to') || /^\d+%$/.test(r.selector)) continue; // keyframes
    for (const sel of splitSelectors(r.selector)) assert.match(sel, /^\.wk-diagram[\w-]*([\s.>:[]|$)/, `diagram.css: "${sel}" is not scoped`);
  }
});

test('token coverage: every var(--wk-…) used is defined by @wails-kit/ui tokens.css', () => {
  const files = [...walk(path.join(root, 'src')), ...walk(path.join(root, 'demo')), path.join(root, 'dist', 'diagram.js')].filter(p => /\.(css|js|html)$/.test(p));
  for (const file of files) {
    for (const [, name] of read(file).matchAll(/var\(\s*(--wk-[\w-]+)/g)) assert.ok(kitTokens.has(name), `${rel(file)} uses ${name}, which ui/src/tokens.css does not declare`);
    for (const [, name] of read(file).matchAll(/'(--wk-[\w-]*\w)'/g)) assert.ok(kitTokens.has(name), `${rel(file)} reads ${name}, which ui/src/tokens.css does not declare`);
  }
});

test('the demo stylesheet uses tokens only and scopes every selector', () => {
  for (const r of rules(read(path.join(root, 'demo', 'demo.css')))) {
    for (const [prop, value] of r.decls) assert.deepEqual(banned(r, value), [], `demo.css: ${r.selector} { ${prop}: ${value} }`);
    for (const sel of r.selector.split(',')) assert.match(sel.trim(), /^:where\(\.wk-root\)\s/, `demo.css: "${sel.trim()}" is not scoped under :where(.wk-root)`);
  }
});

test('nothing names an app, and nothing loads from the network', () => {
  const shipped = [...walk(path.join(root, 'src')), ...walk(path.join(root, 'demo')), ...walk(path.join(root, 'dist')), ...walk(path.join(root, 'scripts')),
    path.join(root, 'README.md'), path.join(root, 'package.json')];
  for (const file of shipped) {
    const text = read(file);
    for (const app of ['muster', 'peanut']) assert.ok(!new RegExp(app, 'i').test(text), `${rel(file)} names ${app}`);
  }
  // src/ imports packages (bundled at build), its own files and, from
  // index.js, the lazy ./mermaid.js beside it in dist/.
  for (const file of walk(path.join(root, 'src')).filter(p => p.endsWith('.js'))) {
    const text = read(file);
    for (const [, ref] of text.matchAll(/^\s*(?:import|export)\s[^;]*?from\s*'([^']+)'/gm)) {
      assert.ok(!/^[a-z][a-z0-9+.-]*:|^\//i.test(ref), `${rel(file)} imports ${ref}`);
      if (ref.startsWith('.')) assert.ok(path.resolve(path.dirname(file), ref).startsWith(path.join(root, 'src')), `${rel(file)} imports ${ref} from outside src/`);
    }
    const dynamic = [...text.matchAll(/\bimport\s*\(\s*(['"`][^'"`]*['"`])\s*\)/g)].map(m => m[1]);
    assert.deepEqual(dynamic, rel(file) === 'src/index.js' ? ["'./mermaid.js'"] : [], `${rel(file)} imports at run time`);
    assert.ok(!/\bfetch\(|new Worker|XMLHttpRequest|WebSocket/.test(text), `${rel(file)} loads something at run time`);
  }
  // The demo loads the package's dist/ and @wails-kit/ui's stylesheet and theme, by relative path.
  for (const file of walk(path.join(root, 'demo'))) {
    const text = read(file);
    const refs = [...text.matchAll(/<(?!a\b)[a-z]+\b[^>]*?\s(?:src|href)\s*=\s*["']([^"']+)["']/gi), ...text.matchAll(/^\s*import\s[^;]*?from\s*'([^']+)'/gm)].map(m => m[1]);
    for (const ref of refs) {
      if (ref === 'data:,') continue;
      assert.ok(!/^[a-z][a-z0-9+.-]*:|^\//i.test(ref), `${rel(file)} loads ${ref}`);
      const target = path.resolve(path.dirname(file), ref);
      assert.ok(target.startsWith(root) || target.startsWith(path.join(root, '..', 'ui') + path.sep), `${rel(file)} loads ${ref} from outside the diagram and ui packages`);
    }
  }
});
