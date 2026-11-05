// SPDX-License-Identifier: AGPL-3.0-only
package production

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dockercollector "github.com/drilonrecica/binnacle/internal/collector/docker"
	hostcollector "github.com/drilonrecica/binnacle/internal/collector/host"
	"github.com/drilonrecica/binnacle/internal/coolify"
	"github.com/drilonrecica/binnacle/internal/dockerapi"
	"github.com/drilonrecica/binnacle/internal/events"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/resources"
	"github.com/drilonrecica/binnacle/internal/storage"
	"golang.org/x/sys/unix"
)

type MetadataCache interface {
	Get(id string) (dockercollector.Metadata, bool)
	Set(v dockercollector.Metadata)
	Remove(id string)
}

type Sampler struct {
	Engine   *metrics.Engine
	Docker   dockerapi.Client
	Cache    MetadataCache
	HostProc string
	// HostSys is the host's sysfs; <HostSys>/block lists the whole devices
	// whose diskstats are summed. Empty means "/sys".
	HostSys string
	// HostRoot is the read-only bind mount of the host's "/" under which
	// filesystems are measured with statfs; "/" for a standalone binary.
	HostRoot             string
	DataDir              string
	Interval             func() time.Duration
	MaxDockerConcurrency int
	Store                interface {
		UpsertHost(context.Context, string, string, string) error
		UpsertResource(context.Context, storage.Resource) error
		ArchiveMissingResources(context.Context, []string, time.Time) error
	}
	cancel            context.CancelFunc
	mu                sync.Mutex
	previousCPU       hostcollector.CPUCounters
	haveCPU           bool
	previousNetwork   hostcollector.NetworkCounters
	previousNetworkAt time.Time
	previousDisk      hostcollector.DiskCounters
	previousDiskAt    time.Time
	previousStats     map[string]dockerSample
	lastResources     []metrics.ResourceSnapshot
	hostFailures      int
	dockerFailures    int
	LastDurationNanos atomic.Int64
	// statfs and mountPoint replace unix.Statfs and isMountPoint in tests.
	statfs     func(string, *unix.Statfs_t) error
	mountPoint func(string) bool
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
				s.handleDockerEvent(ctx, event)
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
	started := time.Now()
	defer func() { s.LastDurationNanos.Store(time.Since(started).Nanoseconds()) }()
	// started keeps its monotonic reading for elapsed time between ticks;
	// UTC() strips it, so only the stored and published copy is UTC (COL-8).
	now := started.UTC()
	host, filesystems, boot, fsReason, hostErr := s.collectHost(started)
	if len(filesystems) > 0 {
		s.Engine.PublishFilesystems(filesystems)
	}
	collectors := map[string]metrics.CollectorHealth{}
	if hostErr != nil {
		s.hostFailures++
		collectors["host"] = health("host", s.hostFailures, hostErr, now)
	} else {
		s.hostFailures = 0
		collectors["host"] = health("host", 0, nil, now)
		if fsReason != "" {
			h := collectors["host"]
			h.State, h.Reason = metrics.CollectorDegraded, fsReason
			collectors["host"] = h
		}
	}
	resourceValues, dockerErr := s.collectDocker(ctx, started, host.MemoryTotalBytes)
	if dockerErr != nil {
		s.dockerFailures++
		collectors["docker"] = health("docker", s.dockerFailures, dockerErr, now)
		resourceValues = append([]metrics.ResourceSnapshot(nil), s.lastResources...)
	} else {
		s.dockerFailures = 0
		collectors["docker"] = health("docker", 0, nil, now)
		s.lastResources = append([]metrics.ResourceSnapshot(nil), resourceValues...)
		if s.Store != nil {
			_ = s.Store.UpsertHost(ctx, "host", storage.HostIdentity("", "binnacle-local-host"), "Server")
			ids := make([]string, 0, len(resourceValues))
			for _, resource := range resourceValues {
				ids = append(ids, string(resource.ID))
				_ = s.Store.UpsertResource(ctx, storage.Resource{ID: string(resource.ID), HostID: "host", StableKey: resource.StableKey, SourceKind: resource.SourceKind, Name: resource.Name, Context: resource.Context, ProjectName: resource.Project, EnvironmentName: resource.Environment, Category: resource.Category, Status: string(resource.Status)})
			}
			_ = s.Store.ArchiveMissingResources(ctx, ids, now.Add(-5*time.Minute))
		}
	}
	s.Engine.Publish(metrics.Snapshot{At: now, BootIdentity: metrics.BootIdentity(boot), Host: host, Resources: resourceValues, Collectors: collectors}, pending...)
}
func (s *Sampler) handleDockerEvent(ctx context.Context, event dockerapi.Event) {
	if s.Cache == nil {
		return
	}
	switch strings.ToLower(event.Action) {
	case "destroy":
		s.Cache.Remove(event.ID)
	case "die", "stop", "pause":
		if v, ok := s.Cache.Get(event.ID); ok {
			v.State = strings.ToLower(event.Action)
			s.Cache.Set(v)
		}
	case "start", "unpause", "restart", "health_status":
		// Refresh metadata on lifecycle/health changes.
		if inspect, err := s.Docker.Inspect(ctx, event.ID); err == nil {
			s.Cache.Set(dockercollector.Metadata{ID: inspect.ID, Name: inspect.Name, Image: inspect.Image, Created: inspect.Created, State: inspect.State, Health: inspect.Health, Labels: inspect.Labels, Environment: inspect.Environment, Networks: inspect.Networks, Mounts: inspect.Mounts})
		}
	}
}

func (s *Sampler) CollectionDuration() time.Duration {
	if s == nil {
		return 0
	}
	return time.Duration(s.LastDurationNanos.Load())
}

// collectHost reads one host sample. tick should carry a monotonic clock
// reading: rates use it for elapsed time, and its UTC copy stamps the sample.
// The returned reason is non-empty when some host metrics are unavailable.
func (s *Sampler) collectHost(tick time.Time) (metrics.HostObservation, []metrics.FilesystemObservation, string, string, error) {
	now := tick.UTC()
	read := func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(s.HostProc, name)) }
	statRaw, err := read("stat")
	if err != nil {
		return metrics.HostObservation{}, nil, "", "", err
	}
	stats, err := hostcollector.ParseProcStat(string(statRaw))
	if err != nil {
		return metrics.HostObservation{}, nil, "", "", err
	}
	memRaw, err := read("meminfo")
	if err != nil {
		return metrics.HostObservation{}, nil, "", "", err
	}
	memory, err := hostcollector.ParseMeminfo(string(memRaw))
	if err != nil {
		return metrics.HostObservation{}, nil, "", "", err
	}
	loadRaw, err := read("loadavg")
	if err != nil {
		return metrics.HostObservation{}, nil, "", "", err
	}
	load, err := hostcollector.ParseLoadavgFull(string(loadRaw))
	if err != nil {
		return metrics.HostObservation{}, nil, "", "", err
	}
	uptimeRaw, err := read("uptime")
	if err != nil {
		return metrics.HostObservation{}, nil, "", "", err
	}
	uptime, err := hostcollector.ParseUptime(string(uptimeRaw))
	if err != nil {
		return metrics.HostObservation{}, nil, "", "", err
	}
	// PID 1 of the host's procfs is in the host network namespace; net/dev
	// without it resolves through the reader's own PID (COL-2).
	networkRaw, err := read("1/net/dev")
	if err != nil {
		return metrics.HostObservation{}, nil, "", "", err
	}
	networks, err := hostcollector.ParseNetDev(string(networkRaw))
	if err != nil {
		return metrics.HostObservation{}, nil, "", "", err
	}
	network := hostcollector.AggregateNetwork(networks)

	disk, diskErr := s.diskCounters(read)

	var cpu, cpuUser, cpuSystem, cpuIOWait, cpuSteal *float64
	if s.haveCPU {
		usage := hostcollector.CPUDelta(s.previousCPU, stats["cpu"])
		cpu, cpuUser, cpuSystem, cpuIOWait, cpuSteal = usage.Busy, usage.User, usage.System, usage.IOWait, usage.Steal
	}
	s.previousCPU, s.haveCPU = stats["cpu"], true

	var rx, tx, rxPackets, txPackets *float64
	var rxErrors, txErrors, rxDrops, txDrops *int64
	if !s.previousNetworkAt.IsZero() {
		elapsed := tick.Sub(s.previousNetworkAt).Seconds()
		rx = hostcollector.Rate(network.RXBytes, s.previousNetwork.RXBytes, elapsed)
		tx = hostcollector.Rate(network.TXBytes, s.previousNetwork.TXBytes, elapsed)
		rxPackets = hostcollector.Rate(network.RXPackets, s.previousNetwork.RXPackets, elapsed)
		txPackets = hostcollector.Rate(network.TXPackets, s.previousNetwork.TXPackets, elapsed)
		rxErrors = deltaInt64(network.RXErrors, s.previousNetwork.RXErrors)
		txErrors = deltaInt64(network.TXErrors, s.previousNetwork.TXErrors)
		rxDrops = deltaInt64(network.RXDrops, s.previousNetwork.RXDrops)
		txDrops = deltaInt64(network.TXDrops, s.previousNetwork.TXDrops)
	}
	s.previousNetwork, s.previousNetworkAt = network, tick

	var diskReadBPS, diskWriteBPS, diskReadIOPS, diskWriteIOPS *float64
	if diskErr == nil && !s.previousDiskAt.IsZero() {
		elapsed := tick.Sub(s.previousDiskAt).Seconds()
		diskReadBPS = hostcollector.Rate(hostcollector.SectorToBytes(disk.ReadSectors), hostcollector.SectorToBytes(s.previousDisk.ReadSectors), elapsed)
		diskWriteBPS = hostcollector.Rate(hostcollector.SectorToBytes(disk.WriteSectors), hostcollector.SectorToBytes(s.previousDisk.WriteSectors), elapsed)
		diskReadIOPS = hostcollector.Rate(disk.Reads, s.previousDisk.Reads, elapsed)
		diskWriteIOPS = hostcollector.Rate(disk.Writes, s.previousDisk.Writes, elapsed)
	}
	if diskErr != nil {
		// Unknown, not zero; the next good sample starts a fresh baseline.
		s.previousDisk, s.previousDiskAt = hostcollector.DiskCounters{}, time.Time{}
	} else {
		s.previousDisk, s.previousDiskAt = disk, tick
	}

	used, total := int64(memory.Used), int64(memory.Total)
	available := int64(memory.Available)
	cached := int64(memory.Cached)
	buffers := int64(memory.Buffers)
	var memoryPercent *float64
	if memory.Total > 0 {
		v := float64(memory.Used) * 100 / float64(memory.Total)
		memoryPercent = &v
	}
	swapUsed := int64(memory.SwapTotal - memory.SwapFree)
	swapTotal := int64(memory.SwapTotal)
	var swapPercent *float64
	if memory.SwapTotal > 0 {
		v := float64(memory.SwapTotal-memory.SwapFree) * 100 / float64(memory.SwapTotal)
		swapPercent = &v
	}
	load1, load5, load15 := load.One, load.Five, load.Fifteen

	observation := metrics.HostObservation{
		At: now, CPUPercent: cpu, CPUUserPercent: cpuUser, CPUSystemPercent: cpuSystem, CPUIOWaitPercent: cpuIOWait, CPUStealPercent: cpuSteal,
		MemoryUsedBytes: &used, MemoryTotalBytes: &total, MemoryAvailableBytes: &available, MemoryPercent: memoryPercent,
		MemoryCachedBytes: &cached, MemoryBuffersBytes: &buffers, SwapUsedBytes: &swapUsed, SwapTotalBytes: &swapTotal, SwapPercent: swapPercent,
		Load1: &load1, Load5: &load5, Load15: &load15,
		NetworkRXBPS: rx, NetworkTXBPS: tx, NetworkRXPacketsPS: rxPackets, NetworkTXPacketsPS: txPackets,
		NetworkRXErrorsDelta: rxErrors, NetworkTXErrorsDelta: txErrors, NetworkRXDropsDelta: rxDrops, NetworkTXDropsDelta: txDrops,
		DiskReadBPS: diskReadBPS, DiskWriteBPS: diskWriteBPS, DiskReadIOPS: diskReadIOPS, DiskWriteIOPS: diskWriteIOPS,
		UptimeSeconds: &uptime,
	}

	fsystems, fsReason := s.collectFilesystems(now)
	if len(fsystems) > 0 {
		var totalBytes, usedBytes int64
		for _, fs := range fsystems {
			if fs.TotalBytes != nil {
				totalBytes += *fs.TotalBytes
			}
			if fs.UsedBytes != nil {
				usedBytes += *fs.UsedBytes
			}
		}
		observation.DiskTotalBytes, observation.DiskUsedBytes = &totalBytes, &usedBytes
	}

	reason := fsReason
	if diskErr != nil {
		reason = joinReason(reason, "disk I/O unavailable: "+diskErr.Error())
	}
	bootRaw, _ := read("sys/kernel/random/boot_id")
	return observation, fsystems, strings.TrimSpace(string(bootRaw)), reason, nil
}

// diskCounters sums /proc/diskstats over the whole devices listed in
// <HostSys>/block, so partitions and stacked devices are not double counted
// (COL-3). Without the device list the counters are unknown.
func (s *Sampler) diskCounters(read func(string) ([]byte, error)) (hostcollector.DiskCounters, error) {
	raw, err := read("diskstats")
	if err != nil {
		return hostcollector.DiskCounters{}, err
	}
	disks, err := hostcollector.ParseDiskstats(string(raw))
	if err != nil {
		return hostcollector.DiskCounters{}, err
	}
	hostSys := s.HostSys
	if hostSys == "" {
		hostSys = "/sys"
	}
	whole, err := hostcollector.WholeDevices(filepath.Join(hostSys, "block"))
	if err != nil {
		return hostcollector.DiskCounters{}, err
	}
	return hostcollector.SumWholeDevices(disks, whole), nil
}

func joinReason(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

func deltaInt64(current, previous uint64) *int64 {
	if current < previous {
		return nil
	}
	v := int64(current - previous)
	return &v
}

// selfMountinfo lists the reader's own mounts; it is used only to confirm
// that HostRoot is a mount point, never as a source of host filesystems.
const selfMountinfo = "/proc/self/mountinfo"

// collectFilesystems reads the host mount namespace from <HostProc>/1/mountinfo
// and measures each retained mount with statfs(<HostRoot>/<target>). The
// returned reason is non-empty when filesystem metrics are partial or missing;
// the other host metrics are unaffected (ADR 022 §6).
func (s *Sampler) collectFilesystems(now time.Time) ([]metrics.FilesystemObservation, string) {
	statfs := s.statfs
	if statfs == nil {
		statfs = unix.Statfs
	}
	hostRoot := s.HostRoot
	if hostRoot == "" {
		hostRoot = "/"
	}
	mountPoint := s.mountPoint
	if mountPoint == nil {
		mountPoint = isMountPoint
	}
	if !mountPoint(hostRoot) {
		return nil, "host root not mounted; filesystem metrics unavailable"
	}
	raw, err := os.ReadFile(filepath.Join(s.HostProc, "1", "mountinfo"))
	if err != nil {
		return nil, "host mounts unavailable: " + err.Error()
	}
	mounts, err := hostcollector.ParseMountinfo(string(raw))
	if err != nil {
		return nil, "host mounts unavailable: " + err.Error()
	}
	dataDevice := ""
	if s.DataDir != "" {
		var st unix.Stat_t
		if unix.Stat(s.DataDir, &st) == nil {
			dev := uint64(st.Dev) // Dev is uint32 on some architectures
			dataDevice = fmt.Sprintf("%d:%d", unix.Major(dev), unix.Minor(dev))
		}
	}
	selected := hostcollector.SelectFilesystems(mounts, dataDevice)
	out := make([]metrics.FilesystemObservation, 0, len(selected)+1)
	skipped, firstSkipped := 0, ""
	haveData := false
	for _, m := range selected {
		var fs unix.Statfs_t
		if err := statfs(filepath.Join(hostRoot, m.Target), &fs); err != nil {
			skipped++
			if firstSkipped == "" {
				firstSkipped = m.Target + ": " + err.Error()
			}
			continue
		}
		haveData = haveData || m.Device() == dataDevice
		out = append(out, filesystemObservation(now, m.Source+"|"+m.Target, m.Target, m.FSType, fs))
	}
	// The data directory's filesystem is always reported: the storage budget's
	// free-disk floor depends on it, even when the host namespace does not
	// show it (a standalone binary on a private mount, for example).
	if !haveData && dataDevice != "" {
		var fs unix.Statfs_t
		if statfs(s.DataDir, &fs) == nil {
			out = append(out, filesystemObservation(now, "data|"+s.DataDir, s.DataDir, "", fs))
		}
	}
	if skipped > 0 {
		return out, fmt.Sprintf("%d of %d filesystems skipped: %s", skipped, len(selected), firstSkipped)
	}
	return out, ""
}

func filesystemObservation(now time.Time, key, target, fsType string, fs unix.Statfs_t) metrics.FilesystemObservation {
	total := int64(fs.Blocks) * int64(fs.Bsize)
	available := int64(fs.Bavail) * int64(fs.Bsize)
	free := int64(fs.Bfree) * int64(fs.Bsize)
	used := total - free
	var usedPct *float64
	if total > 0 {
		v := float64(used) * 100 / float64(total)
		usedPct = &v
	}
	var totalInodes, usedInodes *int64
	if fs.Files > 0 {
		ti := int64(fs.Files)
		ui := int64(fs.Files - fs.Ffree)
		totalInodes, usedInodes = &ti, &ui
	}
	return metrics.FilesystemObservation{
		At: now, MountKey: key, MountPoint: target, FSType: fsType,
		TotalBytes: &total, UsedBytes: &used, AvailableBytes: &available, UsedPercent: usedPct,
		InodesTotal: totalInodes, InodesUsed: usedInodes,
	}
}

// isMountPoint reports whether path is a mount point: "/" always is, any other
// path is when the reader's own mountinfo lists it or its device differs from
// its parent's.
func isMountPoint(path string) bool {
	path = filepath.Clean(path)
	if path == "/" {
		return true
	}
	if raw, err := os.ReadFile(selfMountinfo); err == nil {
		if mounts, err := hostcollector.ParseMountinfo(string(raw)); err == nil {
			for _, m := range mounts {
				if m.Target == path {
					return true
				}
			}
		}
	}
	var st, parent unix.Stat_t
	if unix.Stat(path, &st) != nil || unix.Stat(filepath.Dir(path), &parent) != nil {
		return false
	}
	return st.Dev != parent.Dev
}

type resourceGroup struct {
	identity                                     resources.Identity
	category, environment                        string
	infrastructure                               bool
	components                                   []metrics.ResourceComponent
	cpu, memory, rx, tx, read, write             float64
	cpuOK, memoryOK, rxOK, txOK, readOK, writeOK bool
	status                                       []metrics.ResourceStatus
	manualName, manualContext                    bool
}

// collectDocker samples every container. tick should carry a monotonic clock
// reading for the elapsed time of IO rates; its UTC copy stamps the result.
func (s *Sampler) collectDocker(ctx context.Context, tick time.Time, hostTotal *int64) ([]metrics.ResourceSnapshot, error) {
	now := tick.UTC()
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
		inspect, inspectErr := s.inspect(ctx, container.ID)
		if inspectErr != nil {
			return nil, inspectErr
		}
		identity := resources.ResolveMetadata(resources.Metadata{Labels: inspect.Labels, Environment: inspect.Environment, ContainerName: inspect.Name, Image: inspect.Image})
		resolvedCategory := category(inspect.Labels, identity)
		if identity.Context == "" {
			identity.Context = resolvedCategory
		}
		group := groups[identity.StableKey]
		if group == nil {
			group = &resourceGroup{identity: identity, category: resolvedCategory, environment: inspect.Labels["coolify.environment"], manualName: strings.TrimSpace(inspect.Labels["binnacle.name"]) != "", manualContext: strings.TrimSpace(inspect.Labels["binnacle.context"]) != ""}
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
		status := containerStatus(inspect.State, inspect.Health)
		group.status = append(group.status, status)
		component := metrics.ResourceComponent{ID: metrics.ContainerID(container.ID), Name: inspect.Name, Status: status, RuntimeState: inspect.State, HealthStatus: inspect.Health}
		if inspect.State != "running" {
			group.components = append(group.components, component)
			continue
		}
		stats, statsErr := s.Docker.Stats(ctx, container.ID)
		if statsErr != nil {
			return nil, statsErr
		}
		memory := dockercollector.NormalizeMemory(dockercollector.MemoryStats{Usage: stats.Memory.Usage, Limit: stats.Memory.Limit, InactiveFile: stats.Memory.InactiveFile, PIDs: stats.PIDs}, uint64Value(hostTotal))
		component.MemoryBytes = int64PtrFromFloat64Bytes(memory.WorkingSet)
		component.PIDs = stats.PIDs
		if memory.WorkingSet != nil {
			group.memory += *memory.WorkingSet
			group.memoryOK = true
		}
		if previous, ok := s.previousStats[container.ID]; ok {
			cpu := dockercollector.NormalizeCPU(dockercollector.CPUStats{Total: previous.value.CPU.TotalUsage, System: previous.value.CPU.SystemUsage, Online: previous.value.CPU.OnlineCPUs}, dockercollector.CPUStats{Total: stats.CPU.TotalUsage, System: stats.CPU.SystemUsage, Online: stats.CPU.OnlineCPUs}, stats.CPU.OnlineCPUs)
			component.CPUHostPercent = cpu.HostPercent
			if cpu.HostPercent != nil {
				group.cpu += *cpu.HostPercent
				group.cpuOK = true
			}
			io := dockercollector.NormalizeIO(dockerIO(previous.value.IO), dockerIO(stats.IO), tick.Sub(previous.at).Seconds())
			component.RXBPS, component.TXBPS, component.BlockReadBPS, component.BlockWriteBPS = io.RX, io.TX, io.Read, io.Write
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
		group.components = append(group.components, component)
		s.previousStats[container.ID] = dockerSample{value: stats, at: tick}
	}
	result := make([]metrics.ResourceSnapshot, 0, len(groups))
	for stable, group := range groups {
		id := resourceID(stable)
		result = append(result, metrics.ResourceSnapshot{ID: id, Name: group.identity.Name, Status: resources.RollupStatus(group.status), CPUHostPercent: number(group.cpu, group.cpuOK), MemoryBytes: integer(group.memory, group.memoryOK), RXBPS: number(group.rx, group.rxOK), TXBPS: number(group.tx, group.txOK), BlockReadBPS: number(group.read, group.readOK), BlockWriteBPS: number(group.write, group.writeOK), LastSeenAt: now, Category: group.category, Context: group.identity.Context, Project: group.identity.Project, Environment: group.environment, Infrastructure: group.infrastructure, Components: group.components, StableKey: stable, SourceKind: group.identity.Source, ManualName: group.manualName, ManualContext: group.manualContext})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func dockerIO(v dockerapi.IOCounters) dockercollector.IOCounters {
	return dockercollector.IOCounters{RX: v.RX, TX: v.TX, Read: v.Read, Write: v.Write, Network: v.Network}
}

func (s *Sampler) inspect(ctx context.Context, id string) (dockerapi.Inspect, error) {
	if s.Cache != nil {
		if v, ok := s.Cache.Get(id); ok {
			return dockerapi.Inspect{ID: v.ID, Name: v.Name, Image: v.Image, Created: v.Created, State: v.State, Health: v.Health, Labels: v.Labels, Environment: v.Environment, Networks: v.Networks, Mounts: v.Mounts}, nil
		}
	}
	inspect, err := s.Docker.Inspect(ctx, id)
	if err != nil {
		return dockerapi.Inspect{}, err
	}
	if s.Cache != nil {
		s.Cache.Set(dockercollector.Metadata{ID: inspect.ID, Name: inspect.Name, Image: inspect.Image, Created: inspect.Created, State: inspect.State, Health: inspect.Health, Labels: inspect.Labels, Environment: inspect.Environment, Networks: inspect.Networks, Mounts: inspect.Mounts})
	}
	return inspect, nil
}

func containerStatus(state, health string) metrics.ResourceStatus {
	switch state {
	case "running":
		if health == "unhealthy" {
			return metrics.StatusDown
		}
		if health == "starting" {
			return metrics.StatusUnknown
		}
		return metrics.StatusHealthy
	case "paused":
		return metrics.StatusPaused
	case "exited", "dead":
		return metrics.StatusDown
	}
	return metrics.StatusUnknown
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
	if value := strings.ToLower(labels["binnacle.category"]); resources.ValidCategory(value) {
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
func int64PtrFromFloat64Bytes(v *float64) *int64 {
	if v == nil {
		return nil
	}
	n := int64(*v)
	return &n
}
