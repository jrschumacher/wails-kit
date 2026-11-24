package tasks

import (
	"fmt"

	"github.com/jrschumacher/wails-kit/errors"
)

// Bindings is the part of the service the frontend may call. Wails v3 binds
// every exported method of a registered service, so register Bindings()
// rather than the Service, which also has the app's Add, Sleep, Wake and
// Close:
//
//	application.NewService(svc.Bindings())
type Bindings struct {
	svc *Service
}

// Bindings returns the frontend's view of the service.
func (s *Service) Bindings() *Bindings { return &Bindings{svc: s} }

// Tasks returns a snapshot of every task in the list.
func (b *Bindings) Tasks() []Snapshot { return b.svc.Tasks() }

// Get returns one task's snapshot, or a tasks_not_found error. (The
// Service's Get returns a bool instead, which Wails would send to
// JavaScript as an array.)
func (b *Bindings) Get(id string) (Snapshot, error) {
	snap, ok := b.svc.Get(id)
	if !ok {
		return Snapshot{}, errors.New(ErrNotFound, fmt.Sprintf("task %q is not in the list", id), nil)
	}
	return snap, nil
}

// Stop asks to stop a task.
func (b *Bindings) Stop(id string) error { return b.svc.Stop(id) }

// Pause asks to pause a running or waiting task.
func (b *Bindings) Pause(id string) error { return b.svc.Pause(id) }

// Resume asks to resume a paused task.
func (b *Bindings) Resume(id string) error { return b.svc.Resume(id) }

// Retry runs a failed task again, or a waiting one now.
func (b *Bindings) Retry(id string) error { return b.svc.Retry(id) }

// Dismiss removes a finished task and its parts.
func (b *Bindings) Dismiss(id string) error { return b.svc.Dismiss(id) }

// DismissFinished removes every finished task whose parts have finished.
func (b *Bindings) DismissFinished() int { return b.svc.DismissFinished() }
