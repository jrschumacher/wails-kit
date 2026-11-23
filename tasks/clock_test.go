package tasks

import (
	"sort"
	"sync"
	"time"
)

// fakeClock is a manual clock. Advance fires due timers in order; Suspend
// moves the wall clock without letting timers see the time pass, as a
// machine asleep does.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
	seq    int
}

type fakeTimer struct {
	c       *fakeClock
	at      time.Time
	f       func()
	seq     int
	stopped bool
	fired   bool
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 23, 14, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	t := &fakeTimer{c: c, at: c.now.Add(d), f: f, seq: c.seq}
	c.timers = append(c.timers, t)
	return t
}

func (t *fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	was := !t.stopped && !t.fired
	t.stopped = true
	return was
}

// Advance moves the clock forward by d, firing each due timer at its time.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	target := c.now.Add(d)
	for {
		var due []*fakeTimer
		for _, t := range c.timers {
			if !t.stopped && !t.fired && !t.at.After(target) {
				due = append(due, t)
			}
		}
		if len(due) == 0 {
			break
		}
		sort.Slice(due, func(i, j int) bool {
			if due[i].at.Equal(due[j].at) {
				return due[i].seq < due[j].seq
			}
			return due[i].at.Before(due[j].at)
		})
		t := due[0]
		t.fired = true
		c.now = t.at
		c.mu.Unlock()
		t.f()
		c.mu.Lock()
	}
	c.now = target
	c.mu.Unlock()
}

// Suspend moves the wall clock forward by d and pushes every pending timer
// back by d, as a suspended monotonic clock would.
func (c *fakeClock) Suspend(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for _, t := range c.timers {
		t.at = t.at.Add(d)
	}
}
