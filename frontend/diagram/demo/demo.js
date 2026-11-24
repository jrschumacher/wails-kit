// Writes the diagram's review page: each diagram type the kit draws, a
// syntax error and an unsupported type. Used by index.html (no theme) and
// themed.html (ui/theme/example.css). It loads the prebuilt
// dist/diagram.js, as an app vendors it; that loads dist/mermaid.js.
import { wkDiagram } from '../dist/diagram.js';

const pages = [['index.html', 'Neutral'], ['themed.html', 'Example theme']];
const here = location.pathname.split('/').pop() || 'index.html';

export const samples = {
  flowchart: ['Release flow', `flowchart LR
  push[Push a tag] --> ci{Checks pass?}
  ci -->|yes| build[Build the apps]
  ci -->|no| fix[Fix and push again]
  fix --> push
  build --> sign[Sign and notarise]
  sign --> publish[(Publish the release)]`],
  sequence: ['Update check', `sequenceDiagram
  autonumber
  participant App
  participant Updates as Update service
  participant GitHub
  App->>Updates: Check for updates
  activate Updates
  Updates->>GitHub: Latest release
  GitHub-->>Updates: v1.4.0
  Note right of Updates: Compare with the running version
  Updates-->>App: Update available
  deactivate Updates
  loop Every 6 hours
    App->>Updates: Check again
  end`],
  state: ['Task states', `stateDiagram-v2
  [*] --> Queued
  Queued --> Running: start
  Running --> Paused: pause
  Paused --> Running: resume
  Running --> Done: finish
  Running --> Failed: error
  Failed --> Queued: retry
  state Running {
    [*] --> Fetching
    Fetching --> Writing
  }
  Done --> [*]`],
  class: ['Settings model', `classDiagram
  class Service {
    +Schema schema
    +GetValues() map
    +SetValues(values) error
  }
  class Schema {
    +Group[] groups
  }
  class Field {
    +string key
    +string type
    +bool advanced
  }
  Service --> Schema
  Schema "1" *-- "many" Field : groups`],
  er: ['Store', `erDiagram
  REPO ||--o{ RUN : has
  RUN ||--|{ JOB : runs
  JOB }o--|| MACHINE : "runs on"
  REPO {
    string owner
    string name
  }
  RUN {
    int id
    string status
  }`],
  gantt: ['Release plan', `gantt
  title Release 1.4
  dateFormat YYYY-MM-DD
  section Build
  Diagram package :done, d1, 2026-11-02, 5d
  Editor package  :active, d2, after d1, 6d
  section Ship
  Review          :crit, r1, after d2, 3d
  Release         :milestone, after r1, 0d`],
};
export const broken = {
  syntax: ['Broken flowchart', `flowchart LR
  a --> b
  b --> --> c`],
  unsupported: ['Pie chart', `pie title Pets
  "Dogs" : 386
  "Cats" : 85`],
};

const main = document.getElementById('demo');
main.innerHTML = `
  <div class="demo-head">
    <div><h1 class="wk-title">@wails-kit/diagram</h1>
    <p class="wk-secondary">Diagrams from Mermaid text, themed by the kit's tokens. Drag to pan, wheel to zoom; with focus, arrow keys pan, + and - zoom, 0 fits.</p></div>
    <div class="demo-row">${pages.filter(([href]) => href !== here).map(([href, text]) => `<a class="wk-link" href="${href}">${text}</a>`).join('')}
      <span class="wk-segmented" role="group" aria-label="Appearance">
        <button type="button" class="wk-button wk-button--small" data-theme-set="">System</button>
        <button type="button" class="wk-button wk-button--small" data-theme-set="light">Light</button>
        <button type="button" class="wk-button wk-button--small" data-theme-set="dark">Dark</button>
      </span></div>
  </div>
  <section class="demo-section"><h2 class="wk-heading">Diagram types</h2><div class="demo-grid" id="demo-types"></div></section>
  <section class="demo-section"><h2 class="wk-heading">Errors</h2><div class="demo-grid" id="demo-errors"></div></section>
  <section class="demo-section"><h2 class="wk-heading">Events</h2><pre class="demo-log" id="demo-log" aria-live="off"></pre></section>`;

for (const b of main.querySelectorAll('[data-theme-set]')) {
  b.addEventListener('click', () => {
    if (b.dataset.themeSet) document.documentElement.dataset.theme = b.dataset.themeSet;
    else delete document.documentElement.dataset.theme;
  });
}
const log = document.getElementById('demo-log');
const add = (host, id, [label, source]) => {
  const figure = document.createElement('figure');
  figure.className = 'demo-figure';
  const el = wkDiagram({ id: `demo-${id}`, label, source, onexport: ({ format, filename, blob }) => { log.textContent = `export ${format} ${filename} (${blob.size} bytes)`; } });
  el.addEventListener('wk-error', e => { log.textContent = `wk-error ${id}: line ${e.detail.line} (${e.detail.kind})`; });
  figure.append(el);
  host.append(figure);
  return el;
};
const all = [
  ...Object.entries(samples).map(([id, s]) => add(document.getElementById('demo-types'), id, s)),
  ...Object.entries(broken).map(([id, s]) => add(document.getElementById('demo-errors'), id, s)),
];
await Promise.all(all.map(el => el.ready));
window.demoReady = true;
