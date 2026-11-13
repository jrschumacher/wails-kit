// Static checks on the stylesheets, the theme files and the package's
// boundaries. Run with: npm test (node --test test/).
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { COLOUR, rules, tokens, banned, isTokenSelector, backdropProblems, over, contrast } from './css.mjs';

const root = fileURLToPath(new URL('../', import.meta.url));
const src = path.join(root, 'src');
const themeDir = path.join(root, 'theme');
const read = p => readFileSync(p, 'utf8');
const walk = dir => readdirSync(dir).flatMap(name => {
  const p = path.join(dir, name);
  return statSync(p).isDirectory() ? walk(p) : [p];
});
const rel = p => path.relative(root, p);
const kitCss = walk(src).filter(p => p.endsWith('.css') && !p.endsWith('ui.css'));
const themes = walk(themeDir).filter(p => p.endsWith('.css'));
const demoCss = path.join(root, 'demo', 'demo.css');
const kitTokens = () => tokens(read(path.join(src, 'tokens.css')));

// The tokens a theme file may set (theme/theme.css documents them).
export const THEME_TOKENS = [
  '--wk-color-accent', '--wk-color-on-accent', '--wk-color-selection',
  '--wk-color-working', '--wk-color-working-ring',
  '--wk-color-attention', '--wk-color-attention-text', '--wk-color-attention-fill', '--wk-color-destructive-text',
  '--wk-color-chart-1', '--wk-color-chart-3',
  '--wk-font-family', '--wk-font-features',
  '--wk-radius-control', '--wk-radius-group',
];
// Properties a component sets on itself for its variants to change.
const LOCAL = new Set(['--wk-dot-color', '--wk-chart-c']);

test('the colour check catches every kind of literal', () => {
  const component = { selector: '.wk-x', forced: false };
  const forced = { selector: '.wk-x', forced: true };
  for (const v of ['teal', 'Teal', '1px solid rebeccapurple', '#abc', '#a1b2c3d4', 'rgb(1 2 3)', 'rgba(0,0,0,.5)', 'hsl(0 0% 0%)', 'hwb(0 0% 0%)',
    'lab(50% 0 0)', 'lch(50% 0 0)', 'oklab(.5 0 0)', 'oklch(.5 0 0)', 'color(srgb 1 0 0)', 'linear-gradient(navy, transparent)', 'CanvasText']) {
    assert.notDeepEqual(banned(component, v), [], `${v} passes`);
  }
  for (const v of ['var(--wk-color-text)', 'transparent', 'currentColor', 'inherit', '1px solid var(--wk-color-hairline)', '"red"',
    'linear-gradient(45deg, transparent 50%, currentColor 50%)', 'background-color .12s', 'none', 'tabular-nums']) {
    assert.deepEqual(banned(component, v), [], `${v} fails`);
  }
  assert.deepEqual(banned(forced, 'Highlight'), []);
  assert.notDeepEqual(banned(forced, 'teal'), []);
});

test('colours in component rules come from tokens (::backdrop and forced colours excepted)', () => {
  for (const file of [...kitCss, ...themes, demoCss]) {
    for (const r of rules(read(file))) {
      if (isTokenSelector(r.selector) || r.selector === '@font-face') continue;
      for (const [prop, value] of r.decls) {
        if (r.selector.includes('::backdrop')) {
          // ::backdrop does not inherit custom properties in older WebKit
          // (macOS 13): it keeps a literal colour, with its own dark override.
          assert.ok(!value.includes('var('), `${rel(file)}: ${r.selector} uses a token; it must use a literal colour`);
          continue;
        }
        assert.deepEqual(banned(r, value), [], `${rel(file)}: ${r.selector} { ${prop}: ${value} } hard-codes a colour outside the token blocks`);
      }
    }
  }
});

test('every ::backdrop rule has a dark override, under the media query and under data-theme="dark"', () => {
  for (const file of kitCss) assert.deepEqual(backdropProblems(read(file)), [], rel(file));
});

test('token coverage: every var(--wk-…) used anywhere in the package is defined by tokens.css', () => {
  const declared = kitTokens().light;
  const files = [...walk(src), ...walk(path.join(root, 'demo')), ...themes, path.join(root, 'dist', 'wails-kit-ui.css'), path.join(root, 'dist', 'wails-kit-ui.js')]
    .filter(p => /\.(css|js|mjs|html)$/.test(p));
  for (const file of files) {
    for (const [, name] of read(file).matchAll(/var\(\s*(--wk-[\w-]+)/g)) {
      assert.ok(declared.has(name) || LOCAL.has(name), `${rel(file)} uses ${name}, which tokens.css does not declare`);
    }
  }
  for (const file of kitCss.filter(f => !f.endsWith('tokens.css'))) {
    for (const r of rules(read(file))) {
      for (const [p] of r.decls) if (p.startsWith('--wk-')) assert.ok(LOCAL.has(p), `${rel(file)}: ${r.selector} sets ${p}; tokens live in tokens.css`);
    }
  }
});

test('token blocks: light on :root, dark under the media query and again under data-theme="dark", with the same values', () => {
  for (const file of [path.join(src, 'tokens.css'), ...themes]) {
    const t = tokens(read(file));
    assert.deepEqual(t.misplaced, [], `${rel(file)}: token blocks outside :root / :root:not([data-theme="light"]) in the dark media query / :root[data-theme="dark"]`);
    assert.ok(t.dark.size > 0, `${rel(file)} has no dark tokens`);
    assert.deepEqual([...t.attrDark.entries()].sort(), [...t.dark.entries()].sort(), `${rel(file)}: the data-theme="dark" block must hold the media query's dark values`);
  }
});

test('every colour token has a light and a dark value', () => {
  const kit = kitTokens();
  assert.ok(kit.light.size >= 40, `tokens.css declares only ${kit.light.size} tokens`);
  for (const [name, value] of kit.light) {
    if (COLOUR.test(value)) assert.ok(kit.dark.has(name), `tokens.css: ${name} has no dark value`);
  }
  for (const [name, value] of kit.dark) {
    assert.ok(kit.light.has(name), `tokens.css: ${name} has a dark value but no light one`);
    assert.ok(COLOUR.test(value), `tokens.css: ${name} changes in dark but is not a colour`);
  }
});

test('a theme sets only the documented theme tokens, each colour in light and dark', () => {
  const kit = kitTokens();
  const doc = read(path.join(themeDir, 'theme.css'));
  for (const name of THEME_TOKENS) {
    assert.ok(kit.light.has(name), `${name} is a theme token the kit does not define`);
    assert.ok(doc.includes(name), `theme/theme.css does not document ${name}`);
  }
  assert.ok(themes.length >= 2, 'theme/ holds theme.css and an example');
  for (const file of themes) {
    const t = tokens(read(file));
    for (const [name, value] of t.light) {
      assert.ok(THEME_TOKENS.includes(name), `${rel(file)} sets ${name}, which is not a theme token`);
      if (COLOUR.test(value)) assert.ok(t.dark.has(name), `${rel(file)}: ${name} has no dark value`);
    }
    for (const name of t.dark.keys()) assert.ok(t.light.has(name), `${rel(file)}: ${name} has a dark value but no light one`);
  }
});

test('theme/theme.css as shipped holds the kit defaults, so linking it unchanged changes nothing', () => {
  const kit = kitTokens();
  const t = tokens(read(path.join(themeDir, 'theme.css')));
  const norm = v => v.replace(/\s+/g, '');
  for (const name of THEME_TOKENS) {
    assert.equal(norm(t.light.get(name) ?? ''), norm(kit.light.get(name)), `${name} (light)`);
    if (kit.dark.has(name)) assert.equal(norm(t.dark.get(name) ?? ''), norm(kit.dark.get(name)), `${name} (dark)`);
  }
});

test('secondary text on a selected row or a count chip reaches 4.5:1, and the working dot\'s ring 3:1, with every theme', () => {
  const kit = kitTokens();
  for (const file of [null, ...themes]) {
    const theme = file ? tokens(read(file)) : null;
    const label = file ? rel(file) : 'kit';
    for (const block of ['light', 'dark']) {
      const v = name => theme?.[block].get(name) || kit[block].get(name) || kit.light.get(name);
      const text = v('--wk-color-text-secondary-on-selected');
      for (const surface of ['--wk-color-bg', '--wk-color-sidebar', '--wk-color-surface']) {
        const ground = v(surface);
        const selected = over(v('--wk-color-selection'), ground);
        const grounds = { selected, chip: over(v('--wk-color-fill-strong'), ground), 'chip on selected': over(v('--wk-color-fill-strong'), selected) };
        for (const [what, g] of Object.entries(grounds)) {
          const ratio = contrast(text, g);
          assert.ok(ratio >= 4.5, `${label} ${block}: secondary text on ${what} over ${surface} is ${ratio.toFixed(2)}:1`);
        }
      }
      const ring = contrast(v('--wk-color-working-ring'), v('--wk-color-bg'));
      assert.ok(ring >= 3, `${label} ${block}: the working ring is ${ring.toFixed(2)}:1 on the background`);
      const onAccent = contrast(v('--wk-color-on-accent'), v('--wk-color-accent'));
      assert.ok(onAccent >= 4.5, `${label} ${block}: text on the accent is ${onAccent.toFixed(2)}:1`);
    }
  }
});

// Split a selector list at top-level commas (not those inside :where() etc).
const selectors = list => {
  const out = [];
  let depth = 0, start = 0;
  for (let i = 0; i < list.length; i++) {
    if (list[i] === '(') depth++;
    else if (list[i] === ')') depth--;
    else if (list[i] === ',' && depth === 0) { out.push(list.slice(start, i).trim()); start = i + 1; }
  }
  out.push(list.slice(start).trim());
  return out;
};

test('every selector is scoped to a wk- class or element, so linking the kit never restyles a host app', () => {
  for (const file of [...kitCss, ...themes]) {
    for (const r of rules(read(file))) {
      if (r.selector.startsWith('@')) continue;
      for (const sel of selectors(r.selector)) {
        if (isTokenSelector(sel)) {
          for (const [prop] of r.decls) assert.ok(prop.startsWith('--wk-'), `${rel(file)}: :root sets ${prop}; :root is for --wk-* tokens only`);
          continue;
        }
        assert.ok(/\.wk-|(^|[\s>+~(,])wk-[a-z]/.test(sel), `${rel(file)}: "${sel}" is not scoped to a wk- class or element`);
      }
    }
  }
});

test('one CSS entry and one ES module entry; no shadow DOM, customized built-ins or popover API', () => {
  const entry = read(path.join(src, 'ui.css'));
  const imported = [...entry.matchAll(/@import\s+"([^"]+)"/g)].map(m => m[1]);
  assert.deepEqual(imported, ['tokens.css', 'base.css', 'components.css', 'settings.css']);
  for (const file of walk(src).filter(p => p.endsWith('.js'))) {
    const js = read(file);
    assert.ok(!/attachShadow|\bis\s*=|\{\s*extends\s*:|showPopover|togglePopover|\bpopover\s*=|\[popover\]/.test(js), `${rel(file)} uses shadow DOM, a customized built-in or the popover API`);
  }
  const all = walk(src).filter(p => p.endsWith('.js')).map(read).join('\n');
  for (const name of ['wk-dialog', 'wk-menu', 'wk-context-menu', 'wk-tabs', 'wk-tooltip', 'wk-settings']) {
    assert.match(all, new RegExp(`define\\w*\\('${name}'`), `nothing defines ${name}`);
  }
});

test('nothing names an app, and nothing loads from outside the package', () => {
  const shipped = [...walk(src), ...themes, ...walk(path.join(root, 'demo')), ...walk(path.join(root, 'dist')), ...walk(path.join(root, 'scripts')),
    path.join(root, 'README.md'), path.join(root, 'package.json')];
  const outside = ref => !ref.startsWith('data:') && (/^[a-z][a-z0-9+.-]*:|^\//i.test(ref));
  for (const file of shipped) {
    const text = read(file);
    for (const app of ['muster', 'peanut']) assert.ok(!new RegExp(app, 'i').test(text), `${rel(file)} names ${app}`);
    const refs = [];
    if (file.endsWith('.css')) {
      for (const m of text.matchAll(/@import\s+(?:url\()?\s*["']?([^"')\s;]+)/g)) refs.push(m[1]);
      for (const m of text.matchAll(/url\(\s*["']?([^"')]+)/g)) refs.push(m[1]);
    } else if (/\.m?js$/.test(file)) {
      for (const m of text.matchAll(/^\s*import\s*(?:[\w*{}\s,]+from\s*)?\(?\s*["'`]([^"'`]+)["'`]/gm)) refs.push(m[1]);
      for (const m of text.matchAll(/new URL\(\s*["'`]([^"'`]+)["'`]/g)) refs.push(m[1]);
    } else if (file.endsWith('.html')) {
      for (const m of text.matchAll(/<(?!a\b)[a-z]+\b[^>]*?\s(?:src|href|srcset|data)\s*=\s*["']([^"']+)["']/gi)) refs.push(m[1]);
    }
    if (file.startsWith(src) || file.startsWith(themeDir)) {
      // The kit itself loads nothing outside its own folder.
      for (const ref of refs) assert.ok(!outside(ref) && !/(^|\/)\.\.(\/|$)/.test(ref), `${rel(file)} loads ${ref} from outside its folder`);
    } else {
      for (const ref of refs) if (!/^node:/.test(ref)) assert.ok(!outside(ref), `${rel(file)} loads ${ref} from outside the package`);
    }
  }
});
