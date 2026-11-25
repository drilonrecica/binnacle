// SPDX-License-Identifier: AGPL-3.0-only
package sweep

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type gatedProber struct {
	mu   sync.Mutex
	ran  map[string]int
	gate chan struct{}
}

func (p *gatedProber) Probe(ctx context.Context, d Domain) Result {
	select {
	case <-p.gate:
	case <-ctx.Done():
	}
	p.mu.Lock()
	p.ran[d.ID]++
	p.mu.Unlock()
	return healthyResult(d.ID, time.Now().UTC())
}

func (p *gatedProber) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.ran)
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestSchedulerDeduplicatesRunningDomain(t *testing.T) {
	s := NewScheduler(nil, nil, 1)
	if !s.reserve("d") || s.reserve("d") {
		t.Fatal("duplicate reservation succeeded")
	}
	s.release("d")
	if !s.reserve("d") {
		t.Fatal("reservation was not released")
	}
}

func TestSchedulerQueueIsBounded(t *testing.T) {
	s := NewScheduler(nil, &gatedProber{}, 3)
	if got, want := cap(s.queue), 6; got != want {
		t.Fatalf("queue capacity=%d want %d", got, want)
	}
	if s.Name() != "domain-sweep" {
		t.Fatalf("name=%q", s.Name())
	}
}

func TestSchedulerStartNeedsDependencies(t *testing.T) {
	if err := NewScheduler(nil, nil, 1).Start(context.Background()); err == nil {
		t.Fatal("scheduler started without a repository and prober")
	}
}

// A full queue skips due domains for one tick and counts them; it does not
// end the scheduler (EVT-4).
func TestSchedulerKeepsSchedulingAfterQueueIsFull(t *testing.T) {
	ids := []string{"a.example", "b.example", "c.example", "d.example", "e.example", "f.example"}
	_, repo := sweepRepository(t, ids...)
	prober := &gatedProber{ran: map[string]int{}, gate: make(chan struct{})}
	s := NewScheduler(repo, prober, 1) // one worker, queue of two
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
			t.Fatal("full queue never skipped a due domain")
		}
		time.Sleep(20 * time.Millisecond)
	}
	close(prober.gate)
	for prober.count() < len(ids) {
		if time.Now().After(deadline) {
			t.Fatalf("ran %d of %d due domains after the queue was full; skipped=%d", prober.count(), len(ids), s.Skipped())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSchedulerExcludesReservedDomainsFromDue(t *testing.T) {
	_, repo := sweepRepository(t, "a.example", "b.example", "c.example")
	ctx := context.Background()
	s := NewScheduler(repo, &gatedProber{ran: map[string]int{}, gate: make(chan struct{})}, 1)
	s.Logger = quietLogger()
	s.reserve("id-a.example")
	s.reserve("id-b.example")
	s.tick(ctx)
	if len(s.queue) != 1 {
		t.Fatalf("queued %d domains, want 1", len(s.queue))
	}
	if d := <-s.queue; d.ID != "id-c.example" {
		t.Fatalf("queued %s, want c", d.ID)
	}
	if s.Skipped() != 0 {
		t.Fatalf("skipped=%d", s.Skipped())
	}
}

// A scheduler that cannot load its due domains counts every failure and
// logs it once per interval, not once per tick (RUN-7).
func TestSchedulerCountsAndRateLimitsFailures(t *testing.T) {
	ctx := context.Background()
	store, repo := sweepRepository(t)
	if _, err := store.Writer().Exec("DROP TABLE sweep_domains"); err != nil {
		t.Fatal(err)
	}
	var logs strings.Builder
	s := NewScheduler(repo, &gatedProber{ran: map[string]int{}, gate: make(chan struct{})}, 1)
	s.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	for range 3 {
		s.tick(ctx)
	}
	if s.Failures() != 3 {
		t.Fatalf("failures=%d, want 3", s.Failures())
	}
	if n := strings.Count(logs.String(), "loading due domains failed"); n != 1 {
		t.Fatalf("logged %d times, want 1:\n%s", n, logs.String())
	}
}

func TestSchedulerRunNow(t *testing.T) {
	ctx := context.Background()
	_, repo := sweepRepository(t, "a.example", "b.example")
	prober := &gatedProber{ran: map[string]int{}, gate: make(chan struct{})}
	close(prober.gate)
	s := NewScheduler(repo, prober, 1)
	s.Logger = quietLogger()
	result, err := s.RunNow(ctx, "id-a.example")
	if err != nil || result.HTTPStatus != 200 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	entry, err := repo.Get(ctx, "id-a.example")
	if err != nil || entry.State == nil || entry.State.HTTPStatus != 200 {
		t.Fatalf("state after RunNow=%+v err=%v", entry.State, err)
	}
	disabled := entry.Domain
	disabled.Enabled = false
	if err = repo.Update(ctx, disabled); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RunNow(ctx, "id-a.example"); err == nil {
		t.Fatal("disabled domain ran")
	}
	if _, err = s.RunNow(ctx, "missing"); err == nil {
		t.Fatal("missing domain ran")
	}
	s.reserve("id-b.example")
	if _, err = s.RunNow(ctx, "id-b.example"); err == nil {
		t.Fatal("domain already being swept ran twice")
	}
}
