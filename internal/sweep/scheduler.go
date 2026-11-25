// SPDX-License-Identifier: AGPL-3.0-only

package sweep

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/drilonrecica/binnacle/internal/observe"
)

// Runner is what the scheduler runs for each due domain; *Prober in
// production, a fixed-answer prober in demo mode and tests.
type Runner interface {
	Probe(context.Context, Domain) Result
}

// Scheduler is the sweep's one bounded queue and fixed worker pool, the same
// shape as the HTTP check scheduler (ADR 015): a one-second tick loads the
// due domains, a full queue skips the rest until the next tick, and a domain
// that is queued or running is not offered again.
type Scheduler struct {
	Repo           *Repository
	Prober         Runner
	MaxConcurrency int
	Logger         *slog.Logger
	queue          chan Domain
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	mu             sync.Mutex
	active         map[string]struct{}
	now            func() time.Time
	skipped        atomic.Int64
	// failures counts scheduler errors since start: loading the due domains
	// or storing a result failed. A domain that is down is a result, not a
	// failure (RUN-7).
	failures   observe.Counter
	errlog     observe.ErrorLog
	errlogOnce sync.Once
}

func NewScheduler(repo *Repository, prober Runner, max int) *Scheduler {
	if max < 1 {
		max = 1
	}
	return &Scheduler{Repo: repo, Prober: prober, MaxConcurrency: max, Logger: slog.Default(), queue: make(chan Domain, max*2), now: time.Now, active: map[string]struct{}{}}
}

// Failures reports how many times loading due domains or storing a result
// failed since start.
func (s *Scheduler) Failures() uint64 { return s.failures.Load() }

// Skipped reports how many due domains were not queued because the worker
// queue was full. A skipped domain stays due and is offered again on the
// next tick.
func (s *Scheduler) Skipped() int64 { return s.skipped.Load() }

func (s *Scheduler) Name() string { return "domain-sweep" }

func (s *Scheduler) Start(ctx context.Context) error {
	if s.Repo == nil || s.Prober == nil {
		return errors.New("domain sweep dependencies unavailable")
	}
	ctx, s.cancel = context.WithCancel(ctx)
	for i := 0; i < s.MaxConcurrency; i++ {
		s.wg.Add(1)
		go s.worker(ctx)
	}
	s.wg.Add(1)
	go s.schedule(ctx)
	return nil
}

func (s *Scheduler) Stop(context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	return nil
}

func (s *Scheduler) schedule(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// tick queues the domains that are due and not already queued or running.
func (s *Scheduler) tick(ctx context.Context) {
	domains, err := s.Repo.Due(ctx, s.now().UTC(), cap(s.queue), s.reserved()...)
	if err != nil {
		if ctx.Err() == nil {
			s.failures.Inc()
			s.errorLog().Error("loading due domains failed", "error", err)
		}
		return
	}
	skipped := 0
	for _, d := range domains {
		if !s.reserve(d.ID) {
			continue
		}
		select {
		case s.queue <- d:
		case <-ctx.Done():
			s.release(d.ID)
			return
		default:
			s.release(d.ID)
			skipped++
		}
	}
	if skipped > 0 {
		total := s.skipped.Add(int64(skipped))
		s.errorLog().Warn("domain sweep queue full; due domains skipped until the next tick", "skipped", skipped, "total_skipped", total)
	}
}

// reserved returns the ids of domains currently queued or running, at most
// the number Repository.Due accepts.
func (s *Scheduler) reserved() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, min(len(s.active), 1000))
	for id := range s.active {
		if len(ids) == 1000 {
			break
		}
		ids = append(ids, id)
	}
	return ids
}

func (s *Scheduler) worker(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case d := <-s.queue:
			result := s.Prober.Probe(ctx, d).completed()
			if changes, err := s.Repo.SaveResult(ctx, d, result); err != nil {
				if ctx.Err() == nil {
					s.failures.Inc()
					s.errorLog().Error("saving domain sweep result failed", "domain", d.Domain, "error", err)
				}
			} else if len(changes) > 0 {
				s.logger().Info("domain changed", "domain", d.Domain, "changes", changes)
			}
			s.release(d.ID)
		}
	}
}

func (s *Scheduler) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

// errorLog is the scheduler's rate-limited log; it writes through logger().
func (s *Scheduler) errorLog() *observe.ErrorLog {
	s.errlogOnce.Do(func() { s.errlog.Logger = s.logger })
	return &s.errlog
}

// RunNow probes one enabled domain at once and stores the result.
func (s *Scheduler) RunNow(ctx context.Context, id string) (Result, error) {
	entry, err := s.Repo.Get(ctx, id)
	if err != nil {
		return Result{}, err
	}
	if !entry.Enabled {
		return Result{}, errors.New("domain is disabled")
	}
	if !s.reserve(entry.ID) {
		return Result{}, errors.New("domain is already being swept")
	}
	defer s.release(entry.ID)
	result := s.Prober.Probe(ctx, entry.Domain).completed()
	if _, err = s.Repo.SaveResult(ctx, entry.Domain, result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (s *Scheduler) reserve(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.active[id]; ok {
		return false
	}
	s.active[id] = struct{}{}
	return true
}
func (s *Scheduler) release(id string) { s.mu.Lock(); delete(s.active, id); s.mu.Unlock() }
