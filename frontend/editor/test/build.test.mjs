// dist/ is what scripts/build.mjs makes from src/ and node_modules: one
// module per entry, sharing dist/chunk-*.js files they import by relative
// path and nothing else, no source shared between entries outside
// src/shared/, and the licences of everything bundled.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { build, root, ENTRIES, CHUNK, STYLE_MOD_PATCH } from '../scripts/build.mjs';

const built = await build();
const read = p => readFileSync(path.join(root, p), 'utf8');

test('the committed dist/ matches a fresh build (run npm run build)', () => {
  assert.ok(Object.keys(built.files).some(f => CHUNK.test(f)), 'the build made no shared chunk');
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

test('each dist module parses, imports only dist/ chunks by relative path, and loads nothing else at run time', () => {
  const scripts = Object.keys(built.files).filter(f => f.endsWith('.js'));
  assert.deepEqual(scripts.filter(f => !CHUNK.test(f)).sort(), ENTRIES.map(n => `${n}.js`).sort());
  const dir = mkdtempSync(path.join(tmpdir(), 'wk-editor-'));
  try {
    for (const file of scripts) {
      const target = path.join(dir, file.replace(/\.js$/, '.mjs'));
      writeFileSync(target, built.files[file]);
      execFileSync(process.execPath, ['--check', target]);
    }
  } finally { rmSync(dir, { recursive: true, force: true }); }
  for (const file of scripts) {
    const js = built.files[file];
    for (const i of built.imports[file]) {
      assert.match(i, /^dist\/chunk-[A-Z0-9]+\.js$/, `${file} imports ${i}`);
      assert.ok(path.basename(i) in built.files, `${file} imports ${i}, which the build did not make`);
    }
    // In the text: static imports of ./chunk-*.js only.
    for (const [, ref] of js.matchAll(/(?:^|[;}\n])\s*(?:import|export)\s*(?:\{[^}]*\}\s*from\s*|\*\s*as\s+\w+\s*from\s*)?"([^"]+)"/g)) assert.match(ref, /^\.\/chunk-[A-Z0-9]+\.js$/, `${file} imports ${ref}`);
    assert.ok(!/\bimport\s*\(/.test(js), `${file} has a dynamic import`);
    for (const api of [/\bnew Worker\b/, /\bSharedWorker\b/, /\bimportScripts\b/, /\bfetch\(/, /\bXMLHttpRequest\b/, /\bWebSocket\b/, /\bEventSource\b/, /\bsendBeacon\b/]) {
      assert.ok(!api.test(js), `${file} uses ${api}`);
    }
  }
});

test('the entries share one copy of CodeMirror in a chunk; no entry bundles another entry\'s source', () => {
  const owner = input => Object.keys(built.inputs).filter(f => built.inputs[f].includes(input));
  for (const shared of ['node_modules/@codemirror/state/dist/index.js', 'node_modules/@codemirror/view/dist/index.js', 'node_modules/style-mod/src/style-mod.js']) {
    const files = owner(shared);
    assert.equal(files.length, 1, `${shared} is in ${files.join(', ')}`);
    assert.match(files[0], CHUNK, `${shared} is in an entry, not a shared chunk`);
    for (const name of ENTRIES) assert.ok(built.loads[name].includes(files[0]), `${name}.js does not load ${shared}`);
  }
  for (const name of ENTRIES) {
    const own = built.loads[name].flatMap(f => built.inputs[f]).filter(f => f.startsWith('src/'));
    assert.ok(own.includes(`src/${name}/index.js`));
    for (const file of own) {
      assert.ok(file.startsWith(`src/${name}/`) || file.startsWith('src/shared/'), `${name}.js loads ${file}, from another entry`);
    }
  }
  // marked is only the Markdown editor's.
  assert.ok(!built.loads.code.some(f => built.inputs[f].some(i => i.includes('node_modules/marked/'))), 'code.js loads marked');
});

test('the style-mod patch is applied, so CodeMirror styles pass a CSP without inline styles, in every entry', () => {
  assert.ok(!readFileSync(path.join(root, 'node_modules/style-mod/src/style-mod.js'), 'utf8').includes(STYLE_MOD_PATCH.to));
  const files = Object.keys(built.inputs).filter(f => built.inputs[f].some(i => i.includes('node_modules/style-mod/')));
  assert.equal(files.length, 1);
  // Minified, the patched test reads `if(X.adoptedStyleSheets&&Y.CSSStyleSheet)` with no `!X.head` before it.
  const js = built.files[files[0]];
  assert.match(js, /if\(\w+\.adoptedStyleSheets&&\w+\.CSSStyleSheet\)/, `${files[0]}: style-mod is not patched`);
  assert.doesNotMatch(js, /!\w+\.head&&\w+\.adoptedStyleSheets/, `${files[0]}: style-mod still writes a <style> element on a document`);
  for (const name of ENTRIES) assert.ok(built.loads[name].includes(files[0]), `${name}.js does not load the patched style-mod`);
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
  for (const name of built.packages) {
    const loaders = ENTRIES.filter(e => built.loads[e].some(f => built.inputs[f].some(i => i.includes(`node_modules/${name}/`))));
    assert.ok(text.includes(`\nLoaded by: ${loaders.map(e => `${e}.js`).join(', ')}\n`), `${name}: wrong "Loaded by"`);
  }
  assert.match(text, /\nmarked [\d.]+ \(MIT\)\nLoaded by: markdown\.js\n/);
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
