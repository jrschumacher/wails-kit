import { describe, expect, it } from "vitest";
import type { Field } from "@wails-kit/types";
import { resolveOptions } from "./options";

describe("resolveOptions", () => {
  it("returns static options when no dynamic options", () => {
    const field: Field = {
      key: "provider",
      type: "select",
      label: "Provider",
      options: [
        { label: "A", value: "a" },
        { label: "B", value: "b" },
      ],
    };
    expect(resolveOptions(field, {})).toEqual([
      { label: "A", value: "a" },
      { label: "B", value: "b" },
    ]);
  });

  it("returns empty array when no options at all", () => {
    const field: Field = { key: "x", type: "select", label: "X" };
    expect(resolveOptions(field, {})).toEqual([]);
  });

  it("resolves dynamic options based on controlling field", () => {
    const field: Field = {
      key: "model",
      type: "select",
      label: "Model",
      dynamicOptions: {
        dependsOn: "provider",
        options: {
          anthropic: [{ label: "Claude", value: "claude" }],
          openai: [{ label: "GPT-4o", value: "gpt-4o" }],
        },
      },
    };
    expect(resolveOptions(field, { provider: "openai" })).toEqual([
      { label: "GPT-4o", value: "gpt-4o" },
    ]);
    expect(resolveOptions(field, { provider: "anthropic" })).toEqual([
      { label: "Claude", value: "claude" },
    ]);
  });

  it("returns empty array for unrecognized controlling value", () => {
    const field: Field = {
      key: "model",
      type: "select",
      label: "Model",
      dynamicOptions: {
        dependsOn: "provider",
        options: {
          openai: [{ label: "GPT-4o", value: "gpt-4o" }],
        },
      },
    };
    expect(resolveOptions(field, { provider: "unknown" })).toEqual([]);
  });
});
