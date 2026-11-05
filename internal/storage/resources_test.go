// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestArchivedResourcesRemainDiscoverableAndCanReactivate(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.UpsertHost(ctx, "host", HostIdentity("", "test"), "Server"); err != nil {
		t.Fatal(err)
	}
	resource := Resource{ID: "res_archived", HostID: "host", StableKey: "compose:project:web", SourceKind: "compose", Name: "Web", Context: "project/production", ProjectName: "Project", Category: "service", Status: "healthy"}
	if err := manager.UpsertResource(ctx, resource); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.db.ExecContext(ctx, "UPDATE resources SET last_seen_at=? WHERE id=?", time.Now().Add(-10*time.Minute).UnixMilli(), resource.ID); err != nil {
		t.Fatal(err)
	}
	if err := manager.ArchiveMissingResources(ctx, nil, time.Now().Add(-5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	archived, err := manager.ArchivedResources(ctx)
	if err != nil || len(archived) != 1 || archived[0].ProjectName != "Project" || archived[0].Context != "project/production" || archived[0].ArchivedAt == nil {
		t.Fatalf("archived=%+v err=%v", archived, err)
	}
	if err = manager.UpsertResource(ctx, resource); err != nil {
		t.Fatal(err)
	}
	value, err := manager.Resource(ctx, resource.ID)
	if err != nil || value.Status != "healthy" || value.ArchivedAt != nil {
		t.Fatalf("resource=%+v err=%v", value, err)
	}
}

// STO-4: one collector pass's host, resources and archiving are one write
// transaction, not one per row.
func TestStoreInventoryIsOneWriteTransaction(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	inventory := Inventory{HostID: "host", HostIdentity: HostIdentity("", "test"), HostName: "Server", ArchiveBefore: time.Now().Add(-5 * time.Minute)}
	for _, key := range []string{"web", "db", "worker"} {
		inventory.Resources = append(inventory.Resources, Resource{ID: "res_" + key, HostID: "host", StableKey: key, SourceKind: "compose", Name: key, Category: "service", Status: "healthy"})
	}
	before := WriteTransactions()
	if err := manager.StoreInventory(ctx, inventory); err != nil {
		t.Fatal(err)
	}
	if writes := WriteTransactions() - before; writes != 1 {
		t.Fatalf("StoreInventory committed %d write transactions, want 1", writes)
	}
	var n int
	if err := manager.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM resources WHERE status='healthy'").Scan(&n); err != nil || n != 3 {
		t.Fatalf("resources=%d err=%v", n, err)
	}
	// A resource no longer listed and unseen long enough is archived in the
	// same pass; the listed ones stay.
	if _, err := manager.db.ExecContext(ctx, "UPDATE resources SET last_seen_at=? WHERE id='res_worker'", time.Now().Add(-10*time.Minute).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	inventory.Resources = inventory.Resources[:2]
	if err := manager.StoreInventory(ctx, inventory); err != nil {
		t.Fatal(err)
	}
	archived, err := manager.ArchivedResources(ctx)
	if err != nil || len(archived) != 1 || archived[0].ID != "res_worker" {
		t.Fatalf("archived=%+v err=%v", archived, err)
	}
}
