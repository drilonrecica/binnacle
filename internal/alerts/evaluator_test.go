// SPDX-License-Identifier: AGPL-3.0-only
package alerts_test

import (
	"context"
	"testing"
	"time"

	"github.com/drilonrecica/binnacle/internal/alerts"
	"github.com/drilonrecica/binnacle/internal/metrics"
	"github.com/drilonrecica/binnacle/internal/storage"
)

func hostCPU(engine *metrics.Engine, percent float64) {
	engine.Publish(metrics.Snapshot{Sequence: 1, At: time.Now().UTC(), Host: metrics.HostObservation{CPUPercent: &percent}})
}

// STO-4: an evaluation whose phase did not change writes nothing. Healthy
// targets get no state row and a pending target is written only when it
// enters and leaves the phase.
func TestEvaluatorSkipsUnchangedPhases(t *testing.T) {
	ctx := context.Background()
	store, repo := healthStore(t)
	engine := metrics.NewEngine(4)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	evaluator := alerts.NewEvaluator(repo, engine)
	evaluator.Now = func() time.Time { return now }
	tick := func() {
		t.Helper()
		if err := evaluator.Evaluate(ctx); err != nil {
			t.Fatal(err)
		}
		now = now.Add(10 * time.Second)
	}
	hostCPU(engine, 20)
	before := storage.WriteTransactions()
	for range 5 {
		tick()
	}
	if writes := storage.WriteTransactions() - before; writes != 0 {
		t.Fatalf("five healthy evaluations wrote %d transactions", writes)
	}
	var rows int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM alert_evaluation_state`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("healthy targets stored %d state rows err=%v", rows, err)
	}

	hostCPU(engine, 95)
	before = storage.WriteTransactions()
	tick() // healthy -> pending
	if writes := storage.WriteTransactions() - before; writes != 1 {
		t.Fatalf("entering pending wrote %d transactions, want 1", writes)
	}
	var evaluated int64
	if err := store.DB().QueryRowContext(ctx, `SELECT last_evaluated_at FROM alert_evaluation_state WHERE dedup_key='host_cpu_warning:server'`).Scan(&evaluated); err != nil {
		t.Fatal(err)
	}
	before = storage.WriteTransactions()
	for range 4 { // still pending
		tick()
	}
	if writes := storage.WriteTransactions() - before; writes != 0 {
		t.Fatalf("four pending evaluations wrote %d transactions", writes)
	}
	var phase string
	var still int64
	if err := store.DB().QueryRowContext(ctx, `SELECT phase,last_evaluated_at FROM alert_evaluation_state WHERE dedup_key='host_cpu_warning:server'`).Scan(&phase, &still); err != nil || phase != "pending" || still != evaluated {
		t.Fatalf("phase=%s evaluated=%d want pending/%d err=%v", phase, still, evaluated, err)
	}

	hostCPU(engine, 20)
	tick() // pending -> healthy is a change and is written
	if err := store.DB().QueryRowContext(ctx, `SELECT phase FROM alert_evaluation_state WHERE dedup_key='host_cpu_warning:server'`).Scan(&phase); err != nil || phase != "healthy" {
		t.Fatalf("phase=%s err=%v", phase, err)
	}
}

// A deployment is correlated on every tick for ten minutes; its grace period
// is written once, and not recreated after it ended.
func TestEvaluatorWritesDeploymentGraceOnce(t *testing.T) {
	ctx := context.Background()
	store, repo := healthStore(t)
	engine := metrics.NewEngine(4)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	if _, err := store.DB().ExecContext(ctx, `INSERT INTO events(id,ts,host_id,resource_id,type,severity,summary,source,created_at)VALUES('evt_deploy',?,'host','res_test','deployment','info','deployed','coolify',?)`, now.UnixMilli(), now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	evaluator := alerts.NewEvaluator(repo, engine)
	evaluator.Now = func() time.Time { return now }
	if err := evaluator.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	var ends int64
	if err := store.DB().QueryRowContext(ctx, `SELECT ends_at FROM deployment_grace_periods WHERE resource_id='res_test'`).Scan(&ends); err != nil || ends != now.Add(5*time.Minute).Unix() {
		t.Fatalf("grace ends=%d err=%v", ends, err)
	}
	before := storage.WriteTransactions()
	for range 5 {
		now = now.Add(10 * time.Second)
		if err := evaluator.Evaluate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if writes := storage.WriteTransactions() - before; writes != 0 {
		t.Fatalf("re-correlating an unchanged deployment wrote %d transactions", writes)
	}
	// After the grace period ended and cleanup removed it, correlation within
	// the ten-minute window does not recreate it.
	now = now.Add(6 * time.Minute)
	if err := evaluator.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM deployment_grace_periods`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("ended grace period rows=%d err=%v", rows, err)
	}
}
