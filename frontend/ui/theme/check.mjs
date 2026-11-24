#!/usr/bin/env node
// The theme contract (THEMING.md) as a check. No dependencies.
//
// From the command line, on an app's theme file:
//   node node_modules/@wails-kit/ui/theme/check.mjs path/to/theme.css
//   node node_modules/@wails-kit/ui/theme/check.mjs path/to/theme.css --except working-on-bg/light
// It prints each problem and exits 1 when there are any, 0 when the theme
// keeps the contract.
//
// As a module (import { checkTheme } from '@wails-kit/ui/theme/check'):
// checkTheme(themeCss, kitCss) returns the problems with a theme file, each
// { rule, message }; [] means it keeps the contract. kitCss is the kit's
// src/tokens.css. A rule id names one rule in one appearance (for example
// "working-on-bg/light"), so a known gap can be recorded as a named
// exception instead of loosening the rule:
//   checkTheme(css, kit, { exceptions: { 'working-on-bg/light': 'why' } })
// An exception that no longer matches a problem is itself a problem.

// ---- Reading a stylesheet ----

const DARK = /^@media\s*\(\s*prefers-color-scheme\s*:\s*dark\s*\)$/;
const FORCED = /^@media\s*\(\s*forced-colors\s*:\s*active\s*\)$/;
const ATTR_DARK = /^:root\[data-theme="dark"\]/;

// Every rule with declarations: { selector, decls: [[prop, value]], atRules,
// dark, forced, attrDark }.
export function rules(css) {
  css = css.replace(/\/\*[\s\S]*?\*\//g, '');
  const out = [];
  const stack = [];
  let start = 0;
  for (let i = 0; i < css.length; i++) {
    const c = css[i];
    if (c === '{') {
      stack.push(css.slice(start, i).trim().replace(/\s+/g, ' '));
      start = i + 1;
    } else if (c === '}') {
      const body = css.slice(start, i);
      const selector = stack.pop();
      if (body.includes(':')) {
        const decls = body.split(';').map(d => d.trim()).filter(Boolean).map(d => {
          const at = d.indexOf(':');
          return [d.slice(0, at).trim(), d.slice(at + 1).trim()];
        }).filter(([p]) => p && !p.startsWith('@'));
        out.push({ selector, decls, atRules: [...stack], dark: stack.some(s => DARK.test(s)), forced: stack.some(s => FORCED.test(s)), attrDark: ATTR_DARK.test(selector) });
      }
      start = i + 1;
    } else if (c === ';' && stack.length === 0) {
      start = i + 1; // top-level @import
    }
  }
  return out;
}

// The theme: light on :root; dark under the media query on
// :root:not([data-theme="light"]), and again on :root[data-theme="dark"], so
// the attribute wins over the system setting either way.
export const TOKEN_SELECTORS = { light: ':root', dark: ':root:not([data-theme="light"])', attrDark: ':root[data-theme="dark"]' };
export const isTokenSelector = selector => Object.values(TOKEN_SELECTORS).includes(selector);

// The --wk-* tokens a sheet sets: light (:root), dark (the media query) and
// attrDark (data-theme="dark"). Blocks inside @supports (color: color-mix())
// are the derived colours, kept apart in derived.light / .dark / .attrDark
// so the literals outside stay the fallback. misplaced lists token blocks
// that follow neither pattern, such as a bare :root inside the media query.
// A theme's own values for those four where color-mix() is missing, in
// @supports not (color: color-mix()), are kept apart in fallback.
const SUPPORTS_MIX = /^@supports\s*\(\s*color\s*:\s*color-mix\(/;
const SUPPORTS_NO_MIX = /^@supports\s+not\s*\(\s*color\s*:\s*color-mix\(/;
export function tokens(css) {
  const light = new Map(), dark = new Map(), attrDark = new Map(), misplaced = [];
  const derived = { light: new Map(), dark: new Map(), attrDark: new Map() };
  const fallback = { light: new Map(), dark: new Map(), attrDark: new Map() };
  for (const r of rules(css)) {
    if (!isTokenSelector(r.selector)) continue;
    const inMix = r.atRules.some(a => SUPPORTS_MIX.test(a));
    const noMix = r.atRules.some(a => SUPPORTS_NO_MIX.test(a));
    const others = r.atRules.filter(a => !SUPPORTS_MIX.test(a) && !SUPPORTS_NO_MIX.test(a) && !DARK.test(a));
    const into = inMix ? derived : noMix ? fallback : { light, dark, attrDark };
    const map = others.length ? null
      : !r.dark && r.selector === TOKEN_SELECTORS.light ? into.light
      : r.dark && r.selector === TOKEN_SELECTORS.dark ? into.dark
      : !r.dark && r.selector === TOKEN_SELECTORS.attrDark ? into.attrDark : null;
    if (!map) { misplaced.push(`${r.atRules.join(' ')} ${r.selector}`.trim()); continue; }
    for (const [p, v] of r.decls) if (p.startsWith('--wk-')) map.set(p, v);
  }
  return { light, dark, attrDark, derived, fallback, misplaced };
}

// ---- Colour ----

// An opaque #rrggbb, or rgba(r, g, b, a) as derive() writes it.
const parse = c => {
  const m = String(c).replace(/\s+/g, '').match(/^#([0-9a-f]{6})$|^rgba\((\d+),(\d+),(\d+),([\d.]+)\)$/i);
  if (!m) throw new Error(`not a colour: ${c}`);
  return m[1] ? [[0, 2, 4].map(i => parseInt(m[1].slice(i, i + 2), 16)), 1] : [[+m[2], +m[3], +m[4]], +m[5]];
};
export const isOpaqueHex = v => /^#[0-9a-f]{6}$/i.test(String(v).trim());

// A colour laid over an opaque ground, as #rrggbb.
export function over(top, ground) {
  const [t, a] = parse(top), [g, ga] = parse(ground);
  if (ga !== 1) throw new Error(`a ground must be opaque: ${ground}`);
  return '#' + t.map((c, i) => Math.round(c * a + g[i] * (1 - a)).toString(16).padStart(2, '0')).join('');
}

// WCAG 2 contrast of two opaque colours. A translucent colour has no
// contrast of its own: lay it over its ground first (over()).
export function contrast(a, b) {
  const lum = c => {
    const [v, alpha] = parse(c);
    if (alpha !== 1) throw new Error(`translucent colour ${c}; lay it over its ground first`);
    const [r, g, bl] = v.map(x => { x /= 255; return x <= .03928 ? x / 12.92 : ((x + .055) / 1.055) ** 2.4; });
    return .2126 * r + .7152 * g + .0722 * bl;
  };
  const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p);
  return (x + .05) / (y + .05);
}

// ---- The contract ----

const c = n => `--wk-color-${n}`;
// Must set: the app's identity.
export const IDENTITY = ['accent', 'on-accent', 'working', 'attention', 'attention-text'].map(c);
// May set, all ten or none: the grounds.
export const GROUNDS = ['bg', 'sidebar', 'surface', 'control', 'hairline', 'hairline-strong', 'control-border', 'field-border', 'text', 'text-secondary'].map(c);
// May set: fallbacks after the kit's first font (system-ui, the platform's UI
// font, which a theme keeps), and radii within 2px of the kit's.
export const FONT = '--wk-font-family';
export const RADII = ['--wk-radius-control', '--wk-radius-group', '--wk-radius-window'];
// Must not set: the kit derives these from accent and attention.
export const DERIVED = ['selection', 'chart-1', 'destructive-text', 'attention-fill'].map(c);
export const THEME_TOKENS = [...IDENTITY, ...GROUNDS, FONT, ...RADII];
const COLOURS = new Set([...IDENTITY, ...GROUNDS]);
const MODES = ['light', 'dark'];

const rgb = hex => hex.slice(1).match(/../g).map(x => parseInt(x, 16)).join(', ');
// The four derived colours for an appearance, from the accent and attention
// (each #rrggbb), as rgba() over transparent: what
// color-mix(in srgb, X p%, transparent) is.
export function derive(get, mode) {
  const dark = mode === 'dark';
  return {
    [c('selection')]: `rgba(${rgb(get(c('accent')))}, ${dark ? .16 : .12})`,
    [c('chart-1')]: get(c('accent')),
    [c('destructive-text')]: get(c('attention-text')),
    [c('attention-fill')]: `rgba(${rgb(get(c('attention')))}, ${dark ? .12 : .08})`,
  };
}

const firstEntry = v => (v || '').split(',')[0].trim();
const firstFamily = v => firstEntry(v).replace(/^["']|["']$/g, '').toLowerCase();
// A quoted name is a font family, never a generic keyword: "system-ui" in
// quotes names a font called system-ui, which WKWebView and WebView2 lack.
const isQuoted = v => /^["']/.test(firstEntry(v));
const px = v => (/^-?[\d.]+px$/.test(v || '') ? parseFloat(v) : NaN);

// The increased-contrast mixes in the kit's stylesheet: the token, and the
// share of --wk-color-text mixed into --wk-color-bg, from the
// :root:root:root block under prefers-contrast: more.
export const CONTRAST_MORE_SELECTOR = ':root:root:root';
export function contrastMore(kitCss) {
  const out = new Map();
  for (const r of rules(kitCss)) {
    if (r.selector !== CONTRAST_MORE_SELECTOR || !r.atRules.some(a => /prefers-contrast:\s*more/.test(a))) continue;
    for (const [name, value] of r.decls) {
      const m = value.match(/^color-mix\(in srgb, var\(--wk-color-text\) ([\d.]+)%, var\(--wk-color-bg\)\)$/);
      if (m) out.set(name, +m[1]);
    }
  }
  return out;
}

// The share of --wk-color-text in the increased-contrast fill-strong tint
// (color-mix(in srgb, var(--wk-color-text) N%, transparent)).
function fillShare(kitCss) {
  for (const r of rules(kitCss)) {
    if (r.selector !== CONTRAST_MORE_SELECTOR) continue;
    for (const [name, value] of r.decls) {
      const m = name === '--wk-color-fill-strong' && value.match(/^color-mix\(in srgb, var\(--wk-color-text\) ([\d.]+)%, transparent\)$/);
      if (m) return +m[1];
    }
  }
  return undefined;
}

export function checkTheme(themeCss, kitCss, { exceptions = {}, requireIdentity = true } = {}) {
  const problems = [];
  const add = (rule, message) => problems.push({ rule, message });
  const kit = tokens(kitCss);
  const theme = tokens(themeCss);

  // Shape: the three token blocks (and @font-face), nothing else, the two
  // dark ones alike.
  for (const r of rules(themeCss)) {
    const where = `${r.atRules.join(' ')} ${r.selector}`.trim();
    if (r.selector === '@font-face' && !r.atRules.length) continue;
    if (!isTokenSelector(r.selector)) add('blocks', `${where}: a theme sets tokens in the three theme blocks only (and @font-face); no other rules`);
    else for (const [p] of r.decls) if (!p.startsWith('--wk-')) add('blocks', `${where} sets ${p}; the theme blocks hold --wk-* tokens only`);
  }
  for (const m of theme.misplaced) add('blocks', `token block outside the three theme blocks: ${m}`);
  const same = (a, b) => JSON.stringify([...a.entries()].sort()) === JSON.stringify([...b.entries()].sort());
  if (!same(theme.dark, theme.attrDark)) add('blocks', 'the data-theme="dark" block must hold the same values as the dark media query block');
  for (const mode of ['light', 'dark', 'attrDark']) if (theme.derived[mode].size) add('derived', 'a theme does not derive colours (@supports color-mix): the kit does');
  if (!same(theme.fallback.dark, theme.fallback.attrDark)) add('blocks', 'the fallback blocks for dark (@supports not color-mix) must hold the same values');

  // What it sets.
  for (const [name] of theme.light) {
    if (DERIVED.includes(name)) add('derived', `${name} is derived by the kit from the accent or attention; a theme does not set it`);
    else if (!THEME_TOKENS.includes(name)) add('tokens', `${name} is not a theme token`);
  }
  for (const [name] of theme.dark) if (!theme.light.has(name)) add('tokens', `${name} has a dark value but no light one`);
  for (const name of COLOURS) if (theme.light.has(name) && !theme.dark.has(name)) add('tokens', `${name} has no dark value`);
  if (requireIdentity) for (const name of IDENTITY) if (!theme.light.has(name)) add('identity', `a theme sets ${name}`);
  const grounds = GROUNDS.filter(g => theme.light.has(g));
  if (grounds.length && grounds.length < GROUNDS.length) add('grounds', `a theme sets all ten grounds or none; it sets ${grounds.length}, missing ${GROUNDS.filter(g => !theme.light.has(g)).join(', ')}`);
  for (const name of RADII) {
    if (!theme.light.has(name)) continue;
    const t = px(theme.light.get(name)), k = px(kit.light.get(name));
    if (!(Math.abs(t - k) <= 2)) add('radii', `${name} is ${theme.light.get(name)}; it stays within 2px of the kit's ${kit.light.get(name)}`);
  }
  if (theme.light.has(FONT) && firstFamily(theme.light.get(FONT)) !== firstFamily(kit.light.get(FONT))) {
    add('font', `${FONT} starts with "${firstFamily(theme.light.get(FONT))}"; a theme only adds fallbacks after the kit's "${firstFamily(kit.light.get(FONT))}"`);
  } else if (theme.light.has(FONT) && isQuoted(theme.light.get(FONT)) && !isQuoted(kit.light.get(FONT))) {
    add('font', `${FONT} starts with ${firstEntry(theme.light.get(FONT))} in quotes, a font name; the kit's ${firstEntry(kit.light.get(FONT))} is a keyword, so a theme writes it unquoted`);
  }

  // Format: identity and ground colours are opaque #rrggbb, so the kit can
  // derive from them and contrast can be measured. An appearance with a
  // malformed colour is reported here and not measured below.
  const malformed = new Set();
  for (const mode of MODES) {
    for (const [name, value] of theme[mode]) {
      if (COLOURS.has(name) && !isOpaqueHex(value)) {
        add(`format/${mode}`, `${mode}: ${name} is ${value}; identity and ground colours are opaque #rrggbb`);
        malformed.add(mode);
      }
    }
  }
  // A colour the theme leaves out of dark falls back to the kit's dark
  // value (and is reported above); sizes and fonts are the same in both.
  const resolve = mode => name => theme[mode].get(name) ?? (COLOURS.has(name) ? undefined : theme.light.get(name)) ?? kit[mode].get(name) ?? kit.light.get(name);

  // Fallback for engines without color-mix(): only the four derived
  // colours, each what the kit would derive from the theme's own.
  const norm = v => (v || '').replace(/\s+/g, '').replace(/,0?\./g, ',.').toLowerCase();
  for (const mode of MODES) {
    const block = theme.fallback[mode];
    if (!block.size) continue;
    const want = malformed.has(mode) ? {} : derive(resolve(mode), mode);
    for (const [name, value] of block) {
      if (!DERIVED.includes(name)) add('fallback', `${name} in @supports not (color-mix): only the four derived colours go there`);
      else if (name in want && norm(value) !== norm(want[name])) add(`fallback/${mode}`, `${mode}: ${name} is ${value} in the fallback; the kit derives ${want[name]}`);
    }
  }

  // Contrast, in each appearance, with the theme over the kit and the
  // derived colours worked out as the kit does.
  for (const mode of MODES) {
    if (malformed.has(mode)) continue;
    const raw = resolve(mode);
    const d = derive(raw, mode);
    const get = name => d[name] ?? raw(name);
    const ratio = (rule, a, b, min, what) => {
      let r;
      try { r = contrast(get(a), b.startsWith('#') ? b : get(b)); } catch (e) { add(`${rule}/${mode}`, `${what}: ${e.message}`); return; }
      if (r < min) add(`${rule}/${mode}`, `${mode}: ${what} is ${r.toFixed(2)}:1, under ${min}:1`);
    };
    for (const ground of ['bg', 'sidebar', 'surface', 'control']) {
      ratio('text', c('text'), c(ground), 4.5, `text on ${ground}`);
      ratio('text-secondary', c('text-secondary'), c(ground), 4.5, `secondary text on ${ground}`);
    }
    for (const ground of ['bg', 'surface']) ratio('field-border', c('field-border'), c(ground), 3, `field border on ${ground}`);
    ratio('working-on-bg', c('working'), c('bg'), 3, 'working on the window');
    ratio('attention-on-bg', c('attention'), c('bg'), 3, 'attention on the window');
    ratio('working-attention', c('working'), c('attention'), 1.5, 'working against attention');
    ratio('on-accent', c('on-accent'), c('accent'), 4.5, 'text on the accent');
    // Secondary text on a selected row or a count chip (kit-held colours on
    // the theme's grounds and derived selection).
    for (const ground of ['bg', 'sidebar', 'surface']) {
      const g = get(c(ground));
      const selected = over(get(c('selection')), g);
      const on = { selected, chip: over(get(c('fill-strong')), g), 'chip on selected': over(get(c('fill-strong')), selected) };
      for (const [what, under] of Object.entries(on)) ratio('on-selected', c('text-secondary-on-selected'), under, 4.5, `secondary text on ${what} over ${ground}`);
    }
  }

  // Increased contrast: under prefers-contrast: more the kit mixes
  // secondary text, the field border and the fill tints from the theme's
  // text and bg (the last block of tokens.css). Each mix must pass the
  // usual minimum, and must reach the theme's own value without the setting
  // or a ceiling, whichever is lower: a theme already stronger than the
  // ceiling may come down to it, a weaker one may not get weaker.
  const mixes = contrastMore(kitCss);
  for (const mode of MODES) {
    if (malformed.has(mode) || !mixes.size) continue;
    const raw = resolve(mode);
    const d = derive(raw, mode);
    const get = name => d[name] ?? raw(name);
    const share = name => mixes.get(name);
    const mixed = name => share(name) === undefined ? undefined : over(`rgba(${rgb(get(c('text')))}, ${share(name) / 100})`, get(c('bg')));
    const check = (name, pairs, min, ceiling, what) => {
      const more = mixed(name);
      if (!more) return;
      for (const [where, usualGround, moreGround] of pairs) {
        const r = contrast(more, moreGround), usual = contrast(get(name), usualGround);
        const floor = Math.min(usual, ceiling);
        if (r < min) add(`contrast-more/${mode}`, `${mode}, increased contrast: ${what} on ${where} is ${r.toFixed(2)}:1, under ${min}:1`);
        else if (r < floor) add(`contrast-more/${mode}`, `${mode}, increased contrast: ${what} on ${where} is ${r.toFixed(2)}:1, under ${floor.toFixed(2)}:1 (the usual ${usual.toFixed(2)}:1, or ${ceiling}:1 if that is lower)`);
      }
    };
    const plain = grounds => grounds.map(g => [g, get(c(g)), get(c(g))]);
    check(c('text-secondary'), plain(['bg', 'sidebar', 'surface', 'control']), 4.5, 7, 'secondary text');
    check(c('field-border'), plain(['bg', 'surface']), 3, 4.5, 'the field border');
    // Secondary text on a selected row or a count chip: the chip's tint is
    // mixed too under increased contrast.
    const fill = fillShare(kitCss);
    const moreTint = fill === undefined ? get(c('fill-strong')) : `rgba(${rgb(get(c('text')))}, ${fill / 100})`;
    const pairs = [];
    for (const ground of ['bg', 'sidebar', 'surface']) {
      const g = get(c(ground));
      const selected = over(get(c('selection')), g);
      pairs.push([`selected over ${ground}`, selected, selected]);
      pairs.push([`chip over ${ground}`, over(get(c('fill-strong')), g), over(moreTint, g)]);
      pairs.push([`chip on selected over ${ground}`, over(get(c('fill-strong')), selected), over(moreTint, selected)]);
    }
    check(c('text-secondary-on-selected'), pairs, 4.5, 7, 'secondary text on a selection');
  }

  const left = problems.filter(p => !(p.rule in exceptions));
  for (const rule of Object.keys(exceptions)) if (!problems.some(p => p.rule === rule)) left.push({ rule: 'exceptions', message: `the exception ${rule} matches no problem; remove it` });
  return left;
}

// ---- Command line ----

async function main(args) {
  const { readFileSync } = await import('node:fs');
  const files = [], exceptions = {};
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--except' && args[i + 1]) exceptions[args[++i]] = 'named on the command line';
    else if (args[i].startsWith('--except=')) exceptions[args[i].slice(9)] = 'named on the command line';
    else files.push(args[i]);
  }
  if (files.length !== 1) {
    console.error('usage: node theme/check.mjs path/to/theme.css [--except rule/appearance ...]');
    return 2;
  }
  const kit = readFileSync(new URL('../src/tokens.css', import.meta.url), 'utf8');
  const problems = checkTheme(readFileSync(files[0], 'utf8'), kit, { exceptions });
  for (const p of problems) console.log(`${p.rule}: ${p.message}`);
  if (problems.length) { console.log(`${files[0]}: ${problems.length} problem(s); see THEMING.md`); return 1; }
  console.log(`${files[0]}: keeps the theme contract`);
  return 0;
}

if (typeof process !== 'undefined' && process.argv?.[1]) {
  const { pathToFileURL } = await import('node:url');
  const { realpathSync } = await import('node:fs');
  let self = false;
  try { self = pathToFileURL(realpathSync(process.argv[1])).href === import.meta.url; } catch { /* not run as a script */ }
  if (self) process.exitCode = await main(process.argv.slice(2));
}
