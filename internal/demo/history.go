// SPDX-License-Identifier: AGPL-3.0-only

package demo

import (
	"context"
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
	start := now.Add(-24 * time.Hour)
	step := uint64(0)
	for at := start; !at.After(now); {
		if err := store.WriteBatch(ctx, metrics.PersistenceBatch{Snapshot: generator.SnapshotAt(step, at)}); err != nil {
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
