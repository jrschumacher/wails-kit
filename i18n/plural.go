package i18n

import (
	"fmt"

	"golang.org/x/text/feature/plural"
)

// TN resolves t as a pluralized message for count n, selecting the CLDR
// plural category for the current locale via golang.org/x/text/feature/plural
// — never hand-rolled (naive "%d items" is wrong in most languages: Polish
// alone has four cardinal categories, Arabic six). If the catalog entry for
// t.Key isn't a plural object, or has no string for the selected category,
// the entry's (or t.Other's) "other" form is used instead — every
// well-formed plural entry has one (catalogEntry.UnmarshalJSON requires it).
//
// n is always the first fmt.Sprintf argument (the count is nearly always
// wanted in the message, e.g. "%d items"); args, if any, follow it.
func (l *Localizer) TN(t Text, n int, args ...any) string {
	format := t.Other

	if entry := l.lookup(t.Key); entry != nil {
		format = entry.Other
		if entry.Forms != nil {
			l.mu.RLock()
			tag := l.locale
			l.mu.RUnlock()

			abs := n
			if abs < 0 {
				abs = -abs
			}
			form := plural.Cardinal.MatchPlural(tag, abs, 0, 0, 0, 0)
			if text, ok := entry.Forms[form]; ok {
				format = text
			}
		}
	}

	all := make([]any, 0, len(args)+1)
	all = append(all, n)
	all = append(all, args...)
	return fmt.Sprintf(format, all...)
}
