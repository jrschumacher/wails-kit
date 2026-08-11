package permissions

import "github.com/jrschumacher/wails-kit/v2/i18n"

// Prompt copy — the "why we're asking" and "you said no, here's the fix"
// strings a permission dialog shows — ships as i18n.Text constants so
// wording is consistent across apps built on the kit and so it flows
// through the same catalog machinery as everything else (see AD-5).
// Service.Text resolves these against a configured WithLocalizer; a
// webview frontend instead reads the equivalent keys out of the app's
// merged i18n catalog (GetCatalog) directly, keyed exactly as shown below.
//
// These are deliberately data, not a method on Service that takes a Kind
// and returns "the" copy — an app decides its own layout and may want
// only the rationale, only the denied guidance, or neither (some apps
// build entirely custom copy and only need the Status).
var (
	// RationaleNotifications is shown before Request(ctx, Notifications)
	// — the "why we're asking" copy for StatusNotDetermined.
	RationaleNotifications = i18n.T("wailskit.permissions.notifications.rationale",
		"Allow notifications to get alerted about important updates.")
	// DeniedNotifications is shown for StatusDenied — direct the user to
	// OpenSystemSettings(Notifications); the app cannot re-prompt.
	DeniedNotifications = i18n.T("wailskit.permissions.notifications.denied",
		"Notifications are turned off. Enable them in System Settings to get alerted about important updates.")

	// RationaleAccessibility is shown before Request(ctx, Accessibility).
	RationaleAccessibility = i18n.T("wailskit.permissions.accessibility.rationale",
		"Accessibility access lets this app control other applications on your behalf.")
	// DeniedAccessibility is shown for StatusDenied. Accessibility can
	// never be granted programmatically — this copy should always pair
	// with a button that calls OpenSystemSettings(Accessibility).
	DeniedAccessibility = i18n.T("wailskit.permissions.accessibility.denied",
		"Accessibility access is off. Turn it on in System Settings, then relaunch the app.")

	// RationaleFullDiskAccess is shown before directing the user to
	// System Settings for FullDiskAccess (there is no OS prompt to show
	// first — Request just opens System Settings).
	RationaleFullDiskAccess = i18n.T("wailskit.permissions.full_disk_access.rationale",
		"Full Disk Access lets this app read files outside its sandbox, such as other apps' data.")
	// DeniedFullDiskAccess is shown for StatusDenied.
	DeniedFullDiskAccess = i18n.T("wailskit.permissions.full_disk_access.denied",
		"Full Disk Access is off. Turn it on in System Settings, then relaunch the app.")

	// Unsupported is generic copy for StatusUnsupported — the Kind isn't
	// a concept on this platform. Apps typically just hide the feature
	// instead of showing this, but it's provided for apps that prefer an
	// explicit "not available here" message over silence.
	Unsupported = i18n.T("wailskit.permissions.unsupported",
		"This permission isn't available on your operating system.")
)
