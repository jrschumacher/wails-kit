// The no-CSP test page's script: one rich-text editor in each format.
import { wkRichTextEditor } from '../../dist/richtext.js';

const demo = document.getElementById('demo');
demo.append(
  wkRichTextEditor({ id: 'demo-rich', label: 'Markdown' }),
  wkRichTextEditor({ id: 'demo-rich-html', label: 'HTML', format: 'html' }),
);
window.demoReady = true;
