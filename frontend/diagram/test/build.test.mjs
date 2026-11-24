// dist/ is what scripts/build.mjs makes from src/ and node_modules: a small
// diagram.js that loads mermaid.js on first use, nothing from the network,
// nothing a strict CSP blocks, and the licences of everything bundled.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { build, root, KEEP, EMPTY, licenceOf, PATCHES, REFUSED_SHAPES, INLINED, inlinedMarks } from '../scripts/build.mjs';

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
  // @wails-kit/ui's timings are bundled from the kit's source, so the vendored module stands alone.
  assert.ok(built.inputs['diagram.js'].includes('../ui/src/motion.js'), 'diagram.js does not bundle @wails-kit/ui\'s timings');
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
    // The code, without the licence banners esbuild gathers at the end.
    const js = built.files[name].replace(/\/\*! Bundled license information:[\s\S]*\*\/\s*$/, '');
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

test('the build patches name no generated identifier (Mermaid\'s id38 and the like change between builds)', () => {
  for (const [, pairs] of PATCHES) {
    for (const [, to] of pairs) assert.doesNotMatch(to, /\b[a-z]+\d+\b/i, `a patch names a generated identifier: ${to}`);
  }
});

test('the image and icon shapes refuse to draw', () => {
  const js = built.files['mermaid.js'];
  const count = what => (js.match(new RegExp(`\\b[\\w$]+\\("${what}"\\)`, 'g')) || []).length;
  const want = {};
  for (const what of Object.values(REFUSED_SHAPES)) want[what] = (want[what] || 0) + 1;
  for (const [what, n] of Object.entries(want)) assert.equal(count(what), n, `${what}: ${count(what)} refusals in mermaid.js`);
});

test('mermaid.js keeps the libraries\' licence banners', () => {
  const tail = built.files['mermaid.js'].match(/\/\*! Bundled license information:[\s\S]*\*\/\s*$/)?.[0] ?? '';
  assert.match(tail, /@license DOMPurify \d+\.\d+\.\d+ \| \(c\) Cure53/);
  assert.match(tail, /Lodash/);
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
  // And the packages Mermaid's own build inlines, with their licence text.
  for (const [name, { version, license }] of Object.entries(INLINED)) {
    assert.ok(text.includes(`\n${name} ${version} (${license})\nBundled in: mermaid.js (inlined by Mermaid's own build)\n`), `${name} is missing from the licences`);
    assert.ok(!built.packages.includes(name));
  }
  assert.match(text, /Copyright \(C\) 2011-2015 by Vitaly Puzrin/);
  assert.match(text, /Copyright \(c\) 2016 Wilson Page/);
  assert.equal(text.split(/^-{72}\n/m).length, (built.packages.length + built.inlined.length) * 2 + 1);
  assert.match(text, /Permission is hereby granted, free of charge/);
});

test('every package Mermaid inlines into the bundled files has a licence entry', () => {
  // Mermaid marks an inlined package with a path comment naming it (and,
  // for a pnpm path, its version).
  const seen = new Map();
  for (const file of built.inputs['mermaid.js'].filter(f => f.startsWith('node_modules/mermaid/'))) {
    for (const [name, version] of inlinedMarks(read(file))) seen.set(name, version ?? seen.get(name) ?? null);
  }
  assert.ok(seen.size >= 2);
  for (const [name, version] of seen) {
    assert.ok(INLINED[name], `${name} has no INLINED entry`);
    if (version) assert.equal(INLINED[name].version, version, `${name}@${version}: INLINED has ${INLINED[name].version}`);
  }
  // Both path forms are read.
  assert.deepEqual(inlinedMarks('// ../../node_modules/.pnpm/js-yaml@4.3.2/node_modules/js-yaml/dist/js-yaml.mjs\n// ../../node_modules/@scope/pkg/index.js\n// node_modules/fastdom/fastdom.js\n// src/x.ts'),
    [['js-yaml', '4.3.2'], ['@scope/pkg', null], ['fastdom', null]]);
  // And the inlined code is there: js-yaml's error class and fastdom.
  assert.match(built.files['mermaid.js'], /YAMLException/);
  assert.match(built.files['mermaid.js'], /fastdom/);
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
