// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

func openTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	m := New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := m.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	if err := m.UpsertHost(context.Background(), "host", "host-identity", "test-host"); err != nil {
		t.Fatal(err)
	}
	return m
}

// COL-9: a burst inside a 10 s window reaches the raw history and the rollup
// maximum. Five 2 s ticks go through the engine and persistence, and the
// spike that was neither the first nor the last tick is what the 1 m rollup
// reports as its maximum.
func TestWindowRangesReachRawHistoryAndRollups(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	if err := m.UpsertResource(ctx, Resource{ID: "res_web", HostID: "host", StableKey: "web", SourceKind: "compose", Name: "web", Category: "service", Status: "healthy"}); err != nil {
		t.Fatal(err)
	}
	engine := metrics.NewEngine(8)
	bucket := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	cpu := []float64{10, 95, 10, 10, 25}
	for i, v := range cpu {
		at := bucket.Add(time.Duration(i) * 2 * time.Second)
		value := v
		resource := metrics.ResourceSnapshot{ID: "res_web", Name: "web", LastSeenAt: at, CPUHostPercent: &value,
			Components: []metrics.ResourceComponent{{ID: "c0ffee123456", Name: "web_1", CPUHostPercent: &value}}}
		engine.Publish(metrics.Snapshot{At: at, Host: metrics.HostObservation{At: at, CPUPercent: &value}, Resources: []metrics.ResourceSnapshot{resource}})
	}
	batch := engine.PersistenceBatch()
	if err := m.WriteBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	ts := batch.Snapshot.At.UnixMilli()

	var hostAvg, hostMax, resourceAvg, resourceMax float64
	if err := m.db.QueryRowContext(ctx, "SELECT cpu_busy_pct,cpu_busy_pct_max FROM host_samples_10s WHERE ts=?", ts).Scan(&hostAvg, &hostMax); err != nil {
		t.Fatal(err)
	}
	if err := m.db.QueryRowContext(ctx, "SELECT cpu_host_pct,cpu_host_pct_max FROM resource_samples_10s WHERE ts=? AND resource_id='res_web'", ts).Scan(&resourceAvg, &resourceMax); err != nil {
		t.Fatal(err)
	}
	if hostAvg != 30 || hostMax != 95 || resourceAvg != 30 || resourceMax != 95 {
		t.Fatalf("raw host avg=%v max=%v resource avg=%v max=%v, want 30 and 95", hostAvg, hostMax, resourceAvg, resourceMax)
	}

	points, err := m.metricPoints(ctx, MetricQuery{Scope: "host", From: bucket, To: bucket.Add(time.Minute)}, MetricCPU, ResolutionRaw)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || *points[0].Min != 10 || *points[0].Avg != 30 || *points[0].Max != 95 {
		t.Fatalf("raw history points=%+v, want min 10 avg 30 max 95", points)
	}

	if err := m.RollupOnce(ctx, bucket.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var min, max float64
	if err := m.db.QueryRowContext(ctx, "SELECT cpu_min,cpu_max FROM host_rollups_1m WHERE ts=?", bucket.UnixMilli()).Scan(&min, &max); err != nil {
		t.Fatal(err)
	}
	if min != 10 || max != 95 {
		t.Fatalf("host rollup min=%v max=%v, want the window extremes 10 and 95", min, max)
	}
	if err := m.db.QueryRowContext(ctx, "SELECT cpu_min,cpu_max FROM resource_rollups_1m WHERE ts=? AND resource_id='res_web'", bucket.UnixMilli()).Scan(&min, &max); err != nil {
		t.Fatal(err)
	}
	if min != 10 || max != 95 {
		t.Fatalf("resource rollup min=%v max=%v, want the window extremes 10 and 95", min, max)
	}
}

// Rows written before migration 024 have no ranges; raw history and rollups
// fall back to the sample value.
func TestSamplesWithoutRangesFallBackToTheirValue(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	bucket := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	for i, v := range []float64{20, 40} {
		if _, err := m.db.ExecContext(ctx, "INSERT INTO host_samples_10s(ts,host_id,cpu_busy_pct,disk_read_iops,disk_write_iops) VALUES(?,'host',?,?,?)", bucket.Add(time.Duration(i)*10*time.Second).UnixMilli(), v, v, 1); err != nil {
			t.Fatal(err)
		}
	}
	points, err := m.metricPoints(ctx, MetricQuery{Scope: "host", From: bucket, To: bucket.Add(time.Minute)}, MetricDiskIOPS, ResolutionRaw)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 || *points[1].Min != 41 || *points[1].Max != 41 {
		t.Fatalf("raw iops points=%+v, want min=max=avg", points)
	}
	if err := m.RollupOnce(ctx, bucket.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var min, max float64
	if err := m.db.QueryRowContext(ctx, "SELECT cpu_min,cpu_max FROM host_rollups_1m WHERE ts=?", bucket.UnixMilli()).Scan(&min, &max); err != nil {
		t.Fatal(err)
	}
	if min != 20 || max != 40 {
		t.Fatalf("legacy rollup min=%v max=%v", min, max)
	}
}

// A Docker tick that started before one batch and was published after it is
// in the next window with a LastSeenAt older than the previous batch time; it
// is still a new observation and must be stored.
func TestWriteBatchKeepsATickThatStraddlesABatch(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	if err := m.UpsertResource(ctx, Resource{ID: "res_web", HostID: "host", StableKey: "web", SourceKind: "compose", Name: "web", Category: "service", Status: "healthy"}); err != nil {
		t.Fatal(err)
	}
	cpu := 10.0
	base := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	write := func(seq metrics.Sequence, at, seen time.Time) {
		t.Helper()
		r := metrics.ResourceSnapshot{ID: "res_web", Name: "web", LastSeenAt: seen, CPUHostPercent: &cpu}
		if err := m.WriteBatch(ctx, metrics.PersistenceBatch{Snapshot: metrics.Snapshot{Sequence: seq, At: at, Host: metrics.HostObservation{At: at}, Resources: []metrics.ResourceSnapshot{r}}}); err != nil {
			t.Fatal(err)
		}
	}
	write(1, base, base.Add(-3*time.Second))
	second := base.Add(10 * time.Second)
	write(2, second, base.Add(-time.Second))
	var rows int
	if err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM resource_samples_10s WHERE ts=?", second.UnixMilli()).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("straddling tick rows=%d, want 1", rows)
	}
}
