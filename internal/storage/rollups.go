// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"fmt"
	"time"
)

// RollupOnce materializes closed raw buckets. Replacing an existing bucket is
// idempotent and lets late samples repair the aggregate before retention runs.
func (m *Manager) RollupOnce(ctx context.Context, now time.Time) error {
	if m.db == nil {
		return fmt.Errorf("storage is not open")
	}
	for _, tier := range []struct {
		name string
		size time.Duration
	}{{"1m", time.Minute}, {"15m", 15 * time.Minute}, {"1h", time.Hour}} {
		bucket := tier.size.Milliseconds()
		cutoff := now.UTC().Truncate(tier.size).UnixMilli()
		host := fmt.Sprintf(`INSERT OR REPLACE INTO host_rollups_%s(ts,cpu_avg,cpu_min,cpu_max,sample_count,memory_avg,memory_min,memory_max,memory_count,network_rx_avg,network_rx_min,network_rx_max,network_rx_count,network_tx_avg,network_tx_min,network_tx_max,network_tx_count)
SELECT (ts/%d)*%d,AVG(cpu_busy_pct),MIN(cpu_busy_pct),MAX(cpu_busy_pct),COUNT(cpu_busy_pct),AVG(memory_used_bytes),MIN(memory_used_bytes),MAX(memory_used_bytes),COUNT(memory_used_bytes),AVG(network_rx_bps),MIN(network_rx_bps),MAX(network_rx_bps),COUNT(network_rx_bps),AVG(network_tx_bps),MIN(network_tx_bps),MAX(network_tx_bps),COUNT(network_tx_bps)
FROM host_samples_10s WHERE ts<? GROUP BY (ts/%d)`, tier.name, bucket, bucket, bucket)
		if _, err := m.db.ExecContext(ctx, host, cutoff); err != nil {
			return err
		}
		resource := fmt.Sprintf(`INSERT OR REPLACE INTO resource_rollups_%s(ts,resource_id,cpu_avg,cpu_min,cpu_max,sample_count,memory_avg,memory_min,memory_max,memory_count,network_rx_avg,network_rx_min,network_rx_max,network_rx_count,network_tx_avg,network_tx_min,network_tx_max,network_tx_count,block_read_avg,block_read_min,block_read_max,block_read_count,block_write_avg,block_write_min,block_write_max,block_write_count)
SELECT (ts/%d)*%d,resource_id,AVG(cpu_host_pct),MIN(cpu_host_pct),MAX(cpu_host_pct),COUNT(cpu_host_pct),AVG(memory_working_set_bytes),MIN(memory_working_set_bytes),MAX(memory_working_set_bytes),COUNT(memory_working_set_bytes),AVG(network_rx_bps),MIN(network_rx_bps),MAX(network_rx_bps),COUNT(network_rx_bps),AVG(network_tx_bps),MIN(network_tx_bps),MAX(network_tx_bps),COUNT(network_tx_bps),AVG(block_read_bps),MIN(block_read_bps),MAX(block_read_bps),COUNT(block_read_bps),AVG(block_write_bps),MIN(block_write_bps),MAX(block_write_bps),COUNT(block_write_bps)
FROM resource_samples_10s WHERE ts<? GROUP BY resource_id,(ts/%d)`, tier.name, bucket, bucket, bucket)
		if _, err := m.db.ExecContext(ctx, resource, cutoff); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) runRollups(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		_ = m.RollupOnce(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
