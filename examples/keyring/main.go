// Command keyring-example demonstrates keyring.EnvelopeStore: encrypted,
// arbitrarily-large secrets backed by a single wrapping key.
//
// It uses keyring.NewMemoryStore() instead of keyring.NewOSStore() so this
// example never touches the real OS keychain — safe to run repeatedly and
// safe to run in CI. In a real app, swap in keyring.NewOSStore(appName) to
// hold the wrapping key in the platform credential manager.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
	"github.com/jrschumacher/wails-kit/v2/keyring"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// A throwaway config directory, standing in for dirs.Config() in a real
	// app. EnvelopeStore's secrets.json lives here, not in dirs.Data(): on
	// Linux the two differ, and credentials belong with configuration.
	tmp, err := os.MkdirTemp("", "wails-kit-keyring-example-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	dirs := appdirs.New("keyring-example", appdirs.WithConfigDir(tmp))

	// MemoryStore holds the 32-byte wrapping key. This is what makes the
	// example self-contained: nothing here touches the OS keychain.
	wrappingKeyStore := keyring.NewMemoryStore()

	store, err := keyring.NewEnvelopeStore(dirs, wrappingKeyStore)
	if err != nil {
		return fmt.Errorf("new envelope store: %w", err)
	}
	fmt.Println("secrets file:", store.Path())

	// Set writes are durable: temp file, fsync, rename. The wrapping key is
	// generated on this first write and stored in wrappingKeyStore.
	if err := store.Set("llm.anthropic.secret", "sk-ant-example-not-a-real-key"); err != nil {
		return fmt.Errorf("set: %w", err)
	}
	if err := store.Set("service-account.json", `{"type":"service_account","project_id":"demo"}`); err != nil {
		return fmt.Errorf("set: %w", err)
	}

	secret, err := store.Get("llm.anthropic.secret")
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	fmt.Println("retrieved secret:", secret)

	// EnvelopeStore implements the optional KeyLister extension because its
	// entries live in one file it fully controls. keyring.Store itself has
	// no Keys method — most OS keyrings can't enumerate items portably — so
	// callers that need enumeration type-assert, exactly as they would if
	// store were only known through its Store interface.
	var asStore keyring.Store = store
	if lister, ok := asStore.(keyring.KeyLister); ok {
		keys, err := lister.Keys()
		if err != nil {
			return fmt.Errorf("keys: %w", err)
		}
		fmt.Println("stored keys:", keys)
	}

	// Rotate re-encrypts every entry under a fresh wrapping key. Existing
	// values remain readable afterward.
	if err := store.Rotate(); err != nil {
		return fmt.Errorf("rotate: %w", err)
	}
	secretAfterRotate, err := store.Get("llm.anthropic.secret")
	if err != nil {
		return fmt.Errorf("get after rotate: %w", err)
	}
	fmt.Println("secret survives rotation:", secretAfterRotate == secret)

	// Deleting an absent key is not an error.
	if err := store.Delete("llm.anthropic.secret"); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	if _, err := store.Get("llm.anthropic.secret"); errors.Is(err, keyring.ErrNotFound) {
		fmt.Println("deleted key now reports ErrNotFound, as expected")
	}

	return nil
}
