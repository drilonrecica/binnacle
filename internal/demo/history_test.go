// SPDX-License-Identifier: AGPL-3.0-only

package demo

import (
	"context"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

type historyStore struct {
	hasHistory bool
	batches    []metrics.PersistenceBatch
	rolledAt   time.Time
}

func (s *historyStore) HasMetricHistory(context.Context) (bool, error) {
	return s.hasHistory, nil
}
func (s *historyStore) WriteBatch(_ context.Context, batch metrics.PersistenceBatch) error {
	s.batches = append(s.batches, batch)
	return nil
}
func (s *historyStore) RollupOnce(_ context.Context, at time.Time) error {
	s.rolledAt = at
	return nil
}

func TestSeedHistoryPopulatesFreshDemoDatabase(t *testing.T) {
	now := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	store := &historyStore{}
	generator := New(1, fixedClock{now})
	if err := SeedHistory(context.Background(), store, generator, now); err != nil {
		t.Fatal(err)
	}
	if len(store.batches) < 2000 {
		t.Fatalf("seeded batches=%d", len(store.batches))
	}
	if got := store.batches[0].Snapshot.At; !got.Equal(now.Add(-24 * time.Hour)) {
		t.Fatalf("first sample=%s", got)
	}
	last := store.batches[len(store.batches)-1].Snapshot
	if !last.At.Equal(now) || last.Host.NetworkRXBPS == nil || len(last.Resources) == 0 || last.Resources[0].BlockReadBPS == nil {
		t.Fatalf("incomplete final sample=%+v", last)
	}
	if !store.rolledAt.Equal(now) {
		t.Fatalf("rollup time=%s", store.rolledAt)
	}
}

func TestSeedHistoryPreservesExistingDemoHistory(t *testing.T) {
	now := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	store := &historyStore{hasHistory: true}
	if err := SeedHistory(context.Background(), store, New(1, fixedClock{now}), now); err != nil {
		t.Fatal(err)
	}
	if len(store.batches) != 0 || !store.rolledAt.IsZero() {
		t.Fatal("existing history was modified")
	}
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }
