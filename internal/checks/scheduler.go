// SPDX-License-Identifier: AGPL-3.0-only

package checks

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Scheduler struct {
	Repo   *Repository
	Runner interface {
		Run(context.Context, Check) Result
	}
	MaxConcurrency int
	queue          chan Check
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	mu             sync.Mutex
	active         map[string]struct{}
	now            func() time.Time
}

func NewScheduler(repo *Repository, runner interface {
	Run(context.Context, Check) Result
}, max int) *Scheduler {
	if max < 1 {
		max = 1
	}
	return &Scheduler{Repo: repo, Runner: runner, MaxConcurrency: max, queue: make(chan Check, max*2), now: time.Now, active: map[string]struct{}{}}
}
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
			checks, err := s.Repo.Due(ctx, s.now().UTC(), cap(s.queue))
			if err != nil {
				continue
			}
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
					return
				}
			}
		}
	}
}
func (s *Scheduler) worker(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case c := <-s.queue:
			result := s.Runner.Run(ctx, c)
			_ = s.Repo.SaveResult(ctx, c, result)
			s.release(c.ID)
		}
	}
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
