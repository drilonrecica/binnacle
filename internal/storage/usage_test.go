// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"math"
	"testing"
	"time"
)

// P3-12: usage histories come back in 15-minute buckets of mean time, used
// and capacity, for one mount and only within the range.
func TestUsageHistoriesAreBucketed(t *testing.T) {
	ctx := context.Background()
	m := openTestManager(t)
	start := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	for minute := range 120 {
		ts := start.Add(time.Duration(minute) * time.Minute).UnixMilli()
		used, available := int64(1000+minute), int64(500)
		for _, key := range []string{"root", "other"} {
			if _, err := m.db.ExecContext(ctx, `INSERT INTO filesystem_samples_1m(ts,host_id,mount_key,mount_point,used_bytes,available_bytes) VALUES(?,?,?,?,?,?)`, ts, hostID, key, "/"+key, used, available); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := m.db.ExecContext(ctx, `INSERT INTO host_rollups_1m(ts,sample_count,memory_avg,memory_count) VALUES(?,6,?,6)`, ts, float64(2000+minute)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := m.FilesystemUsage(ctx, "root", start.Add(30*time.Minute), start.Add(90*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	// Minutes 30..90 inclusive: buckets 30-44, 45-59, 60-74, 75-89 and 90.
	if len(got) != 5 {
		t.Fatalf("buckets=%d %+v", len(got), got)
	}
	first := got[0]
	if want := start.Add(37 * time.Minute); first.At.Sub(want).Abs() > time.Second || math.Abs(first.Used-1037) > 1e-9 || math.Abs(first.Capacity-1537) > 1e-9 {
		t.Fatalf("first bucket=%+v", first)
	}
	if last := got[4]; last.Used != 1090 || !last.At.Equal(start.Add(90*time.Minute)) {
		t.Fatalf("last bucket=%+v", last)
	}
	memory, err := m.MemoryUsage(ctx, start, start.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(memory) != 8 || memory[0].Used != 2007 || memory[0].Capacity != 0 {
		t.Fatalf("memory buckets=%d first=%+v", len(memory), memory[0])
	}
	if none, err := m.FilesystemUsage(ctx, "missing", start, start.Add(time.Hour)); err != nil || len(none) != 0 {
		t.Fatalf("unknown mount=%v err=%v", none, err)
	}
}
