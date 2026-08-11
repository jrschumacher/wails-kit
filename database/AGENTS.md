# database — agent notes

## Purpose

`database` opens a SQLite database (via `modernc.org/sqlite`, pure Go, no
CGO) with a fixed set of safety pragmas, runs goose migrations, and offers
baseline-stamping for adopting the package into an app with existing tables.
It owns connection setup, pragma application, migration execution, schema
version guarding, and pre-migration backups. It does not own schema design,
query building, or an ORM layer — callers get a plain `*sql.DB` back via
`DB()` and write their own SQL.

## Public API (load-bearing signatures)

```go
func New(opts ...Option) (*DB, error)

func WithAppName(name string) Option        // OS-standard data dir via appdirs
func WithPath(path string) Option           // explicit file path
func WithMigrations(fsys fs.FS) Option       // goose SQL migrations, FS root = migration dir
func WithPragmas(pragmas map[string]string) Option // merge with defaults; "" disables a default
func WithBaselineVersion(n int64) Option
func WithBackupBeforeMigration(enabled bool) Option
func WithMaxBackups(n int) Option
func WithDB(db *sql.DB) Option               // caller-owned *sql.DB; see Landmines
func WithEmitter(e *events.Emitter) Option

func (d *DB) DB() *sql.DB
func (d *DB) Path() string
func (d *DB) Version() (int64, error)        // (0, nil) only if no migrations applied yet
func (d *DB) Close() error                   // no-op if WithDB supplied an external *sql.DB
```

## Invariants (do not break)

- **Pragmas must hold on every pooled connection, not just the first one
  Exec happens to land on.** `*sql.DB` is a connection *pool*; every SQLite
  pragma except `journal_mode` (persisted in the file header) is
  per-connection. For a database this package opens itself, pragmas are
  encoded into the DSN as modernc's `_pragma` query parameters (`buildDSN`),
  which the driver re-applies on every new physical connection
  (`modernc.org/sqlite@v1.46.1` `conn.go:75`, `applyQueryParams` called from
  `newConn`, which `Driver.Open` calls per connection — verify against the
  pinned version if it changes). Don't revert to Exec-ing "PRAGMA ..." once
  after `sql.Open` for the owned path — that's the exact defect
  `TestForeignKeysEveryConnection` guards against, and it's the kind of bug
  that looks fine in every test until the pool grows past one connection.
- **`New()` still Pings the owned connection before returning.**
  `sql.Open` never dials; without a forced first connection, the database
  file wouldn't exist yet and a bad pragma in the DSN wouldn't surface until
  the caller's first query. Don't remove the `PingContext` call.
- **`baseline()`'s table creation and every stamped row are one transaction.**
  SQLite DDL is transactional, so a failure anywhere in the loop rolls back
  the `CREATE TABLE` too. This matters specifically because the function's
  own early-exit check ("`goose_db_version` already exists → no-op") would
  otherwise treat a half-stamped table left by a previous failed run as a
  *completed* baseline and permanently skip the remaining versions. Don't
  split the `CREATE TABLE` and the `INSERT` loop back onto `d.db` directly.
- **`Version()` distinguishes "no `goose_db_version` table yet" (legit `(0,
  nil)`) from every other query/scan failure (returned as an error).**
  Collapsing all `Scan` errors into `(0, nil)` — the pre-fix behavior — hides
  real failures (closed connection, I/O error, corruption) behind a
  plausible-looking zero.
- Backups use `VACUUM INTO`, never a raw file copy — it's the only approach
  that's correct under WAL mode without stopping writers first.

## Dependencies & insulation

- `appdirs` — resolves `{dataDir}/data.db` when `WithAppName` is used.
- `errors` (kit package) — every failure is an `errors.Code`-tagged
  `*errors.UserError`, registered via `RegisterMessages` in `init()`.
- `events` — optional (`WithEmitter`); nil emitter is a no-op via `d.emit`.
- `github.com/pressly/goose/v3` — migration engine; `database` only talks to
  it through `goose.NewProvider(goose.DialectSQLite3, ...)`.
- `modernc.org/sqlite` — the driver; imported blank for `database/sql`
  registration, and directly (via DSN construction, not API calls) for
  pragma handling.
- No `wails/v3` import; this package is Wails-free per the AD-4 allowlist.

## Extension points

- New construction knobs go through `Option`, following the existing
  `With*` pattern — don't add non-functional-option constructor parameters.
- New default pragmas go in `defaultPragmas`; anything added there must be
  safe to set unconditionally on every connection (it will be).
- A read-replica / multi-file mode is out of scope — this package is
  "one SQLite file, one `*sql.DB`" by design.

## Testing

- Doubles: `t.TempDir()`, `testing/fstest.MapFS` for migrations,
  `events.NewMemoryEmitter()` + `events.NewEmitter(...)`.
- `go test -race ./database/` must cover: pragmas surviving pool growth
  (`TestForeignKeysEveryConnection` — pins one connection in an open
  transaction, then forces the pool to open a second and asserts pragmas
  hold there too; this is the one test in the suite that actually exercises
  more than one physical connection), the `WithDB` external-pool clamp
  (`TestNew_WithExternalDB_PragmasAppliedToEveryQuery`), `Version()`'s
  error-vs-zero split (`TestVersion_PropagatesScanErrors`,
  `TestVersion_FreshDatabaseReturnsZeroNoError`), and baseline atomicity
  (`TestNew_WithBaselineVersion_TransactionalOnFailure` — forces a genuine
  mid-transaction failure via `PRAGMA max_page_count`, not just a
  first-statement failure, so it actually distinguishes the fix from the old
  behavior).
- Not automatable: real multi-GB WAL growth / disk-full backup behavior
  beyond what `max_page_count` can cheaply simulate.

## File map

- `database.go` — `DB`, all `Option` functions, `New`, pragma/DSN handling,
  migration + baseline + backup logic, `Version`, `Close`.
- `database_test.go` — full test suite.
- `README.md` — quickstart, migrations, baseline version, schema version
  guard, backups, pragma semantics (read this section if you touch pragma
  handling — it explains the pool-vs-connection distinction in user terms).

## Landmines

- **`WithDB` pragma handling is fundamentally weaker than the owned path.**
  Because the caller opened the `*sql.DB` (we don't control its DSN), `New()`
  falls back to `SetMaxOpenConns(1)` + one-time `Exec` for pragmas — correct,
  but it forcibly clamps a caller's pool to a single connection as a side
  effect of calling `New(WithDB(...))`. This is documented on `WithDB`'s doc
  comment and in the README; don't be surprised by it, and don't "fix" it by
  reverting to unclamped Exec (that reintroduces the original defect for the
  `WithDB` path).
- **DSN construction assumes a `file:`-prefixed URI is safe for the target
  path.** This matches `modernc.org/sqlite`'s own test suite convention, but
  hasn't been verified against Windows drive-letter paths (`C:\...`), which
  can be ambiguous inside a `file:` URI without further escaping
  (`file:///C:/...`). Not exercised by this package's test suite (macOS/CI
  only). If Windows path handling ever needs work, start in `buildDSN`.
- `applyPragmas` (the Exec-based path) is now only correct when the caller
  has already guaranteed a single connection — it's not a general-purpose
  "apply pragmas safely" helper. Don't reuse it elsewhere without re-reading
  why it's paired with `SetMaxOpenConns(1)`.
- `createBackup` and the `PRAGMA user_version = N` stamp in `migrate()` still
  run via `d.db.Exec` against the pool rather than the DSN. This is fine —
  both are database-level state (`user_version` is stored in the file
  header like `journal_mode`; `VACUUM INTO` doesn't have per-connection
  pragma semantics to worry about) — but don't assume everything in this
  file went through the DSN-pragma treatment; only the `defaultPragmas`/
  `WithPragmas` map did.
