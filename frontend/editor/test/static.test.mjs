// Static checks, the same rules @wails-kit/ui holds itself to: colours come
// from --wk-* tokens that ui/src/tokens.css defines, every selector is scoped,
// nothing names an app, and nothing loads from the network.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import path from 'node:path';
import { root } from '../scripts/build.mjs';
import { editorTheme, syntax } from '../src/shared/theme-spec.js';
import { previewStyles, editorStyles } from '../src/markdown/style-spec.js';
import { richtextStyles } from '../src/richtext/style-spec.js';
// The kit's own CSS helpers and tokens: this package builds on @wails-kit/ui.
import { banned, rules, tokens } from '../../ui/test/css.mjs';

const read = p => readFileSync(p, 'utf8');
const walk = dir => readdirSync(dir).flatMap(name => {
  const p = path.join(dir, name);
  return statSync(p).isDirectory() ? walk(p) : [p];
});
const rel = p => path.relative(root, p);
const kitTokens = tokens(read(path.join(root, '..', 'ui', 'src', 'tokens.css'))).light;
const forcedKey = /^@media\s*\(\s*forced-colors\s*:\s*active\s*\)$/;

// Every [selector, property, value, forced] in the theme spec.
function declarations(spec, forced = false, selector = '') {
  const out = [];
  for (const [key, value] of Object.entries(spec)) {
    if (value && typeof value === 'object') out.push(...declarations(value, forced || forcedKey.test(key), key.startsWith('@') ? selector : key));
    else out.push([selector, key, String(value), forced]);
  }
  return out;
}
const themeDecls = [...declarations(editorTheme), ...syntax.map(([names, style]) => Object.entries(style).map(([p, v]) => [names.join(' '), p, v, false])).flat()];

test('theme colours come from tokens (system colours only for forced colours)', () => {
  assert.ok(themeDecls.length > 40);
  for (const [selector, prop, value, forced] of themeDecls) {
    assert.deepEqual(banned({ forced }, value), [], `theme-spec.js: ${selector} { ${prop}: ${value} } hard-codes a colour`);
  }
  // Forced colours keep the selection, the caret and the focus visible.
  const forced = themeDecls.filter(d => d[3]).map(d => `${d[0]} ${d[1]}`);
  for (const want of ['.cm-content caretColor', '.cm-cursor', '&.cm-editor.cm-focused outlineColor']) {
    assert.ok(forced.some(f => f.includes(want)), `no forced-colours rule for ${want}`);
  }
});

test('the editor theme uses the tokens the issue names', () => {
  const uses = (selector, prop, token) => themeDecls.some(([s, p, v]) => s.includes(selector) && p === prop && v.includes(`var(${token})`));
  assert.ok(uses('&.cm-editor', 'backgroundColor', '--wk-color-control'));
  assert.ok(uses('&.cm-editor', 'color', '--wk-color-text'));
  assert.ok(uses('.cm-gutters', 'color', '--wk-color-text-secondary'));
  assert.ok(uses('.cm-gutters', 'backgroundImage', '--wk-color-fill'));
  assert.ok(uses('.cm-selectionBackground', 'background', '--wk-color-selection'));
  assert.ok(uses('.cm-cursor', 'borderLeft', '--wk-color-accent'));
  assert.ok(uses('&.cm-editor.cm-focused', 'outline', '--wk-color-accent'));
  assert.ok(uses('.cm-scroller', 'fontFamily', '--wk-font-family-mono'));
  assert.ok(uses('.cm-scroller', 'fontSize', '--wk-font-size-sm'));
  for (const name of ['keyword', 'string', 'number', 'comment', 'type', 'function']) {
    assert.ok(syntax.some(([, style]) => style.color === `var(--wk-color-code-${name})`), `no syntax rule uses --wk-color-code-${name}`);
  }
});

test('every theme selector is scoped to the editor ("&.cm-editor")', () => {
  for (const [selector] of declarations(editorTheme)) {
    for (const part of selector.split(',')) assert.match(part.trim(), /^&\.cm-editor([\s.>:]|$)/, `theme-spec.js: "${part.trim()}" is not scoped to the editor`);
  }
});

test('the Markdown editor and preview styles: tokens only, scoped selectors, forced-colours rules', () => {
  for (const [name, spec, scope] of [['previewStyles', previewStyles, /^\.wk-markdown([\s.>:[]|$)/], ['editorStyles', editorStyles, /^\.wk-markdown-editor/], ['richtextStyles', richtextStyles, /^\.wk-richtext-editor([\s.>:[]|$)/]]) {
    const decls = declarations(spec);
    assert.ok(decls.length > 15, name);
    for (const [selector, prop, value, forced] of decls) {
      assert.deepEqual(banned({ forced }, value), [], `style-spec.js ${name}: ${selector} { ${prop}: ${value} } hard-codes a colour`);
      // The selector list's commas, not those inside :is().
      for (const part of selector.split(/,(?![^(]*\))/)) assert.match(part.trim(), scope, `style-spec.js ${name}: "${part.trim()}" is not scoped`);
    }
    assert.ok(decls.some(d => d[3]), `${name} has no forced-colours rules`);
  }
  // The preview uses the kit's type and colour tokens.
  const uses = (spec, selector, prop, token) => declarations(spec).some(([s, p, v]) => s === selector && p === prop && v.includes(`var(${token})`));
  assert.ok(uses(previewStyles, '.wk-markdown', 'color', '--wk-color-text'));
  assert.ok(uses(previewStyles, '.wk-markdown', 'fontSize', '--wk-font-size-base'));
  assert.ok(uses(editorStyles, '.wk-markdown-editor-count[data-over]', 'color', '--wk-color-attention-text'));
  // The rich-text box is a field: the control colours and the kit's focus ring.
  const content = '.wk-richtext-editor .wk-richtext-editor-content';
  assert.ok(uses(richtextStyles, content, 'backgroundColor', '--wk-color-control'));
  assert.ok(uses(richtextStyles, content, 'border', '--wk-color-control-border'));
  assert.ok(uses(richtextStyles, content, 'caretColor', '--wk-color-accent'));
  assert.ok(uses(richtextStyles, `${content}:focus`, 'outline', '--wk-color-accent'));
  assert.ok(uses(richtextStyles, '.wk-richtext-editor .wk-richtext-editor-toolbar .wk-button[aria-pressed="true"]', 'backgroundColor', '--wk-color-selection'));
});

test('token coverage: every var(--wk-…) used is defined by @wails-kit/ui tokens.css', () => {
  const files = [...walk(path.join(root, 'src')), ...walk(path.join(root, 'demo')), ...walk(path.join(root, 'dist'))].filter(p => /\.(css|js|html)$/.test(p));
  for (const file of files) {
    for (const [, name] of read(file).matchAll(/var\(\s*(--wk-[\w-]+)/g)) assert.ok(kitTokens.has(name), `${rel(file)} uses ${name}, which ui/src/tokens.css does not declare`);
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
  // src/ imports packages (bundled at build) and its own files, nothing else.
  for (const file of walk(path.join(root, 'src')).filter(p => p.endsWith('.js'))) {
    for (const [, ref] of read(file).matchAll(/^\s*(?:import|export)\s[^;]*?from\s*'([^']+)'/gm)) {
      assert.ok(!/^[a-z][a-z0-9+.-]*:|^\//i.test(ref), `${rel(file)} imports ${ref}`);
      if (ref.startsWith('.')) assert.ok(path.resolve(path.dirname(file), ref).startsWith(path.join(root, 'src')), `${rel(file)} imports ${ref} from outside src/`);
    }
    assert.ok(!/\bimport\s*\(|\bfetch\(|new Worker|XMLHttpRequest|WebSocket/.test(read(file)), `${rel(file)} loads something at run time`);
  }
  // The demo loads the package's dist/ and @wails-kit/ui's stylesheet and theme, by relative path.
  for (const file of walk(path.join(root, 'demo'))) {
    const text = read(file);
    const refs = [...text.matchAll(/<(?!a\b)[a-z]+\b[^>]*?\s(?:src|href)\s*=\s*["']([^"']+)["']/gi), ...text.matchAll(/^\s*import\s[^;]*?from\s*'([^']+)'/gm)].map(m => m[1]);
    for (const ref of refs) {
      if (ref === 'data:,') continue;
      assert.ok(!/^[a-z][a-z0-9+.-]*:|^\//i.test(ref), `${rel(file)} loads ${ref}`);
      const target = path.resolve(path.dirname(file), ref);
      assert.ok(target.startsWith(root) || target.startsWith(path.join(root, '..', 'ui') + path.sep), `${rel(file)} loads ${ref} from outside the editor and ui packages`);
    }
  }
});
