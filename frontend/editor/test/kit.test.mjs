// The editors' helpers for @wails-kit/ui's wkTask and announcer (src/shared/kit.js).
import test from 'node:test';
import assert from 'node:assert/strict';
import { retryRefusal } from '../src/shared/kit.js';

test('retryRefusal says why Retry sends nothing, with the label or without one', () => {
  assert.equal(retryRefusal('Comment', {}), null);
  assert.equal(retryRefusal('Comment', { disabled: true }), 'Couldn’t send: Comment is turned off');
  assert.equal(retryRefusal('Comment', { busy: true }), 'Couldn’t send: Comment is still sending');
  assert.equal(retryRefusal('', { disabled: true }), 'Couldn’t send: it’s turned off');
  assert.equal(retryRefusal('', { busy: true }), 'Couldn’t send: it’s still sending');
});
