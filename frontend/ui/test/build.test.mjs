// dist/ is what scripts/build.mjs makes from src/, and the one-file module
// exports exactly what src/index.js exports.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { buildCss, buildJs } from '../scripts/build.mjs';

const root = fileURLToPath(new URL('../', import.meta.url));
const read = p => readFileSync(path.join(root, p), 'utf8');

test('the committed dist/ matches a fresh build (run npm run build)', () => {
  assert.equal(read('dist/wails-kit-ui.css'), buildCss(), 'dist/wails-kit-ui.css is stale');
  assert.equal(read('dist/wails-kit-ui.js'), buildJs(), 'dist/wails-kit-ui.js is stale');
});

test('the built module parses, imports nothing, and exports what src/index.js exports', () => {
  const dir = mkdtempSync(path.join(tmpdir(), 'wk-ui-'));
  try {
    const file = path.join(dir, 'wails-kit-ui.mjs');
    writeFileSync(file, buildJs());
    execFileSync(process.execPath, ['--check', file]);
  } finally { rmSync(dir, { recursive: true, force: true }); }
  const js = buildJs();
  assert.ok(!/^\s*import\s/m.test(js) && !/\bimport\s*\(/.test(js), 'dist module imports something');
  const names = text => {
    const out = new Set();
    for (const [, list] of text.matchAll(/^export\s*\{([^}]*)\}/gm)) for (const n of list.split(',')) if (n.trim()) out.add(n.trim().split(/\s+as\s+/).pop());
    for (const [, n] of text.matchAll(/^export\s+(?:async\s+)?(?:function\*?|class|const|let)\s+([\w$]+)/gm)) out.add(n);
    return [...out].sort();
  };
  assert.deepEqual(names(js), names(read('src/index.js')));
});

test('the CSS bundle holds every imported sheet, in order, and no @import', () => {
  const css = buildCss();
  assert.ok(!/@import/.test(css));
  const order = ['tokens.css', 'base.css', 'components.css', 'settings.css'].map(f => css.indexOf(`---- ${f} ----`));
  assert.ok(order.every(i => i >= 0) && order.every((i, k) => k === 0 || i > order[k - 1]), `sheets out of order: ${order}`);
});
