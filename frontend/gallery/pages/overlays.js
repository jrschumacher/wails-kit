// Overlays: dialogs and sheets, the popover surface and the tooltip.
import { icons, statGlyphs } from './shared.js';

export default [
  {
    id: 'dialog', title: 'Dialog, sheet and alert', group: 'overlays', card: 'Sheet',
    intro: '<code>&lt;wk-dialog&gt;</code> opens a native <code>&lt;dialog&gt;</code> modally; focus returns to the opener. <code>data-wk-open</code> opens one, <code>data-wk-close</code> buttons set its return value, and <code>light-dismiss</code> lets a click outside close a dialog that asks nothing.',
    examples: [{
      title: 'Open each',
      html: `<div class="g-row">
  <button class="wk-button" type="button" id="g-dialog-open" data-wk-open="g-dialog">Open dialog</button>
  <button class="wk-button" type="button" id="g-sheet-open" data-wk-open="g-sheet">Open sheet</button>
  <button class="wk-button" type="button" id="g-long-sheet-open" data-wk-open="g-long-sheet">Open long sheet</button>
  <button class="wk-button" type="button" id="g-alert-open" data-wk-open="g-alert">Open alert</button>
  <button class="wk-button" type="button" id="g-info-open" data-wk-open="g-info">Open notice</button>
</div>
<wk-dialog id="g-alert">
  <dialog class="wk-dialog wk-dialog--alert" role="alertdialog" aria-labelledby="g-alert-title" aria-describedby="g-alert-body">
    ${icons.warn}
    <h3 class="wk-dialog-title" id="g-alert-title">Stop this job?</h3>
    <p class="wk-dialog-body" id="g-alert-body">It will fail. You can run it again.</p>
    <div class="wk-dialog-actions">
      <button class="wk-button wk-button--destructive" type="button" value="stop" data-wk-close>Stop job</button>
      <button class="wk-button wk-button--primary" type="button" value="keep" data-wk-close autofocus>Keep running</button>
    </div>
  </dialog>
</wk-dialog>
<wk-dialog id="g-dialog">
  <dialog class="wk-dialog" aria-labelledby="g-dialog-title">
    <h3 class="wk-dialog-title" id="g-dialog-title">Delete “summary.pdf”?</h3>
    <p class="wk-dialog-body">It is deleted for everyone who can open it. This can’t be undone.</p>
    <div class="wk-dialog-actions">
      <button class="wk-button" type="button" value="cancel" data-wk-close autofocus>Cancel</button>
      <button class="wk-button wk-button--destructive" type="button" value="remove" data-wk-close>Delete</button>
    </div>
  </dialog>
</wk-dialog>
<wk-dialog id="g-sheet">
  <dialog class="wk-dialog wk-dialog--sheet" aria-labelledby="g-sheet-title">
    <h3 class="wk-dialog-title" id="g-sheet-title">Rename file</h3>
    <p class="wk-dialog-body">The new name is shown to everyone who can open it.</p>
    <p class="wk-dialog-body"><label class="wk-label" for="g-sheet-name">Name</label><input id="g-sheet-name" class="wk-field g-wide" value="summary.pdf"></p>
    <div class="wk-dialog-actions">
      <button class="wk-button wk-dialog-start" type="button" value="help" data-wk-close>Help</button>
      <button class="wk-button" type="button" value="cancel" data-wk-close>Cancel</button>
      <button class="wk-button wk-button--primary" type="button" value="rename" data-wk-close>Rename</button>
    </div>
  </dialog>
</wk-dialog>
<wk-dialog id="g-long-sheet">
  <dialog class="wk-dialog wk-dialog--sheet" aria-labelledby="g-long-sheet-title">
    <header class="wk-dialog-header"><h3 class="wk-dialog-title" id="g-long-sheet-title">Details</h3></header>
    <div class="wk-dialog-scroll" tabindex="0" role="region" aria-labelledby="g-long-sheet-title">
      <h4 class="wk-section-heading">Now</h4>
      <div class="wk-stats-grid">
        <div class="wk-stat wk-stat--tile"><span class="wk-stat-glyph" aria-hidden="true">${statGlyphs.grid}</span><b class="wk-stat-figure">1 of 2</b><span class="wk-stat-label">items in use</span></div>
        <div class="wk-stat wk-stat--tile"><span class="wk-stat-glyph" aria-hidden="true">${statGlyphs.time}</span><b class="wk-stat-figure">12 min</b><span class="wk-stat-label">since the last change</span></div>
      </div>
      <p class="wk-dialog-indent">Plain text under a heading sits behind a rule (wk-dialog-indent).</p>
      ${Array.from({ length: 5 }, (_, i) => `<h4 class="wk-section-heading">Section ${i + 1}</h4><p class="wk-dialog-indent">Only the middle scrolls; the header and the footer bar stay put.</p>`).join('\n      ')}
    </div>
    <footer class="wk-dialog-footer"><button class="wk-button wk-button--primary" type="button" value="done" data-wk-close autofocus>Done</button></footer>
  </dialog>
</wk-dialog>
<wk-dialog id="g-info" light-dismiss>
  <dialog class="wk-dialog" aria-labelledby="g-info-title">
    <h3 class="wk-dialog-title" id="g-info-title">About this page</h3>
    <p class="wk-dialog-body">A notice that asks nothing, so a click outside it closes it (light-dismiss).</p>
    <div class="wk-dialog-actions"><button class="wk-button" type="button" data-wk-close>Close</button></div>
  </dialog>
</wk-dialog>
<pre class="g-log" id="g-dialog-log" aria-label="Dialog events" tabindex="0"></pre>`,
      after(el) {
        const log = el.querySelector('#g-dialog-log');
        const note = e => { log.textContent = `${e.type} on #${e.target.id}${e.detail?.returnValue ? `: ${e.detail.returnValue}` : ''}\n${log.textContent}`.split('\n').slice(0, 4).join('\n'); };
        el.addEventListener('wk-open', note);
        el.addEventListener('wk-close', note);
      },
    }, {
      title: 'Open, as they look',
      note: 'Static copies of the dialog and the alert surfaces.',
      html: `<div class="g-row g-static" aria-hidden="true">
  <div class="wk-dialog g-in-flow">
    <h3 class="wk-dialog-title">Dialog surface</h3>
    <p class="wk-dialog-body">A static copy, to show it open.</p>
    <div class="wk-dialog-actions"><span class="wk-button">Cancel</span><span class="wk-button wk-button--primary">Continue</span></div>
  </div>
  <div class="wk-dialog wk-dialog--alert g-in-flow">
    ${icons.warn}
    <h3 class="wk-dialog-title">Stop this job?</h3>
    <p class="wk-dialog-body">It will fail. You can run it again.</p>
    <div class="wk-dialog-actions"><span class="wk-button wk-button--destructive">Stop job</span><span class="wk-button wk-button--primary">Keep running</span></div>
  </div>
</div>`,
    }],
  },
  {
    id: 'popover', title: 'Popover surface', group: 'overlays', card: 'Popover',
    intro: 'The surface of a tray window or a panel that drops from a button: header, sections, footer.',
    examples: [{
      title: 'A tray popover',
      html: `<div class="wk-popover g-popover">
  <div class="wk-popover-header"><span class="wk-dot" data-shape="ringed" data-tone="working" aria-hidden="true"></span><b>Syncing</b><span class="wk-toolbar-spacer"></span><span class="wk-caption">3 of 5</span></div>
  <div class="wk-popover-section">A popover surface for a tray window: header, sections, footer.</div>
  <div class="wk-popover-section wk-caption">Second section</div>
  <div class="wk-popover-footer"><a class="wk-link" href="#/popover">Details</a><button class="wk-button wk-button--small" type="button">Pause</button></div>
</div>`,
    }],
  },
  {
    id: 'tooltip', title: 'Tooltip', group: 'overlays',
    intro: '<code>&lt;wk-tooltip&gt;</code> shows on focus, and after a hover delay; Esc hides it. It fires <code>wk-toggle</code>.',
    examples: [{
      title: 'Hover or focus',
      html: `<div class="g-row">
  <wk-tooltip id="g-tooltip"><button class="wk-button" type="button">Hover or focus me</button><span role="tooltip" class="wk-tooltip" hidden>Copies the code to the clipboard</span></wk-tooltip>
</div>`,
    }, {
      title: 'Shown, as it looks',
      html: '<div class="g-row g-static" aria-hidden="true"><span class="wk-tooltip g-in-flow">Copies the code to the clipboard</span></div>',
    }],
  },
];
