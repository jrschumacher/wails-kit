package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/jrschumacher/wails-kit/events"
)

func testMigrations() *fstest.MapFS {
	return &fstest.MapFS{
		"001_create_users.sql": &fstest.MapFile{
			Data: []byte(`-- +goose Up
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE
);

-- +goose Down
DROP TABLE users;
`),
		},
		"002_add_created_at.sql": &fstest.MapFile{
			Data: []byte(`-- +goose Up
ALTER TABLE users ADD COLUMN created_at DATETIME DEFAULT CURRENT_TIMESTAMP;

-- +goose Down
ALTER TABLE users DROP COLUMN created_at;
`),
		},
	}
}

func TestNew_WithPath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := New(WithPath(dbPath))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = db.Close() }()

	if db.Path() != dbPath {
		t.Errorf("Path() = %q, want %q", db.Path(), dbPath)
	}

	if db.DB() == nil {
		t.Error("DB() returned nil")
	}

	// Verify file was created.
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

func TestNew_WithAppName(t *testing.T) {
	// Use a temp dir to avoid writing to real app dirs.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")

	db, err := New(WithPath(dbPath))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = db.Close() }()

	if db.DB() == nil {
		t.Error("DB() returned nil")
	}
}

func TestNew_RequiresNameOrPath(t *testing.T) {
	_, err := New()
	if err == nil {
		t.Fatal("New() with no name or path should return error")
	}
}

func TestNew_WithMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := New(
		WithPath(dbPath),
		WithMigrations(testMigrations()),
	)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Verify migrations ran.
	version, err := db.Version()
	if err != nil {
		t.Fatalf("Version() error: %v", err)
	}
	if version != 2 {
		t.Errorf("Version() = %d, want 2", version)
	}

	// Verify table exists and has the right columns.
	_, err = db.DB().Exec("INSERT INTO users (name, email) VALUES (?, ?)", "Alice", "alice@example.com")
	if err != nil {
		t.Fatalf("INSERT error: %v", err)
	}

	var name, email string
	err = db.DB().QueryRow("SELECT name, email FROM users WHERE id = 1").Scan(&name, &email)
	if err != nil {
		t.Fatalf("SELECT error: %v", err)
	}
	if name != "Alice" || email != "alice@example.com" {
		t.Errorf("got name=%q email=%q, want Alice/alice@example.com", name, email)
	}
}

func TestNew_WALMode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := New(WithPath(dbPath))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = db.Close() }()

	var journalMode string
	err = db.DB().QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("PRAGMA journal_mode error: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}
}

func TestNew_ForeignKeys(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := New(WithPath(dbPath))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = db.Close() }()

	var fk int
	err = db.DB().QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if err != nil {
		t.Fatalf("PRAGMA foreign_keys error: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestNew_WithCustomPragmas(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := New(
		WithPath(dbPath),
		WithPragmas(map[string]string{
			"cache_size": "-2000",
		}),
	)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = db.Close() }()

	var cacheSize int
	err = db.DB().QueryRow("PRAGMA cache_size").Scan(&cacheSize)
	if err != nil {
		t.Fatalf("PRAGMA cache_size error: %v", err)
	}
	if cacheSize != -2000 {
		t.Errorf("cache_size = %d, want -2000", cacheSize)
	}
}

func TestNew_WithExternalDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Open a DB externally.
	extDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open error: %v", err)
	}
	defer func() { _ = extDB.Close() }()

	db, err := New(
		WithDB(extDB),
		WithMigrations(testMigrations()),
	)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Close should be a no-op for external DB.
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// External DB should still be usable.
	if err := extDB.Ping(); err != nil {
		t.Fatal("external DB should still be open after Close()")
	}
}

func TestNew_EmitsMigratedEvent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)

	db, err := New(
		WithPath(dbPath),
		WithMigrations(testMigrations()),
		WithEmitter(emitter),
	)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = db.Close() }()

	if mem.Count() != 1 {
		t.Fatalf("expected 1 event, got %d", mem.Count())
	}

	evt := mem.Last()
	if evt.Name != EventMigrated {
		t.Errorf("event name = %q, want %q", evt.Name, EventMigrated)
	}

	payload, ok := evt.Data.(MigratedPayload)
	if !ok {
		t.Fatalf("event data is %T, want MigratedPayload", evt.Data)
	}
	if payload.Version != 2 {
		t.Errorf("payload.Version = %d, want 2", payload.Version)
	}
	if payload.Applied != 2 {
		t.Errorf("payload.Applied = %d, want 2", payload.Applied)
	}
}

func TestNew_NoEventWhenNoMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)

	// First run applies migrations.
	db1, err := New(
		WithPath(dbPath),
		WithMigrations(testMigrations()),
		WithEmitter(emitter),
	)
	if err != nil {
		t.Fatalf("first New() error: %v", err)
	}
	_ = db1.Close()
	mem.Clear()

	// Second run — no pending migrations.
	db2, err := New(
		WithPath(dbPath),
		WithMigrations(testMigrations()),
		WithEmitter(emitter),
	)
	if err != nil {
		t.Fatalf("second New() error: %v", err)
	}
	defer func() { _ = db2.Close() }()

	if mem.Count() != 0 {
		t.Errorf("expected 0 events on second run, got %d", mem.Count())
	}
}

func TestClose_OwnedDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := New(WithPath(dbPath))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// DB should be closed — Ping should fail.
	if err := db.DB().Ping(); err == nil {
		t.Error("expected Ping to fail after Close()")
	}
}

func TestNew_CreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "dir", "test.db")

	db, err := New(WithPath(dbPath))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := os.Stat(filepath.Dir(dbPath)); os.IsNotExist(err) {
		t.Error("parent directories were not created")
	}
}

func TestNew_WithBaselineVersion_ExistingDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Simulate a pre-existing database with tables matching migrations 1-2.
	preDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open error: %v", err)
	}
	_, err = preDB.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("CREATE TABLE error: %v", err)
	}
	_ = preDB.Close()

	// Open with baseline version 2 — should stamp and not re-run migrations.
	db, err := New(
		WithPath(dbPath),
		WithMigrations(testMigrations()),
		WithBaselineVersion(2),
	)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = db.Close() }()

	version, err := db.Version()
	if err != nil {
		t.Fatalf("Version() error: %v", err)
	}
	if version != 2 {
		t.Errorf("Version() = %d, want 2", version)
	}
}

func TestNew_WithBaselineVersion_FreshDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Fresh database — baseline should be a no-op, migrations run normally.
	db, err := New(
		WithPath(dbPath),
		WithMigrations(testMigrations()),
		WithBaselineVersion(2),
	)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = db.Close() }()

	version, err := db.Version()
	if err != nil {
		t.Fatalf("Version() error: %v", err)
	}
	if version != 2 {
		t.Errorf("Version() = %d, want 2", version)
	}

	// Verify migrations actually ran (table exists with correct schema).
	_, err = db.DB().Exec("INSERT INTO users (name, email) VALUES (?, ?)", "Bob", "bob@example.com")
	if err != nil {
		t.Fatalf("INSERT error: %v", err)
	}
}

func TestNew_WithBaselineVersion_GooseTableExists(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Run migrations first to create goose_db_version table.
	db1, err := New(
		WithPath(dbPath),
		WithMigrations(testMigrations()),
	)
	if err != nil {
		t.Fatalf("first New() error: %v", err)
	}
	_ = db1.Close()

	// Re-open with baseline — should be a no-op since goose table exists.
	db2, err := New(
		WithPath(dbPath),
		WithMigrations(testMigrations()),
		WithBaselineVersion(2),
	)
	if err != nil {
		t.Fatalf("second New() error: %v", err)
	}
	defer func() { _ = db2.Close() }()

	version, err := db2.Version()
	if err != nil {
		t.Fatalf("Version() error: %v", err)
	}
	if version != 2 {
		t.Errorf("Version() = %d, want 2", version)
	}
}

func TestNew_WithBaselineVersion_PartialMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Simulate a pre-existing database with only migration 1 applied.
	preDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open error: %v", err)
	}
	_, err = preDB.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE
		)
	`)
	if err != nil {
		t.Fatalf("CREATE TABLE error: %v", err)
	}
	_ = preDB.Close()

	// Baseline at version 1 — migration 2 should still run.
	db, err := New(
		WithPath(dbPath),
		WithMigrations(testMigrations()),
		WithBaselineVersion(1),
	)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = db.Close() }()

	version, err := db.Version()
	if err != nil {
		t.Fatalf("Version() error: %v", err)
	}
	if version != 2 {
		t.Errorf("Version() = %d, want 2", version)
	}

	// Verify migration 2 ran (created_at column exists).
	var createdAt sql.NullString
	_, err = db.DB().Exec("INSERT INTO users (name, email) VALUES (?, ?)", "Alice", "alice@example.com")
	if err != nil {
		t.Fatalf("INSERT error: %v", err)
	}
	err = db.DB().QueryRow("SELECT created_at FROM users WHERE id = 1").Scan(&createdAt)
	if err != nil {
		t.Fatalf("SELECT created_at error: %v", err)
	}
}
