// SPDX-License-Identifier: AGPL-3.0-only
package events

import (
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/dockerapi"
)

func TestCorrelate(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	window := time.Minute
	cases := []struct {
		name string
		old  Instance
		next Instance
		want DeploymentConfidence
		ok   bool
		over time.Duration
	}{
		{"different image within window is confirmed", Instance{ID: "a", ResourceID: "r", Image: "app:1", Stopped: base}, Instance{ID: "b", ResourceID: "r", Image: "app:2", Started: base.Add(10 * time.Second)}, Confirmed, true, 0},
		{"same image within window is replacement", Instance{ID: "a", ResourceID: "r", Image: "app:1", Stopped: base}, Instance{ID: "b", ResourceID: "r", Image: "app:1", Started: base.Add(10 * time.Second)}, Replacement, true, 0},
		{"overlap is likely with overlap duration", Instance{ID: "a", ResourceID: "r", Stopped: base}, Instance{ID: "b", ResourceID: "r", Started: base.Add(-5 * time.Second)}, Likely, true, 5 * time.Second},
		{"gap beyond window is not a deployment", Instance{ID: "a", ResourceID: "r", Stopped: base}, Instance{ID: "b", ResourceID: "r", Started: base.Add(2 * time.Minute)}, "", false, 0},
		{"gap exactly at window correlates", Instance{ID: "a", ResourceID: "r", Stopped: base}, Instance{ID: "b", ResourceID: "r", Started: base.Add(window)}, Replacement, true, 0},
		{"different resources never correlate", Instance{ID: "a", ResourceID: "r1", Stopped: base}, Instance{ID: "b", ResourceID: "r2", Started: base}, "", false, 0},
		{"empty resource never correlates", Instance{ID: "a", Stopped: base}, Instance{ID: "b", Started: base}, "", false, 0},
		{"unstarted next never correlates", Instance{ID: "a", ResourceID: "r", Stopped: base}, Instance{ID: "b", ResourceID: "r"}, "", false, 0},
		{"running old instance treats gap as zero", Instance{ID: "a", ResourceID: "r", Image: "x"}, Instance{ID: "b", ResourceID: "r", Image: "y", Started: base}, Confirmed, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Correlate(tc.old, tc.next, window)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if got.Confidence != tc.want || got.Overlap != tc.over || got.OldID != tc.old.ID || got.NewID != tc.next.ID {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestNormalizeDocker(t *testing.T) {
	const id = "0123456789abcdef"
	at := "2026-01-01T12:00:00Z"
	cases := []struct {
		action string
		oom    bool
		want   string
		ok     bool
	}{
		{"start", false, "container_start", true},
		{"STOP", false, "container_stop", true},
		{"die", false, "container_die", true},
		{"die", true, "container_oom", true},
		{"oom", false, "container_oom", true},
		{"destroy", false, "container_destroy", true},
		{"restart", false, "container_restart", true},
		{"health_status: unhealthy", false, "container_health_status_change", true},
		{"health_status", false, "container_health_status_change", true},
		{"exec_create: sh", false, "", false},
		{"", false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			ev, ok := NormalizeDocker(dockerapi.Event{ID: id, Action: tc.action, Time: at}, tc.oom)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if ev.Type != tc.want {
				t.Fatalf("type = %q, want %q", ev.Type, tc.want)
			}
			if string(ev.ContainerInstance) != id || ev.CorrelationKey != id {
				t.Fatalf("ids not carried: %+v", ev)
			}
			if !ev.At.Equal(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)) {
				t.Fatalf("at = %v", ev.At)
			}
		})
	}
}

func TestNormalizeDockerBadTimeYieldsZeroUTC(t *testing.T) {
	ev, ok := NormalizeDocker(dockerapi.Event{ID: "x", Action: "start", Time: "nope"}, false)
	if !ok || !ev.At.IsZero() {
		t.Fatalf("ok=%v at=%v", ok, ev.At)
	}
}
