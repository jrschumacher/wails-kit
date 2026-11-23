// dist/ is what scripts/build.mjs makes from src/ and node_modules: one
// standalone module per entry, no imports between entries, and the licences
// of everything bundled.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { build, root, ENTRIES, STYLE_MOD_PATCH } from '../scripts/build.mjs';

const built = await build();
const read = p => readFileSync(path.join(root, p), 'utf8');

test('the committed dist/ matches a fresh build (run npm run build)', () => {
  assert.deepEqual(readdirSync(path.join(root, 'dist')).sort(), Object.keys(built.files).sort(), 'dist/ holds other files than the build makes');
  for (const [name, text] of Object.entries(built.files)) assert.equal(read(`dist/${name}`), text, `dist/${name} is stale`);
});

test('every entry is a subpath export pointing at its dist module and types', () => {
  const pkg = JSON.parse(read('package.json'));
  for (const name of ENTRIES) {
    assert.deepEqual(pkg.exports[`./${name}`], { types: `./dist/${name}.d.ts`, default: `./dist/${name}.js` });
  }
  assert.deepEqual(Object.keys(pkg.exports).filter(k => k !== './package.json').sort(), ENTRIES.map(n => `./${n}`).sort());
});

test('each dist module parses, imports nothing and loads nothing at run time', () => {
  const dir = mkdtempSync(path.join(tmpdir(), 'wk-editor-'));
  try {
    for (const name of ENTRIES) {
      const file = path.join(dir, `${name}.mjs`);
      writeFileSync(file, built.files[`${name}.js`]);
      execFileSync(process.execPath, ['--check', file]);
    }
  } finally { rmSync(dir, { recursive: true, force: true }); }
  for (const name of ENTRIES) {
    const js = built.files[`${name}.js`];
    assert.deepEqual(built.imports[name], [], `${name}.js imports something`);
    assert.ok(!/\bimport\s*\(/.test(js), `${name}.js has a dynamic import`);
    for (const api of [/\bnew Worker\b/, /\bSharedWorker\b/, /\bimportScripts\b/, /\bfetch\(/, /\bXMLHttpRequest\b/, /\bWebSocket\b/, /\bEventSource\b/, /\bsendBeacon\b/]) {
      assert.ok(!api.test(js), `${name}.js uses ${api}`);
    }
  }
});

test('the entries do not import each other', () => {
  for (const name of ENTRIES) {
    const own = built.inputs[name].filter(f => f.startsWith('src/'));
    assert.ok(own.includes(`src/${name}/index.js`));
    for (const file of own) {
      assert.ok(file.startsWith(`src/${name}/`) || file.startsWith('src/shared/'), `${name}.js bundles ${file}, from another entry`);
    }
  }
});

test('the style-mod patch is applied, so CodeMirror styles pass a CSP without inline styles', () => {
  assert.ok(!readFileSync(path.join(root, 'node_modules/style-mod/src/style-mod.js'), 'utf8').includes(STYLE_MOD_PATCH.to));
  for (const name of ENTRIES) {
    // Minified, the patched test reads `if(X.adoptedStyleSheets&&Y.CSSStyleSheet)` with no `!X.head` before it.
    const js = built.files[`${name}.js`];
    assert.match(js, /if\(\w+\.adoptedStyleSheets&&\w+\.CSSStyleSheet\)/, `${name}.js: style-mod is not patched`);
    assert.doesNotMatch(js, /!\w+\.head&&\w+\.adoptedStyleSheets/, `${name}.js: style-mod still writes a <style> element on a document`);
  }
});

test('THIRD-PARTY-LICENSES.txt names every bundled package with its version, licence and licence text', () => {
  const text = built.files['THIRD-PARTY-LICENSES.txt'];
  const allowed = new Set(['MIT', 'ISC', 'BSD-2-Clause', 'BSD-3-Clause', 'Apache-2.0', '0BSD']);
  assert.ok(built.packages.length > 10 && built.packages.includes('@codemirror/view'), 'CodeMirror is not bundled');
  for (const name of built.packages) {
    const meta = JSON.parse(read(`node_modules/${name}/package.json`));
    assert.ok(allowed.has(meta.license), `${name} is ${meta.license}`);
    assert.ok(text.includes(`\n${name} ${meta.version} (${meta.license})\n`), `${name} is missing from the licences`);
  }
  const sections = text.split(/^-{72}\n/m).length;
  assert.equal(sections, built.packages.length * 2 + 1);
  assert.match(text, /Permission is hereby granted, free of charge/);
});

test('dependencies are pinned exactly and locked', () => {
  const pkg = JSON.parse(read('package.json'));
  const lock = JSON.parse(read('package-lock.json'));
  assert.deepEqual(pkg.dependencies ?? {}, {}, 'everything is bundled at build time: devDependencies only');
  for (const [name, version] of Object.entries(pkg.devDependencies)) {
    assert.match(version, /^\d+\.\d+\.\d+$/, `${name} is not pinned exactly`);
    assert.equal(lock.packages[`node_modules/${name}`]?.version, version, `${name} is not locked at ${version}`);
  }
});
