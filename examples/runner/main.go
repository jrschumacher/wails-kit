// Command runner-example demonstrates runner.Queue backed by
// runner/flatfile: a Durable profile, a handler that succeeds, one that
// fails once then succeeds (showing retry-with-backoff), and one that
// exhausts its attempts and lands in the dead-letter queue. It prints the
// resulting flat-file JSONL so its human-readability is visible directly,
// not just asserted in a test.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jrschumacher/wails-kit/v2/events"
	"github.com/jrschumacher/wails-kit/v2/runner"
	"github.com/jrschumacher/wails-kit/v2/runner/flatfile"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dir, err := os.MkdirTemp("", "runner-example")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	queuePath := filepath.Join(dir, "queue.jsonl")

	store, err := flatfile.New(flatfile.WithPath(queuePath))
	if err != nil {
		return fmt.Errorf("new flatfile store: %w", err)
	}

	mem := events.NewMemoryEmitter()
	emitter := events.NewEmitter(mem)

	// Durable(), with MaxAttempts overridden down to 2 and backoff zeroed
	// out — this is the "profiles, not knobs" pattern in action: pick a
	// profile, override the one or two fields you care about. A real app
	// would leave Backoff at its default (exponential, 1s..5m, jitter);
	// this example zeroes it purely so it finishes in a couple of ticks
	// instead of minutes.
	profile := runner.Durable()
	profile.MaxAttempts = 2
	profile.Backoff = func(int) time.Duration { return 0 }

	// WithWorkers(3): all three demo jobs are due on the same tick, and
	// the default of 1 worker would serialize them across several ticks.
	// A real desktop app usually keeps the default of 1.
	q, err := runner.New(profile, runner.WithStore(store), runner.WithEmitter(emitter), runner.WithWorkers(3))
	if err != nil {
		return fmt.Errorf("new queue: %w", err)
	}

	q.Handle("send_email", func(_ context.Context, job runner.Job) error {
		fmt.Printf("  send_email: sending payload %s\n", job.Payload)
		return nil
	})

	var attempts int
	q.Handle("flaky_sync", func(_ context.Context, job runner.Job) error {
		attempts++
		if attempts == 1 {
			fmt.Println("  flaky_sync: transient failure, will retry")
			return fmt.Errorf("connection reset")
		}
		fmt.Println("  flaky_sync: succeeded on retry")
		return nil
	})

	q.Handle("always_fails", func(context.Context, runner.Job) error {
		fmt.Println("  always_fails: failing (will exhaust attempts and dead-letter)")
		return fmt.Errorf("permanently broken")
	})

	ctx := context.Background()
	if _, err := q.Enqueue(ctx, "send_email", map[string]string{"to": "user@example.com"}); err != nil {
		return fmt.Errorf("enqueue send_email: %w", err)
	}
	if _, err := q.Enqueue(ctx, "flaky_sync", map[string]string{"repo": "wails-kit"}); err != nil {
		return fmt.Errorf("enqueue flaky_sync: %w", err)
	}
	if _, err := q.Enqueue(ctx, "always_fails", map[string]string{"reason": "demo"}); err != nil {
		return fmt.Errorf("enqueue always_fails: %w", err)
	}

	// Start ticks once a second; two ticks are enough for flaky_sync's
	// retry and always_fails's second (terminal) attempt to land. The
	// extra margin (4s) is just headroom against tick-boundary timing.
	runCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	fmt.Println("\nstarting queue...")
	if err := q.Start(runCtx); err != nil {
		return fmt.Errorf("start queue: %w", err)
	}
	if err := q.Close(); err != nil {
		return fmt.Errorf("close queue: %w", err)
	}

	fmt.Println("\nevents observed:")
	for _, e := range mem.Events() {
		fmt.Printf("  %s\n", e.Name)
	}

	dead, err := q.Dead()
	if err != nil {
		return fmt.Errorf("dead: %w", err)
	}
	fmt.Println("\ndead-lettered jobs:")
	for _, j := range dead {
		fmt.Printf("  %s (%s): %s\n", j.ID, j.Type, j.LastError)
	}

	data, err := os.ReadFile(queuePath)
	if err != nil {
		return fmt.Errorf("read queue file: %w", err)
	}
	fmt.Println("\nqueue.jsonl (note: real field names, inline JSON payload, no base64):")
	fmt.Print(string(data))

	return nil
}
