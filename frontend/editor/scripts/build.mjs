#!/usr/bin/env node
// Builds dist/ for apps to vendor: one ES module per editor entry, with its
// libraries bundled (esbuild, minified, code splitting), so it loads nothing
// at run time beyond dist/'s own files.
//   dist/<entry>.js                one module per entry in ENTRIES
//   dist/chunk-<hash>.js           code the entries share (CodeMirror, the
//                                  theme, the languages), imported by the
//                                  entries with relative ./chunk-*.js imports,
//                                  so an app that loads two editors loads one
//                                  CodeMirror
//   dist/<entry>.d.ts              its TypeScript types (copied from src/)
//   dist/THIRD-PARTY-LICENSES.txt  the licence of every bundled package,
//                                  read from node_modules, and the entries
//                                  that load it
// Usage: node scripts/build.mjs [outDir]  (default: dist). It removes stale
// dist/chunk-*.js files.
// test/build.test.mjs builds in memory and checks that the committed dist/
// matches.
import { readFileSync, writeFileSync, mkdirSync, readdirSync, existsSync, unlinkSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import * as esbuild from 'esbuild';

export const root = fileURLToPath(new URL('..', import.meta.url));
const pkg = JSON.parse(readFileSync(path.join(root, 'package.json'), 'utf8'));

// One entry per editor: dist/<name>.js from src/<name>/index.js. An entry
// may import src/shared/ but never another entry's folder.
export const ENTRIES = ['code', 'markdown'];

const banner = `/* ${pkg.name} ${pkg.version}. Built by scripts/build.mjs from src/; do not edit. `
  + 'Bundles third-party code; see THIRD-PARTY-LICENSES.txt beside this file. */';
export const CHUNK = /^chunk-[A-Z0-9]+\.js$/;

// CodeMirror injects its styles through style-mod, which on a document
// writes a <style> element: a page whose CSP has no 'unsafe-inline' style-src
// (the kit's pages use default-src 'self') blocks it, and the editor renders
// unstyled. Constructable stylesheets adopted by the document are not inline
// styles, and style-mod already uses them for shadow roots, so the build lets
// it use them for the document too (Chrome/WebView2 73+, Safari 16.4+). A
// browser without them still gets the <style> element. The build fails if
// style-mod changes and the line is gone.
export const STYLE_MOD_PATCH = {
  from: 'if (!root.head && root.adoptedStyleSheets && win.CSSStyleSheet) {',
  to: 'if (root.adoptedStyleSheets && win.CSSStyleSheet) { // patched by @wails-kit/editor scripts/build.mjs (CSP)',
};
const adoptStyles = {
  name: 'style-mod-adopted-sheets',
  setup(b) {
    b.onLoad({ filter: /[\\/]node_modules[\\/]style-mod[\\/].*\.js$/ }, args => {
      const text = readFileSync(args.path, 'utf8');
      if (!text.includes(STYLE_MOD_PATCH.from)) throw new Error(`${args.path}: the style-mod line scripts/build.mjs patches is gone; review STYLE_MOD_PATCH`);
      return { contents: text.replace(STYLE_MOD_PATCH.from, STYLE_MOD_PATCH.to), loader: 'js' };
    });
  },
};

// The package a bundled file belongs to: its folder under node_modules.
function packageOf(input) {
  const m = input.match(/(?:^|\/)node_modules\/((?:@[^/]+\/)?[^/]+)\//);
  return m ? m[1] : null;
}

function licenceText(name) {
  const dir = path.join(root, 'node_modules', name);
  const file = readdirSync(dir).filter(f => /^(licen[cs]e|copying)(\.(md|txt))?$/i.test(f)).sort()[0];
  if (!file) throw new Error(`${name} has no licence file`);
  return readFileSync(path.join(dir, file), 'utf8').replace(/\r\n/g, '\n').trim();
}

/**
 * Builds every entry in memory: { files: { [name]: text }, inputs and imports:
 * { [file]: [path] } for each .js file built, loads: { [entry]: [file] } (the
 * entry and the chunks it loads, transitively), packages }.
 */
export async function build() {
  const result = await esbuild.build({
    absWorkingDir: root,
    entryPoints: Object.fromEntries(ENTRIES.map(name => [name, `src/${name}/index.js`])),
    bundle: true,
    splitting: true,
    format: 'esm',
    platform: 'browser',
    target: 'es2020',
    minify: true,
    legalComments: 'none',
    charset: 'utf8',
    banner: { js: banner },
    outdir: 'dist',
    chunkNames: 'chunk-[hash]',
    write: false,
    metafile: true,
    logLevel: 'silent',
    plugins: [adoptStyles],
  });
  const files = {}, inputs = {}, imports = {};
  for (const out of result.outputFiles) files[path.basename(out.path)] = out.text;
  for (const [out, meta] of Object.entries(result.metafile.outputs)) {
    const file = path.basename(out);
    inputs[file] = Object.keys(meta.inputs).sort();
    imports[file] = meta.imports.map(i => i.path);
  }
  for (const file of Object.keys(files)) {
    if (!ENTRIES.includes(file.replace(/\.js$/, '')) && !CHUNK.test(file)) throw new Error(`the build made ${file}, neither an entry nor a chunk`);
  }
  // Each entry and the chunks it loads.
  const loads = {};
  for (const name of ENTRIES) {
    const seen = new Set();
    const visit = file => {
      if (seen.has(file)) return;
      seen.add(file);
      for (const i of imports[file]) visit(path.basename(i));
    };
    visit(`${name}.js`);
    loads[name] = [...seen].sort();
  }
  const bundled = new Map();
  for (const name of ENTRIES) {
    for (const file of loads[name]) {
      for (const input of inputs[file]) {
        const p = packageOf(input);
        if (!p) continue;
        if (!bundled.has(p)) bundled.set(p, new Set());
        bundled.get(p).add(name);
      }
    }
    const types = path.join(root, 'src', name, 'index.d.ts');
    if (!existsSync(types)) throw new Error(`src/${name}/index.d.ts is missing`);
    files[`${name}.d.ts`] = readFileSync(types, 'utf8');
  }
  const sections = [...bundled.keys()].sort().map(name => {
    const meta = JSON.parse(readFileSync(path.join(root, 'node_modules', name, 'package.json'), 'utf8'));
    const rule = '-'.repeat(72);
    return `${rule}\n${name} ${meta.version} (${meta.license})\nLoaded by: ${[...bundled.get(name)].sort().map(e => `${e}.js`).join(', ')}\n${rule}\n\n${licenceText(name)}\n`;
  });
  files['THIRD-PARTY-LICENSES.txt'] = `Third-party software bundled in ${pkg.name} ${pkg.version} dist/ (the entries and their dist/chunk-*.js files).\n`
    + 'Generated by scripts/build.mjs from the packages in node_modules; do not edit.\n\n' + sections.join('\n');
  return { files, inputs, imports, loads, packages: [...bundled.keys()].sort() };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const out = path.resolve(root, process.argv[2] || 'dist');
  mkdirSync(out, { recursive: true });
  const { files } = await build();
  for (const old of readdirSync(out)) if (CHUNK.test(old) && !(old in files)) unlinkSync(path.join(out, old));
  for (const [name, text] of Object.entries(files)) writeFileSync(path.join(out, name), text);
  console.log(`built ${Object.keys(files).map(f => `${path.relative(process.cwd(), out) || '.'}/${f}`).join(', ')}`);
}
