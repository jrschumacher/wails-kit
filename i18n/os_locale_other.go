//go:build !darwin

package i18n

// osLocales is the OS-source tier of the resolution order on platforms
// other than darwin. Per OQ-5, on Linux/Windows the env vars (LC_ALL,
// LC_MESSAGES, LANG — checked one tier above this one) already are the OS
// locale source, so there is nothing further to read here. A documented
// follow-up (not this WP) is GetUserDefaultLocaleName on Windows for
// GUI-launched processes with no environment locale set.
var osLocales = func() []string { return nil }
