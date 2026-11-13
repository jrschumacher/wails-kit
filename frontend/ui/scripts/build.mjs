#!/usr/bin/env node
// Builds dist/ for apps without a bundler: one stylesheet and one ES module.
//   dist/wails-kit-ui.css  src/ui.css with its @imports inlined, in order
//   dist/wails-kit-ui.js   the modules src/index.js exports from, joined in
//                          dependency order, with the imports between them
//                          removed; it exports what src/index.js exports
// No dependencies. Usage: node scripts/build.mjs [outDir]  (default: dist)
// test/build.test.mjs builds into a temporary folder and checks that the
// committed dist/ matches.
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = fileURLToPath(new URL('..', import.meta.url));
const src = path.join(root, 'src');
const pkg = JSON.parse(readFileSync(path.join(root, 'package.json'), 'utf8'));
const banner = `/* ${pkg.name} ${pkg.version}. Built by scripts/build.mjs from src/; do not edit. */\n`;

export function buildCss() {
  const entry = readFileSync(path.join(src, 'ui.css'), 'utf8');
  const parts = [];
  for (const [, file] of entry.matchAll(/^@import\s+"([^"]+)";\s*$/gm)) {
    parts.push(`/* ---- ${file} ---- */\n${readFileSync(path.join(src, file), 'utf8').trimEnd()}\n`);
  }
  if (!parts.length) throw new Error('src/ui.css imports nothing');
  return banner + parts.join('\n');
}

// The modules index.js re-exports from, each after the modules it imports.
function moduleOrder() {
  const index = readFileSync(path.join(src, 'index.js'), 'utf8');
  const entries = [...index.matchAll(/from\s+'\.\/([^']+)'/g)].map(m => m[1]);
  const order = [], seen = new Set();
  const visit = file => {
    if (seen.has(file)) return;
    seen.add(file);
    const text = readFileSync(path.join(src, file), 'utf8');
    for (const [, dep] of text.matchAll(/^import\s[^;]*?from\s+'\.\/([^']+)';\s*$/gm)) visit(dep);
    order.push(file);
  };
  entries.forEach(visit);
  return order;
}

export function buildJs() {
  const parts = moduleOrder().map(file => {
    const text = readFileSync(path.join(src, file), 'utf8')
      .replace(/^import\s[^;]*?from\s+'\.\/[^']+';\s*\n/gm, '')
      .replace(/^export\s*\{[^}]*\}\s*from\s+'\.\/[^']+';\s*\n/gm, '');
    if (/^\s*import\s/m.test(text) || /\bimport\s*\(/.test(text)) throw new Error(`src/${file} imports from outside the package`);
    return `// ---- ${file} ----\n${text.trimEnd()}\n`;
  });
  return banner + parts.join('\n');
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const out = path.resolve(root, process.argv[2] || 'dist');
  mkdirSync(out, { recursive: true });
  writeFileSync(path.join(out, 'wails-kit-ui.css'), buildCss());
  writeFileSync(path.join(out, 'wails-kit-ui.js'), buildJs());
  console.log(`built ${path.relative(process.cwd(), out) || '.'}/wails-kit-ui.css and wails-kit-ui.js`);
}
