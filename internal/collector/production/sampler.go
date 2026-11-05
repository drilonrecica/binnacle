// SPDX-License-Identifier: AGPL-3.0-only
package production

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math/rand/v2"
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
	Retain(keep map[string]struct{}) int
	Apply(e dockerapi.Event) dockercollector.EventEffect
}

// Docker state and events constants (ADR 022 §2, §5). They are constants so
// the bounds cannot be configured away.
const (
	// metadataMaxAge forces a re-inspect of containers whose cached metadata
	// is older than this, even when no event or state change asked for one.
	metadataMaxAge = 10 * time.Minute
	// eventCallTimeout bounds the Inspect that resolves an uncached
	// container's resource for an event.
	eventCallTimeout = 5 * time.Second
	// eventQueueSize bounds the events forwarded from the subscription to the
	// sampler loop; when full the oldest is dropped and a re-discover asked.
	eventQueueSize = 256
	// pendingEventLimit bounds the normalised events waiting for the next
	// published snapshot.
	pendingEventLimit = 128
	// Resubscription backoff: doubles from the minimum to the cap with ±20 %
	// jitter, and resets once a stream stayed open for eventsStableAfter.
	eventsMinBackoff  = time.Second
	eventsMaxBackoff  = 60 * time.Second
	eventsStableAfter = 60 * time.Second
)

// Docker tick bounds (ADR 022 §2, §3). Constants, like the ones above.
const (
	// dockerTickDeadline bounds one whole Docker tick; containers not reached
	// before it count as per-container failures.
	dockerTickDeadline = 20 * time.Second
	// skipLogInterval coalesces the warning for a skipped container to one
	// line per container per interval.
	skipLogInterval = time.Minute
	// inventoryRefreshInterval is how often an unchanged host and resource
	// set is rewritten (STO-4). A changed set is written on the tick that saw
	// it. The refresh keeps resources.last_seen_at well inside
	// inventoryArchiveAfter, so a resource still present is never archived.
	inventoryRefreshInterval = time.Minute
	// inventoryArchiveAfter archives resources unseen for this long.
	inventoryArchiveAfter = 5 * time.Minute
)

// Sampler runs the host loop and the Docker loop in separate goroutines and
// publishes one snapshot merged from the latest result of each (ADR 022 §1),
// so a slow or hung Docker daemon never delays host metrics.
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
	HostRoot string
	DataDir  string
	// Interval is the host loop interval; DockerInterval the Docker loop's,
	// which falls back to Interval when nil.
	Interval             func() time.Duration
	DockerInterval       func() time.Duration
	MaxDockerConcurrency int
	// Logger receives collector diagnostics; nil means slog.Default().
	Logger *slog.Logger
	Store  interface {
		StoreInventory(context.Context, storage.Inventory) error
	}
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// mu guards merged, the only state both loops touch.
	mu     sync.Mutex
	merged mergedState

	// Host loop state.
	previousCPU       hostcollector.CPUCounters
	haveCPU           bool
	previousNetwork   hostcollector.NetworkCounters
	previousNetworkAt time.Time
	previousDisk      hostcollector.DiskCounters
	previousDiskAt    time.Time
	hostFailures      int
	hostFreshAt       time.Time
	hostNanos         atomic.Int64

	// Docker loop state.
	previousStats  map[string]dockerSample
	lastResources  []metrics.ResourceSnapshot
	dockerFailures int
	dockerFreshAt  time.Time
	dockerNanos    atomic.Int64
	// skipLogged holds when a skipped container was last logged.
	skipLogged map[string]time.Time
	// storedInventory is the resource set last written to the store and
	// storedInventoryAt when; nil until the first successful write.
	storedInventory   map[string]storage.Resource
	storedInventoryAt time.Time
	// rediscover asks the next Docker tick to re-inspect every container: set
	// after an events resubscription and after a dropped event.
	rediscover atomic.Bool
	// eventsDisconnected is true while the events stream is down and being
	// resubscribed; the docker collector reason says so.
	eventsDisconnected atomic.Bool
	eventsDropped      atomic.Int64

	// eventsBackoff overrides the resubscription backoff bounds in tests:
	// minimum, cap and the open duration after which it resets.
	eventsBackoff *[3]time.Duration
	// callTimeout and tickDeadline override the Docker bounds in tests.
	callTimeout, tickDeadline time.Duration
	// statfs and mountPoint replace unix.Statfs and isMountPoint in tests.
	statfs     func(string, *unix.Statfs_t) error
	mountPoint func(string) bool
}

// mergedState is the latest result of each loop plus the events waiting for
// the next publish.
type mergedState struct {
	hostTicked   bool
	host         metrics.HostObservation
	boot         string
	hostHealth   metrics.CollectorHealth
	dockerTicked bool
	resources    []metrics.ResourceSnapshot
	dockerHealth metrics.CollectorHealth
	pending      []metrics.Event
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
	s.wg.Add(2)
	go func() { defer s.wg.Done(); s.hostLoop(ctx) }()
	go func() { defer s.wg.Done(); s.dockerLoop(ctx) }()
	return nil
}

// Stop cancels both loops and waits for them within ctx before closing the
// Docker client, so an in-flight call ends through its context rather than on
// a closed transport (ADR 022 §7).
func (s *Sampler) Stop(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		s.logger().Warn("collector loops did not stop before the shutdown deadline")
	}
	if closer, ok := s.Docker.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

// hostLoop collects the host, publishes, then waits one host interval.
func (s *Sampler) hostLoop(ctx context.Context) {
	for {
		s.hostTick()
		if !sleep(ctx, interval(s.Interval)) {
			return
		}
	}
}

// dockerLoop collects Docker, publishes, then waits one Docker interval while
// applying events as they arrive. It owns the events goroutine.
func (s *Sampler) dockerLoop(ctx context.Context) {
	var dockerEvents <-chan dockerapi.Event
	if s.Docker != nil {
		queue := make(chan dockerapi.Event, eventQueueSize)
		s.wg.Add(1)
		go func() { defer s.wg.Done(); s.watchEvents(ctx, queue) }()
		dockerEvents = queue
	}
	every := s.DockerInterval
	if every == nil {
		every = s.Interval
	}
	for {
		s.dockerTick(ctx)
		timer := time.NewTimer(interval(every))
	wait:
		for {
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case event := <-dockerEvents:
				if normalized, accepted := s.handleDockerEvent(ctx, event); accepted {
					s.addPending(normalized)
				}
			case <-timer.C:
				break wait
			}
		}
	}
}

// interval returns the configured loop interval, 2 s when unset or below 1 s.
func interval(configured func() time.Duration) time.Duration {
	if configured != nil {
		if v := configured(); v >= time.Second {
			return v
		}
	}
	return 2 * time.Second
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// hostTick reads one host sample, merges it and publishes.
func (s *Sampler) hostTick() {
	started := time.Now()
	defer func() { s.hostNanos.Store(time.Since(started).Nanoseconds()) }()
	// started keeps its monotonic reading for elapsed time between ticks;
	// UTC() strips it, so only the stored and published copy is UTC (COL-8).
	now := started.UTC()
	host, filesystems, boot, fsReason, err := s.collectHost(started)
	if len(filesystems) > 0 {
		s.Engine.PublishFilesystems(filesystems)
	}
	var h metrics.CollectorHealth
	if err != nil {
		s.hostFailures++
		h = health("host", s.hostFailures, err, s.hostFreshAt)
	} else {
		s.hostFailures = 0
		s.hostFreshAt = now
		h = health("host", 0, nil, now)
		if fsReason != "" {
			h.State, h.Reason = metrics.CollectorDegraded, fsReason
		}
	}
	s.mu.Lock()
	s.merged.hostTicked = true
	s.merged.host, s.merged.hostHealth = host, h
	if err == nil {
		s.merged.boot = boot
	}
	s.mu.Unlock()
	s.publish()
}

// dockerTick runs one bounded Docker collection, merges it and publishes.
// Only a List failure fails the tick; then the last resource set is
// republished as stale, never as fresh measurements (COL-5).
func (s *Sampler) dockerTick(ctx context.Context) {
	started := time.Now()
	defer func() { s.dockerNanos.Store(time.Since(started).Nanoseconds()) }()
	now := started.UTC()
	deadline := s.tickDeadline
	if deadline <= 0 {
		deadline = dockerTickDeadline
	}
	tickCtx, cancel := context.WithTimeout(ctx, deadline)
	resources, partial, err := s.collectDocker(tickCtx, started, s.hostMemoryTotal())
	cancel()
	if ctx.Err() != nil {
		return
	}
	var h metrics.CollectorHealth
	if err != nil {
		s.dockerFailures++
		h = health("docker", s.dockerFailures, err, s.dockerFreshAt)
		resources = staleResources(s.lastResources)
	} else {
		s.dockerFailures = 0
		s.dockerFreshAt = now
		h = health("docker", 0, nil, now)
		h.Reason = partial
		if s.eventsDisconnected.Load() {
			// State still comes from List every tick; only the fast path
			// between ticks is missing (ADR 022 §5).
			h.Reason = joinReason(h.Reason, "events disconnected")
		}
		s.lastResources = resources
		s.storeResources(ctx, resources, now)
	}
	s.mu.Lock()
	s.merged.dockerTicked = true
	s.merged.resources, s.merged.dockerHealth = resources, h
	s.mu.Unlock()
	s.publish()
}

// storeResources records the host and resource identities of a successful
// tick in one transaction, and only when the set changed or the last write is
// inventoryRefreshInterval old (STO-4). A failed write is retried next tick.
func (s *Sampler) storeResources(ctx context.Context, resources []metrics.ResourceSnapshot, now time.Time) {
	if s.Store == nil {
		return
	}
	current := make(map[string]storage.Resource, len(resources))
	list := make([]storage.Resource, 0, len(resources))
	for _, resource := range resources {
		r := storage.Resource{ID: string(resource.ID), HostID: "host", StableKey: resource.StableKey, SourceKind: resource.SourceKind, Name: resource.Name, Context: resource.Context, ProjectName: resource.Project, EnvironmentName: resource.Environment, Category: resource.Category, Status: string(resource.Status)}
		current[r.ID] = r
		list = append(list, r)
	}
	if s.storedInventory != nil && maps.Equal(current, s.storedInventory) && now.Sub(s.storedInventoryAt) < inventoryRefreshInterval {
		return
	}
	inventory := storage.Inventory{HostID: "host", HostIdentity: storage.HostIdentity("", "binnacle-local-host"), HostName: "Server", Resources: list, ArchiveBefore: now.Add(-inventoryArchiveAfter)}
	if err := s.Store.StoreInventory(ctx, inventory); err != nil {
		s.logger().Warn("storing host and resource identities failed", "resources", len(list), "error", err.Error())
		return
	}
	s.storedInventory, s.storedInventoryAt = current, now
}

// staleResources copies the last successful resource set with identities
// only: every metric nil, status unknown, LastSeenAt unchanged, Stale set.
func staleResources(last []metrics.ResourceSnapshot) []metrics.ResourceSnapshot {
	out := make([]metrics.ResourceSnapshot, 0, len(last))
	for _, r := range last {
		stale := metrics.ResourceSnapshot{ID: r.ID, Name: r.Name, Status: metrics.StatusUnknown, LastSeenAt: r.LastSeenAt, Stale: true, Category: r.Category, Context: r.Context, Project: r.Project, Environment: r.Environment, Infrastructure: r.Infrastructure, StableKey: r.StableKey, SourceKind: r.SourceKind, ManualName: r.ManualName, ManualContext: r.ManualContext}
		for _, c := range r.Components {
			stale.Components = append(stale.Components, metrics.ResourceComponent{ID: c.ID, Name: c.Name, Status: metrics.StatusUnknown})
		}
		out = append(out, stale)
	}
	return out
}

func (s *Sampler) hostMemoryTotal() *int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.merged.host.MemoryTotalBytes
}

// addPending queues one event for the next publish, dropping the oldest
// beyond pendingEventLimit.
func (s *Sampler) addPending(event metrics.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.merged.pending) == pendingEventLimit {
		s.merged.pending = s.merged.pending[1:]
	}
	s.merged.pending = append(s.merged.pending, event)
}

// publish sends the merged snapshot. Nothing is published before the first
// host tick, so a snapshot never carries an empty host by accident.
// Snapshot.At is the publish time; HostObservation.At and LastSeenAt carry
// the time of the tick that produced each half (ADR 022 §1).
func (s *Sampler) publish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := &s.merged
	if !m.hostTicked {
		return
	}
	collectors := map[string]metrics.CollectorHealth{"host": m.hostHealth}
	if m.dockerTicked {
		collectors["docker"] = m.dockerHealth
	}
	pending := m.pending
	m.pending = nil
	s.Engine.Publish(metrics.Snapshot{At: time.Now().UTC(), BootIdentity: metrics.BootIdentity(m.boot), Host: m.host, Resources: m.resources, Collectors: collectors}, pending...)
}

// watchEvents keeps one Docker events subscription open and forwards its
// events to queue. When the stream closes for any reason but ctx ending, it
// resubscribes with exponential backoff and asks the next tick for a full
// re-discover, since anything may have changed while it was down.
func (s *Sampler) watchEvents(ctx context.Context, queue chan dockerapi.Event) {
	minimum, maximum, stableAfter := eventsMinBackoff, eventsMaxBackoff, eventsStableAfter
	if s.eventsBackoff != nil {
		minimum, maximum, stableAfter = s.eventsBackoff[0], s.eventsBackoff[1], s.eventsBackoff[2]
	}
	delay := minimum
	for subscription := 0; ; subscription++ {
		if subscription > 0 {
			s.rediscover.Store(true)
		}
		opened := time.Now()
		stream := s.Docker.Events(ctx)
		s.eventsDisconnected.Store(false)
	forward:
		for stream != nil {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-stream:
				if !ok {
					break forward
				}
				s.enqueueEvent(queue, event)
			}
		}
		if ctx.Err() != nil {
			return
		}
		s.eventsDisconnected.Store(true)
		if time.Since(opened) >= stableAfter {
			delay = minimum
		}
		wait := jitter(delay)
		s.logger().Warn("docker events stream closed; resubscribing", "retry_in", wait.String())
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(delay*2, maximum)
	}
}

// enqueueEvent forwards one event without blocking the subscription: when the
// queue is full the oldest event is dropped, counted, and a re-discover asked,
// so a lost event delays a metadata refresh by one tick but never loses state.
func (s *Sampler) enqueueEvent(queue chan dockerapi.Event, event dockerapi.Event) {
	select {
	case queue <- event:
		return
	default:
	}
	select {
	case <-queue:
	default:
	}
	s.eventsDropped.Add(1)
	s.rediscover.Store(true)
	select {
	case queue <- event:
	default:
	}
}

// jitter spreads d by ±20 %.
func jitter(d time.Duration) time.Duration {
	return time.Duration(float64(d) * (0.8 + 0.4*rand.Float64()))
}

// handleDockerEvent normalises one Docker event, attaches the resource id of
// its container (EVT-3), and applies it to the metadata cache. The resource is
// resolved before the cache is updated, because destroy removes the entry.
func (s *Sampler) handleDockerEvent(ctx context.Context, event dockerapi.Event) (metrics.Event, bool) {
	normalized, accepted := events.NormalizeDocker(event, false)
	if accepted {
		normalized.ResourceID = s.eventResource(ctx, event)
	}
	if s.Cache != nil && s.Cache.Apply(event).DropStats {
		delete(s.previousStats, event.ID)
	}
	return normalized, accepted
}

// eventResource resolves the container of an event to its resource id through
// the same identity resolution as the tick. A container not cached yet (an
// event seen before the first tick that lists it) is inspected once. When
// resolution fails the event keeps an empty resource id.
func (s *Sampler) eventResource(ctx context.Context, event dockerapi.Event) metrics.ResourceID {
	if s.Cache != nil {
		if v, ok := s.Cache.Get(event.ID); ok {
			return containerResourceID(v.Inspect())
		}
	}
	if s.Docker == nil || strings.EqualFold(event.Action, "destroy") {
		s.logger().Debug("docker event without a known container", "container", event.ID, "action", event.Action)
		return ""
	}
	request, cancel := context.WithTimeout(ctx, eventCallTimeout)
	inspect, err := s.Docker.Inspect(request, event.ID)
	cancel()
	if err != nil {
		s.logger().Debug("docker event container could not be resolved", "container", event.ID, "action", event.Action, "error", err.Error())
		return ""
	}
	if s.Cache != nil {
		s.Cache.Set(dockercollector.MetadataFromInspect(inspect, time.Now()))
	}
	return containerResourceID(inspect)
}

func (s *Sampler) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

// CollectionDuration is the longer of the latest host and Docker tick.
func (s *Sampler) CollectionDuration() time.Duration {
	if s == nil {
		return 0
	}
	return max(s.HostDuration(), s.DockerDuration())
}

// HostDuration is the duration of the latest host tick.
func (s *Sampler) HostDuration() time.Duration { return time.Duration(s.hostNanos.Load()) }

// DockerDuration is the duration of the latest Docker tick.
func (s *Sampler) DockerDuration() time.Duration { return time.Duration(s.dockerNanos.Load()) }

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
// Only a List error is returned. An Inspect or Stats error, including the
// deadline of ctx, skips that one container: its component is unknown with nil
// metrics and its previous sample is dropped. The returned reason counts the
// skipped containers and is empty when none was skipped (ADR 022 §3).
func (s *Sampler) collectDocker(ctx context.Context, tick time.Time, hostTotal *int64) ([]metrics.ResourceSnapshot, string, error) {
	now := tick.UTC()
	if s.Docker == nil {
		return nil, "", errors.New("Docker client is not configured")
	}
	request, cancel := context.WithTimeout(ctx, s.dockerCallTimeout())
	containers, err := s.Docker.List(request)
	cancel()
	if err != nil {
		return nil, "", err
	}
	if s.previousStats == nil {
		s.previousStats = map[string]dockerSample{}
	}
	// The list is the source of truth for which containers exist: every
	// per-container map shrinks to it on every tick (COL-8).
	present := make(map[string]struct{}, len(containers))
	for _, container := range containers {
		present[container.ID] = struct{}{}
	}
	for id := range s.previousStats {
		if _, ok := present[id]; !ok {
			delete(s.previousStats, id)
		}
	}
	for id := range s.skipLogged {
		if _, ok := present[id]; !ok {
			delete(s.skipLogged, id)
		}
	}
	if s.Cache != nil {
		s.Cache.Retain(present)
	}

	full := s.rediscover.Swap(false)
	inspects := make(map[string]dockerapi.Inspect, len(containers))
	var toInspect []string
	for _, container := range containers {
		if v, ok := s.cachedInspect(container, tick, full); ok {
			inspects[container.ID] = v
		} else {
			toInspect = append(toInspect, container.ID)
		}
	}
	listed := make(map[string]dockerapi.Container, len(containers))
	for _, container := range containers {
		listed[container.ID] = container
	}
	failures := map[string]error{}
	for id, result := range dockercollector.Collect(ctx, toInspect, s.MaxDockerConcurrency, s.dockerCallTimeout(), s.Docker.Inspect) {
		if result.Err != nil {
			failures[id] = result.Err
			if full {
				s.rediscover.Store(true)
			}
			// Cached metadata, though due for a refresh, still places the
			// container in its resource; without it the container is omitted.
			if s.Cache != nil {
				if v, ok := s.Cache.Get(id); ok {
					inspect := v.Inspect()
					if state := listed[id].State; state != "" {
						inspect.State = state
					}
					inspects[id] = inspect
				}
			}
			continue
		}
		inspect := result.Value
		if state := listed[id].State; state != "" {
			inspect.State = state
		}
		if s.Cache != nil {
			s.Cache.Set(dockercollector.MetadataFromInspect(inspect, tick))
		}
		inspects[id] = inspect
	}

	var running []string
	for _, container := range containers {
		if inspect, ok := inspects[container.ID]; ok && failures[container.ID] == nil && inspect.State == "running" {
			running = append(running, container.ID)
		}
	}
	stats := dockercollector.Collect(ctx, running, s.MaxDockerConcurrency, s.dockerCallTimeout(), s.Docker.Stats)
	for id, result := range stats {
		if result.Err != nil {
			failures[id] = result.Err
		}
	}

	groups := map[string]*resourceGroup{}
	for _, container := range containers {
		inspect, ok := inspects[container.ID]
		if !ok {
			delete(s.previousStats, container.ID)
			continue
		}
		identity := containerIdentity(inspect)
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
		if failures[container.ID] != nil {
			// Skipped: unknown with nil metrics, and no rate across the gap.
			delete(s.previousStats, container.ID)
			group.status = append(group.status, metrics.StatusUnknown)
			group.components = append(group.components, metrics.ResourceComponent{ID: metrics.ContainerID(container.ID), Name: inspect.Name, Status: metrics.StatusUnknown, RuntimeState: container.State})
			continue
		}
		status := containerStatus(inspect.State, inspect.Health)
		group.status = append(group.status, status)
		component := metrics.ResourceComponent{ID: metrics.ContainerID(container.ID), Name: inspect.Name, Status: status, RuntimeState: inspect.State, HealthStatus: inspect.Health}
		if inspect.State != "running" {
			// No rate across a stop, even when the die event was lost.
			delete(s.previousStats, container.ID)
			group.components = append(group.components, component)
			continue
		}
		sample := stats[container.ID].Value
		s.addStats(group, &component, s.previousStats[container.ID], sample, tick, hostTotal)
		group.components = append(group.components, component)
		s.previousStats[container.ID] = dockerSample{value: sample, at: tick}
	}
	result := make([]metrics.ResourceSnapshot, 0, len(groups))
	for stable, group := range groups {
		id := resourceID(stable)
		result = append(result, metrics.ResourceSnapshot{ID: id, Name: group.identity.Name, Status: resources.RollupStatus(group.status), CPUHostPercent: number(group.cpu, group.cpuOK), MemoryBytes: integer(group.memory, group.memoryOK), RXBPS: number(group.rx, group.rxOK), TXBPS: number(group.tx, group.txOK), BlockReadBPS: number(group.read, group.readOK), BlockWriteBPS: number(group.write, group.writeOK), LastSeenAt: now, Category: group.category, Context: group.identity.Context, Project: group.identity.Project, Environment: group.environment, Infrastructure: group.infrastructure, Components: group.components, StableKey: stable, SourceKind: group.identity.Source, ManualName: group.manualName, ManualContext: group.manualContext})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, s.skipReason(ctx, containers, failures, tick), nil
}

// addStats fills a running component from its sample and adds it to the
// group's aggregates; rates need the container's previous sample.
func (s *Sampler) addStats(group *resourceGroup, component *metrics.ResourceComponent, previous dockerSample, stats dockerapi.Stats, tick time.Time, hostTotal *int64) {
	memory := dockercollector.NormalizeMemory(dockercollector.MemoryStats{Usage: stats.Memory.Usage, Limit: stats.Memory.Limit, InactiveFile: stats.Memory.InactiveFile, PIDs: stats.PIDs}, uint64Value(hostTotal))
	component.MemoryBytes = int64PtrFromFloat64Bytes(memory.WorkingSet)
	component.PIDs = stats.PIDs
	if memory.WorkingSet != nil {
		group.memory += *memory.WorkingSet
		group.memoryOK = true
	}
	if previous.at.IsZero() {
		return
	}
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

// skipReason logs each skipped container at most once per skipLogInterval and
// summarises the skips for the collector reason, for example
// "3 of 30 containers skipped: c1: no such container".
func (s *Sampler) skipReason(ctx context.Context, containers []dockerapi.Container, failures map[string]error, tick time.Time) string {
	if len(failures) == 0 {
		return ""
	}
	if s.skipLogged == nil {
		s.skipLogged = map[string]time.Time{}
	}
	first := ""
	for _, container := range containers {
		err := failures[container.ID]
		if err == nil {
			continue
		}
		if first == "" {
			first = shortID(container.ID) + ": " + err.Error()
		}
		if last, ok := s.skipLogged[container.ID]; !ok || tick.Sub(last) >= skipLogInterval {
			s.skipLogged[container.ID] = tick
			s.logger().Warn("docker container skipped this tick", "container", container.ID, "error", err.Error())
		}
	}
	reason := fmt.Sprintf("%d of %d containers skipped: %s", len(failures), len(containers), first)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		reason += "; tick deadline reached"
	}
	return reason
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func (s *Sampler) dockerCallTimeout() time.Duration {
	if s.callTimeout > 0 {
		return s.callTimeout
	}
	return dockercollector.CallTimeout
}

func dockerIO(v dockerapi.IOCounters) dockercollector.IOCounters {
	return dockercollector.IOCounters{RX: v.RX, TX: v.TX, Read: v.Read, Write: v.Write, Network: v.Network}
}

// cachedInspect returns the cached metadata of one listed container unless
// the container is new, its listed state differs from the cached one, an
// event marked it, it is older than metadataMaxAge, or full asks for a
// re-discover (ADR 022 §2); then the container must be inspected. The listed
// state always wins over the inspected one, so the runtime state is at most
// one tick old even when events are lost.
func (s *Sampler) cachedInspect(container dockerapi.Container, tick time.Time, full bool) (dockerapi.Inspect, bool) {
	if s.Cache == nil || full {
		return dockerapi.Inspect{}, false
	}
	v, ok := s.Cache.Get(container.ID)
	if !ok || v.Reinspect || tick.Sub(v.InspectedAt) >= metadataMaxAge || (container.State != "" && container.State != v.State) {
		return dockerapi.Inspect{}, false
	}
	return v.Inspect(), true
}

// containerIdentity resolves a container's logical resource identity.
func containerIdentity(inspect dockerapi.Inspect) resources.Identity {
	return resources.ResolveMetadata(resources.Metadata{Labels: inspect.Labels, Environment: inspect.Environment, ContainerName: inspect.Name, Image: inspect.Image})
}

// containerResourceID is the resource id the tick publishes the container
// under; events use it so their counters group by resource.
func containerResourceID(inspect dockerapi.Inspect) metrics.ResourceID {
	return resourceID(containerIdentity(inspect).StableKey)
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
