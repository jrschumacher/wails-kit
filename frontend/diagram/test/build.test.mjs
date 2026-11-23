// dist/ is what scripts/build.mjs makes from src/ and node_modules: a small
// diagram.js that loads mermaid.js on first use, nothing from the network,
// nothing a strict CSP blocks, and the licences of everything bundled.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { build, root, KEEP, EMPTY, licenceOf } from '../scripts/build.mjs';

const built = await build();
const read = p => readFileSync(path.join(root, p), 'utf8');
const MODULES = ['diagram.js', 'mermaid.js'];

test('the committed dist/ matches a fresh build (run npm run build)', () => {
  assert.deepEqual(readdirSync(path.join(root, 'dist')).sort(), Object.keys(built.files).sort(), 'dist/ holds other files than the build makes');
  for (const [name, text] of Object.entries(built.files)) assert.equal(read(`dist/${name}`), text, `dist/${name} is stale`);
});

test('the package exports dist/diagram.js and its types', () => {
  const pkg = JSON.parse(read('package.json'));
  assert.deepEqual(pkg.exports['.'], { types: './dist/diagram.d.ts', default: './dist/diagram.js' });
  assert.deepEqual(Object.keys(pkg.exports).sort(), ['.', './package.json']);
  assert.equal(built.files['diagram.d.ts'], read('src/index.d.ts'));
});

test('diagram.js is small and loads only ./mermaid.js, lazily; mermaid.js imports nothing', () => {
  assert.deepEqual(built.imports['diagram.js'], [{ path: './mermaid.js', kind: 'dynamic-import' }]);
  assert.deepEqual(built.imports['mermaid.js'], []);
  assert.ok(Buffer.byteLength(built.files['diagram.js']) < 40_000, 'diagram.js should stay small: Mermaid belongs in mermaid.js');
  assert.ok(!built.inputs['diagram.js'].some(f => f.includes('node_modules')), 'diagram.js bundles a package');
  assert.equal(built.files['diagram.js'].match(/\bimport\s*\(/g).length, 1);
  assert.ok(!/\bimport\s*\(/.test(built.files['mermaid.js']), 'mermaid.js has a dynamic import');
});

test('each module parses, and neither fetches, evaluates code or starts a worker', () => {
  const dir = mkdtempSync(path.join(tmpdir(), 'wk-diagram-'));
  try {
    for (const name of MODULES) {
      const file = path.join(dir, name.replace(/\.js$/, '.mjs'));
      writeFileSync(file, built.files[name]);
      execFileSync(process.execPath, ['--check', file]);
    }
  } finally { rmSync(dir, { recursive: true, force: true }); }
  for (const name of MODULES) {
    const js = built.files[name];
    for (const api of [/\bfetch\(/, /\beval\(/, /\bFunction\(/, /\bnew Worker\b/, /\bSharedWorker\b/, /\bimportScripts\b/, /\bXMLHttpRequest\b/, /\bWebSocket\b/, /\bEventSource\b/, /\bsendBeacon\b/, /setTimeout\(\s*["'`]/]) {
      assert.ok(!api.test(js), `${name} uses ${api}`);
    }
    // No address to load from (namespace URIs are not requests).
    const urls = [...js.matchAll(/https?:\/\/[^\s"'`)]+/g)].map(m => m[0]).filter(u => !/^http:\/\/www\.w3\.org\//.test(u) && !/github\.com\/markedjs\/marked/.test(u));
    assert.deepEqual(urls, [], `${name} names URLs`);
  }
});

test('mermaid.js holds the six diagram types and dagre, and leaves out the rest', () => {
  const inputs = built.inputs['mermaid.js'];
  for (const chunk of ['flowDiagram', 'sequenceDiagram', 'classDiagram-v2', 'stateDiagram-v2', 'erDiagram', 'ganttDiagram', 'dagre']) {
    assert.ok(inputs.some(f => new RegExp(`/${chunk}-[A-Z0-9]+\\.mjs$`).test(f)), `${chunk} is not bundled`);
  }
  const chunks = inputs.filter(f => /mermaid\/dist\/chunks\/mermaid\.core\/(?!chunk-)/.test(f));
  for (const f of chunks) assert.ok(KEEP.test(`./${path.basename(f)}`), `${path.basename(f)} is bundled but not kept`);
  for (const pkg of ['katex', 'cytoscape', 'elkjs', 'cytoscape-cose-bilkent', 'cytoscape-fcose', ...EMPTY]) {
    assert.ok(!built.packages.includes(pkg), `${pkg} is bundled`);
  }
  for (const f of inputs.filter(i => i.startsWith('wk-stub:'))) assert.ok(!KEEP.test(f.slice(8)), f);
});

test('Mermaid\'s styling is patched to go through src/mermaid/csp.js', () => {
  const js = built.files['mermaid.js'];
  assert.ok(built.inputs['mermaid.js'].includes('src/mermaid/csp.js'));
  // One <style> element and one style attribute are made directly: csp.js's
  // fallbacks for a browser without constructable stylesheets and an
  // element without a style property.
  assert.equal(js.match(/createElement\("style"\)/g)?.length, 1, 'Mermaid makes a <style> element');
  assert.equal(js.match(/setAttribute\("style"/g)?.length, 1, 'a style attribute is set directly');
  assert.ok(js.includes('data-wk-style'), 'style attributes are not stashed before DOMPurify');
  // The build left node_modules as it was.
  assert.ok(read('node_modules/mermaid/dist/mermaid.core.mjs').includes('const style1 = document.createElement("style");'));
});

test('THIRD-PARTY-LICENSES.txt names every bundled package with its version, licence and licence text', () => {
  const text = built.files['THIRD-PARTY-LICENSES.txt'];
  const allowed = new Set(['MIT', 'ISC', 'BSD-2-Clause', 'BSD-3-Clause', 'Apache-2.0', '0BSD', '(MPL-2.0 OR Apache-2.0)']);
  assert.ok(built.packages.includes('mermaid') && built.packages.includes('dompurify'));
  for (const name of built.packages) {
    const meta = JSON.parse(read(`node_modules/${name}/package.json`));
    const licence = licenceOf(name, meta);
    assert.ok(allowed.has(licence), `${name} is ${licence}`);
    assert.ok(text.includes(`\n${name} ${meta.version} (${licence})\n`), `${name} is missing from the licences`);
  }
  assert.equal(text.split(/^-{72}\n/m).length, built.packages.length * 2 + 1);
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
