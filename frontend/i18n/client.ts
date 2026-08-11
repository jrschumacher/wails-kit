import type { Catalog } from "@wails-kit/types";
import { resolvePlural, resolveText } from "./resolve.js";
import type { Text } from "./text.js";

/** Default event name — mirrors i18n.EventChanged (i18n/localizer.go). */
export const DEFAULT_EVENT_NAME = "i18n:changed";

export interface I18nClientOptions {
  /** Fetches the current resolved catalog — typically i18n.Binding.GetCatalog. */
  fetchCatalog: () => Promise<Catalog>;
  /** Fetches the current locale tag — typically i18n.Binding.GetLocale. */
  fetchLocale: () => Promise<string>;
  /**
   * Subscribes to the app's runtime event bus and returns an unsubscribe
   * function. Typically wraps `wailsjs/runtime`'s EventsOn, e.g.:
   *
   *   subscribe: (name, handler) => {
   *     EventsOn(name, handler);
   *     return () => EventsOff(name);
   *   }
   *
   * This package stays Wails-JS-free (headless, like every other
   * @wails-kit/* logic package) — the caller supplies the actual event
   * transport. Omit this to manage refresh() calls yourself instead.
   */
  subscribe?: (eventName: string, handler: () => void) => () => void;
  /** Event name to subscribe to. Defaults to DEFAULT_EVENT_NAME. */
  eventName?: string;
}

export interface I18nClient {
  /** Resolve a Text against the currently cached catalog. */
  t(txt: Text, ...args: unknown[]): string;
  /** Resolve a Text's plural form against the currently cached catalog. */
  tn(txt: Text, n: number, ...args: unknown[]): string;
  /** The currently cached locale tag. "" until the first refresh() resolves. */
  locale(): string;
  /** The currently cached catalog. {} until the first refresh() resolves. */
  catalog(): Catalog;
  /** Re-fetches catalog and locale. Called automatically on the subscribed event. */
  refresh(): Promise<void>;
  /** Resolves once the first refresh() (run automatically at creation) completes. */
  ready: Promise<void>;
  /** Unsubscribes from the event bus, if subscribe was provided. */
  destroy(): void;
}

/**
 * createI18nClient wires catalog fetching, CLDR plural resolution, and
 * automatic refetch-on-i18n:changed into one small stateful helper —
 * the "catalog consumption ... + i18n:changed refetch helper" this package
 * exists for (docs/v2-roadmap.md WP-33). It holds no framework dependency:
 * wrap client.t/tn in your own reactive store (a Svelte store, a React
 * context, a Vue ref, ...) to get re-renders on locale change.
 */
export function createI18nClient(opts: I18nClientOptions): I18nClient {
  let catalog: Catalog = {};
  let locale = "";
  let unsubscribe: (() => void) | undefined;

  async function refresh(): Promise<void> {
    const [nextCatalog, nextLocale] = await Promise.all([
      opts.fetchCatalog(),
      opts.fetchLocale(),
    ]);
    catalog = nextCatalog;
    locale = nextLocale;
  }

  const ready = refresh();

  if (opts.subscribe) {
    unsubscribe = opts.subscribe(opts.eventName ?? DEFAULT_EVENT_NAME, () => {
      void refresh();
    });
  }

  return {
    t: (txt, ...args) => resolveText(catalog, txt, ...args),
    tn: (txt, n, ...args) => resolvePlural(catalog, txt, n, locale, ...args),
    locale: () => locale,
    catalog: () => catalog,
    refresh,
    ready,
    destroy: () => {
      unsubscribe?.();
      unsubscribe = undefined;
    },
  };
}
