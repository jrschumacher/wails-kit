// Package settings provides a schema-driven settings framework for Wails
// desktop applications.
//
// The Go backend declares what the settings are — their keys, types, options,
// defaults, validation rules and visibility conditions — and any frontend
// renders a settings page from that declaration. Adding a setting means editing
// a Go struct literal, not editing a Go struct literal and then a form
// component, a TypeScript type and a validation schema to match.
//
// # The schema-driven model
//
// A Schema is a list of Groups, each a list of Fields. A Field has a Key (a
// flat, dotted string such as "llm.provider" — the store is a single flat map),
// a FieldType, and optional Options, Default, Validation and Condition. A
// Condition gates a field's visibility on another field's value, so the
// frontend can show and hide controls without knowing what any of them mean. A
// Group may also carry ComputeFuncs: derived, read-only values recalculated on
// every read and never persisted.
//
// A Service ties a schema to a config file and a secret store:
//
//	svc, err := settings.NewService(
//		settings.WithAppName("my-app"),
//		settings.WithGroup(myGroup()),
//		settings.WithOnChange(func(values map[string]any) { ... }),
//	)
//	if err != nil {
//		return err // unusable environment or a structurally broken schema
//	}
//	if c := svc.Corruption(); c != nil {
//		// tell the user their settings were reset; see "Corrupt config files"
//	}
//
// # Two kinds of validation
//
// ValidateSchema checks the schema the developer wrote and returns an error:
// an uncompilable pattern, a Condition pointing at a field that does not exist,
// a select with no options. None of it is anything an end user can cause, so it
// is fatal at construction — NewService calls it for you.
//
// Validate checks the values the user entered and returns one ValidationError
// per problem, addressed by field key so the frontend can render each one next
// to its input. Service.SetValues runs it before writing anything.
//
// # Frontend bindings
//
// Wails binds every exported method of a bound value, and a Service has
// GetSecret and GetValuesWithSecrets on it. Binding the Service directly would
// therefore publish the user's raw API keys to the webview. Bind Service.Bindings
// instead — a narrow struct exposing exactly GetSchema, GetValues, SetValues and
// Corruption — and keep the Service itself in Go:
//
//	app := application.New(application.Options{
//		Services: []application.Service{
//			application.NewService(svc.Bindings()), // correct
//		},
//	})
//
// # Secrets
//
// Fields of type FieldPassword are never written to the config file. They are
// routed to a SecretStore, which defaults to the OS keyring — Keychain on
// macOS, Credential Manager on Windows, Secret Service on Linux. There is no
// silent fallback to disk when the keyring is unreachable: an app that believes
// its keys are in the Keychain while they sit readable in a backup is a worse
// outcome than a loud failure. WithPlaintextFileSecrets is the explicit,
// named opt-in for environments where the keyring genuinely does not exist.
//
// Real secret values are reachable from Go and never from the frontend:
//
//   - Service.GetValues, the map that crosses the bridge, reports a stored
//     secret as SecretSentinel and omits one that was never set.
//   - SetValues treats SecretSentinel as "leave the stored secret alone", so a
//     settings page can render every field and post the whole form back without
//     clobbering a key the user did not touch. An empty string clears it; any
//     other value replaces it.
//   - Service.GetValuesWithSecrets and Service.GetSecret return the real values.
//     They are Go-only by construction: neither is on Bindings.
//
// # Corrupt config files
//
// A settings.json containing invalid JSON cannot be recovered by retrying, and
// used to make every subsequent save fail — leaving a non-technical user with
// an app that would not remember anything and a file they could not find. The
// file is instead renamed to "<path>.corrupt-<timestamp>" and the service
// continues on defaults, so the app stays usable and saveable.
//
// Because that recovery discards the user's preferences, it is reported rather
// than logged and forgotten: check Service.Corruption after NewService and tell
// the user what happened and where their old file went. Only malformed JSON is
// quarantined; a file that cannot be read at all still surfaces as an error.
//
// # File location
//
// Settings persist to the platform's user configuration directory:
//
//	macOS    ~/Library/Application Support/<appName>/settings.json
//	Windows  %AppData%\<appName>\settings.json
//	Linux    ~/.config/<appName>/settings.json
//
// The directory is created 0700 and the file 0600, and writes go through a
// temp file and an atomic rename so a crash mid-write cannot truncate it.
// WithStorePath overrides the location entirely, which is what tests should
// use to stay off the developer's real config directory.
package settings
