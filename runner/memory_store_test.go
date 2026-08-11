package runner

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMemoryStoreAppendAndDue(t *testing.T) {
	s := newMemoryStore()
	defer func() { _ = s.Close() }()

	now := time.Now()
	job := Job{ID: "a", Type: "t", Payload: json.RawMessage(`{}`), State: JobStatePending, EnqueuedAt: now, NextRunAt: now}
	if err := s.Append(job); err != nil {
		t.Fatalf("Append: %v", err)
	}

	due, err := s.Due(now, 10)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 1 || due[0].ID != "a" {
		t.Fatalf("Due = %+v, want [a]", due)
	}
}

func TestMemoryStoreAppendDuplicateIDErrors(t *testing.T) {
	s := newMemoryStore()
	defer func() { _ = s.Close() }()

	job := Job{ID: "a", State: JobStatePending}
	if err := s.Append(job); err != nil {
		t.Fatalf("first Append: %v", err)
	}
	if err := s.Append(job); err == nil {
		t.Fatal("second Append with same ID: want error, got nil")
	}
}

func TestMemoryStoreUpdateUnknownErrors(t *testing.T) {
	s := newMemoryStore()
	defer func() { _ = s.Close() }()

	if err := s.Update(Job{ID: "nope"}); err == nil {
		t.Fatal("Update on unknown ID: want error, got nil")
	}
}

func TestMemoryStoreDueIncludesRunning(t *testing.T) {
	s := newMemoryStore()
	defer func() { _ = s.Close() }()

	now := time.Now()
	job := Job{ID: "a", State: JobStateRunning, EnqueuedAt: now, NextRunAt: now.Add(time.Hour)}
	if err := s.Append(job); err != nil {
		t.Fatalf("Append: %v", err)
	}

	due, err := s.Due(now, 10)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("Due = %+v, want the running job regardless of NextRunAt", due)
	}
}

func TestMemoryStoreSweepRemovesOldDoneAndFailed(t *testing.T) {
	s := newMemoryStore()
	defer func() { _ = s.Close() }()

	now := time.Now()
	old := now.Add(-time.Hour)
	if err := s.Append(Job{ID: "done", State: JobStateDone, NextRunAt: old}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Append(Job{ID: "failed", State: JobStateFailed, NextRunAt: old}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Append(Job{ID: "dead", State: JobStateDead, NextRunAt: old}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if err := s.Sweep(time.Minute, now); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	doneList, _ := s.List(JobStateDone)
	failedList, _ := s.List(JobStateFailed)
	deadList, _ := s.List(JobStateDead)
	if len(doneList) != 0 {
		t.Errorf("done jobs after Sweep = %+v, want none", doneList)
	}
	if len(failedList) != 0 {
		t.Errorf("failed jobs after Sweep = %+v, want none", failedList)
	}
	if len(deadList) != 1 {
		t.Errorf("dead jobs after Sweep = %+v, want preserved", deadList)
	}
}

func TestMemoryStoreListFiltersByState(t *testing.T) {
	s := newMemoryStore()
	defer func() { _ = s.Close() }()

	if err := s.Append(Job{ID: "p", State: JobStatePending}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Append(Job{ID: "r", State: JobStateRunning}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	pending, err := s.List(JobStatePending)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "p" {
		t.Fatalf("List(pending) = %+v, want [p]", pending)
	}
}
