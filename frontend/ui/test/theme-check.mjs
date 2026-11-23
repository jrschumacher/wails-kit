// The theme contract (THEMING.md) as a check. checkTheme(themeCss, kitCss)
// returns the problems with a theme file, each { rule, message }; [] means
// it keeps the contract. A rule id names one rule in one appearance (for
// example "working-on-bg/light"), so a known gap can be recorded as a named
// exception instead of loosening the rule:
//   checkTheme(css, kit, { exceptions: { 'working-on-bg/light': 'why' } })
// An exception that no longer matches a problem is itself a problem.
import { tokens, contrast, over } from './css.mjs';

const c = n => `--wk-color-${n}`;
// Must set: the app's identity.
export const IDENTITY = ['accent', 'on-accent', 'working', 'attention', 'attention-text'].map(c);
// May set, all ten or none: the grounds.
export const GROUNDS = ['bg', 'sidebar', 'surface', 'control', 'hairline', 'hairline-strong', 'control-border', 'field-border', 'text', 'text-secondary'].map(c);
// May set: fallbacks after the kit's font, and radii within 2px of the kit's.
export const FONT = '--wk-font-family';
export const RADII = ['--wk-radius-control', '--wk-radius-group', '--wk-radius-window'];
// Must not set: the kit derives these from accent and attention.
export const DERIVED = ['selection', 'chart-1', 'destructive-text', 'attention-fill'].map(c);
export const THEME_TOKENS = [...IDENTITY, ...GROUNDS, FONT, ...RADII];
const COLOURS = new Set([...IDENTITY, ...GROUNDS]);
const MODES = ['light', 'dark'];

const rgb = hex => hex.slice(1).match(/../g).map(x => parseInt(x, 16)).join(', ');
// The four derived colours for an appearance, from the accent and attention,
// as rgba() over transparent: what color-mix(in srgb, X p%, transparent) is.
export function derive(get, mode) {
  const dark = mode === 'dark';
  return {
    [c('selection')]: `rgba(${rgb(get(c('accent')))}, ${dark ? .16 : .12})`,
    [c('chart-1')]: get(c('accent')),
    [c('destructive-text')]: get(c('attention-text')),
    [c('attention-fill')]: `rgba(${rgb(get(c('attention')))}, ${dark ? .12 : .08})`,
  };
}

const firstFamily = v => (v || '').split(',')[0].trim().replace(/^["']|["']$/g, '').toLowerCase();
const px = v => (/^-?[\d.]+px$/.test(v || '') ? parseFloat(v) : NaN);

export function checkTheme(themeCss, kitCss, { exceptions = {}, requireIdentity = true } = {}) {
  const problems = [];
  const add = (rule, message) => problems.push({ rule, message });
  const kit = tokens(kitCss);
  const theme = tokens(themeCss);

  // Shape: the three token blocks, the two dark ones alike.
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
  }

  // Fallback for engines without color-mix(): only the four derived
  // colours, each what the kit would derive from the theme's own.
  const norm = v => (v || '').replace(/\s+/g, '').replace(/,0?\./g, ',.').toLowerCase();
  for (const mode of MODES) {
    const block = theme.fallback[mode];
    if (!block.size) continue;
    const raw = name => theme[mode].get(name) ?? (COLOURS.has(name) ? undefined : theme.light.get(name)) ?? kit[mode].get(name) ?? kit.light.get(name);
    const want = derive(raw, mode);
    for (const [name, value] of block) {
      if (!DERIVED.includes(name)) add('fallback', `${name} in @supports not (color-mix): only the four derived colours go there`);
      else if (norm(value) !== norm(want[name])) add(`fallback/${mode}`, `${mode}: ${name} is ${value} in the fallback; the kit derives ${want[name]}`);
    }
  }

  // Contrast, in each appearance, with the theme over the kit and the
  // derived colours worked out as the kit does.
  for (const mode of MODES) {
    // A colour the theme leaves out of dark falls back to the kit's dark
    // value (and is reported above); sizes and fonts are the same in both.
    const raw = name => theme[mode].get(name) ?? (COLOURS.has(name) ? undefined : theme.light.get(name)) ?? kit[mode].get(name) ?? kit.light.get(name);
    const d = derive(raw, mode);
    const get = name => d[name] ?? raw(name);
    const ratio = (rule, a, b, min, what) => {
      let r;
      try { r = contrast(get(a), b.startsWith('#') || b.startsWith('rgba') ? b : get(b)); } catch (e) { add(`${rule}/${mode}`, `${what}: ${e.message} (use #rrggbb)`); return; }
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

  const left = problems.filter(p => !(p.rule in exceptions));
  for (const rule of Object.keys(exceptions)) if (!problems.some(p => p.rule === rule)) left.push({ rule: 'exceptions', message: `the exception ${rule} matches no problem; remove it` });
  return left;
}
