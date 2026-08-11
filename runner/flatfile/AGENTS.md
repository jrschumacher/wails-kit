# runner/flatfile — agent notes

## Purpose

`flatfile` is a `runner.Store` backed by a JSONL file: one job-state record
per line, append-only, with periodic compaction. It owns on-disk
persistence and the human-readability guarantee (real field names, inline
JSON payload, RFC3339 timestamps, no base64). It does not own job
lifecycle/retry/dead-letter policy — that's `runner.Queue`.

## Public API (load-bearing signatures)

```go
type Store struct{ /* unexported */ }
func New(opts ...Option) (*Store, error)
func WithPath(path string) Option // required

func (s *Store) Append(job runner.Job) error // errors if job.ID already exists
func (s *Store) Update(job runner.Job) error // errors if job.ID is unknown
func (s *Store) Due(now time.Time, limit int) ([]runner.Job, error)
func (s *Store) Sweep(retention time.Duration, now time.Time) error
func (s *Store) List(state runner.JobState) ([]runner.Job, error) // implements runner.Lister
func (s *Store) Close() error // no-op; no long-lived file handle
```

## Invariants (do not break)

- **Human-readable, byte-level, always.** Payload is `json.RawMessage`
  embedded inline (`"payload":{...}`) — never a `[]byte` field (which
  base64-encodes) and never double-marshaled into an escaped string.
  `TestGoldenHumanReadable` asserts on actual file bytes, not just
  `json.Unmarshal` success; any change here must keep that test meaningful,
  not looser.
- **Field order is pinned by the local `record` type**, independent of
  `runner.Job`'s Go struct declaration order — a future reordering of
  `runner.Job`'s fields must not silently reorder the on-disk format.
  `record`'s field order is the contract; update it deliberately, not
  incidentally.
- **`Sweep` must never remove `JobStateDead` records.** `DeadLetter`, not
  `Retention`, controls a dead-lettered job's lifetime — see
  `TestStoreContract/SweepPreservesDeadJobs` in the shared `storetest`
  suite.
- **Compaction is deterministic: sorted by `EnqueuedAt` then `ID`.** An
  unchanged-content compaction must produce byte-identical output — this
  is what keeps `git diff` quiet on an idle queue. Don't introduce map
  iteration order (or anything else nondeterministic) into
  `compactLocked`'s output ordering.
- **Every `Append`/`Update` `fsync`s before returning.** A committed write
  must survive a crash immediately after — don't buffer writes across
  calls or defer the `fsync`.
- **`Due` must include running jobs regardless of `NextRunAt`** — this is
  half of `runner`'s crash-recovery mechanism (see `runner/AGENTS.md`); the
  other half (skip if already tracked in-process) lives in `runner.Queue`,
  not here.

## Dependencies & insulation

- `runner` — `Job`, `JobState`, `Store`, `Lister`. One direction only:
  `runner` must never import `flatfile` back.
- No `wails/v3` import — not on the AD-4 allowlist.
- No `i18n`/`errors` package integration: `Store`'s errors (duplicate ID,
  unknown ID, I/O failures) are plumbing-level, translated or ignored by
  `runner.Queue` before anything reaches a user-facing surface. Don't add
  `errors.Code` registrations here without a concrete user-facing case —
  there currently isn't one.

## Extension points

- A future `WithAppName`/`appdirs`-based default path would go here as a
  second `Option`, alongside (not replacing) `WithPath` — see the README's
  "Usage" section for why `WithPath` is deliberately the only one today.
- A `sqlitestore` sibling (WP-41) reuses `runner/storetest.Run` against its
  own `Store` — don't add flatfile-specific assertions to `storetest`
  itself; put them in this package's own `store_test.go`.

## Testing

- `runner/storetest.Run` — the shared `Store` contract suite, run via
  `TestStoreContract`.
- `testdata/queue.jsonl` — committed golden fixture; `TestGoldenHumanReadable`
  greps its actual bytes for real field names and absence of
  base64/escaped-JSON patterns.
- `TestCompactionDeterministic` — compacts twice over identical logical
  content, asserts byte-for-byte equal output.
- `TestCrashMidAppendRecovers` — hand-writes a file with one complete line
  followed by a truncated, unparseable fragment (simulating a crash
  mid-write) and asserts `New` loads the complete record and ignores the
  fragment.
- `go test -race ./runner/flatfile/` must cover all of the above plus
  duplicate-ID/unknown-ID errors and reopen-replays-latest-state-per-job.

## File map

- `store.go` — `record` (on-disk shape + `marshalRecord`), `Store`,
  `Option`/`WithPath`, `New`/`loadExisting`, `Append`/`Update`/
  `appendLineLocked`, `Due`, `Sweep`/`compactLocked`, `List`, `Close`.
- `testdata/queue.jsonl` — committed golden fixture.

## Landmines

- **A line that fails to parse is silently skipped, at any position in the
  file, not just the last line.** This is deliberate leniency for the
  crash-mid-append case (see `TestCrashMidAppendRecovers`), but it means
  genuine mid-file corruption (not just a truncated trailing write) is
  also silently swallowed rather than surfaced as an error. There is no
  currently-planned mitigation beyond "compaction periodically rewrites
  the file cleanly, shrinking the window where stray corruption could
  exist unnoticed."
- **`Sweep` skips compaction entirely if nothing changed** (`s.dirty ==
  false` and nothing aged out) — don't assume every `Sweep` call rewrites
  the file; `TestCompactionDeterministic` relies on this to compare two
  *actual* compaction passes, not two no-ops.
- **`bufio.Scanner`'s buffer is raised to 4MB** (`loadExisting`) because
  the default 64KB token limit would silently fail to load a large-payload
  job's line. A payload larger than 4MB will still fail to load — there is
  no streaming/chunked payload support.
