package i18n

// Binding is the frontend-safe surface of Localizer, following the same
// split settings.Service uses (settings/binding.go): Wails v3 binds every
// exported method of a registered service, so register only Binding, never
// *Localizer directly, once Localizer grows backend-only methods.
// Currently every Localizer method here is already safe to expose, but the
// split is the load-bearing convention other kit packages (and wailsbridge)
// rely on, so it starts here rather than being bolted on later.
type Binding struct {
	l *Localizer
}

// Binding returns the frontend-safe binding surface for l. Register the
// returned value with Wails (application.NewService(l.Binding())).
func (l *Localizer) Binding() *Binding {
	return &Binding{l: l}
}

// GetCatalog returns the merged, locale-resolved catalog. See Localizer.Catalog.
func (b *Binding) GetCatalog() map[string]any {
	return b.l.Catalog()
}

// GetLocale returns the current locale. See Localizer.Locale.
func (b *Binding) GetLocale() string {
	return b.l.Locale()
}

// SetLocale changes the current locale. See Localizer.SetLocale.
func (b *Binding) SetLocale(tag string) error {
	return b.l.SetLocale(tag)
}
