// SPDX-License-Identifier: AGPL-3.0-only
package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/metrics"
)

// COL-5: persistence stores a resource observation once. A stale resource
// (failed Docker tick) and one not observed since the previous stored batch
// (Docker tick still running) leave a gap instead of a flat line.
func TestWriteBatchSkipsStaleAndAlreadyStoredResources(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := New(filepath.Join(dir, "binnacle.db"), filepath.Join(dir, "run"))
	if err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := manager.UpsertHost(ctx, "host", "host-identity", "test-host"); err != nil {
		t.Fatal(err)
	}
	if err := manager.UpsertResource(ctx, Resource{ID: "res_web", HostID: "host", StableKey: "web", SourceKind: "compose", Name: "web", Category: "service", Status: "healthy"}); err != nil {
		t.Fatal(err)
	}
	cpu := 10.0
	base := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	write := func(seq metrics.Sequence, at, seen time.Time, stale bool) {
		t.Helper()
		resource := metrics.ResourceSnapshot{ID: "res_web", Name: "web", LastSeenAt: seen, Stale: stale, CPUHostPercent: &cpu,
			Components: []metrics.ResourceComponent{{ID: "c0ffee123456", Name: "web_1", CPUHostPercent: &cpu}}}
		snap := metrics.Snapshot{Sequence: seq, At: at, Host: metrics.HostObservation{At: at}, Resources: []metrics.ResourceSnapshot{resource}}
		if err := manager.WriteBatch(ctx, metrics.PersistenceBatch{Snapshot: snap}); err != nil {
			t.Fatal(err)
		}
	}
	rows := func(at time.Time) (resources, components, hosts int) {
		t.Helper()
		for table, dst := range map[string]*int{"resource_samples_10s": &resources, "container_instance_samples_10s": &components, "host_samples_10s": &hosts} {
			if err := manager.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE ts=?", at.UnixMilli()).Scan(dst); err != nil {
				t.Fatal(err)
			}
		}
		return
	}

	first := base
	write(1, first, first.Add(-time.Second), false)
	if r, c, h := rows(first); r != 1 || c != 1 || h != 1 {
		t.Fatalf("fresh batch rows resource=%d component=%d host=%d", r, c, h)
	}

	// The Docker tick has not finished since: same observation again.
	second := base.Add(10 * time.Second)
	write(2, second, first.Add(-time.Second), false)
	if r, c, h := rows(second); r != 0 || c != 0 || h != 1 {
		t.Fatalf("republished observation rows resource=%d component=%d host=%d, want host only", r, c, h)
	}

	// A failed Docker tick: stale, even with a newer LastSeenAt.
	third := base.Add(20 * time.Second)
	write(3, third, third, true)
	if r, c, h := rows(third); r != 0 || c != 0 || h != 1 {
		t.Fatalf("stale batch rows resource=%d component=%d host=%d, want host only", r, c, h)
	}

	fourth := base.Add(30 * time.Second)
	write(4, fourth, fourth.Add(-time.Second), false)
	if r, c, _ := rows(fourth); r != 1 || c != 1 {
		t.Fatalf("recovered batch rows resource=%d component=%d", r, c)
	}
}
