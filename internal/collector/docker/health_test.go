// SPDX-License-Identifier: AGPL-3.0-only
package docker

import (
	"errors"
	"testing"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

func TestHealthTrackerTransitions(t *testing.T) {
	var h HealthTracker
	boom := errors.New("boom")
	want := []metrics.CollectorState{
		metrics.CollectorHealthy, metrics.CollectorHealthy, // 1-2 failures: still healthy
		metrics.CollectorDegraded, metrics.CollectorDegraded, metrics.CollectorDegraded, // 3-5
		metrics.CollectorDown, metrics.CollectorDown, // 6+
	}
	for i, w := range want {
		if got := h.Observe(boom); got != w {
			t.Fatalf("failure %d: got %q want %q", i+1, got, w)
		}
	}
	if got := h.Observe(nil); got != metrics.CollectorHealthy || h.Failures != 0 {
		t.Fatalf("success must reset: state=%q failures=%d", got, h.Failures)
	}
	if got := h.Observe(boom); got != metrics.CollectorHealthy {
		t.Fatalf("one failure after recovery stays healthy, got %q", got)
	}
}
