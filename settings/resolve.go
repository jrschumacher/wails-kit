package settings

import "github.com/jrschumacher/wails-kit/v2/i18n"

// resolveText resolves t through l, falling back to t.Other when l is nil.
// This is the entire "localization is opt-in" contract: a Service built
// without WithLocalizer resolves every i18n.Text to its literal Other value,
// so a consumer that never touches i18n sees no behavior change from
// pre-WP-12 plain-string labels.
func resolveText(t i18n.Text, l *i18n.Localizer) string {
	if l == nil {
		return t.Other
	}
	return l.T(t)
}

func resolveSelectOptions(opts []SelectOption, l *i18n.Localizer) []ResolvedSelectOption {
	if opts == nil {
		return nil
	}
	out := make([]ResolvedSelectOption, len(opts))
	for i, o := range opts {
		out[i] = ResolvedSelectOption{Value: o.Value, Label: resolveText(o.Label, l)}
	}
	return out
}

func resolveDynamicOptions(d *DynamicOptions, l *i18n.Localizer) *ResolvedDynamicOptions {
	if d == nil {
		return nil
	}
	options := make(map[string][]ResolvedSelectOption, len(d.Options))
	for k, v := range d.Options {
		options[k] = resolveSelectOptions(v, l)
	}
	return &ResolvedDynamicOptions{DependsOn: d.DependsOn, Options: options}
}

func resolveField(f Field, l *i18n.Localizer) ResolvedField {
	return ResolvedField{
		Key:            f.Key,
		Type:           f.Type,
		Label:          resolveText(f.Label, l),
		Description:    resolveText(f.Description, l),
		Placeholder:    resolveText(f.Placeholder, l),
		Default:        f.Default,
		Options:        resolveSelectOptions(f.Options, l),
		DynamicOptions: resolveDynamicOptions(f.DynamicOptions, l),
		Condition:      f.Condition,
		Validation:     f.Validation,
		Advanced:       f.Advanced,
	}
}

func resolveGroup(g Group, l *i18n.Localizer) ResolvedGroup {
	fields := make([]ResolvedField, len(g.Fields))
	for i, f := range g.Fields {
		fields[i] = resolveField(f, l)
	}
	return ResolvedGroup{Key: g.Key, Label: resolveText(g.Label, l), Fields: fields}
}

// resolveSchema resolves every group and field in s against l (nil-safe —
// see resolveText). Called fresh on every GetSchema call, not cached, so a
// Localizer.SetLocale that fires i18n:changed is immediately reflected the
// next time a consumer refetches the schema.
func resolveSchema(s Schema, l *i18n.Localizer) ResolvedSchema {
	groups := make([]ResolvedGroup, len(s.Groups))
	for i, g := range s.Groups {
		groups[i] = resolveGroup(g, l)
	}
	return ResolvedSchema{Groups: groups}
}
