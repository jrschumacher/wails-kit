import { describe, expect, it } from "vitest";
import type { Catalog } from "@wails-kit/types";
import { resolvePlural, resolveText } from "./resolve.js";
import { T } from "./text.js";

describe("resolveText", () => {
  it("falls back to Text.other when the catalog has no entry", () => {
    const catalog: Catalog = {};
    expect(resolveText(catalog, T("myapp.greeting", "Hello, %s!"), "Ada")).toBe(
      "Hello, Ada!",
    );
  });

  it("resolves a plain-string catalog entry over the fallback", () => {
    const catalog: Catalog = { "myapp.greeting": "Bonjour, %s !" };
    expect(resolveText(catalog, T("myapp.greeting", "Hello, %s!"), "Ada")).toBe(
      "Bonjour, Ada !",
    );
  });

  it("uses a plural entry's 'other' form for resolveText", () => {
    // A caller might resolveText() a key that happens to be registered as
    // plural elsewhere — resolveText always takes "other", matching
    // Localizer.T's behavior in Go (i18n/localizer.go).
    const catalog: Catalog = {
      "myapp.items": { one: "%d item", other: "%d items" },
    };
    expect(resolveText(catalog, T("myapp.items", "%d items"), 5)).toBe(
      "5 items",
    );
  });

  it("applies no formatting when called with no args", () => {
    const catalog: Catalog = { "myapp.static": "Static text" };
    expect(resolveText(catalog, T("myapp.static", "fallback"))).toBe(
      "Static text",
    );
  });
});

describe("resolvePlural", () => {
  const catalog: Catalog = {
    "myapp.items": { one: "%d item", other: "%d items" },
  };
  const txt = T("myapp.items", "%d items");

  it("selects the singular category for n=1 in en", () => {
    expect(resolvePlural(catalog, txt, 1, "en")).toBe("1 item");
  });

  it("selects the plural category for n=5 in en", () => {
    expect(resolvePlural(catalog, txt, 5, "en")).toBe("5 items");
  });

  it("selects the plural category for n=0 in en (no 'zero' form)", () => {
    expect(resolvePlural(catalog, txt, 0, "en")).toBe("0 items");
  });

  it("falls back to 'other' when the selected category is absent from the entry", () => {
    // Polish has distinct "few"/"many" categories english lacks; the entry
    // below only carries one/other, so a "few"-selecting n must still
    // resolve via 'other' rather than throwing or returning undefined.
    const sparse: Catalog = { "myapp.items": { other: "%d elementów" } };
    expect(resolvePlural(sparse, txt, 3, "pl")).toBe("3 elementów");
  });

  it("falls back to Text.other when the catalog has no entry at all", () => {
    expect(resolvePlural({}, txt, 5, "en")).toBe("5 items");
  });

  it("falls back to a plain-string entry (non-plural) unchanged", () => {
    const plain: Catalog = { "myapp.items": "%d thing(s)" };
    expect(resolvePlural(plain, txt, 5, "en")).toBe("5 thing(s)");
  });

  it("falls back to 'other' for an unparseable locale", () => {
    expect(resolvePlural(catalog, txt, 5, "not-a-real-locale-tag!!")).toBe(
      "5 items",
    );
  });

  it("puts n first and extra args after, mirroring Localizer.TN in Go", () => {
    // i18n/plural.go's TN always does fmt.Sprintf(format, append([]any{n},
    // args...)...) — n is always the first substitution, so a plural
    // template's first verb should be the count (e.g. "%d files in %s"),
    // not an arbitrary earlier verb.
    const withFolder: Catalog = {
      "myapp.files": { one: "%d file in %s", other: "%d files in %s" },
    };
    const t = T("myapp.files", "%d files in %s");
    expect(resolvePlural(withFolder, t, 3, "en", "Documents")).toBe(
      "3 files in Documents",
    );
  });
});
