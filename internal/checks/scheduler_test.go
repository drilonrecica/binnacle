// SPDX-License-Identifier: AGPL-3.0-only
package checks

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
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
