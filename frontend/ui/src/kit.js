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
