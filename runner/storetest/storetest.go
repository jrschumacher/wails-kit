// Package storetest is a shared contract-test suite for runner.Store
// implementations. runner's own in-memory store and runner/flatfile both
// run it; runner/sqlitestore (WP-41) is expected to run it verbatim too —
// see docs/v2-roadmap.md WP-40/WP-41.
package storetest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jrschumacher/wails-kit/v2/runner"
)

// Run exercises the runner.Store contract against a fresh store returned
// by newStore for each subtest. newStore must return an empty store every
// call (e.g. backed by t.TempDir() for a file-based store).
func Run(t *testing.T, newStore func(t *testing.T) runner.Store) {
	t.Helper()

	t.Run("DueOrderingIsDeterministic", func(t *testing.T) {
		// runner.go's Store contract requires "earliest NextRunAt, then
		// EnqueuedAt, then ID". The suite previously did not check ordering
		// at all, and the two shipped stores diverged as a result: flatfile
		// sorted by EnqueuedAt then ID and ignored NextRunAt entirely, while
		// the memory store followed the contract. Two stores that each pass
		// their own tests and disagree on semantics is precisely what this
		// package exists to prevent, so the gap is closed here rather than
		// in any one store's tests.
		s := newStore(t)
		defer func() { _ = s.Close() }()

		base := time.Now().Truncate(time.Second)
		// Deliberately appended in an order matching none of the sort keys.
		jobs := []runner.Job{
			// Latest NextRunAt, earliest EnqueuedAt — must sort last.
			{ID: "c", Type: "t", State: runner.JobStatePending,
				EnqueuedAt: base, NextRunAt: base.Add(2 * time.Second)},
			// Same NextRunAt as "b", later EnqueuedAt — tie broken by EnqueuedAt.
			{ID: "a", Type: "t", State: runner.JobStatePending,
				EnqueuedAt: base.Add(2 * time.Second), NextRunAt: base},
			{ID: "b", Type: "t", State: runner.JobStatePending,
				EnqueuedAt: base.Add(time.Second), NextRunAt: base},
		}
		for _, j := range jobs {
			if err := s.Append(j); err != nil {
				t.Fatalf("Append(%s): %v", j.ID, err)
			}
		}

		due, err := s.Due(base.Add(time.Hour), 10)
		if err != nil {
			t.Fatalf("Due: %v", err)
		}
		got := make([]string, len(due))
		for i, j := range due {
			got[i] = j.ID
		}
		// b and a share NextRunAt; b enqueued earlier so it precedes a.
		// c has the latest NextRunAt despite the earliest EnqueuedAt.
		want := []string{"b", "a", "c"}
		if len(got) != len(want) {
			t.Fatalf("Due returned %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("Due order = %v, want %v (contract: NextRunAt, then EnqueuedAt, then ID)", got, want)
			}
		}
	})

	t.Run("AppendThenDue", func(t *testing.T) {
		s := newStore(t)
		defer func() { _ = s.Close() }()

		now := time.Now()
		job := runner.Job{
			ID:         "job-1",
			Type:       "noop",
			Payload:    json.RawMessage(`{"n":1}`),
			State:      runner.JobStatePending,
			EnqueuedAt: now,
			NextRunAt:  now,
		}
		if err := s.Append(job); err != nil {
			t.Fatalf("Append: %v", err)
		}

		due, err := s.Due(now.Add(time.Second), 10)
		if err != nil {
			t.Fatalf("Due: %v", err)
		}
		if len(due) != 1 || due[0].ID != "job-1" {
			t.Fatalf("Due = %+v, want [job-1]", due)
		}
	})

	t.Run("DueExcludesFutureJobs", func(t *testing.T) {
		s := newStore(t)
		defer func() { _ = s.Close() }()

		now := time.Now()
		future := runner.Job{
			ID:         "future",
			Type:       "noop",
			Payload:    json.RawMessage(`{}`),
			State:      runner.JobStatePending,
			EnqueuedAt: now,
			NextRunAt:  now.Add(time.Hour),
		}
		if err := s.Append(future); err != nil {
			t.Fatalf("Append: %v", err)
		}

		due, err := s.Due(now, 10)
		if err != nil {
			t.Fatalf("Due: %v", err)
		}
		if len(due) != 0 {
			t.Fatalf("Due = %+v, want none (job is not due yet)", due)
		}
	})

	t.Run("DueIncludesRunningRegardlessOfNextRunAt", func(t *testing.T) {
		s := newStore(t)
		defer func() { _ = s.Close() }()

		now := time.Now()
		running := runner.Job{
			ID:         "orphaned",
			Type:       "noop",
			Payload:    json.RawMessage(`{}`),
			State:      runner.JobStateRunning,
			EnqueuedAt: now,
			NextRunAt:  now.Add(time.Hour), // irrelevant for a running job
		}
		if err := s.Append(running); err != nil {
			t.Fatalf("Append: %v", err)
		}

		due, err := s.Due(now, 10)
		if err != nil {
			t.Fatalf("Due: %v", err)
		}
		if len(due) != 1 || due[0].ID != "orphaned" {
			t.Fatalf("Due = %+v, want [orphaned] (running jobs are always due — crash recovery)", due)
		}
	})

	t.Run("UpdateChangesState", func(t *testing.T) {
		s := newStore(t)
		defer func() { _ = s.Close() }()

		now := time.Now()
		job := runner.Job{
			ID:         "job-1",
			Type:       "noop",
			Payload:    json.RawMessage(`{}`),
			State:      runner.JobStatePending,
			EnqueuedAt: now,
			NextRunAt:  now,
		}
		if err := s.Append(job); err != nil {
			t.Fatalf("Append: %v", err)
		}

		job.State = runner.JobStateDone
		job.NextRunAt = now
		if err := s.Update(job); err != nil {
			t.Fatalf("Update: %v", err)
		}

		due, err := s.Due(now.Add(time.Second), 10)
		if err != nil {
			t.Fatalf("Due: %v", err)
		}
		if len(due) != 0 {
			t.Fatalf("Due = %+v, want none (job is done)", due)
		}
	})

	t.Run("UpdateUnknownJobErrors", func(t *testing.T) {
		s := newStore(t)
		defer func() { _ = s.Close() }()

		err := s.Update(runner.Job{ID: "does-not-exist", State: runner.JobStateDone})
		if err == nil {
			t.Fatal("Update on an unknown job ID: want error, got nil")
		}
	})

	t.Run("DueRespectsLimit", func(t *testing.T) {
		s := newStore(t)
		defer func() { _ = s.Close() }()

		now := time.Now()
		for i := 0; i < 5; i++ {
			job := runner.Job{
				ID:         "job-" + string(rune('a'+i)),
				Type:       "noop",
				Payload:    json.RawMessage(`{}`),
				State:      runner.JobStatePending,
				EnqueuedAt: now,
				NextRunAt:  now,
			}
			if err := s.Append(job); err != nil {
				t.Fatalf("Append: %v", err)
			}
		}

		due, err := s.Due(now, 2)
		if err != nil {
			t.Fatalf("Due: %v", err)
		}
		if len(due) != 2 {
			t.Fatalf("Due returned %d jobs, want 2 (limit)", len(due))
		}
	})

	t.Run("SweepRemovesOldTerminalJobs", func(t *testing.T) {
		s := newStore(t)
		defer func() { _ = s.Close() }()

		now := time.Now()
		done := runner.Job{
			ID:         "done-job",
			Type:       "noop",
			Payload:    json.RawMessage(`{}`),
			State:      runner.JobStateDone,
			EnqueuedAt: now.Add(-time.Hour),
			NextRunAt:  now.Add(-time.Hour), // completion time, repurposed field
		}
		if err := s.Append(done); err != nil {
			t.Fatalf("Append: %v", err)
		}

		if err := s.Sweep(time.Minute, now); err != nil {
			t.Fatalf("Sweep: %v", err)
		}

		if lister, ok := s.(runner.Lister); ok {
			jobs, err := lister.List(runner.JobStateDone)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(jobs) != 0 {
				t.Fatalf("List(done) after Sweep = %+v, want none", jobs)
			}
		}
	})

	t.Run("SweepPreservesDeadJobs", func(t *testing.T) {
		s := newStore(t)
		defer func() { _ = s.Close() }()

		now := time.Now()
		dead := runner.Job{
			ID:         "dead-job",
			Type:       "noop",
			Payload:    json.RawMessage(`{}`),
			State:      runner.JobStateDead,
			EnqueuedAt: now.Add(-24 * time.Hour),
			NextRunAt:  now.Add(-24 * time.Hour),
		}
		if err := s.Append(dead); err != nil {
			t.Fatalf("Append: %v", err)
		}

		if err := s.Sweep(0, now); err != nil {
			t.Fatalf("Sweep: %v", err)
		}

		lister, ok := s.(runner.Lister)
		if !ok {
			t.Skip("store does not implement Lister")
		}
		jobs, err := lister.List(runner.JobStateDead)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(jobs) != 1 {
			t.Fatalf("List(dead) after Sweep = %+v, want the dead job preserved", jobs)
		}
	})

	t.Run("DeleteRemovesJob", func(t *testing.T) {
		// M2: Deleter is what makes runner.Queue.Discard able to actually
		// purge a dead-lettered job's record — without it, a dead job on
		// this Store would be stuck forever (Sweep never touches
		// JobStateDead, and there was no other removal path at all).
		s := newStore(t)
		defer func() { _ = s.Close() }()

		deleter, ok := s.(runner.Deleter)
		if !ok {
			t.Skip("store does not implement Deleter")
		}

		now := time.Now()
		dead := runner.Job{
			ID:         "dead-job",
			Type:       "noop",
			Payload:    json.RawMessage(`{}`),
			State:      runner.JobStateDead,
			EnqueuedAt: now,
			NextRunAt:  now,
		}
		if err := s.Append(dead); err != nil {
			t.Fatalf("Append: %v", err)
		}

		if err := deleter.Delete("dead-job"); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		if lister, ok := s.(runner.Lister); ok {
			jobs, err := lister.List(runner.JobStateDead)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(jobs) != 0 {
				t.Fatalf("List(dead) after Delete = %+v, want none", jobs)
			}
		}

		if err := deleter.Delete("does-not-exist"); err == nil {
			t.Fatal("Delete on an unknown ID: want error, got nil")
		}
	})

	t.Run("PayloadRoundTrips", func(t *testing.T) {
		s := newStore(t)
		defer func() { _ = s.Close() }()

		now := time.Now()
		payload := json.RawMessage(`{"greeting":"hello","n":42}`)
		job := runner.Job{
			ID:         "job-1",
			Type:       "noop",
			Payload:    payload,
			State:      runner.JobStatePending,
			EnqueuedAt: now,
			NextRunAt:  now,
		}
		if err := s.Append(job); err != nil {
			t.Fatalf("Append: %v", err)
		}

		due, err := s.Due(now.Add(time.Second), 10)
		if err != nil {
			t.Fatalf("Due: %v", err)
		}
		if len(due) != 1 {
			t.Fatalf("Due = %+v, want 1 job", due)
		}
		var got map[string]any
		if err := json.Unmarshal(due[0].Payload, &got); err != nil {
			t.Fatalf("payload did not round-trip as JSON: %v", err)
		}
		if got["greeting"] != "hello" {
			t.Fatalf("payload = %v, want greeting=hello", got)
		}
	})

	t.Run("EmptyPayloadNormalizesToNull", func(t *testing.T) {
		// Pins the M5 divergence: an empty (non-nil, zero-length)
		// json.RawMessage payload was accepted verbatim by the in-memory
		// store, caused a marshal error in runner/flatfile
		// ("unexpected end of JSON input" — json.RawMessage{}'s own
		// MarshalJSON returns zero bytes, which isn't valid JSON on its
		// own), and normalized to the JSON literal null in sqlitestore.
		// Every Store must now agree: empty/nil payload round-trips as
		// the JSON literal null.
		s := newStore(t)
		defer func() { _ = s.Close() }()

		now := time.Now()
		job := runner.Job{
			ID:         "empty-payload",
			Type:       "noop",
			Payload:    json.RawMessage{},
			State:      runner.JobStatePending,
			EnqueuedAt: now,
			NextRunAt:  now,
		}
		if err := s.Append(job); err != nil {
			t.Fatalf("Append with empty payload: %v", err)
		}

		due, err := s.Due(now.Add(time.Second), 10)
		if err != nil {
			t.Fatalf("Due: %v", err)
		}
		if len(due) != 1 {
			t.Fatalf("Due = %+v, want 1 job", due)
		}
		if got := string(due[0].Payload); got != "null" {
			t.Fatalf("empty payload round-tripped as %q, want the JSON literal null", got)
		}

		// Update with a nil payload must normalize the same way.
		job.Payload = nil
		job.State = runner.JobStateDone
		job.NextRunAt = now
		if err := s.Update(job); err != nil {
			t.Fatalf("Update with nil payload: %v", err)
		}
		if lister, ok := s.(runner.Lister); ok {
			done, err := lister.List(runner.JobStateDone)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(done) != 1 || string(done[0].Payload) != "null" {
				t.Fatalf("List(done) = %+v, want payload normalized to null", done)
			}
		}
	})
}
