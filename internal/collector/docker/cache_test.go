// SPDX-License-Identifier: AGPL-3.0-only
package docker

import (
	"testing"

	"github.com/drilonrecica/binnacle/internal/dockerapi"
)

func TestHealthFromAction(t *testing.T) {
	for _, tc := range []struct {
		action, health string
		ok             bool
	}{
		{"health_status: healthy", "healthy", true},
		{"health_status: unhealthy", "unhealthy", true},
		{"Health_Status:starting", "starting", true},
		{"health_status", "", true},
		{"exec_start: sh", "", false},
		{"start", "", false},
	} {
		health, ok := HealthFromAction(tc.action)
		if health != tc.health || ok != tc.ok {
			t.Errorf("HealthFromAction(%q)=%q,%v want %q,%v", tc.action, health, ok, tc.health, tc.ok)
		}
	}
}

func TestApplyIgnoresUncachedAndUnknownEvents(t *testing.T) {
	c := NewCache()
	if effect := c.Apply(dockerapi.Event{ID: "x", Action: "start"}); effect != (EventEffect{}) || c.Len() != 0 {
		t.Fatalf("uncached start effect=%+v len=%d: must not create an entry", effect, c.Len())
	}
	c.Set(Metadata{ID: "a", State: "running", Health: "healthy"})
	if effect := c.Apply(dockerapi.Event{ID: "a", Action: "exec_start: sh"}); effect != (EventEffect{}) {
		t.Fatalf("exec effect=%+v", effect)
	}
	if v, _ := c.Get("a"); v.State != "running" || v.Health != "healthy" || v.Reinspect {
		t.Fatalf("exec changed the entry: %+v", v)
	}
	if removed := c.Retain(map[string]struct{}{}); removed != 1 || c.Len() != 0 {
		t.Fatalf("Retain removed=%d len=%d", removed, c.Len())
	}
}
