// app.js — hand-written, no bundler, no generated bindings. A real app
// should prefer generated TS bindings (imported from
// `frontend/bindings/<full-go-import-path>/<service>` in wails-kit v2 —
// NOT v2 Wails' `wailsjs/go/...`, which is a different, older tool this
// project does not use). This example calls through the runtime's generic
// call surface instead, window.wails.Call.ByName(fullyQualifiedMethodName,
// ...args), because there is no codegen step in `go build`/`go run` for a
// bare example — see kit/wailsbridge/README.md and the WP-31 final report
// for why. The fully qualified name is
// "<go-import-path>.<RegisteredTypeName>.<MethodName>" — verified against
// wails/v3 pkg/application/bindings.go's own fqn construction
// (`fmt.Sprintf("%s.%s.%s", packagePath, typeName, methodName)`), not
// guessed.
(() => {
  const { Call, Events } = window.wails;

  const SETTINGS_PKG = "github.com/jrschumacher/wails-kit/v2/settings";
  const GET_SCHEMA = `${SETTINGS_PKG}.Binding.GetSchema`;
  const GET_VALUES = `${SETTINGS_PKG}.Binding.GetValues`;
  const SET_VALUES = `${SETTINGS_PKG}.Binding.SetValues`;

  const groupsEl = document.getElementById("settings-groups");
  const formEl = document.getElementById("settings-form");
  const statusEl = document.getElementById("save-status");
  const panelEl = document.getElementById("settings-panel");
  const themeValueEl = document.getElementById("theme-value");

  // --- Appearance ------------------------------------------------------
  //
  // wailsbridge does not register an appearance Wails service (there is
  // nothing for the frontend to *call* — mode changes go through the
  // generic settings form below, via the "appearance" group's "mode"
  // field, which is part of GetSchema/GetValues/SetValues like any other
  // setting). The frontend's only appearance-specific job is applying the
  // resolved theme: an initial guess from prefers-color-scheme (there is
  // no "get current theme" RPC — only change notifications), corrected
  // and kept live by the appearance:changed event thereafter. This is the
  // same value wailsbridge.ManageWindow uses for the window background
  // (see app.css's colour constants).
  function setTheme(theme) {
    document.documentElement.setAttribute("data-theme", theme);
    themeValueEl.textContent = theme;
  }

  setTheme(window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");

  Events.On("appearance:changed", (ev) => {
    if (ev.data && ev.data.resolved) {
      setTheme(ev.data.resolved);
    }
  });

  // --- Settings menu shortcut -------------------------------------------
  //
  // shortcuts.Manager emits this when the user presses the Settings
  // accelerator (Cmd+, / Ctrl+,) or picks the menu item — see
  // shortcuts/README.md. This panel is always visible in this minimal
  // example, so the handler just scrolls it into view.
  Events.On("settings:open", () => {
    panelEl.scrollIntoView({ behavior: "smooth" });
  });

  // --- Settings form -----------------------------------------------------

  function fieldInputHTML(field, value) {
    const id = `field-${field.key}`;
    switch (field.type) {
      case "password":
        // GetValues never returns the raw secret (settings.Binding masks
        // it) — the input starts empty; leaving it empty on Save means
        // "don't change this field", matching settings.Service.SetValues'
        // "only submitted keys are persisted" contract.
        return `<input type="password" id="${id}" name="${field.key}" placeholder="(unchanged)" />`;
      case "toggle":
        return `<input type="checkbox" id="${id}" name="${field.key}" ${value ? "checked" : ""} />`;
      case "select": {
        const options = (field.options || [])
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

  function renderGroups(schema, values) {
    groupsEl.innerHTML = schema.groups
      .map((group) => {
        const fields = group.fields
          .map((field) => {
            const value = values[field.key];
            return `
              <div class="field">
                <label for="field-${field.key}">${field.label}</label>
                ${fieldInputHTML(field, value)}
                ${field.description ? `<div class="description">${field.description}</div>` : ""}
              </div>
            `;
          })
          .join("");
        return `<fieldset><legend>${group.label}</legend>${fields}</fieldset>`;
      })
      .join("");
  }

  function collectValues(schema) {
    const values = {};
    for (const group of schema.groups) {
      for (const field of group.fields) {
        const input = document.getElementById(`field-${field.key}`);
        if (!input) continue;
        if (field.type === "computed") continue;
        if (field.type === "password" && input.value === "") continue; // "unchanged"
        values[field.key] = field.type === "toggle" ? input.checked : input.value;
      }
    }
    return values;
  }

  let currentSchema = null;

  async function loadSettings() {
    const [schema, values] = await Promise.all([Call.ByName(GET_SCHEMA), Call.ByName(GET_VALUES)]);
    currentSchema = schema;
    renderGroups(schema, values);
  }

  formEl.addEventListener("submit", async (e) => {
    e.preventDefault();
    statusEl.textContent = "Saving…";
    try {
      const values = collectValues(currentSchema);
      const validationErrors = await Call.ByName(SET_VALUES, values);
      if (validationErrors && validationErrors.length > 0) {
        statusEl.textContent = validationErrors.map((v) => v.message).join("; ");
        return;
      }
      statusEl.textContent = "Saved.";
      await loadSettings(); // re-render with the persisted values (e.g. select coercion)
    } catch (err) {
      statusEl.textContent = `Error: ${err && err.message ? err.message : err}`;
    }
  });

  // Re-fetch on locale changes too, since GetSchema resolves labels
  // through the current locale — see settings/README.md's "i18n:changed"
  // refetch-on-change recommendation, which this mirrors for
  // appearance's own settings-group labels changing language.
  Events.On("i18n:changed", () => {
    loadSettings().catch((err) => console.error("reload after i18n:changed failed:", err));
  });

  loadSettings().catch((err) => {
    statusEl.textContent = `Failed to load settings: ${err && err.message ? err.message : err}`;
  });
})();
