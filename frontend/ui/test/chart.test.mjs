// The chart's layout rules that need no DOM (src/chart.js).
import test from 'node:test';
import assert from 'node:assert/strict';
import { niceTicks, labelStep, runs, roundedTop, roundedEnd, tableModel, readout } from '../src/chart.js';

test('chart ticks are round, start at 0 and reach the data', () => {
  assert.deepEqual(niceTicks(0), [0, 1]);
  assert.deepEqual(niceTicks(21), [0, 10, 20, 30]);
  assert.deepEqual(niceTicks(100), [0, 50, 100]);
  assert.deepEqual(niceTicks(7, 4, true), [0, 2, 4, 6, 8]);
  assert.deepEqual(niceTicks(1, 4, true), [0, 1], 'whole steps for counts');
  for (const max of [0.3, 3, 17, 240, 9999]) assert.ok(niceTicks(max).at(-1) >= max, `ticks reach ${max}`);
});

test('chart labels thin out until they fit', () => {
  assert.equal(labelStep(40, 30), 1);
  assert.equal(labelStep(20, 30), 2);
  assert.equal(labelStep(8, 30), 5);
});

test('a line breaks at a gap, and a single point is its own run', () => {
  assert.deepEqual(runs([1, 2, null, 4, null, null, 7, 8]), [[[0, 1], [1, 2]], [[3, 4]], [[6, 7], [7, 8]]]);
  assert.deepEqual(runs([null, null]), []);
});

test('a bar rounds its data end only, and never more than half its size', () => {
  assert.match(roundedTop(10, 20, 24, 50), /^M10 70V24a4 4 0 0 1 4 -4H30a4 4 0 0 1 4 4V70z$/);
  assert.match(roundedTop(10, 20, 24, 50, 0), /^M10 70V20H34V70z$/);
  assert.match(roundedTop(0, 0, 24, 2), /a2 2 /, 'a short bar rounds by its height');
  assert.match(roundedEnd(0, 0, 50, 24), /^M0 0H46a4 4 0 0 1 4 4V20a4 4 0 0 1 -4 4H0z$/);
});

test('the table view has the chart\'s numbers, a total for a stack, and a dash for no data', () => {
  const spec = {
    label: 'Items', categoryHeader: 'Day', stacked: true, format: v => `${v} items`,
    categories: [{ label: 'Mon 9', name: 'Monday 9 Nov' }, { label: 'Tue 10' }],
    series: [{ label: 'a', tone: 1, values: [2, null] }, { label: 'b', tone: 2, values: [3, 4] }],
  };
  assert.deepEqual(tableModel(spec), { head: ['Day', 'a', 'b', 'Total'], rows: [['Monday 9 Nov', '2 items', '3 items', '5 items'], ['Tue 10', '–', '4 items', '4 items']] });
  const line = { label: 'Wait', categories: [{ label: 'x' }], series: [{ label: 'Median', values: [5] }], band: { label: 'Up to 9 in 10', values: [9] } };
  assert.deepEqual(tableModel(line).head, ['', 'Median', 'Up to 9 in 10']);
});

test('the readout names the category and lists each series that has a value', () => {
  const spec = {
    label: 'Items', kind: 'bars', stacked: true, shadeLabel: 'Weekend', format: String,
    categories: [{ label: 'Sat 14', name: 'Saturday 14 Nov', shaded: true }, { label: 'Mon' }],
    series: [{ label: 'a', tone: 1, values: [2, 0] }, { label: 'b', tone: 2, values: [0, 0] }],
  };
  assert.equal(readout(spec, 0).text, 'Saturday 14 Nov (weekend): a 2');
  assert.equal(readout(spec, 1).text, 'Mon: No data');
});
