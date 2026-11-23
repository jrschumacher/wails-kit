// The diff's rules that need no DOM (src/diff.js): the parser, the split
// view's rows and the word-level marks.
import test from 'node:test';
import assert from 'node:assert/strict';
import { parseUnifiedDiff, diffSplitRows, diffWordRanges } from '../src/diff.js';

const lines = (file, k = 0) => file.hunks[k].lines.map(l => [l.kind, l.old, l.new, l.text]);

const multi = `From 1234 Mon Sep 17 00:00:00 2001
Subject: [PATCH] a commit message before the first file

diff --git a/src/app.js b/src/app.js
index 83db48f..bf269f4 100644
--- a/src/app.js
+++ b/src/app.js
@@ -1,4 +1,5 @@ export function start() {
 const a = 1;
-const b = 2;
+const b = 3;
+const c = 4;
 const d = 5;
 const e = 6;
@@ -20,3 +21,2 @@ function stop() {
 one
-two
 three
diff --git a/docs/new.md b/docs/new.md
new file mode 100644
index 0000000..e69de29
--- /dev/null
+++ b/docs/new.md
@@ -0,0 +1,2 @@
+# Title
+--- a line that looks like a header
diff --git a/old.txt b/old.txt
deleted file mode 100644
index e69de29..0000000
--- a/old.txt
+++ /dev/null
@@ -1 +0,0 @@
-gone
`;

test('a git diff with several files: paths, statuses, counts and line numbers', () => {
  const files = parseUnifiedDiff(multi);
  assert.deepEqual(files.map(f => [f.path, f.oldPath, f.status, f.additions, f.deletions]), [
    ['src/app.js', 'src/app.js', 'modified', 2, 2],
    ['docs/new.md', 'docs/new.md', 'added', 2, 0],
    ['old.txt', 'old.txt', 'deleted', 0, 1],
  ]);
  const [app, added, deleted] = files;
  assert.equal(app.hunks.length, 2);
  assert.deepEqual(app.hunks.map(h => [h.oldStart, h.oldLines, h.newStart, h.newLines, h.section]), [[1, 4, 1, 5, 'export function start() {'], [20, 3, 21, 2, 'function stop() {']]);
  assert.equal(app.hunks[0].header, '@@ -1,4 +1,5 @@ export function start() {');
  assert.deepEqual(lines(app), [
    ['context', 1, 1, 'const a = 1;'], ['del', 2, null, 'const b = 2;'], ['add', null, 2, 'const b = 3;'], ['add', null, 3, 'const c = 4;'],
    ['context', 3, 4, 'const d = 5;'], ['context', 4, 5, 'const e = 6;'],
  ]);
  assert.deepEqual(lines(app, 1), [['context', 20, 21, 'one'], ['del', 21, null, 'two'], ['context', 22, 22, 'three']]);
  assert.deepEqual(lines(added), [['add', null, 1, '# Title'], ['add', null, 2, '--- a line that looks like a header']], 'a hunk reads its counted lines, whatever they start with');
  assert.equal(added.newMode, '100644');
  assert.deepEqual(lines(deleted), [['del', 1, null, 'gone']]);
});

test('a rename, with and without changes, and a path with spaces', () => {
  const files = parseUnifiedDiff(`diff --git a/lib/old name.go b/lib/new name.go
similarity index 92%
rename from lib/old name.go
rename to lib/new name.go
index 1111111..2222222 100644
--- a/lib/old name.go
+++ b/lib/new name.go
@@ -3 +3 @@
-x := 1
+x := 2
diff --git a/a.txt b/b.txt
similarity index 100%
rename from a.txt
rename to b.txt
diff --git a/same name.txt b/same name.txt
old mode 100644
new mode 100755
`);
  assert.deepEqual(files.map(f => [f.oldPath, f.path, f.status, f.hunks.length]), [
    ['lib/old name.go', 'lib/new name.go', 'renamed', 1],
    ['a.txt', 'b.txt', 'renamed', 0],
    ['same name.txt', 'same name.txt', 'modified', 0],
  ]);
  assert.deepEqual([files[2].oldMode, files[2].newMode], ['100644', '100755']);
});

test('binary files: "Binary files … differ" and a GIT binary patch', () => {
  const files = parseUnifiedDiff(`diff --git a/logo.png b/logo.png
new file mode 100644
index 0000000..d1e2f3a
Binary files /dev/null and b/logo.png differ
diff --git a/icon.ico b/icon.ico
index 1234567..89abcde 100644
GIT binary patch
literal 12
TcmZ?wbhEHbRA2y--@-x

literal 0
HcmV?d00001

diff --git a/b.txt b/b.txt
--- a/b.txt
+++ b/b.txt
@@ -1 +1 @@
-a
+b
`);
  assert.deepEqual(files.map(f => [f.path, f.status, f.binary, f.hunks.length]), [
    ['logo.png', 'added', true, 0], ['icon.ico', 'modified', true, 0], ['b.txt', 'modified', false, 1],
  ]);
});

test('"\\ No newline at end of file" marks the line before it, on either side', () => {
  const [file] = parseUnifiedDiff(`--- a/n.txt
+++ b/n.txt
@@ -1,2 +1,2 @@
 keep
-last
\\ No newline at end of file
+last
`);
  assert.deepEqual(lines(file), [['context', 1, 1, 'keep'], ['del', 2, null, 'last'], ['add', null, 2, 'last']]);
  assert.deepEqual(file.hunks[0].lines.map(l => Boolean(l.noNewline)), [false, true, false]);
  assert.equal(file.path, 'n.txt', 'the a/ and b/ prefixes go');
});

test('CRLF line endings parse like LF, and a context line stripped to nothing still counts', () => {
  const lf = 'diff --git a/w.txt b/w.txt\n--- a/w.txt\n+++ b/w.txt\n@@ -1,3 +1,3 @@\n one\n\n-two\n+2\n';
  assert.deepEqual(parseUnifiedDiff(lf.replace(/\n/g, '\r\n')), parseUnifiedDiff(lf));
  assert.deepEqual(lines(parseUnifiedDiff(lf)[0]), [['context', 1, 1, 'one'], ['context', 2, 2, ''], ['del', 3, null, 'two'], ['add', null, 3, '2']]);
});

test('empty hunks, an empty diff, and a plain diff -u with several files', () => {
  assert.deepEqual(parseUnifiedDiff(''), []);
  assert.deepEqual(parseUnifiedDiff(null), []);
  const [empty] = parseUnifiedDiff('diff --git a/e b/e\n--- a/e\n+++ b/e\n@@ -0,0 +0,0 @@\n');
  assert.deepEqual(empty.hunks.map(h => [h.oldStart, h.oldLines, h.newStart, h.newLines, h.lines.length]), [[0, 0, 0, 0, 0]]);
  const plain = parseUnifiedDiff('--- one.txt\t2026-01-01 10:00:00\n+++ one.txt\t2026-01-02 10:00:00\n@@ -1 +1 @@\n-a\n+b\n--- two.txt\n+++ two.txt\n@@ -1 +1,2 @@\n x\n+y\n');
  assert.deepEqual(plain.map(f => [f.path, f.additions, f.deletions]), [['one.txt', 1, 1], ['two.txt', 1, 0]]);
});

test('quoted paths with escapes', () => {
  const [file] = parseUnifiedDiff('diff --git "a/caf\\303\\251 \\"x\\".txt" "b/caf\\303\\251 \\"x\\".txt"\n--- "a/caf\\303\\251 \\"x\\".txt"\n+++ "b/caf\\303\\251 \\"x\\".txt"\n@@ -1 +1 @@\n-a\n+b\n');
  assert.equal(file.path, 'café "x".txt');
  const [bare] = parseUnifiedDiff('diff --git "a/sp ace.bin" "b/sp ace.bin"\nBinary files "a/sp ace.bin" and "b/sp ace.bin" differ\n');
  assert.deepEqual([bare.path, bare.binary], ['sp ace.bin', true], 'the diff --git line alone names the file');
});

test('the split view pairs a run of removed lines with the added lines after it', () => {
  const [file] = parseUnifiedDiff(multi);
  const side = l => l && `${l.kind}:${l.text}`;
  assert.deepEqual(diffSplitRows(file.hunks[0].lines).map(r => [side(r.left), side(r.right)]), [
    ['context:const a = 1;', 'context:const a = 1;'],
    ['del:const b = 2;', 'add:const b = 3;'],
    [null, 'add:const c = 4;'],
    ['context:const d = 5;', 'context:const d = 5;'],
    ['context:const e = 6;', 'context:const e = 6;'],
  ]);
  assert.deepEqual(diffSplitRows(file.hunks[1].lines).map(r => [side(r.left), side(r.right)]), [
    ['context:one', 'context:one'], ['del:two', null], ['context:three', 'context:three'],
  ]);
  // Every line appears once on its side.
  const all = file.hunks.flatMap(h => h.lines);
  const rows = file.hunks.flatMap(h => diffSplitRows(h.lines));
  assert.equal(rows.filter(r => r.left).length, all.filter(l => l.kind !== 'add').length);
  assert.equal(rows.filter(r => r.right).length, all.filter(l => l.kind !== 'del').length);
});

test('word marks: what changed between a removed line and the added line that replaces it', () => {
  const cut = (text, ranges) => ranges.map(([s, e]) => text.slice(s, e));
  const pair = (a, b) => { const r = diffWordRanges(a, b); return [cut(a, r.del), cut(b, r.add)]; };
  assert.deepEqual(pair('const b = 2;', 'const b = 3;'), [['2'], ['3']]);
  assert.deepEqual(pair('return a + b', 'return c + d'), [['a', 'b'], ['c', 'd']]);
  assert.deepEqual(pair('let total = price', 'let grand total = price'), [[], ['grand ']], 'an insertion marks only the new words');
  assert.deepEqual(pair('if (ready && open)', 'if (ready && open && visible)'), [[], [' && visible']]);
  assert.deepEqual(pair('a b c d', 'a x y d'), [['b c'], ['x y']], 'changed words with spaces between them read as one change');
  assert.deepEqual(pair('  indent', '    indent'), [['  '], ['    ']]);
  assert.deepEqual(diffWordRanges('alpha beta', 'gamma delta'), { del: [], add: [] }, 'nothing in common: the whole line changed');
  assert.deepEqual(diffWordRanges('same', 'same'), { del: [], add: [] });
  assert.deepEqual(pair('naïve café', 'naïve cafés'), [['café'], ['cafés']], 'words are Unicode words');
  // Above the limit only the common start and end are kept.
  const long = Array.from({ length: 400 }, (_, i) => `w${i}`).join(' ');
  const r = diffWordRanges(`${long} end`, `${long.replace('w200', 'x')} end`, 100);
  assert.deepEqual(cut(`${long} end`, r.del), ['w200']);
});

// Real `git diff --cached` output for one change (a binary edit, a rename
// with a mode change and an edit, a deleted file with a space in its name, an
// added file whose name needs quoting), with each kind of path prefix git
// can write: a/ b/, --no-prefix, diff.mnemonicPrefix (c/ i/), and
// --src-prefix=SRC/ --dst-prefix=DST/.
const GIT = {
  default: `diff --git a/b.png b/b.png
index f584f40..6bf43ff 100644
Binary files a/b.png and b/b.png differ
diff --git a/foo.txt b/moved.txt
old mode 100644
new mode 100755
similarity index 50%
rename from foo.txt
rename to moved.txt
index 422c2b7..0f7bc76
--- a/foo.txt
+++ b/moved.txt
@@ -1,2 +1,2 @@
 a
-b
+c
diff --git a/sp ace.txt b/sp ace.txt
deleted file mode 100644
index 587be6b..0000000
--- a/sp ace.txt\t
+++ /dev/null
@@ -1 +0,0 @@
-x
diff --git "a/tab\\t\\303\\251.txt" "b/tab\\t\\303\\251.txt"
new file mode 100644
index 0000000..77811bc
--- /dev/null
+++ "b/tab\\t\\303\\251.txt"
@@ -0,0 +1,2 @@
+y
+y2
`,
  noPrefix: `diff --git b.png b.png
index f584f40..6bf43ff 100644
Binary files b.png and b.png differ
diff --git foo.txt moved.txt
old mode 100644
new mode 100755
similarity index 50%
rename from foo.txt
rename to moved.txt
index 422c2b7..0f7bc76
--- foo.txt
+++ moved.txt
@@ -1,2 +1,2 @@
 a
-b
+c
diff --git sp ace.txt sp ace.txt
deleted file mode 100644
index 587be6b..0000000
--- sp ace.txt\t
+++ /dev/null
@@ -1 +0,0 @@
-x
diff --git "tab\\t\\303\\251.txt" "tab\\t\\303\\251.txt"
new file mode 100644
index 0000000..77811bc
--- /dev/null
+++ "tab\\t\\303\\251.txt"
@@ -0,0 +1,2 @@
+y
+y2
`,
  mnemonic: `diff --git c/b.png i/b.png
index f584f40..6bf43ff 100644
Binary files c/b.png and i/b.png differ
diff --git c/foo.txt i/moved.txt
old mode 100644
new mode 100755
similarity index 50%
rename from foo.txt
rename to moved.txt
index 422c2b7..0f7bc76
--- c/foo.txt
+++ i/moved.txt
@@ -1,2 +1,2 @@
 a
-b
+c
diff --git c/sp ace.txt i/sp ace.txt
deleted file mode 100644
index 587be6b..0000000
--- c/sp ace.txt\t
+++ /dev/null
@@ -1 +0,0 @@
-x
diff --git "c/tab\\t\\303\\251.txt" "i/tab\\t\\303\\251.txt"
new file mode 100644
index 0000000..77811bc
--- /dev/null
+++ "i/tab\\t\\303\\251.txt"
@@ -0,0 +1,2 @@
+y
+y2
`,
  custom: `diff --git SRC/b.png DST/b.png
index f584f40..6bf43ff 100644
Binary files SRC/b.png and DST/b.png differ
diff --git SRC/foo.txt DST/moved.txt
old mode 100644
new mode 100755
similarity index 50%
rename from foo.txt
rename to moved.txt
index 422c2b7..0f7bc76
--- SRC/foo.txt
+++ DST/moved.txt
@@ -1,2 +1,2 @@
 a
-b
+c
diff --git SRC/sp ace.txt DST/sp ace.txt
deleted file mode 100644
index 587be6b..0000000
--- SRC/sp ace.txt\t
+++ /dev/null
@@ -1 +0,0 @@
-x
diff --git "SRC/tab\\t\\303\\251.txt" "DST/tab\\t\\303\\251.txt"
new file mode 100644
index 0000000..77811bc
--- /dev/null
+++ "DST/tab\\t\\303\\251.txt"
@@ -0,0 +1,2 @@
+y
+y2
`,
};

test('real git output: every prefix style gives the same paths', () => {
  for (const [style, text] of Object.entries(GIT)) {
    const files = parseUnifiedDiff(text);
    assert.deepEqual(files.map(f => [f.oldPath, f.path, f.status, f.binary]), [
      ['b.png', 'b.png', 'modified', true],
      ['foo.txt', 'moved.txt', 'renamed', false],
      ['sp ace.txt', 'sp ace.txt', 'deleted', false],
      ['tab\té.txt', 'tab\té.txt', 'added', false],
    ], style);
    assert.deepEqual([files[1].oldMode, files[1].newMode], ['100644', '100755'], style);
    assert.deepEqual(lines(files[2]), [['del', 1, null, 'x']], style);
  }
});

test('plain diffs: prefixes seen on both sides are dropped, and reused for a /dev/null side; no pattern, no change', () => {
  const hg = parseUnifiedDiff('--- a/one.txt\n+++ b/one.txt\n@@ -1 +1 @@\n-a\n+b\n--- /dev/null\n+++ b/two.txt\n@@ -0,0 +1 @@\n+c\n--- a/three.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-d\n');
  assert.deepEqual(hg.map(f => [f.path, f.status]), [['one.txt', 'modified'], ['two.txt', 'added'], ['three.txt', 'deleted']]);
  const ru = parseUnifiedDiff('--- old/src/x.c\t2026-01-01\n+++ new/src/x.c\t2026-01-02\n@@ -1 +1 @@\n-a\n+b\n');
  assert.equal(ru[0].path, 'src/x.c', 'diff -ru old new');
  const same = parseUnifiedDiff('--- src/x.c\n+++ src/x.c\n@@ -1 +1 @@\n-a\n+b\n');
  assert.equal(same[0].path, 'src/x.c', 'equal paths have no prefix');
  const lone = parseUnifiedDiff('--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1 @@\n+c\n');
  assert.equal(lone[0].path, 'b/new.txt', 'with nothing to learn a prefix from, the path stays as written');
  const two = parseUnifiedDiff('--- left.txt\n+++ right.txt\n@@ -1 +1 @@\n-a\n+b\n');
  assert.deepEqual([two[0].path, two[0].status], ['right.txt', 'modified']);
});

test('the split view advances past a line of an unknown kind', () => {
  assert.deepEqual(diffSplitRows([{ kind: 'weird', text: '?' }]), [{ left: { kind: 'weird', text: '?' }, right: { kind: 'weird', text: '?' } }]);
  assert.equal(diffSplitRows([{ kind: 'added' }, { kind: 'del' }, { kind: undefined }]).length, 3);
});

test('word marks keep combining marks, emoji sequences and flags whole, and skip very long lines', () => {
  const cut = (text, ranges) => ranges.map(([s, e]) => text.slice(s, e));
  const pair = (a, b) => { const r = diffWordRanges(a, b); return [cut(a, r.del), cut(b, r.add)]; };
  assert.deepEqual(pair('cafe\u0301 au lait', 'cafe au lait'), [['cafe\u0301'], ['cafe']], 'a combining accent stays with its letter');
  assert.deepEqual(pair('\u{1F468}\u200D\u{1F469}\u200D\u{1F467} ok', '\u{1F468}\u200D\u{1F469}\u200D\u{1F466} ok'), [['\u{1F468}\u200D\u{1F469}\u200D\u{1F467}'], ['\u{1F468}\u200D\u{1F469}\u200D\u{1F466}']], 'a ZWJ sequence is one token');
  assert.deepEqual(pair('\u{1F44D}\u{1F3FD} ok', '\u{1F44D}\u{1F3FF} ok'), [['\u{1F44D}\u{1F3FD}'], ['\u{1F44D}\u{1F3FF}']], 'a skin tone stays with its emoji');
  assert.deepEqual(pair('\u{1F1EB}\u{1F1F7} ok', '\u{1F1E9}\u{1F1EA} ok'), [['\u{1F1EB}\u{1F1F7}'], ['\u{1F1E9}\u{1F1EA}']], 'a flag is one token');
  const long = 'x '.repeat(6000);
  assert.deepEqual(diffWordRanges(`${long}a`, `${long}b`), { del: [], add: [] }, 'over 10,000 characters: no marks');
});
