package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewStore_DefaultPath(t *testing.T) {
	s := NewStore("myapp")
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".myapp", "settings.json")
	if s.Path() != expected {
		t.Errorf("expected path %q, got %q", expected, s.Path())
	}
}

func TestNewStore_WithPath(t *testing.T) {
	custom := "/tmp/custom/settings.json"
	s := NewStore("myapp", WithPath(custom))
	if s.Path() != custom {
		t.Errorf("expected path %q, got %q", custom, s.Path())
	}
}

func TestLoad_DefaultsWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent", "settings.json")
	s := NewStore("app", WithPath(path))
	s.SetDefaults(map[string]any{
		"theme": "dark",
		"count": 42,
	})

	values, err := s.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values["theme"] != "dark" {
		t.Errorf("expected theme=dark, got %v", values["theme"])
	}
	if values["count"] != 42 {
		t.Errorf("expected count=42, got %v", values["count"])
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s := NewStore("app", WithPath(path))

	input := map[string]any{
		"name":    "test",
		"enabled": true,
		"count":   float64(7),
	}
	if err := s.Save(input); err != nil {
		t.Fatalf("save error: %v", err)
	}

	values, err := s.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if values["name"] != "test" {
		t.Errorf("expected name=test, got %v", values["name"])
	}
	if values["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", values["enabled"])
	}
	if values["count"] != float64(7) {
		t.Errorf("expected count=7, got %v", values["count"])
	}
}

func TestLoad_SavedValuesOverrideDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s := NewStore("app", WithPath(path))
	s.SetDefaults(map[string]any{
		"theme":    "light",
		"language": "en",
	})

	if err := s.Save(map[string]any{"theme": "dark"}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	values, err := s.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if values["theme"] != "dark" {
		t.Errorf("expected saved theme=dark to override default, got %v", values["theme"])
	}
	if values["language"] != "en" {
		t.Errorf("expected default language=en to remain, got %v", values["language"])
	}
}

func TestSave_DirectoryPermissions(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "newdir")
	path := filepath.Join(subdir, "settings.json")
	s := NewStore("app", WithPath(path))

	if err := s.Save(map[string]any{"key": "value"}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	info, err := os.Stat(subdir)
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("expected directory permissions 0700, got %04o", perm)
	}
}

func TestSave_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s := NewStore("app", WithPath(path))

	if err := s.Save(map[string]any{"key": "value"}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected file permissions 0600, got %04o", perm)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := os.WriteFile(path, []byte("{invalid json"), 0600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	s := NewStore("app", WithPath(path))
	s.SetDefaults(map[string]any{"fallback": "yes"})

	values, err := s.Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}

	var syntaxErr *json.SyntaxError
	if !isJSONSyntaxError(err, &syntaxErr) {
		t.Errorf("expected json.SyntaxError, got %T: %v", err, err)
	}

	if values["fallback"] != "yes" {
		t.Errorf("expected defaults to be returned on error, got %v", values["fallback"])
	}
}

func isJSONSyntaxError(err error, target **json.SyntaxError) bool {
	switch e := err.(type) {
	case *json.SyntaxError:
		*target = e
		return true
	default:
		return false
	}
}
