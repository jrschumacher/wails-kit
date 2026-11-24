// The window's wkTask and announcer, from @wails-kit/ui: the app's spec.kit
// ({ wkTask, announce }), else what the kit's module publishes when it is in
// the page (the first copy of the kit sets document.wkTask and
// document.wkAnnounce). The editors bundle no part of the kit (its kit.js,
// which wkTask needs, defines the custom elements the app loads itself), so
// they look these up when they need them, and work without them.

const docOf = node => node?.ownerDocument || (typeof document === 'undefined' ? null : document);

/** The window's announce(words, { assertive }), or null without the kit. */
export function kitAnnounce(node, kit) {
  if (typeof kit?.announce === 'function') return kit.announce;
  const doc = docOf(node);
  return typeof doc?.wkAnnounce === 'function' ? doc.wkAnnounce : null;
}

/** The window's wkTask(spec), or null without the kit. */
export function kitTask(node, kit) {
  if (typeof kit?.wkTask === 'function') return kit.wkTask;
  const doc = docOf(node);
  return typeof doc?.wkTask === 'function' ? doc.wkTask : null;
}

/**
 * Keeps the submit controls busy while onsubmit's promise is pending. With
 * the kit, through wkTask: aria-disabled and data-busy at once, a spinner in
 * the first button after 400 ms that stays at least 500 ms, the words
 * announced when it appears and the outcome when it ends, the task in the
 * window's task list (a failure waits there with Retry), and each control's
 * earlier aria-disabled back at the end. Without it, the controls are busy
 * at once until the promise settles, and get their earlier aria-disabled
 * back.
 *
 * spec: { words, doneWords, failWords, timeout, timeoutWords, subject,
 * retry, kit }. No timeout unless given: the submit is the app's call, and
 * the editor stays busy until it settles in any case. After a timeout the
 * task has ended, but the controls stay busy (aria-disabled, data-busy, no
 * spinner) until the promise settles, so they don't look ready while a press
 * would do nothing.
 *
 * Returns { task, ended }: the wkTask handle (null without the kit) and a
 * promise that resolves when the controls are back at rest.
 */
// Marks controls busy; the returned function puts back what they had.
function markBusy(controls) {
  const before = controls.map(c => [c.getAttribute('aria-disabled'), c.getAttribute('data-busy')]);
  for (const c of controls) { c.setAttribute('aria-disabled', 'true'); c.setAttribute('data-busy', ''); }
  return () => controls.forEach((c, i) => {
    const [aria, busy] = before[i];
    if (aria === null) c.removeAttribute('aria-disabled'); else c.setAttribute('aria-disabled', aria);
    if (busy === null) c.removeAttribute('data-busy'); else c.setAttribute('data-busy', busy);
  });
}

export function busySubmit(node, controls, result, spec = {}) {
  const wkTask = kitTask(node, spec.kit);
  const settled = result.then(() => {}, () => {});
  if (wkTask) {
    let settledYet = false;
    settled.then(() => { settledYet = true; });
    let end;
    const taskEnded = new Promise(r => { end = r; });
    // At rest when the task has ended and the promise has settled.
    const ended = taskEnded.then(() => {
      if (settledYet) return undefined;
      const rest = markBusy(controls);
      return settled.then(rest);
    });
    const task = wkTask({
      control: controls,
      words: spec.words || 'Sending…',
      subject: spec.subject,
      timeout: spec.timeout ?? false,
      timeoutWords: spec.timeoutWords || 'Couldn’t confirm it was sent',
      onEnd: () => end(),
    });
    result.then(
      () => task.done({ words: spec.doneWords || 'Sent' }),
      error => task.fail({ words: spec.failWords || 'Couldn’t send', error, retry: spec.retry }),
    );
    return { task, ended };
  }
  const rest = markBusy(controls);
  return { task: null, ended: settled.then(rest) };
}
