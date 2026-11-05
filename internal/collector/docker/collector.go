// SPDX-License-Identifier: AGPL-3.0-only
package docker

import (
	"context"
	"sync"
	"time"
)

// Bounds of one Docker tick's worker pool (ADR 022 §4). They are constants so
// the bounds cannot be configured away; the concurrency comes from
// Docker.MaxConcurrency and falls back to DefaultConcurrency.
const (
	// CallTimeout bounds every single Docker call made through the pool.
	CallTimeout = 5 * time.Second
	// DefaultConcurrency is used when no positive concurrency is given.
	DefaultConcurrency = 4
)

// Result is the outcome of one pooled call.
type Result[T any] struct {
	Value T
	Err   error
}

// Collect runs call once per id with at most concurrency calls in flight.
// Every call gets its own timeout derived from ctx. Every id has an entry in
// the returned map: ids that were not reached before ctx ended carry ctx's
// error, so a tick deadline turns the remaining containers into per-container
// failures rather than failing the tick (ADR 022 §3, §4).
func Collect[T any](ctx context.Context, ids []string, concurrency int, timeout time.Duration, call func(context.Context, string) (T, error)) map[string]Result[T] {
	if concurrency < 1 {
		concurrency = DefaultConcurrency
	}
	if timeout <= 0 {
		timeout = CallTimeout
	}
	results := make(map[string]Result[T], len(ids))
	if len(ids) == 0 {
		return results
	}
	var mu sync.Mutex
	jobs := make(chan string)
	var workers sync.WaitGroup
	for range min(concurrency, len(ids)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for id := range jobs {
				request, cancel := context.WithTimeout(ctx, timeout)
				value, err := call(request, id)
				cancel()
				mu.Lock()
				results[id] = Result[T]{Value: value, Err: err}
				mu.Unlock()
			}
		}()
	}
dispatch:
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		select {
		case jobs <- id:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	for _, id := range ids {
		if _, ok := results[id]; !ok {
			results[id] = Result[T]{Err: ctx.Err()}
		}
	}
	return results
}
