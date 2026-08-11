// Settings page rendered from settings.Binding.GetSchema()/GetValues(),
// theme wiring from appearance:changed, a health badge from
// health.Binding.GetSnapshot()/health:changed, and an update toast from
// updates:*. No framework, no generated bindings — see AGENTS.md, "Why
// Call.ByName instead of generated bindings" for why, and how to switch.
import { Call, Events } from "@wailsio/runtime";
import type {
  Schema,
  Snapshot,
  AppearanceChangedPayload,
  UpdateAvailablePayload,
  UpdateDownloadingPayload,
  UpdateReadyPayload,
  UpdateErrorPayload,
} from "@wails-kit/types";
import {
  AppearanceChanged,
  HealthChanged,
  I18nChanged,
  UpdateAvailable,
  UpdateDownloading,
  UpdateReady,
  UpdateError,
} from "@wails-kit/types";
import { conditionMet, resolveOptions, validate } from "@wails-kit/settings";

// Fully-qualified binding names: "<go-import-path>.<RegisteredTypeName>.<Method>",
// verified against wails/v3's pkg/application/bindings.go FQN construction
// (fmt.Sprintf("%s.%s.%s", packagePath, typeName, methodName)) — see
// kit/wailsbridge/README.md and examples/kit-gui/frontend/app.js for the
// same pattern with fuller explanation.
const SETTINGS = "github.com/jrschumacher/wails-kit/v2/settings.Binding";
const HEALTH = "github.com/jrschumacher/wails-kit/v2/health.Binding";
// Kept for the "enable updates" opt-in below — see main.go.
// const UPDATES = "github.com/jrschumacher/wails-kit/v2/kit/wailsbridge.UpdatesBinding";

type EventEnvelope<T> = { data: T };

// --- Theme -------------------------------------------------------------
//
// wailsbridge does not register an appearance Wails service — mode changes
// go through the generic settings form below, via the "appearance" group's
// "mode" field, the same as any other setting. The frontend's only
// appearance-specific job is applying the resolved theme: an initial guess
// from prefers-color-scheme (there is no "get current theme" RPC, only
// change notifications), corrected and kept live by appearance:changed.
// This is the same value wailsbridge.ManageWindow uses for the window
// background — see style.css's colour variables.
function applyTheme(theme: "light" | "dark") {
  document.documentElement.setAttribute("data-theme", theme);
}
applyTheme(window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
Events.On(AppearanceChanged, (ev: EventEnvelope<AppearanceChangedPayload>) => {
  applyTheme(ev.data.resolved);
});

// --- Settings form -------------------------------------------------------

const groupsEl = document.getElementById("settings-groups")!;
const formEl = document.getElementById("settings-form") as HTMLFormElement;
const statusEl = document.getElementById("save-status")!;

type Field = Schema["groups"][number]["fields"][number];

// field.condition/dynamicOptions make a field's visibility or option list
// depend on another field's current value (e.g. an "auth method" picker
// gating an API-key field) — conditionMet/resolveOptions are the same pure
// logic settings/validate.go applies server-side, from @wails-kit/settings,
// so this doesn't reimplement (and risk drifting from) that behaviour.
function fieldInputHTML(field: Field, value: unknown, values: Record<string, unknown>): string {
  const id = `field-${field.key}`;
  switch (field.type) {
    case "password":
      // GetValues never returns the raw secret (settings.Binding masks it)
      // — leaving this empty on submit means "don't change this field".
      return `<input type="password" id="${id}" name="${field.key}" placeholder="(unchanged)" />`;
    case "toggle":
      return `<input type="checkbox" id="${id}" name="${field.key}" ${value ? "checked" : ""} />`;
    case "select": {
      const options = resolveOptions(field, values)
        .map((o) => `<option value="${o.value}" ${o.value === value ? "selected" : ""}>${o.label}</option>`)
        .join("");
      return `<select id="${id}" name="${field.key}">${options}</select>`;
    }
    case "computed":
      return `<input type="text" id="${id}" value="${value ?? ""}" disabled />`;
    default:
      return `<input type="text" id="${id}" name="${field.key}" value="${value ?? ""}" placeholder="${field.placeholder ?? ""}" />`;
  }
}

function renderGroups(schema: Schema, values: Record<string, unknown>) {
  groupsEl.innerHTML = schema.groups
    .map((group) => {
      const fields = group.fields
        .filter((field) => conditionMet(field.condition, values))
        .map((field) => {
          const value = values[field.key];
          return `
            <div class="field">
              <label for="field-${field.key}">${field.label}</label>
              ${fieldInputHTML(field, value, values)}
              ${field.description ? `<div class="description">${field.description}</div>` : ""}
            </div>
          `;
        })
        .join("");
      return fields ? `<fieldset><legend>${group.label}</legend>${fields}</fieldset>` : "";
    })
    .join("");
}

function collectValues(schema: Schema): Record<string, unknown> {
  const values: Record<string, unknown> = {};
  for (const group of schema.groups) {
    for (const field of group.fields) {
      const input = document.getElementById(`field-${field.key}`) as
        | HTMLInputElement
        | HTMLSelectElement
        | null;
      if (!input) continue;
      if (field.type === "computed") continue;
      if (field.type === "password" && (input as HTMLInputElement).value === "") continue;
      values[field.key] = field.type === "toggle" ? (input as HTMLInputElement).checked : input.value;
    }
  }
  return values;
}

let currentSchema: Schema | null = null;

async function loadSettings() {
  const [schema, values] = (await Promise.all([
    Call.ByName(`${SETTINGS}.GetSchema`),
    Call.ByName(`${SETTINGS}.GetValues`),
  ])) as [Schema, Record<string, unknown>];
  currentSchema = schema;
  renderGroups(schema, values);
}

// Re-render on any field change (not every keystroke — "change" fires on
// blur/selection, not per-character) so a field whose visibility or option
// list depends on another field stays live. collectValues snapshots the DOM
// first, so in-progress edits to unrelated fields survive the re-render.
formEl.addEventListener("change", (e) => {
  if (!currentSchema) return;
  if (!(e.target instanceof HTMLElement) || !e.target.id.startsWith("field-")) return;
  renderGroups(currentSchema, collectValues(currentSchema));
});

formEl.addEventListener("submit", async (e) => {
  e.preventDefault();
  if (!currentSchema) return;
  statusEl.textContent = "Saving…";
  try {
    const values = collectValues(currentSchema);
    const clientErrors = validate(currentSchema, values);
    if (clientErrors.length > 0) {
      statusEl.textContent = clientErrors.map((v) => v.message).join("; ");
      return;
    }
    const validationErrors = (await Call.ByName(`${SETTINGS}.SetValues`, values)) as
      | Array<{ field: string; message: string }>
      | null;
    if (validationErrors && validationErrors.length > 0) {
      statusEl.textContent = validationErrors.map((v) => v.message).join("; ");
      return;
    }
    statusEl.textContent = "Saved.";
    await loadSettings(); // re-render with persisted values (e.g. select coercion)
  } catch (err) {
    statusEl.textContent = `Error: ${err instanceof Error ? err.message : String(err)}`;
  }
});

// Re-fetch on locale changes too: GetSchema resolves labels through the
// current locale — see settings/README.md's "i18n:changed" recommendation.
Events.On(I18nChanged, () => {
  loadSettings().catch((err) => console.error("reload after i18n:changed failed:", err));
});

loadSettings().catch((err) => {
  statusEl.textContent = `Failed to load settings: ${err instanceof Error ? err.message : String(err)}`;
});

// --- Health badge --------------------------------------------------------

const healthBadgeEl = document.getElementById("health-badge")!;

function renderHealth(snapshot: Snapshot) {
  healthBadgeEl.dataset.state = snapshot.overall;
  healthBadgeEl.title = snapshot.offline
    ? "Offline"
    : snapshot.checks.map((c) => `${c.name}: ${c.state}`).join("\n") || "No health checks registered";
}

function refreshHealth() {
  Call.ByName(`${HEALTH}.GetSnapshot`)
    .then((snapshot: Snapshot) => renderHealth(snapshot))
    .catch((err: unknown) => console.error("health.GetSnapshot failed:", err));
}

refreshHealth();
Events.On(HealthChanged, refreshHealth);

// --- Update toast ----------------------------------------------------------
//
// Dormant until main.go's kit.WithGitHubRepo(...) is uncommented — with no
// GitHub repo configured, kit.Updates stays nil, wailsbridge never
// registers UpdatesBinding, and these events simply never fire.

const toastEl = document.getElementById("update-toast")!;
const toastMessageEl = document.getElementById("update-toast-message")!;
document.getElementById("update-toast-dismiss")!.addEventListener("click", () => {
  toastEl.hidden = true;
});

function showToast(message: string) {
  toastMessageEl.textContent = message;
  toastEl.hidden = false;
}

Events.On(UpdateAvailable, (ev: EventEnvelope<UpdateAvailablePayload>) => {
  showToast(`Update ${ev.data.version} is available.`);
});
Events.On(UpdateDownloading, (ev: EventEnvelope<UpdateDownloadingPayload>) => {
  showToast(`Downloading update ${ev.data.version}… ${Math.round(ev.data.progress * 100)}%`);
});
Events.On(UpdateReady, (ev: EventEnvelope<UpdateReadyPayload>) => {
  showToast(`Update ${ev.data.version} downloaded — restart to apply.`);
});
Events.On(UpdateError, (ev: EventEnvelope<UpdateErrorPayload>) => {
  showToast(`Update check failed: ${ev.data.message}`);
});

// Uncomment to actively check for updates on launch (requires
// kit.WithGitHubRepo(...) above to be uncommented in main.go too):
// Call.ByName(`${UPDATES}.CheckForUpdate`).catch((err: unknown) =>
//   console.error("updates.CheckForUpdate failed:", err),
// );
