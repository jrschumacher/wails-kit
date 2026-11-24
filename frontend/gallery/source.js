// What the gallery shows as an example's markup and script: source text
// without its common indent, and a function's body as an app would write it.
// No DOM, so test/static.test.mjs checks it in node.
// Source text without its common indent, and without blank first and last lines.
export function dedent(text) {
  const lines = String(text).replace(/\t/g, '  ').split('\n');
  while (lines.length && !lines[0].trim()) lines.shift();
  while (lines.length && !lines[lines.length - 1].trim()) lines.pop();
  const indent = Math.min(...lines.slice(1).filter(l => l.trim()).map(l => l.match(/^ */)[0].length), Infinity);
  return lines.map((l, i) => (i === 0 ? l.trimStart() : l.slice(Number.isFinite(indent) ? indent : 0)).trimEnd()).join('\n');
}
// A function's body as an app would write it: the source minus its signature.
export function body(fn) {
  // Every example's function has its parameters in parentheses: skip to the
  // one that closes them, then take what is between the braces after it.
  const text = String(fn);
  let i = text.indexOf('(');
  for (let depth = 0; i < text.length; i++) {
    if (text[i] === '(') depth++;
    else if (text[i] === ')' && --depth === 0) break;
  }
  const open = text.indexOf('{', i);
  return dedent(text.slice(open + 1, text.lastIndexOf('}')));
}
