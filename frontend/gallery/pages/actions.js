// Actions: buttons, links, busy controls, copy buttons and menus.
import { icons, tryButton } from './shared.js';

export default [
  {
    id: 'button', title: 'Button', group: 'actions', card: 'Button',
    intro: 'A native <code>&lt;button&gt;</code> that does one thing now. Hover and pressed states need a pointer: hover any button to see its tint, and press Tab for the focus ring.',
    examples: [{
      title: 'Kinds and sizes',
      html: `<div class="g-row">
  <button class="wk-button" type="button">Default</button>
  <button class="wk-button wk-button--primary" type="button">Primary</button>
  <button class="wk-button wk-button--destructive" type="button">Delete…</button>
  <button class="wk-button wk-button--quiet" type="button">Quiet</button>
  <button class="wk-button wk-button--small" type="button">Small</button>
  <button class="wk-button wk-button--icon wk-button--quiet" type="button" aria-label="Toggle sidebar">${icons.panel}</button>
  <a class="wk-button" href="#/button">A link styled as a button</a>
</div>`,
    }, {
      title: 'Disabled',
      html: `<div class="g-row">
  <button class="wk-button" type="button" disabled>Disabled</button>
  <button class="wk-button wk-button--primary" type="button" disabled>Primary disabled</button>
  <button class="wk-button wk-button--destructive" type="button" disabled>Destructive disabled</button>
</div>`,
    }, {
      title: 'Locked',
      note: 'wk-button--locked with aria-disabled: still focusable, so its tooltip is reachable.',
      html: `<div class="g-row">
  <button class="wk-button wk-button--locked" type="button" aria-disabled="true" title="Needs manage access">${icons.lock}Invite</button>
  <button class="wk-button wk-button--small wk-button--locked" type="button" aria-disabled="true" title="Needs manage access">${icons.lock}Stop…</button>
</div>`,
    }],
  },
  {
    id: 'link', title: 'Link', group: 'actions', card: 'Link',
    intro: 'For navigation, and for an action that reads as text. An <code>aria-expanded</code> link draws a disclosure chevron.',
    examples: [{
      title: 'Links and link buttons',
      html: `<div class="g-row">
  <a class="wk-link" href="#/link">A link</a>
  <button class="wk-link" type="button">A link-styled button</button>
  <button class="wk-link" type="button" disabled>Disabled link button</button>
</div>`,
    }, {
      title: 'Disclosure',
      note: 'A link that shows a section in place: closed, and open. Press either.',
      html: `<div class="g-stack">
  <button class="wk-link" type="button" aria-expanded="false" aria-controls="g-more">Advanced</button>
  <p id="g-more" class="wk-caption" hidden>The section it shows.</p>
  <button class="wk-link" type="button" aria-expanded="true" aria-controls="g-more-open">Details</button>
  <p id="g-more-open" class="wk-caption">The section it shows, open.</p>
</div>`,
      after(el) {
        for (const link of el.querySelectorAll('[aria-controls]')) {
          link.addEventListener('click', () => {
            const open = link.getAttribute('aria-expanded') !== 'true';
            link.setAttribute('aria-expanded', String(open));
            document.getElementById(link.getAttribute('aria-controls')).hidden = !open;
          });
        }
      },
    }],
  },
  {
    id: 'busy', title: 'Busy state', group: 'actions',
    intro: 'A busy control (<code>aria-disabled="true"</code> and <code>data-busy</code>) keeps its pressed face, its focus and its name; a second press does nothing. After 400 ms a spinner takes the label\'s place and the width holds. <code>wkTask</code> runs it.',
    examples: [{
      title: 'Busy, with and without the spinner',
      html: `<div class="g-row" id="g-busy">
  <button class="wk-button" type="button" aria-disabled="true" data-busy>Pressed</button>
  <button class="wk-button" type="button" aria-disabled="true" data-busy><span class="wk-spinner" aria-hidden="true"></span>Export</button>
  <button class="wk-button wk-button--primary" type="button" aria-disabled="true" data-busy><span class="wk-spinner" aria-hidden="true"></span>Keep running</button>
  <button class="wk-button wk-button--small" type="button" aria-disabled="true" data-busy><span class="wk-spinner" aria-hidden="true"></span>Check</button>
  <button class="wk-button wk-button--quiet" type="button" aria-disabled="true" data-busy><span class="wk-spinner" aria-hidden="true"></span>Refresh</button>
</div>`,
    }, {
      title: 'Try it: the lifecycle on a button',
      note: 'Quick ends in 200 ms, inside the quiet wait: no spinner ever shows. Slow is busy at once, shows the spinner at 400 ms and is done after 2 s. Failing fails after 1.5 s. The outcome goes to the announcer and appears beside.',
      html: `<div class="g-row">
  ${tryButton('g-busy-quick', 'Quick')}
  ${tryButton('g-busy-slow', 'Slow')}
  ${tryButton('g-busy-fail', 'Failing')}
  <span class="wk-caption" id="g-busy-words" aria-hidden="true"></span>
</div>`,
      after(el, { task: wkTask, track, later }) {
        const words = el.querySelector('#g-busy-words');
        const run = (button, ms, ok) => button.addEventListener('click', () => {
          words.textContent = '';
          const task = wkTask({ control: button, words: 'Exporting' });
          track(later(ms).then(() => (ok
            ? task.done('Exported').then(() => { words.textContent = 'Exported'; })
            : task.fail({ words: 'Couldn’t export', error: new Error('disk full') }).then(() => { words.textContent = 'Couldn’t export'; }))));
        });
        run(el.querySelector('#g-busy-quick'), 200, true);
        run(el.querySelector('#g-busy-slow'), 2000, true);
        run(el.querySelector('#g-busy-fail'), 1500, false);
      },
    }],
  },
  {
    id: 'copy', title: 'Copy button', group: 'actions', card: 'CodeBlock',
    intro: 'A <code>wk-copy</code> copies by itself: <code>data-wk-copy</code>\'s text, the element <code>data-wk-copy-from</code> names, or inside a <code>wk-code</code> the code beside it. "Copied" shows for 2 s above it (<code>wk-copy-tip</code>) and goes to the announcer; a failed copy says "Couldn’t copy".',
    examples: [{
      title: 'Try it: copy',
      html: `<div class="g-row">
  <button class="wk-copy" type="button" aria-label="Copy join code" title="Copy" data-wk-copy="K7-42" data-gallery-try>${icons.copy}</button>
  <span class="wk-caption">data-wk-copy="K7-42"</span>
</div>
<div class="g-row">
  <input class="wk-field" id="g-token" value="tok_example_123" aria-label="Token" readonly>
  <button class="wk-copy" type="button" aria-label="Copy the token" title="Copy" data-wk-copy-from="g-token" data-gallery-try>${icons.copy}</button>
</div>`,
    }, {
      title: 'Try it: a copy that fails',
      note: 'The app replaces wkCopy.write; here, with a writer that refuses.',
      html: `<button class="wk-copy" type="button" aria-label="Copy, failing" title="Copy" id="g-copy-fail" data-gallery-try>${icons.copy}</button>`,
      after(el, { kit, track }) {
        const button = el.querySelector('#g-copy-fail');
        button.addEventListener('click', e => {
          e.preventDefault();
          const write = kit.wkCopy.write;
          kit.wkCopy.write = () => Promise.resolve(false);
          track(kit.wkCopy(button, 'never copied').finally(() => { kit.wkCopy.write = write; }));
        });
      },
    }],
  },
  {
    id: 'menu', title: 'Menu', group: 'actions', card: 'Menu',
    intro: '<code>&lt;wk-menu&gt;</code> is a menu button; <code>&lt;wk-context-menu&gt;</code> opens the same menu on right-click, Shift+F10 or the Menu key; <code>wk-menu--inline</code> lays the items out in place. Each fires <code>wk-select</code>.',
    examples: [{
      title: 'Menu button and context menu, beside a tooltip',
      note: 'With a menu open, the first Esc closes the menu, even while the pointer shows the tooltip.',
      html: `<div class="g-row">
  <wk-menu id="g-menu">
    <button class="wk-button" type="button">Actions</button>
    <div role="menu" class="wk-menu" hidden>
      <p class="wk-menu-heading" role="presentation">This file</p>
      <button role="menuitem" type="button" data-value="copy">Copy details<span class="wk-menu-shortcut">Ctrl+C</span></button>
      <button role="menuitem" type="button" data-value="open">Open in browser</button>
      <button role="menuitem" type="button" aria-disabled="true" data-value="pause">Pause (disabled)</button>
      <div role="separator"></div>
      <button role="menuitemcheckbox" type="button" aria-checked="true" data-value="compact">Compact rows</button>
      <div role="separator"></div>
      <button role="menuitem" type="button" data-value="remove">Remove…</button>
    </div>
  </wk-menu>
  <div class="g-target" id="g-context-target" tabindex="0">Right-click here, or focus it and press Shift+F10</div>
  <wk-context-menu for="g-context-target" id="g-context">
    <div role="menu" class="wk-menu" aria-label="Item actions" hidden>
      <button role="menuitem" type="button" data-value="rename">Rename</button>
      <button role="menuitem" type="button" data-value="duplicate">Duplicate</button>
      <div role="separator"></div>
      <button role="menuitemradio" type="button" aria-checked="true" data-value="small">Small</button>
      <button role="menuitemradio" type="button" aria-checked="false" data-value="large">Large</button>
    </div>
  </wk-context-menu>
  <wk-tooltip id="g-menu-tip"><button class="wk-button" type="button">Hover or focus me</button><span role="tooltip" class="wk-tooltip" hidden>Copies the code to the clipboard</span></wk-tooltip>
</div>
<pre class="g-log" id="g-menu-log" aria-label="Menu events" tabindex="0"></pre>`,
      after(el) {
        const log = el.querySelector('#g-menu-log');
        const note = e => { log.textContent = `${e.type} on #${e.target.id}: ${e.detail?.value ?? (e.detail?.open ? 'open' : 'closed')}\n${log.textContent}`.split('\n').slice(0, 4).join('\n'); };
        el.addEventListener('wk-select', note);
        el.addEventListener('wk-toggle', note);
      },
    }, {
      title: 'Open, as it looks',
      note: 'A static copy, with the first item highlighted.',
      html: `<div class="g-row g-static" aria-hidden="true">
  <div class="wk-menu g-in-flow">
    <div role="menuitem" class="g-hl">Copy details<span class="wk-menu-shortcut">Ctrl+C</span></div>
    <div role="menuitem">Open in browser</div>
    <div role="menuitem" aria-disabled="true">Pause (disabled)</div>
    <div role="separator"></div>
    <div role="menuitemcheckbox" aria-checked="true">Compact rows</div>
  </div>
</div>`,
    }, {
      title: 'Inline menu',
      note: 'wk-menu--inline in a &lt;wk-menu&gt; with no button, in a popover surface: the same keys in place, one tab stop. The first item is a link.',
      html: `<div class="wk-popover g-inline-menu">
  <wk-menu id="g-inline">
    <div role="menu" class="wk-menu wk-menu--inline" aria-label="App">
      <a role="menuitem" href="#g-inline" data-value="help">Help<span class="wk-menu-shortcut">F1</span></a>
      <button role="menuitem" type="button" data-value="settings">Settings…<span class="wk-menu-shortcut">Ctrl+,</span></button>
      <button role="menuitem" type="button" aria-disabled="true" data-value="update">Check for updates (disabled)</button>
      <div role="separator"></div>
      <button role="menuitem" type="button" data-value="quit">Quit<span class="wk-menu-shortcut">Ctrl+Q</span></button>
    </div>
  </wk-menu>
</div>`,
    }],
  },
];
