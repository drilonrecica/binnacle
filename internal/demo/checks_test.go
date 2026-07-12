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
