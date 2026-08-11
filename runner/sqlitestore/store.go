// Package sqlitestore is a runner.Store backed by SQLite via the kit's
// database package, so it reuses that package's per-connection pragma
// guarantee (WAL, busy_timeout, foreign_keys baked into the connection DSN
// — see database/database.go and the WP-04 fix it documents) rather than
// re-deriving it. Do not Exec "PRAGMA ..." statements in this package after
// opening a connection of its own; that is exactly the defect WP-04 fixed
// in database.
//
// Ownership of the underlying *sql.DB is a deliberate choice exposed via
// two mutually exclusive options:
//
//   - WithPath: sqlitestore opens and owns a dedicated database.DB (its own
//     SQLite file, its own pragmas, closed by Store.Close). This is the
//     default shape, analogous to runner/flatfile's WithPath.
//   - WithDatabase: the caller supplies an existing *database.DB — one it
//     already opened (via database.New, possibly itself using
//     database.WithDB to wrap a *sql.DB the caller doesn't control the DSN
//     of). Store never closes a supplied database.DB; the caller owns its
//     lifecycle. Whatever pragma guarantees that database.DB carries (full
//     DSN-baked guarantees if database.New opened it directly, or the
//     SetMaxOpenConns(1)-constrained guarantee if the caller used
//     database.WithDB) propagate unchanged to this Store — sqlitestore adds
//     no pragma management of its own.
//
// Schema migrations: this package ships exactly one goose migration
// (migrations/001_create_jobs.sql), creating the jobs table and an index on
// (state, next_run_at). It is applied automatically on every New(), using
// its own goose version-tracking table (see migrationTable) so it never
// collides with an application's own migrations when sharing a database.DB
// via WithDatabase. Future schema changes are additional numbered
// migration files in the same directory, applied incrementally the same
// way database.New handles its own migrations — there is no other
// migration mechanism (no down-migration tooling exposed, no manual DDL
// path) and no schema versioning beyond what goose already tracks.
//
// Human-readability: the jobs table stores Payload as a TEXT column
// containing the job's raw JSON verbatim — never base64, never a
// JSON-encoded string of JSON. `sqlite3 queue.db 'select * from jobs'`
// shows the same payload a human would see in runner/flatfile's JSONL.
package sqlitestore

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/jrschumacher/wails-kit/v2/database"
	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/i18n"
	"github.com/jrschumacher/wails-kit/v2/runner"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var rawMigrationsFS embed.FS

// migrationsFS strips the "migrations/" prefix embed.FS keeps, since goose
// expects migration files at the FS root (same fs.Sub step
// examples/database/main.go uses for the same reason).
var migrationsFS = func() fs.FS {
	sub, err := fs.Sub(rawMigrationsFS, "migrations")
	if err != nil {
		// Unreachable: "migrations" is a directory embed.FS always has,
		// verified at compile time by the go:embed directive above.
		panic("runner/sqlitestore: sub fs: " + err.Error())
	}
	return sub
}()

// migrationTable is this package's own goose version-tracking table name —
// distinct from goose's "goose_db_version" default so that a database.DB
// shared with an application (WithDatabase) never has its migration
// history collide with sqlitestore's.
const migrationTable = "goose_db_version_runner_sqlitestore"

// timeLayout is a fixed-width (30-byte), always-UTC RFC3339Nano variant.
// time.Time's default JSON encoding (RFC3339Nano) trims trailing zeros in
// the fractional part, which breaks lexicographic ordering of the TEXT
// column that the (state, next_run_at) index and the Due/Sweep queries
// depend on — ".5" sorts before ".499999999" as a string despite being
// the later instant. Fixing the width up front keeps next_run_at both
// human-readable (a plain timestamp, not an opaque epoch integer) and
// correctly sortable/comparable as SQLite TEXT.
const timeLayout = "2006-01-02T15:04:05.000000000Z"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(timeLayout, s)
}

// Error codes for sqlitestore-level failures a user may eventually see
// (database open/migrate/query problems). Store-contract programmer errors
// — Append on a duplicate ID, Update on an unknown ID — stay plain errors,
// matching runner/flatfile and runner's built-in memory store: those are
// bugs in the calling code (runner.Queue), never surfaced to an end user.
const (
	ErrStoreOpen    errors.Code = "runner_sqlitestore_open"
	ErrStoreMigrate errors.Code = "runner_sqlitestore_migrate"
	ErrStoreQuery   errors.Code = "runner_sqlitestore_query"
)

func init() {
	errors.RegisterMessages(map[errors.Code]i18n.Text{
		ErrStoreOpen:    i18n.T("wailskit.runner.sqlitestore.errors.open", "Unable to open the background job database. Please check file permissions and try again."),
		ErrStoreMigrate: i18n.T("wailskit.runner.sqlitestore.errors.migrate", "Background job database migration failed. Please contact support."),
		ErrStoreQuery:   i18n.T("wailskit.runner.sqlitestore.errors.query", "A background job database operation failed. Please try again."),
	})
}

// Store is a runner.Store backed by SQLite. Construct with New; the zero
// value is not usable.
type Store struct {
	db    *database.DB
	owned bool // true when New opened db itself (WithPath) and must Close it
}

// Option configures a Store at construction.
type Option func(*config)

type config struct {
	path string
	db   *database.DB
}

// WithPath opens and owns a dedicated database.DB at path, applying this
// package's migrations to it. Mutually exclusive with WithDatabase.
func WithPath(path string) Option {
	return func(c *config) { c.path = path }
}

// WithDatabase supplies an existing *database.DB for the Store to use.
// Store never closes it — the caller retains ownership and lifecycle
// responsibility, exactly like database.WithDB's contract one level up.
// Mutually exclusive with WithPath. See the package doc comment for the
// pragma-inheritance and migration-table-isolation implications.
func WithDatabase(db *database.DB) Option {
	return func(c *config) { c.db = db }
}

// New constructs a Store. Exactly one of WithPath or WithDatabase is
// required.
func New(opts ...Option) (*Store, error) {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}

	switch {
	case cfg.path != "" && cfg.db != nil:
		return nil, errors.New(ErrStoreOpen, "runner/sqlitestore: WithPath and WithDatabase are mutually exclusive", nil)
	case cfg.path == "" && cfg.db == nil:
		return nil, errors.New(ErrStoreOpen, "runner/sqlitestore: WithPath or WithDatabase is required", nil)
	}

	s := &Store{}
	if cfg.db != nil {
		s.db = cfg.db
		s.owned = false
	} else {
		db, err := database.New(database.WithPath(cfg.path))
		if err != nil {
			return nil, errors.Wrap(ErrStoreOpen, "runner/sqlitestore: open database", err)
		}
		s.db = db
		s.owned = true
	}

	if err := s.migrate(); err != nil {
		if s.owned {
			_ = s.db.Close()
		}
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	provider, err := goose.NewProvider(goose.DialectSQLite3, s.db.DB(), migrationsFS, goose.WithTableName(migrationTable))
	if err != nil {
		return errors.Wrap(ErrStoreMigrate, "runner/sqlitestore: create goose provider", err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		return errors.Wrap(ErrStoreMigrate, "runner/sqlitestore: run migrations", err)
	}
	return nil
}

// Append adds a newly enqueued job. It errors if job.ID already exists.
func (s *Store) Append(job runner.Job) error {
	payload := job.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("null")
	}

	_, err := s.db.DB().Exec(
		`INSERT INTO jobs (id, type, payload, state, attempts, idempotency_key, enqueued_at, next_run_at, last_error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Type, string(payload), string(job.State), job.Attempts, job.IdempotencyKey,
		formatTime(job.EnqueuedAt), formatTime(job.NextRunAt), job.LastError,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("runner/sqlitestore: job %q already exists", job.ID)
		}
		return errors.Wrap(ErrStoreQuery, fmt.Sprintf("runner/sqlitestore: insert job %q", job.ID), err)
	}
	return nil
}

// Update persists job's current state. It errors if job.ID is unknown.
func (s *Store) Update(job runner.Job) error {
	payload := job.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("null")
	}

	res, err := s.db.DB().Exec(
		`UPDATE jobs SET type = ?, payload = ?, state = ?, attempts = ?, idempotency_key = ?,
		 enqueued_at = ?, next_run_at = ?, last_error = ? WHERE id = ?`,
		job.Type, string(payload), string(job.State), job.Attempts, job.IdempotencyKey,
		formatTime(job.EnqueuedAt), formatTime(job.NextRunAt), job.LastError, job.ID,
	)
	if err != nil {
		return errors.Wrap(ErrStoreQuery, fmt.Sprintf("runner/sqlitestore: update job %q", job.ID), err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(ErrStoreQuery, fmt.Sprintf("runner/sqlitestore: update job %q: rows affected", job.ID), err)
	}
	if n == 0 {
		return fmt.Errorf("runner/sqlitestore: job %q not found", job.ID)
	}
	return nil
}

// Due returns jobs ready to dispatch — see runner.Store.Due's contract
// (pending-and-due, plus any running job regardless of NextRunAt).
// Ordering is deterministic: next_run_at, then enqueued_at, then id — the
// order runner.go's Store.Due doc comment specifies.
func (s *Store) Due(now time.Time, limit int) ([]runner.Job, error) {
	if limit <= 0 {
		limit = -1 // SQLite: a negative LIMIT means "no limit".
	}

	rows, err := s.db.DB().Query(
		`SELECT id, type, payload, state, attempts, idempotency_key, enqueued_at, next_run_at, last_error
		 FROM jobs
		 WHERE state = ? OR (state = ? AND next_run_at <= ?)
		 ORDER BY next_run_at ASC, enqueued_at ASC, id ASC
		 LIMIT ?`,
		string(runner.JobStateRunning), string(runner.JobStatePending), formatTime(now), limit,
	)
	if err != nil {
		return nil, errors.Wrap(ErrStoreQuery, "runner/sqlitestore: query due jobs", err)
	}
	return scanJobs(rows)
}

// Sweep removes done/failed jobs whose NextRunAt (repurposed as completion
// time for terminal jobs — see runner.Job.NextRunAt) is older than
// now.Add(-retention). Dead-lettered jobs (JobStateDead) are never removed
// here.
func (s *Store) Sweep(retention time.Duration, now time.Time) error {
	cutoff := now.Add(-retention)
	_, err := s.db.DB().Exec(
		`DELETE FROM jobs WHERE state IN (?, ?) AND next_run_at <= ?`,
		string(runner.JobStateDone), string(runner.JobStateFailed), formatTime(cutoff),
	)
	if err != nil {
		return errors.Wrap(ErrStoreQuery, "runner/sqlitestore: sweep", err)
	}
	return nil
}

// List returns every job currently in the given state — implements
// runner.Lister.
func (s *Store) List(state runner.JobState) ([]runner.Job, error) {
	rows, err := s.db.DB().Query(
		`SELECT id, type, payload, state, attempts, idempotency_key, enqueued_at, next_run_at, last_error
		 FROM jobs WHERE state = ? ORDER BY enqueued_at ASC, id ASC`,
		string(state),
	)
	if err != nil {
		return nil, errors.Wrap(ErrStoreQuery, "runner/sqlitestore: list jobs", err)
	}
	return scanJobs(rows)
}

// Close closes the underlying database.DB if Store opened it (WithPath).
// A database.DB supplied via WithDatabase is left open — the caller owns
// its lifecycle.
func (s *Store) Close() error {
	if s.owned {
		return s.db.Close()
	}
	return nil
}

// Delete implements runner.Deleter — permanently removes id's row.
func (s *Store) Delete(id string) error {
	res, err := s.db.DB().Exec(`DELETE FROM jobs WHERE id = ?`, id)
	if err != nil {
		return errors.Wrap(ErrStoreQuery, fmt.Sprintf("runner/sqlitestore: delete job %q", id), err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(ErrStoreQuery, fmt.Sprintf("runner/sqlitestore: delete job %q: rows affected", id), err)
	}
	if n == 0 {
		return fmt.Errorf("runner/sqlitestore: job %q not found", id)
	}
	return nil
}

func scanJobs(rows *sql.Rows) ([]runner.Job, error) {
	defer func() { _ = rows.Close() }()

	var jobs []runner.Job
	for rows.Next() {
		var (
			job                   runner.Job
			state                 string
			payload               string
			enqueuedAt, nextRunAt string
		)
		if err := rows.Scan(&job.ID, &job.Type, &payload, &state, &job.Attempts,
			&job.IdempotencyKey, &enqueuedAt, &nextRunAt, &job.LastError); err != nil {
			return nil, errors.Wrap(ErrStoreQuery, "runner/sqlitestore: scan job row", err)
		}
		job.Payload = json.RawMessage(payload)
		job.State = runner.JobState(state)

		var err error
		if job.EnqueuedAt, err = parseTime(enqueuedAt); err != nil {
			return nil, errors.Wrap(ErrStoreQuery, fmt.Sprintf("runner/sqlitestore: parse enqueued_at for job %q", job.ID), err)
		}
		if job.NextRunAt, err = parseTime(nextRunAt); err != nil {
			return nil, errors.Wrap(ErrStoreQuery, fmt.Sprintf("runner/sqlitestore: parse next_run_at for job %q", job.ID), err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(ErrStoreQuery, "runner/sqlitestore: iterate job rows", err)
	}
	return jobs, nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint
// failure. modernc.org/sqlite doesn't export stable per-constraint error
// codes as simple package-level constants (they live in an internal
// generated bindings package); the driver's error text reliably contains
// SQLite's own constraint-failure message, so that's what's matched here —
// see modernc.org/sqlite's own tests, which assert on this exact substring.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
