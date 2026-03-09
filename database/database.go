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
	"sort"
	"strings"

	"github.com/jrschumacher/wails-kit/appdirs"
	"github.com/jrschumacher/wails-kit/errors"
	"github.com/jrschumacher/wails-kit/events"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

// Error codes for the database package.
const (
	ErrDatabaseOpen            errors.Code = "database_open"
	ErrDatabaseMigrate         errors.Code = "database_migrate"
	ErrDatabaseBaseline        errors.Code = "database_baseline"
	ErrDatabaseVersionMismatch errors.Code = "database_version_mismatch"
	ErrDatabaseBackup          errors.Code = "database_backup"
)

func init() {
	errors.RegisterMessages(map[errors.Code]string{
		ErrDatabaseOpen:            "Unable to open the database. Please check file permissions and try again.",
		ErrDatabaseMigrate:         "Database migration failed. Please contact support.",
		ErrDatabaseBaseline:        "Database baseline failed. Please contact support.",
		ErrDatabaseVersionMismatch: "The database was created by a newer version of this app. Please update the app.",
		ErrDatabaseBackup:          "Failed to create a database backup before migration. Please check disk space and try again.",
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
	db                    *sql.DB
	emitter               *events.Emitter
	path                  string
	owned                 bool // true if we opened the *sql.DB and should close it
	appName               string
	migrations            fs.FS
	pragmas               map[string]string
	baselineVersion       int64
	backupBeforeMigration bool
	maxBackups            int
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

// WithBaselineVersion stamps migration versions 0 through n as applied when
// the database has existing tables but no goose version tracking. This handles
// the "baseline migration" problem when adopting wails-kit in an app that
// already has a schema matching migration n.
//
// If the goose_db_version table already exists, this is a no-op.
// If the database has no user tables (fresh database), this is a no-op.
func WithBaselineVersion(n int64) Option {
	return func(d *DB) {
		d.baselineVersion = n
	}
}

// WithBackupBeforeMigration enables automatic database backup before running
// pending migrations. When enabled, a copy of the database file is created
// (e.g., data.db.backup-v2) before any new migrations are applied. Backups
// are only created when there are pending migrations and the database file
// exists (not on fresh installs). Old backups beyond the retention limit are
// automatically cleaned up (default 3, configurable via WithMaxBackups).
func WithBackupBeforeMigration(enabled bool) Option {
	return func(d *DB) {
		d.backupBeforeMigration = enabled
	}
}

// WithMaxBackups sets the maximum number of pre-migration backups to retain.
// Oldest backups are deleted when the limit is exceeded. Defaults to 3.
// Only effective when WithBackupBeforeMigration is enabled.
func WithMaxBackups(n int) Option {
	return func(d *DB) {
		d.maxBackups = n
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
			_ = d.db.Close()
		}
		return nil, err
	}

	// Apply baseline version if configured.
	if d.baselineVersion > 0 {
		if err := d.baseline(); err != nil {
			if d.owned {
				_ = d.db.Close()
			}
			return nil, err
		}
	}

	// Run migrations if provided.
	if d.migrations != nil {
		if err := d.migrate(); err != nil {
			if d.owned {
				_ = d.db.Close()
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

	// Determine the max migration version from the migration sources.
	sources := provider.ListSources()
	var maxVersion int64
	if len(sources) > 0 {
		maxVersion = sources[len(sources)-1].Version
	}

	// Read current user_version for version guard and backup decisions.
	var userVersion int64
	if maxVersion > 0 {
		if err := d.db.QueryRow("PRAGMA user_version").Scan(&userVersion); err != nil {
			return errors.Wrap(ErrDatabaseMigrate, "read user_version", err)
		}
		// Schema version guard: reject if the DB was migrated by a newer app.
		if userVersion > maxVersion {
			return errors.New(ErrDatabaseVersionMismatch,
				fmt.Sprintf("database schema version %d is newer than this app supports (max %d); please update the app", userVersion, maxVersion), nil)
		}
	}

	// Pre-migration backup if enabled.
	// Skip for fresh databases (user_version == 0 means no prior version stamp).
	if d.backupBeforeMigration && d.path != "" && userVersion > 0 {
		pending, err := provider.HasPending(context.Background())
		if err != nil {
			return errors.Wrap(ErrDatabaseMigrate, "check pending migrations", err)
		}
		if pending {
			if err := d.createBackup(); err != nil {
				return err
			}
		}
	}

	results, err := provider.Up(context.Background())
	if err != nil {
		return errors.Wrap(ErrDatabaseMigrate, "run migrations", err)
	}

	// Stamp PRAGMA user_version so future downgrades are detected.
	if maxVersion > 0 {
		if _, err := d.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", maxVersion)); err != nil {
			return errors.Wrap(ErrDatabaseMigrate, "set user_version", err)
		}
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

func (d *DB) baseline() error {
	// Check if goose_db_version table already exists.
	var gooseTableExists bool
	err := d.db.QueryRow(
		"SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name='goose_db_version'",
	).Scan(&gooseTableExists)
	if err != nil {
		return errors.Wrap(ErrDatabaseBaseline, "check goose table", err)
	}
	if gooseTableExists {
		return nil
	}

	// Check if the database has any user tables.
	var hasUserTables bool
	err = d.db.QueryRow(
		"SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'",
	).Scan(&hasUserTables)
	if err != nil {
		return errors.Wrap(ErrDatabaseBaseline, "check user tables", err)
	}
	if !hasUserTables {
		return nil
	}

	// Database has existing tables but no goose tracking — stamp baseline.
	_, err = d.db.Exec(`CREATE TABLE goose_db_version (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version_id INTEGER NOT NULL,
		is_applied INTEGER NOT NULL,
		tstamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return errors.Wrap(ErrDatabaseBaseline, "create goose table", err)
	}

	for v := int64(0); v <= d.baselineVersion; v++ {
		_, err = d.db.Exec(
			"INSERT INTO goose_db_version (version_id, is_applied) VALUES (?, ?)", v, 1,
		)
		if err != nil {
			return errors.Wrap(ErrDatabaseBaseline, fmt.Sprintf("stamp version %d", v), err)
		}
	}

	return nil
}

func (d *DB) createBackup() error {
	// Read current version for the backup filename.
	var currentVersion int64
	_ = d.db.QueryRow("PRAGMA user_version").Scan(&currentVersion)

	backupPath := fmt.Sprintf("%s.backup-v%d", d.path, currentVersion)

	// Remove existing backup at this path (e.g., from a previous failed attempt).
	_ = os.Remove(backupPath)

	// Use VACUUM INTO for a consistent copy that handles WAL mode correctly.
	escapedPath := strings.ReplaceAll(backupPath, "'", "''")
	if _, err := d.db.Exec(fmt.Sprintf("VACUUM INTO '%s'", escapedPath)); err != nil {
		return errors.Wrap(ErrDatabaseBackup, "create backup", err)
	}

	d.cleanOldBackups()
	return nil
}

func (d *DB) cleanOldBackups() {
	maxBackups := d.maxBackups
	if maxBackups <= 0 {
		maxBackups = 3
	}

	pattern := d.path + ".backup-v*"
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) <= maxBackups {
		return
	}

	// Sort by modification time (oldest first).
	sort.Slice(matches, func(i, j int) bool {
		fi, errI := os.Stat(matches[i])
		fj, errJ := os.Stat(matches[j])
		if errI != nil || errJ != nil {
			return false
		}
		return fi.ModTime().Before(fj.ModTime())
	})

	for _, m := range matches[:len(matches)-maxBackups] {
		_ = os.Remove(m)
	}
}

func (d *DB) emit(name string, data any) {
	if d.emitter != nil {
		d.emitter.Emit(name, data)
	}
}
