// Package database provides SQLite database management with schema migrations
// for Wails desktop apps. It uses goose for migration management and modernc.org/sqlite
// as a pure-Go SQLite driver (no CGO required).
package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jrschumacher/wails-kit/v2/appdirs"
	"github.com/jrschumacher/wails-kit/v2/errors"
	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/i18n"
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
	ErrDatabaseVersion         errors.Code = "database_version"
)

func init() {
	errors.RegisterMessages(map[errors.Code]i18n.Text{
		ErrDatabaseOpen:            i18n.T("wailskit.database.errors.database_open", "Unable to open the database. Please check file permissions and try again."),
		ErrDatabaseMigrate:         i18n.T("wailskit.database.errors.database_migrate", "Database migration failed. Please contact support."),
		ErrDatabaseBaseline:        i18n.T("wailskit.database.errors.database_baseline", "Database baseline failed. Please contact support."),
		ErrDatabaseVersionMismatch: i18n.T("wailskit.database.errors.database_version_mismatch", "The database was created by a newer version of this app. Please update the app."),
		ErrDatabaseBackup:          i18n.T("wailskit.database.errors.database_backup", "Failed to create a database backup before migration. Please check disk space and try again."),
		ErrDatabaseVersion:         i18n.T("wailskit.database.errors.database_version", "Unable to determine the database schema version. Please contact support."),
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
	"journal_mode":       "WAL",
	"busy_timeout":       "5000",
	"foreign_keys":       "ON",
	"synchronous":        "NORMAL",
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
// WithPath/WithAppName are ignored for opening but Path() will still return
// whatever was configured.
//
// Pragmas are still applied, but with a caveat: SQLite pragmas other than
// journal_mode are per-connection, and an externally-provided *sql.DB was
// already opened with a DSN this package does not control, so pragmas cannot
// be baked into it the way New() does for a database it opens itself (see
// buildDSN). To keep the "every connection sees these pragmas" guarantee
// honest, New() forces db.SetMaxOpenConns(1) on an external *sql.DB before
// applying pragmas via Exec, so the single pooled connection that exists
// keeps them for the lifetime of the process. This is a real behavior change
// (external DBs used to allow pool growth silently and unsafely) and a real
// constraint (no concurrent readers) — defensible for a desktop app's local
// SQLite file, but callers relying on concurrent access to a WithDB-supplied
// pool should not use this package's pragma management and should apply
// their own DSN-based pragmas instead.
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

	if d.db == nil {
		// We own the connection: resolve the path and open it ourselves.
		if err := d.resolvePath(); err != nil {
			return nil, err
		}

		// Ensure parent directory exists.
		dir := filepath.Dir(d.path)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, errors.Wrap(ErrDatabaseOpen, fmt.Sprintf("create directory %s", dir), err)
		}

		// Bake pragmas into the connection DSN via modernc.org/sqlite's
		// _pragma query parameter instead of Exec-ing "PRAGMA ..." after
		// Open. sql.DB is a connection *pool*; Exec runs on whichever
		// connection happens to be free, and SQLite pragmas other than
		// journal_mode are per-connection. A pool that grows past one
		// connection (any concurrent query, e.g. a held transaction plus a
		// second read) previously got foreign_keys=OFF and busy_timeout=0 on
		// every connection beyond the first, silently disabling FK
		// enforcement and turning lock contention into immediate
		// SQLITE_BUSY instead of a bounded wait. Query params in the DSN
		// are applied by the driver on every new physical connection
		// (modernc.org/sqlite@v1.46.1 conn.go:75, applyQueryParams called
		// from newConn, which Driver.Open calls per connection), so this
		// holds regardless of how large the pool grows. See
		// TestForeignKeysEveryConnection.
		db, err := sql.Open("sqlite", d.buildDSN())
		if err != nil {
			return nil, errors.Wrap(ErrDatabaseOpen, fmt.Sprintf("open %s", d.path), err)
		}
		// sql.Open never dials; it just validates the DSN. Ping forces the
		// first physical connection now, both so the database file exists
		// immediately after New() returns (callers relied on this — see
		// TestNew_WithPath) and so a bad pragma value in the DSN surfaces
		// here rather than on the caller's first query.
		if err := db.PingContext(context.Background()); err != nil {
			_ = db.Close()
			return nil, errors.Wrap(ErrDatabaseOpen, fmt.Sprintf("open %s", d.path), err)
		}
		d.db = db
	} else {
		// External *sql.DB: we don't control its DSN, so pragmas can't be
		// baked in per-connection. Force a single connection so the Exec'd
		// pragmas below stay valid for every query (see WithDB doc comment).
		d.db.SetMaxOpenConns(1)
		if err := d.applyPragmas(); err != nil {
			return nil, err
		}
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
//
// The absence of goose_db_version (fresh database) is checked explicitly and
// treated as version 0. Any other query/scan failure (closed connection,
// I/O error, corrupt database) is returned as an error rather than silently
// reported as version 0 — the previous implementation collapsed every Scan
// error into "no migrations applied, no error", which hid real failures
// behind a valid-looking zero.
func (d *DB) Version() (int64, error) {
	var gooseTableExists bool
	err := d.db.QueryRow(
		"SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name='goose_db_version'",
	).Scan(&gooseTableExists)
	if err != nil {
		return 0, errors.Wrap(ErrDatabaseVersion, "check goose_db_version table", err)
	}
	if !gooseTableExists {
		return 0, nil
	}

	var version sql.NullInt64
	err = d.db.QueryRow(
		"SELECT MAX(version_id) FROM goose_db_version WHERE version_id > 0",
	).Scan(&version)
	if err != nil {
		return 0, errors.Wrap(ErrDatabaseVersion, "read goose_db_version", err)
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

// buildDSN returns the DSN used to open an owned SQLite connection, with
// active pragmas encoded as modernc.org/sqlite's `_pragma` query parameter so
// the driver re-applies them on every new physical connection it opens (see
// the comment in New()). Pragmas set to "" (disabled) are omitted. Keys are
// sorted for a deterministic, testable DSN.
func (d *DB) buildDSN() string {
	active := make([]string, 0, len(d.pragmas))
	for key, value := range d.pragmas {
		if value == "" {
			continue
		}
		active = append(active, key)
	}
	if len(active) == 0 {
		return d.path
	}
	sort.Strings(active)

	q := url.Values{}
	for _, key := range active {
		q.Add("_pragma", key+"="+d.pragmas[key])
	}
	return "file:" + d.path + "?" + q.Encode()
}

// applyPragmas Exec's each configured pragma against d.db. Only correct as a
// per-connection guarantee when d.db is limited to a single connection (see
// callers: it's used exclusively for the WithDB / external *sql.DB path,
// after New() calls SetMaxOpenConns(1)).
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
	//
	// The table creation and every stamped row are wrapped in a single
	// transaction (SQLite DDL is transactional, so CREATE TABLE rolls back
	// too) so a mid-loop failure — disk full, a bad baselineVersion, the
	// process dying — can't leave goose_db_version half-stamped. A
	// half-stamped table is worse than no table: the early-exit check above
	// ("if gooseTableExists return nil") would see it on the next run and
	// treat baselining as already done, permanently skipping the remaining
	// versions. Wrapping in a transaction means a failure leaves no table at
	// all, so the next run retries baselining from scratch.
	tx, err := d.db.Begin()
	if err != nil {
		return errors.Wrap(ErrDatabaseBaseline, "begin baseline transaction", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	_, err = tx.Exec(`CREATE TABLE goose_db_version (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version_id INTEGER NOT NULL,
		is_applied INTEGER NOT NULL,
		tstamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return errors.Wrap(ErrDatabaseBaseline, "create goose table", err)
	}

	for v := int64(0); v <= d.baselineVersion; v++ {
		_, err = tx.Exec(
			"INSERT INTO goose_db_version (version_id, is_applied) VALUES (?, ?)", v, 1,
		)
		if err != nil {
			return errors.Wrap(ErrDatabaseBaseline, fmt.Sprintf("stamp version %d", v), err)
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(ErrDatabaseBaseline, "commit baseline transaction", err)
	}
	committed = true

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
