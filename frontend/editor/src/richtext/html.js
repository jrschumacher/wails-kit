// HTML in and out of the rich-text editor's document.
//
// Out (serializeHtml, no DOM): a string built from the document, so it holds
// only the elements and attributes the editor makes, the Markdown preview's
// allow-list: p, h1, h2, strong, em, code, a (href: http, https or mailto;
// rel), ul, ol (start), li, blockquote, pre, hr and br, with code's
// data-language. Text and attributes are escaped.
//
// In (parseHtml, in a browser): the HTML is cleaned as text first
// (cleanHtml: no handlers, resource URLs or inline styles survive it), then
// parsed into an inert document (DOMParser) and read through the schema's
// rules, so anything the editor does not offer is dropped, its text kept.
import { DOMParser as ProseMirrorParser } from 'prosemirror-model';
import { safeUrl } from '../shared/url.js';
import { isStackOverflow } from '../shared/stack.js';
import { sourceDoc } from './markdown.js';
import { tooDeep } from './limits.js';

const escapeText = text => text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
const escapeAttr = text => escapeText(text).replace(/"/g, '&quot;');

const MARK_TAGS = { strong: 'strong', em: 'em', code: 'code' };

function openMark(mark) {
  if (mark.type.name === 'link') return `<a href="${escapeAttr(safeUrl(mark.attrs.href) || '')}" rel="noopener noreferrer">`;
  return `<${MARK_TAGS[mark.type.name]}>`;
}
const closeMark = mark => (mark.type.name === 'link' ? '</a>' : `</${MARK_TAGS[mark.type.name]}>`);

function inline(block) {
  let out = '';
  const active = [];
  const closeTo = n => { while (active.length > n) out += closeMark(active.pop()); };
  block.forEach(node => {
    const marks = node.marks;
    let keep = 0;
    while (keep < active.length && keep < marks.length && active[keep].eq(marks[keep])) keep++;
    closeTo(keep);
    for (const mark of marks.slice(keep)) { out += openMark(mark); active.push(mark); }
    if (node.isText) out += escapeText(node.text);
    else if (node.type.name === 'hard_break') out += '<br>';
  });
  closeTo(0);
  return out;
}

function blocks(node, tight = false) {
  let out = '';
  node.forEach(child => { out += block(child, tight); });
  return out;
}

function block(node, tight) {
  switch (node.type.name) {
    // A tight list's paragraphs are bare in their items.
    case 'paragraph': return tight ? inline(node) : `<p>${inline(node)}</p>`;
    case 'heading': return `<h${node.attrs.level}>${inline(node)}</h${node.attrs.level}>`;
    case 'blockquote': return `<blockquote>${blocks(node)}</blockquote>`;
    case 'horizontal_rule': return '<hr>';
    case 'code_block': {
      const lang = node.attrs.language ? ` data-language="${escapeAttr(node.attrs.language)}"` : '';
      return `<pre><code${lang}>${escapeText(node.textContent)}</code></pre>`;
    }
    case 'bullet_list': case 'ordered_list': {
      const tag = node.type.name === 'bullet_list' ? 'ul' : 'ol';
      const start = tag === 'ol' && node.attrs.order !== 1 ? ` start="${node.attrs.order}"` : '';
      let items = '';
      node.forEach(item => {
        // A bare paragraph runs into a block after it: only the first goes bare.
        let inner = '';
        item.forEach((child, _, i) => { inner += block(child, node.attrs.tight && i === 0); });
        items += `<li>${inner}</li>`;
      });
      return `<${tag}${start}>${items}</${tag}>`;
    }
    default: return `<p>${escapeText(node.textContent)}</p>`;
  }
}

/** A document as HTML, under the allow-list above. */
export function serializeHtml(doc) {
  return blocks(doc);
}

// Untrusted HTML (a paste, a drop, a value in format 'html') is cleaned as
// text, before any parser sees it, so no document, live or inert, ever holds
// an event handler, a resource URL or an inline style from it:
// - every start tag keeps only the attributes the schema reads: href on <a>
//   (http, https or mailto only), start on <ol>, class and data-language on
//   <pre> and <code>; a style's font-weight and font-style become
//   data-wk-style (the schema reads bold and italic from it; parsing a style
//   attribute under a strict CSP logs a violation). Event handlers, src,
//   srcset, srcdoc, data, action and the rest are gone. The first of a
//   repeated attribute counts, as in the parser.
// - elements that load or run something, or whose text is not text, are
//   dropped: void ones (img, embed, link, meta, …) lose their tag, the others
//   (script, style, iframe, svg, …) lose their content too, up to their end
//   tag. A tag whose name is not plain letters and digits (with single hyphens
//   between them) is dropped.
// - comments (<!--…-->, ending at --> or --!>, or at once for <!--> and
//   <!--->), declarations and processing instructions (<!…>, <?…>) and bogus
//   end tags (</ …>) are dropped whole.
// - every other "<" (in text, or one the tokenizer would not read as a tag)
//   is written &lt;. So the only "<" in the output open tags this function
//   writes itself: dropped markup can never join the text around it into a
//   new tag ("<<img>img onerror=…>"), and cleaning twice changes nothing.
// The scan follows the HTML tokenizer's tag and attribute states (quoted and
// unquoted values, "/" between attributes, a "<" or quote inside a name), and
// reads every "<" followed by a letter as a tag, inside raw text too, so it
// can only clean more than the parser would read as tags, never less.
const VOID_DROP = new Set(['img', 'image', 'embed', 'link', 'meta', 'base', 'input', 'source', 'track', 'frame', 'area', 'param', 'keygen', 'wbr', 'basefont', 'bgsound', 'isindex']);
const INERT = new Set(['script', 'style', 'iframe', 'object', 'applet', 'svg', 'math', 'video', 'audio', 'picture', 'canvas', 'select', 'textarea', 'button', 'noscript', 'noembed', 'noframes', 'frameset', 'template', 'title', 'xmp', 'plaintext', 'portal', 'fencedframe', 'map', 'form', 'dialog']);
const KEEP = { a: ['href'], ol: ['start'], pre: ['class', 'data-language'], code: ['class', 'data-language'] };
const quote = value => `"${value.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;')}"`;
const unescapeAttr = value => value.replace(/&quot;/g, '"').replace(/&#0*39;|&apos;/g, '\'').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&');
const letter = c => !!c && /[a-zA-Z]/.test(c);

// The tag at html[lt] ("<name" or "</name"), read as the tokenizer reads it:
// { end, name, closing, attrs: [[name, value | null]] }.
function readTag(html, lt) {
  const closing = html[lt + 1] === '/';
  let at = lt + (closing ? 2 : 1);
  const name = /^[^\s/>]*/.exec(html.slice(at))[0];
  at += name.length;
  const attrs = [];
  while (at < html.length) {
    while (at < html.length && /[\s/]/.test(html[at])) at++;
    if (at >= html.length || html[at] === '>') break;
    const n = /^=?[^\s/>=]*/.exec(html.slice(at))[0];
    at += n.length;
    let ws = at;
    while (ws < html.length && /\s/.test(html[ws])) ws++;
    let value = null;
    if (html[ws] === '=') {
      at = ws + 1;
      while (at < html.length && /\s/.test(html[at])) at++;
      const q = html[at];
      if (q === '"' || q === "'") {
        const close = html.indexOf(q, at + 1);
        value = html.slice(at + 1, close < 0 ? html.length : close);
        at = close < 0 ? html.length : close + 1;
      } else {
        value = /^[^\s>]*/.exec(html.slice(at))[0];
        at += value.length;
      }
    }
    attrs.push([n.toLowerCase(), value]);
  }
  return { end: at < html.length ? at + 1 : html.length, name: name.toLowerCase(), closing, attrs };
}

// The end of a comment starting at html[lt] ("<!--").
function commentEnd(html, lt) {
  const body = lt + 4;
  if (html[body] === '>') return body + 1;
  if (html.startsWith('->', body)) return body + 2;
  const ends = ['-->', '--!>'].map(e => [html.indexOf(e, body), e.length]).filter(([at]) => at >= 0);
  if (!ends.length) return html.length;
  const [at, length] = ends.sort((a, b) => a[0] - b[0])[0];
  return at + length;
}

function writeTag({ name, closing, attrs }) {
  if (closing) return `</${name}>`;
  let kept = '';
  const seen = new Set();
  for (const [n, value] of attrs) {
    // The parser keeps the first of a repeated attribute.
    if (seen.has(n)) continue;
    seen.add(n);
    if (value == null) continue;
    if (n === 'style') {
      // Only what the schema reads: bold and italic.
      const decls = unescapeAttr(value).split(';').map(d => d.trim()).filter(d => /^font-(weight|style)\s*:\s*[\w ]+$/i.test(d));
      if (decls.length) kept += ` data-wk-style=${quote(decls.join('; '))}`;
    } else if ((KEEP[name] || []).includes(n)) {
      const v = unescapeAttr(value);
      if (n === 'href' && !safeUrl(v)) continue;
      kept += ` ${n}=${quote(v)}`;
    }
  }
  return `<${name}${kept}>`;
}

/** Untrusted HTML cleaned as text (above): safe to hand to any HTML parser. */
export function cleanHtml(html) {
  html = String(html ?? '');
  let out = '', i = 0;
  while (i < html.length) {
    const lt = html.indexOf('<', i);
    if (lt < 0) { out += html.slice(i); break; }
    out += html.slice(i, lt);
    const next = html[lt + 1];
    if (html.startsWith('<!--', lt)) { i = commentEnd(html, lt); continue; }
    if (next === '!' || next === '?' || (next === '/' && !letter(html[lt + 2]) && lt + 2 < html.length)) {
      // A declaration, a processing instruction or a bogus end tag: to its ">".
      const gt = html.indexOf('>', lt + 1);
      i = gt < 0 ? html.length : gt + 1;
      continue;
    }
    if (!letter(next) && !(next === '/' && letter(html[lt + 2]))) {
      // Not a tag: a "<" in the text.
      out += '&lt;';
      i = lt + 1;
      continue;
    }
    const t = readTag(html, lt);
    i = t.end;
    if (!/^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(t.name) || VOID_DROP.has(t.name)) continue;
    if (INERT.has(t.name)) {
      // Dropped with its content, up to its first end tag (where the parser
      // ends a raw-text element too); anything after is cleaned as usual.
      if (!t.closing) {
        const close = new RegExp(`</${t.name}[\\s/>]`, 'ig');
        close.lastIndex = t.end;
        const c = close.exec(html);
        const after = c ? html.indexOf('>', c.index) : -1;
        i = after < 0 ? html.length : after + 1;
      }
      continue;
    }
    out += writeTag(t);
  }
  return out;
}

const BLOCKS = new Set(['address', 'article', 'aside', 'blockquote', 'dd', 'details', 'div', 'dl', 'dt', 'fieldset', 'figcaption', 'figure', 'footer',
  'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'header', 'hr', 'li', 'main', 'nav', 'ol', 'p', 'pre', 'section', 'summary', 'table', 'tr', 'td', 'th', 'ul']);

// An element's text, a line per block (paragraph, heading, list item, …)
// and per <br>, blank lines dropped: what innerText gives in a rendered
// page, which an inert document is not. Iterative, for any depth.
function textByBlocks(root) {
  const lines = [];
  let line = '';
  const flush = () => { if (line.trim()) lines.push(line.replace(/\s+/g, ' ').trim()); line = ''; };
  const stack = [[root, false]];
  while (stack.length) {
    const [node, leaving] = stack.pop();
    if (leaving) { flush(); continue; }
    if (node.nodeType === 3) { line += node.data; continue; }
    if (node.nodeType !== 1 && node.nodeType !== 9 && node.nodeType !== 11) continue;
    const name = node.localName;
    if (name === 'br') { flush(); continue; }
    const block = BLOCKS.has(name);
    if (block) { flush(); stack.push([node, true]); }
    for (let c = node.lastChild; c; c = c.previousSibling) stack.push([c, false]);
  }
  flush();
  return lines.join('\n');
}

/**
 * HTML as a document, cut down to the schema. Never throws: HTML the parser
 * fails on (a stack overflow on deep nesting) or that nests deeper than the
 * Markdown limit loads as its text, a paragraph per line.
 */
export function parseHtml(schema, html) {
  const body = new DOMParser().parseFromString(cleanHtml(html), 'text/html').body;
  try {
    const doc = ProseMirrorParser.fromSchema(schema).parse(body);
    if (!tooDeep(doc)) return doc;
  } catch (e) {
    if (!isStackOverflow(e)) throw e;
  }
  return sourceDoc(schema, textByBlocks(body));
}
