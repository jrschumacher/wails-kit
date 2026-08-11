# runner/sqlitestore — agent notes

## Purpose

`sqlitestore` is a `runner.Store` backed by SQLite via the kit's `database`
package. It owns the `jobs` table schema/migration and SQL persistence. It
does not own job lifecycle/retry/dead-letter policy (`runner.Queue`), and it
does not own SQLite connection/pragma management — that's `database`'s job,
reused, not reimplemented.

## Public API (load-bearing signatures)

```go
type Store struct{ /* unexported */ }
func New(opts ...Option) (*Store, error) // exactly one of WithPath/WithDatabase required
func WithPath(path string) Option        // Store opens+owns a dedicated database.DB
func WithDatabase(db *database.DB) Option // Store uses a caller-owned database.DB; never closes it

func (s *Store) Append(job runner.Job) error // errors if job.ID already exists
func (s *Store) Update(job runner.Job) error // errors if job.ID is unknown
func (s *Store) Due(now time.Time, limit int) ([]runner.Job, error)
func (s *Store) Sweep(retention time.Duration, now time.Time) error
func (s *Store) List(state runner.JobState) ([]runner.Job, error) // implements runner.Lister
func (s *Store) Close() error // closes the DB only if Store opened it (WithPath)
```

## Invariants (do not break)

- **Never `Exec("PRAGMA ...")` after opening a connection.** That is
  precisely the WP-04 defect `database/database.go` fixed (SQLite pragmas
  other than `journal_mode` are per-*connection*; `Exec` on a `*sql.DB` pool
  only reliably reaches whichever connection is idle). This package must
  never open a `*sql.DB` of its own — every path goes through
  `database.DB`, which owns pragma correctness. If a future change needs a
  new pragma, it belongs in `database`'s default set or `WithPragmas`, not
  here.
- **`migrationTable` (`goose_db_version_runner_sqlitestore`) must stay
  distinct from goose's default `goose_db_version`.** This is what makes
  `WithDatabase` safe to share with an application's own migrations — two
  independent goose histories on one `*sql.DB`, tracked in separate tables.
  Changing this to the default table name would silently corrupt version
  tracking for any `WithDatabase` caller with its own migrations.
- **`Store.Close` closes the underlying `database.DB` only when `owned` is
  true (the `WithPath` path).** A `WithDatabase`-supplied `database.DB` must
  outlive `Store.Close` — the caller owns it. See
  `TestWithDatabaseSharesExternalDB`.
- **Timestamps are stored as fixed-width UTC RFC3339-nanosecond `TEXT`**
  (`timeLayout`), not `time.Time`'s default JSON encoding. The default
  encoding trims trailing fractional zeros, which breaks lexicographic
  ordering — and therefore the `(state, next_run_at)` index and `Due`'s
  `next_run_at <= ?` — for the exact same instant expressed with a shorter
  string. Don't switch to `time.Time.MarshalJSON`/`RFC3339Nano` without
  re-deriving this.
- **`Due` never adds a Go-level lock around its SQL.** Concurrency safety
  comes from SQLite (WAL + `busy_timeout`, inherited from `database.DB`),
  not a `sync.Mutex`. Don't add one "for safety" — it would serialize what
  SQLite is already safely serializing, and if it's ever needed for some
  other reason, events must never be emitted while it's held (see
  `runner/AGENTS.md`'s invariant — the same deadlock class has hit four kit
  packages). `Store` itself emits no events today; if that changes, this is
  the landmine to remember.
- **Payload is stored as raw `TEXT`, never re-encoded.** `job.Payload` is
  already `json.RawMessage`; it's bound to the `payload` column as-is
  (`string(payload)`), not base64, not double-marshaled. See
  `TestPayloadStoredAsPlainJSON`.

## Dependencies & insulation

- `database` (kit package) — the only source of a `*sql.DB` this package
  ever touches. `runner` — `Job`, `JobState`, `Store`, `Lister`.
  `errors`/`i18n` — `RegisterMessages` in `init()`, `wailskit.runner.sqlitestore.*`
  keys in `locales/en.json`.
- No `wails/v3` import — not on the AD-4 allowlist.
- One direction only: `runner` must never import `sqlitestore` back;
  `sqlitestore` must never import `runner/flatfile` (siblings, not
  dependents of each other).

## Extension points

- Schema changes: add a new numbered file to `migrations/`, applied
  incrementally by goose on the next `New` — same mechanism `database`
  itself uses for `WithMigrations`. There is currently no down-migration
  path exposed and no manual DDL escape hatch; don't invent one ad hoc.
- Don't add flatfile-specific or sqlitestore-specific assertions to
  `runner/storetest` — it's the shared contract every `Store` runs
  verbatim. SQLite-only behavior (schema, migration-table isolation,
  connection/pragma propagation, payload column shape) belongs in this
  package's own `store_test.go`, matching how `runner/flatfile` keeps its
  golden-file/compaction tests local.

## Testing

- `runner/storetest.Run` — the shared `Store` contract suite
  (`TestStoreContract`).
- `TestBusyTimeoutHeldOnSecondConnection` — mirrors
  `database.TestForeignKeysEveryConnection`: pins one connection in an open
  transaction, forces a second from the pool, asserts pragmas hold there
  too. Don't "simplify" this back to checking pragmas on the first/only
  connection — that's exactly how the original WP-04 defect shipped
  unnoticed.
- `TestConcurrentAppendUnderWAL` — live concurrent writers; must not
  observe `SQLITE_BUSY`/"database is locked".
- `TestWithDatabaseSharesExternalDB` — migration-table isolation +
  `Close` not closing a shared `database.DB`.
- `go test -race ./runner/sqlitestore/` must cover all of the above.

## File map

- `store.go` — `Store`, `Option`s, `New`/`migrate`, `Append`/`Update`/
  `Due`/`Sweep`/`List`/`Close`, `scanJobs`, time-format helpers, error
  codes + `init()`.
- `migrations/001_create_jobs.sql` — the one shipped schema migration.
- `locales/en.json` — this package's i18n catalog.

## Landmines

- **`New` on `WithPath` leaves a partially-created database file behind on
  a failed migration** (it calls `s.db.Close()` but does not remove the
  file) — same behavior as `database.New` itself; not a regression, just
  worth knowing before "fixing" it in isolation here.
- **`isUniqueViolation` matches on `err.Error()` substring
  (`"UNIQUE constraint failed"`)**, not a typed SQLite error code —
  `modernc.org/sqlite`'s per-constraint codes live in an internal generated
  bindings package, not a stable public API. If the driver's error message
  format ever changes, this detection silently stops working and duplicate
  `Append` falls through to the generic `ErrStoreQuery` wrap instead of the
  plain "already exists" error `storetest` doesn't specifically assert the
  wording of, but `runner.Queue` callers might come to depend on.
