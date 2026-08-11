package settings

import (
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/jrschumacher/wails-kit/v2/i18n"
)

// frCatalog is a minimal WithCatalog source translating one label and one
// validation message into French, used by every test in this file that
// needs a locale actually resolving through the catalog (as opposed to
// falling back to Text.Other).
func frCatalog() fstest.MapFS {
	return fstest.MapFS{
		"locales/fr.json": &fstest.MapFile{Data: []byte(`{
			"test.settings.theme.label": "Thème",
			"wailskit.settings.validation.required": "%s est requis"
		}`)},
	}
}

func themeGroup() Group {
	return Group{
		Key:   "appearance",
		Label: i18n.T("test.settings.appearance.label", "Appearance"),
		Fields: []Field{
			{
				Key:        "theme",
				Type:       FieldSelect,
				Label:      i18n.T("test.settings.theme.label", "Theme"),
				Validation: &Validation{Required: true},
				Default:    "system",
				Options: []SelectOption{
					{Value: "system", Label: i18n.T("test.settings.theme.option.system", "System")},
				},
			},
		},
	}
}

// TestSchemaResolvesLocale pins the core WP-12 behavior: a Field.Label with
// a catalog entry resolves through the service's localizer, and — the
// "locale switching re-resolving labels" requirement — a later
// l.SetLocale is picked up by the very next GetSchema call. GetSchema does
// not cache a resolved schema; resolution happens fresh on every call.
func TestSchemaResolvesLocale(t *testing.T) {
	l, err := i18n.New(i18n.WithCatalog(frCatalog()), i18n.WithLocale("en"))
	if err != nil {
		t.Fatalf("i18n.New: %v", err)
	}

	svc := NewService(WithGroup(themeGroup()), WithLocalizer(l))

	schema := svc.GetSchema()
	label := schema.Groups[0].Fields[0].Label
	if label != "Theme" {
		t.Fatalf("expected English fallback %q, got %q", "Theme", label)
	}

	if err := l.SetLocale("fr"); err != nil {
		t.Fatalf("SetLocale: %v", err)
	}

	schema = svc.GetSchema()
	label = schema.Groups[0].Fields[0].Label
	if label != "Thème" {
		t.Errorf("expected re-resolved French label %q after SetLocale, got %q", "Thème", label)
	}
}

// TestSchemaNoLocalizerFallsBackToLiteral is the "opt-in" contract: a
// Service built without WithLocalizer must render every i18n.Text as its
// literal Other value — no panic, no empty strings, no behavior different
// from a hypothetical caller that never touches i18n at all.
func TestSchemaNoLocalizerFallsBackToLiteral(t *testing.T) {
	svc := NewService(WithGroup(themeGroup()))

	schema := svc.GetSchema()
	group := schema.Groups[0]
	if group.Label != "Appearance" {
		t.Errorf("group.Label = %q, want literal %q", group.Label, "Appearance")
	}
	field := group.Fields[0]
	if field.Label != "Theme" {
		t.Errorf("field.Label = %q, want literal %q", field.Label, "Theme")
	}
	if len(field.Options) != 1 || field.Options[0].Label != "System" {
		t.Errorf("field.Options[0].Label = %#v, want literal %q", field.Options, "System")
	}
}

// TestSchemaWireShapeUnchanged locks the JSON shape GetSchema produces to
// exactly what it was before WP-12 (plain strings for every label, same
// field names) — AD-5's "no frontend change in rendering logic is needed"
// promise. If this test's expected JSON ever needs to change, the frontend
// contract changed and that's a deliberate, documented break, not an
// incidental one.
func TestSchemaWireShapeUnchanged(t *testing.T) {
	svc := NewService(WithGroup(themeGroup()))

	data, err := json.Marshal(svc.GetSchema())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	groups, ok := decoded["groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("expected decoded.groups to be a 1-element array, got %#v", decoded["groups"])
	}
	group, ok := groups[0].(map[string]any)
	if !ok {
		t.Fatalf("expected group to decode as an object, got %#v", groups[0])
	}
	if label, ok := group["label"].(string); !ok || label != "Appearance" {
		t.Errorf("group.label = %#v, want plain string %q", group["label"], "Appearance")
	}

	fields, ok := group["fields"].([]any)
	if !ok || len(fields) != 1 {
		t.Fatalf("expected group.fields to be a 1-element array, got %#v", group["fields"])
	}
	field, ok := fields[0].(map[string]any)
	if !ok {
		t.Fatalf("expected field to decode as an object, got %#v", fields[0])
	}
	if label, ok := field["label"].(string); !ok || label != "Theme" {
		t.Errorf("field.label = %#v, want plain string %q", field["label"], "Theme")
	}

	options, ok := field["options"].([]any)
	if !ok || len(options) != 1 {
		t.Fatalf("expected field.options to be a 1-element array, got %#v", field["options"])
	}
	option, ok := options[0].(map[string]any)
	if !ok {
		t.Fatalf("expected option to decode as an object, got %#v", options[0])
	}
	if label, ok := option["label"].(string); !ok || label != "System" {
		t.Errorf("option.label = %#v, want plain string %q", option["label"], "System")
	}
}

// TestValidate_MessagesLocalize checks the other half of AD-5's "Validate
// error messages become i18n.Text": both the message template and the
// resolved field label inside it localize when a localizer is supplied,
// and both fall back to the built-in English text when it's nil (already
// covered by every other TestValidate_* in this package, which pass nil).
func TestValidate_MessagesLocalize(t *testing.T) {
	l, err := i18n.New(i18n.WithCatalog(frCatalog()), i18n.WithLocale("fr"))
	if err != nil {
		t.Fatalf("i18n.New: %v", err)
	}

	schema := Schema{Groups: []Group{{
		Key:   "g",
		Label: i18n.T("test.g", "G"),
		Fields: []Field{{
			Key:        "theme",
			Type:       FieldText,
			Label:      i18n.T("test.settings.theme.label", "Theme"),
			Validation: &Validation{Required: true},
		}},
	}}}

	errs := Validate(schema, map[string]any{}, l)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	// "%s est requis" (catalog) + "Thème" (catalog, from frCatalog) resolved
	// together — proves both halves of the message go through l, not just
	// the template.
	if want := "Thème est requis"; errs[0].Message != want {
		t.Errorf("errs[0].Message = %q, want %q", errs[0].Message, want)
	}
}

// TestLocaleGroup exercises settings.LocaleGroup, the WP-12 replacement for
// the pre-WP-12 i18n.Localizer.SettingsGroup (moved here to avoid an
// i18n<->settings import cycle — see locale.go's doc comment).
func TestLocaleGroup(t *testing.T) {
	l, err := i18n.New(i18n.WithCatalog(fstest.MapFS{
		"locales/fr.json": &fstest.MapFile{Data: []byte(`{}`)},
	}))
	if err != nil {
		t.Fatalf("i18n.New: %v", err)
	}

	group := LocaleGroup(l)
	if group.Key != "i18n" {
		t.Errorf("group.Key = %q, want %q", group.Key, "i18n")
	}
	if len(group.Fields) != 1 {
		t.Fatalf("expected exactly one field, got %d", len(group.Fields))
	}
	field := group.Fields[0]
	if field.Key != i18n.SettingLocale {
		t.Errorf("field.Key = %q, want %q", field.Key, i18n.SettingLocale)
	}
	if field.Default != "system" {
		t.Errorf("field.Default = %#v, want %q", field.Default, "system")
	}
	if len(field.Options) < 2 || field.Options[0].Value != "system" {
		t.Fatalf("field.Options = %#v, want \"system\" first plus the fr catalog entry", field.Options)
	}
	var sawFrench bool
	for _, o := range field.Options[1:] {
		if o.Value == "fr" {
			sawFrench = true
		}
	}
	if !sawFrench {
		t.Errorf("expected the fr catalog to appear as a locale option, got %#v", field.Options)
	}

	// The resolved schema must carry plain strings, same as any other group.
	svc := NewService(WithGroup(group), WithLocalizer(l))
	resolved := svc.GetSchema().Groups[0]
	if resolved.Label == "" {
		t.Error("expected a non-empty resolved group label")
	}
}

// TestWireLocale_LiveSwitch pins M6: a locale picked through LocaleGroup's
// field must take effect immediately in the same process, not just on next
// launch. Before WireLocale existed, SetValues persisted the new
// i18n.SettingLocale value like any other field (nothing rejected it — the
// field "worked") but nothing told the already-constructed *i18n.Localizer
// to re-resolve, so l.Locale() silently stayed on the old value until the
// app restarted and i18n.WithSettings' tier-2 resolution picked it up fresh.
func TestWireLocale_LiveSwitch(t *testing.T) {
	l, err := i18n.New(i18n.WithCatalog(frCatalog()), i18n.WithLocale("en"))
	if err != nil {
		t.Fatalf("i18n.New: %v", err)
	}

	svc := NewService(WithGroup(LocaleGroup(l)), WithLocalizer(l))
	WireLocale(svc, l)

	if got := l.Locale(); got != "en" {
		t.Fatalf("Locale() before switch = %q, want %q", got, "en")
	}

	errs, err := svc.SetValues(map[string]any{i18n.SettingLocale: "fr"})
	if err != nil {
		t.Fatalf("SetValues: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected the locale switch to validate, got %v", errs)
	}
	if got := l.Locale(); got != "fr" {
		t.Errorf("Locale() after SetValues(i18n.locale=fr) = %q, want %q (live switch never reached the Localizer)", got, "fr")
	}

	// GetSchema resolves fresh every call (existing invariant) — confirms
	// the wiring is real and something downstream actually observes it, not
	// just an internal field on l that nothing reads.
	schema := svc.GetSchema()
	if got := schema.Groups[0].Fields[0].Label; got == "" {
		t.Errorf("unexpected empty schema label after locale switch")
	}

	// "system" (the field's own default, meaning "defer to lower tiers")
	// must not be forwarded to SetLocale — it isn't a valid BCP-47 tag, and
	// must not error or change the resolved locale away from the explicit
	// choice already made.
	if _, err := svc.SetValues(map[string]any{i18n.SettingLocale: "system"}); err != nil {
		t.Fatalf("SetValues(system): %v", err)
	}
	if got := l.Locale(); got != "fr" {
		t.Errorf("Locale() after SetValues(i18n.locale=system) = %q, want unchanged %q", got, "fr")
	}
}
