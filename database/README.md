# database

SQLite database management with schema migrations for Wails desktop apps. Uses [goose](https://github.com/pressly/goose) for migrations and [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) as a pure-Go driver (no CGO required).

## Usage

```go
import (
    "embed"
    "github.com/jrschumacher/wails-kit/database"
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

Applied automatically to every connection:

| Pragma | Value | Purpose |
|--------|-------|---------|
| `journal_mode` | `WAL` | Better concurrent read performance |
| `busy_timeout` | `5000` | Wait 5s on lock contention instead of failing |
| `foreign_keys` | `ON` | Enforce foreign key constraints |
| `synchronous` | `NORMAL` | Safe with WAL, better write performance |
| `journal_size_limit` | `67108864` | Cap WAL file at 64MB |

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
