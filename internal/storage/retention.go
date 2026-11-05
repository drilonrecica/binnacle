// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"os"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

// SetRetention updates the automatic retention policy at runtime.
func (m *Manager) SetRetention(r RetentionCutoffs) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retention = r
}

// SetBudgetTarget updates the soft storage budget target.
func (m *Manager) SetBudgetTarget(bytes int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.budget.TargetBytes = bytes
}

// EmergencyPause reports whether raw persistence should pause because the
// database volume reached the emergency budget threshold.
func (m *Manager) EmergencyPause() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.emergencyPause
}

// Budget returns the latest evaluated budget state and used bytes.
func (m *Manager) Budget() DatabaseBudget {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.budget
}

func (m *Manager) runRetentionAndBudget(ctx context.Context) {
	// Evaluate budget frequently; run retention hourly.
	budgetTick := time.NewTicker(time.Minute)
	defer budgetTick.Stop()
	retentionTick := time.NewTicker(time.Hour)
	defer retentionTick.Stop()
	m.evaluateBudgetAndRetention(ctx, true)
	for {
		select {
		case <-ctx.Done():
			return
		case <-budgetTick.C:
			m.evaluateBudgetAndRetention(ctx, false)
		case <-retentionTick.C:
			m.evaluateBudgetAndRetention(ctx, true)
		}
	}
}

func (m *Manager) evaluateBudgetAndRetention(ctx context.Context, runRetention bool) {
	used, err := m.databaseSize()
	if err != nil {
		m.logger().Warn("database size measurement failed", "error", err)
		return
	}
	m.mu.Lock()
	target := m.budget.TargetBytes
	state := EvaluateBudget(used, target, 0.80, 0.95, 0.98)
	m.budget = DatabaseBudget{UsedBytes: used, TargetBytes: target, State: state}
	wasPaused := m.emergencyPause
	m.emergencyPause = state == BudgetEmergency
	retention := m.retention
	m.mu.Unlock()

	if wasPaused != m.emergencyPause {
		// Log state transitions outside the lock.
		event := metrics.Event{At: time.Now().UTC(), Type: "persistence_resumed", Severity: "info", Message: "Raw history persistence resumed; database volume below emergency threshold"}
		if m.emergencyPause {
			event = metrics.Event{At: time.Now().UTC(), Type: "persistence_emergency", Severity: "critical", Message: "Raw history persistence paused; database volume reached emergency threshold"}
		}
		if err := m.WriteEvent(ctx, event); err != nil && ctx.Err() == nil {
			m.logger().Error("persistence state event was not stored", "type", event.Type, "error", err)
		}
	}

	if !runRetention {
		return
	}
	// At critical or emergency, run retention more aggressively.
	aggressive := state == BudgetCritical || state == BudgetEmergency
	if err := m.applyRetention(ctx, time.Now(), retention, aggressive); err != nil && ctx.Err() == nil {
		m.logger().Error("retention pass failed", "error", err)
	}
}

func (m *Manager) databaseSize() (int64, error) {
	var total int64
	for _, name := range []string{m.Path, m.Path + "-wal", m.Path + "-shm"} {
		info, err := os.Stat(name)
		if err == nil {
			total += info.Size()
		}
	}
	return total, nil
}

// applyRetention deletes rows below each table's cutoff. Every cutoff is
// aligned down to a whole hour, and the raw sample cutoff never passes the
// final 1h buckets (ADR 021 §5), so a bucket is either complete or untouched.
func (m *Manager) applyRetention(ctx context.Context, now time.Time, r RetentionCutoffs, aggressive bool) error {
	started := time.Now()
	defer func() {
		m.mu.Lock()
		m.retentionDuration = time.Since(started)
		m.mu.Unlock()
	}()
	if r.Raw == 0 {
		return nil
	}
	now = now.UTC()
	rawCutoff, err := m.rawRetentionCutoff(ctx, now, r.Raw)
	if err != nil {
		return err
	}
	aligned := func(keep time.Duration) int64 {
		if keep <= 0 {
			return 0
		}
		return now.Add(-keep).Truncate(time.Hour).UnixMilli()
	}
	cutoffs := map[string]int64{
		"host_samples_10s":               rawCutoff,
		"resource_samples_10s":           rawCutoff,
		"container_instance_samples_10s": rawCutoff,
		"filesystem_samples_1m":          aligned(r.Raw),
		"network_interface_samples_1m":   aligned(r.Raw),
		"events":                         aligned(r.Raw),
		"collector_state_events":         aligned(r.Raw),
		"host_rollups_1m":                aligned(r.OneMinute),
		"resource_rollups_1m":            aligned(r.OneMinute),
		"host_rollups_15m":               aligned(r.FifteenMinute),
		"resource_rollups_15m":           aligned(r.FifteenMinute),
		"host_rollups_1h":                aligned(r.OneHour),
		"resource_rollups_1h":            aligned(r.OneHour),
	}
	limit := 500
	if aggressive {
		limit = 2000
	}
	for table, cutoff := range cutoffs {
		if cutoff <= 0 {
			continue
		}
		for {
			var n int64
			err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE ts<?", cutoff).Scan(&n)
			if err != nil || n == 0 {
				break
			}
			_, err = m.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE rowid IN (SELECT rowid FROM "+table+" WHERE ts<? LIMIT ?)", cutoff, limit)
			if err != nil {
				return err
			}
			if aggressive {
				// Yield briefly to avoid long write locks during aggressive cleanup.
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(10 * time.Millisecond):
				}
			}
		}
	}
	// Vacuum only when there is meaningful free space and not during aggressive runs,
	// to avoid blocking the database during an emergency.
	if !aggressive {
		if err := m.vacuumIfWorthwhile(ctx); err != nil && ctx.Err() == nil {
			m.logger().Warn("vacuum after retention failed", "error", err)
		}
	}
	return nil
}

// rawRetentionCutoff is C from ADR 021 §5: the raw cutoff aligned down to a
// whole hour and never above the 1h tier watermarks, so raw rows are deleted
// only below buckets that were computed from complete data. It persists C as
// raw_deleted_below (never decreasing) on the writer before any DELETE runs,
// which is what stops a rollup pass from recomputing a bucket retention is
// emptying. It returns 0 when nothing raw may be deleted yet, for example
// before the first rollup pass wrote a watermark.
func (m *Manager) rawRetentionCutoff(ctx context.Context, now time.Time, raw time.Duration) (int64, error) {
	cutoff := now.Add(-raw).Truncate(time.Hour).UnixMilli()
	tx, err := m.writer.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, table := range []string{"host_rollups_1h", "resource_rollups_1h"} {
		closedUntil, _, err := readWatermark(ctx, tx, table)
		if err != nil {
			return 0, err
		}
		if closedUntil < cutoff {
			cutoff = closedUntil
		}
	}
	if cutoff <= 0 {
		return 0, nil
	}
	existing, _, err := readWatermark(ctx, tx, rawDeletedBelowKey)
	if err != nil {
		return 0, err
	}
	if existing < cutoff {
		if err := writeWatermark(ctx, tx, rawDeletedBelowKey, cutoff, now); err != nil {
			return 0, err
		}
	}
	return cutoff, tx.Commit()
}

func (m *Manager) vacuumIfWorthwhile(ctx context.Context) error {
	var pages, freelist int
	if err := m.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		return err
	}
	if err := m.db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&freelist); err != nil {
		return err
	}
	if pages == 0 || float64(freelist)/float64(pages) < 0.20 {
		return nil
	}
	_, err := m.db.ExecContext(ctx, "VACUUM")
	return err
}
