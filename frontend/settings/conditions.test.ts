import { describe, expect, it } from "vitest";
import { conditionMet } from "./conditions";

describe("conditionMet", () => {
  it("returns true when condition is undefined", () => {
    expect(conditionMet(undefined, {})).toBe(true);
  });

  it("returns true when field value matches one of equals", () => {
    const cond = { field: "provider", equals: ["openai", "anthropic"] };
    expect(conditionMet(cond, { provider: "openai" })).toBe(true);
    expect(conditionMet(cond, { provider: "anthropic" })).toBe(true);
  });

  it("returns false when field value does not match", () => {
    const cond = { field: "provider", equals: ["openai", "anthropic"] };
    expect(conditionMet(cond, { provider: "local" })).toBe(false);
  });

  it("returns false when field is missing", () => {
    const cond = { field: "provider", equals: ["openai"] };
    expect(conditionMet(cond, {})).toBe(false);
  });

  it("treats non-string values via String() coercion", () => {
    const cond = { field: "count", equals: ["5"] };
    expect(conditionMet(cond, { count: 5 })).toBe(true);
  });
});
