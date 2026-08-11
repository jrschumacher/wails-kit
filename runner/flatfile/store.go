// Package flatfile is a runner.Store backed by a JSONL (newline-delimited
// JSON) file: one job-state record per line, append-only, with periodic
// compaction. It is the kit's default Durable/BestEffort backing store.
//
// Human-readability is a hard requirement, not a preference — the first
// consumer keeps its job queue in git and expects `git diff` on it to mean
// something to a human or an LLM reading the diff. Concretely:
//
//   - Real field names, matching runner.Job's JSON tags exactly.
//   - Payload is embedded as raw, inline JSON — never base64, never a
//     JSON-encoded string of JSON (json.RawMessage marshals verbatim).
//   - Timestamps are RFC3339 (time.Time's default JSON encoding).
//   - Field order is fixed by an explicit local record type (see
//     marshalRecord), independent of runner.Job's own struct field order.
//
// A job's history is every line with its ID, in file order; the latest
// line for a given ID is its current state (last-write-wins). Compaction
// (see Store.Sweep) rewrites the file keeping exactly one line per live
// job, sorted by EnqueuedAt then ID — deterministic, so a compaction that
// changes nothing produces a byte-identical file and an empty `git diff`.
package flatfile

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/jrschumacher/wails-kit/v2/runner"
)

// record is the on-disk shape of one JSONL line. It exists separately from
// runner.Job so the file format's field order is pinned here, not
// incidentally inherited from runner.Job's Go struct declaration order.
type record struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	State          runner.JobState `json:"state"`
	Attempts       int             `json:"attempts"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	EnqueuedAt     time.Time       `json:"enqueued_at"`
	NextRunAt      time.Time       `json:"next_run_at"`
	LastError      string          `json:"last_error,omitempty"`
}

func toRecord(job runner.Job) record {
	return record{
		ID:             job.ID,
		Type:           job.Type,
		Payload:        job.Payload,
		State:          job.State,
		Attempts:       job.Attempts,
		IdempotencyKey: job.IdempotencyKey,
		EnqueuedAt:     job.EnqueuedAt,
		NextRunAt:      job.NextRunAt,
		LastError:      job.LastError,
	}
}

func (r record) toJob() runner.Job {
	return runner.Job{
		ID:             r.ID,
		Type:           r.Type,
		Payload:        r.Payload,
		State:          r.State,
		Attempts:       r.Attempts,
		IdempotencyKey: r.IdempotencyKey,
		EnqueuedAt:     r.EnqueuedAt,
		NextRunAt:      r.NextRunAt,
		LastError:      r.LastError,
	}
}

func marshalRecord(job runner.Job) ([]byte, error) {
	return json.Marshal(toRecord(job))
}

// Store is a runner.Store backed by a JSONL file at Path. Construct with
// New; the zero value is not usable.
type Store struct {
	path string

	mu     sync.Mutex
	latest map[string]runner.Job // job ID -> current (last-write-wins) state
	dirty  bool                  // true when latest has changed since the last compaction
}

// Option configures a Store at construction.
type Option func(*config)

type config struct {
	path string
}

// WithPath sets the JSONL file's path. Required — New returns an error
// without it. The parent directory is created if missing.
//
// There is deliberately no WithAppName/appdirs-based default (unlike
// state.Store): the whole point of this Store is that its path is
// something the app chooses deliberately — typically inside a repo the
// app already manages (Prune's workspace repo, kept in git) — not a
// hidden per-OS data directory.
func WithPath(path string) Option {
	return func(c *config) { c.path = path }
}

// New constructs a Store, loading any existing JSONL file at the
// configured path. A missing file is not an error (a fresh queue starts
// empty); a malformed trailing line (e.g. a partial write left by a crash
// mid-Append) is skipped rather than treated as a fatal error — see
// Store's "Landmines" in AGENTS.md.
func New(opts ...Option) (*Store, error) {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.path == "" {
		return nil, fmt.Errorf("runner/flatfile: WithPath is required")
	}

	if err := os.MkdirAll(filepath.Dir(cfg.path), 0o755); err != nil {
		return nil, fmt.Errorf("runner/flatfile: create directory: %w", err)
	}

	s := &Store{
		path:   cfg.path,
		latest: make(map[string]runner.Job),
	}
	if err := s.loadExisting(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) loadExisting() error {
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("runner/flatfile: open %s: %w", s.path, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	// Payloads can legitimately exceed bufio.Scanner's 64KB default —
	// raise the ceiling generously (4MB) rather than fail on a large job.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var r record
		if err := json.Unmarshal(line, &r); err != nil {
			// Lenient by design: a line that fails to parse (most
			// commonly a partial trailing write left by a crash
			// mid-Append) is skipped, not fatal. This is what makes
			// "a crash mid-append recovers" true — see
			// TestCrashMidAppendRecovers.
			continue
		}
		s.latest[r.ID] = r.toJob()
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("runner/flatfile: read %s: %w", s.path, err)
	}
	return nil
}

// Append adds a newly enqueued job. It errors if job.ID already exists.
func (s *Store) Append(job runner.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.latest[job.ID]; exists {
		return fmt.Errorf("runner/flatfile: job %q already exists", job.ID)
	}
	if err := s.appendLineLocked(job); err != nil {
		return err
	}
	s.latest[job.ID] = job
	s.dirty = true
	return nil
}

// Update persists job's current state. It errors if job.ID is unknown.
func (s *Store) Update(job runner.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.latest[job.ID]; !exists {
		return fmt.Errorf("runner/flatfile: job %q not found", job.ID)
	}
	if err := s.appendLineLocked(job); err != nil {
		return err
	}
	s.latest[job.ID] = job
	s.dirty = true
	return nil
}

// appendLineLocked writes one JSONL line and fsyncs it before returning,
// so a committed Append/Update survives a crash immediately after. Callers
// must hold s.mu.
func (s *Store) appendLineLocked(job runner.Job) error {
	line, err := marshalRecord(job)
	if err != nil {
		return fmt.Errorf("runner/flatfile: marshal job %q: %w", job.ID, err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("runner/flatfile: open %s: %w", s.path, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("runner/flatfile: write %s: %w", s.path, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("runner/flatfile: sync %s: %w", s.path, err)
	}
	return nil
}

// Due returns jobs ready to dispatch — see runner.Store.Due's contract
// (pending-and-due, plus any running job regardless of NextRunAt).
func (s *Store) Due(now time.Time, limit int) ([]runner.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var due []runner.Job
	for _, j := range s.latest {
		switch {
		case j.State == runner.JobStateRunning:
			due = append(due, j)
		case j.State == runner.JobStatePending && !j.NextRunAt.After(now):
			due = append(due, j)
		}
	}
	sortByEnqueuedThenID(due)
	if limit > 0 && len(due) > limit {
		due = due[:limit]
	}
	return due, nil
}

// Sweep removes done/failed jobs older than retention, then compacts the
// file if anything changed since the last compaction (a deleted job, or
// any Append/Update). Dead-lettered jobs (JobStateDead) are never removed
// here — DeadLetter, not Retention, controls their lifetime.
func (s *Store) Sweep(retention time.Duration, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := now.Add(-retention)
	removed := false
	for id, j := range s.latest {
		if j.State != runner.JobStateDone && j.State != runner.JobStateFailed {
			continue
		}
		if !j.NextRunAt.After(cutoff) {
			delete(s.latest, id)
			removed = true
		}
	}

	if !s.dirty && !removed {
		return nil
	}
	if err := s.compactLocked(); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

// compactLocked rewrites the file to contain exactly one line per entry in
// s.latest, sorted deterministically (EnqueuedAt then ID) so an
// unchanged-content compaction produces a byte-identical file. The write
// is atomic (write-to-tmp, then rename) — the same pattern state.Store
// uses for the same reason: a crash mid-write must never leave a
// half-written file in the real path.
func (s *Store) compactLocked() error {
	jobs := make([]runner.Job, 0, len(s.latest))
	for _, j := range s.latest {
		jobs = append(jobs, j)
	}
	sortByEnqueuedThenID(jobs)

	var buf bytes.Buffer
	for _, j := range jobs {
		line, err := marshalRecord(j)
		if err != nil {
			return fmt.Errorf("runner/flatfile: marshal job %q: %w", j.ID, err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("runner/flatfile: write %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("runner/flatfile: rename %s to %s: %w", tmpPath, s.path, err)
	}
	return nil
}

// List returns every job currently in the given state — implements
// runner.Lister.
func (s *Store) List(state runner.JobState) ([]runner.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []runner.Job
	for _, j := range s.latest {
		if j.State == state {
			out = append(out, j)
		}
	}
	sortByEnqueuedThenID(out)
	return out, nil
}

// Close is a no-op: Store holds no long-lived file handle (each
// Append/Update opens, writes, fsyncs, and closes independently).
func (s *Store) Close() error { return nil }

func sortByEnqueuedThenID(jobs []runner.Job) {
	// Contract order is NextRunAt, then EnqueuedAt, then ID (see the Store
	// docs in runner.go). NextRunAt was previously omitted here, so a job
	// scheduled far in the future but enqueued early sorted ahead of one
	// due immediately — a real divergence from the memory store that went
	// unnoticed because the shared contract suite did not assert ordering.
	sort.Slice(jobs, func(i, j int) bool {
		a, b := jobs[i], jobs[j]
		if !a.NextRunAt.Equal(b.NextRunAt) {
			return a.NextRunAt.Before(b.NextRunAt)
		}
		if !a.EnqueuedAt.Equal(b.EnqueuedAt) {
			return a.EnqueuedAt.Before(b.EnqueuedAt)
		}
		return a.ID < b.ID
	})
}
