// Containers: panels, tables, lists, stats, code blocks, headings and tabs.
import { icons, statGlyphs, stepper } from './shared.js';

export default [
  {
    id: 'panel', title: 'Panel and facts', group: 'containers', card: 'Panel',
    intro: 'A box for a few rows: default, dashed (something to check rather than change) and ruled rows. <code>wk-facts</code> is a short list of names and values.',
    examples: [{
      title: 'Kinds',
      html: `<div class="g-grid">
  <section class="wk-panel" aria-label="Default panel">
    <div class="wk-panel-row"><span>Copies</span>${stepper('g-panel-copies', 'Copies', 2)}</div>
    <p class="wk-panel-hint">Keeps 2 copies of each file.</p>
    <div class="wk-panel-row"><span>Remove from the team</span><button class="wk-button wk-button--destructive" type="button">Remove…</button></div>
  </section>
  <section class="wk-panel wk-panel--dashed" aria-label="Dashed panel">
    <span class="wk-secondary">Address</span><strong class="wk-mono">server.example:8443</strong>
    <p class="wk-panel-hint">Something to check rather than change.</p>
  </section>
  <section class="wk-panel wk-panel--rows" aria-label="Panel with rows">
    <div class="wk-settings-row"><span class="wk-settings-label">wk-panel--rows</span><input class="wk-switch" type="checkbox" role="switch" checked aria-label="On"></div>
    <div class="wk-settings-row"><span class="wk-settings-label">Rows separated by hairlines</span><input class="wk-checkbox" type="checkbox" aria-label="Off"></div>
  </section>
</div>`,
    }, {
      title: 'Facts',
      html: '<dl class="wk-facts"><dt>Kind</dt><dd>Two items</dd><dt>Version</dt><dd>1.0</dd><dt>Created</dt><dd>14 Mar, 1:32 pm</dd></dl>',
    }],
  },
  {
    id: 'table', title: 'Table', group: 'containers', card: 'Table',
    intro: 'A row can be current (<code>aria-current</code>), selected (<code>aria-selected</code>, which wins over the zebra tint) or muted (<code>data-muted</code>: it no longer counts).',
    examples: [{
      title: 'Plain, with a current row',
      html: `<table class="wk-table">
  <thead><tr><th>Name</th><th>Kind</th><th>Status</th><th class="wk-num">Size</th><th><span class="wk-visually-hidden">Actions</span></th></tr></thead>
  <tbody>
    <tr><td><span class="wk-secondary">reports/</span>summary.pdf</td><td>PDF document</td><td><span class="g-state"><span class="wk-dot" data-shape="ringed" data-tone="working" aria-hidden="true"></span>Uploading</span></td><td class="wk-num">1.2 MB</td><td><a class="wk-link" href="#/table">Open</a></td></tr>
    <tr aria-current="true"><td><span class="wk-secondary">reports/</span>figures.csv</td><td>Spreadsheet</td><td><span class="g-state"><span class="wk-dot" aria-hidden="true"></span>Saved</span></td><td class="wk-num">48 KB</td><td><a class="wk-link" href="#/table">Open</a></td></tr>
    <tr><td><span class="wk-secondary">reports/</span>draft.txt</td><td>Text</td><td class="wk-secondary">Queued</td><td class="wk-num wk-secondary">2 KB</td><td></td></tr>
  </tbody>
</table>`,
    }, {
      title: 'Zebra, with names, actions, a muted and a selected row',
      html: `<table class="wk-table wk-table--zebra">
  <thead><tr><th>Name</th><th>State</th><th class="wk-num">Runs</th><th><span class="wk-visually-hidden">Actions</span></th></tr></thead>
  <tbody>
    <tr><td><button class="wk-table-name" type="button">alpha</button></td><td><span class="g-state"><span class="wk-dot" data-tone="working" aria-hidden="true"></span>Running</span></td><td class="wk-num">19</td><td><span class="wk-table-actions"><button class="wk-button wk-button--small" type="button">Pause</button><button class="wk-button wk-button--small" type="button">Stop…</button></span></td></tr>
    <tr><td><button class="wk-table-name" type="button">beta</button></td><td><span class="g-state"><span class="wk-dot" aria-hidden="true"></span>Ready</span></td><td class="wk-num">4</td><td><span class="wk-table-actions"><button class="wk-button wk-button--small" type="button">Pause</button></span></td></tr>
    <tr data-muted><td><button class="wk-table-name" type="button">gamma</button></td><td><span class="g-state"><span class="wk-dot" data-shape="dashed" aria-hidden="true"></span>Removed</span></td><td class="wk-num">0</td><td></td></tr>
    <tr aria-selected="true"><td><button class="wk-table-name" type="button">delta</button></td><td><span class="g-state"><span class="wk-dot" data-shape="half" aria-hidden="true"></span>On break</span></td><td class="wk-num">7</td><td><span class="wk-table-actions"><button class="wk-button wk-button--small" type="button">Resume</button></span></td></tr>
  </tbody>
</table>`,
    }],
  },
  {
    id: 'list', title: 'List', group: 'containers', card: 'List',
    intro: 'Selectable rows of any height, laid out by the app. The app moves focus as the design system describes; this page does: Up and Down move, Home and End jump, Space or a click selects.',
    examples: [{
      title: 'Two lists, the first with a selected row',
      html: `<div class="g-lists">
  <div>
    <h3 class="wk-list-heading">Active <span>3</span></h3>
    <ul class="wk-list" role="listbox" aria-label="Active">
      <li class="wk-list-row" role="option" aria-selected="true" tabindex="0"><span class="wk-dot-label"><span class="wk-dot" data-tone="working" aria-hidden="true"></span><b>Export the quarterly report</b></span><span class="wk-secondary g-list-meta">Running · started 2 min ago</span><span class="g-list-chips"><span class="wk-chip">PDF</span><span class="wk-chip">48 pages</span></span></li>
      <li class="wk-list-row" role="option" aria-selected="false" tabindex="-1"><span class="wk-dot-label"><span class="wk-dot" data-shape="hollow" aria-hidden="true"></span><b>Sync the shared folder</b></span><span class="wk-secondary g-list-meta">Queued · after the export</span></li>
      <li class="wk-list-row" role="option" aria-selected="false" tabindex="-1"><span class="wk-dot-label"><span class="wk-dot" data-tone="attention" aria-hidden="true"></span><b>Upload figures.csv</b></span><span class="wk-secondary g-list-meta">Failed · the server refused the file</span></li>
    </ul>
  </div>
  <div>
    <h3 class="wk-list-heading">Recent <span>41</span></h3>
    <ul class="wk-list" role="listbox" aria-label="Recent">
      <li class="wk-list-row" role="option" aria-selected="false" tabindex="0"><span class="wk-dot-label"><span class="wk-dot" aria-hidden="true"></span><b>Back up the workspace</b></span><span class="wk-secondary g-list-meta">Done · 38 min ago</span></li>
      <li class="wk-list-row" role="option" aria-selected="false" tabindex="-1"><span class="wk-dot-label"><span class="wk-dot" data-shape="bar" aria-hidden="true"></span><b>Rebuild the search index</b></span><span class="wk-secondary g-list-meta">Skipped · 3 h ago</span></li>
    </ul>
  </div>
</div>`,
      after(el) {
        for (const list of el.querySelectorAll('.wk-list')) {
          const rows = () => [...list.querySelectorAll('.wk-list-row')];
          const focusRow = row => { for (const r of rows()) r.tabIndex = r === row ? 0 : -1; row.focus(); };
          const select = row => { for (const r of rows()) r.setAttribute('aria-selected', String(r === row)); };
          list.addEventListener('click', e => { const row = e.target.closest('.wk-list-row'); if (row) { focusRow(row); select(row); } });
          list.addEventListener('keydown', e => {
            const all = rows(), at = all.indexOf(e.target.closest('.wk-list-row'));
            if (at < 0) return;
            const to = { ArrowDown: Math.min(at + 1, all.length - 1), ArrowUp: Math.max(at - 1, 0), Home: 0, End: all.length - 1 }[e.key];
            if (to !== undefined) { e.preventDefault(); focusRow(all[to]); } else if (e.key === ' ') { e.preventDefault(); select(all[at]); }
          });
        }
      },
    }],
  },
  {
    id: 'stat', title: 'Stat', group: 'containers', card: 'Stat',
    intro: 'A figure and what it counts: two up (<code>wk-stats-grid</code>), as tiles with a glyph (<code>wk-stat--tile</code>), or a ruled row of large figures (<code>wk-stats-row</code>).',
    examples: [{
      title: 'Plain',
      html: `<div class="wk-stats-grid">
  <div class="wk-stat"><b class="wk-stat-figure">1 of 2</b><span class="wk-stat-label">items in use</span></div>
  <div class="wk-stat"><b class="wk-stat-figure">3<small> / 4</small></b><span class="wk-stat-label">with a unit in small</span></div>
</div>`,
    }, {
      title: 'Tiles',
      html: `<div class="wk-stats-grid">
  <div class="wk-stat wk-stat--tile"><span class="wk-stat-glyph" aria-hidden="true">${statGlyphs.grid}</span><b class="wk-stat-figure">1 of 2</b><span class="wk-stat-label">items in use</span></div>
  <div class="wk-stat wk-stat--tile"><span class="wk-stat-glyph" aria-hidden="true">${statGlyphs.ok}</span><b class="wk-stat-figure">OK</b><span class="wk-stat-label">the last 12 worked</span></div>
</div>`,
    }, {
      title: 'Row',
      html: `<div class="wk-stats-row">
  <div class="wk-stat"><b class="wk-stat-figure">3<small> / 4</small></b><span class="wk-stat-label">busy</span></div>
  <div class="wk-stat"><b class="wk-stat-figure">2</b><span class="wk-stat-label">waiting · may be low</span></div>
  <div class="wk-stat"><b class="wk-stat-figure">4 min</b><span class="wk-stat-label">longest wait now</span></div>
</div>`,
    }],
  },
  {
    id: 'code', title: 'Code block', group: 'containers', card: 'CodeBlock',
    intro: 'One value with the copy button, or lines in a <code>&lt;pre&gt;</code>; <code>wk-code--scroll</code> stops at 200 px, and a pre that scrolls takes focus, with a role and a name.',
    examples: [{
      title: 'One value and many lines',
      html: `<div class="g-grid">
  <div class="wk-code"><code>join:v1:AAAA-BBBB-CCCC-DDDD-EEEE-FFFF</code><button class="wk-copy" type="button" aria-label="Copy the code" title="Copy">${icons.copy}</button></div>
  <div class="wk-code wk-code--scroll"><pre tabindex="0" role="region" aria-label="Log">${Array.from({ length: 14 }, (_, i) => `12:${String(i).padStart(2, '0')}:07 step ${i + 1} finished`).join('\n')}</pre></div>
</div>`,
    }],
  },
  {
    id: 'section-heading', title: 'Section heading', group: 'containers', card: 'SectionHeading',
    intro: 'One style at every depth: the words, an optional aside, an optional link at the end; <code>--rule</code> draws a hairline above.',
    examples: [{
      title: 'Plain, with an aside and a link, and ruled',
      html: `<div class="g-stack">
  <h3 class="wk-section-heading">Now</h3>
  <h3 class="wk-section-heading">Recent <span class="wk-section-heading-aside">last 14 days</span><a class="wk-link" href="#/section-heading">All items</a></h3>
  <h3 class="wk-section-heading wk-section-heading--rule">Starts a card</h3>
</div>`,
    }],
  },
  {
    id: 'tabs', title: 'Tabs', group: 'containers',
    intro: '<code>&lt;wk-tabs&gt;</code>: WAI-ARIA tabs with automatic activation. Arrow keys move between tabs, Home and End jump; it fires <code>wk-change</code>.',
    examples: [{
      title: 'With a disabled tab',
      html: `<wk-tabs id="g-tabs">
  <div role="tablist" class="wk-tablist" aria-label="Example panes">
    <button role="tab" type="button" aria-selected="true">General</button>
    <button role="tab" type="button">Sharing</button>
    <button role="tab" type="button" disabled>Disabled</button>
    <button role="tab" type="button">About</button>
  </div>
  <div role="tabpanel"><p>The General pane.</p></div>
  <div role="tabpanel"><p>The Sharing pane.</p></div>
  <div role="tabpanel"><p>Never shown.</p></div>
  <div role="tabpanel"><p>The About pane.</p></div>
</wk-tabs>`,
    }],
  },
];
