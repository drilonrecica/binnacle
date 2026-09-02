// SPDX-License-Identifier: AGPL-3.0-only

package demo

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

type Clock interface{ Now() time.Time }
type Generator struct {
	seed       uint64
	clock      Clock
	Containers int
}

func New(seed uint64, clock Clock) *Generator {
	return &Generator{seed: seed, clock: clock, Containers: 1}
}
func (g *Generator) Snapshot(step uint64) metrics.Snapshot {
	return g.SnapshotAt(step, g.clock.Now().UTC())
}

// template describes one synthetic logical resource. The catalogue repeats
// across projects so grouping, infrastructure, and multi-container resources
// all appear with the default container count.
type template struct {
	name, category string
	infrastructure bool
	components     int
	memoryBase     int64
}

var templates = []template{
	{"web", "application", false, 2, 512 << 20},
	{"api", "application", false, 2, 768 << 20},
	{"worker", "worker", false, 1, 384 << 20},
	{"postgres", "database", true, 1, 1536 << 20},
	{"redis", "cache", true, 1, 128 << 20},
	{"scheduler", "worker", false, 1, 256 << 20},
	{"docs", "application", false, 1, 96 << 20},
	{"search", "service", false, 3, 1024 << 20},
	{"proxy", "proxy", true, 1, 64 << 20},
	{"admin", "application", false, 1, 320 << 20},
}

var projects = []string{"shop", "blog", "internal", "labs"}

const demoBootOffset = 19*24*time.Hour + 6*time.Hour

func describe(index int) (template, string, string) {
	tpl := templates[index%len(templates)]
	round := index / len(templates)
	project := projects[round%len(projects)]
	environment := "production"
	if round%2 == 1 {
		environment = "staging"
	}
	return tpl, project, environment
}

func (g *Generator) SnapshotAt(step uint64, now time.Time) metrics.Snapshot {
	r := rand.New(rand.NewPCG(g.seed, step))
	now = now.UTC()
	// Smooth host signals: a slow daily wave plus bounded jitter.
	phase := float64(step%1800) / 1800 * 2 * math.Pi
	cpu := 18 + 12*math.Sin(phase) + r.Float64()*10
	user := cpu * 0.7
	system := cpu * 0.22
	iowait := cpu * 0.06
	steal := cpu * 0.02
	memoryTotal := int64(8 << 30)
	memory := int64(3<<30) + int64(float64(1<<30)*math.Sin(phase/2)) + int64(r.Uint64()%uint64(256<<20))
	memoryAvailable := memoryTotal - memory
	memoryPct := float64(memory) / float64(memoryTotal) * 100
	swapTotal := int64(2 << 30)
	swapUsed := int64(180 << 20)
	swapPct := float64(swapUsed) / float64(swapTotal) * 100
	load1 := 0.6 + cpu/40 + r.Float64()*0.3
	load5 := load1*0.9 + 0.05
	load15 := load1*0.8 + 0.1
	rx := 200_000 + r.Float64()*8_000_000
	tx := 100_000 + r.Float64()*4_000_000
	diskRead := 400_000 + r.Float64()*2_000_000
	diskWrite := 900_000 + r.Float64()*3_000_000
	diskReadIops := diskRead / 32_000
	diskWriteIops := diskWrite / 32_000
	diskTotal := int64(80 << 30)
	diskUsed := int64(42<<30) + int64(step%86_400)*int64(3<<10)
	uptime := (demoBootOffset + time.Duration(step)*2*time.Second).Seconds()

	count := max(g.Containers, 1)
	resources := make([]metrics.ResourceSnapshot, count)
	for i := 0; i < count; i++ {
		tpl, project, environment := describe(i)
		status := metrics.StatusHealthy
		offset := step + uint64(i)
		if offset%11 == 0 {
			status = metrics.StatusDegraded
		}
		if offset%23 == 0 {
			status = metrics.StatusDown
		}
		if (offset/40)%29 == 0 && i%7 == 3 {
			status = metrics.StatusUnknown
		}
		name := fmt.Sprintf("%s-%s", tpl.name, project)
		id := fmt.Sprintf("res_demo_%d", i+1)
		baseCPU := 1.5 + float64(i%5)*1.8
		if tpl.infrastructure {
			baseCPU = 0.8
		}
		resCPU := baseCPU + baseCPU*0.6*math.Sin(phase*float64(1+i%3)+float64(i)) + r.Float64()*baseCPU*0.25
		if status == metrics.StatusDown {
			resCPU = 0
		}
		resMem := tpl.memoryBase + int64(r.Int64()%(tpl.memoryBase/4+1))
		resRX, resTX := rx/(float64(i)+1.5), tx/(float64(i)+1.5)
		blockRead, blockWrite := resRX/4, resTX/4
		components := make([]metrics.ResourceComponent, tpl.components)
		for c := 0; c < tpl.components; c++ {
			share := 1 / float64(tpl.components)
			cpuShare := resCPU * share
			memShare := int64(float64(resMem) * share)
			cRX, cTX := resRX*share, resTX*share
			cBlockRead, cBlockWrite := blockRead*share, blockWrite*share
			pids := uint64(4 + c*3 + i%5)
			componentStatus := status
			runtime, health := "running", "healthy"
			if status == metrics.StatusDown {
				runtime, health = "exited", ""
			}
			if status == metrics.StatusUnknown {
				health = "starting"
			}
			if status == metrics.StatusDegraded && c == tpl.components-1 {
				health = "unhealthy"
			} else if status == metrics.StatusDegraded {
				componentStatus = metrics.StatusHealthy
			}
			components[c] = metrics.ResourceComponent{
				ID:             metrics.ContainerID(fmt.Sprintf("%s-c%d-%012d", id, c+1, (i+1)*100+c)),
				Name:           fmt.Sprintf("%s-%d", name, c+1),
				Status:         componentStatus,
				RuntimeState:   runtime,
				HealthStatus:   health,
				CPUHostPercent: &cpuShare,
				MemoryBytes:    &memShare,
				RXBPS:          &cRX,
				TXBPS:          &cTX,
				BlockReadBPS:   &cBlockRead,
				BlockWriteBPS:  &cBlockWrite,
				PIDs:           &pids,
			}
		}
		resources[i] = metrics.ResourceSnapshot{
			ID:             metrics.ResourceID(id),
			Name:           name,
			Status:         status,
			SignalStatus:   status,
			CPUHostPercent: &resCPU,
			MemoryBytes:    &resMem,
			RXBPS:          &resRX,
			TXBPS:          &resTX,
			BlockReadBPS:   &blockRead,
			BlockWriteBPS:  &blockWrite,
			LastSeenAt:     now,
			Category:       tpl.category,
			Project:        project,
			Environment:    environment,
			Infrastructure: tpl.infrastructure,
			Components:     components,
			StableKey:      name,
			SourceKind:     "compose",
		}
	}
	dockerState := metrics.CollectorHealthy
	dockerReason := ""
	if step%97 >= 92 {
		dockerState = metrics.CollectorDegraded
		dockerReason = "Docker API responded slowly"
	}
	return metrics.Snapshot{
		Sequence:     metrics.Sequence(step + 1),
		At:           now,
		BootIdentity: "demo-boot-1",
		Host: metrics.HostObservation{
			At:                   now,
			CPUPercent:           &cpu,
			CPUUserPercent:       &user,
			CPUSystemPercent:     &system,
			CPUIOWaitPercent:     &iowait,
			CPUStealPercent:      &steal,
			MemoryUsedBytes:      &memory,
			MemoryTotalBytes:     &memoryTotal,
			MemoryAvailableBytes: &memoryAvailable,
			MemoryPercent:        &memoryPct,
			SwapUsedBytes:        &swapUsed,
			SwapTotalBytes:       &swapTotal,
			SwapPercent:          &swapPct,
			Load1:                &load1,
			Load5:                &load5,
			Load15:               &load15,
			NetworkRXBPS:         &rx,
			NetworkTXBPS:         &tx,
			DiskReadBPS:          &diskRead,
			DiskWriteBPS:         &diskWrite,
			DiskReadIOPS:         &diskReadIops,
			DiskWriteIOPS:        &diskWriteIops,
			DiskUsedBytes:        &diskUsed,
			DiskTotalBytes:       &diskTotal,
			UptimeSeconds:        &uptime,
		},
		Resources: resources,
		Collectors: map[string]metrics.CollectorHealth{
			"host":   {Name: "host", State: metrics.CollectorHealthy, FreshAt: now},
			"docker": {Name: "docker", State: dockerState, Reason: dockerReason, FreshAt: now},
		},
	}
}

// FilesystemsAt returns per-mount usage so the filesystem alert families and
// the Host page have realistic data in demo mode.
func (g *Generator) FilesystemsAt(step uint64, now time.Time) []metrics.FilesystemObservation {
	now = now.UTC()
	mount := func(key, point, fsType string, total, used, inodesTotal, inodesUsed int64) metrics.FilesystemObservation {
		available := total - used
		usedPct := float64(used) / float64(total) * 100
		inodesPct := float64(inodesUsed) / float64(inodesTotal) * 100
		return metrics.FilesystemObservation{At: now, MountKey: key, MountPoint: point, FSType: fsType, TotalBytes: &total, UsedBytes: &used, AvailableBytes: &available, UsedPercent: &usedPct, InodesTotal: &inodesTotal, InodesUsed: &inodesUsed, InodesUsedPercent: &inodesPct}
	}
	drift := int64(step%86_400) * int64(3<<10)
	return []metrics.FilesystemObservation{
		mount("root", "/", "ext4", 80<<30, 42<<30+drift, 5_242_880, 812_331),
		mount("docker", "/var/lib/docker", "xfs", 200<<30, 163<<30+drift*4, 104_857_600, 9_812_331),
		mount("boot", "/boot", "vfat", 1<<30, 302<<20, 0+65_536, 402),
	}
}

func (g *Generator) Events(step uint64) []metrics.Event {
	now := g.clock.Now().UTC()
	switch {
	case step%97 == 92:
		return []metrics.Event{{ID: metrics.Sequence(step + 1), At: now, Type: "collector_degraded", Severity: "warning", Message: "Docker collector degraded: API responded slowly", Details: `{"collector":"docker"}`}}
	case step%29 == 0:
		return []metrics.Event{{ID: metrics.Sequence(step + 1), At: now, Type: "container_oom", ResourceID: "res_demo_8", ContainerInstance: "res_demo_8-c1-000000000800", Severity: "critical", Message: "search-shop-1 was killed by the kernel OOM handler", Details: `{"container":"search-shop-1"}`}}
	case step%17 == 0:
		return []metrics.Event{{ID: metrics.Sequence(step + 1), At: now, Type: "deployment", ResourceID: "res_demo_1", Severity: "info", Message: "web-shop deployed (containers replaced)", Details: `{"confidence":"confirmed"}`}}
	case step%11 == 0:
		return []metrics.Event{{ID: metrics.Sequence(step + 1), At: now, Type: "container_restart", ResourceID: "res_demo_3", ContainerInstance: "res_demo_3-c1-000000000300", Severity: "warning", Message: "worker-shop-1 restarted", Details: `{"exitCode":137}`}}
	case step%7 == 0:
		return []metrics.Event{{ID: metrics.Sequence(step + 1), At: now, Type: "container_health_status_change", ResourceID: "res_demo_2", ContainerInstance: "res_demo_2-c2-000000000201", Severity: "info", Message: "api-shop-2 health changed to healthy"}}
	}
	return nil
}
