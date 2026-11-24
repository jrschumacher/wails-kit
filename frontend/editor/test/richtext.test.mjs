// The rich-text editor's document rules, in node: Markdown in and out
// (round trips, the canonical spelling, a seeded random check), the limits
// on hostile input, links, the HTML output's allow-list and the feature set.
// Parsing HTML (paste, format: 'html') needs a browser's DOMParser: the
// browser checks (test/browser.mjs) cover it.
import test from 'node:test';
import assert from 'node:assert/strict';
import { buildSchema, featureSet, linkTarget, languageOf, FEATURES } from '../src/richtext/schema.js';
import { parseMarkdown, serializeMarkdown, sourceDoc, sourceText } from '../src/richtext/markdown.js';
import { serializeHtml } from '../src/richtext/html.js';
import { nesting, tooDeep } from '../src/richtext/limits.js';
import { MAX_MARKDOWN_LENGTH, MAX_MARKDOWN_DEPTH, MAX_MARKDOWN_DELIMITERS } from '../src/shared/markdown-limits.js';

const schema = buildSchema();
const { nodes, marks } = schema;
const load = (text, s = schema) => parseMarkdown(s, text);
const roundTrip = (text, s = schema) => serializeMarkdown(load(text, s).doc);
// Attributes Markdown cannot always keep (a tight list it must write loose).
const shape = doc => JSON.stringify(doc.toJSON(), (k, v) => (k === 'tight' ? undefined : v));

// Runs fn, failing if it throws or takes longer than ms.
function timed(ms, label, fn) {
  const start = performance.now();
  let out;
  assert.doesNotThrow(() => { out = fn(); }, label);
  const took = performance.now() - start;
  assert.ok(took < ms, `${label}: ${Math.round(took)} ms (budget ${ms} ms)`);
  return out;
}

test('Markdown round-trips: each feature in its canonical spelling comes back unchanged', () => {
  const cases = {
    paragraphs: 'One paragraph.\n\nAnother one.',
    headings: '# Heading\n\n## Subheading\n\nText.',
    bold: 'Some **bold** text, and **bold**ly in a word.',
    italic: 'Some _italic_ text, and un*believ*able in a word.',
    'inline code': 'Run `go test ./...` or ``a ` b``, and `` `tick` ``.',
    links: 'See [the guide](https://example.com/guide), [mail](mailto:a@example.com) and [paren](https://en.wikipedia.org/wiki/Go_(game)).',
    'bare links': 'At https://example.com/x, www.example.org or a@example.com, and <https://example.com/y>z.',
    'formatted links': '[**bold** and _italic_ link](https://example.com/)',
    'marks together': '**_both_** and **bold _then both_**',
    'bulleted lists': '- one\n- two\n  - nested\n  - again\n- three',
    'numbered lists': '1. one\n2. two\n\n1) another list\n\n3. starts at three\n4. four',
    'loose lists': '- one\n\n  second paragraph\n\n- two',
    'lists in a row': '- first list\n\n* second list',
    quotes: '> A quote with a line break\\\n> here.\n>\n> > nested',
    'code blocks': '```go\nfunc main() {\n\tfmt.Println("hi")\n}\n```\n\n````\n```\n````',
    'horizontal rule': 'Above.\n\n---\n\nBelow.',
    'line breaks': 'Line one\\\nline two',
    'escaped text': '\\*not bold\\*, \\_not italic\\_, \\`not code\\`, \\[not a link\\], \\~ \\<b> \\&amp; snake_case',
    'escaped line starts': '\\# not a heading\n\n\\- not a list\n\n1\\. not a list\n\n\\> not a quote\n\n\\---',
    'lists of everything': '- **bold** item\n- [link](https://example.com/)\n- `code`\n  > quoted\n- ```\n  code\n  ```',
    empty: '',
  };
  for (const [feature, text] of Object.entries(cases)) {
    const { doc, source } = load(text);
    assert.equal(source, false, feature);
    assert.equal(serializeMarkdown(doc), text, feature);
  }
});

test('Markdown: other spellings settle on the canonical one after one save, and stay there', () => {
  for (const [text, want] of [
    ['*italic* and __bold__', '_italic_ and **bold**'],
    ['Heading\n=======\n\nSub\n---', '# Heading\n\n## Sub'],
    ['### Three\n\n###### Six', '## Three\n\n## Six'],
    ['* a\n* b', '- a\n- b'],
    ['+ a', '- a'],
    ['1) a\n2) b', '1. a\n2. b'],
    ['    indented code', '```\nindented code\n```'],
    ['~~~js\nx\n~~~', '```js\nx\n```'],
    ['***\n\n___', '---\n\n---'],
    ['soft\nwrapped line', 'soft wrapped line'],
    ['two spaces  \nbreak', 'two spaces\\\nbreak'],
    ['&copy; &#65;', '© A'],
    ['[a](https://example.com "Title")', '[a](https://example.com)'],
    ['[a][r]\n\n[r]: https://example.com/r', '[a](https://example.com/r)'],
  ]) {
    const once = roundTrip(text);
    assert.equal(once, want, JSON.stringify(text));
    assert.equal(roundTrip(once), once, `${JSON.stringify(text)} is not stable`);
  }
});

test('Markdown: what the editor does not offer comes in as text (tables, task lists, strikethrough, images, raw HTML)', () => {
  assert.equal(roundTrip('| a | b |\n|---|---|\n| 1 | **2** |'), 'a | b\n\n1 | **2**');
  assert.equal(roundTrip('- [x] done\n- [ ] todo'), '- \\[x\\] done\n- \\[ \\] todo');
  assert.equal(roundTrip('~~gone~~'), 'gone');
  // An image is a link to it, as the renderer shows it; nothing loads.
  assert.equal(roundTrip('![a chart](https://example.com/c.png)'), '[a chart](https://example.com/c.png)');
  assert.equal(roundTrip('<script>alert(1)</script>'), '\\<script>alert(1)\\</script>');
  assert.equal(roundTrip('a <img src=x onerror=alert(1)> b'), 'a \\<img src=x onerror=alert(1)> b');
});

test('Markdown: links go only to http, https and mailto URLs; anything else is its text', () => {
  for (const href of ['javascript:alert(1)', 'JaVaScRiPt:alert(1)', '&#106;avascript:alert(1)', 'data:text/html,x', 'vbscript:x', 'file:///etc/passwd', '/relative', '#anchor', '//evil.example/x']) {
    const { doc } = load(`[click](${href})`);
    let linked = false;
    doc.descendants(n => { if (marks.link.isInSet(n.marks)) linked = true; });
    assert.equal(linked, false, href);
    assert.equal(doc.textContent, 'click', href);
  }
  assert.equal(linkTarget('java\tscript:alert(1)'), null);
  assert.equal(linkTarget('https://example.com/a?b=1&amp;c=2'), 'https://example.com/a?b=1&c=2');
  // As written when Markdown can hold it; normalised when not.
  assert.equal(linkTarget('https://example.com'), 'https://example.com');
  assert.equal(linkTarget('https://example.com/a b'), 'https://example.com/a%20b');
  assert.equal(linkTarget('https://example.com/a)b'), 'https://example.com/a)b');
  assert.equal(serializeMarkdown(nodes.doc.create(null, nodes.paragraph.create(null, schema.text('x', [marks.link.create({ href: 'https://example.com/a)b' })])))), '[x](https://example.com/a\\)b)');
  // Code in a link's text round-trips, unless it holds a bracket, backslash or
  // backtick, which marked would read as the link's: then it goes as text.
  assert.equal(roundTrip('[run `go test`](https://example.com/)'), '[run `go test`](https://example.com/)');
  const link = marks.link.create({ href: 'https://example.com/' });
  const coded = nodes.doc.create(null, nodes.paragraph.create(null, schema.text('a[0]', [link, marks.code.create()])));
  assert.equal(serializeMarkdown(coded), '[a\\[0\\]](https://example.com/)');
});

test('HTML out: only the allow-listed elements and attributes, text escaped, links safe', () => {
  const { doc } = load('# T <b>\n\n**b** _i_ `c` [l](https://example.com/x "t")\\\nline\n\n- a\n\n1. b\n\n> q\n\n```go\n<x>\n```\n\n---\n\n3. three');
  const html = serializeHtml(doc);
  assert.equal(html, '<h1>T &lt;b&gt;</h1><p><strong>b</strong> <em>i</em> <code>c</code> <a href="https://example.com/x" rel="noopener noreferrer">l</a><br>line</p>'
    + '<ul><li>a</li></ul><ol><li>b</li></ol><blockquote><p>q</p></blockquote><pre><code data-language="go">&lt;x&gt;</code></pre><hr><ol start="3"><li>three</li></ol>');
  // Every tag and attribute is on the Markdown preview's allow-list.
  const allowed = new Set(['p', 'h1', 'h2', 'strong', 'em', 'code', 'pre', 'blockquote', 'ul', 'ol', 'li', 'a', 'br', 'hr']);
  const attrs = new Set(['href', 'rel', 'start', 'data-language']);
  for (const [, tag, rest] of html.matchAll(/<([a-z0-9]+)([^>]*)>/g)) {
    assert.ok(allowed.has(tag), tag);
    for (const [, name] of rest.matchAll(/\s([a-z-]+)=/g)) assert.ok(attrs.has(name), `${tag} ${name}`);
  }
  // A link's href is the normalised safe URL, quotes and all escaped.
  const tricky = nodes.doc.create(null, nodes.paragraph.create(null, schema.text('"x"', [marks.link.create({ href: 'https://example.com/?q="><script>' })])));
  assert.equal(serializeHtml(tricky), '<p><a href="https://example.com/?q=%22%3E%3Cscript%3E" rel="noopener noreferrer">"x"</a></p>');
  // A loose list keeps its paragraphs.
  assert.equal(serializeHtml(load('- a\n\n- b').doc), '<ul><li><p>a</p></li><li><p>b</p></li></ul>');
});

test('features: the schema holds only what the editor offers; loaded content is cut down to it', () => {
  assert.deepEqual([...featureSet()], FEATURES);
  assert.throws(() => featureSet(['bold', 'tables']), /unknown feature "tables"/);
  assert.throws(() => featureSet('bold'), TypeError);
  const few = buildSchema(featureSet(['bold', 'link', 'bulletList']));
  assert.deepEqual(Object.keys(few.nodes).sort(), ['bullet_list', 'doc', 'hard_break', 'list_item', 'paragraph', 'text']);
  assert.deepEqual(Object.keys(few.marks).sort(), ['link', 'strong']);
  assert.equal(roundTrip('# Title\n\n**b** _i_ `c` [l](https://example.com/)\n\n1. one\n\n> q\n\n```\ncode\n```\n\n---\n\n- x', few),
    'Title\n\n**b** i c [l](https://example.com/)\n\none\n\nq\n\ncode\n\n- x');
  assert.equal(languageOf('go extra'), 'go');
  assert.equal(languageOf('a`b'), null);
});

// ---- Seeded random documents ----------------------------------------------
function random(seed) {
  return () => { seed = (Math.imul(seed, 1103515245) + 12345) & 0x7fffffff; return seed / 0x7fffffff; };
}
function generator(rnd, alphabet, { codeInLinks = true } = {}) {
  const pick = a => a[Math.floor(rnd() * a.length)];
  const word = () => {
    let s = '';
    for (let i = 0, n = 1 + Math.floor(rnd() * 3); i < n; i++) s += pick(alphabet);
    return s.trim().replace(/\s+/g, ' ') || 'z';
  };
  const inline = (breaks = true) => {
    const out = [];
    for (let i = 0, n = 1 + Math.floor(rnd() * 4); i < n; i++) {
      let ms = [];
      if (rnd() < .3) ms.push(marks.strong.create());
      if (rnd() < .3) ms.push(marks.em.create());
      if (rnd() < .15) ms.push(marks.link.create({ href: pick(['https://example.com/a', 'https://example.com/(x)', 'mailto:a@example.com']) }));
      if (rnd() < .2) ms = [...(codeInLinks ? ms.filter(m => m.type === marks.link) : []), marks.code.create()];
      out.push(schema.text(word(), ms.reduce((set, m) => m.addToSet(set), [])));
      if (i < n - 1) out.push(breaks && rnd() < .1 ? nodes.hard_break.create() : schema.text(' '));
    }
    return out;
  };
  const block = depth => {
    const r = rnd();
    if (depth < 3 && r < .12) return nodes.blockquote.create(null, blocks(depth + 1));
    if (depth < 3 && r < .25) {
      const type = rnd() < .5 ? nodes.bullet_list : nodes.ordered_list;
      const items = Array.from({ length: 1 + Math.floor(rnd() * 3) }, () => nodes.list_item.create(null, [nodes.paragraph.create(null, inline()), ...(rnd() < .3 ? blocks(depth + 1) : [])]));
      return type.create(type === nodes.ordered_list ? { order: pick([1, 1, 3]), tight: rnd() < .6 } : { tight: rnd() < .6 }, items);
    }
    if (r < .32) return nodes.heading.create({ level: pick([1, 2]) }, inline(false));
    if (r < .38) return nodes.code_block.create({ language: pick([null, 'go']) }, schema.text(pick(['x = 1', 'a\n```\nb', '  indented\n\nblank'])));
    if (r < .42) return nodes.horizontal_rule.create();
    return nodes.paragraph.create(null, inline());
  };
  const blocks = depth => Array.from({ length: 1 + Math.floor(rnd() * 3) }, () => block(depth));
  return () => nodes.doc.create(null, blocks(0));
}
const textblocks = doc => {
  const out = [];
  doc.descendants(n => { if (n.isTextblock) { out.push(n.textContent); return false; } return true; });
  return out;
};

test('random documents of ordinary text: Markdown out and in again gives the same document', () => {
  const make = generator(random(133), ['word', 'text', ' ', 'it\'s', 'a.b', '(x)', 'x_y', 'C:\\dir', '#tag', '50%', 'a+b', 'é', '"q"', '1.', '-', 'v2.0', 'e.g.,', '!', '*', '<b>', '&amp;'], { codeInLinks: false });
  for (let i = 0; i < 1500; i++) {
    const doc = make();
    const text = serializeMarkdown(doc);
    const back = load(text);
    assert.equal(back.source, false, text);
    assert.equal(shape(back.doc), shape(doc), `document ${i}:\n${text}`);
  }
});

test('random documents of hostile text: no text is lost, and the Markdown is stable after one save', () => {
  // Every word full of Markdown's punctuation, marks glued to words: some
  // formatting Markdown cannot spell (emphasis opening on punctuation right
  // after a letter) may shift, but never the text.
  const make = generator(random(7), ['a', 'word', ' ', '*', '_', '`', '[', ']', '\\', '#', '>', '-', '1.', '<b', '&amp;', '!', '~', '|', '(', ')', 'é', '+', '=', 'x_y']);
  for (let i = 0; i < 1500; i++) {
    const doc = make();
    const text = serializeMarkdown(doc);
    const back = load(text).doc;
    assert.deepEqual(textblocks(back), textblocks(doc), `document ${i}:\n${text}`);
    const again = serializeMarkdown(back);
    assert.equal(roundTrip(again), again, `document ${i} is not stable:\n${text}`);
  }
});

// ---- Hostile input ---------------------------------------------------------

test('hostile Markdown loads as its source, quickly, and goes out exactly as it came', () => {
  for (const [label, text] of [
    ['2,000 nested quotes', '>'.repeat(2000) + ' x'],
    ['10,000 emphasis markers', '*'.repeat(5000) + 'a' + '*'.repeat(5000)],
    ['5,000 quoted list items', '> - '.repeat(5000)],
    ['a 1000-level list', Array.from({ length: 1000 }, (_, i) => `${'  '.repeat(i)}- level ${i + 1}`).join('\n')],
    ['nested links', '[a '.repeat(5000) + 'b' + '](x)'.repeat(5000)],
    ['past the length limit', `**bold** ${'x'.repeat(MAX_MARKDOWN_LENGTH)}`],
    ['CRLF lines past the delimiter limit', `line one\r\n${'*a '.repeat(MAX_MARKDOWN_DELIMITERS + 2)}\r\nlast`],
  ]) {
    const { doc, source } = timed(1000, label, () => load(text));
    assert.equal(source, true, label);
    // One plain paragraph per line, no formatting.
    let marked = false;
    doc.descendants(n => { if (n.marks.length || (n.isBlock && n.type !== nodes.paragraph && n.type !== nodes.doc)) marked = true; });
    assert.equal(marked, false, label);
    assert.equal(doc.childCount, text.split(/\r\n?|\n/).length, label);
    assert.equal(sourceText(doc), text.replace(/\r\n?/g, '\n'), label);
  }
  // Under the limits, it is Markdown.
  assert.equal(load(`${'> '.repeat(MAX_MARKDOWN_DEPTH - 1)}x`).source, false);
  assert.equal(load('*a* '.repeat(MAX_MARKDOWN_DELIMITERS / 2)).source, false);
  assert.equal(sourceText(sourceDoc(schema, 'a\n\nb')), 'a\n\nb');
});

test('nesting: the editor\'s documents stay under the Markdown depth limit, so what it saves loads as Markdown', () => {
  const p = () => nodes.paragraph.create(null, schema.text('x'));
  const deep = (n, type) => {
    let node = p();
    for (let i = 0; i < n; i++) node = type === 'quote' ? nodes.blockquote.create(null, node) : nodes.bullet_list.create(null, nodes.list_item.create(null, i ? [p(), node] : [node]));
    return nodes.doc.create(null, node);
  };
  assert.equal(nesting(deep(3, 'quote')), 6);
  assert.equal(nesting(nodes.doc.create(null, nodes.ordered_list.create({ order: 999999999 }, nodes.list_item.create(null, p())))), 7);
  for (const type of ['quote', 'list']) {
    for (let n = 1; n < 40; n++) {
      const doc = deep(n, type);
      if (tooDeep(doc)) break;
      // Every document the guard lets through saves to Markdown under the limit.
      const { source } = load(serializeMarkdown(doc));
      assert.equal(source, false, `${type} ${n}`);
    }
    assert.equal(tooDeep(deep(MAX_MARKDOWN_DEPTH, type)), true, type);
  }
});
