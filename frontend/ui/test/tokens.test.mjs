// Static checks on the stylesheets, the theme files and the package's
// boundaries. Run with: npm test (node --test test/).
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { COLOUR, rules, tokens, banned, isTokenSelector, backdropProblems, contrast, over } from './css.mjs';
import { checkTheme, contrastMore, derive, THEME_TOKENS, DERIVED, GROUNDS, IDENTITY } from '../theme/check.mjs';

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

const kitSource = () => read(path.join(src, 'tokens.css'));
const fixtures = path.join(root, 'test', 'fixtures');
// Properties a component sets on itself for its variants to change, and
// --wk-range-value and --wk-progress-value, which the app sets on a wk-range
// and a wk-progress.
const LOCAL = new Set(['--wk-dot-color', '--wk-dot-ground', '--wk-chart-c', '--wk-range-value', '--wk-progress-value', '--wk-diff-digits', '--wk-diff-indent']);

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

test('--wk-dot-ground, the surface a status dot sits on, follows wk-color-bg in light and dark', () => {
  const kit = kitTokens();
  const dot = rules(read(path.join(src, 'components.css'))).find(r => r.selector === '.wk-dot' && !r.atRules.length);
  const ground = Object.fromEntries(dot.decls)['--wk-dot-ground'];
  assert.equal(ground, 'var(--wk-color-bg)');
  assert.ok(kit.light.has('--wk-color-bg') && kit.dark.has('--wk-color-bg') && kit.attrDark.has('--wk-color-bg'), 'wk-color-bg has light and dark values');
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

test('theme/theme.css documents every theme token, and every theme token is a kit token', () => {
  const kit = kitTokens();
  const doc = read(path.join(themeDir, 'theme.css'));
  for (const name of THEME_TOKENS) {
    assert.ok(kit.light.has(name), `${name} is a theme token the kit does not define`);
    assert.ok(doc.includes(name), `theme/theme.css does not set ${name}`);
  }
  assert.ok(themes.length >= 2, 'theme/ holds theme.css and an example');
});

test('the theme check passes the kit\'s defaults, theme/theme.css, theme/example.css and a theme with tinted grounds', () => {
  assert.deepEqual(checkTheme('', kitSource(), { requireIdentity: false }), [], 'the kit with no theme');
  for (const file of [...themes, path.join(fixtures, 'warm-grounds.css')]) assert.deepEqual(checkTheme(read(file), kitSource()), [], rel(file));
});

test('the theme check fails on a working tone under 3:1 on the window; a named exception records the known gap', () => {
  const gold = read(path.join(fixtures, 'gold-working.css'));
  assert.deepEqual(checkTheme(gold, kitSource()).map(p => p.rule), ['working-on-bg/light']);
  // The design system records a gold working tone from the design system
  // (2.6:1 on the light window) as a known gap: the dot's shape and its words carry the state. Not loosened.
  assert.deepEqual(checkTheme(gold, kitSource(), { exceptions: { 'working-on-bg/light': 'gold working tone, 2.6:1; a known gap in the design system' } }), []);
  assert.deepEqual(checkTheme(read(path.join(themeDir, 'example.css')), kitSource(), { exceptions: { 'working-on-bg/light': 'stale' } }).map(p => p.rule), ['exceptions'], 'an exception that matches nothing is reported');
});

test('the theme check holds increased contrast to the contract: a low-contrast text that passes without it fails under contrast-more', () => {
  const low = read(path.join(fixtures, 'low-text.css'));
  const kit = kitSource();
  // Without the setting it passes: grey text at 4.5:1 or more on every ground.
  const without = kit.slice(0, kit.indexOf('/* Increased contrast'));
  assert.deepEqual(checkTheme(low, without), [], 'passes with no increased-contrast block');
  const problems = checkTheme(low, kit);
  assert.deepEqual([...new Set(problems.map(p => p.rule))], ['contrast-more/light', 'contrast-more/dark']);
  assert.ok(problems.some(p => /secondary text on sidebar is 3\.14:1, under 4\.5:1/.test(p.message)), problems.map(p => p.message).join('\n'));
  assert.ok(problems.some(p => /field border on bg is 2\.78:1, under 3:1/.test(p.message)));
  // Over the minimum but weaker than the theme's own value, which is under
  // the 7:1 ceiling: the mix may not come down.
  const weaker = low.replace('--wk-color-text:#707070;--wk-color-text-secondary:#707070;', '--wk-color-text:#4a4a4a;--wk-color-text-secondary:#606060;');
  const w = checkTheme(weaker, kit).filter(p => p.rule === 'contrast-more/light' && /secondary text on bg /.test(p.message));
  assert.equal(w.length, 1, checkTheme(weaker, kit).map(p => p.message).join('\n'));
  assert.match(w[0].message, /is 5\.33:1, under 6\.\d\d:1 \(the usual 6\.\d\d:1/, 'the floor is the usual value, under the ceiling');
  // A theme already stronger than the ceiling may come down to it: 11.37:1
  // secondary text in light becomes 8.86:1, over 7:1.
  assert.deepEqual(checkTheme(read(path.join(fixtures, 'strong-secondary.css')), kit), []);
  // The kit's secondary text on a selection is checked too: a dark accent's
  // selection under the mixed on-selected text.
  const onSelected = checkTheme(low, kit).filter(p => /on a selection/.test(p.message));
  assert.ok(onSelected.length > 0, 'secondary text on a selection is not checked under increased contrast');
  assert.deepEqual(contrastMore(kit), new Map(Object.entries(CONTRAST_MIX)), 'the check reads the mixes from tokens.css');
});

test('the theme check catches each way a theme breaks the contract', () => {
  const base = read(path.join(themeDir, 'example.css'));
  const kit = kitSource();
  // Adds declarations to every one of the theme's three blocks.
  const plus = (light, dark = light) => base.replace(/:root \{/, `:root {\n  ${light}`).replace(/(:root:not\(\[data-theme="light"\]\) \{)/, `$1\n    ${dark}`).replace(/(:root\[data-theme="dark"\] \{)/, `$1\n  ${dark}`);
  const rulesOf = css => [...new Set(checkTheme(css, kit).map(p => p.rule.replace(/\/.*/, '')))].sort();
  assert.deepEqual(rulesOf(plus('--wk-color-selection: rgba(0, 0, 0, .1);')), ['derived']);
  assert.deepEqual(rulesOf(plus('--wk-color-chart-1: #000000;')), ['derived']);
  assert.deepEqual(rulesOf(plus('--wk-color-thumb: #ffffff;')), ['tokens']);
  assert.deepEqual(rulesOf(plus('--wk-color-bg: #fcfcfb;', '--wk-color-bg: #181a1f;')), ['grounds'], 'one ground of ten');
  assert.deepEqual(rulesOf(base.replace(/ *--wk-color-accent: #7fcfb4;\n/g, '')), ['tokens'], 'a colour with no dark value');
  assert.deepEqual(rulesOf(base.replace('--wk-radius-group: 10px;', '--wk-radius-group: 11px;')), ['radii']);
  assert.deepEqual(rulesOf(base.replace('--wk-radius-control: 7px;', '--wk-radius-control: 7px;\n  --wk-font-family: "Avenir Next", sans-serif;')), ['font']);
  assert.deepEqual(rulesOf(base.replace('--wk-color-accent: #1f6b55;\n', '')), ['identity', 'tokens']);
  // Grounds: all ten from the warm fixture, then each rule broken in turn.
  const warm = read(path.join(fixtures, 'warm-grounds.css'));
  assert.deepEqual(rulesOf(warm.replace('--wk-color-field-border: #8a857a;', '--wk-color-field-border: #b9b4a9;')), ['field-border']);
  assert.deepEqual(rulesOf(warm.replace('--wk-color-text-secondary: #655f53;', '--wk-color-text-secondary: #8f897d;')), ['text-secondary']);
  assert.deepEqual(rulesOf(warm.replace('--wk-color-working: #b4740f;', '--wk-color-working: #b04a20;')), ['working-attention']);
  assert.deepEqual(rulesOf(warm.replace(/--wk-color-sidebar: #f1efe9;\n/, '')), ['grounds', 'tokens'], 'one ground left out of light (its dark value is left alone)');
  assert.deepEqual(rulesOf(warm.replace(/(@media[\s\S]*?--wk-color-accent: )#84c99a/, '$1#84c99b')), ['blocks']);
  assert.deepEqual(rulesOf(warm.replace('--wk-color-text: #23201b;', '--wk-color-text: #77726a;')), ['contrast-more', 'text'], 'low text fails, and its increased-contrast mixes fail too');
  assert.deepEqual(rulesOf(base.replace('--wk-color-attention: #bc3529;', '--wk-color-attention: #e89a92;')), ['attention-on-bg']);
  assert.deepEqual(rulesOf(base.replace('--wk-color-on-accent: #ffffff;', '--wk-color-on-accent: #6fa894;')), ['on-accent']);
  assert.deepEqual(rulesOf(warm.replace('--wk-color-accent: #2d6a45;', '--wk-color-accent: #000000;')), ['on-selected'], 'a black accent darkens the selected row under the kit\'s secondary text');
  // Shape: the three token blocks and @font-face, nothing else.
  assert.deepEqual(rulesOf(base + 'html { --wk-color-selection: red; }\n'), ['blocks']);
  assert.deepEqual(rulesOf(base + '[data-theme="dark"] { --wk-color-selection: red; }\n'), ['blocks']);
  assert.deepEqual(rulesOf(base + '.wk-button { color: red; }\n'), ['blocks']);
  assert.deepEqual(rulesOf(base + ':root { color: red; }\n'), ['blocks']);
  assert.deepEqual(rulesOf(base + '@media (min-width: 600px) { :root { --wk-color-accent: #1f6b55; } }\n'), ['blocks'], 'a token block under another at-rule');
  assert.deepEqual(rulesOf(base + '@supports (color: color-mix(in srgb, red, blue)) { :root { --wk-color-selection: color-mix(in srgb, var(--wk-color-accent) 20%, transparent); } }\n'), ['derived'], 'a theme with its own color-mix() block');
  assert.deepEqual(rulesOf(base + '@font-face { font-family: "App Sans"; src: url("fonts/app-sans.woff2") format("woff2"); }\n'), [], '@font-face is allowed');
  // Format: identity and ground colours are opaque #rrggbb, reported rather than thrown.
  assert.deepEqual(rulesOf(warm.replace('--wk-color-text-secondary: #655f53;', '--wk-color-text-secondary: rgba(0, 0, 0, .55);')), ['format'], 'a translucent secondary text');
  assert.deepEqual(rulesOf(base.replace('--wk-color-accent: #1f6b55;', '--wk-color-accent: #fff;')), ['format'], 'a short hex accent');
  assert.deepEqual(rulesOf(warm.replace('--wk-color-surface: #ffffff;', '--wk-color-surface: rgb(255, 255, 255);')), ['format'], 'an rgb() surface');
  // The fallback for engines without color-mix(): the four, as the kit derives them.
  const fallback = (sel, fill) => `@supports not (color: color-mix(in srgb, red, blue)) { ${sel} { --wk-color-selection: rgba(31, 107, 85, .12); --wk-color-chart-1: #1f6b55; --wk-color-destructive-text: #a32d22; --wk-color-attention-fill: ${fill}; } }\n`;
  const withFallback = fill => base + fallback(':root', fill);
  assert.deepEqual(rulesOf(withFallback('rgba(188, 53, 41, .08)')), []);
  assert.deepEqual(rulesOf(withFallback('rgba(188, 53, 41, .2)')), ['fallback']);
  assert.deepEqual(rulesOf(base + '@supports not (color: color-mix(in srgb, red, blue)) { :root { --wk-color-thumb: #ffffff; } }'), ['fallback']);
});

test('theme/check.mjs runs from the command line: exit 0 on a theme that keeps the contract, 1 with its problems', () => {
  const cli = path.join(themeDir, 'check.mjs');
  const run = (...args) => { try { return { code: 0, out: execFileSync(process.execPath, [cli, ...args], { encoding: 'utf8' }) }; } catch (e) { return { code: e.status, out: e.stdout }; } };
  assert.equal(run(path.join(themeDir, 'example.css')).code, 0);
  const gold = run(path.join(fixtures, 'gold-working.css'));
  assert.equal(gold.code, 1);
  assert.match(gold.out, /^working-on-bg\/light: /m);
  assert.equal(run(path.join(fixtures, 'gold-working.css'), '--except', 'working-on-bg/light').code, 0);
  assert.equal(run().code, 2, 'usage');
});

test('contrast refuses a translucent colour; lay it over its ground first', () => {
  assert.throws(() => contrast('rgba(0, 0, 0, .5)', '#ffffff'), /translucent/);
  assert.ok(contrast(over('rgba(0, 0, 0, .5)', '#ffffff'), '#ffffff') > 3);
});

test('derived colours: selection, chart-1, destructive text and attention fill come from the accent and attention inside @supports, with literal fallbacks that match', () => {
  const kit = kitTokens();
  for (const mode of ['light', 'dark', 'attrDark']) {
    assert.deepEqual([...kit.derived[mode].keys()].sort(), [...DERIVED].sort(), `${mode}: the derived block sets exactly the four`);
    const dark = mode !== 'light';
    const want = {
      '--wk-color-selection': `color-mix(in srgb, var(--wk-color-accent) ${dark ? 16 : 12}%, transparent)`,
      '--wk-color-chart-1': 'var(--wk-color-accent)',
      '--wk-color-destructive-text': 'var(--wk-color-attention-text)',
      '--wk-color-attention-fill': `color-mix(in srgb, var(--wk-color-attention) ${dark ? 12 : 8}%, transparent)`,
    };
    for (const [name, value] of Object.entries(want)) assert.equal(kit.derived[mode].get(name), value, `${mode} ${name}`);
    // The fallback literals are what the derivation gives for the kit's own colours.
    const block = mode === 'light' ? kit.light : kit[mode];
    const get = name => block.get(name) ?? kit.light.get(name);
    const norm = v => v.replace(/\s+/g, '').replace(/,0?\./g, ',.');
    for (const [name, value] of Object.entries(derive(get, dark ? 'dark' : 'light'))) assert.equal(norm(get(name)), norm(value), `${mode}: the fallback for ${name}`);
  }
  for (const file of themes) assert.equal(tokens(read(file)).derived.light.size, 0, `${rel(file)} derives nothing`);
});

test('field border: text fields, selects, search, steppers, checkboxes, radios and switches use --wk-color-field-border; buttons and segmented controls keep the soft control border', () => {
  const css = rules(read(path.join(src, 'components.css'))).filter(r => !r.atRules.length);
  const border = sel => css.filter(r => r.selector === sel).flatMap(r => r.decls).filter(([p]) => p === 'border').map(([, v]) => v).join(' ');
  for (const sel of ['.wk-field', '.wk-select', '.wk-checkbox', '.wk-radio', '.wk-switch']) assert.match(border(sel), /--wk-color-field-border/, sel);
  assert.match(css.find(r => r.selector === '.wk-stepper-arrows').decls.find(([p]) => p === 'border')[1], /--wk-color-field-border/);
  assert.match(border('.wk-button'), /--wk-color-control-border/);
  assert.ok(GROUNDS.includes('--wk-color-field-border') && IDENTITY.length === 5);
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

// The syntax colours of @wails-kit/editor's code editor and highlighter.
// They are kit-held (a theme does not set them).
export const CODE_TOKENS = ['keyword', 'string', 'number', 'comment', 'type', 'function'].map(n => `--wk-color-code-${n}`);

test('every code syntax colour reaches 4.5:1 on the control ground, in light and dark', () => {
  const kit = kitTokens();
  for (const name of CODE_TOKENS) {
    assert.ok(!THEME_TOKENS.includes(name), `${name} is kit-held, not a theme token`);
    for (const block of ['light', 'dark', 'attrDark']) {
      const colour = kit[block].get(name), ground = kit[block].get('--wk-color-control');
      assert.ok(colour && ground, `tokens.css: ${name} or --wk-color-control has no ${block} value`);
      const ratio = contrast(colour, ground);
      assert.ok(ratio >= 4.5, `tokens.css ${block}: ${name} is ${ratio.toFixed(2)}:1 on --wk-color-control`);
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
      // A keyframe's from, to and percentages: its name must be wk- instead.
      const keyframes = r.atRules.find(a => a.startsWith('@keyframes'));
      if (keyframes) { assert.match(keyframes, /^@keyframes wk-[a-z-]+$/, `${rel(file)}: ${keyframes} is not named wk-`); continue; }
      for (const sel of selectors(r.selector)) {
        if (isTokenSelector(sel) || sel === CONTRAST_SELECTOR) {
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
  const shipped = [...walk(src), ...walk(themeDir), ...walk(path.join(root, 'demo')), ...walk(path.join(root, 'dist')).filter(p => !p.endsWith('.woff2')), ...walk(path.join(root, 'scripts')),
    path.join(root, 'README.md'), path.join(root, 'THEMING.md'), path.join(root, 'package.json')];
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
      // theme/check.mjs is a Node script: it uses Node's built-ins and reads
      // the kit's own tokens.css, both inside the package. src/ loads the
      // package's own fonts/, shared by src/ and dist/.
      const allowed = ref => (file === path.join(themeDir, 'check.mjs') && (/^node:/.test(ref) || ref === '../src/tokens.css')) || (file.startsWith(src) && /^\.\.\/fonts\/[\w.-]+\.woff2$/.test(ref));
      for (const ref of refs) if (!allowed(ref)) assert.ok(!outside(ref) && !/(^|\/)\.\.(\/|$)/.test(ref), `${rel(file)} loads ${ref} from outside its folder`);
    } else {
      for (const ref of refs) if (!/^node:/.test(ref)) assert.ok(!outside(ref), `${rel(file)} loads ${ref} from outside the package`);
    }
  }
});

test('diff tints: text and secondary text on every diff tint reach 4.5:1, and the tints differ in light and dark', () => {
  const kit = kitTokens();
  for (const block of ['light', 'dark']) {
    const v = name => kit[block].get(name) || kit.light.get(name);
    const tints = ['--wk-color-diff-add', '--wk-color-diff-del', '--wk-color-diff-add-strong', '--wk-color-diff-del-strong'];
    for (const tint of tints) assert.ok(kit[block].has(tint), `${tint} has no ${block} value`);
    for (const surface of ['--wk-color-surface', '--wk-color-bg']) {
      for (const tint of tints) {
        const row = over(v(tint), v(surface));
        const text = contrast(v('--wk-color-text'), row);
        assert.ok(text >= 4.5, `${block}: text on ${tint} over ${surface} is ${text.toFixed(2)}:1`);
        // The gutter's numbers and the markers sit on the row tints.
        if (!tint.endsWith('-strong')) {
          const secondary = contrast(v('--wk-color-text-secondary'), row);
          assert.ok(secondary >= 4.5, `${block}: secondary text on ${tint} over ${surface} is ${secondary.toFixed(2)}:1`);
        }
        // A changed word inside a paired line sits on the strong tint over the row tint.
        if (tint.endsWith('-strong')) {
          const word = over(v(tint), over(v(tint.replace('-strong', '')), v(surface)));
          const ratio = contrast(v('--wk-color-text'), word);
          assert.ok(ratio >= 4.5, `${block}: text on a changed word (${tint}) over ${surface} is ${ratio.toFixed(2)}:1`);
        }
      }
      assert.notEqual(over(v('--wk-color-diff-add'), v(surface)), over(v('--wk-color-diff-del'), v(surface)));
    }
  }
});

test('Inter is bundled: two variable woff2 files and the OFL, each named by an @font-face that resolves from src/ and from dist/ (its own copy)', () => {
  const fontsDir = path.join(root, 'fonts');
  assert.deepEqual(readdirSync(fontsDir).sort(), ['Inter-latin-ext.woff2', 'Inter-latin.woff2', 'OFL-Inter.txt']);
  assert.match(read(path.join(fontsDir, 'OFL-Inter.txt')), /SIL Open Font License, Version 1\.1/);
  for (const file of readdirSync(fontsDir).filter(f => f.endsWith('.woff2'))) assert.equal(readFileSync(path.join(fontsDir, file)).subarray(0, 4).toString('latin1'), 'wOF2', `${file} is a woff2 file`);
  for (const css of [path.join(src, 'base.css'), path.join(root, 'dist', 'wails-kit-ui.css')]) {
    const faces = rules(read(css)).filter(r => r.selector === '@font-face').map(r => Object.fromEntries(r.decls));
    assert.equal(faces.length, 2, rel(css));
    for (const face of faces) {
      assert.equal(face['font-family'], 'Inter');
      assert.equal(face['font-weight'], '100 900');
      assert.equal(face['font-display'], 'swap');
      assert.ok(face['unicode-range'], 'each face has a unicode-range');
      const url = face.src.match(/url\("([^"]+)"\)/)[1];
      assert.ok(statSync(path.resolve(path.dirname(css), url)).isFile(), `${rel(css)}: ${url} resolves to a file`);
    }
  }
  const kit = kitTokens();
  assert.match(kit.light.get('--wk-font-family'), /^Inter,/);
  assert.equal(kit.light.get('--wk-font-features'), '"cv11", "ss03"');
  const pkg = JSON.parse(read(path.join(root, 'package.json')));
  assert.ok(pkg.files.includes('fonts') && pkg.exports['./fonts/*'] === './fonts/*', 'package.json ships and exports fonts/');
});

test('the design system\'s names: wk-code and wk-chip--access, not the old wk-codeblock and wk-chip--lock', () => {
  const files = [...walk(src), ...themes, ...walk(path.join(root, 'demo')), ...walk(path.join(root, 'dist')), ...walk(path.join(root, 'test')).filter(p => !p.endsWith('tokens.test.mjs')), path.join(root, 'README.md')];
  for (const file of files) assert.doesNotMatch(read(file), /wk-codeblock|wk-chip--lock/, `${rel(file)} uses an old name`);
  const css = read(path.join(src, 'components.css'));
  for (const selector of ['.wk-code {', '.wk-code--scroll > pre', '.wk-chip--access > svg', '.wk-chip--access[data-level="manage"]']) assert.ok(css.includes(selector), `components.css defines ${selector}`);
});

// Increased contrast: one block in tokens.css, under prefers-contrast: more
// and @supports color-mix(), on :root:root:root so it outweighs a theme's
// dark blocks. It mixes lines, outlines, secondary text and tints from the
// theme's own text and bg.
const CONTRAST_SELECTOR = ':root:root:root';
const CONTRAST_MIX = {
  '--wk-color-text-secondary': 82, '--wk-color-text-secondary-on-selected': 88, '--wk-color-hairline': 32,
  '--wk-color-hairline-strong': 45, '--wk-color-control-border': 55, '--wk-color-field-border': 70,
};
const contrastBlock = () => {
  const found = rules(kitSource()).filter(r => r.selector === CONTRAST_SELECTOR);
  assert.equal(found.length, 1, 'one increased-contrast block');
  return found[0];
};

test('increased contrast: one block under prefers-contrast: more, mixing only line, outline, secondary-text and tint tokens from text and bg', () => {
  const r = contrastBlock();
  assert.ok(r.atRules.some(a => /^@media\s*\(prefers-contrast:\s*more\)$/.test(a)), r.atRules.join(' '));
  assert.ok(r.atRules.some(a => /^@supports\s*\(color:\s*color-mix\(/.test(a)), 'inside @supports color-mix()');
  const declared = kitTokens().light;
  const decls = new Map(r.decls);
  for (const [name, pct] of Object.entries(CONTRAST_MIX)) {
    assert.equal(decls.get(name), `color-mix(in srgb, var(--wk-color-text) ${pct}%, var(--wk-color-bg))`, name);
  }
  assert.equal(decls.get('--wk-color-fill'), 'color-mix(in srgb, var(--wk-color-text) 8%, transparent)');
  assert.equal(decls.get('--wk-color-fill-strong'), 'color-mix(in srgb, var(--wk-color-text) 14%, transparent)');
  assert.equal(decls.size, Object.keys(CONTRAST_MIX).length + 2, 'sets nothing else');
  for (const name of decls.keys()) assert.ok(declared.has(name), `${name} is a kit token`);
});

test('increased contrast is stronger than the usual values and reaches 7:1 for secondary text, 4.5:1 for field borders, in the kit and the example theme', () => {
  const rgba = (hex, a) => `rgba(${[1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16)).join(', ')}, ${a})`;
  const kit = kitTokens();
  for (const file of [path.join(src, 'tokens.css'), path.join(themeDir, 'example.css')]) {
    const t = tokens(read(file));
    for (const mode of ['light', 'dark']) {
      const g = n => t[mode].get(n) ?? kit[mode].get(n);
      const mix = pct => over(rgba(g('--wk-color-text'), pct / 100), g('--wk-color-bg'));
      const where = `${rel(file)} (${mode})`;
      for (const [name, pct] of Object.entries(CONTRAST_MIX)) {
        if (name.endsWith('on-selected')) continue; // it sits on a tint, not a ground
        assert.ok(contrast(mix(pct), g('--wk-color-bg')) > contrast(g(name), g('--wk-color-bg')), `${where}: ${name} is no stronger`);
      }
      for (const ground of ['bg', 'sidebar', 'surface', 'control']) {
        assert.ok(contrast(mix(82), g(`--wk-color-${ground}`)) >= 7, `${where}: secondary text under 7:1 on ${ground}`);
        assert.ok(contrast(mix(55), g(`--wk-color-${ground}`)) >= 3, `${where}: control border under 3:1 on ${ground}`);
      }
      for (const ground of ['bg', 'surface']) {
        assert.ok(contrast(mix(70), g(`--wk-color-${ground}`)) >= 4.5, `${where}: field border under 4.5:1 on ${ground}`);
      }
    }
  }
});
