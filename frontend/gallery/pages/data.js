// Data: charts, diffs with review comments, and diagrams.
import { tryButton } from './shared.js';

// A made-up change, as `git diff` prints it, and the file before it (the
// context "Show more" reveals comes from it).
const before = [
  'package server', '', 'import (', '\t"context"', '\t"errors"', '\t"net/http"', '\t"time"', ')', '',
  '// Server answers requests until its context ends.', 'type Server struct {', '\taddr    string',
  '\ttimeout time.Duration', '\tmux     *http.ServeMux', '}', '',
  '// New returns a server that listens on addr.', 'func New(addr string) *Server {',
  '\treturn &Server{addr: addr, timeout: 5 * time.Second, mux: http.NewServeMux()}', '}', '',
  '// Handle adds a route.', 'func (s *Server) Handle(pattern string, h http.Handler) {', '\ts.mux.Handle(pattern, h)', '}', '',
  '// Run serves until ctx is done, then shuts down.', 'func (s *Server) Run(ctx context.Context) error {',
  '\tsrv := &http.Server{Addr: s.addr, Handler: s.mux}', '\tgo func() {', '\t\t<-ctx.Done()',
  '\t\tshutdown, cancel := context.WithTimeout(context.Background(), s.timeout)', '\t\tdefer cancel()',
  '\t\t_ = srv.Shutdown(shutdown)', '\t}()', '\terr := srv.ListenAndServe()',
  '\tif errors.Is(err, http.ErrServerClosed) {', '\t\treturn nil', '\t}', '\treturn err', '}',
];
const ctx = (from, to) => before.slice(from - 1, to).map(l => ` ${l}`);
const generated = ['{', '  "version": 3,', '  "entries": ['];
for (let i = 0; i < 300; i++) generated.push(`-    { "name": "entry-${i}", "hash": "${(i * 2654435761 % 4294967296).toString(16).padStart(8, '0')}" },`, `+    { "name": "entry-${i}", "hash": "${(i * 40503 % 65536).toString(16).padStart(8, '0')}" },`);
const changes = [
  'diff --git a/server/server.go b/server/server.go', 'index 3f1c2a4..8b9d0e7 100644', '--- a/server/server.go', '+++ b/server/server.go',
  '@@ -11,9 +11,10 @@ import (', ...ctx(11, 12), '-\ttimeout time.Duration', '+\ttimeout  time.Duration', '+\tmaxBytes int64', ...ctx(14, 16),
  '-// New returns a server that listens on addr.', '-func New(addr string) *Server {', '-\treturn &Server{addr: addr, timeout: 5 * time.Second, mux: http.NewServeMux()}',
  '+// New returns a server that listens on addr and reads at most maxBytes of a body.', '+func New(addr string, maxBytes int64) *Server {', '+\treturn &Server{addr: addr, timeout: 10 * time.Second, maxBytes: maxBytes, mux: http.NewServeMux()}',
  '@@ -28,9 +29,11 @@ func (s *Server) Handle(pattern string, h http.Handler) {', ...ctx(28, 28),
  '-\tsrv := &http.Server{Addr: s.addr, Handler: s.mux}', '+\tsrv := &http.Server{Addr: s.addr, Handler: http.MaxBytesHandler(s.mux, s.maxBytes)}',
  ...ctx(30, 33), '-\t\t_ = srv.Shutdown(shutdown)', '+\t\tif err := srv.Shutdown(shutdown); err != nil {', '+\t\t\tsrv.Close()', '+\t\t}', ...ctx(35, 35),
  '-\terr := srv.ListenAndServe()', '+\terr := srv.ListenAndServe() // blocks until shutdown',
  'diff --git a/docs/serve.md b/docs/running.md', 'similarity index 88%', 'rename from docs/serve.md', 'rename to docs/running.md', 'index 1a2b3c4..5d6e7f8 100644',
  '--- a/docs/serve.md', '+++ b/docs/running.md', '@@ -1,4 +1,4 @@', '-# Serving', '+# Running the server', ' ', ' Start the server with `run`. It stops when the app quits,',
  '-and waits five seconds for open requests.', '+and waits ten seconds for open requests.', '\\ No newline at end of file',
  'diff --git a/assets/icon.png b/assets/icon.png', 'index 0b1c2d3..4e5f6a7 100644', 'Binary files a/assets/icon.png and b/assets/icon.png differ',
  'diff --git a/data/index.lock.json b/data/index.lock.json', 'index 7a7a7a7..8b8b8b8 100644', '--- a/data/index.lock.json', '+++ b/data/index.lock.json',
  '@@ -1,303 +1,303 @@', ...generated.slice(0, 3).map(l => ` ${l}`), ...generated.slice(3),
  'diff --git a/server/limits.go b/server/limits.go', 'new file mode 100644', 'index 0000000..9c8d7e6', '--- /dev/null', '+++ b/server/limits.go', '@@ -0,0 +1,4 @@',
  '+package server', '+', '+// DefaultMaxBytes is the largest request body New accepts by default.', '+const DefaultMaxBytes = 1 << 20',
  'diff --git a/scripts/run.sh b/scripts/run.sh', 'old mode 100644', 'new mode 100755',
].join('\n') + '\n';
const reviewed = changes.split('diff --git a/assets/')[0];
const getContext = (_file, range) => before.slice(range.oldStart - 1, range.oldEnd);

const days = Array.from({ length: 14 }, (_, i) => ({
  label: `${['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'][i % 7]} ${3 + i}`,
  name: `${['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'][i % 7]} ${3 + i} Nov`,
  shaded: i % 7 > 4,
}));
const wave = (k, base) => days.map((d, i) => (d.shaded ? Math.floor(base / 3) : base + ((i * 5 + k * 3) % 7)));
const jobs = [{ key: 'a', label: 'Alder', tone: 1, values: wave(0, 8) }, { key: 'b', label: 'Birch', tone: 2, values: wave(1, 5) }, { key: 'c', label: 'Cedar', tone: 4, values: wave(2, 3) }, { key: 'o', label: 'Other', tone: 'neutral', values: wave(3, 1) }];

export default [
  {
    id: 'chart', title: 'Chart', group: 'data',
    intro: '<code>wkChart(spec)</code>: bars, stacked bars, or a line with a band, as plain SVG, with a legend, hover and arrow-key readout, and a Chart/Table switch. Colours are chart series 1 to 5. Focus a chart and press the arrow keys to read each value.',
    examples: [{
      title: 'Stacked columns, weekends shaded',
      build(host, { kit }) {
        host.append(kit.wkChart({
          id: 'g-stacked', kind: 'bars', stacked: true, integer: true, label: 'Jobs per day', caption: 'UTC', summary: '14 days',
          categoryHeader: 'Day', shadeLabel: 'Weekend', categories: days, series: jobs,
        }));
      },
    }, {
      title: 'A line with a band, and a gap in the data',
      build(host, { kit }) {
        const wait = [20, 25, null, 40, 35, 30, 28, null, 22, 33, 41, 38, 30, 27];
        host.append(kit.wkChart({
          id: 'g-line', kind: 'line', label: 'Wait before a job starts', summary: 'median and 9 in 10', categoryHeader: 'Day', shadeLabel: 'Weekend', categories: days,
          format: s => `${s} s`, series: [{ key: 'p50', label: 'Median wait', tone: 1, values: wait }],
          band: { key: 'p90', label: 'Up to 9 in 10', tone: 1, values: wait.map(v => (v == null ? null : v + 40)) },
        }));
      },
    }, {
      title: 'A one-bar split, bars by cause, two series side by side, and the same bars as their table',
      build(host, { kit }) {
        const grid = document.createElement('div');
        grid.className = 'g-grid-wide';
        grid.append(kit.wkChart({
          id: 'g-split', kind: 'bars', stacked: true, orientation: 'horizontal', label: 'Share of time', legendValues: true, format: v => `${v}%`,
          categories: [{ label: '', name: 'Share of time' }],
          series: [{ key: 'a', label: 'Alder', tone: 1, values: [52] }, { key: 'b', label: 'Birch', tone: 2, values: [31] }, { key: 'c', label: 'Cedar', tone: 4, values: [17] }],
        }), kit.wkChart({
          id: 'g-rows', kind: 'bars', orientation: 'horizontal', integer: true, label: 'Failures by cause', categoryHeader: 'Cause',
          categories: [{ label: 'The job itself failed' }, { label: 'Stopped by a manager' }, { label: 'Could not start' }],
          series: [{ key: 'f', label: 'Failed jobs', tone: 1, values: [7, 2, 1] }],
        }), kit.wkChart({
          id: 'g-rows-grouped', kind: 'bars', orientation: 'horizontal', label: 'Jobs a day, before and after the upgrade', categoryHeader: 'Machine', format: v => v.toFixed(1),
          categories: [{ label: 'alder' }, { label: 'birch' }, { label: 'cedar' }],
          series: [{ key: 'before', label: 'Before', tone: 'neutral', values: [2, 4.5, 1] }, { key: 'after', label: 'After', tone: 1, values: [6.4, 3, 1.5] }],
        }), kit.wkChart({
          id: 'g-rows-table', kind: 'bars', orientation: 'horizontal', integer: true, label: 'Failures by cause, as a table', view: 'table', categoryHeader: 'Cause',
          categories: [{ label: 'The job itself failed' }, { label: 'Stopped by a manager' }, { label: 'Could not start' }],
          series: [{ key: 'f', label: 'Failed jobs', tone: 1, values: [7, 2, 1] }],
        }));
        host.append(grid);
      },
    }, {
      title: 'No data, and loading',
      note: 'A chart with nothing to draw says so. A loading chart keeps its frame and legend and shows a spinner from 400 ms; "Try it" loads one, then swaps the drawn chart in once wkChart.ready() allows.',
      html: `<div class="g-row">${tryButton('g-chart-load', 'Load the chart')}</div><div class="g-grid-wide" id="g-chart-states"></div>`,
      after(el, { kit, track, later }) {
        const host = el.querySelector('#g-chart-states');
        host.append(kit.wkChart({ id: 'g-empty', kind: 'bars', label: 'Jobs per day', emptyText: 'No jobs in these 14 days', categories: [], series: [] }));
        const spec = { id: 'g-loading', kind: 'bars', stacked: true, integer: true, label: 'Jobs per day', categories: days, series: jobs.map(s => ({ ...s, values: [] })) };
        let fig = kit.wkChart({ ...spec, loading: 'Fetching jobs' });
        host.append(fig);
        el.querySelector('#g-chart-load').addEventListener('click', () => track((async () => {
          const loading = kit.wkChart({ ...spec, loading: 'Fetching jobs' });
          fig.replaceWith(loading);
          fig = loading;
          await later(1500);
          await kit.wkChart.ready(loading);
          fig = kit.wkChart({ ...spec, series: jobs });
          loading.replaceWith(fig);
        })()));
      },
    }],
  },
  {
    id: 'diff', title: 'Diff', group: 'data',
    intro: '<code>wkDiff(spec)</code>: a read-only diff, unified or split, with word marks, "Show more" context, collapsible files and keyboard navigation (n and p for changes, ] and [ for files). Here with syntax colours from <code>@wails-kit/editor</code>\'s highlighter.',
    examples: [{
      title: 'Unified: two hunks, a rename, a binary file, a large generated file (collapsed), a new file and a mode change',
      async build(host, { kit }) {
        const { highlighter } = await import('../../editor/dist/code.js');
        host.append(kit.wkDiff({
          id: 'g-diff', label: 'Changes in this example', diff: changes, getContext, highlight: highlighter(),
          generated: file => file.path.endsWith('.lock.json'),
        }));
      },
    }, {
      title: 'Split, old beside new',
      build(host, { kit }) {
        host.append(kit.wkDiff({ id: 'g-diff-split', label: 'The first two files, split', view: 'split', diff: reviewed, getContext }));
      },
    }, {
      title: 'No changes',
      build(host, { kit }) {
        host.append(kit.wkDiff({ id: 'g-diff-none', label: 'No changes', diff: '' }));
      },
    }],
  },
  {
    id: 'diff-comments', title: 'Diff review comments', group: 'data',
    intro: 'Review threads on a diff: an open thread, a range, a resolved thread (collapsed), a comment the app hasn\'t confirmed (pending), one that failed (Retry), and an outdated thread. Hover a line for the + button, drag or Shift-click line numbers for a range; on a focused line press c. A comment containing "fail" is refused.',
    examples: [{
      title: 'The default comment box',
      build(host, { kit }) {
        host.append(reviewDiff(kit, 'g-review', {}));
      },
    }, {
      title: 'With @wails-kit/editor\'s Markdown editor as the box',
      note: 'The adapter in the ui README: the editor is the box, and posted comments render as Markdown.',
      async build(host, { kit }) {
        const { wkMarkdownEditor, renderMarkdown } = await import('../../editor/dist/markdown.js');
        const onlink = () => {};
        host.append(reviewDiff(kit, 'g-review-md', {
          view: 'split',
          commentBox: ({ value, placeholder, label, onSubmit, onCancel }) => wkMarkdownEditor({ value, placeholder, label, onlink, onsubmit: onSubmit, oncancel: onCancel }),
          renderBody: entry => {
            const body = document.createElement('div');
            body.className = 'wk-markdown';
            body.append(renderMarkdown(entry.body, { onlink }));
            return body;
          },
        }));
      },
    }],
  },
  {
    id: 'diagram', title: 'Diagram', group: 'data',
    intro: '<code>@wails-kit/diagram</code>: each of its six types (<code>diagramTypes</code>), Mermaid text drawn with the kit\'s tokens, with pan and zoom (drag, the wheel, or with focus the arrow keys, + and -, and 0 to fit). Mermaid loads on first use. A syntax error and an unsupported type show their error.',
    examples: [{
      title: 'Flowchart and sequence',
      async build(host) {
        const { wkDiagram } = await import('../../diagram/dist/diagram.js');
        const grid = document.createElement('div');
        grid.className = 'g-grid-wide';
        const flow = wkDiagram({ id: 'g-flow', label: 'Release flow', source: 'flowchart LR\n  push[Push a tag] --> ci{Checks pass?}\n  ci -->|yes| build[Build the apps]\n  ci -->|no| fix[Fix and push again]\n  fix --> push' });
        const seq = wkDiagram({ id: 'g-seq', label: 'Update check', source: 'sequenceDiagram\n  participant App\n  participant Updates as Update service\n  App->>Updates: Check for updates\n  Updates-->>App: Update available' });
        grid.append(flow, seq);
        host.append(grid);
        await Promise.all([flow.ready, seq.ready]);
      },
    }, {
      title: 'State, class, entity-relationship and Gantt',
      async build(host) {
        const { wkDiagram } = await import('../../diagram/dist/diagram.js');
        const grid = document.createElement('div');
        grid.className = 'g-grid-wide';
        const all = [
          wkDiagram({ id: 'g-state', label: 'Task states', source: 'stateDiagram-v2\n  [*] --> Queued\n  Queued --> Running: start\n  Running --> Done: finish\n  Running --> Failed: error\n  Failed --> Queued: retry\n  Done --> [*]' }),
          wkDiagram({ id: 'g-class', label: 'Settings model', source: 'classDiagram\n  class Service {\n    +GetValues() map\n    +SetValues(values) error\n  }\n  class Field {\n    +string key\n    +bool advanced\n  }\n  Service "1" *-- "many" Field : schema' }),
          wkDiagram({ id: 'g-er', label: 'Store', source: 'erDiagram\n  REPO ||--o{ RUN : has\n  RUN ||--|{ JOB : runs\n  RUN {\n    int id\n    string status\n  }' }),
          wkDiagram({ id: 'g-gantt', label: 'Release plan', source: 'gantt\n  title Release 1.4\n  dateFormat YYYY-MM-DD\n  section Build\n  Diagram package :done, d1, 2026-11-02, 5d\n  Editor package  :active, d2, after d1, 6d\n  section Ship\n  Release         :milestone, after d2, 0d' }),
        ];
        grid.append(...all);
        host.append(grid);
        await Promise.all(all.map(d => d.ready));
      },
    }, {
      title: 'The errors: a syntax error and an unsupported type',
      async build(host) {
        const { wkDiagram } = await import('../../diagram/dist/diagram.js');
        const grid = document.createElement('div');
        grid.className = 'g-grid-wide';
        const all = [
          wkDiagram({ id: 'g-broken', label: 'Broken flowchart', source: 'flowchart LR\n  a --> b\n  b --> --> c' }),
          wkDiagram({ id: 'g-pie', label: 'Pie chart', source: 'pie title Pets\n  "Dogs" : 386\n  "Cats" : 85' }),
        ];
        grid.append(...all);
        host.append(grid);
        await Promise.all(all.map(d => d.ready));
      },
    }],
  },
];

// A review: the app's threads, and a made-up server that confirms a comment
// after a moment, or refuses one containing "fail".
// An avatar is an http(s) or data:image URL; this one is drawn here, so
// nothing is fetched.
const avatar = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20"><rect width="20" height="20" fill="#7a8a9a"/><text x="10" y="14" font-size="11" text-anchor="middle" fill="#fff" font-family="sans-serif">A</text></svg>')}`;
function reviewDiff(kit, rid, options) {
  const ago = minutes => new Date(Date.now() - minutes * 60000).toISOString();
  let threads = [
    { id: `${rid}-1`, path: 'server/server.go', side: 'new', line: 14, entries: [
      { author: 'ana', avatar: avatar, time: ago(190), body: 'Should maxBytes have a default?\nA zero here rejects every body.' },
      { author: 'sam', time: ago(42), body: 'Good catch: New will fall back to DefaultMaxBytes.' }] },
    { id: `${rid}-2`, path: 'server/server.go', side: 'new', line: 18, endLine: 20, entries: [
      { author: 'sam', time: ago(30), body: 'The doc comment and the signature changed together; callers need updating.' }] },
    { id: `${rid}-3`, path: 'server/server.go', side: 'old', line: 17, resolved: true, entries: [
      { author: 'ana', time: ago(60 * 26), body: 'Is five seconds enough?' }, { author: 'sam', time: ago(60 * 25), body: 'Raised to ten.' }, { author: 'ana', time: ago(60 * 24), body: 'Thanks.' }] },
    { id: `${rid}-4`, path: 'server/server.go', side: 'new', line: 30, entries: [
      { author: 'You', time: ago(0), body: 'MaxBytesHandler wraps the whole mux; is that intended?', pending: true }] },
    { id: `${rid}-5`, path: 'docs/running.md', side: 'new', line: 1, entries: [
      { author: 'You', time: ago(2), body: 'Maybe "Running" alone?', error: 'HTTP 502: bad gateway from the review server' }] },
    { id: `${rid}-6`, path: 'server/server.go', side: 'new', line: 5, entries: [
      { author: 'ana', time: ago(60 * 24 * 3), body: 'Sort these imports.' }] },
  ];
  let next = 0;
  const later = (fn, ms = 700) => new Promise((resolve, reject) => setTimeout(() => { try { resolve(fn()); } catch (e) { reject(e); } }, ms));
  const refuse = text => { if (/fail/i.test(text)) throw new Error('HTTP 502: bad gateway from the review server'); };
  const update = list => { threads = list; diff.setComments(threads); };
  const diff = kit.wkDiff({
    id: rid, label: options.view === 'split' ? 'Review, split' : 'Review', diff: reviewed, getContext,
    comments: threads, author: { name: 'You' }, ...options,
    oncomment: c => later(() => { refuse(c.body); update([...threads, { id: `${rid}-new-${++next}`, path: c.path, side: c.side, line: c.line, endLine: c.endLine, entries: [{ author: 'You', time: new Date().toISOString(), body: c.body }] }]); }),
    onreply: (tid, text) => later(() => { refuse(text); update(threads.map(t => (t.id === tid ? { ...t, entries: [...t.entries, { author: 'You', time: new Date().toISOString(), body: text }] } : t))); }),
    onresolve: (tid, resolved) => update(threads.map(t => (t.id === tid ? { ...t, resolved } : t))),
    onretry: (tid, entry) => {
      const k = threads.find(t => t.id === tid).entries.indexOf(entry);
      const mark = change => update(threads.map(t => (t.id === tid ? { ...t, entries: t.entries.map((e, j) => (j === k ? { ...e, ...change } : e)) } : t)));
      mark({ error: undefined, pending: true });
      later(() => mark({ pending: false }));
    },
  });
  return diff;
}
