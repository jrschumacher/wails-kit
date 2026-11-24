// Markdown in and out of the rich-text editor's document, with no DOM (node
// tests it: test/richtext.test.mjs).
//
// In: marked's lexer (the Markdown renderer's parser, so both read text the
// same way) under the renderer's limits scan. Text past a limit, or that the
// parser fails on, is not parsed: it loads as its source, one plain
// paragraph per line (`source: true`), and goes out again exactly as it is.
//
// Out: one canonical spelling per feature, so text in that spelling comes
// back unchanged and any other spelling settles on it after one save:
//   # Heading, ## Subheading, **bold**, _italic_ (*italic* inside a word),
//   `code`, [text](url) (a bare URL, an e-mail address or <url> when the
//   text is the URL), - bullets (* for a list right after another), 1.
//   numbers (1) after another numbered list), > quotes, ``` fences with
//   their language, --- rules, a backslash before a line break, and a blank
//   line between blocks (none between the items of a tight list).
import { Lexer } from 'marked';
import { Fragment } from 'prosemirror-model';
import { decodeEntities } from '../shared/url.js';
import { pastMarkdownLimits } from '../shared/markdown-limits.js';
import { linkTarget, languageOf } from './schema.js';

// ---- In -------------------------------------------------------------------

class Reader {
  constructor(schema) { this.schema = schema; this.nodes = schema.nodes; this.marks = schema.marks; }

  text(text, marks) {
    // A soft line break inside a paragraph is a space, as the renderer shows it.
    text = text.replace(/\r\n?|\n/g, ' ');
    return text ? [this.schema.text(text, marks)] : [];
  }

  mark(type, attrs, marks) {
    return type ? type.create(attrs).addToSet(marks) : marks;
  }

  inline(tokens, marks = []) {
    const out = [];
    for (const t of tokens || []) {
      switch (t.type) {
        case 'text': out.push(...(t.tokens ? this.inline(t.tokens, marks) : this.text(decodeEntities(t.text), marks))); break;
        case 'escape': out.push(...this.text(t.text, marks)); break;
        case 'strong': out.push(...this.inline(t.tokens, this.mark(this.marks.strong, null, marks))); break;
        case 'em': out.push(...this.inline(t.tokens, this.mark(this.marks.em, null, marks))); break;
        // Not offered: the text stays, the strike goes.
        case 'del': out.push(...this.inline(t.tokens, marks)); break;
        case 'codespan': out.push(...this.text(t.text, this.mark(this.marks.code, null, marks))); break;
        case 'br': out.push(this.nodes.hard_break.create()); break;
        case 'link': {
          const href = linkTarget(t.href);
          out.push(...this.inline(t.tokens, href ? this.mark(this.marks.link, { href }, marks) : marks));
          break;
        }
        case 'image': {
          // No images: a link to it, labelled with its text, as the renderer shows one.
          const href = linkTarget(t.href);
          const alt = decodeEntities(t.text || '');
          out.push(...this.text(alt || (href ?? ''), href ? this.mark(this.marks.link, { href }, marks) : marks));
          break;
        }
        // Raw HTML is text, as written.
        case 'html': out.push(...this.text(t.text, marks)); break;
        default: out.push(...this.text(t.raw ?? t.text ?? '', marks));
      }
    }
    return out;
  }

  paragraph(content) { return this.nodes.paragraph.create(null, content); }

  // Lines of text as one paragraph with line breaks.
  lines(text) {
    const content = [];
    for (const [i, line] of String(text).split(/\r\n?|\n/).entries()) {
      if (i) content.push(this.nodes.hard_break.create());
      if (line) content.push(this.schema.text(line));
    }
    return this.paragraph(content);
  }

  blocks(tokens) {
    const out = [];
    for (const t of tokens || []) out.push(...this.block(t));
    return out;
  }

  block(t) {
    const { nodes } = this;
    switch (t.type) {
      case 'space': case 'def': return [];
      case 'paragraph': return [this.paragraph(this.inline(t.tokens))];
      case 'heading': {
        const content = this.inline(t.tokens);
        return [nodes.heading ? nodes.heading.create({ level: t.depth <= 1 ? 1 : 2 }, content) : this.paragraph(content)];
      }
      case 'hr': return nodes.horizontal_rule ? [nodes.horizontal_rule.create()] : [];
      case 'blockquote': {
        const inner = this.blocks(t.tokens);
        if (!nodes.blockquote) return inner;
        return [nodes.blockquote.create(null, inner.length ? inner : [this.paragraph()])];
      }
      case 'code': {
        if (!nodes.code_block) return [this.lines(t.text)];
        return [nodes.code_block.create({ language: languageOf(t.lang) }, t.text ? this.schema.text(t.text) : null)];
      }
      case 'list': return this.list(t);
      case 'table': {
        // No tables: each row a paragraph of its cells.
        const row = cells => {
          const content = [];
          for (const [i, c] of cells.entries()) {
            if (i) content.push(...this.text(' | '));
            content.push(...this.inline(c.tokens));
          }
          return this.paragraph(content);
        };
        return [row(t.header), ...t.rows.map(row)];
      }
      case 'html': return [this.lines(t.text.replace(/\n+$/, ''))];
      case 'text': return [this.paragraph(t.tokens ? this.inline(t.tokens) : this.text(decodeEntities(t.text)))];
      default: return [t.tokens ? this.paragraph(this.inline(t.tokens)) : this.lines(t.raw || '')];
    }
  }

  list(t) {
    const type = t.ordered ? this.nodes.ordered_list : this.nodes.bullet_list;
    const items = t.items.map(item => {
      const blocks = this.blocks(item.tokens.filter(x => x.type !== 'checkbox'));
      // No task lists: the box is kept as text.
      if (item.task) {
        const box = item.checked ? '[x] ' : '[ ] ';
        if (blocks[0]?.type === this.nodes.paragraph) blocks[0] = this.paragraph(Fragment.from(this.schema.text(box)).append(blocks[0].content));
        else blocks.unshift(this.paragraph(this.text(box.trim())));
      }
      return blocks;
    });
    if (!type) return items.flat();
    const attrs = { tight: !t.loose, ...(t.ordered ? { order: Number.isInteger(t.start) ? Math.min(t.start, 999999999) : 1 } : {}) };
    return [type.create(attrs, items.map(blocks => {
      if (blocks[0]?.type !== this.nodes.paragraph) blocks.unshift(this.paragraph());
      return this.nodes.list_item.create(null, blocks);
    }))];
  }
}

/** Plain text as a document: one paragraph per line, no formatting. */
export function sourceDoc(schema, text) {
  const lines = String(text ?? '').split(/\r\n?|\n/);
  return schema.nodes.doc.create(null, lines.map(line => schema.nodes.paragraph.create(null, line ? schema.text(line) : null)));
}

/** The text of a sourceDoc: its paragraphs' text, a line each. */
export function sourceText(doc) {
  const lines = [];
  doc.forEach(block => lines.push(block.textBetween(0, block.content.size, '\n', '\n')));
  return lines.join('\n');
}

/**
 * Markdown as { doc, source }: the document, and whether the text was past
 * the limits (or failed to parse) and loaded as its source (sourceDoc). It
 * does not throw for any text.
 */
export function parseMarkdown(schema, text) {
  text = String(text ?? '');
  if (pastMarkdownLimits(text)) return { doc: sourceDoc(schema, text), source: true };
  try {
    const blocks = new Reader(schema).blocks(Lexer.lex(text, { gfm: true, breaks: false, async: false }));
    const doc = schema.nodes.doc.create(null, blocks.length ? blocks : [schema.nodes.paragraph.create()]);
    doc.check();
    return { doc, source: false };
  } catch {
    // A stack overflow (or any other failure) on text the scan let through.
    return { doc: sourceDoc(schema, text), source: true };
  }
}

// ---- Out ------------------------------------------------------------------

const PUNCT = /[!-/:-@[-`{-~]/;
const alnum = c => !!c && /[\p{L}\p{N}]/u.test(c);
const space = c => !c || /\s/.test(c);

// Text as Markdown that reads back as the same text. `before` and `after`
// are the characters around it in the output ('' at a line's edge).
function escapeText(text, before, after) {
  let out = '';
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    const prev = i ? text[i - 1] : before;
    const next = i + 1 < text.length ? text[i + 1] : after;
    // A backslash at the end of a run goes as a character reference:
    // marked reads "\\" before a closing delimiter as an escape of it.
    if (c === '\\') out += i + 1 === text.length ? '&#92;' : PUNCT.test(next) ? '\\\\' : '\\';
    else if (c === '*' || c === '`' || c === '[' || c === ']' || c === '~') out += `\\${c}`;
    // An underscore inside a word cannot open or close emphasis.
    else if (c === '_') out += alnum(prev) && alnum(next) ? '_' : '\\_';
    else if (c === '<') out += /[A-Za-z/!?]/.test(next || '') ? '\\<' : '<';
    else if (c === '&') out += /^&(#x?[0-9a-f]+|[a-z][a-z0-9]*);/i.test(text.slice(i)) ? '\\&' : '&';
    // Plain text that GFM would make a link (a URL, a www. address, an
    // e-mail address) is escaped, so it is still plain text after a save.
    else if (c === ':' && next === '/' && /(?:^|[^\p{L}\p{N}])(?:https?|ftp)$/iu.test(before + text.slice(0, i))) out += '\\:';
    else if (c === '.' && /(?:^|[^\p{L}\p{N}])www$/iu.test(before + text.slice(0, i))) out += '\\.';
    else if (c === '@' && /[\w.+-]/.test(prev || '') && /\w/.test(next || '')) out += '\\@';
    else out += c;
  }
  return out;
}

// A line's start that would read as a block: a heading, quote, list item,
// rule or setext underline. The line is already escaped inline.
function escapeLineStart(line) {
  line = line.replace(/^[ \t]+/, '');
  if (/^#{1,6}(?:[ \t]|$)/.test(line) || line.startsWith('>')) return `\\${line}`;
  if (/^[-+](?:[ \t]|$)/.test(line)) return `\\${line}`;
  if (/^(?:-+|=+)[ \t]*$/.test(line)) return `\\${line}`;
  const n = line.match(/^(\d{1,9})([.)])(?:[ \t]|$)/);
  if (n) return `${n[1]}\\${line.slice(n[1].length)}`;
  // A table's delimiter row.
  if (line.includes('|') && /^\|?[ \t]*:?-+:?[ \t]*(?:\|[ \t]*:?-+:?[ \t]*)*\|?[ \t]*$/.test(line)) return line.replace(/\|/g, '\\|');
  return line;
}

function codeSpan(text) {
  const longest = Math.max(0, ...(text.match(/`+/g) || []).map(r => r.length));
  const fence = '`'.repeat(longest + 1);
  const pad = text.startsWith('`') || text.endsWith('`') || (text.startsWith(' ') && text.endsWith(' ') && text.trim()) ? ' ' : '';
  return `${fence}${pad}${text}${pad}${fence}`;
}

// A link's destination: its parentheses escaped unless they pair up.
function destination(href) {
  href = href.replace(/\\/g, '\\\\');
  let open = 0, paired = true;
  for (const c of href) {
    if (c === '(') open++;
    else if (c === ')' && --open < 0) paired = false;
  }
  return paired && !open ? href : href.replace(/[()]/g, '\\$&');
}

const prefix = (a, b) => { let i = 0; while (i < a.length && i < b.length && a[i].eq(b[i])) i++; return i; };

// How a link whose whole text is its target can be written without
// brackets: bare (GFM's autolinks read it back), in <>, or not at all.
// `after` is the text that follows. GFM reads a bare URL or address after a
// space or "(" and up to a space, leaving trailing ?!.,: out.
function autolink(text, href, before, after) {
  const bareOk = (space(before) || before === '(') && /^[?!.,:]*(?:\s|$)/.test(after);
  if (text === href && /^https?:\/\/[\w.-]+(?:[/?#][^\s<>]*)?$/.test(href) && !/[?!.,:*_~'")]$/.test(href) && !/[*_~[\]`\\&]/.test(href)) {
    return bareOk ? href : `<${href}>`;
  }
  if (href === `http://${text}` && /^www\.[\w-]+(?:\.[\w-]+)+$/.test(text) && bareOk) return text;
  if (href === `mailto:${text}` && /^[\w.+-]+@[\w-]+(?:\.[\w-]+)+$/.test(text) && !/[._-]$/.test(text)) return bareOk ? text : `<${text}>`;
  if (text === href && /^[a-z][a-z0-9+.-]{1,31}:[^\s<>]*$/i.test(href) && !/[*_~[\]`\\&]/.test(href)) return `<${href}>`;
  return null;
}

// A textblock's inline content as Markdown: one line, or several split by
// hard breaks ("\\\n"). `heading`: a line break is a space (a heading is one line).
// Emphasis that Markdown cannot spell: a delimiter run opens only where it
// is left-flanking and closes only where it is right-flanking (CommonMark).
// An italic ".js" right after "file" cannot open; nor can a bold "(x)" after
// "f". Such a mark is dropped for that span rather than written as literal
// asterisks. Neighbours are read from the text, without the delimiters of
// other marks.
const punct = c => !!c && /[\p{P}\p{S}]/u.test(c);
function dropUnflanked(runs) {
  for (const name of ['strong', 'em']) {
    for (let i = 0; i < runs.length;) {
      const has = r => !r.br && r.marks.some(m => m.type.name === name);
      if (!has(runs[i])) { i++; continue; }
      let j = i;
      while (j < runs.length && has(runs[j])) j++;
      const text = runs.slice(i, j).map(r => r.text).join('');
      const inner = text.trim();
      const prev = /^\s/.test(text) ? ' ' : (runs[i - 1] && !runs[i - 1].br ? runs[i - 1].text.slice(-1) : '');
      const next = /\s$/.test(text) ? ' ' : (runs[j] && !runs[j].br ? runs[j].text[0] : '');
      const first = inner[0], last = inner.slice(-1);
      const opens = !!first && (!punct(first) || space(prev) || punct(prev));
      const closes = !!last && (!punct(last) || space(next) || punct(next));
      if (!opens || !closes) for (let k = i; k < j; k++) runs[k].marks = runs[k].marks.filter(m => m.type.name !== name);
      i = j;
    }
  }
}

// A textblock's inline content as lines, cached per (immutable) node. The
// result is read back with marked: if any text would change, marks Markdown
// cannot spell are dropped (dropUnflanked), then, if that is not enough, all
// bold and italic in the block, so the text itself never changes.
const inlineCache = new WeakMap();
function inlineMarkdown(block, { heading = false } = {}) {
  const key = heading ? 'h' : 'p';
  const cached = inlineCache.get(block)?.[key];
  if (cached) return cached;
  // The text as Markdown keeps it: no spaces at a line's ends, no line
  // breaks at the end.
  const norm = text => text.split('\n').map(l => l.trim()).join('\n').replace(/\n+$/, '');
  const textOf = node => norm(node.textBetween(0, node.content.size, '', heading ? ' ' : '\n'));
  const want = textOf(block);
  const reads = lines => {
    const md = lines.join('\n');
    if (!md || pastMarkdownLimits(md)) return true;
    const { doc, source } = parseMarkdown(block.type.schema, md);
    return !source && doc.childCount === 1 && textOf(doc.firstChild) === want;
  };
  let lines = writeInline(runsOf(block), heading);
  if (!reads(lines)) {
    const runs = runsOf(block);
    dropUnflanked(runs);
    lines = writeInline(runs, heading);
    if (!reads(lines)) {
      const plain = runsOf(block);
      for (const r of plain) r.marks = r.marks.filter(m => m.type.name !== 'strong' && m.type.name !== 'em');
      lines = writeInline(plain, heading);
    }
  }
  inlineCache.set(block, { ...inlineCache.get(block), [key]: lines });
  return lines;
}

function runsOf(block) {
  const runs = [];
  block.forEach(node => {
    if (node.isText) runs.push({ text: node.text, marks: node.marks });
    else if (node.type.name === 'hard_break') runs.push({ br: true, marks: [] });
  });
  // marked reads brackets, backslashes and backticks in a code span inside a link's
  // text as the link's own: such code goes as plain (escaped) text, so the
  // text survives without its code look.
  for (const r of runs) {
    if (!r.br && /[[\]\\`]/.test(r.text) && r.marks.some(m => m.type.name === 'link') && r.marks.some(m => m.type.name === 'code')) {
      r.marks = r.marks.filter(m => m.type.name !== 'code');
    }
  }
  // Whitespace-only text keeps only the marks it continues on both sides.
  for (let i = 0; i < runs.length; i++) {
    const r = runs[i];
    if (r.br || /\S/.test(r.text)) continue;
    const prev = runs[i - 1]?.marks || [], next = runs[i + 1]?.marks || [];
    r.marks = r.marks.filter(m => m.isInSet(prev) && m.isInSet(next) && m.type.name !== 'code');
  }
  return runs;
}

function writeInline(runs, heading) {
  const lines = [];
  let out = '';
  const active = []; // { mark, close }
  const closeTo = n => { while (active.length > n) out += active.pop().close; };
  const textAfter = i => {
    const r = runs[i];
    return !r || r.br ? '' : r.text;
  };
  const charAfter = i => textAfter(i).slice(0, 1);
  for (let i = 0; i < runs.length; i++) {
    const run = runs[i];
    if (run.br) {
      closeTo(0);
      if (heading) { out += ' '; continue; }
      lines.push(out);
      out = '';
      continue;
    }
    const marks = run.marks;
    let text = run.text;
    closeTo(prefix(active.map(a => a.mark), marks));
    // Leading whitespace goes outside marks that open here.
    const opening = marks.slice(active.length);
    // A code span keeps its spaces.
    const isCode = marks.some(m => m.type.name === 'code');
    if (opening.length && !isCode) {
      const lead = text.match(/^\s*/)[0];
      if (lead) { out += lead; text = text.slice(lead.length); }
    }
    // Trailing whitespace goes outside marks that close after it.
    const next = runs[i + 1] && !runs[i + 1].br ? runs[i + 1].marks : [];
    const keep = prefix(marks, next);
    let trail = '';
    if (keep < marks.length && !isCode) {
      trail = text.match(/\s*$/)[0];
      text = text.slice(0, text.length - trail.length);
    }
    // An autolink: a link whose one run, with no other marks, is its target.
    const linkAt = opening.findIndex(m => m.type.name === 'link');
    if (linkAt === 0 && opening.length === 1 && marks.length === 1 && keep === 0 && !trail.length && text) {
      const auto = autolink(text, marks[0].attrs.href, out.slice(-1), textAfter(i + 1));
      if (auto != null) { out += auto; continue; }
    }
    for (const mark of opening) {
      const name = mark.type.name;
      let open, close;
      if (name === 'link') {
        // "!" before "[" would make an image.
        if (out.endsWith('!') && !out.endsWith('\\!')) out = `${out.slice(0, -1)}\\!`;
        open = '['; close = `](${destination(mark.attrs.href)})`;
      } else if (name === 'strong') { open = close = '**'; }
      else if (name === 'em') {
        // Inside a word, only * opens and closes emphasis.
        let j = i;
        while (j < runs.length && !runs[j].br && mark.isInSet(runs[j].marks)) j++;
        const afterEm = j === i + 1 && trail ? trail[0] : charAfter(j);
        open = close = alnum(out.slice(-1)) || alnum(afterEm) ? '*' : '_';
      } else if (name === 'code') { open = ''; close = ''; }
      out += open;
      active.push({ mark, close });
    }
    if (isCode) out += codeSpan(text);
    else out += escapeText(text, out.slice(-1), trail ? trail[0] : charAfter(i + 1));
    if (trail) { closeTo(keep); out += trail; }
  }
  closeTo(0);
  lines.push(out);
  const done = lines.map(l => escapeLineStart(l).replace(/[ \t]+$/, ''));
  // Line breaks at the end show nothing: drop them.
  while (done.length > 1 && !done[done.length - 1]) done.pop();
  // A backslash at the end of a line is a line break.
  return done.map((l, i) => (i < done.length - 1 ? `${l}\\` : l));
}

// Whether a tight list can be written tight: Markdown has no way to put two
// paragraphs, or a paragraph and a rule, in an item without a blank line, and
// some blocks cannot follow a paragraph without one.
function canBeTight(list) {
  let ok = true;
  list.forEach(item => {
    for (let i = 0; i + 1 < item.childCount; i++) {
      const a = item.child(i), b = item.child(i + 1);
      // A paragraph right after a list or quote would run on into it.
      if (b.type.name === 'paragraph' && /_list$|^blockquote$/.test(a.type.name)) ok = false;
      // Two quotes in a row would read as one.
      if (a.type.name === 'blockquote' && /^(blockquote|horizontal_rule)$/.test(b.type.name)) ok = false;
      if (a.type.name !== 'paragraph') continue;
      const n = b.type.name;
      if (n === 'paragraph' || n === 'horizontal_rule' || n === 'heading') ok = false;
      if (n === 'ordered_list' && b.attrs.order !== 1) ok = false;
      if ((n === 'bullet_list' || n === 'ordered_list') && !b.firstChild.firstChild.content.size) ok = false;
    }
  });
  return ok;
}

const indent = (lines, first, rest) => lines.map((l, i) => (l ? (i ? rest : first) + l : (i ? '' : first.trimEnd())));

// A block as lines. `previous` is the block before it in the same container.
function blockLines(node, alternate) {
  switch (node.type.name) {
    case 'paragraph': {
      const lines = inlineMarkdown(node);
      return lines.length === 1 && !lines[0] ? [] : lines;
    }
    case 'heading': {
      // A closing run of # would be read as the heading's end.
      const text = inlineMarkdown(node, { heading: true })[0].replace(/(^|\s)(#+)$/, '$1\\$2');
      return [`${'#'.repeat(node.attrs.level)} ${text}`.trimEnd()];
    }
    case 'horizontal_rule': return ['---'];
    case 'code_block': {
      const text = node.textContent;
      const longest = Math.max(2, ...(text.match(/^[ \t]*`{3,}/gm) || []).map(r => r.trim().length));
      const fence = '`'.repeat(longest + 1);
      return [`${fence}${node.attrs.language || ''}`, ...(text ? text.split('\n') : []), fence];
    }
    case 'blockquote': {
      const lines = containerLines(node);
      return lines.length ? lines.map(l => (l ? `> ${l}` : '>')) : ['>'];
    }
    case 'bullet_list': case 'ordered_list': {
      const ordered = node.type.name === 'ordered_list';
      // A list right after one of the same kind uses the other marker, or
      // the two would read as one list.
      const other = alternate;
      const tight = node.attrs.tight && canBeTight(node);
      const out = [];
      node.forEach((item, _, i) => {
        const marker = ordered ? `${node.attrs.order + i}${other ? ')' : '.'} ` : `${other ? '*' : '-'} `;
        const lines = containerLines(item, tight);
        if (i && !tight) out.push('');
        out.push(...indent(lines.length ? lines : [''], marker, ' '.repeat(marker.length)));
      });
      return out;
    }
    default: return [node.textContent];
  }
}

// A container's blocks as lines: a blank line between blocks, or none in a
// tight list item.
function containerLines(node, tight = false) {
  const out = [];
  let previous = null, alternate = false;
  node.forEach(child => {
    alternate = previous?.type === child.type && child.type.name.endsWith('_list') ? !alternate : false;
    const lines = blockLines(child, alternate);
    if (lines.length) {
      if (out.length) out.push(...(tight ? [] : ['']));
      out.push(...lines);
    }
    previous = child;
  });
  return out;
}

/** A document as Markdown, in the canonical spelling above. */
export function serializeMarkdown(doc) {
  return containerLines(doc).join('\n');
}
