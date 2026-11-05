// SPDX-License-Identifier: AGPL-3.0-only
package production

import (
	"context"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

type inventoryStore struct{ writes []storage.Inventory }

func (s *inventoryStore) StoreInventory(_ context.Context, inv storage.Inventory) error {
	s.writes = append(s.writes, inv)
	return nil
}

// STO-4: the host and resource identities are written when they change or
// once a refresh interval, not on every Docker tick.
func TestStoreResourcesWritesOnlyChangesOrOnceAMinute(t *testing.T) {
	ctx := context.Background()
	store := &inventoryStore{}
	s := &Sampler{Store: store}
	web := metrics.ResourceSnapshot{ID: "res_web", Name: "web", StableKey: "web", SourceKind: "compose", Category: "service", Status: metrics.StatusHealthy}
	start := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)

	s.storeResources(ctx, []metrics.ResourceSnapshot{web}, start)
	if len(store.writes) != 1 || len(store.writes[0].Resources) != 1 {
		t.Fatalf("first tick writes=%+v", store.writes)
	}
	if want := start.Add(-inventoryArchiveAfter); !store.writes[0].ArchiveBefore.Equal(want) || store.writes[0].HostID != "host" {
		t.Fatalf("inventory=%+v", store.writes[0])
	}
	// 29 unchanged ticks two seconds apart write nothing.
	for i := 1; i < 30; i++ {
		s.storeResources(ctx, []metrics.ResourceSnapshot{web}, start.Add(time.Duration(i)*2*time.Second))
	}
	if len(store.writes) != 1 {
		t.Fatalf("unchanged ticks wrote %d times, want 1", len(store.writes))
	}
	// A status change is written on the tick that saw it.
	down := web
	down.Status = metrics.StatusDown
	s.storeResources(ctx, []metrics.ResourceSnapshot{down}, start.Add(62*time.Second))
	if len(store.writes) != 2 || store.writes[1].Resources[0].Status != string(metrics.StatusDown) {
		t.Fatalf("status change not written: %+v", store.writes)
	}
	// A new resource is written immediately too.
	db := metrics.ResourceSnapshot{ID: "res_db", Name: "db", StableKey: "db", SourceKind: "compose", Category: "database", Status: metrics.StatusHealthy}
	s.storeResources(ctx, []metrics.ResourceSnapshot{down, db}, start.Add(64*time.Second))
	if len(store.writes) != 3 || len(store.writes[2].Resources) != 2 {
		t.Fatalf("new resource not written: %+v", store.writes)
	}
	// Unchanged again: refreshed once the interval since the last write passed.
	s.storeResources(ctx, []metrics.ResourceSnapshot{db, down}, start.Add(64*time.Second+inventoryRefreshInterval-time.Second))
	if len(store.writes) != 3 {
		t.Fatalf("refresh before the interval: %d writes", len(store.writes))
	}
	s.storeResources(ctx, []metrics.ResourceSnapshot{db, down}, start.Add(64*time.Second+inventoryRefreshInterval))
	if len(store.writes) != 4 {
		t.Fatalf("no refresh after the interval: %d writes", len(store.writes))
	}
	if inventoryRefreshInterval >= inventoryArchiveAfter {
		t.Fatal("refresh interval must stay below the archive age")
	}
}

type failingInventoryStore struct{ calls int }

func (s *failingInventoryStore) StoreInventory(context.Context, storage.Inventory) error {
	s.calls++
	return context.DeadlineExceeded
}

// A failed write is not remembered as stored, so the next tick retries it.
func TestStoreResourcesRetriesAfterAFailedWrite(t *testing.T) {
	store := &failingInventoryStore{}
	s := &Sampler{Store: store}
	web := metrics.ResourceSnapshot{ID: "res_web", Name: "web", StableKey: "web", Status: metrics.StatusHealthy}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	s.storeResources(context.Background(), []metrics.ResourceSnapshot{web}, now)
	s.storeResources(context.Background(), []metrics.ResourceSnapshot{web}, now.Add(2*time.Second))
	if store.calls != 2 {
		t.Fatalf("calls=%d, want a retry after the failure", store.calls)
	}
}
