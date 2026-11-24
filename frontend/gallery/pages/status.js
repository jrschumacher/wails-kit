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
      html: `<div class="g-row"><span class="wk-spinner" role="progressbar" aria-label="Fetching runs"></span><span class="wk-caption">Fetching runs…</span></div>
<div class="g-bars">
  <div class="g-stack"><div class="wk-progress" role="progressbar" aria-label="Copying files" id="g-progress"></div><span class="wk-caption">12 of 40 files</span></div>
  <div class="g-stack"><div class="wk-progress" role="progressbar" aria-label="Fetching runs"></div><span class="wk-caption">Fetching runs…</span></div>
</div>`,
      after(el, { kit }) {
        kit.setProgress(el.querySelector('#g-progress'), { done: 12, total: 40, unit: 'files' });
      },
    }, {
      title: 'Try it: a bar that a task moves',
      note: 'Hidden until 400 ms, indeterminate until the work reports a total, then 1 of 10 to 10 of 10, hidden again at the end.',
      html: `<div class="g-row">${tryButton('g-bar-try', 'Copy 10 files')}</div>
<div class="g-bars"><div class="g-stack"><div class="wk-progress" role="progressbar" aria-label="Copying files" id="g-bar"></div></div></div>`,
      after(el, { kit, track, later }) {
        const button = el.querySelector('#g-bar-try');
        const bar = el.querySelector('#g-bar');
        bar.hidden = true;
        button.addEventListener('click', () => track((async () => {
          const task = kit.wkTask({ control: button, words: 'Copying files', bar });
          await later(1200);
          for (let n = 1; n <= 10; n++) { task.progress({ done: n, total: 10, unit: 'files' }); await later(250); }
          await task.done('Copied 10 files');
        })()));
      },
    }],
  },
  {
    id: 'task', title: 'Task lifecycle', group: 'status',
    intro: 'Every state of BEHAVIOUR.md\'s action lifecycle, run by <code>wkTask</code> with its real timings on a row\'s state cell. Press a row\'s "Try it". Waiting and the timeout take 30 s and 2 min, so this page calls <code>waiting()</code> itself; each outcome goes to the announcer once.',
    examples: [{
      title: 'Try it: each state',
      wide: true,
      html: `<table class="wk-table g-tasks" id="g-tasks">
  <thead><tr><th>State</th><th>The row's state</th><th><span class="wk-visually-hidden">Actions</span></th></tr></thead>
  <tbody>
    <tr data-scenario="quick"><th scope="row">Quiet wait <span class="wk-caption">(done in 200 ms: no indicator)</span></th><td>Ready</td><td></td></tr>
    <tr data-scenario="done"><th scope="row">Working, then Done</th><td>Ready</td><td></td></tr>
    <tr data-scenario="show"><th scope="row">Done, with Show <span class="wk-caption">(the result is elsewhere)</span></th><td>Ready</td><td></td></tr>
    <tr data-scenario="measured"><th scope="row">Measured progress</th><td>Ready</td><td></td></tr>
    <tr data-scenario="long"><th scope="row">Long, with Stop</th><td>Ready</td><td></td></tr>
    <tr data-scenario="pause"><th scope="row">Paused and resumed</th><td>Ready</td><td></td></tr>
    <tr data-scenario="waiting"><th scope="row">Waiting</th><td>Ready</td><td></td></tr>
    <tr data-scenario="stalled"><th scope="row">Stalled</th><td>Ready</td><td></td></tr>
    <tr data-scenario="queued"><th scope="row">Queued</th><td>Ready</td><td></td></tr>
    <tr data-scenario="failed"><th scope="row">Failed</th><td>Ready</td><td></td></tr>
    <tr data-scenario="cancelled"><th scope="row">Cancelled</th><td>Ready</td><td></td></tr>
    <tr data-scenario="stopped"><th scope="row">Stopped</th><td>Ready</td><td></td></tr>
    <tr data-scenario="parts"><th scope="row">Work in parts <span class="wk-caption">(one part fails)</span></th><td><span class="g-stack"><span>Reviewer 1</span><span>Reviewer 2</span><span>Reviewer 3</span></span></td><td></td></tr>
  </tbody>
</table>`,
      after(el, { kit, track, later }) {
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
            const task = kit.wkTask({ words: 'Exporting', ...specs[name], control: button, target: name === 'parts' ? undefined : cell, subject: row.querySelector('th').firstChild.textContent.trim(), onEnd: ended });
            scenarios[name](task, row);
            track(end);
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
      after(el, { kit, track, later }) {
        const summary = kit.wkTask.summary();
        el.querySelector('#g-tasks-footer').append(summary);
        const start = el.querySelector('#g-tasks-start');
        start.addEventListener('click', () => {
          const run = (spec, fn) => new Promise(resolve => { const t = kit.wkTask({ ...spec, onEnd: resolve }); fn(t); });
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
            later(600).then(() => kit.wkTask.showList({ opener: start })),
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
    id: 'skeleton', title: 'Skeleton rows', group: 'status',
    intro: 'For a table that loads: from 400 ms the body\'s rows give way to still bars at the real row height, for at least 500 ms, then the new rows swap in, in one step. The table has <code>aria-busy</code> until then.',
    examples: [{
      title: 'Try it: reload the table',
      note: 'Fast answers in 200 ms (no skeleton ever shows); Slow in 1.5 s.',
      html: `<div class="g-row">${tryButton('g-skeleton-fast', 'Reload (fast)')}${tryButton('g-skeleton-slow', 'Reload (slow)')}</div>
<table class="wk-table" id="g-skeleton-table">
  <thead><tr><th>Name</th><th>State</th><th class="wk-num">Runs</th></tr></thead>
  <tbody><tr><td>alpha</td><td>Running</td><td class="wk-num">19</td></tr><tr><td>beta</td><td>Ready</td><td class="wk-num">4</td></tr><tr><td>gamma</td><td>Offline</td><td class="wk-num">0</td></tr></tbody>
</table>`,
      after(el, { kit, track, later }) {
        const body = el.querySelector('tbody');
        let n = 0;
        const draw = () => {
          n++;
          body.innerHTML = ['alpha', 'beta', 'gamma'].map((name, i) => `<tr><td>${name}</td><td>${['Running', 'Ready', 'Offline'][i]}</td><td class="wk-num">${(i + 1) * 3 + n}</td></tr>`).join('');
        };
        const reload = ms => () => track((async () => {
          const loading = kit.wkSkeleton(body);
          await later(ms);
          await loading.done(draw);
        })());
        el.querySelector('#g-skeleton-fast').addEventListener('click', reload(200));
        el.querySelector('#g-skeleton-slow').addEventListener('click', reload(1500));
      },
    }],
  },
];
