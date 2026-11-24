// The gallery's static server: serves the frontend packages it shows and the
// design system's component cards on 127.0.0.1, and nothing else. Run it
// with `npm run gallery` and open the URL it prints. test/browser.mjs uses
// the same server.
//
//   /gallery/…            frontend/gallery/
//   /ui/…                 frontend/ui/       (@wails-kit/ui: src/, dist/, theme/, fonts/, test fixtures)
//   /editor/…             frontend/editor/   (@wails-kit/editor: dist/ and its demo pages)
//   /diagram/…            frontend/diagram/  (@wails-kit/diagram: dist/ and its demo pages)
//   /design-system/<Name>/README.md   docs/design-system/components/<Name>/README.md
//
// Any other path, a path with "..", and anything under node_modules/ is a 404.
import http from 'node:http';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const frontend = fileURLToPath(new URL('../', import.meta.url));
const cards = fileURLToPath(new URL('../../docs/design-system/components/', import.meta.url));
const types = {
  '.html': 'text/html; charset=utf-8', '.js': 'text/javascript; charset=utf-8', '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8', '.md': 'text/markdown; charset=utf-8', '.woff2': 'font/woff2', '.svg': 'image/svg+xml',
  '.png': 'image/png', '.txt': 'text/plain; charset=utf-8',
};

// The file a request path maps to, or null when the server refuses it.
export function resolve(pathname) {
  if (pathname.includes('..') || pathname.includes('\\') || pathname.includes('\0') || pathname.includes('/node_modules/')) return null;
  const card = /^\/design-system\/([A-Za-z]+)\/README\.md$/.exec(pathname);
  if (card) return path.join(cards, card[1], 'README.md');
  if (!/^\/(gallery|ui|editor|diagram)\//.test(pathname)) return null;
  const file = pathname.endsWith('/') ? `${pathname}index.html` : pathname;
  return path.join(frontend, file);
}

export function createServer() {
  return http.createServer(async (req, res) => {
    try {
      let pathname;
      try { pathname = decodeURIComponent(new URL(req.url, 'http://localhost').pathname); } catch { res.writeHead(400).end(); return; }
      if (pathname === '/' || pathname === '/gallery') { res.writeHead(302, { Location: '/gallery/' }).end(); return; }
      const file = resolve(pathname);
      if (!file) { res.writeHead(404).end(); return; }
      const data = await readFile(file);
      res.setHeader('Content-Type', types[path.extname(file)] || 'application/octet-stream');
      res.setHeader('Cache-Control', 'no-store');
      res.end(data);
    } catch { res.writeHead(404).end(); }
  });
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
