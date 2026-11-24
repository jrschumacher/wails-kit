// Static checks for wkTask and the announcer: the stylesheet's task rules
// draw only with tokens and have forced-colours rules, the announcer loads
// without a DOM, and the exports are declared. Their behaviour is checked in
// the browser (test/browser.mjs), on a fake clock.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { selectorsOf, banned } from './css.mjs';

const read = rel => readFileSync(fileURLToPath(new URL(rel, import.meta.url)), 'utf8');
const css = read('../src/components.css');
const all = selectorsOf(css);

test('task styles: every wk-task and wk-announcer rule uses tokens only', () => {
  const ours = all.filter(r => /\.wk-task-|\.wk-announcer/.test(r.selector));
  assert.ok(ours.length > 10);
  for (const r of ours) for (const [prop, value] of r.decls) assert.deepEqual(banned(r, value), [], `${r.selector} { ${prop}: ${value} }`);
});

test('task styles: the reserved Stop keeps its space; failed words and the summary have forced-colours rules', () => {
  const plain = all.filter(r => !r.atRules.length);
  const reserved = plain.find(r => r.selector === '.wk-task-stop[data-reserved]');
  assert.deepEqual(reserved?.decls, [['visibility', 'hidden']], 'visibility: hidden keeps the space and leaves the tab order');
  const forced = all.filter(r => r.forced).map(r => r.selector);
  for (const s of ['.wk-task-state[data-state="failed"] > .wk-task-words', '.wk-task-row[data-state="failed"] > .wk-task-text > .wk-task-line', '.wk-task-summary']) {
    assert.ok(forced.includes(s), `no forced-colours rule for ${s}`);
  }
});

test('announce loads and does nothing without a DOM', async () => {
  const { announce } = await import('../src/announce.js');
  assert.equal(typeof announce, 'function');
  assert.doesNotThrow(() => announce('Saved', { assertive: true }));
});

test('index.js exports announce and wkTask, and index.d.ts declares them', () => {
  const index = read('../src/index.js');
  assert.match(index, /export \{ announce \} from '\.\/announce\.js';/);
  assert.match(index, /export \{ wkTask \} from '\.\/task\.js';/);
  const dts = read('../src/index.d.ts');
  for (const name of ['export function announce(', 'export const wkTask:', 'follow(id: string', 'connect(options:', "'wk-task': CustomEvent<TaskEventDetail>"]) assert.ok(dts.includes(name), name);
});

test('the diff view sends its failure to the announcer and has no live region of its own', () => {
  const diff = read('../src/diff.js');
  assert.ok(!/aria-live/.test(diff), 'diff.js sets aria-live');
  assert.ok(!/\.disabled = /.test(diff), 'Show more is busy, never disabled');
  assert.match(diff, /announce\(/);
});
