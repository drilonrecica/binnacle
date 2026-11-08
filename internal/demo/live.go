// SPDX-License-Identifier: AGPL-3.0-only
package demo

import (
	"math"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

// Identity is the machine the demo pretends to be.
var Identity = metrics.HostIdentity{
	Hostname: "demo-vps-01",
	OS:       "Ubuntu 24.04.1 LTS",
	Kernel:   "6.8.0-48-generic",
	CPUModel: "Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz",
	CPUCores: 4,
}

type hostDetail struct {
	nice, irq, softirq, idle                    float64
	cores                                       []float64
	contextSwitches, interrupts, softInterrupts float64
	running, blocked                            uint64
	memoryFree, memoryActive, memoryInactive    int64
	memoryCached, memoryBuffers                 int64
	interfaces                                  []metrics.InterfaceRate
	disks                                       []metrics.DiskDeviceRate
}

func liveHostDetail(step uint64, phase, cpu, iowait float64, memory, available int64, rx, tx, diskRead, diskWrite, diskReadIops, diskWriteIops float64) hostDetail {
	d := hostDetail{
		nice:            0.4,
		irq:             0.2,
		softirq:         cpu * 0.03,
		idle:            math.Max(0, 100-cpu-iowait),
		contextSwitches: 9_000 + 3_000*math.Sin(phase*3),
		interrupts:      6_000 + 2_000*math.Sin(phase*2),
		softInterrupts:  3_000 + 800*math.Cos(phase*2),
		running:         1 + step%3,
		blocked:         step % 7 / 6,
		memoryFree:      available / 6,
		memoryActive:    memory * 3 / 4,
		memoryInactive:  available / 2,
		memoryCached:    available * 3 / 5,
		memoryBuffers:   192 << 20,
	}
	for core := 0; core < Identity.CPUCores; core++ {
		v := cpu * (1 + 0.45*math.Sin(phase*float64(3+core)+float64(core)))
		d.cores = append(d.cores, math.Min(100, math.Max(0, v)))
	}
	rate := func(v float64) *float64 { return &v }
	d.interfaces = []metrics.InterfaceRate{
		{Name: "docker0", RXBPS: rate(rx / 5), TXBPS: rate(tx / 5)},
		{Name: "eth0", RXBPS: rate(rx), TXBPS: rate(tx)},
		{Name: "lo", RXBPS: rate(4_800), TXBPS: rate(4_800)},
	}
	d.disks = []metrics.DiskDeviceRate{
		{Name: "sda", ReadBPS: rate(diskRead), WriteBPS: rate(diskWrite), ReadIOPS: rate(diskReadIops), WriteIOPS: rate(diskWriteIops)},
		{Name: "sda1", ReadBPS: rate(diskRead * 0.95), WriteBPS: rate(diskWrite * 0.95), ReadIOPS: rate(diskReadIops * 0.95), WriteIOPS: rate(diskWriteIops * 0.95)},
		{Name: "sda15", ReadBPS: rate(0), WriteBPS: rate(0), ReadIOPS: rate(0), WriteIOPS: rate(0)},
	}
	return d
}

var demoImages = map[string]string{
	"postgres": "postgres:16-alpine",
	"redis":    "redis:7-alpine",
	"proxy":    "caddy:2.8",
	"search":   "getmeili/meilisearch:v1.10",
}

var demoPorts = map[string][]string{
	"web":      {"3000/tcp"},
	"api":      {"8080/tcp"},
	"postgres": {"5432/tcp"},
	"redis":    {"6379/tcp"},
	"proxy":    {"443->443/tcp", "443->443/udp", "80->80/tcp"},
	"search":   {"7700/tcp"},
	"docs":     {"80/tcp"},
	"admin":    {"3001/tcp"},
}

// describeComponent adds the live-view detail to a demo container: image,
// ports, start time and an effective memory limit (a set limit for databases
// and caches, host memory for the rest).
func describeComponent(component *metrics.ResourceComponent, tpl template, index, replica int, hostMemory int64, now time.Time, status metrics.ResourceStatus) {
	image, ok := demoImages[tpl.name]
	if !ok {
		image = "ghcr.io/demo/" + tpl.name + ":latest"
	}
	component.Image = image
	component.Ports = demoPorts[tpl.name]
	limit := hostMemory
	if tpl.infrastructure && tpl.name != "proxy" {
		limit = tpl.memoryBase * 2
	}
	component.MemoryLimitBytes = &limit
	if status != metrics.StatusDown {
		age := time.Duration(1+(index*7+replica*3)%40) * 6 * time.Hour
		component.StartedAt = now.Add(-age).Format(time.RFC3339)
	}
}
