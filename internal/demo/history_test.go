// SPDX-License-Identifier: AGPL-3.0-only

package demo

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
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
func (s *historyStore) RebuildRollups(_ context.Context, at time.Time) error {
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

// RUN-9: the benchmark's long run seeds raw 10 s samples for the whole span,
// starting at a whole hour so a span-1h raw cutoff deletes exactly one hour,
// with filesystems once a minute.
func TestSeedRawHistoryWritesTenSecondSamplesForTheSpan(t *testing.T) {
	now := time.Date(2026, 7, 13, 10, 25, 37, 0, time.UTC)
	store := &historyStore{}
	if err := SeedRawHistory(context.Background(), store, New(1, fixedClock{now}), now, 3*time.Hour); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 7, 13, 7, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 13, 10, 25, 30, 0, time.UTC)
	if want := int(end.Sub(start)/(10*time.Second)) + 1; len(store.batches) != want {
		t.Fatalf("batches=%d, want %d", len(store.batches), want)
	}
	filesystems := 0
	for i, batch := range store.batches {
		if at := batch.Snapshot.At; !at.Equal(start.Add(time.Duration(i) * 10 * time.Second)) {
			t.Fatalf("batch %d at %s", i, at)
		}
		if len(batch.Filesystems) > 0 {
			filesystems++
			if at := batch.Snapshot.At; at.Second() != 0 {
				t.Fatalf("filesystems sampled at %s", at)
			}
		}
	}
	if want := int(end.Sub(start)/time.Minute) + 1; filesystems != want {
		t.Fatalf("filesystem samples=%d, want %d", filesystems, want)
	}
	if !store.rolledAt.Equal(end) {
		t.Fatalf("rollup time=%s", store.rolledAt)
	}
	if err := SeedRawHistory(context.Background(), &historyStore{}, New(1, fixedClock{now}), now, time.Minute); err == nil {
		t.Fatal("a raw span under an hour was accepted")
	}
}

// RUN-9: the benchmark seeds 100 containers. Template and project repeat
// every 40 containers, and a repeated stable key made seedResources skip the
// resource, so its samples failed the foreign key.
func TestSeedRawHistoryStoresEveryContainerBeyondTheTemplateCycle(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := storage.New(filepath.Join(dir, "db.sqlite"), filepath.Join(dir, "runtime"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	generator := New(1, fixedClock{now: now})
	generator.Containers = 100
	if err := SeedRawHistory(ctx, store, generator, now, time.Hour); err != nil {
		t.Fatal(err)
	}
	var resources, sampled int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM resources`).Scan(&resources); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(DISTINCT resource_id) FROM resource_samples_10s`).Scan(&sampled); err != nil {
		t.Fatal(err)
	}
	if resources != 100 || sampled != 100 {
		t.Fatalf("resources=%d sampled=%d, want 100", resources, sampled)
	}
}
