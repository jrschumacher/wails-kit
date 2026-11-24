// The design system's guidance for a component: the "When to use" and "Do
// and don't" sections of docs/design-system/components/<Name>/README.md,
// drawn as plain DOM (paragraphs, bullets, inline code and bold). Text is
// only ever set with textContent: the card is data, not markup.
const KEEP = ['When to use', "Do and don't"];

export function parseCard(text) {
  const sections = [];
  let current = null;
  for (const line of text.split('\n')) {
    const heading = /^##\s+(.*)$/.exec(line);
    if (heading) { current = KEEP.includes(heading[1].trim()) ? { title: heading[1].trim(), blocks: [] } : null; if (current) sections.push(current); continue; }
    if (!current) continue;
    const item = /^\s*[-*]\s+(.*)$/.exec(line);
    if (item) {
      const last = current.blocks[current.blocks.length - 1];
      if (last?.kind === 'list') last.items.push(item[1]); else current.blocks.push({ kind: 'list', items: [item[1]] });
    } else if (line.trim()) {
      const last = current.blocks[current.blocks.length - 1];
      if (last?.kind === 'p') last.text += ` ${line.trim()}`; else current.blocks.push({ kind: 'p', text: line.trim() });
    } else {
      current.blocks.push({ kind: 'gap' });
    }
  }
  for (const s of sections) s.blocks = s.blocks.filter(b => b.kind !== 'gap');
  return sections;
}

// Inline `code` and **bold**; everything else is text.
export function inline(text) {
  const frag = document.createDocumentFragment();
  const re = /`([^`]+)`|\*\*([^*]+)\*\*/g;
  let at = 0;
  for (let m; (m = re.exec(text));) {
    if (m.index > at) frag.append(text.slice(at, m.index));
    const el = document.createElement(m[1] !== undefined ? 'code' : 'b');
    el.textContent = m[1] ?? m[2];
    frag.append(el);
    at = re.lastIndex;
  }
  if (at < text.length) frag.append(text.slice(at));
  return frag;
}

export async function renderCard(host, name, depth) {
  let text;
  try {
    const res = await fetch(`../design-system/${encodeURIComponent(name)}/README.md`);
    if (!res.ok) throw new Error(String(res.status));
    text = await res.text();
  } catch {
    const p = document.createElement('p');
    p.className = 'wk-caption';
    p.textContent = 'The card could not be read.';
    host.append(p);
    return;
  }
  for (const section of parseCard(text)) {
    const h = document.createElement(`h${Math.min(depth, 6)}`);
    h.className = 'wk-caption gallery-card-heading';
    h.textContent = section.title;
    host.append(h);
    for (const block of section.blocks) {
      if (block.kind === 'p') {
        const p = document.createElement('p');
        p.append(inline(block.text));
        host.append(p);
      } else {
        const ul = document.createElement('ul');
        for (const item of block.items) {
          const li = document.createElement('li');
          li.append(inline(item));
          ul.append(li);
        }
        host.append(ul);
      }
    }
  }
  const from = document.createElement('p');
  from.className = 'wk-small';
  from.textContent = `docs/design-system/components/${name}/README.md, a reference export; its examples name the app it was drawn for.`;
  host.append(from);
}
