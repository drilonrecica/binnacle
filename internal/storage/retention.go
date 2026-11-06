// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Retention, budget and vacuum bounds from ADR 021 §5 and §7. They are
// constants so the bounds cannot be configured away.
const (
	// defaultRetentionBootDelay is how long after Start the first retention
	// pass runs: late enough not to compete with boot, early enough that a
	// process restarted hourly still runs retention (STO-6).
	defaultRetentionBootDelay = time.Minute
	// defaultRetentionPassBudget bounds one pass; an exhausted pass stops,
	// reports RetentionIncomplete and resumes on the next tick.
	defaultRetentionPassBudget = 30 * time.Second
	retentionBatch             = 500
	retentionBatchAggressive   = 2000
	retentionYield             = 10 * time.Millisecond
	// vacuumBatchPages is the incremental_vacuum step (about 4 MiB) and
	// vacuumMaxPagesPerPass the most one pass reclaims (about 100 MiB).
	vacuumBatchPages      = 1000
	vacuumMaxPagesPerPass = 25000
	// vacuumFreelistTarget stops the vacuum once the freelist is this share of
	// page_count.
	vacuumFreelistTarget = 0.10
	budgetWarningRatio   = 0.80
	budgetCriticalRatio  = 0.95
	budgetEmergencyRatio = 0.98
	// defaultArchivedResourceCap bounds archived resources when the 1h tier
	// is unlimited (ADR 021 §6); archivedPruneBatch bounds one pass.
	defaultArchivedResourceCap = 1000
	archivedPruneBatch         = 100
)

// errRetentionBudgetExhausted ends a pass that used its whole duration budget.
var errRetentionBudgetExhausted = errors.New("retention pass budget exhausted")

// SetRetention updates the automatic retention policy at runtime and wakes
// the retention worker so the new policy is applied at once (STO-6).
func (m *Manager) SetRetention(r RetentionCutoffs) {
	m.mu.Lock()
	m.retention = r
	wake := m.retentionWake
	m.mu.Unlock()
	if wake != nil {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

// SetBudgetTarget updates the soft storage budget target.
func (m *Manager) SetBudgetTarget(bytes int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.budget.TargetBytes = bytes
}

// EmergencyPause reports whether raw persistence should pause. It is set when
// the budget stays at emergency after retention was shortened and cleared
// once usage drops below the critical threshold (ADR 021 §7).
func (m *Manager) EmergencyPause() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.emergencyPause
}

// Budget returns the latest evaluated budget state.
func (m *Manager) Budget() DatabaseBudget {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.budget.copy()
}

func (b DatabaseBudget) copy() DatabaseBudget {
	b.Shortened = append([]string(nil), b.Shortened...)
	return b
}

func (m *Manager) runRetentionAndBudget(ctx context.Context) {
	// The budget is evaluated at boot and every minute; retention runs
	// shortly after boot, then hourly, and whenever SetRetention changes the
	// policy (ADR 021 §5). An emergency budget runs the shortening pass on
	// whichever tick observed it.
	budgetTick := time.NewTicker(time.Minute)
	defer budgetTick.Stop()
	retentionTick := time.NewTicker(time.Hour)
	defer retentionTick.Stop()
	m.mu.Lock()
	bootDelay, wake := m.retentionBootDelay, m.retentionWake
	m.mu.Unlock()
	if bootDelay <= 0 {
		bootDelay = defaultRetentionBootDelay
	}
	boot := time.NewTimer(bootDelay)
	defer boot.Stop()
	m.evaluateAt(ctx, time.Now(), false)
	for {
		select {
		case <-ctx.Done():
			return
		case <-budgetTick.C:
			m.evaluateAt(ctx, time.Now(), false)
		case <-boot.C:
			m.evaluateAt(ctx, time.Now(), true)
		case <-retentionTick.C:
			m.evaluateAt(ctx, time.Now(), true)
		case <-wake:
			m.evaluateAt(ctx, time.Now(), true)
		}
	}
}

// evaluateAt measures the budget, runs retention when asked or when the
// budget is at emergency, and updates the persistence pause.
func (m *Manager) evaluateAt(ctx context.Context, now time.Time, runRetention bool) {
	state, err := m.measureBudget(ctx)
	if err != nil {
		if ctx.Err() == nil {
			m.errorLog().Warn("database budget measurement failed", "error", err)
		}
		return
	}
	m.mu.Lock()
	retention := m.retention
	m.mu.Unlock()
	switch {
	case state == BudgetEmergency:
		state = m.shortenAtEmergency(ctx, now, retention)
	case runRetention:
		if err := m.applyRetention(ctx, now, retention, state == BudgetCritical); err != nil && ctx.Err() == nil {
			m.errorLog().Error("retention pass failed", "error", err)
		}
		m.setShortened(nil)
		if state, err = m.measureBudget(ctx); err != nil {
			if ctx.Err() == nil {
				m.errorLog().Warn("database budget measurement failed", "error", err)
			}
			return
		}
	}
	m.updatePause(ctx, now, state)
}

// shortenAtEmergency is the ladder of ADR 021 §7: raw retention is cut to
// half, then the 1m tier, each step followed by vacuum and a re-measure; the
// 15m and 1h tiers are never shortened. It returns the final budget state.
func (m *Manager) shortenAtEmergency(ctx context.Context, now time.Time, r RetentionCutoffs) BudgetState {
	state := BudgetEmergency
	if r.Raw <= 0 {
		// No policy yet (before settings load): nothing can be shortened.
		return state
	}
	m.mu.Lock()
	previous := strings.Join(m.budget.Shortened, ",")
	m.mu.Unlock()
	var shortened []string
	steps := []struct {
		tier  string
		apply func(*RetentionCutoffs)
	}{
		{"raw", func(c *RetentionCutoffs) { c.Raw /= 2 }},
		{"1m", func(c *RetentionCutoffs) { c.OneMinute /= 2 }},
	}
	effective := r
	for _, step := range steps {
		step.apply(&effective)
		shortened = append(shortened, step.tier)
		m.setShortened(shortened)
		if err := m.applyRetention(ctx, now, effective, true); err != nil && ctx.Err() == nil {
			m.errorLog().Error("emergency retention pass failed", "tier", step.tier, "error", err)
		}
		next, err := m.measureBudget(ctx)
		if err != nil {
			if ctx.Err() == nil {
				m.errorLog().Warn("database budget measurement failed", "error", err)
			}
			break
		}
		state = next
		if state != BudgetEmergency {
			break
		}
	}
	// One event per change of the shortened set, not one per minute of a
	// sustained emergency.
	if current := strings.Join(shortened, ","); current != previous {
		m.writeStorageEvent(ctx, now, metrics.EventRetentionShortened, "warning", "Database budget at emergency; retention shortened for the "+strings.Join(shortened, " and ")+" tier(s) to half the configured length")
	}
	return state
}

func (m *Manager) setShortened(tiers []string) {
	m.mu.Lock()
	m.budget.Shortened = append([]string(nil), tiers...)
	m.mu.Unlock()
}

// updatePause applies the hysteresis of ADR 021 §7: the pause is set at
// emergency and cleared only below critical, and each transition is an event.
func (m *Manager) updatePause(ctx context.Context, now time.Time, state BudgetState) {
	m.mu.Lock()
	was := m.emergencyPause
	switch state {
	case BudgetEmergency:
		m.emergencyPause = true
	case BudgetOK, BudgetWarning:
		m.emergencyPause = false
	}
	m.budget.Paused = m.emergencyPause
	paused := m.emergencyPause
	m.mu.Unlock()
	if was == paused {
		return
	}
	if paused {
		m.writeStorageEvent(ctx, now, metrics.EventPersistenceEmergency, "critical", "Raw history persistence paused; database budget still at emergency after shortening retention")
		return
	}
	m.writeStorageEvent(ctx, now, metrics.EventPersistenceResumed, "info", "Raw history persistence resumed; database usage below the critical threshold")
}

// writeStorageEvent records one of the storage worker's own events. Its id
// comes from the shared ULID generator, so it never collides with another
// storage event or an engine event (EVT-2, ADR 021 §9).
func (m *Manager) writeStorageEvent(ctx context.Context, now time.Time, eventType, severity, message string) {
	ms := now.UTC().UnixMilli()
	if _, err := m.db.ExecContext(ctx, "INSERT OR IGNORE INTO events(id,ts,host_id,type,severity,summary,source,created_at) VALUES(?,?,?,?,?,?,'binnacle',?)", metrics.NewEventID(), ms, hostID, eventType, severity, message, ms); err != nil && ctx.Err() == nil {
		m.errorLog().Error("persistence state event was not stored", "type", eventType, "error", err)
	}
}

// measureBudget evaluates (page_count - freelist_count) * page_size against
// the target and the free-disk floor, stores the result and returns the state.
func (m *Manager) measureBudget(ctx context.Context) (BudgetState, error) {
	used, err := m.liveBytes(ctx)
	if err != nil {
		return "", err
	}
	free, freeErr := m.diskFree(filepath.Dir(m.Path))
	if freeErr != nil && ctx.Err() == nil {
		m.errorLog().Warn("free disk space measurement failed", "error", freeErr)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state := EvaluateBudget(used, m.budget.TargetBytes, budgetWarningRatio, budgetCriticalRatio, budgetEmergencyRatio)
	if freeErr == nil && free < MinimumFreeSpace {
		state = BudgetEmergency
	}
	m.budget.UsedBytes, m.budget.State = used, state
	if freeErr == nil {
		m.budget.FreeBytes = int64(free)
	}
	return state, nil
}

// liveBytes is the budget measure of ADR 021 §7: live pages times page size,
// read on the reader pool. The WAL and shm files are excluded because they
// are bounded independently.
func (m *Manager) liveBytes(ctx context.Context) (int64, error) {
	var pages, freelist, pageSize int64
	for name, dst := range map[string]*int64{"page_count": &pages, "freelist_count": &freelist, "page_size": &pageSize} {
		if err := m.db.QueryRowContext(ctx, "PRAGMA "+name).Scan(dst); err != nil {
			return 0, fmt.Errorf("pragma %s: %w", name, err)
		}
	}
	live := pages - freelist
	if live < 0 {
		live = 0
	}
	return live * pageSize, nil
}

// retentionTable is one table retention prunes: rows whose column is below
// the cutoff, in the cutoff's unit.
type retentionTable struct {
	table, column string
	cutoff        int64
}

// applyRetention deletes rows below each table's cutoff in batches, yielding
// between batches, bounded by the pass budget, then reclaims space with an
// incremental vacuum and truncates the WAL. Every cutoff is aligned down to a
// whole hour, and the raw sample cutoff never passes the final 1h buckets
// (ADR 021 §5), so a bucket is either complete or untouched.
func (m *Manager) applyRetention(ctx context.Context, now time.Time, r RetentionCutoffs, aggressive bool) (err error) {
	started := time.Now()
	m.mu.Lock()
	budget := m.retentionPassBudget
	m.mu.Unlock()
	if budget <= 0 {
		budget = defaultRetentionPassBudget
	}
	deadline := started.Add(budget)
	incomplete := false
	defer func() {
		elapsed := time.Since(started)
		m.retentionLatency.Observe(elapsed)
		if err != nil && ctx.Err() == nil {
			m.retentionFailures.Inc()
		}
		m.mu.Lock()
		m.retentionDuration = elapsed
		m.retentionIncomplete = incomplete
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
	tables := []retentionTable{
		{"host_samples_10s", "ts", rawCutoff},
		{"resource_samples_10s", "ts", rawCutoff},
		{"container_instance_samples_10s", "ts", rawCutoff},
		{"filesystem_samples_1m", "ts", aligned(r.Raw)},
		{"network_interface_samples_1m", "ts", aligned(r.Raw)},
		{"events", "ts", aligned(r.Raw)},
		{"collector_state_events", "ts", aligned(r.Raw)},
		{"host_rollups_1m", "ts", aligned(r.OneMinute)},
		{"resource_rollups_1m", "ts", aligned(r.OneMinute)},
		{"host_rollups_15m", "ts", aligned(r.FifteenMinute)},
		{"resource_rollups_15m", "ts", aligned(r.FifteenMinute)},
		{"host_rollups_1h", "ts", aligned(r.OneHour)},
		{"resource_rollups_1h", "ts", aligned(r.OneHour)},
	}
	// ADR 021 §6: the tables that had no retention. alert_evaluation_state
	// stores seconds, every other table milliseconds.
	tables = append(tables,
		retentionTable{"coolify_deployments", "updated_at", aligned(r.Raw)},
		retentionTable{"alert_evaluation_state", "last_evaluated_at", aligned(r.Raw) / 1000},
	)
	limit := retentionBatch
	if aggressive {
		limit = retentionBatchAggressive
	}
	steps := make([]func() error, 0, len(tables)+2)
	for _, t := range tables {
		if t.cutoff <= 0 {
			continue
		}
		steps = append(steps, func() error {
			return m.deleteBatches(ctx, t.table, t.column+"<?", []any{t.cutoff}, limit, deadline)
		})
	}
	if cutoff := aligned(r.OneMinute); cutoff > 0 {
		// Finished instances only: the later of stopped_at and destroyed_at is
		// below the cutoff. Running instances (both NULL) are never deleted.
		steps = append(steps, func() error {
			return m.deleteBatches(ctx, "container_instances", "(stopped_at IS NOT NULL OR destroyed_at IS NOT NULL) AND COALESCE(stopped_at,0)<? AND COALESCE(destroyed_at,0)<?", []any{cutoff, cutoff}, limit, deadline)
		})
	}
	steps = append(steps, func() error { return m.pruneArchivedResources(ctx, now, aligned(r.OneHour), deadline) })
	for _, step := range steps {
		err := step()
		if errors.Is(err, errRetentionBudgetExhausted) {
			incomplete = true
			break
		}
		if err != nil {
			return err
		}
	}
	if err := m.incrementalVacuum(ctx, deadline); err != nil && ctx.Err() == nil {
		m.vacuumFailures.Inc()
		m.errorLog().Warn("incremental vacuum after retention failed", "error", err)
	}
	if err := m.truncateWAL(ctx); err != nil && ctx.Err() == nil {
		m.vacuumFailures.Inc()
		m.errorLog().Warn("WAL checkpoint after retention failed", "error", err)
	}
	return nil
}

// deleteBatches deletes the rows matching where in batches of limit, looping
// on RowsAffected rather than a COUNT per iteration (STO-7) and yielding
// between batches so persistence batches interleave. It returns
// errRetentionBudgetExhausted when the deadline passes before it is done.
func (m *Manager) deleteBatches(ctx context.Context, table, where string, args []any, limit int, deadline time.Time) error {
	query := deleteBatchSQL(table, where)
	for {
		if time.Now().After(deadline) {
			return errRetentionBudgetExhausted
		}
		result, err := m.db.ExecContext(ctx, query, append(append([]any(nil), args...), limit)...)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n < int64(limit) {
			return nil
		}
		if err := yield(ctx, retentionYield); err != nil {
			return err
		}
	}
}

// deleteBatchSQL deletes up to one batch (the last argument) of the rows of
// table matching where. A raw sample table's ts range is served by its ts
// index (STO-7).
func deleteBatchSQL(table, where string) string {
	return "DELETE FROM " + table + " WHERE rowid IN (SELECT rowid FROM " + table + " WHERE " + where + " LIMIT ?)"
}

// archivedUnreferenced is the predicate of ADR 021 §6 for an archived
// resource nothing points at any more: no check or incident, and no sample,
// rollup or event row. Container instances are the resource's own metadata
// and are deleted with it.
const archivedUnreferenced = `status='archived' AND archived_at IS NOT NULL
 AND NOT EXISTS(SELECT 1 FROM health_checks WHERE resource_id=r.id)
 AND NOT EXISTS(SELECT 1 FROM incidents WHERE target_id=r.id)
 AND NOT EXISTS(SELECT 1 FROM resource_samples_10s WHERE resource_id=r.id)
 AND NOT EXISTS(SELECT 1 FROM resource_rollups_1m WHERE resource_id=r.id)
 AND NOT EXISTS(SELECT 1 FROM resource_rollups_15m WHERE resource_id=r.id)
 AND NOT EXISTS(SELECT 1 FROM resource_rollups_1h WHERE resource_id=r.id)
 AND NOT EXISTS(SELECT 1 FROM events WHERE resource_id=r.id)`

// pruneArchivedResources applies ADR 021 §6 to archived resources. With a
// bounded 1h tier, resources archived before its cutoff and referenced by
// nothing are deleted directly, with their container instances. With an
// unlimited 1h tier a count cap applies instead: the oldest archived resource
// beyond the cap is handed to the existing deletion job, which removes its
// history as well, one job per pass because only one job runs at a time.
func (m *Manager) pruneArchivedResources(ctx context.Context, now time.Time, cutoff int64, deadline time.Time) error {
	if cutoff > 0 {
		rows, err := m.db.QueryContext(ctx, "SELECT id FROM resources r WHERE "+archivedUnreferenced+" AND archived_at<? ORDER BY archived_at,id LIMIT ?", cutoff, archivedPruneBatch)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range ids {
			if time.Now().After(deadline) {
				return errRetentionBudgetExhausted
			}
			if err := m.deleteArchivedResource(ctx, id); err != nil {
				return err
			}
			if err := yield(ctx, retentionYield); err != nil {
				return err
			}
		}
		return nil
	}
	m.mu.Lock()
	limit := m.archivedResourceCap
	m.mu.Unlock()
	if limit <= 0 {
		limit = defaultArchivedResourceCap
	}
	var archived int
	if err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM resources WHERE status='archived'").Scan(&archived); err != nil {
		return err
	}
	if archived <= limit {
		return nil
	}
	var id string
	err := m.db.QueryRowContext(ctx, "SELECT id FROM resources r WHERE status='archived' AND NOT EXISTS(SELECT 1 FROM health_checks WHERE resource_id=r.id) AND NOT EXISTS(SELECT 1 FROM incidents WHERE target_id=r.id) ORDER BY archived_at,id LIMIT 1").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return m.queueArchivedDeletion(ctx, id, now)
}

// deleteArchivedResource removes one archived resource and its container
// instances in one write transaction.
func (m *Manager) deleteArchivedResource(ctx context.Context, id string) error {
	tx, err := m.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM container_instances WHERE resource_id=?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM resources WHERE id=? AND status='archived'", id); err != nil {
		return err
	}
	return tx.Commit()
}

// queueArchivedDeletion queues the existing archived-resource deletion job
// for id on behalf of retention, unless a job is already active.
func (m *Manager) queueArchivedDeletion(ctx context.Context, id string, now time.Time) error {
	tx, err := m.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM history_deletion_jobs WHERE state IN ('queued','running','cancelling')").Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return nil
	}
	fence := now.UTC().UnixMilli()
	total, err := m.deletionCountTx(ctx, tx, DeleteArchived, id, 0, fence)
	if err != nil {
		return err
	}
	jobID, err := newDeletionID()
	if err != nil {
		return err
	}
	request := DeletionRequest{Kind: DeleteArchived, ResourceID: id}
	if _, err := tx.ExecContext(ctx, "INSERT INTO history_deletion_jobs(id,kind,resource_id,before_ts,fence_ts,confirmation,state,requested_by,requested_at,total_rows) VALUES(?,?,?,NULL,?,?,'queued','retention',?,?)", jobID, DeleteArchived, id, fence, confirmationFor(request), fence, total); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	m.logger().Info("archived resource beyond the cap handed to the deletion job", "resource", id, "job", jobID, "rows", total)
	return nil
}

func yield(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
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

// incrementalVacuum returns freelist pages to the filesystem in batches of
// vacuumBatchPages with a yield between them, until the freelist is under
// vacuumFreelistTarget of page_count, vacuumMaxPagesPerPass were reclaimed,
// or the deadline passed (ADR 021 §7). On a database that still has
// auto_vacuum=none (conversion pending) the pragma is a no-op.
func (m *Manager) incrementalVacuum(ctx context.Context, deadline time.Time) error {
	started := time.Now()
	defer func() {
		elapsed := time.Since(started)
		m.vacuumLatency.Observe(elapsed)
		m.mu.Lock()
		m.vacuumDuration = elapsed
		m.mu.Unlock()
	}()
	reclaimed := 0
	for reclaimed < vacuumMaxPagesPerPass {
		var pages, freelist float64
		if err := m.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
			return err
		}
		if err := m.db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&freelist); err != nil {
			return err
		}
		if pages == 0 || freelist/pages < vacuumFreelistTarget || freelist == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return nil
		}
		before := freelist
		// The pragma yields one row per step, so it has to be drained.
		rows, err := m.writer.QueryContext(ctx, fmt.Sprintf("PRAGMA incremental_vacuum(%d)", vacuumBatchPages))
		if err != nil {
			return err
		}
		for rows.Next() { //nolint:revive // drain: one row per vacuumed page
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if err := m.db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&freelist); err != nil {
			return err
		}
		if freelist >= before {
			// auto_vacuum is not incremental on this database; nothing to do.
			return nil
		}
		reclaimed += int(before - freelist)
		if err := yield(ctx, retentionYield); err != nil {
			return err
		}
	}
	return nil
}

// truncateWAL checkpoints and truncates the write-ahead log so its size after
// retention is bounded by the next batch, not by the pass.
func (m *Manager) truncateWAL(ctx context.Context) error {
	var busy, logFrames, checkpointed int
	if err := m.writer.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed); err != nil {
		return err
	}
	if busy != 0 {
		m.logger().Debug("WAL checkpoint could not complete; a reader holds a snapshot", "log_frames", logFrames, "checkpointed", checkpointed)
	}
	return nil
}

// diskFree is the free space available to Binnacle on the filesystem that
// holds path, overridable in tests.
func (m *Manager) diskFree(path string) (uint64, error) {
	if m.freeDisk != nil {
		return m.freeDisk(path)
	}
	return freeDiskBytes(path)
}

// convertAutoVacuum is the one-time conversion of ADR 021 §8: a database
// created before incremental auto-vacuum is VACUUMed once at start when the
// disk has room for the copy; otherwise the conversion is reported pending
// and retried at the next start. Callers hold m.mu.
func (m *Manager) convertAutoVacuum(ctx context.Context, writer execQueryer) error {
	var mode int
	if err := writer.QueryRowContext(ctx, "PRAGMA auto_vacuum").Scan(&mode); err != nil {
		return fmt.Errorf("read auto_vacuum: %w", err)
	}
	if mode == 2 {
		m.vacuumPending = false
		return nil
	}
	var size int64
	if info, err := os.Stat(m.Path); err == nil {
		size = info.Size()
	}
	free, err := m.diskFree(filepath.Dir(m.Path))
	if err != nil {
		return err
	}
	if free < uint64(size)+MinimumFreeSpace {
		m.vacuumPending = true
		m.logger().Warn("database is not using incremental auto-vacuum and the disk has no room for the one-time conversion; space is not reclaimed until it runs", "database_bytes", size, "free_bytes", free)
		return nil
	}
	started := time.Now()
	if _, err := writer.ExecContext(ctx, "PRAGMA auto_vacuum=INCREMENTAL"); err != nil {
		return fmt.Errorf("set auto_vacuum: %w", err)
	}
	if _, err := writer.ExecContext(ctx, "VACUUM"); err != nil {
		m.vacuumPending = true
		m.logger().Warn("one-time VACUUM to incremental auto-vacuum failed; it is retried at the next start", "error", err)
		return nil
	}
	m.vacuumPending = false
	m.logger().Info("database converted to incremental auto-vacuum", "duration", time.Since(started).String(), "database_bytes", size)
	return nil
}
