// SPDX-License-Identifier: AGPL-3.0-only
package production

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	dockercollector "github.com/drilonrecica/talos/internal/collector/docker"
	hostcollector "github.com/drilonrecica/talos/internal/collector/host"
	"github.com/drilonrecica/talos/internal/coolify"
	"github.com/drilonrecica/talos/internal/dockerapi"
	"github.com/drilonrecica/talos/internal/events"
	"github.com/drilonrecica/talos/internal/metrics"
	"github.com/drilonrecica/talos/internal/resources"
	"golang.org/x/sys/unix"
)

type Sampler struct {
	Engine               *metrics.Engine
	Docker               dockerapi.Client
	HostProc             string
	DataDir              string
	Interval             func() time.Duration
	MaxDockerConcurrency int
	cancel               context.CancelFunc
	mu                   sync.Mutex
	previousCPU          hostcollector.CPUCounters
	haveCPU              bool
	previousNetwork      hostcollector.NetworkCounters
	previousNetworkAt    time.Time
	previousStats        map[string]dockerSample
	lastResources        []metrics.ResourceSnapshot
	hostFailures         int
	dockerFailures       int
}
type dockerSample struct {
	value dockerapi.Stats
	at    time.Time
}

func (s *Sampler) Start(ctx context.Context) error {
	if s.Engine == nil {
		return errors.New("metrics engine is required")
	}
	ctx, s.cancel = context.WithCancel(ctx)
	go s.run(ctx)
	return nil
}
func (s *Sampler) Stop(context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	if closer, ok := s.Docker.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

func (s *Sampler) run(ctx context.Context) {
	var dockerEvents <-chan dockerapi.Event
	if s.Docker != nil {
		dockerEvents = s.Docker.Events(ctx)
	}
	pending := make([]metrics.Event, 0, 16)
	for {
		s.collect(ctx, pending)
		pending = pending[:0]
		interval := 2 * time.Second
		if s.Interval != nil && s.Interval() >= time.Second {
			interval = s.Interval()
		}
		timer := time.NewTimer(interval)
	wait:
		for {
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case event, ok := <-dockerEvents:
				if !ok {
					dockerEvents = nil
					continue
				}
				if normalized, accepted := events.NormalizeDocker(event, false); accepted {
					if len(pending) == 128 {
						pending = pending[1:]
					}
					pending = append(pending, normalized)
				}
			case <-timer.C:
				break wait
			}
		}
	}
}

func (s *Sampler) collect(ctx context.Context, pending []metrics.Event) {
	now := time.Now().UTC()
	host, boot, hostErr := s.collectHost(now)
	collectors := map[string]metrics.CollectorHealth{}
	if hostErr != nil {
		s.hostFailures++
		collectors["host"] = health("host", s.hostFailures, hostErr, now)
	} else {
		s.hostFailures = 0
		collectors["host"] = health("host", 0, nil, now)
	}
	resourceValues, dockerErr := s.collectDocker(ctx, now, host.MemoryTotalBytes)
	if dockerErr != nil {
		s.dockerFailures++
		collectors["docker"] = health("docker", s.dockerFailures, dockerErr, now)
		resourceValues = append([]metrics.ResourceSnapshot(nil), s.lastResources...)
	} else {
		s.dockerFailures = 0
		collectors["docker"] = health("docker", 0, nil, now)
		s.lastResources = append([]metrics.ResourceSnapshot(nil), resourceValues...)
	}
	s.Engine.Publish(metrics.Snapshot{At: now, BootIdentity: metrics.BootIdentity(boot), Host: host, Resources: resourceValues, Collectors: collectors}, pending...)
}

func (s *Sampler) collectHost(now time.Time) (metrics.HostObservation, string, error) {
	read := func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(s.HostProc, name)) }
	statRaw, err := read("stat")
	if err != nil {
		return metrics.HostObservation{}, "", err
	}
	stats, err := hostcollector.ParseProcStat(string(statRaw))
	if err != nil {
		return metrics.HostObservation{}, "", err
	}
	memRaw, err := read("meminfo")
	if err != nil {
		return metrics.HostObservation{}, "", err
	}
	memory, err := hostcollector.ParseMeminfo(string(memRaw))
	if err != nil {
		return metrics.HostObservation{}, "", err
	}
	loadRaw, err := read("loadavg")
	if err != nil {
		return metrics.HostObservation{}, "", err
	}
	load, err := hostcollector.ParseLoadavg(string(loadRaw))
	if err != nil {
		return metrics.HostObservation{}, "", err
	}
	uptimeRaw, err := read("uptime")
	if err != nil {
		return metrics.HostObservation{}, "", err
	}
	uptime, err := hostcollector.ParseUptime(string(uptimeRaw))
	if err != nil {
		return metrics.HostObservation{}, "", err
	}
	networkRaw, err := read("net/dev")
	if err != nil {
		return metrics.HostObservation{}, "", err
	}
	networks, err := hostcollector.ParseNetDev(string(networkRaw))
	if err != nil {
		return metrics.HostObservation{}, "", err
	}
	network := hostcollector.AggregateNetwork(networks)
	var cpu *float64
	if s.haveCPU {
		cpu = hostcollector.CPUDelta(s.previousCPU, stats["cpu"]).Busy
	}
	s.previousCPU, s.haveCPU = stats["cpu"], true
	var rx, tx *float64
	if !s.previousNetworkAt.IsZero() {
		elapsed := now.Sub(s.previousNetworkAt).Seconds()
		rx, tx = hostcollector.Rate(network.RXBytes, s.previousNetwork.RXBytes, elapsed), hostcollector.Rate(network.TXBytes, s.previousNetwork.TXBytes, elapsed)
	}
	s.previousNetwork, s.previousNetworkAt = network, now
	used, total := int64(memory.Used), int64(memory.Total)
	memoryPercent := float64(memory.Used) * 100 / float64(memory.Total)
	observation := metrics.HostObservation{At: now, CPUPercent: cpu, MemoryUsedBytes: &used, MemoryTotalBytes: &total, MemoryPercent: &memoryPercent, Load1: &load, NetworkRXBPS: rx, NetworkTXBPS: tx, UptimeSeconds: &uptime}
	var fs unix.Statfs_t
	if unix.Statfs(filepath.Join(s.HostProc, "1/root"), &fs) == nil {
		diskTotal := int64(fs.Blocks) * int64(fs.Bsize)
		diskUsed := diskTotal - int64(fs.Bavail)*int64(fs.Bsize)
		observation.DiskTotalBytes, observation.DiskUsedBytes = &diskTotal, &diskUsed
	}
	bootRaw, _ := read("sys/kernel/random/boot_id")
	return observation, strings.TrimSpace(string(bootRaw)), nil
}

type resourceGroup struct {
	identity                                     resources.Identity
	category, environment                        string
	infrastructure                               bool
	components                                   []metrics.ResourceComponent
	cpu, memory, rx, tx, read, write             float64
	cpuOK, memoryOK, rxOK, txOK, readOK, writeOK bool
	status                                       []metrics.ResourceStatus
}

func (s *Sampler) collectDocker(ctx context.Context, now time.Time, hostTotal *int64) ([]metrics.ResourceSnapshot, error) {
	if s.Docker == nil {
		return nil, errors.New("Docker client is not configured")
	}
	containers, err := s.Docker.List(ctx)
	if err != nil {
		return nil, err
	}
	groups := map[string]*resourceGroup{}
	if s.previousStats == nil {
		s.previousStats = map[string]dockerSample{}
	}
	for _, container := range containers {
		inspect, inspectErr := s.Docker.Inspect(ctx, container.ID)
		if inspectErr != nil {
			return nil, inspectErr
		}
		if inspect.State != "running" {
			continue
		}
		stats, statsErr := s.Docker.Stats(ctx, container.ID)
		if statsErr != nil {
			return nil, statsErr
		}
		identity := resources.Resolve(inspect.Labels, inspect.Name, "")
		group := groups[identity.StableKey]
		if group == nil {
			group = &resourceGroup{identity: identity, category: category(inspect.Labels, identity), environment: inspect.Labels["coolify.environment"]}
			if metadata, ok := coolify.Resolve(inspect.Labels); ok {
				group.infrastructure = metadata.Infrastructure
				if metadata.Environment != "" {
					group.environment = metadata.Environment
				}
				if metadata.Project != "" {
					group.identity.Project = metadata.Project
				}
			}
			groups[identity.StableKey] = group
		}
		status := metrics.StatusHealthy
		if inspect.Health == "unhealthy" {
			status = metrics.StatusDown
		} else if inspect.Health == "starting" {
			status = metrics.StatusUnknown
		}
		group.status = append(group.status, status)
		group.components = append(group.components, metrics.ResourceComponent{ID: metrics.ContainerID(container.ID), Name: inspect.Name, Status: status})
		memory := dockercollector.NormalizeMemory(dockercollector.MemoryStats{Usage: stats.Memory.Usage, Limit: stats.Memory.Limit, InactiveFile: stats.Memory.InactiveFile, PIDs: stats.PIDs}, uint64Value(hostTotal))
		if memory.WorkingSet != nil {
			group.memory += *memory.WorkingSet
			group.memoryOK = true
		}
		if previous, ok := s.previousStats[container.ID]; ok {
			cpu := dockercollector.NormalizeCPU(dockercollector.CPUStats{Total: previous.value.CPU.TotalUsage, System: previous.value.CPU.SystemUsage, Online: previous.value.CPU.OnlineCPUs}, dockercollector.CPUStats{Total: stats.CPU.TotalUsage, System: stats.CPU.SystemUsage, Online: stats.CPU.OnlineCPUs}, stats.CPU.OnlineCPUs)
			if cpu.HostPercent != nil {
				group.cpu += *cpu.HostPercent
				group.cpuOK = true
			}
			io := dockercollector.NormalizeIO(dockercollector.IOCounters{RX: previous.value.IO.RX, TX: previous.value.IO.TX, Read: previous.value.IO.Read, Write: previous.value.IO.Write}, dockercollector.IOCounters{RX: stats.IO.RX, TX: stats.IO.TX, Read: stats.IO.Read, Write: stats.IO.Write}, now.Sub(previous.at).Seconds())
			if io.RX != nil {
				group.rx += *io.RX
				group.rxOK = true
			}
			if io.TX != nil {
				group.tx += *io.TX
				group.txOK = true
			}
			if io.Read != nil {
				group.read += *io.Read
				group.readOK = true
			}
			if io.Write != nil {
				group.write += *io.Write
				group.writeOK = true
			}
		}
		s.previousStats[container.ID] = dockerSample{value: stats, at: now}
	}
	result := make([]metrics.ResourceSnapshot, 0, len(groups))
	for stable, group := range groups {
		id := resourceID(stable)
		result = append(result, metrics.ResourceSnapshot{ID: id, Name: group.identity.Name, Status: resources.RollupStatus(group.status), CPUHostPercent: number(group.cpu, group.cpuOK), MemoryBytes: integer(group.memory, group.memoryOK), RXBPS: number(group.rx, group.rxOK), TXBPS: number(group.tx, group.txOK), BlockReadBPS: number(group.read, group.readOK), BlockWriteBPS: number(group.write, group.writeOK), LastSeenAt: now, Category: group.category, Project: group.identity.Project, Environment: group.environment, Infrastructure: group.infrastructure, Components: group.components})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func health(name string, failures int, err error, now time.Time) metrics.CollectorHealth {
	state := metrics.CollectorHealthy
	if failures >= 6 {
		state = metrics.CollectorDown
	} else if failures >= 3 {
		state = metrics.CollectorDegraded
	}
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return metrics.CollectorHealth{Name: name, State: state, Reason: reason, FreshAt: now}
}
func resourceID(stable string) metrics.ResourceID {
	sum := sha256.Sum256([]byte(stable))
	return metrics.ResourceID("res_" + hex.EncodeToString(sum[:8]))
}
func category(labels map[string]string, identity resources.Identity) string {
	if value := strings.ToLower(labels["talos.category"]); resources.ValidCategory(value) {
		return value
	}
	if labels["coolify.type"] == "infrastructure" {
		return "infrastructure"
	}
	if identity.Source == "compose" || identity.Source == "coolify" {
		return "service"
	}
	return "unmanaged"
}
func number(value float64, ok bool) *float64 {
	if !ok {
		return nil
	}
	return &value
}
func integer(value float64, ok bool) *int64 {
	if !ok {
		return nil
	}
	result := int64(value)
	return &result
}
func uint64Value(value *int64) uint64 {
	if value == nil || *value < 0 {
		return 0
	}
	return uint64(*value)
}
