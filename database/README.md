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
