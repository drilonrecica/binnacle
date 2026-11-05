// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// Rollup fixtures: samples are keyed by their timestamp; the value lands in
// cpu_busy_pct so cpu_avg and sample_count tell whether a bucket was
// (re)computed.
func insertHostSample(t *testing.T, m *Manager, at time.Time, value float64) {
	t.Helper()
	if _, err := m.db.ExecContext(context.Background(), "INSERT INTO host_samples_10s(ts,host_id,cpu_busy_pct) VALUES(?,'host',?)", at.UnixMilli(), value); err != nil {
		t.Fatal(err)
	}
}

func insertResourceSample(t *testing.T, m *Manager, at time.Time, value float64) {
	t.Helper()
	if _, err := m.db.ExecContext(context.Background(), "INSERT INTO resource_samples_10s(ts,resource_id,cpu_host_pct,active_instance_count) VALUES(?,'res_test',?,1)", at.UnixMilli(), value); err != nil {
		t.Fatal(err)
	}
}

// insertHostSamplesEveryMinute writes one sample per minute in [from, until).
func insertHostSamplesEveryMinute(t *testing.T, m *Manager, from, until time.Time, value float64) {
	t.Helper()
	for at := from; at.Before(until); at = at.Add(time.Minute) {
		insertHostSample(t, m, at, value)
	}
}

// hostBucket returns cpu_avg and sample_count of one host rollup bucket; ok is
// false when the bucket does not exist.
func hostBucket(t *testing.T, m *Manager, tier string, at time.Time) (avg float64, count int, ok bool) {
	t.Helper()
	var avgValue *float64
	rows, err := m.db.QueryContext(context.Background(), "SELECT cpu_avg,sample_count FROM host_rollups_"+tier+" WHERE ts=?", at.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, 0, false
	}
	if err := rows.Scan(&avgValue, &count); err != nil {
		t.Fatal(err)
	}
	if avgValue != nil {
		avg = *avgValue
	}
	return avg, count, true
}

func expectHostBucket(t *testing.T, m *Manager, tier string, at time.Time, wantAvg float64, wantCount int) {
	t.Helper()
	avg, count, ok := hostBucket(t, m, tier, at)
	if !ok {
		t.Fatalf("%s bucket %s missing", tier, at.Format(time.RFC3339))
	}
	if avg != wantAvg || count != wantCount {
		t.Fatalf("%s bucket %s avg=%v count=%d, want avg=%v count=%d", tier, at.Format(time.RFC3339), avg, count, wantAvg, wantCount)
	}
}

func watermark(t *testing.T, m *Manager, name string) (int64, bool) {
	t.Helper()
	var value int64
	rows, err := m.db.QueryContext(context.Background(), "SELECT closed_until FROM rollup_watermarks WHERE table_name=?", name)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, false
	}
	if err := rows.Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value, true
}

func expectWatermark(t *testing.T, m *Manager, name string, want time.Time) {
	t.Helper()
	got, ok := watermark(t, m, name)
	if !ok {
		t.Fatalf("watermark %s missing", name)
	}
	if got != want.UnixMilli() {
		t.Fatalf("watermark %s=%s want %s", name, time.UnixMilli(got).UTC().Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

var rollupBase = time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)

// STO-1: a pass recomputes only the buckets a late batch could still touch
// (closed_until minus the lateness window), never the whole raw history.
func TestRollupPassOnlyRecomputesRecentlyClosedBuckets(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	insertHostSample(t, m, rollupBase, 10)
	insertHostSample(t, m, rollupBase.Add(10*time.Second), 30)
	first := rollupBase.Add(3*time.Hour + 30*time.Second) // 15:00:30
	if err := m.RollupOnce(ctx, first); err != nil {
		t.Fatal(err)
	}
	expectHostBucket(t, m, "1m", rollupBase, 20, 2)
	expectHostBucket(t, m, "1h", rollupBase, 20, 2)
	for _, name := range []string{"host_rollups_1m", "host_rollups_15m", "host_rollups_1h", "resource_rollups_1m", "resource_rollups_15m", "resource_rollups_1h"} {
		expectWatermark(t, m, name, rollupBase.Add(3*time.Hour))
	}

	// A sample older than the lateness window must not change its bucket; a
	// sample inside the window must.
	insertHostSample(t, m, rollupBase.Add(20*time.Second), 1000)
	late := rollupBase.Add(2*time.Hour + 59*time.Minute) // 14:59
	insertHostSample(t, m, late, 50)
	if err := m.RollupOnce(ctx, first.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	expectHostBucket(t, m, "1m", rollupBase, 20, 2)
	expectHostBucket(t, m, "1h", rollupBase, 20, 2)
	expectHostBucket(t, m, "1m", late, 50, 1)
	expectHostBucket(t, m, "15m", late.Truncate(15*time.Minute), 50, 1)
	expectHostBucket(t, m, "1h", late.Truncate(time.Hour), 50, 1)
}

func TestRollupWatermarkSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path, runtimeDir := filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run")
	m := New(path, runtimeDir)
	if err := m.Open(ctx); err != nil {
		t.Fatal(err)
	}
	insertHostSample(t, m, rollupBase, 10)
	insertHostSample(t, m, rollupBase.Add(10*time.Second), 30)
	first := rollupBase.Add(3*time.Hour + 30*time.Second)
	if err := m.RollupOnce(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := New(path, runtimeDir)
	if err := reopened.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	expectWatermark(t, reopened, "host_rollups_1h", rollupBase.Add(3*time.Hour))
	// Without the persisted watermark the pass would run the repair and fold
	// this sample into the old bucket.
	insertHostSample(t, reopened, rollupBase.Add(20*time.Second), 1000)
	if err := reopened.RollupOnce(ctx, first.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	expectHostBucket(t, reopened, "1m", rollupBase, 20, 2)
	expectHostBucket(t, reopened, "1h", rollupBase, 20, 2)
	expectWatermark(t, reopened, "host_rollups_1m", rollupBase.Add(3*time.Hour+time.Minute))
}

// ADR 021 §8: the first pass on a database without watermarks recomputes every
// bucket still backed by raw data, in bounded chunks, and leaves the bucket
// that contains the oldest raw row and buckets without raw rows untouched.
func TestFirstRollupPassRepairsBucketsBackedByRaw(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	day := rollupBase.Add(-12 * time.Hour) // 00:00
	oldest := day.Add(90 * time.Minute)    // 01:30
	insertHostSamplesEveryMinute(t, m, oldest, day.Add(15*time.Hour), 1)
	insertResourceSample(t, m, day.Add(5*time.Hour), 7)
	// Corrupted rows a pre-upgrade retention edge left behind.
	for _, row := range []struct {
		table string
		ts    time.Time
		count int
	}{
		{"host_rollups_1h", day.Add(13 * time.Hour), 1},
		{"host_rollups_15m", day.Add(13 * time.Hour), 1},
		{"host_rollups_1h", day.Add(time.Hour), 7}, // contains the oldest raw row
		{"host_rollups_1h", day, 5},                // no raw rows left
	} {
		if _, err := m.db.ExecContext(ctx, "INSERT INTO "+row.table+"(ts,cpu_avg,sample_count) VALUES(?,999,?)", row.ts.UnixMilli(), row.count); err != nil {
			t.Fatal(err)
		}
	}
	now := day.Add(15*time.Hour + 30*time.Second)
	if err := m.RollupOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	expectHostBucket(t, m, "1h", day.Add(13*time.Hour), 1, 60)
	expectHostBucket(t, m, "15m", day.Add(13*time.Hour), 1, 15)
	for hour := 2; hour < 15; hour++ {
		expectHostBucket(t, m, "1h", day.Add(time.Duration(hour)*time.Hour), 1, 60)
	}
	expectHostBucket(t, m, "1m", day.Add(7*time.Hour), 1, 1)
	expectHostBucket(t, m, "1h", day.Add(time.Hour), 999, 7)
	expectHostBucket(t, m, "1h", day, 999, 5)
	var count int
	if err := m.db.QueryRowContext(ctx, "SELECT sample_count FROM resource_rollups_1h WHERE resource_id='res_test' AND ts=?", day.Add(5*time.Hour).UnixMilli()).Scan(&count); err != nil || count != 1 {
		t.Fatalf("resource 1h bucket count=%d err=%v", count, err)
	}
	expectWatermark(t, m, "raw_deleted_below", day.Add(2*time.Hour))
	for _, name := range []string{"host_rollups_1m", "host_rollups_15m", "host_rollups_1h", "resource_rollups_1m", "resource_rollups_15m", "resource_rollups_1h"} {
		expectWatermark(t, m, name, day.Add(15*time.Hour))
	}
}

func TestFirstRollupPassOnEmptyDatabaseWritesWatermarks(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := rollupBase.Add(30 * time.Second)
	if err := m.RollupOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	expectWatermark(t, m, "host_rollups_1h", rollupBase)
	expectWatermark(t, m, "raw_deleted_below", time.UnixMilli(0).UTC())
}

// Demo seeding writes history after the first pass; RebuildRollups recomputes
// every bucket the raw rows still back.
func TestRebuildRollupsRecomputesHistoryWrittenAfterFirstPass(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	now := rollupBase.Add(3*time.Hour + 30*time.Second)
	if err := m.RollupOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	insertHostSample(t, m, rollupBase, 10)
	insertHostSample(t, m, rollupBase.Add(10*time.Second), 30)
	if err := m.RollupOnce(ctx, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := hostBucket(t, m, "1h", rollupBase); ok {
		t.Fatal("incremental pass must not reach back three hours")
	}
	if err := m.RebuildRollups(ctx, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	expectHostBucket(t, m, "1h", rollupBase, 20, 2)
	expectHostBucket(t, m, "1m", rollupBase, 20, 2)
	expectWatermark(t, m, "host_rollups_1h", rollupBase.Add(3*time.Hour))
}

func TestRollupDurationIsReported(t *testing.T) {
	m := openTestManager(t)
	if d := m.Health().RollupDuration; d != 0 {
		t.Fatalf("duration before any pass=%s", d)
	}
	if err := m.RollupOnce(context.Background(), rollupBase); err != nil {
		t.Fatal(err)
	}
	if d := m.Health().RollupDuration; d <= 0 {
		t.Fatalf("duration after pass=%s", d)
	}
}
