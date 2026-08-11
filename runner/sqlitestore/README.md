# runner/sqlitestore

`sqlitestore.Store` is a `runner.Store` backed by SQLite, built on the kit's
`database` package. It's the alternative to `runner/flatfile` for apps that
want a queryable, indexed job store rather than a JSONL file in git — a
plain desktop app's local data directory, not a repo a human reads diffs of.

## Why `database`, and why that matters for correctness

SQLite pragmas other than `journal_mode` are per-*connection*, not
per-database. A pool that opens more than one physical connection —
inevitable under any real concurrency — silently loses `foreign_keys`
enforcement and `busy_timeout` (turning lock contention into immediate
`SQLITE_BUSY` instead of a bounded wait) on every connection after the
first, unless those pragmas are baked into the connection DSN itself. This
was a real, shipped defect in the kit's `database` package (see
`database/database.go` and its `TestForeignKeysEveryConnection`), fixed by
encoding pragmas into the DSN's `_pragma` query parameter so the driver
reapplies them on every connection it opens.

`sqlitestore` does not re-derive that guarantee — it reuses it, entirely, by
never opening a `*sql.DB` on its own. Every path through `New` produces or
receives a `*database.DB`, and every SQL statement in this package goes
through `database.DB.DB()`. See `TestBusyTimeoutHeldOnSecondConnection` and
`TestConcurrentAppendUnderWAL` in `store_test.go` for the regression
coverage, run the same way `database`'s own test forces a second pooled
connection (pin one connection in an open transaction, then request another
from the pool).

## Ownership: `WithPath` vs `WithDatabase`

```go
import "github.com/jrschumacher/wails-kit/v2/runner/sqlitestore"

// Store owns a dedicated SQLite file and closes it on Store.Close.
store, err := sqlitestore.New(sqlitestore.WithPath("queue.db"))

// Store uses a database.DB the app already constructed and owns; Store
// never closes it.
db, err := database.New(database.WithPath("app.db"), database.WithMigrations(appMigrations))
store, err := sqlitestore.New(sqlitestore.WithDatabase(db))
```

`WithPath` and `WithDatabase` are mutually exclusive; `New` requires exactly
one.

- **`WithPath`** is the default shape, analogous to `flatfile.WithPath`: a
  dedicated file for the job queue, opened via `database.New` (so it gets
  the full DSN-baked pragma guarantee), migrated, and closed by
  `Store.Close`.
- **`WithDatabase`** accepts an existing `*database.DB` — typically an
  app's main database, already open with its own migrations applied. `Store`
  adds its own `jobs` table to it (see "Schema" below) and never closes it;
  the caller owns its lifecycle. **If that `database.DB` was itself built
  via `database.WithDB`** (wrapping a `*sql.DB` the `database` package
  doesn't control the DSN of), it carries a weaker guarantee: `database.New`
  forces `SetMaxOpenConns(1)` on it and `Exec`s pragmas once, rather than
  baking them into the DSN, because it cannot control how that `*sql.DB`
  was opened. That constraint — a single connection, no concurrent readers
  — propagates unchanged to `sqlitestore` in that configuration. `sqlitestore`
  adds no pragma management of its own in either case; whatever guarantee
  the supplied `database.DB` carries is exactly the guarantee `Store` gets.

## Schema and migrations

`sqlitestore` ships exactly one goose migration
(`migrations/001_create_jobs.sql`), applied automatically by `New`:

```sql
CREATE TABLE jobs (
    id, type, payload, state, attempts, idempotency_key,
    enqueued_at, next_run_at, last_error
);
CREATE INDEX idx_jobs_state_next_run_at ON jobs (state, next_run_at);
```

Column names match `runner.Job`'s JSON tags. It's applied under its own
goose version-tracking table (`goose_db_version_runner_sqlitestore`, not
goose's default `goose_db_version`), so sharing a `database.DB` via
`WithDatabase` never collides with an application's own migration history —
see `TestWithDatabaseSharesExternalDB`.

There is no schema-change mechanism beyond this today: future changes are
additional numbered migration files applied incrementally on the next
`New`, the same way `database.New` handles its own `WithMigrations`. There
is no down-migration tooling exposed and no manual DDL path.

## Human-readability

The `payload` column is `TEXT`, holding the job's raw JSON exactly as
provided — never base64, never a re-escaped string of JSON.
`sqlite3 queue.db 'select * from jobs'` shows the same payload a human
would see in `runner/flatfile`'s JSONL. There's no committed golden-file
requirement here the way there is for `flatfile` (a SQLite file isn't
git-diffable text), but `TestPayloadStoredAsPlainJSON` pins the same
guarantee at the column level.

Timestamps (`enqueued_at`, `next_run_at`) are stored as fixed-width,
always-UTC RFC3339-with-nanoseconds text (`2006-01-02T15:04:05.000000000Z`)
rather than `time.Time`'s default JSON encoding, which trims trailing
fractional zeros and would sort incorrectly as SQLite `TEXT` — the fixed
width keeps both readability and correct ordering for the `(state,
next_run_at)` index and the `Due`/`Sweep` queries.

## Usage

```go
store, err := sqlitestore.New(sqlitestore.WithPath("queue.db"))
if err != nil {
    log.Fatal(err)
}
defer store.Close()

q, err := runner.New(runner.Durable(), runner.WithStore(store))
```

## `Lister`

`Store` implements `runner.Lister` (`List(state) ([]Job, error)`), which
`runner.Queue` uses for `Dead()` and to bootstrap `MaxQueueDepth`/
idempotency-key accounting on `Start` — restart-durable bookkeeping, not
just within-process.

## Events

`Store` does not emit events itself — that's `runner.Queue`'s
responsibility (`runner:job_done`/`job_failed`/`job_dead`), consistent with
`runner/flatfile`. `Store`'s own methods hold no long-lived in-process lock
across their SQL calls (concurrency is SQLite's, via WAL + `busy_timeout`,
not a Go `sync.Mutex`), so the "never emit while holding a lock" invariant
that bit four other kit packages doesn't apply here by construction — see
`AGENTS.md` if that ever changes.

## Error codes

- `runner_sqlitestore_open` — the database file couldn't be opened.
- `runner_sqlitestore_migrate` — the `jobs` table migration failed.
- `runner_sqlitestore_query` — a query/exec against `jobs` failed.

`Store`-contract programmer errors (`Append` on a duplicate ID, `Update` on
an unknown ID) are plain errors, not kit error codes — matching
`runner/flatfile` and `runner`'s built-in memory store. Those are bugs in
the calling code (`runner.Queue`), never meant to reach an end user as a
localized message.
