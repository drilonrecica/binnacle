// SPDX-License-Identifier: AGPL-3.0-only

package checks

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type Scheduler struct {
	Repo   *Repository
	Runner interface {
		Run(context.Context, Check) Result
	}
	MaxConcurrency int
	Logger         *slog.Logger
	queue          chan Check
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	mu             sync.Mutex
	active         map[string]struct{}
	now            func() time.Time
	skipped        atomic.Int64
}

func NewScheduler(repo *Repository, runner interface {
	Run(context.Context, Check) Result
}, max int) *Scheduler {
	if max < 1 {
		max = 1
	}
	return &Scheduler{Repo: repo, Runner: runner, MaxConcurrency: max, Logger: slog.Default(), queue: make(chan Check, max*2), now: time.Now, active: map[string]struct{}{}}
}

// Skipped reports how many due checks were not queued because the worker
// queue was full. A skipped check stays due and is offered again on the next
// tick (EVT-4).
func (s *Scheduler) Skipped() int64 { return s.skipped.Load() }

func (s *Scheduler) Start(ctx context.Context) error {
	if s.Repo == nil || s.Runner == nil {
		return errors.New("checks scheduler dependencies unavailable")
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

// tick queues the checks that are due and not already queued or running. A
// full queue skips the remaining checks for this tick only; the scheduler
// keeps running and counts every skip (EVT-4).
func (s *Scheduler) tick(ctx context.Context) {
	checks, err := s.Repo.Due(ctx, s.now().UTC(), cap(s.queue), s.reserved()...)
	if err != nil {
		if ctx.Err() == nil {
			s.logger().Error("loading due checks failed", "error", err)
		}
		return
	}
	skipped := 0
	for _, c := range checks {
		if !s.reserve(c.ID) {
			continue
		}
		select {
		case s.queue <- c:
		case <-ctx.Done():
			s.release(c.ID)
			return
		default:
			s.release(c.ID)
			skipped++
		}
	}
	if skipped > 0 {
		total := s.skipped.Add(int64(skipped))
		s.logger().Warn("check queue full; due checks skipped until the next tick", "skipped", skipped, "total_skipped", total)
	}
}

// reserved returns the ids of checks currently queued or running, at most
// the number Repository.Due accepts; any beyond that are still rejected by
// reserve.
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
		case c := <-s.queue:
			result := s.Runner.Run(ctx, c)
			if err := s.Repo.SaveResult(ctx, c, result); err != nil && ctx.Err() == nil {
				s.logger().Error("saving check result failed", "check", c.ID, "error", err)
			}
			s.release(c.ID)
		}
	}
}

func (s *Scheduler) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func (s *Scheduler) RunNow(ctx context.Context, id string) (Result, error) {
	c, err := s.Repo.Get(ctx, id)
	if err != nil {
		return Result{}, err
	}
	if !c.Enabled {
		return Result{}, errors.New("check is disabled")
	}
	if !s.reserve(c.ID) {
		return Result{}, errors.New("check is already running")
	}
	defer s.release(c.ID)
	result := s.Runner.Run(ctx, c)
	if err = s.Repo.SaveResult(ctx, c, result); err != nil {
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
