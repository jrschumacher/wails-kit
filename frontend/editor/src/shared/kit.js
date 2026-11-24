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
 * the editor stays busy until it settles in any case.
 *
 * Returns { task, ended }: the wkTask handle (null without the kit) and a
 * promise that resolves when the controls are back at rest.
 */
export function busySubmit(node, controls, result, spec = {}) {
  const wkTask = kitTask(node, spec.kit);
  if (wkTask) {
    let end;
    const ended = new Promise(r => { end = r; });
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
  const before = controls.map(c => [c.getAttribute('aria-disabled'), c.getAttribute('data-busy')]);
  for (const c of controls) { c.setAttribute('aria-disabled', 'true'); c.setAttribute('data-busy', ''); }
  const rest = () => {
    controls.forEach((c, i) => {
      const [aria, busy] = before[i];
      if (aria === null) c.removeAttribute('aria-disabled'); else c.setAttribute('aria-disabled', aria);
      if (busy === null) c.removeAttribute('data-busy'); else c.setAttribute('data-busy', busy);
    });
  };
  return { task: null, ended: result.then(rest, rest) };
}
