// Status: chips, notices, empty states, error boxes, progress, tasks, the
// task list, toasts and skeleton rows.
import { icons, tryButton } from './shared.js';

export default [
  {
    id: 'chip', title: 'Chip', group: 'status', card: 'Chip',
    intro: 'A label, a count, an absence (muted, dashed) or what the person may do (access). A chip may lead with a small status dot, then the item and its state in words.',
    examples: [{
      title: 'Kinds',
      html: `<div class="g-row">
  <span class="wk-chip">Label</span>
  <button class="wk-chip" type="button">Button chip</button>
  <button class="wk-chip" type="button" disabled>Disabled</button>
  <span class="wk-chip wk-chip--count">6</span>
  <span class="wk-chip wk-chip--count wk-chip--attention">2</span>
  <span class="wk-chip"><span class="wk-dot" data-size="sm" data-tone="working" aria-hidden="true"></span>export · running</span>
  <span class="wk-chip"><span class="wk-dot" data-size="sm" aria-hidden="true"></span>check · 3 findings</span>
  <span class="wk-chip wk-chip--muted">No runs yet</span>
  <span class="wk-chip wk-chip--access">${icons.eye}View only</span>
  <span class="wk-chip wk-chip--access" data-level="manage">${icons.lock}Can manage</span>
</div>`,
    }],
  },
  {
    id: 'notice', title: 'Notice', group: 'status', card: 'Notice',
    intro: 'Neutral (the default) or attention (<code>data-tone="attention"</code>), with or without actions; inline for one line.',
    examples: [{
      title: 'Tones and inline',
      html: `<div class="g-stack">
  <div class="wk-notice" role="status">${icons.warn}
    <div class="wk-notice-body"><b>Jobs for Linux ARM64 are waiting.</b><p>The one machine that takes them is in use. They start when it is free.</p></div>
  </div>
  <div class="wk-notice" data-tone="attention" role="alert">${icons.warn}
    <div class="wk-notice-body"><b>The server can't reach the network.</b><p>No job can start. Trying again at 2:18 pm.</p></div>
    <div class="wk-notice-actions"><button class="wk-button wk-button--primary" type="button">Try now</button><a class="wk-link" href="#/notice">Details</a></div>
  </div>
  <div class="wk-notice wk-notice--inline"><div class="wk-notice-body">Settings saved. One needs a restart.</div>
    <div class="wk-notice-actions"><button class="wk-button wk-button--small" type="button">Review settings</button></div></div>
</div>`,
    }],
  },
  {
    id: 'empty', title: 'Empty state', group: 'status', card: 'EmptyState',
    intro: 'A title, one line and at most one button: what is true now, and what happens next without the person.',
    examples: [{
      title: 'In a pane',
      html: `<div class="wk-empty g-frame">
  <h3 class="wk-empty-title">Nothing here yet</h3>
  <p>Runs show here when a job starts.</p>
  <button class="wk-button" type="button">Start a job</button>
</div>`,
    }],
  },
  {
    id: 'errorbox', title: 'Error box', group: 'status', card: 'ErrorBox',
    intro: 'The raw error, under the words that say what failed: its facts, and the message in a focusable <code>&lt;pre&gt;</code>, with a copy button.',
    examples: [{
      title: 'An error with facts',
      html: `<div class="wk-errorbox g-measure" role="group" aria-label="Error details">
  <div class="wk-errorbox-head"><span>Couldn't download the image</span><button class="wk-copy" type="button" aria-label="Copy error details" title="Copy" data-wk-copy-from="g-error-message">${icons.copy}</button></div>
  <dl class="wk-facts"><dt>Failed</dt><dd>5 times in a row, last at 2:13 pm</dd><dt>Next try</dt><dd>2:18 pm</dd></dl>
  <pre tabindex="0" role="region" aria-label="Error message" id="g-error-message">pull registry.example/image:2.3: Get "https://registry.example/v2/": dial tcp: lookup registry.example: no such host</pre>
</div>`,
    }],
  },
  {
    id: 'progress', title: 'Spinner and progress bar', group: 'status',
    intro: 'A spinner stays a spinner; a bar starts indeterminate and becomes determinate once the total is known, and only moves forward. <code>setProgress</code> changes the bar\'s value attributes at most every 5 s. With reduced motion the sweep becomes a fade.',
    examples: [{
      title: 'Spinner, determinate and indeterminate',
      html: `<div class="g-row"><span class="wk-spinner" role="progressbar" aria-label="Fetching runs" id="g-spinner"></span><span class="wk-caption">Fetching runs…</span></div>
<div class="g-bars">
  <div class="g-stack"><div class="wk-progress" role="progressbar" aria-label="Copying files" id="g-progress"></div><span class="wk-caption">12 of 40 files</span></div>
  <div class="g-stack"><div class="wk-progress" role="progressbar" aria-label="Fetching runs" id="g-progress-sweep"></div><span class="wk-caption">Fetching runs…</span></div>
</div>`,
      after(el, { kit }) {
        kit.setProgress(el.querySelector('#g-progress'), { done: 12, total: 40, unit: 'files' });
      },
    }, {
      title: 'Try it: a bar that a task moves',
      note: 'Hidden until 400 ms, indeterminate until the work reports a total, then 1 of 10 to 10 of 10, hidden again at the end.',
      html: `<div class="g-row">${tryButton('g-bar-try', 'Copy 10 files')}</div>
<div class="g-bars"><div class="g-stack"><div class="wk-progress" role="progressbar" aria-label="Copying files" id="g-bar"></div></div></div>`,
      after(el, { task: wkTask, track, later }) {
        const button = el.querySelector('#g-bar-try');
        const bar = el.querySelector('#g-bar');
        bar.hidden = true;
        button.addEventListener('click', () => track((async () => {
          const task = wkTask({ control: button, words: 'Copying files', bar });
          await later(1200);
          for (let n = 1; n <= 10; n++) { task.progress({ done: n, total: 10, unit: 'files' }); await later(250); }
          await task.done('Copied 10 files');
        })()));
      },
    }],
  },
  {
    id: 'task', title: 'Task lifecycle', group: 'status',
    intro: 'The states of BEHAVIOUR.md\'s action lifecycle, run by <code>wkTask</code> with its real timings on a row\'s state cell. Press a row\'s "Try it". Waiting takes 30 s, so its row calls <code>waiting()</code> itself; the 2-minute timeout is shortened to 5 s on its row (<code>timeout: 5000</code>). Leaving the page cancels whatever is still running.',
    examples: [{
      title: 'Try it: one state per row',
      wide: true,
      html: `<table class="wk-table g-tasks" id="g-tasks">
  <thead><tr><th>State</th><th>The row's state</th><th><span class="wk-visually-hidden">Actions</span></th></tr></thead>
  <tbody>
    <tr data-scenario="quick"><th scope="row">Quiet wait <span class="wk-caption">(done in 200 ms: no indicator)</span></th><td>Ready</td><td></td></tr>
    <tr data-scenario="done"><th scope="row">Working, then Done</th><td>Ready</td><td></td></tr>
    <tr data-scenario="show"><th scope="row">Done, with Show <span class="wk-caption">(the result is elsewhere)</span></th><td>Ready</td><td></td></tr>
    <tr data-scenario="measured"><th scope="row">Measured progress</th><td>Ready</td><td></td></tr>
    <tr data-scenario="long"><th scope="row">Long, with Stop</th><td>Ready</td><td></td></tr>
    <tr data-scenario="maybe"><th scope="row">Long wait, timed <span class="wk-caption">(might be long: Stop shows at 10 s, in space kept for it)</span></th><td>Ready</td><td></td></tr>
    <tr data-scenario="asks"><th scope="row">Stop…, with its Alert <span class="wk-caption">(stopping loses work: it asks first)</span></th><td>Ready</td><td></td></tr>
    <tr data-scenario="esc"><th scope="row">Esc cancels short work <span class="wk-caption">(press Esc while it runs)</span></th><td>Ready</td><td></td></tr>
    <tr data-scenario="pause"><th scope="row">Paused and resumed</th><td>Ready</td><td></td></tr>
    <tr data-scenario="waiting"><th scope="row">Waiting</th><td>Ready</td><td></td></tr>
    <tr data-scenario="stalled"><th scope="row">Stalled</th><td>Ready</td><td></td></tr>
    <tr data-scenario="queued"><th scope="row">Queued</th><td>Ready</td><td></td></tr>
    <tr data-scenario="failed"><th scope="row">Failed</th><td>Ready</td><td></td></tr>
    <tr data-scenario="timeout"><th scope="row">Timed out <span class="wk-caption">(no progress: the timeout, shortened to 5 s)</span></th><td>Ready</td><td></td></tr>
    <tr data-scenario="cancelled"><th scope="row">Cancelled</th><td>Ready</td><td></td></tr>
    <tr data-scenario="stopped"><th scope="row">Stopped</th><td>Ready</td><td></td></tr>
    <tr data-scenario="parts"><th scope="row">Work in parts <span class="wk-caption">(one part fails)</span></th><td><span class="g-stack"><span>Reviewer 1</span><span>Reviewer 2</span><span>Reviewer 3</span></span></td><td></td></tr>
  </tbody>
</table>
<wk-dialog id="g-stop-alert">
  <dialog class="wk-dialog wk-dialog--alert" role="alertdialog" aria-labelledby="g-stop-alert-title" aria-describedby="g-stop-alert-body">
    <h3 class="wk-dialog-title" id="g-stop-alert-title">Stop copying?</h3>
    <p class="wk-dialog-body" id="g-stop-alert-body">Some files are copied; the rest won’t be.</p>
    <div class="wk-dialog-actions">
      <button class="wk-button wk-button--destructive" type="button" value="stop" data-wk-close>Stop copying</button>
      <button class="wk-button wk-button--primary" type="button" value="keep" data-wk-close autofocus>Keep copying</button>
    </div>
  </dialog>
</wk-dialog>`,
      after(el, { task: wkTask, track, later }) {
        // Stop… asks first: onStop opens the Alert, and resolves to false
        // (keep the task running) unless the person chooses to stop.
        const alert = el.querySelector('#g-stop-alert');
        const askToStop = t => new Promise(resolve => {
          alert.querySelector('#g-stop-alert-body').textContent = `${t.count || 'Some files'} are copied; the rest won’t be.`;
          alert.addEventListener('wk-close', e => resolve(e.detail?.returnValue === 'stop'), { once: true });
          alert.show();
        });
        const scenarios = {
          quick: async t => { await later(200); return t.done('Exported'); },
          done: async t => { await later(2000); return t.done({ words: 'Exported', visible: false }); },
          show: async t => { await later(1500); return t.done({ words: 'Report exported', visible: false, show: () => {}, showLabel: 'Show' }); },
          parts: async (t, row) => {
            const cells = [...row.cells[1].querySelectorAll('.g-stack > span')];
            const parts = cells.map((target, i) => t.part({ target, words: `Reviewer ${i + 1} reviewing` }));
            await later(1200);
            parts[0].done({ words: 'Reviewer 1 passed', visible: false });
            await later(600);
            parts[1].fail({ words: 'Reviewer 2 couldn’t run', error: new Error('exit 1') });
            await later(600);
            return parts[2].done({ words: 'Reviewer 3 passed', visible: false });
          },
          measured: async t => { for (let n = 0; n <= 8; n++) { t.progress({ done: n * 5, total: 40, unit: 'files' }); await later(300); } return t.done({ words: 'Copied 40 files', visible: false }); },
          long: async t => { await later(6000); return t.done({ words: 'Synced', visible: false }); },
          maybe: async t => { await later(15000); return t.done({ words: 'Synced', visible: false }); },
          asks: async t => { for (let n = 0; n <= 40 && !t.signal.aborted; n += 2) { t.progress({ done: n, total: 40, unit: 'files' }); await later(500); } if (!t.signal.aborted) await t.done({ words: 'Copied 40 files', visible: false }); },
          esc: async t => { await later(4000); return t.done({ words: 'Checked', visible: false }); },
          timeout: async () => {},
          pause: async t => { await later(6000); return t.done({ words: 'Synced', visible: false }); },
          waiting: async t => { await later(800); t.waiting('Waiting for the server…'); await later(2500); t.progress({ done: 1, total: 2 }); await later(800); return t.done({ words: 'Uploaded', visible: false }); },
          stalled: async t => { await later(800); t.stalled('Sign-in expired · Sign in'); await later(3000); return t.fail({ words: 'Couldn’t sync: sign in again', error: new Error('401') }); },
          queued: async t => { await later(2000); t.running('Exporting'); await later(1500); return t.done({ words: 'Exported', visible: false }); },
          failed: async t => { await later(1500); return t.fail({ words: 'Couldn’t export', error: new Error('disk full'), retry: () => {} }); },
          cancelled: async t => { await later(1500); return t.cancelled(); },
          stopped: async t => { for (let n = 0; n < 4; n++) { t.progress({ done: n * 4, total: 40, unit: 'files' }); await later(400); } return t.stopped(); },
        };
        const specs = {
          long: { expect: 'long', stop: 'stop', words: 'Syncing' },
          pause: { expect: 'long', stop: 'pause', words: 'Syncing', onPause: () => true, onResume: () => true },
          queued: { stop: 'cancel', queued: 'Queued · after 2 others', words: 'Exporting' },
          cancelled: { expect: 'long', stop: 'cancel', words: 'Exporting' },
          stopped: { expect: 'long', stop: 'stop', words: 'Copying files' },
          maybe: { expect: 'maybe-long', stop: 'stop', words: 'Syncing' },
          asks: { expect: 'long', stop: 'stop-asks', words: 'Copying files', onStop: askToStop },
          esc: { stop: 'cancel', words: 'Checking' },
          timeout: { words: 'Exporting', timeout: 5000 },
          parts: { words: 'Running reviewers', unit: 'reviewers' },
        };
        for (const row of el.querySelectorAll('tr[data-scenario]')) {
          const name = row.dataset.scenario;
          const button = document.createElement('button');
          button.className = 'wk-button wk-button--small';
          button.type = 'button';
          button.textContent = 'Try it';
          button.dataset.galleryTry = '';
          button.setAttribute('aria-label', `Try it: ${row.querySelector('th').firstChild.textContent.trim()}`);
          row.cells[2].append(button);
          button.addEventListener('click', () => {
            const cell = row.cells[1];
            if (name !== 'parts') cell.textContent = 'Ready';
            let ended;
            const end = new Promise(resolve => { ended = resolve; });
            const task = wkTask({ words: 'Exporting', ...specs[name], control: button, target: name === 'parts' ? undefined : cell, subject: row.querySelector('th').firstChild.textContent.trim(), onEnd: ended });
            track(Promise.all([end, scenarios[name](task, row)]));
          });
        }
      },
    }],
  },
  {
    id: 'task-list', title: 'Task list and summary', group: 'status',
    intro: '<code>wkTask.summary()</code> counts the tasks by state while two or more run or one has failed; pressing it opens the task list (<code>wkTask.showList()</code>), a Sheet with each task\'s state and actions.',
    examples: [{
      title: 'Try it: start four tasks, then open the list',
      html: `<div class="g-window">
  <div class="g-window-body g-row">${tryButton('g-tasks-start', 'Start four tasks and show them')}${tryButton('g-tasks-open', 'Open the task list')}</div>
  <footer class="wk-footer-bar" id="g-tasks-footer"></footer>
</div>`,
      after(el, { kit, task: wkTask, track, later }) {
        const summary = kit.wkTask.summary();
        el.querySelector('#g-tasks-footer').append(summary);
        const start = el.querySelector('#g-tasks-start');
        start.addEventListener('click', () => {
          const run = (spec, fn) => new Promise(resolve => { const t = wkTask({ ...spec, onEnd: resolve }); fn(t); });
          track(Promise.all([
            run({ words: 'Syncing the shared folder', subject: 'Shared folder', expect: 'long', stop: 'stop' }, t => later(5000).then(() => t.done('Synced'))),
            run({ words: 'Uploading figures.csv', subject: 'figures.csv', expect: 'long', stop: 'pause', onPause: () => true, onResume: () => true }, t => later(300).then(() => t.paused('Paused · 3 of 8 files')).then(() => later(5000)).then(() => t.done('Uploaded'))),
            run({ words: 'Exporting the report', subject: 'Quarterly report' }, t => later(1200).then(() => t.fail({ words: 'Couldn’t export the report', error: new Error('disk full'), retry: () => {} }))),
            run({ words: 'Running reviewers', subject: 'Reviewers', unit: 'reviewers', expect: 'long', stop: 'stop' }, t => {
              const parts = [1, 2, 3].map(i => t.part({ words: `Reviewer ${i} reviewing`, subject: `Reviewer ${i}` }));
              later(2500).then(() => parts[0].done('Reviewer 1 passed'));
              later(4000).then(() => { parts[1].done('Reviewer 2 passed'); parts[2].done('Reviewer 3 passed'); });
            }),
            // The list opens once they have started, to show them running.
            later(600, () => kit.wkTask.showList({ opener: start })),
          ]));
        });
        el.querySelector('#g-tasks-open').addEventListener('click', e => kit.wkTask.showList({ opener: e.currentTarget }));
        return () => summary.dispose?.();
      },
    }],
  },
  {
    id: 'toast', title: 'Toast', group: 'status',
    intro: 'A brief outcome at the bottom of the window, one at a time: 6 s, or 10 s with an action, counted only while the window has focus and the pointer and focus are outside it. F6 moves focus to its action and back; Esc dismisses it. Its words go to the announcer; it never takes focus.',
    examples: [{
      title: 'Try it',
      html: `<div class="g-row">
  ${tryButton('g-toast-plain', 'Show a toast')}
  ${tryButton('g-toast-action', 'Archive, with Undo')}
  <span class="wk-caption" id="g-toast-words"></span>
</div>`,
      after(el, { kit, track }) {
        const words = el.querySelector('#g-toast-words');
        const shown = toast => track(new Promise(resolve => {
          // Settles once the toast is in the page; it goes by itself.
          requestAnimationFrame(() => resolve(toast));
        }));
        el.querySelector('#g-toast-plain').addEventListener('click', () => shown(kit.wkToast({ words: 'Link copied' })));
        el.querySelector('#g-toast-action').addEventListener('click', () => {
          words.textContent = 'Archived';
          shown(kit.wkToast({ words: 'Conversation archived', action: { label: 'Undo', run: () => { words.textContent = 'Back in the list'; } } }));
        });
        return () => document.querySelector('.wk-toast')?.remove();
      },
    }],
  },
  {
    id: 'undo', title: 'App undo', group: 'status',
    intro: '<code>wkUndo</code> is the frontend of the app\'s undo stack, which lives on the Go side (the <code>shortcuts</code> package\'s <code>UndoStack</code>). Here a stand-in for that stack runs in the page, and two buttons stand in for the native Edit menu: like a menu bar, they don\'t take focus. A toast\'s Undo and Edit &gt; Undo use the same stack; with text focus, Edit &gt; Undo undoes the typing first.',
    examples: [{
      title: 'Try it: archive, then undo from the toast or from Edit > Undo',
      note: 'Type in the field, then press Edit &gt; Undo: the typing goes first, then the archive.',
      html: `<div class="g-row">
  ${tryButton('g-undo-archive', 'Archive a conversation')}
  <input class="wk-field" id="g-undo-field" aria-label="Type, then Edit > Undo" placeholder="Type, then Edit > Undo">
</div>
<div class="g-row" role="group" aria-label="Edit menu (a stand-in)">
  <button class="wk-button" type="button" id="g-edit-undo">Undo</button>
  <button class="wk-button" type="button" id="g-edit-redo">Redo</button>
</div>
<div class="g-row"><ul class="g-plain-list" id="g-undo-list" aria-label="Conversations"></ul></div>`,
      after(el, { kit, track }) {
        // A stand-in for the Go side: shortcuts.UndoStack's events and
        // Bindings(), as an app connects them with Wails's Events.On.
        const conversations = ['Plans for May', 'Lunch on Friday', 'The quarterly report', 'Holiday rota'];
        const list = el.querySelector('#g-undo-list');
        const drawList = () => { list.replaceChildren(...conversations.map(name => Object.assign(document.createElement('li'), { textContent: name }))); };
        drawList();
        const listeners = {};
        const emit = (name, payload) => { for (const fn of listeners[name] || []) fn(payload); };
        const go = { undo: [], redo: [], seq: 0, textFocus: false };
        const entry = (e, verb) => (e ? { id: e.id, name: e.name, label: `${verb} ${e.name}` } : null);
        const state = () => ({ undo: entry(go.undo.at(-1), 'Undo'), redo: entry(go.redo.at(-1), 'Redo'), busy: false, textFocus: go.textFocus });
        const changed = () => emit('undo:changed', state());
        const push = (name, undo, redo) => { const id = `g-undo-${++go.seq}`; go.undo.push({ id, name, undo, redo }); go.redo = []; changed(); return id; };
        const undo = id => {
          const top = go.undo.at(-1);
          if (!top) throw { code: 'undo_empty' };
          if (id && top.id !== id) throw { code: 'undo_stale' };
          top.undo(); go.undo.pop(); go.redo.push(top); changed();
        };
        const redo = () => { const top = go.redo.pop(); if (!top) throw { code: 'undo_empty' }; top.redo(); go.undo.push(top); changed(); };
        const disconnect = kit.wkUndo.connect({
          on: (name, fn) => { (listeners[name] ??= []).push(fn); return () => { listeners[name] = listeners[name].filter(f => f !== fn); }; },
          bindings: {
            State: async () => state(),
            Undo: async () => undo(),
            Redo: async () => redo(),
            UndoID: async id => undo(id),
            SetTextFocus: async on => { go.textFocus = on; changed(); },
          },
        });
        // Edit > Undo and Redo, as UndoStack.MenuUndo does them: typing first.
        const menu = op => () => {
          if (go.textFocus) { emit('undo:text', { op }); return; }
          try { if (op === 'undo') undo(); else redo(); } catch { /* nothing to undo */ }
        };
        const undoItem = el.querySelector('#g-edit-undo'), redoItem = el.querySelector('#g-edit-redo');
        for (const [b, op] of [[undoItem, 'undo'], [redoItem, 'redo']]) {
          b.addEventListener('mousedown', e => e.preventDefault()); // a menu bar doesn't take focus
          b.addEventListener('click', menu(op));
        }
        // The menu's labels follow the stack, as the native menu's do.
        const label = () => {
          const s = kit.wkUndo.state();
          undoItem.textContent = s.textFocus ? 'Edit > Undo Typing' : `Edit > ${s.undo?.label || 'Undo'}`;
          redoItem.textContent = s.textFocus ? 'Edit > Redo Typing' : `Edit > ${s.redo?.label || 'Redo'}`;
          undoItem.setAttribute('aria-disabled', String(!s.textFocus && !s.undo));
          redoItem.setAttribute('aria-disabled', String(!s.textFocus && !s.redo));
        };
        document.addEventListener('wk-undo-change', label);
        label();
        el.querySelector('#g-undo-archive').addEventListener('click', () => {
          if (!conversations.length) return;
          const [name] = conversations.splice(0, 1);
          drawList();
          const id = push('Archive', () => { conversations.unshift(name); drawList(); }, () => { conversations.splice(conversations.indexOf(name), 1); drawList(); });
          const toast = kit.wkToast({ words: `“${name}” archived`, action: kit.wkUndo.action(id) });
          track(new Promise(resolve => requestAnimationFrame(() => resolve(toast))));
        });
        return () => {
          document.removeEventListener('wk-undo-change', label);
          disconnect();
          document.querySelector('.wk-toast')?.remove();
        };
      },
    }],
  },
  {
    id: 'announcer', title: 'Announcer', group: 'status',
    intro: '<code>announce(words)</code> says words through two visually hidden live regions, polite and assertive, in the page from load; words within 500 ms are joined. A modal makes the rest of the page inert, live regions too, so while one is open the regions sit inside it. <code>wk-dialog</code> moves them, and so does any <code>&lt;dialog&gt;</code> in the document when its <code>open</code> attribute changes. The kit can\'t see a modal <code>&lt;dialog&gt;</code> inside another component\'s shadow root: for that one the app calls <code>placeAnnouncer(dialog, open)</code> when it opens and closes.',
    examples: [{
      title: 'Try it: say something, from the page, a dialog and a modal the kit can\'t see',
      note: 'What the regions say shows below, with where they are. The dialog needs nothing from the app; the modal in a shadow root needs placeAnnouncer.',
      html: `<div class="g-row">
  ${tryButton('g-say', 'Say “Saved”')}
  ${tryButton('g-say-dialog', 'Open a dialog, and say something there')}
  ${tryButton('g-say-modal', 'Open a modal in a shadow root, and say something there')}
</div>
<p class="wk-caption">The regions are in <b id="g-announcer-home">the page</b>.</p>
<pre class="g-log" id="g-said" aria-label="What the announcer said" tabindex="0"></pre>
<dialog class="wk-dialog" id="g-plain-modal" aria-labelledby="g-plain-modal-title">
  <h3 class="wk-dialog-title" id="g-plain-modal-title">A plain dialog</h3>
  <p class="wk-dialog-body">Opened with showModal(), not wk-dialog. The kit sees its open attribute change and moves the regions into it by itself.</p>
  <div class="wk-dialog-actions"><button class="wk-button wk-button--primary" type="button" id="g-plain-close">Close</button></div>
</dialog>
<div id="g-shadow-host"></div>`,
      after(el, { kit, track }) {
        const log = el.querySelector('#g-said');
        const home = el.querySelector('#g-announcer-home');
        const plain = el.querySelector('#g-plain-modal');
        const host = document.wkAnnouncer;
        // Another component's modal, in its shadow root: the kit's observer
        // and its dialog events don't reach in there.
        const shadow = el.querySelector('#g-shadow-host').attachShadow({ mode: 'open' });
        const sheet = document.getElementById('gallery-kit').cloneNode();
        sheet.removeAttribute('id');
        const box = document.createElement('div');
        box.className = 'wk-root';
        box.innerHTML = `<dialog class="wk-dialog" aria-labelledby="t"><h3 class="wk-dialog-title" id="t">A modal in a shadow root</h3>
<p class="wk-dialog-body">The kit can't see this dialog open, so the page calls placeAnnouncer(dialog, true), and placeAnnouncer(dialog, false) when it closes.</p>
<div class="wk-dialog-actions"><button class="wk-button wk-button--primary" type="button">Close</button></div></dialog>`;
        shadow.append(sheet, box);
        const modal = box.querySelector('dialog');
        // A readout of the regions: what they say, and where they are.
        const show = () => {
          const where = host.parentNode === modal ? 'the modal in the shadow root' : host.parentNode === plain ? 'the dialog' : 'the page';
          if (home.textContent !== where) home.textContent = where;
          for (const r of host.querySelectorAll('[aria-live]')) {
            if (r.textContent && r.textContent !== r.dataset.gallerySaid) log.textContent = `${r.getAttribute('aria-live')}: ${r.textContent}\n${log.textContent}`.split('\n').slice(0, 4).join('\n');
            r.dataset.gallerySaid = r.textContent;
          }
        };
        // The regions' own changes wherever they are, and their moves.
        const watch = new MutationObserver(show);
        watch.observe(document.documentElement, { subtree: true, childList: true, characterData: true });
        watch.observe(host, { subtree: true, childList: true, characterData: true });
        const said = () => track(new Promise(resolve => setTimeout(resolve, 600)));
        el.querySelector('#g-say').addEventListener('click', () => { kit.announce('Saved'); said(); });
        el.querySelector('#g-say-dialog').addEventListener('click', () => {
          plain.showModal();
          kit.announce('Said from inside the dialog');
          said();
        });
        el.querySelector('#g-plain-close').addEventListener('click', () => plain.close());
        el.querySelector('#g-say-modal').addEventListener('click', () => {
          modal.showModal();
          kit.placeAnnouncer(modal, true);
          kit.announce('Said from inside the modal');
          said();
        });
        modal.querySelector('button').addEventListener('click', () => modal.close());
        modal.addEventListener('close', () => kit.placeAnnouncer(modal, false));
        return () => {
          watch.disconnect();
          for (const d of [plain, modal]) if (d.open) d.close();
        };
      },
    }],
  },
  {
    id: 'skeleton', title: 'Skeleton rows', group: 'status',
    intro: 'For a table that loads: from 400 ms the body\'s rows give way to still bars at the real row height, for at least 500 ms, then the new rows swap in, in one step. The table has <code>aria-busy</code> until then. After 3 s (<code>timings.loadingLine</code>) a line says what\'s loading and from where; the kit doesn\'t draw it yet (BEHAVIOUR.md, Known gaps), so the app adds it, as this page does.',
    examples: [{
      title: 'Try it: reload the table',
      note: 'Fast answers in 200 ms (no skeleton ever shows); Slow in 1.5 s; Very slow in 5 s, with the loading line from 3 s.',
      html: `<div class="g-row">${tryButton('g-skeleton-fast', 'Reload (fast)')}${tryButton('g-skeleton-slow', 'Reload (slow)')}${tryButton('g-skeleton-long', 'Reload (very slow)')}</div>
<table class="wk-table" id="g-skeleton-table">
  <thead><tr><th>Name</th><th>State</th><th class="wk-num">Runs</th></tr></thead>
  <tbody><tr><td>alpha</td><td>Running</td><td class="wk-num">19</td></tr><tr><td>beta</td><td>Ready</td><td class="wk-num">4</td></tr><tr><td>gamma</td><td>Offline</td><td class="wk-num">0</td></tr></tbody>
</table>
<p class="wk-caption" id="g-skeleton-line" hidden>Fetching runs from server.example…</p>`,
      after(el, { kit, track, later }) {
        const body = el.querySelector('tbody');
        const line = el.querySelector('#g-skeleton-line');
        let n = 0;
        const draw = () => {
          n++;
          body.innerHTML = ['alpha', 'beta', 'gamma'].map((name, i) => `<tr><td>${name}</td><td>${['Running', 'Ready', 'Offline'][i]}</td><td class="wk-num">${(i + 1) * 3 + n}</td></tr>`).join('');
        };
        const reload = ms => () => track((async () => {
          const loading = kit.wkSkeleton(body);
          // The loading line is the app's: shown after 3 s, gone with the skeleton.
          let waited = true;
          later(kit.timings.loadingLine, () => { if (waited) line.hidden = false; });
          await later(ms);
          await loading.done(() => { waited = false; line.hidden = true; draw(); });
        })());
        el.querySelector('#g-skeleton-fast').addEventListener('click', reload(200));
        el.querySelector('#g-skeleton-slow').addEventListener('click', reload(1500));
        el.querySelector('#g-skeleton-long').addEventListener('click', reload(5000));
      },
    }],
  },
];
