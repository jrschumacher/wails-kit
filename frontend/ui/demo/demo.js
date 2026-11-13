// Writes the kit's review page: every component in each state it can show
// without a pointer, the type scale, the tokens and every status dot. Used by
// index.html (the neutral kit, from src/), themed.html (with
// theme/example.css) and dist.html (the prebuilt dist/ files). The body's
// data-module names the module the page loaded, so the charts come from it.
const { wkChart } = await import(document.body.dataset.module || '../src/index.js');

const themed = document.body.dataset.demo === 'themed';
const pages = [['index.html', 'Neutral'], ['themed.html', 'Example theme'], ['dist.html', 'Prebuilt dist'], ['settings.html', 'Settings window']];
const here = location.pathname.split('/').pop() || 'index.html';
const other = pages.filter(([href]) => href !== here).map(([href, text]) => `<a class="wk-link" href="${href}">${text}</a>`).join('');

const shapes = ['filled', 'ringed', 'half', 'hollow', 'dashed', 'bar'];
const tones = ['neutral', 'working', 'attention'];
const dots = `<table class="wk-table demo-dots">
  <thead><tr><th>Shape</th>${tones.map(t => `<th>${t}</th>`).join('')}</tr></thead>
  <tbody>${shapes.map(s => `<tr><th scope="row">${s}</th>${tones.map(t =>
    `<td><span class="wk-dot" data-size="sm" data-shape="${s}" data-tone="${t}" aria-hidden="true"></span>
     <span class="wk-dot" data-shape="${s}" data-tone="${t}" role="img" aria-label="${s} ${t}"></span>
     <span class="wk-dot demo-dot-large" data-shape="${s}" data-tone="${t}" aria-hidden="true"></span></td>`).join('')}</tr>`).join('')}</tbody>
</table>`;

const icon = path => `<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">${path}</svg>`;
const icons = {
  now: icon('<path d="M2.5 11a5.5 5.5 0 0 1 11 0"/><path d="M8 11l2.5-3"/>'),
  history: icon('<path d="M3 13V8M8 13V3M13 13V6"/>'),
  items: icon('<rect x="2.5" y="3" width="11" height="4" rx="1"/><rect x="2.5" y="9" width="11" height="4" rx="1"/>'),
  book: icon('<path d="M4 2.5h8v11l-4-2.5-4 2.5z"/>'),
  panel: icon('<rect x="2" y="3" width="12" height="10" rx="1.5"/><path d="M6 3v10"/>'),
  lock: icon('<rect x="3" y="7" width="10" height="7" rx="1.3"/><path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2"/>'),
  eye: icon('<path d="M1.5 8S4 3.5 8 3.5 14.5 8 14.5 8 12 12.5 8 12.5 1.5 8 1.5 8z"/><circle cx="8" cy="8" r="2"/>'),
  copy: icon('<rect x="5.5" y="5.5" width="8" height="8" rx="1.6"/><path d="M10.5 3.5v-.2a1.3 1.3 0 0 0-1.3-1.3H3.8a1.3 1.3 0 0 0-1.3 1.3v5.4a1.3 1.3 0 0 0 1.3 1.3h.2"/>'),
  warn: icon('<circle cx="8" cy="8" r="6.3"/><path d="M8 4.8v4M8 11v.3"/>'),
};
// Stat tile glyphs: 18 px, drawn in currentColor.
const statGlyph = path => `<svg viewBox="0 0 18 18" aria-hidden="true"><g fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">${path}</g></svg>`;
const statGlyphs = {
  grid: statGlyph('<rect x="3" y="3" width="5" height="5" rx="1" fill="currentColor"/><rect x="10" y="3" width="5" height="5" rx="1"/><rect x="3" y="10" width="5" height="5" rx="1"/><rect x="10" y="10" width="5" height="5" rx="1"/>'),
  ok: statGlyph('<path d="M9 2.5 14.5 4.5V9c0 3.2-2.3 5.4-5.5 6.5C5.8 14.4 3.5 12.2 3.5 9V4.5z"/><path d="M6.6 9l1.8 1.8 3.2-3.4"/>'),
  time: statGlyph('<circle cx="9" cy="9" r="6.5"/><path d="M9 5.5V9l2.5 1.5"/>'),
};
const glyph = path => `<svg viewBox="0 0 22 22" aria-hidden="true"><g fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">${path}</g></svg>`;
const glyphs = {
  general: glyph('<circle cx="11" cy="11" r="3"/><path d="M11 2.5v2.5M11 17v2.5M2.5 11H5M17 11h2.5M5 5l1.8 1.8M15.2 15.2 17 17M5 17l1.8-1.8M15.2 6.8 17 5"/>'),
  sharing: glyph('<rect x="4" y="4.5" width="14" height="10" rx="1.3"/><path d="M2 17.5h18"/>'),
  about: glyph('<circle cx="11" cy="11" r="8.5"/><path d="M11 10v5.5M11 6.8v.2"/>'),
};
const stepper = (id, label, value, more = true, fewer = true, text = false) => `<span class="wk-stepper"><input id="${id}" class="wk-field" ${text ? '' : 'type="number" '}inputmode="numeric" min="1" max="8" value="${value}" aria-label="${label}"><span class="wk-stepper-arrows"><button type="button" aria-label="More"${more ? '' : ' disabled'}></button><button type="button" aria-label="Fewer"${fewer ? '' : ' disabled'}></button></span></span>`;

const section = (id, title, body, note = '') => `<section class="demo-section" id="${id}" aria-labelledby="${id}-h">
  <h2 class="wk-heading" id="${id}-h">${title}</h2>${note ? `<p class="wk-caption demo-note">${note}</p>` : ''}${body}</section>`;

document.getElementById('demo').innerHTML = `
<header class="demo-head">
  <div><h1 class="wk-title">Kit</h1>
  <p class="wk-caption">${themed ? 'With the example theme (theme/example.css).' : 'Neutral: the kit\'s stylesheet only, no theme.'} Follows the system appearance unless an appearance is picked here (it sets &lt;html data-theme&gt;).</p></div>
  <nav class="demo-nav demo-row">${other}
    <div class="wk-segmented" role="radiogroup" aria-label="Appearance" id="demo-theme">
      <label><input type="radio" name="demo-theme" value="" checked><span>System</span></label>
      <label><input type="radio" name="demo-theme" value="light"><span>Light</span></label>
      <label><input type="radio" name="demo-theme" value="dark"><span>Dark</span></label>
    </div></nav>
</header>
<p class="wk-caption demo-note">Hover and pressed states cannot be shown on a still page. Press Tab to see the focus ring (2 px, accent); hover any control to see its hover tint.</p>

${section('type', 'Type scale', `
<div class="demo-stack">
  <p class="wk-title">Title, 24 px</p>
  <p class="wk-title-2">Title 2, 20 px</p>
  <p class="wk-heading">Heading, 15 px semibold</p>
  <p class="wk-subheading">Subheading, 13 px semibold</p>
  <p class="wk-body">Body, 13 px. The quick brown fox jumps over the lazy dog.</p>
  <p class="wk-caption">Caption, 12 px secondary</p>
  <p class="wk-small">Small, 11 px secondary</p>
  <p><span class="wk-figure">129</span> <span class="wk-secondary">figure, tabular numerals:</span> <span class="wk-num">11:02 0:41 3:18</span></p>
  <p><code>code: system monospace</code></p>
</div>`)}

${section('tokens', 'Tokens', `<div id="demo-swatches" class="demo-swatches"></div>
<dl id="demo-values" class="demo-values"></dl>`)}

${section('dots', 'Status dot', dots, 'Shapes × tones, small (data-size="sm", 8 px), at the default size and at 14 px. Tones: neutral, working, attention; the working tone draws its outline in wk-color-working-ring. No state names.')}

${section('buttons', 'Buttons and links', `
<div class="demo-row">
  <button class="wk-button" type="button">Default</button>
  <button class="wk-button wk-button--primary" type="button">Primary</button>
  <button class="wk-button wk-button--destructive" type="button">Delete…</button>
  <button class="wk-button wk-button--quiet" type="button">Quiet</button>
  <button class="wk-button wk-button--small" type="button">Small</button>
  <button class="wk-button wk-button--icon wk-button--quiet" type="button" aria-label="Toggle sidebar">${icons.panel}</button>
</div>
<div class="demo-row">
  <button class="wk-button" type="button" disabled>Disabled</button>
  <button class="wk-button wk-button--primary" type="button" disabled>Primary disabled</button>
  <button class="wk-button wk-button--destructive" type="button" disabled>Destructive disabled</button>
  <a class="wk-button" href="#buttons">A link styled as a button</a>
</div>
<div class="demo-row">
  <button class="wk-button wk-button--locked" type="button" aria-disabled="true" title="Needs manage access">${icons.lock}Invite</button>
  <button class="wk-button wk-button--small wk-button--locked" type="button" aria-disabled="true" title="Needs manage access">${icons.lock}Stop…</button>
  <span class="wk-caption">Locked (wk-button--locked with aria-disabled): focusable, so its tooltip is reachable.</span>
</div>
<div class="demo-row">
  <a class="wk-link" href="#buttons">A link</a>
  <button class="wk-link" type="button">A link-styled button</button>
  <button class="wk-link" type="button" disabled>Disabled link button</button>
</div>
<div class="demo-row">
  <button class="wk-link" type="button" aria-expanded="false" aria-controls="demo-more">Advanced</button>
  <button class="wk-link" type="button" aria-expanded="true" aria-controls="demo-more">Advanced (open)</button>
  <span class="wk-caption">A link that shows a section in place: aria-expanded draws the chevron.</span>
</div>
<p id="demo-more" class="wk-caption" hidden>The section it shows.</p>`)}

${section('fields', 'Fields', `
<div class="demo-grid">
  <div><label class="wk-label" for="f1">Name</label><input id="f1" class="wk-field demo-wide" value="Notes"><p class="wk-help">Help text under a field.</p></div>
  <div><label class="wk-label" for="f2">Placeholder</label><input id="f2" class="wk-field demo-wide" placeholder="name@example.com"></div>
  <div><label class="wk-label" for="f3">Invalid</label><input id="f3" class="wk-field demo-wide" value="not an address" aria-invalid="true" aria-describedby="f3-e"><p class="wk-error" id="f3-e">Enter an email address.</p></div>
  <div><label class="wk-label" for="f4">Disabled</label><input id="f4" class="wk-field demo-wide" value="Locked" disabled></div>
  <div><label class="wk-label" for="f5">Number</label><input id="f5" class="wk-field demo-narrow wk-num" type="number" value="2" min="1" max="8"></div>
  <div><label class="wk-label" for="f6">Search</label><div class="wk-search demo-wide"><input id="f6" class="wk-field" type="search" placeholder="Search"></div></div>
  <div class="demo-span"><label class="wk-label" for="f7">Text area</label><textarea id="f7" class="wk-field demo-wide">Several lines
of text.</textarea></div>
</div>`)}

${section('choices', 'Checkbox, radio button, switch, select, filters, segmented control', `
<div class="demo-row">
  <label class="wk-check"><input class="wk-checkbox" type="checkbox"> Off</label>
  <label class="wk-check"><input class="wk-checkbox" type="checkbox" checked> On</label>
  <label class="wk-check"><input class="wk-checkbox" type="checkbox" id="demo-mixed"> Mixed</label>
  <label class="wk-check"><input class="wk-checkbox" type="checkbox" disabled> Disabled</label>
  <label class="wk-check"><input class="wk-checkbox" type="checkbox" checked disabled> Disabled on</label>
</div>
<div class="demo-row" role="radiogroup" aria-label="Radio buttons">
  <label class="wk-check"><input class="wk-radio" type="radio" name="demo-radio"> Off</label>
  <label class="wk-check"><input class="wk-radio" type="radio" name="demo-radio" checked> On</label>
  <label class="wk-check"><input class="wk-radio" type="radio" name="demo-radio-disabled" disabled> Disabled</label>
  <label class="wk-check"><input class="wk-radio" type="radio" name="demo-radio-disabled" checked disabled> Disabled on</label>
</div>
<div class="demo-row">
  <label class="wk-check"><input class="wk-switch" type="checkbox" role="switch" id="demo-switch"> Off</label>
  <label class="wk-check"><input class="wk-switch" type="checkbox" role="switch" checked> On</label>
  <label class="wk-check"><input class="wk-switch" type="checkbox" role="switch" disabled> Disabled</label>
  <label class="wk-check"><input class="wk-switch" type="checkbox" role="switch" checked disabled> Disabled on</label>
</div>
<div class="demo-row">
  <select class="wk-select" aria-label="Range"><option>Last 14 days</option><option>Last 30 days</option></select>
  <select class="wk-select" aria-label="Disabled select" disabled><option>Disabled</option></select>
</div>
<div class="wk-filters" role="group" aria-label="Filter" id="demo-filters">
  <button class="wk-filter" type="button" aria-pressed="true">All <span class="wk-filter-count">12</span></button>
  <button class="wk-filter" type="button" aria-pressed="false">Open <span class="wk-filter-count">3</span></button>
  <button class="wk-filter" type="button" aria-pressed="false">Closed <span class="wk-filter-count">1</span></button>
  <button class="wk-filter" type="button" aria-pressed="false" disabled>Archived</button>
</div>
<div class="demo-row">
  <div class="wk-segmented" role="radiogroup" aria-label="Period" id="demo-segmented">
    <label><input type="radio" name="period" value="day" checked><span>Day</span></label>
    <label><input type="radio" name="period" value="week"><span>Week</span></label>
    <label><input type="radio" name="period" value="month"><span>Month</span></label>
    <label><input type="radio" name="period" value="year" disabled><span>Year</span></label>
  </div>
</div>`)}

${section('chips', 'Chips and pills', `
<div class="demo-row">
  <span class="wk-chip">Label</span>
  <button class="wk-chip" type="button">Button chip</button>
  <button class="wk-chip" type="button" disabled>Disabled</button>
  <span class="wk-chip wk-chip--count">6</span>
  <span class="wk-chip wk-chip--count wk-chip--attention">2</span>
  <span class="wk-chip wk-chip--lock">${icons.eye}View only</span>
  <span class="wk-chip wk-chip--lock" data-level="manage">${icons.lock}Can manage</span>
</div>`, 'A count chip\'s number uses wk-color-text-secondary-on-selected, which reaches 4.5:1 on its tint and on a selected row.')}

${section('table', 'Table', `
<table class="wk-table">
  <thead><tr><th>Name</th><th>Kind</th><th>Status</th><th class="wk-num">Size</th><th><span class="demo-sr">Actions</span></th></tr></thead>
  <tbody>
    <tr><td><span class="wk-secondary">reports/</span>summary.pdf</td><td>PDF document</td><td><span class="demo-state"><span class="wk-dot" data-shape="ringed" data-tone="working" aria-hidden="true"></span>Uploading</span></td><td class="wk-num">1.2 MB</td><td><a class="wk-link" href="#table">Open</a></td></tr>
    <tr aria-current="true"><td><span class="wk-secondary">reports/</span>figures.csv</td><td>Spreadsheet</td><td><span class="demo-state"><span class="wk-dot" aria-hidden="true"></span>Saved</span></td><td class="wk-num">48 KB</td><td><a class="wk-link" href="#table">Open</a></td></tr>
    <tr><td><span class="wk-secondary">reports/</span>draft.txt</td><td>Text</td><td class="wk-secondary">Queued</td><td class="wk-num wk-secondary">2 KB</td><td></td></tr>
  </tbody>
</table>`, 'The second row is the current one (aria-current="true"); aria-selected="true" looks the same inside a grid.')}

${section('layout', 'Three-pane layout: sidebar, toolbar, list, inspector', `
<div class="wk-three-pane demo-frame">
  <nav class="wk-sidebar" aria-label="Sections">
    <ul>
      <li><a class="wk-sidebar-item" href="#layout" aria-current="page">${icons.now}<span class="wk-sidebar-label">Now</span></a></li>
      <li><a class="wk-sidebar-item" href="#layout">${icons.history}<span class="wk-sidebar-label">History</span></a></li>
      <li><details open><summary class="wk-sidebar-item">${icons.items}<span class="wk-sidebar-label">Items</span><span class="wk-chip wk-chip--count">2</span></summary>
        <ul>
          <li><a class="wk-sidebar-item" href="#layout"><span class="wk-dot" data-tone="working" aria-hidden="true"></span><span class="wk-sidebar-label">alpha</span></a></li>
          <li><a class="wk-sidebar-item" href="#layout"><span class="wk-dot" data-shape="dashed" data-tone="neutral" aria-hidden="true"></span><span class="wk-sidebar-label">beta</span></a></li>
        </ul></details></li>
      <li><a class="wk-sidebar-item" href="#layout">${icons.book}<span class="wk-sidebar-label">Sources</span><span class="wk-chip wk-chip--count wk-chip--attention">1</span></a></li>
    </ul>
    <p class="wk-sidebar-heading">Group heading</p>
    <ul><li><a class="wk-sidebar-item" href="#layout"><span class="wk-dot" data-shape="half" aria-hidden="true"></span><span class="wk-sidebar-label">gamma</span></a></li></ul>
    <div class="wk-sidebar-footer">server.example</div>
  </nav>
  <section class="wk-pane-list" aria-label="List">
    <header class="wk-toolbar">
      <button class="wk-button wk-button--quiet wk-button--icon" type="button" aria-label="Toggle sidebar">${icons.panel}</button>
      <h3 class="wk-toolbar-title">Now</h3>
      <span class="wk-toolbar-spacer"></span>
      <div class="wk-search"><input class="wk-field demo-search" type="search" placeholder="Search" aria-label="Search"></div>
      <span class="wk-chip">Read only</span>
      <button class="wk-button" type="button">New</button>
    </header>
    <table class="wk-table">
      <thead><tr><th>Name</th><th>State</th><th class="wk-num">Items</th></tr></thead>
      <tbody>
        <tr><td><b>alpha</b></td><td><span class="demo-state"><span class="wk-dot" data-shape="ringed" data-tone="working" aria-hidden="true"></span>Working</span></td><td class="wk-num">19</td></tr>
        <tr aria-current="true"><td><b>beta</b></td><td><span class="demo-state"><span class="wk-dot" aria-hidden="true"></span>Ready</span></td><td class="wk-num">4</td></tr>
        <tr><td><b>gamma</b></td><td><span class="demo-state"><span class="wk-dot" data-tone="attention" aria-hidden="true"></span>Needs you</span></td><td class="wk-num">0</td></tr>
      </tbody>
    </table>
  </section>
  <aside class="wk-pane-inspector" aria-label="Inspector">
    <h3 class="wk-heading">beta</h3>
    <p class="wk-caption">Ready since 1:32 pm</p>
    <h4 class="wk-settings-heading">Details</h4>
    <div class="wk-settings-group">
      <div class="wk-settings-row"><span class="wk-settings-label">Members</span><span class="wk-num">2</span></div>
      <div class="wk-settings-row"><span class="wk-settings-label">Created</span><span>14 Mar</span></div>
    </div>
  </aside>
</div>`)}

${section('settings', 'Settings group and rows', `
<div class="demo-settings">
  <h3 class="wk-settings-heading">General</h3>
  <div class="wk-settings-group">
    <div class="wk-settings-row"><label class="wk-settings-label" for="s1">Open at login<span class="wk-settings-help">Starts in the background when you sign in.</span></label><input id="s1" class="wk-switch" type="checkbox" role="switch" checked></div>
    <div class="wk-settings-row"><label class="wk-settings-label" for="s1b">Show a notice when a download fails</label><input id="s1b" class="wk-checkbox" type="checkbox"></div>
    <div class="wk-settings-row"><label class="wk-check wk-check--row"><span class="wk-settings-label">Copies<small class="wk-settings-help">A row that is one label (wk-check--row): the whole row is the hit area.</small></span>${stepper('s2', 'Copies', 2)}</label></div>
    <div class="wk-settings-row"><label class="wk-check wk-check--row"><span>Keep a backup</span><input class="wk-switch" type="checkbox" role="switch"></label></div>
    <div class="wk-settings-row"><label class="wk-settings-label" for="s3">Check for updates</label><select id="s3" class="wk-select"><option>Daily</option><option>Weekly</option></select></div>
    <div class="wk-settings-row"><span class="wk-settings-label">Reset all settings<span class="wk-settings-help">Puts every setting back to its default.</span></span><button class="wk-button wk-button--destructive" type="button">Reset…</button></div>
  </div>
  <p class="wk-settings-note">A note under a group.</p>
</div>`)}

${section('settings-tabs', 'Settings tabs', `
<div class="wk-settings-tabs demo-settings" role="tablist" aria-label="Settings" id="demo-settings-tabs">
  <button class="wk-settings-tab" role="tab" type="button" aria-selected="true">${glyphs.general}General</button>
  <button class="wk-settings-tab" role="tab" type="button" aria-selected="false" tabindex="-1" data-attention aria-describedby="demo-attention-why">${glyphs.sharing}Sharing</button>
  <button class="wk-settings-tab" role="tab" type="button" aria-selected="false" tabindex="-1">${glyphs.about}About</button>
</div>
<p class="wk-caption demo-note" id="demo-attention-why">Sharing carries data-attention: a field there needs review.</p>`)}

${section('panels', 'Panel', `
<div class="demo-grid">
  <section class="wk-panel" aria-label="Default panel">
    <div class="wk-panel-row"><span>Copies</span>${stepper('demo-panel-slots', 'Copies', 2)}</div>
    <p class="wk-panel-hint">Keeps 2 copies of each file.</p>
    <div class="wk-panel-row"><span>Remove from the team</span><button class="wk-button wk-button--destructive" type="button">Remove…</button></div>
  </section>
  <section class="wk-panel wk-panel--dashed" aria-label="Dashed panel">
    <span class="wk-secondary">Address</span><strong class="wk-mono">server.example:8443</strong>
    <p class="wk-panel-hint">Something to check rather than change.</p>
  </section>
  <section class="wk-panel wk-panel--rows" aria-label="Panel with rows" id="demo-panel-rows">
    <div class="wk-settings-row"><span class="wk-settings-label">wk-panel wk-panel--rows</span><input class="wk-switch" type="checkbox" role="switch" checked aria-label="On"></div>
    <div class="wk-settings-row"><span class="wk-settings-label">Rows separated by hairlines</span><input class="wk-checkbox" type="checkbox" aria-label="Off"></div>
  </section>
  <section class="wk-settings-group" aria-label="Settings group" id="demo-settings-group">
    <div class="wk-settings-row"><span class="wk-settings-label">wk-settings-group (older name)</span><input class="wk-switch" type="checkbox" role="switch" checked aria-label="On"></div>
    <div class="wk-settings-row"><span class="wk-settings-label">Draws the same box</span><input class="wk-checkbox" type="checkbox" aria-label="Off"></div>
  </section>
</div>`)}

${section('notices', 'Notice', `
<div class="demo-stack">
  <div class="wk-notice" role="status">${icons.warn}
    <div class="wk-notice-body"><b>Jobs for Linux ARM64 are waiting.</b><p>The one machine that takes them is in use. They start when it is free.</p></div>
  </div>
  <div class="wk-notice" data-tone="attention" role="alert">${icons.warn}
    <div class="wk-notice-body"><b>The server can't reach the network.</b><p>No job can start. Trying again at 2:18 pm.</p></div>
    <div class="wk-notice-actions"><button class="wk-button wk-button--primary" type="button">Try now</button><a class="wk-link" href="#notices">Details</a></div>
  </div>
  <div class="wk-notice wk-notice--inline"><div class="wk-notice-body">Settings saved. One needs a restart.</div>
    <div class="wk-notice-actions"><button class="wk-button wk-button--small" type="button">Review settings</button></div></div>
</div>`, 'Neutral (default) and attention (data-tone="attention", wk-color-attention-fill), each with or without actions; inline for one line.')}

${section('empty', 'Empty state and dot label', `
<div class="demo-row"><span class="wk-dot-label"><span class="wk-dot" data-size="sm" aria-hidden="true"></span><span>Connected</span></span>
  <span class="wk-dot-label"><span class="wk-dot" data-size="sm" data-shape="hollow" aria-hidden="true"></span><span>Reconnecting</span></span></div>
<div class="wk-empty demo-frame">
  <h3 class="wk-empty-title">Nothing here yet</h3>
  <p>Say what is true now, and what happens next without the person.</p>
  <button class="wk-button" type="button">One action</button>
</div>`, 'The empty state is a title, one line and at most one button. A dot label is a small dot and its words, as one line.')}

${section('errors', 'Error box', `
<div class="demo-grid">
  <div class="wk-errorbox" role="group" aria-label="Error details">
    <div class="wk-errorbox-head"><span>Couldn't download the image</span><button class="wk-copy" type="button" aria-label="Copy error details" title="Copy">${icons.copy}</button></div>
    <dl class="wk-facts"><dt>Failed</dt><dd>5 times in a row, last at 2:13 pm</dd><dt>Next try</dt><dd>2:18 pm</dd></dl>
    <pre tabindex="0" role="region" aria-label="Error message">pull registry.example/image:2.3: Get "https://registry.example/v2/": dial tcp: lookup registry.example: no such host</pre>
  </div>
</div>`)}

${section('stats', 'Stat', `
<div class="demo-grid">
  <div class="wk-stats-grid demo-span" id="demo-stats-plain">
    <div class="wk-stat"><b class="wk-stat-figure">1 of 2</b><span class="wk-stat-label">items in use</span></div>
    <div class="wk-stat"><b class="wk-stat-figure">3<small> / 4</small></b><span class="wk-stat-label">with a unit in small</span></div>
  </div>
  <div class="wk-stats-grid demo-span" id="demo-stats-tile">
    <div class="wk-stat wk-stat--tile"><span class="wk-stat-glyph" aria-hidden="true">${statGlyphs.grid}</span><b class="wk-stat-figure">1 of 2</b><span class="wk-stat-label">items in use</span></div>
    <div class="wk-stat wk-stat--tile"><span class="wk-stat-glyph" aria-hidden="true">${statGlyphs.ok}</span><b class="wk-stat-figure">OK</b><span class="wk-stat-label">the last 12 worked</span></div>
  </div>
  <div class="wk-stats-row demo-span" id="demo-stats-row">
    <div class="wk-stat"><b class="wk-stat-figure">3<small> / 4</small></b><span class="wk-stat-label">busy</span></div>
    <div class="wk-stat"><b class="wk-stat-figure">2</b><span class="wk-stat-label">waiting · may be low</span></div>
    <div class="wk-stat"><b class="wk-stat-figure">4 min</b><span class="wk-stat-label">longest wait now</span></div>
  </div>
</div>`, 'Plain two up (wk-stats-grid); tiles with a glyph in currentColor (wk-stat--tile); a ruled row of large figures (wk-stats-row).')}

${section('section-headings', 'Section heading', `
<div class="demo-stack">
  <h3 class="wk-section-heading">Now</h3>
  <h3 class="wk-section-heading">Recent <span class="wk-section-heading-aside">last 14 days</span><a class="wk-link" href="#section-headings">All items</a></h3>
  <h3 class="wk-section-heading wk-section-heading--rule">Starts a card</h3>
</div>`, 'One style at every depth: the words, an optional aside, an optional link at the end; --rule draws a hairline above.')}

${section('code', 'Code block', `
<div class="demo-grid">
  <div class="wk-codeblock"><code>join:v1:AAAA-BBBB-CCCC-DDDD-EEEE-FFFF</code><button class="wk-copy" type="button" aria-label="Copy the code" title="Copy">${icons.copy}</button></div>
  <div class="wk-codeblock wk-codeblock--scroll"><pre tabindex="0" role="region" aria-label="Log">${Array.from({ length: 14 }, (_, i) => `12:${String(i).padStart(2, '0')}:07 step ${i + 1} finished`).join('\n')}</pre></div>
</div>`, 'One value with the copy button; lines in a pre (wk-codeblock--scroll stops at 200 px; a pre that scrolls takes focus, with a role and a name, so the keyboard can scroll it).')}

${section('steppers', 'Stepper', `
<div class="demo-row">
  <label class="wk-label" for="demo-stepper">Jobs at once</label>${stepper('demo-stepper', 'Jobs at once', 2)}
  ${stepper('demo-stepper-max', 'At the limit', 8, false)}
  ${stepper('demo-stepper-text', 'A text field', 3, true, true, true)}
  <span class="wk-stepper"><input class="wk-field" type="number" inputmode="numeric" min="1" max="8" value="0" aria-label="Invalid" aria-invalid="true"><span class="wk-stepper-arrows"><button type="button" aria-label="More"></button><button type="button" aria-label="Fewer" disabled></button></span></span>
</div>`, 'The arrows step the value between min and max (kit.js), fire input and change, and disable at a limit. The first two are type="number" (their own spinner is hidden), the third a text field, the fourth is aria-invalid.')}

${section('chart', 'Chart (wk-chart)', `<div class="demo-charts" id="demo-charts"></div>`, 'Stacked columns with weekends shaded, a line with a band (a gap in the data breaks the line), a one-bar split, and bars by cause (shown as its table). Every chart has a legend, 2 px gaps between marks, hover and arrow-key readout, and a Chart/Table switch. The colours are chart series 1 to 5.')}

${section('footer-bar', 'Footer bar', `
<div class="demo-stack">
  <footer class="wk-footer-bar" id="demo-footer-bar"><button class="wk-link" type="button">Details</button><button class="wk-link" type="button">Open</button></footer>
  <footer class="wk-footer-bar"><button class="wk-link" type="button">Details</button><button class="wk-link" type="button" hidden>Open</button></footer>
</div>`, 'The bottom edge of a window: a hairline, then links. The first sits at the start, the last at the end; hiding one (hidden) leaves the other at the start.')}

${section('tabs', 'Tabs (wk-tabs)', `
<wk-tabs id="demo-tabs">
  <div role="tablist" class="wk-tablist" aria-label="Demo panes">
    <button role="tab" type="button" aria-selected="true">General</button>
    <button role="tab" type="button">Sharing</button>
    <button role="tab" type="button" disabled>Disabled</button>
    <button role="tab" type="button">About</button>
  </div>
  <div role="tabpanel"><p>The General pane. Arrow keys move between tabs; Home and End jump.</p></div>
  <div role="tabpanel"><p>The Sharing pane.</p></div>
  <div role="tabpanel"><p>Never shown.</p></div>
  <div role="tabpanel"><p>The About pane.</p></div>
</wk-tabs>`)}

${section('menus', 'Menu, context menu, tooltip', `
<div class="demo-row">
  <wk-menu id="demo-menu">
    <button class="wk-button" type="button">Actions</button>
    <div role="menu" class="wk-menu" hidden>
      <button role="menuitem" type="button" data-value="copy">Copy details<span class="wk-menu-shortcut">Ctrl+C</span></button>
      <button role="menuitem" type="button" data-value="open">Open in browser</button>
      <button role="menuitem" type="button" aria-disabled="true" data-value="pause">Pause (disabled)</button>
      <div role="separator"></div>
      <button role="menuitemcheckbox" type="button" aria-checked="true" data-value="compact">Compact rows</button>
      <div role="separator"></div>
      <button role="menuitem" type="button" data-value="remove">Remove…</button>
    </div>
  </wk-menu>
  <div class="demo-target" id="demo-context-target" tabindex="0">Right-click here, or focus it and press Shift+F10</div>
  <wk-context-menu for="demo-context-target" id="demo-context">
    <div role="menu" class="wk-menu" aria-label="Item actions" hidden>
      <button role="menuitem" type="button" data-value="rename">Rename</button>
      <button role="menuitem" type="button" data-value="duplicate">Duplicate</button>
      <div role="separator"></div>
      <button role="menuitemradio" type="button" aria-checked="true" data-value="small">Small</button>
      <button role="menuitemradio" type="button" aria-checked="false" data-value="large">Large</button>
    </div>
  </wk-context-menu>
  <wk-tooltip id="demo-tooltip"><button class="wk-button" type="button">Hover or focus me</button><span role="tooltip" class="wk-tooltip" hidden>Copies the code to the clipboard</span></wk-tooltip>
</div>
<p class="wk-caption demo-note">Open, as they look (static copies):</p>
<div class="demo-row demo-static" aria-hidden="true">
  <div class="wk-menu demo-in-flow">
    <div role="menuitem" class="demo-hl">Copy details<span class="wk-menu-shortcut">Ctrl+C</span></div>
    <div role="menuitem">Open in browser</div>
    <div role="menuitem" aria-disabled="true">Pause (disabled)</div>
    <div role="separator"></div>
    <div role="menuitemcheckbox" aria-checked="true">Compact rows</div>
  </div>
  <span class="wk-tooltip demo-in-flow">Copies the code to the clipboard</span>
</div>`)}

${section('dialogs', 'Dialog, sheet, popover surface', `
<div class="demo-row">
  <button class="wk-button" type="button" data-wk-open="demo-dialog" id="demo-dialog-open">Open dialog</button>
  <button class="wk-button" type="button" data-wk-open="demo-sheet" id="demo-sheet-open">Open sheet</button>
  <button class="wk-button" type="button" data-wk-open="demo-info" id="demo-info-open">Open notice</button>
  <button class="wk-button" type="button" data-wk-open="demo-long-sheet" id="demo-long-sheet-open">Open long sheet</button>
  <button class="wk-button" type="button" data-wk-open="demo-alert" id="demo-alert-open">Open alert</button>
</div>
<wk-dialog id="demo-alert">
  <dialog class="wk-dialog wk-dialog--alert" role="alertdialog" aria-labelledby="demo-alert-title" aria-describedby="demo-alert-body">
    ${icons.warn}
    <h3 class="wk-dialog-title" id="demo-alert-title">Stop this job?</h3>
    <p class="wk-dialog-body" id="demo-alert-body">It will fail on GitHub. You can re-run it there.</p>
    <div class="wk-dialog-actions">
      <button class="wk-button wk-button--destructive" type="button" value="stop" data-wk-close>Stop job</button>
      <button class="wk-button wk-button--primary" type="button" value="keep" data-wk-close autofocus>Keep running</button>
    </div>
  </dialog>
</wk-dialog>
<wk-dialog id="demo-dialog">
  <dialog class="wk-dialog">
    <h3 class="wk-dialog-title">Delete “summary.pdf”?</h3>
    <p class="wk-dialog-body">It is deleted for everyone who can open it. This can’t be undone.</p>
    <div class="wk-dialog-actions">
      <button class="wk-button" type="button" value="cancel" data-wk-close autofocus>Cancel</button>
      <button class="wk-button wk-button--destructive" type="button" value="remove" data-wk-close>Delete</button>
    </div>
  </dialog>
</wk-dialog>
<wk-dialog id="demo-sheet">
  <dialog class="wk-dialog wk-dialog--sheet">
    <h3 class="wk-dialog-title">Rename file</h3>
    <p class="wk-dialog-body">The new name is shown to everyone who can open it.</p>
    <p class="wk-dialog-body"><label class="wk-label" for="sheet-name">Name</label><input id="sheet-name" class="wk-field demo-wide" value="summary.pdf"></p>
    <div class="wk-dialog-actions">
      <button class="wk-button wk-dialog-start" type="button" value="help" data-wk-close>Help</button>
      <button class="wk-button" type="button" value="cancel" data-wk-close>Cancel</button>
      <button class="wk-button wk-button--primary" type="button" value="rename" data-wk-close>Rename</button>
    </div>
  </dialog>
</wk-dialog>
<wk-dialog id="demo-long-sheet">
  <dialog class="wk-dialog wk-dialog--sheet" aria-labelledby="demo-long-sheet-title">
    <header class="wk-dialog-header"><h3 class="wk-dialog-title" id="demo-long-sheet-title">Details</h3></header>
    <div class="wk-dialog-scroll" tabindex="0" role="region" aria-labelledby="demo-long-sheet-title">
      <h4 class="wk-section-heading">Now</h4>
      <div class="wk-stats-grid">
        <div class="wk-stat wk-stat--tile"><span class="wk-stat-glyph" aria-hidden="true">${statGlyphs.grid}</span><b class="wk-stat-figure">1 of 2</b><span class="wk-stat-label">items in use</span></div>
        <div class="wk-stat wk-stat--tile"><span class="wk-stat-glyph" aria-hidden="true">${statGlyphs.time}</span><b class="wk-stat-figure">12 min</b><span class="wk-stat-label">since the last change</span></div>
      </div>
      <p class="wk-dialog-indent">Plain text under a heading sits behind a rule (wk-dialog-indent).</p>
      ${Array.from({ length: 6 }, (_, i) => `<h4 class="wk-section-heading">Section ${i + 1}</h4><p class="wk-dialog-indent">Only the middle scrolls; the header and the footer bar stay put.</p>`).join('')}
      <h4 class="wk-section-heading">Set up as</h4>
      <dl class="wk-facts wk-dialog-indent"><dt>Kind</dt><dd>Two items</dd><dt>Version</dt><dd>1.0</dd></dl>
    </div>
    <footer class="wk-dialog-footer"><button class="wk-button wk-button--primary" type="button" value="done" data-wk-close autofocus>Done</button></footer>
  </dialog>
</wk-dialog>
<wk-dialog id="demo-info" light-dismiss>
  <dialog class="wk-dialog">
    <h3 class="wk-dialog-title">About this page</h3>
    <p class="wk-dialog-body">A notice that asks nothing, so a click outside it closes it (light-dismiss).</p>
    <div class="wk-dialog-actions"><button class="wk-button" type="button" data-wk-close>Close</button></div>
  </dialog>
</wk-dialog>
<div class="demo-row demo-static" aria-hidden="true">
  <div class="wk-dialog demo-in-flow">
    <h3 class="wk-dialog-title">Dialog surface</h3>
    <p class="wk-dialog-body">A static copy, to show it open.</p>
    <div class="wk-dialog-actions"><span class="wk-button">Cancel</span><span class="wk-button wk-button--primary">Continue</span></div>
  </div>
  <div class="wk-dialog wk-dialog--alert demo-in-flow">
    ${icons.warn}
    <h3 class="wk-dialog-title">Stop this job?</h3>
    <p class="wk-dialog-body">It will fail on GitHub. You can re-run it there.</p>
    <div class="wk-dialog-actions"><span class="wk-button wk-button--destructive">Stop job</span><span class="wk-button wk-button--primary">Keep running</span></div>
  </div>
  <div class="wk-popover demo-popover">
    <div class="wk-popover-header"><span class="wk-dot" data-shape="ringed" data-tone="working"></span><b>Syncing</b><span class="wk-toolbar-spacer"></span><span class="wk-caption">3 of 5</span></div>
    <div class="wk-popover-section">A popover surface for a tray window: header, sections, footer.</div>
    <div class="wk-popover-section wk-caption">Second section</div>
    <div class="wk-popover-footer"><span class="wk-link">Details</span><span class="wk-button wk-button--small">Pause</span></div>
  </div>
</div>`)}

${section('events', 'Events', '<pre id="demo-log" class="demo-log" aria-live="polite"></pre>', 'wk-* events fired by the components above.')}
`;

// Demo-only behaviour: the appearance picker, filter buttons toggle, the
// mixed checkbox, the log.
document.getElementById('demo-theme').addEventListener('change', e => {
  if (e.target.value) document.documentElement.dataset.theme = e.target.value;
  else delete document.documentElement.dataset.theme;
});
document.getElementById('demo-mixed').indeterminate = true;
document.getElementById('demo-filters').addEventListener('click', e => {
  const b = e.target.closest('.wk-filter');
  if (b && !b.disabled) b.setAttribute('aria-pressed', String(b.getAttribute('aria-pressed') !== 'true'));
});
const log = document.getElementById('demo-log');
window.kitEvents = [];
for (const type of ['wk-open', 'wk-close', 'wk-select', 'wk-change', 'wk-toggle']) {
  document.addEventListener(type, e => {
    const d = e.detail || {};
    const summary = { returnValue: d.returnValue, value: d.value, checked: d.checked, index: d.index, open: d.open, context: d.context?.id };
    window.kitEvents.push(JSON.parse(JSON.stringify({ type, target: e.target.id, ...summary })));
    log.textContent = `${type} on #${e.target.id} ${JSON.stringify(summary)}\n` + log.textContent.split('\n').slice(0, 7).join('\n');
  });
}

// Tokens: read every --wk-* custom property the page's :root rules declare.
const names = new Set();
const walk = rules => {
  for (const rule of rules) {
    if (rule.cssRules) walk(rule.cssRules);
    if (rule.styleSheet) walk(rule.styleSheet.cssRules);
    if (rule.selectorText === ':root') for (const p of rule.style) if (p.startsWith('--wk-')) names.add(p);
  }
};
for (const sheet of document.styleSheets) walk(sheet.cssRules);
const root = getComputedStyle(document.documentElement);
const swatches = document.getElementById('demo-swatches');
const values = document.getElementById('demo-values');
for (const name of [...names].sort()) {
  const value = root.getPropertyValue(name).trim();
  if (name.startsWith('--wk-color-') || name.startsWith('--wk-shadow-')) {
    const card = document.createElement('figure');
    card.className = 'demo-swatch';
    const chip = document.createElement('span');
    chip.className = 'demo-chip';
    if (name.startsWith('--wk-color-')) chip.style.background = `var(${name})`;
    else chip.style.boxShadow = `var(${name})`;
    const cap = document.createElement('figcaption');
    cap.innerHTML = '<code></code><span class="wk-small"></span>';
    cap.firstChild.textContent = name.slice(5);
    cap.lastChild.textContent = value;
    card.append(chip, cap);
    swatches.append(card);
  } else {
    const dt = document.createElement('dt');
    const dd = document.createElement('dd');
    dt.textContent = name.slice(5);
    dd.textContent = value;
    values.append(dt, dd);
  }
}

// Chart specimens: made-up numbers, built with the same call an app makes.
{
  const day = i => ({ label: ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'][i % 7] + ' ' + (3 + i), name: ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'][i % 7] + ' ' + (3 + i) + ' Nov', shaded: i % 7 > 4 });
  const days = Array.from({ length: 14 }, (_, i) => day(i));
  const wave = (k, base) => days.map((d, i) => d.shaded ? Math.floor(base / 3) : base + ((i * 5 + k * 3) % 7));
  const host = document.getElementById('demo-charts');
  const frame = (_name, el) => { const box = document.createElement('div'); box.className = 'demo-chart'; box.append(el); host.append(box); return box; };
  frame('stacked', wkChart({
    id: 'demo-stacked', kind: 'bars', stacked: true, integer: true, label: 'Jobs per day', caption: 'UTC', summary: '14 days',
    categoryHeader: 'Day', shadeLabel: 'Weekend', categories: days,
    series: [{ key: 'a', label: 'Alder', tone: 1, values: wave(0, 8) }, { key: 'b', label: 'Birch', tone: 2, values: wave(1, 5) }, { key: 'c', label: 'Cedar', tone: 4, values: wave(2, 3) }, { key: 'o', label: 'Other', tone: 'neutral', values: wave(3, 1) }],
  }));
  const wait = [20, 25, null, 40, 35, 30, 28, null, 22, 33, 41, 38, 30, 27];
  frame('line', wkChart({
    id: 'demo-line', kind: 'line', label: 'Wait before a job starts', summary: 'median and 9 in 10', categoryHeader: 'Day', shadeLabel: 'Weekend', categories: days,
    format: s => `${s} s`, ticks: max => [0, 30, 60, 90, 120].filter((v, i, a) => i === 0 || a[i - 1] < max),
    series: [{ key: 'p50', label: 'Median wait', tone: 1, values: wait }],
    band: { key: 'p90', label: 'Up to 9 in 10', tone: 1, values: wait.map(v => v == null ? null : v + 40) },
  }));
  frame('split', wkChart({
    id: 'demo-split', kind: 'bars', stacked: true, orientation: 'horizontal', label: 'Share of time', legendValues: true, format: v => `${v}%`,
    categories: [{ label: '', name: 'Share of time' }],
    series: [{ key: 'a', label: 'Alder', tone: 1, values: [52] }, { key: 'b', label: 'Birch', tone: 2, values: [31] }, { key: 'c', label: 'Cedar', tone: 4, values: [17] }],
  }));
  frame('rows', wkChart({
    id: 'demo-rows', kind: 'bars', orientation: 'horizontal', integer: true, label: 'Failures by cause', view: 'table', categoryHeader: 'Cause',
    categories: [{ label: 'The job itself failed' }, { label: 'Stopped by a manager' }, { label: 'Could not start' }],
    series: [{ key: 'f', label: 'Failed jobs', tone: 1, values: [7, 2, 1] }],
  }));
}
