// The timings and motion values (src/motion.js and tokens.css), and the
// static rules of the busy state, the spinner and the progress bar
// (components.css). BEHAVIOUR.md is the spec.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { timings, motion, reducedMotion } from '../src/motion.js';
import { tokens, selectorsOf, banned } from './css.mjs';

const read = f => readFileSync(fileURLToPath(new URL(f, import.meta.url)), 'utf8');
const css = read('../src/components.css');
const all = selectorsOf(css);
const plain = all.filter(r => !r.atRules.length);
const forced = all.filter(r => r.forced);
const reduced = all.filter(r => r.atRules.some(a => /prefers-reduced-motion:\s*reduce/.test(a)));
const decls = r => Object.fromEntries(r.decls);

test('timings: exactly the values BEHAVIOUR.md lists, frozen', () => {
  assert.deepEqual({ ...timings }, {
    indicatorDelay: 400, indicatorMin: 500, stopDelay: 10_000, waiting: 30_000, timeout: 120_000, estimateStable: 5_000,
    loadingLine: 3_000, recheckDelay: 500, toast: 6_000, toastWithAction: 10_000, copied: 2_000, progressValueEvery: 5_000,
    backoff: [2_000, 4_000, 8_000, 16_000, 30_000], backoffJitter: 0.2, wakeSettle: 2_000,
  });
  assert.ok(Object.isFrozen(timings) && Object.isFrozen(timings.backoff));
  // The page lists the same object literally: keep the two in step.
  const page = read('../BEHAVIOUR.md').replace(/\s+/g, ' ');
  const listed = page.match(/timings = Object\.freeze\(\{([^}]*)\}\)/)[1];
  const values = Object.fromEntries([...listed.matchAll(/(\w+): (\[[^\]]*\]|[\d_.]+)/g)].map(([, k, v]) => [k, JSON.parse(v.replaceAll('_', ''))]));
  assert.deepEqual(values, { ...timings, backoff: [...timings.backoff] });
});

test('motion: the --wk-duration-* and --wk-ease-* tokens, with the same values as motion in src/motion.js', () => {
  const kit = tokens(read('../src/tokens.css')).light;
  assert.equal(kit.get('--wk-duration-fast'), `${motion.durationFast}ms`);
  assert.equal(kit.get('--wk-duration-base'), `${motion.durationBase}ms`);
  assert.equal(kit.get('--wk-duration-slow'), `${motion.durationSlow}ms`);
  assert.equal(kit.get('--wk-ease-out'), motion.easeOut);
  assert.equal(kit.get('--wk-ease-in'), motion.easeIn);
  assert.deepEqual([motion.durationFast, motion.durationBase, motion.durationSlow], [100, 200, 300]);
  assert.ok(Object.isFrozen(motion));
  assert.equal(reducedMotion(), false, 'no matchMedia, no reduced motion');
});

test('no transition in the kit has a literal duration: each uses a --wk-duration-* token (the switch\'s too)', () => {
  for (const file of ['../src/components.css', '../src/base.css', '../src/settings.css']) {
    for (const r of selectorsOf(read(file))) {
      for (const [p, v] of r.decls) if (/^transition(-duration)?$/.test(p) && v !== 'none') assert.doesNotMatch(v, /\d(m?s)\b/, `${file}: ${r.selector} { ${p}: ${v} }`);
    }
  }
  const sw = plain.find(r => r.selector === '.wk-switch');
  assert.match(decls(sw).transition, /var\(--wk-duration-fast\)/);
});

// Kit names the design system lacks (proposed for it), each a rule here.
const PROPOSED = {
  Busy: ['.wk-button[data-busy]', '.wk-button--quiet[data-busy]', '.wk-filter[data-busy]', 'button.wk-chip[data-busy]', '.wk-copy[data-busy]', 'button.wk-link[data-busy]'],
  Spinner: ['.wk-spinner', '.wk-button--small > .wk-spinner', '.wk-button:has(> .wk-spinner)', '.wk-button > .wk-spinner'],
  Progress: ['.wk-progress', '.wk-progress:not([aria-valuenow])', '.wk-progress:not([aria-valuenow])::before'],
};
for (const [name, selectors] of Object.entries(PROPOSED)) {
  test(`${name}: every selector is defined, with tokens only`, () => {
    for (const selector of selectors) {
      const found = plain.filter(r => r.selector === selector);
      assert.ok(found.length, `components.css does not define ${selector}`);
      for (const r of found) for (const [p, v] of r.decls) assert.deepEqual(banned(r, v), [], `${selector} { ${p}: ${v} } is not a token`);
    }
  });
}

test('busy: the pressed face and the progress cursor, overriding the 45% aria-disabled look', () => {
  const busy = decls(plain.find(r => r.selector === '.wk-button[data-busy]'));
  assert.equal(busy.opacity, '1');
  assert.equal(busy.cursor, 'progress');
  assert.match(busy['background-image'], /var\(--wk-color-fill-strong\)/);
  // Same specificity as .wk-button[aria-disabled="true"], so it must come later.
  assert.ok(css.indexOf('.wk-button[data-busy]') > css.indexOf('.wk-button[aria-disabled="true"]'));
});

test('spinner: a border in currentColor that keeps turning under reduced motion; the bar: 4 px, working on fill-strong', () => {
  const spinner = decls(plain.find(r => r.selector === '.wk-spinner'));
  assert.match(spinner.border, /currentColor/);
  assert.match(spinner.animation, /^wk-spin /);
  assert.ok(!reduced.some(r => r.selector.includes('wk-spinner')), 'nothing stops the spinner under reduced motion');
  const bar = decls(plain.find(r => r.selector === '.wk-progress'));
  assert.equal(bar.height, '4px');
  assert.match(bar.background, /linear-gradient\(var\(--wk-color-working\), var\(--wk-color-working\)\) 0 \/ var\(--wk-progress-value, 0%\) 100% no-repeat, var\(--wk-color-fill-strong\)/);
  const sweep = reduced.find(r => r.selector === '.wk-progress:not([aria-valuenow])::before');
  assert.ok(sweep, 'the sweep has a reduced-motion rule');
  assert.match(decls(sweep).animation, /^wk-progress-fade .*infinite/, 'under reduced motion the sweep fades in place, never still');
  assert.doesNotMatch(css.match(/@keyframes wk-progress-fade \{[^}]*\}[^}]*\}/)[0], /transform/, 'the fade does not move');
});

test('forced colours: the bar\'s fill is Highlight inside a CanvasText outline; the spinner keeps its border; a button with a spinner keeps its label hidden', () => {
  const bar = forced.find(r => r.selector === '.wk-progress');
  assert.ok(bar, 'no forced-colours rule for .wk-progress');
  assert.equal(decls(bar)['forced-color-adjust'], 'none');
  assert.match(decls(bar).outline, /solid CanvasText/);
  assert.match(decls(bar).background, /linear-gradient\(Highlight, Highlight\)/);
  assert.match(decls(forced.find(r => r.selector === '.wk-progress:not([aria-valuenow])::before')).background, /Highlight/);
  assert.match(decls(forced.find(r => r.selector === '.wk-spinner'))['border-color'], /currentColor/);
  assert.equal(decls(forced.find(r => r.selector === '.wk-button:has(> .wk-spinner)'))['forced-color-adjust'], 'none');
});
