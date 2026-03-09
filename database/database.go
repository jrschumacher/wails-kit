// Package database provides SQLite database management with schema migrations
// for Wails desktop apps. It uses goose for migration management and modernc.org/sqlite
// as a pure-Go SQLite driver (no CGO required).
package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jrschumacher/wails-kit/appdirs"
	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/events"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

// Error codes for the database package.
const (
	ErrDatabaseOpen    errors.Code = "database_open"
	ErrDatabaseMigrate errors.Code = "database_migrate"
)

func init() {
	errors.RegisterMessages(map[errors.Code]string{
		ErrDatabaseOpen:    "Unable to open the database. Please check file permissions and try again.",
		ErrDatabaseMigrate: "Database migration failed. Please contact support.",
	})
}

// Event names emitted by the database package.
const (
	EventMigrated = "database:migrated"
)

// MigratedPayload is emitted after migrations complete successfully.
type MigratedPayload struct {
	Version int64 `json:"version"`
	Applied int   `json:"applied"`
}

// Default pragmas applied to every database connection.
var defaultPragmas = map[string]string{
	"journal_mode":      "WAL",
	"busy_timeout":      "5000",
	"foreign_keys":      "ON",
	"synchronous":       "NORMAL",
	"journal_size_limit": "67108864",
}

// DB manages a SQLite database with schema migrations.
type DB struct {
	db         *sql.DB
	emitter    *events.Emitter
	path       string
	owned      bool // true if we opened the *sql.DB and should close it
	appName    string
	migrations fs.FS
	pragmas    map[string]string
}

// Option configures a DB instance.
type Option func(*DB)

// WithAppName sets the application name, used to derive the database path
// via appdirs (e.g., ~/Library/Application Support/{app}/data.db on macOS).
func WithAppName(name string) Option {
	return func(d *DB) {
		d.appName = name
	}
}

// WithPath sets an explicit path for the database file, overriding the
// OS-standard path derived from WithAppName.
func WithPath(path string) Option {
	return func(d *DB) {
		d.path = path
	}
}

// WithMigrations provides an fs.FS (typically an embed.FS) containing SQL
// migration files for goose. Files should follow goose naming conventions
// (e.g., 001_create_users.sql).
func WithMigrations(migrations fs.FS) Option {
	return func(d *DB) {
		d.migrations = migrations
	}
}

// WithEmitter sets the event emitter for database lifecycle events.
func WithEmitter(e *events.Emitter) Option {
	return func(d *DB) {
		d.emitter = e
	}
}

// WithPragmas overrides the default SQLite pragmas. The provided map is merged
// with defaults; set a key to empty string to disable a default pragma.
func WithPragmas(pragmas map[string]string) Option {
	return func(d *DB) {
		for k, v := range pragmas {
			d.pragmas[k] = v
		}
	}
}

// WithDB provides an existing *sql.DB connection. When set, the database
// package will not open or close the connection — the caller retains ownership.
// Pragmas are still applied. WithPath/WithAppName are ignored for opening but
// Path() will still return whatever was configured.
func WithDB(db *sql.DB) Option {
	return func(d *DB) {
		d.db = db
		d.owned = false
	}
}

// New creates and configures a new database instance. It opens the SQLite
// database, applies pragmas, and runs any pending migrations.
func New(opts ...Option) (*DB, error) {
	d := &DB{
		owned:   true,
		pragmas: make(map[string]string),
	}

	// Copy defaults into pragmas map.
	for k, v := range defaultPragmas {
		d.pragmas[k] = v
	}

	for _, opt := range opts {
		opt(d)
	}

	// Resolve database path if we need to open a connection.
	if d.db == nil {
		if err := d.resolvePath(); err != nil {
			return nil, err
		}

		// Ensure parent directory exists.
		dir := filepath.Dir(d.path)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, errors.Wrap(ErrDatabaseOpen, fmt.Sprintf("create directory %s", dir), err)
		}

		db, err := sql.Open("sqlite", d.path)
		if err != nil {
			return nil, errors.Wrap(ErrDatabaseOpen, fmt.Sprintf("open %s", d.path), err)
		}
		d.db = db
	}

	// Apply pragmas.
	if err := d.applyPragmas(); err != nil {
		if d.owned {
			d.db.Close()
		}
		return nil, err
	}

	// Run migrations if provided.
	if d.migrations != nil {
		if err := d.migrate(); err != nil {
			if d.owned {
				d.db.Close()
			}
			return nil, err
		}
	}

	return d, nil
}

// DB returns the underlying *sql.DB for direct queries.
func (d *DB) DB() *sql.DB {
	return d.db
}

// Path returns the database file path. Empty if an external *sql.DB was provided
// without a path.
func (d *DB) Path() string {
	return d.path
}

// Version returns the current migration version. Returns 0 if no migrations
// have been applied.
func (d *DB) Version() (int64, error) {
	row := d.db.QueryRow("SELECT MAX(version_id) FROM goose_db_version WHERE version_id > 0")
	var version sql.NullInt64
	if err := row.Scan(&version); err != nil {
		// Table doesn't exist — no migrations have been applied.
		return 0, nil
	}
	return version.Int64, nil
}

// Close closes the database connection if it was opened by this package.
// If an external *sql.DB was provided via WithDB, Close is a no-op.
func (d *DB) Close() error {
	if d.owned && d.db != nil {
		return d.db.Close()
	}
	return nil
}

func (d *DB) resolvePath() error {
	if d.path != "" {
		return nil
	}
	if d.appName == "" {
		return errors.New(ErrDatabaseOpen, "either WithAppName or WithPath is required", nil)
	}
	dirs := appdirs.New(d.appName)
	d.path = filepath.Join(dirs.Data(), "data.db")
	return nil
}

func (d *DB) applyPragmas() error {
	for key, value := range d.pragmas {
		if value == "" {
			continue
		}
		_, err := d.db.Exec(fmt.Sprintf("PRAGMA %s = %s", key, value))
		if err != nil {
			return errors.Wrap(ErrDatabaseOpen, fmt.Sprintf("set pragma %s=%s", key, value), err)
		}
	}
	return nil
}

func (d *DB) migrate() error {
	provider, err := goose.NewProvider(goose.DialectSQLite3, d.db, d.migrations)
	if err != nil {
		return errors.Wrap(ErrDatabaseMigrate, "create goose provider", err)
	}

	results, err := provider.Up(context.Background())
	if err != nil {
		return errors.Wrap(ErrDatabaseMigrate, "run migrations", err)
	}

	if len(results) > 0 {
		version, _ := provider.GetDBVersion(context.Background())
		d.emit(EventMigrated, MigratedPayload{
			Version: version,
			Applied: len(results),
		})
	}

	return nil
}

func (d *DB) emit(name string, data any) {
	if d.emitter != nil {
		d.emitter.Emit(name, data)
	}
}
