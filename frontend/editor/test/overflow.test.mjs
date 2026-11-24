// The Markdown editor's safety net (src/markdown/overflow.js), on real
// CodeMirror state with a stand-in view: a "language" in the compartment
// that overflows the stack when the text says so.
import test from 'node:test';
import assert from 'node:assert/strict';
import { EditorState, Compartment, StateField } from '@codemirror/state';
import { overflowDispatch, isStackOverflow, isPlain } from '../src/markdown/overflow.js';

// Throws when the document holds the trigger, as Lezer does on hostile text.
const parser = (trigger, error) => StateField.define({
  create: s => { if (s.doc.toString().includes(trigger)) throw error(); return 0; },
  update: (v, tr) => { if (tr.docChanged && tr.newDoc.toString().includes(trigger)) throw error(); return v + 1; },
});
const overflow = () => new RangeError('Maximum call stack size exceeded');

// What EditorView.update does first: refuse a transaction from another
// state, and read each new state (where a field's update throws) before
// changing anything.
class View {
  constructor(state) { this.state = state; this.updates = 0; }
  update(trs) {
    let state = this.state;
    for (const tr of trs) {
      if (tr.startState !== state) throw new RangeError("Trying to update state with a transaction that doesn't start from the previous state.");
      state = tr.state;
    }
    this.state = state;
    this.updates++;
  }
}

function setup(error = overflow) {
  const language = new Compartment();
  const state = EditorState.create({ doc: 'start', extensions: language.of(parser('BOOM', error)) });
  const view = new View(state);
  let plains = 0;
  const dispatch = overflowDispatch(language, () => plains++);
  return { language, view, dispatch, plains: () => plains };
}

test('isStackOverflow: only a RangeError about the call stack', () => {
  assert.equal(isStackOverflow(overflow()), true);
  assert.equal(isStackOverflow(new RangeError('Maximum call stack size exceeded.')), true);
  assert.equal(isStackOverflow(new RangeError('Invalid position 9 in document of length 5')), false);
  assert.equal(isStackOverflow(new Error('Maximum call stack size exceeded')), false);
  assert.equal(isStackOverflow(null), false);
});

test('an edit that overflows is applied again without highlighting, as one edit with its user event', () => {
  const { language, view, dispatch, plains } = setup();
  const tr = view.state.update({ changes: { from: 5, insert: ' BOOM' }, selection: { anchor: 10 }, userEvent: 'input.paste' });
  dispatch([tr], view);
  assert.equal(view.state.doc.toString(), 'start BOOM');
  assert.equal(view.state.selection.main.head, 10);
  assert.equal(isPlain(language, view.state), true);
  assert.equal(plains(), 1);
  assert.equal(view.updates, 1);
  // Several transactions at once are composed into the one retry.
  const s = setup();
  const a = s.view.state.update({ changes: { from: 0, insert: 'x' } });
  const b = a.state.update({ changes: { from: 6, insert: 'BOOM' } });
  s.dispatch([a, b], s.view);
  assert.equal(s.view.state.doc.toString(), 'xstartBOOM');
});

test('a transaction from a stale state rethrows CodeMirror\'s own error and changes nothing', () => {
  const { language, view, dispatch, plains } = setup();
  const stale = view.state.update({ changes: { from: 0, insert: 'BOOM ' } });
  dispatch([view.state.update({ changes: { from: 5, insert: '!' } })], view);
  assert.throws(() => dispatch([stale], view), e => e instanceof RangeError && /previous state/.test(e.message));
  assert.equal(view.state.doc.toString(), 'start!');
  assert.equal(isPlain(language, view.state), false, 'highlighting changed');
  assert.equal(plains(), 0, 'the net changed');
});

test('an overflow from a stale transaction is not retried on the current state', () => {
  // A view that reads the new state before it checks where the transaction
  // starts, so the stale transaction's overflow is what it throws.
  class EagerView extends View {
    update(trs) { for (const tr of trs) void tr.state; super.update(trs); }
  }
  const language = new Compartment();
  const view = new EagerView(EditorState.create({ doc: 'start', extensions: language.of(parser('BOOM', overflow)) }));
  let plains = 0;
  const dispatch = overflowDispatch(language, () => plains++);
  const stale = view.state.update({ changes: { from: 0, insert: 'BOOM ' } });
  // The text moves on, keeping its length, so the stale changes would still fit it.
  dispatch([view.state.update({ changes: { from: 2, to: 3, insert: 'x' } })], view);
  assert.throws(() => dispatch([stale], view), e => isStackOverflow(e));
  assert.equal(view.state.doc.toString(), 'stxrt', 'the stale edit was applied to the current text');
  assert.equal(isPlain(language, view.state), false);
  assert.equal(plains, 0);
});

test('another error is rethrown as it is, and so is the original when the retry fails too', () => {
  const other = new RangeError('not an overflow');
  const s = setup(() => other);
  assert.throws(() => s.dispatch([s.view.state.update({ changes: { from: 0, insert: 'BOOM' } })], s.view), e => e === other);
  assert.equal(isPlain(s.language, s.view.state), false);
  assert.equal(s.plains(), 0);
  // The retry overflows as well (here, from outside the compartment): the first error is thrown.
  const first = overflow();
  const language = new Compartment();
  const state = EditorState.create({ doc: 'start', extensions: [language.of([]), parser('BOOM', () => new RangeError('Maximum call stack size exceeded (retry)'))] });
  const view = new View(state);
  let plains = 0;
  // Highlighting is already off: nothing to retry.
  assert.throws(() => overflowDispatch(language, () => plains++)([view.state.update({ changes: { from: 0, insert: 'BOOM' } })], view), /retry/);
  const on = new Compartment();
  const state2 = EditorState.create({ doc: 'start', extensions: [on.of(parser('BOOM', () => first)), parser('BOOM', () => new RangeError('Maximum call stack size exceeded (retry)'))] });
  const view2 = new View(state2);
  assert.throws(() => overflowDispatch(on, () => plains++)([view2.state.update({ changes: { from: 0, insert: 'BOOM' } })], view2), e => e === first);
  assert.equal(view2.state.doc.toString(), 'start');
  assert.equal(isPlain(on, view2.state), false);
  assert.equal(plains, 0);
});
