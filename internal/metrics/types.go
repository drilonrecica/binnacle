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
	At              time.Time `json:"at"`
	CPUPercent      *float64  `json:"cpuPct"`
	MemoryUsedBytes *int64    `json:"memoryUsedBytes"`
	MemoryPercent   *float64  `json:"memoryPct"`
	Load1           *float64  `json:"load1"`
	NetworkRXBPS    *float64  `json:"networkRxBps"`
	NetworkTXBPS    *float64  `json:"networkTxBps"`
}
type ContainerObservation struct {
	ID             ContainerID    `json:"id"`
	ResourceID     ResourceID     `json:"resourceId"`
	At             time.Time      `json:"at"`
	CPUHostPercent *float64       `json:"cpuHostPct"`
	MemoryBytes    *int64         `json:"memoryBytes"`
	RXBPS          *float64       `json:"rxBps"`
	TXBPS          *float64       `json:"txBps"`
	Status         ResourceStatus `json:"status"`
}
type ResourceSnapshot struct {
	ID             ResourceID     `json:"id"`
	Name           string         `json:"name"`
	Status         ResourceStatus `json:"status"`
	CPUHostPercent *float64       `json:"cpuHostPct"`
	MemoryBytes    *int64         `json:"memoryBytes"`
	RXBPS          *float64       `json:"rxBps"`
	TXBPS          *float64       `json:"txBps"`
	LastSeenAt     time.Time      `json:"lastSeenAt"`
}
type CollectorHealth struct {
	Name    string         `json:"name"`
	State   CollectorState `json:"state"`
	Reason  string         `json:"reason,omitempty"`
	FreshAt time.Time      `json:"freshAt"`
}
type Event struct {
	ID         Sequence   `json:"id"`
	At         time.Time  `json:"at"`
	Type       string     `json:"type"`
	ResourceID ResourceID `json:"resourceId,omitempty"`
	Message    string     `json:"message"`
}
type Snapshot struct {
	Sequence     Sequence                   `json:"seq"`
	At           time.Time                  `json:"ts"`
	BootIdentity BootIdentity               `json:"bootIdentity"`
	Host         HostObservation            `json:"host"`
	Resources    []ResourceSnapshot         `json:"resources"`
	Collectors   map[string]CollectorHealth `json:"collectors"`
}
type PersistenceBatch struct {
	Snapshot Snapshot
	Events   []Event
}
type TimeRange struct{ From, To time.Time }

func (r TimeRange) Validate() error {
	if r.From.IsZero() || r.To.IsZero() || !r.From.Before(r.To) {
		return fmt.Errorf("time range start must precede end")
	}
	return nil
}
func UTC(t time.Time) time.Time { return t.UTC() }
