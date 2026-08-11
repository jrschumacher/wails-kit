# runner/flatfile

`flatfile.Store` is a `runner.Store` backed by a JSONL (newline-delimited
JSON) file: one job-state record per line, append-only, with periodic
compaction. It's the kit's default `Durable`/`BestEffort` backing store.

## Why JSONL, and why it must stay human-readable

The first consumer of this package (Prune) keeps its job queue committed in
git and runs an LLM over the diff. That only works if the format is
actually readable — not a technical requirement satisfied by "it round-trips
through `json.Unmarshal`", but a design constraint on every field:

- Real field names, matching `runner.Job`'s JSON tags exactly (`id`,
  `type`, `payload`, `state`, `attempts`, `idempotency_key`, `enqueued_at`,
  `next_run_at`, `last_error`).
- `Payload` is embedded as raw, inline JSON — `"payload":{"to":"a@b.com"}`,
  never `"payload":"eyJ0byI6..."` and never a JSON-encoded string of JSON.
- Timestamps are RFC3339 (`time.Time`'s default JSON encoding).
- A compaction pass sorts by `enqueued_at` then `id` — deterministic, so
  compacting a queue that hasn't logically changed produces a
  byte-identical file and an empty diff.

`runner/flatfile/testdata/queue.jsonl` is a committed example; open it
directly to see the format, or read it in `TestGoldenHumanReadable`, which
asserts on the actual bytes (not just successful parsing).

## Usage

```go
import "github.com/jrschumacher/wails-kit/v2/runner/flatfile"

store, err := flatfile.New(flatfile.WithPath("queue.jsonl"))
if err != nil {
    log.Fatal(err)
}
defer store.Close()

q, err := runner.New(runner.Durable(), runner.WithStore(store))
```

`WithPath` is required — there's no `appdirs`-based default the way
`state.Store` has one. The whole point of this store is that its path is
something the app chooses deliberately (typically inside a repo it already
manages), not a hidden per-OS data directory.

## How it works

- **Append-only.** `Append`/`Update` both write a new line; they never edit
  an existing one. A job's current state is whichever line for its `id`
  appears last in the file.
- **In-memory index.** `New` replays the whole file once, keeping only the
  latest record per job ID in memory (`Store.latest`). Every `Due`/`List`
  call reads from that index, not the file.
- **fsync on every write.** `Append`/`Update` each open, write one line,
  `fsync`, and close — a committed write survives a crash immediately
  after.
- **Compaction on `Sweep`.** `Sweep` (called every tick by `runner.Queue`)
  removes aged-out `done`/`failed` records from the in-memory index, then —
  only if something actually changed since the last compaction — rewrites
  the file to contain exactly one line per live job, atomically (write to a
  `.tmp` file, then `rename`). Dead-lettered jobs (`JobStateDead`) are never
  removed by `Sweep`; `DeadLetter`, not `Retention`, controls their
  lifetime.
- **Lenient recovery.** A line that fails to parse — most commonly a
  partial write left by a crash mid-`Append` — is silently skipped on load,
  not treated as fatal. See "Landmines" in `AGENTS.md` for the trade-off
  this implies.

## `Lister`

`Store` implements `runner.Lister` (`List(state) ([]Job, error)`), which
`runner.Queue` uses opportunistically for `Dead()` and to bootstrap
`MaxQueueDepth`/idempotency-key accounting on `Start` — so a flatfile-backed
queue's bookkeeping is accurate across a restart, not just within one
process's lifetime.
