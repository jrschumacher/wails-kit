// wkTask: the action lifecycle (BEHAVIOUR.md, "The action lifecycle" and
// "Building blocks › wkTask"). The kit runs the clocks, the busy control,
// the indicator and its ARIA, the space for Stop, Esc, announcements and the
// task list; the app writes the words, reports progress and does (and
// stops) the work.
//
//   wkTask(spec)                  a task the page runs itself
//   wkTask.connect({ subscribe, bindings })
//                                 the Go tasks service: its tasks:changed
//                                 snapshots and its Bindings()
//   wkTask.follow(id, options)    draw a task the Go side runs
//   wkTask.summary()              "3 tasks running · 1 failed": a button
//                                 that opens the task list
//   wkTask.showList()             opens the task list (Window > Tasks)
//   wkTask.words                  the kit's own words, for localising
//
// Every state change fires a bubbling wk-task event on the target, else the
// first control, else the document: detail { id, state, words, ...payload }.

import { timings } from './motion.js';
import { setProgress } from './kit.js';
import { announce } from './announce.js';

const ACTIVE = new Set(['queued', 'running', 'waiting', 'paused', 'stalled']);
const FINAL = new Set(['done', 'failed', 'cancelled', 'stopped']);
const STOP_MODES = new Set(['cancel', 'stop', 'stop-asks', 'pause', 'unsafe']);

const taskPlural = (n, one, many) => `${n} ${n === 1 ? one : many}`;
// "34 s", "3 min", "3 h 12 m" (BEHAVIOUR.md, "Writing the words").
function duration(ms) {
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s} s`;
  const m = Math.round(s / 60);
  if (m < 60) return `${m} min`;
  const h = Math.floor(m / 60), rest = m % 60;
  return rest ? `${h} h ${rest} m` : `${h} h`;
}

// The kit's own words. An app that writes another language replaces them.
const words = {
  cancel: 'Cancel',
  stop: 'Stop',
  stopAsks: 'Stop…',
  pause: 'Pause',
  resume: 'Resume',
  paused: 'Paused',
  stopped: 'Stopped',
  cancelled: 'Cancelled',
  failedState: 'Failed',
  unsafe: 'This can’t be stopped',
  timedOut: ms => `Timed out after ${duration(ms)}`,
  count: (done, total, unit) => `${done} of ${total}${unit ? ` ${unit}` : ''}`,
  step: (step, steps) => `Step ${step} of ${steps}`,
  summary: ({ finished, failed, total, unit }) => `${finished} of ${total}${unit ? ` ${unit}` : ''} finished${failed ? ` · ${failed} failed` : ''}`,
  retryInTasks: 'Retry is in Tasks.',
  tasks: 'Tasks',
  noTasks: 'No tasks',
  running: n => taskPlural(n, 'task running', 'tasks running'),
  queuedCount: n => `${n} queued`,
  pausedCount: n => `${n} paused`,
  stalledCount: n => `${n} need${n === 1 ? 's' : ''} you`,
  failed: n => `${n} failed`,
  // When a call to the Go side fails, by binding.
  couldNot: {
    Stop: 'Couldn\u2019t stop the task',
    Pause: 'Couldn\u2019t pause the task',
    Resume: 'Couldn\u2019t resume the task',
    Retry: 'Couldn\u2019t retry the task',
    Dismiss: 'Couldn\u2019t dismiss the task',
    DismissFinished: 'Couldn\u2019t clear finished tasks',
  },
  retry: 'Retry',
  retryNow: 'Retry now',
  dismiss: 'Dismiss',
  clearFinished: 'Clear finished',
  close: 'Done',
  nextTry: at => {
    const ms = at - Date.now();
    if (ms < 60_000) return `Next try in ${Math.max(1, Math.ceil(ms / 1000))} s`;
    return `Next try ${new Date(at).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' }).toLowerCase()}`;
  },
};

const isMac = () => /^mac|^ip(hone|ad|od)/i.test(navigator.userAgentData?.platform || navigator.platform || '');
const taskEl = (tag, className, text) => {
  const e = document.createElement(tag);
  if (className) e.className = className;
  if (text != null) e.textContent = text;
  return e;
};
const dot = tone => {
  const d = taskEl('span', 'wk-dot');
  if (tone) d.dataset.tone = tone;
  d.setAttribute('aria-hidden', 'true');
  return d;
};
const emitTask = (node, detail) => {
  const at = node?.isConnected ? node : document;
  at.dispatchEvent(new CustomEvent('wk-task', { bubbles: true, detail }));
};

// Makes parent's children exactly nodes, in order, moving only what is out
// of place, so the element with focus keeps it (a removed and reinserted
// element loses focus). If focus was lost all the same, it goes back.
function syncChildren(parent, nodes) {
  const had = document.activeElement;
  const want = new Set(nodes);
  for (const n of [...parent.childNodes]) if (!want.has(n)) n.remove();
  let ref = parent.firstChild;
  for (const n of nodes) {
    if (n === ref) { ref = ref.nextSibling; continue; }
    parent.insertBefore(n, ref);
  }
  if (had && had !== document.activeElement && parent.contains(had)) had.focus({ preventScroll: true });
}

// ---- Busy controls -------------------------------------------------------
// A control may be busy for more than one task; it goes back to rest, with
// the aria-disabled it had before (a locked control stays locked), when
// the last of them ends.
const busy = new WeakMap();
function makeBusy(control) {
  const b = busy.get(control);
  if (b) { b.count++; return; }
  busy.set(control, { count: 1, aria: control.getAttribute('aria-disabled'), data: control.getAttribute('data-busy') });
  control.setAttribute('aria-disabled', 'true');
  control.setAttribute('data-busy', '');
}
// The spinner in a button, shared like the busy state: it goes when the
// last task that showed it ends.
const spinners = new WeakMap();
function addSpinner(button) {
  const sp = spinners.get(button);
  if (sp) { sp.count++; return; }
  const el = taskEl('span', 'wk-spinner');
  el.setAttribute('aria-hidden', 'true');
  button.prepend(el);
  spinners.set(button, { count: 1, el });
}
function dropSpinner(button) {
  const sp = spinners.get(button);
  if (!sp || --sp.count > 0) return;
  spinners.delete(button);
  sp.el.remove();
}
function release(control) {
  const b = busy.get(control);
  if (!b || --b.count > 0) return;
  busy.delete(control);
  if (b.aria === null) control.removeAttribute('aria-disabled'); else control.setAttribute('aria-disabled', b.aria);
  if (b.data === null) control.removeAttribute('data-busy'); else control.setAttribute('data-busy', b.data);
}

// ---- The list of tasks ---------------------------------------------------
// The page's tasks (top-level wkTask tasks, while they run or once failed)
// and the Go side's (from wkTask.connect). The summary and the task list
// draw from it.
const local = new Map();       // id -> Task
let remote = [];               // the latest Go snapshots
let connection = null;         // { bindings, confirmStop, unsubscribe, seen }
const followers = new Map();   // id -> Set<Task>
const watchers = new Set();
const summaries = new Set();   // summary buttons in use
let list = null;               // the task list, once built
// Whether the window has somewhere to find a failed task: a task list.
const hasTaskList = () => Boolean(list) || [...summaries].some(b => b.isConnected);
let notifyQueued = false;
function changed() {
  if (notifyQueued) return;
  notifyQueued = true;
  queueMicrotask(() => { notifyQueued = false; for (const w of watchers) w(); });
}
// Every task, as snapshots: the Go side's in its order, then the page's.
function entries() {
  const out = remote.map(s => ({ ...s, source: 'go' }));
  for (const task of local.values()) {
    // Work that ends in the quiet wait never shows here.
    if (task.shownAt == null && task.state === 'running') continue;
    out.push({ ...task.snapshot(), source: 'kit' });
    for (const part of task.parts) out.push({ ...part.snapshot(), source: 'kit' });
  }
  return out;
}

let taskUid = 0;

class Task {
  constructor(spec = {}, { external = false, listed = true, parent = null } = {}) {
    if (spec.stop != null && spec.stop !== false && !STOP_MODES.has(spec.stop)) throw new TypeError(`wkTask: stop must be one of ${[...STOP_MODES].join(', ')} or false, not ${spec.stop}`);
    this.spec = spec;
    this.id = String(spec.id ?? `wk-task-${++taskUid}`);
    this.external = external;
    this.parent = parent;
    this.parts = [];
    this.controls = [].concat(spec.control ?? []).filter(Boolean);
    this.target = spec.target || null;
    this.bar = spec.bar || null;
    this.stopMode = spec.stop || false;
    this.expect = spec.expect || 'short';
    this.words = String(spec.words ?? '');
    this.progressState = {};
    this.state = null;
    this.timers = {};
    this.ended = false;
    this.abort = new AbortController();
    this.signal = this.abort.signal;
    this.shownAt = null;
    this.saved = null;
    this.stopEl = null;
    this.created = performance.now();
    for (const c of this.controls) makeBusy(c);
    if (this.bar) this.bar.hidden = true;
    if (this.target) this.target.setAttribute('data-wk-task', this.id);
    this.reserveStop();
    this.listenKeys();
    if (listed && !parent) { local.set(this.id, this); changed(); }
    if (spec.queued) this.queue(String(spec.queued));
    else this.start(undefined, spec.elapsed || 0);
  }

  // ---- what others read ----
  get stage() {
    const p = this.progressState;
    return [p.steps ? words.step(p.step, p.steps) : '', p.stage || ''].filter(Boolean).join(' · ');
  }
  get count() {
    const p = this.progressState;
    return p.total > 0 ? words.count(p.done ?? 0, p.total, p.unit || '') : '';
  }
  // What is announced: the words and the stage, never a count (a value
  // that ticks stays out of the live regions).
  get said() { return [this.words, this.stage].filter(Boolean).join(' · '); }
  get text() {
    if (FINAL.has(this.state)) return this.words;
    const unsafe = this.stopMode === 'unsafe' && this.stopShown && ACTIVE.has(this.state) ? words.unsafe : '';
    return [this.words, this.stage, this.count, unsafe].filter(Boolean).join(' · ');
  }
  snapshot() {
    const p = this.progressState;
    return {
      id: this.id, parent: this.parent?.id ?? '', subject: String(this.spec.subject ?? ''), kind: String(this.spec.kind ?? ''),
      state: this.state, done: p.done ?? 0, total: p.total ?? 0, unit: p.unit ?? '', step: p.step ?? 0, steps: p.steps ?? 0, stage: p.stage ?? '',
      words: this.words, stop: this.stopMode, error: this.error ?? null, retry: this.retryFn ? 'manual' : 'unsafe', nextTry: null,
      startedAt: null, updatedAt: Date.now(),
    };
  }

  fire(state, payload = {}) {
    emitTask(this.target || this.controls[0], { id: this.id, state, words: this.words, ...payload });
    if (!this.parent) changed();
    else this.parent.partChanged();
  }

  // ---- clocks ----
  timer(name, ms, fn) {
    clearTimeout(this.timers[name]);
    this.timers[name] = ms == null ? 0 : setTimeout(() => { this.timers[name] = 0; fn(); }, Math.max(0, ms));
  }
  clearTimers() { for (const name of Object.keys(this.timers)) clearTimeout(this.timers[name]); this.timers = {}; }

  // The waiting switch (30 s) and the timeout (2 min) count from the last
  // report. Off while queued, paused or stalled, and when the Go side runs
  // them (follow).
  quiet({ restart = true } = {}) {
    if (this.external || this.ended) return;
    // A task in parts is as quiet as its parts: each has its own clocks.
    if (this.parts.length) { this.timer('waiting', null); this.timer('timeout', null); return; }
    const counting = this.state === 'running' || this.state === 'waiting';
    const timeout = this.ownTimeout === undefined ? (this.spec.timeout ?? timings.timeout) : this.ownTimeout;
    this.timer('waiting', counting && this.state === 'running' ? timings.waiting : null, () => this.kitWaiting());
    // The kit's own switch to Waiting is no report: the timeout runs on.
    if (restart || !counting) this.timer('timeout', counting && timeout !== false ? timeout : null, () => this.timedOut(timeout));
  }

  queue(text) {
    this.state = 'queued';
    this.words = text;
    this.showStop();
    this.draw();
    this.quiet();
    this.fire('queued');
  }

  // Running, from the press or from Queued. elapsed: how long it has already
  // run (a Go task drawn after a reload), so the indicator and Stop come no
  // later than they would have.
  start(text, elapsed = 0) {
    if (this.ended) return;
    const from = this.state;
    this.state = 'running';
    if (text) this.words = text;
    this.startedAt = performance.now() - elapsed;
    this.timer('indicator', timings.indicatorDelay - elapsed, () => this.showIndicator());
    if (this.expect === 'long' || from === 'queued') this.showStop();
    else if (this.expect === 'maybe-long') this.timer('stop', timings.stopDelay - elapsed, () => this.showStop());
    this.draw();
    this.quiet();
    this.fire('running');
  }

  // ---- the indicator ----
  showIndicator() {
    if (this.ended || this.shownAt != null) return;
    this.shownAt = performance.now();
    if (this.target) this.draw();
    else {
      const button = this.controls.find(c => c.classList.contains('wk-button'));
      if (button) { addSpinner(button); this.spinner = button; }
    }
    if (this.bar) {
      this.bar.hidden = false;
      if (this.progressState.total > 0) this.moveBar('now');
    }
    if (!this.parent && !this.quietStart) announce(this.said);
    if (!this.parent) changed();
  }

  moveBar(aria = true) {
    const p = this.progressState;
    if (!this.bar || this.shownAt == null || !(p.total > 0)) return;
    const count = words.count(Math.min(p.done ?? 0, p.total), p.total, p.unit || '');
    setProgress(this.bar, { done: p.done ?? 0, total: p.total, unit: p.unit || '', text: this.stage ? `${this.stage} · ${count}` : count }, { aria });
  }

  // The target shows the state while the indicator does, in place of its
  // own content: a dot and the words, then the Stop button if it has one.
  draw() {
    const t = this.target;
    if (!t) return;
    if (this.shownAt == null && (this.state === 'running' || this.state === 'waiting')) return;
    if (!this.saved) this.saved = [...t.childNodes].filter(n => n !== this.stopEl);
    // One line, changed in place; the Stop button stays where it is, so
    // focus on it survives every redraw.
    if (!this.line) {
      this.line = taskEl('span', 'wk-task-state');
      this.lineDot = dot(null);
      this.lineWords = taskEl('span', 'wk-task-words');
    }
    const tone = this.state === 'running' ? 'working' : this.state === 'stalled' || this.state === 'failed' ? 'attention' : null;
    this.line.dataset.state = this.state;
    if (tone) this.lineDot.dataset.tone = tone; else delete this.lineDot.dataset.tone;
    const withDot = this.state !== 'queued' && !(FINAL.has(this.state) && this.state !== 'failed');
    syncChildren(this.line, withDot ? [this.lineDot, this.lineWords] : [this.lineWords]);
    if (this.lineWords.textContent !== this.text) this.lineWords.textContent = this.text;
    syncChildren(t, [this.line, ...(this.stopEl && ACTIVE.has(this.state) ? [this.stopEl] : [])]);
  }

  // ---- Stop, Cancel, Pause ----
  get stopLabel() {
    if (this.stopMode === 'pause') return this.state === 'paused' ? words.resume : words.pause;
    return this.stopMode === 'cancel' || this.state === 'queued' ? words.cancel : this.stopMode === 'stop-asks' ? words.stopAsks : words.stop;
  }
  // Work that may be long keeps the button's space from the start, hidden,
  // and shows it at 10 s, so nothing moves under the pointer.
  reserveStop() {
    const mode = this.stopMode;
    if (!mode || mode === 'unsafe' || (this.expect === 'short' && !this.spec.queued)) return;
    const b = taskEl('button', 'wk-button wk-button--small wk-task-stop');
    b.type = 'button';
    b.dataset.reserved = '';
    b.dataset.wkTask = this.id;
    b.textContent = this.stopLabel;
    b.addEventListener('click', () => (this.stopMode === 'pause' ? this.togglePause() : this.stop()));
    this.stopEl = b;
    const where = this.spec.stopIn;
    if (where) where.append(b);
    else if (this.controls.length) this.controls[this.controls.length - 1].after(b);
    else if (this.target) this.target.append(b);
    else this.stopEl = null;
  }
  showStop() {
    if (this.ended || this.stopShown) return;
    this.stopShown = true;
    if (this.stopEl) this.stopEl.removeAttribute('data-reserved');
    if (this.stopMode === 'unsafe') this.draw();
  }
  relabel() { if (this.stopEl) this.stopEl.textContent = this.stopLabel; }

  // Cancel or Stop: the app stops the work in onStop (or the kit aborts
  // task.signal). onStop returning false (the person said no, or the app
  // will report the end itself) leaves the task as it is.
  async stop() {
    if (this.ended || this.stopping || !this.stopMode || this.stopMode === 'unsafe') return;
    if (this.stopMode === 'pause' && this.state !== 'queued') return this.togglePause();
    if (this.parts.length && !this.spec.onStop) { for (const p of this.parts) p.stop(); return; }
    this.stopping = true;
    if (this.stopEl) makeBusy(this.stopEl);
    let r;
    try { r = this.spec.onStop ? await this.spec.onStop(this) : undefined; } catch { r = false; }
    this.stopping = false;
    if (this.stopEl) release(this.stopEl);
    if (r === false || this.ended) return;
    this.abort.abort(new DOMException('Stopped', 'AbortError'));
    if (this.state === 'queued' || this.stopMode === 'cancel') this.cancelled();
    else this.stopped();
  }
  async togglePause() {
    if (this.ended || this.stopping) return;
    const resuming = this.state === 'paused';
    this.stopping = true;
    if (this.stopEl) makeBusy(this.stopEl);
    let r;
    try { r = await (resuming ? this.spec.onResume : this.spec.onPause)?.(this); } catch { r = false; }
    this.stopping = false;
    if (this.stopEl) release(this.stopEl);
    if (r === false || this.ended) return;
    if (resuming && this.state === 'paused') this.resumed();
    else if (!resuming && (this.state === 'running' || this.state === 'waiting')) this.paused();
  }

  // Esc (and ⌘. on macOS) cancels short work the person is waiting for in
  // that place: focus in the scope. In a Sheet the first Esc cancels the
  // work, and the Sheet stays open.
  get scope() {
    if (this.spec.scope) return [this.spec.scope];
    const anchor = this.controls[0] || this.target;
    const dialog = anchor?.closest?.('dialog');
    if (dialog) return [dialog];
    return [...this.controls, this.target?.closest('tr, [role="row"], .wk-list-row') || this.target, this.stopEl].filter(Boolean);
  }
  inScope() {
    const a = document.activeElement;
    return Boolean(a && a !== document.body && this.scope.some(s => s.contains(a)));
  }
  listenKeys() {
    const esc = this.spec.esc ?? (this.stopMode === 'cancel' && this.expect !== 'long');
    if (!esc || this.external) return;
    this.keys = new AbortController();
    // In the bubble phase, after the page: an Escape a menu, a tooltip or
    // the app already used (and prevented) is left alone. ⌘. only on macOS.
    window.addEventListener('keydown', e => {
      const key = (e.key === 'Escape' && !e.ctrlKey && !e.metaKey && !e.altKey) || (e.metaKey && e.key === '.' && isMac());
      if (!key || e.defaultPrevented || e.isComposing || this.stopping || !this.inScope()) return;
      // Prevented, the key doesn't close the Sheet it's in either.
      e.preventDefault();
      e.stopPropagation();
      this.stop();
    }, { signal: this.keys.signal });
  }

  // ---- reports from the app ----
  progress(p = {}) {
    if (this.ended) return;
    const before = this.stage;
    const s = this.progressState;
    for (const k of ['done', 'total', 'unit', 'step', 'steps', 'stage']) if (p[k] !== undefined) s[k] = p[k];
    if (this.state === 'waiting' || this.state === 'stalled') this.back('running');
    else if (this.state === 'queued') this.start();
    const stageChanged = this.stage !== before;
    if (stageChanged && this.shownAt != null && !this.parent) announce(this.stage);
    this.moveBar(stageChanged ? 'now' : true);
    this.draw();
    this.quiet();
    if (!this.parent) changed(); else this.parent.partChanged();
  }
  setWords(text) {
    if (this.ended || !text) return;
    this.words = String(text);
    if (this.kitWaited) { this.kitWaited = null; this.back('running'); }
    this.draw();
    this.quiet();
    changed();
  }
  // Back to running from waiting or stalled: the words from before a
  // switch the kit made.
  back(state) {
    if (this.kitWaited) this.words = this.kitWaited;
    this.kitWaited = null;
    this.ownTimeout = undefined;
    this.state = state;
    this.draw();
    this.fire(state);
  }
  kitWaiting() {
    if (this.ended || this.state !== 'running') return;
    this.kitWaited = this.words;
    this.enter('waiting', this.spec.waitingWords, { restart: false });
  }
  waiting(text, { timeout } = {}) {
    if (this.ended) return;
    this.kitWaited = null;
    if (timeout !== undefined) this.ownTimeout = timeout;
    this.enter('waiting', text);
  }
  // A neutral state that stands still: the words change, the bar's value
  // catches up with its fill at once, and the new words are announced.
  enter(state, text, { restart = true } = {}) {
    if (this.state === 'queued' && state !== 'queued') this.start();
    const changedWords = text && text !== this.words;
    if (text) this.words = String(text);
    this.state = state;
    this.moveBar('now');
    this.relabel();
    this.draw();
    if (changedWords && this.shownAt != null && !this.parent) announce(this.said);
    this.quiet({ restart });
    this.fire(state);
  }
  paused(text) {
    if (this.ended) return;
    this.pausedFrom = this.words;
    this.enter('paused', text || words.paused);
  }
  resumed(text) {
    if (this.ended || this.state !== 'paused') return;
    this.words = text || this.pausedFrom || this.words;
    this.state = 'running';
    this.moveBar('now');
    this.relabel();
    this.draw();
    if (this.shownAt != null && !this.parent) announce(this.said);
    this.quiet();
    this.fire('running');
  }
  stalled(text) { if (!this.ended) this.enter('stalled', text); }
  queued(text) { if (!this.ended) this.queue(text || this.words); }
  running(text) {
    if (this.ended) return;
    if (this.state === 'queued') this.start(text);
    else if (this.state !== 'running') { this.back('running'); if (text) this.setWords(text); }
    else if (text && text !== this.words) this.setWords(text);
  }
  timedOut(ms) {
    if (this.ended) return;
    this.abort.abort(new DOMException('Timed out', 'TimeoutError'));
    this.spec.onTimeout?.(this);
    this.fail({ words: this.spec.timeoutWords || words.timedOut(ms), reason: 'timeout' });
  }

  // ---- the end ----
  done(arg) {
    const o = typeof arg === 'string' ? { words: arg } : arg || {};
    if (!o.words) throw new TypeError('wkTask: done() needs words; they are always announced');
    return this.finish('done', o);
  }
  fail(arg) {
    const o = typeof arg === 'string' ? { words: arg } : arg || {};
    if (!o.words) throw new TypeError('wkTask: fail() needs words');
    return this.finish('failed', o);
  }
  stopped(text) {
    const o = typeof text === 'object' && text ? text : { words: text };
    return this.finish('stopped', { ...o, words: o.words || [words.stopped, this.count].filter(Boolean).join(' · ') });
  }
  cancelled(o = {}) { return this.finish('cancelled', { ...o, words: words.cancelled }); }

  // The indicator stays at least 500 ms once shown: the end waits for it,
  // then the indicator goes, the control returns to rest, the words are
  // announced and wk-task fires, in one step. The promise resolves in that
  // step: draw the result there.
  finish(state, o) {
    if (this.ended) return Promise.resolve(null);
    this.ended = true;
    this.clearTimers();
    this.keys?.abort();
    if (state !== 'done') this.abort.abort(new DOMException(state, 'AbortError'));
    const hold = this.shownAt == null ? 0 : this.shownAt + timings.indicatorMin - performance.now();
    return new Promise(resolve => {
      const swap = () => resolve(this.swap(state, o));
      if (hold > 0) setTimeout(swap, hold); else swap();
    });
  }

  swap(state, o) {
    const assertive = state === 'failed' && !o.quiet && this.inScope();
    const stopHadFocus = this.stopEl && this.stopEl.contains(document.activeElement);
    this.state = state;
    this.words = String(o.words);
    if (o.error !== undefined) this.error = o.error;
    if (o.retry) this.retryFn = o.retry;
    if (this.spinner) dropSpinner(this.spinner);
    this.spinner = null;
    for (const c of this.controls) release(c);
    this.stopEl?.remove();
    if (stopHadFocus) {
      // Focus goes to the control, else the row (or the target), made
      // focusable for it if it isn't; never to the page.
      const row = this.target?.closest('tr, [role="row"], .wk-list-row') || this.target;
      const moved = [...this.controls, row].some(n => { n?.focus?.({ preventScroll: true }); return n && document.activeElement === n; });
      if (!moved && row) { if (!row.hasAttribute('tabindex')) row.tabIndex = -1; row.focus({ preventScroll: true }); }
    }
    if (this.bar) this.bar.hidden = true;
    const t = this.target;
    if (t) {
      t.removeAttribute('data-wk-task');
      // Done and Cancelled put the target back as it was; words stay only
      // when the result isn't visible. Failed and Stopped say what happened.
      const back = state === 'cancelled' || (state === 'done' && o.visible !== false);
      if (back) { if (this.saved) t.replaceChildren(...this.saved); }
      else {
        if (!this.saved) this.saved = [...t.childNodes];
        this.draw();
        if (state === 'done' && o.show) {
          const show = taskEl('button', 'wk-link wk-task-show', o.showLabel || 'Show');
          show.type = 'button';
          show.addEventListener('click', () => o.show());
          t.append(' ', show);
        }
      }
    }
    const announced = !this.parent && !o.quiet && (state !== 'cancelled' || this.shownAt != null);
    if (announced) {
      let text = this.words;
      if (state === 'failed' && !assertive && this.retryFn && hasTaskList()) text = `${text}${/[.!?]$/.test(text) ? '' : '.'} ${words.retryInTasks}`;
      announce(text, { assertive });
    }
    const { words: _, quiet: __, ...payload } = o;
    const detail = { id: this.id, state, words: this.words, ...payload };
    // The task list keeps a failed task until it is retried or dismissed.
    if (!this.parent && state !== 'failed') local.delete(this.id);
    this.fire(state, payload);
    this.spec.onEnd?.(detail);
    return detail;
  }

  // ---- work in parts ----
  part(spec = {}) {
    if (this.ended) throw new Error('wkTask: part() after the task ended');
    const p = new Task({ timeout: this.spec.timeout, stop: this.stopMode, esc: false, ...spec, id: spec.id ?? `${this.id}-${this.parts.length + 1}` }, { parent: this, listed: false });
    this.parts.push(p);
    this.quiet();
    this.partChanged();
    return p;
  }
  partChanged() {
    if (this.ended || !this.parts.length) return;
    const ended = this.parts.filter(p => p.ended);
    if (this.progressState.total === undefined) this.partProgress = true;
    if (this.partProgress) { this.progressState.done = ended.length; this.progressState.total = this.parts.length; this.progressState.unit = this.spec.unit || ''; this.moveBar(); this.draw(); }
    changed();
    if (ended.length < this.parts.length || this.closing) return;
    // Settle in a microtask: a part that ends at once, before the next is
    // added, doesn't end the task.
    this.closing = true;
    queueMicrotask(() => { this.closing = false; this.settleParts(); });
  }
  settleParts() {
    if (this.ended || this.parts.some(p => !p.ended)) return;
    const count = s => this.parts.filter(p => p.state === s).length;
    const n = this.parts.length, finished = count('done'), failed = count('failed');
    const summary = (this.spec.summary || words.summary)({ finished, failed, stopped: count('stopped'), cancelled: count('cancelled'), total: n, unit: this.spec.unit || '' });
    const parts = this.parts.map(p => ({ id: p.id, state: p.state, words: p.words }));
    if (failed) this.finish('failed', { words: summary, parts });
    else if (finished === n) this.finish('done', { words: summary, parts });
    else if (count('cancelled') === n) this.finish('cancelled', { parts });
    else this.finish('stopped', { words: summary, parts });
  }
}

// The handle the app holds: the lifecycle's verbs, and nothing else.
function handle(task) {
  return {
    get id() { return task.id; },
    get state() { return task.state; },
    get signal() { return task.signal; },
    progress: p => task.progress(p),
    setWords: w => task.setWords(w),
    queued: w => task.queued(w),
    running: w => task.running(w),
    waiting: (w, o) => task.waiting(w, o),
    paused: w => task.paused(w),
    resumed: w => task.resumed(w),
    stalled: w => task.stalled(w),
    done: o => task.done(o),
    fail: o => task.fail(o),
    stopped: w => task.stopped(w),
    cancelled: o => task.cancelled(o),
    stop: () => task.stop(),
    // Takes an ended task (a failed one waits there) out of the task list.
    dismiss: () => { if (task.ended && !task.parent && local.get(task.id) === task) { local.delete(task.id); changed(); } },
    part: spec => handle(task.part(spec)),
  };
}

function wkTask(spec = {}) {
  return handle(new Task(spec, { listed: spec.listed !== false }));
}

// ---- The Go side ---------------------------------------------------------
// subscribe(listener) calls listener with each tasks:changed payload
// ({ tasks }) and may return a function that stops it; bindings is the
// tasks service's Bindings() (Tasks, Get, Stop, Pause, Resume, Retry,
// Dismiss, DismissFinished). confirmStop(snapshot) is asked before a
// "stop-asks" task is stopped; it resolves to true to stop.
function connect({ subscribe, bindings = {}, confirmStop } = {}) {
  if (typeof subscribe !== 'function') throw new TypeError('wkTask.connect: subscribe must be a function');
  connection?.disconnect();
  const conn = { bindings, confirmStop, seen: false };
  const take = list => { remote = Array.isArray(list) ? list.slice() : []; for (const [id, set] of followers) for (const f of set) f.apply(remote.find(s => s.id === id)); changed(); };
  const off = subscribe(payload => { conn.seen = true; take(payload?.tasks); });
  conn.disconnect = () => {
    if (typeof off === 'function') off();
    if (connection !== conn) return;
    connection = null;
    remote = [];
    // Followed tasks end quietly: their controls go back to rest.
    const all = [...followers.values()].flatMap(set => [...set]);
    followers.clear();
    for (const f of all) f.end();
    changed();
  };
  connection = conn;
  // A reloaded window asks for the tasks as they are, unless an event has
  // already brought them.
  Promise.resolve(bindings.Tasks?.()).then(list => { if (!conn.seen && connection === conn && list) take(list); }, () => {});
  return conn.disconnect;
}

// Calls a binding. A failure is said in words (the raw error stays out):
// assertive while focus is in the task list, where the person pressed it.
const call = (name, ...args) => Promise.resolve().then(() => connection?.bindings?.[name]?.(...args)).catch(() => {
  const said = words.couldNot?.[name];
  if (said) announce(said, { assertive: Boolean(list?.host.open && list.dialog.contains(document.activeElement)) });
});

async function stopRemote(snapshot) {
  if (snapshot.stop === 'stop-asks' && connection?.confirmStop && !(await connection.confirmStop(snapshot))) return false;
  await call('Stop', snapshot.id);
  return false;
}

// Draws a Go task on the control and target, from its snapshots. The kit
// keeps the delays, the indicator, Stop and announcements; the Go side keeps
// the waiting and timeout clocks and the words. A task already finished
// when it is first seen is drawn without being announced.
function follow(id, { control, target, bar, scope, stopIn, expect } = {}) {
  if (!connection) throw new Error('wkTask.follow: call wkTask.connect first');
  id = String(id);
  let task = null, last = null;
  const f = {
    end() { if (task && !task.ended) task.finish('cancelled', { words: words.cancelled, quiet: true }); },
    apply(snap) {
      if (!snap) {
        // Gone from the list (dismissed): put things back, quietly.
        if (task && !task.ended) task.finish('cancelled', { words: words.cancelled, quiet: true });
        if (task || connection?.seen) unfollow();
        return;
      }
      if (!task) {
        const elapsed = snap.startedAt ? Math.max(0, Date.now() - snap.startedAt) : 0;
        const stop = snap.stop || false;
        task = new Task({
          id, control, target, bar, scope, stopIn, words: snap.words, subject: snap.subject, kind: snap.kind, stop,
          expect: expect || (stop && stop !== 'unsafe' ? 'maybe-long' : 'short'),
          queued: snap.state === 'queued' ? snap.words : undefined, elapsed,
          onStop: () => stopRemote(snap),
          onPause: () => call('Pause', id).then(() => false),
          onResume: () => call('Resume', id).then(() => false),
        }, { external: true, listed: false });
        task.follower = true;
        if (FINAL.has(snap.state)) { task.quietStart = true; task.timers.indicator && clearTimeout(task.timers.indicator); }
      }
      const prev = last;
      last = snap;
      if (task.ended) return;
      const quiet = !prev && FINAL.has(snap.state);
      task.stopMode = snap.stop || false;
      if (snap.total > 0 || snap.steps > 0) {
        const p = task.progressState;
        if (p.done !== snap.done || p.total !== snap.total || p.unit !== snap.unit || p.step !== snap.step || p.steps !== snap.steps || p.stage !== snap.stage) {
          task.progress({ done: snap.done, total: snap.total, unit: snap.unit, step: snap.step, steps: snap.steps, stage: snap.stage });
        }
      }
      switch (snap.state) {
        case 'queued': if (task.state !== 'queued') task.queued(snap.words); break;
        case 'running': if (task.state === 'paused') task.resumed(snap.words); else task.running(snap.words); break;
        case 'waiting': if (task.state !== 'waiting' || task.words !== snap.words) task.waiting(snap.words); break;
        case 'paused': if (task.state !== 'paused' || task.words !== snap.words) task.paused(snap.words); break;
        case 'stalled': if (task.state !== 'stalled' || task.words !== snap.words) task.stalled(snap.words); break;
        case 'done': task.finish('done', { words: snap.words, quiet }); break;
        case 'failed': task.finish('failed', { words: snap.words || snap.error?.userMsg || words.failedState, error: snap.error, ...(snap.retry !== 'unsafe' ? { retry: () => call('Retry', id) } : {}), quiet }); break;
        case 'cancelled': task.finish('cancelled', { words: words.cancelled, quiet }); break;
        case 'stopped': task.finish('stopped', { words: snap.words || words.stopped, quiet }); break;
      }
    },
  };
  const unfollow = () => {
    const set = followers.get(id);
    set?.delete(f);
    if (set && !set.size) followers.delete(id);
  };
  if (!followers.has(id)) followers.set(id, new Set());
  followers.get(id).add(f);
  const now = remote.find(s => s.id === id);
  if (now) f.apply(now);
  else if (connection.seen || remote.length) {
    // Not in the list we have: ask once; a task that isn't there is gone.
    Promise.resolve().then(() => connection?.bindings?.Get?.(id)).then(s => { if (s && !task) f.apply(s); }, () => { if (!task) unfollow(); });
  }
  return {
    get id() { return id; },
    get state() { return task?.state ?? null; },
    stop: () => task?.stop(),
    unfollow: () => { unfollow(); if (task && !task.ended) task.finish('cancelled', { words: words.cancelled, quiet: true }); },
  };
}

// ---- The summary and the task list -----------------------------------------
// Top-level tasks by what the person can tell apart: running (Waiting
// counts: it may finish by itself), queued, paused, stalled and failed.
function counts(all = entries()) {
  const top = all.filter(s => !s.parent);
  const n = (...states) => top.filter(s => states.includes(s.state)).length;
  const c = { running: n('running', 'waiting'), queued: n('queued'), paused: n('paused'), stalled: n('stalled'), failed: n('failed') };
  c.active = c.running + c.queued + c.paused + c.stalled;
  c.attention = c.failed > 0 || c.stalled > 0;
  return c;
}

// The summary shows while two or more tasks run, or any has failed: one
// button for the toolbar or the footer bar. Not a live region: each task
// announces its own changes.
function summary() {
  const b = taskEl('button', 'wk-task-summary');
  b.type = 'button';
  b.setAttribute('aria-haspopup', 'dialog');
  const d = taskEl('span', 'wk-dot');
  d.dataset.size = 'sm';
  d.setAttribute('aria-hidden', 'true');
  const text = taskEl('span', 'wk-task-summary-words');
  b.append(d, text);
  const draw = () => {
    const c = counts();
    // It stays while the list it opened is open, or while it has focus,
    // so focus never goes with it; it hides once focus leaves.
    if (!b.isConnected && b.wasConnected) { dispose(); return; }
    if (b.isConnected) b.wasConnected = true;
    b.hidden = !(c.active >= 2 || c.attention || list?.host.open || document.activeElement === b);
    text.textContent = [
      c.running ? words.running(c.running) : '', c.queued ? words.queuedCount(c.queued) : '', c.paused ? words.pausedCount(c.paused) : '',
      c.stalled ? words.stalledCount(c.stalled) : '', c.failed ? words.failed(c.failed) : '',
    ].filter(Boolean).join(' · ');
    if (c.attention) d.dataset.tone = 'attention';
    else if (c.running) d.dataset.tone = 'working';
    else delete d.dataset.tone;
  };
  // dispose(): stop updating it. A summary taken out of the page stops by
  // itself at the next change; make a new one to put back.
  const dispose = () => { watchers.delete(draw); summaries.delete(b); };
  b.dispose = dispose;
  watchers.add(draw);
  summaries.add(b);
  draw();
  b.addEventListener('click', () => showList({ opener: b }));
  b.addEventListener('focusout', draw);
  return b;
}

function buildList() {
  const host = document.createElement('wk-dialog');
  host.className = 'wk-task-list-host';
  const dialog = taskEl('dialog', 'wk-dialog wk-dialog--sheet wk-task-list');
  const head = taskEl('div', 'wk-dialog-header');
  const title = taskEl('h2', 'wk-dialog-title', words.tasks);
  title.id = 'wk-task-list-title';
  dialog.setAttribute('aria-labelledby', title.id);
  head.append(title);
  const scroll = taskEl('div', 'wk-dialog-scroll');
  scroll.tabIndex = 0;
  scroll.setAttribute('aria-labelledby', title.id);
  scroll.autofocus = true;
  const rows = taskEl('ul', 'wk-task-rows');
  const none = taskEl('p', 'wk-task-none', words.noTasks);
  scroll.append(rows, none);
  const foot = taskEl('div', 'wk-dialog-footer');
  const clear = taskEl('button', 'wk-button wk-dialog-start', words.clearFinished);
  clear.type = 'button';
  clear.dataset.action = 'clear';
  const close = taskEl('button', 'wk-button wk-button--primary', words.close);
  close.type = 'button';
  close.dataset.wkClose = '';
  foot.append(clear, close);
  dialog.append(head, scroll, foot);
  host.append(dialog);
  document.body.append(host);
  list = { host, dialog, rows, none, clear, scroll, ticker: 0 };
  dialog.addEventListener('click', e => {
    const b = e.target.closest?.('button[data-action]');
    if (!b || !dialog.contains(b) || b.getAttribute('aria-disabled') === 'true') return;
    act(b.dataset.action, b.closest('[data-id]')?.dataset.id);
  });
  watchers.add(() => { if (host.open) drawList(); });
  host.addEventListener('wk-close', () => { clearInterval(list.ticker); list.ticker = 0; changed(); });
}

function act(action, id) {
  const all = entries();
  const s = all.find(e => e.id === id);
  const kit = id && (local.get(id) || [...local.values()].flatMap(t => t.parts).find(p => p.id === id));
  switch (action) {
    case 'clear':
      call('DismissFinished');
      for (const [k, t] of local) if (t.ended) local.delete(k);
      changed();
      break;
    case 'dismiss':
      if (s?.source === 'go') call('Dismiss', id); else { local.delete(id); changed(); }
      break;
    case 'retry':
      if (s?.source === 'go') call('Retry', id);
      else if (kit?.retryFn) { local.delete(id); changed(); kit.retryFn(); }
      break;
    case 'stop': case 'pause': case 'resume':
      if (s?.source === 'go') {
        if (action === 'stop') stopRemote(s);
        else call(action === 'pause' ? 'Pause' : 'Resume', id);
      } else if (kit) { if (action === 'stop') kit.stop(); else kit.togglePause(); }
      break;
  }
}

function rowActions(s) {
  const out = [];
  if (ACTIVE.has(s.state)) {
    if (s.state === 'waiting' && s.error && s.source === 'go') out.push(['retry', words.retryNow]);
    if (s.stop === 'pause' && s.state !== 'queued') out.push([s.state === 'paused' ? 'resume' : 'pause', s.state === 'paused' ? words.resume : words.pause]);
    else if (s.stop && s.stop !== 'unsafe') out.push(['stop', s.stop === 'cancel' || s.state === 'queued' ? words.cancel : s.stop === 'stop-asks' ? words.stopAsks : words.stop]);
  } else if (!s.parent) {
    if (s.state === 'failed' && s.retry !== 'unsafe') out.push(['retry', words.retry]);
    out.push(['dismiss', words.dismiss]);
  }
  return out;
}

function rowText(s) {
  const count = s.total > 0 ? words.count(s.done, s.total, s.unit) : '';
  const stage = [s.steps ? words.step(s.step, s.steps) : '', s.stage].filter(Boolean).join(' · ');
  const said = s.state === 'failed' ? s.words || s.error?.userMsg || '' : s.words;
  const extra = s.nextTry && s.state === 'waiting' ? words.nextTry(s.nextTry) : s.stop === 'unsafe' && ACTIVE.has(s.state) ? words.unsafe : '';
  const title = s.subject || said;
  const line = [s.subject ? said : '', ACTIVE.has(s.state) ? stage : '', ACTIVE.has(s.state) ? count : '', extra].filter(Boolean).join(' · ');
  return { title, line };
}

function drawRow(li, s) {
  li.dataset.state = s.state;
  const tone = s.state === 'running' ? 'working' : s.state === 'failed' || s.state === 'stalled' ? 'attention' : null;
  let d = li.querySelector(':scope > .wk-dot');
  if (!d) { d = dot(tone); li.prepend(d); }
  if (tone) d.dataset.tone = tone; else delete d.dataset.tone;
  d.dataset.shape = FINAL.has(s.state) && s.state !== 'failed' ? 'hollow' : 'filled';
  let text = li.querySelector(':scope > .wk-task-text');
  if (!text) { text = taskEl('div', 'wk-task-text'); text.append(taskEl('span', 'wk-task-title'), taskEl('span', 'wk-task-line')); d.after(text); }
  const t = rowText(s);
  text.children[0].textContent = t.title;
  text.children[1].textContent = t.line;
  text.children[1].hidden = !t.line;
  let actions = li.querySelector(':scope > .wk-task-actions');
  if (!actions) { actions = taskEl('div', 'wk-task-actions'); text.after(actions); }
  const want = rowActions(s);
  const had = document.activeElement;
  const keep = new Map([...actions.children].map(b => [b.dataset.action, b]));
  const next = want.map(([action, label]) => {
    let b = keep.get(action);
    if (!b) { b = taskEl('button', 'wk-button wk-button--small'); b.type = 'button'; b.dataset.action = action; }
    b.textContent = label;
    b.setAttribute('aria-describedby', li.dataset.titleId);
    keep.delete(action);
    return b;
  });
  syncChildren(actions, next);
  if (had && [...keep.values()].includes(had)) (next[0] || li).focus({ preventScroll: true });
}

function drawList() {
  const { rows, none, clear } = list;
  const all = entries();
  const byParent = new Map();
  for (const s of all) if (s.parent) { if (!byParent.has(s.parent)) byParent.set(s.parent, []); byParent.get(s.parent).push(s); }
  const top = all.filter(s => !s.parent);
  const had = document.activeElement;
  const old = new Map([...rows.querySelectorAll('li[data-id]')].map(li => [li.dataset.id, li]));
  const fill = (ul, items) => {
    const lis = items.map(s => {
      let li = old.get(s.id);
      old.delete(s.id);
      if (!li) {
        li = taskEl('li', 'wk-task-row');
        li.dataset.id = s.id;
        li.tabIndex = -1;
        li.dataset.titleId = `wk-task-row-${++taskUid}`;
      }
      drawRow(li, s);
      li.querySelector(':scope > .wk-task-text > .wk-task-title').id = li.dataset.titleId;
      const parts = byParent.get(s.id);
      let sub = li.querySelector(':scope > .wk-task-parts');
      if (parts?.length) {
        if (!sub) { sub = taskEl('ul', 'wk-task-parts'); li.append(sub); }
        fill(sub, parts);
      } else sub?.remove();
      return li;
    });
    syncChildren(ul, lis);
  };
  // A row that goes while focus is in it hands focus to the next row.
  const staying = new Set(all.map(s => s.id));
  const sib = [...rows.querySelectorAll('li[data-id]')];
  const lost = sib.find(li => !staying.has(li.dataset.id) && li.contains(had));
  let after = null;
  if (lost) {
    const i = sib.indexOf(lost);
    after = sib.slice(i + 1).find(li => staying.has(li.dataset.id)) || sib.slice(0, i).reverse().find(li => staying.has(li.dataset.id));
  }
  fill(rows, top);
  none.hidden = top.length > 0;
  rows.hidden = top.length === 0;
  clear.setAttribute('aria-disabled', String(!all.some(s => FINAL.has(s.state) && !s.parent)));
  if (lost) (after?.isConnected ? after : list.scroll).focus({ preventScroll: true });
  // "Next try in 8 s" counts down while the list is open.
  const ticking = all.some(s => s.nextTry && s.state === 'waiting');
  if (ticking && !list.ticker) list.ticker = setInterval(() => { if (list.host.open) drawList(); }, 1000);
  if (!ticking && list.ticker) { clearInterval(list.ticker); list.ticker = 0; }
}

// Opens the task list: from the summary, or from Window > Tasks.
function showList({ opener } = {}) {
  if (!list) buildList();
  drawList();
  list.host.show({ opener });
  return list.host;
}

wkTask.connect = connect;
wkTask.follow = follow;
wkTask.summary = summary;
wkTask.showList = showList;
wkTask.words = words;

// For opt-in packages (@wails-kit/editor) that use the window's wkTask
// without bundling the kit: document.wkTask, the first copy of the kit's in
// the window, so their tasks join its task list.
if (typeof document !== 'undefined') document.wkTask ??= wkTask;

export { wkTask };
