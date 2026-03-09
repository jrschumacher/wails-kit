package shortcuts

import (
	"testing"

	"github.com/jrschumacher/wails-kit/events"
)

func TestNew(t *testing.T) {
	m := New()
	if m.appMenu || m.editMenu || m.viewMenu || m.windowMenu || m.fileMenu || m.settings {
		t.Fatal("expected all options to be false by default")
	}
}

func TestWithDefaults(t *testing.T) {
	m := New(WithDefaults())
	if !m.appMenu {
		t.Error("expected appMenu to be true")
	}
	if !m.fileMenu {
		t.Error("expected fileMenu to be true")
	}
	if !m.editMenu {
		t.Error("expected editMenu to be true")
	}
	if !m.viewMenu {
		t.Error("expected viewMenu to be true")
	}
	if !m.windowMenu {
		t.Error("expected windowMenu to be true")
	}
}

func TestWithSettings(t *testing.T) {
	m := New(WithSettings())
	if !m.settings {
		t.Error("expected settings to be true")
	}
}

func TestWithEmitter(t *testing.T) {
	mem := events.NewMemoryEmitter()
	e := events.NewEmitter(mem)
	m := New(WithEmitter(e))
	if m.emitter == nil {
		t.Fatal("expected emitter to be set")
	}
}

func TestEmitWithNilEmitter(t *testing.T) {
	m := New()
	// Should not panic.
	m.emit(EventSettingsOpen, nil)
}

func TestEmitWithEmitter(t *testing.T) {
	mem := events.NewMemoryEmitter()
	e := events.NewEmitter(mem)
	m := New(WithEmitter(e))

	m.emit(EventSettingsOpen, nil)

	if mem.Count() != 1 {
		t.Fatalf("expected 1 event, got %d", mem.Count())
	}
	last := mem.Last()
	if last.Name != EventSettingsOpen {
		t.Errorf("expected event name %q, got %q", EventSettingsOpen, last.Name)
	}
}

func TestIndividualOptions(t *testing.T) {
	tests := []struct {
		name string
		opt  Option
		check func(*Manager) bool
	}{
		{"AppMenu", WithAppMenu(), func(m *Manager) bool { return m.appMenu }},
		{"FileMenu", WithFileMenu(), func(m *Manager) bool { return m.fileMenu }},
		{"EditMenu", WithEditMenu(), func(m *Manager) bool { return m.editMenu }},
		{"ViewMenu", WithViewMenu(), func(m *Manager) bool { return m.viewMenu }},
		{"WindowMenu", WithWindowMenu(), func(m *Manager) bool { return m.windowMenu }},
		{"Settings", WithSettings(), func(m *Manager) bool { return m.settings }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(tt.opt)
			if !tt.check(m) {
				t.Errorf("expected %s to be enabled", tt.name)
			}
		})
	}
}
