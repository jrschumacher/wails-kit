package settings

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jrschumacher/wails-kit/v2/i18n"
)

// TestBindingSurface is the actual guard for defect #1: GetSecret must
// never be reachable through the type registered with Wails. Wails v3
// binds every exported method of a registered service, so the Binding
// type's exported method set IS the frontend-callable surface. This test
// pins that surface to exactly {GetSchema, GetValues, SetValues} via
// reflection, so a future change that adds a method to Binding (or widens
// it to embed *Service) fails loudly here instead of quietly shipping a
// secret leak.
func TestBindingSurface(t *testing.T) {
	svc := NewService()
	b := svc.Binding()

	bindingType := reflect.TypeOf(b)

	want := map[string]bool{
		"GetSchema": true,
		"GetValues": true,
		"SetValues": true,
	}

	got := make(map[string]bool, bindingType.NumMethod())
	for i := 0; i < bindingType.NumMethod(); i++ {
		got[bindingType.Method(i).Name] = true
	}

	for name := range got {
		if !want[name] {
			t.Errorf("Binding exposes unexpected exported method %q — Wails binds every exported method of a registered service, so this becomes reachable from the webview", name)
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("Binding is missing expected method %q", name)
		}
	}

	if _, ok := bindingType.MethodByName("GetSecret"); ok {
		t.Fatal("Binding must not expose GetSecret — registering it with Wails would publish raw API keys to the frontend")
	}
	if _, ok := bindingType.MethodByName("GetValuesWithSecrets"); ok {
		t.Fatal("Binding must not expose a with-secrets accessor")
	}
}

// TestBinding_DelegatesToService is a sanity check that Binding isn't just
// structurally narrow but actually functions as a pass-through to the
// underlying Service, so wiring it up doesn't lose behavior.
func TestBinding_DelegatesToService(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(
		WithStoragePath(filepath.Join(dir, "settings.json")),
		WithGroup(Group{
			Key:   "g",
			Label: i18n.Text{Other: "G"},
			Fields: []Field{
				{Key: "name", Type: FieldText, Label: i18n.Text{Other: "Name"}, Default: "x"},
			},
		}),
	)
	b := svc.Binding()

	if len(b.GetSchema().Groups) != 1 {
		t.Fatalf("expected Binding.GetSchema to mirror Service.GetSchema")
	}

	values, err := b.GetValues()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values["name"] != "x" {
		t.Errorf("expected name=x via Binding.GetValues, got %v", values["name"])
	}

	if errs, err := b.SetValues(map[string]any{"name": "y"}); err != nil || errs != nil {
		t.Fatalf("unexpected SetValues result: errs=%v err=%v", errs, err)
	}
	values, _ = svc.GetValues()
	if values["name"] != "y" {
		t.Errorf("expected Binding.SetValues to persist through to Service, got %v", values["name"])
	}
}
