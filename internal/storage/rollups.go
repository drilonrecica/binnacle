// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Rollup bounds from ADR 021 §4 and §8. They are constants, not settings, so
// the bounds cannot be configured away.
const (
	// rollupLateness is how far behind closed_until a pass re-aggregates, so a
	// persistence batch that lands late still repairs its bucket.
	rollupLateness = 2 * time.Minute
	// rollupChunk bounds the raw span one write transaction aggregates; the
	// repair after the upgrade and any pass that fell behind proceed in these
	// steps and yield between them.
	rollupChunk = 6 * time.Hour
	// rawDeletedBelowKey is the rollup_watermarks row that holds the raw
	// retention cutoff (ADR 021 §5): no bucket below it is ever recomputed.
	rawDeletedBelowKey = "raw_deleted_below"
	// rollupYield is the pause between chunks so persistence batches
	// interleave with a long repair.
	rollupYield = 10 * time.Millisecond
)

type rollupTier struct {
	name string
	size time.Duration
}

var rollupTiers = []rollupTier{{"1m", time.Minute}, {"15m", 15 * time.Minute}, {"1h", time.Hour}}

var rollupScopes = []string{"host", "resource"}

// RollupOnce runs one incremental pass over every tier and scope. For each
// table it aggregates raw samples from max(closed_until - lateness,
// raw_deleted_below) up to the start of the currently open bucket, replacing
// those buckets, and persists the new closed_until in the same transaction.
// A table without a watermark gets the one-off repair of ADR 021 §8: every
// bucket from the first whole hour of raw data is recomputed in bounded
// chunks. A raw sample is the mean of its 10 s window, so the minimum and
// maximum come from the window ranges (COL-9); rows written before migration
// 024 have none and fall back to the sample value.
func (m *Manager) RollupOnce(ctx context.Context, now time.Time) error {
	if m.db == nil {
		return errors.New("storage is not open")
	}
	m.rollupMu.Lock()
	defer m.rollupMu.Unlock()
	return m.timedRollupPass(ctx, now.UTC())
}

// RebuildRollups discards the tier watermarks and runs the repair pass, so
// every bucket still backed by raw rows is recomputed. Demo seeding uses it
// after writing history older than the lateness window. raw_deleted_below is
// kept: buckets below it were never complete and must stay as they are.
func (m *Manager) RebuildRollups(ctx context.Context, now time.Time) error {
	if m.db == nil {
		return errors.New("storage is not open")
	}
	m.rollupMu.Lock()
	defer m.rollupMu.Unlock()
	if _, err := m.writer.ExecContext(ctx, "DELETE FROM rollup_watermarks WHERE table_name<>?", rawDeletedBelowKey); err != nil {
		return err
	}
	return m.timedRollupPass(ctx, now.UTC())
}

func (m *Manager) timedRollupPass(ctx context.Context, now time.Time) error {
	started := time.Now()
	err := m.rollupPass(ctx, now)
	m.mu.Lock()
	m.rollupDuration = time.Since(started)
	m.mu.Unlock()
	return err
}

func (m *Manager) rollupPass(ctx context.Context, now time.Time) error {
	for _, tier := range rollupTiers {
		for _, scope := range rollupScopes {
			if err := m.rollupTable(ctx, now, tier, scope); err != nil {
				return fmt.Errorf("rollup %s_rollups_%s: %w", scope, tier.name, err)
			}
		}
	}
	return nil
}

func (m *Manager) rollupTable(ctx context.Context, now time.Time, tier rollupTier, scope string) error {
	table := scope + "_rollups_" + tier.name
	upper := now.Truncate(tier.size).UnixMilli()
	for {
		done, err := m.rollupChunk(ctx, now, table, scope, tier, upper)
		if err != nil || done {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(rollupYield):
		}
	}
}

// rollupChunk aggregates at most rollupChunk of raw data for one table in one
// write transaction and advances closed_until. The watermarks are read inside
// the transaction: retention persists raw_deleted_below on the same writer
// before it deletes, so a pass can never aggregate a bucket retention is
// emptying. It reports done when closed_until reached upper.
func (m *Manager) rollupChunk(ctx context.Context, now time.Time, table, scope string, tier rollupTier, upper int64) (bool, error) {
	tx, err := m.writer.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	closedUntil, present, err := readWatermark(ctx, tx, table)
	if err != nil {
		return false, err
	}
	rawDeletedBelow, _, err := readWatermark(ctx, tx, rawDeletedBelowKey)
	if err != nil {
		return false, err
	}
	var lower int64
	if present {
		lower = truncateMillis(closedUntil-rollupLateness.Milliseconds(), tier.size)
	} else {
		// First pass after the upgrade (ADR 021 §8): start at the first whole
		// hour of raw data; the hour that contains the oldest row is already
		// partial and is left as it is.
		var hasRaw bool
		lower, hasRaw, err = repairStart(ctx, tx, upper)
		if err != nil {
			return false, err
		}
		if hasRaw && lower > rawDeletedBelow {
			rawDeletedBelow = lower
		}
		if err := writeWatermark(ctx, tx, rawDeletedBelowKey, rawDeletedBelow, now); err != nil {
			return false, err
		}
	}
	if lower < rawDeletedBelow {
		lower = rawDeletedBelow
	}
	to := upper
	if lower+rollupChunk.Milliseconds() < upper {
		to = lower + rollupChunk.Milliseconds()
	}
	if lower < to {
		if _, err := tx.ExecContext(ctx, rollupStatement(scope, tier), lower, to); err != nil {
			return false, err
		}
	}
	// closed_until never moves backwards, so a pass after a clock step back
	// recomputes nothing and keeps the watermark.
	if to > closedUntil || !present {
		if err := writeWatermark(ctx, tx, table, to, now); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return to >= upper, nil
}

// repairStart returns the first whole hour at or after the oldest raw sample
// (capped at upper), and whether any raw sample exists; without one the
// repair has nothing to recompute and raw_deleted_below must stay as it is.
func repairStart(ctx context.Context, tx *sql.Tx, upper int64) (int64, bool, error) {
	var oldest sql.NullInt64
	err := tx.QueryRowContext(ctx, "SELECT MIN(ts) FROM (SELECT MIN(ts) AS ts FROM host_samples_10s UNION ALL SELECT MIN(ts) FROM resource_samples_10s)").Scan(&oldest)
	if err != nil {
		return 0, false, err
	}
	if !oldest.Valid {
		return upper, false, nil
	}
	hour := time.Hour.Milliseconds()
	lower := ((oldest.Int64 + hour - 1) / hour) * hour
	if lower > upper {
		lower = upper
	}
	return lower, true, nil
}

func truncateMillis(value int64, size time.Duration) int64 {
	if value <= 0 {
		return 0
	}
	bucket := size.Milliseconds()
	return (value / bucket) * bucket
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func readWatermark(ctx context.Context, q rowQuerier, name string) (value int64, present bool, err error) {
	err = q.QueryRowContext(ctx, "SELECT closed_until FROM rollup_watermarks WHERE table_name=?", name).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return value, err == nil, err
}

func writeWatermark(ctx context.Context, tx *sql.Tx, name string, value int64, now time.Time) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO rollup_watermarks(table_name,closed_until,updated_at) VALUES(?,?,?) ON CONFLICT(table_name) DO UPDATE SET closed_until=excluded.closed_until,updated_at=excluded.updated_at", name, value, now.UnixMilli())
	return err
}

func rollupStatement(scope string, tier rollupTier) string {
	bucket := tier.size.Milliseconds()
	if scope == "host" {
		return fmt.Sprintf(`INSERT OR REPLACE INTO host_rollups_%s(
	ts,
	cpu_avg,cpu_min,cpu_max,sample_count,
	memory_avg,memory_min,memory_max,memory_count,
	network_rx_avg,network_rx_min,network_rx_max,network_rx_count,
	network_tx_avg,network_tx_min,network_tx_max,network_tx_count,
	cpu_user_avg,cpu_user_min,cpu_user_max,cpu_user_count,
	cpu_system_avg,cpu_system_min,cpu_system_max,cpu_system_count,
	cpu_iowait_avg,cpu_iowait_min,cpu_iowait_max,cpu_iowait_count,
	cpu_steal_avg,cpu_steal_min,cpu_steal_max,cpu_steal_count,
	load_1_avg,load_1_min,load_1_max,load_1_count,
	load_5_avg,load_5_min,load_5_max,load_5_count,
	load_15_avg,load_15_min,load_15_max,load_15_count,
	swap_used_avg,swap_used_min,swap_used_max,swap_used_count,
	disk_read_avg,disk_read_min,disk_read_max,disk_read_count,
	disk_write_avg,disk_write_min,disk_write_max,disk_write_count,
	disk_iops_avg,disk_iops_min,disk_iops_max,disk_iops_count
)
SELECT (ts/%d)*%d,
	AVG(cpu_busy_pct),MIN(COALESCE(cpu_busy_pct_min,cpu_busy_pct)),MAX(COALESCE(cpu_busy_pct_max,cpu_busy_pct)),COUNT(cpu_busy_pct),
	AVG(memory_used_bytes),MIN(COALESCE(memory_used_bytes_min,memory_used_bytes)),MAX(COALESCE(memory_used_bytes_max,memory_used_bytes)),COUNT(memory_used_bytes),
	AVG(network_rx_bps),MIN(COALESCE(network_rx_bps_min,network_rx_bps)),MAX(COALESCE(network_rx_bps_max,network_rx_bps)),COUNT(network_rx_bps),
	AVG(network_tx_bps),MIN(COALESCE(network_tx_bps_min,network_tx_bps)),MAX(COALESCE(network_tx_bps_max,network_tx_bps)),COUNT(network_tx_bps),
	AVG(cpu_user_pct),MIN(COALESCE(cpu_user_pct_min,cpu_user_pct)),MAX(COALESCE(cpu_user_pct_max,cpu_user_pct)),COUNT(cpu_user_pct),
	AVG(cpu_system_pct),MIN(COALESCE(cpu_system_pct_min,cpu_system_pct)),MAX(COALESCE(cpu_system_pct_max,cpu_system_pct)),COUNT(cpu_system_pct),
	AVG(cpu_iowait_pct),MIN(COALESCE(cpu_iowait_pct_min,cpu_iowait_pct)),MAX(COALESCE(cpu_iowait_pct_max,cpu_iowait_pct)),COUNT(cpu_iowait_pct),
	AVG(cpu_steal_pct),MIN(COALESCE(cpu_steal_pct_min,cpu_steal_pct)),MAX(COALESCE(cpu_steal_pct_max,cpu_steal_pct)),COUNT(cpu_steal_pct),
	AVG(load_1),MIN(COALESCE(load_1_min,load_1)),MAX(COALESCE(load_1_max,load_1)),COUNT(load_1),
	AVG(load_5),MIN(COALESCE(load_5_min,load_5)),MAX(COALESCE(load_5_max,load_5)),COUNT(load_5),
	AVG(load_15),MIN(COALESCE(load_15_min,load_15)),MAX(COALESCE(load_15_max,load_15)),COUNT(load_15),
	AVG(swap_used_bytes),MIN(COALESCE(swap_used_bytes_min,swap_used_bytes)),MAX(COALESCE(swap_used_bytes_max,swap_used_bytes)),COUNT(swap_used_bytes),
	AVG(disk_read_bps),MIN(COALESCE(disk_read_bps_min,disk_read_bps)),MAX(COALESCE(disk_read_bps_max,disk_read_bps)),COUNT(disk_read_bps),
	AVG(disk_write_bps),MIN(COALESCE(disk_write_bps_min,disk_write_bps)),MAX(COALESCE(disk_write_bps_max,disk_write_bps)),COUNT(disk_write_bps),
	AVG(disk_read_iops+disk_write_iops),MIN(COALESCE(disk_iops_min,disk_read_iops+disk_write_iops)),MAX(COALESCE(disk_iops_max,disk_read_iops+disk_write_iops)),COUNT(disk_read_iops+disk_write_iops)
FROM host_samples_10s WHERE ts>=? AND ts<? GROUP BY (ts/%d)`, tier.name, bucket, bucket, bucket)
	}
	return fmt.Sprintf(`INSERT OR REPLACE INTO resource_rollups_%s(ts,resource_id,cpu_avg,cpu_min,cpu_max,sample_count,memory_avg,memory_min,memory_max,memory_count,network_rx_avg,network_rx_min,network_rx_max,network_rx_count,network_tx_avg,network_tx_min,network_tx_max,network_tx_count,block_read_avg,block_read_min,block_read_max,block_read_count,block_write_avg,block_write_min,block_write_max,block_write_count)
SELECT (ts/%d)*%d,resource_id,AVG(cpu_host_pct),MIN(COALESCE(cpu_host_pct_min,cpu_host_pct)),MAX(COALESCE(cpu_host_pct_max,cpu_host_pct)),COUNT(cpu_host_pct),AVG(memory_working_set_bytes),MIN(COALESCE(memory_working_set_bytes_min,memory_working_set_bytes)),MAX(COALESCE(memory_working_set_bytes_max,memory_working_set_bytes)),COUNT(memory_working_set_bytes),AVG(network_rx_bps),MIN(COALESCE(network_rx_bps_min,network_rx_bps)),MAX(COALESCE(network_rx_bps_max,network_rx_bps)),COUNT(network_rx_bps),AVG(network_tx_bps),MIN(COALESCE(network_tx_bps_min,network_tx_bps)),MAX(COALESCE(network_tx_bps_max,network_tx_bps)),COUNT(network_tx_bps),AVG(block_read_bps),MIN(COALESCE(block_read_bps_min,block_read_bps)),MAX(COALESCE(block_read_bps_max,block_read_bps)),COUNT(block_read_bps),AVG(block_write_bps),MIN(COALESCE(block_write_bps_min,block_write_bps)),MAX(COALESCE(block_write_bps_max,block_write_bps)),COUNT(block_write_bps)
FROM resource_samples_10s WHERE ts>=? AND ts<? GROUP BY resource_id,(ts/%d)`, tier.name, bucket, bucket, bucket)
}

func (m *Manager) runRollups(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		if err := m.RollupOnce(ctx, time.Now()); err != nil && ctx.Err() == nil {
			m.logger().Error("rollup pass failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
