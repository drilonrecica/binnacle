// SPDX-License-Identifier: AGPL-3.0-only
package events

import (
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/dockerapi"
)

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
