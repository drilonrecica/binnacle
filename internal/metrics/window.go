// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"math"
	"time"
)

// Range is the smallest and largest value one metric took across the ticks of
// a persistence window. Both are nil when no tick measured the metric.
type Range struct {
	Min, Max *float64
}

// HostRanges holds the per-window range of every host metric the rollups
// keep a minimum and maximum for.
type HostRanges struct {
	CPU, CPUUser, CPUSystem, CPUIOWait, CPUSteal Range
	Load1, Load5, Load15                         Range
	MemoryUsed, SwapUsed                         Range
	NetworkRX, NetworkTX                         Range
	DiskRead, DiskWrite                          Range
	// DiskIOPS is the range of read plus write IOPS of the same tick.
	DiskIOPS Range
}

// ResourceRanges holds the per-window range of every resource metric the
// rollups keep a minimum and maximum for.
type ResourceRanges struct {
	CPU, Memory, RX, TX, BlockRead, BlockWrite Range
}

// stat is a running summary of one series over a window: constant size
// however many ticks the window holds.
type stat struct {
	n             int
	sum, min, max float64
}

func (s *stat) add(v float64) {
	if s.n == 0 || v < s.min {
		s.min = v
	}
	if s.n == 0 || v > s.max {
		s.max = v
	}
	s.sum += v
	s.n++
}

func (s *stat) addFloat(v *float64) {
	if v != nil {
		s.add(*v)
	}
}

func (s *stat) addInt(v *int64) {
	if v != nil {
		s.add(float64(*v))
	}
}

func (s *stat) mean() *float64 {
	if s.n == 0 {
		return nil
	}
	v := s.sum / float64(s.n)
	return &v
}

func (s *stat) meanInt() *int64 {
	if s.n == 0 {
		return nil
	}
	v := int64(math.Round(s.sum / float64(s.n)))
	return &v
}

func (s *stat) total() *int64 {
	if s.n == 0 {
		return nil
	}
	v := int64(math.Round(s.sum))
	return &v
}

func (s *stat) span() Range {
	if s.n == 0 {
		return Range{}
	}
	lo, hi := s.min, s.max
	return Range{Min: &lo, Max: &hi}
}

// Host fields by how a window combines them. Rates and gauges are averaged,
// counter deltas summed, and every field not listed (totals, uptime, used disk
// of the data filesystem) keeps the value of the last tick.
type hostFloatField struct {
	field func(*HostObservation) **float64
	span  func(*HostRanges) *Range
}

type hostIntField struct {
	field func(*HostObservation) **int64
	span  func(*HostRanges) *Range
}

var hostFloatFields = []hostFloatField{
	{func(h *HostObservation) **float64 { return &h.CPUPercent }, func(r *HostRanges) *Range { return &r.CPU }},
	{func(h *HostObservation) **float64 { return &h.CPUUserPercent }, func(r *HostRanges) *Range { return &r.CPUUser }},
	{func(h *HostObservation) **float64 { return &h.CPUSystemPercent }, func(r *HostRanges) *Range { return &r.CPUSystem }},
	{func(h *HostObservation) **float64 { return &h.CPUIOWaitPercent }, func(r *HostRanges) *Range { return &r.CPUIOWait }},
	{func(h *HostObservation) **float64 { return &h.CPUStealPercent }, func(r *HostRanges) *Range { return &r.CPUSteal }},
	{func(h *HostObservation) **float64 { return &h.Load1 }, func(r *HostRanges) *Range { return &r.Load1 }},
	{func(h *HostObservation) **float64 { return &h.Load5 }, func(r *HostRanges) *Range { return &r.Load5 }},
	{func(h *HostObservation) **float64 { return &h.Load15 }, func(r *HostRanges) *Range { return &r.Load15 }},
	{func(h *HostObservation) **float64 { return &h.NetworkRXBPS }, func(r *HostRanges) *Range { return &r.NetworkRX }},
	{func(h *HostObservation) **float64 { return &h.NetworkTXBPS }, func(r *HostRanges) *Range { return &r.NetworkTX }},
	{func(h *HostObservation) **float64 { return &h.DiskReadBPS }, func(r *HostRanges) *Range { return &r.DiskRead }},
	{func(h *HostObservation) **float64 { return &h.DiskWriteBPS }, func(r *HostRanges) *Range { return &r.DiskWrite }},
	{func(h *HostObservation) **float64 { return &h.MemoryPercent }, nil},
	{func(h *HostObservation) **float64 { return &h.SwapPercent }, nil},
	{func(h *HostObservation) **float64 { return &h.NetworkRXPacketsPS }, nil},
	{func(h *HostObservation) **float64 { return &h.NetworkTXPacketsPS }, nil},
	{func(h *HostObservation) **float64 { return &h.DiskReadIOPS }, nil},
	{func(h *HostObservation) **float64 { return &h.DiskWriteIOPS }, nil},
}

var hostGaugeFields = []hostIntField{
	{func(h *HostObservation) **int64 { return &h.MemoryUsedBytes }, func(r *HostRanges) *Range { return &r.MemoryUsed }},
	{func(h *HostObservation) **int64 { return &h.SwapUsedBytes }, func(r *HostRanges) *Range { return &r.SwapUsed }},
	{func(h *HostObservation) **int64 { return &h.MemoryAvailableBytes }, nil},
	{func(h *HostObservation) **int64 { return &h.MemoryCachedBytes }, nil},
	{func(h *HostObservation) **int64 { return &h.MemoryBuffersBytes }, nil},
}

var hostDeltaFields = []func(*HostObservation) **int64{
	func(h *HostObservation) **int64 { return &h.NetworkRXErrorsDelta },
	func(h *HostObservation) **int64 { return &h.NetworkTXErrorsDelta },
	func(h *HostObservation) **int64 { return &h.NetworkRXDropsDelta },
	func(h *HostObservation) **int64 { return &h.NetworkTXDropsDelta },
}

type hostWindow struct {
	ticks  int
	last   HostObservation
	floats []stat
	gauges []stat
	deltas []stat
	iops   stat
}

func newHostWindow() *hostWindow {
	return &hostWindow{floats: make([]stat, len(hostFloatFields)), gauges: make([]stat, len(hostGaugeFields)), deltas: make([]stat, len(hostDeltaFields))}
}

func (w *hostWindow) add(h HostObservation) {
	w.ticks++
	w.last = h
	for i, f := range hostFloatFields {
		w.floats[i].addFloat(*f.field(&h))
	}
	for i, f := range hostGaugeFields {
		w.gauges[i].addInt(*f.field(&h))
	}
	for i, f := range hostDeltaFields {
		w.deltas[i].addInt(*f(&h))
	}
	if h.DiskReadIOPS != nil && h.DiskWriteIOPS != nil {
		w.iops.add(*h.DiskReadIOPS + *h.DiskWriteIOPS)
	}
}

func (w *hostWindow) result() (HostObservation, HostRanges) {
	out := w.last
	var ranges HostRanges
	for i, f := range hostFloatFields {
		*f.field(&out) = w.floats[i].mean()
		if f.span != nil {
			*f.span(&ranges) = w.floats[i].span()
		}
	}
	for i, f := range hostGaugeFields {
		*f.field(&out) = w.gauges[i].meanInt()
		if f.span != nil {
			*f.span(&ranges) = w.gauges[i].span()
		}
	}
	for i, f := range hostDeltaFields {
		*f(&out) = w.deltas[i].total()
	}
	ranges.DiskIOPS = w.iops.span()
	return out, ranges
}

// containerMetrics are the six metrics a resource and each of its components
// carry, in the order cpu, memory, rx, tx, block read, block write.
type containerMetrics [6]stat

func (m *containerMetrics) add(cpu *float64, memory *int64, rx, tx, read, write *float64) {
	m[0].addFloat(cpu)
	m[1].addInt(memory)
	m[2].addFloat(rx)
	m[3].addFloat(tx)
	m[4].addFloat(read)
	m[5].addFloat(write)
}

func (m *containerMetrics) ranges() ResourceRanges {
	return ResourceRanges{CPU: m[0].span(), Memory: m[1].span(), RX: m[2].span(), TX: m[3].span(), BlockRead: m[4].span(), BlockWrite: m[5].span()}
}

type componentWindow struct {
	last    ResourceComponent
	metrics containerMetrics
}

type resourceWindow struct {
	last       ResourceSnapshot
	metrics    containerMetrics
	components map[ContainerID]*componentWindow
	order      []ContainerID
}

func (w *resourceWindow) add(r ResourceSnapshot) {
	w.last = r
	w.metrics.add(r.CPUHostPercent, r.MemoryBytes, r.RXBPS, r.TXBPS, r.BlockReadBPS, r.BlockWriteBPS)
	for _, c := range r.Components {
		cw, ok := w.components[c.ID]
		if !ok {
			cw = &componentWindow{}
			w.components[c.ID] = cw
			w.order = append(w.order, c.ID)
		}
		cw.last = c
		cw.metrics.add(c.CPUHostPercent, c.MemoryBytes, c.RXBPS, c.TXBPS, c.BlockReadBPS, c.BlockWriteBPS)
	}
}

func (w *resourceWindow) result() (ResourceSnapshot, ResourceRanges) {
	out := w.last
	m := &w.metrics
	out.CPUHostPercent, out.MemoryBytes, out.RXBPS, out.TXBPS, out.BlockReadBPS, out.BlockWriteBPS = m[0].mean(), m[1].meanInt(), m[2].mean(), m[3].mean(), m[4].mean(), m[5].mean()
	// Every container seen in the window, so a container replaced mid-window
	// keeps the samples it reported before it went away.
	out.Components = make([]ResourceComponent, 0, len(w.order))
	for _, id := range w.order {
		cw := w.components[id]
		c := cw.last
		cm := &cw.metrics
		c.CPUHostPercent, c.MemoryBytes, c.RXBPS, c.TXBPS, c.BlockReadBPS, c.BlockWriteBPS = cm[0].mean(), cm[1].meanInt(), cm[2].mean(), cm[3].mean(), cm[4].mean(), cm[5].mean()
		out.Components = append(out.Components, c)
	}
	return out, m.ranges()
}

// window accumulates every distinct observation published between two
// persistence batches (COL-9). Both collector loops publish, and each publish
// repeats the other loop's latest result, so an observation is identified by
// its tick time: HostObservation.At for the host, LastSeenAt per resource.
// Stale resources are never accumulated. Its size is bounded by the number of
// resources observed in one window, not by the number of ticks.
type window struct {
	hostAt    time.Time
	host      *hostWindow
	seen      map[ResourceID]time.Time
	resources map[ResourceID]*resourceWindow
	order     []ResourceID
}

func (w *window) observe(s Snapshot) {
	if s.Host.At.IsZero() || !s.Host.At.Equal(w.hostAt) {
		w.hostAt = s.Host.At
		if w.host == nil {
			w.host = newHostWindow()
		}
		w.host.add(s.Host)
	}
	seen := make(map[ResourceID]time.Time, len(s.Resources))
	for _, r := range s.Resources {
		seen[r.ID] = r.LastSeenAt
		if r.Stale {
			continue
		}
		if previous, ok := w.seen[r.ID]; ok && !r.LastSeenAt.IsZero() && previous.Equal(r.LastSeenAt) {
			continue
		}
		if w.resources == nil {
			w.resources = map[ResourceID]*resourceWindow{}
		}
		rw, ok := w.resources[r.ID]
		if !ok {
			rw = &resourceWindow{components: map[ContainerID]*componentWindow{}}
			w.resources[r.ID] = rw
			w.order = append(w.order, r.ID)
		}
		rw.add(r)
	}
	w.seen = seen
}

// drain turns the window into a persistence batch built on latest, the most
// recent published snapshot, and starts a new window. latest keeps its time,
// sequence, collectors and filesystems; its host is replaced by the window
// aggregate when the window saw a host tick, and its resources by those
// observed in the window, so a resource no tick measured is not stored again.
func (w *window) drain(latest Snapshot) PersistenceBatch {
	batch := PersistenceBatch{Snapshot: latest}
	if w.host != nil {
		host, ranges := w.host.result()
		batch.Snapshot.Host = host
		batch.HostRanges = &ranges
		batch.HostTicks = w.host.ticks
	}
	batch.Snapshot.Resources = make([]ResourceSnapshot, 0, len(w.order))
	if len(w.order) > 0 {
		batch.ResourceRanges = make(map[ResourceID]ResourceRanges, len(w.order))
	}
	for _, id := range w.order {
		r, ranges := w.resources[id].result()
		batch.Snapshot.Resources = append(batch.Snapshot.Resources, r)
		batch.ResourceRanges[id] = ranges
	}
	w.host, w.resources, w.order = nil, nil, nil
	return batch
}
