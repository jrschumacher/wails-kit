package flatfile

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/v2/runner"
	"github.com/jrschumacher/wails-kit/v2/runner/storetest"
)

func TestStoreContract(t *testing.T) {
	storetest.Run(t, func(t *testing.T) runner.Store {
		t.Helper()
		s, err := New(WithPath(filepath.Join(t.TempDir(), "queue.jsonl")))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return s
	})
}

func TestNewRequiresPath(t *testing.T) {
	if _, err := New(); err == nil {
		t.Fatal("New without WithPath: want error, got nil")
	}
}

func TestNewWithMissingFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.jsonl")
	s, err := New(WithPath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	due, err := s.Due(time.Now(), 10)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("Due on a fresh store = %+v, want none", due)
	}
}

// TestGoldenHumanReadable is the hard requirement from docs/v2-roadmap.md
// WP-40: the on-disk format must be genuinely readable by a human or an
// LLM running `git diff` on it. It asserts on the actual bytes of a
// committed golden fixture — not just that json.Unmarshal succeeds.
func TestGoldenHumanReadable(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "queue.jsonl"))
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}

	content := string(data)

	// Real field names, not abbreviations or numeric codes.
	for _, want := range []string{
		`"id":`, `"type":`, `"payload":`, `"state":`, `"attempts":`,
		`"enqueued_at":`, `"next_run_at":`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("golden file missing expected field %q", want)
		}
	}

	// The payload must appear as inline, readable JSON — never base64,
	// never a JSON-encoded string of JSON.
	if !strings.Contains(content, `"payload":{`) {
		t.Error(`golden file's payload is not embedded as inline JSON (expected "payload":{...})`)
	}
	if strings.Contains(content, "base64") {
		t.Error("golden file mentions base64 — payload must never be base64-encoded")
	}
	if strings.Contains(strings.ToLower(content), `"payload":"`) {
		t.Error(`golden file's payload looks like an escaped JSON string, not inline JSON`)
	}

	// Every non-blank line must be valid, single-line JSON — this is a
	// JSONL file, not a JSON array or pretty-printed blob.
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("golden file has no lines")
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var r record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Errorf("golden line %d is not valid JSON: %v\nline: %s", i, err, line)
		}
	}
}

// TestCompactionDeterministic proves that compacting a store whose
// content hasn't logically changed produces byte-identical output — the
// property that keeps `git diff` quiet on an idle queue.
func TestCompactionDeterministic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.jsonl")
	s, err := New(WithPath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	now := time.Now().Truncate(time.Second).UTC()
	for i, id := range []string{"c", "a", "b"} {
		job := runner.Job{
			ID:         id,
			Type:       "noop",
			Payload:    json.RawMessage(`{"n":` + string(rune('0'+i)) + `}`),
			State:      runner.JobStateDone,
			EnqueuedAt: now.Add(time.Duration(i) * time.Second),
			NextRunAt:  now.Add(time.Duration(i) * time.Second),
		}
		if err := s.Append(job); err != nil {
			t.Fatalf("Append(%s): %v", id, err)
		}
	}

	if err := s.Sweep(time.Hour, now); err != nil { // nothing old enough to remove; forces a compaction pass
		t.Fatalf("Sweep: %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after first compaction: %v", err)
	}

	// Force a second compaction with no logical change and compare bytes.
	s2, err := New(WithPath(path))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = s2.Close() }()
	if err := s2.forceCompact(); err != nil {
		t.Fatalf("forceCompact: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after second compaction: %v", err)
	}

	if !bytes.Equal(first, second) {
		t.Fatalf("compaction is not deterministic:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	// Sorted by EnqueuedAt: "c" was appended first chronologically in this
	// test's loop (i=0), so it must appear first in the compacted file
	// despite being appended before "a" and "b" in insertion order too —
	// this assertion is really about EnqueuedAt ordering, not insertion
	// order, so re-derive expected order explicitly.
	lines := strings.Split(strings.TrimRight(string(first), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("compacted file has %d lines, want 3", len(lines))
	}
	var ids []string
	for _, line := range lines {
		var r record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("unmarshal compacted line: %v", err)
		}
		ids = append(ids, r.ID)
	}
	want := []string{"c", "a", "b"} // c, a, b appended with increasing EnqueuedAt in that order
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("compacted order = %v, want %v (sorted by EnqueuedAt)", ids, want)
		}
	}
}

// forceCompact is a test-only hook that runs compaction unconditionally
// (bypassing the dirty-tracking short-circuit in Sweep), so
// TestCompactionDeterministic can compare two compaction passes over
// identical logical content.
func (s *Store) forceCompact() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.compactLocked()
}

// TestCrashMidAppendRecovers simulates a process that crashed while
// writing the last line of the file (a partial, unterminated JSON
// fragment). New must load successfully, ignoring the corrupt trailing
// line, and retain every complete record before it.
func TestCrashMidAppendRecovers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.jsonl")

	now := time.Now().Truncate(time.Second).UTC()
	good := runner.Job{
		ID:         "good-job",
		Type:       "noop",
		Payload:    json.RawMessage(`{}`),
		State:      runner.JobStatePending,
		EnqueuedAt: now,
		NextRunAt:  now,
	}
	goodLine, err := marshalRecord(good)
	if err != nil {
		t.Fatalf("marshalRecord: %v", err)
	}

	var content bytes.Buffer
	content.Write(goodLine)
	content.WriteByte('\n')
	// A partial line: the crash happened mid-write, before the closing
	// brace and newline ever reached disk.
	content.WriteString(`{"id":"partial-job","type":"noop","payload":{"n":1},"stat`)

	if err := os.WriteFile(path, content.Bytes(), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	s, err := New(WithPath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	due, err := s.Due(now.Add(time.Second), 10)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 1 || due[0].ID != "good-job" {
		t.Fatalf("Due = %+v, want only [good-job] (the partial trailing record must be skipped)", due)
	}

	// The store must still be writable after recovering from a partial
	// trailing line — a new Append should succeed normally.
	if err := s.Append(runner.Job{ID: "new-job", Type: "noop", Payload: json.RawMessage(`{}`), State: runner.JobStatePending, EnqueuedAt: now, NextRunAt: now}); err != nil {
		t.Fatalf("Append after recovery: %v", err)
	}
}

func TestUpdateUnknownJobErrors(t *testing.T) {
	s, err := New(WithPath(filepath.Join(t.TempDir(), "queue.jsonl")))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	if err := s.Update(runner.Job{ID: "nope"}); err == nil {
		t.Fatal("Update on unknown ID: want error, got nil")
	}
}

func TestAppendDuplicateIDErrors(t *testing.T) {
	s, err := New(WithPath(filepath.Join(t.TempDir(), "queue.jsonl")))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	job := runner.Job{ID: "dup", Type: "t", Payload: json.RawMessage(`{}`), State: runner.JobStatePending}
	if err := s.Append(job); err != nil {
		t.Fatalf("first Append: %v", err)
	}
	if err := s.Append(job); err == nil {
		t.Fatal("second Append with same ID: want error, got nil")
	}
}

func TestReopenReplaysLatestStatePerJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.jsonl")
	now := time.Now().Truncate(time.Second).UTC()

	s, err := New(WithPath(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	job := runner.Job{ID: "job-1", Type: "t", Payload: json.RawMessage(`{}`), State: runner.JobStatePending, EnqueuedAt: now, NextRunAt: now}
	if err := s.Append(job); err != nil {
		t.Fatalf("Append: %v", err)
	}
	job.State = runner.JobStateDone
	job.NextRunAt = now
	if err := s.Update(job); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := New(WithPath(path))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	jobs, err := reopened.List(runner.JobStateDone)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "job-1" {
		t.Fatalf("List(done) after reopen = %+v, want [job-1] (latest state must survive a restart)", jobs)
	}

	pending, err := reopened.List(runner.JobStatePending)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("List(pending) after reopen = %+v, want none — the pending line is superseded by the done line", pending)
	}
}
