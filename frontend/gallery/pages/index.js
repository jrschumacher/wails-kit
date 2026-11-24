// The gallery's pages, in the sidebar's order, grouped like the design
// system's cards. A page: { id, title, group, card?, intro?, examples }; an
// example: { title, note?, html?, build?(host, ctx), after?(host, ctx) }.
// html is drawn and shown as the markup; build and after run once the page
// is in the document, and their source is shown as the script.
import foundations from './foundations.js';
import actions from './actions.js';
import forms from './forms.js';
import containers from './containers.js';
import status from './status.js';
import overlays from './overlays.js';
import layout from './layout.js';
import data from './data.js';
import editors from './editors.js';

export const groups = [
  { id: 'foundations', title: 'Foundations' },
  { id: 'actions', title: 'Actions' },
  { id: 'forms', title: 'Forms' },
  { id: 'containers', title: 'Containers' },
  { id: 'status', title: 'Status' },
  { id: 'overlays', title: 'Overlays' },
  { id: 'layout', title: 'Layout' },
  { id: 'data', title: 'Data' },
  { id: 'editors', title: 'Editors' },
];
export const pages = [...foundations, ...actions, ...forms, ...containers, ...status, ...overlays, ...layout, ...data, ...editors];
