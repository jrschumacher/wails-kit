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
	s.jobs[job.ID] = normalizeEmptyPayload(job)
	return nil
}

func (s *memoryStore) Update(job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; !exists {
		return fmt.Errorf("runner: job %q not found", job.ID)
	}
	s.jobs[job.ID] = normalizeEmptyPayload(job)
	return nil
}

// normalizeEmptyPayload maps a nil or zero-length Payload to the JSON
// literal "null", matching runner/sqlitestore's existing normalization
// (see that package's Append/Update). Without this, an empty payload's
// on-disk/in-memory representation diverged by Store: accepted verbatim
// here, a marshal error in runner/flatfile (json.RawMessage{} is not valid
// JSON on its own), and "null" in sqlitestore — see storetest's
// EmptyPayloadNormalizesToNull, which is the shared-contract case that
// pins this now that all three Stores agree.
func normalizeEmptyPayload(job Job) Job {
	if len(job.Payload) == 0 {
		job.Payload = jsonNull
	}
	return job
}

var jsonNull = []byte("null")

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

// Delete implements Deleter — permanently removes id's record.
func (s *memoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[id]; !exists {
		return fmt.Errorf("runner: job %q not found", id)
	}
	delete(s.jobs, id)
	return nil
}

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
