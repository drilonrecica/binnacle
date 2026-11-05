// SPDX-License-Identifier: AGPL-3.0-only
package docker

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestCollectBoundsConcurrencyAndTimesOutEachCall(t *testing.T) {
	ids := make([]string, 10)
	for i := range ids {
		ids[i] = fmt.Sprint(i)
	}
	var inFlight, peak atomic.Int32
	results := Collect(context.Background(), ids, 3, 20*time.Millisecond, func(ctx context.Context, id string) (string, error) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		if id == "7" {
			<-ctx.Done()
			return "", ctx.Err()
		}
		time.Sleep(5 * time.Millisecond)
		return "ok " + id, nil
	})
	if peak.Load() > 3 {
		t.Fatalf("peak in flight=%d, want at most 3", peak.Load())
	}
	if len(results) != len(ids) {
		t.Fatalf("results=%d want %d", len(results), len(ids))
	}
	if r := results["7"]; !errors.Is(r.Err, context.DeadlineExceeded) {
		t.Fatalf("blocked call result=%+v, want its own timeout", r)
	}
	if r := results["3"]; r.Err != nil || r.Value != "ok 3" {
		t.Fatalf("result=%+v", r)
	}
}

func TestCollectReportsUnreachedIDsWithContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	results := Collect(ctx, []string{"a", "b", "c"}, 1, time.Second, func(_ context.Context, id string) (int, error) {
		if id == "a" {
			cancel()
		}
		return 1, nil
	})
	if results["a"].Err != nil || !errors.Is(results["c"].Err, context.Canceled) {
		t.Fatalf("results=%+v", results)
	}
}
