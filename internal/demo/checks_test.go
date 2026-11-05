// SPDX-License-Identifier: AGPL-3.0-only
package demo

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/alerts"
	"github.com/drilonrecica/binnacle/internal/checks"
	"github.com/drilonrecica/binnacle/internal/storage"
)

func TestCheckRunnerIsDeterministicAndOutboundFree(t *testing.T) {
	runner := CheckRunner{}
	success := runner.Run(context.Background(), checks.Check{ID: "demo-check-001"})
	failure := runner.Run(context.Background(), checks.Check{ID: "demo-check-failure"})
	if success.Status != "success" || failure.FailureCode != checks.FailureUnexpectedStatus {
		t.Fatalf("success=%+v failure=%+v", success, failure)
	}
	if success.LatencyMS != failure.LatencyMS {
		t.Fatal("demo latency is not deterministic")
	}
}

func TestSeedChecksAlertsUsesExistingDemoResources(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := storage.New(filepath.Join(dir, "db.sqlite"), filepath.Join(dir, "runtime"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	generator := New(1, fixedClock{now: now})
	generator.Containers = 3
	if err := SeedHistory(ctx, store, generator, now); err != nil {
		t.Fatal(err)
	}
	repo := alerts.NewRepository(store.DB())
	if err := repo.SeedDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	if err := SeedChecksAlerts(ctx, store.DB(), 10, 3, now); err != nil {
		t.Fatal(err)
	}
	var checksCount, alertsCount int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM health_checks`).Scan(&checksCount); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM alerts WHERE status='firing'`).Scan(&alertsCount); err != nil {
		t.Fatal(err)
	}
	if checksCount != 10 || alertsCount < 1 {
		t.Fatalf("checks=%d alerts=%d", checksCount, alertsCount)
	}
}

// RUN-9: the benchmark runs the scheduler against the demo runner. A demo
// result must schedule the next run one interval from now, not from the
// epoch, or every check is due again on the next tick.
func TestDemoCheckResultSchedulesTheNextRunFromNow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := storage.New(filepath.Join(dir, "db.sqlite"), filepath.Join(dir, "runtime"))
	if err := store.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	if err := alerts.NewRepository(store.DB()).SeedDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	if err := SeedChecksAlerts(ctx, store.DB(), 3, 3, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	repo := checks.NewRepository(store.DB())
	due, err := repo.Due(ctx, now, 10)
	if err != nil || len(due) != 3 {
		t.Fatalf("due before run=%d err=%v", len(due), err)
	}
	for _, c := range due {
		if err := repo.SaveResult(ctx, c, CheckRunner{}.Run(ctx, c)); err != nil {
			t.Fatal(err)
		}
	}
	if due, err = repo.Due(ctx, now.Add(time.Second), 10); err != nil || len(due) != 0 {
		t.Fatalf("checks due again right after running: %d err=%v", len(due), err)
	}
}
