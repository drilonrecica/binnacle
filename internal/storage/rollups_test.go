// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestRollupsPreserveTypedStatistics(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := New(filepath.Join(dir, "talos.db"), filepath.Join(dir, "run"))
	if err := m.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	bucket := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	for i, value := range []float64{10, 30} {
		ts := bucket.Add(time.Duration(i) * 10 * time.Second).UnixMilli()
		if _, err := m.db.ExecContext(ctx, "INSERT INTO host_samples_10s(ts,host_id,memory_used_bytes,network_rx_bps) VALUES(?,'host',?,?)", ts, int64(value), value); err != nil {
			t.Fatal(err)
		}
		if _, err := m.db.ExecContext(ctx, "INSERT INTO resource_samples_10s(ts,resource_id,memory_working_set_bytes,block_read_bps,active_instance_count) VALUES(?,'res_test',?,?,1)", ts, int64(value), value); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.RollupOnce(ctx, bucket.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var min, avg, max float64
	var count int
	if err := m.db.QueryRowContext(ctx, "SELECT memory_min,memory_avg,memory_max,memory_count FROM host_rollups_1m WHERE ts=?", bucket.UnixMilli()).Scan(&min, &avg, &max, &count); err != nil {
		t.Fatal(err)
	}
	if min != 10 || avg != 20 || max != 30 || count != 2 {
		t.Fatalf("host min=%v avg=%v max=%v count=%d", min, avg, max, count)
	}
	if err := m.db.QueryRowContext(ctx, "SELECT block_read_min,block_read_avg,block_read_max,block_read_count FROM resource_rollups_1m WHERE resource_id='res_test' AND ts=?", bucket.UnixMilli()).Scan(&min, &avg, &max, &count); err != nil {
		t.Fatal(err)
	}
	if min != 10 || avg != 20 || max != 30 || count != 2 {
		t.Fatalf("resource min=%v avg=%v max=%v count=%d", min, avg, max, count)
	}
}
