// Copy to clipboard for wk-copy (BEHAVIOUR.md, "Patterns › Copy to
// clipboard"). The kit copies; the button keeps its accessible name ("Copy
// join code") and its glyph. "Copied" shows for timings.copied (2 s) in a
// tooltip above the button and goes to the polite announcer once per press;
// on failure, "Couldn't copy".
//
// A wk-copy button copies, on a click, with no script of the app's:
//   data-wk-copy="text"          that text
//   data-wk-copy-from="id"       the text (or a field's value) of the element with that id
//   inside a wk-code             the <code> or <pre> beside it
// Otherwise the app calls wkCopy(button, text) from its own click handler.
//
//   wkCopy(button, text) -> Promise<boolean>   copies, then shows and says the outcome
//   wkCopy.write = async text => {}            replaces the clipboard writer (Wails's Clipboard.SetText, say)
//   wkCopy.words = { copied, failed }
//
// It fires wk-copy on the button: detail { ok }. The text isn't in the
// event: it may be a secret.

import { timings } from './motion.js';
import { announce } from './announce.js';

const copyTips = new WeakMap(); // button -> { tip, timer }

function copyHome() {
  const modals = [...document.querySelectorAll('dialog')].filter(d => d.matches(':modal'));
  return modals.pop() || document.body;
}

// Above the button, centred, inside the viewport (below it when there's no
// room above).
function copyPlace(tip, rect) {
  tip.style.setProperty('left', '0px');
  tip.style.setProperty('top', '0px');
  const box = tip.getBoundingClientRect();
  const vw = document.documentElement.clientWidth;
  let top = rect.top - 6 - box.height;
  if (top < 4) top = rect.bottom + 6;
  const left = Math.min(Math.max(4, rect.left + rect.width / 2 - box.width / 2), vw - 4 - box.width);
  tip.style.setProperty('left', `${Math.round(left)}px`);
  tip.style.setProperty('top', `${Math.round(top)}px`);
}

function copyHide(button) {
  const s = copyTips.get(button);
  if (!s) return;
  clearTimeout(s.timer);
  s.tip.remove();
  s.off.abort();
  copyTips.delete(button);
}

function copyShow(button, text) {
  copyHide(button);
  // The button's own hover tooltip gives way.
  button.closest('wk-tooltip')?.hide?.();
  const tip = document.createElement('span');
  tip.className = 'wk-tooltip wk-copy-tip';
  // Said through the announcer, once: not read again from here.
  tip.setAttribute('aria-hidden', 'true');
  tip.textContent = text;
  copyHome().append(tip);
  copyPlace(tip, button.getBoundingClientRect());
  const off = new AbortController();
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && !e.defaultPrevented) { e.preventDefault(); copyHide(button); }
  }, { signal: off.signal });
  copyTips.set(button, { tip, off, timer: setTimeout(() => copyHide(button), timings.copied) });
}

async function wkCopy(button, text) {
  let ok = false;
  try {
    await wkCopy.write(String(text ?? ''));
    ok = true;
  } catch { ok = false; }
  if (button?.isConnected) {
    copyShow(button, ok ? wkCopy.words.copied : wkCopy.words.failed);
    button.dispatchEvent(new CustomEvent('wk-copy', { bubbles: true, detail: { ok } }));
  }
  // A failure while the person still waits on the button is assertive.
  announce(ok ? wkCopy.words.copied : wkCopy.words.failed, { assertive: !ok && document.activeElement === button });
  return ok;
}
wkCopy.write = text => {
  if (!navigator.clipboard?.writeText) return Promise.reject(new Error('no clipboard'));
  return navigator.clipboard.writeText(text);
};
wkCopy.words = { copied: 'Copied', failed: 'Couldn’t copy' };

// The text a wk-copy button copies by itself, or null when the app copies.
function copySource(button) {
  if (button.hasAttribute('data-wk-copy')) return button.getAttribute('data-wk-copy');
  const from = button.getAttribute('data-wk-copy-from');
  if (from) {
    const el = document.getElementById(from);
    if (!el) return null;
    return 'value' in el && (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) ? el.value : el.textContent;
  }
  const code = button.parentElement?.matches('.wk-code') ? button.parentElement.querySelector(':scope > pre, :scope > code') : null;
  return code ? code.textContent : null;
}

if (typeof document !== 'undefined') {
  document.addEventListener('click', e => {
    const button = e.target instanceof Element ? e.target.closest('.wk-copy') : null;
    if (!button || e.defaultPrevented) return;
    const text = copySource(button);
    if (text == null) return;
    wkCopy(button, text);
  });
}

export { wkCopy };
