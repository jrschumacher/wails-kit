// Markdown to DOM, safe for untrusted text (pull request bodies, comments).
// marked's lexer (MIT, no dependencies) parses CommonMark with the GFM
// tables, task lists, strikethrough and autolinks; this module builds the
// DOM from its tokens with createElement and textContent, so the output holds
// only the elements and attributes listed here. Nothing is ever parsed as
// HTML: raw HTML in the text shows as the text it is.
//
// - Links: http, https and mailto only (anything else, relative URLs and
//   javascript: among them, renders as its text). A link carries
//   rel="noopener noreferrer"; a click never navigates the page: it goes to
//   onlink(url), which the app opens (in Wails, in the system browser).
// - Images: none load by default, since a remote image would tell its server
//   that the text was read. An image renders as a link to it, unless its
//   https host is in imageHosts.
// - Code blocks take the editors' syntax colours (highlightBlock).
import { Lexer } from 'marked';
import { StyleModule } from 'style-mod';
import { highlightBlock } from '../shared/highlight.js';
import { previewStyles } from './style-spec.js';

const ENTITIES = {
  amp: '&', lt: '<', gt: '>', quot: '"', apos: '\'', nbsp: ' ', copy: '©', reg: '®', trade: '™',
  hellip: '…', mdash: '—', ndash: '–', lsquo: '‘', rsquo: '’', ldquo: '“', rdquo: '”', laquo: '«', raquo: '»',
  times: '×', divide: '÷', middot: '·', bull: '•', deg: '°', plusmn: '±', para: '¶', sect: '§', euro: '€',
  pound: '£', yen: '¥', cent: '¢', larr: '←', rarr: '→', uarr: '↑', darr: '↓', harr: '↔', le: '≤', ge: '≥', ne: '≠',
};

/**
 * Decodes HTML character references once: numeric ones and a set of common
 * named ones (marked leaves them encoded); an unknown name stays as written.
 */
export function decodeEntities(text) {
  return String(text).replace(/&(#x[0-9a-f]{1,6}|#[0-9]{1,7}|[a-z][a-z0-9]{1,31});/gi, (whole, ref) => {
    if (ref[0] !== '#') return Object.hasOwn(ENTITIES, ref) ? ENTITIES[ref] : whole;
    const code = ref[1] === 'x' || ref[1] === 'X' ? parseInt(ref.slice(2), 16) : parseInt(ref.slice(1), 10);
    return code > 0 && code <= 0x10ffff && !(code >= 0xd800 && code <= 0xdfff) ? String.fromCodePoint(code) : '�';
  });
}

const LINK_PROTOCOLS = new Set(['http:', 'https:', 'mailto:']);

/**
 * The URL a link may carry, normalised, or null: an absolute http, https or
 * mailto URL. Character references are decoded first and the browser's own
 * URL parser decides the scheme, so "&#106;avascript:" and "java\tscript:"
 * are refused like "javascript:".
 */
export function safeUrl(href) {
  if (typeof href !== 'string') return null;
  let url;
  try { url = new URL(decodeEntities(href).trim()); } catch { return null; }
  return LINK_PROTOCOLS.has(url.protocol) ? url.href : null;
}

// The image URL to load, or null: https, on a host the app allows.
// The host (with its port, unless it is https's default) must equal an
// allowed one exactly: no subdomains, no trailing dot, no other port.
function imageUrl(href, hosts) {
  const url = safeUrl(href);
  if (!url || !hosts.length) return null;
  const { protocol, host } = new URL(url);
  return protocol === 'https:' && hosts.includes(host) ? url : null;
}

// An imageHosts entry as URL.host reads it ("Images.Example.com:443" →
// "images.example.com"), or null when it is not a bare host[:port].
function normaliseHost(entry) {
  const text = String(entry).trim();
  if (!text || /[/?#@\\\s]/.test(text)) return null;
  try { return new URL(`https://${text}/`).host; } catch { return null; }
}

let previewModule = null;
/** The preview's stylesheet, adopted by the document once. */
export function mountPreviewStyles() {
  if (typeof document === 'undefined' || !document.defaultView) return;
  previewModule ??= new StyleModule(previewStyles);
  StyleModule.mount(document, previewModule);
}

class Builder {
  constructor(options) {
    this.onlink = typeof options.onlink === 'function' ? options.onlink : null;
    this.hosts = Array.isArray(options.imageHosts) ? options.imageHosts.map(normaliseHost).filter(Boolean) : [];
    this.highlight = options.highlight !== false;
  }

  el(tag, attrs = {}, children = []) {
    const node = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) if (v != null && v !== false) node.setAttribute(k, v === true ? '' : String(v));
    for (const child of children) node.append(child);
    return node;
  }

  link(url, title, children) {
    const a = this.el('a', { class: 'wk-link', href: url, rel: 'noopener noreferrer', title: title ? decodeEntities(title) : null }, children);
    // Never navigate the webview: the app opens the link.
    a.addEventListener('click', e => { e.preventDefault(); this.onlink?.(url); });
    a.addEventListener('auxclick', e => e.preventDefault());
    return a;
  }

  blocks(tokens, parent) {
    for (const t of tokens || []) {
      const node = this.block(t);
      if (node) parent.append(node);
    }
    return parent;
  }

  block(t) {
    switch (t.type) {
      case 'space': case 'def': return null;
      case 'paragraph': return this.inline(t.tokens, this.el('p'));
      case 'heading': return this.inline(t.tokens, this.el(`h${Math.min(Math.max(t.depth, 1), 6)}`));
      case 'hr': return this.el('hr');
      case 'blockquote': return this.blocks(t.tokens, this.el('blockquote'));
      case 'code': {
        const lang = (t.lang || '').trim().split(/\s+/)[0] || null;
        const code = this.el('code', { 'data-language': lang });
        if (this.highlight) code.append(highlightBlock(t.text, lang)); else code.textContent = t.text;
        return this.el('div', { class: 'wk-code' }, [this.el('pre', {}, [code])]);
      }
      case 'list': return this.list(t);
      case 'table': return this.table(t);
      // Raw HTML shows as the text it is.
      case 'html': return this.el('p', {}, [t.text.replace(/\n+$/, '')]);
      case 'text': return t.tokens ? this.inline(t.tokens, document.createDocumentFragment()) : document.createTextNode(decodeEntities(t.text));
      default: return t.tokens ? this.inline(t.tokens, this.el('p')) : this.el('p', {}, [t.raw || '']);
    }
  }

  list(t) {
    const start = t.ordered && t.start !== '' && Number(t.start) !== 1 ? Number(t.start) : null;
    const list = this.el(t.ordered ? 'ol' : 'ul', { start });
    for (const item of t.items) {
      const li = this.el('li', { class: item.task ? 'wk-markdown-task' : null });
      if (item.task) {
        li.append(this.el('input', { type: 'checkbox', class: 'wk-checkbox', disabled: true, checked: !!item.checked, 'aria-label': item.checked ? 'Done' : 'Not done' }), ' ');
      }
      this.blocks(item.tokens.filter(x => x.type !== 'checkbox'), li);
      list.append(li);
    }
    return list;
  }

  table(t) {
    const cell = (tag, c) => this.inline(c.tokens, this.el(tag, { 'data-align': c.align || null, scope: tag === 'th' ? 'col' : null }));
    const head = this.el('thead', {}, [this.el('tr', {}, t.header.map(c => cell('th', c)))]);
    const body = this.el('tbody', {}, t.rows.map(row => this.el('tr', {}, row.map(c => cell('td', c)))));
    return this.el('div', { class: 'wk-markdown-table' }, [this.el('table', { class: 'wk-table' }, [head, ...(t.rows.length ? [body] : [])])]);
  }

  inline(tokens, parent) {
    for (const t of tokens || []) parent.append(this.span(t));
    return parent;
  }

  span(t) {
    switch (t.type) {
      case 'text': return t.tokens ? this.inline(t.tokens, document.createDocumentFragment()) : document.createTextNode(decodeEntities(t.text));
      case 'escape': return document.createTextNode(t.text);
      case 'strong': return this.inline(t.tokens, this.el('strong'));
      case 'em': return this.inline(t.tokens, this.el('em'));
      case 'del': return this.inline(t.tokens, this.el('del'));
      case 'codespan': return this.el('code', {}, [t.text]);
      case 'br': return this.el('br');
      case 'link': {
        const url = safeUrl(t.href);
        const children = this.inline(t.tokens, document.createDocumentFragment());
        return url ? this.link(url, t.title, [children]) : children;
      }
      case 'image': {
        const alt = decodeEntities(t.text || '');
        const src = imageUrl(t.href, this.hosts);
        if (src) return this.el('img', { src, alt, title: t.title ? decodeEntities(t.title) : null, loading: 'lazy', decoding: 'async', referrerpolicy: 'no-referrer' });
        // Not loaded: a link to the image, labelled with its text.
        const url = safeUrl(t.href);
        const text = alt || url || '';
        return url ? this.link(url, t.title, [text]) : document.createTextNode(alt);
      }
      // Raw HTML shows as the text it is.
      case 'html': return document.createTextNode(t.text);
      default: return document.createTextNode(t.raw ?? t.text ?? '');
    }
  }
}

// Limits. marked's lexer (and CodeMirror's Markdown parser and highlighter)
// recurse for each nested quote, list, emphasis run and bracket, so hostile
// text a few kilobytes long can overflow the stack, and a deeply nested list
// takes time quadratic in its depth. Text past a limit, or that fails to
// parse, renders as its source in one plain paragraph.
/** Longest text rendered as Markdown, in characters (UTF-16 code units). */
export const MAX_MARKDOWN_LENGTH = 100000;
/** Deepest nesting of quotes and lists rendered as Markdown. */
export const MAX_MARKDOWN_DEPTH = 32;
/** Most emphasis and link delimiters (* _ ~ [ ]) in one paragraph rendered as Markdown. */
export const MAX_MARKDOWN_DELIMITERS = 2000;

const container = /^[ \t]*(?:>|[-*+](?=[ \t]|$)|\d{1,9}[.)](?=[ \t]|$))[ \t]?/;
// A fence opening: three or more backticks (whose info string has none) or tildes.
const opening = /^( {0,3})(`{3,}|~{3,})(.*)$/;
const closing = /^[ \t]*(`{3,}|~{3,})[ \t]*$/;
const columns = ws => ws.replace(/\t/g, '    ').length;
const leading = line => columns(line.match(/^[ \t]*/)[0]);

/**
 * Scans the text for the limits above: { depth, delimiters }, the deepest
 * nesting of quotes and lists on a line (the markers it starts with, plus
 * its list indentation at two columns a level) and the most delimiters in
 * one paragraph, both outside fenced code. Either stops counting once past
 * its limit. It errs on the high side: where the parser might read a line
 * as code or not (a fence in a list item that a less indented line ends), it
 * is counted.
 */
export function markdownLimits(text) {
  let depth = 0, delimiters = 0, paragraph = 0, fence = null;
  for (const line of String(text ?? '').split(/\r\n?|\n/)) {
    if (fence) {
      const c = line.match(closing);
      if (c && c[1][0] === fence.char && c[1].length >= fence.length) { fence = null; continue; }
      // Fenced code ends with its container: a non-blank line less indented
      // than the fence is read as text again.
      if (!line.trim() || leading(line) >= fence.column) continue;
      fence = null;
    }
    if (!line.trim()) { paragraph = 0; continue; }
    let rest = line, level = 0, m;
    while ((m = rest.match(container))) {
      level++;
      rest = rest.slice(m[0].length);
      if (level > MAX_MARKDOWN_DEPTH) break;
    }
    if (level) level += Math.floor(leading(line) / 2);
    if (level > depth) depth = level;
    const f = rest.match(opening);
    if (f && !(f[2][0] === '`' && f[3].includes('`'))) {
      fence = { char: f[2][0], length: f[2].length, column: columns(line.slice(0, line.length - rest.length)) + f[1].length };
      paragraph = 0;
      continue;
    }
    for (let i = 0; i < rest.length; i++) {
      const ch = rest.charCodeAt(i);
      // * _ ~ [ ]
      if (ch === 42 || ch === 95 || ch === 126 || ch === 91 || ch === 93) paragraph++;
    }
    if (paragraph > delimiters) delimiters = paragraph;
    if (depth > MAX_MARKDOWN_DEPTH && delimiters > MAX_MARKDOWN_DELIMITERS) break;
  }
  return { depth, delimiters };
}

/** markdownLimits(text).depth. */
export function nestingDepth(text) {
  return markdownLimits(text).depth;
}

/**
 * Whether the text is past a limit (length, depth or delimiters), so it
 * renders as its source. The editor uses it to drop Markdown highlighting.
 */
export function pastMarkdownLimits(text) {
  text = String(text ?? '');
  if (text.length > MAX_MARKDOWN_LENGTH) return true;
  const { depth, delimiters } = markdownLimits(text);
  return depth > MAX_MARKDOWN_DEPTH || delimiters > MAX_MARKDOWN_DELIMITERS;
}

// The text as it is, in one paragraph that keeps its line breaks.
function asSource(text) {
  const fragment = document.createDocumentFragment();
  const p = document.createElement('p');
  p.setAttribute('class', 'wk-markdown-source');
  p.textContent = text;
  if (text) fragment.append(p);
  return fragment;
}

/**
 * Markdown text as a DocumentFragment, under the rules above. Put it in an
 * element with the class wk-markdown, which the module styles. It does not
 * throw: text past MAX_MARKDOWN_LENGTH, MAX_MARKDOWN_DEPTH or
 * MAX_MARKDOWN_DELIMITERS (see markdownLimits), or that the
 * parser cannot handle, comes back as its source in a plain paragraph
 * (p.wk-markdown-source).
 *
 * options: onlink(url) for a link's click; imageHosts: hosts whose https
 * images load; highlight: false for plain code blocks.
 */
export function renderMarkdown(text, options = {}) {
  mountPreviewStyles();
  text = String(text ?? '');
  if (pastMarkdownLimits(text)) return asSource(text);
  try {
    const tokens = Lexer.lex(text, { gfm: true, breaks: false, async: false });
    return new Builder(options).blocks(tokens, document.createDocumentFragment());
  } catch {
    // A stack overflow (or any other parser failure) on hostile text.
    return asSource(text);
  }
}

/**
 * renderMarkdown's output in a <div class="wk-markdown">, ready to place
 * (a diff view's comment body, a posted comment).
 */
export function renderMarkdownElement(text, options = {}) {
  const el = document.createElement('div');
  el.setAttribute('class', 'wk-markdown');
  el.append(renderMarkdown(text, options));
  return el;
}
