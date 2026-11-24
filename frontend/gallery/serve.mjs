// The gallery's static server: serves the frontend packages it shows and the
// design system's component cards on 127.0.0.1, and nothing else. Run it
// with `npm run gallery` and open the URL it prints. test/browser.mjs uses
// the same server.
//
//   /gallery/…            frontend/gallery/
//   /ui/…                 frontend/ui/       (@wails-kit/ui: src/, dist/, theme/, test fixtures)
//   /editor/…             frontend/editor/   (@wails-kit/editor: dist/ and its demo pages)
//   /diagram/…            frontend/diagram/  (@wails-kit/diagram: dist/ and its demo pages)
//   /design-system/<Name>/README.md   docs/design-system/components/<Name>/README.md
//
// Any other path, a path with "..", a dotfile or dot folder, anything under
// node_modules/, and a file whose real path (after symlinks) leaves the
// folder the request named (frontend/<root>/ or the cards' folder), or lands
// in a node_modules/ or dot folder there, is a 404. A request whose Host isn't
// 127.0.0.1 or localhost on the server's port is refused (421), so a page
// on another site can't reach it through DNS rebinding.
import http from 'node:http';
import { readFile, realpath } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const frontend = fileURLToPath(new URL('../', import.meta.url));
const cards = fileURLToPath(new URL('../../docs/design-system/components/', import.meta.url));
const types = {
  '.html': 'text/html; charset=utf-8', '.js': 'text/javascript; charset=utf-8', '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8', '.md': 'text/markdown; charset=utf-8', '.svg': 'image/svg+xml',
  '.png': 'image/png', '.txt': 'text/plain; charset=utf-8',
};

// The file a request path maps to, or null when the server refuses it.
export function resolve(pathname) {
  if (pathname.includes('..') || pathname.includes('\\') || pathname.includes('\0') || pathname.includes('/node_modules/') || pathname.includes('/.')) return null;
  const card = /^\/design-system\/([A-Za-z]+)\/README\.md$/.exec(pathname);
  if (card) return path.join(cards, card[1], 'README.md');
  if (!/^\/(gallery|ui|editor|diagram)\//.test(pathname)) return null;
  const file = pathname.endsWith('/') ? `${pathname}index.html` : pathname;
  return path.join(frontend, file);
}

// The file's real path, or null when it leaves the folder the request named
// (frontend/<root>/, or the cards' folder), or lands in a node_modules/ or
// dot folder inside it.
export async function inside(file) {
  const root = file.startsWith(cards) ? cards : path.join(frontend, path.relative(frontend, file).split(path.sep)[0]);
  const base = await realpath(root);
  const real = await realpath(file);
  if (!real.startsWith(base + path.sep)) return null;
  return path.relative(base, real).split(path.sep).some(p => p === 'node_modules' || p.startsWith('.')) ? null : real;
}
// The Host header names this server: 127.0.0.1 or localhost, on its port.
export function hostAllowed(host, port) {
  return host === `127.0.0.1:${port}` || host === `localhost:${port}`;
}

export function createServer() {
  const server = http.createServer(async (req, res) => {
    try {
      if (!hostAllowed(req.headers.host, server.address()?.port)) { res.writeHead(421).end(); return; }
      let pathname;
      try { pathname = decodeURIComponent(new URL(req.url, 'http://localhost').pathname); } catch { res.writeHead(400).end(); return; }
      if (pathname === '/' || pathname === '/gallery') { res.writeHead(302, { Location: '/gallery/' }).end(); return; }
      const named = resolve(pathname);
      const file = named && await inside(named);
      if (!file) { res.writeHead(404).end(); return; }
      const data = await readFile(file);
      res.setHeader('Content-Type', types[path.extname(file)] || 'application/octet-stream');
      res.setHeader('Cache-Control', 'no-store');
      res.end(data);
    } catch { res.writeHead(404).end(); }
  });
  return server;
}

// Starts the server on 127.0.0.1 and resolves to its origin.
export async function listen(server, port = 0) {
  await new Promise((done, fail) => { server.once('error', fail); server.listen(port, '127.0.0.1', done); });
  return `http://127.0.0.1:${server.address().port}`;
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  const origin = await listen(createServer(), Number(process.env.PORT) || 5178);
  console.log(`Component gallery: ${origin}/gallery/`);
}
