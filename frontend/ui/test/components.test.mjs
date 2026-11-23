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
  Utilities: ['.wk-visually-hidden', '.wk-pane-scroll', '.wk-pane-scroll:focus-visible'],
  Sidebar: ['.wk-sidebar-detail'],
  Chip: ['.wk-chip--muted', '.wk-chip > .wk-dot', 'button.wk-chip', 'button.wk-chip:hover:not(:disabled)', 'button.wk-chip:disabled'],
  Table: ['.wk-table--zebra tbody tr:nth-child(even) > *', '.wk-table-name', '.wk-table-name:hover', '.wk-table-actions', '.wk-table tbody tr[data-muted] > *', '.wk-table tbody tr[data-muted] .wk-table-name'],
  Toolbar: ['.wk-toolbar-subtitle'],
  StatusDot: ['.wk-dot[data-tone="working"]:not([data-shape]):not([data-size="sm"])', '.wk-dot[data-tone="working"][data-shape="filled"]:not([data-size="sm"])'],
};
// Selectors the design system redraws in forced colours.
const FORCED = ['.wk-dot[data-tone="working"]:not([data-shape]):not([data-size="sm"])', '.wk-dot[data-tone="working"][data-shape="filled"]:not([data-size="sm"])'];

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
  for (const selector of FORCED) assert.ok(forced.find(r => r.selector === selector), `no forced-colours rule for ${selector}`);
});
