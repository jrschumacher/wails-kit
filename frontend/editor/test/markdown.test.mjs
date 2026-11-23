// The Markdown renderer's rules, in node: it builds DOM through a small fake
// document (createElement, createTextNode, createDocumentFragment), and the
// output is serialised to HTML-like text to compare. Code blocks are not
// highlighted here (the browser checks cover that).
import test from 'node:test';
import assert from 'node:assert/strict';

class Node {
  constructor(tag) { this.tag = tag; this.attrs = new Map(); this.children = []; this.listeners = {}; }
  append(...nodes) {
    for (const n of nodes) {
      if (n instanceof Node && n.tag === '#fragment') this.children.push(...n.children.splice(0));
      else this.children.push(typeof n === 'string' ? new Text(n) : n);
    }
  }
  setAttribute(k, v) { this.attrs.set(k, String(v)); }
  getAttribute(k) { return this.attrs.has(k) ? this.attrs.get(k) : null; }
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
  set textContent(t) { this.children = [new Text(t)]; }
  get textContent() { return this.children.map(c => c.textContent).join(''); }
  all(pred, out = []) { for (const c of this.children) if (c instanceof Node) { if (pred(c)) out.push(c); c.all(pred, out); } return out; }
  get html() {
    if (this.tag === '#fragment') return this.children.map(c => c.html).join('');
    const attrs = [...this.attrs].map(([k, v]) => (v === '' ? ` ${k}` : ` ${k}="${v}"`)).join('');
    return `<${this.tag}${attrs}>${this.children.map(c => c.html).join('')}</${this.tag}>`;
  }
}
class Text {
  constructor(t) { this.textContent = String(t); }
  get html() { return this.textContent.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;'); }
}
// Imported before the fake document exists, so CodeMirror loads as it does
// in node; the renderer reads the global document when it builds.
const { renderMarkdown, safeUrl, decodeEntities } = await import('../src/markdown/render.js');
globalThis.document = {
  createElement: tag => new Node(tag),
  createTextNode: t => new Text(t),
  createDocumentFragment: () => new Node('#fragment'),
};
const render = (text, options = {}) => renderMarkdown(text, { highlight: false, ...options });
const html = (text, options) => render(text, options).html;
const ALLOWED = new Set(['p', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'strong', 'em', 'del', 'code', 'pre', 'div', 'blockquote', 'ul', 'ol', 'li', 'input',
  'table', 'thead', 'tbody', 'tr', 'th', 'td', 'a', 'br', 'hr', 'img']);
const ATTRS = new Set(['class', 'href', 'rel', 'title', 'start', 'type', 'disabled', 'checked', 'aria-label', 'data-align', 'scope', 'data-language',
  'src', 'alt', 'loading', 'decoding', 'referrerpolicy']);
// Every element and attribute is on the allow-list; no URL attribute holds anything but http(s) or mailto.
function assertSafe(fragment, label) {
  for (const n of fragment.all(() => true)) {
    assert.ok(ALLOWED.has(n.tag), `${label}: <${n.tag}>`);
    for (const [k, v] of n.attrs) {
      assert.ok(ATTRS.has(k), `${label}: <${n.tag} ${k}>`);
      if (k === 'href' || k === 'src') assert.match(v, /^(https?:\/\/|mailto:)/, `${label}: ${k}=${v}`);
    }
  }
}

test('sanitising: script, event handlers, iframes, SVG, style and other raw HTML come out as text', () => {
  const payloads = [
    '<script>alert(1)</script>',
    'hi <img src=x onerror=alert(1)> there',
    '<iframe src="https://example.com"></iframe>',
    '<svg><script>alert(1)</script><a xlink:href="javascript:alert(1)">x</a></svg>',
    '<svg onload=alert(1)>',
    '<style>body{display:none}</style>',
    '<a href="javascript:alert(1)">x</a>',
    '<div onclick="alert(1)">*text*</div>',
    'x <span onmouseover="alert(1)">y</span> z',
    '<math><mi xlink:href="javascript:alert(1)">x</mi></math>',
    '<object data="x.swf"></object><embed src="x">',
    '<form action="https://evil.example"><input name=a></form>',
    '<meta http-equiv="refresh" content="0;url=https://evil.example">',
    '<!-- comment --> <![CDATA[ x ]]>',
  ];
  for (const p of payloads) {
    const f = render(p);
    assertSafe(f, p);
    assert.equal(f.all(n => n.tag === 'a').length, 0, `${p}: made a link`);
    // The markup shows as written.
    assert.ok(f.textContent.includes(p.match(/<[a-z!]+/i)[0]), `${p}: the HTML is not shown as text (${f.html})`);
  }
});

test('links: only absolute http, https and mailto URLs; anything else is its text', () => {
  const bad = ['javascript:alert(1)', 'JaVaScRiPt:alert(1)', '&#106;avascript:alert(1)', '&#x6A;avascript:alert(1)', 'java%0ascript:alert(1)',
    'data:text/html,<script>alert(1)</script>', 'vbscript:msgbox(1)', 'file:///etc/passwd', '/relative', '#anchor', 'relative/path', '//evil.example/x',
    ' javascript:alert(1)', 'blob:https://x/y', 'about:blank'];
  for (const href of bad) {
    const f = render(`[click](${href.includes(' ') ? `<${href}>` : href})`);
    assertSafe(f, href);
    assert.equal(f.all(n => n.tag === 'a').length, 0, `${href} became a link: ${f.html}`);
    assert.equal(f.textContent, 'click');
  }
  // Tabs and newlines inside a scheme do not get past the URL parser.
  assert.equal(safeUrl('java\tscript:alert(1)'), null);
  assert.equal(safeUrl('java\nscript:alert(1)'), null);
  assert.equal(safeUrl('\u0001javascript:alert(1)'), null);
  assert.equal(safeUrl('https://example.com/a?b=1&amp;c=2'), 'https://example.com/a?b=1&c=2');
  const good = render('[a](https://example.com/x "Title") <https://example.com/auto> www.example.org [m](mailto:a@example.com)');
  assertSafe(good, 'good');
  const links = good.all(n => n.tag === 'a');
  assert.deepEqual(links.map(a => a.getAttribute('href')), ['https://example.com/x', 'https://example.com/auto', 'http://www.example.org/', 'mailto:a@example.com']);
  for (const a of links) {
    assert.equal(a.getAttribute('rel'), 'noopener noreferrer');
    assert.equal(a.getAttribute('class'), 'wk-link');
    assert.equal(a.getAttribute('target'), null);
  }
  assert.equal(links[0].getAttribute('title'), 'Title');
});

test('a link click goes to onlink and never navigates', () => {
  const seen = [];
  const f = render('[a](https://example.com/x)', { onlink: url => seen.push(url) });
  const a = f.all(n => n.tag === 'a')[0];
  let prevented = 0;
  for (const fn of a.listeners.click) fn({ preventDefault: () => prevented++ });
  for (const fn of a.listeners.auxclick) fn({ preventDefault: () => prevented++ });
  assert.deepEqual(seen, ['https://example.com/x']);
  assert.equal(prevented, 2);
  // Without onlink, the click still does not navigate.
  const b = render('[a](https://example.com/x)').all(n => n.tag === 'a')[0];
  let stopped = false;
  b.listeners.click[0]({ preventDefault: () => { stopped = true; } });
  assert.equal(stopped, true);
});

test('images: none load by default (a link instead); https images load from allowed hosts only', () => {
  const md = '![a chart](https://images.example.com/c.png) ![](https://other.example/p.png) ![x](http://images.example.com/c.png) ![js](javascript:alert(1)) ![d](data:image/png;base64,AAAA)';
  const none = render(md);
  assertSafe(none, 'images');
  assert.equal(none.all(n => n.tag === 'img').length, 0);
  assert.deepEqual(none.all(n => n.tag === 'a').map(a => [a.textContent, a.getAttribute('href')]), [
    ['a chart', 'https://images.example.com/c.png'], ['https://other.example/p.png', 'https://other.example/p.png'], ['x', 'http://images.example.com/c.png'],
  ]);
  assert.ok(none.textContent.includes('js') && none.textContent.includes('d'));
  const allowed = render(md, { imageHosts: ['Images.Example.com'] });
  assertSafe(allowed, 'allowed');
  const imgs = allowed.all(n => n.tag === 'img');
  assert.deepEqual(imgs.map(i => i.getAttribute('src')), ['https://images.example.com/c.png']);
  assert.equal(imgs[0].getAttribute('alt'), 'a chart');
  assert.equal(imgs[0].getAttribute('referrerpolicy'), 'no-referrer');
});

test('CommonMark: headings, emphasis, code, quotes, lists, rules, breaks, escapes and entities', () => {
  assert.equal(html('# One\n\n## Two\n\nSetext\n---'), '<h1>One</h1><h2>Two</h2><h2>Setext</h2>');
  assert.equal(html('*em* **strong** `a < b` \\*not\\*'), '<p><em>em</em> <strong>strong</strong> <code>a &lt; b</code> *not*</p>');
  assert.equal(html('> quoted\n> more'), '<blockquote><p>quoted\nmore</p></blockquote>');
  assert.equal(html('- a\n- b\n  - c'), '<ul><li>a</li><li>b<ul><li>c</li></ul></li></ul>');
  assert.equal(html('3. x\n4. y'), '<ol start="3"><li>x</li><li>y</li></ol>');
  assert.equal(html('a\n\n***\n\nb'), '<p>a</p><hr></hr><p>b</p>');
  assert.equal(html('line  \nbreak'), '<p>line<br></br>break</p>');
  assert.equal(html('&amp; &lt;b&gt; &copy; &#65; &#x42; &#38;amp; &nosuch;'), '<p>&amp; &lt;b&gt; © A B &amp;amp; &amp;nosuch;</p>');
  assert.equal(html('```go\nfunc f() { return 1 < 2 }\n```'), '<div class="wk-code"><pre><code data-language="go">func f() { return 1 &lt; 2 }</code></pre></div>');
  assert.equal(html('    indented'), '<div class="wk-code"><pre><code>indented</code></pre></div>');
  assert.equal(decodeEntities('&#0; &#xD800;'), '� �');
});

test('GFM: tables with alignment, task lists, strikethrough', () => {
  assert.equal(html('| a | b | c |\n|:--|:-:|--:|\n| 1 | *2* | 3 |'),
    '<div class="wk-markdown-table"><table class="wk-table"><thead><tr><th data-align="left" scope="col">a</th><th data-align="center" scope="col">b</th><th data-align="right" scope="col">c</th></tr></thead>'
    + '<tbody><tr><td data-align="left">1</td><td data-align="center"><em>2</em></td><td data-align="right">3</td></tr></tbody></table></div>');
  assert.equal(html('- [x] done\n- [ ] todo'),
    '<ul><li class="wk-markdown-task"><input type="checkbox" class="wk-checkbox" disabled checked aria-label="Done"></input> done</li>'
    + '<li class="wk-markdown-task"><input type="checkbox" class="wk-checkbox" disabled aria-label="Not done"></input> todo</li></ul>');
  assert.equal(html('~~gone~~ and ~one~'), '<p><del>gone</del> and <del>one</del></p>');
});

test('empty and odd input', () => {
  assert.equal(html(''), '');
  assert.equal(html(null), '');
  assert.equal(html('   \n\n'), '');
  // Reference links resolve; the definition renders nothing.
  assert.equal(html('[a][r]\n\n[r]: https://example.com'), '<p><a class="wk-link" href="https://example.com/" rel="noopener noreferrer">a</a></p>');
  // Deep nesting does not throw.
  assert.doesNotThrow(() => render('>'.repeat(500) + ' x'));
  assert.doesNotThrow(() => render('*'.repeat(5000)));
});
