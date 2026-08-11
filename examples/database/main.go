// Command database-example demonstrates database.New: embedded goose
// migrations, the default pragma set, and the migrated event.
//
// It uses database.WithPath into a throwaway temp directory instead of
// database.WithAppName, so this example never touches real OS-standard app
// directories — safe to run repeatedly and safe to run in CI.
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/jrschumacher/wails-kit/v2/database"
	"github.com/jrschumacher/wails-kit/v2/events"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	tmp, err := os.MkdirTemp("", "wails-kit-database-example-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	// //go:embed keeps the "migrations/" directory prefix in the embedded
	// FS; goose expects migration files at the FS root, so strip it with
	// fs.Sub.
	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("sub fs: %w", err)
	}

	// A MemoryEmitter lets this example print every event database emits
	// without wiring up a real frontend event bus.
	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)

	dbPath := filepath.Join(tmp, "app.db")
	db, err := database.New(
		database.WithPath(dbPath),
		database.WithMigrations(migrations),
		database.WithEmitter(emitter),
	)
	if err != nil {
		return fmt.Errorf("new: %w", err)
	}
	defer func() { _ = db.Close() }()

	for _, evt := range mem.Events() {
		fmt.Printf("event: %s %+v\n", evt.Name, evt.Data)
	}

	version, err := db.Version()
	if err != nil {
		return fmt.Errorf("version: %w", err)
	}
	fmt.Println("schema version:", version)

	// The default pragmas (see README "Default pragmas") are baked into the
	// connection string, not Exec'd once against the pool, so they hold on
	// every connection the pool ever opens — not just the first.
	var foreignKeys int
	if err := db.DB().QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("pragma foreign_keys: %w", err)
	}
	fmt.Println("foreign_keys enforced:", foreignKeys == 1)

	if _, err := db.DB().Exec(
		"INSERT INTO notes (body) VALUES (?)", "wails-kit database example",
	); err != nil {
		return fmt.Errorf("insert: %w", err)
	}

	var body string
	if err := db.DB().QueryRow("SELECT body FROM notes WHERE id = 1").Scan(&body); err != nil {
		return fmt.Errorf("select: %w", err)
	}
	fmt.Println("stored note:", body)

	return nil
}
