// The Markdown editor's safety net under the limits scan: text the scan
// misses can still overflow the stack in CodeMirror's Markdown parser. A
// stack overflow while applying an edit is applied again with the language
// compartment emptied (no highlighting), so the edit is kept and nothing
// throws. Plain CodeMirror state, no DOM, so node tests it
// (test/overflow.test.mjs).
import { Transaction } from '@codemirror/state';
import { isStackOverflow } from '../shared/stack.js';

export { isStackOverflow };

/** Whether the compartment holds no extension (highlighting off) in a state. */
export function isPlain(compartment, state) {
  const content = compartment.get(state);
  return content == null || (Array.isArray(content) && content.length === 0);
}

/**
 * CodeMirror's dispatchTransactions hook. view.update reads each
 * transaction's new state before it changes anything, so when that throws the
 * view is as it was. On a stack overflow, while highlighting is on and the
 * transactions start from the view's state, the edit is applied again as one
 * transaction (the same changes, selection and user event) that also empties
 * the compartment; onplain() is called once that succeeds. Any other error,
 * or a retry that fails, rethrows the original error and changes nothing.
 */
export function overflowDispatch(compartment, onplain) {
  return (trs, view) => {
    try { view.update(trs); } catch (e) {
      if (!isStackOverflow(e) || !trs.length || trs[0].startState !== view.state || isPlain(compartment, view.state)) throw e;
      let changes = trs[0].changes;
      for (const tr of trs.slice(1)) changes = changes.compose(tr.changes);
      const last = trs[trs.length - 1];
      const userEvent = last.annotation(Transaction.userEvent);
      try {
        view.update([view.state.update({
          changes, selection: last.selection, scrollIntoView: last.scrollIntoView,
          effects: compartment.reconfigure([]), ...(userEvent ? { userEvent } : {}),
        })]);
      } catch { throw e; }
      onplain();
    }
  };
}
