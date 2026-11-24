// Mounts rendered Markdown's stylesheet (markdown-style-spec.js) on the
// document once, as an adopted stylesheet, for the Markdown preview and the
// rich-text editor alike.
import { StyleModule } from 'style-mod';
import { previewStyles } from './markdown-style-spec.js';

let previewModule = null;
/** The preview's stylesheet, adopted by the document once. */
export function mountPreviewStyles() {
  if (typeof document === 'undefined' || !document.defaultView) return;
  previewModule ??= new StyleModule(previewStyles);
  StyleModule.mount(document, previewModule);
}
