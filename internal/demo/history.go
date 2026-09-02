// SPDX-License-Identifier: AGPL-3.0-only

package demo

import (
	"context"
	"database/sql"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

type HistoryStore interface {
	HasMetricHistory(context.Context) (bool, error)
	WriteBatch(context.Context, metrics.PersistenceBatch) error
	RollupOnce(context.Context, time.Time) error
}

// SeedHistory gives a fresh demo database enough deterministic data to exercise
// every history resolution without changing production databases.
func SeedHistory(ctx context.Context, store HistoryStore, generator *Generator, now time.Time) error {
	hasHistory, err := store.HasMetricHistory(ctx)
	if err != nil || hasHistory {
		return err
	}
	now = now.UTC().Truncate(10 * time.Second)
	// Component samples reference resource rows, which production creates
	// through identity resolution. Demo mode creates them here instead.
	if backed, ok := store.(interface{ DB() *sql.DB }); ok {
		if err := seedResources(ctx, backed.DB(), generator, now); err != nil {
			return err
		}
	}
	start := now.Add(-24 * time.Hour)
	step := uint64(0)
	for at := start; !at.After(now); {
		if err := store.WriteBatch(ctx, metrics.PersistenceBatch{Snapshot: generator.SnapshotAt(step, at), Filesystems: generator.FilesystemsAt(step, at)}); err != nil {
			return err
		}
		step++
		interval := time.Minute
		if at.After(now.Add(-2*time.Hour)) || at.Equal(now.Add(-2*time.Hour)) {
			interval = 10 * time.Second
		}
		at = at.Add(interval)
	}
	return store.RollupOnce(ctx, now)
}

func seedResources(ctx context.Context, db *sql.DB, generator *Generator, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO hosts(id,identity_hash,name,updated_at)VALUES('host','host','Demo server',?)`, now.Format(time.RFC3339)); err != nil {
		return err
	}
	for _, resource := range generator.SnapshotAt(0, now).Resources {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO resources(id,host_id,stable_key,source_kind,name,project_name,environment_name,category,status,first_seen_at,last_seen_at)VALUES(?,'host',?,?,?,?,?,?,'healthy',?,?)`, string(resource.ID), resource.StableKey, resource.SourceKind, resource.Name, resource.Project, resource.Environment, resource.Category, now.Add(-24*time.Hour).UnixMilli(), now.UnixMilli()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
