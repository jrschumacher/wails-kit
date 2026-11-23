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
const hooks = `import { adoptRules as __wkAdoptRules, stashStyles as __wkStashStyles, setAttr as __wkSetAttr } from ${csp};\n`;

// [file pattern, [from, to] pairs, whether the file needs the csp.js hooks].
// Each pair must match exactly once.
export const PATCHES = [
  [/[\\/]mermaid[\\/]dist[\\/]mermaid\.core\.mjs$/, [
    // The diagram's <style> element: an adopted stylesheet instead.
    ['const style1 = document.createElement("style");\n  style1.innerHTML = rules;\n  svg.insertBefore(style1, firstChild);',
      '__wkAdoptRules(svg, rules, id38);'],
    // Style attributes leave the markup DOMPurify parses.
    ['let code2 = root.select(enclosingDivID_selector).node().innerHTML;',
      '__wkStashStyles(root.select(enclosingDivID_selector).node());\n    let code2 = root.select(enclosingDivID_selector).node().innerHTML;'],
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
// And in every Mermaid file: el.setAttribute("style", x) goes through the
// CSSOM. STYLE_CALLS is how many the bundle had at this Mermaid version; the
// build fails if that changes, so a new call gets looked at.
const STYLE_CALL = /([\w$.]+)\.setAttribute\("style",\s*/g;
export const STYLE_CALLS = 1;
const MERMAID_FILE = /[\\/]mermaid[\\/]dist[\\/].*\.mjs$/;

const applied = new Set();
let styleCalls = 0;

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
      }
      return { contents: (needsHooks ? hooks : '') + text, loader: 'js', resolveDir: path.dirname(args.path) };
    });
  },
};

// The modules: [output, entry, options].
export const MODULES = [
  ['diagram.js', 'src/index.js', { external: ['./mermaid.js'], loader: { '.css': 'text' } }],
  ['mermaid.js', 'src/mermaid/index.js', { plugins: [patch] }],
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
  if (styleCalls !== STYLE_CALLS) throw new Error(`scripts/build.mjs: Mermaid has ${styleCalls} setAttribute("style", …) calls, not ${STYLE_CALLS}; check them and update STYLE_CALLS`);
  const types = path.join(root, 'src', 'index.d.ts');
  if (!existsSync(types)) throw new Error('src/index.d.ts is missing');
  files['diagram.d.ts'] = readFileSync(types, 'utf8');
  const sections = [...bundled.keys()].sort().map(name => {
    const meta = JSON.parse(readFileSync(path.join(root, 'node_modules', name, 'package.json'), 'utf8'));
    const rule = '-'.repeat(72);
    return `${rule}\n${name} ${meta.version} (${licenceOf(name, meta)})\nBundled in: ${[...bundled.get(name)].sort().join(', ')}\n${rule}\n\n${licenceText(name)}\n`;
  });
  files['THIRD-PARTY-LICENSES.txt'] = `Third-party software bundled in ${pkg.name} ${pkg.version} dist/.\n`
    + 'Generated by scripts/build.mjs from the packages in node_modules; do not edit.\n\n' + sections.join('\n');
  return { files, inputs, imports, packages: [...bundled.keys()].sort() };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const out = path.resolve(root, process.argv[2] || 'dist');
  mkdirSync(out, { recursive: true });
  const { files } = await build();
  for (const [name, text] of Object.entries(files)) writeFileSync(path.join(out, name), text);
  console.log(`built ${Object.entries(files).map(([f, t]) => `${path.relative(process.cwd(), out) || '.'}/${f} (${Buffer.byteLength(t)} bytes)`).join(', ')}`);
}
