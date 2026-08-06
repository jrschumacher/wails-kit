package main

import (
	"sync/atomic"
	"time"

	"github.com/jrschumacher/wails-kit/settings"
)

// Settings keys for the update group. Exported as constants so the Go side and
// the schema cannot drift; the frontend gets them from GetSchema.
const (
	KeyAutoCheck      = "update.autoCheck"
	KeyChannel        = "update.channel"
	KeyCurrentVersion = "update.currentVersion"
	KeyLastChecked    = "update.lastChecked"
)

// Update channel values. These are the strings stored in update.channel and
// passed to updater.Config.Channel.
const (
	ChannelStable = "stable"
	ChannelBeta   = "beta"
)

// UpdatePrefs owns the update settings group and the small piece of runtime
// state it displays.
//
// It contributes a settings.Group like any other application code — the kit's
// settings package knows nothing about updates, and this file adds no Wails
// dependency to the kit.
type UpdatePrefs struct {
	version string

	// lastChecked is Unix seconds, or 0 for "never". It is deliberately
	// in-memory: update.lastChecked is a FieldComputed, and computed fields are
	// never persisted (SetValues strips them). The display therefore resets to
	// "Never" on each launch. Persisting it would mean adding a real writable
	// field, which the user would then see as an editable control.
	lastChecked atomic.Int64
}

func NewUpdatePrefs(version string) *UpdatePrefs {
	return &UpdatePrefs{version: version}
}

// MarkChecked records that an update check just happened.
func (p *UpdatePrefs) MarkChecked(t time.Time) {
	p.lastChecked.Store(t.Unix())
}

// Group returns the settings group describing update preferences.
//
// Note what is NOT here: a "Check Now" button. The schema has no button field
// type — every field type is a value the service persists — so the action lives
// in the application's own UI and calls AppService.CheckForUpdates, which drives
// the updater directly. Buttons are behaviour, not settings.
func (p *UpdatePrefs) Group() settings.Group {
	return settings.Group{
		Key:   "update",
		Label: "Updates",
		Fields: []settings.Field{
			{
				Key:         KeyAutoCheck,
				Type:        settings.FieldToggle,
				Label:       "Check for updates automatically",
				Description: "Takes effect after the next restart.",
				Default:     true,
			},
			{
				Key:         KeyChannel,
				Type:        settings.FieldSelect,
				Label:       "Update channel",
				Description: "Beta receives prereleases. Takes effect after the next restart.",
				Default:     ChannelStable,
				Options: []settings.SelectOption{
					{Label: "Stable", Value: ChannelStable},
					{Label: "Beta", Value: ChannelBeta},
				},
			},
			{
				Key:   KeyCurrentVersion,
				Type:  settings.FieldComputed,
				Label: "Current version",
			},
			{
				Key:   KeyLastChecked,
				Type:  settings.FieldComputed,
				Label: "Last checked",
			},
		},
		// A ComputeFunc must live in the group that declares its field, and
		// every FieldComputed must have one — ValidateSchema rejects both
		// mistakes at NewService time.
		ComputeFuncs: map[string]settings.ComputeFunc{
			KeyCurrentVersion: func(map[string]any) any {
				return p.version
			},
			KeyLastChecked: func(map[string]any) any {
				sec := p.lastChecked.Load()
				if sec == 0 {
					return "Never"
				}
				return time.Unix(sec, 0).Format(time.RFC1123)
			},
		},
	}
}

// autoCheckEnabled reads update.autoCheck out of a settings values map.
//
// The default is true, so an absent or wrongly-typed value means "on". Values
// arriving from the frontend cross a JSON bridge, so a bool is a bool but a
// number would be a float64 — read defensively rather than asserting.
func autoCheckEnabled(values map[string]any) bool {
	v, ok := values[KeyAutoCheck].(bool)
	if !ok {
		return true
	}
	return v
}

// channelOf reads update.channel, falling back to stable.
func channelOf(values map[string]any) string {
	switch v, _ := values[KeyChannel].(string); v {
	case ChannelBeta:
		return ChannelBeta
	default:
		return ChannelStable
	}
}
