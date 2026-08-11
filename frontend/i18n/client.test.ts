import { describe, expect, it, vi } from "vitest";
import type { Catalog } from "@wails-kit/types";
import { createI18nClient, DEFAULT_EVENT_NAME } from "./client.js";
import { T } from "./text.js";

function deferred<V>() {
  let resolve!: (v: V) => void;
  const promise = new Promise<V>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

describe("createI18nClient", () => {
  it("fetches the catalog and locale once, up front", async () => {
    const fetchCatalog = vi.fn().mockResolvedValue({ "myapp.greeting": "Hi, %s!" } satisfies Catalog);
    const fetchLocale = vi.fn().mockResolvedValue("en");

    const client = createI18nClient({ fetchCatalog, fetchLocale });
    await client.ready;

    expect(fetchCatalog).toHaveBeenCalledTimes(1);
    expect(fetchLocale).toHaveBeenCalledTimes(1);
    expect(client.locale()).toBe("en");
    expect(client.t(T("myapp.greeting", "Hello, %s!"), "Ada")).toBe("Hi, Ada!");
  });

  it("t()/tn() resolve against a placeholder catalog before ready settles", async () => {
    const { promise, resolve } = deferred<Catalog>();
    const client = createI18nClient({
      fetchCatalog: () => promise,
      fetchLocale: () => Promise.resolve("en"),
    });

    // Before the first fetch resolves, catalog() is {} — t() must still
    // fall back to Text.other rather than throwing.
    expect(client.t(T("myapp.greeting", "Hello, %s!"), "Ada")).toBe(
      "Hello, Ada!",
    );

    resolve({ "myapp.greeting": "Bonjour, %s !" });
    await client.ready;
    expect(client.t(T("myapp.greeting", "Hello, %s!"), "Ada")).toBe(
      "Bonjour, Ada !",
    );
  });

  it("subscribes to the event bus and re-fetches on the event", async () => {
    let handler: (() => void) | undefined;
    const unsubscribe = vi.fn();
    const subscribe = vi.fn((eventName: string, h: () => void) => {
      expect(eventName).toBe(DEFAULT_EVENT_NAME);
      handler = h;
      return unsubscribe;
    });
    const fetchCatalog = vi
      .fn()
      .mockResolvedValueOnce({ "k": "v1" } satisfies Catalog)
      .mockResolvedValueOnce({ "k": "v2" } satisfies Catalog);
    const fetchLocale = vi
      .fn()
      .mockResolvedValueOnce("en")
      .mockResolvedValueOnce("fr");

    const client = createI18nClient({ fetchCatalog, fetchLocale, subscribe });
    await client.ready;
    expect(client.locale()).toBe("en");
    expect(fetchCatalog).toHaveBeenCalledTimes(1);

    handler?.();
    // refresh() is fire-and-forget from the subscription handler; wait a
    // tick for its promise chain to settle.
    await Promise.resolve();
    await Promise.resolve();

    expect(fetchCatalog).toHaveBeenCalledTimes(2);
    expect(client.locale()).toBe("fr");

    client.destroy();
    expect(unsubscribe).toHaveBeenCalledTimes(1);
  });

  it("uses a custom event name when provided", async () => {
    const subscribe = vi.fn(() => () => {});
    const client = createI18nClient({
      fetchCatalog: () => Promise.resolve({}),
      fetchLocale: () => Promise.resolve("en"),
      subscribe,
      eventName: "myapp:locale-changed",
    });
    await client.ready;

    expect(subscribe).toHaveBeenCalledWith(
      "myapp:locale-changed",
      expect.any(Function),
    );
  });

  it("does not throw destroy() when no subscribe was provided", async () => {
    const client = createI18nClient({
      fetchCatalog: () => Promise.resolve({}),
      fetchLocale: () => Promise.resolve("en"),
    });
    await client.ready;
    expect(() => client.destroy()).not.toThrow();
  });

  it("refresh() can be called manually without a subscription", async () => {
    const fetchCatalog = vi
      .fn()
      .mockResolvedValueOnce({ k: "v1" } satisfies Catalog)
      .mockResolvedValueOnce({ k: "v2" } satisfies Catalog);
    const client = createI18nClient({
      fetchCatalog,
      fetchLocale: () => Promise.resolve("en"),
    });
    await client.ready;
    expect(client.catalog()).toEqual({ k: "v1" });

    await client.refresh();
    expect(client.catalog()).toEqual({ k: "v2" });
  });
});
