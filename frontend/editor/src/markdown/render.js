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
import { highlightBlock } from '../shared/highlight.js';
import { mountPreviewStyles } from '../shared/markdown-styles.js';
import { decodeEntities, safeUrl } from '../shared/url.js';
import { pastMarkdownLimits } from '../shared/markdown-limits.js';

export { decodeEntities, safeUrl, mountPreviewStyles };
export {
  MAX_MARKDOWN_LENGTH, MAX_MARKDOWN_DEPTH, MAX_MARKDOWN_DELIMITERS, MAX_MARKDOWN_BLOCK_DELIMITERS,
  markdownLimits, exceedsMarkdownLimits, nestingDepth, pastMarkdownLimits,
} from '../shared/markdown-limits.js';

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
