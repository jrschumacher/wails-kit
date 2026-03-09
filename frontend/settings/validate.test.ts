import { describe, expect, it } from "vitest";
import type { Field, Schema } from "@wails-kit/types";
import {
  validate,
  CodeRequired,
  CodePattern,
  CodeMinLen,
  CodeMaxLen,
  CodeMin,
  CodeMax,
  CodeInvalidType,
} from "./validate";

function makeSchema(...fields: Field[]): Schema {
  return { groups: [{ key: "test", label: "Test", fields }] };
}

describe("validate", () => {
  it("returns error for required field missing", () => {
    const schema = makeSchema({
      key: "name",
      type: "text",
      label: "Name",
      validation: { required: true },
    });
    const errs = validate(schema, {});
    expect(errs).toHaveLength(1);
    expect(errs[0].field).toBe("name");
    expect(errs[0].message).toBe("Name is required");
    expect(errs[0].code).toBe(CodeRequired);
  });

  it("returns error for required field empty string", () => {
    const schema = makeSchema({
      key: "name",
      type: "text",
      label: "Name",
      validation: { required: true },
    });
    const errs = validate(schema, { name: "" });
    expect(errs).toHaveLength(1);
  });

  it("passes for required field present", () => {
    const schema = makeSchema({
      key: "name",
      type: "text",
      label: "Name",
      validation: { required: true },
    });
    expect(validate(schema, { name: "Alice" })).toHaveLength(0);
  });

  it("passes valid pattern", () => {
    const schema = makeSchema({
      key: "email",
      type: "text",
      label: "Email",
      validation: { pattern: "^[^@]+@[^@]+\\.[^@]+$" },
    });
    expect(validate(schema, { email: "user@example.com" })).toHaveLength(0);
  });

  it("returns error for pattern mismatch", () => {
    const schema = makeSchema({
      key: "email",
      type: "text",
      label: "Email",
      validation: { pattern: "^[^@]+@[^@]+\\.[^@]+$" },
    });
    const errs = validate(schema, { email: "notanemail" });
    expect(errs).toHaveLength(1);
    expect(errs[0].message).toBe("Email has invalid format");
    expect(errs[0].code).toBe(CodePattern);
  });

  it("returns error for minLen violation", () => {
    const schema = makeSchema({
      key: "password",
      type: "password",
      label: "Password",
      validation: { minLen: 8 },
    });
    const errs = validate(schema, { password: "short" });
    expect(errs).toHaveLength(1);
    expect(errs[0].message).toBe("Password must be at least 8 characters");
    expect(errs[0].code).toBe(CodeMinLen);
  });

  it("returns error for maxLen violation", () => {
    const schema = makeSchema({
      key: "code",
      type: "text",
      label: "Code",
      validation: { maxLen: 4 },
    });
    const errs = validate(schema, { code: "toolong" });
    expect(errs).toHaveLength(1);
    expect(errs[0].message).toBe("Code must be at most 4 characters");
    expect(errs[0].code).toBe(CodeMaxLen);
  });

  it("passes minLen when long enough", () => {
    const schema = makeSchema({
      key: "password",
      type: "password",
      label: "Password",
      validation: { minLen: 8 },
    });
    expect(validate(schema, { password: "longenoughpassword" })).toHaveLength(0);
  });

  it("counts UTF-8 runes for minLen", () => {
    const schema = makeSchema({
      key: "name",
      type: "text",
      label: "Name",
      validation: { minLen: 3 },
    });
    // "日本語" is 3 code points — should pass MinLen=3
    expect(validate(schema, { name: "日本語" })).toHaveLength(0);
  });

  it("counts UTF-8 runes for maxLen", () => {
    const schema = makeSchema({
      key: "name",
      type: "text",
      label: "Name",
      validation: { maxLen: 4 },
    });
    // 3 code points — should pass MaxLen=4
    expect(validate(schema, { name: "日本語" })).toHaveLength(0);
    // 5 code points — should fail MaxLen=4
    expect(validate(schema, { name: "日本語五六" })).toHaveLength(1);
  });

  it("returns error for number below min", () => {
    const schema = makeSchema({
      key: "age",
      type: "number",
      label: "Age",
      validation: { min: 18 },
    });
    const errs = validate(schema, { age: 10 });
    expect(errs).toHaveLength(1);
    expect(errs[0].message).toBe("Age must be at least 18");
    expect(errs[0].code).toBe(CodeMin);
  });

  it("returns error for number above max", () => {
    const schema = makeSchema({
      key: "count",
      type: "number",
      label: "Count",
      validation: { max: 100 },
    });
    const errs = validate(schema, { count: 200 });
    expect(errs).toHaveLength(1);
    expect(errs[0].message).toBe("Count must be at most 100");
    expect(errs[0].code).toBe(CodeMax);
  });

  it("passes number within range", () => {
    const schema = makeSchema({
      key: "age",
      type: "number",
      label: "Age",
      validation: { min: 18 },
    });
    expect(validate(schema, { age: 25 })).toHaveLength(0);
  });

  it("validates integer values for min/max", () => {
    const schema = makeSchema({
      key: "count",
      type: "number",
      label: "Count",
      validation: { min: 1, max: 10 },
    });
    expect(validate(schema, { count: 5 })).toHaveLength(0);
    expect(validate(schema, { count: 0 })).toHaveLength(1);
  });

  it("returns error for toggle with non-boolean value", () => {
    const schema = makeSchema({
      key: "enabled",
      type: "toggle",
      label: "Enabled",
    });
    expect(validate(schema, { enabled: true })).toHaveLength(0);
    expect(validate(schema, {})).toHaveLength(0);
    const errs = validate(schema, { enabled: "yes" });
    expect(errs).toHaveLength(1);
    expect(errs[0].message).toBe("Enabled must be true or false");
    expect(errs[0].code).toBe(CodeInvalidType);
  });

  it("skips validation when condition is not met", () => {
    const schema = makeSchema(
      { key: "provider", type: "select", label: "Provider" },
      {
        key: "api_key",
        type: "password",
        label: "API Key",
        validation: { required: true },
        condition: { field: "provider", equals: ["openai", "anthropic"] },
      },
    );
    // provider is "local" -> condition not met -> api_key not validated
    expect(validate(schema, { provider: "local" })).toHaveLength(0);
    // provider is "openai" -> condition met -> api_key required
    const errs = validate(schema, { provider: "openai" });
    expect(errs).toHaveLength(1);
    expect(errs[0].field).toBe("api_key");
  });

  it("validates select option membership", () => {
    const schema = makeSchema({
      key: "provider",
      type: "select",
      label: "Provider",
      options: [
        { label: "Anthropic", value: "anthropic" },
        { label: "OpenAI", value: "openai" },
      ],
    });
    const errs = validate(schema, { provider: "invalid" });
    expect(errs).toHaveLength(1);
    expect(errs[0].field).toBe("provider");
  });

  it("validates dynamic select option membership", () => {
    const schema = makeSchema(
      {
        key: "provider",
        type: "select",
        label: "Provider",
        options: [
          { label: "Anthropic", value: "anthropic" },
          { label: "OpenAI", value: "openai" },
        ],
      },
      {
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
      },
    );
    // "claude" not valid when provider is "openai"
    const errs = validate(schema, { provider: "openai", model: "claude" });
    expect(errs).toHaveLength(1);
    expect(errs[0].field).toBe("model");

    // "gpt-4o" valid when provider is "openai"
    expect(validate(schema, { provider: "openai", model: "gpt-4o" })).toHaveLength(0);
  });

  it("returns no errors for field without validation", () => {
    const schema = makeSchema({
      key: "notes",
      type: "text",
      label: "Notes",
    });
    expect(validate(schema, {})).toHaveLength(0);
  });

  it("returns multiple errors for multiple invalid fields", () => {
    const schema = makeSchema(
      {
        key: "name",
        type: "text",
        label: "Name",
        validation: { required: true },
      },
      {
        key: "email",
        type: "text",
        label: "Email",
        validation: { required: true, pattern: "^[^@]+@[^@]+\\.[^@]+$" },
      },
    );
    const errs = validate(schema, {});
    expect(errs).toHaveLength(2);
    const fields = errs.map((e) => e.field);
    expect(fields).toContain("name");
    expect(fields).toContain("email");
  });
});
