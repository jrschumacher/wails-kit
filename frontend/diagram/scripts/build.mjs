#!/usr/bin/env node
// Builds dist/ for apps to vendor (esbuild, minified):
//   dist/diagram.js                wkDiagram() and the pan and zoom frame;
//                                  small, with the frame's stylesheet inlined
//   dist/mermaid.js                Mermaid and its libraries, which
//                                  diagram.js loads (import('./mermaid.js'))
//                                  the first time a diagram renders
//   dist/diagram.d.ts              the TypeScript types (copied from src/)
//   dist/THIRD-PARTY-LICENSES.txt  the licence of every bundled package,
//                                  read from node_modules
// Usage: node scripts/build.mjs [outDir]  (default: dist)
// test/build.test.mjs builds in memory and checks that the committed dist/
// matches.
//
// mermaid.js holds only the diagram types the kit offers (flowchart,
// sequence, state, class, entity-relationship, Gantt) and the dagre layout:
// every other diagram, layout, KaTeX and icon pack Mermaid would load with
// import() is replaced by a module that throws (STUB). The bundle has no
// fetch, eval or new Function: d3-fetch and d3-dsv are left out (EMPTY), and
// lodash's global lookup is patched (PATCHES). Mermaid's styling is patched
// to go through src/mermaid/csp.js so it draws under a strict CSP (see the
// comment there). The build fails if a patched line has changed.
import { readFileSync, writeFileSync, mkdirSync, readdirSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import * as esbuild from 'esbuild';

export const root = fileURLToPath(new URL('..', import.meta.url));
const pkg = JSON.parse(readFileSync(path.join(root, 'package.json'), 'utf8'));

// The chunks Mermaid loads with import() that the kit keeps: the six diagram
// types and the dagre layout. Matched on the chunk's file name.
export const KEEP = /^\.\/(?:chunks\/mermaid\.core\/)?(flowDiagram|sequenceDiagram|classDiagram-v2|stateDiagram-v2|erDiagram|ganttDiagram|dagre)-[A-Z0-9]+\.mjs$/;
// Packages left out of the bundle entirely (Mermaid never calls them for the
// kit's diagrams; they hold fetch() and new Function()).
export const EMPTY = ['d3-dsv', 'd3-fetch'];

const csp = JSON.stringify(path.join(root, 'src', 'mermaid', 'csp.js'));
const hooks = `import { adoptRules as __wkAdoptRules, stashStyles as __wkStashStyles, setAttr as __wkSetAttr, refuse as __wkRefuse } from ${csp};\n`;

// [file pattern, [from, to] pairs, whether the file needs the csp.js hooks].
// Each `from` must match exactly once, so the build fails if Mermaid changes
// the code; a `to` names only names from Mermaid's source (svg, rules,
// root), never one its bundler generated (id38), which test/build.test.mjs
// checks.
export const PATCHES = [
  [/[\\/]mermaid[\\/]dist[\\/]mermaid\.core\.mjs$/, [
    // The diagram's <style> element: an adopted stylesheet instead.
    ['const style1 = document.createElement("style");\n  style1.innerHTML = rules;\n  svg.insertBefore(style1, firstChild);',
      '__wkAdoptRules(svg, rules);'],
    // Style attributes leave the markup DOMPurify parses.
    ['root.select(enclosingDivID_selector).node().innerHTML;',
      '(__wkStashStyles(root.select(enclosingDivID_selector).node()), root.select(enclosingDivID_selector).node().innerHTML);'],
  ], true],
  // d3's selection.attr("style", …) and transition.attr.
  [/[\\/]d3-selection[\\/]dist[\\/]d3-selection\.mjs$/, [
    ['    this.setAttribute(name, value);\n  };\n}\n\nfunction attrConstantNS', '    __wkSetAttr(this, name, value);\n  };\n}\n\nfunction attrConstantNS'],
    ['    else this.setAttribute(name, v);\n  };\n}\n\nfunction attrFunctionNS', '    else __wkSetAttr(this, name, v);\n  };\n}\n\nfunction attrFunctionNS'],
  ], true],
  [/[\\/]d3-transition[\\/]dist[\\/]d3-transition\.mjs$/, [
    ['this.setAttribute(name, i.call(this, t));', '__wkSetAttr(this, name, i.call(this, t));'],
  ], true],
  // lodash finds the global object with Function('return this')(); it never
  // gets that far in a browser, but a CSP-safe bundle has no Function().
  [/[\\/]lodash-es[\\/]_root\.js$/, [
    ["Function('return this')()", 'globalThis'],
  ], false],
];
// Shapes that load a picture: the image shape (new Image().src, before any
// sanitizing) and the icon shapes (an icon's SVG through d3's .html(), whose
// style attributes a strict CSP blocks). Each refuses to draw, in whichever
// Mermaid file holds it; the build fails unless each is found exactly once.
export const REFUSED_SHAPES = { imageSquare: 'Images', icon: 'Icons', iconCircle: 'Icons', iconRounded: 'Icons', iconSquare: 'Icons' };
const SHAPE_FN = /^async function (\w+)\(parent, node, \{[^\n]*\) \{\n/gm;

// And in every Mermaid file: el.setAttribute("style", x) goes through the
// CSSOM. STYLE_CALLS is how many the bundle had at this Mermaid version; the
// build fails if that changes, so a new call gets looked at.
const STYLE_CALL = /([\w$.]+)\.setAttribute\("style",\s*/g;
export const STYLE_CALLS = 1;
const MERMAID_FILE = /[\\/]mermaid[\\/]dist[\\/].*\.mjs$/;

// Packages Mermaid's own build inlines into its files, so they are not in
// node_modules: their licences are kept by hand in scripts/licences/. Mermaid
// marks each with a path comment (// …/node_modules/.pnpm/<name>@<version>…);
// the build fails if a bundled file names a package that is not here, at
// this version, or if one here is no longer bundled.
export const INLINED = {
  'js-yaml': { version: '4.3.2', license: 'MIT', file: 'scripts/licences/js-yaml.txt' },
  fastdom: { version: '1.0.12', license: 'MIT', file: 'scripts/licences/fastdom.txt' },
};
const INLINED_MARK = /^\/\/ \S*node_modules\/\.pnpm\/((?:@[^+/]+\+)?[^@/]+)@([^_/]+)/gm;
const inlinedSeen = new Map();

const applied = new Set();
let styleCalls = 0;
const refused = new Map();

const patch = {
  name: 'wk-patch',
  setup(b) {
    b.onResolve({ filter: /.*/ }, args => {
      if (args.kind === 'dynamic-import' && !KEEP.test(args.path)) return { path: args.path, namespace: 'wk-stub' };
      if (EMPTY.includes(args.path)) return { path: args.path, namespace: 'wk-empty' };
      return undefined;
    });
    b.onLoad({ filter: /.*/, namespace: 'wk-stub' }, args => ({
      contents: `throw new Error(${JSON.stringify(`@wails-kit/diagram does not bundle ${args.path.replace(/^.*\//, '').replace(/-[A-Z0-9]+\.mjs$/, '')}`)});`,
      loader: 'js',
    }));
    b.onLoad({ filter: /.*/, namespace: 'wk-empty' }, () => ({ contents: 'export {};', loader: 'js' }));
    b.onLoad({ filter: /\.m?js$/ }, args => {
      const found = PATCHES.find(([file]) => file.test(args.path));
      const mermaidFile = MERMAID_FILE.test(args.path);
      if (!found && !mermaidFile) return undefined;
      let text = readFileSync(args.path, 'utf8');
      if (mermaidFile) for (const [, name, version] of text.matchAll(INLINED_MARK)) inlinedSeen.set(name.replace('+', '/'), version);
      let needsHooks = false;
      if (found) {
        for (const [from, to] of found[1]) {
          if (text.split(from).length !== 2) throw new Error(`${path.relative(root, args.path)}: the line scripts/build.mjs patches is gone or not unique; review PATCHES:\n${from}`);
          text = text.replace(from, to);
          applied.add(from);
        }
        needsHooks = found[2];
      }
      if (mermaidFile) {
        text = text.replace(STYLE_CALL, (_, el) => { styleCalls++; needsHooks = true; return `__wkSetAttr(${el}, "style", `; });
        text = text.replace(SHAPE_FN, (line, name) => {
          if (!Object.hasOwn(REFUSED_SHAPES, name)) return line;
          refused.set(name, (refused.get(name) || 0) + 1);
          needsHooks = true;
          return `${line}  __wkRefuse(${JSON.stringify(REFUSED_SHAPES[name])});\n`;
        });
      }
      return { contents: (needsHooks ? hooks : '') + text, loader: 'js', resolveDir: path.dirname(args.path) };
    });
  },
};

// The modules: [output, entry, options].
export const MODULES = [
  ['diagram.js', 'src/index.js', { external: ['./mermaid.js'], loader: { '.css': 'text' } }],
  // legalComments 'eof' keeps the libraries' /*! @license */ banners (DOMPurify's carries its copyright).
  ['mermaid.js', 'src/mermaid/index.js', { plugins: [patch], legalComments: 'eof' }],
];

const banner = (file, what) => `/* ${pkg.name} ${pkg.version} ${file}: ${what}. Built by scripts/build.mjs from src/; do not edit. `
  + 'Bundles third-party code; see THIRD-PARTY-LICENSES.txt beside this file. */';

// Packages whose package.json names no licence: the licence their licence
// file states (the build checks the file says so).
export const LICENCE_FALLBACK = { khroma: ['MIT', /^The MIT License/] };

export function licenceOf(name, meta) {
  if (meta.license) return meta.license;
  const fallback = LICENCE_FALLBACK[name];
  if (!fallback || !fallback[1].test(licenceText(name))) throw new Error(`${name} names no licence; check its licence file and add it to LICENCE_FALLBACK`);
  return fallback[0];
}

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

/** Builds in memory: { files: { [name]: text }, inputs and imports: { [module]: [path] }, packages, bytes }. */
export async function build() {
  const files = {}, inputs = {}, imports = {}, bundled = new Map();
  applied.clear();
  styleCalls = 0;
  refused.clear();
  inlinedSeen.clear();
  for (const [file, entry, options] of MODULES) {
    const result = await esbuild.build({
      absWorkingDir: root,
      entryPoints: [entry],
      bundle: true,
      format: 'esm',
      platform: 'browser',
      target: 'es2020',
      minify: true,
      legalComments: 'none',
      charset: 'utf8',
      banner: { js: banner(file, file === 'diagram.js' ? 'wkDiagram()' : 'Mermaid, loaded by diagram.js on first use') },
      write: false,
      metafile: true,
      logLevel: 'silent',
      ...options,
    });
    files[file] = result.outputFiles[0].text;
    inputs[file] = Object.keys(result.metafile.inputs).sort();
    imports[file] = Object.values(result.metafile.outputs)[0].imports.map(i => ({ path: i.path, kind: i.kind }));
    for (const input of inputs[file]) {
      const p = packageOf(input);
      if (!p) continue;
      if (!bundled.has(p)) bundled.set(p, new Set());
      bundled.get(p).add(file);
    }
  }
  const missed = PATCHES.flatMap(([, pairs]) => pairs.map(([from]) => from)).filter(from => !applied.has(from));
  if (missed.length) throw new Error(`scripts/build.mjs: patches not applied (is the file still bundled?):\n${missed.join('\n')}`);
  for (const [name, version] of inlinedSeen) {
    if (INLINED[name]?.version !== version) throw new Error(`scripts/build.mjs: Mermaid inlines ${name}@${version}; add its licence to INLINED and scripts/licences/`);
  }
  for (const name of Object.keys(INLINED)) {
    if (!inlinedSeen.has(name)) throw new Error(`scripts/build.mjs: ${name} is in INLINED but no bundled Mermaid file inlines it; remove it`);
  }
  for (const name of Object.keys(REFUSED_SHAPES)) {
    if (refused.get(name) !== 1) throw new Error(`scripts/build.mjs: the ${name} shape was found ${refused.get(name) || 0} times, not once; review REFUSED_SHAPES`);
  }
  if (styleCalls !== STYLE_CALLS) throw new Error(`scripts/build.mjs: Mermaid has ${styleCalls} setAttribute("style", …) calls, not ${STYLE_CALLS}; check them and update STYLE_CALLS`);
  const types = path.join(root, 'src', 'index.d.ts');
  if (!existsSync(types)) throw new Error('src/index.d.ts is missing');
  files['diagram.d.ts'] = readFileSync(types, 'utf8');
  const rule = '-'.repeat(72);
  const sections = [
    ...[...bundled.keys()].map(name => {
      const meta = JSON.parse(readFileSync(path.join(root, 'node_modules', name, 'package.json'), 'utf8'));
      return [name, `${rule}\n${name} ${meta.version} (${licenceOf(name, meta)})\nBundled in: ${[...bundled.get(name)].sort().join(', ')}\n${rule}\n\n${licenceText(name)}\n`];
    }),
    ...Object.entries(INLINED).map(([name, { version, license, file }]) => [name,
      `${rule}\n${name} ${version} (${license})\nBundled in: mermaid.js (inlined by Mermaid's own build)\n${rule}\n\n${readFileSync(path.join(root, file), 'utf8').replace(/\r\n/g, '\n').trim()}\n`]),
  ].sort((a, b) => (a[0] < b[0] ? -1 : 1)).map(([, text]) => text);
  files['THIRD-PARTY-LICENSES.txt'] = `Third-party software bundled in ${pkg.name} ${pkg.version} dist/.\n`
    + 'Generated by scripts/build.mjs from the packages in node_modules; do not edit.\n\n' + sections.join('\n');
  return { files, inputs, imports, packages: [...bundled.keys()].sort(), inlined: Object.keys(INLINED).sort() };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const out = path.resolve(root, process.argv[2] || 'dist');
  mkdirSync(out, { recursive: true });
  const { files } = await build();
  for (const [name, text] of Object.entries(files)) writeFileSync(path.join(out, name), text);
  console.log(`built ${Object.entries(files).map(([f, t]) => `${path.relative(process.cwd(), out) || '.'}/${f} (${Buffer.byteLength(t)} bytes)`).join(', ')}`);
}
