// SPDX-License-Identifier: AGPL-3.0-only

package demo

import (
	"context"
	"fmt"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

type HistoryStore interface {
	HasMetricHistory(context.Context) (bool, error)
	WriteBatch(context.Context, metrics.PersistenceBatch) error
	// RebuildRollups recomputes every bucket the seeded raw rows back; the
	// incremental pass only covers recently closed buckets.
	RebuildRollups(context.Context, time.Time) error
}

// SeedHistory gives a fresh demo database enough deterministic data to exercise
// every history resolution without changing production databases.
func SeedHistory(ctx context.Context, store HistoryStore, generator *Generator, now time.Time) error {
	return seed(ctx, store, generator, now, 0)
}

// SeedRawHistory seeds a fresh demo database with raw 10 s samples for at
// least span, the load a server has after span of collection at the default
// interval; the benchmark uses it (RUN-9). The first sample is the whole hour
// at or before now-span, so a retention pass with a raw cutoff of span-1h
// deletes exactly one hour of raw history, the share an hourly pass deletes
// at steady state. Filesystems are sampled once a minute as in production.
func SeedRawHistory(ctx context.Context, store HistoryStore, generator *Generator, now time.Time, span time.Duration) error {
	if span < time.Hour {
		return fmt.Errorf("raw demo history must span at least an hour, got %s", span)
	}
	return seed(ctx, store, generator, now, span)
}

// seed writes the history; a zero raw span is the default shape (24 h at one
// minute, the latest 2 h at 10 s).
func seed(ctx context.Context, store HistoryStore, generator *Generator, now time.Time, raw time.Duration) error {
	hasHistory, err := store.HasMetricHistory(ctx)
	if err != nil || hasHistory {
		return err
	}
	now = now.UTC().Truncate(10 * time.Second)
	start := now.Add(-24 * time.Hour)
	if raw > 0 {
		start = now.Add(-raw).Truncate(time.Hour)
	}
	// Component samples reference resource rows, which production creates
	// through identity resolution. Demo mode creates them here instead.
	if backed, ok := store.(interface{ DB() *storage.DB }); ok {
		if err := seedResources(ctx, backed.DB(), generator, start, now); err != nil {
			return err
		}
	}
	step := uint64(0)
	for at := start; !at.After(now); {
		batch := metrics.PersistenceBatch{Snapshot: generator.SnapshotAt(step, at)}
		if raw == 0 || at.Truncate(time.Minute).Equal(at) {
			batch.Filesystems = generator.FilesystemsAt(step, at)
		}
		if err := store.WriteBatch(ctx, batch); err != nil {
			return err
		}
		step++
		interval := time.Minute
		if raw > 0 || !at.Before(now.Add(-2*time.Hour)) {
			interval = 10 * time.Second
		}
		at = at.Add(interval)
	}
	return store.RebuildRollups(ctx, now)
}

func seedResources(ctx context.Context, db *storage.DB, generator *Generator, start, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO hosts(id,identity_hash,name,updated_at)VALUES('host','host','Demo server',?)`, now.Format(time.RFC3339)); err != nil {
		return err
	}
	for _, resource := range generator.SnapshotAt(0, now).Resources {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO resources(id,host_id,stable_key,source_kind,name,project_name,environment_name,category,status,first_seen_at,last_seen_at)VALUES(?,'host',?,?,?,?,?,?,'healthy',?,?)`, string(resource.ID), resource.StableKey, resource.SourceKind, resource.Name, resource.Project, resource.Environment, resource.Category, start.UnixMilli(), now.UnixMilli()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
