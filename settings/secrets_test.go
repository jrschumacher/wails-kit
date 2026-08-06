package settings

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// secretStoreContract exercises the behaviour every SecretStore must provide.
// The error contract is the load-bearing part: callers distinguish "the user
// has not set this key" from "the backend is broken", and those must never be
// conflated.
func secretStoreContract(t *testing.T, newStore func(t *testing.T) SecretStore) {
	t.Run("get on an unset key reports not found", func(t *testing.T) {
		s := newStore(t)
		v, err := s.Get("absent")
		if !errors.Is(err, ErrSecretNotFound) {
			t.Fatalf("expected ErrSecretNotFound, got %v", err)
		}
		if v != "" {
			t.Errorf("expected an empty value alongside the error, got %q", v)
		}
	})

	t.Run("set then get round-trips", func(t *testing.T) {
		s := newStore(t)
		if err := s.Set("k", "sk-ant-secret"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		v, err := s.Get("k")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if v != "sk-ant-secret" {
			t.Errorf("got %q, want %q", v, "sk-ant-secret")
		}
	})

	t.Run("set overwrites", func(t *testing.T) {
		s := newStore(t)
		if err := s.Set("k", "first"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if err := s.Set("k", "second"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		v, _ := s.Get("k")
		if v != "second" {
			t.Errorf("got %q, want %q", v, "second")
		}
	})

	t.Run("delete removes the value", func(t *testing.T) {
		s := newStore(t)
		if err := s.Set("k", "v"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if err := s.Delete("k"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := s.Get("k"); !errors.Is(err, ErrSecretNotFound) {
			t.Fatalf("expected ErrSecretNotFound after delete, got %v", err)
		}
	})

	t.Run("delete of an absent key succeeds", func(t *testing.T) {
		s := newStore(t)
		if err := s.Delete("never-set"); err != nil {
			t.Errorf("deleting an absent key must be a no-op, got %v", err)
		}
	})

	t.Run("keys are independent", func(t *testing.T) {
		s := newStore(t)
		if err := s.Set("a", "1"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if err := s.Set("b", "2"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if err := s.Delete("a"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if v, err := s.Get("b"); err != nil || v != "2" {
			t.Errorf("deleting one key disturbed another: %q, %v", v, err)
		}
	})

	t.Run("empty value round-trips as a stored value", func(t *testing.T) {
		// Distinct from "not set": a caller that explicitly stores "" gets ""
		// back, not ErrSecretNotFound. Service-level code maps an empty
		// submission to Delete rather than relying on this.
		s := newStore(t)
		if err := s.Set("k", ""); err != nil {
			t.Fatalf("Set: %v", err)
		}
		v, err := s.Get("k")
		if err != nil {
			t.Fatalf("expected a stored empty value, got %v", err)
		}
		if v != "" {
			t.Errorf("got %q, want empty", v)
		}
	})
}

func TestMemorySecretStore_Contract(t *testing.T) {
	secretStoreContract(t, func(t *testing.T) SecretStore {
		return NewMemorySecretStore()
	})
}

func TestFileSecretStore_Contract(t *testing.T) {
	secretStoreContract(t, func(t *testing.T) SecretStore {
		return NewPlaintextFileSecretStore(filepath.Join(t.TempDir(), "secrets.json"))
	})
}

func TestFileSecretStore_PersistsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")

	first := NewPlaintextFileSecretStore(path)
	if err := first.Set("k", "v"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	second := NewPlaintextFileSecretStore(path)
	v, err := second.Get("k")
	if err != nil {
		t.Fatalf("Get from a fresh instance: %v", err)
	}
	if v != "v" {
		t.Errorf("got %q, want %q", v, "v")
	}
}

func TestFileSecretStore_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "secrets.json")

	s := NewPlaintextFileSecretStore(path)
	if err := s.Set("k", "v"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("secret file must be 0600, got %04o", perm)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0700 {
		t.Errorf("secret dir must be 0700, got %04o", perm)
	}
}

func TestFileSecretStore_CorruptFileIsReportedAsUnavailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	s := NewPlaintextFileSecretStore(path)
	_, err := s.Get("k")
	if errors.Is(err, ErrSecretNotFound) {
		t.Fatal("a corrupt secret file must not be reported as a missing key: the caller would silently re-prompt and overwrite")
	}
	if !errors.Is(err, ErrSecretStoreUnavailable) {
		t.Fatalf("expected ErrSecretStoreUnavailable, got %v", err)
	}
}

func TestFileSecretStore_ConcurrentAccessIsSafe(t *testing.T) {
	s := NewPlaintextFileSecretStore(filepath.Join(t.TempDir(), "secrets.json"))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := string(rune('a' + i))
			if err := s.Set(key, "v"); err != nil {
				t.Errorf("Set: %v", err)
				return
			}
			if _, err := s.Get(key); err != nil {
				t.Errorf("Get: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

// The keyring store must not be exercised against the developer's real
// Keychain, so this only pins the error mapping surface.
func TestKeyringSecretStore_ImplementsSecretStore(t *testing.T) {
	var _ SecretStore = NewKeyringSecretStore("wails-kit-test")
}

func TestErrSecretNotFound_IsDistinctFromUnavailable(t *testing.T) {
	if errors.Is(ErrSecretNotFound, ErrSecretStoreUnavailable) {
		t.Error("a missing key must not satisfy errors.Is(err, ErrSecretStoreUnavailable)")
	}
	if errors.Is(ErrSecretStoreUnavailable, ErrSecretNotFound) {
		t.Error("an unavailable backend must not satisfy errors.Is(err, ErrSecretNotFound)")
	}
}

// failingSecretStore stands in for a keyring that cannot be reached at all:
// headless Linux with no D-Bus, a locked login keychain, a sandboxed container.
type failingSecretStore struct{ err error }

func (f *failingSecretStore) Get(string) (string, error) { return "", f.err }
func (f *failingSecretStore) Set(string, string) error   { return f.err }
func (f *failingSecretStore) Delete(string) error        { return f.err }
