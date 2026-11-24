// HTML in and out of the rich-text editor's document.
//
// Out (serializeHtml, no DOM): a string built from the document, so it holds
// only the elements and attributes the editor makes, the Markdown preview's
// allow-list: p, h1, h2, strong, em, code, a (href: http, https or mailto;
// rel), ul, ol (start), li, blockquote, pre, hr and br, with code's
// data-language. Text and attributes are escaped.
//
// In (parseHtml, in a browser): the HTML is parsed into an inert document
// (DOMParser: no scripts run, nothing loads), then read through the schema's
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

/**
 * HTML with its style attributes renamed data-wk-style and its <style>
 * elements made inert <template>s, before the browser parses it: under a
 * strict CSP, parsing a style attribute logs a violation (the style is
 * blocked anyway). Only tags change; the text between them is left alone.
 * The schema reads bold and italic from data-wk-style.
 */
export function withoutInlineStyles(html) {
  return String(html)
    .replace(/<[a-zA-Z][^>]*>/g, tag => tag.replace(/(\s)style(\s*=)/gi, '$1data-wk-style$2'))
    .replace(/<(\/?)style\b/gi, '<$1template');
}

/**
 * HTML as a document, cut down to the schema. Never throws: HTML the parser
 * fails on (a stack overflow on deep nesting) or that nests deeper than the
 * Markdown limit loads as its text, a paragraph per line.
 */
export function parseHtml(schema, html) {
  const body = new DOMParser().parseFromString(withoutInlineStyles(html ?? ''), 'text/html').body;
  try {
    const doc = ProseMirrorParser.fromSchema(schema).parse(body);
    if (!tooDeep(doc)) return doc;
  } catch (e) {
    if (!isStackOverflow(e)) throw e;
  }
  return sourceDoc(schema, body.innerText ?? body.textContent ?? '');
}
