// The rich-text editor's nesting limit: a document never nests quotes and
// lists so deep that its Markdown would be past MAX_MARKDOWN_DEPTH, so what
// it saves loads again as Markdown (in it, the Markdown editor and the
// renderer). No DOM, so node tests it.
import { MAX_MARKDOWN_DEPTH } from '../shared/markdown-limits.js';

// What a container adds to the limits scan's depth for the lines inside it,
// at most: its marker, and its content's indent at two columns a level.
function cost(node) {
  switch (node.type.name) {
    case 'blockquote': return 2;
    case 'bullet_list': return 2;
    case 'ordered_list': return 1 + Math.ceil((String(node.attrs.order + Math.max(0, node.childCount - 1)).length + 2) / 2);
    default: return 0;
  }
}

/** The document's nesting, as the limits scan would count its Markdown (an upper bound). */
export function nesting(doc) {
  let deepest = 0;
  const walk = (node, depth) => {
    if (depth > deepest) deepest = depth;
    if (node.isTextblock || node.isLeaf) return;
    node.forEach(child => walk(child, depth + cost(child)));
  };
  walk(doc, 0);
  return deepest;
}

/** Whether the document nests past the limit. */
export function tooDeep(doc) {
  return nesting(doc) > MAX_MARKDOWN_DEPTH;
}
