package sqlitestore

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jrschumacher/wails-kit/v2/database"
	"github.com/jrschumacher/wails-kit/v2/runner"
	"github.com/jrschumacher/wails-kit/v2/runner/storetest"
)

func TestStoreContract(t *testing.T) {
	storetest.Run(t, func(t *testing.T) runner.Store {
		t.Helper()
		s, err := New(WithPath(filepath.Join(t.TempDir(), "queue.db")))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return s
	})
}

func TestNewRequiresPathOrDatabase(t *testing.T) {
	if _, err := New(); err == nil {
		t.Fatal("New with neither WithPath nor WithDatabase: want error, got nil")
	}
}

func TestNewRejectsBothPathAndDatabase(t *testing.T) {
	dir := t.TempDir()
	db, err := database.New(database.WithPath(filepath.Join(dir, "app.db")))
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	_, err = New(WithPath(filepath.Join(dir, "queue.db")), WithDatabase(db))
	if err == nil {
		t.Fatal("New with both WithPath and WithDatabase: want error, got nil")
	}
}

// TestMigrationCreatesJobsTableAndIndex is the SQLite-specific regression
// test for WP-41's schema requirement: a single goose migration creates the
// jobs table plus an index on (state, next_run_at). This can't be expressed
// by the storetest contract suite (which only knows the Store interface,
// not SQL schema) — it belongs here.
func TestMigrationCreatesJobsTableAndIndex(t *testing.T) {
	dir := t.TempDir()
	s, err := New(WithPath(filepath.Join(dir, "queue.db")))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	var tableName string
	err = s.db.DB().QueryRow(
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'jobs'",
	).Scan(&tableName)
	if err != nil {
		t.Fatalf("jobs table not found: %v", err)
	}

	var indexName string
	err = s.db.DB().QueryRow(
		"SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'idx_jobs_state_next_run_at'",
	).Scan(&indexName)
	if err != nil {
		t.Fatalf("idx_jobs_state_next_run_at index not found: %v", err)
	}
}

// TestReopenIsIdempotent verifies goose migrations don't re-run (and don't
// error) against a database that already has them applied — the ordinary
// "app restarts, opens the same queue.db again" path.
func TestReopenIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.db")

	s1, err := New(WithPath(path))
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	now := time.Now()
	if err := s1.Append(runner.Job{
		ID: "job-1", Type: "noop", Payload: json.RawMessage(`{}`),
		State: runner.JobStatePending, EnqueuedAt: now, NextRunAt: now,
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := New(WithPath(path))
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	defer func() { _ = s2.Close() }()

	due, err := s2.Due(now.Add(time.Second), 10)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 1 || due[0].ID != "job-1" {
		t.Fatalf("Due after reopen = %+v, want [job-1] (data must survive reopen)", due)
	}
}

// TestWithDatabaseSharesExternalDB verifies the WithDatabase path: an
// application's own database.DB (with its own unrelated migration/table)
// can be shared with sqlitestore, sqlitestore's migration lands under its
// own goose version-tracking table without colliding with the app's, and
// Store.Close does not close the shared database.DB.
func TestWithDatabaseSharesExternalDB(t *testing.T) {
	dir := t.TempDir()

	fsys := fstest.MapFS{
		"001_create_notes.sql": &fstest.MapFile{
			Data: []byte("-- +goose Up\nCREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT);\n-- +goose Down\nDROP TABLE notes;\n"),
		},
	}

	db, err := database.New(database.WithPath(filepath.Join(dir, "app.db")), database.WithMigrations(fsys))
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	s, err := New(WithDatabase(db))
	if err != nil {
		t.Fatalf("sqlitestore.New: %v", err)
	}

	// Both the app's own table and sqlitestore's jobs table must exist.
	for _, table := range []string{"notes", "jobs"} {
		var name string
		if err := db.DB().QueryRow(
			"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table,
		).Scan(&name); err != nil {
			t.Fatalf("table %q not found: %v", table, err)
		}
	}

	// Both goose version-tracking tables must exist, independently.
	for _, table := range []string{"goose_db_version", migrationTable} {
		var name string
		if err := db.DB().QueryRow(
			"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table,
		).Scan(&name); err != nil {
			t.Fatalf("version table %q not found: %v", table, err)
		}
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Store.Close: %v", err)
	}
	// The shared database.DB must still be usable — Store.Close must not
	// have closed it.
	if err := db.DB().Ping(); err != nil {
		t.Fatalf("shared database.DB unusable after Store.Close: %v", err)
	}
}

// TestPayloadStoredAsPlainJSON is the SQLite analogue of runner/flatfile's
// golden-file human-readability test. There's no committed golden file
// requirement for SQLite (the format is opaque anyway), but the payload
// column itself must still hold plain, inline JSON — never base64, never a
// re-escaped string of JSON — so `sqlite3 queue.db 'select * from jobs'`
// shows a human what's queued.
func TestPayloadStoredAsPlainJSON(t *testing.T) {
	dir := t.TempDir()
	s, err := New(WithPath(filepath.Join(dir, "queue.db")))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	now := time.Now()
	payload := json.RawMessage(`{"to":"a@b.com","subject":"hi"}`)
	if err := s.Append(runner.Job{
		ID: "job-1", Type: "send_email", Payload: payload,
		State: runner.JobStatePending, EnqueuedAt: now, NextRunAt: now,
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	var raw string
	if err := s.db.DB().QueryRow("SELECT payload FROM jobs WHERE id = ?", "job-1").Scan(&raw); err != nil {
		t.Fatalf("select payload: %v", err)
	}

	if raw != string(payload) {
		t.Fatalf("payload column = %q, want %q (byte-identical, no re-encoding)", raw, string(payload))
	}
	if strings.Contains(raw, "\\u") || !strings.HasPrefix(strings.TrimSpace(raw), "{") {
		t.Fatalf("payload column does not look like inline JSON: %q", raw)
	}
}

// TestBusyTimeoutHeldOnSecondConnection mirrors
// database.TestForeignKeysEveryConnection: SQLite pragmas other than
// journal_mode are per-connection, so a Store whose correctness depends on
// busy_timeout (bounded waits under WAL instead of immediate SQLITE_BUSY)
// must be tested against a connection the pool opens *after* the first is
// pinned busy — not just whichever connection happens to be idle. This
// exercises the guarantee through Store's own database.DB, the same way an
// application would end up using it.
func TestBusyTimeoutHeldOnSecondConnection(t *testing.T) {
	dir := t.TempDir()
	s, err := New(WithPath(filepath.Join(dir, "queue.db")))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	sqlDB := s.db.DB()
	sqlDB.SetMaxOpenConns(2)

	ctx := context.Background()

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	conn2, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	defer func() { _ = conn2.Close() }()

	var busyTimeout int
	if err := conn2.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout on second pooled connection = %d, want 5000", busyTimeout)
	}

	var journalMode string
	if err := conn2.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode on second pooled connection = %q, want %q", journalMode, "wal")
	}
}

// TestConcurrentAppendUnderWAL is a live-fire version of the pragma check
// above: many goroutines Append distinct jobs concurrently. Without
// busy_timeout propagating to every pooled connection (the WP-04 defect),
// this reliably produces "database is locked" (SQLITE_BUSY) errors instead
// of the writers waiting their turn.
func TestConcurrentAppendUnderWAL(t *testing.T) {
	dir := t.TempDir()
	s, err := New(WithPath(filepath.Join(dir, "queue.db")))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	const n = 25
	now := time.Now()

	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			job := runner.Job{
				ID:         fmt.Sprintf("job-%d", i),
				Type:       "noop",
				Payload:    json.RawMessage(`{}`),
				State:      runner.JobStatePending,
				EnqueuedAt: now,
				NextRunAt:  now,
			}
			errCh <- s.Append(job)
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Errorf("concurrent Append: %v", err)
		}
	}

	due, err := s.Due(now.Add(time.Second), 0)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != n {
		t.Fatalf("Due after concurrent Append = %d jobs, want %d", len(due), n)
	}
}

func TestUpdateUnknownJobReturnsPlainError(t *testing.T) {
	dir := t.TempDir()
	s, err := New(WithPath(filepath.Join(dir, "queue.db")))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	err = s.Update(runner.Job{ID: "missing", State: runner.JobStateDone})
	if err == nil {
		t.Fatal("Update on unknown ID: want error, got nil")
	}
}

func TestAppendDuplicateIDReturnsPlainError(t *testing.T) {
	dir := t.TempDir()
	s, err := New(WithPath(filepath.Join(dir, "queue.db")))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	now := time.Now()
	job := runner.Job{ID: "job-1", Type: "noop", Payload: json.RawMessage(`{}`), State: runner.JobStatePending, EnqueuedAt: now, NextRunAt: now}
	if err := s.Append(job); err != nil {
		t.Fatalf("first Append: %v", err)
	}
	if err := s.Append(job); err == nil {
		t.Fatal("second Append with duplicate ID: want error, got nil")
	}
}
