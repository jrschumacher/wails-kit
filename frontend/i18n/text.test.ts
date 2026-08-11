import { describe, expect, it } from "vitest";
import { T, format } from "./text.js";

describe("T", () => {
  it("builds a Text literal", () => {
    expect(T("myapp.greeting", "Hello, %s!")).toEqual({
      key: "myapp.greeting",
      other: "Hello, %s!",
    });
  });
});

describe("format", () => {
  it("returns the template unchanged with no args", () => {
    expect(format("no verbs here", [])).toBe("no verbs here");
  });

  it("substitutes %s", () => {
    expect(format("Hello, %s!", ["Ada"])).toBe("Hello, Ada!");
  });

  it("substitutes %d as an integer", () => {
    expect(format("%s must be at least %d characters", ["Password", 8.9])).toBe(
      "Password must be at least 8 characters",
    );
  });

  it("substitutes %v generically", () => {
    expect(format("value: %v", [true])).toBe("value: true");
  });

  it("substitutes %f", () => {
    expect(format("%f", [1.5])).toBe("1.5");
  });

  it("consumes args left to right across mixed verbs", () => {
    expect(format("%s scored %d (%f%%)", ["Ada", 9, 90.5])).toBe(
      "Ada scored 9 (90.5%)",
    );
  });

  it("renders a literal %% without consuming an arg", () => {
    expect(format("100%% done, %s", ["ok"])).toBe("100% done, ok");
  });
});
