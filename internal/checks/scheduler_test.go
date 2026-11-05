// SPDX-License-Identifier: AGPL-3.0-only
package checks

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/storage"
)

type blockingRunner struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (r *blockingRunner) Run(ctx context.Context, c Check) Result {
	r.calls.Add(1)
	select {
	case r.started <- struct{}{}:
	default:
		{
		}
	}
	select {
	case <-r.release:
	case <-ctx.Done():
	}
	return Result{CheckID: c.ID, Status: "success", CheckedAt: time.Now().UTC()}
}
func TestSchedulerDeduplicatesRunningCheck(t *testing.T) {
	runner := &blockingRunner{started: make(chan struct{}, 1), release: make(chan struct{})}
	s := NewScheduler(nil, nil, 1)
	s.Runner = runner
	if !s.reserve("check") {
		t.Fatal("first reservation failed")
	}
	if s.reserve("check") {
		t.Fatal("duplicate reservation succeeded")
	}
	s.release("check")
	if !s.reserve("check") {
		t.Fatal("reservation was not released")
	}
}
func TestSchedulerQueueIsBounded(t *testing.T) {
	s := NewScheduler(nil, &blockingRunner{}, 3)
	if got, want := cap(s.queue), 6; got != want {
		t.Fatalf("queue capacity=%d want %d", got, want)
	}
}

func schedulerRepository(t *testing.T, ids ...string) *Repository {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	store := storage.New(filepath.Join(dir, "db.sqlite"), filepath.Join(dir, "runtime"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db := store.DB()
	now := time.Now().Unix()
	if _, err := db.Exec(`INSERT INTO hosts(id,identity_hash,name,updated_at)VALUES('host','host','host',?)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO resources(id,host_id,stable_key,source_kind,name,category,status,first_seen_at,last_seen_at)VALUES('res','host','res','docker','res','application','healthy',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	for _, id := range ids {
		c := Check{ID: id, ResourceID: "res", Name: id, URL: "https://example.test/" + id, Method: "GET", Interval: time.Hour, Timeout: 5 * time.Second, ExpectedStatusMin: 200, ExpectedStatusMax: 299, Required: true, Enabled: true}
		if err := repo.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

type gatedRunner struct {
	mu   sync.Mutex
	ran  map[string]int
	gate chan struct{}
}

func (r *gatedRunner) Run(ctx context.Context, c Check) Result {
	select {
	case <-r.gate:
	case <-ctx.Done():
	}
	r.mu.Lock()
	r.ran[c.ID]++
	r.mu.Unlock()
	return Result{CheckID: c.ID, Status: "success", CheckedAt: time.Now().UTC()}
}

func (r *gatedRunner) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.ran)
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// EVT-4: a full queue skips due checks for one tick and counts them; it does
// not end the scheduler. Before the fix the schedule goroutine returned on
// the first full queue and the remaining checks never ran.
func TestSchedulerKeepsSchedulingAfterQueueIsFull(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e", "f"}
	repo := schedulerRepository(t, ids...)
	runner := &gatedRunner{ran: map[string]int{}, gate: make(chan struct{})}
	s := NewScheduler(repo, runner, 1) // one worker, queue of two
	s.Logger = quietLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(context.Background())
	deadline := time.Now().Add(10 * time.Second)
	for s.Skipped() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("full queue never skipped a due check")
		}
		time.Sleep(20 * time.Millisecond)
	}
	close(runner.gate)
	for runner.count() < len(ids) {
		if time.Now().After(deadline) {
			t.Fatalf("ran %d of %d due checks after the queue was full; skipped=%d", runner.count(), len(ids), s.Skipped())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// EVT-4: checks that are queued or running keep a past next_run_at until
// their result is saved; Due must not return them, or they take every slot
// of the bounded query and starve other due checks.
func TestSchedulerExcludesReservedChecksFromDue(t *testing.T) {
	repo := schedulerRepository(t, "a", "b", "c")
	ctx := context.Background()
	due, err := repo.Due(ctx, time.Now().UTC(), 2, "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != "c" {
		t.Fatalf("due=%v want only c", due)
	}

	s := NewScheduler(repo, &gatedRunner{ran: map[string]int{}, gate: make(chan struct{})}, 1)
	s.Logger = quietLogger()
	s.reserve("a")
	s.reserve("b")
	s.tick(ctx)
	if len(s.queue) != 1 {
		t.Fatalf("queued %d checks, want 1", len(s.queue))
	}
	if c := <-s.queue; c.ID != "c" {
		t.Fatalf("queued %s, want c", c.ID)
	}
	if s.Skipped() != 0 {
		t.Fatalf("skipped=%d", s.Skipped())
	}
}

// EVT-4: every due check that does not fit is counted, and the queue is
// filled on the next tick.
func TestSchedulerCountsSkippedChecks(t *testing.T) {
	repo := schedulerRepository(t, "a", "b", "c", "d")
	ctx := context.Background()
	s := NewScheduler(repo, &gatedRunner{ran: map[string]int{}, gate: make(chan struct{})}, 1)
	s.Logger = quietLogger()
	s.queue <- Check{ID: "other"}
	s.tick(ctx) // Due returns a,b; one fits
	if s.Skipped() != 1 || len(s.queue) != 2 {
		t.Fatalf("skipped=%d queued=%d want 1/2", s.Skipped(), len(s.queue))
	}
	<-s.queue
	<-s.queue
	s.release("other")
	s.tick(ctx) // a is still reserved; b and c fit
	if s.Skipped() != 1 || len(s.queue) != 2 {
		t.Fatalf("skipped=%d queued=%d want 1/2", s.Skipped(), len(s.queue))
	}
	for range 2 {
		if c := <-s.queue; c.ID == "a" {
			t.Fatal("reserved check queued twice")
		}
	}
}
