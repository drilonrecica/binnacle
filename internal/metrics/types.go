// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"fmt"
	"strings"
	"time"
)

type ResourceID string
type ContainerID string
type BootIdentity string
type Sequence uint64
type Unit string

const (
	UnitBytes          Unit = "bytes"
	UnitBytesPerSecond Unit = "bytes_per_second"
	UnitPercent        Unit = "percent"
	UnitCount          Unit = "count"
)

func (u Unit) Valid() bool {
	switch u {
	case UnitBytes, UnitBytesPerSecond, UnitPercent, UnitCount:
		return true
	}
	return false
}
func (id ResourceID) Valid() bool  { return strings.HasPrefix(string(id), "res_") && len(id) > 4 }
func (id ContainerID) Valid() bool { return len(id) >= 12 }

type ResourceStatus string

const (
	StatusHealthy  ResourceStatus = "healthy"
	StatusPaused   ResourceStatus = "paused"
	StatusUnknown  ResourceStatus = "unknown"
	StatusDegraded ResourceStatus = "degraded"
	StatusDown     ResourceStatus = "down"
	StatusArchived ResourceStatus = "archived"
)

func (s ResourceStatus) Valid() bool {
	switch s {
	case StatusHealthy, StatusPaused, StatusUnknown, StatusDegraded, StatusDown, StatusArchived:
		return true
	}
	return false
}

type CollectorState string

const (
	CollectorHealthy  CollectorState = "healthy"
	CollectorDegraded CollectorState = "degraded"
	CollectorDown     CollectorState = "down"
	CollectorUnknown  CollectorState = "unknown"
)

func (s CollectorState) Valid() bool {
	switch s {
	case CollectorHealthy, CollectorDegraded, CollectorDown, CollectorUnknown:
		return true
	}
	return false
}

type HostObservation struct {
	At                   time.Time `json:"at"`
	CPUPercent           *float64  `json:"cpuPct"`
	CPUUserPercent       *float64  `json:"cpuUserPct"`
	CPUSystemPercent     *float64  `json:"cpuSystemPct"`
	CPUIOWaitPercent     *float64  `json:"cpuIOWaitPct"`
	CPUStealPercent      *float64  `json:"cpuStealPct"`
	MemoryUsedBytes      *int64    `json:"memoryUsedBytes"`
	MemoryTotalBytes     *int64    `json:"memoryTotalBytes"`
	MemoryAvailableBytes *int64    `json:"memoryAvailableBytes"`
	MemoryPercent        *float64  `json:"memoryPct"`
	MemoryCachedBytes    *int64    `json:"memoryCachedBytes"`
	MemoryBuffersBytes   *int64    `json:"memoryBuffersBytes"`
	SwapUsedBytes        *int64    `json:"swapUsedBytes"`
	SwapTotalBytes       *int64    `json:"swapTotalBytes"`
	SwapPercent          *float64  `json:"swapPct"`
	Load1                *float64  `json:"load1"`
	Load5                *float64  `json:"load5"`
	Load15               *float64  `json:"load15"`
	NetworkRXBPS         *float64  `json:"networkRxBps"`
	NetworkTXBPS         *float64  `json:"networkTxBps"`
	NetworkRXPacketsPS   *float64  `json:"networkRxPacketsPs"`
	NetworkTXPacketsPS   *float64  `json:"networkTxPacketsPs"`
	NetworkRXErrorsDelta *int64    `json:"networkRxErrorsDelta"`
	NetworkTXErrorsDelta *int64    `json:"networkTxErrorsDelta"`
	NetworkRXDropsDelta  *int64    `json:"networkRxDropsDelta"`
	NetworkTXDropsDelta  *int64    `json:"networkTxDropsDelta"`
	DiskReadBPS          *float64  `json:"diskReadBps"`
	DiskWriteBPS         *float64  `json:"diskWriteBps"`
	DiskReadIOPS         *float64  `json:"diskReadIops"`
	DiskWriteIOPS        *float64  `json:"diskWriteIops"`
	DiskUsedBytes        *int64    `json:"diskUsedBytes"`
	DiskTotalBytes       *int64    `json:"diskTotalBytes"`
	UptimeSeconds        *float64  `json:"uptimeSeconds"`
	// MemoryTrend is the time-to-full estimate of used memory; the engine
	// attaches it, collectors leave it zero.
	MemoryTrend Trend `json:"memoryTrend"`

	// Detail for the Server › Live view. Live only: none of it is stored as
	// history, and each list is bounded (see the host collector).
	CPUNicePercent      *float64         `json:"cpuNicePct,omitempty"`
	CPUIRQPercent       *float64         `json:"cpuIrqPct,omitempty"`
	CPUSoftIRQPercent   *float64         `json:"cpuSoftirqPct,omitempty"`
	CPUIdlePercent      *float64         `json:"cpuIdlePct,omitempty"`
	CPUCores            []float64        `json:"cpuCores,omitempty"`
	ContextSwitchesPS   *float64         `json:"contextSwitchesPs,omitempty"`
	InterruptsPS        *float64         `json:"interruptsPs,omitempty"`
	SoftInterruptsPS    *float64         `json:"softInterruptsPs,omitempty"`
	ProcsRunning        *uint64          `json:"procsRunning,omitempty"`
	ProcsBlocked        *uint64          `json:"procsBlocked,omitempty"`
	MemoryFreeBytes     *int64           `json:"memoryFreeBytes,omitempty"`
	MemoryActiveBytes   *int64           `json:"memoryActiveBytes,omitempty"`
	MemoryInactiveBytes *int64           `json:"memoryInactiveBytes,omitempty"`
	Interfaces          []InterfaceRate  `json:"interfaces,omitempty"`
	Disks               []DiskDeviceRate `json:"disks,omitempty"`
}

// InterfaceRate is one network interface's throughput between two samples.
type InterfaceRate struct {
	Name  string   `json:"name"`
	RXBPS *float64 `json:"rxBps"`
	TXBPS *float64 `json:"txBps"`
}

// DiskDeviceRate is one block device's throughput between two samples.
type DiskDeviceRate struct {
	Name      string   `json:"name"`
	ReadBPS   *float64 `json:"readBps"`
	WriteBPS  *float64 `json:"writeBps"`
	ReadIOPS  *float64 `json:"readIops"`
	WriteIOPS *float64 `json:"writeIops"`
}

// HostIdentity describes the machine; it is read once and does not change
// while Binnacle runs. Empty fields were not readable.
type HostIdentity struct {
	Hostname string `json:"hostname,omitempty"`
	OS       string `json:"os,omitempty"`
	Kernel   string `json:"kernel,omitempty"`
	CPUModel string `json:"cpuModel,omitempty"`
	CPUCores int    `json:"cpuCores,omitempty"`
}
type ContainerObservation struct {
	ID             ContainerID    `json:"id"`
	ResourceID     ResourceID     `json:"resourceId"`
	At             time.Time      `json:"at"`
	CPUHostPercent *float64       `json:"cpuHostPct"`
	MemoryBytes    *int64         `json:"memoryBytes"`
	RXBPS          *float64       `json:"rxBps"`
	TXBPS          *float64       `json:"txBps"`
	BlockReadBPS   *float64       `json:"blockReadBps"`
	BlockWriteBPS  *float64       `json:"blockWriteBps"`
	Status         ResourceStatus `json:"status"`
}
type ResourceComponent struct {
	ID             ContainerID    `json:"id"`
	Name           string         `json:"name"`
	Status         ResourceStatus `json:"status"`
	RuntimeState   string         `json:"runtimeState,omitempty"`
	HealthStatus   string         `json:"healthStatus,omitempty"`
	CPUHostPercent *float64       `json:"cpuHostPct,omitempty"`
	MemoryBytes    *int64         `json:"memoryBytes,omitempty"`
	RXBPS          *float64       `json:"rxBps,omitempty"`
	TXBPS          *float64       `json:"txBps,omitempty"`
	BlockReadBPS   *float64       `json:"blockReadBps,omitempty"`
	BlockWriteBPS  *float64       `json:"blockWriteBps,omitempty"`
	PIDs           *uint64        `json:"pids,omitempty"`
	// Live-view detail: the memory limit in force, when the container
	// started, its image and published ports. Kept out of the live frames,
	// which every open page receives every 2 s; GET /api/v1/server/containers
	// serves them to the one view that shows them.
	MemoryLimitBytes *int64   `json:"-"`
	StartedAt        string   `json:"-"`
	Image            string   `json:"-"`
	Ports            []string `json:"-"`
}

// ContainerDetail is the slowly changing detail of one container.
type ContainerDetail struct {
	ID               ContainerID `json:"id"`
	MemoryLimitBytes *int64      `json:"memoryLimitBytes,omitempty"`
	StartedAt        string      `json:"startedAt,omitempty"`
	Image            string      `json:"image,omitempty"`
	Ports            []string    `json:"ports,omitempty"`
}

// ContainerDetails lists the detail of every container in a snapshot.
func ContainerDetails(resources []ResourceSnapshot) []ContainerDetail {
	out := []ContainerDetail{}
	for _, resource := range resources {
		for _, c := range resource.Components {
			out = append(out, ContainerDetail{ID: c.ID, MemoryLimitBytes: c.MemoryLimitBytes, StartedAt: c.StartedAt, Image: c.Image, Ports: c.Ports})
		}
	}
	return out
}

type ResourceSnapshot struct {
	ID             ResourceID     `json:"id"`
	Name           string         `json:"name"`
	Status         ResourceStatus `json:"status"`
	SignalStatus   ResourceStatus `json:"signalStatus"`
	CPUHostPercent *float64       `json:"cpuHostPct"`
	MemoryBytes    *int64         `json:"memoryBytes"`
	RXBPS          *float64       `json:"rxBps"`
	TXBPS          *float64       `json:"txBps"`
	BlockReadBPS   *float64       `json:"blockReadBps"`
	BlockWriteBPS  *float64       `json:"blockWriteBps"`
	LastSeenAt     time.Time      `json:"lastSeenAt"`
	// Stale marks a resource republished from the last successful Docker
	// tick after a failed one: identities only, every metric nil, status
	// unknown and LastSeenAt unchanged. Persistence never stores it.
	Stale          bool                `json:"stale"`
	Category       string              `json:"category,omitempty"`
	Context        string              `json:"context,omitempty"`
	Project        string              `json:"project,omitempty"`
	Environment    string              `json:"environment,omitempty"`
	Infrastructure bool                `json:"infrastructure,omitempty"`
	Components     []ResourceComponent `json:"components,omitempty"`
	StableKey      string              `json:"-"`
	SourceKind     string              `json:"-"`
	ManualName     bool                `json:"-"`
	ManualContext  bool                `json:"-"`
}
type CollectorHealth struct {
	Name    string         `json:"name"`
	State   CollectorState `json:"state"`
	Reason  string         `json:"reason,omitempty"`
	FreshAt time.Time      `json:"freshAt"`
}

// Event is one discrete occurrence. ID is a ULID that is unique across
// process starts and is the stored row's key; Seq is the engine's
// per-process message sequence that the live stream resumes from
// (ADR 021 §9).
type Event struct {
	ID                string      `json:"id"`
	Seq               Sequence    `json:"seq"`
	At                time.Time   `json:"at"`
	Type              string      `json:"type"`
	ResourceID        ResourceID  `json:"resourceId,omitempty"`
	ContainerInstance ContainerID `json:"containerInstanceId,omitempty"`
	Severity          string      `json:"severity,omitempty"`
	Message           string      `json:"message"`
	Details           string      `json:"details,omitempty"`
	CorrelationKey    string      `json:"correlationKey,omitempty"`
}
type Snapshot struct {
	Sequence     Sequence                   `json:"seq"`
	At           time.Time                  `json:"ts"`
	BootIdentity BootIdentity               `json:"bootIdentity"`
	Host         HostObservation            `json:"host"`
	Resources    []ResourceSnapshot         `json:"resources"`
	Collectors   map[string]CollectorHealth `json:"collectors"`
	Filesystems  []FilesystemObservation    `json:"filesystems,omitempty"`
}
type FilesystemObservation struct {
	At                time.Time `json:"at"`
	MountKey          string    `json:"mountKey"`
	MountPoint        string    `json:"mountPoint"`
	FSType            string    `json:"fsType"`
	TotalBytes        *int64    `json:"totalBytes"`
	UsedBytes         *int64    `json:"usedBytes"`
	AvailableBytes    *int64    `json:"availableBytes"`
	UsedPercent       *float64  `json:"usedPct"`
	InodesTotal       *int64    `json:"inodesTotal"`
	InodesUsed        *int64    `json:"inodesUsed"`
	InodesUsedPercent *float64  `json:"inodesUsedPct"`
	// Role is FilesystemRoot, FilesystemDocker, FilesystemData or
	// FilesystemOther; the host collector sets it.
	Role string `json:"role"`
	// Trend is the time-to-full estimate of used bytes; the engine attaches
	// it, collectors leave it zero.
	Trend Trend `json:"trend"`
}

// Filesystem roles. A mount that matches several takes the first in this
// order: root, docker, data, other.
const (
	FilesystemRoot   = "root"
	FilesystemDocker = "docker"
	FilesystemData   = "data"
	FilesystemOther  = "other"
)

// PersistenceBatch is one persistence window. Snapshot carries the window
// aggregate of every tick published since the previous batch (means for rates
// and gauges, sums for counter deltas, the last value otherwise); the ranges
// carry each metric's minimum and maximum over those ticks. Ranges are nil
// when the window saw no tick of that kind; storage then falls back to the
// snapshot value.
type PersistenceBatch struct {
	Snapshot       Snapshot
	Events         []Event
	Filesystems    []FilesystemObservation
	HostRanges     *HostRanges
	ResourceRanges map[ResourceID]ResourceRanges
	// HostTicks is the number of host ticks aggregated into Snapshot.Host;
	// zero means none, and Snapshot.Host is the last published observation.
	HostTicks int
}

type TimeRange struct{ From, To time.Time }

func (r TimeRange) Validate() error {
	if r.From.IsZero() || r.To.IsZero() || !r.From.Before(r.To) {
		return fmt.Errorf("time range start must precede end")
	}
	return nil
}
func UTC(t time.Time) time.Time { return t.UTC() }
