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
//     collapseAbove: 400,               // files with more lines than this start collapsed,
//     collapseCharsAbove: 200000,       // and so do files with more characters than this
//     generated: file => false,         // generated files start collapsed
//     headingLevel: 3,                  // each file's header is a heading of this level
//     highlight: (file, line) => fragment,  // optional syntax highlighting of one line
//     getContext: (file, range) => lines,   // optional: the unchanged lines between hunks,
//                                       // range { oldStart, oldEnd, newStart, newEnd } (1-based,
//                                       // inclusive); returns strings, or a promise of them
//     contextStep: 20,                  // lines one "Show more" reveals
//     emptyText: 'No changes',          // shown for a diff with no files
//     onrender: (fileElement, file) => {},  // after a file's rows are built: when it is
//                                       // first expanded and on every rebuild (a view switch)
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
// A path from a ---/+++ line, without git's quotes or diff's timestamp.
const field = path => unquote(path.split('\t')[0].trim());
// git puts a prefix on each side's path: a/ and b/ by default; c/, i/, w/ or
// o/ with diff.mnemonicPrefix; anything with --src-prefix and --dst-prefix;
// nothing with --no-prefix. Two paths name one file when they are equal (no
// prefix) or differ only in a first folder each (the prefixes): the file's
// path and each side's prefix, or null.
function samePath(a, b) {
  if (a === b) return { path: a, prefixes: ['', ''] };
  const cut = p => { const i = p.indexOf('/'); return i > 0 ? [p.slice(0, i + 1), p.slice(i + 1)] : null; };
  const x = cut(a), y = cut(b);
  return x && y && x[1] && x[1] === y[1] ? { path: x[1], prefixes: [x[0], y[0]] } : null;
}
// The file "diff --git <old> <new>" names when both sides name the same one
// (anything but a rename or a copy, whose own lines name the paths). With
// spaces in the name, the first split that names one file wins.
function gitHeader(rest) {
  const quoted = /^"(?:[^"\\]|\\.)*"$/;
  for (let i = rest.indexOf(' '); i > 0; i = rest.indexOf(' ', i + 1)) {
    const a = rest.slice(0, i), b = rest.slice(i + 1);
    if (a.startsWith('"') !== quoted.test(a) || b.startsWith('"') !== quoted.test(b)) continue;
    const same = samePath(unquote(a), unquote(b));
    if (same) return same;
  }
  return null;
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
    file = { path: '', oldPath: '', status: 'modified', binary: false, oldMode: null, newMode: null, additions: 0, deletions: 0, hunks: [], git, sawOld: false, named: false, rawOld: null, rawNew: null };
    files.push(file); hunk = null; binaryPatch = false;
  };
  const last = () => hunk?.lines.at(-1);
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    // A hunk whose header claims more lines than follow ends where the next
    // file starts: a diff --git line, or ---, +++ and @@ one after another.
    if (hunk && (oldLeft > 0 || newLeft > 0) && (line.startsWith('diff --git ')
      || (line.startsWith('--- ') && /^\+\+\+ /.test(lines[i + 1] || '') && (lines[i + 2] || '').startsWith('@@')))) oldLeft = newLeft = 0;
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
      const same = gitHeader(line.slice(11));
      if (same) { file.path = file.oldPath = same.path; file.named = true; }
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
      const p = field(line.slice(4));
      if (p === '/dev/null') file.status = 'added'; else file.rawOld = p;
    } else if (line.startsWith('+++ ')) {
      const p = field(line.slice(4));
      if (p === '/dev/null') file.status = 'deleted'; else file.rawNew = p;
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
  // Paths from the ---/+++ lines, when nothing better named them: without
  // the prefixes those lines show, or the ones an earlier file showed when
  // one side is /dev/null; unchanged when there is no prefix to see.
  let prefixes = null;
  return files.map(f => {
    const { git, sawOld, named, rawOld, rawNew, ...out } = f;
    const renamed = out.status === 'renamed' || out.status === 'copied';
    if (!named && !renamed && (rawOld != null || rawNew != null)) {
      const same = rawOld != null && rawNew != null ? samePath(rawOld, rawNew) : null;
      if (same) { out.path = same.path; if (same.prefixes[0]) prefixes = same.prefixes; }
      else if (rawNew != null && rawOld != null) out.path = rawNew; // two files compared by name: show the new one
      else {
        const raw = rawOld ?? rawNew, prefix = prefixes?.[rawOld != null ? 0 : 1];
        out.path = prefix && raw.startsWith(prefix) ? raw.slice(prefix.length) : raw;
      }
    }
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
 * facing an empty side. A line of any other kind is treated as context.
 */
export function diffSplitRows(lines) {
  const rows = [];
  for (let i = 0; i < lines.length;) {
    const line = lines[i];
    if (line?.kind !== 'del' && line?.kind !== 'add') { rows.push({ left: line, right: line }); i++; continue; }
    const dels = [], adds = [];
    while (i < lines.length && lines[i].kind === 'del') dels.push(lines[i++]);
    while (i < lines.length && lines[i].kind === 'add') adds.push(lines[i++]);
    for (let k = 0; k < Math.max(dels.length, adds.length); k++) rows.push({ left: dels[k] || null, right: adds[k] || null });
  }
  return rows;
}

// Words keep their combining marks; an emoji keeps its modifiers and the
// emoji joined to it (a ZWJ sequence), and a flag both its letters.
const TOKEN = /\s+|[\p{L}\p{M}\p{N}_]+|\p{RI}\p{RI}|\p{Extended_Pictographic}[\p{M}\p{Emoji_Modifier}]*(?:\u200d\p{Extended_Pictographic}[\p{M}\p{Emoji_Modifier}]*)*|[^\s\p{L}\p{M}\p{N}_]\p{M}*/gu;
// Lines longer than this get no word marks: comparing them would cost more
// than the marks are worth (a minified file, say).
const WORD_LINE_MAX = 10000;
const tokenize = text => [...text.matchAll(TOKEN)].map(m => ({ text: m[0], start: m.index, end: m.index + m[0].length }));

/**
 * The parts of a removed line (`before`) and the added line that replaces it
 * (`after`) that differ, word by word, as [start, end) character ranges:
 * { del: [[s, e], …], add: [[s, e], …] }. Lines with no word in common get no
 * ranges (the whole line changed; the row's tint says so). Above `limit`
 * (tokens before × tokens after) only the common start and end are matched;
 * a line longer than 10,000 characters gets no ranges.
 */
export function diffWordRanges(before, after, limit = 40000) {
  if (before.length > WORD_LINE_MAX || after.length > WORD_LINE_MAX) return { del: [], add: [] };
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
// context it fetches, with counts and line numbers filled in. A line of an
// unknown kind becomes context.
const KINDS = new Set(['context', 'add', 'del']);
function normalize(input) {
  const files = typeof input === 'string' ? parseUnifiedDiff(input) : (input || []);
  return files.map(f => {
    const hunks = (f.hunks || []).map(h => {
      const range = (h.header || '').match(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$/);
      // Any kind but add and del is context, so every line has a place.
      const kinds = (h.lines || []).map(l => ({ ...l, kind: KINDS.has(l.kind) ? l.kind : 'context', text: String(l.text ?? '') }));
      const on = side => kinds.filter(l => l.kind === 'context' || l.kind === (side === 'old' ? 'del' : 'add'));
      const given = side => on(side).find(l => l[side] != null);
      // A given number sets where the count starts; else the header; else 1.
      const start = side => {
        const at = h[side + 'Start'] ?? (range ? +range[side === 'old' ? 1 : 3] : null);
        if (at != null) return at;
        const g = given(side);
        return g ? g[side] - on(side).indexOf(g) : (on(side).length ? 1 : 0);
      };
      const oldStart = start('old'), newStart = start('new');
      // Lines without numbers take them from the hunk's start, in order.
      let o = oldStart, n = newStart;
      const lines = kinds.map(l => {
        const old = l.kind === 'add' ? null : (l.old ?? o);
        const neu = l.kind === 'del' ? null : (l.new ?? n);
        if (old != null) o = old + 1;
        if (neu != null) n = neu + 1;
        return { ...l, old, new: neu };
      });
      const oldLines = h.oldLines ?? (range ? (range[2] == null ? 1 : +range[2]) : on('old').length);
      const newLines = h.newLines ?? (range ? (range[4] == null ? 1 : +range[4]) : on('new').length);
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
// A node the highlighter returned, or a copy when it is already on the page
// or was used before (the same node for every line); a fragment empties when
// it is inserted, so its first content is kept to copy.
const usedNodes = new WeakMap();
function fresh(node) {
  if (usedNodes.has(node)) return usedNodes.get(node).cloneNode(true);
  if (node.isConnected) return node.cloneNode(true);
  usedNodes.set(node, node.nodeType === 11 ? node.cloneNode(true) : node);
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

// What a screen reader hears before a line, the same in both views.
const lineLabel = line => line.kind === 'add' ? `Added ${line.new}: `
  : line.kind === 'del' ? `Removed ${line.old}: `
  : line.old === line.new ? `Line ${line.new}: ` : `Line ${line.old}, now ${line.new}: `;
// Leading whitespace in columns (a tab to the next multiple of 4, as
// tab-size), for the hanging indent of a wrapped line; at most 24.
function indentOf(text) {
  let col = 0;
  for (const ch of text) {
    if (ch === ' ') col++;
    else if (ch === '\t') col = (Math.floor(col / 4) + 1) * 4;
    else break;
    if (col >= 24) return 24;
  }
  return col;
}
const MARK = { add: '+', del: MINUS, context: ' ' };

/** Builds the diff view. */
export function wkDiff(spec) {
  const id = spec.id || `wk-diff-${++diffSequence}`;
  const label = spec.label || 'Changes';
  const files = normalize(spec.diff);
  const collapseAbove = spec.collapseAbove ?? 400;
  const collapseCharsAbove = spec.collapseCharsAbove ?? 200000;
  const step = Math.max(1, spec.contextStep || 20);
  let view = spec.view === 'split' ? 'split' : 'unified';

  const root = el('section', 'wk-diff');
  root.id = id;
  root.setAttribute('aria-label', label);
  if (!files.length) { el('p', 'wk-diff-none', spec.emptyText || 'No changes', root); return root; }
  root.tabIndex = 0;
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
  const charCount = f => f.hunks.reduce((n, h) => n + h.lines.reduce((m, l) => m + l.text.length, 0), 0);
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
    const large = lineCount(file) > collapseAbove || charCount(file) > collapseCharsAbove;
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
    fill(state);
    spec.onrender?.(state.box, file);
  }
  function fill(state) {
    const { file, body } = state;
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
    const canExpand = !hunk.noMore && spec.getContext && file.status !== 'added' && file.status !== 'deleted' && g.to >= g.from;
    if (canExpand) {
      const more = el('button', 'wk-link wk-diff-more', null, row);
      more.type = 'button';
      more.disabled = Boolean(hunk.loading);
      moreLabel(file, k, more);
      const note = el('span', 'wk-diff-more-note', null, row);
      note.setAttribute('aria-live', 'polite');
      more.addEventListener('click', () => showMore(state, hunk, k));
    }
    el('span', 'wk-diff-hunk-text', hunk.header, row).setAttribute('aria-hidden', 'true');
    return row;
  }

  function moreLabel(file, k, more) {
    const g = gap(file, k), n = g.to - g.from + 1;
    more.textContent = n > step ? `Show ${step} more lines` : `Show ${plural(n, 'hidden line', 'hidden lines')}`;
  }
  // Fetches the hidden lines above hunk k. The view may be rebuilt (a view
  // switch) while they load, so the rows go after the header as it is then.
  async function showMore(state, hunk, k) {
    const { file } = state;
    const header = () => state.built && state.body.querySelector(`.wk-diff-hunk[data-hunk="${k}"]`);
    const parts = h => h ? { more: h.querySelector('.wk-diff-more'), note: h.querySelector('.wk-diff-more-note') } : {};
    const cur = gap(file, k);
    const from = Math.max(cur.from, cur.to - step + 1), to = cur.to;
    hunk.loading = true;
    { const { more, note } = parts(header()); if (more) more.disabled = true; if (note) note.textContent = ''; }
    let got, failed = false;
    try { got = await spec.getContext(file, { oldStart: from, oldEnd: to, newStart: from + cur.shift, newEnd: to + cur.shift }); } catch { failed = true; }
    hunk.loading = false;
    const row = header();
    const { more, note } = parts(row);
    const hadFocus = more && document.activeElement === more;
    if (more) more.disabled = false;
    if (failed) { if (note) note.textContent = 'Couldn\u2019t load lines'; return; }
    const close = () => { more?.remove(); if (hadFocus) row.focus(); };
    if (!Array.isArray(got) || !got.length) { hunk.noMore = true; close(); return; }
    got = got.slice(-(to - from + 1));
    const first = to - got.length + 1;
    const added = got.map((text, j) => ({ kind: 'context', old: first + j, new: first + j + cur.shift, text: String(text) }));
    hunk.lines.unshift(...added);
    hunk.oldStart = first; hunk.oldLines += added.length;
    hunk.newStart = first + cur.shift; hunk.newLines += added.length;
    // A file that is not built picks the lines up when it is.
    if (row) row.after(...rowsFor(file, added));
    if (gap(file, k).to < gap(file, k).from) close(); else if (more) moreLabel(file, k, more);
    if (row) spec.onrender?.(state.box, file);
  }

  function code(file, line, ranges, parent) {
    const cell = el('span', 'wk-diff-code', null, parent);
    let lit = null;
    try { lit = spec.highlight?.(file, line); } catch { lit = null; } // a highlighter that fails leaves plain text
    if (lit instanceof Node) lit = fresh(lit);
    if (lit != null && lit !== false) cell.append(lit);
    else cell.textContent = line.text;
    const indent = indentOf(line.text);
    if (indent) cell.style.setProperty('--wk-diff-indent', String(indent));
    if (ranges?.length && cell.textContent === line.text) markRanges(cell, ranges);
    if (line.noNewline) el('span', 'wk-diff-eof', 'No newline at end of file', parent);
    return cell;
  }
  // side: 'old' or 'new' for a half of the split view, null for a unified row.
  function cells(file, line, ranges, row, side) {
    sr(lineLabel(line), row);
    if (!side) {
      el('span', 'wk-diff-num', line.old == null ? '' : String(line.old), row).setAttribute('aria-hidden', 'true');
      el('span', 'wk-diff-num', line.new == null ? '' : String(line.new), row).setAttribute('aria-hidden', 'true');
    } else el('span', 'wk-diff-num', String(line[side]), row).setAttribute('aria-hidden', 'true');
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
    // The side: the half the selection starts in, else the one it ends in,
    // else the first it crosses, else the new side; none in the unified view.
    const halfAt = (c, offset, end) => {
      const node = c.nodeType === 1 ? (c.childNodes[end ? offset - 1 : offset] || c) : c;
      return (node.nodeType === 1 ? node : node.parentElement)?.closest('.wk-diff-half');
    };
    const crossed = [...root.querySelectorAll('.wk-diff-half')].find(h => range.intersectsNode(h));
    const side = crossed ? (halfAt(range.startContainer, range.startOffset) || halfAt(range.endContainer, range.endOffset, true) || crossed).dataset.side || 'new' : undefined;
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
