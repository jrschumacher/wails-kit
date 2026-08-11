package runner

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// memoryStore is the built-in Store used automatically for PersistNone
// profiles when no WithStore option is supplied. It implements Lister too,
// so PersistNone queues get full MaxQueueDepth/idempotency/Dead() fidelity
// for the lifetime of the process (which is the entire lifetime of an
// in-memory store anyway — nothing survives a restart by design).
type memoryStore struct {
	mu   sync.Mutex
	jobs map[string]Job
}

func newMemoryStore() *memoryStore {
	return &memoryStore{jobs: make(map[string]Job)}
}

func (s *memoryStore) Append(job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; exists {
		return fmt.Errorf("runner: job %q already exists", job.ID)
	}
	s.jobs[job.ID] = job
	return nil
}

func (s *memoryStore) Update(job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; !exists {
		return fmt.Errorf("runner: job %q not found", job.ID)
	}
	s.jobs[job.ID] = job
	return nil
}

func (s *memoryStore) Due(now time.Time, limit int) ([]Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var due []Job
	for _, j := range s.jobs {
		if j.State == JobStateRunning {
			due = append(due, j)
			continue
		}
		if j.State == JobStatePending && !j.NextRunAt.After(now) {
			due = append(due, j)
		}
	}
	sortJobs(due)
	if limit > 0 && len(due) > limit {
		due = due[:limit]
	}
	return due, nil
}

func (s *memoryStore) Sweep(retention time.Duration, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := now.Add(-retention)
	for id, j := range s.jobs {
		if j.State != JobStateDone && j.State != JobStateFailed {
			continue
		}
		if !j.NextRunAt.After(cutoff) {
			delete(s.jobs, id)
		}
	}
	return nil
}

func (s *memoryStore) Close() error { return nil }

func (s *memoryStore) List(state JobState) ([]Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []Job
	for _, j := range s.jobs {
		if j.State == state {
			out = append(out, j)
		}
	}
	sortJobs(out)
	return out, nil
}

// sortJobs orders deterministically: NextRunAt, then EnqueuedAt, then ID.
// Deterministic ordering keeps tests (and, for flatfile, `git diff`)
// stable — see the Store.Due doc comment.
func sortJobs(jobs []Job) {
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
