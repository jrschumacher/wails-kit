// The window's wkTask and announcer, from @wails-kit/ui's module when it is
// in the page: the first copy of the kit sets document.wkTask and
// document.wkAnnounce. The editors bundle no part of the kit (its kit.js,
// which wkTask needs, defines the custom elements the app loads itself), so
// they look these up when they need them, and work without them.

/** The window's announce(words, { assertive }), or null without the kit. */
export function kitAnnounce(node) {
  const doc = node?.ownerDocument || (typeof document === 'undefined' ? null : document);
  return typeof doc?.wkAnnounce === 'function' ? doc.wkAnnounce : null;
}

/**
 * Keeps the submit controls busy while onsubmit's promise is pending. With
 * the kit, through wkTask: aria-disabled and data-busy at once, a spinner in
 * the first button after 400 ms that stays at least 500 ms, the words
 * announced when it appears and the outcome when it ends, the task in the
 * window's task list, and each control's earlier aria-disabled back at the
 * end. Without it, the controls are busy at once until the promise settles,
 * and get their earlier aria-disabled back. Resolves when the controls are
 * back at rest.
 *
 * spec: { words, doneWords, failWords, timeout, subject, retry }.
 */
export function busySubmit(node, controls, result, spec = {}) {
  const doc = node?.ownerDocument || document;
  return new Promise(end => {
    if (typeof doc.wkTask === 'function') {
      const task = doc.wkTask({
        control: controls,
        words: spec.words || 'Sending',
        subject: spec.subject,
        ...(spec.timeout !== undefined ? { timeout: spec.timeout } : {}),
        onEnd: () => end(),
      });
      result.then(
        () => task.done({ words: spec.doneWords || 'Sent' }),
        error => task.fail({ words: spec.failWords || 'Couldn’t send', error, retry: spec.retry }),
      );
      return;
    }
    const before = controls.map(c => [c.getAttribute('aria-disabled'), c.getAttribute('data-busy')]);
    for (const c of controls) { c.setAttribute('aria-disabled', 'true'); c.setAttribute('data-busy', ''); }
    const rest = () => {
      controls.forEach((c, i) => {
        const [aria, busy] = before[i];
        if (aria === null) c.removeAttribute('aria-disabled'); else c.setAttribute('aria-disabled', aria);
        if (busy === null) c.removeAttribute('data-busy'); else c.setAttribute('data-busy', busy);
      });
      end();
    };
    result.then(rest, rest);
  });
}
