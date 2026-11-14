/* @wails-kit/ui 0.1.0. Built by scripts/build.mjs from src/; do not edit. */
// ---- kit.js ----
// @wails-kit/ui custom elements for the components that need
// behaviour a native element does not give. Light DOM only, no shadow DOM,
// no customized built-ins, no popover API, no dependencies. Data comes in
// through attributes and properties and goes out through bubbling DOM events
// named wk-*.
//
//   <wk-dialog>        a native <dialog>: modal, focus return, wk-open/wk-close
//   <wk-menu>          WAI-ARIA menu button; wk-select
//   <wk-context-menu>  the same menu, opened by right-click, Shift+F10 or the
//                      Menu key; wk-select
//   <wk-tabs>          WAI-ARIA tabs, automatic activation; wk-change
//   <wk-tooltip>       WAI-ARIA tooltip; wk-toggle
//
// Any element with data-wk-open="<id of a wk-dialog>" opens that dialog.
//
// A stepper (<span class="wk-stepper">) is plain markup: this module steps its
// input from the two arrow buttons, fires input and change, and disables an
// arrow at the input's min or max. syncSteppers() redraws the arrows after the
// app sets a value itself.
//
// Every element works when the app builds the DOM imperatively: it may be
// connected before its children exist (it wires itself when they arrive, and
// rewires when they are replaced), moved (disconnected and reconnected), or
// removed while open. Everything an element adds to the document, the window
// or other elements is removed when it is disconnected.

let uid = 0;
// An id for el, keeping the one it has; a new one is free in the document.
function ensureId(el, prefix) {
  if (el.id) return el.id;
  let id;
  do id = `${prefix}-${++uid}`; while (document.getElementById(id));
  el.id = id;
  return id;
}
const emit = (el, type, detail) => el.dispatchEvent(new CustomEvent(type, { bubbles: true, detail }));
const MARGIN = 4;
const FOCUSABLE = 'button, a[href], input, select, textarea, summary, [tabindex]';

// Places a fixed-position surface next to a rectangle, inside the viewport.
// align is 'start' (left edges meet) or 'center'; it flips to the other side
// when the preferred side has no room.
function place(surface, rect, { side = 'below', align = 'start', gap = 4 } = {}) {
  surface.style.left = '0px';
  surface.style.top = '0px';
  const box = surface.getBoundingClientRect();
  const vw = document.documentElement.clientWidth;
  const vh = document.documentElement.clientHeight;
  const below = rect.bottom + gap;
  const above = rect.top - gap - box.height;
  let top = side === 'above' ? above : below;
  if (side === 'below' && top + box.height > vh - MARGIN && above >= MARGIN) top = above;
  if (side === 'above' && top < MARGIN) top = below;
  let left = align === 'center' ? rect.left + rect.width / 2 - box.width / 2 : rect.left;
  left = Math.min(left, vw - MARGIN - box.width);
  top = Math.min(top, vh - MARGIN - box.height);
  surface.style.left = `${Math.round(Math.max(MARGIN, left))}px`;
  surface.style.top = `${Math.round(Math.max(MARGIN, top))}px`;
}

const inView = el => {
  const r = el.getBoundingClientRect();
  return r.bottom > 0 && r.right > 0 && r.top < innerHeight && r.left < innerWidth;
};
// Focuses el if it can take focus, without scrolling when it is already in
// view. Returns whether it now has focus.
function focusEl(el) {
  if (!el || el === document.body || !el.isConnected || typeof el.focus !== 'function') return false;
  el.focus(inView(el) ? { preventScroll: true } : undefined);
  return document.activeElement === el;
}
const focusless = () => !document.activeElement || document.activeElement === document.body;
const scrollOf = el => (el === document ? [scrollX, scrollY] : [el.scrollLeft, el.scrollTop]);

// Calls wire() whenever the element's own children change (and, for
// targets added with watch(), their subtree), until disconnected.
class Wired extends HTMLElement {
  constructor() {
    super();
    this.observer = new MutationObserver(() => this.wire());
  }

  connectedCallback() {
    this.observer.observe(this, { childList: true });
    this.wire();
  }

  disconnectedCallback() {
    this.observer?.disconnect();
    this.unwire();
  }

  // A fresh signal for the listeners of one wiring; unwire() aborts it.
  rewire() {
    this.wiring?.abort();
    this.wiring = new AbortController();
    return this.wiring.signal;
  }

  unwire() {
    this.wiring?.abort();
    this.wiring = null;
  }
}

// ---------------------------------------------------------------------------
// Menus. One controller drives a [role=menu] element for both the menu
// button and the context menu (WAI-ARIA APG "Menu Button" and "Menu" keyboard
// interaction): Up/Down move and wrap, Home/End jump, a printable key moves
// to the next item that starts with it, Enter/Space activate, Escape closes
// and returns focus, Tab closes. Disabled items (aria-disabled) can be
// focused but not activated. A pointer down outside, a resize, the window
// losing focus, or a scroll that moves the menu's anchor also close it.
// Whenever it closes with focus inside it, focus goes back where it was
// before the menu opened, else to the anchor (the button or the context
// target).
const focusItem = el => el.focus({ preventScroll: true });

class MenuController {
  constructor(host, menu, { onToggle, fallbacks = () => [] } = {}) {
    this.host = host;
    this.menu = menu;
    this.onToggle = onToggle;
    this.fallbacks = fallbacks;
    this.returnFocus = null;
    this.wiring = new AbortController();
    const signal = this.wiring.signal;
    this.onOutside = e => { if (!this.host.contains(e.target)) this.close(); };
    this.onViewport = () => this.close();
    // A scroll closes the menu only if a container holding its anchor has
    // scrolled since the menu opened. The event of a scroll that happened
    // before (focusing the anchor can cause one) arrives after the menu
    // opens, and is ignored; so is a scroll of anything else.
    this.onScroll = e => {
      const scroller = e.target === document.documentElement || e.target === document.scrollingElement ? document : e.target;
      const before = this.scrolls?.get(scroller);
      if (!before) return;
      const now = scrollOf(scroller);
      if (now[0] !== before[0] || now[1] !== before[1]) this.close();
    };
    menu.hidden = true;
    menu.tabIndex = -1;
    ensureId(menu, 'wk-menu');
    menu.addEventListener('keydown', e => this.keydown(e), { signal });
    menu.addEventListener('click', e => {
      const item = e.target.closest('[role^="menuitem"]');
      if (item && menu.contains(item)) this.activate(item);
    }, { signal });
    menu.addEventListener('pointermove', e => {
      const item = e.target.closest('[role^="menuitem"]');
      if (item && menu.contains(item) && document.activeElement !== item) focusItem(item);
    }, { signal });
  }

  get isOpen() { return !this.menu.hidden; }

  items() {
    return [...this.menu.querySelectorAll('[role="menuitem"],[role="menuitemcheckbox"],[role="menuitemradio"]')]
      .filter(el => !el.hidden && !el.closest('[hidden]:not([role="menu"])'));
  }

  open(rect, { focus = 'first', returnFocus = document.activeElement, side = 'below', anchor = null } = {}) {
    this.returnFocus = returnFocus;
    this.scrolls = new Map([[document, scrollOf(document)]]);
    for (let el = anchor?.parentElement; el; el = el.parentElement) this.scrolls.set(el, scrollOf(el));
    this.menu.hidden = false;
    for (const item of this.items()) item.tabIndex = -1;
    place(this.menu, rect, { side, gap: rect.width || rect.height ? 4 : 0 });
    const items = this.items();
    focusItem((focus === 'last' ? items[items.length - 1] : items[0]) || this.menu);
    this.opened = new AbortController();
    const signal = this.opened.signal;
    document.addEventListener('pointerdown', this.onOutside, { capture: true, signal });
    window.addEventListener('resize', this.onViewport, { signal });
    window.addEventListener('blur', this.onViewport, { signal });
    document.addEventListener('scroll', this.onScroll, { capture: true, signal });
    this.onToggle?.(true);
    emit(this.host, 'wk-toggle', { open: true });
  }

  // restore: false only when the menu reopens at once and takes focus itself.
  close({ restore = true } = {}) {
    if (!this.isOpen) return;
    const hadFocus = this.menu.contains(document.activeElement) || focusless();
    this.menu.hidden = true;
    this.opened?.abort();
    this.opened = null;
    if (restore && hadFocus) [this.returnFocus, ...this.fallbacks()].some(focusEl);
    this.returnFocus = null;
    this.scrolls = null;
    this.onToggle?.(false);
    emit(this.host, 'wk-toggle', { open: false });
  }

  // Closes the menu and removes every listener this controller added.
  destroy() {
    this.close();
    this.wiring.abort();
  }

  move(delta) {
    const items = this.items();
    if (!items.length) return;
    const at = items.indexOf(document.activeElement);
    const next = at < 0 ? (delta > 0 ? 0 : items.length - 1) : (at + delta + items.length) % items.length;
    focusItem(items[next]);
  }

  typeahead(key) {
    const items = this.items();
    const at = items.indexOf(document.activeElement);
    const k = key.toLowerCase();
    for (let i = 1; i <= items.length; i++) {
      const item = items[(at + i) % items.length];
      if (item.textContent.trim().toLowerCase().startsWith(k)) { focusItem(item); return; }
    }
  }

  keydown(e) {
    const items = this.items();
    switch (e.key) {
      case 'ArrowDown': e.preventDefault(); this.move(1); break;
      case 'ArrowUp': e.preventDefault(); this.move(-1); break;
      case 'Home': case 'PageUp': e.preventDefault(); if (items[0]) focusItem(items[0]); break;
      case 'End': case 'PageDown': e.preventDefault(); if (items.length) focusItem(items[items.length - 1]); break;
      case 'Escape': e.preventDefault(); e.stopPropagation(); this.close(); break;
      case 'Tab': this.close(); break; // focus returns, then Tab moves on from there
      case 'Enter': case ' ': {
        e.preventDefault();
        const item = e.target.closest('[role^="menuitem"]');
        if (item) this.activate(item);
        break;
      }
      default:
        if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) { e.preventDefault(); this.typeahead(e.key); }
    }
  }

  activate(item) {
    if (item.getAttribute('aria-disabled') === 'true') return;
    const role = item.getAttribute('role');
    if (role === 'menuitemcheckbox') {
      item.setAttribute('aria-checked', String(item.getAttribute('aria-checked') !== 'true'));
    } else if (role === 'menuitemradio') {
      const group = item.closest('[role="group"]') || this.menu;
      for (const r of group.querySelectorAll('[role="menuitemradio"]')) r.setAttribute('aria-checked', String(r === item));
    }
    const detail = {
      item,
      value: item.dataset.value ?? (item.value || item.textContent.trim()),
      checked: role === 'menuitem' ? undefined : item.getAttribute('aria-checked') === 'true',
      context: this.host.context ?? null,
    };
    this.close();
    emit(this.host, 'wk-select', detail);
  }
}

// <wk-menu>: a button followed by a [role=menu] (give the menu the hidden
// attribute in markup, so it does not show before the element upgrades).
// The WAI-ARIA menu button pattern: Enter, Space or a click opens it on the
// first item; Down opens it on the first item and Up on the last.
class WkMenu extends Wired {
  wire() {
    if (!this.isConnected) return false;
    const button = this.querySelector(':scope > button');
    const menu = this.querySelector(':scope > [role="menu"]');
    if (this.controller && button === this.button && menu === this.controller.menu) return true;
    this.unwire();
    if (!button || !menu) return false;
    this.button = button;
    this.controller = new MenuController(this, menu, {
      onToggle: open => button.setAttribute('aria-expanded', String(open)),
      fallbacks: () => [button],
    });
    const signal = this.controller.wiring.signal;
    button.setAttribute('aria-haspopup', 'menu');
    button.setAttribute('aria-controls', menu.id);
    button.setAttribute('aria-expanded', 'false');
    if (!menu.hasAttribute('aria-labelledby') && !menu.hasAttribute('aria-label')) {
      menu.setAttribute('aria-labelledby', ensureId(button, 'wk-menu-button'));
    }
    button.addEventListener('click', () => {
      if (this.controller.isOpen) this.controller.close();
      else this.show();
    }, { signal });
    button.addEventListener('keydown', e => {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        this.show(e.key === 'ArrowUp' ? 'last' : 'first');
      }
    }, { signal });
    return true;
  }

  unwire() {
    this.controller?.destroy();
    this.controller = null;
    this.button = null;
  }

  get open() { return Boolean(this.controller?.isOpen); }
  show(focus = 'first') {
    if (this.wire()) this.controller.open(this.button.getBoundingClientRect(), { focus, returnFocus: this.button, anchor: this.button });
  }
  close() { this.controller?.close(); }
}

// <wk-context-menu for="id">: a [role=menu] (hidden in markup) for the
// element named by for, or the parent element. The target is looked up each
// time a menu could open, so it may appear, change or be replaced later.
// Opens at the pointer on right-click, and at the focused element on
// Shift+F10 or the Menu key; focus returns where it was. The wk-select
// event's detail.context is the element the menu was opened on. A context
// menu whose target lies inside another's wins inside it.
const contextMenus = new Set();

class WkContextMenu extends Wired {
  static get observedAttributes() { return ['for']; }

  connectedCallback() {
    contextMenus.add(this);
    super.connectedCallback();
  }

  disconnectedCallback() {
    contextMenus.delete(this);
    super.disconnectedCallback();
  }

  attributeChangedCallback() {
    if (this.open && this.target !== this.openTarget) this.controller.close();
  }

  get target() {
    const id = this.getAttribute('for');
    return id ? document.getElementById(id) : this.parentElement;
  }

  wire() {
    if (!this.isConnected) return false;
    const menu = this.querySelector(':scope > [role="menu"]');
    if (this.controller && menu === this.controller.menu) return true;
    this.unwire();
    if (!menu) return false;
    this.controller = new MenuController(this, menu, { fallbacks: () => [this.context, this.openTarget] });
    const signal = this.controller.wiring.signal;
    document.addEventListener('contextmenu', e => this.onContextMenu(e), { signal });
    document.addEventListener('keydown', e => this.onKeydown(e), { signal });
    return true;
  }

  unwire() {
    this.controller?.destroy();
    this.controller = null;
  }

  // The target, if the event is for this menu: inside the target, outside
  // this element, and not inside a nested context menu's target.
  claims(e) {
    const target = this.target;
    if (!target || !(e.target instanceof Node) || !target.contains(e.target)) return null;
    for (const other of contextMenus) {
      if (other === this) continue;
      const inner = other.target;
      if (inner && inner !== target && target.contains(inner) && inner.contains(e.target)) return null;
    }
    return target;
  }

  onContextMenu(e) {
    if (this.contains(e.target)) { e.preventDefault(); return; }
    const target = this.claims(e);
    if (!target) return;
    e.preventDefault();
    // Shift+F10 and the Menu key can also fire contextmenu: the key already opened it.
    if (this.controller.isOpen && performance.now() - this.openedByKey < 500) return;
    this.openAt(e.target, new DOMRect(e.clientX, e.clientY, 0, 0), target);
  }

  onKeydown(e) {
    if (!((e.key === 'F10' && e.shiftKey) || e.key === 'ContextMenu') || this.contains(e.target)) return;
    const target = this.claims(e);
    if (!target) return;
    e.preventDefault();
    const r = e.target.getBoundingClientRect();
    this.openedByKey = performance.now();
    this.openAt(e.target, new DOMRect(r.left, r.bottom, 0, 0), target);
  }

  openAt(context, rect, target = this.target) {
    if (!this.wire()) return;
    const back = this.controller.isOpen ? this.controller.returnFocus : document.activeElement;
    if (this.controller.isOpen) this.controller.close({ restore: false });
    this.context = context;
    this.openTarget = target;
    this.controller.open(rect, { returnFocus: back, anchor: context });
  }

  get open() { return Boolean(this.controller?.isOpen); }
  close() { this.controller?.close(); }
}

// <wk-tabs>: a [role=tablist] of [role=tab] buttons and one [role=tabpanel]
// per tab (by aria-controls, else in order). The WAI-ARIA tabs pattern with
// automatic activation: Left/Right (Up/Down when aria-orientation is
// vertical) move and select, wrapping; Home/End select the first and last;
// only the selected tab is in the tab order. Disabled tabs are skipped.
class WkTabs extends Wired {
  wire() {
    if (!this.isConnected) return false;
    const tablist = this.querySelector(':scope > [role="tablist"]');
    const tabs = tablist ? [...tablist.querySelectorAll('[role="tab"]')] : [];
    const panels = [...this.querySelectorAll(':scope > [role="tabpanel"]')];
    const parts = [tablist, ...tabs, null, ...panels];
    if (this.wiring && parts.length === this.parts.length && parts.every((p, i) => p === this.parts[i])) return true;
    const selected = this.wiring ? this.tabs()[this.selectedIndex] : null;
    this.unwire();
    this.parts = parts;
    // Tabs may be added inside the tablist later too.
    this.observer.disconnect();
    this.observer.observe(this, { childList: true });
    if (!tablist) return false;
    this.observer.observe(tablist, { childList: true, subtree: true });
    this.tablist = tablist;
    const signal = this.rewire();
    tabs.forEach((tab, i) => {
      ensureId(tab, 'wk-tab');
      const panel = document.getElementById(tab.getAttribute('aria-controls')) || panels[i];
      if (!panel) return;
      tab.setAttribute('aria-controls', ensureId(panel, 'wk-tabpanel'));
      panel.setAttribute('aria-labelledby', tab.id);
      if (!panel.hasAttribute('tabindex')) panel.tabIndex = 0;
    });
    const start = tabs.includes(selected) ? tabs.indexOf(selected) : tabs.findIndex(t => t.getAttribute('aria-selected') === 'true');
    this.select(start < 0 ? 0 : start, { quiet: true });
    tablist.addEventListener('click', e => {
      const tab = e.target.closest('[role="tab"]');
      if (tab && !this.disabled(tab)) this.select(this.tabs().indexOf(tab));
    }, { signal });
    tablist.addEventListener('keydown', e => this.keydown(e), { signal });
    return true;
  }

  unwire() {
    super.unwire();
    this.parts = [];
  }

  tabs() { return this.tablist ? [...this.tablist.querySelectorAll('[role="tab"]')] : []; }
  disabled(tab) { return tab.disabled || tab.getAttribute('aria-disabled') === 'true'; }
  get selectedIndex() { return this.tabs().findIndex(t => t.getAttribute('aria-selected') === 'true'); }
  set selectedIndex(i) { if (this.wire()) this.select(i, { quiet: true }); }

  select(index, { focus = false, quiet = false } = {}) {
    const tabs = this.tabs();
    if (!tabs[index]) return;
    const before = this.selectedIndex;
    tabs.forEach((tab, i) => {
      const on = i === index;
      tab.setAttribute('aria-selected', String(on));
      tab.tabIndex = on ? 0 : -1;
      const panel = document.getElementById(tab.getAttribute('aria-controls'));
      if (panel) panel.hidden = !on;
    });
    if (focus) tabs[index].focus();
    if (!quiet && before !== index) {
      emit(this, 'wk-change', { index, tab: tabs[index], panel: document.getElementById(tabs[index].getAttribute('aria-controls')) });
    }
  }

  keydown(e) {
    const tabs = this.tabs();
    const at = tabs.indexOf(e.target.closest('[role="tab"]'));
    if (at < 0) return;
    const vertical = this.tablist.getAttribute('aria-orientation') === 'vertical';
    const prev = vertical ? 'ArrowUp' : 'ArrowLeft';
    const next = vertical ? 'ArrowDown' : 'ArrowRight';
    const step = (from, delta) => {
      for (let i = 1; i <= tabs.length; i++) {
        const j = (from + delta * i + tabs.length * i) % tabs.length;
        if (!this.disabled(tabs[j])) return j;
      }
      return from;
    };
    let to;
    if (e.key === next) to = step(at, 1);
    else if (e.key === prev) to = step(at, -1);
    else if (e.key === 'Home') to = step(tabs.length - 1, 1);
    else if (e.key === 'End') to = step(0, -1);
    else return;
    e.preventDefault();
    this.select(to, { focus: true });
  }
}

// <wk-tooltip>: an element followed by a [role=tooltip] (hidden in markup).
// The WAI-ARIA tooltip pattern: the tip describes its trigger
// (aria-describedby), shows on focus and, after a short delay, on hover. It
// stays while the pointer is over the trigger or the tip (WCAG 1.4.13) or
// focus is in the trigger, and hides when both have gone, or on Escape,
// which leaves focus where it is. An Escape that something else already
// handled (a menu closing, say) is left alone. delay="ms" sets the hover
// delay.
class WkTooltip extends Wired {
  wire() {
    if (!this.isConnected) return false;
    const tip = this.querySelector(':scope > [role="tooltip"]');
    const trigger = [...this.children].find(el => el !== tip) || null;
    if (this.wiring && tip === this.tip && trigger === this.trigger) return true;
    this.unwire();
    if (!tip || !trigger) return false;
    this.tip = tip;
    this.trigger = trigger;
    tip.hidden = true;
    const id = ensureId(tip, 'wk-tooltip');
    const described = (trigger.getAttribute('aria-describedby') || '').split(/\s+/).filter(Boolean);
    if (!described.includes(id)) trigger.setAttribute('aria-describedby', [...described, id].join(' '));
    const signal = this.rewire();
    this.addEventListener('pointerenter', () => {
      clearTimeout(this.timer);
      this.timer = setTimeout(() => this.show(), Number(this.getAttribute('delay') ?? 500));
    }, { signal });
    this.addEventListener('pointerleave', () => { if (!this.contains(document.activeElement)) this.hide(); }, { signal });
    this.addEventListener('focusin', () => this.show(), { signal });
    this.addEventListener('focusout', e => { if (!this.contains(e.relatedTarget)) this.hide(); }, { signal });
    return true;
  }

  unwire() {
    this.hide();
    super.unwire();
    if (this.trigger && this.tip) {
      const rest = (this.trigger.getAttribute('aria-describedby') || '').split(/\s+/).filter(t => t && t !== this.tip.id);
      if (rest.length) this.trigger.setAttribute('aria-describedby', rest.join(' '));
      else this.trigger.removeAttribute('aria-describedby');
    }
    this.tip = this.trigger = null;
  }

  get open() { return Boolean(this.tip && !this.tip.hidden); }

  show() {
    clearTimeout(this.timer);
    if (!this.wire() || this.open) return;
    this.tip.hidden = false;
    place(this.tip, this.trigger.getBoundingClientRect(), { side: 'above', align: 'center', gap: 6 });
    this.shown = new AbortController();
    document.addEventListener('keydown', e => {
      if (e.key === 'Escape' && !e.defaultPrevented) { e.preventDefault(); this.hide(); }
    }, { signal: this.shown.signal });
    emit(this, 'wk-toggle', { open: true });
  }

  hide() {
    clearTimeout(this.timer);
    if (!this.open) return;
    this.tip.hidden = true;
    this.shown?.abort();
    this.shown = null;
    emit(this, 'wk-toggle', { open: false });
  }
}

// <wk-dialog>: wraps a native <dialog> and opens it modally, so the browser
// gives the WAI-ARIA modal dialog behaviour (focus moves in and is held
// there, the page behind is inert, Escape cancels). The element adds:
// - focus return to the opener: the element that had focus when it opened,
//   or else the element whose click opened it (WebKit does not focus a
//   clicked button), restored by the kit rather than left to the browser;
// - an open attribute that reflects and drives the state;
// - [data-wk-close] buttons (their value becomes returnValue);
// - the wk-open and wk-close events;
// - light dismiss, only with the light-dismiss attribute: a click whose
//   press and release are both on the backdrop closes it with returnValue
//   "". Off by default. Use it for a dialog that only informs; a dialog or
//   sheet that asks a question or holds input the user could lose must not
//   have it.
// Removing an open wk-dialog closes it without a wk-close event.
class WkDialog extends Wired {
  static get observedAttributes() { return ['open']; }

  wire() {
    if (!this.isConnected) return false;
    const dialog = this.querySelector(':scope > dialog');
    if (this.wiring && dialog === this.dialog) return true;
    this.unwire();
    if (!dialog) return false;
    this.dialog = dialog;
    const signal = this.rewire();
    const title = dialog.querySelector('.wk-dialog-title');
    if (title && !dialog.hasAttribute('aria-labelledby') && !dialog.hasAttribute('aria-label')) {
      dialog.setAttribute('aria-labelledby', ensureId(title, 'wk-dialog-title'));
    }
    dialog.addEventListener('close', () => {
      this.reflect(false);
      const back = this.opener;
      this.opener = null;
      focusEl(back);
      emit(this, 'wk-close', { returnValue: dialog.returnValue });
    }, { signal });
    const onBackdrop = e => {
      if (e.target !== dialog) return false;
      const r = dialog.getBoundingClientRect();
      return e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom;
    };
    dialog.addEventListener('pointerdown', e => { this.pressedBackdrop = onBackdrop(e); }, { signal });
    dialog.addEventListener('click', e => {
      const pressed = this.pressedBackdrop;
      this.pressedBackdrop = false;
      const closer = e.target.closest('[data-wk-close]');
      if (closer && dialog.contains(closer)) { this.close(closer.value || closer.dataset.wkClose || ''); return; }
      if (pressed && onBackdrop(e) && this.hasAttribute('light-dismiss')) this.close('');
    }, { signal });
    if (this.hasAttribute('open') && !dialog.open) this.show();
    return true;
  }

  unwire() {
    super.unwire();
    const dialog = this.dialog;
    this.dialog = null;
    if (!dialog?.open) return;
    // Removed or replaced while open: close it and put focus back, quietly.
    const back = this.opener;
    this.opener = null;
    dialog.close();
    this.reflect(false);
    if (focusless()) focusEl(back);
  }

  reflect(open) {
    this.syncing = true;
    this.toggleAttribute('open', open);
    this.syncing = false;
  }

  attributeChangedCallback(name, old, value) {
    if (this.syncing) return;
    if (value !== null) this.show();
    else this.close();
  }

  get open() { return Boolean(this.dialog?.open); }

  // opener: the element to return focus to when nothing had focus, as when
  // a click in WebKit opened it.
  show({ opener } = {}) {
    if (!this.wire() || this.dialog.open) return;
    const clicked = window.event?.type === 'click' && window.event.target instanceof Element ? window.event.target.closest(FOCUSABLE) : null;
    this.opener = focusless() ? opener || clicked : document.activeElement;
    this.dialog.returnValue = '';
    this.dialog.showModal();
    this.reflect(true);
    emit(this, 'wk-open', {});
  }

  close(returnValue = '') {
    if (this.dialog?.open) this.dialog.close(returnValue);
  }
}

document.addEventListener('click', e => {
  const opener = e.target.closest?.('[data-wk-open]');
  if (!opener) return;
  const target = document.getElementById(opener.dataset.wkOpen);
  if (target instanceof WkDialog) { e.preventDefault(); target.show({ opener }); }
});

// ---- Stepper ---------------------------------------------------------------
const stepperLimit = (input, name, fallback) => { const n = input[name] === '' ? NaN : Number(input[name]); return Number.isFinite(n) ? n : fallback; };
// Disables the arrow that cannot go further, for one stepper or every stepper under root.
function syncSteppers(root = document) {
  for (const stepper of root.matches?.('.wk-stepper') ? [root] : root.querySelectorAll('.wk-stepper')) {
    const input = stepper.querySelector('input'), [more, fewer] = stepper.querySelectorAll('.wk-stepper-arrows > button');
    if (!input || !more || !fewer) continue;
    const value = parseFloat(input.value), locked = input.disabled || input.readOnly;
    more.disabled = locked || (Number.isFinite(value) && value >= stepperLimit(input, 'max', Infinity));
    fewer.disabled = locked || (Number.isFinite(value) && value <= stepperLimit(input, 'min', -Infinity));
  }
}
function stepBy(input, direction) {
  const step = stepperLimit(input, 'step', 1) > 0 ? stepperLimit(input, 'step', 1) : 1;
  const min = stepperLimit(input, 'min', -Infinity), max = stepperLimit(input, 'max', Infinity);
  const value = parseFloat(input.value);
  // Like the native spinner, step to the next value on the min/step grid: a
  // typed 2.5 goes up to 3 and down to 2, never to 3.5 or 1.5. An empty or
  // invalid field starts from its nearest limit (or 0).
  const base = Number.isFinite(min) ? min : 0, k = (value - base) / step;
  const onGrid = direction > 0 ? Math.floor(k + 1e-9) + 1 : Math.ceil(k - 1e-9) - 1;
  const from = Number.isFinite(value) ? Number((base + onGrid * step).toFixed(10)) : direction > 0 ? base : (Number.isFinite(max) ? max : 0);
  const next = Math.min(max, Math.max(min, from));
  if (String(next) === input.value) return;
  input.value = String(next);
  input.dispatchEvent(new Event('input', { bubbles: true }));
  input.dispatchEvent(new Event('change', { bubbles: true }));
}
document.addEventListener('click', e => {
  const button = e.target instanceof Element ? e.target.closest('.wk-stepper-arrows > button') : null;
  const stepper = button?.closest('.wk-stepper'), input = stepper?.querySelector('input');
  if (!input || button.disabled || input.disabled || input.readOnly) return;
  stepBy(input, button === stepper.querySelector('.wk-stepper-arrows > button') ? 1 : -1);
  syncSteppers(stepper);
  // An arrow that has just reached its limit is disabled: keep the keyboard on the stepper.
  if (button.disabled) input.focus();
});
document.addEventListener('input', e => { if (e.target instanceof Element && e.target.matches('.wk-stepper > input')) syncSteppers(e.target.closest('.wk-stepper')); });

const define = (name, cls) => { if (!customElements.get(name)) customElements.define(name, cls); };
define('wk-menu', WkMenu);
define('wk-context-menu', WkContextMenu);
define('wk-tabs', WkTabs);
define('wk-tooltip', WkTooltip);
define('wk-dialog', WkDialog);

export { WkMenu, WkContextMenu, WkTabs, WkTooltip, WkDialog, syncSteppers };

// ---- chart.js ----
// wk-chart: bars, stacked bars, and a line with a band, drawn as
// plain SVG with no dependencies. Every chart carries a legend, a 2 px gap between
// touching marks, hover and keyboard readout, and a table view that shows the same
// numbers as a kit table. Colours come from the kit's tokens through data-tone
// ("1" to "5" are --wk-color-chart-N, "neutral" is a secondary ink), so a chart
// follows light and dark with the page.
//
// The layout rules are plain functions with no DOM (niceTicks, labelStep, runs,
// roundedTop, roundedEnd, tableModel, readout), so node can test them
// (test/chart.test.mjs). wkChart builds the figure.
//
//   wkChart({
//     id: 'jobs',                       // unique on the page; names the controls
//     kind: 'bars' | 'line',
//     label: 'Jobs per day',            // the chart's title and accessible name
//     summary: '14 days, 134 jobs',     // optional: read after the title
//     caption: 'UTC',                   // optional: small text beside the title
//     categories: [{ label: 'Sat 8', name: 'Saturday 8 Nov', shaded: true }],
//     series: [{ key, label, tone: 1, values: [3, 0, null] }],  // null is "no data"
//     band: { key, label, tone: 1, values: [...] },  // line only: from the line up to these
//     stacked: false, orientation: 'vertical' | 'horizontal',   // bars only
//     format: v => String(v), ticks: max => [0, 5, 10], integer: false,
//     legendValues: false,              // show each series' total in the legend
//     shadeLabel: 'Weekend',            // names the shaded categories in the legend
//     categoryHeader: 'Day',            // the table's first column
//     view: 'chart' | 'table', onview: view => {},
//     selection: -1, onselection: index => {}, // category index, or -1 for none
//   }) -> <figure class="wk-chart">
const SVG = 'http://www.w3.org/2000/svg';
const GAP = 2;          // the surface between touching marks
const BAR_MAX = 24;     // a bar is never thicker than this
const RADIUS = 4;       // the data end of a bar
const CHAR = 6.4;       // an estimate of a label's width per character at 11 px
const HEIGHT = 220;

// ---- Rules with no DOM ---------------------------------------------------

/** Round tick values from 0 up to at least `max`, about `count` of them. `integer` keeps whole steps. */
export function niceTicks(max, count = 4, integer = false) {
  if (!(max > 0)) return [0, 1];
  const raw = max / count, mag = 10 ** Math.floor(Math.log10(raw));
  let step = [1, 2, 5, 10].map(m => m * mag).find(s => s >= raw);
  if (integer) step = Math.max(1, Math.ceil(step));
  const n = Math.ceil(max / step - 1e-9);
  return Array.from({ length: n + 1 }, (_, i) => Math.round(i * step * 1e9) / 1e9);
}

/** Show every k-th label so labels `width` px wide fit in bands `band` px apart. */
export function labelStep(band, width) { return Math.max(1, Math.ceil((width + 6) / Math.max(band, 1))); }

/** The runs of consecutive non-null values: [[index, value], ...] for each, so a gap breaks the line. */
export function runs(values) {
  const out = [];
  let run = null;
  values.forEach((v, i) => {
    if (v == null || Number.isNaN(v)) { run = null; return; }
    if (!run) out.push(run = []);
    run.push([i, v]);
  });
  return out;
}

const n2 = v => Math.round(v * 100) / 100;
// A box with its top corners rounded (a column's data end); r 0 is a plain box.
export function roundedTop(x, y, w, h, r = RADIUS) {
  r = Math.min(r, w / 2, h);
  return `M${n2(x)} ${n2(y + h)}V${n2(y + r)}${r ? `a${r} ${r} 0 0 1 ${r} ${-r}H${n2(x + w - r)}a${r} ${r} 0 0 1 ${r} ${r}` : `H${n2(x + w)}`}V${n2(y + h)}z`;
}
// A box with its right corners rounded (a row's data end).
export function roundedEnd(x, y, w, h, r = RADIUS) {
  r = Math.min(r, h / 2, w);
  return `M${n2(x)} ${n2(y)}H${n2(x + w - r)}${r ? `a${r} ${r} 0 0 1 ${r} ${r}V${n2(y + h - r)}a${r} ${r} 0 0 1 ${-r} ${r}` : `V${n2(y + h)}`}H${n2(x)}z`;
}

const stackedTotal = spec => spec.stacked && spec.series.length > 1;
const categoryName = (spec, c) => c.name || c.label || spec.label;

/** The table view's content: the same numbers as the chart, as header and rows of text. */
export function tableModel(spec) {
  const fmt = spec.tableFormat || spec.format || String;
  const cols = spec.series.map(s => ({ label: s.label, values: s.values }));
  if (spec.band) cols.push({ label: spec.band.label, values: spec.band.values });
  const cell = v => v == null ? '–' : fmt(v);
  const head = [spec.categoryHeader || '', ...cols.map(c => c.label)];
  if (stackedTotal(spec)) head.push('Total');
  const rows = spec.categories.map((c, i) => {
    const row = [categoryName(spec, c), ...cols.map(col => cell(col.values[i]))];
    if (stackedTotal(spec)) row.push(fmt(spec.series.reduce((sum, s) => sum + (s.values[i] || 0), 0)));
    return row;
  });
  return { head, rows };
}

/** What the tooltip and the live readout say for category i: a title and one row per series. */
export function readout(spec, i) {
  const fmt = spec.format || String;
  const c = spec.categories[i];
  const rows = [];
  for (const s of spec.series) if (s.values[i] != null && !(spec.stacked && !(s.values[i] > 0))) rows.push({ tone: s.tone, shape: spec.kind === 'line' ? 'line' : 'bar', label: s.label, text: fmt(s.values[i]) });
  if (spec.band && spec.band.values[i] != null) rows.push({ tone: spec.band.tone, shape: 'band', label: spec.band.label, text: fmt(spec.band.values[i]) });
  const title = categoryName(spec, c) + (c.shaded && spec.shadeLabel ? ` (${spec.shadeLabel.toLowerCase()})` : '');
  const text = rows.length ? rows.map(r => `${r.label} ${r.text}`).join(', ') : 'No data';
  return { title, rows, text: `${title}: ${text}` };
}

// ---- Drawing -------------------------------------------------------------
let sequence = 0;
function svgEl(tag, attrs, parent) {
  const el = document.createElementNS(SVG, tag);
  for (const [name, value] of Object.entries(attrs || {})) if (value != null && value !== false) el.setAttribute(name, value);
  parent?.append(el);
  return el;
}
function htmlEl(tag, cls, text, parent) {
  const el = document.createElement(tag);
  if (cls) el.className = cls;
  if (text != null) el.textContent = text;
  parent?.append(el);
  return el;
}
const sum = list => list.reduce((a, b) => a + (b || 0), 0);
const clip = (text, chars) => text.length > chars ? `${text.slice(0, Math.max(chars - 1, 1))}…` : text;

// Each draw returns the <svg> and how to place the tooltip for a category.
function drawColumns(spec, width) {
  const fmt = spec.format || String, n = spec.categories.length, line = spec.kind === 'line';
  const totals = spec.categories.map((_, i) => {
    const vals = spec.series.map(s => s.values[i] || 0);
    return spec.stacked ? sum(vals) : Math.max(0, ...vals, spec.band?.values[i] || 0);
  });
  const ticks = (spec.ticks || (max => niceTicks(max, 4, spec.integer)))(Math.max(0, ...totals));
  const top = ticks.at(-1) || 1;
  const m = { l: Math.max(...ticks.map(t => fmt(t).length)) * CHAR + 14, r: 8, t: 8, b: 24 };
  const pw = Math.max(width - m.l - m.r, 10), ph = HEIGHT - m.t - m.b, band = pw / Math.max(n, 1);
  const y = v => m.t + ph - (v / top) * ph;
  const svg = svgEl('svg', { width, height: HEIGHT, viewBox: `0 0 ${width} ${HEIGHT}`, 'aria-hidden': 'true', focusable: 'false' });
  spec.categories.forEach((c, i) => { if (c.shaded) svgEl('rect', { class: 'wk-chart-shade', x: n2(m.l + i * band), y: m.t, width: n2(band), height: ph }, svg); });
  const active = svgEl('rect', { class: 'wk-chart-active', y: m.t, width: n2(band), height: ph, hidden: '' }, svg);
  for (const t of ticks) {
    svgEl('line', { class: t === 0 ? 'wk-chart-axis' : 'wk-chart-grid', x1: m.l, x2: m.l + pw, y1: n2(y(t)), y2: n2(y(t)) }, svg);
    svgEl('text', { class: 'wk-chart-tick', x: m.l - 8, y: n2(y(t)), 'text-anchor': 'end', 'dominant-baseline': 'central' }, svg).textContent = fmt(t);
  }
  const step = labelStep(band, Math.max(0, ...spec.categories.map(c => c.label.length)) * CHAR);
  spec.categories.forEach((c, i) => {
    if (i % step === 0) svgEl('text', { class: 'wk-chart-tick', x: n2(m.l + (i + .5) * band), y: HEIGHT - 8, 'text-anchor': 'middle' }, svg).textContent = c.label;
  });
  const dots = [];
  if (!line) {
    const k = spec.stacked ? 1 : spec.series.length;
    const bw = Math.max(2, Math.min(BAR_MAX, (band * (spec.stacked ? 1 : .8) - GAP * (spec.stacked ? 2 : k + 1)) / k));
    spec.categories.forEach((_, i) => {
      if (spec.stacked) {
        let acc = 0;
        const segments = [];
        for (const s of spec.series) { const v = s.values[i]; if (v > 0) { segments.push({ s, from: y(acc), to: y(acc + v) }); acc += v; } }
        segments.forEach((g, j) => {
          const x = m.l + i * band + (band - bw) / 2;
          const bottom = g.from - (j > 0 ? GAP : 0), h = Math.max(bottom - g.to, 1.5);
          svgEl('path', { class: 'wk-chart-bar', 'data-tone': g.s.tone, d: roundedTop(x, bottom - h, bw, h, j === segments.length - 1 ? RADIUS : 0) }, svg);
        });
      } else {
        const group = k * bw + (k - 1) * GAP, x0 = m.l + i * band + (band - group) / 2;
        spec.series.forEach((s, j) => {
          const v = s.values[i];
          if (!(v > 0)) return;
          const h = Math.max(y(0) - y(v), 1.5);
          svgEl('path', { class: 'wk-chart-bar', 'data-tone': s.tone, d: roundedTop(x0 + j * (bw + GAP), y(0) - h, bw, h) }, svg);
        });
      }
    });
  } else {
    const x = i => m.l + (i + .5) * band;
    if (spec.band) {
      for (const run of runs(spec.band.values.map((v, i) => v != null && spec.series[0].values[i] != null ? v : null))) {
        if (run.length === 1) {
          const [i, v] = run[0], xx = n2(x(i)), yy = n2(y(v));
          // A one-point band has no area. Show its range and cap the upper value.
          svgEl('path', { class: 'wk-chart-line wk-chart-band-single', 'data-tone': spec.band.tone,
            d: `M${xx} ${n2(y(spec.series[0].values[i]))}V${yy}M${n2(xx - 8)} ${yy}H${n2(xx + 8)}` }, svg);
          continue;
        }
        const up = run.map(([i, v]) => `${n2(x(i))} ${n2(y(v))}`);
        const down = run.map(([i]) => `${n2(x(i))} ${n2(y(spec.series[0].values[i]))}`).reverse();
        svgEl('path', { class: 'wk-chart-band', 'data-tone': spec.band.tone, d: `M${up.join('L')}L${down.join('L')}z` }, svg);
      }
    }
    spec.series.forEach(s => {
      for (const run of runs(s.values)) {
        if (run.length > 1) svgEl('path', { class: 'wk-chart-line', 'data-tone': s.tone, d: `M${run.map(([i, v]) => `${n2(x(i))} ${n2(y(v))}`).join('L')}` }, svg);
        else svgEl('circle', { class: 'wk-chart-dot', 'data-tone': s.tone, cx: n2(x(run[0][0])), cy: n2(y(run[0][1])), r: 4 }, svg);
      }
      dots.push(svgEl('circle', { class: 'wk-chart-dot', 'data-tone': s.tone, r: 4, hidden: '' }, svg));
    });
  }
  return {
    svg,
    index: (px, py) => Math.min(n - 1, Math.max(0, Math.floor((px - m.l) / band))),
    show(i) {
      active.removeAttribute('hidden'); active.setAttribute('x', n2(m.l + i * band));
      dots.forEach((d, j) => {
        const v = spec.series[j].values[i];
        d.toggleAttribute('hidden', v == null);
        if (v != null) { d.setAttribute('cx', n2(m.l + (i + .5) * band)); d.setAttribute('cy', n2(y(v))); }
      });
      return { x: m.l + (i + .5) * band, y: m.t };
    },
    hide() { active.setAttribute('hidden', ''); dots.forEach(d => d.setAttribute('hidden', '')); },
    horizontal: false,
  };
}

function drawRows(spec, width) {
  const fmt = spec.format || String, n = spec.categories.length;
  const single = n === 1 && spec.stacked;
  const bh = single ? 28 : BAR_MAX, rowH = single ? 28 : bh + 10, height = n * rowH;
  const labelled = spec.categories.some(c => c.label);
  const labelChars = Math.max(0, ...spec.categories.map(c => c.label.length));
  const labelW = labelled ? Math.min(width * .5, labelChars * CHAR + 16) : 0;
  const totals = spec.categories.map((_, i) => spec.stacked ? sum(spec.series.map(s => s.values[i])) : Math.max(0, ...spec.series.map(s => s.values[i] || 0)));
  const max = Math.max(...totals, 0) || 1;
  const valueW = spec.stacked ? 0 : Math.max(...spec.series.flatMap(s => s.values.map(v => fmt(v || 0).length)), 1) * CHAR + 12;
  const pw = Math.max(width - labelW - valueW, 10);
  const svg = svgEl('svg', { width, height, viewBox: `0 0 ${width} ${height}`, 'aria-hidden': 'true', focusable: 'false' });
  const active = svgEl('rect', { class: 'wk-chart-active', x: 0, width, height: rowH, hidden: '' }, svg);
  spec.categories.forEach((c, i) => {
    const cy = i * rowH + rowH / 2, yy = cy - bh / 2;
    if (labelled) svgEl('text', { class: 'wk-chart-tick', x: labelW - 8, y: n2(cy), 'text-anchor': 'end', 'dominant-baseline': 'central' }, svg).textContent = clip(c.label, Math.floor((labelW - 12) / CHAR));
    if (spec.stacked) {
      const segments = spec.series.filter(s => s.values[i] > 0);
      let x = labelW;
      segments.forEach((s, j) => {
        const w = Math.max((s.values[i] / max) * pw - (j > 0 ? GAP : 0), 1.5);
        svgEl('path', { class: 'wk-chart-bar', 'data-tone': s.tone, d: (j === segments.length - 1 ? roundedEnd : (a, b, c2, d) => `M${a} ${b}h${n2(c2)}v${d}h${n2(-c2)}z`)(n2(x + (j > 0 ? GAP : 0)), n2(yy), n2(w), bh) }, svg);
        x += (s.values[i] / max) * pw;
      });
    } else {
      spec.series.forEach(s => {
        const v = s.values[i] || 0;
        if (v > 0) svgEl('path', { class: 'wk-chart-bar', 'data-tone': s.tone, d: roundedEnd(labelW, yy, Math.max((v / max) * pw, 1.5), bh) }, svg);
        svgEl('text', { class: 'wk-chart-value', x: n2(labelW + (v / max) * pw + 6), y: n2(cy), 'dominant-baseline': 'central' }, svg).textContent = fmt(v);
      });
    }
  });
  return {
    svg,
    index: (px, py) => Math.min(n - 1, Math.max(0, Math.floor(py / rowH))),
    show(i) { active.removeAttribute('hidden'); active.setAttribute('y', i * rowH); return { x: labelW + pw / 2, y: i * rowH + rowH }; },
    hide() { active.setAttribute('hidden', ''); },
    horizontal: true,
  };
}

function legend(spec) {
  const fmt = spec.format || String;
  const list = htmlEl('ul', 'wk-chart-legend');
  const entry = (shape, tone, text) => {
    const li = htmlEl('li', 'wk-chart-legend-item', null, list);
    const key = htmlEl('span', 'wk-chart-key', null, li);
    key.dataset.shape = shape; if (tone != null) key.dataset.tone = tone; key.setAttribute('aria-hidden', 'true');
    htmlEl('span', null, text, li);
  };
  for (const s of spec.series) entry(spec.kind === 'line' ? 'line' : 'bar', s.tone, spec.legendValues ? `${s.label} ${fmt(sum(s.values))}` : s.label);
  if (spec.band) entry('band', spec.band.tone, spec.band.label);
  if (spec.shadeLabel && spec.categories.some(c => c.shaded)) entry('shade', null, spec.shadeLabel);
  return list;
}

// Detached plots must not be retained by the shared ResizeObserver. Keep their
// callbacks weakly, so charts made before attachment (or reattached later) can
// resume observing without keeping discarded charts alive.
const sized = new WeakMap(); // plot → its redraw
const observing = new Set(); // only connected plots; swept on every DOM mutation
let sizeObserver = null, connectionObserver = null;
function syncSize(plot) {
  if (!sized.has(plot)) return;
  if (plot.isConnected) {
    if (!observing.has(plot)) {
      observing.add(plot);
      sizeObserver.observe(plot);
    }
  } else if (observing.delete(plot)) sizeObserver.unobserve(plot);
}
function syncSizesIn(node) {
  if (node.nodeType !== 1) return;
  syncSize(node);
  node.querySelectorAll('.wk-chart-plot').forEach(syncSize);
}
function watchSize(plot, redraw) {
  if (typeof ResizeObserver !== 'function') return null;
  sizeObserver ??= new ResizeObserver(entries => { for (const e of entries) if (e.target.isConnected) sized.get(e.target)?.(); });
  connectionObserver ??= new MutationObserver(records => {
    for (const observed of observing) if (!observed.isConnected) syncSize(observed);
    for (const record of records) {
      record.removedNodes.forEach(syncSizesIn);
      record.addedNodes.forEach(syncSizesIn);
    }
  });
  connectionObserver.observe(document, { childList: true, subtree: true });
  sized.set(plot, redraw);
  syncSize(plot);
  return plot;
}
function unwatchSize(plot) {
  sized.delete(plot);
  if (observing.delete(plot)) sizeObserver.unobserve(plot);
}

/** Builds the chart's <figure>. It redraws itself when its width changes. */
export function wkChart(spec) {
  const id = spec.id || `wk-chart-${++sequence}`;
  let view = spec.view === 'table' ? 'table' : 'chart';
  const fig = htmlEl('figure', 'wk-chart');
  fig.id = id; fig.setAttribute('aria-labelledby', `${id}-title`);
  const head = htmlEl('div', 'wk-chart-head', null, fig);
  const title = htmlEl('h3', 'wk-chart-title', spec.label, head);
  title.id = `${id}-title`;
  if (spec.caption) htmlEl('span', 'wk-chart-caption', spec.caption, title);
  const toggle = htmlEl('div', 'wk-segmented wk-chart-view', null, head);
  toggle.setAttribute('role', 'radiogroup'); toggle.setAttribute('aria-label', `${spec.label} view`);
  for (const [value, text] of [['chart', 'Chart'], ['table', 'Table']]) {
    const label = htmlEl('label', null, null, toggle), input = htmlEl('input', null, null, label);
    input.type = 'radio'; input.name = `${id}-view`; input.value = value; input.checked = value === view;
    htmlEl('span', null, text, label);
    input.addEventListener('change', () => { if (input.checked) { view = value; spec.onview?.(value); paint(); } });
  }
  fig.append(legend(spec));
  const body = htmlEl('div', 'wk-chart-body', null, fig);
  const status = htmlEl('p', 'wk-chart-readout', null, fig);
  status.setAttribute('role', 'status');
  let observed = null, current = null, width = 0, selection = Number.isInteger(spec.selection) ? spec.selection : -1, activeIndex = selection;
  const empty = spec.categories.length === 0 || spec.series.length === 0;

  function tooltip(plot, tip, at, i) {
    const r = readout(spec, i);
    tip.replaceChildren();
    htmlEl('p', 'wk-chart-tip-title', r.title, tip);
    for (const row of r.rows) {
      const line = htmlEl('p', 'wk-chart-tip-row', null, tip);
      const key = htmlEl('span', 'wk-chart-key', null, line);
      key.dataset.shape = row.shape; key.dataset.tone = row.tone; key.setAttribute('aria-hidden', 'true');
      htmlEl('strong', null, row.text, line);
      htmlEl('span', 'wk-chart-tip-name', row.label, line);
    }
    tip.removeAttribute('hidden');
    const flip = at.x > plot.clientWidth / 2;
    tip.style.left = flip ? 'auto' : `${Math.round(at.x + 12)}px`;
    tip.style.right = flip ? `${Math.round(plot.clientWidth - at.x + 12)}px` : 'auto';
    tip.style.top = `${Math.round(Math.max(0, at.y))}px`;
    status.textContent = r.text;
  }

  function drawChart(plot, w) {
    width = w;
    const drawn = (spec.orientation === 'horizontal' && spec.kind !== 'line' ? drawRows : drawColumns)(spec, w);
    current = drawn;
    plot.querySelector('svg')?.remove();
    plot.prepend(drawn.svg);
    const tip = plot.querySelector('.wk-chart-tip');
    const activate = i => { activeIndex = i; selection = i; spec.onselection?.(i); tooltip(plot, tip, drawn.show(i), i); };
    const clear = () => { activeIndex = -1; selection = -1; spec.onselection?.(-1); drawn.hide(); tip.setAttribute('hidden', ''); status.textContent = ''; };
    drawn.svg.addEventListener('pointermove', e => { const box = drawn.svg.getBoundingClientRect(); activate(drawn.index(e.clientX - box.left, e.clientY - box.top)); });
    drawn.svg.addEventListener('pointerleave', () => { if (document.activeElement !== plot) clear(); });
    plot.onkeydown = e => {
      const n = spec.categories.length, next = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
      if (e.key === 'Escape' && activeIndex >= 0) { clear(); e.preventDefault(); e.stopPropagation(); }
      else if (next || e.key === 'Home' || e.key === 'End') {
        e.preventDefault();
        activate(e.key === 'Home' ? 0 : e.key === 'End' ? n - 1 : Math.min(n - 1, Math.max(0, activeIndex < 0 ? (next > 0 ? 0 : n - 1) : activeIndex + next)));
      }
    };
    plot.onblur = clear;
    if (activeIndex >= 0) activate(Math.min(activeIndex, spec.categories.length - 1));
  }

  function paint() {
    if (observed) unwatchSize(observed); observed = null; current = null; activeIndex = selection; status.textContent = '';
    body.replaceChildren();
    if (view === 'table') {
      const { head: header, rows } = tableModel(spec);
      const scroll = htmlEl('div', 'wk-chart-scroll', null, body);
      scroll.tabIndex = 0; scroll.setAttribute('role', 'region'); scroll.setAttribute('aria-label', `${spec.label}, table`); scroll.id = `${id}-table`;
      const table = htmlEl('table', 'wk-table', null, scroll);
      const tr = htmlEl('tr', null, null, htmlEl('thead', null, null, table));
      header.forEach((text, i) => { const th = htmlEl('th', i ? 'wk-num' : null, text, tr); th.scope = 'col'; });
      const tbody = htmlEl('tbody', null, null, table);
      for (const row of rows) {
        const r = htmlEl('tr', null, null, tbody);
        row.forEach((text, i) => { const cell = htmlEl(i ? 'td' : 'th', i ? 'wk-num' : null, text, r); if (!i) cell.scope = 'row'; });
      }
      return;
    }
    if (empty) { htmlEl('p', 'wk-chart-none', spec.emptyText || 'No data', body); return; }
    const plot = htmlEl('div', 'wk-chart-plot', null, body);
    plot.tabIndex = 0; plot.setAttribute('role', 'group');
    plot.setAttribute('aria-label', `${spec.label}${spec.summary ? `, ${spec.summary}` : ''}. Use the arrow keys to read each value.`);
    htmlEl('div', 'wk-chart-tip', null, plot).setAttribute('hidden', '');
    drawChart(plot, Math.round(plot.clientWidth) || 640);
    observed = watchSize(plot, () => {
      const w = Math.round(plot.clientWidth);
      if (w > 0 && w !== width) drawChart(plot, w);
    });
  }
  paint();
  return fig;
}

// ---- settings-model.js ----
// The settings window's rules, with no DOM, so node can test them
// (test/settings.test.mjs). They read wails-kit's settings schema: the JSON
// that settings.Service.GetSchema() returns (frontend/types settings.ts).
// <wk-settings> (settings.js) draws what settingsModel() describes.

/** What GetValues() returns for a password field that holds a secret. Sending it back leaves the secret unchanged. */
export const SECRET_MASK = '••••••••';

/** Whether a field with this condition shows: no condition, or the controlling field's value is one of condition.equals. */
export function conditionMet(condition, values) {
  if (!condition) return true;
  const value = String(values?.[condition.field] ?? '');
  return (condition.equals || []).some(eq => value === eq);
}

/** The options a select field offers now: its dynamic options for the controlling field's value, else its static options. */
export function resolveOptions(field, values) {
  if (field.dynamicOptions) {
    const on = String(values?.[field.dynamicOptions.dependsOn] ?? '');
    return field.dynamicOptions.options?.[on] ?? [];
  }
  return field.options ?? [];
}

/** The first message for each field key, from settings.ValidationError[] (or null). */
export function errorsByField(errors) {
  const out = new Map();
  for (const e of errors || []) if (e && e.field && !out.has(e.field)) out.set(e.field, e.message || e.code || 'Invalid value');
  return out;
}

const fallback = field => {
  if (field.default !== undefined && field.default !== null) return field.default;
  if (field.type === 'toggle') return false;
  if (field.type === 'number') return null;
  return '';
};

/** A field's value: values[key], else its default, else false for a toggle, null for a number and "" otherwise. */
export function fieldValue(field, values) {
  const v = values?.[field.key];
  return v === undefined ? fallback(field) : v;
}

/**
 * The value a control holds, as the schema's type: a toggle's checked state
 * as a boolean, a number field's text as a number (null when empty or not a
 * number), anything else as a string.
 */
export function readValue(field, raw) {
  if (field.type === 'toggle') return Boolean(raw);
  if (field.type === 'number') {
    const text = String(raw ?? '').trim();
    if (text === '') return null;
    const n = Number(text);
    return Number.isFinite(n) ? n : null;
  }
  return raw == null ? '' : String(raw);
}

/** A value as text for a control or a read-only row. */
export function displayValue(field, value) {
  if (value == null) return '';
  if (field.type === 'toggle') return value ? 'On' : 'Off';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

/**
 * The selects whose options follow `key`, with the value each must take now:
 * when a controlling field changes, a dependent select whose value is no
 * longer offered moves to its first option ("" when it offers none).
 */
export function dependentResets(schema, key, values) {
  const out = {};
  for (const group of schema?.groups || []) {
    for (const field of group.fields || []) {
      if (field.type !== 'select' || field.dynamicOptions?.dependsOn !== key) continue;
      const options = resolveOptions(field, values);
      const value = String(fieldValue(field, values) ?? '');
      if (!options.some(o => o.value === value)) out[field.key] = options[0]?.value ?? '';
    }
  }
  return out;
}

/**
 * What the window shows for a schema, its values and the errors to show:
 *   { groups: [{ key, label, attention, rows, advanced }] }
 * rows and advanced hold one row per field, in schema order (advanced holds
 * the fields marked advanced): { key, type, label, description, placeholder,
 * visible, value, text, options, missing, disabled, error, min, max,
 * required }. attention is true when a visible field in the group has an
 * error. A select whose value is not among its options has missing: true; a
 * select with no options is disabled.
 */
export function settingsModel(schema, values = {}, errors = []) {
  const byField = errorsByField(errors);
  const groups = (schema?.groups || []).map(group => {
    const rows = [], advanced = [];
    for (const field of group.fields || []) {
      const visible = conditionMet(field.condition, values);
      const value = fieldValue(field, values);
      const v = field.validation || {};
      const row = {
        key: field.key,
        type: field.type,
        label: field.label || field.key,
        description: field.description || '',
        placeholder: field.placeholder || '',
        visible,
        value,
        text: displayValue(field, value),
        options: null,
        missing: false,
        disabled: false,
        error: visible ? byField.get(field.key) ?? null : null,
        min: v.min ?? null,
        max: v.max ?? null,
        required: Boolean(v.required),
      };
      if (field.type === 'select') {
        row.options = resolveOptions(field, values);
        row.missing = !row.options.some(o => o.value === String(value ?? ''));
        row.disabled = row.options.length === 0;
      }
      (field.advanced ? advanced : rows).push(row);
    }
    const attention = [...rows, ...advanced].some(r => r.error);
    return { key: group.key, label: group.label || group.key, attention, rows, advanced };
  });
  return { groups };
}

// ---- settings.js ----
// <wk-settings>: a settings window drawn from wails-kit's settings schema
// (settings.Service.GetSchema()). Light DOM, built from the kit's own pieces:
// settings tabs (one per group, through <wk-tabs>) when there is more than one
// group, a wk-panel--rows of wk-settings-row per group, an "Advanced"
// disclosure for fields marked advanced, and kit controls per field type:
//
//   text      <input class="wk-field">
//   password  <input class="wk-field" type="password">; shows the secret mask
//             when a secret is stored, and sends it back unchanged unless edited
//   select    <select class="wk-select">; dynamic options follow their field
//   toggle    <input class="wk-switch" role="switch">, the whole row its label
//   number    a wk-stepper, with validation.min and max as its limits
//   computed  read-only text
//
// Data in through properties: schema, values, errors (settings.ValidationError[]
// from SetValues), icons ({ groupKey: svg markup or a Node }) and validate (an
// optional (schema, values) => errors, such as @wails-kit/settings' validate).
// Data out through one bubbling event, wk-settings-change, fired when a control
// commits a value: detail { key, value, values, errors }. values holds every
// field; errors is what validate returned ([] without it). The app saves
// (SetValues) and sets errors from the reply.
let settingsSeq = 0;
const SETTINGS_EVENT = 'wk-settings-change';
const make = (tag, cls, text) => {
  const el = document.createElement(tag);
  if (cls) el.className = cls;
  if (text != null) el.textContent = text;
  return el;
};
const sameOptions = (a, b) => a.length === b.length && a.every((o, i) => o.value === b[i].value && o.label === b[i].label);

class WkSettings extends HTMLElement {
  constructor() {
    super();
    this.idPrefix = `wk-settings-${++settingsSeq}`;
    this.state = { schema: null, values: {}, errors: [], icons: {}, validate: null };
    this.rowsByKey = new Map();
    this.fieldsByKey = new Map();
    this.tabsByGroup = new Map();
    this.advancedByGroup = new Map();
  }

  connectedCallback() {
    if (!this.built && this.state.schema) this.render();
  }

  get schema() { return this.state.schema; }
  set schema(schema) { this.state.schema = schema || null; this.render(); }

  get values() { return { ...this.state.values }; }
  set values(values) { this.state.values = { ...(values || {}) }; this.sync(); }

  get errors() { return [...this.state.errors]; }
  set errors(errors) { this.state.errors = Array.isArray(errors) ? [...errors] : []; this.sync(); }

  get icons() { return this.state.icons; }
  set icons(icons) { this.state.icons = icons || {}; this.render(); }

  get validate() { return this.state.validate; }
  set validate(fn) { this.state.validate = typeof fn === 'function' ? fn : null; }

  idFor(key, part) { return `${this.idPrefix}-${String(key).replace(/[^\w-]/g, '_')}-${part}`; }

  // Rebuilds everything from the schema. Called when the schema or icons change.
  render() {
    const schema = this.state.schema;
    this.replaceChildren();
    this.rowsByKey.clear(); this.fieldsByKey.clear(); this.tabsByGroup.clear(); this.advancedByGroup.clear();
    this.built = Boolean(schema);
    if (!schema) return;
    const model = settingsModel(schema, this.state.values, this.state.errors);
    for (const group of schema.groups || []) for (const field of group.fields || []) this.fieldsByKey.set(field.key, field);
    if (!model.groups.length) {
      const empty = make('div', 'wk-empty');
      empty.append(make('p', null, 'There are no settings.'));
      this.append(empty);
      return;
    }
    const panes = model.groups.map(g => this.pane(g, model.groups.length === 1));
    if (model.groups.length === 1) {
      this.append(panes[0]);
    } else {
      const tabs = document.createElement('wk-tabs');
      tabs.className = 'wk-settings-window';
      const list = make('div', 'wk-settings-tabs');
      list.setAttribute('role', 'tablist');
      list.setAttribute('aria-label', this.getAttribute('label') || 'Settings');
      model.groups.forEach((g, i) => {
        const tab = make('button', 'wk-settings-tab');
        tab.type = 'button';
        tab.setAttribute('role', 'tab');
        tab.id = this.idFor(g.key, 'tab');
        tab.setAttribute('aria-controls', panes[i].id);
        tab.setAttribute('aria-selected', String(i === 0));
        const icon = this.state.icons[g.key];
        if (icon instanceof Node) tab.append(icon.cloneNode(true));
        else if (typeof icon === 'string' && icon) tab.insertAdjacentHTML('beforeend', icon);
        tab.append(make('span', null, g.label));
        list.append(tab);
        this.tabsByGroup.set(g.key, tab);
      });
      tabs.append(list, ...panes);
      this.append(tabs);
    }
    this.sync();
  }

  pane(group, only) {
    const pane = make('div', 'wk-settings-pane');
    pane.id = this.idFor(group.key, 'pane');
    pane.dataset.group = group.key;
    if (!only) pane.setAttribute('role', 'tabpanel');
    else pane.append(make('h2', 'wk-settings-heading', group.label));
    if (group.rows.length) {
      const box = make('div', 'wk-panel wk-panel--rows');
      for (const row of group.rows) box.append(this.row(row));
      pane.append(box);
    }
    if (group.advanced.length) {
      const toggle = make('button', 'wk-link wk-settings-advanced', 'Advanced');
      toggle.type = 'button';
      const box = make('div', 'wk-panel wk-panel--rows');
      box.id = this.idFor(group.key, 'advanced');
      box.hidden = true;
      toggle.setAttribute('aria-expanded', 'false');
      toggle.setAttribute('aria-controls', box.id);
      toggle.addEventListener('click', () => this.expand(group.key, toggle.getAttribute('aria-expanded') !== 'true'));
      for (const row of group.advanced) box.append(this.row(row));
      pane.append(toggle, box);
      this.advancedByGroup.set(group.key, { toggle, box });
    }
    return pane;
  }

  expand(groupKey, open) {
    const a = this.advancedByGroup.get(groupKey);
    if (!a) return;
    a.toggle.setAttribute('aria-expanded', String(open));
    a.box.hidden = !open;
  }

  row(r) {
    const field = this.fieldsByKey.get(r.key);
    const row = make('div', 'wk-settings-row');
    row.dataset.key = r.key;
    row.dataset.type = r.type;
    const controlId = this.idFor(r.key, 'control');
    const helpId = this.idFor(r.key, 'help');
    const errorId = this.idFor(r.key, 'error');
    const error = make('p', 'wk-error');
    error.id = errorId;
    error.hidden = true;
    let control;
    if (r.type === 'toggle') {
      const label = make('label', 'wk-check wk-check--row');
      const words = make('span', 'wk-settings-label');
      words.append(make('span', null, r.label));
      if (r.description) { const help = make('small', 'wk-settings-help', r.description); help.id = helpId; words.append(help); }
      control = make('input', 'wk-switch');
      control.type = 'checkbox';
      control.setAttribute('role', 'switch');
      label.append(words, control);
      row.append(label);
    } else {
      const words = make('div', 'wk-settings-label');
      const label = make(r.type === 'computed' ? 'span' : 'label', null, r.label);
      label.id = this.idFor(r.key, 'label');
      if (r.type !== 'computed') label.htmlFor = controlId;
      words.append(label);
      if (r.description) { const help = make('span', 'wk-settings-help', r.description); help.id = helpId; words.append(help); }
      row.append(words);
      if (r.type === 'select') {
        control = make('select', 'wk-select');
        row.append(control);
      } else if (r.type === 'number') {
        const stepper = make('span', 'wk-stepper');
        control = make('input', 'wk-field wk-num');
        control.type = 'number';
        control.inputMode = 'numeric';
        if (r.min != null) control.min = String(r.min);
        if (r.max != null) control.max = String(r.max);
        const arrows = make('span', 'wk-stepper-arrows');
        for (const name of ['More', 'Fewer']) { const b = make('button'); b.type = 'button'; b.setAttribute('aria-label', name); arrows.append(b); }
        stepper.append(control, arrows);
        row.append(stepper);
      } else if (r.type === 'computed') {
        control = make('output', 'wk-settings-value');
        control.setAttribute('aria-labelledby', label.id);
        row.append(control);
      } else {
        control = make('input', 'wk-field wk-settings-control');
        control.type = r.type === 'password' ? 'password' : 'text';
        control.autocomplete = 'off';
        control.spellcheck = false;
        if (r.placeholder) control.placeholder = r.placeholder;
        if (r.type === 'password') control.addEventListener('focus', () => { if (control.value === SECRET_MASK) control.select(); });
        row.append(control);
      }
    }
    control.id = controlId;
    if (r.required && r.type !== 'computed' && r.type !== 'toggle') control.setAttribute('aria-required', 'true');
    if (r.type !== 'computed') {
      control.addEventListener('change', () => this.commit(field, r.type === 'toggle' ? control.checked : control.value));
    }
    row.append(error);
    this.rowsByKey.set(r.key, { row, control, error, helpId: r.description ? helpId : null, options: null });
    return row;
  }

  commit(field, raw) {
    const values = this.state.values;
    values[field.key] = readValue(field, raw);
    Object.assign(values, dependentResets(this.state.schema, field.key, values));
    let errors = [];
    if (this.state.validate) {
      errors = this.state.validate(this.state.schema, { ...values }) || [];
      this.state.errors = [...errors];
    } else {
      // The app's errors stay until it sets new ones, except on the field just changed.
      this.state.errors = this.state.errors.filter(e => e.field !== field.key);
    }
    this.sync();
    this.dispatchEvent(new CustomEvent(SETTINGS_EVENT, { bubbles: true, detail: { key: field.key, value: values[field.key], values: { ...values }, errors: [...errors] } }));
  }

  // Brings every control in line with the values and errors, keeping the DOM
  // (and focus) as it is.
  sync() {
    const schema = this.state.schema;
    if (!this.built || !schema) return;
    const model = settingsModel(schema, this.state.values, this.state.errors);
    for (const group of model.groups) {
      for (const r of [...group.rows, ...group.advanced]) {
        const parts = this.rowsByKey.get(r.key);
        if (!parts) continue;
        const { row, control, error } = parts;
        row.hidden = !r.visible;
        if (r.type === 'toggle') control.checked = Boolean(r.value);
        else if (r.type === 'computed') control.textContent = r.text;
        else if (r.type === 'select') {
          const options = [...(r.missing ? [{ value: '', label: r.disabled ? 'None available' : 'Choose…', placeholder: true }] : []), ...r.options];
          if (!parts.options || !sameOptions(parts.options, options)) {
            control.replaceChildren(...options.map(o => {
              const opt = make('option', null, o.label);
              opt.value = o.value;
              if (o.placeholder) { opt.disabled = true; }
              return opt;
            }));
            parts.options = options;
          }
          control.value = r.missing ? '' : String(r.value);
          control.disabled = r.disabled;
        } else if (document.activeElement !== control || control.value === '') {
          control.value = r.text;
        }
        if (r.type === 'number') syncSteppers(row);
        const describedBy = [parts.helpId, r.error ? error.id : null].filter(Boolean);
        if (describedBy.length) control.setAttribute('aria-describedby', describedBy.join(' '));
        else control.removeAttribute('aria-describedby');
        if (r.error) { control.setAttribute('aria-invalid', 'true'); error.textContent = r.error; error.hidden = false; }
        else { control.removeAttribute('aria-invalid'); error.textContent = ''; error.hidden = true; }
      }
      this.tabsByGroup.get(group.key)?.toggleAttribute('data-attention', group.attention);
      if (group.advanced.some(r => r.error)) this.expand(group.key, true);
    }
  }
}

const defineSettings = (name, cls) => { if (!customElements.get(name)) customElements.define(name, cls); };
defineSettings('wk-settings', WkSettings);

export { WkSettings };
