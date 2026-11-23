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
function imageUrl(href, hosts) {
  const url = safeUrl(href);
  if (!url || !hosts.length) return null;
  const { protocol, hostname } = new URL(url);
  return protocol === 'https:' && hosts.includes(hostname.toLowerCase()) ? url : null;
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
    this.hosts = Array.isArray(options.imageHosts) ? options.imageHosts.map(h => String(h).toLowerCase()) : [];
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

/**
 * Markdown text as a DocumentFragment, under the rules above. Put it in an
 * element with the class wk-markdown, which the module styles.
 *
 * options: onlink(url) for a link's click; imageHosts: hosts whose https
 * images load; highlight: false for plain code blocks.
 */
export function renderMarkdown(text, options = {}) {
  mountPreviewStyles();
  const tokens = Lexer.lex(String(text ?? ''), { gfm: true, breaks: false, async: false });
  return new Builder(options).blocks(tokens, document.createDocumentFragment());
}
