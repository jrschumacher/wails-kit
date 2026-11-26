// Layout: the three-pane window, the sidebar, the toolbar and the footer bar.
import { icons } from './shared.js';

const sidebar = id => `<nav class="wk-sidebar${id === 'three' ? '' : ' g-sidebar'}" aria-label="Sections (${id})">
    <ul>
      <li><a class="wk-sidebar-item" href="#/sidebar" aria-current="page">${icons.now}<span class="wk-sidebar-label">Now</span></a></li>
      <li><a class="wk-sidebar-item" href="#/sidebar">${icons.history}<span class="wk-sidebar-label">History</span></a></li>
      <li><details open><summary class="wk-sidebar-item">${icons.items}<span class="wk-sidebar-label">Items</span><span class="wk-chip wk-chip--count">2</span></summary>
        <ul>
          <li><a class="wk-sidebar-item" href="#/sidebar"><span class="wk-dot" data-size="sm" data-tone="working" aria-hidden="true"></span><span class="wk-sidebar-label">alpha</span><span class="wk-sidebar-detail">Running</span></a></li>
          <li><a class="wk-sidebar-item" href="#/sidebar"><span class="wk-dot" data-size="sm" data-shape="dashed" aria-hidden="true"></span><span class="wk-sidebar-label">beta</span><span class="wk-sidebar-detail">Idle</span></a></li>
        </ul></details></li>
      <li><a class="wk-sidebar-item" href="#/sidebar">${icons.book}<span class="wk-sidebar-label">Sources</span><span class="wk-chip wk-chip--count wk-chip--attention">1</span></a></li>
    </ul>
    <p class="wk-sidebar-heading">Group heading</p>
    <ul><li><a class="wk-sidebar-item" href="#/sidebar"><span class="wk-dot" data-size="sm" data-shape="half" aria-hidden="true"></span><span class="wk-sidebar-label">gamma</span></a></li></ul>
    <div class="wk-sidebar-footer">server.example</div>
  </nav>`;
const toolbar = `<header class="wk-toolbar">
      <button class="wk-button wk-button--quiet wk-button--icon" type="button" aria-label="Toggle sidebar">${icons.panel}</button>
      <h3 class="wk-toolbar-title">Now <span class="wk-toolbar-subtitle">last 14 days</span></h3>
      <span class="wk-toolbar-spacer"></span>
      <div class="wk-search"><input class="wk-field g-search" type="search" placeholder="Search" aria-label="Search"></div>
      <span class="wk-chip">Read only</span>
      <button class="wk-button" type="button">New</button>
    </header>`;

export default [
  {
    id: 'three-pane', title: 'Three-pane window', group: 'layout', card: 'ThreePane',
    intro: 'Sidebar, list and inspector. <code>wk-pane-scroll</code> is a pane body that scrolls under a fixed toolbar (give it <code>tabindex="0"</code> and a name).',
    examples: [{
      title: 'A window',
      html: `<div class="wk-three-pane g-frame">
  ${sidebar('three')}
  <section class="wk-pane-list" aria-label="List">
    ${toolbar}
    <div class="wk-pane-scroll" tabindex="0" aria-label="Items" id="g-pane-scroll">
    <table class="wk-table">
      <thead><tr><th>Name</th><th>State</th><th class="wk-num">Items</th></tr></thead>
      <tbody>
        <tr><td><b>alpha</b></td><td><span class="g-state"><span class="wk-dot" data-shape="ringed" data-tone="working" aria-hidden="true"></span>Working</span></td><td class="wk-num">19</td></tr>
        <tr aria-current="true"><td><b>beta</b></td><td><span class="g-state"><span class="wk-dot" aria-hidden="true"></span>Ready</span></td><td class="wk-num">4</td></tr>
        <tr><td><b>gamma</b></td><td><span class="g-state"><span class="wk-dot" data-tone="attention" aria-hidden="true"></span>Needs you</span></td><td class="wk-num">0</td></tr>
        ${['delta', 'epsilon', 'zeta', 'eta', 'theta', 'iota'].map(n => `<tr><td><b>${n}</b></td><td><span class="g-state"><span class="wk-dot" data-shape="hollow" aria-hidden="true"></span>Offline</span></td><td class="wk-num">0</td></tr>`).join('')}
      </tbody>
    </table>
    </div>
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
</div>`,
    }],
  },
  {
    id: 'sidebar', title: 'Sidebar', group: 'layout', card: 'Sidebar',
    intro: 'Items with a glyph or a dot, a count, short trailing text (<code>wk-sidebar-detail</code>), a group that folds, a heading and a footer. The current item has <code>aria-current="page"</code>. This gallery\'s own navigation is one.',
    examples: [{ title: 'Every part', html: sidebar('sidebar') }],
  },
  {
    id: 'toolbar', title: 'Toolbar', group: 'layout', card: 'Toolbar',
    intro: 'The title (with what the view counts over), a spacer, then search and actions. Keep a space before the subtitle so the words read right when copied.',
    examples: [{ title: 'A pane\'s toolbar', html: `<div class="g-boxed">${toolbar}</div>` }],
  },
  {
    id: 'footer-bar', title: 'Footer bar', group: 'layout',
    intro: 'The bottom edge of a window: a hairline, then links. The first sits at the start, the last at the end; hiding one leaves the other at the start.',
    examples: [{
      title: 'Two links, and one',
      html: `<div class="g-stack">
  <footer class="wk-footer-bar"><button class="wk-link" type="button">Details</button><button class="wk-link" type="button">Open</button></footer>
  <footer class="wk-footer-bar"><button class="wk-link" type="button">Details</button><button class="wk-link" type="button" hidden>Open</button></footer>
</div>`,
    }],
  },
  {
    id: 'mosaic', title: 'Mosaic', group: 'layout',
    intro: 'An overview made of tiles in mixed sizes, packed without holes: <code>wk-mosaic</code> around <code>wk-tile</code>s, a quarter wide by default, with <code>--third</code>, <code>--half</code>, <code>--two-thirds</code>, <code>--full</code> and <code>--tall</code>. <code>--feature</code> is the one tile the screen leads with. Each tile has a title, a body and a caption that says where its content came from. Narrow the window: tiles widen to half, then full width.',
    examples: [{
      title: 'An overview',
      html: `<div class="wk-mosaic">
  <article class="wk-tile wk-tile--half wk-tile--tall wk-tile--feature" aria-label="Most read this week">
    <h3 class="wk-tile-title">Most read this week</h3>
    <div class="wk-tile-body"><div class="wk-stat wk-stat--display-lg"><b class="wk-stat-figure">965<small> reads</small></b><span class="wk-stat-label">Getting started with the importer</span></div></div>
    <p class="wk-tile-caption"><span class="wk-dot" data-size="sm" aria-hidden="true"></span>From reports · as of 2:50 pm</p>
  </article>
  <article class="wk-tile" aria-label="Documents">
    <h3 class="wk-tile-title">Documents</h3>
    <div class="wk-tile-body"><div class="wk-stat wk-stat--display"><b class="wk-stat-figure">31</b><span class="wk-stat-label">added in 7 days</span></div></div>
    <p class="wk-tile-caption"><span class="wk-dot" data-size="sm" aria-hidden="true"></span>From reports · as of 2:50 pm</p>
  </article>
  <article class="wk-tile" aria-label="Authors">
    <h3 class="wk-tile-title">Authors</h3>
    <div class="wk-tile-body"><div class="wk-stat wk-stat--display"><b class="wk-stat-figure">28</b><span class="wk-stat-label">wrote at least one</span></div></div>
    <p class="wk-tile-caption"><span class="wk-dot" data-size="sm" aria-hidden="true"></span>From reports · as of 2:50 pm</p>
  </article>
  <article class="wk-tile" aria-label="Drafts">
    <h3 class="wk-tile-title">Drafts</h3>
    <div class="wk-tile-body"><div class="wk-stat wk-stat--display"><b class="wk-stat-figure">11<small> of 31</small></b><span class="wk-stat-label">not published yet</span></div></div>
    <p class="wk-tile-caption"><span class="wk-dot" data-size="sm" aria-hidden="true"></span>From reports · as of 2:50 pm</p>
  </article>
  <article class="wk-tile" aria-label="Shared">
    <h3 class="wk-tile-title">Shared</h3>
    <div class="wk-tile-body"><div class="wk-stat wk-stat--display"><b class="wk-stat-figure" data-unknown>?</b><span class="wk-stat-label">unknown until the next sync</span></div></div>
    <p class="wk-tile-caption"><span class="wk-dot" data-size="sm" aria-hidden="true"></span>Sharing service · not reached</p>
  </article>
  <article class="wk-tile wk-tile--two-thirds" aria-label="Busiest days">
    <h3 class="wk-tile-title">Busiest days</h3>
    <div class="wk-tile-body"><p class="wk-secondary">A chart or a short list goes here.</p></div>
    <p class="wk-tile-caption"><span class="wk-dot" data-size="sm" aria-hidden="true"></span>From reports · as of 2:50 pm</p>
  </article>
  <article class="wk-tile wk-tile--third" aria-label="Left out">
    <h3 class="wk-tile-title">Left out</h3>
    <div class="wk-tile-body"><div class="wk-stat wk-stat--display"><b class="wk-stat-figure">21<small> documents</small></b><span class="wk-stat-label">archived or over the limit</span></div></div>
    <p class="wk-tile-caption"><span class="wk-dot" data-size="sm" aria-hidden="true"></span>From reports · as of 2:50 pm</p>
  </article>
  <article class="wk-tile wk-tile--full" aria-label="Every document">
    <h3 class="wk-tile-title">Every document</h3>
    <div class="wk-tile-body"><p class="wk-secondary">A table goes here.</p></div>
    <p class="wk-tile-caption"><span class="wk-dot" data-size="sm" aria-hidden="true"></span>From reports · as of 2:50 pm</p>
  </article>
</div>`,
    }],
  },
];
