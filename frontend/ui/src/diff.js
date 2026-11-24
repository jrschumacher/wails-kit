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
//     // Review comments (see README, "Review comments"):
//     comments: [{ id, path, side: 'old'|'new', line, endLine?, resolved?, outdated?,
//                  entries: [{ author, avatar?, time, body, bodyHtml?, pending?, error? }] }],
//     canComment: true,                 // or (file, side, line) => boolean; needs oncomment
//     commentBox: spec => box,          // the box's factory: { value, placeholder, label,
//                                       // onSubmit, onCancel } -> { element, value, focus(),
//                                       // destroy() }; defaults to a wk-field textarea
//     renderBody: entry => node,        // a comment's body; defaults to its plain text
//     oncomment: ({ path, side, line, endLine, body }) => promise?,
//     onreply: (threadId, body) => promise?, onresolve: (threadId, resolved) => {},
//     onretry: (threadId, entry) => {}, // Retry on an entry the app marked with error
//     author: { name, avatar },         // who a pending comment shows as (default "You")
//     now: () => Date.now(),            // the clock relative times are measured from
//   }) -> <section class="wk-diff">, with setComments(threads) to replace the threads
//
// Keys, while focus is in the diff: n and p move to the next or previous hunk,
// ] and [ to the next or previous file, N and P to the next or previous thread.
// With commenting on, lines take focus: the arrow keys move between them
// (left and right change side in the split view), Shift with an arrow extends
// a range, and c opens a comment box. Copying copies code only: the gutter,
// the markers and the screen-reader labels are left out.

import { announce } from './announce.js';
import { timings } from './motion.js';

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

/**
 * Where review threads go in a file: { placed, outdated }. A thread is placed
 * when its line (and its endLine, for a range) is shown on its side ('old' or
 * 'new'; anything else is 'new'), both in one hunk: { thread, side, line,
 * endLine, hunk }, endLine being the line it is drawn under. Otherwise, or
 * when the thread says it is outdated, it is outdated: { thread, side, line,
 * endLine, outdated: true }. Threads for other paths (not the file's path or
 * old path) are in neither list.
 */
export function diffCommentPlaces(file, threads) {
  const placed = [], outdated = [];
  const paths = new Set([file?.path, file?.oldPath].filter(Boolean));
  // Each side's line numbers, indexed once: line -> its hunk.
  const hunkOf = { old: new Map(), new: new Map() };
  (file?.hunks || []).forEach((h, k) => { for (const l of h.lines) for (const s of ['old', 'new']) if (l[s] != null && !hunkOf[s].has(l[s])) hunkOf[s].set(l[s], k); });
  for (const thread of threads || []) {
    if (!thread || !paths.has(thread.path)) continue;
    const side = thread.side === 'old' ? 'old' : 'new';
    let line = Number(thread.line), endLine = thread.endLine == null ? line : Number(thread.endLine);
    if (endLine < line) [line, endLine] = [endLine, line];
    const hunk = !thread.outdated && Number.isInteger(line) && Number.isInteger(endLine) && hunkOf[side].has(line) && hunkOf[side].get(line) === hunkOf[side].get(endLine) ? hunkOf[side].get(line) : -1;
    if (hunk < 0) outdated.push({ thread, side, line, endLine, outdated: true });
    else placed.push({ thread, side, line, endLine, hunk });
  }
  return { placed, outdated };
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
  // Review comments: on when the app can take a new comment or has threads.
  const commenting = typeof spec.oncomment === 'function' && spec.canComment !== false;
  const reviewing = commenting || Array.isArray(spec.comments);

  const root = el('section', 'wk-diff');
  root.id = id;
  root.setAttribute('aria-label', label);
  if (!files.length) { el('p', 'wk-diff-none', spec.emptyText || 'No changes', root); return root; }
  root.tabIndex = 0;
  if (reviewing) root.dataset.review = '';
  root.setAttribute('aria-keyshortcuts', `n p ] [${reviewing ? ' N P' : ''}${commenting ? ' c' : ''}`);
  const keys = sr(`Press n or p for the next or previous change, ] or [ for the next or previous file${reviewing ? ', N or P for the next or previous comment thread' : ''}.${commenting ? ' On a line, press c to comment; Shift with an arrow key selects lines.' : ''}`, root);
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

  // ---- Review comments -------------------------------------------------
  // Threads come from the app (spec.comments, then setComments). The view
  // keeps only what the app has not seen yet: open boxes (drafts, by line
  // and by thread for replies), comments it is posting (a promise from
  // oncomment or onreply), resolves in flight, which resolved threads are
  // expanded, and the selected lines. Drafts are kept as elements and moved
  // into the rows each time a file is rebuilt, so a view switch keeps what
  // was typed. A change to one thread or box redraws only the row it is in.
  let comments = Array.isArray(spec.comments) ? spec.comments : [];
  const posting = [];                 // new threads being posted
  const replying = new Map();         // thread id -> entries being posted
  const drafts = new Map();           // `${path}\0${side}:${line}:${endLine}` -> draft
  const replies = new Map();          // thread id -> draft
  const expanded = new Set();         // resolved threads shown open
  const resolving = new Map();        // thread id -> { shown, timer } while onresolve's promise runs
  const timers = new Set();
  const kept = new Map();             // thread id -> a gone thread kept, outdated, for its reply box with text
  let localSequence = 0, threadSequence = 0, clock = null, destroyed = false;
  const later = (fn, ms) => { const t = setTimeout(() => { timers.delete(t); if (!destroyed) fn(); }, ms); timers.add(t); return t; };
  const now = () => (spec.now ? +spec.now() : Date.now());
  const author = spec.author || {};
  // Outcomes and selections are said by the window's announcer (announce.js),
  // which also makes the same words twice read twice.
  const allowed = (file, side, line) => commenting && Number.isInteger(line)
    && (typeof spec.canComment !== 'function' || Boolean(spec.canComment(file, side, line)));
  // A thread's file: the one whose path is the thread's, else the one whose old path is.
  const byPath = new Map(), byOldPath = new Map();
  files.forEach((f, i) => { if (!byPath.has(f.path)) byPath.set(f.path, i); if (!byOldPath.has(f.oldPath)) byOldPath.set(f.oldPath, i); });
  const fileOf = thread => byPath.get(thread.path) ?? byOldPath.get(thread.path) ?? -1;
  const threadsOf = state => [...comments, ...posting, ...kept.values()].filter(t => t && fileOf(t) === state.index);
  const where = (side, line, endLine) => `${side === 'old' ? 'old ' : ''}${endLine !== line ? `lines ${line} to ${endLine}` : `line ${line}`}`;
  const sameId = (a, b) => String(a) === String(b);

  function countComments() {
    for (const state of states) {
      const n = comments.filter(t => t && fileOf(t) === state.index).reduce((m, t) => m + (t.entries?.length || 0), 0);
      state.talk.textContent = n ? plural(n, 'comment', 'comments') : '';
      state.talk.hidden = !n;
    }
  }

  // A line row: the side a comment on it goes to (an added or context line
  // on the new side, a removed one on the old; in the split view the side
  // asked for, if that half has a line), its number there, and the cell that
  // shows the + button and the selection.
  const isPair = row => row.classList.contains('wk-diff-pair');
  const half = (row, side) => { for (const c of row.children) if (c.dataset.side === side) return c; return null; };
  function sideOf(row, wanted = 'new') {
    if (!isPair(row)) return row.dataset.kind === 'del' ? 'old' : 'new';
    const has = s => half(row, s)?.dataset[s] != null;
    return has(wanted) ? wanted : has('old') ? 'old' : 'new';
  }
  const numberOf = (row, side) => { const n = Number((isPair(row) ? half(row, side) : row)?.dataset[side]); return Number.isInteger(n) && n > 0 ? n : null; };
  const cellOf = (row, side) => (isPair(row) ? half(row, side) : row);
  const stateOf = node => states.find(s => s.body.contains(node));
  // The row showing a line, from an index built once per build of the rows.
  function rowFor(state, side, line) {
    if (!state.rows) {
      state.rows = new Map();
      for (const r of state.body.querySelectorAll('.wk-diff-rows > .wk-diff-line')) {
        for (const s of ['old', 'new']) { const n = numberOf(r, s); if (n != null) state.rows.set(`${s}:${n}`, r); }
      }
    }
    const row = state.rows.get(`${side}:${line}`);
    return row && state.body.contains(row) ? row : null;
  }
  // The next line or hunk header after (dir 1) or before (dir -1) a row.
  function nextStop(node, dir) {
    for (let x = dir > 0 ? node.nextElementSibling : node.previousElementSibling; x; x = dir > 0 ? x.nextElementSibling : x.previousElementSibling) {
      if (x.classList.contains('wk-diff-line') || x.classList.contains('wk-diff-hunk')) return x;
    }
    return null;
  }
  // The rows from a to b (either order) when no hunk header is between them.
  function rowsBetween(a, b) {
    if (a.parentNode !== b.parentNode) return null;
    const [first, last] = a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_PRECEDING ? [b, a] : [a, b];
    const out = [];
    for (let n = first; n; n = n.nextElementSibling) {
      if (n.classList.contains('wk-diff-hunk')) return null;
      if (n.classList.contains('wk-diff-line')) out.push(n);
      if (n === last) return out;
    }
    return null;
  }
  // A selection or a single line as { state, side, line, endLine }, or null.
  function rangeOf(anchor, end, side) {
    const rows = rowsBetween(anchor, end);
    const state = stateOf(anchor);
    if (!rows || !state) return null;
    const nums = rows.map(r => numberOf(r, side)).filter(n => n != null);
    if (!nums.length) return null;
    const line = Math.min(...nums), endLine = Math.max(...nums);
    return allowed(state.file, side, line) && allowed(state.file, side, endLine) ? { state, side, line, endLine } : null;
  }

  // ---- Selection: Shift with an arrow key, Shift-click on line numbers, or
  // a drag on the + button. The selected cells carry data-selected.
  let selection = null; // { anchor, end, side }
  function select(next) {
    for (const cell of root.querySelectorAll('[data-selected]')) delete cell.dataset.selected;
    selection = next && rowsBetween(next.anchor, next.end) ? next : null;
    if (!selection) return;
    for (const row of rowsBetween(selection.anchor, selection.end)) {
      const cell = cellOf(row, selection.side);
      if (cell && (!isPair(row) || numberOf(row, selection.side) != null)) cell.dataset.selected = '';
    }
  }
  const selected = row => selection && rowsBetween(selection.anchor, selection.end)?.includes(row) ? rangeOf(selection.anchor, selection.end, selection.side) : null;

  // ---- The + button: one per diff, moved to the line under the pointer or
  // with focus. It is not a tab stop; on a focused line, c does the same.
  let plus = null, plusRow = null, plusSide = null, focusSide = 'new', drag = null;
  if (commenting) {
    plus = el('button', 'wk-diff-add-comment', '+');
    plus.type = 'button'; plus.tabIndex = -1;
    plus.addEventListener('pointerdown', e => {
      if (e.button !== 0 || !plusRow) return;
      e.preventDefault();
      const at = plusRow, side = plusSide;
      const from = e.shiftKey && selection && selection.side === side && rowsBetween(selection.anchor, at) ? selection.anchor : at;
      drag = { anchor: from, end: at, side, moved: false, start: selected(at) };
      select({ anchor: from, end: at, side });
      try { plus.setPointerCapture(e.pointerId); } catch { /* a synthetic pointer */ }
    });
    plus.addEventListener('pointermove', e => {
      if (!drag) return;
      const row = document.elementFromPoint(e.clientX, e.clientY)?.closest('.wk-diff-line');
      if (!row || row === drag.end || !rowsBetween(drag.anchor, row) || numberOf(row, drag.side) == null) return;
      drag.end = row; drag.moved = true;
      select({ anchor: drag.anchor, end: row, side: drag.side });
    });
    const finish = open => {
      if (!drag) return;
      const d = drag;
      drag = null;
      // A plain click inside a selection comments on the selection.
      const range = !d.moved && d.anchor === d.end && d.start ? d.start : rangeOf(d.anchor, d.end, d.side);
      select(null);
      if (open && range) openDraft(range.state, range.side, range.line, range.endLine);
    };
    plus.addEventListener('pointerup', () => finish(true));
    plus.addEventListener('pointercancel', () => finish(false));
    // A click with no pointer (assistive technology).
    plus.addEventListener('click', e => {
      if (e.detail !== 0 || !plusRow) return;
      const range = selected(plusRow) || rangeOf(plusRow, plusRow, plusSide);
      if (range) openDraft(range.state, range.side, range.line, range.endLine);
    });
  }
  function showPlus(row, side) {
    if (!plus || drag) return;
    const state = row && stateOf(row);
    const s = row && sideOf(row, side);
    const n = row && numberOf(row, s);
    if (!state || !allowed(state.file, s, n)) { plus.remove(); plusRow = null; return; }
    plusRow = row; plusSide = s;
    plus.setAttribute('aria-label', `Comment on ${state.file.path}, ${where(s, n, n)}`);
    const cell = cellOf(row, s);
    if (plus.parentNode !== cell) cell.prepend(plus);
  }
  function focusRow(row, side) {
    if (!row) return;
    const s = sideOf(row, side);
    focusSide = s;
    if (isPair(row)) row.dataset.focusSide = s;
    row.focus({ preventScroll: true });
    row.scrollIntoView?.({ block: 'nearest' });
    showPlus(row, s);
  }
  if (commenting) {
    // Only a pointer that moved: the browser also reports one when what is
    // under a still pointer changes, which would take the button from a
    // line the keyboard just reached.
    let pointer = '';
    root.addEventListener('mousemove', e => {
      const at = `${e.clientX},${e.clientY}`;
      if (at === pointer) return;
      pointer = at;
      const row = e.target.closest?.('.wk-diff-rows > .wk-diff-line');
      if (!row || !root.contains(row)) return;
      const side = e.target.closest('.wk-diff-half')?.dataset.side || focusSide;
      if (row !== plusRow || (isPair(row) && sideOf(row, side) !== plusSide)) showPlus(row, side);
    });
    root.addEventListener('mouseleave', () => {
      const a = document.activeElement;
      showPlus(a?.classList?.contains('wk-diff-line') && root.contains(a) ? a : null, focusSide);
    });
    root.addEventListener('focusin', e => {
      if (e.target.classList?.contains('wk-diff-line')) {
        const s = sideOf(e.target, focusSide);
        if (isPair(e.target)) e.target.dataset.focusSide = s;
        showPlus(e.target, s);
      }
    });
    // A click on a line number focuses the line; with Shift, it selects
    // from the line selected or focused before.
    root.addEventListener('click', e => {
      const num = e.target.closest?.('.wk-diff-num');
      const row = num?.closest('.wk-diff-line');
      if (!row || !root.contains(row)) return;
      const side = isPair(row) ? num.closest('.wk-diff-half').dataset.side : sideOf(row);
      if (numberOf(row, side) == null) return;
      const from = selection?.anchor || (document.activeElement?.classList?.contains('wk-diff-line') ? document.activeElement : null);
      if (e.shiftKey && from && rowsBetween(from, row) && (selection ? selection.side === side : true)) select({ anchor: from, end: row, side });
      else select({ anchor: row, end: row, side });
      focusRow(row, side);
    });
  }

  // Keys on a focused line or hunk header: arrows move, Shift extends the
  // selection, left and right change side (split), c comments, Escape
  // clears the selection. True when the key was used.
  function lineKey(e) {
    const t = e.target;
    const onLine = t.classList?.contains('wk-diff-line'), onHunk = t.classList?.contains('wk-diff-hunk');
    if (!root.contains(t) || (!onLine && !onHunk)) return false;
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      const next = nextStop(t, e.key === 'ArrowDown' ? 1 : -1);
      e.preventDefault();
      if (!next) return true;
      if (e.shiftKey && onLine && next.classList.contains('wk-diff-line')) {
        const side = selection?.side || sideOf(t, focusSide);
        const anchor = selection && rowsBetween(selection.anchor, t) ? selection.anchor : t;
        if (!rowsBetween(anchor, next)) return true;
        select({ anchor, end: next, side });
        focusRow(next, side);
        const range = rangeOf(anchor, next, side);
        if (range) announce(`${side === 'old' ? 'Old l' : 'L'}${range.line === range.endLine ? `ine ${range.line}` : `ines ${range.line} to ${range.endLine}`} selected`);
        return true;
      }
      if (e.shiftKey) return true;
      select(null);
      if (next.classList.contains('wk-diff-line')) focusRow(next, focusSide);
      else { next.focus(); next.scrollIntoView?.({ block: 'nearest' }); }
      return true;
    }
    if (!onLine) return false;
    if ((e.key === 'ArrowLeft' || e.key === 'ArrowRight') && isPair(t) && !e.shiftKey) {
      const side = e.key === 'ArrowLeft' ? 'old' : 'new';
      if (numberOf(t, side) == null) return true;
      e.preventDefault();
      if (selection && selection.side !== side) select(null);
      focusRow(t, side);
      return true;
    }
    if (e.key === 'c' && !e.shiftKey) {
      const range = selected(t) || rangeOf(t, t, sideOf(t, focusSide));
      if (!range) return false;
      e.preventDefault();
      select(null);
      openDraft(range.state, range.side, range.line, range.endLine);
      return true;
    }
    if (e.key === 'Escape' && selection) { e.preventDefault(); select(null); return true; }
    return false;
  }

  // ---- Comment boxes ---------------------------------------------------
  // The box comes from spec.commentBox when given (an editor, say), else a
  // textarea. The view draws Cancel and the submit button around it, and
  // handles Escape and Ctrl or Cmd+Enter unless the box already did, or an
  // input method is composing.
  function textBox({ value, placeholder, label }) {
    const area = document.createElement('textarea');
    area.className = 'wk-field wk-diff-compose-field';
    area.rows = 3;
    area.value = value;
    area.placeholder = placeholder;
    area.setAttribute('aria-label', label);
    return { element: area, get value() { return area.value; }, set value(v) { area.value = v; }, focus: () => area.focus(), destroy() {} };
  }
  function asBox(made) {
    // A bare element (an editor element with value, focus() and destroy()).
    if (made instanceof Node) return { element: made, get value() { return made.value ?? made.textContent; }, set value(v) { made.value = v; }, focus: () => made.focus?.(), destroy: () => made.destroy?.() };
    return made && made.element instanceof Node ? made : null;
  }
  const textOf = draft => String(draft.box.value ?? '');
  // draft: { state, path, side, line, endLine } for a new thread, or
  // { state, thread } for a reply. Fills in frame and box.
  function compose(draft, value, label, submitText) {
    const frame = el('div', 'wk-diff-compose');
    if (draft.side) frame.dataset.side = draft.side;
    const boxSpec = { value: String(value ?? ''), placeholder: spec.commentPlaceholder || (draft.thread ? 'Reply' : 'Leave a comment'), label, onSubmit: () => submit(draft), onCancel: () => cancel(draft) };
    let box = null;
    if (typeof spec.commentBox === 'function') { try { box = asBox(spec.commentBox(boxSpec)); } catch { box = null; } }
    box ||= textBox(boxSpec);
    frame.append(box.element);
    const actions = el('div', 'wk-diff-compose-actions', null, frame);
    const no = el('button', 'wk-button wk-button--small', 'Cancel', actions);
    no.type = 'button'; no.dataset.action = 'cancel';
    no.addEventListener('click', () => cancel(draft));
    const yes = el('button', 'wk-button wk-button--primary wk-button--small', submitText, actions);
    yes.type = 'button'; yes.dataset.action = 'submit';
    yes.addEventListener('click', () => submit(draft));
    frame.addEventListener('keydown', e => {
      if (e.defaultPrevented || e.isComposing || e.keyCode === 229) return;
      if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); cancel(draft); }
      else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); submit(draft); }
    });
    Object.assign(draft, { frame, box });
    return draft;
  }
  const focusBox = draft => { (draft.box.focus ? draft.box.focus() : draft.box.element.focus?.()); draft.frame.scrollIntoView?.({ block: 'nearest' }); };
  function close(draft) {
    if (draft.closed) return false;
    draft.closed = true;
    if (draft.thread) { replies.delete(draft.thread.id); kept.delete(String(draft.thread.id)); } else drafts.delete(draft.key);
    try { draft.box.destroy?.(); } catch { /* the box's own clean-up failed */ }
    draft.frame.remove();
    return true;
  }
  // Puts text in a box already open: an empty box gives way to a new one
  // holding it; one with text gets it added at the end, if the box takes a
  // new value. True when the text is in the box; null when the empty box
  // was closed for a new one.
  function restore(draft, text) {
    if (!text) return true;
    if (!textOf(draft).trim()) { close(draft); return null; }
    const before = textOf(draft);
    try { draft.box.value = `${before.replace(/\s+$/, '')}\n\n${text}`; } catch { /* a box whose value can't be set */ }
    return textOf(draft).includes(text);
  }
  // Opens a box under a line (or range), or focuses the one there. With
  // value, the text goes in it (see restore). True when value is in a box.
  function openDraft(state, side, line, endLine, value = '') {
    const key = `${state.file.path}\0${side}:${line}:${endLine}`;
    let draft = drafts.get(key), placed = true;
    if (draft && value) { placed = restore(draft, value); if (placed === null) { draft = null; placed = true; } }
    if (!draft) {
      draft = compose({ key, state, path: state.file.path, side, line, endLine }, value, `Comment on ${state.file.path}, ${where(side, line, endLine)}`, 'Comment');
      drafts.set(key, draft);
    }
    refreshAt(state, side, endLine);
    focusBox(draft);
    return placed;
  }
  function openReply(state, thread, value = '') {
    let draft = replies.get(thread.id), placed = true;
    if (draft && value) { placed = restore(draft, value); if (placed === null) { draft = null; placed = true; } }
    if (!draft) {
      draft = compose({ state, thread }, value, `Reply to the comments on ${state.file.path}`, 'Reply');
      replies.set(thread.id, draft);
    }
    refreshThread(state, thread.id);
    focusBox(draft);
    return placed;
  }
  function cancel(draft) {
    if (!close(draft)) return;
    const { state } = draft;
    if (draft.thread) {
      refreshThread(state, draft.thread.id);
      const reply = threadEl(state, draft.thread.id)?.querySelector('[data-action="reply"]');
      // A kept thread goes with its box: focus moves to the file's toggle.
      (reply || state.button).focus();
    } else {
      refreshAt(state, draft.side, draft.endLine);
      focusRow(rowFor(state, draft.side, draft.endLine), draft.side);
    }
  }
  function submit(draft) {
    if (draft.closed) return;
    const body = textOf(draft);
    if (!body.trim()) { focusBox(draft); return; }
    close(draft);
    const { state } = draft;
    if (draft.thread) send(state, draft.thread, { body });
    else send(state, null, { path: draft.path, side: draft.side, line: draft.line, endLine: draft.endLine, body });
  }

  // Posts a new comment (thread null) or a reply. A promise from the app
  // shows the comment as pending until it settles. When it resolves, the
  // copy stays (as posted) until the app's comments hold it (a thread, or
  // for a reply an entry, with the id the promise resolved with; else an
  // entry with the same text and author), checked then and on each
  // setComments, or for POSTED_KEEP at most. When it rejects, the copy shows the error with Retry and Edit. A
  // shown working dot stays at least timings.indicatorMin. Anything else
  // from the callback: the app shows the comment.
  const POSTED_KEEP = 10_000;
  function send(state, thread, info, entry) {
    let result;
    try { result = thread ? spec.onreply?.(thread.id, info.body) : spec.oncomment(info); } catch (err) { result = Promise.reject(err); }
    if (!result || typeof result.then !== 'function') {
      if (!thread) { refreshAt(state, info.side, info.endLine); focusRow(rowFor(state, info.side, info.endLine), info.side); }
      else { refreshThread(state, thread.id); threadEl(state, thread.id)?.focus(); }
      return;
    }
    entry ||= { author: author.name || 'You', avatar: author.avatar, time: new Date(now()), body: info.body };
    Object.assign(entry, { pending: true, posted: false, error: undefined, since: Date.now(), shownAt: 0, landed: undefined });
    let own = null;
    if (!thread) {
      own = posting.find(t => t.entries[0] === entry);
      if (!own) { own = { id: `${id}-posting-${++localSequence}`, path: info.path, side: info.side, line: info.line, endLine: info.endLine, entries: [entry], local: info }; posting.push(own); }
    } else if (!(replying.get(thread.id) || []).includes(entry)) replying.set(thread.id, [...(replying.get(thread.id) || []), entry]);
    const redraw = () => (own ? refreshAt(state, info.side, info.endLine) : refreshThread(state, thread.id));
    redraw();
    threadEl(state, own ? own.id : thread.id)?.focus();
    const drop = () => {
      if (own) { const k = posting.indexOf(own); if (k >= 0) posting.splice(k, 1); }
      else replying.set(thread.id, (replying.get(thread.id) || []).filter(x => x !== entry));
    };
    entry.drop = drop;
    // Whether the app's comments hold this comment now.
    const same = e => e && (entry.landed != null ? e.id != null && sameId(e.id, entry.landed) : e.body === entry.body && e.author === entry.author);
    entry.covered = () => (own
      ? comments.some(t => t && (entry.landed != null ? sameId(t.id, entry.landed) : t.path === info.path && (t.entries || []).some(same)))
      : (comments.find(t => t && sameId(t.id, thread.id))?.entries || []).some(same));
    // Once the dot has shown, it stays its minimum before the outcome replaces it.
    const settle = fn => { const hold = entry.shownAt ? timings.indicatorMin - (Date.now() - entry.shownAt) : 0; if (hold > 0) later(fn, hold); else fn(); };
    result.then(value => settle(() => {
      const landed = value != null && typeof value === 'object' ? value.id : (typeof value === 'string' || typeof value === 'number' ? value : undefined);
      Object.assign(entry, { pending: false, posted: true, landed });
      announce(thread ? 'Reply posted' : 'Comment posted');
      if (entry.covered()) drop();
      else later(() => { if (entry.posted) { drop(); redraw(); } }, POSTED_KEEP);
      redraw();
    }), err => settle(() => {
      Object.assign(entry, { pending: false, error: err?.message || String(err || 'Unknown error'), retry: () => send(state, thread, info, entry) });
      // Edit puts the text back in a box: a new one, or added to the text of
      // one already open there. The failed comment stays if that can't be done.
      entry.edit = () => {
        const placed = own ? openDraft(state, info.side, info.line, info.endLine, info.body) : openReply(state, thread, info.body);
        if (placed) drop();
        redraw();
        const draft = own ? drafts.get(`${info.path}\0${info.side}:${info.line}:${info.endLine}`) : replies.get(thread.id);
        if (draft) focusBox(draft);
      };
      announce(thread ? 'Couldn’t post the reply' : 'Couldn’t post the comment');
      redraw();
    }));
  }

  // Resolve and Unresolve: a promise from onresolve makes the button busy
  // (data-busy, aria-disabled) until it settles, with a spinner after
  // timings.indicatorDelay that stays at least timings.indicatorMin.
  function resolve(state, thread, to) {
    const tid = String(thread.id);
    if (resolving.has(tid)) return;
    let result;
    try { result = spec.onresolve(thread.id, to); } catch (err) { result = Promise.reject(err); }
    if (!result || typeof result.then !== 'function') return;
    const job = { shown: 0, timer: 0 };
    resolving.set(tid, job);
    refreshThread(state, tid);
    job.timer = later(() => { job.shown = Date.now(); refreshThread(state, tid); }, timings.indicatorDelay);
    const end = failed => {
      const finish = () => {
        if (destroyed) return;
        clearTimeout(job.timer); timers.delete(job.timer);
        resolving.delete(tid);
        if (failed) announce(to ? 'Couldn’t resolve the comments' : 'Couldn’t reopen the comments');
        refreshThread(state, tid);
      };
      const hold = job.shown ? timings.indicatorMin - (Date.now() - job.shown) : 0;
      if (hold > 0) later(finish, hold); else finish();
    };
    result.then(() => end(false), () => end(true));
  }

  // ---- Threads ---------------------------------------------------------
  const threadEl = (state, tid) => [...state.body.querySelectorAll('.wk-diff-thread')].find(x => sameId(x.dataset.thread, tid));
  function relative(ms) {
    const s = (now() - ms) / 1000;
    if (Math.abs(s) < 45) return 'just now';
    const units = [['year', 31557600], ['month', 2629800], ['week', 604800], ['day', 86400], ['hour', 3600], ['minute', 60]];
    const [unit, size] = units.find(([, n]) => Math.abs(s) >= n) || units.at(-1);
    return new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' }).format(-Math.round(s / size), unit);
  }
  // Relative times are redrawn each minute while the diff is on the page.
  function tick() {
    if (clock || destroyed) return;
    clock = setInterval(() => {
      if (!root.isConnected) { clearInterval(clock); clock = null; return; }
      for (const t of root.querySelectorAll('time.wk-diff-time')) t.textContent = relative(Date.parse(t.dateTime));
    }, 60_000);
  }
  // Avatars: http(s) or data:image URLs only. A redraw moves the same <img>,
  // so the picture is not fetched again.
  const avatars = new WeakMap();
  const avatarURL = src => typeof src === 'string' && /^(https?:\/\/|data:image\/)/i.test(src.trim()) ? src.trim() : null;
  function avatarOf(entry) {
    const src = avatarURL(entry.avatar);
    if (!src) return null;
    let img = avatars.get(entry);
    if (!img || img.getAttribute('src') !== src) {
      img = el('img', 'wk-diff-avatar'); img.alt = ''; img.src = src;
      avatars.set(entry, img);
    }
    return img;
  }
  function entryEl(state, thread, entry) {
    const item = el('li', 'wk-diff-comment');
    const head = el('p', 'wk-diff-comment-head', null, item);
    const img = avatarOf(entry);
    if (img) head.append(img);
    el('span', 'wk-diff-author', entry.author || 'Unknown', head);
    const at = entry.time instanceof Date ? entry.time : entry.time != null && entry.time !== '' ? new Date(entry.time) : null;
    if (at && !Number.isNaN(at.getTime())) {
      sr(', ', head);
      const time = el('time', 'wk-diff-time', relative(at.getTime()), head);
      time.dateTime = at.toISOString(); time.title = at.toLocaleString(undefined);
      tick();
    }
    sr(': ', head);
    const body = el('div', 'wk-diff-comment-body', null, item);
    let made = null;
    try { made = spec.renderBody?.(entry); } catch { made = null; }
    if (made instanceof Node) body.append(made);
    else { body.textContent = String(entry.body ?? ''); body.dataset.plain = ''; }
    if (entry.pending) {
      item.dataset.state = 'pending';
      const work = el('span', 'wk-dot-label wk-diff-comment-state', null, head);
      el('span', 'wk-dot', null, work).dataset.tone = 'working';
      work.firstChild.dataset.size = 'sm';
      work.firstChild.setAttribute('aria-hidden', 'true');
      el('span', null, 'Posting…', work);
      // Shown after timings.indicatorDelay (BEHAVIOUR.md); an app's own pending entry at once.
      const wait = entry.since ? timings.indicatorDelay - (Date.now() - entry.since) : 0;
      if (wait > 0) {
        work.hidden = true;
        later(() => { if (entry.pending) { entry.shownAt ||= Date.now(); work.hidden = false; } }, wait);
      } else if (entry.since) entry.shownAt ||= Date.now();
    } else if (entry.error) {
      item.dataset.state = 'error';
      const words = (replying.get(thread.id) || []).includes(entry) ? 'Couldn’t post the reply' : 'Couldn’t post the comment';
      const boxEl = el('div', 'wk-errorbox wk-diff-errorbox', null, item);
      boxEl.setAttribute('role', 'group'); boxEl.setAttribute('aria-label', words);
      const top = el('div', 'wk-errorbox-head', null, boxEl);
      el('span', null, words, top);
      const copy = el('button', 'wk-copy', null, top);
      copy.type = 'button'; copy.title = 'Copy'; copy.setAttribute('aria-label', 'Copy error details');
      copy.append(copyGlyph());
      copy.addEventListener('click', () => { navigator.clipboard?.writeText(`${words}\n${entry.error}`).catch(() => {}); });
      el('pre', null, String(entry.error), boxEl);
      const acts = el('div', 'wk-diff-comment-actions', null, item);
      const retry = entry.retry || (spec.onretry && (() => spec.onretry(thread.id, entry)));
      if (retry) { const b = el('button', 'wk-button wk-button--small', 'Retry', acts); b.type = 'button'; b.dataset.action = 'retry'; b.addEventListener('click', retry); }
      if (entry.edit) { const b = el('button', 'wk-button wk-button--small', 'Edit', acts); b.type = 'button'; b.dataset.action = 'edit'; b.addEventListener('click', entry.edit); }
      if (!acts.childElementCount) acts.remove();
    }
    return item;
  }
  function copyGlyph() {
    const ns = 'http://www.w3.org/2000/svg';
    const svg = document.createElementNS(ns, 'svg');
    svg.setAttribute('viewBox', '0 0 16 16'); svg.setAttribute('fill', 'none'); svg.setAttribute('stroke', 'currentColor'); svg.setAttribute('stroke-width', '1.5'); svg.setAttribute('aria-hidden', 'true');
    for (const d of ['M5.5 7.1a1.6 1.6 0 0 1 1.6-1.6h4.8a1.6 1.6 0 0 1 1.6 1.6v4.8a1.6 1.6 0 0 1-1.6 1.6H7.1a1.6 1.6 0 0 1-1.6-1.6z', 'M10.5 3.5v-.2a1.3 1.3 0 0 0-1.3-1.3H3.8a1.3 1.3 0 0 0-1.3 1.3v5.4a1.3 1.3 0 0 0 1.3 1.3h.2']) {
      const path = document.createElementNS(ns, 'path'); path.setAttribute('d', d); svg.append(path);
    }
    return svg;
  }
  function threadView(state, place) {
    const { thread, side, line, endLine, outdated } = place;
    const tid = String(thread.id);
    const box = el('div', 'wk-diff-thread');
    box.dataset.thread = tid; box.dataset.side = side;
    box.tabIndex = -1; box.setAttribute('role', 'region');
    const spot = Number.isInteger(line) ? where(side, line, endLine) : '';
    box.setAttribute('aria-label', `${outdated ? 'Outdated comments' : 'Comments'} on ${state.file.path}${spot ? `, ${spot}` : ''}`);
    if (outdated) { box.dataset.outdated = ''; el('p', 'wk-diff-thread-head', `Outdated${spot ? ` · ${spot}` : ''}`, box); }
    if (thread.local) box.dataset.local = '';
    const entries = [...(thread.entries || []), ...(replying.get(thread.id) || [])];
    const resolved = Boolean(thread.resolved);
    const inner = el('div', 'wk-diff-thread-body');
    if (resolved) {
      box.dataset.resolved = '';
      const open = expanded.has(tid);
      const toggle = el('button', 'wk-link wk-diff-thread-toggle', `Resolved · ${plural(entries.length, 'comment', 'comments')}`, box);
      toggle.type = 'button'; toggle.dataset.action = 'toggle';
      toggle.setAttribute('aria-expanded', String(open));
      inner.id = `${id}-thread-${++threadSequence}`;
      toggle.setAttribute('aria-controls', inner.id);
      inner.hidden = !open;
      toggle.addEventListener('click', () => {
        if (expanded.has(tid)) expanded.delete(tid); else expanded.add(tid);
        refreshThread(state, tid);
      });
    }
    box.append(inner);
    const list = el('ol', 'wk-diff-comment-list', null, inner);
    for (const entry of entries) list.append(entryEl(state, thread, entry));
    const draft = replies.get(thread.id);
    if (!thread.local) {
      const acts = el('div', 'wk-diff-thread-actions', null, inner);
      if (spec.onreply && !draft) {
        const b = el('button', 'wk-button wk-button--small', 'Reply', acts);
        b.type = 'button'; b.dataset.action = 'reply';
        b.addEventListener('click', () => openReply(state, thread));
      }
      if (spec.onresolve && !thread.gone) {
        const b = el('button', 'wk-button wk-button--quiet wk-button--small', resolved ? 'Unresolve' : 'Resolve', acts);
        b.type = 'button'; b.dataset.action = 'resolve';
        const job = resolving.get(tid);
        if (job) {
          b.dataset.busy = ''; b.setAttribute('aria-disabled', 'true');
          if (job.shown) { const spin = el('span', 'wk-spinner'); spin.setAttribute('aria-hidden', 'true'); b.prepend(spin); }
        }
        b.addEventListener('click', () => resolve(state, thread, !resolved));
      }
      if (!acts.childElementCount) acts.remove();
      if (draft) inner.append(draft.frame);
    }
    return box;
  }

  // ---- Placing threads -------------------------------------------------
  // Focus stays where it was across a redraw: on the same box, the same
  // thread's same button (or its toggle, or the thread), a thread in the
  // same place, the line, the next thread, the file's toggle, else the diff.
  function keepFocus(state, redraw) {
    const a = document.activeElement;
    let had = null;
    if (a && a !== state.body && state.body.contains(a)) {
      const thread = a.closest('.wk-diff-thread');
      had = { el: a, thread: thread?.dataset.thread, action: a.dataset?.action, anchor: a.closest('.wk-diff-comments')?.previousElementSibling, index: thread ? [...state.body.querySelectorAll('.wk-diff-thread')].indexOf(thread) : -1 };
    }
    redraw();
    if (!had) return;
    if (had.el.isConnected) { if (document.activeElement !== had.el) had.el.focus({ preventScroll: true }); return; }
    const visible = x => x && x.isConnected && x.offsetParent !== null;
    const thread = had.thread != null && threadEl(state, had.thread);
    const threads = [...state.body.querySelectorAll('.wk-diff-thread')].filter(visible);
    const target = (thread && [thread.querySelector(`[data-action="${had.action}"]`), thread.querySelector('[data-action="toggle"]'), thread].find(visible))
      || (had.anchor?.isConnected && had.anchor.nextElementSibling?.classList.contains('wk-diff-comments') && had.anchor.nextElementSibling.querySelector('.wk-diff-thread'))
      || (had.anchor?.isConnected && had.anchor.classList.contains('wk-diff-line') && had.anchor.hasAttribute('tabindex') && had.anchor)
      || (had.index >= 0 && (threads[had.index] || threads.at(-1)))
      || (visible(state.button) && state.button)
      || root;
    target.focus({ preventScroll: true });
  }
  // A row's threads and boxes: those whose line (the range's end) is in it.
  function slotItems(state, row, placed) {
    const items = [];
    for (const place of placed) if (numberOf(row, place.side) === place.endLine) items.push(threadView(state, place));
    for (const draft of drafts.values()) if (draft.state === state && numberOf(row, draft.side) === draft.endLine) items.push(draft.frame);
    return items;
  }
  function fillSlot(row, items) {
    let box = row.nextElementSibling?.classList.contains('wk-diff-comments') ? row.nextElementSibling : null;
    if (!items.length) { box?.remove(); return; }
    if (!box) { box = el('div', 'wk-diff-comments'); row.after(box); }
    box.replaceChildren(...items);
  }
  // The outdated threads, and any box whose line is not shown, at the top.
  function fillOutdated(state, outdated) {
    const items = outdated.map(place => threadView(state, place));
    for (const draft of drafts.values()) if (draft.state === state && !rowFor(state, draft.side, draft.endLine)) items.push(draft.frame);
    let top = state.body.firstElementChild?.classList.contains('wk-diff-outdated') ? state.body.firstElementChild : null;
    if (!items.length) { top?.remove(); return; }
    if (!top) { top = el('div', 'wk-diff-outdated'); state.body.prepend(top); }
    top.replaceChildren(...items);
  }
  // Every thread and box of a built file.
  function placeThreads(state) {
    if (!state.built) return;
    keepFocus(state, () => {
      const { placed, outdated } = diffCommentPlaces(state.file, threadsOf(state));
      const rows = new Set();
      for (const place of placed) { const row = rowFor(state, place.side, place.endLine); if (row) rows.add(row); }
      for (const draft of drafts.values()) if (draft.state === state) { const row = rowFor(state, draft.side, draft.endLine); if (row) rows.add(row); }
      for (const old of state.body.querySelectorAll('.wk-diff-comments')) if (!rows.has(old.previousElementSibling)) old.remove();
      for (const row of rows) fillSlot(row, slotItems(state, row, placed));
      fillOutdated(state, outdated);
    });
  }
  // Only the row under one line, or the one holding a thread.
  function refreshAt(state, side, line) {
    if (!state.built) return;
    const row = rowFor(state, side, line);
    if (!row) { refreshOutdated(state); return; }
    keepFocus(state, () => fillSlot(row, slotItems(state, row, diffCommentPlaces(state.file, threadsOf(state)).placed)));
  }
  function refreshOutdated(state) {
    if (!state.built) return;
    keepFocus(state, () => fillOutdated(state, diffCommentPlaces(state.file, threadsOf(state)).outdated));
  }
  function refreshThread(state, tid) {
    const host = threadEl(state, tid)?.parentElement;
    if (host?.classList.contains('wk-diff-comments')) keepFocus(state, () => fillSlot(host.previousElementSibling, slotItems(state, host.previousElementSibling, diffCommentPlaces(state.file, threadsOf(state)).placed)));
    else if (host) refreshOutdated(state);
    else placeThreads(state);
  }

  /** Replaces the threads (the app's comments) and redraws them. */
  root.setComments = threads => {
    if (destroyed) return;
    comments = Array.isArray(threads) ? threads : [];
    // Copies of comments that have landed go once the app's comments hold them.
    for (const t of [...posting]) if (t.entries[0]?.posted && t.entries[0].covered()) t.entries[0].drop();
    for (const list of replying.values()) for (const e of [...list]) if (e.posted && e.covered()) e.drop();
    // A reply box for a thread that has gone goes with it, unless it has
    // text: then the thread is kept, outdated, until the box is sent or
    // cancelled (or the thread comes back).
    for (const [tid, draft] of [...replies]) {
      const key = String(tid);
      if (comments.some(t => t && sameId(t.id, tid))) { kept.delete(key); continue; }
      if (textOf(draft).trim()) { if (!kept.has(key)) kept.set(key, { ...draft.thread, outdated: true, gone: true }); }
      else close(draft);
    }
    countComments();
    for (const state of states) placeThreads(state);
  };
  /** Closes every open box (destroying injected editors) and stops the clocks. */
  root.destroy = () => {
    for (const draft of [...drafts.values(), ...replies.values()]) close(draft);
    for (const t of timers) clearTimeout(t);
    timers.clear();
    if (clock) clearInterval(clock);
    clock = null;
    destroyed = true;
  };

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
    const talk = el('span', 'wk-diff-comment-count', null, header);
    talk.hidden = true;
    const body = el('div', 'wk-diff-body', null, box);
    body.id = `${fid}-body`;
    body.setAttribute('role', 'group'); body.setAttribute('aria-labelledby', button.id);
    const large = lineCount(file) > collapseAbove || charCount(file) > collapseCharsAbove;
    const state = { file, index, box, button, body, talk, open: file.collapsed != null ? !file.collapsed : !(large || generated || file.binary), built: false };
    button.addEventListener('click', () => { state.open = !state.open; paint(state); });
    paint(state);
    return state;
  });

  countComments();

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
    state.rows = null;
    placeThreads(state);
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
      setBusy(more, hunk.loading);
      moreLabel(file, k, more);
      // The note is not a live region: a failure goes to the window's
      // announcer once (BEHAVIOUR.md, "Announcer").
      el('span', 'wk-diff-more-note', null, row);
      more.addEventListener('click', () => { if (!hunk.loading) showMore(state, hunk, k); });
    }
    el('span', 'wk-diff-hunk-text', hunk.header, row).setAttribute('aria-hidden', 'true');
    return row;
  }

  // Loading: busy, never disabled, so the button keeps focus (BEHAVIOUR.md,
  // "Accessibility"); kit.js drops the presses a busy control gets.
  function setBusy(more, on) {
    if (!more) return;
    if (on) { more.setAttribute('aria-disabled', 'true'); more.setAttribute('data-busy', ''); }
    else { more.removeAttribute('aria-disabled'); more.removeAttribute('data-busy'); }
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
    { const { more, note } = parts(header()); setBusy(more, true); if (note) note.textContent = ''; }
    let got, failed = false;
    try { got = await spec.getContext(file, { oldStart: from, oldEnd: to, newStart: from + cur.shift, newEnd: to + cur.shift }); } catch { failed = true; }
    hunk.loading = false;
    const row = header();
    const { more, note } = parts(row);
    const hadFocus = more && document.activeElement === more;
    setBusy(more, false);
    if (failed) {
      if (note) note.textContent = 'Couldn\u2019t load lines';
      // Assertive only while the person waits on it: focus still on the button.
      announce('Couldn\u2019t load lines', { assertive: hadFocus });
      return;
    }
    const close = () => { more?.remove(); if (hadFocus) row.focus(); };
    if (!Array.isArray(got) || !got.length) { hunk.noMore = true; close(); return; }
    got = got.slice(-(to - from + 1));
    const first = to - got.length + 1;
    const added = got.map((text, j) => ({ kind: 'context', old: first + j, new: first + j + cur.shift, text: String(text) }));
    hunk.lines.unshift(...added);
    hunk.oldStart = first; hunk.oldLines += added.length;
    hunk.newStart = first + cur.shift; hunk.newLines += added.length;
    // A file that is not built picks the lines up when it is.
    if (row) { row.after(...rowsFor(file, added)); state.rows = null; placeThreads(state); }
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
        if (commenting) row.tabIndex = -1;
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
      if (commenting) row.tabIndex = -1;
      for (const [line, side] of [[left, 'old'], [right, 'new']]) {
        const half = el('div', 'wk-diff-half', null, row);
        half.dataset.side = side;
        if (!line) { half.dataset.kind = 'empty'; continue; }
        half.dataset.kind = line.kind;
        if (line[side] != null) half.dataset[side] = String(line[side]);
        cells(file, line, words.get(line), half, side);
        // A context line is on both sides: read it once. Its parts are hidden,
        // not the half, which takes focus and the + button.
        if (line.kind === 'context' && side === 'new') for (const part of half.children) part.setAttribute('aria-hidden', 'true');
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
    // Not while typing: in a field, in anything contenteditable (even a
    // disabled editor, contenteditable="false"), a code editor, or a comment
    // box and anything in it.
    if (e.isComposing || e.keyCode === 229) return;
    if (t.isContentEditable || t.closest?.('textarea, select, input:not([type="radio"], [type="checkbox"], [type="button"]), [contenteditable], .cm-editor, .wk-diff-compose')) return;
    if (commenting && lineKey(e)) return;
    const move = { n: ['.wk-diff-hunk', 1], p: ['.wk-diff-hunk', -1], ']': ['.wk-diff-toggle', 1], '[': ['.wk-diff-toggle', -1], N: ['.wk-diff-thread', 1], P: ['.wk-diff-thread', -1] }[e.key];
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
