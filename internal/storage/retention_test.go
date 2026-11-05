// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"testing"
	"time"
)

func rawHostCount(t *testing.T, m *Manager, from time.Time) int {
	t.Helper()
	var n int
	if err := m.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM host_samples_10s WHERE ts>=?", from.UnixMilli()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// STO-2: raw rows are deleted only below a whole-hour boundary, so the 1h
// bucket at the edge keeps its complete aggregate, and a straggler row below
// the persisted raw_deleted_below never triggers a recompute.
func TestRetentionAlignsRawCutoffToHourAndKeepsFinalBuckets(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	insertHostSamplesEveryMinute(t, m, rollupBase, rollupBase.Add(2*time.Hour), 1)
	now := rollupBase.Add(3*time.Hour + 30*time.Minute + 30*time.Second) // 15:30:30
	if err := m.RollupOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	expectHostBucket(t, m, "1h", rollupBase, 1, 60)
	expectHostBucket(t, m, "1h", rollupBase.Add(time.Hour), 1, 60)

	// now-2h is 13:30:30; the cutoff must align down to 13:00.
	if err := m.applyRetention(ctx, now, RetentionCutoffs{Raw: 2 * time.Hour, OneMinute: 100 * time.Minute}, false); err != nil {
		t.Fatal(err)
	}
	if got := rawHostCount(t, m, time.Time{}); got != 60 {
		t.Fatalf("raw rows after retention=%d want 60", got)
	}
	if got := rawHostCount(t, m, rollupBase.Add(time.Hour)); got != 60 {
		t.Fatalf("raw rows at or after 13:00=%d want 60", got)
	}
	expectWatermark(t, m, "raw_deleted_below", rollupBase.Add(time.Hour))
	// Rollup tier cutoffs align down to the hour too: now-100m is 13:50:30,
	// so every 1m bucket from 13:00 on stays.
	var oneMinute int
	if err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM host_rollups_1m").Scan(&oneMinute); err != nil {
		t.Fatal(err)
	}
	if oneMinute != 60 {
		t.Fatalf("1m rollup rows after retention=%d want 60", oneMinute)
	}

	// A raw row that survives below raw_deleted_below (a partially deleted
	// hour) must never overwrite the final aggregate.
	insertHostSample(t, m, rollupBase.Add(30*time.Minute), 1000)
	if err := m.RollupOnce(ctx, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	expectHostBucket(t, m, "1h", rollupBase, 1, 60)
	expectHostBucket(t, m, "15m", rollupBase.Add(30*time.Minute), 1, 15)
}

func TestRetentionNeverDeletesRawAheadOfHourWatermark(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	insertHostSamplesEveryMinute(t, m, rollupBase, rollupBase.Add(2*time.Hour), 1)
	dayLater := rollupBase.Add(24*time.Hour + 30*time.Second)
	policy := RetentionCutoffs{Raw: 2 * time.Hour}

	// No rollup pass yet: nothing is final, so nothing raw is deleted.
	if err := m.applyRetention(ctx, dayLater, policy, false); err != nil {
		t.Fatal(err)
	}
	if got := rawHostCount(t, m, time.Time{}); got != 120 {
		t.Fatalf("raw rows deleted before any rollup pass: %d left", got)
	}

	// The 1h tier closed at 13:00; retention stops there although the
	// configured cutoff is a day later.
	if err := m.RollupOnce(ctx, rollupBase.Add(90*time.Minute+30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := m.applyRetention(ctx, dayLater, policy, false); err != nil {
		t.Fatal(err)
	}
	if got := rawHostCount(t, m, time.Time{}); got != 60 {
		t.Fatalf("raw rows after retention=%d want 60", got)
	}
	expectWatermark(t, m, "raw_deleted_below", rollupBase.Add(time.Hour))
}

func TestRetentionDurationIsReported(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	if d := m.Health().RetentionDuration; d != 0 {
		t.Fatalf("duration before any pass=%s", d)
	}
	if err := m.applyRetention(ctx, rollupBase, RetentionCutoffs{Raw: time.Hour}, false); err != nil {
		t.Fatal(err)
	}
	if d := m.Health().RetentionDuration; d <= 0 {
		t.Fatalf("duration after pass=%s", d)
	}
}
