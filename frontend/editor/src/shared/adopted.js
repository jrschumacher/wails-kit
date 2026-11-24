// The editors' styles reach the page as stylesheets adopted by the document
// (see scripts/build.mjs). Without adoptedStyleSheets (WebKit before Safari
// 16.4) they fall back to a <style> element, which a strict CSP blocks, and
// the editor renders broken: say so once. No editor library here, so every
// entry can use it.
let warned = false;
export function warnWithoutAdoptedSheets() {
  if (warned || typeof document === 'undefined' || 'adoptedStyleSheets' in document) return;
  warned = true;
  console.warn('@wails-kit/editor: this browser has no document.adoptedStyleSheets, so the editor\'s styles need a CSP that allows inline styles. It needs WebView2, Safari 16.4+ (macOS 13.3) or WebKitGTK 2.40+.');
}
