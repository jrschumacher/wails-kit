# @wails-kit/settings

Headless TypeScript library for settings schema logic. Framework-agnostic pure functions for condition evaluation, dynamic option resolution, and client-side validation.

Mirrors the Go `settings` package validation logic so frontend UIs can validate before calling the backend.

## Install

```bash
npm install @wails-kit/settings
```

## Usage

### Condition evaluation

Determine whether a field should be visible based on current values:

```ts
import { conditionMet } from "@wails-kit/settings";

const visible = conditionMet(field.condition, values);
```

Returns `true` if the condition is met (or if there is no condition).

### Dynamic option resolution

Resolve the active options for a select field:

```ts
import { resolveOptions } from "@wails-kit/settings";

const options = resolveOptions(field, values);
// Returns SelectOption[] — static options or dynamic options based on controlling field
```

### Validation

Validate all fields in a schema against current values:

```ts
import { validate } from "@wails-kit/settings";

const errors = validate(schema, values);
// Returns ValidationError[] — empty array if valid
```

Re-check one field as the person types, with their draft in the values (the
settings window does this once a field shows an error):

```ts
import { validateKey } from "@wails-kit/settings";

const errors = validateKey(schema, { ...values, [key]: draft }, key);
// The errors validate() would return for that field; [] when it is hidden or unknown
```

Each `ValidationError` has:

| Property  | Type             | Description               |
|-----------|------------------|---------------------------|
| `field`   | `string`         | Field key                 |
| `message` | `string`         | User-facing error message |
| `code`    | `ValidationCode` | Machine-readable code     |

### Error codes

| Code             | Meaning                                  |
|------------------|------------------------------------------|
| `required`       | Required field is missing or empty       |
| `pattern`        | Value doesn't match regex pattern        |
| `min_length`     | String shorter than minimum (rune count) |
| `max_length`     | String longer than maximum (rune count)  |
| `min`            | Number below minimum                     |
| `max`            | Number above maximum                     |
| `invalid_type`   | Wrong type (e.g. non-boolean toggle)     |
| `invalid_option` | Select value not in allowed options      |
| `check`          | The field's back-end check failed (`CodeCheck`; Go `Service.CheckValue`, never returned by `validate`) |

## Design

- **Pure functions** — no side effects, no framework dependencies
- **Mirrors Go** — identical validation logic and error messages to `settings/validate.go`
- **Unicode-aware** — string length checks count Unicode code points, not bytes
- **Depends on** `@wails-kit/types` for schema type definitions
