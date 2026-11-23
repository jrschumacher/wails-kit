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
