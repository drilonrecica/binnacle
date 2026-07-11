// SPDX-License-Identifier: AGPL-3.0-only

package demo

import (
	"math/rand/v2"
	"time"

	"github.com/drilonrecica/talos/internal/metrics"
)

type Clock interface{ Now() time.Time }
type Generator struct {
	seed  uint64
	clock Clock
}

func New(seed uint64, clock Clock) *Generator { return &Generator{seed: seed, clock: clock} }
func (g *Generator) Snapshot(step uint64) metrics.Snapshot {
	r := rand.New(rand.NewPCG(g.seed, step))
	now := g.clock.Now().UTC()
	cpu := 5 + r.Float64()*40
	memory := int64(2<<30) + int64(r.Uint64()%uint64(2<<30))
	status := metrics.StatusHealthy
	if step%11 == 0 {
		status = metrics.StatusDegraded
	}
	if step%17 == 0 {
		status = metrics.StatusArchived
	}
	return metrics.Snapshot{Sequence: metrics.Sequence(step + 1), At: now, BootIdentity: "demo-boot-1", Host: metrics.HostObservation{At: now, CPUPercent: &cpu, MemoryUsedBytes: &memory}, Resources: []metrics.ResourceSnapshot{{ID: "res_demo_web", Name: "Demo web", Status: status, CPUHostPercent: &cpu, MemoryBytes: &memory, LastSeenAt: now}}, Collectors: map[string]metrics.CollectorHealth{"host": {Name: "host", State: metrics.CollectorHealthy, FreshAt: now}, "docker": {Name: "docker", State: metrics.CollectorHealthy, FreshAt: now}}}
}
func (g *Generator) Events(step uint64) []metrics.Event {
	now := g.clock.Now().UTC()
	switch {
	case step%17 == 0:
		return []metrics.Event{{ID: metrics.Sequence(step + 1), At: now, Type: "resource_archived", ResourceID: "res_demo_web", Message: "Demo resource archived"}}
	case step%11 == 0:
		return []metrics.Event{{ID: metrics.Sequence(step + 1), At: now, Type: "collector_degraded", Message: "Demo collector degraded"}}
	case step%7 == 0:
		return []metrics.Event{{ID: metrics.Sequence(step + 1), At: now, Type: "oom", ResourceID: "res_demo_web", Message: "Demo out-of-memory restart"}}
	}
	return nil
}
