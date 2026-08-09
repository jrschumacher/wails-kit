//go:build !darwin

package appearance

// newDefaultSource is nil on every platform other than darwin: there is no
// no-Wails OS theme signal available here (see i18n/os_locale_other.go for
// the same story with locale detection). NewService's "nil source -> light"
// resolution rule applies (Source's doc comment). A live source for these
// platforms is kit/wailsbridge's job (WP-31), wired explicitly via
// WithSource.
func newDefaultSource() Source { return nil }
