# database

SQLite database management with schema migrations for Wails desktop apps. Uses [goose](https://github.com/pressly/goose) for migrations and [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) as a pure-Go driver (no CGO required).

## Usage

```go
import (
    "embed"
    "github.com/jrschumacher/wails-kit/v2/database"
)

//go:embed migrations/*.sql
var migrations embed.FS

db, err := database.New(
    database.WithAppName("my-app"),           // OS-standard data dir
    database.WithMigrations(migrations),      // embedded SQL migrations
)
if err != nil {
    log.Fatal(err)
}
defer db.Close()

// Use the underlying *sql.DB directly
db.DB().QueryRow("SELECT name FROM users WHERE id = ?", 1)
```

### Database path

`WithAppName` stores the database in the OS-standard data directory via `appdirs`:

| OS      | Path                                              |
|---------|-------------------------------------------------|
| macOS   | `~/Library/Application Support/{app}/data.db`   |
| Linux   | `~/.local/share/{app}/data.db`                  |
| Windows | `%AppData%/{app}/data.db`                       |

Use `WithPath` for an explicit location:

```go
db, err := database.New(
    database.WithPath("/path/to/my.db"),
    database.WithMigrations(migrations),
)
```

### Migrations

Write standard [goose SQL migrations](https://pressly.github.io/goose/blog/2022/overview/#sql-migrations) and embed them:

```sql
-- migrations/001_create_users.sql

-- +goose Up
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE
);

-- +goose Down
DROP TABLE users;
```

Migrations run automatically on `New()`. Use `Version()` to check the current schema version:

```go
version, err := db.Version()
```

`Version()` returns `(0, nil)` only for the legitimate "no migrations applied
yet" case (no `goose_db_version` table). Any other failure — a closed
connection, an I/O error, a corrupt database — is returned as an error
(`database_version`) rather than silently reported as version 0.

### Baseline version (adopting wails-kit with existing tables)

When integrating wails-kit into an app that already has SQLite tables, migrations will fail because goose tries to run all migrations from scratch. Use `WithBaselineVersion` to stamp existing migrations as applied:

```go
db, err := database.New(
    database.WithPath(path),
    database.WithMigrations(migrations),
    database.WithBaselineVersion(2), // stamp versions 0-2 if no goose table exists
)
```

**Behavior:**
- If `goose_db_version` table already exists → no-op (goose is already tracking)
- If the database has no user tables (fresh) → no-op (let goose run from scratch)
- If the database has tables but no goose tracking → creates `goose_db_version` and stamps versions 0 through n, then runs any remaining migrations

Table creation and every stamped row happen inside a single transaction. If
stamping fails partway (disk full, process killed mid-loop), the whole
transaction rolls back — including the `CREATE TABLE`, since SQLite DDL is
transactional — so the next run sees no `goose_db_version` table and retries
baselining from scratch. Without this, a partial stamp would look like a
*completed* baseline to the no-op check above and permanently skip the
remaining versions.

### Schema version guard

The database package automatically protects against schema version mismatches caused by app downgrades. After running migrations, `PRAGMA user_version` is set to the highest migration version. On subsequent opens, if the database's version is higher than the app's max migration, a clear error is returned instead of silently proceeding:

```
database schema version 5 is newer than this app supports (max 3); please update the app
```

This is automatic — no configuration needed.

### Pre-migration backup

Enable automatic backups before migrations run. If any migrations are pending, the database is copied before they are applied:

```go
db, err := database.New(
    database.WithPath(path),
    database.WithMigrations(migrations),
    database.WithBackupBeforeMigration(true),
)
```

**Behavior:**
- Only creates a backup when there are pending migrations (not on every startup)
- Names the backup with the current version: `data.db.backup-v2`
- Keeps at most 3 backups by default, deleting oldest (configurable via `WithMaxBackups`)
- Skips backup for fresh installs (no prior version stamp)
- Uses `VACUUM INTO` for a consistent copy that handles WAL mode correctly

```go
database.WithBackupBeforeMigration(true),
database.WithMaxBackups(5), // keep 5 backups instead of default 3
```

### External database connection

If you manage the `*sql.DB` yourself:

```go
db, err := database.New(
    database.WithDB(existingDB),
    database.WithMigrations(migrations),
)
// db.Close() is a no-op — caller retains ownership
```

**Caveat:** because this package doesn't control the DSN an externally-provided `*sql.DB` was opened with, it can't bake pragmas into it per-connection the way it does when it opens the database itself (see "Default pragmas" below). To keep pragmas honest, `New()` calls `existingDB.SetMaxOpenConns(1)` before applying them — the one pooled connection that then ever exists keeps them for the life of the process. This is a real constraint (no concurrent readers on that pool) that only applies to `WithDB`; a database opened via `WithPath`/`WithAppName` is unaffected and its pool can grow freely.

## Options

| Option | Description |
|--------|-------------|
| `WithAppName(name)` | Derive database path from OS-standard app directories |
| `WithPath(path)` | Explicit database file path |
| `WithMigrations(fs)` | `fs.FS` containing goose SQL migration files |
| `WithEmitter(e)` | Event emitter for lifecycle events |
| `WithPragmas(map)` | Override or extend default SQLite pragmas |
| `WithBaselineVersion(n)` | Stamp versions 0–n as applied for pre-existing databases |
| `WithBackupBeforeMigration(bool)` | Create a backup before running pending migrations |
| `WithMaxBackups(n)` | Maximum number of pre-migration backups to retain (default 3) |
| `WithDB(db)` | Use an existing `*sql.DB` (caller retains ownership) |

## Default pragmas

| Pragma | Value | Purpose |
|--------|-------|---------|
| `journal_mode` | `WAL` | Better concurrent read performance |
| `busy_timeout` | `5000` | Wait 5s on lock contention instead of failing |
| `foreign_keys` | `ON` | Enforce foreign key constraints |
| `synchronous` | `NORMAL` | Safe with WAL, better write performance |
| `journal_size_limit` | `67108864` | Cap WAL file at 64MB |

**How pragmas are actually applied — read this if you use `WithPragmas`.**
`*sql.DB` is a connection *pool*, and every SQLite pragma except `journal_mode`
(which is stored in the database file itself) is **per-connection**. Running
`PRAGMA foreign_keys = ON` once via `Exec` — which is what earlier versions of
this package did — only reaches whichever single connection happens to
service that call. As soon as the pool opens a second connection (any
concurrent query — a held transaction plus a second read is enough),
that connection has `foreign_keys` back to its SQLite default (`OFF`) and
`busy_timeout` at `0`, silently disabling foreign-key enforcement and turning
lock contention into an immediate `SQLITE_BUSY` instead of a bounded wait.

For a database this package opens itself (`WithPath`/`WithAppName`), pragmas
are instead encoded into the connection string as
[modernc.org/sqlite's](https://pkg.go.dev/modernc.org/sqlite) `_pragma` query
parameters (`file:app.db?_pragma=foreign_keys=ON&_pragma=busy_timeout=5000`).
The driver re-applies them on **every** new physical connection it opens, so
the guarantee holds no matter how large the pool grows. This is verified by
`TestForeignKeysEveryConnection`, which pins one connection with an open
transaction, forces the pool to open a second, and asserts the pragmas hold
there too.

For a `WithDB`-supplied external database, see the caveat in "External
database connection" above — pragmas are still applied, but by clamping the
pool to one connection, not via the DSN.

Override with `WithPragmas`:

```go
database.WithPragmas(map[string]string{
    "cache_size":  "-4000",    // add a pragma
    "synchronous": "FULL",     // override a default
    "foreign_keys": "",        // disable a default (empty string skips it)
})
```

## Events

| Event | Payload | When |
|-------|---------|------|
| `database:migrated` | `MigratedPayload{Version, Applied}` | After migrations complete (only if migrations were applied) |

## Error codes

| Code | User message |
|------|-------------|
| `database_open` | Unable to open the database. Please check file permissions and try again. |
| `database_migrate` | Database migration failed. Please contact support. |
| `database_baseline` | Database baseline failed. Please contact support. |
| `database_version_mismatch` | The database was created by a newer version of this app. Please update the app. |
| `database_backup` | Failed to create a database backup before migration. Please check disk space and try again. |
| `database_version` | Unable to determine the database schema version. Please contact support. |
