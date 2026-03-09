package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewStore_DefaultPath(t *testing.T) {
	s := NewStore("myapp")
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("no config dir available")
	}
	expected := filepath.Join(configDir, "myapp", "settings.json")
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

func TestSave_MergeSemantic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s := NewStore("app", WithPath(path))

	// Save initial values
	if err := s.Save(map[string]any{"a": "1", "b": "2"}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Save partial update — should merge, not overwrite
	if err := s.Save(map[string]any{"b": "updated", "c": "3"}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	values, err := s.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if values["a"] != "1" {
		t.Errorf("expected a=1 to be preserved, got %v", values["a"])
	}
	if values["b"] != "updated" {
		t.Errorf("expected b=updated, got %v", values["b"])
	}
	if values["c"] != "3" {
		t.Errorf("expected c=3, got %v", values["c"])
	}
}

func TestSave_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s := NewStore("app", WithPath(path))

	if err := s.Save(map[string]any{"key": "value"}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Temp file should not remain
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("expected temp file to be cleaned up after atomic write")
	}

	// Actual file should exist
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected settings file to exist: %v", err)
	}
}

func TestLoad_StripsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	// Write a file with an extra key
	data, _ := json.Marshal(map[string]any{"known": "yes", "stale": "garbage"})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	s := NewStore("app", WithPath(path))
	s.SetKnownKeys(map[string]bool{"known": true})

	values, err := s.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if values["known"] != "yes" {
		t.Errorf("expected known=yes, got %v", values["known"])
	}
	if _, ok := values["stale"]; ok {
		t.Error("expected unknown key 'stale' to be stripped")
	}
}

func TestSave_StripsUnknownKeysFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	data, _ := json.Marshal(map[string]any{"known": "yes", "stale": "garbage"})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	s := NewStore("app", WithPath(path))
	s.SetKnownKeys(map[string]bool{"known": true})

	if err := s.Save(map[string]any{"known": "updated"}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	var saved map[string]any
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if saved["known"] != "updated" {
		t.Fatalf("expected known=updated, got %v", saved["known"])
	}
	if _, ok := saved["stale"]; ok {
		t.Fatal("expected stale key to be removed from disk")
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
