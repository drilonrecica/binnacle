// SPDX-License-Identifier: AGPL-3.0-only

package metrics

import (
	"testing"
	"time"
)

func f64(v float64) *float64 { return &v }
func i64(v int64) *int64     { return &v }

func wantFloat(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s=%v, want %v", name, deref(got), want)
	}
}

func wantRange(t *testing.T, name string, got Range, lo, hi float64) {
	t.Helper()
	if got.Min == nil || got.Max == nil || *got.Min != lo || *got.Max != hi {
		t.Fatalf("%s range=[%v,%v], want [%v,%v]", name, deref(got.Min), deref(got.Max), lo, hi)
	}
}

func deref(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

// COL-9: a burst between two persistence batches reaches storage. Five 2 s
// host ticks with one CPU spike are persisted as their mean with the spike as
// the maximum, not as whichever tick happened to be the latest.
func TestPersistenceBatchAggregatesHostTicksOfTheWindow(t *testing.T) {
	e := NewEngine(8)
	base := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	cpu := []float64{10, 10, 90, 10, 20}
	for i, v := range cpu {
		at := base.Add(time.Duration(i) * 2 * time.Second)
		h := HostObservation{At: at, CPUPercent: f64(v), NetworkRXBPS: f64(v * 100), MemoryUsedBytes: i64(int64(v) * 1000),
			MemoryTotalBytes: i64(int64(1000 + i)), NetworkRXErrorsDelta: i64(1), DiskReadIOPS: f64(v), DiskWriteIOPS: f64(1), UptimeSeconds: f64(float64(i))}
		e.Publish(Snapshot{At: at, Host: h})
		// The Docker loop republishes the same host observation: not a new tick.
		e.Publish(Snapshot{At: at.Add(time.Second), Host: h})
	}
	b := e.PersistenceBatch()
	if b.HostTicks != 5 {
		t.Fatalf("host ticks=%d, want 5 (republishes must not count)", b.HostTicks)
	}
	h := b.Snapshot.Host
	wantFloat(t, "cpu mean", h.CPUPercent, 28)
	wantFloat(t, "rx mean", h.NetworkRXBPS, 2800)
	if h.MemoryUsedBytes == nil || *h.MemoryUsedBytes != 28000 {
		t.Fatalf("memory mean=%v", h.MemoryUsedBytes)
	}
	if h.NetworkRXErrorsDelta == nil || *h.NetworkRXErrorsDelta != 5 {
		t.Fatalf("errors delta=%v, want the window sum 5", h.NetworkRXErrorsDelta)
	}
	if h.MemoryTotalBytes == nil || *h.MemoryTotalBytes != 1004 || *h.UptimeSeconds != 4 {
		t.Fatalf("totals must keep the last tick: total=%v uptime=%v", h.MemoryTotalBytes, h.UptimeSeconds)
	}
	if !h.At.Equal(base.Add(8 * time.Second)) {
		t.Fatalf("host at=%v, want the last tick", h.At)
	}
	r := b.HostRanges
	if r == nil {
		t.Fatal("host ranges missing")
	}
	wantRange(t, "cpu", r.CPU, 10, 90)
	wantRange(t, "rx", r.NetworkRX, 1000, 9000)
	wantRange(t, "memory", r.MemoryUsed, 10000, 90000)
	wantRange(t, "iops", r.DiskIOPS, 11, 91)
	if r.Load1.Min != nil || r.Load1.Max != nil {
		t.Fatalf("unmeasured metric has a range: %+v", r.Load1)
	}

	// The next window starts empty: no new host tick keeps the last
	// observation as it is, without ranges.
	next := e.PersistenceBatch()
	if next.HostTicks != 0 || next.HostRanges != nil {
		t.Fatalf("empty window ticks=%d ranges=%+v", next.HostTicks, next.HostRanges)
	}
	wantFloat(t, "empty window cpu", next.Snapshot.Host.CPUPercent, 20)
}

// COL-9: resources and their components are aggregated per Docker tick; a
// republished observation, a stale resource and a resource not measured in
// the window add nothing.
func TestPersistenceBatchAggregatesResourceTicksOfTheWindow(t *testing.T) {
	e := NewEngine(8)
	base := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	host := HostObservation{At: base}
	resource := func(seen time.Time, cpu float64, mem int64, stale bool, containers ...ContainerID) ResourceSnapshot {
		r := ResourceSnapshot{ID: "res_web", Name: "web", LastSeenAt: seen, Stale: stale, CPUHostPercent: f64(cpu), MemoryBytes: i64(mem)}
		for _, id := range containers {
			r.Components = append(r.Components, ResourceComponent{ID: id, Name: string(id), CPUHostPercent: f64(cpu), MemoryBytes: i64(mem)})
		}
		return r
	}
	t0, t1 := base, base.Add(2*time.Second)
	e.Publish(Snapshot{At: t0, Host: host, Resources: []ResourceSnapshot{resource(t0, 10, 100, false, "aaaaaaaaaaaa")}})
	// Host loop republishes the same Docker observation.
	e.Publish(Snapshot{At: t0.Add(time.Second), Host: host, Resources: []ResourceSnapshot{resource(t0, 10, 100, false, "aaaaaaaaaaaa")}})
	// The container is replaced in the next tick.
	e.Publish(Snapshot{At: t1, Host: host, Resources: []ResourceSnapshot{resource(t1, 50, 300, false, "bbbbbbbbbbbb")}})
	// A failed tick republishes it stale.
	e.Publish(Snapshot{At: t1.Add(2 * time.Second), Host: host, Resources: []ResourceSnapshot{resource(t1, 0, 0, true, "bbbbbbbbbbbb")}})

	b := e.PersistenceBatch()
	if len(b.Snapshot.Resources) != 1 {
		t.Fatalf("resources=%d, want 1", len(b.Snapshot.Resources))
	}
	r := b.Snapshot.Resources[0]
	wantFloat(t, "resource cpu mean", r.CPUHostPercent, 30)
	if r.MemoryBytes == nil || *r.MemoryBytes != 200 || r.Stale || !r.LastSeenAt.Equal(t1) {
		t.Fatalf("resource=%+v, want memory mean 200 from the last fresh tick", r)
	}
	rr, ok := b.ResourceRanges["res_web"]
	if !ok {
		t.Fatal("resource ranges missing")
	}
	wantRange(t, "resource cpu", rr.CPU, 10, 50)
	wantRange(t, "resource memory", rr.Memory, 100, 300)
	if len(r.Components) != 2 || r.Components[0].ID != "aaaaaaaaaaaa" || r.Components[1].ID != "bbbbbbbbbbbb" {
		t.Fatalf("components=%+v, want both containers seen in the window", r.Components)
	}
	wantFloat(t, "replaced container cpu", r.Components[0].CPUHostPercent, 10)
	wantFloat(t, "new container cpu", r.Components[1].CPUHostPercent, 50)

	// Only the stale republish follows: nothing new to store.
	e.Publish(Snapshot{At: t1.Add(4 * time.Second), Host: host, Resources: []ResourceSnapshot{resource(t1, 0, 0, true, "bbbbbbbbbbbb")}})
	if next := e.PersistenceBatch(); len(next.Snapshot.Resources) != 0 || next.ResourceRanges != nil {
		t.Fatalf("window without a fresh Docker tick stored %+v", next.Snapshot.Resources)
	}
}
