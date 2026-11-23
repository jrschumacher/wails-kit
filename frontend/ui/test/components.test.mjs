// The components the Wails Kit Design System defines are in the package
// under the same class names (docs/design-system/components/bundle.css is
// the reference), and draw only with tokens. Apps that copied the design
// system's stylesheet swap it for this package with no renames.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { selectorsOf, banned } from './css.mjs';

const css = readFileSync(fileURLToPath(new URL('../src/components.css', import.meta.url)), 'utf8');
const all = selectorsOf(css);
const plain = all.filter(r => !r.atRules.length);
const forced = all.filter(r => r.forced);

// Selectors from the design system, each a rule of its own here.
const DESIGN_SYSTEM = {
  List: ['.wk-list', '.wk-list-row', '.wk-list-row:hover', '.wk-list-row[aria-selected="true"]', '.wk-list-row:focus-visible', '.wk-list-heading', '.wk-list-heading > span'],
  Menu: ['.wk-menu a[role^="menuitem"]', '.wk-menu--inline'],
  Range: ['.wk-range', '.wk-range::-webkit-slider-runnable-track', '.wk-range::-moz-range-track', '.wk-range::-webkit-slider-thumb', '.wk-range::-moz-range-thumb', '.wk-range:disabled', '.wk-range:focus-visible'],
};
// Selectors the design system redraws in forced colours.
const FORCED = ['.wk-list-row[aria-selected="true"]', '.wk-range::-webkit-slider-runnable-track', '.wk-range::-webkit-slider-thumb'];

for (const [card, selectors] of Object.entries(DESIGN_SYSTEM)) {
  test(`${card}: every design-system selector is defined, with tokens only`, () => {
    for (const selector of selectors) {
      const found = plain.filter(r => r.selector === selector);
      assert.ok(found.length, `components.css does not define ${selector}`);
      for (const r of found) for (const [prop, value] of r.decls) assert.deepEqual(banned(r, value), [], `${selector} { ${prop}: ${value} } is not a token`);
    }
  });
}

test('forced colours: the design system\'s redrawn states are redrawn here', () => {
  for (const selector of FORCED) {
    const found = forced.find(r => r.selector === selector);
    assert.ok(found, `no forced-colours rule for ${selector}`);
    assert.ok(found.decls.some(([p, v]) => p === 'forced-color-adjust' && v === 'none'), `${selector}: forced-color-adjust: none`);
  }
});

test('wk-range fills its track from --wk-range-value, in the accent over fill-strong', () => {
  const track = plain.find(r => r.selector === '.wk-range::-webkit-slider-runnable-track');
  const bg = Object.fromEntries(track.decls).background;
  assert.match(bg, /linear-gradient\(var\(--wk-color-accent\), var\(--wk-color-accent\)\) 0 \/ var\(--wk-range-value, 50%\) 100% no-repeat, var\(--wk-color-fill-strong\)/);
  const thumb = Object.fromEntries(plain.find(r => r.selector === '.wk-range::-webkit-slider-thumb').decls);
  assert.equal(thumb.background, 'var(--wk-color-thumb)');
});
