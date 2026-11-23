// wk-diff: a read-only diff, unified or split (old | new side by side), built
// as plain DOM with no dependencies. Each file has a header (its path, a rename
// as old → new, a status chip, +n −m counts) and collapses with a wk-link
// disclosure; large, generated and binary files start collapsed, and a file's
// rows are built only when it is first expanded. Added and removed lines are
// tinted (--wk-color-diff-add, --wk-color-diff-del) and carry a + or − marker,
// so colour is never the only cue; when a removed and an added line pair up,
// the words that changed are marked (--wk-color-diff-*-strong).
//
// The rules with no DOM (parseUnifiedDiff, diffSplitRows, diffWordRanges) are
// plain functions, so node can test them (test/diff.test.mjs). wkDiff builds
// the view.
//
//   wkDiff({
//     id: 'changes',                    // unique on the page; names the parts
//     label: 'Changes',                 // the region's accessible name
//     diff: 'diff --git a/x b/x\n…',    // a unified diff (git format, one or more files)
//                                       // or files already parsed (see parseUnifiedDiff)
//     view: 'unified' | 'split', onview: view => {},
//     wrap: false,                      // unified: long lines wrap (true) or scroll (false);
//                                       // the split view always wraps, so its halves stay aligned
//     collapseAbove: 400,               // files with more lines than this start collapsed
//     generated: file => false,         // generated files start collapsed
//     headingLevel: 3,                  // each file's header is a heading of this level
//     highlight: (file, line) => fragment,  // optional syntax highlighting of one line
//     getContext: (file, range) => lines,   // optional: the unchanged lines between hunks,
//                                       // range { oldStart, oldEnd, newStart, newEnd } (1-based,
//                                       // inclusive); returns strings, or a promise of them
//     contextStep: 20,                  // lines one "Show more" reveals
//   }) -> <section class="wk-diff">
//
// Keys, while focus is in the diff: n and p move to the next or previous hunk,
// ] and [ to the next or previous file. Copying copies code only: the gutter,
// the markers and the screen-reader labels are left out.

// ---- Rules with no DOM ---------------------------------------------------

// A path as git writes it: "a/x", or C-quoted when it holds unusual bytes.
function unquote(path) {
  if (!path.startsWith('"') || !path.endsWith('"') || path.length < 2) return path;
  const bytes = [], esc = { n: 10, t: 9, r: 13, b: 8, f: 12, v: 11, a: 7, '"': 34, '\\': 92 };
  const enc = new TextEncoder();
  const body = path.slice(1, -1);
  for (let i = 0; i < body.length; i++) {
    if (body[i] !== '\\') { bytes.push(...enc.encode(body[i])); continue; }
    const oct = body.slice(i + 1, i + 4);
    if (/^[0-7]{3}$/.test(oct)) { bytes.push(parseInt(oct, 8)); i += 3; } else { bytes.push(esc[body[i + 1]] ?? body.charCodeAt(i + 1)); i += 1; }
  }
  return new TextDecoder().decode(new Uint8Array(bytes));
}
// A path from a ---/+++ line, without its timestamp or its a/ or b/ prefix.
const strip = (path, prefix) => {
  const p = unquote(path.split('\t')[0].trim());
  return p.startsWith(prefix) ? p.slice(2) : p;
};
// The two paths of "diff --git a/x b/y". With spaces in the names the split is
// ambiguous; equal halves win, and the ---/+++ or rename lines that follow
// settle the rest.
function gitPaths(rest) {
  const quoted = rest.match(/^("(?:[^"\\]|\\.)*"|\S+) ("(?:[^"\\]|\\.)*"|\S+)$/);
  if (rest.startsWith('"') && quoted) return [strip(quoted[1], 'a/'), strip(quoted[2], 'b/')];
  const half = (rest.length - 1) / 2;
  if (Number.isInteger(half) && rest[half] === ' ' && rest.slice(2, half) === rest.slice(half + 3)) return [rest.slice(2, half), rest.slice(half + 3)];
  if (quoted) return [strip(quoted[1], 'a/'), strip(quoted[2], 'b/')];
  const m = rest.match(/^a\/(.*) b\/(.*)$/);
  return m ? [m[1], m[2]] : [rest, rest];
}

/**
 * Parses a unified diff (git's format, or plain `diff -u`; one file or many;
 * LF or CRLF) into files:
 *   { path, oldPath, status: 'modified'|'added'|'deleted'|'renamed'|'copied',
 *     binary, oldMode, newMode, additions, deletions,
 *     hunks: [{ header, oldStart, oldLines, newStart, newLines, section,
 *               lines: [{ kind: 'context'|'add'|'del', old, new, text, noNewline? }] }] }
 * `old` and `new` are line numbers (null on the side a line is not on).
 * noNewline marks a last line that had no newline at the end of its file.
 */
export function parseUnifiedDiff(text) {
  const lines = String(text ?? '').split(/\r?\n/);
  if (lines.at(-1) === '') lines.pop();
  const files = [];
  let file = null, hunk = null, oldLeft = 0, newLeft = 0, oldNo = 0, newNo = 0, binaryPatch = false;
  const begin = git => {
    file = { path: '', oldPath: '', status: 'modified', binary: false, oldMode: null, newMode: null, additions: 0, deletions: 0, hunks: [], git, sawOld: false };
    files.push(file); hunk = null; binaryPatch = false;
  };
  const last = () => hunk?.lines.at(-1);
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (hunk && (oldLeft > 0 || newLeft > 0)) {
      const c = line[0];
      if (c === '+' && newLeft > 0) { hunk.lines.push({ kind: 'add', old: null, new: newNo++, text: line.slice(1) }); newLeft--; file.additions++; continue; }
      if (c === '-' && oldLeft > 0) { hunk.lines.push({ kind: 'del', old: oldNo++, new: null, text: line.slice(1) }); oldLeft--; file.deletions++; continue; }
      if ((c === ' ' || line === '') && oldLeft > 0 && newLeft > 0) { hunk.lines.push({ kind: 'context', old: oldNo++, new: newNo++, text: line.slice(1) }); oldLeft--; newLeft--; continue; }
      if (c === '\\') { if (last()) last().noNewline = true; continue; }
      // A hunk cut short: read the line again as a header.
      oldLeft = newLeft = 0;
    }
    if (line.startsWith('\\')) { if (last()) last().noNewline = true; continue; }
    if (line.startsWith('diff --git ')) {
      begin(true);
      [file.oldPath, file.path] = gitPaths(line.slice(11));
      continue;
    }
    const range = line.match(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$/);
    if (range) {
      if (!file) begin(false);
      hunk = { header: line, oldStart: +range[1], oldLines: range[2] == null ? 1 : +range[2], newStart: +range[3], newLines: range[4] == null ? 1 : +range[4], section: range[5] || '', lines: [] };
      file.hunks.push(hunk);
      oldNo = hunk.oldStart; newNo = hunk.newStart; oldLeft = hunk.oldLines; newLeft = hunk.newLines;
      continue;
    }
    if (binaryPatch) continue;
    if (line.startsWith('--- ') && (!file || file.hunks.length || file.sawOld || !file.git) && /^\+\+\+ /.test(lines[i + 1] || '')) begin(false);
    if (!file) continue; // text before the first file (a commit message)
    if (line.startsWith('--- ')) {
      file.sawOld = true; hunk = null;
      const p = strip(line.slice(4), 'a/');
      if (p === '/dev/null') file.status = 'added'; else file.oldPath = p;
    } else if (line.startsWith('+++ ')) {
      const p = strip(line.slice(4), 'b/');
      if (p === '/dev/null') file.status = 'deleted'; else file.path = p;
    } else if (line.startsWith('new file mode ')) { file.status = 'added'; file.newMode = line.slice(14).trim(); }
    else if (line.startsWith('deleted file mode ')) { file.status = 'deleted'; file.oldMode = line.slice(18).trim(); }
    else if (line.startsWith('old mode ')) file.oldMode = line.slice(9).trim();
    else if (line.startsWith('new mode ')) file.newMode = line.slice(9).trim();
    else if (line.startsWith('rename from ')) { file.status = 'renamed'; file.oldPath = unquote(line.slice(12)); }
    else if (line.startsWith('rename to ')) { file.status = 'renamed'; file.path = unquote(line.slice(10)); }
    else if (line.startsWith('copy from ')) { file.status = 'copied'; file.oldPath = unquote(line.slice(10)); }
    else if (line.startsWith('copy to ')) { file.status = 'copied'; file.path = unquote(line.slice(8)); }
    else if (line.startsWith('GIT binary patch')) { file.binary = true; binaryPatch = true; }
    else if (/^Binary files .* differ$/.test(line)) {
      file.binary = true;
      const m = line.match(/^Binary files (.*) and (.*) differ$/);
      if (m && m[1] === '/dev/null') file.status = 'added';
      if (m && m[2] === '/dev/null') file.status = 'deleted';
    }
  }
  return files.map(f => {
    const { git, sawOld, ...out } = f;
    if (!out.path) out.path = out.oldPath;
    if (out.status !== 'renamed' && out.status !== 'copied') out.oldPath = out.path;
    if (out.status === 'renamed' && out.oldPath === out.path) out.status = 'modified';
    return out;
  });
}

/**
 * The split view's rows for a hunk's lines: [{ left, right }], each side a
 * line or null. A context line sits on both sides; a run of removed lines and
 * the run of added lines after it pair up row by row, the longer run's rest
 * facing an empty side.
 */
export function diffSplitRows(lines) {
  const rows = [];
  for (let i = 0; i < lines.length;) {
    const line = lines[i];
    if (line.kind === 'context') { rows.push({ left: line, right: line }); i++; continue; }
    const dels = [], adds = [];
    while (i < lines.length && lines[i].kind === 'del') dels.push(lines[i++]);
    while (i < lines.length && lines[i].kind === 'add') adds.push(lines[i++]);
    for (let k = 0; k < Math.max(dels.length, adds.length); k++) rows.push({ left: dels[k] || null, right: adds[k] || null });
  }
  return rows;
}

const TOKEN = /\s+|[\p{L}\p{N}_]+|[^\s\p{L}\p{N}_]/gu;
const tokenize = text => [...text.matchAll(TOKEN)].map(m => ({ text: m[0], start: m.index, end: m.index + m[0].length }));

/**
 * The parts of a removed line (`before`) and the added line that replaces it
 * (`after`) that differ, word by word, as [start, end) character ranges:
 * { del: [[s, e], …], add: [[s, e], …] }. Lines with no word in common get no
 * ranges (the whole line changed; the row's tint says so). Above `limit`
 * (tokens before × tokens after) only the common start and end are matched.
 */
export function diffWordRanges(before, after, limit = 40000) {
  const a = tokenize(before), b = tokenize(after);
  let p = 0;
  while (p < a.length && p < b.length && a[p].text === b[p].text) p++;
  let s = 0;
  while (s < a.length - p && s < b.length - p && a[a.length - 1 - s].text === b[b.length - 1 - s].text) s++;
  const keepA = new Uint8Array(a.length), keepB = new Uint8Array(b.length);
  for (let k = 0; k < p; k++) keepA[k] = keepB[k] = 1;
  for (let k = 1; k <= s; k++) keepA[a.length - k] = keepB[b.length - k] = 1;
  const n = a.length - p - s, m = b.length - p - s;
  if (n > 0 && m > 0 && n * m <= limit) {
    // Longest common subsequence of the middle tokens.
    const w = m + 1, t = new Uint32Array((n + 1) * w);
    for (let x = n - 1; x >= 0; x--) for (let y = m - 1; y >= 0; y--) {
      t[x * w + y] = a[p + x].text === b[p + y].text ? t[(x + 1) * w + y + 1] + 1 : Math.max(t[(x + 1) * w + y], t[x * w + y + 1]);
    }
    for (let x = 0, y = 0; x < n && y < m;) {
      if (a[p + x].text === b[p + y].text) { keepA[p + x] = keepB[p + y] = 1; x++; y++; }
      else if (t[(x + 1) * w + y] >= t[x * w + y + 1]) x++; else y++;
    }
  }
  const words = (toks, keep) => toks.some((tok, k) => keep[k] && /\S/.test(tok.text));
  if (!words(a, keepA) || !words(b, keepB)) return { del: [], add: [] };
  // Changed words with only unchanged spaces between them read as one change.
  const ranges = (toks, keep) => {
    const out = [];
    let open = null;
    toks.forEach((tok, k) => {
      if (!keep[k]) { if (open) open[1] = tok.end; else out.push(open = [tok.start, tok.end]); }
      else if (/\S/.test(tok.text)) open = null;
    });
    return out;
  };
  return { del: ranges(a, keepA), add: ranges(b, keepB) };
}

// Files from a diff string or from the app, copied so the view can add the
// context it fetches, with counts and line numbers filled in.
function normalize(input) {
  const files = typeof input === 'string' ? parseUnifiedDiff(input) : (input || []);
  return files.map(f => {
    const hunks = (f.hunks || []).map(h => {
      const range = (h.header || '').match(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$/);
      const lines = (h.lines || []).map(l => ({ ...l, old: l.old ?? null, new: l.new ?? null, text: l.text ?? '' }));
      const first = k => lines.find(l => l[k] != null)?.[k];
      const count = k => lines.filter(l => l[k] != null).length;
      const oldStart = h.oldStart ?? (range ? +range[1] : first('old') ?? 0);
      const newStart = h.newStart ?? (range ? +range[3] : first('new') ?? 0);
      const oldLines = h.oldLines ?? (range ? (range[2] == null ? 1 : +range[2]) : count('old'));
      const newLines = h.newLines ?? (range ? (range[4] == null ? 1 : +range[4]) : count('new'));
      const header = h.header || `@@ -${oldStart},${oldLines} +${newStart},${newLines} @@`;
      return { header, oldStart, oldLines, newStart, newLines, section: h.section ?? (range?.[5] || ''), lines };
    });
    const all = hunks.flatMap(h => h.lines);
    const binary = Boolean(f.binary) || f.status === 'binary';
    return {
      ...f, path: f.path || f.oldPath || '', oldPath: f.oldPath || f.path || '',
      status: f.status === 'binary' || !f.status ? 'modified' : f.status, binary, hunks,
      additions: f.additions ?? all.filter(l => l.kind === 'add').length,
      deletions: f.deletions ?? all.filter(l => l.kind === 'del').length,
    };
  });
}

// ---- Drawing -------------------------------------------------------------
let diffSequence = 0;
const MINUS = '−';
function el(tag, cls, text, parent) {
  const node = document.createElement(tag);
  if (cls) node.className = cls;
  if (text != null) node.textContent = text;
  parent?.append(node);
  return node;
}
const sr = (text, parent) => el('span', 'wk-diff-sr', text, parent);
const plural = (n, one, many) => `${n.toLocaleString('en-US')} ${n === 1 ? one : many}`;

// Wraps the [start, end) character ranges of a cell's text in <mark>.
function markRanges(cell, ranges) {
  if (!ranges.length) return;
  const walker = document.createTreeWalker(cell, NodeFilter.SHOW_TEXT);
  const nodes = [];
  for (let n = walker.nextNode(), at = 0; n; n = walker.nextNode()) { nodes.push([n, at]); at += n.data.length; }
  for (const [node, at] of nodes.reverse()) {
    const end = at + node.data.length;
    for (const [s, e] of [...ranges].reverse()) {
      const from = Math.max(s, at), to = Math.min(e, end);
      if (from >= to) continue;
      node.splitText(to - at);
      const middle = node.splitText(from - at);
      const mark = document.createElement('mark');
      mark.className = 'wk-diff-word';
      middle.replaceWith(mark);
      mark.append(middle);
    }
  }
}

const LABEL = { add: 'Added', del: 'Removed', context: 'Line' };
const MARK = { add: '+', del: MINUS, context: ' ' };

/** Builds the diff view. */
export function wkDiff(spec) {
  const id = spec.id || `wk-diff-${++diffSequence}`;
  const label = spec.label || 'Changes';
  const files = normalize(spec.diff);
  const collapseAbove = spec.collapseAbove ?? 400;
  const step = Math.max(1, spec.contextStep || 20);
  let view = spec.view === 'split' ? 'split' : 'unified';

  const root = el('section', 'wk-diff');
  root.id = id; root.tabIndex = 0;
  root.setAttribute('aria-label', label);
  root.setAttribute('aria-keyshortcuts', 'n p ] [');
  const keys = sr('Press n or p for the next or previous change, ] or [ for the next or previous file.', root);
  keys.id = `${id}-keys`;
  root.setAttribute('aria-describedby', keys.id);

  const head = el('div', 'wk-diff-head', null, root);
  const summary = el('p', 'wk-diff-summary', null, head);
  const adds = files.reduce((n, f) => n + f.additions, 0), dels = files.reduce((n, f) => n + f.deletions, 0);
  el('span', null, `${plural(files.length, 'file', 'files')} changed`, summary);
  counts(adds, dels, summary);
  const toggle = el('div', 'wk-segmented wk-diff-view', null, head);
  toggle.setAttribute('role', 'radiogroup'); toggle.setAttribute('aria-label', `${label} view`);
  for (const [value, text] of [['unified', 'Unified'], ['split', 'Split']]) {
    const lab = el('label', null, null, toggle), input = el('input', null, null, lab);
    input.id = `${id}-view-${value}`; input.type = 'radio'; input.name = `${id}-view`; input.value = value; input.checked = value === view;
    el('span', null, text, lab);
    input.addEventListener('change', () => {
      if (!input.checked) return;
      view = value; spec.onview?.(value);
      for (const state of states) if (state.built) { state.built = false; if (state.open) build(state); }
    });
  }
  if (!files.length) el('p', 'wk-diff-none', spec.emptyText || 'No changes', root);

  function counts(a, d, parent) {
    const box = el('span', 'wk-diff-counts', null, parent);
    const shown = el('span', null, null, box);
    shown.setAttribute('aria-hidden', 'true');
    el('span', 'wk-diff-count-add', `+${a.toLocaleString('en-US')}`, shown);
    el('span', 'wk-diff-count-del', `${MINUS}${d.toLocaleString('en-US')}`, shown);
    sr(`${plural(a, 'line', 'lines')} added, ${plural(d, 'line', 'lines')} removed`, box);
    return box;
  }

  const lineCount = f => f.hunks.reduce((n, h) => n + h.lines.length, 0);
  const states = files.map((file, index) => {
    const fid = `${id}-file-${index}`;
    const box = el('div', 'wk-diff-file', null, root);
    box.id = fid; box.dataset.path = file.path;
    const header = el('div', 'wk-diff-file-head', null, box);
    const level = Math.min(6, Math.max(2, spec.headingLevel || 3));
    const title = el(`h${level}`, 'wk-diff-file-title', null, header);
    const button = el('button', 'wk-link wk-diff-toggle', null, title);
    button.type = 'button'; button.id = `${fid}-toggle`;
    button.setAttribute('aria-controls', `${fid}-body`);
    if (file.status === 'renamed' || file.status === 'copied') {
      el('span', 'wk-diff-path', file.oldPath, button);
      el('span', 'wk-diff-arrow', ' → ', button).setAttribute('aria-hidden', 'true');
      sr(file.status === 'renamed' ? ' renamed to ' : ' copied to ', button);
      el('span', 'wk-diff-path', file.path, button);
    } else el('span', 'wk-diff-path', file.path, button);
    const generated = Boolean(file.generated ?? spec.generated?.(file));
    const chips = [];
    if (file.status !== 'modified') chips.push(file.status);
    if (file.binary) chips.push('binary');
    if (generated) chips.push('generated');
    for (const chip of chips) el('span', 'wk-chip wk-diff-chip', chip, header).dataset.status = chip;
    if (!file.binary) counts(file.additions, file.deletions, header);
    const body = el('div', 'wk-diff-body', null, box);
    body.id = `${fid}-body`;
    body.setAttribute('role', 'group'); body.setAttribute('aria-labelledby', button.id);
    const large = lineCount(file) > collapseAbove;
    const state = { file, index, box, button, body, open: file.collapsed != null ? !file.collapsed : !(large || generated || file.binary), built: false };
    button.addEventListener('click', () => { state.open = !state.open; paint(state); });
    paint(state);
    return state;
  });

  function paint(state) {
    state.button.setAttribute('aria-expanded', String(state.open));
    state.body.hidden = !state.open;
    if (state.open && !state.built) build(state);
  }

  function build(state) {
    const { file, body } = state;
    state.built = true;
    body.replaceChildren();
    body.dataset.view = view;
    body.toggleAttribute('data-wrap', Boolean(spec.wrap) || view === 'split');
    if (file.binary) { el('p', 'wk-diff-note', 'Binary file not shown.', body); return; }
    if (!file.hunks.length) {
      const modes = file.oldMode && file.newMode && file.oldMode !== file.newMode;
      el('p', 'wk-diff-note', modes ? `File mode changed from ${file.oldMode} to ${file.newMode}.`
        : file.status === 'renamed' ? 'File renamed without changes.' : 'No changes to show.', body);
      return;
    }
    const max = Math.max(...file.hunks.map(h => Math.max(h.oldStart + h.oldLines, h.newStart + h.newLines)));
    body.style.setProperty('--wk-diff-digits', String(String(max).length));
    const rows = el('div', 'wk-diff-rows', null, body);
    const frag = document.createDocumentFragment();
    file.hunks.forEach((hunk, k) => {
      frag.append(hunkHeader(state, hunk, k));
      frag.append(...rowsFor(file, hunk.lines));
    });
    rows.append(frag);
  }

  // The unchanged lines between hunk k and the one before it (or the top of
  // the file): old line numbers [from, to] and the offset to new numbers.
  function gap(file, k) {
    const h = file.hunks[k], prev = file.hunks[k - 1];
    const start = h.oldLines ? h.oldStart : h.oldStart + 1;
    const prevOld = prev ? (prev.oldLines ? prev.oldStart + prev.oldLines - 1 : prev.oldStart) : 0;
    const prevNew = prev ? (prev.newLines ? prev.newStart + prev.newLines - 1 : prev.newStart) : 0;
    return { from: prevOld + 1, to: start - 1, shift: prevNew - prevOld };
  }

  function hunkHeader(state, hunk, k) {
    const { file } = state;
    const row = el('div', 'wk-diff-hunk');
    row.tabIndex = -1; row.dataset.hunk = String(k);
    sr(`Change ${k + 1} of ${file.hunks.length} in ${file.path}, ${hunk.oldLines ? `from line ${hunk.oldStart}` : `after line ${hunk.oldStart}`}: `, row);
    const g = gap(file, k);
    const canExpand = spec.getContext && file.status !== 'added' && file.status !== 'deleted' && g.to >= g.from;
    if (canExpand) {
      const more = el('button', 'wk-link wk-diff-more', null, row);
      more.type = 'button';
      const label = () => { const n = gap(file, k).to - gap(file, k).from + 1; more.textContent = n > step ? `Show ${step} more lines` : `Show ${plural(n, 'hidden line', 'hidden lines')}`; };
      label();
      more.addEventListener('click', async () => {
        const cur = gap(file, k);
        const from = Math.max(cur.from, cur.to - step + 1), to = cur.to;
        more.disabled = true;
        let got;
        try { got = await spec.getContext(file, { oldStart: from, oldEnd: to, newStart: from + cur.shift, newEnd: to + cur.shift }); } catch { got = null; }
        more.disabled = false;
        if (!Array.isArray(got) || !got.length) { more.remove(); row.focus(); return; }
        got = got.slice(-(to - from + 1));
        const first = to - got.length + 1;
        const added = got.map((text, j) => ({ kind: 'context', old: first + j, new: first + j + cur.shift, text: String(text) }));
        hunk.lines.unshift(...added);
        hunk.oldStart = first; hunk.oldLines += added.length;
        hunk.newStart = first + cur.shift; hunk.newLines += added.length;
        // Rows built for another view are rebuilt when that view is shown.
        if (row.isConnected) row.after(...rowsFor(file, added));
        if (gap(file, k).to < gap(file, k).from) { more.remove(); row.focus(); } else label();
      });
    }
    el('span', 'wk-diff-hunk-text', hunk.header, row).setAttribute('aria-hidden', 'true');
    return row;
  }

  function code(file, line, ranges, parent) {
    const cell = el('span', 'wk-diff-code', null, parent);
    const lit = spec.highlight?.(file, line);
    if (lit != null && lit !== false) cell.append(lit);
    else cell.textContent = line.text;
    if (ranges?.length && cell.textContent === line.text) markRanges(cell, ranges);
    if (line.noNewline) el('span', 'wk-diff-eof', 'No newline at end of file', parent);
    return cell;
  }
  // side: 'old' or 'new' for a half of the split view, null for a unified row.
  function cells(file, line, ranges, row, side) {
    const n = side ? line[side] : line.kind === 'del' ? line.old : line.new;
    const unified = !side;
    sr(`${LABEL[line.kind]} ${n}: `, row);
    if (unified) {
      el('span', 'wk-diff-num', line.old == null ? '' : String(line.old), row).setAttribute('aria-hidden', 'true');
      el('span', 'wk-diff-num', line.new == null ? '' : String(line.new), row).setAttribute('aria-hidden', 'true');
    } else el('span', 'wk-diff-num', String(n), row).setAttribute('aria-hidden', 'true');
    el('span', 'wk-diff-mark', MARK[line.kind], row).setAttribute('aria-hidden', 'true');
    code(file, line, ranges, row);
  }

  function rowsFor(file, lines) {
    const pairs = diffSplitRows(lines);
    const words = new Map();
    for (const { left, right } of pairs) {
      if (left?.kind === 'del' && right?.kind === 'add') {
        const r = diffWordRanges(left.text, right.text);
        words.set(left, r.del); words.set(right, r.add);
      }
    }
    const out = [];
    if (view === 'unified') {
      for (const line of lines) {
        const row = el('div', 'wk-diff-line');
        row.dataset.kind = line.kind;
        if (line.old != null) row.dataset.old = String(line.old);
        if (line.new != null) row.dataset.new = String(line.new);
        cells(file, line, words.get(line), row, null);
        out.push(row);
      }
      return out;
    }
    for (const { left, right } of pairs) {
      const row = el('div', 'wk-diff-line wk-diff-pair');
      for (const [line, side] of [[left, 'old'], [right, 'new']]) {
        const half = el('div', 'wk-diff-half', null, row);
        half.dataset.side = side;
        if (!line) { half.dataset.kind = 'empty'; continue; }
        half.dataset.kind = line.kind;
        if (line[side] != null) half.dataset[side] = String(line[side]);
        // A context line is on both sides: read it once.
        if (line.kind === 'context' && side === 'new') half.setAttribute('aria-hidden', 'true');
        cells(file, line, words.get(line), half, side);
      }
      out.push(row);
    }
    return out;
  }

  // n / p: next or previous hunk; ] / [: next or previous file. From where
  // focus is, or where the text cursor is when the region itself has focus.
  root.addEventListener('keydown', e => {
    if (e.altKey || e.ctrlKey || e.metaKey || e.defaultPrevented) return;
    const t = e.target;
    if (t.isContentEditable || /^(TEXTAREA|SELECT)$/.test(t.tagName) || (t.tagName === 'INPUT' && !/^(radio|checkbox|button)$/.test(t.type))) return;
    const move = { n: ['.wk-diff-hunk', 1], p: ['.wk-diff-hunk', -1], ']': ['.wk-diff-toggle', 1], '[': ['.wk-diff-toggle', -1] }[e.key];
    if (!move) return;
    const [selector, dir] = move;
    let from = document.activeElement;
    if (from === root) { const a = document.getSelection()?.anchorNode; if (a && root.contains(a) && a !== root) from = a; }
    const targets = [...root.querySelectorAll(selector)].filter(x => x.offsetParent !== null);
    const pos = x => from.compareDocumentPosition(x);
    const next = dir > 0
      ? targets.find(x => from === root || (x !== from && !x.contains(from) && (pos(x) & Node.DOCUMENT_POSITION_FOLLOWING)))
      : targets.findLast(x => from !== root && x !== from && !x.contains(from) && (pos(x) & Node.DOCUMENT_POSITION_PRECEDING));
    if (!next) return;
    e.preventDefault();
    next.focus();
    next.scrollIntoView?.({ block: 'nearest' });
  });

  // Copy the code only. In the split view, only the side the selection
  // started on, so old and new lines do not interleave.
  root.addEventListener('copy', e => {
    const sel = document.getSelection();
    if (!sel || sel.isCollapsed || !sel.rangeCount || !e.clipboardData) return;
    const range = sel.getRangeAt(0);
    const c = range.startContainer;
    const start = c.nodeType === 1 ? (c.childNodes[range.startOffset] || c) : c.parentElement;
    const startHalf = (start.nodeType === 1 ? start : start.parentElement)?.closest('.wk-diff-half');
    const side = startHalf?.dataset.side;
    const lines = [];
    for (const cell of root.querySelectorAll('.wk-diff-code')) {
      if (!range.intersectsNode(cell)) continue;
      if (side && cell.closest('.wk-diff-half')?.dataset.side !== side) continue;
      const part = document.createRange();
      part.selectNodeContents(cell);
      if (range.compareBoundaryPoints(Range.START_TO_START, part) > 0) part.setStart(range.startContainer, range.startOffset);
      if (range.compareBoundaryPoints(Range.END_TO_END, part) < 0) part.setEnd(range.endContainer, range.endOffset);
      lines.push(part.toString());
    }
    if (!lines.length) return;
    e.clipboardData.setData('text/plain', lines.join('\n'));
    e.preventDefault();
  });

  return root;
}
